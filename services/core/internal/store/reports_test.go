package store_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/store"
)

var (
	march    = time.Date(2026, 3, 1, 0, 0, 0, 0, time.FixedZone("MSK", 3*3600))
	marchEnd = march.AddDate(0, 1, 0)
)

func rowsBy(rep *store.Report, key string) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, r := range rep.Rows {
		out[fmt.Sprint(r[key])] = r
	}
	return out
}

func eq(t *testing.T, what string, got, want any) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("%s: получено %v, ожидалось %v", what, got, want)
	}
}

// Q04 — рейтинг мастеров (комплексный отчёт), значения рассчитаны вручную по фикстуре.
func TestReportBarberRanking(t *testing.T) {
	rep, err := rst.RunReport(ctx(t), "q04_barber_ranking.sql", march, marchEnd)
	must(t, err)
	r := rowsBy(rep, "full_name")
	v := r["Артём Волков"]
	eq(t, "b1.completed", v["completed"], 4)
	eq(t, "b1.revenue", v["revenue"], 6200)
	eq(t, "b1.avg_check", v["avg_check"], 1550)
	eq(t, "b1.avg_rating", v["avg_rating"], 4.5)
	eq(t, "b1.utilization", v["utilization_pct"], 7.2)
	eq(t, "b1.rank_in_branch", v["rank_in_branch"], 1)
	eq(t, "b1.vs_branch_avg", v["revenue_vs_branch_avg"], 2250)
	eq(t, "b1.rating_rank", v["rating_rank"], 1)
	g := r["Глеб Глебов"]
	eq(t, "b2.revenue", g["revenue"], 1700)
	eq(t, "b2.no_show_pct", g["no_show_pct"], 66.7)
	eq(t, "b2.rank_in_branch", g["rank_in_branch"], 2)
	eq(t, "b2.rating_rank", g["rating_rank"], 3)
	a := r["Тимур Алиев"]
	eq(t, "b3.revenue", a["revenue"], 5000)
	eq(t, "b3.avg_rating", a["avg_rating"], 4)
	eq(t, "b3.rank_in_branch", a["rank_in_branch"], 1)
}

// Q06 — популярность услуг, доли внутри категории и HAVING.
func TestReportTopServices(t *testing.T) {
	rep, err := rst.RunReport(ctx(t), "q06_top_services.sql", march, marchEnd, 1)
	must(t, err)
	r := rowsBy(rep, "service")
	eq(t, "rows", len(rep.Rows), 3)
	eq(t, "Стрижка.times", r["Стрижка"]["times_sold"], 4)
	eq(t, "Стрижка.revenue", r["Стрижка"]["revenue"], 6000)
	eq(t, "Стрижка.share", r["Стрижка"]["share_in_category_pct"], 53.6)
	eq(t, "Комплекс.share", r["Комплекс"]["share_in_category_pct"], 46.4)
	eq(t, "Борода.revenue", r["Борода"]["revenue"], 1700)
	eq(t, "Борода.share", r["Борода"]["share_in_category_pct"], 100)

	rep, err = rst.RunReport(ctx(t), "q06_top_services.sql", march, marchEnd, 3)
	must(t, err)
	eq(t, "HAVING count >= 3", len(rep.Rows), 1)
}

// Q08 — неявки: GROUP BY + HAVING.
func TestReportNoShows(t *testing.T) {
	rep, err := rst.RunReport(ctx(t), "q08_no_show_offenders.sql", march, 2)
	must(t, err)
	eq(t, "rows", len(rep.Rows), 1)
	row := rep.Rows[0]
	eq(t, "client", row["full_name"], "Неявкин Пётр")
	eq(t, "no_shows", row["no_shows"], 2)
	eq(t, "visits_due", row["visits_due"], 3)
	eq(t, "pct", row["no_show_pct"], 66.7)
	eq(t, "lost", row["lost_revenue"], 2000)
}

// Q11 — нарастающий итог и скользящее среднее (дни без выручки присутствуют).
func TestReportDailyRevenue(t *testing.T) {
	rep, err := rst.RunReport(ctx(t), "q11_daily_revenue.sql", "2026-03-15", nil)
	must(t, err)
	eq(t, "days", len(rep.Rows), 31)
	r := rowsBy(rep, "day")
	eq(t, "02.03", r["2026-03-02"]["revenue"], 6700)
	eq(t, "03.03", r["2026-03-03"]["revenue"], 4700)
	eq(t, "04.03 running", r["2026-03-04"]["running_total"], 11400)
	eq(t, "05.03 running", r["2026-03-05"]["running_total"], 12900)
	eq(t, "07.03 moving avg", r["2026-03-07"]["moving_avg_7d"], 1843)
	eq(t, "31.03 running", r["2026-03-31"]["running_total"], 12900)

	rep, err = rst.RunReport(ctx(t), "q11_daily_revenue.sql", "2026-03-15", 1) // только филиал А
	must(t, err)
	eq(t, "branch A total", rep.Rows[30]["running_total"], 7900)
}

// Q07 — отток: NOT EXISTS исключает клиента с будущей записью.
func TestReportChurnRisk(t *testing.T) {
	asOf := time.Date(2026, 5, 1, 0, 0, 0, 0, time.FixedZone("MSK", 3*3600))
	rep, err := rst.RunReport(ctx(t), "q07_churn_risk.sql", asOf, 2, 30)
	must(t, err)
	eq(t, "rows", len(rep.Rows), 1)
	eq(t, "client", rep.Rows[0]["full_name"], "Клиент Первый")
	eq(t, "favourite", rep.Rows[0]["favourite_barber"], "Глеб Глебов")
	eq(t, "days", rep.Rows[0]["days_since_last"], 59)
}

// Остальные отчёты выполняются без ошибок и возвращают данные.
func TestReportsSmoke(t *testing.T) {
	cases := []struct {
		file string
		args []any
	}{
		{"q01_barber_day_schedule.sql", []any{1, "2026-03-02"}},
		{"q02_client_history.sql", []any{1}},
		{"q03_revenue_by_branch_month.sql", []any{march, marchEnd}},
		{"q05_rfm_segmentation.sql", []any{marchEnd}},
		{"q09_utilization_weekly.sql", []any{"2026-03-01", "2026-03-31", nil}},
		{"q10_cohort_retention.sql", []any{"2026-01-01"}},
	}
	for _, c := range cases {
		rep, err := rst.RunReport(ctx(t), c.file, c.args...)
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		if len(rep.Rows) == 0 {
			t.Errorf("%s: пустой результат", c.file)
		}
	}
	rep, err := rst.RunReport(ctx(t), "q01_barber_day_schedule.sql", 1, "2026-03-02")
	must(t, err)
	eq(t, "q01 rows", len(rep.Rows), 3)
	eq(t, "q01 first", rep.Rows[0]["time_from"], "10:00")

	kpi, err := rst.KPI(ctx(t), &march, nil)
	must(t, err)
	eq(t, "mv rows (март)", len(kpi.Rows), 3)
}
