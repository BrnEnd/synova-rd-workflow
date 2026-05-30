package domain

import "time"

// Deal represents a CRM deal (negociação) from RD Station.
type Deal struct {
	ID        string
	Name      string
	Stage     Stage
	Owner     DealOwner
	Contacts  []Contact
	CreatedAt time.Time
	UpdatedAt time.Time
}

// DealOwner identifies the RD Station user responsible for a deal.
type DealOwner struct {
	ID    string
	Name  string
	Email string
}

// Stage represents a pipeline stage in RD Station CRM.
type Stage struct {
	ID   string
	Name string
}

// Activity represents a manual annotation registered in a deal.
type Activity struct {
	ID   string
	Text string
	Date string
}
