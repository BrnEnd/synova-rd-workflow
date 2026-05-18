package domain

import "time"

// Deal represents a CRM deal (negociação) from RD Station.
type Deal struct {
	ID        string
	Name      string
	Stage     Stage
	Contacts  []Contact
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Stage represents a pipeline stage in RD Station CRM.
type Stage struct {
	ID   string
	Name string
}
