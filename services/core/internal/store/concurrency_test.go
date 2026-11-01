package store_test

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/apperr"
	"github.com/RomanKovalev007/barber_crm/services/core/internal/store"
)

// Аномалия «двойное бронирование»: наивная проверка «SELECT пересечений → INSERT»
// на READ COMMITTED пропускает обе конкурентные записи (фантом: проверяемых строк
// ещё нет, блокировать нечего). Воспроизводим на копии таблицы без EXCLUDE.
func TestDoubleBookingAnomalyNaive(t *testing.T) {
	c := ctx(t)
	_, err := tdb.Admin.Exec(c, `
		DROP TABLE IF EXISTS public.naive_appointments;
		CREATE TABLE public.naive_appointments (id serial, barber_id int, starts_at timestamptz, ends_at timestamptz);
		GRANT ALL ON public.naive_appointments, public.naive_appointments_id_seq TO crm_app;`)
	must(t, err)
	naive := func(start string) error {
		return pgx.BeginFunc(c, tdb.App, func(tx pgx.Tx) error {
			var busy bool
			if err := tx.QueryRow(c, `SELECT EXISTS (SELECT 1 FROM public.naive_appointments
				WHERE barber_id = 1 AND starts_at < $1::timestamptz + interval '1 hour' AND ends_at > $1::timestamptz)`,
				start).Scan(&busy); err != nil {
				return err
			}
			if busy {
				return apperr.New(http.StatusConflict, "busy", "занято")
			}
			_, _ = tx.Exec(c, `SELECT pg_sleep(0.3)`) // окно гонки
			_, err := tx.Exec(c, `INSERT INTO public.naive_appointments (barber_id, starts_at, ends_at)
				VALUES (1, $1, $1::timestamptz + interval '1 hour')`, start)
			return err
		})
	}
	var wg sync.WaitGroup
	for _, s := range []string{"2027-01-10 10:00+03", "2027-01-10 10:30+03"} {
		wg.Add(1)
		go func(s string) { defer wg.Done(); _ = naive(s) }(s)
	}
	wg.Wait()
	var n int
	must(t, tdb.Admin.QueryRow(c, `SELECT count(*) FROM public.naive_appointments`).Scan(&n))
	if n != 2 {
		t.Fatalf("ожидалось воспроизведение аномалии (2 пересекающиеся записи), получено %d", n)
	}
	t.Logf("аномалия воспроизведена: %d пересекающихся записи у одного мастера", n)
}

// Защита: EXCLUDE-ограничение. 10 параллельных попыток записаться на
// пересекающееся время — ровно одна успешна, остальные получают 409.
func TestConcurrentBookingSameSlot(t *testing.T) {
	c := ctx(t)
	const n = 10
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := st.Book(c, store.BookInput{
				ClientPhone: fmt.Sprintf("+791677700%02d", i),
				ClientName:  "Гонщик", BarberID: 3,
				StartsAt:    at(2, 10, 0).Add(time.Duration(i*4) * time.Minute), // 45 мин, старт в пределах 36 мин — попарно пересекаются
				ServiceIDs:  []int{1}, Source: "online"})
			results <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	ok, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			ok++
		case apperr.FromDB(err).Status == http.StatusConflict && sqlstate(err) == "23P01":
			conflicts++
		default:
			t.Errorf("неожиданная ошибка: %v", err)
		}
	}
	if ok != 1 || conflicts != n-1 {
		t.Fatalf("успешно %d, конфликтов %d — ожидалось 1 и %d", ok, conflicts, n-1)
	}
	var active int
	must(t, tdb.Admin.QueryRow(c, `SELECT count(*) FROM crm.appointments WHERE barber_id = 3
		AND starts_at::date = $1::date AND status <> 'cancelled'`, day(2)).Scan(&active))
	if active != 1 {
		t.Fatalf("в БД %d активных записей на это время", active)
	}
}

// Аномалия «потерянное обновление» бонусного баланса и её предотвращение:
// два параллельных завершения визитов одного клиента списывают бонусы.
// complete_appointment блокирует бонусный счёт (SELECT ... FOR UPDATE), поэтому
// вторая транзакция ждёт первую и видит уже уменьшенный баланс.
func TestConcurrentBonusRedeem(t *testing.T) {
	c := ctx(t)
	// клиент 1: начисляем 600 бонусов и создаём два незакрытых визита в прошлом по 2000 ₽
	var ids [2]int64
	tx, err := tdb.Admin.Begin(c)
	must(t, err)
	_, err = tx.Exec(c, `SET LOCAL crm.history_load = 'on';
		INSERT INTO crm.bonus_transactions (client_id, kind, amount) VALUES (1, 'adjustment', 600)`)
	must(t, err)
	for i, ts := range []string{"2026-03-06 12:00+03", "2026-03-06 14:00+03"} {
		must(t, tx.QueryRow(c, `INSERT INTO crm.appointments (client_id, barber_id, starts_at, ends_at)
			VALUES (1, 3, $1, $1::timestamptz + interval '45 minutes') RETURNING appointment_id`, ts).Scan(&ids[i]))
		_, err = tx.Exec(c, `INSERT INTO crm.appointment_services VALUES ($1, 1, 2000, 45)`, ids[i])
		must(t, err)
	}
	must(t, tx.Commit(c))
	var before float64
	must(t, tdb.Admin.QueryRow(c, `SELECT bonus_balance FROM crm.loyalty_accounts WHERE client_id = 1`).Scan(&before))

	var wg sync.WaitGroup
	errs := make([]error, 2)
	start := make(chan struct{})
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = st.Complete(context.Background(), ids[i], "card", 500) // каждый хочет списать 500
		}(i)
	}
	close(start)
	wg.Wait()

	okCnt := 0
	for _, e := range errs {
		if e == nil {
			okCnt++
		} else if sqlstate(e) != "CR009" {
			t.Fatalf("неожиданная ошибка: %v", e)
		}
	}
	if okCnt != 1 {
		t.Fatalf("успешных списаний: %d, ожидалось 1 (баланс %v)", okCnt, before)
	}
	// журнал и баланс согласованы: баланс = сумма операций
	var balance, ledger float64
	must(t, tdb.Admin.QueryRow(c, `SELECT la.bonus_balance, (SELECT sum(amount) FROM crm.bonus_transactions b WHERE b.client_id = 1)
		FROM crm.loyalty_accounts la WHERE la.client_id = 1`).Scan(&balance, &ledger))
	if balance != ledger || balance < 0 {
		t.Fatalf("баланс %v не совпадает с журналом %v", balance, ledger)
	}
}
