package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/model"
)

type BarberInput struct {
	BranchID     int     `json:"branch_id"`
	FullName     string  `json:"full_name"`
	Phone        string  `json:"phone"`
	Login        string  `json:"login"`
	Password     string  `json:"password,omitempty"`
	PasswordHash string  `json:"-"`
	Grade        string  `json:"grade"`
	HiredAt      *string `json:"hired_at"`
	FiredAt      *string `json:"fired_at"`
}

type BarberFilter struct {
	BranchID *int
	Grade    string
	Active   *bool
	Search   string
}

const barberCols = `b.barber_id, b.branch_id, br.name AS branch_name, b.full_name, b.phone, b.login,
	b.grade::text AS grade, to_char(b.hired_at, 'YYYY-MM-DD') AS hired_at,
	to_char(b.fired_at, 'YYYY-MM-DD') AS fired_at, b.created_at, b.updated_at`
const barberFrom = `crm.barbers b JOIN crm.branches br ON br.branch_id = b.branch_id`

func (s *Store) ListBarbers(ctx context.Context, bf BarberFilter, p ListParams) (model.Page[model.Barber], error) {
	var f Filter
	if bf.BranchID != nil {
		f.Add("b.branch_id = ?", *bf.BranchID)
	}
	if bf.Grade != "" {
		f.Add("b.grade::text = ?", bf.Grade)
	}
	if bf.Active != nil {
		f.Add("(b.fired_at IS NULL) = ?", *bf.Active)
	}
	if bf.Search != "" {
		f.Add("b.full_name ILIKE '%' || ? || '%'", bf.Search)
	}
	return list[model.Barber](ctx, s.Pool, barberCols, barberFrom, f, p)
}

func (s *Store) GetBarber(ctx context.Context, id int) (*model.Barber, error) {
	return getOne[model.Barber](ctx, s.Pool, "SELECT "+barberCols+" FROM "+barberFrom+" WHERE b.barber_id = $1", id)
}

// CreateBarber создаёт мастера; настройки записи создаёт триггер (1:1).
func (s *Store) CreateBarber(ctx context.Context, in BarberInput) (*model.Barber, error) {
	var id int
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO crm.barbers (branch_id, full_name, phone, login, password_hash, grade, hired_at, fired_at)
			VALUES ($1, $2, $3, $4, $5, $6::crm.barber_grade, coalesce($7::date, current_date), $8::date)
			RETURNING barber_id`,
			in.BranchID, in.FullName, in.Phone, in.Login, in.PasswordHash, in.Grade, in.HiredAt, in.FiredAt).Scan(&id)
	})
	if err != nil {
		return nil, err
	}
	return s.GetBarber(ctx, id)
}

func (s *Store) UpdateBarber(ctx context.Context, id int, in BarberInput) (*model.Barber, error) {
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return execOne(ctx, tx, `
			UPDATE crm.barbers SET branch_id = $2, full_name = $3, phone = $4, login = $5,
			       grade = $6::crm.barber_grade, hired_at = coalesce($7::date, hired_at), fired_at = $8::date,
			       password_hash = coalesce(nullif($9, ''), password_hash)
			WHERE barber_id = $1`,
			id, in.BranchID, in.FullName, in.Phone, in.Login, in.Grade, in.HiredAt, in.FiredAt, in.PasswordHash)
	})
	if err != nil {
		return nil, err
	}
	return s.GetBarber(ctx, id)
}

func (s *Store) DeleteBarber(ctx context.Context, id int) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		return execOne(ctx, tx, "DELETE FROM crm.barbers WHERE barber_id = $1", id)
	})
}

// ── Настройки онлайн-записи ─────────────────────────────────────────────────

const settingsCols = `barber_id, slot_step_min, compact_slots, min_lead_min, max_days_ahead, updated_at`

func (s *Store) GetBarberSettings(ctx context.Context, barberID int) (*model.BarberSettings, error) {
	return getOne[model.BarberSettings](ctx, s.Pool,
		"SELECT "+settingsCols+" FROM crm.barber_settings WHERE barber_id = $1", barberID)
}

func (s *Store) UpdateBarberSettings(ctx context.Context, in model.BarberSettings) (*model.BarberSettings, error) {
	var out *model.BarberSettings
	err := s.tx(ctx, func(tx pgx.Tx) (err error) {
		out, err = getOne[model.BarberSettings](ctx, tx, `
			UPDATE crm.barber_settings SET slot_step_min = $2, compact_slots = $3, min_lead_min = $4,
			       max_days_ahead = $5
			WHERE barber_id = $1 RETURNING `+settingsCols,
			in.BarberID, in.SlotStepMin, in.CompactSlots, in.MinLeadMin, in.MaxDaysAhead)
		return err
	})
	return out, err
}

// ── Прайс мастера (M:N barbers × services) ──────────────────────────────────

type BarberServiceInput struct {
	Price       float64 `json:"price"`
	DurationMin int     `json:"duration_min"`
	IsActive    *bool   `json:"is_active"`
}

const barberServiceCols = `bs.barber_id, bs.service_id, s.name AS service_name, bs.price, bs.duration_min, bs.is_active`

func (s *Store) ListBarberServices(ctx context.Context, barberID int, onlyActive bool) ([]model.BarberService, error) {
	return getAll[model.BarberService](ctx, s.Pool, `
		SELECT `+barberServiceCols+`
		FROM crm.barber_services bs JOIN crm.services s ON s.service_id = bs.service_id
		WHERE bs.barber_id = $1 AND (NOT $2 OR (bs.is_active AND s.is_active))
		ORDER BY s.name`, barberID, onlyActive)
}

// UpsertBarberService добавляет услугу в прайс мастера или изменяет цену/длительность.
func (s *Store) UpsertBarberService(ctx context.Context, barberID, serviceID int, in BarberServiceInput) (*model.BarberService, error) {
	var out *model.BarberService
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO crm.barber_services (barber_id, service_id, price, duration_min, is_active)
			VALUES ($1, $2, $3, $4, coalesce($5, true))
			ON CONFLICT (barber_id, service_id) DO UPDATE
			SET price = EXCLUDED.price, duration_min = EXCLUDED.duration_min,
			    is_active = coalesce($5, crm.barber_services.is_active)`,
			barberID, serviceID, in.Price, in.DurationMin, in.IsActive); err != nil {
			return err
		}
		var err error
		out, err = getOne[model.BarberService](ctx, tx, `
			SELECT `+barberServiceCols+`
			FROM crm.barber_services bs JOIN crm.services s ON s.service_id = bs.service_id
			WHERE bs.barber_id = $1 AND bs.service_id = $2`, barberID, serviceID)
		return err
	})
	return out, err
}

func (s *Store) DeleteBarberService(ctx context.Context, barberID, serviceID int) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		return execOne(ctx, tx, "DELETE FROM crm.barber_services WHERE barber_id = $1 AND service_id = $2",
			barberID, serviceID)
	})
}
