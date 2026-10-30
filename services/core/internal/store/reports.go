package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqlfiles "github.com/RomanKovalev007/barber_crm/services/core/sql"
)

// Report — результат аналитического запроса: строки как упорядоченные пары колонка→значение.
type Report struct {
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
}

// RunReport выполняет запрос из sql/queries в транзакции READ ONLY REPEATABLE READ:
// многоэтапный отчёт видит один согласованный снимок данных.
func (s *Store) RunReport(ctx context.Context, file string, args ...any) (*Report, error) {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	return collectReport(ctx, tx, sqlfiles.MustQuery(file), args...)
}

func collectReport(ctx context.Context, q querier, sql string, args ...any) (*Report, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rep := &Report{Rows: []map[string]any{}, Columns: []string{}}
	for _, fd := range rows.FieldDescriptions() {
		rep.Columns = append(rep.Columns, fd.Name)
	}
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make(map[string]any, len(vals))
		for i, v := range vals {
			row[rep.Columns[i]] = normalize(v)
		}
		rep.Rows = append(rep.Rows, row)
	}
	return rep, rows.Err()
}

// normalize приводит значения pgx к JSON-дружественным типам.
func normalize(v any) any {
	switch x := v.(type) {
	case pgtype.Numeric:
		f, err := x.Float64Value()
		if err != nil || !f.Valid {
			return nil
		}
		return f.Float64
	case time.Time:
		if x.Location() == time.UTC && x.Hour() == 0 && x.Minute() == 0 && x.Second() == 0 {
			return x.Format("2006-01-02") // date
		}
		return x
	case []byte:
		return string(x)
	}
	return v
}

// RefreshKPI обновляет материализованное представление KPI (REFRESH CONCURRENTLY).
func (s *Store) RefreshKPI(ctx context.Context) (time.Time, error) {
	var ts time.Time
	err := s.Pool.QueryRow(ctx, `SELECT crm.refresh_kpi()`).Scan(&ts)
	return ts, err
}

// KPI — данные материализованного представления mv_barber_monthly_kpi.
func (s *Store) KPI(ctx context.Context, month *time.Time, barberID *int) (*Report, error) {
	return s.runSQL(ctx, `
		SELECT k.*, b.full_name AS barber_name
		FROM crm.mv_barber_monthly_kpi k JOIN crm.barbers b ON b.barber_id = k.barber_id
		WHERE ($1::date IS NULL OR k.month = date_trunc('month', $1::date)::date)
		  AND ($2::int IS NULL OR k.barber_id = $2)
		ORDER BY k.month DESC, k.revenue DESC`, month, barberID)
}

func (s *Store) runSQL(ctx context.Context, sql string, args ...any) (*Report, error) {
	return collectReport(ctx, s.Pool, sql, args...)
}
