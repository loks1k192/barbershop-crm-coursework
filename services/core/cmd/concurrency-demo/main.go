// Демонстрация транзакций и конкурентного доступа (раздел 5 ТЗ).
//
//	TEST_PG_ADMIN_URL=postgres://postgres:postgres@localhost:5433/postgres go run ./cmd/concurrency-demo
//
// Создаёт одноразовую БД (миграции + контрольные данные), выполняет сценарии
// двумя независимыми подключениями A и B и печатает хронологию событий.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/apperr"
	"github.com/RomanKovalev007/barber_crm/services/core/internal/db"
	"github.com/RomanKovalev007/barber_crm/services/core/internal/testdb"
)

var (
	t0 time.Time
	mu sync.Mutex
)

func logf(conn, format string, a ...any) {
	mu.Lock()
	defer mu.Unlock()
	fmt.Printf("  %6.0f мс  [%s] %s\n", float64(time.Since(t0).Microseconds())/1000, conn, fmt.Sprintf(format, a...))
}

func header(title string) {
	fmt.Printf("\n══ %s ══\n", title)
	t0 = time.Now()
}

func describe(err error) string {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		e := apperr.FromDB(err)
		return fmt.Sprintf("SQLSTATE %s → HTTP %d %s: %s", pg.Code, e.Status, e.Code, pg.Message)
	}
	return err.Error()
}

func main() {
	if testdb.AdminURLFromEnv() == "" {
		fmt.Println("укажите TEST_PG_ADMIN_URL (суперпользователь PostgreSQL)")
		os.Exit(2)
	}
	ctx := context.Background()
	d, err := testdb.Create(ctx, 0, true)
	if err != nil {
		fmt.Println("подготовка БД:", err)
		os.Exit(1)
	}
	defer d.Close()
	fmt.Println("Одноразовая БД:", d.Name)

	atomicity(ctx, d)
	naiveDoubleBooking(ctx, d)
	excludeDoubleBooking(ctx, d)
	serializableDoubleBooking(ctx, d)
	naiveLostUpdate(ctx, d)
	lockedBonusRedeem(ctx, d)
}

func state(ctx context.Context, p *pgxpool.Pool, id int64) string {
	var s string
	_ = p.QueryRow(ctx, `SELECT concat('статус=', a.status, ', оплат=', (SELECT count(*) FROM crm.payments WHERE appointment_id = a.appointment_id),
		', бонус.операций=', (SELECT count(*) FROM crm.bonus_transactions WHERE appointment_id = a.appointment_id),
		', баланс=', la.bonus_balance, ', история=', (SELECT count(*) FROM crm.appointment_status_history WHERE appointment_id = a.appointment_id))
		FROM crm.appointments a JOIN crm.loyalty_accounts la ON la.client_id = a.client_id WHERE a.appointment_id = $1`, id).Scan(&s)
	return s
}

// 1. Атомарность: успешное выполнение и полный откат составной операции.
func atomicity(ctx context.Context, d *testdb.DB) {
	header("1. Атомарность complete_appointment: откат при ошибке и успешное выполнение")
	const id = 12 // визит клиента 3 (бонусов 500), 2500 ₽
	logf("—", "до операции:      %s", state(ctx, d.Admin, id))
	_, err := d.App.Exec(ctx, `SELECT * FROM crm.complete_appointment($1, 'card', 900)`, id)
	logf("A", "списать 900 бонусов → ошибка: %s", describe(err))
	logf("—", "после ошибки:     %s  (ничего не изменилось)", state(ctx, d.Admin, id))
	var total, accrued, balance float64
	err = d.App.QueryRow(ctx, `SELECT total_amount, bonus_accrued, bonus_balance FROM crm.complete_appointment($1, 'card', 300)`, id).
		Scan(&total, &accrued, &balance)
	if err != nil {
		logf("A", "ошибка: %s", describe(err))
		return
	}
	logf("A", "списать 300 бонусов → успех: чек %.0f, начислено %.0f, баланс %.0f", total, accrued, balance)
	logf("—", "после успеха:     %s", state(ctx, d.Admin, id))
}

// 2. Аномалия: наивная проверка пересечений + INSERT на READ COMMITTED.
func naiveDoubleBooking(ctx context.Context, d *testdb.DB) {
	header("2. Аномалия «двойное бронирование»: SELECT-проверка + INSERT без ограничения (READ COMMITTED)")
	_, _ = d.Admin.Exec(ctx, `DROP TABLE IF EXISTS public.naive_appointments;
		CREATE TABLE public.naive_appointments (id serial, barber_id int, starts_at timestamptz, ends_at timestamptz)`)
	book := func(conn, start string, delay time.Duration) {
		tx, _ := d.Admin.Begin(ctx)
		defer tx.Rollback(ctx)
		var busy bool
		_ = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM public.naive_appointments WHERE barber_id = 1
			AND starts_at < $1::timestamptz + interval '1 hour' AND ends_at > $1::timestamptz)`, start).Scan(&busy)
		logf(conn, "BEGIN; проверка пересечений для %s → занято=%v", start[11:16], busy)
		time.Sleep(delay)
		if busy {
			logf(conn, "ROLLBACK — время занято")
			return
		}
		_, _ = tx.Exec(ctx, `INSERT INTO public.naive_appointments (barber_id, starts_at, ends_at) VALUES (1, $1, $1::timestamptz + interval '1 hour')`, start)
		_ = tx.Commit(ctx)
		logf(conn, "INSERT; COMMIT — запись создана")
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); book("A", "2027-01-10 10:00+03", 300*time.Millisecond) }()
	time.Sleep(50 * time.Millisecond)
	go func() { defer wg.Done(); book("B", "2027-01-10 10:30+03", 300*time.Millisecond) }()
	wg.Wait()
	var n int
	_ = d.Admin.QueryRow(ctx, `SELECT count(*) FROM public.naive_appointments`).Scan(&n)
	logf("—", "ИТОГ: у мастера %d пересекающиеся записи (10:00–11:00 и 10:30–11:30) — инвариант нарушен", n)
	logf("—", "причина: проверяемых строк ещё нет, FOR UPDATE блокировать нечего (фантом); обе транзакции видят «свободно»")
}

// 3. Предотвращение: EXCLUDE USING gist — вторая транзакция ждёт первую и получает 23P01.
func excludeDoubleBooking(ctx context.Context, d *testdb.DB) {
	header("3. Решение: ограничение EXCLUDE (barber_id =, tstzrange &&) — конфликт и его обработка")
	day := time.Now().AddDate(0, 0, 2).Format("2006-01-02")
	run := func(conn, phone, hhmm string, hold time.Duration, done chan<- struct{}) {
		tx, _ := d.App.Begin(ctx)
		defer tx.Rollback(ctx)
		logf(conn, "BEGIN; book_appointment(мастер 3, %s %s, 45 мин)", day, hhmm)
		var id int64
		err := tx.QueryRow(ctx, `SELECT crm.book_appointment($1, 'Клиент '||$2, 3, ($3||' '||$4||'+03')::timestamptz, '{1}', 'phone')`,
			phone, conn, day, hhmm).Scan(&id)
		if err != nil {
			logf(conn, "ошибка: %s", describe(err))
			logf(conn, "ROLLBACK; клиенту — «время уже занято», предложить другие слоты")
			if done != nil {
				close(done)
			}
			return
		}
		logf(conn, "запись #%d вставлена (индекс GiST содержит интервал, транзакция открыта)", id)
		if done != nil {
			close(done)
		}
		time.Sleep(hold)
		_ = tx.Commit(ctx)
		logf(conn, "COMMIT")
	}
	aInserted := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); run("A", "+79161230001", "12:00", 400*time.Millisecond, aInserted) }()
	<-aInserted
	go func() {
		defer wg.Done()
		logf("B", "пытается записаться на пересекающееся время — ждёт исхода транзакции A…")
		run("B", "+79161230002", "12:30", 0, nil)
	}()
	wg.Wait()
	logf("—", "ИТОГ: одна запись; проверка ограничения учитывает незафиксированные строки других транзакций")
}

// 4. Альтернатива: SERIALIZABLE + повтор транзакции.
func serializableDoubleBooking(ctx context.Context, d *testdb.DB) {
	header("4. Альтернатива: наивная логика на SERIALIZABLE — ошибка сериализации 40001 и повтор")
	_, _ = d.Admin.Exec(ctx, `TRUNCATE public.naive_appointments`)
	attempt := map[string]int{}
	var amu sync.Mutex
	book := func(conn, start string) error {
		return db.InTx(ctx, d.Admin, pgx.Serializable, func(tx pgx.Tx) error {
			amu.Lock()
			attempt[conn]++
			n := attempt[conn]
			amu.Unlock()
			var busy bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM public.naive_appointments WHERE barber_id = 1
				AND starts_at < $1::timestamptz + interval '1 hour' AND ends_at > $1::timestamptz)`, start).Scan(&busy); err != nil {
				return err
			}
			logf(conn, "попытка %d: BEGIN ISOLATION LEVEL SERIALIZABLE; проверка → занято=%v", n, busy)
			if busy {
				return apperr.New(409, "time_slot_conflict", "время занято")
			}
			time.Sleep(200 * time.Millisecond)
			if _, err := tx.Exec(ctx, `INSERT INTO public.naive_appointments (barber_id, starts_at, ends_at)
				VALUES (1, $1, $1::timestamptz + interval '1 hour')`, start); err != nil {
				logf(conn, "INSERT → %s", describe(err))
				return err
			}
			logf(conn, "INSERT выполнен, COMMIT…")
			return nil
		})
	}
	var wg sync.WaitGroup
	for _, c := range []struct{ conn, start string }{{"A", "2027-01-11 10:00+03"}, {"B", "2027-01-11 10:30+03"}} {
		wg.Add(1)
		go func(conn, start string) {
			defer wg.Done()
			if err := book(conn, start); err != nil {
				if strings.Contains(err.Error(), "40001") {
					logf(conn, "COMMIT отклонён: %s", describe(err))
				} else {
					logf(conn, "итог: %v", err)
				}
				return
			}
			logf(conn, "итог: запись создана")
		}(c.conn, c.start)
		time.Sleep(30 * time.Millisecond)
	}
	wg.Wait()
	var n int
	_ = d.Admin.QueryRow(ctx, `SELECT count(*) FROM public.naive_appointments`).Scan(&n)
	logf("—", "ИТОГ: записей %d. SSI обнаружил зависимость чтение-запись; db.InTx повторил транзакцию, повтор увидел конфликт", n)
}

// 5. Аномалия «потерянное обновление» бонусного баланса при read-modify-write в приложении.
func naiveLostUpdate(ctx context.Context, d *testdb.DB) {
	header("5. Аномалия «потерянное обновление»: чтение баланса → вычисление в приложении → запись")
	_, _ = d.Admin.Exec(ctx, `DROP TABLE IF EXISTS public.naive_balance;
		CREATE TABLE public.naive_balance (client_id int PRIMARY KEY, balance numeric CHECK (balance >= 0));
		INSERT INTO public.naive_balance VALUES (1, 500)`)
	redeem := func(conn string, amount float64) {
		tx, _ := d.Admin.Begin(ctx)
		defer tx.Rollback(ctx)
		var bal float64
		_ = tx.QueryRow(ctx, `SELECT balance FROM public.naive_balance WHERE client_id = 1`).Scan(&bal)
		logf(conn, "прочитан баланс %.0f, списываем %.0f", bal, amount)
		time.Sleep(200 * time.Millisecond)
		if bal < amount {
			logf(conn, "недостаточно бонусов")
			return
		}
		_, _ = tx.Exec(ctx, `UPDATE public.naive_balance SET balance = $1 WHERE client_id = 1`, bal-amount)
		_ = tx.Commit(ctx)
		logf(conn, "записан баланс %.0f, COMMIT", bal-amount)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); redeem("A", 400) }()
	time.Sleep(30 * time.Millisecond)
	go func() { defer wg.Done(); redeem("B", 400) }()
	wg.Wait()
	var bal float64
	_ = d.Admin.QueryRow(ctx, `SELECT balance FROM public.naive_balance`).Scan(&bal)
	logf("—", "ИТОГ: списано 800 из 500, а баланс = %.0f — обновление A потеряно", bal)
}

// 6. Решение: блокировка бонусного счёта SELECT ... FOR UPDATE внутри complete_appointment.
func lockedBonusRedeem(ctx context.Context, d *testdb.DB) {
	header("6. Решение: SELECT … FOR UPDATE бонусного счёта в complete_appointment")
	var ids [2]int64
	tx, _ := d.Admin.Begin(ctx)
	_, _ = tx.Exec(ctx, `SET LOCAL crm.history_load = 'on';
		INSERT INTO crm.bonus_transactions (client_id, kind, amount) VALUES (5, 'adjustment', 500)`)
	for i, ts := range []string{"2026-03-06 12:00+03", "2026-03-06 14:00+03"} {
		_ = tx.QueryRow(ctx, `INSERT INTO crm.appointments (client_id, barber_id, starts_at, ends_at)
			VALUES (5, 3, $1, $1::timestamptz + interval '45 minutes') RETURNING appointment_id`, ts).Scan(&ids[i])
		_, _ = tx.Exec(ctx, `INSERT INTO crm.appointment_services VALUES ($1, 1, 2000, 45)`, ids[i])
	}
	_ = tx.Commit(ctx)
	logf("—", "клиент 5: баланс 500; два незакрытых визита по 2000 ₽")

	run := func(conn string, id int64, hold time.Duration, started chan<- struct{}) {
		tx, _ := d.App.Begin(ctx)
		defer tx.Rollback(ctx)
		logf(conn, "BEGIN; complete_appointment(визит %d, списать 400)", id)
		var bal float64
		err := tx.QueryRow(ctx, `SELECT bonus_balance FROM crm.complete_appointment($1, 'card', 400)`, id).Scan(&bal)
		if started != nil {
			close(started)
		}
		if err != nil {
			logf(conn, "ошибка: %s", describe(err))
			logf(conn, "ROLLBACK — визит не закрыт, клиенту предложено оплатить без бонусов")
			return
		}
		logf(conn, "выполнено, баланс в транзакции %.0f (строка счёта заблокирована)", bal)
		time.Sleep(hold)
		_ = tx.Commit(ctx)
		logf(conn, "COMMIT")
	}
	aDone := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); run("A", ids[0], 400*time.Millisecond, aDone) }()
	<-aDone
	go func() {
		defer wg.Done()
		logf("B", "ожидает блокировку бонусного счёта…")
		run("B", ids[1], 0, nil)
	}()
	wg.Wait()
	var bal, ledger float64
	_ = d.Admin.QueryRow(ctx, `SELECT bonus_balance, (SELECT sum(amount) FROM crm.bonus_transactions WHERE client_id = 5)
		FROM crm.loyalty_accounts WHERE client_id = 5`).Scan(&bal, &ledger)
	logf("—", "ИТОГ: баланс %.0f = сумма журнала %.0f; повторного списания нет", bal, ledger)
}
