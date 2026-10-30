package store

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/apperr"
	"github.com/RomanKovalev007/barber_crm/services/core/internal/model"
)

type AppointmentFilter struct {
	BarberID *int
	ClientID *int64
	BranchID *int
	Status   []string
	From     *time.Time
	To       *time.Time
}

const appointmentCols = `appointment_id, starts_at, ends_at, status::text AS status, source::text AS source,
	client_id, client_name, client_phone, barber_id, barber_name, branch_id, branch_name, services,
	total_price, total_duration_min, paid_amount, rating, comment, cancel_reason, created_at, updated_at`

func (s *Store) ListAppointments(ctx context.Context, af AppointmentFilter, p ListParams) (model.Page[model.Appointment], error) {
	var f Filter
	if af.BarberID != nil {
		f.Add("barber_id = ?", *af.BarberID)
	}
	if af.ClientID != nil {
		f.Add("client_id = ?", *af.ClientID)
	}
	if af.BranchID != nil {
		f.Add("branch_id = ?", *af.BranchID)
	}
	if len(af.Status) > 0 {
		f.Add("status::text = ANY (?)", af.Status)
	}
	if af.From != nil {
		f.Add("starts_at >= ?", *af.From)
	}
	if af.To != nil {
		f.Add("starts_at < ?", *af.To)
	}
	return list[model.Appointment](ctx, s.Pool, appointmentCols, "crm.v_appointment_details", f, p)
}

func (s *Store) GetAppointment(ctx context.Context, id int64) (*model.Appointment, error) {
	return getOne[model.Appointment](ctx, s.Pool,
		"SELECT "+appointmentCols+" FROM crm.v_appointment_details WHERE appointment_id = $1", id)
}

func (s *Store) AppointmentServices(ctx context.Context, id int64) ([]model.AppointmentService, error) {
	return getAll[model.AppointmentService](ctx, s.Pool, `
		SELECT x.service_id, s.name, x.price, x.duration_min
		FROM crm.appointment_services x JOIN crm.services s ON s.service_id = x.service_id
		WHERE x.appointment_id = $1 ORDER BY s.name`, id)
}

func (s *Store) AppointmentHistory(ctx context.Context, id int64) ([]model.StatusChange, error) {
	return getAll[model.StatusChange](ctx, s.Pool, `
		SELECT history_id, old_status::text AS old_status, new_status::text AS new_status, changed_at, changed_by
		FROM crm.appointment_status_history WHERE appointment_id = $1 ORDER BY changed_at, history_id`, id)
}

// FreeSlots — свободные слоты мастера на дату под суммарную длительность услуг.
// Все услуги должны быть в активном прайсе мастера (иначе длительность неизвестна).
func (s *Store) FreeSlots(ctx context.Context, barberID int, date string, serviceIDs []int, respectLead bool) ([]model.Slot, error) {
	var cnt, dur int
	if err := s.Pool.QueryRow(ctx, `
		SELECT count(*), coalesce(sum(bs.duration_min), 0)
		FROM crm.barber_services bs JOIN crm.services sv ON sv.service_id = bs.service_id
		WHERE bs.barber_id = $1 AND bs.service_id = ANY ($2) AND bs.is_active AND sv.is_active`,
		barberID, serviceIDs).Scan(&cnt, &dur); err != nil {
		return nil, err
	}
	if cnt != len(uniqueInts(serviceIDs)) {
		return nil, apperr.New(http.StatusUnprocessableEntity, "invalid_booking",
			"Мастер не оказывает одну или несколько выбранных услуг")
	}
	return getAll[model.Slot](ctx, s.Pool,
		`SELECT slot_start, slot_end FROM crm.free_slots($1, $2::date, $3, $4)`, barberID, date, dur, respectLead)
}

func uniqueInts(xs []int) map[int]struct{} {
	m := make(map[int]struct{}, len(xs))
	for _, x := range xs {
		m[x] = struct{}{}
	}
	return m
}

type BookInput struct {
	ClientPhone string    `json:"client_phone"`
	ClientName  string    `json:"client_name"`
	BarberID    int       `json:"barber_id"`
	StartsAt    time.Time `json:"starts_at"`
	ServiceIDs  []int     `json:"service_ids"`
	Source      string    `json:"source"`
	Comment     *string   `json:"comment"`
}

// Book — бизнес-сценарий «запись клиента» (функция crm.book_appointment, атомарно).
func (s *Store) Book(ctx context.Context, in BookInput) (*model.Appointment, error) {
	var id int64
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT crm.book_appointment($1, $2, $3, $4, $5, $6::crm.booking_source, $7)`,
			in.ClientPhone, in.ClientName, in.BarberID, in.StartsAt, in.ServiceIDs, in.Source, in.Comment).Scan(&id)
	})
	if err != nil {
		return nil, err
	}
	return s.GetAppointment(ctx, id)
}

// SetStatus — подтверждение, начало визита, неявка. Допустимость перехода
// проверяет триггер trg_appointments_validate.
func (s *Store) SetStatus(ctx context.Context, id int64, status string) (*model.Appointment, error) {
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return execOne(ctx, tx, `UPDATE crm.appointments SET status = $2::crm.appointment_status
			WHERE appointment_id = $1`, id, status)
	})
	if err != nil {
		return nil, err
	}
	return s.GetAppointment(ctx, id)
}

func (s *Store) UpdateComment(ctx context.Context, id int64, comment *string) (*model.Appointment, error) {
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return execOne(ctx, tx, `UPDATE crm.appointments SET comment = $2 WHERE appointment_id = $1`, id, comment)
	})
	if err != nil {
		return nil, err
	}
	return s.GetAppointment(ctx, id)
}

func (s *Store) Cancel(ctx context.Context, id int64, reason string) (*model.Appointment, error) {
	err := s.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `SELECT crm.cancel_appointment($1, $2)`, id, reason)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.GetAppointment(ctx, id)
}

func (s *Store) Reschedule(ctx context.Context, id int64, newStart time.Time) (*model.Appointment, error) {
	err := s.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `SELECT crm.reschedule_appointment($1, $2)`, id, newStart)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.GetAppointment(ctx, id)
}

// Complete — бизнес-сценарий «завершение визита с оплатой и бонусами».
func (s *Store) Complete(ctx context.Context, id int64, method string, bonus float64) (*model.CompleteResult, error) {
	var out *model.CompleteResult
	err := s.tx(ctx, func(tx pgx.Tx) (err error) {
		out, err = getOne[model.CompleteResult](ctx, tx, `
			SELECT total_amount, bonus_redeemed, paid_money, bonus_accrued, bonus_balance, loyalty_level::text AS loyalty_level
			FROM crm.complete_appointment($1, $2::crm.payment_method, $3)`, id, method, bonus)
		return err
	})
	return out, err
}

func (s *Store) DeleteAppointment(ctx context.Context, id int64) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		return execOne(ctx, tx, "DELETE FROM crm.appointments WHERE appointment_id = $1", id)
	})
}

// ── Отзывы ──────────────────────────────────────────────────────────────────

func (s *Store) AddReview(ctx context.Context, appointmentID int64, rating int16, comment *string) (*model.Review, error) {
	var out *model.Review
	err := s.tx(ctx, func(tx pgx.Tx) (err error) {
		out, err = getOne[model.Review](ctx, tx, `
			INSERT INTO crm.reviews (appointment_id, rating, comment) VALUES ($1, $2, $3)
			RETURNING appointment_id, rating, comment, created_at`, appointmentID, rating, comment)
		return err
	})
	return out, err
}

func (s *Store) DeleteReview(ctx context.Context, appointmentID int64) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		return execOne(ctx, tx, "DELETE FROM crm.reviews WHERE appointment_id = $1", appointmentID)
	})
}
