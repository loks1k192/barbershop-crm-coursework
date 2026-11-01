package store_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/store"
)

func ptr[T any](v T) *T { return &v }

func TestBranchCRUD(t *testing.T) {
	c := ctx(t)
	b, err := st.CreateBranch(c, store.BranchInput{Name: "Филиал В", Address: "ул. Новая, 3",
		Phone: "+74950000003", OpensAt: "08:00", ClosesAt: "20:00"})
	must(t, err)
	if !b.IsActive || b.OpensAt != "08:00" {
		t.Fatalf("неожиданные данные: %+v", b)
	}
	_, err = st.CreateBranch(c, store.BranchInput{Name: "Филиал В", Address: "x", Phone: "+74950000009",
		OpensAt: "08:00", ClosesAt: "20:00"})
	wantHTTP(t, err, http.StatusConflict) // UNIQUE name

	b2, err := st.UpdateBranch(c, b.ID, store.BranchInput{Name: "Филиал В", Address: "ул. Новая, 5",
		Phone: "+74950000003", OpensAt: "09:00", ClosesAt: "21:00", IsActive: ptr(false)})
	must(t, err)
	if b2.Address != "ул. Новая, 5" || b2.IsActive || !b2.UpdatedAt.After(b.UpdatedAt) {
		t.Fatalf("обновление не применилось: %+v", b2)
	}
	page, err := st.ListBranches(c, store.BranchFilter{Active: ptr(false)},
		store.ListParams{Page: 1, PageSize: 10, Sort: "name ASC"})
	must(t, err)
	if page.Total != 1 || page.Items[0].ID != b.ID {
		t.Fatalf("фильтр active=false: %+v", page)
	}
	must(t, st.DeleteBranch(c, b.ID))
	_, err = st.GetBranch(c, b.ID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("ожидался ErrNoRows, получено %v", err)
	}
	if err := st.DeleteBranch(c, b.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("повторное удаление: %v", err)
	}
}

func TestClientCRUDAndListing(t *testing.T) {
	c := ctx(t)
	cl, err := st.CreateClient(c, store.ClientInput{Phone: "+79161110000", LastName: ptr("Сидоров"),
		FirstName: "Олег", MiddleName: ptr("Петрович"), Email: ptr("Oleg@Example.com"), BirthDate: ptr("1990-05-01")})
	must(t, err)
	if cl.FullName != "Сидоров Олег Петрович" || *cl.Email != "oleg@example.com" || cl.Level != "bronze" {
		t.Fatalf("клиент: %+v", cl)
	}
	// бонусный счёт создан триггером (1:1)
	if n := scalar[int](t, `SELECT count(*) FROM crm.loyalty_accounts WHERE client_id = $1`, cl.ID); n != 1 {
		t.Fatal("бонусный счёт не создан")
	}
	cl2, err := st.UpdateClient(c, cl.ID, store.ClientInput{Phone: "+79161110000", LastName: ptr("Сидоров"),
		FirstName: "Олег", Notes: "VIP", MarketingConsent: true})
	must(t, err)
	if cl2.Notes != "VIP" || !cl2.MarketingConsent || cl2.Email != nil {
		t.Fatalf("обновление: %+v", cl2)
	}
	// изменение попало в аудит с перечнем изменённых столбцов
	cols := scalar[[]string](t, `SELECT changed_columns FROM audit.audit_log
		WHERE table_name = 'crm.clients' AND operation = 'UPDATE' AND row_pk->>'client_id' = ($1::bigint)::text
		ORDER BY audit_id DESC LIMIT 1`, cl.ID)
	if len(cols) == 0 {
		t.Fatal("нет записи аудита")
	}

	// фильтрация + сортировка + пагинация
	p1, err := st.ListClients(c, store.ClientFilter{Search: "Клиент"}, store.ListParams{Page: 1, PageSize: 2, Sort: "c.client_id ASC"})
	must(t, err)
	p2, err := st.ListClients(c, store.ClientFilter{Search: "Клиент"}, store.ListParams{Page: 2, PageSize: 2, Sort: "c.client_id ASC"})
	must(t, err)
	if p1.Total != 4 || len(p1.Items) != 2 || len(p2.Items) != 2 || p1.Items[1].ID >= p2.Items[0].ID {
		t.Fatalf("пагинация: total=%d p1=%d p2=%d", p1.Total, len(p1.Items), len(p2.Items))
	}
	top, err := st.ListClients(c, store.ClientFilter{MinVisits: ptr(2)}, store.ListParams{Page: 1, PageSize: 10, Sort: "st.visits_count DESC, c.client_id"})
	must(t, err)
	for i, it := range top.Items {
		if it.VisitsCount < 2 || (i > 0 && it.VisitsCount > top.Items[i-1].VisitsCount) {
			t.Fatalf("min_visits/сортировка нарушены: %+v", top.Items)
		}
	}
	if top.Total == 0 {
		t.Fatal("min_visits: пустой результат")
	}

	must(t, st.DeleteClient(c, cl.ID)) // без истории — удаляется (каскадно и бонусный счёт)
	if n := scalar[int](t, `SELECT count(*) FROM crm.loyalty_accounts WHERE client_id = $1`, cl.ID); n != 0 {
		t.Fatal("бонусный счёт не удалён каскадно")
	}
}

func TestBarberAndPriceList(t *testing.T) {
	c := ctx(t)
	b, err := st.CreateBarber(c, store.BarberInput{BranchID: 2, FullName: "Новый Мастер", Phone: "+79000000099",
		Login: "newbie", PasswordHash: "hash", Grade: "junior"})
	must(t, err)
	s, err := st.GetBarberSettings(c, b.ID) // создано триггером (1:1)
	must(t, err)
	if s.SlotStepMin != 15 {
		t.Fatalf("настройки по умолчанию: %+v", s)
	}
	bs, err := st.UpsertBarberService(c, b.ID, 1, store.BarberServiceInput{Price: 900, DurationMin: 60})
	must(t, err)
	bs, err = st.UpsertBarberService(c, b.ID, 1, store.BarberServiceInput{Price: 950, DurationMin: 55})
	must(t, err)
	if bs.Price != 950 || bs.DurationMin != 55 {
		t.Fatalf("upsert прайса: %+v", bs)
	}
	_, err = st.UpsertBarberService(c, b.ID, 2, store.BarberServiceInput{Price: 500, DurationMin: 33})
	wantHTTP(t, err, http.StatusUnprocessableEntity) // CHECK кратности 5
	list, err := st.ListBarberServices(c, b.ID, true)
	must(t, err)
	if len(list) != 1 {
		t.Fatalf("прайс: %+v", list)
	}
	must(t, st.DeleteBarber(c, b.ID)) // без истории; прайс и настройки — каскадно
}

func TestShiftCRUD(t *testing.T) {
	c := ctx(t)
	sh, err := st.CreateShift(c, store.ShiftInput{BarberID: 2, BranchID: 2, WorkDate: "2026-12-01", StartsAt: "12:00", EndsAt: "20:00"})
	must(t, err)
	_, err = st.CreateShift(c, store.ShiftInput{BarberID: 2, BranchID: 1, WorkDate: "2026-12-01", StartsAt: "10:00", EndsAt: "18:00"})
	wantHTTP(t, err, http.StatusConflict) // UNIQUE (barber_id, work_date)
	_, err = st.UpdateShift(c, sh.ID, store.ShiftInput{BarberID: 2, BranchID: 2, WorkDate: "2026-12-01", StartsAt: "12:00", EndsAt: "23:00"})
	wantHTTP(t, err, http.StatusUnprocessableEntity) // после закрытия филиала
	page, err := st.ListShifts(c, store.ShiftFilter{BarberID: ptr(2), DateFrom: "2026-12-01"}, store.ListParams{Page: 1, PageSize: 5, Sort: "ws.work_date"})
	must(t, err)
	if page.Total != 1 {
		t.Fatalf("фильтр смен: %+v", page)
	}
	must(t, st.DeleteShift(c, sh.ID))
}
