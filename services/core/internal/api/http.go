package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/apperr"
	"github.com/RomanKovalev007/barber_crm/services/core/internal/store"
)

const maxBody = 1 << 20

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (a *API) fail(w http.ResponseWriter, r *http.Request, err error) {
	e := apperr.FromDB(err)
	if e.Status >= 500 {
		a.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	} else {
		a.log.Info("request rejected", "path", r.URL.Path, "code", e.Code, "sqlstate", e.SQLState, "msg", e.Message)
	}
	writeJSON(w, e.Status, map[string]any{"error": e})
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return apperr.BadRequest("Некорректный JSON: " + err.Error())
	}
	return nil
}

func pathInt(r *http.Request, name string) (int64, error) {
	v, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || v <= 0 {
		return 0, apperr.BadRequest(fmt.Sprintf("Параметр пути %s должен быть положительным целым числом", name))
	}
	return v, nil
}

// ── параметры запроса ───────────────────────────────────────────────────────

type query struct {
	r    *http.Request
	errs map[string]string
}

func q(r *http.Request) *query { return &query{r: r, errs: map[string]string{}} }

func (q *query) str(name string) string { return strings.TrimSpace(q.r.URL.Query().Get(name)) }

func (q *query) optInt(name string) *int {
	s := q.str(name)
	if s == "" {
		return nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		q.errs[name] = "ожидается целое число"
		return nil
	}
	return &v
}

func (q *query) optInt64(name string) *int64 {
	p := q.optInt(name)
	if p == nil {
		return nil
	}
	v := int64(*p)
	return &v
}

func (q *query) optBool(name string) *bool {
	s := q.str(name)
	if s == "" {
		return nil
	}
	v, err := strconv.ParseBool(s)
	if err != nil {
		q.errs[name] = "ожидается true/false"
		return nil
	}
	return &v
}

// optTime принимает RFC 3339 или дату YYYY-MM-DD (начало дня, Москва).
func (q *query) optTime(name string) *time.Time {
	s := q.str(name)
	if s == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
	}
	if t, err := time.ParseInLocation("2006-01-02", s, moscow); err == nil {
		return &t
	}
	q.errs[name] = "ожидается дата YYYY-MM-DD или RFC 3339"
	return nil
}

func (q *query) date(name string, def time.Time) string {
	s := q.str(name)
	if s == "" {
		return def.Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", s); err != nil {
		q.errs[name] = "ожидается дата YYYY-MM-DD"
	}
	return s
}

func (q *query) intList(name string) []int {
	s := q.str(name)
	if s == "" {
		return nil
	}
	var out []int
	for _, p := range strings.Split(s, ",") {
		v, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || v <= 0 {
			q.errs[name] = "ожидается список id через запятую"
			return nil
		}
		out = append(out, v)
	}
	return out
}

// list разбирает page, page_size и sort. sort — поля через запятую, '-' — по убыванию;
// разрешены только поля из allowed (имя API → выражение SQL), что исключает SQL-инъекцию.
func (q *query) list(allowed map[string]string, def string) store.ListParams {
	p := store.ListParams{Page: 1, PageSize: 20}
	if v := q.optInt("page"); v != nil {
		if *v < 1 {
			q.errs["page"] = "должно быть >= 1"
		} else {
			p.Page = *v
		}
	}
	if v := q.optInt("page_size"); v != nil {
		if *v < 1 || *v > 100 {
			q.errs["page_size"] = "допустимо от 1 до 100"
		} else {
			p.PageSize = *v
		}
	}
	sortParam := q.str("sort")
	if sortParam == "" {
		sortParam = def
	}
	var parts []string
	for _, f := range strings.Split(sortParam, ",") {
		f = strings.TrimSpace(f)
		dir := "ASC"
		if strings.HasPrefix(f, "-") {
			dir, f = "DESC", f[1:]
		}
		col, ok := allowed[f]
		if !ok {
			keys := make([]string, 0, len(allowed))
			for k := range allowed {
				keys = append(keys, k)
			}
			q.errs["sort"] = "недопустимое поле сортировки «" + f + "»; доступны: " + strings.Join(keys, ", ")
			return p
		}
		parts = append(parts, col+" "+dir)
	}
	p.Sort = strings.Join(parts, ", ")
	return p
}

func (q *query) err() error {
	if len(q.errs) == 0 {
		return nil
	}
	return &apperr.Error{Status: http.StatusBadRequest, Code: "validation_error",
		Message: "Некорректные параметры запроса", Fields: q.errs}
}

var moscow = func() *time.Location {
	l, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return time.FixedZone("MSK", 3*3600)
	}
	return l
}()
