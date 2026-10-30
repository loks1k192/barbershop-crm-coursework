// Package model — структуры данных API (json) и строк результатов SQL (db).
package model

import "time"

type Branch struct {
	ID        int       `json:"branch_id" db:"branch_id"`
	Name      string    `json:"name" db:"name"`
	Address   string    `json:"address" db:"address"`
	Phone     string    `json:"phone" db:"phone"`
	OpensAt   string    `json:"opens_at" db:"opens_at"`
	ClosesAt  string    `json:"closes_at" db:"closes_at"`
	IsActive  bool      `json:"is_active" db:"is_active"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type Barber struct {
	ID         int       `json:"barber_id" db:"barber_id"`
	BranchID   int       `json:"branch_id" db:"branch_id"`
	BranchName string    `json:"branch_name" db:"branch_name"`
	FullName   string    `json:"full_name" db:"full_name"`
	Phone      string    `json:"phone" db:"phone"`
	Login      string    `json:"login" db:"login"`
	Grade      string    `json:"grade" db:"grade"`
	HiredAt    string    `json:"hired_at" db:"hired_at"`
	FiredAt    *string   `json:"fired_at" db:"fired_at"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}

type BarberSettings struct {
	BarberID     int       `json:"barber_id" db:"barber_id"`
	SlotStepMin  int       `json:"slot_step_min" db:"slot_step_min"`
	CompactSlots bool      `json:"compact_slots" db:"compact_slots"`
	MinLeadMin   int       `json:"min_lead_min" db:"min_lead_min"`
	MaxDaysAhead int       `json:"max_days_ahead" db:"max_days_ahead"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type Category struct {
	ID          int     `json:"category_id" db:"category_id"`
	Name        string  `json:"name" db:"name"`
	Description *string `json:"description" db:"description"`
}

type Service struct {
	ID              int     `json:"service_id" db:"service_id"`
	CategoryID      int     `json:"category_id" db:"category_id"`
	CategoryName    string  `json:"category_name" db:"category_name"`
	Name            string  `json:"name" db:"name"`
	Description     *string `json:"description" db:"description"`
	BasePrice       float64 `json:"base_price" db:"base_price"`
	BaseDurationMin int     `json:"base_duration_min" db:"base_duration_min"`
	IsActive        bool    `json:"is_active" db:"is_active"`
}

type BarberService struct {
	BarberID    int     `json:"barber_id" db:"barber_id"`
	ServiceID   int     `json:"service_id" db:"service_id"`
	ServiceName string  `json:"service_name" db:"service_name"`
	Price       float64 `json:"price" db:"price"`
	DurationMin int     `json:"duration_min" db:"duration_min"`
	IsActive    bool    `json:"is_active" db:"is_active"`
}

type Shift struct {
	ID         int64  `json:"shift_id" db:"shift_id"`
	BarberID   int    `json:"barber_id" db:"barber_id"`
	BarberName string `json:"barber_name" db:"barber_name"`
	BranchID   int    `json:"branch_id" db:"branch_id"`
	BranchName string `json:"branch_name" db:"branch_name"`
	WorkDate   string `json:"work_date" db:"work_date"`
	StartsAt   string `json:"starts_at" db:"starts_at"`
	EndsAt     string `json:"ends_at" db:"ends_at"`
}

type Client struct {
	ID               int64      `json:"client_id" db:"client_id"`
	Phone            string     `json:"phone" db:"phone"`
	LastName         *string    `json:"last_name" db:"last_name"`
	FirstName        string     `json:"first_name" db:"first_name"`
	MiddleName       *string    `json:"middle_name" db:"middle_name"`
	FullName         string     `json:"full_name" db:"full_name"`
	Email            *string    `json:"email" db:"email"`
	BirthDate        *string    `json:"birth_date" db:"birth_date"`
	Notes            string     `json:"notes" db:"notes"`
	MarketingConsent bool       `json:"marketing_consent" db:"marketing_consent"`
	Level            string     `json:"loyalty_level" db:"level"`
	BonusBalance     float64    `json:"bonus_balance" db:"bonus_balance"`
	TotalSpent       float64    `json:"total_spent" db:"total_spent"`
	VisitsCount      int64      `json:"visits_count" db:"visits_count"`
	NoShowCount      int64      `json:"no_show_count" db:"no_show_count"`
	LastVisitAt      *time.Time `json:"last_visit_at" db:"last_visit_at"`
	NextVisitAt      *time.Time `json:"next_visit_at" db:"next_visit_at"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
}

type BonusTx struct {
	ID            int64     `json:"bonus_tx_id" db:"bonus_tx_id"`
	AppointmentID *int64    `json:"appointment_id" db:"appointment_id"`
	Kind          string    `json:"kind" db:"kind"`
	Amount        float64   `json:"amount" db:"amount"`
	Comment       *string   `json:"comment" db:"comment"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

type Appointment struct {
	ID               int64     `json:"appointment_id" db:"appointment_id"`
	StartsAt         time.Time `json:"starts_at" db:"starts_at"`
	EndsAt           time.Time `json:"ends_at" db:"ends_at"`
	Status           string    `json:"status" db:"status"`
	Source           string    `json:"source" db:"source"`
	ClientID         int64     `json:"client_id" db:"client_id"`
	ClientName       string    `json:"client_name" db:"client_name"`
	ClientPhone      string    `json:"client_phone" db:"client_phone"`
	BarberID         int       `json:"barber_id" db:"barber_id"`
	BarberName       string    `json:"barber_name" db:"barber_name"`
	BranchID         int       `json:"branch_id" db:"branch_id"`
	BranchName       string    `json:"branch_name" db:"branch_name"`
	Services         *string   `json:"services" db:"services"`
	TotalPrice       float64   `json:"total_price" db:"total_price"`
	TotalDurationMin int64     `json:"total_duration_min" db:"total_duration_min"`
	PaidAmount       float64   `json:"paid_amount" db:"paid_amount"`
	Rating           *int16    `json:"rating" db:"rating"`
	Comment          *string   `json:"comment" db:"comment"`
	CancelReason     *string   `json:"cancel_reason" db:"cancel_reason"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

type AppointmentService struct {
	ServiceID   int     `json:"service_id" db:"service_id"`
	Name        string  `json:"name" db:"name"`
	Price       float64 `json:"price" db:"price"`
	DurationMin int     `json:"duration_min" db:"duration_min"`
}

type StatusChange struct {
	ID        int64     `json:"history_id" db:"history_id"`
	OldStatus *string   `json:"old_status" db:"old_status"`
	NewStatus string    `json:"new_status" db:"new_status"`
	ChangedAt time.Time `json:"changed_at" db:"changed_at"`
	ChangedBy string    `json:"changed_by" db:"changed_by"`
}

type Slot struct {
	Start time.Time `json:"start" db:"slot_start"`
	End   time.Time `json:"end" db:"slot_end"`
}

type CompleteResult struct {
	TotalAmount   float64 `json:"total_amount" db:"total_amount"`
	BonusRedeemed float64 `json:"bonus_redeemed" db:"bonus_redeemed"`
	PaidMoney     float64 `json:"paid_money" db:"paid_money"`
	BonusAccrued  float64 `json:"bonus_accrued" db:"bonus_accrued"`
	BonusBalance  float64 `json:"bonus_balance" db:"bonus_balance"`
	LoyaltyLevel  string  `json:"loyalty_level" db:"loyalty_level"`
}

type Review struct {
	AppointmentID int64     `json:"appointment_id" db:"appointment_id"`
	Rating        int16     `json:"rating" db:"rating"`
	Comment       *string   `json:"comment" db:"comment"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

type ImportError struct {
	LineNo  int    `json:"line_no" db:"line_no"`
	Field   string `json:"field" db:"field"`
	Message string `json:"message" db:"message"`
}

type ImportResult struct {
	BatchID  int64         `json:"batch_id"`
	Total    int           `json:"total"`
	Inserted int           `json:"inserted"`
	Updated  int           `json:"updated"`
	Rejected int           `json:"rejected"`
	Errors   []ImportError `json:"errors"`
}

type AuditEntry struct {
	ID             int64          `json:"audit_id" db:"audit_id"`
	TableName      string         `json:"table_name" db:"table_name"`
	Operation      string         `json:"operation" db:"operation"`
	RowPK          map[string]any `json:"row_pk" db:"row_pk"`
	OldData        map[string]any `json:"old_data" db:"old_data"`
	NewData        map[string]any `json:"new_data" db:"new_data"`
	ChangedColumns []string       `json:"changed_columns" db:"changed_columns"`
	DBUser         string         `json:"db_user" db:"db_user"`
	AppUser        *string        `json:"app_user" db:"app_user"`
	ChangedAt      time.Time      `json:"changed_at" db:"changed_at"`
}

// Page — результат постраничного запроса коллекции.
type Page[T any] struct {
	Items    []T   `json:"items"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
}
