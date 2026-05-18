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
	"strings"
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

// Handler holds all dependencies for the WhatsApp webhook.
type Handler struct {
	conv        *convSvc.Service
	nlpSvc      nlp.ServiceInterface
	router      *intentRouter.Router
	sender      Sender
	verifyToken string
	appSecret   string
	logger      *slog.Logger
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
	return &Handler{
		conv:        conv,
		nlpSvc:      nlpSvc,
		router:      router,
		sender:      sender,
		verifyToken: verifyToken,
		appSecret:   appSecret,
		logger:      logger,
	}
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

	inbound, ok := payload.firstTextMessage()
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
	if msg == "" || len(msg) > 4096 || !whatsappFromRegex.MatchString(inbound.From) {
		return nil
	}

	ctx := c.Request.Context()
	start := time.Now()
	from := "+" + inbound.From

	session, err := h.conv.GetOrCreateSession(ctx, from)
	if err != nil {
		return fmt.Errorf("get/create session: %w", err)
	}

	history, err := h.conv.GetRecentHistory(ctx, session.ID)
	if err != nil {
		return fmt.Errorf("get history: %w", err)
	}

	intent, err := h.nlpSvc.ParseIntent(ctx, history, msg)
	if err != nil {
		h.logger.WarnContext(ctx, "nlp parse intent failed", "session_id", session.ID, "wamid", inbound.ID, "error", err)
		intent = domain.Intent{Name: domain.IntentUnknown, RawText: msg}
	}

	_ = h.conv.SaveUserMessage(ctx, session.ID, msg, string(intent.Name))
	reply := h.buildReply(ctx, session.ID, intent)
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

func (h *Handler) buildReply(ctx context.Context, sessionID string, intent domain.Intent) string {
	if intent.Name == domain.IntentUnknown {
		return nlp.FallbackResponse()
	}

	result, routeErr := h.router.Route(ctx, intent)
	if routeErr != nil {
		return h.handleRouteError(routeErr, intent)
	}

	formatted, fmtErr := h.nlpSvc.FormatResponse(ctx, intent, result)
	if fmtErr != nil {
		h.logger.WarnContext(ctx, "nlp format response failed", "session_id", sessionID, "error", fmtErr)
		return nlp.ErrorResponse("servico de formatacao")
	}

	return formatted
}

func (h *Handler) handleRouteError(err error, intent domain.Intent) string {
	var multiErr *rdSvc.MultipleDealsError
	if errors.As(err, &multiErr) {
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

func normalizePhoneForLog(from string) string {
	if len(from) <= 4 {
		return from
	}
	return "+" + from[:4] + "***"
}

type inboundTextMessage struct {
	From string
	ID   string
	Body string
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

func (p metaWebhookPayload) firstTextMessage() (inboundTextMessage, bool) {
	for _, entry := range p.Entry {
		for _, change := range entry.Changes {
			for _, msg := range change.Value.Messages {
				if msg.Type != "text" {
					continue
				}
				return inboundTextMessage{From: msg.From, ID: msg.ID, Body: msg.Text.Body}, true
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
