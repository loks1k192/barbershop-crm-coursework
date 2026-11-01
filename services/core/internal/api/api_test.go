package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/api"
	"github.com/RomanKovalev007/barber_crm/services/core/internal/store"
	"github.com/RomanKovalev007/barber_crm/services/core/internal/testdb"
)

var srv *httptest.Server

func TestMain(m *testing.M) {
	if testdb.AdminURLFromEnv() == "" {
		fmt.Println("TEST_PG_ADMIN_URL not set — integration tests skipped")
		os.Exit(0)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	d, err := testdb.Create(ctx, 0, true)
	cancel()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv = httptest.NewServer(api.New(store.New(d.App), log).Router())
	code := m.Run()
	srv.Close()
	d.Close()
	os.Exit(code)
}

type resp struct {
	status int
	body   map[string]any
	raw    string
}

func call(t *testing.T, method, path string, body any) resp {
	t.Helper()
	var rd io.Reader
	if s, ok := body.(string); ok {
		rd = strings.NewReader(s)
	} else if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, srv.URL+path, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User", "e2e-test")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{status: res.StatusCode, raw: string(raw)}
	_ = json.Unmarshal(raw, &out.body)
	return out
}

func expect(t *testing.T, r resp, status int) {
	t.Helper()
	if r.status != status {
		t.Fatalf("HTTP %d, ожидался %d: %s", r.status, status, r.raw)
	}
}

func errCode(r resp) string {
	if e, ok := r.body["error"].(map[string]any); ok {
		return fmt.Sprint(e["code"])
	}
	return ""
}

func tomorrow(hh int) string {
	l, _ := time.LoadLocation("Europe/Moscow")
	d := time.Now().In(l).AddDate(0, 0, 1)
	return time.Date(d.Year(), d.Month(), d.Day(), hh, 0, 0, 0, l).Format(time.RFC3339)
}

func TestValidationAndErrorMapping(t *testing.T) {
	r := call(t, "POST", "/api/v1/clients", map[string]any{"phone": "8916", "first_name": ""})
	expect(t, r, 400)
	if errCode(r) != "validation_error" {
		t.Fatal(r.raw)
	}
	expect(t, call(t, "POST", "/api/v1/clients", `{"phone":`), 400)                         // битый JSON
	expect(t, call(t, "POST", "/api/v1/clients", map[string]any{"unknown": 1}), 400)        // лишнее поле
	expect(t, call(t, "GET", "/api/v1/clients?sort=password", nil), 400)                    // поле сортировки не из белого списка
	expect(t, call(t, "GET", "/api/v1/clients?page_size=1000", nil), 400)                   // лимит страницы
	expect(t, call(t, "GET", "/api/v1/clients/999999", nil), 404)                           // нет строки
	expect(t, call(t, "GET", "/api/v1/clients/abc", nil), 400)                              // некорректный id
	r = call(t, "DELETE", "/api/v1/clients/1", nil)                                         // FK RESTRICT
	expect(t, r, 409)
	if errCode(r) != "in_use" {
		t.Fatal(r.raw)
	}
	r = call(t, "POST", "/api/v1/clients", map[string]any{"phone": "+79160000001", "first_name": "Дубль"})
	expect(t, r, 409) // UNIQUE
	if errCode(r) != "already_exists" {
		t.Fatal(r.raw)
	}
}

func TestBookingFlowHTTP(t *testing.T) {
	r := call(t, "GET", "/api/v1/barbers/3/free-slots?date="+tomorrow(0)[:10]+"&service_ids=1", nil)
	expect(t, r, 200)
	if len(r.body["slots"].([]any)) == 0 {
		t.Fatal("нет свободных слотов")
	}
	book := map[string]any{"client_phone": "+79168880000", "client_name": "Сквозной Тест", "barber_id": 3,
		"starts_at": tomorrow(15), "service_ids": []int{1, 3}}
	r = call(t, "POST", "/api/v1/appointments", book)
	expect(t, r, 201)
	id := int(r.body["appointment_id"].(float64))
	if r.body["total_price"].(float64) != 5000 {
		t.Fatal(r.raw)
	}
	book["client_phone"] = "+79168880001"
	r = call(t, "POST", "/api/v1/appointments", book)
	expect(t, r, 409)
	if errCode(r) != "time_slot_conflict" {
		t.Fatal(r.raw)
	}
	book["starts_at"] = tomorrow(18) // 18:00 + 120 мин > конца смены 19:00
	r = call(t, "POST", "/api/v1/appointments", book)
	expect(t, r, 422)
	if errCode(r) != "schedule_violation" {
		t.Fatal(r.raw)
	}
	expect(t, call(t, "PATCH", fmt.Sprintf("/api/v1/appointments/%d/status", id), map[string]any{"status": "confirmed"}), 200)
	r = call(t, "POST", fmt.Sprintf("/api/v1/appointments/%d/complete", id), map[string]any{"payment_method": "card"})
	expect(t, r, 409) // визит ещё не начался
	expect(t, call(t, "POST", fmt.Sprintf("/api/v1/appointments/%d/cancel", id), map[string]any{"reason": ""}), 400)
	expect(t, call(t, "POST", fmt.Sprintf("/api/v1/appointments/%d/cancel", id), map[string]any{"reason": "Передумал"}), 200)
	r = call(t, "GET", fmt.Sprintf("/api/v1/appointments/%d/history", id), nil)
	expect(t, r, 200)
	if !strings.Contains(r.raw, `"changed_by":"e2e-test"`) {
		t.Fatalf("пользователь приложения не попал в историю: %s", r.raw)
	}
	r = call(t, "GET", fmt.Sprintf("/api/v1/audit?table=crm.appointments&row_id=%d", id), nil)
	expect(t, r, 200)
	if r.body["total"].(float64) < 3 {
		t.Fatalf("аудит: %s", r.raw)
	}
}

func TestListingAndReportsHTTP(t *testing.T) {
	r := call(t, "GET", "/api/v1/appointments?status=completed&barber_id=1&sort=-total_price,appointment_id&page=1&page_size=2", nil)
	expect(t, r, 200)
	items := r.body["items"].([]any)
	if len(items) != 2 || r.body["total"].(float64) < 4 {
		t.Fatal(r.raw)
	}
	if items[0].(map[string]any)["total_price"].(float64) < items[1].(map[string]any)["total_price"].(float64) {
		t.Fatal("сортировка по убыванию цены нарушена")
	}
	r = call(t, "GET", "/api/v1/reports/barber-ranking?from=2026-03-01&to=2026-04-01", nil)
	expect(t, r, 200)
	if len(r.body["rows"].([]any)) != 3 {
		t.Fatal(r.raw)
	}
	expect(t, call(t, "GET", "/api/v1/reports/day-schedule", nil), 400) // нет обязательного barber_id
	expect(t, call(t, "POST", "/api/v1/reports/kpi/refresh", nil), 200)
	r = call(t, "GET", "/api/v1/reports/kpi?month=2026-03-01", nil)
	expect(t, r, 200)
	if len(r.body["rows"].([]any)) != 3 {
		t.Fatal(r.raw)
	}
}
