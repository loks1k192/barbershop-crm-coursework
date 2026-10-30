package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/model"
)

// ── Филиалы ─────────────────────────────────────────────────────────────────

type BranchInput struct {
	Name     string `json:"name"`
	Address  string `json:"address"`
	Phone    string `json:"phone"`
	OpensAt  string `json:"opens_at"`
	ClosesAt string `json:"closes_at"`
	IsActive *bool  `json:"is_active"`
}

const branchCols = `branch_id, name, address, phone, to_char(opens_at, 'HH24:MI') AS opens_at,
	to_char(closes_at, 'HH24:MI') AS closes_at, is_active, created_at, updated_at`

type BranchFilter struct {
	Active *bool
	Search string
}

func (s *Store) ListBranches(ctx context.Context, bf BranchFilter, p ListParams) (model.Page[model.Branch], error) {
	var f Filter
	if bf.Active != nil {
		f.Add("is_active = ?", *bf.Active)
	}
	if bf.Search != "" {
		f.Add("(name ILIKE '%' || ? || '%')", bf.Search)
	}
	return list[model.Branch](ctx, s.Pool, branchCols, "crm.branches", f, p)
}

func (s *Store) GetBranch(ctx context.Context, id int) (*model.Branch, error) {
	return getOne[model.Branch](ctx, s.Pool, "SELECT "+branchCols+" FROM crm.branches WHERE branch_id = $1", id)
}

func (s *Store) CreateBranch(ctx context.Context, in BranchInput) (*model.Branch, error) {
	var out *model.Branch
	err := s.tx(ctx, func(tx pgx.Tx) (err error) {
		out, err = getOne[model.Branch](ctx, tx, `
			INSERT INTO crm.branches (name, address, phone, opens_at, closes_at, is_active)
			VALUES ($1, $2, $3, $4::time, $5::time, coalesce($6, true))
			RETURNING `+branchCols, in.Name, in.Address, in.Phone, in.OpensAt, in.ClosesAt, in.IsActive)
		return err
	})
	return out, err
}

func (s *Store) UpdateBranch(ctx context.Context, id int, in BranchInput) (*model.Branch, error) {
	var out *model.Branch
	err := s.tx(ctx, func(tx pgx.Tx) (err error) {
		out, err = getOne[model.Branch](ctx, tx, `
			UPDATE crm.branches SET name = $2, address = $3, phone = $4, opens_at = $5::time,
			       closes_at = $6::time, is_active = coalesce($7, is_active)
			WHERE branch_id = $1
			RETURNING `+branchCols, id, in.Name, in.Address, in.Phone, in.OpensAt, in.ClosesAt, in.IsActive)
		return err
	})
	return out, err
}

func (s *Store) DeleteBranch(ctx context.Context, id int) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		return execOne(ctx, tx, "DELETE FROM crm.branches WHERE branch_id = $1", id)
	})
}

// ── Категории ───────────────────────────────────────────────────────────────

type CategoryInput struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

func (s *Store) ListCategories(ctx context.Context) ([]model.Category, error) {
	return getAll[model.Category](ctx, s.Pool,
		"SELECT category_id, name, description FROM crm.service_categories ORDER BY name")
}

func (s *Store) CreateCategory(ctx context.Context, in CategoryInput) (*model.Category, error) {
	var out *model.Category
	err := s.tx(ctx, func(tx pgx.Tx) (err error) {
		out, err = getOne[model.Category](ctx, tx, `
			INSERT INTO crm.service_categories (name, description) VALUES ($1, $2)
			RETURNING category_id, name, description`, in.Name, in.Description)
		return err
	})
	return out, err
}

func (s *Store) UpdateCategory(ctx context.Context, id int, in CategoryInput) (*model.Category, error) {
	var out *model.Category
	err := s.tx(ctx, func(tx pgx.Tx) (err error) {
		out, err = getOne[model.Category](ctx, tx, `
			UPDATE crm.service_categories SET name = $2, description = $3 WHERE category_id = $1
			RETURNING category_id, name, description`, id, in.Name, in.Description)
		return err
	})
	return out, err
}

func (s *Store) DeleteCategory(ctx context.Context, id int) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		return execOne(ctx, tx, "DELETE FROM crm.service_categories WHERE category_id = $1", id)
	})
}

// ── Услуги (каталог сети) ───────────────────────────────────────────────────

type ServiceInput struct {
	CategoryID      int     `json:"category_id"`
	Name            string  `json:"name"`
	Description     *string `json:"description"`
	BasePrice       float64 `json:"base_price"`
	BaseDurationMin int     `json:"base_duration_min"`
	IsActive        *bool   `json:"is_active"`
}

type ServiceFilter struct {
	CategoryID *int
	Active     *bool
	Search     string
}

const serviceCols = `s.service_id, s.category_id, c.name AS category_name, s.name, s.description,
	s.base_price, s.base_duration_min, s.is_active`
const serviceFrom = `crm.services s JOIN crm.service_categories c ON c.category_id = s.category_id`

func (s *Store) ListServices(ctx context.Context, sf ServiceFilter, p ListParams) (model.Page[model.Service], error) {
	var f Filter
	if sf.CategoryID != nil {
		f.Add("s.category_id = ?", *sf.CategoryID)
	}
	if sf.Active != nil {
		f.Add("s.is_active = ?", *sf.Active)
	}
	if sf.Search != "" {
		f.Add("s.name ILIKE '%' || ? || '%'", sf.Search)
	}
	return list[model.Service](ctx, s.Pool, serviceCols, serviceFrom, f, p)
}

func (s *Store) GetService(ctx context.Context, id int) (*model.Service, error) {
	return getOne[model.Service](ctx, s.Pool, "SELECT "+serviceCols+" FROM "+serviceFrom+" WHERE s.service_id = $1", id)
}

func (s *Store) CreateService(ctx context.Context, in ServiceInput) (*model.Service, error) {
	var id int
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO crm.services (category_id, name, description, base_price, base_duration_min, is_active)
			VALUES ($1, $2, $3, $4, $5, coalesce($6, true)) RETURNING service_id`,
			in.CategoryID, in.Name, in.Description, in.BasePrice, in.BaseDurationMin, in.IsActive).Scan(&id)
	})
	if err != nil {
		return nil, err
	}
	return s.GetService(ctx, id)
}

func (s *Store) UpdateService(ctx context.Context, id int, in ServiceInput) (*model.Service, error) {
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return execOne(ctx, tx, `
			UPDATE crm.services SET category_id = $2, name = $3, description = $4, base_price = $5,
			       base_duration_min = $6, is_active = coalesce($7, is_active)
			WHERE service_id = $1`,
			id, in.CategoryID, in.Name, in.Description, in.BasePrice, in.BaseDurationMin, in.IsActive)
	})
	if err != nil {
		return nil, err
	}
	return s.GetService(ctx, id)
}

func (s *Store) DeleteService(ctx context.Context, id int) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		return execOne(ctx, tx, "DELETE FROM crm.services WHERE service_id = $1", id)
	})
}
