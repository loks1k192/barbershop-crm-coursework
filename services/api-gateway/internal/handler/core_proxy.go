package handler

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/RomanKovalev007/barber_crm/services/api-gateway/internal/middleware"
)

// CoreProxy проксирует /api/core/* в сервис core (REST API единой БД сети).
// Запрос уже прошёл проверку JWT; идентификатор барбера передаётся заголовком
// X-User и попадает в журнал аудита и историю статусов core.
func CoreProxy(coreURL string) (http.Handler, error) {
	target, err := url.Parse(coreURL)
	if err != nil {
		return nil, err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	base := proxy.Director
	proxy.Director = func(r *http.Request) {
		user := "barber:" + middleware.BarberIDFromCtx(r.Context())
		base(r)
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api/core")
		r.URL.RawPath = ""
		r.Header.Del("Authorization")
		r.Header.Set("X-User", user)
	}
	return proxy, nil
}
