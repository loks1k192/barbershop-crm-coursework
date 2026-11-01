package store_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/apperr"
	"github.com/RomanKovalev007/barber_crm/services/core/internal/store"
	"github.com/RomanKovalev007/barber_crm/services/core/internal/testdb"
)

var (
	tdb *testdb.DB
	st  *store.Store
	// отдельная нетронутая копия контрольных данных для тестов отчётов:
	// остальные тесты изменяют данные и не должны влиять на ожидаемые значения
	rdb *testdb.DB
	rst *store.Store
)

func TestMain(m *testing.M) {
	if testdb.AdminURLFromEnv() == "" {
		fmt.Println("TEST_PG_ADMIN_URL not set — integration tests skipped")
		os.Exit(0)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	var err error
	tdb, err = testdb.Create(ctx, 0, true)
	cancel()
	if err != nil {
		fmt.Println("test db:", err)
		os.Exit(1)
	}
	st = store.New(tdb.App) // все операции — с правами роли приложения crm_app
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Minute)
	rdb, err = testdb.Create(ctx2, 0, true)
	cancel2()
	if err != nil {
		fmt.Println("report db:", err)
		tdb.Close()
		os.Exit(1)
	}
	rst = store.New(rdb.App)
	code := m.Run()
	tdb.Close()
	rdb.Close()
	os.Exit(code)
}

var moscow, _ = time.LoadLocation("Europe/Moscow")

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return c
}

// sqlstate возвращает код ошибки PostgreSQL ("" если это не ошибка СУБД).
func sqlstate(err error) string {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code
	}
	return ""
}

func wantState(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("ожидалась ошибка SQLSTATE %s, получен успех", code)
	}
	if got := sqlstate(err); got != code {
		t.Fatalf("ожидался SQLSTATE %s, получен %q: %v", code, got, err)
	}
}

func wantHTTP(t *testing.T, err error, status int) {
	t.Helper()
	if err == nil {
		t.Fatalf("ожидалась ошибка HTTP %d, получен успех", status)
	}
	if e := apperr.FromDB(err); e.Status != status {
		t.Fatalf("ожидался HTTP %d, получен %d (%s): %v", status, e.Status, e.Code, err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// day возвращает дату через n дней (Москва) и момент времени hh:mm этого дня.
func day(n int) string { return time.Now().In(moscow).AddDate(0, 0, n).Format("2006-01-02") }

func at(n, hh, mm int) time.Time {
	d := time.Now().In(moscow).AddDate(0, 0, n)
	return time.Date(d.Year(), d.Month(), d.Day(), hh, mm, 0, 0, moscow)
}

func scalar[T any](t *testing.T, sql string, args ...any) T {
	t.Helper()
	var v T
	must(t, tdb.Admin.QueryRow(context.Background(), sql, args...).Scan(&v))
	return v
}
