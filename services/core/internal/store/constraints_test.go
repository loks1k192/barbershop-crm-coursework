package store_test

import (
	"net/http"
	"testing"
)

// Ограничения целостности и бизнес-правила, проверяемые СУБД.
// Операции выполняются ролью приложения: проверяется и то, что запрещённые
// напрямую действия (например, запись в payments) недоступны.
func TestIntegrityConstraints(t *testing.T) {
	app := tdb.App
	cases := []struct {
		name  string
		sql   string
		args  []any
		state string
	}{
		{"CHECK: телефон клиента", `INSERT INTO crm.clients (phone, first_name) VALUES ('89161234567', 'Иван')`, nil, "23514"},
		{"UNIQUE: телефон клиента", `INSERT INTO crm.clients (phone, first_name) VALUES ('+79160000001', 'Иван')`, nil, "23505"},
		{"UNIQUE: e-mail без учёта регистра",
			`WITH a AS (INSERT INTO crm.clients (phone, first_name, email) VALUES ('+79169990001', 'А', 'Same@Mail.ru') RETURNING 1)
			 INSERT INTO crm.clients (phone, first_name, email) SELECT '+79169990002', 'Б', 'same@mail.ru' FROM a`, nil, "23505"},
		{"CHECK: цена услуги > 0", `INSERT INTO crm.services (category_id, name, base_price, base_duration_min) VALUES (1, 'X', 0, 30)`, nil, "23514"},
		{"CHECK: длительность кратна 5", `INSERT INTO crm.services (category_id, name, base_price, base_duration_min) VALUES (1, 'X', 100, 33)`, nil, "23514"},
		{"FK: несуществующая категория", `INSERT INTO crm.services (category_id, name, base_price, base_duration_min) VALUES (999, 'X', 100, 30)`, nil, "23503"},
		{"FK RESTRICT: удаление клиента с историей", `DELETE FROM crm.clients WHERE client_id = 1`, nil, "23503"},
		{"FK RESTRICT: удаление филиала с мастерами", `DELETE FROM crm.branches WHERE branch_id = 1`, nil, "23503"},
		{"CHECK: оценка отзыва 1..5", `UPDATE crm.reviews SET rating = 6 WHERE appointment_id = 1`, nil, "23514"},
		{"CHECK: отмена без причины", `UPDATE crm.appointments SET status = 'cancelled' WHERE appointment_id = 11`, nil, "23514"},
		{"Триггер BR-5: из финального статуса нельзя выйти", `UPDATE crm.appointments SET status = 'scheduled' WHERE appointment_id = 1`, nil, "CR002"},
		{"Триггер BR-5: completed → no_show запрещён", `UPDATE crm.appointments SET status = 'no_show' WHERE appointment_id = 2`, nil, "CR002"},
		{"Триггер BR-4: смена вне часов филиала",
			`INSERT INTO crm.work_shifts (barber_id, branch_id, work_date, starts_at, ends_at) VALUES (1, 1, '2026-12-30', '07:00', '12:00')`, nil, "CR003"},
		{"Триггер BR-4: удаление смены с активной записью", `DELETE FROM crm.work_shifts WHERE barber_id = 3 AND work_date = '2026-05-10'`, nil, "CR004"},
		{"Триггер BR-4: сокращение смены с активной записью",
			`UPDATE crm.work_shifts SET ends_at = '11:00' WHERE barber_id = 3 AND work_date = '2026-05-10'`, nil, "CR004"},
		{"Триггер BR-9: отзыв о незавершённом визите", `INSERT INTO crm.reviews (appointment_id, rating) VALUES (12, 5)`, nil, "CR006"},
		{"Триггер BR-9: срок для отзыва истёк (визит в марте)", `INSERT INTO crm.reviews (appointment_id, rating) VALUES (2, 5)`, nil, "CR006"},
		{"Права: приложение не пишет в payments напрямую", `INSERT INTO crm.payments (appointment_id, amount, method) VALUES (12, 100, 'cash')`, nil, "42501"},
		{"Права: приложение не меняет бонусный баланс напрямую", `UPDATE crm.loyalty_accounts SET bonus_balance = 100000 WHERE client_id = 1`, nil, "42501"},
		{"Права: приложение не меняет время записи напрямую", `UPDATE crm.appointments SET starts_at = starts_at + interval '1 hour' WHERE appointment_id = 12`, nil, "42501"},
		{"Права: журнал аудита только для чтения", `DELETE FROM audit.audit_log`, nil, "42501"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := app.Exec(ctx(t), c.sql, c.args...)
			wantState(t, err, c.state)
		})
	}
}

// Ограничения, которые проверяются только «изнутри» (роль-владелец), — EXCLUDE и
// проверка смены для записей, вставленных в обход функции book_appointment.
func TestExclusionAndShiftRules(t *testing.T) {
	adm := tdb.Admin
	t.Run("EXCLUDE: пересечение записей мастера", func(t *testing.T) {
		_, err := adm.Exec(ctx(t), `INSERT INTO crm.appointments (client_id, barber_id, starts_at, ends_at)
			VALUES (2, 3, '2026-05-10 11:30+03', '2026-05-10 12:00+03')`)
		wantState(t, err, "23P01")
	})
	t.Run("EXCLUDE: стык интервалов допустим [start, end)", func(t *testing.T) {
		tx, err := adm.Begin(ctx(t))
		must(t, err)
		defer tx.Rollback(ctx(t))
		_, err = tx.Exec(ctx(t), `INSERT INTO crm.appointments (client_id, barber_id, starts_at, ends_at)
			VALUES (2, 3, '2026-05-10 11:45+03', '2026-05-10 12:30+03')`)
		must(t, err)
	})
	t.Run("EXCLUDE: пересечение записей клиента у разных мастеров", func(t *testing.T) {
		_, err := adm.Exec(ctx(t), `INSERT INTO crm.work_shifts (barber_id, branch_id, work_date, starts_at, ends_at)
			VALUES (1, 1, '2026-05-10', '10:00', '19:00') ON CONFLICT DO NOTHING`)
		must(t, err)
		_, err = adm.Exec(ctx(t), `INSERT INTO crm.appointments (client_id, barber_id, starts_at, ends_at)
			VALUES (5, 1, '2026-05-10 11:15+03', '2026-05-10 12:00+03')`)
		wantState(t, err, "23P01")
	})
	t.Run("Триггер BR-3: запись вне смены", func(t *testing.T) {
		_, err := adm.Exec(ctx(t), `INSERT INTO crm.appointments (client_id, barber_id, starts_at, ends_at)
			VALUES (2, 3, '2026-05-10 18:30+03', '2026-05-10 19:15+03')`)
		wantState(t, err, "CR003")
	})
	t.Run("Триггер BR-3: нет смены на дату", func(t *testing.T) {
		_, err := adm.Exec(ctx(t), `INSERT INTO crm.appointments (client_id, barber_id, starts_at, ends_at)
			VALUES (2, 2, '2026-05-11 11:00+03', '2026-05-11 12:00+03')`)
		wantState(t, err, "CR003")
	})
	t.Run("Триггер BR-5: новая запись не может быть completed", func(t *testing.T) {
		_, err := adm.Exec(ctx(t), `INSERT INTO crm.appointments (client_id, barber_id, starts_at, ends_at, status)
			VALUES (2, 3, '2026-05-10 14:00+03', '2026-05-10 15:00+03', 'completed')`)
		wantState(t, err, "CR002")
	})
	t.Run("Триггер BR-7: оплата незавершённого визита", func(t *testing.T) {
		_, err := adm.Exec(ctx(t), `INSERT INTO crm.payments (appointment_id, amount, method) VALUES (12, 100, 'cash')`)
		wantState(t, err, "CR005")
	})
	t.Run("Триггер BR-7: переплата", func(t *testing.T) {
		_, err := adm.Exec(ctx(t), `INSERT INTO crm.payments (appointment_id, amount, method) VALUES (1, 1, 'cash')`)
		wantState(t, err, "CR005")
	})
	t.Run("CHECK BR-8: отрицательный бонусный баланс", func(t *testing.T) {
		_, err := adm.Exec(ctx(t), `INSERT INTO crm.bonus_transactions (client_id, kind, amount) VALUES (4, 'redeem', -1)`)
		wantState(t, err, "23514")
	})
	t.Run("Маппинг ошибок в HTTP", func(t *testing.T) {
		_, err := adm.Exec(ctx(t), `INSERT INTO crm.appointments (client_id, barber_id, starts_at, ends_at)
			VALUES (2, 3, '2026-05-10 11:30+03', '2026-05-10 12:00+03')`)
		wantHTTP(t, err, http.StatusConflict)
		_, err = adm.Exec(ctx(t), `INSERT INTO crm.reviews (appointment_id, rating) VALUES (12, 5)`)
		wantHTTP(t, err, http.StatusUnprocessableEntity)
	})
}
