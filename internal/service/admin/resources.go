package admin

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"synova-rd-workflow/internal/domain"
)

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrDuplicate    = errors.New("duplicate")
	ErrNotFound     = errors.New("not found")
)

var phoneRegex = regexp.MustCompile(`^\+[1-9]\d{7,14}$`)

type AdminStore interface {
	ConfigStore
	ListCollaborators(ctx context.Context, activeOnly bool) ([]domain.Collaborator, error)
	GetCollaborator(ctx context.Context, id string) (domain.Collaborator, error)
	SaveCollaborator(ctx context.Context, c domain.Collaborator) error
	FindCollaboratorByEmail(ctx context.Context, email string) (domain.Collaborator, error)
	ListAlerts(ctx context.Context, activeOnly bool) ([]domain.Alert, error)
	GetAlert(ctx context.Context, id string) (domain.Alert, error)
	SaveAlert(ctx context.Context, a domain.Alert) error
	ListAllowlist(ctx context.Context, activeOnly bool) ([]domain.AllowlistEntry, error)
	GetAllowlistEntry(ctx context.Context, id string) (domain.AllowlistEntry, error)
	SaveAllowlistEntry(ctx context.Context, e domain.AllowlistEntry) error
	FindAllowlistByPhone(ctx context.Context, phone string) (domain.AllowlistEntry, error)
	WasAlertSent(ctx context.Context, key string) (bool, error)
	MarkAlertSent(ctx context.Context, key string, ttl time.Time) error
}

type EvolutionAdminClient interface {
	AddToAllowlist(ctx context.Context, phone string) error
	RemoveFromAllowlist(ctx context.Context, phone string) error
	SendTextMessage(ctx context.Context, to string, text string) error
	ConnectionState(ctx context.Context) (domain.WhatsAppConnectionState, error)
	ConnectQRCode(ctx context.Context) (domain.WhatsAppQRCode, error)
}

type RDStationClient interface {
	GetDealStages(ctx context.Context) ([]struct {
		ID   string
		Name string
	}, error)
}

type ResourceService struct {
	store     AdminStore
	evolution EvolutionAdminClient
	rd        DealClient
}

type AlertRunResult struct {
	AlertID        string                `json:"alert_id"`
	DealsMatched   int                   `json:"deals_matched"`
	MessagesSent   int                   `json:"messages_sent"`
	SkippedDedup   int                   `json:"skipped_dedup"`
	SendErrors     int                   `json:"send_errors"`
	Forced         bool                  `json:"forced"`
	Recipients     []domain.Collaborator `json:"recipients"`
	MatchedDealIDs []string              `json:"matched_deal_ids"`
}

func NewResourceService(store AdminStore, evolution EvolutionAdminClient, rd DealClient) *ResourceService {
	return &ResourceService{store: store, evolution: evolution, rd: rd}
}

func (s *ResourceService) ListCollaborators(ctx context.Context, activeOnly bool) ([]domain.Collaborator, error) {
	return s.store.ListCollaborators(ctx, activeOnly)
}

func (s *ResourceService) UpsertCollaborator(ctx context.Context, c domain.Collaborator) (domain.Collaborator, error) {
	c.Name = strings.TrimSpace(c.Name)
	c.Email = strings.ToLower(strings.TrimSpace(c.Email))
	c.WhatsApp = normalizeE164(c.WhatsApp)
	c.Role = normalizeRole(c.Role)
	c.RDStationID = strings.TrimSpace(c.RDStationID)
	c.SupervisorID = strings.TrimSpace(c.SupervisorID)
	if c.Name == "" || c.Email == "" || !strings.Contains(c.Email, "@") || !phoneRegex.MatchString(c.WhatsApp) {
		return domain.Collaborator{}, ErrInvalidInput
	}
	if c.Role == "" {
		c.Role = "seller"
	}
	now := time.Now().UTC()
	if c.ID == "" {
		existing, err := s.store.FindCollaboratorByEmail(ctx, c.Email)
		if err != nil {
			return domain.Collaborator{}, err
		}
		if existing.ID != "" {
			return domain.Collaborator{}, ErrDuplicate
		}
		c.ID = uuid.NewString()
		c.CreatedAt = now
		c.Active = true
	} else {
		current, err := s.store.GetCollaborator(ctx, c.ID)
		if err != nil {
			return domain.Collaborator{}, err
		}
		if current.ID == "" {
			return domain.Collaborator{}, ErrNotFound
		}
		if existing, err := s.store.FindCollaboratorByEmail(ctx, c.Email); err != nil {
			return domain.Collaborator{}, err
		} else if existing.ID != "" && existing.ID != c.ID {
			return domain.Collaborator{}, ErrDuplicate
		}
		c.CreatedAt = current.CreatedAt
	}
	c.UpdatedAt = now
	if err := s.store.SaveCollaborator(ctx, c); err != nil {
		return domain.Collaborator{}, err
	}
	return c, nil
}

func (s *ResourceService) ListAlerts(ctx context.Context, activeOnly bool) ([]domain.Alert, error) {
	alerts, err := s.store.ListAlerts(ctx, activeOnly)
	if err != nil {
		return nil, err
	}
	for i := range alerts {
		if alerts[i].RepeatIntervalHours <= 0 {
			alerts[i].RepeatIntervalHours = 48
		}
	}
	return alerts, nil
}

func (s *ResourceService) UpsertAlert(ctx context.Context, a domain.Alert) (domain.Alert, error) {
	a.Name = strings.TrimSpace(a.Name)
	a.DealStageID = strings.TrimSpace(a.DealStageID)
	a.DealStageName = strings.TrimSpace(a.DealStageName)
	a.MessageTemplate = strings.TrimSpace(a.MessageTemplate)
	a.RecipientIDs = uniqueNonEmptyStrings(a.RecipientIDs)
	if a.RepeatIntervalHours <= 0 {
		a.RepeatIntervalHours = 48
	}
	if a.Name == "" || a.DealStageID == "" || a.TimeThresholdHours <= 0 || a.RepeatIntervalHours <= 0 || a.MessageTemplate == "" || len(a.RecipientIDs) == 0 {
		return domain.Alert{}, ErrInvalidInput
	}
	activeRecipients := 0
	for _, recipientID := range a.RecipientIDs {
		c, err := s.store.GetCollaborator(ctx, recipientID)
		if err != nil {
			return domain.Alert{}, err
		}
		if c.ID == "" {
			return domain.Alert{}, fmt.Errorf("%w: recipient", ErrInvalidInput)
		}
		if c.Active {
			activeRecipients++
		}
	}
	if a.Active && activeRecipients == 0 {
		return domain.Alert{}, fmt.Errorf("%w: active_recipient", ErrInvalidInput)
	}
	now := time.Now().UTC()
	if a.ID == "" {
		a.ID = uuid.NewString()
		a.CreatedAt = now
	} else {
		current, err := s.store.GetAlert(ctx, a.ID)
		if err != nil {
			return domain.Alert{}, err
		}
		if current.ID == "" {
			return domain.Alert{}, ErrNotFound
		}
		a.CreatedAt = current.CreatedAt
		a.LastCheckedAt = current.LastCheckedAt
	}
	a.UpdatedAt = now
	if err := s.store.SaveAlert(ctx, a); err != nil {
		return domain.Alert{}, err
	}
	return a, nil
}

func (s *ResourceService) RunAlertNow(ctx context.Context, id string) (AlertRunResult, error) {
	alert, err := s.store.GetAlert(ctx, id)
	if err != nil {
		return AlertRunResult{}, err
	}
	if alert.ID == "" {
		return AlertRunResult{}, ErrNotFound
	}
	if s.rd == nil {
		return AlertRunResult{}, fmt.Errorf("%w: rdstation", ErrInvalidInput)
	}
	return ExecuteAlert(ctx, s.store, s.rd, s.evolution, alert, AlertExecutionOptions{Force: true})
}

func (s *ResourceService) ListAllowlist(ctx context.Context, activeOnly bool) ([]domain.AllowlistEntry, error) {
	return s.store.ListAllowlist(ctx, activeOnly)
}

func (s *ResourceService) UpsertAllowlist(ctx context.Context, e domain.AllowlistEntry) (domain.AllowlistEntry, error) {
	e.PhoneNumber = normalizeE164(e.PhoneNumber)
	e.Label = strings.TrimSpace(e.Label)
	e.Role = normalizeRole(e.Role)
	e.CollaboratorID = strings.TrimSpace(e.CollaboratorID)
	if !phoneRegex.MatchString(e.PhoneNumber) {
		return domain.AllowlistEntry{}, ErrInvalidInput
	}
	if e.Role == "" {
		e.Role = "seller"
	}
	now := time.Now().UTC()
	if e.ID == "" {
		existing, err := s.store.FindAllowlistByPhone(ctx, e.PhoneNumber)
		if err != nil {
			return domain.AllowlistEntry{}, err
		}
		if existing.ID != "" {
			return domain.AllowlistEntry{}, ErrDuplicate
		}
		e.ID = uuid.NewString()
		e.CreatedAt = now
		e.Active = true
	} else {
		current, err := s.store.GetAllowlistEntry(ctx, e.ID)
		if err != nil {
			return domain.AllowlistEntry{}, err
		}
		if current.ID == "" {
			return domain.AllowlistEntry{}, ErrNotFound
		}
		e.CreatedAt = current.CreatedAt
	}
	e.UpdatedAt = now
	if e.Active {
		e.SyncPending = s.evolution.AddToAllowlist(ctx, e.PhoneNumber) != nil
	} else {
		e.SyncPending = s.evolution.RemoveFromAllowlist(ctx, e.PhoneNumber) != nil
	}
	if err := s.store.SaveAllowlistEntry(ctx, e); err != nil {
		return domain.AllowlistEntry{}, err
	}
	return e, nil
}

func (s *ResourceService) SeedAllowlist(ctx context.Context, phones []string) error {
	for _, phone := range phones {
		phone = normalizeE164(phone)
		if !phoneRegex.MatchString(phone) {
			continue
		}
		existing, err := s.store.FindAllowlistByPhone(ctx, phone)
		if err != nil {
			return err
		}
		if existing.ID != "" {
			continue
		}
		_, err = s.UpsertAllowlist(ctx, domain.AllowlistEntry{
			PhoneNumber: phone,
			Label:       "Importado do Docker",
			Active:      true,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *ResourceService) IsAllowed(ctx context.Context, phone string) (bool, error) {
	entry, err := s.store.FindAllowlistByPhone(ctx, phone)
	if err != nil {
		return false, err
	}
	if entry.ID == "" {
		return false, nil
	}
	return entry.Active, nil
}

func (s *ResourceService) WhatsAppStatus(ctx context.Context) (domain.WhatsAppConnectionState, error) {
	state, err := s.evolution.ConnectionState(ctx)
	if err != nil {
		return domain.WhatsAppConnectionState{}, err
	}
	return state, nil
}

func (s *ResourceService) WhatsAppQRCode(ctx context.Context) (domain.WhatsAppQRCode, error) {
	qr, err := s.evolution.ConnectQRCode(ctx)
	if err != nil {
		return domain.WhatsAppQRCode{}, err
	}
	return qr, nil
}

func (s *ResourceService) AccessProfile(ctx context.Context, phone string) (domain.AccessProfile, error) {
	entry, err := s.store.FindAllowlistByPhone(ctx, phone)
	if err != nil {
		return domain.AccessProfile{}, err
	}
	if entry.ID == "" || !entry.Active {
		return domain.AccessProfile{}, nil
	}

	profile := domain.AccessProfile{
		Phone:          normalizeE164(phone),
		Role:           normalizeRole(entry.Role),
		CollaboratorID: entry.CollaboratorID,
	}
	if profile.Role == "" {
		profile.Role = "seller"
	}

	collaborators, err := s.store.ListCollaborators(ctx, false)
	if err != nil {
		return domain.AccessProfile{}, err
	}

	var current domain.Collaborator
	for _, c := range collaborators {
		if entry.CollaboratorID != "" && c.ID == entry.CollaboratorID {
			current = c
			break
		}
		if entry.CollaboratorID == "" && normalizeE164(c.WhatsApp) == normalizeE164(phone) {
			current = c
			break
		}
	}
	if current.ID != "" {
		profile.CollaboratorID = current.ID
		profile.RDStationID = strings.TrimSpace(current.RDStationID)
		if current.Role != "" {
			profile.Role = normalizeRole(current.Role)
		}
	}

	if profile.Role == "supervisor" && profile.CollaboratorID != "" {
		for _, c := range collaborators {
			if !c.Active || c.SupervisorID != profile.CollaboratorID || strings.TrimSpace(c.RDStationID) == "" {
				continue
			}
			profile.TeamRDUserIDs = append(profile.TeamRDUserIDs, strings.TrimSpace(c.RDStationID))
		}
	}

	return profile, nil
}

func normalizeE164(phone string) string {
	phone = strings.TrimSpace(phone)
	phone = strings.TrimPrefix(phone, "+")
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	phone = strings.ReplaceAll(phone, "(", "")
	phone = strings.ReplaceAll(phone, ")", "")
	if phone == "" {
		return ""
	}
	return "+" + phone
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "director", "diretoria", "diretor", "the god", "god":
		return "director"
	case "supervisor":
		return "supervisor"
	case "seller", "vendedor", "pj", "pf":
		return "seller"
	default:
		return ""
	}
}
