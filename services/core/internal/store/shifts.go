package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/model"
)

type ShiftInput struct {
	BarberID int    `json:"barber_id"`
	BranchID int    `json:"branch_id"`
	WorkDate string `json:"work_date"`
	StartsAt string `json:"starts_at"`
	EndsAt   string `json:"ends_at"`
}

type ShiftFilter struct {
	BarberID *int
	BranchID *int
	DateFrom string
	DateTo   string
}

const shiftCols = `ws.shift_id, ws.barber_id, b.full_name AS barber_name, ws.branch_id, br.name AS branch_name,
	to_char(ws.work_date, 'YYYY-MM-DD') AS work_date, to_char(ws.starts_at, 'HH24:MI') AS starts_at,
	to_char(ws.ends_at, 'HH24:MI') AS ends_at`
const shiftFrom = `crm.work_shifts ws
	JOIN crm.barbers b   ON b.barber_id = ws.barber_id
	JOIN crm.branches br ON br.branch_id = ws.branch_id`

func (s *Store) ListShifts(ctx context.Context, sf ShiftFilter, p ListParams) (model.Page[model.Shift], error) {
	var f Filter
	if sf.BarberID != nil {
		f.Add("ws.barber_id = ?", *sf.BarberID)
	}
	if sf.BranchID != nil {
		f.Add("ws.branch_id = ?", *sf.BranchID)
	}
	if sf.DateFrom != "" {
		f.Add("ws.work_date >= ?::date", sf.DateFrom)
	}
	if sf.DateTo != "" {
		f.Add("ws.work_date <= ?::date", sf.DateTo)
	}
	return list[model.Shift](ctx, s.Pool, shiftCols, shiftFrom, f, p)
}

func (s *Store) GetShift(ctx context.Context, id int64) (*model.Shift, error) {
	return getOne[model.Shift](ctx, s.Pool, "SELECT "+shiftCols+" FROM "+shiftFrom+" WHERE ws.shift_id = $1", id)
}

func (s *Store) CreateShift(ctx context.Context, in ShiftInput) (*model.Shift, error) {
	var id int64
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO crm.work_shifts (barber_id, branch_id, work_date, starts_at, ends_at)
			VALUES ($1, $2, $3::date, $4::time, $5::time) RETURNING shift_id`,
			in.BarberID, in.BranchID, in.WorkDate, in.StartsAt, in.EndsAt).Scan(&id)
	})
	if err != nil {
		return nil, err
	}
	return s.GetShift(ctx, id)
}

func (s *Store) UpdateShift(ctx context.Context, id int64, in ShiftInput) (*model.Shift, error) {
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return execOne(ctx, tx, `
			UPDATE crm.work_shifts SET barber_id = $2, branch_id = $3, work_date = $4::date,
			       starts_at = $5::time, ends_at = $6::time
			WHERE shift_id = $1`, id, in.BarberID, in.BranchID, in.WorkDate, in.StartsAt, in.EndsAt)
	})
	if err != nil {
		return nil, err
	}
	return s.GetShift(ctx, id)
}

func (s *Store) DeleteShift(ctx context.Context, id int64) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		return execOne(ctx, tx, "DELETE FROM crm.work_shifts WHERE shift_id = $1", id)
	})
}
