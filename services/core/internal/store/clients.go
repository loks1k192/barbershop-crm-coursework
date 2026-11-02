package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/model"
)

type ClientInput struct {
	Phone            string  `json:"phone"`
	LastName         *string `json:"last_name"`
	FirstName        string  `json:"first_name"`
	MiddleName       *string `json:"middle_name"`
	Email            *string `json:"email"`
	BirthDate        *string `json:"birth_date"`
	Notes            string  `json:"notes"`
	MarketingConsent bool    `json:"marketing_consent"`
}

type ClientFilter struct {
	Search    string // ФИО, телефон или e-mail (подстрока)
	Level     string
	Consent   *bool
	MinVisits *int
}

const clientCols = `c.client_id, c.phone, c.last_name, c.first_name, c.middle_name, c.full_name, c.email,
	to_char(c.birth_date, 'YYYY-MM-DD') AS birth_date, c.notes, c.marketing_consent,
	st.level::text AS level, st.bonus_balance, st.total_spent, st.visits_count, st.no_show_count,
	st.last_visit_at, st.next_visit_at, c.created_at`
const clientFrom = `crm.clients c JOIN crm.v_client_stats st ON st.client_id = c.client_id`

func (s *Store) ListClients(ctx context.Context, cf ClientFilter, p ListParams) (model.Page[model.Client], error) {
	var f Filter
	if cf.Search != "" {
		// выражение совпадает с триграммным индексом clients_search_trgm_idx (миграция 0010)
		f.Add("crm.client_search_text(c.full_name, c.phone, c.email) LIKE '%' || lower(?) || '%'", cf.Search)
	}
	if cf.Level != "" {
		f.Add("st.level::text = ?", cf.Level)
	}
	if cf.Consent != nil {
		f.Add("c.marketing_consent = ?", *cf.Consent)
	}
	if cf.MinVisits != nil {
		f.Add("st.visits_count >= ?", *cf.MinVisits)
	}
	return list[model.Client](ctx, s.Pool, clientCols, clientFrom, f, p)
}

func (s *Store) GetClient(ctx context.Context, id int64) (*model.Client, error) {
	return getOne[model.Client](ctx, s.Pool, "SELECT "+clientCols+" FROM "+clientFrom+" WHERE c.client_id = $1", id)
}

// CreateClient создаёт клиента; бонусный счёт (1:1) создаёт триггер.
func (s *Store) CreateClient(ctx context.Context, in ClientInput) (*model.Client, error) {
	var id int64
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO crm.clients (phone, last_name, first_name, middle_name, email, birth_date, notes, marketing_consent)
			VALUES ($1, $2, $3, $4, lower($5), $6::date, $7, $8) RETURNING client_id`,
			in.Phone, in.LastName, in.FirstName, in.MiddleName, in.Email, in.BirthDate, in.Notes, in.MarketingConsent).Scan(&id)
	})
	if err != nil {
		return nil, err
	}
	return s.GetClient(ctx, id)
}

func (s *Store) UpdateClient(ctx context.Context, id int64, in ClientInput) (*model.Client, error) {
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return execOne(ctx, tx, `
			UPDATE crm.clients SET phone = $2, last_name = $3, first_name = $4, middle_name = $5,
			       email = lower($6), birth_date = $7::date, notes = $8, marketing_consent = $9
			WHERE client_id = $1`,
			id, in.Phone, in.LastName, in.FirstName, in.MiddleName, in.Email, in.BirthDate, in.Notes, in.MarketingConsent)
	})
	if err != nil {
		return nil, err
	}
	return s.GetClient(ctx, id)
}

// DeleteClient: клиента с историей визитов удалить нельзя (FK ON DELETE RESTRICT → 409).
func (s *Store) DeleteClient(ctx context.Context, id int64) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		return execOne(ctx, tx, "DELETE FROM crm.clients WHERE client_id = $1", id)
	})
}

func (s *Store) ClientBonusHistory(ctx context.Context, clientID int64) ([]model.BonusTx, error) {
	return getAll[model.BonusTx](ctx, s.Pool, `
		SELECT bonus_tx_id, appointment_id, kind::text AS kind, amount, comment, created_at
		FROM crm.bonus_transactions WHERE client_id = $1 ORDER BY created_at DESC, bonus_tx_id DESC`, clientID)
}
