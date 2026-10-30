package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/model"
	"github.com/RomanKovalev007/barber_crm/services/core/internal/store"
)

var appointmentSort = map[string]string{"starts_at": "starts_at", "appointment_id": "appointment_id",
	"status": "status", "client_name": "client_name", "barber_name": "barber_name",
	"total_price": "total_price", "created_at": "created_at"}

var allStatuses = []string{"scheduled", "confirmed", "in_progress", "completed", "cancelled", "no_show"}

func moscowNow() time.Time { return time.Now().In(moscow) }

func (a *API) listAppointments(w http.ResponseWriter, r *http.Request) {
	qp := q(r)
	f := store.AppointmentFilter{BarberID: qp.optInt("barber_id"), ClientID: qp.optInt64("client_id"),
		BranchID: qp.optInt("branch_id"), From: qp.optTime("from"), To: qp.optTime("to")}
	if s := qp.str("status"); s != "" {
		v := newV()
		for _, st := range strings.Split(s, ",") {
			st = strings.TrimSpace(st)
			v.oneOf("status", st, allStatuses...)
			f.Status = append(f.Status, st)
		}
		if err := v.err(); err != nil {
			a.fail(w, r, err)
			return
		}
	}
	p := qp.list(appointmentSort, "-starts_at")
	if err := qp.err(); err != nil {
		a.fail(w, r, err)
		return
	}
	res, err := a.s.ListAppointments(r.Context(), f, p)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type appointmentDetail struct {
	*model.Appointment
	Items []model.AppointmentService `json:"items"`
}

func (a *API) getAppointment(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err == nil {
		var ap *model.Appointment
		if ap, err = a.s.GetAppointment(r.Context(), id); err == nil {
			var items []model.AppointmentService
			if items, err = a.s.AppointmentServices(r.Context(), id); err == nil {
				writeJSON(w, http.StatusOK, appointmentDetail{ap, items})
				return
			}
		}
	}
	a.fail(w, r, err)
}

// GET /barbers/{id}/free-slots?date=YYYY-MM-DD&service_ids=1,2[&ignore_lead=true]
func (a *API) freeSlots(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	qp := q(r)
	date := qp.date("date", moscowNow())
	ids := qp.intList("service_ids")
	if len(ids) == 0 && qp.errs["service_ids"] == "" {
		qp.errs["service_ids"] = "обязательный параметр"
	}
	ignoreLead := qp.optBool("ignore_lead")
	if err := qp.err(); err != nil {
		a.fail(w, r, err)
		return
	}
	slots, err := a.s.FreeSlots(r.Context(), int(id), date, ids, ignoreLead == nil || !*ignoreLead)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"barber_id": id, "date": date, "slots": slots})
}

// POST /appointments — сценарий 1: запись клиента.
func (a *API) bookAppointment(w http.ResponseWriter, r *http.Request) {
	var in store.BookInput
	err := decode(r, &in)
	if err == nil {
		in.ClientName = strings.TrimSpace(in.ClientName)
		in.Comment = trimPtr(in.Comment)
		if in.Source == "" {
			in.Source = "online"
		}
		v := newV()
		v.phone("client_phone", in.ClientPhone)
		v.required("client_name", in.ClientName)
		v.maxLen("client_name", in.ClientName, 150)
		v.positive("barber_id", in.BarberID)
		v.check(!in.StartsAt.IsZero(), "starts_at", "обязательное поле (RFC 3339)")
		v.check(in.StartsAt.Second() == 0 && in.StartsAt.Nanosecond() == 0, "starts_at", "время с точностью до минуты")
		v.check(len(in.ServiceIDs) > 0 && len(in.ServiceIDs) <= 5, "service_ids", "от 1 до 5 услуг")
		v.oneOf("source", in.Source, "online", "phone", "walk_in", "admin")
		err = v.err()
	}
	if err == nil {
		var ap *model.Appointment
		if ap, err = a.s.Book(r.Context(), in); err == nil {
			writeJSON(w, http.StatusCreated, ap)
			return
		}
	}
	a.fail(w, r, err)
}

// PATCH /appointments/{id} — изменение комментария.
func (a *API) patchAppointment(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	var in struct {
		Comment *string `json:"comment"`
	}
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil {
		var ap *model.Appointment
		if ap, err = a.s.UpdateComment(r.Context(), id, trimPtr(in.Comment)); err == nil {
			writeJSON(w, http.StatusOK, ap)
			return
		}
	}
	a.fail(w, r, err)
}

// PATCH /appointments/{id}/status — confirmed / in_progress / no_show.
func (a *API) setStatus(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	var in struct {
		Status string `json:"status"`
	}
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil {
		v := newV()
		v.oneOf("status", in.Status, "confirmed", "in_progress", "no_show")
		err = v.err()
	}
	if err == nil {
		var ap *model.Appointment
		if ap, err = a.s.SetStatus(r.Context(), id, in.Status); err == nil {
			writeJSON(w, http.StatusOK, ap)
			return
		}
	}
	a.fail(w, r, err)
}

// POST /appointments/{id}/cancel — сценарий 3: отмена с причиной.
func (a *API) cancelAppointment(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	var in struct {
		Reason string `json:"reason"`
	}
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil {
		v := newV()
		v.required("reason", in.Reason)
		v.maxLen("reason", in.Reason, 500)
		err = v.err()
	}
	if err == nil {
		var ap *model.Appointment
		if ap, err = a.s.Cancel(r.Context(), id, strings.TrimSpace(in.Reason)); err == nil {
			writeJSON(w, http.StatusOK, ap)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) rescheduleAppointment(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	var in struct {
		StartsAt time.Time `json:"starts_at"`
	}
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil {
		v := newV()
		v.check(!in.StartsAt.IsZero(), "starts_at", "обязательное поле (RFC 3339)")
		v.check(in.StartsAt.After(time.Now()), "starts_at", "перенос возможен только на будущее время")
		err = v.err()
	}
	if err == nil {
		var ap *model.Appointment
		if ap, err = a.s.Reschedule(r.Context(), id, in.StartsAt.Truncate(time.Minute)); err == nil {
			writeJSON(w, http.StatusOK, ap)
			return
		}
	}
	a.fail(w, r, err)
}

// POST /appointments/{id}/complete — сценарий 2: завершение визита, оплата, бонусы.
func (a *API) completeAppointment(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	var in struct {
		PaymentMethod string  `json:"payment_method"`
		BonusToRedeem float64 `json:"bonus_to_redeem"`
	}
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil {
		v := newV()
		v.oneOf("payment_method", in.PaymentMethod, "cash", "card", "online")
		v.check(in.BonusToRedeem >= 0, "bonus_to_redeem", "не может быть отрицательным")
		err = v.err()
	}
	if err == nil {
		var res *model.CompleteResult
		if res, err = a.s.Complete(r.Context(), id, in.PaymentMethod, in.BonusToRedeem); err == nil {
			writeJSON(w, http.StatusOK, res)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) appointmentHistory(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err == nil {
		if _, err = a.s.GetAppointment(r.Context(), id); err == nil {
			var res []model.StatusChange
			if res, err = a.s.AppointmentHistory(r.Context(), id); err == nil {
				writeJSON(w, http.StatusOK, res)
				return
			}
		}
	}
	a.fail(w, r, err)
}

func (a *API) deleteAppointment(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err == nil {
		err = a.s.DeleteAppointment(r.Context(), id)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /appointments/{id}/review — сценарий 4: отзыв о визите.
func (a *API) addReview(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	var in struct {
		Rating  int     `json:"rating"`
		Comment *string `json:"comment"`
	}
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil {
		v := newV()
		v.between("rating", in.Rating, 1, 5)
		if in.Comment != nil {
			v.maxLen("comment", *in.Comment, 2000)
		}
		err = v.err()
	}
	if err == nil {
		var rv *model.Review
		if rv, err = a.s.AddReview(r.Context(), id, int16(in.Rating), trimPtr(in.Comment)); err == nil {
			writeJSON(w, http.StatusCreated, rv)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) deleteReview(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err == nil {
		err = a.s.DeleteReview(r.Context(), id)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
