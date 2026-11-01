// Package testdb поднимает изолированную тестовую БД: создаёт базу, применяет
// миграции, загружает контрольный набор данных и создаёт временную логин-роль —
// члена crm_app, чтобы тесты выполнялись с правами приложения.
//
// Требуется переменная TEST_PG_ADMIN_URL (суперпользователь), например
// postgres://postgres:postgres@localhost:5433/postgres. Без неё тесты пропускаются.
package testdb

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/migrate"
	"github.com/RomanKovalev007/barber_crm/services/core/migrations"
)

type DB struct {
	Name     string
	AdminURL string // суперпользователь, тестовая БД
	AppURL   string // логин-роль с правами crm_app
	Admin    *pgxpool.Pool
	App      *pgxpool.Pool
	appRole  string
	rootURL  string
}

func AdminURLFromEnv() string { return os.Getenv("TEST_PG_ADMIN_URL") }

func withDB(raw, db, user, pass string) string {
	u, _ := url.Parse(raw)
	u.Path = "/" + db
	if user != "" {
		u.User = url.UserPassword(user, pass)
	}
	return u.String()
}

// Create создаёт БД. maxVersion > 0 — применить миграции только до этой версии
// и не загружать фикстуру (для теста миграций).
func Create(ctx context.Context, maxVersion int, withFixture bool) (*DB, error) {
	root := AdminURLFromEnv()
	if root == "" {
		return nil, fmt.Errorf("TEST_PG_ADMIN_URL not set")
	}
	suffix := fmt.Sprintf("%d_%d", time.Now().UnixNano()%1e9, rand.Intn(1e6))
	d := &DB{Name: "barbershop_test_" + suffix, rootURL: root, appRole: "crm_app_test_" + suffix}

	rc, err := pgx.Connect(ctx, root)
	if err != nil {
		return nil, err
	}
	defer rc.Close(ctx)
	if _, err := rc.Exec(ctx, "CREATE DATABASE "+d.Name); err != nil {
		return nil, err
	}
	d.AdminURL = withDB(root, d.Name, "", "")

	conn, err := pgx.Connect(ctx, d.AdminURL)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	ms, err := migrate.Load(migrations.FS)
	if err != nil {
		return nil, err
	}
	if err := migrate.New(conn, ms).Up(ctx, maxVersion); err != nil {
		return nil, err
	}
	if maxVersion == 0 {
		pass := "pw_" + suffix
		if _, err := conn.Exec(ctx, fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD '%s' IN ROLE crm_app", d.appRole, pass)); err != nil {
			return nil, err
		}
		d.AppURL = withDB(root, d.Name, d.appRole, pass)
	}
	if withFixture {
		if err := LoadFile(ctx, conn, "testdata/fixture.sql"); err != nil {
			return nil, fmt.Errorf("fixture: %w", err)
		}
	}
	if d.Admin, err = pgxpool.New(ctx, d.AdminURL); err != nil {
		return nil, err
	}
	if d.AppURL != "" {
		cfg, _ := pgxpool.ParseConfig(d.AppURL)
		cfg.MaxConns = 20
		cfg.ConnConfig.RuntimeParams["timezone"] = "Europe/Moscow"
		if d.App, err = pgxpool.NewWithConfig(ctx, cfg); err != nil {
			return nil, err
		}
	}
	return d, nil
}

// LoadFile выполняет SQL-файл относительно корня модуля services/core.
func LoadFile(ctx context.Context, conn *pgx.Conn, rel string) error {
	_, thisFile, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return err
	}
	_, err = conn.Exec(ctx, string(b))
	return err
}

func (d *DB) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if d.App != nil {
		d.App.Close()
	}
	if d.Admin != nil {
		d.Admin.Close()
	}
	rc, err := pgx.Connect(ctx, d.rootURL)
	if err != nil {
		return
	}
	defer rc.Close(ctx)
	_, _ = rc.Exec(ctx, "DROP DATABASE IF EXISTS "+d.Name+" WITH (FORCE)")
	if d.AppURL != "" {
		_, _ = rc.Exec(ctx, "DROP ROLE IF EXISTS "+d.appRole)
	}
}
