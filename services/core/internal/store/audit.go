package store

import (
	"context"
	"time"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/model"
)

type AuditFilter struct {
	Table     string
	Operation string
	RowID     *int64
	From      *time.Time
}

const auditCols = `audit_id, table_name, operation, row_pk, old_data, new_data, changed_columns,
	db_user, app_user, changed_at`

func (s *Store) ListAudit(ctx context.Context, af AuditFilter, p ListParams) (model.Page[model.AuditEntry], error) {
	var f Filter
	if af.Table != "" {
		f.Add("table_name = ?", af.Table)
	}
	if af.Operation != "" {
		f.Add("operation = ?", af.Operation)
	}
	if af.RowID != nil {
		// любой столбец ключа строки равен значению
		f.Add("EXISTS (SELECT 1 FROM jsonb_each_text(row_pk) k WHERE k.value = (?::bigint)::text)", *af.RowID)
	}
	if af.From != nil {
		f.Add("changed_at >= ?", *af.From)
	}
	return list[model.AuditEntry](ctx, s.Pool, auditCols, "audit.audit_log", f, p)
}
