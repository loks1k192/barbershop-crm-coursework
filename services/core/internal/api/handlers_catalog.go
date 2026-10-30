package api

import (
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/model"
	"github.com/RomanKovalev007/barber_crm/services/core/internal/store"
)

// ── Филиалы ─────────────────────────────────────────────────────────────────

var branchSort = map[string]string{"name": "name", "branch_id": "branch_id", "created_at": "created_at"}

func validateBranch(in *store.BranchInput) error {
	in.Name, in.Address = strings.TrimSpace(in.Name), strings.TrimSpace(in.Address)
	v := newV()
	v.required("name", in.Name)
	v.maxLen("name", in.Name, 100)
	v.required("address", in.Address)
	v.phone("phone", in.Phone)
	v.clock("opens_at", in.OpensAt)
	v.clock("closes_at", in.ClosesAt)
	if v.err() == nil {
		v.check(in.ClosesAt > in.OpensAt, "closes_at", "должно быть позже opens_at")
	}
	return v.err()
}

func (a *API) listBranches(w http.ResponseWriter, r *http.Request) {
	qp := q(r)
	f := store.BranchFilter{Active: qp.optBool("active"), Search: qp.str("search")}
	p := qp.list(branchSort, "name")
	if err := qp.err(); err != nil {
		a.fail(w, r, err)
		return
	}
	res, err := a.s.ListBranches(r.Context(), f, p)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *API) getBranch(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err == nil {
		var b *model.Branch
		if b, err = a.s.GetBranch(r.Context(), int(id)); err == nil {
			writeJSON(w, http.StatusOK, b)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) createBranch(w http.ResponseWriter, r *http.Request) {
	var in store.BranchInput
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := validateBranch(&in); err != nil {
		a.fail(w, r, err)
		return
	}
	b, err := a.s.CreateBranch(r.Context(), in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (a *API) updateBranch(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	var in store.BranchInput
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil {
		err = validateBranch(&in)
	}
	if err == nil {
		var b *model.Branch
		if b, err = a.s.UpdateBranch(r.Context(), int(id), in); err == nil {
			writeJSON(w, http.StatusOK, b)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) deleteBranch(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err == nil {
		err = a.s.DeleteBranch(r.Context(), int(id))
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Категории ───────────────────────────────────────────────────────────────

func (a *API) listCategories(w http.ResponseWriter, r *http.Request) {
	res, err := a.s.ListCategories(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func validateCategory(in *store.CategoryInput) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = trimPtr(in.Description)
	v := newV()
	v.required("name", in.Name)
	v.maxLen("name", in.Name, 100)
	return v.err()
}

func (a *API) createCategory(w http.ResponseWriter, r *http.Request) {
	var in store.CategoryInput
	err := decode(r, &in)
	if err == nil {
		err = validateCategory(&in)
	}
	if err == nil {
		var c *model.Category
		if c, err = a.s.CreateCategory(r.Context(), in); err == nil {
			writeJSON(w, http.StatusCreated, c)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) updateCategory(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	var in store.CategoryInput
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil {
		err = validateCategory(&in)
	}
	if err == nil {
		var c *model.Category
		if c, err = a.s.UpdateCategory(r.Context(), int(id), in); err == nil {
			writeJSON(w, http.StatusOK, c)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) deleteCategory(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err == nil {
		err = a.s.DeleteCategory(r.Context(), int(id))
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Услуги ──────────────────────────────────────────────────────────────────

var serviceSort = map[string]string{"name": "s.name", "service_id": "s.service_id",
	"base_price": "s.base_price", "category": "c.name"}

func validateService(in *store.ServiceInput) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = trimPtr(in.Description)
	v := newV()
	v.positive("category_id", in.CategoryID)
	v.required("name", in.Name)
	v.maxLen("name", in.Name, 100)
	v.check(in.BasePrice > 0 && in.BasePrice < 1_000_000, "base_price", "должна быть от 0 до 1 000 000")
	v.between("base_duration_min", in.BaseDurationMin, 5, 480)
	v.check(in.BaseDurationMin%5 == 0, "base_duration_min", "должна быть кратна 5 минутам")
	return v.err()
}

func (a *API) listServices(w http.ResponseWriter, r *http.Request) {
	qp := q(r)
	f := store.ServiceFilter{CategoryID: qp.optInt("category_id"), Active: qp.optBool("active"), Search: qp.str("search")}
	p := qp.list(serviceSort, "category,name")
	if err := qp.err(); err != nil {
		a.fail(w, r, err)
		return
	}
	res, err := a.s.ListServices(r.Context(), f, p)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *API) getService(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err == nil {
		var s *model.Service
		if s, err = a.s.GetService(r.Context(), int(id)); err == nil {
			writeJSON(w, http.StatusOK, s)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) createService(w http.ResponseWriter, r *http.Request) {
	var in store.ServiceInput
	err := decode(r, &in)
	if err == nil {
		err = validateService(&in)
	}
	if err == nil {
		var s *model.Service
		if s, err = a.s.CreateService(r.Context(), in); err == nil {
			writeJSON(w, http.StatusCreated, s)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) updateService(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	var in store.ServiceInput
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil {
		err = validateService(&in)
	}
	if err == nil {
		var s *model.Service
		if s, err = a.s.UpdateService(r.Context(), int(id), in); err == nil {
			writeJSON(w, http.StatusOK, s)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) deleteService(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err == nil {
		err = a.s.DeleteService(r.Context(), int(id))
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Мастера ─────────────────────────────────────────────────────────────────

var barberSort = map[string]string{"full_name": "b.full_name", "barber_id": "b.barber_id",
	"grade": "b.grade", "hired_at": "b.hired_at", "branch": "br.name"}

func validateBarber(in *store.BarberInput, creating bool) error {
	in.FullName, in.Login = strings.TrimSpace(in.FullName), strings.TrimSpace(in.Login)
	v := newV()
	v.positive("branch_id", in.BranchID)
	v.required("full_name", in.FullName)
	v.maxLen("full_name", in.FullName, 150)
	v.phone("phone", in.Phone)
	v.check(loginRe.MatchString(in.Login), "login", "латиница/цифры/_/., 3–32 символа, начинается с буквы")
	if creating || in.Password != "" {
		v.check(len(in.Password) >= 8 && len(in.Password) <= 72, "password", "от 8 до 72 символов")
	}
	v.oneOf("grade", in.Grade, "junior", "middle", "senior", "top")
	v.optDate("hired_at", in.HiredAt)
	v.optDate("fired_at", in.FiredAt)
	return v.err()
}

func (a *API) listBarbers(w http.ResponseWriter, r *http.Request) {
	qp := q(r)
	f := store.BarberFilter{BranchID: qp.optInt("branch_id"), Grade: qp.str("grade"),
		Active: qp.optBool("active"), Search: qp.str("search")}
	p := qp.list(barberSort, "branch,full_name")
	if err := qp.err(); err != nil {
		a.fail(w, r, err)
		return
	}
	res, err := a.s.ListBarbers(r.Context(), f, p)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *API) getBarber(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err == nil {
		var b *model.Barber
		if b, err = a.s.GetBarber(r.Context(), int(id)); err == nil {
			writeJSON(w, http.StatusOK, b)
			return
		}
	}
	a.fail(w, r, err)
}

func hashPassword(in *store.BarberInput) error {
	if in.Password == "" {
		return nil
	}
	h, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	in.PasswordHash, in.Password = string(h), ""
	return nil
}

func (a *API) createBarber(w http.ResponseWriter, r *http.Request) {
	var in store.BarberInput
	err := decode(r, &in)
	if err == nil {
		err = validateBarber(&in, true)
	}
	if err == nil {
		err = hashPassword(&in)
	}
	if err == nil {
		var b *model.Barber
		if b, err = a.s.CreateBarber(r.Context(), in); err == nil {
			writeJSON(w, http.StatusCreated, b)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) updateBarber(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	var in store.BarberInput
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil {
		err = validateBarber(&in, false)
	}
	if err == nil {
		err = hashPassword(&in)
	}
	if err == nil {
		var b *model.Barber
		if b, err = a.s.UpdateBarber(r.Context(), int(id), in); err == nil {
			writeJSON(w, http.StatusOK, b)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) deleteBarber(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err == nil {
		err = a.s.DeleteBarber(r.Context(), int(id))
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) getBarberSettings(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err == nil {
		var s *model.BarberSettings
		if s, err = a.s.GetBarberSettings(r.Context(), int(id)); err == nil {
			writeJSON(w, http.StatusOK, s)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) updateBarberSettings(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	var in struct {
		SlotStepMin  int  `json:"slot_step_min"`
		CompactSlots bool `json:"compact_slots"`
		MinLeadMin   int  `json:"min_lead_min"`
		MaxDaysAhead int  `json:"max_days_ahead"`
	}
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil {
		v := newV()
		v.oneOf("slot_step_min", itoa(in.SlotStepMin), "5", "10", "15", "20", "30", "60")
		v.between("min_lead_min", in.MinLeadMin, 0, 2880)
		v.between("max_days_ahead", in.MaxDaysAhead, 1, 180)
		err = v.err()
	}
	if err == nil {
		var s *model.BarberSettings
		if s, err = a.s.UpdateBarberSettings(r.Context(), model.BarberSettings{BarberID: int(id),
			SlotStepMin: in.SlotStepMin, CompactSlots: in.CompactSlots, MinLeadMin: in.MinLeadMin,
			MaxDaysAhead: in.MaxDaysAhead}); err == nil {
			writeJSON(w, http.StatusOK, s)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) listBarberServices(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err == nil {
		qp := q(r)
		active := qp.optBool("active")
		if err = qp.err(); err == nil {
			var res []model.BarberService
			if res, err = a.s.ListBarberServices(r.Context(), int(id), active != nil && *active); err == nil {
				writeJSON(w, http.StatusOK, res)
				return
			}
		}
	}
	a.fail(w, r, err)
}

func (a *API) upsertBarberService(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	var sid int64
	if err == nil {
		sid, err = pathInt(r, "service_id")
	}
	var in store.BarberServiceInput
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil {
		v := newV()
		v.check(in.Price > 0 && in.Price < 1_000_000, "price", "должна быть от 0 до 1 000 000")
		v.between("duration_min", in.DurationMin, 5, 480)
		v.check(in.DurationMin%5 == 0, "duration_min", "должна быть кратна 5 минутам")
		err = v.err()
	}
	if err == nil {
		var res *model.BarberService
		if res, err = a.s.UpsertBarberService(r.Context(), int(id), int(sid), in); err == nil {
			writeJSON(w, http.StatusOK, res)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) deleteBarberService(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	var sid int64
	if err == nil {
		sid, err = pathInt(r, "service_id")
	}
	if err == nil {
		err = a.s.DeleteBarberService(r.Context(), int(id), int(sid))
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Смены ───────────────────────────────────────────────────────────────────

var shiftSort = map[string]string{"work_date": "ws.work_date", "barber": "b.full_name",
	"branch": "br.name", "starts_at": "ws.starts_at", "shift_id": "ws.shift_id"}

func validateShift(in *store.ShiftInput) error {
	v := newV()
	v.positive("barber_id", in.BarberID)
	v.positive("branch_id", in.BranchID)
	v.date("work_date", in.WorkDate)
	v.clock("starts_at", in.StartsAt)
	v.clock("ends_at", in.EndsAt)
	if v.err() == nil {
		v.check(in.EndsAt > in.StartsAt, "ends_at", "должно быть позже starts_at")
	}
	return v.err()
}

func (a *API) listShifts(w http.ResponseWriter, r *http.Request) {
	qp := q(r)
	f := store.ShiftFilter{BarberID: qp.optInt("barber_id"), BranchID: qp.optInt("branch_id")}
	if qp.str("date_from") != "" {
		f.DateFrom = qp.date("date_from", moscowNow())
	}
	if qp.str("date_to") != "" {
		f.DateTo = qp.date("date_to", moscowNow())
	}
	p := qp.list(shiftSort, "work_date,barber")
	if err := qp.err(); err != nil {
		a.fail(w, r, err)
		return
	}
	res, err := a.s.ListShifts(r.Context(), f, p)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *API) getShift(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err == nil {
		var s *model.Shift
		if s, err = a.s.GetShift(r.Context(), id); err == nil {
			writeJSON(w, http.StatusOK, s)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) createShift(w http.ResponseWriter, r *http.Request) {
	var in store.ShiftInput
	err := decode(r, &in)
	if err == nil {
		err = validateShift(&in)
	}
	if err == nil {
		var s *model.Shift
		if s, err = a.s.CreateShift(r.Context(), in); err == nil {
			writeJSON(w, http.StatusCreated, s)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) updateShift(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	var in store.ShiftInput
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil {
		err = validateShift(&in)
	}
	if err == nil {
		var s *model.Shift
		if s, err = a.s.UpdateShift(r.Context(), id, in); err == nil {
			writeJSON(w, http.StatusOK, s)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) deleteShift(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err == nil {
		err = a.s.DeleteShift(r.Context(), id)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
