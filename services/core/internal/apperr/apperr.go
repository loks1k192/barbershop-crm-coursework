// Package apperr — ошибки приложения и преобразование ошибок СУБД в HTTP-ответы.
package apperr

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Error struct {
	Status  int               `json:"-"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
	SQLState string           `json:"sqlstate,omitempty"`
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

func New(status int, code, msg string) *Error { return &Error{Status: status, Code: code, Message: msg} }

func NotFound(what string) *Error {
	return New(http.StatusNotFound, "not_found", what+" не найден(а)")
}

func BadRequest(msg string) *Error { return New(http.StatusBadRequest, "bad_request", msg) }

// Коды бизнес-ошибок, поднимаемых функциями и триггерами БД (класс SQLSTATE 'CR').
var businessCodes = map[string]struct {
	status int
	code   string
}{
	"CR001": {http.StatusNotFound, "not_found"},
	"CR002": {http.StatusConflict, "invalid_state"},
	"CR003": {http.StatusUnprocessableEntity, "schedule_violation"},
	"CR004": {http.StatusConflict, "shift_has_appointments"},
	"CR005": {http.StatusUnprocessableEntity, "payment_error"},
	"CR006": {http.StatusUnprocessableEntity, "review_not_allowed"},
	"CR007": {http.StatusUnprocessableEntity, "invalid_booking"},
	"CR008": {http.StatusUnprocessableEntity, "booking_window"},
	"CR009": {http.StatusUnprocessableEntity, "insufficient_bonus"},
	"CR010": {http.StatusConflict, "import_error"},
}

// Понятные сообщения для именованных ограничений.
var constraintMessages = map[string]string{
	"appointments_barber_no_overlap": "Мастер уже занят в это время",
	"appointments_client_no_overlap": "У клиента уже есть запись на пересекающееся время",
	"appointments_cancel_reason_chk": "Причина обязательна для отменённой записи",
	"clients_phone_uq":               "Клиент с таким телефоном уже существует",
	"clients_email_uq":               "Клиент с таким e-mail уже существует",
	"clients_phone_chk":              "Телефон должен быть в формате +7XXXXXXXXXX",
	"clients_email_chk":              "Некорректный e-mail",
	"barbers_login_uq":               "Логин уже занят",
	"barbers_phone_uq":               "Мастер с таким телефоном уже существует",
	"barbers_login_chk":              "Логин: латиница, цифры, '_' и '.', 3–32 символа",
	"barbers_fired_chk":              "Дата увольнения не может быть раньше даты приёма",
	"branches_name_uq":               "Филиал с таким названием уже существует",
	"branches_hours_chk":             "Время закрытия должно быть позже времени открытия",
	"work_shifts_barber_date_uq":     "У мастера уже есть смена на эту дату",
	"work_shifts_time_chk":           "Окончание смены должно быть позже начала",
	"work_shifts_length_chk":         "Смена должна длиться не менее часа",
	"services_name_uq":               "Услуга с таким названием в категории уже существует",
	"service_categories_name_uq":     "Категория с таким названием уже существует",
	"barber_services_pkey":           "Услуга уже есть в прайсе мастера",
	"reviews_pkey":                   "Отзыв на этот визит уже оставлен",
	"reviews_rating_chk":             "Оценка должна быть от 1 до 5",
	"loyalty_balance_chk":            "Недостаточно бонусов на счёте",
	"barber_settings_step_chk":       "Шаг сетки: 5, 10, 15, 20, 30 или 60 минут",
}

// FromDB преобразует ошибку pgx/PostgreSQL в *Error.
func FromDB(err error) *Error {
	if err == nil {
		return nil
	}
	var ae *Error
	if errors.As(err, &ae) {
		return ae
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return NotFound("Объект")
	}
	var pg *pgconn.PgError
	if !errors.As(err, &pg) {
		return &Error{Status: http.StatusInternalServerError, Code: "internal", Message: "Внутренняя ошибка сервера"}
	}
	e := &Error{SQLState: pg.Code, Message: pg.Message}
	if bc, ok := businessCodes[pg.Code]; ok {
		e.Status, e.Code = bc.status, bc.code
		return e
	}
	if m, ok := constraintMessages[pg.ConstraintName]; ok {
		e.Message = m
	}
	switch pg.Code {
	case "23505": // unique_violation
		e.Status, e.Code = http.StatusConflict, "already_exists"
	case "23P01": // exclusion_violation
		e.Status, e.Code = http.StatusConflict, "time_slot_conflict"
	case "23503": // foreign_key_violation
		if strings.Contains(pg.Detail, "is still referenced") || strings.Contains(pg.Message, "update or delete") {
			e.Status, e.Code = http.StatusConflict, "in_use"
			e.Message = "Объект используется в других данных и не может быть удалён"
		} else {
			e.Status, e.Code = http.StatusUnprocessableEntity, "reference_not_found"
			e.Message = "Связанный объект не существует"
		}
	case "23514": // check_violation
		e.Status, e.Code = http.StatusUnprocessableEntity, "constraint_violation"
		if pg.ConstraintName == "loyalty_balance_chk" {
			e.Code = "insufficient_bonus"
		}
	case "23502": // not_null_violation
		e.Status, e.Code = http.StatusUnprocessableEntity, "constraint_violation"
		e.Message = "Не заполнено обязательное поле " + pg.ColumnName
	case "22023", "22P02", "22007", "22008", "22003", "22001":
		e.Status, e.Code = http.StatusBadRequest, "invalid_input"
	case "40001", "40P01":
		e.Status, e.Code = http.StatusServiceUnavailable, "concurrent_update"
		e.Message = "Конфликт параллельных транзакций, повторите запрос"
	case "55P03":
		e.Status, e.Code = http.StatusConflict, "locked"
	case "42501":
		e.Status, e.Code = http.StatusForbidden, "forbidden"
		e.Message = "Недостаточно прав для операции"
	case "57014":
		e.Status, e.Code = http.StatusGatewayTimeout, "timeout"
	default:
		e.Status, e.Code = http.StatusInternalServerError, "db_error"
		e.Message = "Ошибка базы данных"
	}
	return e
}
