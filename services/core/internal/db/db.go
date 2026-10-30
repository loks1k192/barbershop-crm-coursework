// Package db — подключение к PostgreSQL и выполнение транзакций.
package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect создаёт пул соединений. application_name помогает видеть сервис в pg_stat_activity.
func Connect(ctx context.Context, url string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = maxConns
	cfg.MaxConnIdleTime = 5 * time.Minute
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = "barber-core"
	cfg.ConnConfig.RuntimeParams["timezone"] = "Europe/Moscow"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

type ctxKey struct{}

// WithAppUser сохраняет в контексте имя пользователя приложения — оно попадает
// в журнал аудита и историю статусов (crm.app_user).
func WithAppUser(ctx context.Context, user string) context.Context {
	return context.WithValue(ctx, ctxKey{}, user)
}

func AppUser(ctx context.Context) string {
	if u, ok := ctx.Value(ctxKey{}).(string); ok && u != "" {
		return u
	}
	return "api"
}

// MaxAttempts — сколько раз повторять транзакцию при ошибке сериализации/взаимоблокировке.
const MaxAttempts = 3

// Retryable — ошибки, после которых транзакцию корректно повторить целиком.
func Retryable(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "40001" || pgErr.Code == "40P01" // serialization_failure, deadlock_detected
	}
	return false
}

// InTx выполняет fn в транзакции с заданным уровнем изоляции: устанавливает
// crm.app_user (SET LOCAL), фиксирует при успехе, откатывает при любой ошибке и
// повторяет при конфликте сериализации/взаимоблокировке.
func InTx(ctx context.Context, pool *pgxpool.Pool, iso pgx.TxIsoLevel, fn func(pgx.Tx) error) error {
	var err error
	for attempt := 1; attempt <= MaxAttempts; attempt++ {
		err = runTx(ctx, pool, iso, fn)
		if err == nil || !Retryable(err) {
			return err
		}
		time.Sleep(time.Duration(attempt*attempt) * 20 * time.Millisecond)
	}
	return err
}

func runTx(ctx context.Context, pool *pgxpool.Pool, iso pgx.TxIsoLevel, fn func(pgx.Tx) error) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: iso})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // после Commit — no-op
	if _, err := tx.Exec(ctx, `SELECT set_config('crm.app_user', $1, true)`, AppUser(ctx)); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
