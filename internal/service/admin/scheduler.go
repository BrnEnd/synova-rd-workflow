package admin

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	rdClient "synova-rd-workflow/internal/client/rdstation"
	"synova-rd-workflow/internal/domain"
)

type DealClient interface {
	GetDeals(ctx context.Context, params rdClient.GetDealsParams) ([]rdClient.DealResponse, error)
	GetTasks(ctx context.Context, params rdClient.GetTasksParams) ([]rdClient.TaskResponse, error)
	CreateTask(ctx context.Context, params rdClient.CreateTaskParams) (rdClient.TaskResponse, error)
}

type Scheduler struct {
	store     AdminStore
	rd        DealClient
	evolution EvolutionAdminClient
	interval  time.Duration
	logger    *slog.Logger
	mu        sync.Mutex
}

type eligibleDeal struct {
	Deal      rdClient.DealResponse
	UpdatedAt time.Time
}

type AlertExecutionOptions struct {
	Force bool
}

func NewScheduler(store AdminStore, rd DealClient, evolution EvolutionAdminClient, interval time.Duration, logger *slog.Logger) *Scheduler {
	return &Scheduler{store: store, rd: rd, evolution: evolution, interval: interval, logger: logger}
}

func (s *Scheduler) Start(ctx context.Context) {
	if s.interval <= 0 {
		return
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.RunOnce(ctx)
		}
	}
}

func (s *Scheduler) RunOnce(ctx context.Context) {
	if !s.mu.TryLock() {
		return
	}
	defer s.mu.Unlock()

	alerts, err := s.store.ListAlerts(ctx, true)
	if err != nil {
		s.logger.ErrorContext(ctx, "admin scheduler list alerts failed", "component", "admin_scheduler", "error", err)
		return
	}
	var matched, sentCount, errCount int
	for _, alert := range alerts {
		result, err := ExecuteAlert(ctx, s.store, s.rd, s.evolution, alert, AlertExecutionOptions{})
		if err != nil {
			errCount++
			s.logger.ErrorContext(ctx, "admin scheduler alert failed", "component", "admin_scheduler", "alert_id", alert.ID, "error", err)
			continue
		}
		matched += result.DealsMatched
		sentCount += result.MessagesSent
		errCount += result.SendErrors
	}
	s.logger.InfoContext(ctx, "admin scheduler cycle completed", "component", "admin_scheduler", "alerts_evaluated", len(alerts), "deals_matched", matched, "messages_sent", sentCount, "errors", errCount)
}

func ExecuteAlert(ctx context.Context, store AdminStore, rd DealClient, evolution EvolutionAdminClient, alert domain.Alert, opts AlertExecutionOptions) (AlertRunResult, error) {
	recipients, err := activeRecipientCollaborators(ctx, store, alert.RecipientIDs)
	if err != nil {
		return AlertRunResult{}, err
	}
	if len(recipients) == 0 {
		return AlertRunResult{}, fmt.Errorf("%w: recipients", ErrInvalidInput)
	}
	deals, err := rd.GetDeals(ctx, rdClient.GetDealsParams{DealStageID: alert.DealStageID})
	if err != nil {
		return AlertRunResult{}, err
	}

	result := AlertRunResult{AlertID: alert.ID, Recipients: recipients, Forced: opts.Force}
	eligible := make([]eligibleDeal, 0, len(deals))
	cutoff := time.Now().UTC().Add(-time.Duration(alert.TimeThresholdHours) * time.Hour)
	for _, deal := range deals {
		updatedAt, parseErr := time.Parse(time.RFC3339, deal.UpdatedAt)
		if parseErr != nil || updatedAt.After(cutoff) {
			continue
		}
		result.DealsMatched++
		result.MatchedDealIDs = append(result.MatchedDealIDs, deal.ID)
		if !opts.Force {
			key := alert.ID + "#" + deal.ID
			already, err := store.WasAlertSent(ctx, key)
			if err != nil {
				return result, err
			}
			if already {
				result.SkippedDedup++
				continue
			}
		}
		eligible = append(eligible, eligibleDeal{Deal: deal, UpdatedAt: updatedAt})
	}

	if len(eligible) == 0 {
		markAlertChecked(ctx, store, alert)
		return result, nil
	}

	message := renderAggregateMessage(alert, eligible)
	var sendFailed bool
	for _, recipient := range recipients {
		if err := evolution.SendTextMessage(ctx, recipient.WhatsApp, message); err != nil {
			sendFailed = true
			result.SendErrors++
			continue
		}
		result.MessagesSent++
	}
	if !sendFailed {
		repeatInterval := alert.RepeatIntervalHours
		if repeatInterval <= 0 {
			repeatInterval = 48
		}
		ttl := time.Now().UTC().Add(time.Duration(repeatInterval) * time.Hour)
		for _, item := range eligible {
			_ = store.MarkAlertSent(ctx, alert.ID+"#"+item.Deal.ID, ttl)
		}
	}
	markAlertChecked(ctx, store, alert)
	return result, nil
}

func maskPhone(phone string) string {
	phone = strings.TrimPrefix(strings.TrimSpace(phone), "+")
	if len(phone) <= 4 {
		return "+" + phone
	}
	return "+" + phone[:4] + "***" + phone[len(phone)-2:]
}

func (s *Scheduler) activeRecipients(ctx context.Context, ids []string) []string {
	phones := make([]string, 0, len(ids))
	for _, id := range ids {
		c, err := s.store.GetCollaborator(ctx, id)
		if err == nil && c.ID != "" && c.Active {
			phones = append(phones, c.WhatsApp)
		}
	}
	return phones
}

func activeRecipientCollaborators(ctx context.Context, store AdminStore, ids []string) ([]domain.Collaborator, error) {
	recipients := make([]domain.Collaborator, 0, len(ids))
	for _, id := range ids {
		c, err := store.GetCollaborator(ctx, id)
		if err != nil {
			return nil, err
		}
		if c.ID != "" && c.Active {
			recipients = append(recipients, c)
		}
	}
	return recipients, nil
}

func markAlertChecked(ctx context.Context, store AdminStore, alert domain.Alert) {
	alert.LastCheckedAt = time.Now().UTC()
	alert.UpdatedAt = time.Now().UTC()
	_ = store.SaveAlert(ctx, alert)
}

func renderAggregateMessage(alert domain.Alert, deals []eligibleDeal) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Alerta: %s\n", alert.Name)
	fmt.Fprintf(&b, "Estagio: %s\n", alert.DealStageName)
	fmt.Fprintf(&b, "Negociacoes paradas: %d\n\n", len(deals))
	for i, item := range deals {
		line := renderTemplate(alert.MessageTemplate, item.Deal, alert.DealStageName, time.Since(item.UpdatedAt))
		fmt.Fprintf(&b, "%d. %s\n", i+1, line)
		fmt.Fprintf(&b, "Responsavel: %s\n", responsibleName(item.Deal))
		if i < len(deals)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func renderTemplate(template string, deal rdClient.DealResponse, stage string, age time.Duration) string {
	contactName := ""
	if len(deal.Contacts) > 0 {
		contactName = deal.Contacts[0].Name
	}
	replacements := map[string]string{
		"{{deal_name}}":     deal.Name,
		"{{deal_stage}}":    stage,
		"{{days_in_stage}}": fmt.Sprintf("%d", int(age.Hours()/24)),
		"{{contact_name}}":  contactName,
		"{{deal_value}}":    "",
	}
	out := template
	for k, v := range replacements {
		out = strings.ReplaceAll(out, k, v)
	}
	return out
}

func responsibleName(deal rdClient.DealResponse) string {
	switch {
	case strings.TrimSpace(deal.User.Name) != "":
		return strings.TrimSpace(deal.User.Name)
	case strings.TrimSpace(deal.Owner.Name) != "":
		return strings.TrimSpace(deal.Owner.Name)
	case strings.TrimSpace(deal.DealOwner.Name) != "":
		return strings.TrimSpace(deal.DealOwner.Name)
	default:
		return "Nao informado"
	}
}

func firstDealContactName(deal rdClient.DealResponse) string {
	if len(deal.Contacts) == 0 {
		return ""
	}
	return strings.TrimSpace(deal.Contacts[0].Name)
}
