// Package migrate — версионные миграции схемы из встроенных SQL-файлов.
//
// Каждая миграция применяется в отдельной транзакции (DDL в PostgreSQL
// транзакционен): при ошибке схема остаётся в предыдущем согласованном
// состоянии. Параллельный запуск нескольких мигратор-процессов исключается
// advisory-блокировкой. Контрольные суммы применённых миграций сверяются
// с файлами, чтобы обнаружить их изменение «задним числом».
package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
)

const lockKey = 7_346_001

type Migration struct {
	Version  int
	Name     string
	Up, Down string
}

func (m Migration) Checksum() string {
	h := sha256.Sum256([]byte(m.Up))
	return hex.EncodeToString(h[:])
}

var fileRe = regexp.MustCompile(`^(\d{4})_(.+)\.(up|down)\.sql$`)

// Load читает миграции из файловой системы (embed.FS).
func Load(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	byVer := map[int]*Migration{}
	for _, e := range entries {
		m := fileRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		v, _ := strconv.Atoi(m[1])
		body, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, err
		}
		mg := byVer[v]
		if mg == nil {
			mg = &Migration{Version: v, Name: m[2]}
			byVer[v] = mg
		}
		if m[3] == "up" {
			mg.Up = string(body)
		} else {
			mg.Down = string(body)
		}
	}
	out := make([]Migration, 0, len(byVer))
	for _, m := range byVer {
		if m.Up == "" {
			return nil, fmt.Errorf("migration %04d has no up script", m.Version)
		}
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

type Applied struct {
	Version  int
	Name     string
	Checksum string
}

type Migrator struct {
	conn       *pgx.Conn
	migrations []Migration
	Log        func(format string, args ...any)
}

func New(conn *pgx.Conn, migrations []Migration) *Migrator {
	return &Migrator{conn: conn, migrations: migrations, Log: func(string, ...any) {}}
}

func (m *Migrator) init(ctx context.Context) error {
	_, err := m.conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS public.schema_migrations (
			version    integer     PRIMARY KEY,
			name       text        NOT NULL,
			checksum   text        NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return err
	}
	_, err = m.conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockKey)
	return err
}

func (m *Migrator) unlock(ctx context.Context) {
	_, _ = m.conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, lockKey)
}

func (m *Migrator) AppliedList(ctx context.Context) ([]Applied, error) {
	rows, err := m.conn.Query(ctx, `SELECT version, name, checksum FROM public.schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Applied, error) {
		var a Applied
		err := r.Scan(&a.Version, &a.Name, &a.Checksum)
		return a, err
	})
}

// Up применяет все неприменённые миграции с версией <= target (0 — все).
func (m *Migrator) Up(ctx context.Context, target int) error {
	if err := m.init(ctx); err != nil {
		return err
	}
	defer m.unlock(ctx)
	applied, err := m.AppliedList(ctx)
	if err != nil {
		return err
	}
	done := map[int]Applied{}
	for _, a := range applied {
		done[a.Version] = a
	}
	for _, mg := range m.migrations {
		if a, ok := done[mg.Version]; ok {
			if a.Checksum != mg.Checksum() {
				m.Log("WARNING: migration %04d_%s changed after it was applied", mg.Version, mg.Name)
			}
			continue
		}
		if target > 0 && mg.Version > target {
			break
		}
		if err := m.apply(ctx, mg.Up, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO public.schema_migrations (version, name, checksum) VALUES ($1, $2, $3)`,
				mg.Version, mg.Name, mg.Checksum())
			return err
		}); err != nil {
			return fmt.Errorf("migration %04d_%s: %w", mg.Version, mg.Name, err)
		}
		m.Log("applied %04d_%s", mg.Version, mg.Name)
	}
	return nil
}

// Down откатывает последние steps миграций.
func (m *Migrator) Down(ctx context.Context, steps int) error {
	if err := m.init(ctx); err != nil {
		return err
	}
	defer m.unlock(ctx)
	applied, err := m.AppliedList(ctx)
	if err != nil {
		return err
	}
	byVer := map[int]Migration{}
	for _, mg := range m.migrations {
		byVer[mg.Version] = mg
	}
	for i := len(applied) - 1; i >= 0 && steps > 0; i, steps = i-1, steps-1 {
		mg, ok := byVer[applied[i].Version]
		if !ok || mg.Down == "" {
			return fmt.Errorf("no down script for migration %04d", applied[i].Version)
		}
		if err := m.apply(ctx, mg.Down, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `DELETE FROM public.schema_migrations WHERE version = $1`, mg.Version)
			return err
		}); err != nil {
			return fmt.Errorf("rollback %04d_%s: %w", mg.Version, mg.Name, err)
		}
		m.Log("rolled back %04d_%s", mg.Version, mg.Name)
	}
	return nil
}

func (m *Migrator) apply(ctx context.Context, sql string, record func(pgx.Tx) error) error {
	tx, err := m.conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, sql); err != nil {
		return err
	}
	if err := record(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
