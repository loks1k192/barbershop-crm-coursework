package store_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/db"
	"github.com/RomanKovalev007/barber_crm/services/core/internal/store"
)

// Сценарий «запись клиента»: свободные слоты → запись → конфликт → отмена.
func TestBookingScenario(t *testing.T) {
	c := ctx(t)
	slots, err := st.FreeSlots(c, 1, day(1), []int{1, 2}, true)
	must(t, err)
	if len(slots) == 0 || slots[0].End.Sub(slots[0].Start).Minutes() != 75 {
		t.Fatalf("слоты: %+v", slots)
	}
	ap, err := st.Book(c, store.BookInput{ClientPhone: "+79165550000", ClientName: "Петров Пётр Петрович",
		BarberID: 1, StartsAt: at(1, 10, 0), ServiceIDs: []int{1, 2}, Source: "online"})
	must(t, err)
	if ap.Status != "scheduled" || ap.TotalPrice != 2500 || ap.TotalDurationMin != 75 || ap.BranchName != "Филиал А" {
		t.Fatalf("запись: %+v", ap)
	}
	// новый клиент создан и имя разобрано на части
	if fn := scalar[string](t, `SELECT first_name FROM crm.clients WHERE phone = '+79165550000'`); fn != "Пётр" {
		t.Fatalf("имя клиента: %s", fn)
	}
	// занятый интервал исчез из свободных слотов
	slots2, err := st.FreeSlots(c, 1, day(1), []int{1}, true)
	must(t, err)
	for _, s := range slots2 {
		if s.Start.Before(ap.EndsAt) && s.End.After(ap.StartsAt) {
			t.Fatalf("слот %v пересекается с записью", s)
		}
	}
	// тот же мастер, пересекающееся время → 409
	_, err = st.Book(c, store.BookInput{ClientPhone: "+79165550001", ClientName: "Иван", BarberID: 1,
		StartsAt: at(1, 10, 30), ServiceIDs: []int{1}, Source: "phone"})
	wantHTTP(t, err, http.StatusConflict)
	// услуга не из прайса мастера → 422, клиент не создан (атомарность)
	_, err = st.Book(c, store.BookInput{ClientPhone: "+79165550002", ClientName: "Иван", BarberID: 2,
		StartsAt: at(1, 15, 0), ServiceIDs: []int{3}, Source: "phone"})
	wantHTTP(t, err, http.StatusUnprocessableEntity)
	if n := scalar[int](t, `SELECT count(*) FROM crm.clients WHERE phone = '+79165550002'`); n != 0 {
		t.Fatal("клиент создан, хотя запись отклонена")
	}
	// перенос и отмена
	ap2, err := st.Reschedule(c, ap.ID, at(1, 14, 0))
	must(t, err)
	if !ap2.EndsAt.Equal(at(1, 15, 15)) {
		t.Fatalf("перенос: %v", ap2.EndsAt)
	}
	ap3, err := st.Cancel(c, ap.ID, "Клиент заболел")
	must(t, err)
	if ap3.Status != "cancelled" || *ap3.CancelReason != "Клиент заболел" {
		t.Fatalf("отмена: %+v", ap3)
	}
	_, err = st.Cancel(c, ap.ID, "Ещё раз")
	wantHTTP(t, err, http.StatusConflict)
	hist, err := st.AppointmentHistory(c, ap.ID)
	must(t, err)
	if len(hist) != 2 || hist[1].NewStatus != "cancelled" {
		t.Fatalf("история: %+v", hist)
	}
}

// Сценарий «завершение визита»: изменения в 5 таблицах выполняются атомарно.
func TestCompleteVisitScenario(t *testing.T) {
	c := ctx(t)
	// визит 12: клиент 3 (бонусов 500), мастер 1, услуги 1+2 = 2500 ₽, визит в прошлом
	res, err := st.Complete(c, 12, "card", 300)
	must(t, err)
	if res.TotalAmount != 2500 || res.BonusRedeemed != 300 || res.PaidMoney != 2200 {
		t.Fatalf("результат: %+v", res)
	}
	// начисление 3 % (bronze) с суммы без бонусов: round(2200*0.03) = 66
	if res.BonusAccrued != 66 || res.BonusBalance != 500-300+66 {
		t.Fatalf("бонусы: %+v", res)
	}
	if got := scalar[string](t, `SELECT string_agg(method::text || ':' || amount::int, ',' ORDER BY method)
		FROM crm.payments WHERE appointment_id = 12`); got != "card:2200,bonus:300" {
		t.Fatalf("оплаты: %s", got)
	}
	// исторические визиты фикстуры загружены напрямую, поэтому до операции total_spent = 0
	if got := scalar[float64](t, `SELECT total_spent FROM crm.loyalty_accounts WHERE client_id = 3`); got != 2200 {
		t.Fatalf("сумма покупок: %v", got)
	}
	if got := scalar[string](t, `SELECT new_status::text FROM crm.appointment_status_history
		WHERE appointment_id = 12 ORDER BY history_id DESC LIMIT 1`); got != "completed" {
		t.Fatalf("история: %s", got)
	}
	// повторное завершение → 409
	_, err = st.Complete(c, 12, "cash", 0)
	wantHTTP(t, err, http.StatusConflict)
	// визит был в марте — срок для отзыва (30 дней) истёк
	_, err = st.AddReview(c, 12, 5, ptr("Поздний отзыв"))
	wantState(t, err, "CR006")

	// вчерашний визит: завершение → отзыв → повторный отзыв запрещён
	tx, err := tdb.Admin.Begin(c)
	must(t, err)
	var id int64
	_, err = tx.Exec(c, `SET LOCAL crm.history_load = 'on'`)
	must(t, err)
	must(t, tx.QueryRow(c, `INSERT INTO crm.appointments (client_id, barber_id, starts_at, ends_at)
		VALUES (2, 1, $1, $1::timestamptz + interval '45 minutes') RETURNING appointment_id`, at(-1, 12, 0)).Scan(&id))
	_, err = tx.Exec(c, `INSERT INTO crm.appointment_services VALUES ($1, 1, 1500, 45)`, id)
	must(t, err)
	must(t, tx.Commit(c))
	_, err = st.AddReview(c, id, 5, nil)
	wantState(t, err, "CR006") // ещё не завершён
	_, err = st.Complete(c, id, "online", 0)
	must(t, err)
	rv, err := st.AddReview(c, id, 5, ptr("Отлично"))
	must(t, err)
	if rv.Rating != 5 {
		t.Fatal("отзыв")
	}
	_, err = st.AddReview(c, id, 4, nil)
	wantHTTP(t, err, http.StatusConflict) // PK reviews: один отзыв на визит
}

// Полный откат составной операции при ошибке.
func TestAtomicRollback(t *testing.T) {
	c := ctx(t)
	// визит в прошлом для клиента 2 (бонусов нет)
	tx, err := tdb.Admin.Begin(c)
	must(t, err)
	var id int64
	_, err = tx.Exec(c, `SET LOCAL crm.history_load = 'on'`)
	must(t, err)
	must(t, tx.QueryRow(c, `INSERT INTO crm.appointments (client_id, barber_id, starts_at, ends_at)
		VALUES (2, 2, '2026-03-06 15:00+03', '2026-03-06 16:00+03') RETURNING appointment_id`).Scan(&id))
	_, err = tx.Exec(c, `INSERT INTO crm.appointment_services VALUES ($1, 1, 1000, 60)`, id)
	must(t, err)
	must(t, tx.Commit(c))

	snapshot := func() string {
		return scalar[string](t, `SELECT concat_ws('|',
			(SELECT status::text FROM crm.appointments WHERE appointment_id = $1),
			(SELECT count(*) FROM crm.payments WHERE appointment_id = $1),
			(SELECT count(*) FROM crm.bonus_transactions WHERE client_id = 2),
			(SELECT bonus_balance || '/' || total_spent FROM crm.loyalty_accounts WHERE client_id = 2),
			(SELECT count(*) FROM crm.appointment_status_history WHERE appointment_id = $1))`, id)
	}
	before := snapshot()

	t.Run("ошибка внутри функции БД", func(t *testing.T) {
		_, err := st.Complete(c, id, "cash", 100) // бонусов 0 → CR009
		wantState(t, err, "CR009")
		if after := snapshot(); after != before {
			t.Fatalf("частичные изменения:\n до:    %s\n после: %s", before, after)
		}
	})

	t.Run("ошибка после успешной бизнес-функции в той же транзакции", func(t *testing.T) {
		err := db.InTx(c, tdb.App, pgx.ReadCommitted, func(tx pgx.Tx) error {
			if _, err := tx.Exec(c, `SELECT * FROM crm.complete_appointment($1, 'cash', 0)`, id); err != nil {
				t.Fatalf("complete: %v", err)
			}
			// визит уже завершён, оплаты и бонусы записаны — но следующий шаг
			// (ошибочная отметка неявки) нарушает правило переходов статусов
			_, err := tx.Exec(c, `UPDATE crm.appointments SET status = 'no_show' WHERE appointment_id = $1`, id)
			return err
		})
		wantState(t, err, "CR002")
		if after := snapshot(); after != before {
			t.Fatalf("транзакция откатилась не полностью:\n до:    %s\n после: %s", before, after)
		}
	})

	t.Run("успешное выполнение", func(t *testing.T) {
		_, err := st.Complete(c, id, "cash", 0)
		must(t, err)
		after := snapshot()
		if !strings.HasPrefix(after, "completed|1|") || after == before {
			t.Fatalf("после завершения: %s (до: %s)", after, before)
		}
		if n := scalar[int](t, `SELECT count(*) FROM crm.bonus_transactions WHERE appointment_id = $1 AND kind = 'accrual'`, id); n != 1 {
			t.Fatalf("начисление бонусов: %d", n)
		}
	})
}
