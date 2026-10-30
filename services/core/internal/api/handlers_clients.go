package api

import (
	"net/http"
	"strings"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/model"
	"github.com/RomanKovalev007/barber_crm/services/core/internal/store"
)

var clientSort = map[string]string{"full_name": "c.full_name", "client_id": "c.client_id",
	"created_at": "c.created_at", "visits": "st.visits_count", "total_spent": "st.total_spent",
	"last_visit": "st.last_visit_at"}

func validateClient(in *store.ClientInput) error {
	in.FirstName = strings.TrimSpace(in.FirstName)
	in.LastName, in.MiddleName, in.Email = trimPtr(in.LastName), trimPtr(in.MiddleName), trimPtr(in.Email)
	in.BirthDate = trimPtr(in.BirthDate)
	v := newV()
	v.phone("phone", in.Phone)
	v.required("first_name", in.FirstName)
	v.maxLen("first_name", in.FirstName, 60)
	v.optEmail("email", in.Email)
	v.optDate("birth_date", in.BirthDate)
	if in.BirthDate != nil && v.err() == nil {
		v.check(*in.BirthDate >= "1900-01-01" && *in.BirthDate <= moscowNow().Format("2006-01-02"),
			"birth_date", "дата рождения вне допустимого диапазона")
	}
	v.maxLen("notes", in.Notes, 2000)
	return v.err()
}

func (a *API) listClients(w http.ResponseWriter, r *http.Request) {
	qp := q(r)
	f := store.ClientFilter{Search: qp.str("search"), Level: qp.str("level"),
		Consent: qp.optBool("marketing_consent"), MinVisits: qp.optInt("min_visits")}
	if f.Level != "" {
		v := newV()
		v.oneOf("level", f.Level, "bronze", "silver", "gold")
		if err := v.err(); err != nil {
			a.fail(w, r, err)
			return
		}
	}
	p := qp.list(clientSort, "full_name")
	if err := qp.err(); err != nil {
		a.fail(w, r, err)
		return
	}
	res, err := a.s.ListClients(r.Context(), f, p)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *API) getClient(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err == nil {
		var c *model.Client
		if c, err = a.s.GetClient(r.Context(), id); err == nil {
			writeJSON(w, http.StatusOK, c)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) createClient(w http.ResponseWriter, r *http.Request) {
	var in store.ClientInput
	err := decode(r, &in)
	if err == nil {
		err = validateClient(&in)
	}
	if err == nil {
		var c *model.Client
		if c, err = a.s.CreateClient(r.Context(), in); err == nil {
			writeJSON(w, http.StatusCreated, c)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) updateClient(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	var in store.ClientInput
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil {
		err = validateClient(&in)
	}
	if err == nil {
		var c *model.Client
		if c, err = a.s.UpdateClient(r.Context(), id, in); err == nil {
			writeJSON(w, http.StatusOK, c)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) deleteClient(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err == nil {
		err = a.s.DeleteClient(r.Context(), id)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) clientAppointments(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	qp := q(r)
	p := qp.list(appointmentSort, "-starts_at")
	if err := qp.err(); err != nil {
		a.fail(w, r, err)
		return
	}
	res, err := a.s.ListAppointments(r.Context(), store.AppointmentFilter{ClientID: &id}, p)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *API) clientBonusHistory(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err == nil {
		var res []model.BonusTx
		if res, err = a.s.ClientBonusHistory(r.Context(), id); err == nil {
			writeJSON(w, http.StatusOK, res)
			return
		}
	}
	a.fail(w, r, err)
}
