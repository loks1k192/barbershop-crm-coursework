package api

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/apperr"
)

// Валидация входных данных на уровне API: формат, обязательность, диапазоны.
// Это быстрая обратная связь пользователю с понятными сообщениями по полям.
// Те же правила формата продублированы CHECK-ограничениями БД как последний
// рубеж; бизнес-правила (пересечения, смены, статусы) проверяет только СУБД.
type validator struct{ errs map[string]string }

func newV() *validator { return &validator{errs: map[string]string{}} }

var (
	phoneRe = regexp.MustCompile(`^\+7\d{10}$`)
	emailRe = regexp.MustCompile(`(?i)^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}$`)
	loginRe = regexp.MustCompile(`^[a-z][a-z0-9_.]{2,31}$`)
	timeRe  = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
)

func (v *validator) add(field, msg string) {
	if _, ok := v.errs[field]; !ok {
		v.errs[field] = msg
	}
}

func (v *validator) required(field, val string) bool {
	if strings.TrimSpace(val) == "" {
		v.add(field, "обязательное поле")
		return false
	}
	return true
}

func (v *validator) maxLen(field, val string, n int) {
	if len([]rune(val)) > n {
		v.add(field, "слишком длинное значение")
	}
}

func (v *validator) phone(field, val string) {
	if v.required(field, val) && !phoneRe.MatchString(val) {
		v.add(field, "формат +7XXXXXXXXXX")
	}
}

func (v *validator) optEmail(field string, val *string) {
	if val != nil && *val != "" && !emailRe.MatchString(*val) {
		v.add(field, "некорректный e-mail")
	}
}

func (v *validator) clock(field, val string) {
	if v.required(field, val) && !timeRe.MatchString(val) {
		v.add(field, "формат ЧЧ:ММ")
	}
}

func (v *validator) date(field, val string) {
	if v.required(field, val) {
		if _, err := time.Parse("2006-01-02", val); err != nil {
			v.add(field, "формат YYYY-MM-DD")
		}
	}
}

func (v *validator) optDate(field string, val *string) {
	if val != nil && *val != "" {
		v.date(field, *val)
	}
}

func (v *validator) positive(field string, val int) {
	if val <= 0 {
		v.add(field, "должно быть положительным")
	}
}

func (v *validator) between(field string, val, lo, hi int) {
	if val < lo || val > hi {
		v.add(field, "допустимо от "+itoa(lo)+" до "+itoa(hi))
	}
}

func (v *validator) oneOf(field, val string, allowed ...string) {
	for _, a := range allowed {
		if val == a {
			return
		}
	}
	v.add(field, "допустимые значения: "+strings.Join(allowed, ", "))
}

func (v *validator) check(cond bool, field, msg string) {
	if !cond {
		v.add(field, msg)
	}
}

func (v *validator) err() error {
	if len(v.errs) == 0 {
		return nil
	}
	return &apperr.Error{Status: http.StatusBadRequest, Code: "validation_error",
		Message: "Некорректные входные данные", Fields: v.errs}
}

func itoa(i int) string { return strconv.Itoa(i) }

func trimPtr(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}
