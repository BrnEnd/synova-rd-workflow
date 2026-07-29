package domain

import "time"

// Deal represents a CRM deal (negociação) from RD Station.
type Deal struct {
	ID        string
	Name      string
	Stage     Stage
	Owner     DealOwner
	Contacts  []Contact
	Products  []DealProduct
	CreatedAt time.Time
	UpdatedAt time.Time
}

// DealSummaryContext groups CRM data used to generate an executive deal summary.
type DealSummaryContext struct {
	Deal           Deal
	Contacts       []Contact
	Activities     []Activity
	OpenTasks      []Task
	CompletedTasks []Task
}

// DealOwner identifies the RD Station user responsible for a deal.
type DealOwner struct {
	ID    string
	Name  string
	Email string
}

// Stage represents a pipeline stage in RD Station CRM.
type Stage struct {
	ID         string
	Name       string
	PipelineID string
}

// Activity represents a manual annotation registered in a deal.
type Activity struct {
	ID   string
	Text string
	Date string
}

// DealProduct represents a product linked to a deal.
type DealProduct struct {
	ID          string
	Name        string
	Description string
	Amount      float64
	Price       float64
	Total       float64
}

// Task represents a scheduled RD Station task linked to a deal.
type Task struct {
	ID               string
	Subject          string
	Type             string
	Date             string
	Hour             string
	Notes            string
	DealName         string
	ResponsibleNames []string
}

// DealCreationResult reports a created or reused deal and optional follow-up task.
type DealCreationResult struct {
	Deal       Deal
	Task       Task
	TaskError  string
	Idempotent bool
}
