package domain

import "time"

type AdminConfig struct {
	Email              string    `json:"email"`
	PasswordHash       string    `json:"-"`
	MustChangePassword bool      `json:"must_change_password"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type Collaborator struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	WhatsApp  string    `json:"whatsapp"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Alert struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	DealStageID         string    `json:"deal_stage_id"`
	DealStageName       string    `json:"deal_stage_name"`
	TimeThresholdHours  int       `json:"time_threshold_hours"`
	RepeatIntervalHours int       `json:"repeat_interval_hours"`
	MessageTemplate     string    `json:"message_template"`
	RecipientIDs        []string  `json:"recipient_ids"`
	Active              bool      `json:"active"`
	LastCheckedAt       time.Time `json:"last_checked_at,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type AllowlistEntry struct {
	ID          string    `json:"id"`
	PhoneNumber string    `json:"phone_number"`
	Label       string    `json:"label"`
	Active      bool      `json:"active"`
	SyncPending bool      `json:"sync_pending"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AlertSent struct {
	Key    string    `json:"key"`
	SentAt time.Time `json:"sent_at"`
	TTL    int64     `json:"ttl"`
}
