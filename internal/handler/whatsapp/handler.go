package whatsapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"synova-rd-workflow/internal/domain"
	convSvc "synova-rd-workflow/internal/service/conversation"
	intentRouter "synova-rd-workflow/internal/service/intent_router"
	"synova-rd-workflow/internal/service/nlp"
	rdSvc "synova-rd-workflow/internal/service/rdstation"
)

var whatsappFromRegex = regexp.MustCompile(`^\d{10,15}$`)

// Sender is the contract used to send replies back through WhatsApp.
type Sender interface {
	SendTextMessage(ctx context.Context, to string, text string) error
}

type MediaDownloader interface {
	DownloadMedia(ctx context.Context, mediaID string) ([]byte, string, error)
}

type AllowChecker interface {
	IsAllowed(ctx context.Context, phone string) (bool, error)
}

type AccessProfiler interface {
	AccessProfile(ctx context.Context, phone string) (domain.AccessProfile, error)
}

// Handler holds all dependencies for the WhatsApp webhook.
type Handler struct {
	conv            *convSvc.Service
	nlpSvc          nlp.ServiceInterface
	router          *intentRouter.Router
	sender          Sender
	verifyToken     string
	appSecret       string
	logger          *slog.Logger
	allowChecker    AllowChecker
	accessProfiler  AccessProfiler
	mediaDownloader MediaDownloader
	pendingMu       sync.Mutex
	pendingDeals    map[string]pendingDealSelection
	pendingListsMu  sync.Mutex
	pendingLists    map[string][]domain.Deal
	pendingCreateMu sync.Mutex
	pendingCreates  map[string]pendingCreateDeal
	pendingTaskMu   sync.Mutex
	pendingTasks    map[string]pendingScheduledTask
	activeDealsMu   sync.Mutex
	activeDeals     map[string]domain.Deal
}

func (h *Handler) SetAllowChecker(checker AllowChecker) {
	h.allowChecker = checker
	if profiler, ok := checker.(AccessProfiler); ok {
		h.accessProfiler = profiler
	}
}

// New returns a new WhatsApp webhook handler.
func New(
	conv *convSvc.Service,
	nlpSvc nlp.ServiceInterface,
	router *intentRouter.Router,
	sender Sender,
	verifyToken string,
	appSecret string,
	logger *slog.Logger,
) *Handler {
	handler := &Handler{
		conv:           conv,
		nlpSvc:         nlpSvc,
		router:         router,
		sender:         sender,
		verifyToken:    verifyToken,
		appSecret:      appSecret,
		logger:         logger,
		pendingDeals:   make(map[string]pendingDealSelection),
		pendingLists:   make(map[string][]domain.Deal),
		pendingCreates: make(map[string]pendingCreateDeal),
		pendingTasks:   make(map[string]pendingScheduledTask),
		activeDeals:    make(map[string]domain.Deal),
	}
	if downloader, ok := sender.(MediaDownloader); ok {
		handler.mediaDownloader = downloader
	}
	return handler
}

// RegisterRoutes wires the Meta verification and message webhook endpoints.
func (h *Handler) RegisterRoutes(r gin.IRouter) {
	r.GET("/webhook", h.VerifyWebhook)
	r.POST("/webhook", h.HandleWebhook)
}

// VerifyWebhook handles Meta's initial GET challenge.
func (h *Handler) VerifyWebhook(c *gin.Context) {
	if c.Query("hub.mode") != "subscribe" || c.Query("hub.verify_token") != h.verifyToken {
		c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
		return
	}

	c.String(http.StatusOK, c.Query("hub.challenge"))
}

// HandleWebhook validates the Meta signature, processes text messages, and replies through Cloud API.
func (h *Handler) HandleWebhook(c *gin.Context) {
	rawBody, err := c.GetRawData()
	if err != nil {
		h.logger.ErrorContext(c.Request.Context(), "failed to read webhook body", "error", err)
		c.JSON(http.StatusOK, gin.H{})
		return
	}

	h.logger.InfoContext(c.Request.Context(), "whatsapp webhook received",
		"body_bytes", len(rawBody),
		"has_signature", c.GetHeader("X-Hub-Signature-256") != "",
	)

	if !h.validSignature(c.GetHeader("X-Hub-Signature-256"), rawBody) {
		h.logger.WarnContext(c.Request.Context(), "invalid whatsapp webhook signature",
			"body_bytes", len(rawBody),
			"has_signature", c.GetHeader("X-Hub-Signature-256") != "",
		)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var payload metaWebhookPayload
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		h.logger.WarnContext(c.Request.Context(), "invalid whatsapp payload", "error", err)
		c.JSON(http.StatusOK, gin.H{})
		return
	}

	inbound, ok := payload.firstInboundMessage()
	if !ok {
		if status, ok := payload.firstStatus(); ok {
			args := []any{
				"message_id", status.ID,
				"status", status.Status,
				"recipient_id", normalizePhoneForLog(status.RecipientID),
			}
			if status.Errors != nil && len(status.Errors) > 0 {
				args = append(args,
					"error_code", status.Errors[0].Code,
					"error_title", status.Errors[0].Title,
					"error_message", status.Errors[0].Message,
				)
			}
			h.logger.InfoContext(c.Request.Context(), "whatsapp message status", args...)
		} else {
			h.logger.InfoContext(c.Request.Context(), "whatsapp webhook ignored", "reason", payload.ignoreReason())
		}
		c.JSON(http.StatusOK, gin.H{})
		return
	}

	if err := h.processTextMessage(c, inbound); err != nil {
		h.logger.ErrorContext(c.Request.Context(), "failed to process whatsapp message",
			"from", normalizePhoneForLog(inbound.From),
			"wamid", inbound.ID,
			"error", err,
		)
	}

	c.JSON(http.StatusOK, gin.H{})
}

func (h *Handler) validSignature(signature string, rawBody []byte) bool {
	if signature == "" || h.appSecret == "" {
		return false
	}

	mac := hmac.New(sha256.New, []byte(h.appSecret))
	_, _ = mac.Write(rawBody)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(signature))
}

func (h *Handler) processTextMessage(c *gin.Context, inbound inboundTextMessage) error {
	msg := strings.TrimSpace(inbound.Body)
	if msg == "" && inbound.AudioID != "" {
		text, err := h.transcribeAudioMessage(c.Request.Context(), inbound)
		if err != nil {
			return err
		}
		msg = strings.TrimSpace(text)
	}
	if msg == "" || len(msg) > 4096 || !whatsappFromRegex.MatchString(inbound.From) {
		return nil
	}

	ctx := c.Request.Context()
	start := time.Now()
	from := "+" + inbound.From
	if !h.isAllowed(ctx, from) {
		h.logger.WarnContext(ctx, "whatsapp sender not allowed", "from", normalizePhoneForLog(inbound.From))
		return nil
	}

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
			return fmt.Errorf("send whatsapp greeting reply: %w", err)
		}
		return nil
	}

	actor := h.actorForPhone(ctx, from)
	if reply, ok, err := h.handlePendingCreate(ctx, session.ID, msg, actor); ok || err != nil {
		if err != nil {
			return err
		}
		_ = h.conv.SaveUserMessage(ctx, session.ID, msg, "create_deal_pending")
		_ = h.conv.SaveAssistantMessage(ctx, session.ID, reply)
		if err := h.sender.SendTextMessage(ctx, inbound.From, reply); err != nil {
			return fmt.Errorf("send whatsapp pending create reply: %w", err)
		}
		return nil
	}

	if reply, ok, err := h.handlePendingScheduledTask(ctx, session.ID, msg, actor); ok || err != nil {
		if err != nil {
			return err
		}
		_ = h.conv.SaveUserMessage(ctx, session.ID, msg, "create_scheduled_task_pending")
		_ = h.conv.SaveAssistantMessage(ctx, session.ID, reply)
		if err := h.sender.SendTextMessage(ctx, inbound.From, reply); err != nil {
			return fmt.Errorf("send whatsapp pending task reply: %w", err)
		}
		return nil
	}

	if reply, ok, err := h.handlePendingSelection(ctx, session.ID, msg, actor); ok || err != nil {
		if err != nil {
			return err
		}
		_ = h.conv.SaveUserMessage(ctx, session.ID, msg, "deal_selection")
		_ = h.conv.SaveAssistantMessage(ctx, session.ID, reply)
		if err := h.sender.SendTextMessage(ctx, inbound.From, reply); err != nil {
			return fmt.Errorf("send whatsapp selection reply: %w", err)
		}
		h.clearPendingSelection(session.ID)
		h.logger.InfoContext(ctx, "whatsapp selection completed",
			"from", normalizePhoneForLog(inbound.From),
			"wamid", inbound.ID,
			"latency_ms", time.Since(start).Milliseconds(),
		)
		return nil
	}

	intent, err := h.nlpSvc.ParseIntent(ctx, history, msg)
	if err != nil {
		h.logger.WarnContext(ctx, "nlp parse intent failed", "session_id", session.ID, "wamid", inbound.ID, "error", err)
		intent = domain.Intent{Name: domain.IntentUnknown, RawText: msg}
	}
	if isDeleteRequest(msg) {
		intent.Name = domain.IntentDeleteDeal
		intent.RawText = msg
	}

	if intent.Name == domain.IntentCreateDeal {
		reply := h.startCreateDealFlow(session.ID, intent)
		_ = h.conv.SaveUserMessage(ctx, session.ID, msg, string(intent.Name))
		_ = h.conv.SaveAssistantMessage(ctx, session.ID, reply)
		if err := h.sender.SendTextMessage(ctx, inbound.From, reply); err != nil {
			return fmt.Errorf("send whatsapp create flow reply: %w", err)
		}
		return nil
	}

	if intent.Name == domain.IntentCreateScheduledTask {
		h.applyActiveDeal(ctx, session.ID, &intent)
		reply := h.startScheduledTaskFlow(ctx, session.ID, intent, actor)
		_ = h.conv.SaveUserMessage(ctx, session.ID, msg, string(intent.Name))
		_ = h.conv.SaveAssistantMessage(ctx, session.ID, reply)
		if err := h.sender.SendTextMessage(ctx, inbound.From, reply); err != nil {
			return fmt.Errorf("send whatsapp create task flow reply: %w", err)
		}
		return nil
	}

	_ = h.conv.SaveUserMessage(ctx, session.ID, msg, string(intent.Name))
	h.clearPendingList(session.ID)
	reply := h.buildReply(ctx, session.ID, intent, actor)
	_ = h.conv.SaveAssistantMessage(ctx, session.ID, reply)

	if err := h.sender.SendTextMessage(ctx, inbound.From, reply); err != nil {
		return fmt.Errorf("send whatsapp reply: %w", err)
	}

	h.logger.InfoContext(ctx, "whatsapp message completed",
		"from", normalizePhoneForLog(inbound.From),
		"wamid", inbound.ID,
		"intent", string(intent.Name),
		"latency_ms", time.Since(start).Milliseconds(),
	)

	return nil
}

func (h *Handler) transcribeAudioMessage(ctx context.Context, inbound inboundTextMessage) (string, error) {
	if h.mediaDownloader == nil {
		return "", fmt.Errorf("whatsapp audio media downloader unavailable")
	}

	audio, mimeType, err := h.mediaDownloader.DownloadMedia(ctx, inbound.AudioID)
	if err != nil {
		return "", fmt.Errorf("download whatsapp audio: %w", err)
	}
	if inbound.AudioMimeType != "" {
		mimeType = inbound.AudioMimeType
	}

	text, err := h.nlpSvc.TranscribeAudio(ctx, audio, audioFilename(mimeType))
	if err != nil {
		return "", fmt.Errorf("transcribe whatsapp audio: %w", err)
	}

	h.logger.InfoContext(ctx, "whatsapp audio transcribed",
		"from", normalizePhoneForLog(inbound.From),
		"wamid", inbound.ID,
		"audio_bytes", len(audio),
	)

	return text, nil
}

func (h *Handler) buildReply(ctx context.Context, sessionID string, intent domain.Intent, actor intentRouter.Actor) string {
	if intent.Name == domain.IntentUnknown {
		return nlp.FallbackResponse()
	}

	h.applyActiveDeal(ctx, sessionID, &intent)
	result, routeErr := h.router.RouteForActor(ctx, intent, actor)
	if routeErr != nil {
		var multiErr *rdSvc.MultipleDealsError
		if errors.As(routeErr, &multiErr) {
			if active, ok := h.getActiveDeal(sessionID); ok {
				for _, d := range multiErr.Deals {
					if d.ID == active.ID {
						autoResult, autoErr := h.router.ResolveDealSelection(ctx, intent, d, actor)
						if autoErr == nil {
							if deal, ok := autoResult.(domain.Deal); ok {
								h.setActiveDeal(sessionID, deal)
							}
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

	if deal, ok := result.(domain.Deal); ok {
		h.setActiveDeal(sessionID, deal)
	}

	if deals, ok := result.([]domain.Deal); ok {
		if len(deals) == 1 {
			h.setActiveDeal(sessionID, deals[0])
		} else if len(deals) > 1 {
			h.setPendingList(sessionID, deals)
		}
	}

	formatted, fmtErr := h.nlpSvc.FormatResponse(ctx, intent, result)
	if fmtErr != nil {
		h.logger.WarnContext(ctx, "nlp format response failed", "session_id", sessionID, "error", fmtErr)
		return nlp.ErrorResponse("servico de formatacao")
	}

	return formatted
}

func (h *Handler) applyActiveDeal(_ context.Context, sessionID string, intent *domain.Intent) {
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

func (h *Handler) handlePendingCreate(ctx context.Context, sessionID, msg string, actor intentRouter.Actor) (string, bool, error) {
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
			result, err := h.router.RouteForActor(ctx, draft.Intent, actor)
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

	h.pendingMu.Lock()
	pending, exists := h.pendingDeals[sessionID]
	h.pendingMu.Unlock()

	if exists {
		if index < 0 || index >= len(pending.Deals) {
			return fmt.Sprintf("Escolha um numero entre 1 e %d.", len(pending.Deals)), true, nil
		}

		selected := pending.Deals[index]
		intent := pending.Intent
		if intent.Parameters == nil {
			intent.Parameters = map[string]string{}
		}
		intent.Parameters["deal_name"] = selected.Name
		intent.RawText = fmt.Sprintf("%s (opcao %d: %s)", intent.RawText, index+1, selected.Name)

		result, err := h.router.ResolveDealSelection(ctx, intent, selected, actor)
		if err != nil {
			return h.handleRouteError(err, sessionID, intent), true, nil
		}

		if deal, ok := result.(domain.Deal); ok {
			h.setActiveDeal(sessionID, deal)
		} else {
			h.setActiveDeal(sessionID, selected)
		}

		formatted, err := h.nlpSvc.FormatResponse(ctx, intent, result)
		if err != nil {
			h.logger.WarnContext(ctx, "nlp format selection response failed", "session_id", sessionID, "error", err)
			return nlp.ErrorResponse("servico de formatacao"), true, nil
		}
		return formatted, true, nil
	}

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
	deal, ok := h.activeDeals[sessionID]
	h.activeDealsMu.Unlock()
	return deal, ok
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
		return fmt.Sprintf("Encontrei mais de uma negociacao com esse nome. Qual delas voce quer?\n%s",
			strings.Join(names, "\n"))
	}

	var stageErr *rdSvc.StageNotFoundError
	if errors.As(err, &stageErr) {
		return fmt.Sprintf("Nao encontrei o estagio informado. Os estagios disponiveis sao: %s",
			strings.Join(stageErr.Available, ", "))
	}

	var notFoundErr *rdSvc.DealNotFoundError
	if errors.As(err, &notFoundErr) {
		return fmt.Sprintf("Nao encontrei nenhuma negociacao com o nome \"%s\". Verifique o nome e tente novamente.", notFoundErr.Name)
	}

	var missingName *rdSvc.MissingDealNameError
	if errors.As(err, &missingName) {
		return "Preciso saber o nome da negociacao. Pode informar?"
	}

	h.logger.Warn("route error", "intent", string(intent.Name), "error", err)
	return nlp.ErrorResponse("RD Station")
}

func (h *Handler) isAllowed(ctx context.Context, phone string) bool {
	if h.allowChecker == nil {
		return true
	}
	allowed, err := h.allowChecker.IsAllowed(ctx, phone)
	if err != nil {
		h.logger.WarnContext(ctx, "admin allowlist check failed", "from", normalizePhoneForLog(strings.TrimPrefix(phone, "+")), "error", err)
		return false
	}
	return allowed
}

func (h *Handler) actorForPhone(ctx context.Context, phone string) intentRouter.Actor {
	if h.accessProfiler == nil {
		return intentRouter.Actor{Role: "director"}
	}
	profile, err := h.accessProfiler.AccessProfile(ctx, phone)
	if err != nil {
		h.logger.WarnContext(ctx, "access profile lookup failed", "from", normalizePhoneForLog(strings.TrimPrefix(phone, "+")), "error", err)
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
	for _, marker := range []string{" estagio ", " etapa "} {
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
	switch strings.TrimSpace(normalizeIntentText(name)) {
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
	if subject := strings.TrimSpace(p["subject"]); subject != "" && !scheduledTaskSubjectMentioned(raw, subject) {
		p["subject"] = ""
	}
	if date := strings.TrimSpace(p["date"]); date != "" && !scheduledTaskDateMentioned(raw, date) {
		p["date"] = ""
	}
	if hour := strings.TrimSpace(p["hour"]); hour != "" && !scheduledTaskHourMentioned(raw, hour) {
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
	return firstRegexGroup(value, []string{
		`(?i)\bassunto\s+(?:deve\s+(?:ser|ter)|e|é|eh|ser|para)\s+(.+?)(?:,\s*|\s+e\s+(?:a\s+)?(?:hora|horario|horário|data)\b|$)`,
		`(?i)\bcom\s+assunto\s+(.+?)(?:,\s*|\s+e\s+(?:a\s+)?(?:hora|horario|horário|data)\b|$)`,
	})
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
	return firstRegexGroup(value, []string{
		`(?i)\bnegociacao\s+(?:deve\s+ser|e|é|eh|para)\s+(.+?)(?:,\s*|\s+e\s+(?:o\s+)?(?:assunto|horario|horário|hora|data)\b|$)`,
		`(?i)\bnegociação\s+(?:deve\s+ser|e|é|eh|para)\s+(.+?)(?:,\s*|\s+e\s+(?:o\s+)?(?:assunto|horario|horário|hora|data)\b|$)`,
	})
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
	lines := []string{"Tarefa criada com sucesso.", "", fmt.Sprintf("*Assunto:* %s", task.Subject)}
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

func formatRDTaskDate(date, hour string) string {
	date = strings.TrimSpace(date)
	hour = strings.TrimSpace(hour)
	if date == "" {
		return ""
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
		return date
	}
	if hour == "" {
		return day.Format("02/01/2006")
	}
	cleanHour := normalizeTaskHour(hour)
	if _, err := time.Parse("15:04", cleanHour); err == nil {
		return fmt.Sprintf("%s as %s", day.Format("02/01/2006"), cleanHour)
	}
	return day.Format("02/01/2006")
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

func parseSelection(msg string) (int, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(msg))
	if err != nil || value <= 0 {
		return 0, false
	}
	return value - 1, true
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

func normalizePhoneForLog(from string) string {
	if len(from) <= 4 {
		return from
	}
	return "+" + from[:4] + "***"
}

type inboundTextMessage struct {
	From          string
	ID            string
	Body          string
	AudioID       string
	AudioMimeType string
}

type metaWebhookPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		Changes []struct {
			Value struct {
				Messages []struct {
					From string `json:"from"`
					ID   string `json:"id"`
					Type string `json:"type"`
					Text struct {
						Body string `json:"body"`
					} `json:"text"`
					Audio struct {
						ID       string `json:"id"`
						MimeType string `json:"mime_type"`
					} `json:"audio"`
				} `json:"messages"`
				Statuses []messageStatus `json:"statuses"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

type messageStatus struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	RecipientID string `json:"recipient_id"`
	Errors      []struct {
		Code    int    `json:"code"`
		Title   string `json:"title"`
		Message string `json:"message"`
	} `json:"errors"`
}

func (p metaWebhookPayload) firstInboundMessage() (inboundTextMessage, bool) {
	for _, entry := range p.Entry {
		for _, change := range entry.Changes {
			for _, msg := range change.Value.Messages {
				switch msg.Type {
				case "text":
					return inboundTextMessage{From: msg.From, ID: msg.ID, Body: msg.Text.Body}, true
				case "audio":
					return inboundTextMessage{From: msg.From, ID: msg.ID, AudioID: msg.Audio.ID, AudioMimeType: msg.Audio.MimeType}, true
				}
			}
		}
	}
	return inboundTextMessage{}, false
}

func (p metaWebhookPayload) firstStatus() (messageStatus, bool) {
	for _, entry := range p.Entry {
		for _, change := range entry.Changes {
			if len(change.Value.Statuses) > 0 {
				return change.Value.Statuses[0], true
			}
		}
	}
	return messageStatus{}, false
}

func (p metaWebhookPayload) ignoreReason() string {
	if len(p.Entry) == 0 {
		return "no_entry"
	}
	for _, entry := range p.Entry {
		for _, change := range entry.Changes {
			if len(change.Value.Messages) == 0 {
				continue
			}
			return "no_text_message"
		}
	}
	return "no_messages"
}

func audioFilename(mimetype string) string {
	if strings.Contains(mimetype, "mpeg") || strings.Contains(mimetype, "mp3") {
		return "audio.mp3"
	}
	if strings.Contains(mimetype, "wav") {
		return "audio.wav"
	}
	return "audio.ogg"
}
