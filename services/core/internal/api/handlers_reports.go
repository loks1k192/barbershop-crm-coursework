package api

import (
	"net/http"
	"time"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/apperr"
	"github.com/RomanKovalev007/barber_crm/services/core/internal/store"
)

// period разбирает from/to (по умолчанию — последние 30 дней).
func period(qp *query) (time.Time, time.Time) {
	to := moscowNow()
	from := to.AddDate(0, 0, -30)
	if t := qp.optTime("from"); t != nil {
		from = *t
	}
	if t := qp.optTime("to"); t != nil {
		to = *t
	}
	if !from.Before(to) {
		qp.errs["to"] = "конец периода должен быть позже начала"
	}
	return from, to
}

func (a *API) report(w http.ResponseWriter, r *http.Request, qp *query, file string, args ...any) {
	if err := qp.err(); err != nil {
		a.fail(w, r, err)
		return
	}
	rep, err := a.s.RunReport(r.Context(), file, args...)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func requireInt(qp *query, name string) int {
	v := qp.optInt(name)
	if v == nil {
		if _, ok := qp.errs[name]; !ok {
			qp.errs[name] = "обязательный параметр"
		}
		return 0
	}
	return *v
}

func (a *API) reportDaySchedule(w http.ResponseWriter, r *http.Request) {
	qp := q(r)
	barber := requireInt(qp, "barber_id")
	date := qp.date("date", moscowNow())
	a.report(w, r, qp, "q01_barber_day_schedule.sql", barber, date)
}

func (a *API) reportClientHistory(w http.ResponseWriter, r *http.Request) {
	qp := q(r)
	client := requireInt(qp, "client_id")
	a.report(w, r, qp, "q02_client_history.sql", client)
}

func (a *API) reportRevenueByBranch(w http.ResponseWriter, r *http.Request) {
	qp := q(r)
	to := moscowNow()
	from := time.Date(to.Year(), to.Month(), 1, 0, 0, 0, 0, moscow).AddDate(0, -5, 0)
	if t := qp.optTime("from"); t != nil {
		from = *t
	}
	if t := qp.optTime("to"); t != nil {
		to = *t
	}
	a.report(w, r, qp, "q03_revenue_by_branch_month.sql", from, to)
}

func (a *API) reportBarberRanking(w http.ResponseWriter, r *http.Request) {
	qp := q(r)
	from, to := period(qp)
	a.report(w, r, qp, "q04_barber_ranking.sql", from, to)
}

func (a *API) reportRFM(w http.ResponseWriter, r *http.Request) {
	qp := q(r)
	asOf := moscowNow()
	if t := qp.optTime("as_of"); t != nil {
		asOf = *t
	}
	a.report(w, r, qp, "q05_rfm_segmentation.sql", asOf)
}

func (a *API) reportTopServices(w http.ResponseWriter, r *http.Request) {
	qp := q(r)
	from, to := period(qp)
	minCount := 1
	if v := qp.optInt("min_count"); v != nil {
		minCount = *v
	}
	a.report(w, r, qp, "q06_top_services.sql", from, to, minCount)
}

func (a *API) reportChurnRisk(w http.ResponseWriter, r *http.Request) {
	qp := q(r)
	asOf := moscowNow()
	if t := qp.optTime("as_of"); t != nil {
		asOf = *t
	}
	minVisits, days := 3, 45
	if v := qp.optInt("min_visits"); v != nil {
		minVisits = *v
	}
	if v := qp.optInt("days"); v != nil {
		days = *v
	}
	a.report(w, r, qp, "q07_churn_risk.sql", asOf, minVisits, days)
}

func (a *API) reportNoShows(w http.ResponseWriter, r *http.Request) {
	qp := q(r)
	from := moscowNow().AddDate(0, -3, 0)
	if t := qp.optTime("from"); t != nil {
		from = *t
	}
	minNoShows := 2
	if v := qp.optInt("min_no_shows"); v != nil {
		minNoShows = *v
	}
	a.report(w, r, qp, "q08_no_show_offenders.sql", from, minNoShows)
}

func (a *API) reportUtilization(w http.ResponseWriter, r *http.Request) {
	qp := q(r)
	now := moscowNow()
	from := qp.date("from", now.AddDate(0, 0, -28))
	to := qp.date("to", now)
	a.report(w, r, qp, "q09_utilization_weekly.sql", from, to, qp.optInt("branch_id"))
}

func (a *API) reportCohorts(w http.ResponseWriter, r *http.Request) {
	qp := q(r)
	from := qp.date("from", moscowNow().AddDate(0, -6, 0))
	a.report(w, r, qp, "q10_cohort_retention.sql", from)
}

func (a *API) reportDailyRevenue(w http.ResponseWriter, r *http.Request) {
	qp := q(r)
	month := qp.date("month", moscowNow())
	a.report(w, r, qp, "q11_daily_revenue.sql", month, qp.optInt("branch_id"))
}

func (a *API) reportKPI(w http.ResponseWriter, r *http.Request) {
	qp := q(r)
	month := qp.optTime("month")
	barber := qp.optInt("barber_id")
	if err := qp.err(); err != nil {
		a.fail(w, r, err)
		return
	}
	rep, err := a.s.KPI(r.Context(), month, barber)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (a *API) refreshKPI(w http.ResponseWriter, r *http.Request) {
	ts, err := a.s.RefreshKPI(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"refreshed_at": ts})
}

// POST /import/clients?source=name.csv — тело запроса: CSV (UTF-8, заголовок обязателен).
func (a *API) importClients(w http.ResponseWriter, r *http.Request) {
	source := q(r).str("source")
	if source == "" {
		source = "api-upload.csv"
	}
	res, err := a.s.ImportClientsCSV(r.Context(), source, http.MaxBytesReader(w, r.Body, 10<<20))
	if err != nil {
		if apperrIsCSV(err) {
			err = apperr.BadRequest(err.Error())
		}
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func apperrIsCSV(err error) bool {
	for e := err; e != nil; {
		if e == store.ErrBadCSV {
			return true
		}
		u, ok := e.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		e = u.Unwrap()
	}
	return false
}

var auditSort = map[string]string{"changed_at": "changed_at", "audit_id": "audit_id"}

func (a *API) listAudit(w http.ResponseWriter, r *http.Request) {
	qp := q(r)
	f := store.AuditFilter{Table: qp.str("table"), Operation: qp.str("operation"),
		RowID: qp.optInt64("row_id"), From: qp.optTime("from")}
	if f.Operation != "" {
		v := newV()
		v.oneOf("operation", f.Operation, "INSERT", "UPDATE", "DELETE")
		if err := v.err(); err != nil {
			a.fail(w, r, err)
			return
		}
	}
	p := qp.list(auditSort, "-audit_id")
	if err := qp.err(); err != nil {
		a.fail(w, r, err)
		return
	}
	res, err := a.s.ListAudit(r.Context(), f, p)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
