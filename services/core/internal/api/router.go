// Package api — REST API сервиса core (chi).
package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/db"
	"github.com/RomanKovalev007/barber_crm/services/core/internal/store"
)

type API struct {
	s   *store.Store
	log *slog.Logger
}

func New(s *store.Store, log *slog.Logger) *API { return &API{s: s, log: log} }

// Router собирает маршруты. Аутентификация выполняется api-gateway (JWT);
// имя пользователя передаётся заголовком X-User и попадает в аудит.
func (a *API) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, a.logRequests, middleware.Recoverer,
		middleware.Timeout(30*time.Second), appUser)

	r.Get("/health", a.health)
	r.Get("/openapi.yaml", serveOpenAPI)
	r.Get("/docs", serveSwaggerUI)

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/branches", func(r chi.Router) {
			r.Get("/", a.listBranches)
			r.Post("/", a.createBranch)
			r.Get("/{id}", a.getBranch)
			r.Put("/{id}", a.updateBranch)
			r.Delete("/{id}", a.deleteBranch)
		})
		r.Route("/categories", func(r chi.Router) {
			r.Get("/", a.listCategories)
			r.Post("/", a.createCategory)
			r.Put("/{id}", a.updateCategory)
			r.Delete("/{id}", a.deleteCategory)
		})
		r.Route("/services", func(r chi.Router) {
			r.Get("/", a.listServices)
			r.Post("/", a.createService)
			r.Get("/{id}", a.getService)
			r.Put("/{id}", a.updateService)
			r.Delete("/{id}", a.deleteService)
		})
		r.Route("/barbers", func(r chi.Router) {
			r.Get("/", a.listBarbers)
			r.Post("/", a.createBarber)
			r.Get("/{id}", a.getBarber)
			r.Put("/{id}", a.updateBarber)
			r.Delete("/{id}", a.deleteBarber)
			r.Get("/{id}/settings", a.getBarberSettings)
			r.Put("/{id}/settings", a.updateBarberSettings)
			r.Get("/{id}/services", a.listBarberServices)
			r.Put("/{id}/services/{service_id}", a.upsertBarberService)
			r.Delete("/{id}/services/{service_id}", a.deleteBarberService)
			r.Get("/{id}/free-slots", a.freeSlots)
		})
		r.Route("/shifts", func(r chi.Router) {
			r.Get("/", a.listShifts)
			r.Post("/", a.createShift)
			r.Get("/{id}", a.getShift)
			r.Put("/{id}", a.updateShift)
			r.Delete("/{id}", a.deleteShift)
		})
		r.Route("/clients", func(r chi.Router) {
			r.Get("/", a.listClients)
			r.Post("/", a.createClient)
			r.Get("/{id}", a.getClient)
			r.Put("/{id}", a.updateClient)
			r.Delete("/{id}", a.deleteClient)
			r.Get("/{id}/appointments", a.clientAppointments)
			r.Get("/{id}/bonus-history", a.clientBonusHistory)
		})
		r.Route("/appointments", func(r chi.Router) {
			r.Get("/", a.listAppointments)
			r.Post("/", a.bookAppointment)
			r.Get("/{id}", a.getAppointment)
			r.Patch("/{id}", a.patchAppointment)
			r.Delete("/{id}", a.deleteAppointment)
			r.Patch("/{id}/status", a.setStatus)
			r.Post("/{id}/cancel", a.cancelAppointment)
			r.Post("/{id}/reschedule", a.rescheduleAppointment)
			r.Post("/{id}/complete", a.completeAppointment)
			r.Get("/{id}/history", a.appointmentHistory)
			r.Post("/{id}/review", a.addReview)
			r.Delete("/{id}/review", a.deleteReview)
		})
		r.Route("/reports", func(r chi.Router) {
			r.Get("/day-schedule", a.reportDaySchedule)
			r.Get("/client-history", a.reportClientHistory)
			r.Get("/revenue-by-branch", a.reportRevenueByBranch)
			r.Get("/barber-ranking", a.reportBarberRanking)
			r.Get("/rfm", a.reportRFM)
			r.Get("/top-services", a.reportTopServices)
			r.Get("/churn-risk", a.reportChurnRisk)
			r.Get("/no-shows", a.reportNoShows)
			r.Get("/utilization", a.reportUtilization)
			r.Get("/cohorts", a.reportCohorts)
			r.Get("/daily-revenue", a.reportDailyRevenue)
			r.Get("/kpi", a.reportKPI)
			r.Post("/kpi/refresh", a.refreshKPI)
		})
		r.Post("/import/clients", a.importClients)
		r.Get("/audit", a.listAudit)
	})
	return r
}

func appUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := strings.TrimSpace(r.Header.Get("X-User")); u != "" {
			if len(u) > 64 {
				u = u[:64]
			}
			r = r.WithContext(db.WithAppUser(r.Context(), u))
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		a.log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(),
			"duration_ms", time.Since(start).Milliseconds())
	})
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.s.Pool.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
