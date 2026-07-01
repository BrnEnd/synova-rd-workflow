package evolution

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"synova-rd-workflow/internal/domain"
	adminSvc "synova-rd-workflow/internal/service/admin"
	convSvc "synova-rd-workflow/internal/service/conversation"
	intentRouter "synova-rd-workflow/internal/service/intent_router"
	"synova-rd-workflow/internal/service/nlp"
	rdSvc "synova-rd-workflow/internal/service/rdstation"
)

type Sender interface {
	SendTextMessage(ctx context.Context, to string, text string) error
}

type MediaFetcher interface {
	FetchMediaBase64(ctx context.Context, remoteJID, messageID string, fromMe bool) (string, string, error)
}

type AllowChecker interface {
	IsAllowed(ctx context.Context, phone string) (bool, error)
}

type AccessProfiler interface {
	AccessProfile(ctx context.Context, phone string) (domain.AccessProfile, error)
}

type ScheduledTasksProvider interface {
	ListScheduledTasks(ctx context.Context) ([]adminSvc.ScheduledTaskSummary, error)
}

type Handler struct {
	conv            *convSvc.Service
	nlpSvc          nlp.ServiceInterface
	router          *intentRouter.Router
	sender          Sender
	mediaFetcher    MediaFetcher
	logger          *slog.Logger
	allowedNumbers  map[string]struct{}
	allowChecker    AllowChecker
	accessProfiler  AccessProfiler
	tasksProvider   ScheduledTasksProvider
	pendingMu       sync.Mutex
	pendingDeals    map[string]pendingDealSelection
	pendingListsMu  sync.Mutex
	pendingLists    map[string][]domain.Deal
	pendingCreateMu sync.Mutex
	pendingCreates  map[string]pendingCreateDeal
	pendingTaskMu   sync.Mutex
	pendingTasks    map[string]pendingScheduledTask
	rdTaskPagesMu   sync.Mutex
	rdTaskPages     map[string]rdTaskPagination
	activeDealsMu   sync.Mutex
	activeDeals     map[string]domain.Deal
}

func New(conv *convSvc.Service, nlpSvc nlp.ServiceInterface, router *intentRouter.Router, sender Sender, logger *slog.Logger, allowedNumbers []string) *Handler {
	handler := &Handler{
		conv:           conv,
		nlpSvc:         nlpSvc,
		router:         router,
		sender:         sender,
		logger:         logger,
		allowedNumbers: buildAllowedNumbers(allowedNumbers),
		pendingDeals:   make(map[string]pendingDealSelection),
		pendingLists:   make(map[string][]domain.Deal),
		pendingCreates: make(map[string]pendingCreateDeal),
		pendingTasks:   make(map[string]pendingScheduledTask),
		rdTaskPages:    make(map[string]rdTaskPagination),
		activeDeals:    make(map[string]domain.Deal),
	}
	if fetcher, ok := sender.(MediaFetcher); ok {
		handler.mediaFetcher = fetcher
	}
	return handler
}

func (h *Handler) SetAllowChecker(checker AllowChecker) {
	h.allowChecker = checker
	if profiler, ok := checker.(AccessProfiler); ok {
		h.accessProfiler = profiler
	}
	if provider, ok := checker.(ScheduledTasksProvider); ok {
		h.tasksProvider = provider
	}
}

func (h *Handler) RegisterRoutes(r gin.IRouter) {
	r.POST("/evolution/webhook", h.HandleWebhook)
}

func (h *Handler) HandleWebhook(c *gin.Context) {
	var payload webhookPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		h.logger.WarnContext(c.Request.Context(), "invalid evolution payload", "error", err)
		c.JSON(http.StatusOK, gin.H{})
		return
	}

	inbound, ok := payload.inboundMessage(c.Request.Context(), h.nlpSvc, h.mediaFetcher, h.logger)
	if !ok {
		h.logger.InfoContext(c.Request.Context(), "evolution webhook ignored",
			"event", payload.Event,
			"message_type", payload.Data.MessageType,
			"from_me", payload.Data.Key.FromMe,
		)
		c.JSON(http.StatusOK, gin.H{})
		return
	}

	if !h.isAllowed(c.Request.Context(), inbound.From) {
		h.logger.WarnContext(c.Request.Context(), "evolution sender not allowed",
			"from", maskPhone(inbound.From),
			"message_id", inbound.ID,
		)
		c.JSON(http.StatusOK, gin.H{})
		return
	}

	if err := h.processTextMessage(c.Request.Context(), inbound); err != nil {
		h.logger.ErrorContext(c.Request.Context(), "failed to process evolution message",
			"from", maskPhone(inbound.From),
			"message_id", inbound.ID,
			"error", err,
		)
	}

	c.JSON(http.StatusOK, gin.H{})
}

func (h *Handler) processTextMessage(ctx context.Context, inbound inboundTextMessage) error {
	start := time.Now()
	msg := strings.TrimSpace(inbound.Body)
	if msg == "" {
		return nil
	}

	from := "+" + normalizeNumber(inbound.From)
	session, err := h.conv.GetOrCreateSession(ctx, from)
	if err != nil {
		return fmt.Errorf("get/create session: %w", err)
	}

	history, err := h.conv.GetRecentHistory(ctx, session.ID)
	if err != nil {
		return fmt.Errorf("get history: %w", err)
	}

	if isGreeting(msg) {
		reply := GreetingResponse()
		_ = h.conv.SaveUserMessage(ctx, session.ID, msg, "greeting")
		_ = h.conv.SaveAssistantMessage(ctx, session.ID, reply)
		if err := h.sender.SendTextMessage(ctx, inbound.From, reply); err != nil {
			return fmt.Errorf("send evolution greeting reply: %w", err)
		}
		return nil
	}

	if reply, ok, err := h.handlePendingCreate(ctx, session.ID, msg); ok || err != nil {
		if err != nil {
			return err
		}
		_ = h.conv.SaveUserMessage(ctx, session.ID, msg, "create_deal_pending")
		_ = h.conv.SaveAssistantMessage(ctx, session.ID, reply)
		if err := h.sender.SendTextMessage(ctx, inbound.From, reply); err != nil {
			return fmt.Errorf("send evolution pending create reply: %w", err)
		}
		return nil
	}

	if reply, ok, err := h.handlePendingScheduledTask(ctx, session.ID, msg, h.actorForPhone(ctx, from)); ok || err != nil {
		if err != nil {
			return err
		}
		_ = h.conv.SaveUserMessage(ctx, session.ID, msg, "create_scheduled_task_pending")
		_ = h.conv.SaveAssistantMessage(ctx, session.ID, reply)
		if err := h.sender.SendTextMessage(ctx, inbound.From, reply); err != nil {
			return fmt.Errorf("send evolution pending task reply: %w", err)
		}
		return nil
	}

	if reply, ok, err := h.handlePendingSelection(ctx, session.ID, msg, h.actorForPhone(ctx, from)); ok || err != nil {
		if err != nil {
			return err
		}
		_ = h.conv.SaveUserMessage(ctx, session.ID, msg, "deal_selection")
		_ = h.conv.SaveAssistantMessage(ctx, session.ID, reply)
		if err := h.sender.SendTextMessage(ctx, inbound.From, reply); err != nil {
			return fmt.Errorf("send evolution selection reply: %w", err)
		}
		h.clearPendingSelection(session.ID)
		h.logger.InfoContext(ctx, "evolution selection completed",
			"from", maskPhone(inbound.From),
			"message_id", inbound.ID,
			"latency_ms", time.Since(start).Milliseconds(),
		)
		return nil
	}

	if reply, ok := h.handleRDTaskPagination(session.ID, msg); ok {
		_ = h.conv.SaveUserMessage(ctx, session.ID, msg, "get_scheduled_tasks_next_page")
		_ = h.conv.SaveAssistantMessage(ctx, session.ID, reply)
		if err := h.sender.SendTextMessage(ctx, inbound.From, reply); err != nil {
			return fmt.Errorf("send evolution task page reply: %w", err)
		}
		return nil
	}

	if isRecentDuplicate(history, msg) {
		h.logger.InfoContext(ctx, "evolution duplicate user message ignored",
			"from", maskPhone(inbound.From),
			"message_id", inbound.ID,
		)
		return nil
	}

	intent, err := h.nlpSvc.ParseIntent(ctx, history, msg)
	if err != nil {
		h.logger.WarnContext(ctx, "nlp parse intent failed", "session_id", session.ID, "message_id", inbound.ID, "error", err)
		intent = domain.Intent{Name: domain.IntentUnknown, RawText: msg}
	}
	if isDeleteRequest(msg) {
		intent.Name = domain.IntentDeleteDeal
		intent.RawText = msg
	}
	normalizeDealOwnerIntent(&intent)

	if intent.Name == domain.IntentCreateDeal {
		reply := h.startCreateDealFlow(session.ID, intent)
		_ = h.conv.SaveUserMessage(ctx, session.ID, msg, string(intent.Name))
		_ = h.conv.SaveAssistantMessage(ctx, session.ID, reply)
		if err := h.sender.SendTextMessage(ctx, inbound.From, reply); err != nil {
			return fmt.Errorf("send evolution create flow reply: %w", err)
		}
		return nil
	}

	if intent.Name == domain.IntentCreateScheduledTask {
		reply := h.startScheduledTaskFlow(ctx, session.ID, intent, h.actorForPhone(ctx, from))
		_ = h.conv.SaveUserMessage(ctx, session.ID, msg, string(intent.Name))
		_ = h.conv.SaveAssistantMessage(ctx, session.ID, reply)
		if err := h.sender.SendTextMessage(ctx, inbound.From, reply); err != nil {
			return fmt.Errorf("send evolution create task flow reply: %w", err)
		}
		return nil
	}

	_ = h.conv.SaveUserMessage(ctx, session.ID, msg, string(intent.Name))
	// A new NLP-processed message clears any pending list selection.
	h.clearPendingList(session.ID)
	actor := h.actorForPhone(ctx, from)
	reply := h.buildReply(ctx, session.ID, intent, actor)
	_ = h.conv.SaveAssistantMessage(ctx, session.ID, reply)

	if err := h.sender.SendTextMessage(ctx, inbound.From, reply); err != nil {
		return fmt.Errorf("send evolution reply: %w", err)
	}

	h.logger.InfoContext(ctx, "evolution message completed",
		"from", maskPhone(inbound.From),
		"message_id", inbound.ID,
		"intent", string(intent.Name),
		"latency_ms", time.Since(start).Milliseconds(),
	)

	return nil
}

func (h *Handler) isAllowed(ctx context.Context, number string) bool {
	if h.allowChecker != nil {
		allowed, err := h.allowChecker.IsAllowed(ctx, number)
		if err != nil {
			h.logger.WarnContext(ctx, "admin allowlist check failed", "from", maskPhone(number), "error", err)
			return false
		}
		return allowed
	}
	if len(h.allowedNumbers) == 0 {
		return true
	}
	_, ok := h.allowedNumbers[normalizeNumber(number)]
	return ok
}

func (h *Handler) buildReply(ctx context.Context, sessionID string, intent domain.Intent, actor intentRouter.Actor) string {
	if intent.Name == domain.IntentUnknown {
		return nlp.FallbackResponse()
	}
	if intent.Name == domain.IntentGetScheduledTasks {
		if h.tasksProvider == nil {
			return "Ainda nao tenho acesso as tarefas agendadas neste ambiente."
		}
		tasks, err := h.tasksProvider.ListScheduledTasks(ctx)
		if err != nil {
			h.logger.WarnContext(ctx, "scheduled tasks summary failed", "session_id", sessionID, "error", err)
			return nlp.ErrorResponse("consulta de tarefas agendadas")
		}
		return h.formatAndRememberScheduledTasks(sessionID, tasks)
	}

	h.applyActiveDeal(sessionID, &intent)
	result, routeErr := h.router.RouteForActor(ctx, intent, actor)
	if routeErr != nil {
		// If disambiguation is needed, try auto-resolving using the active deal.
		var multiErr *rdSvc.MultipleDealsError
		if errors.As(routeErr, &multiErr) {
			if active, ok := h.getActiveDeal(sessionID); ok {
				for _, d := range multiErr.Deals {
					if d.ID == active.ID {
						autoResult, autoErr := h.router.ResolveDealSelection(ctx, intent, d, actor)
						if autoErr == nil {
							formatted, fmtErr := h.nlpSvc.FormatResponse(ctx, intent, autoResult)
							if fmtErr == nil {
								return formatted
							}
						}
						break
					}
				}
			}
		}
		return h.handleRouteError(routeErr, sessionID, intent)
	}

	// Remember single-deal results as the active deal for follow-up questions.
	if deal, ok := result.(domain.Deal); ok {
		h.setActiveDeal(sessionID, deal)
	}

	// Store deal lists so the user can select by number.
	if deals, ok := result.([]domain.Deal); ok && len(deals) > 1 {
		h.setPendingList(sessionID, deals)
	}

	formatted, fmtErr := h.nlpSvc.FormatResponse(ctx, intent, result)
	if fmtErr != nil {
		h.logger.WarnContext(ctx, "nlp format response failed", "session_id", sessionID, "error", fmtErr)
		return nlp.ErrorResponse("servico de formatacao")
	}
	return formatted
}

func (h *Handler) applyActiveDeal(sessionID string, intent *domain.Intent) {
	if !needsDealName(intent.Name) || strings.TrimSpace(intent.Parameters["deal_name"]) != "" {
		return
	}
	active, ok := h.getActiveDeal(sessionID)
	if !ok || strings.TrimSpace(active.Name) == "" {
		return
	}
	if intent.Parameters == nil {
		intent.Parameters = map[string]string{}
	}
	intent.Parameters["deal_name"] = active.Name
}

func needsDealName(name domain.IntentName) bool {
	switch name {
	case domain.IntentGetDeal,
		domain.IntentGetDealSummary,
		domain.IntentGetDealContacts,
		domain.IntentGetDealActivities,
		domain.IntentCreateDealActivity,
		domain.IntentUpdateDeal,
		domain.IntentMoveDealStage,
		domain.IntentAssociateContactToDeal,
		domain.IntentCreateScheduledTask:
		return true
	default:
		return false
	}
}

func (h *Handler) startCreateDealFlow(sessionID string, intent domain.Intent) string {
	draft := pendingCreateDeal{
		Intent: domain.Intent{
			Name:       domain.IntentCreateDeal,
			Parameters: map[string]string{},
			RawText:    intent.RawText,
		},
	}
	for key, value := range intent.Parameters {
		draft.Intent.Parameters[key] = strings.TrimSpace(value)
	}
	if draft.Intent.Parameters["name"] == "" && draft.Intent.Parameters["deal_name"] != "" {
		draft.Intent.Parameters["name"] = draft.Intent.Parameters["deal_name"]
		delete(draft.Intent.Parameters, "deal_name")
	}
	prepareCreateDealDraft(&draft)
	h.setPendingCreate(sessionID, draft)
	return h.nextCreateDealQuestion(sessionID, draft)
}

func (h *Handler) handlePendingCreate(ctx context.Context, sessionID, msg string) (string, bool, error) {
	draft, ok := h.getPendingCreate(sessionID)
	if !ok {
		return "", false, nil
	}
	normalized := normalizeIntentText(msg)
	if strings.Contains(normalized, " cancelar ") || strings.Contains(normalized, " cancela ") {
		h.clearPendingCreate(sessionID)
		return "Tudo bem, cancelei a criacao da negociacao.", true, nil
	}

	if draft.WaitingForSuggestion != "" {
		if draft.WaitingForSuggestion == "stage" {
			if stage, ok := chooseStageSuggestion(msg, draft.Suggestions); ok {
				draft.Intent.Parameters["stage"] = stage
				draft.WaitingForSuggestion = ""
				draft.Suggestions = nil
				h.setPendingCreate(sessionID, draft)
				return h.nextCreateDealQuestion(sessionID, draft), true, nil
			}
			return fmt.Sprintf("Escolha uma etapa pelo numero ou escreva o nome exatamente como aparece:\n%s", numberedOptions(draft.Suggestions)), true, nil
		}
		if index, ok := parseSelection(msg); ok && index >= 0 && index < len(draft.Suggestions) {
			selected := draft.Suggestions[index]
			draft.Intent.Parameters[draft.WaitingForSuggestion] = selected
			draft.WaitingForSuggestion = ""
			draft.Suggestions = nil
			h.setPendingCreate(sessionID, draft)
			return h.nextCreateDealQuestion(sessionID, draft), true, nil
		}
		return fmt.Sprintf("Escolha uma das opcoes pelo numero:\n%s", numberedOptions(draft.Suggestions)), true, nil
	}

	switch draft.nextField() {
	case "company":
		draft.Intent.Parameters["company"] = strings.TrimSpace(msg)
		prepareCreateDealDraft(&draft)
	case "product":
		draft.Intent.Parameters["product"] = strings.TrimSpace(msg)
		prepareCreateDealDraft(&draft)
	case "name":
		draft.Intent.Parameters["name"] = strings.TrimSpace(msg)
	case "contact_name":
		draft.Intent.Parameters["contact_name"] = strings.TrimSpace(msg)
	case "stage":
		draft.Intent.Parameters["stage"] = strings.TrimSpace(msg)
	case "confirm":
		if isAffirmative(msg) {
			result, err := h.router.Route(ctx, draft.Intent)
			if err != nil {
				var contactErr *rdSvc.ContactNotFoundError
				if errors.As(err, &contactErr) && len(contactErr.Suggestions) > 0 {
					draft.WaitingForSuggestion = "contact_name"
					draft.Suggestions = contactErr.Suggestions
					h.setPendingCreate(sessionID, draft)
					return fmt.Sprintf("Nao encontrei o contato \"%s\". Voce quis dizer algum destes?\n%s", contactErr.Name, numberedOptions(contactErr.Suggestions)), true, nil
				}
				var ownerErr *rdSvc.OwnerNotFoundError
				if errors.As(err, &ownerErr) && len(ownerErr.Suggestions) > 0 {
					draft.WaitingForSuggestion = "owner_name"
					draft.Suggestions = ownerErr.Suggestions
					h.setPendingCreate(sessionID, draft)
					return fmt.Sprintf("Nao encontrei o vendedor \"%s\". Voce quis dizer algum destes?\n%s", ownerErr.Name, numberedOptions(ownerErr.Suggestions)), true, nil
				}
				var stageErr *rdSvc.StageNotFoundError
				if errors.As(err, &stageErr) && len(stageErr.Available) > 0 {
					draft.Intent.Parameters["stage"] = ""
					draft.WaitingForSuggestion = "stage"
					draft.Suggestions = stageErr.Available
					h.setPendingCreate(sessionID, draft)
					return fmt.Sprintf("Nao encontrei a etapa informada. Escolha uma das etapas pelo numero ou pelo nome:\n%s", numberedOptions(stageErr.Available)), true, nil
				}
				return h.handleRouteError(err, sessionID, draft.Intent), true, nil
			}
			h.clearPendingCreate(sessionID)
			formatted, err := h.nlpSvc.FormatResponse(ctx, draft.Intent, result)
			if err != nil {
				h.logger.WarnContext(ctx, "nlp format create response failed", "session_id", sessionID, "error", err)
				return "Negociacao criada com sucesso.", true, nil
			}
			return formatted, true, nil
		}
		if isNegative(msg) {
			h.clearPendingCreate(sessionID)
			return "Sem problema, nao criei a negociacao. Se quiser, me envie os dados de novo.", true, nil
		}
		return "Para criar, responda *sim*. Para cancelar, responda *nao*.", true, nil
	}

	h.setPendingCreate(sessionID, draft)
	return h.nextCreateDealQuestion(sessionID, draft), true, nil
}

func (h *Handler) nextCreateDealQuestion(sessionID string, draft pendingCreateDeal) string {
	switch draft.nextField() {
	case "company":
		h.setPendingCreate(sessionID, draft)
		return "Qual e o cliente ou empresa dessa negociacao?"
	case "product":
		h.setPendingCreate(sessionID, draft)
		return "Qual produto esta sendo negociado?"
	case "name":
		h.setPendingCreate(sessionID, draft)
		return "Claro. Qual e o nome da negociacao que devo criar?"
	case "contact_name":
		h.setPendingCreate(sessionID, draft)
		return "Quem e o contato responsavel no cliente?"
	case "stage":
		h.setPendingCreate(sessionID, draft)
		return "Em qual etapa do funil devo criar essa negociacao?"
	default:
		h.setPendingCreate(sessionID, draft)
		return createDealConfirmation(draft)
	}
}

func (h *Handler) getPendingCreate(sessionID string) (pendingCreateDeal, bool) {
	h.pendingCreateMu.Lock()
	defer h.pendingCreateMu.Unlock()
	draft, ok := h.pendingCreates[sessionID]
	return draft, ok
}

func (h *Handler) setPendingCreate(sessionID string, draft pendingCreateDeal) {
	h.pendingCreateMu.Lock()
	h.pendingCreates[sessionID] = draft
	h.pendingCreateMu.Unlock()
}

func (h *Handler) clearPendingCreate(sessionID string) {
	h.pendingCreateMu.Lock()
	delete(h.pendingCreates, sessionID)
	h.pendingCreateMu.Unlock()
}

func (h *Handler) startScheduledTaskFlow(ctx context.Context, sessionID string, intent domain.Intent, actor intentRouter.Actor) string {
	draft := pendingScheduledTask{
		Intent: domain.Intent{
			Name:       domain.IntentCreateScheduledTask,
			Parameters: map[string]string{},
			RawText:    intent.RawText,
		},
	}
	for key, value := range intent.Parameters {
		draft.Intent.Parameters[key] = strings.TrimSpace(value)
	}
	clearUnmentionedScheduledTaskDefaults(&draft)
	prepareScheduledTaskDraft(&draft)
	h.setPendingScheduledTask(sessionID, draft)
	return h.nextScheduledTaskQuestion(ctx, sessionID, draft, actor)
}

func (h *Handler) handlePendingScheduledTask(ctx context.Context, sessionID, msg string, actor intentRouter.Actor) (string, bool, error) {
	draft, ok := h.getPendingScheduledTask(sessionID)
	if !ok {
		return "", false, nil
	}
	normalized := normalizeIntentText(msg)
	if strings.Contains(normalized, " cancelar ") || strings.Contains(normalized, " cancela ") {
		h.clearPendingScheduledTask(sessionID)
		return "Tudo bem, cancelei a criacao da tarefa.", true, nil
	}
	switch draft.nextField() {
	case "deal_name":
		draft.Intent.Parameters["deal_name"] = strings.TrimSpace(msg)
	case "subject":
		draft.Intent.Parameters["subject"] = strings.TrimSpace(msg)
	case "date":
		draft.Intent.Parameters["date"] = normalizeTaskDate(msg)
	case "hour":
		draft.Intent.Parameters["hour"] = normalizeTaskHour(msg)
	case "confirm":
		if isAffirmative(msg) {
			result, err := h.router.RouteForActor(ctx, draft.Intent, actor)
			if err != nil {
				return h.handleRouteError(err, sessionID, draft.Intent), true, nil
			}
			h.clearPendingScheduledTask(sessionID)
			if task, ok := result.(domain.Task); ok {
				return formatCreatedTask(task), true, nil
			}
			return "Tarefa criada com sucesso.", true, nil
		}
		if isNegative(msg) {
			h.clearPendingScheduledTask(sessionID)
			return "Sem problema, nao criei a tarefa. Se quiser, me envie os dados de novo.", true, nil
		}
		if applyScheduledTaskCorrections(&draft, msg) {
			prepareScheduledTaskDraft(&draft)
			h.setPendingScheduledTask(sessionID, draft)
			return h.nextScheduledTaskQuestion(ctx, sessionID, draft, actor), true, nil
		}
		return "Nao entendi se devo criar ou ajustar algum dado. Responda *sim* para criar, *nao* para cancelar, ou me diga o que quer mudar, por exemplo: assunto, data ou horario.", true, nil
	}
	prepareScheduledTaskDraft(&draft)
	h.setPendingScheduledTask(sessionID, draft)
	return h.nextScheduledTaskQuestion(ctx, sessionID, draft, actor), true, nil
}

func (h *Handler) nextScheduledTaskQuestion(ctx context.Context, sessionID string, draft pendingScheduledTask, actor intentRouter.Actor) string {
	switch draft.nextField() {
	case "deal_name":
		h.setPendingScheduledTask(sessionID, draft)
		return "Em qual negociacao devo criar essa tarefa?"
	case "subject":
		h.setPendingScheduledTask(sessionID, draft)
		return "Qual e o assunto da tarefa?"
	case "date":
		h.setPendingScheduledTask(sessionID, draft)
		return "Para qual data devo agendar? Pode responder como *hoje*, *amanha* ou *DD/MM/AAAA*."
	case "hour":
		h.setPendingScheduledTask(sessionID, draft)
		return "Qual horario? Use HH:MM, por exemplo 14:30."
	default:
		h.setPendingScheduledTask(sessionID, draft)
		return scheduledTaskConfirmation(draft)
	}
}

func (h *Handler) getPendingScheduledTask(sessionID string) (pendingScheduledTask, bool) {
	h.pendingTaskMu.Lock()
	defer h.pendingTaskMu.Unlock()
	draft, ok := h.pendingTasks[sessionID]
	return draft, ok
}

func (h *Handler) setPendingScheduledTask(sessionID string, draft pendingScheduledTask) {
	h.pendingTaskMu.Lock()
	h.pendingTasks[sessionID] = draft
	h.pendingTaskMu.Unlock()
}

func (h *Handler) clearPendingScheduledTask(sessionID string) {
	h.pendingTaskMu.Lock()
	delete(h.pendingTasks, sessionID)
	h.pendingTaskMu.Unlock()
}

func (h *Handler) handlePendingSelection(ctx context.Context, sessionID, msg string, actor intentRouter.Actor) (string, bool, error) {
	index, ok := parseSelection(msg)
	if !ok {
		return "", false, nil
	}

	// Priority 1: pending disambiguation (MultipleDealsError).
	h.pendingMu.Lock()
	pending, exists := h.pendingDeals[sessionID]
	h.pendingMu.Unlock()

	if exists {
		if index < 0 || index >= len(pending.Deals) {
			return fmt.Sprintf("Escolha um numero entre 1 e %d.", len(pending.Deals)), true, nil
		}

		selected := pending.Deals[index]
		intent := pending.Intent
		intent.Parameters["deal_name"] = selected.Name
		intent.RawText = fmt.Sprintf("%s (opcao %d: %s)", intent.RawText, index+1, selected.Name)

		result, err := h.router.ResolveDealSelection(ctx, intent, selected, actor)
		if err != nil {
			return h.handleRouteError(err, sessionID, intent), true, nil
		}

		h.setActiveDeal(sessionID, selected)

		if task, ok := result.(domain.Task); ok {
			return formatCreatedTask(task), true, nil
		}

		formatted, err := h.nlpSvc.FormatResponse(ctx, intent, result)
		if err != nil {
			h.logger.WarnContext(ctx, "nlp format selection response failed", "session_id", sessionID, "error", err)
			return nlp.ErrorResponse("servico de formatacao"), true, nil
		}
		return formatted, true, nil
	}

	// Priority 2: pending list from GetDeals.
	h.pendingListsMu.Lock()
	list, listExists := h.pendingLists[sessionID]
	h.pendingListsMu.Unlock()

	if listExists {
		if index < 0 || index >= len(list) {
			return fmt.Sprintf("Escolha um numero entre 1 e %d.", len(list)), true, nil
		}
		selected := list[index]
		h.setActiveDeal(sessionID, selected)
		h.clearPendingList(sessionID)
		return fmt.Sprintf("Selecionei *%s* (etapa: %s). O que deseja saber sobre essa negociação?", selected.Name, selected.Stage.Name), true, nil
	}

	return "", false, nil
}

func (h *Handler) clearPendingSelection(sessionID string) {
	h.pendingMu.Lock()
	delete(h.pendingDeals, sessionID)
	h.pendingMu.Unlock()
}

func (h *Handler) setPendingList(sessionID string, deals []domain.Deal) {
	h.pendingListsMu.Lock()
	h.pendingLists[sessionID] = deals
	h.pendingListsMu.Unlock()
}

func (h *Handler) clearPendingList(sessionID string) {
	h.pendingListsMu.Lock()
	delete(h.pendingLists, sessionID)
	h.pendingListsMu.Unlock()
}

func (h *Handler) setActiveDeal(sessionID string, deal domain.Deal) {
	h.activeDealsMu.Lock()
	h.activeDeals[sessionID] = deal
	h.activeDealsMu.Unlock()
}

func (h *Handler) getActiveDeal(sessionID string) (domain.Deal, bool) {
	h.activeDealsMu.Lock()
	d, ok := h.activeDeals[sessionID]
	h.activeDealsMu.Unlock()
	return d, ok
}

func (h *Handler) handleRouteError(err error, sessionID string, intent domain.Intent) string {
	if errors.Is(err, intentRouter.ErrPermissionDenied) {
		return "Voce nao tem permissao para executar essa acao. Vendedores acessam apenas os proprios negocios, supervisores acessam os proprios negocios e os da equipe, e exclusoes ficam restritas a diretoria pelo fluxo de aprovacao do RD Station."
	}
	if errors.Is(err, intentRouter.ErrDeletionRequiresApproval) {
		return "A exclusao de negociacoes deve ser feita pelo fluxo de aprovacao do RD Station. Como diretoria, voce pode solicitar a exclusao por la para que o processo registre a aprovacao corretamente."
	}

	var multiErr *rdSvc.MultipleDealsError
	if errors.As(err, &multiErr) {
		h.rememberDealSelection(sessionID, intent, multiErr.Deals)
		names := make([]string, 0, len(multiErr.Deals))
		for i, d := range multiErr.Deals {
			names = append(names, fmt.Sprintf("%d. %s", i+1, d.Name))
		}
		return fmt.Sprintf("Encontrei mais de uma negociacao com esse nome. Qual delas voce quer?\n%s", strings.Join(names, "\n"))
	}

	var stageErr *rdSvc.StageNotFoundError
	if errors.As(err, &stageErr) {
		return fmt.Sprintf("Nao encontrei o estagio informado. Os estagios disponiveis sao: %s", strings.Join(stageErr.Available, ", "))
	}

	var notFoundErr *rdSvc.DealNotFoundError
	if errors.As(err, &notFoundErr) {
		return fmt.Sprintf("Nao encontrei nenhuma negociacao com o nome \"%s\". Verifique o nome e tente novamente.", notFoundErr.Name)
	}

	var missingName *rdSvc.MissingDealNameError
	if errors.As(err, &missingName) {
		return "Preciso saber o nome da negociacao. Pode informar?"
	}

	var contactNotFound *rdSvc.ContactNotFoundError
	if errors.As(err, &contactNotFound) {
		if len(contactNotFound.Suggestions) > 0 {
			return fmt.Sprintf("Nao encontrei nenhum contato com o nome \"%s\". Nomes mais proximos:\n%s", contactNotFound.Name, numberedOptions(contactNotFound.Suggestions))
		}
		return fmt.Sprintf("Nao encontrei nenhum contato com o nome \"%s\". Verifique o nome e tente novamente.", contactNotFound.Name)
	}

	var ownerNotFound *rdSvc.OwnerNotFoundError
	if errors.As(err, &ownerNotFound) {
		if len(ownerNotFound.Suggestions) > 0 {
			return fmt.Sprintf("Nao encontrei responsavel com o nome \"%s\". Voce quis dizer algum destes?\n%s", ownerNotFound.Name, numberedOptions(ownerNotFound.Suggestions))
		}
		return fmt.Sprintf("Nao encontrei responsavel com o nome \"%s\". Verifique o nome e tente novamente.", ownerNotFound.Name)
	}

	h.logger.Warn("route error", "intent", string(intent.Name), "error", err)
	return nlp.ErrorResponse("RD Station")
}

func (h *Handler) actorForPhone(ctx context.Context, phone string) intentRouter.Actor {
	if h.accessProfiler == nil {
		return intentRouter.Actor{Role: "director"}
	}
	profile, err := h.accessProfiler.AccessProfile(ctx, phone)
	if err != nil {
		h.logger.WarnContext(ctx, "access profile lookup failed", "from", maskPhone(phone), "error", err)
		return intentRouter.Actor{Role: "seller"}
	}
	return intentRouter.Actor{Role: profile.Role, RDStationID: profile.RDStationID, TeamRDUserIDs: profile.TeamRDUserIDs}
}

func isGreeting(msg string) bool {
	normalized := strings.ToLower(strings.TrimSpace(msg))
	normalized = strings.Trim(normalized, "!.? ")
	return normalized == "oi"
}

func isDeleteRequest(msg string) bool {
	normalized := strings.ToLower(strings.TrimSpace(msg))
	return strings.Contains(normalized, "apagar") || strings.Contains(normalized, "excluir") || strings.Contains(normalized, "deletar")
}

func isAffirmative(msg string) bool {
	normalized := normalizeIntentText(msg)
	return strings.Contains(normalized, " sim ") || strings.Contains(normalized, " pode ") || strings.Contains(normalized, " confirma ") || strings.Contains(normalized, " confirmar ")
}

func isNegative(msg string) bool {
	normalized := normalizeIntentText(msg)
	return strings.Contains(normalized, " nao ") || strings.Contains(normalized, " cancelar ") || strings.Contains(normalized, " cancela ")
}

func numberedOptions(options []string) string {
	lines := make([]string, 0, len(options))
	for i, option := range options {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, option))
	}
	return strings.Join(lines, "\n")
}

func chooseStageSuggestion(msg string, stages []string) (string, bool) {
	if index, ok := parseSelection(msg); ok && index >= 0 && index < len(stages) {
		return stages[index], true
	}
	candidate := extractStageCandidate(msg)
	normalizedCandidate := normalizeIntentText(candidate)
	for _, stage := range stages {
		if normalizeIntentText(stage) == normalizedCandidate {
			return stage, true
		}
	}
	for _, stage := range stages {
		normalizedStage := normalizeIntentText(stage)
		if strings.Contains(normalizedCandidate, normalizedStage) || strings.Contains(normalizedStage, normalizedCandidate) {
			return stage, true
		}
	}
	return "", false
}

func extractStageCandidate(msg string) string {
	value := strings.TrimSpace(msg)
	normalized := normalizeIntentText(value)
	markers := []string{" estagio ", " etapa "}
	for _, marker := range markers {
		if idx := strings.LastIndex(normalized, marker); idx >= 0 {
			after := strings.TrimSpace(normalized[idx+len(marker):])
			if after != "" {
				return after
			}
		}
	}
	return value
}

func prepareCreateDealDraft(draft *pendingCreateDeal) {
	if draft == nil {
		return
	}
	p := draft.Intent.Parameters
	company := strings.TrimSpace(p["company"])
	product := strings.TrimSpace(p["product"])
	name := strings.TrimSpace(p["name"])
	if company != "" && product != "" && (name == "" || isGenericDealName(name)) {
		p["name"] = company + " - " + product
	}
}

func isGenericDealName(name string) bool {
	normalized := strings.TrimSpace(normalizeIntentText(name))
	switch normalized {
	case "card", "negociacao", "negociacao nova", "nova negociacao":
		return true
	default:
		return false
	}
}

func createDealConfirmation(draft pendingCreateDeal) string {
	p := draft.Intent.Parameters
	lines := []string{
		"Vou criar a negociacao com estes dados:",
		"",
		fmt.Sprintf("*Nome:* %s", p["name"]),
	}
	if p["company"] != "" {
		lines = append(lines, fmt.Sprintf("*Cliente:* %s", p["company"]))
	}
	if p["product"] != "" {
		lines = append(lines, fmt.Sprintf("*Produto:* %s", p["product"]))
	}
	lines = append(lines, fmt.Sprintf("*Contato no cliente:* %s", p["contact_name"]))
	if p["owner_name"] != "" {
		lines = append(lines, fmt.Sprintf("*Vendedor:* %s", p["owner_name"]))
	}
	lines = append(lines, fmt.Sprintf("*Etapa:* %s", p["stage"]))
	if p["notes"] != "" {
		lines = append(lines, fmt.Sprintf("*Observacoes:* %s", p["notes"]))
	}
	lines = append(lines, "", "Posso criar agora? Responda *sim* para confirmar ou *nao* para cancelar.")
	return strings.Join(lines, "\n")
}

const rdTaskPageSize = 10

var (
	fileURIRegex     = regexp.MustCompile(`(?i)file:///[^\s|]+`)
	windowsPathRegex = regexp.MustCompile(`[A-Za-z]:\\[^\s|]+`)
	unixPathRegex    = regexp.MustCompile(`(?:^|\s)/(?:Users|home|tmp|var|mnt|Volumes)/[^\s|]+`)
	controlCharRegex = regexp.MustCompile(`[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]`)
	spaceRegex       = regexp.MustCompile(`\s{2,}`)
)

type taskTemporalStatus int

const (
	taskStatusOverdue taskTemporalStatus = iota
	taskStatusToday
	taskStatusTomorrow
	taskStatusThisWeek
	taskStatusNextWeek
	taskStatusFuture
	taskStatusInvalidDate
)

type formattedRDTask struct {
	Subject          string
	DealName         string
	ResponsibleNames []string
	DateText         string
	Notes            string
	Status           taskTemporalStatus
	SortTime         time.Time
}

type rdTaskPagination struct {
	Tasks []formattedRDTask
	Next  int
}

func (h *Handler) formatAndRememberScheduledTasks(sessionID string, tasks []adminSvc.ScheduledTaskSummary) string {
	now := nowInSaoPaulo()
	rdTasks := collectFormattedRDTasks(tasks, now)
	if len(rdTasks) > 0 {
		if len(rdTasks) > rdTaskPageSize {
			h.rdTaskPagesMu.Lock()
			h.rdTaskPages[sessionID] = rdTaskPagination{Tasks: rdTasks, Next: rdTaskPageSize}
			h.rdTaskPagesMu.Unlock()
		} else {
			h.clearRDTaskPagination(sessionID)
		}
		return formatRDTaskPage(rdTasks, 0, rdTaskPageSize)
	}
	h.clearRDTaskPagination(sessionID)
	return formatScheduledAlertSummaries(tasks)
}

func formatScheduledTasks(tasks []adminSvc.ScheduledTaskSummary) string {
	rdTasks := collectFormattedRDTasks(tasks, nowInSaoPaulo())
	if len(rdTasks) > 0 {
		return formatRDTaskPage(rdTasks, 0, rdTaskPageSize)
	}
	return formatScheduledAlertSummaries(tasks)
}

func collectFormattedRDTasks(summaries []adminSvc.ScheduledTaskSummary, now time.Time) []formattedRDTask {
	items := make([]formattedRDTask, 0)
	for _, summary := range summaries {
		for _, task := range summary.PendingRDTasks {
			items = append(items, prepareFormattedRDTask(task, now))
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i], items[j]
		if taskStatusSortRank(left.Status) != taskStatusSortRank(right.Status) {
			return taskStatusSortRank(left.Status) < taskStatusSortRank(right.Status)
		}
		if left.SortTime.IsZero() {
			return false
		}
		if right.SortTime.IsZero() {
			return true
		}
		return left.SortTime.Before(right.SortTime)
	})
	return items
}

func prepareFormattedRDTask(task adminSvc.ScheduledRDPendingTask, now time.Time) formattedRDTask {
	when, dateText := parseRDTaskDateTime(task.Date, task.Hour)
	status := calculateTaskTemporalStatus(when, now)
	names := make([]string, 0, len(task.ResponsibleNames))
	for _, name := range task.ResponsibleNames {
		if sanitized := sanitizeTaskText(name); sanitized != "" {
			names = append(names, sanitized)
		}
	}
	return formattedRDTask{
		Subject:          valueOrDefault(sanitizeTaskText(task.Subject), "Sem assunto"),
		DealName:         sanitizeTaskText(task.DealName),
		ResponsibleNames: names,
		DateText:         dateText,
		Notes:            sanitizeTaskText(task.Notes),
		Status:           status,
		SortTime:         when,
	}
}

func formatRDTaskPage(tasks []formattedRDTask, start, limit int) string {
	if len(tasks) == 0 {
		return "Nao ha tarefas pendentes do RD Station no momento."
	}
	if start < 0 {
		start = 0
	}
	if limit <= 0 {
		limit = rdTaskPageSize
	}
	end := start + limit
	if end > len(tasks) {
		end = len(tasks)
	}
	lines := []string{
		"*Tarefas pendentes do RD Station*",
		"",
		fmt.Sprintf("Foram encontradas %d tarefas:", len(tasks)),
	}
	for i := start; i < end; i++ {
		task := tasks[i]
		lines = append(lines, "", fmt.Sprintf("%d. *%s*", i+1, task.Subject))
		if task.DealName != "" {
			lines = append(lines, "Negociacao: "+task.DealName)
		}
		if len(task.ResponsibleNames) > 0 {
			lines = append(lines, "Responsavel: "+strings.Join(task.ResponsibleNames, ", "))
		}
		if task.DateText != "" {
			lines = append(lines, "Data: "+task.DateText)
		}
		lines = append(lines, "Status: "+taskTemporalStatusText(task.Status))
		if task.Notes != "" {
			lines = append(lines, "Observacao: "+task.Notes)
		}
	}
	if end < len(tasks) {
		lines = append(lines, "", fmt.Sprintf("Exibindo %d de %d tarefas. Digite \"ver proximas\" para continuar.", end, len(tasks)))
	}
	return strings.Join(lines, "\n")
}

func formatScheduledAlertSummaries(tasks []adminSvc.ScheduledTaskSummary) string {
	if len(tasks) == 0 {
		return "Nao ha tarefas agendadas cadastradas no momento."
	}
	lines := []string{"Tarefas agendadas:"}
	for i, task := range tasks {
		name := valueOrDefault(sanitizeTaskText(task.Name), "Tarefa sem nome")
		status := "inativa"
		if task.Active {
			status = "ativa"
		}
		lines = append(lines, "", fmt.Sprintf("%d. *%s* (%s)", i+1, name, status))
		if stage := sanitizeTaskText(task.DealStageName); stage != "" {
			lines = append(lines, "Etapa monitorada: "+stage)
		}
		if task.TimeThresholdHours > 0 {
			lines = append(lines, fmt.Sprintf("Regra: pendente ha mais de %d horas", task.TimeThresholdHours))
		}
		if !task.LastCheckedAt.IsZero() {
			lines = append(lines, fmt.Sprintf("Ultima verificacao: %s", task.LastCheckedAt.In(saoPauloLocation()).Format("02/01/2006 15:04")))
		}
		if len(task.PendingDeals) == 0 {
			lines = append(lines, "Pendencias: nenhuma negociacao pendente agora.")
			continue
		}
		lines = append(lines, fmt.Sprintf("Pendencias: %d negociacao(oes)", len(task.PendingDeals)))
		limit := len(task.PendingDeals)
		if limit > 8 {
			limit = 8
		}
		for j := 0; j < limit; j++ {
			deal := task.PendingDeals[j]
			parts := []string{sanitizeTaskText(deal.Name)}
			if responsible := sanitizeTaskText(deal.ResponsibleName); responsible != "" {
				parts = append(parts, "responsavel "+responsible)
			}
			if contact := sanitizeTaskText(deal.ContactName); contact != "" {
				parts = append(parts, "contato "+contact)
			}
			parts = append(parts, fmt.Sprintf("parada ha %d dia(s)", deal.DaysPending))
			if deal.AlreadyNotified {
				parts = append(parts, "ja notificada na janela atual")
			}
			lines = append(lines, "- "+strings.Join(nonEmptyStrings(parts), " | "))
		}
		if len(task.PendingDeals) > limit {
			lines = append(lines, fmt.Sprintf("- ...mais %d pendencia(s)", len(task.PendingDeals)-limit))
		}
	}
	return strings.Join(lines, "\n")
}

func (h *Handler) handleRDTaskPagination(sessionID, msg string) (string, bool) {
	normalized := normalizeIntentText(msg)
	if !(strings.Contains(normalized, " ver proximas ") || strings.Contains(normalized, " proximas ") || strings.Contains(normalized, " proxima pagina ")) {
		return "", false
	}
	h.rdTaskPagesMu.Lock()
	page, ok := h.rdTaskPages[sessionID]
	if !ok || page.Next >= len(page.Tasks) {
		h.rdTaskPagesMu.Unlock()
		return "Nao ha proximas tarefas para exibir.", true
	}
	start := page.Next
	page.Next += rdTaskPageSize
	if page.Next >= len(page.Tasks) {
		delete(h.rdTaskPages, sessionID)
	} else {
		h.rdTaskPages[sessionID] = page
	}
	h.rdTaskPagesMu.Unlock()
	return formatRDTaskPage(page.Tasks, start, rdTaskPageSize), true
}

func (h *Handler) clearRDTaskPagination(sessionID string) {
	h.rdTaskPagesMu.Lock()
	delete(h.rdTaskPages, sessionID)
	h.rdTaskPagesMu.Unlock()
}

func formatRDTaskDate(date, hour string) string {
	_, text := parseRDTaskDateTime(date, hour)
	return text
}

func parseRDTaskDateTime(date, hour string) (time.Time, string) {
	date = strings.TrimSpace(date)
	hour = strings.TrimSpace(hour)
	if date == "" {
		return time.Time{}, ""
	}
	loc := saoPauloLocation()
	var day time.Time
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02", "02/01/2006", "02-01-2006"} {
		if parsed, err := time.ParseInLocation(layout, date, loc); err == nil {
			day = parsed.In(loc)
			break
		}
	}
	if day.IsZero() {
		return time.Time{}, sanitizeTaskText(date)
	}
	if hour == "" {
		return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc), day.Format("02/01/2006")
	}
	cleanHour := normalizeTaskHour(hour)
	if parsedHour, err := time.Parse("15:04", cleanHour); err == nil {
		when := time.Date(day.Year(), day.Month(), day.Day(), parsedHour.Hour(), parsedHour.Minute(), 0, 0, loc)
		return when, fmt.Sprintf("%s as %s", day.Format("02/01/2006"), cleanHour)
	}
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc), day.Format("02/01/2006")
}

func calculateTaskTemporalStatus(taskDate, now time.Time) taskTemporalStatus {
	if taskDate.IsZero() {
		return taskStatusInvalidDate
	}
	loc := saoPauloLocation()
	taskDay := dateOnly(taskDate.In(loc))
	today := dateOnly(now.In(loc))
	switch {
	case taskDay.Before(today):
		return taskStatusOverdue
	case taskDay.Equal(today):
		return taskStatusToday
	case taskDay.Equal(today.AddDate(0, 0, 1)):
		return taskStatusTomorrow
	}
	endOfCurrentWeek := endOfWeek(today)
	if !taskDay.After(endOfCurrentWeek) {
		return taskStatusThisWeek
	}
	if !taskDay.After(endOfCurrentWeek.AddDate(0, 0, 7)) {
		return taskStatusNextWeek
	}
	return taskStatusFuture
}

func dateOnly(value time.Time) time.Time {
	loc := value.Location()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, loc)
}

func endOfWeek(day time.Time) time.Time {
	daysUntilSunday := (int(time.Sunday) - int(day.Weekday()) + 7) % 7
	return day.AddDate(0, 0, daysUntilSunday)
}

func taskStatusSortRank(status taskTemporalStatus) int {
	switch status {
	case taskStatusOverdue:
		return 0
	case taskStatusToday:
		return 1
	case taskStatusTomorrow:
		return 2
	case taskStatusThisWeek, taskStatusNextWeek, taskStatusFuture:
		return 3
	default:
		return 4
	}
}

func taskTemporalStatusText(status taskTemporalStatus) string {
	switch status {
	case taskStatusOverdue:
		return "Atrasada"
	case taskStatusToday:
		return "Para hoje"
	case taskStatusTomorrow:
		return "Amanha"
	case taskStatusThisWeek:
		return "Nesta semana"
	case taskStatusNextWeek:
		return "Proxima semana"
	case taskStatusFuture:
		return "Futura"
	default:
		return "Data invalida"
	}
}

func translateTaskMarkup(markup string) string {
	return taskMarkupDisplayText(markup)
}

func taskMarkupDisplayText(markup string) string {
	switch strings.ToLower(strings.TrimSpace(markup)) {
	case "past":
		return "Atrasada"
	case "today":
		return "Para hoje"
	case "tomorrow":
		return "Amanha"
	case "week-0":
		return "Nesta semana"
	case "week-1":
		return "Proxima semana"
	case "future":
		return "Futura"
	default:
		return "Nao informado"
	}
}

func sanitizeTaskText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = fileURIRegex.ReplaceAllString(value, "")
	value = windowsPathRegex.ReplaceAllString(value, "")
	value = unixPathRegex.ReplaceAllString(value, " ")
	value = controlCharRegex.ReplaceAllString(value, "")
	value = spaceRegex.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

func valueOrDefault(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func prepareScheduledTaskDraft(draft *pendingScheduledTask) {
	if draft == nil {
		return
	}
	p := draft.Intent.Parameters
	if p["type"] == "" {
		p["type"] = "task"
	}
	if p["date"] != "" {
		p["date"] = normalizeTaskDate(p["date"])
	}
	if p["hour"] != "" {
		p["hour"] = normalizeTaskHour(p["hour"])
	}
}

func clearUnmentionedScheduledTaskDefaults(draft *pendingScheduledTask) {
	raw := normalizeIntentText(draft.Intent.RawText)
	if raw == "" {
		return
	}
	p := draft.Intent.Parameters
	subject := strings.TrimSpace(p["subject"])
	if subject != "" && !scheduledTaskSubjectMentioned(raw, subject) {
		p["subject"] = ""
	}
	date := strings.TrimSpace(p["date"])
	if date != "" && !scheduledTaskDateMentioned(raw, date) {
		p["date"] = ""
	}
	hour := strings.TrimSpace(p["hour"])
	if hour != "" && !scheduledTaskHourMentioned(raw, hour) {
		p["hour"] = ""
	}
}

func scheduledTaskSubjectMentioned(raw, subject string) bool {
	normalizedSubject := strings.TrimSpace(normalizeIntentText(subject))
	if normalizedSubject == "" || normalizedSubject == "nova tarefa" || normalizedSubject == "tarefa" {
		return false
	}
	return strings.Contains(raw, normalizedSubject)
}

func scheduledTaskDateMentioned(raw, date string) bool {
	if strings.Contains(raw, " hoje ") || strings.Contains(raw, " amanha ") || strings.Contains(raw, " amanhã ") || strings.Contains(raw, " agora ") {
		return true
	}
	return strings.Contains(raw, normalizeIntentText(date)) || regexp.MustCompile(`\b\d{1,2}[/-]\d{1,2}([/-]\d{2,4})?\b`).FindString(raw) != ""
}

func scheduledTaskHourMentioned(raw, hour string) bool {
	if strings.Contains(raw, " agora ") || strings.Contains(raw, " horario ") || strings.Contains(raw, " horário ") || strings.Contains(raw, " hora ") {
		return true
	}
	normalizedHour := strings.TrimSuffix(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(hour)), "h", ":"), ":")
	return normalizedHour != "" && strings.Contains(raw, normalizeIntentText(normalizedHour))
}

func applyScheduledTaskCorrections(draft *pendingScheduledTask, msg string) bool {
	if draft == nil {
		return false
	}
	changed := false
	raw := strings.TrimSpace(msg)
	normalized := normalizeIntentText(raw)
	p := draft.Intent.Parameters

	if subject := extractScheduledTaskSubject(raw); subject != "" {
		p["subject"] = subject
		changed = true
	}
	if strings.Contains(normalized, " agora ") {
		now := nowInSaoPaulo()
		p["date"] = now.Format("2006-01-02")
		p["hour"] = now.Format("15:04")
		changed = true
	} else {
		if date := extractScheduledTaskDate(raw); date != "" {
			p["date"] = normalizeTaskDate(date)
			changed = true
		}
		if hour := extractScheduledTaskHour(raw); hour != "" {
			p["hour"] = normalizeTaskHour(hour)
			changed = true
		}
	}
	if dealName := extractScheduledTaskDealName(raw); dealName != "" {
		p["deal_name"] = dealName
		changed = true
	}
	return changed
}

func extractScheduledTaskSubject(value string) string {
	patterns := []string{
		`(?i)\bassunto\s+(?:deve\s+(?:ser|ter)|e|é|eh|ser|para)\s+(.+?)(?:,\s*|\s+e\s+(?:a\s+)?(?:hora|horario|horário|data)\b|$)`,
		`(?i)\bcom\s+assunto\s+(.+?)(?:,\s*|\s+e\s+(?:a\s+)?(?:hora|horario|horário|data)\b|$)`,
	}
	return firstRegexGroup(value, patterns)
}

func extractScheduledTaskDate(value string) string {
	normalized := normalizeIntentText(value)
	for _, token := range []string{"hoje", "amanha", "amanhã"} {
		if strings.Contains(normalized, " "+token+" ") {
			return token
		}
	}
	return regexp.MustCompile(`\b\d{1,2}[/-]\d{1,2}(?:[/-]\d{2,4})?\b|\b\d{4}-\d{2}-\d{2}\b`).FindString(value)
}

func extractScheduledTaskHour(value string) string {
	hour := firstRegexGroup(value, []string{`(?i)\b(?:hora|horario|horário)\s+(?:deve\s+(?:ser|ter)|e|é|eh|ser|para|as|às)?\s*(\d{1,2}(?::\d{2}|h\d{0,2})?)`})
	if hour != "" {
		return hour
	}
	return regexp.MustCompile(`\b\d{1,2}:\d{2}\b|\b\d{1,2}h\d{0,2}\b`).FindString(value)
}

func extractScheduledTaskDealName(value string) string {
	patterns := []string{
		`(?i)\bnegociacao\s+(?:deve\s+ser|e|é|eh|para)\s+(.+?)(?:,\s*|\s+e\s+(?:o\s+)?(?:assunto|horario|horário|hora|data)\b|$)`,
		`(?i)\bnegociação\s+(?:deve\s+ser|e|é|eh|para)\s+(.+?)(?:,\s*|\s+e\s+(?:o\s+)?(?:assunto|horario|horário|hora|data)\b|$)`,
	}
	return firstRegexGroup(value, patterns)
}

func firstRegexGroup(value string, patterns []string) string {
	for _, pattern := range patterns {
		matches := regexp.MustCompile(pattern).FindStringSubmatch(value)
		if len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}
	return ""
}

func scheduledTaskConfirmation(draft pendingScheduledTask) string {
	p := draft.Intent.Parameters
	lines := []string{
		"Vou criar esta tarefa no RD:",
		"",
		fmt.Sprintf("*Negociacao:* %s", p["deal_name"]),
		fmt.Sprintf("*Assunto:* %s", p["subject"]),
		fmt.Sprintf("*Data:* %s", p["date"]),
		fmt.Sprintf("*Horario:* %s", p["hour"]),
		fmt.Sprintf("*Tipo:* %s", p["type"]),
	}
	if p["owner_name"] != "" {
		lines = append(lines, fmt.Sprintf("*Responsavel:* %s", p["owner_name"]))
	}
	if p["notes"] != "" {
		lines = append(lines, fmt.Sprintf("*Observacoes:* %s", p["notes"]))
	}
	lines = append(lines, "", "Posso criar agora? Responda *sim* para confirmar ou *nao* para cancelar.")
	return strings.Join(lines, "\n")
}

func formatCreatedTask(task domain.Task) string {
	lines := []string{
		"Tarefa criada com sucesso.",
		"",
		fmt.Sprintf("*Assunto:* %s", task.Subject),
	}
	if task.DealName != "" {
		lines = append(lines, fmt.Sprintf("*Negociacao:* %s", task.DealName))
	}
	if task.Date != "" {
		lines = append(lines, fmt.Sprintf("*Data:* %s", formatRDTaskDate(task.Date, task.Hour)))
	}
	if len(task.ResponsibleNames) > 0 {
		lines = append(lines, fmt.Sprintf("*Responsavel:* %s", strings.Join(task.ResponsibleNames, ", ")))
	}
	if task.Notes != "" {
		lines = append(lines, fmt.Sprintf("*Observacoes:* %s", task.Notes))
	}
	return strings.Join(lines, "\n")
}

func normalizeTaskDate(value string) string {
	value = strings.TrimSpace(value)
	normalized := strings.TrimSpace(normalizeIntentText(value))
	now := nowInSaoPaulo()
	switch normalized {
	case "hoje":
		return now.Format("2006-01-02")
	case "amanha", "amanhã":
		return now.AddDate(0, 0, 1).Format("2006-01-02")
	}
	for _, layout := range []string{"2006-01-02", "02/01/2006", "02-01-2006", "02/01/06", "02-01-06"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("2006-01-02")
		}
	}
	return value
}

func normalizeTaskHour(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if normalizeIntentText(value) == "agora" {
		return nowInSaoPaulo().Format("15:04")
	}
	value = strings.ReplaceAll(value, "h", ":")
	value = strings.TrimSuffix(value, ":")
	for _, layout := range []string{"15:04", "15"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("15:04")
		}
	}
	return value
}

func nowInSaoPaulo() time.Time {
	return time.Now().In(saoPauloLocation())
}

func saoPauloLocation() *time.Location {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return time.FixedZone("America/Sao_Paulo", -3*60*60)
	}
	return loc
}

func normalizeDealOwnerIntent(intent *domain.Intent) {
	if intent.Name != domain.IntentGetDeals || intent.Parameters == nil {
		return
	}
	if strings.TrimSpace(intent.Parameters["owner_name"]) != "" {
		return
	}
	raw := normalizeIntentText(intent.RawText)
	if !(strings.Contains(raw, " responsavel") ||
		strings.Contains(raw, " responsaveis") ||
		strings.Contains(raw, " vendedor") ||
		strings.Contains(raw, " dono") ||
		strings.Contains(raw, " do ") ||
		strings.Contains(raw, " da ") ||
		strings.Contains(raw, " atribuida") ||
		strings.Contains(raw, " atribuidas")) {
		return
	}
	if name := strings.TrimSpace(intent.Parameters["name"]); name != "" {
		intent.Parameters["owner_name"] = name
		delete(intent.Parameters, "name")
	} else if name := strings.TrimSpace(intent.Parameters["deal_name"]); name != "" {
		intent.Parameters["owner_name"] = name
		delete(intent.Parameters, "deal_name")
	}
}

func normalizeIntentText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(
		"á", "a", "à", "a", "ã", "a", "â", "a",
		"é", "e", "ê", "e",
		"í", "i",
		"ó", "o", "õ", "o", "ô", "o",
		"ú", "u",
		"ç", "c",
		";", " ",
	).Replace(value)
	return " " + strings.Join(strings.Fields(value), " ") + " "
}

func GreetingResponse() string {
	return `Olá, tudo bem? Eu sou a Sil, a inteligência artificial da Silmax.

Em breve, você poderá contar comigo para acompanhar seus negócios em andamento junto à Silmax, consultar informações sobre negociações, identificar clientes que precisam de contato, acompanhar relatórios, verificar status de processos e acessar outros recursos de apoio à operação comercial.

Por segurança, o acesso às funcionalidades da Sil será realizado exclusivamente pelo número de telefone que está recebendo esta mensagem. Outros números não terão acesso aos recursos e informações vinculados à sua conta.

Neste momento, estou em fase de implantação, e minhas funcionalidades estão sendo preparadas para oferecer uma experiência segura, estável e eficiente.

Por favor, salve este número em seus contatos. Assim que eu estiver disponível para utilização, você será informado por aqui.

Até breve.

Sil

Silmax
Excellence and Quality`
}

func (h *Handler) rememberDealSelection(sessionID string, intent domain.Intent, deals []domain.Deal) {
	if len(deals) == 0 {
		return
	}
	if intent.Parameters == nil {
		intent.Parameters = map[string]string{}
	}
	h.pendingMu.Lock()
	h.pendingDeals[sessionID] = pendingDealSelection{Intent: intent, Deals: deals}
	h.pendingMu.Unlock()
}

type pendingDealSelection struct {
	Intent domain.Intent
	Deals  []domain.Deal
}

type pendingCreateDeal struct {
	Intent               domain.Intent
	WaitingForSuggestion string
	Suggestions          []string
}

func (p pendingCreateDeal) nextField() string {
	prepareCreateDealDraft(&p)
	if strings.TrimSpace(p.Intent.Parameters["name"]) == "" {
		if strings.TrimSpace(p.Intent.Parameters["company"]) == "" {
			return "company"
		}
		if strings.TrimSpace(p.Intent.Parameters["product"]) == "" {
			return "product"
		}
		return "name"
	}
	if strings.TrimSpace(p.Intent.Parameters["contact_name"]) == "" {
		return "contact_name"
	}
	if strings.TrimSpace(p.Intent.Parameters["stage"]) == "" {
		return "stage"
	}
	return "confirm"
}

type pendingScheduledTask struct {
	Intent domain.Intent
}

func (p pendingScheduledTask) nextField() string {
	if strings.TrimSpace(p.Intent.Parameters["deal_name"]) == "" {
		return "deal_name"
	}
	if strings.TrimSpace(p.Intent.Parameters["subject"]) == "" {
		return "subject"
	}
	if strings.TrimSpace(p.Intent.Parameters["date"]) == "" {
		return "date"
	}
	if strings.TrimSpace(p.Intent.Parameters["hour"]) == "" {
		return "hour"
	}
	return "confirm"
}

type inboundTextMessage struct {
	From string
	ID   string
	Body string
}

type webhookPayload struct {
	Event    string `json:"event"`
	Instance string `json:"instance"`
	Data     struct {
		Key struct {
			RemoteJID    string `json:"remoteJid"`
			RemoteJIDAlt string `json:"remoteJidAlt"`
			FromMe       bool   `json:"fromMe"`
			ID           string `json:"id"`
		} `json:"key"`
		Message struct {
			Conversation        string `json:"conversation"`
			ExtendedTextMessage struct {
				Text string `json:"text"`
			} `json:"extendedTextMessage"`
			AudioMessage struct {
				Mimetype string `json:"mimetype"`
			} `json:"audioMessage"`
			Base64 string `json:"base64"`
		} `json:"message"`
		MessageBase64 string `json:"messageBase64"`
		Base64        string `json:"base64"`
		MessageType   string `json:"messageType"`
	} `json:"data"`
}

func (p webhookPayload) inboundMessage(ctx context.Context, transcriber nlp.ServiceInterface, fetcher MediaFetcher, logger *slog.Logger) (inboundTextMessage, bool) {
	if p.Data.Key.FromMe {
		return inboundTextMessage{}, false
	}

	body := p.Data.Message.Conversation
	if body == "" {
		body = p.Data.Message.ExtendedTextMessage.Text
	}
	body = strings.TrimSpace(body)
	from := normalizeNumber(p.Data.Key.RemoteJID)
	if strings.HasSuffix(p.Data.Key.RemoteJID, "@lid") && p.Data.Key.RemoteJIDAlt != "" {
		from = normalizeNumber(p.Data.Key.RemoteJIDAlt)
	}
	if body == "" && isAudioMessageType(p.Data.MessageType) {
		if audio, ok := p.audioBytes(); ok {
			text, err := transcriber.TranscribeAudio(ctx, audio, audioFilename(p.Data.Message.AudioMessage.Mimetype))
			if err == nil {
				body = strings.TrimSpace(text)
			} else if logger != nil {
				logger.WarnContext(ctx, "audio transcription failed", "message_id", p.Data.Key.ID, "error", err)
			}
		} else if fetcher != nil && p.Data.Key.RemoteJID != "" && p.Data.Key.ID != "" {
			raw, mimetype, err := fetcher.FetchMediaBase64(ctx, p.Data.Key.RemoteJID, p.Data.Key.ID, p.Data.Key.FromMe)
			if err != nil {
				if logger != nil {
					logger.WarnContext(ctx, "audio base64 fetch failed", "message_id", p.Data.Key.ID, "error", err)
				}
			} else if audio, ok := decodeBase64(raw); ok {
				if mimetype == "" {
					mimetype = p.Data.Message.AudioMessage.Mimetype
				}
				text, err := transcriber.TranscribeAudio(ctx, audio, audioFilename(mimetype))
				if err == nil {
					body = strings.TrimSpace(text)
				} else if logger != nil {
					logger.WarnContext(ctx, "audio transcription failed", "message_id", p.Data.Key.ID, "error", err)
				}
			}
		}
	}

	if body == "" || from == "" {
		return inboundTextMessage{}, false
	}

	return inboundTextMessage{From: from, ID: p.Data.Key.ID, Body: body}, true
}

func (p webhookPayload) audioBytes() ([]byte, bool) {
	raw := strings.TrimSpace(p.Data.MessageBase64)
	if raw == "" {
		raw = strings.TrimSpace(p.Data.Message.Base64)
	}
	if raw == "" {
		raw = strings.TrimSpace(p.Data.Base64)
	}
	return decodeBase64(raw)
}

func decodeBase64(raw string) ([]byte, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	if idx := strings.Index(raw, ","); idx >= 0 {
		raw = raw[idx+1:]
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, false
	}
	return data, true
}

func isAudioMessageType(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(value, "audio")
}

func audioFilename(mimetype string) string {
	switch {
	case strings.Contains(mimetype, "mpeg"), strings.Contains(mimetype, "mp3"):
		return "audio.mp3"
	case strings.Contains(mimetype, "wav"):
		return "audio.wav"
	default:
		return "audio.ogg"
	}
}

func normalizeNumber(number string) string {
	number = strings.TrimSpace(number)
	number = strings.TrimPrefix(number, "+")
	number = strings.ReplaceAll(number, " ", "")
	number = strings.ReplaceAll(number, "-", "")
	number = strings.ReplaceAll(number, "(", "")
	number = strings.ReplaceAll(number, ")", "")
	number = strings.TrimSuffix(number, "@s.whatsapp.net")
	number = strings.TrimSuffix(number, "@c.us")
	number = strings.TrimSuffix(number, "@lid")
	return number
}

func buildAllowedNumbers(numbers []string) map[string]struct{} {
	if len(numbers) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(numbers))
	for _, number := range numbers {
		normalized := normalizeNumber(number)
		if normalized != "" {
			allowed[normalized] = struct{}{}
		}
	}
	return allowed
}

func parseSelection(msg string) (int, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(msg))
	if err != nil || value <= 0 {
		return 0, false
	}
	return value - 1, true
}

func isRecentDuplicate(history []domain.Message, msg string) bool {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return false
	}
	cutoff := time.Now().Add(-2 * time.Minute)
	for i := len(history) - 1; i >= 0; i-- {
		item := history[i]
		if item.Role != "user" {
			continue
		}
		if strings.TrimSpace(item.Content) == msg && item.CreatedAt.After(cutoff) {
			return true
		}
		return false
	}
	return false
}

func maskPhone(number string) string {
	number = normalizeNumber(number)
	if len(number) <= 4 {
		return number
	}
	return "+" + number[:4] + "***"
}
