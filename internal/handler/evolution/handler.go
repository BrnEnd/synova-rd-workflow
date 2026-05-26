package evolution

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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

type Sender interface {
	SendTextMessage(ctx context.Context, to string, text string) error
}

type AllowChecker interface {
	IsAllowed(ctx context.Context, phone string) (bool, error)
}

type AccessProfiler interface {
	AccessProfile(ctx context.Context, phone string) (domain.AccessProfile, error)
}

type Handler struct {
	conv           *convSvc.Service
	nlpSvc         nlp.ServiceInterface
	router         *intentRouter.Router
	sender         Sender
	logger         *slog.Logger
	allowedNumbers map[string]struct{}
	allowChecker   AllowChecker
	accessProfiler AccessProfiler
	pendingMu      sync.Mutex
	pendingDeals   map[string]pendingDealSelection
	pendingListsMu sync.Mutex
	pendingLists   map[string][]domain.Deal
	activeDealsMu  sync.Mutex
	activeDeals    map[string]domain.Deal
}

func New(conv *convSvc.Service, nlpSvc nlp.ServiceInterface, router *intentRouter.Router, sender Sender, logger *slog.Logger, allowedNumbers []string) *Handler {
	return &Handler{
		conv:           conv,
		nlpSvc:         nlpSvc,
		router:         router,
		sender:         sender,
		logger:         logger,
		allowedNumbers: buildAllowedNumbers(allowedNumbers),
		pendingDeals:   make(map[string]pendingDealSelection),
		pendingLists:   make(map[string][]domain.Deal),
		activeDeals:    make(map[string]domain.Deal),
	}
}

func (h *Handler) SetAllowChecker(checker AllowChecker) {
	h.allowChecker = checker
	if profiler, ok := checker.(AccessProfiler); ok {
		h.accessProfiler = profiler
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

	inbound, ok := payload.inboundText()
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

	if reply, ok, err := h.handlePendingSelection(ctx, session.ID, msg); ok || err != nil {
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

	result, routeErr := h.router.RouteForActor(ctx, intent, actor)
	if routeErr != nil {
		// If disambiguation is needed, try auto-resolving using the active deal.
		var multiErr *rdSvc.MultipleDealsError
		if errors.As(routeErr, &multiErr) {
			if active, ok := h.getActiveDeal(sessionID); ok {
				for _, d := range multiErr.Deals {
					if d.ID == active.ID {
						autoResult, autoErr := h.router.ResolveDealSelection(ctx, intent, d)
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

func (h *Handler) handlePendingSelection(ctx context.Context, sessionID, msg string) (string, bool, error) {
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

		result, err := h.router.ResolveDealSelection(ctx, intent, selected)
		if err != nil {
			return h.handleRouteError(err, sessionID, intent), true, nil
		}

		h.setActiveDeal(sessionID, selected)

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
		return fmt.Sprintf("Nao encontrei nenhum contato com o nome \"%s\". Verifique o nome e tente novamente.", contactNotFound.Name)
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
		} `json:"message"`
		MessageType string `json:"messageType"`
	} `json:"data"`
}

func (p webhookPayload) inboundText() (inboundTextMessage, bool) {
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

	if body == "" || from == "" {
		return inboundTextMessage{}, false
	}

	return inboundTextMessage{From: from, ID: p.Data.Key.ID, Body: body}, true
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
