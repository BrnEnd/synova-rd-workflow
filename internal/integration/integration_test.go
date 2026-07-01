package integration_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	rdClient "synova-rd-workflow/internal/client/rdstation"
	"synova-rd-workflow/internal/domain"
	waHandler "synova-rd-workflow/internal/handler/whatsapp"
	convSvc "synova-rd-workflow/internal/service/conversation"
	intentRouter "synova-rd-workflow/internal/service/intent_router"
	"synova-rd-workflow/internal/service/nlp"
	rdSvc "synova-rd-workflow/internal/service/rdstation"
)

const (
	testVerifyToken = "verify-token"
	testAppSecret   = "app-secret"
)

type memStore struct {
	sessions map[string]domain.Session
	messages []domain.Message
}

func newMemStore() *memStore {
	return &memStore{sessions: make(map[string]domain.Session)}
}

func (m *memStore) GetOrCreateSession(_ context.Context, phone string) (domain.Session, error) {
	if s, ok := m.sessions[phone]; ok {
		return s, nil
	}
	s := domain.Session{ID: phone, PhoneNumber: phone, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	m.sessions[phone] = s
	return s, nil
}

func (m *memStore) GetRecentMessages(_ context.Context, sessionID string, limit int) ([]domain.Message, error) {
	var result []domain.Message
	for _, msg := range m.messages {
		if msg.SessionID == sessionID {
			result = append(result, msg)
		}
	}
	if len(result) > limit {
		result = result[len(result)-limit:]
	}
	return result, nil
}

func (m *memStore) SaveMessage(_ context.Context, msg domain.Message) error {
	m.messages = append(m.messages, msg)
	return nil
}

type mockSender struct {
	to          string
	text        string
	err         error
	media       []byte
	mediaMime   string
	downloadErr error
}

func (m *mockSender) SendTextMessage(_ context.Context, to string, text string) error {
	m.to = to
	m.text = text
	return m.err
}

func (m *mockSender) DownloadMedia(_ context.Context, _ string) ([]byte, string, error) {
	return m.media, m.mediaMime, m.downloadErr
}

type mockNLPService struct {
	intent          domain.Intent
	intents         []domain.Intent
	reply           string
	err             error
	transcribedText string
	transcribeErr   error
	parseCalls      int
}

func (m *mockNLPService) ParseIntent(_ context.Context, _ []domain.Message, input string) (domain.Intent, error) {
	if len(m.intents) > 0 {
		idx := m.parseCalls
		if idx >= len(m.intents) {
			idx = len(m.intents) - 1
		}
		m.parseCalls++
		intent := m.intents[idx]
		intent.RawText = input
		m.intent = intent
		return intent, m.err
	}
	m.parseCalls++
	m.intent.RawText = input
	return m.intent, m.err
}

func (m *mockNLPService) FormatResponse(_ context.Context, _ domain.Intent, _ interface{}) (string, error) {
	if m.reply != "" {
		return m.reply, nil
	}
	return "resultado formatado", nil
}

func (m *mockNLPService) TranscribeAudio(_ context.Context, _ []byte, _ string) (string, error) {
	return m.transcribedText, m.transcribeErr
}

func buildTestRouter(t *testing.T, nlpMock *mockNLPService, sender *mockSender, rdServer *httptest.Server) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	store := newMemStore()
	conv := convSvc.New(store, 10)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	var router *intentRouter.Router
	if rdServer != nil {
		client := rdClient.NewWithBaseURL("test-key", rdServer.URL)
		router = intentRouter.New(rdSvc.New(client))
	} else {
		router = intentRouter.New(rdSvc.New(rdClient.New("unused")))
	}

	var nlpService nlp.ServiceInterface = nlpMock
	if sender == nil {
		sender = &mockSender{}
	}

	handler := waHandler.New(conv, nlpService, router, sender, testVerifyToken, testAppSecret, logger)

	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	handler.RegisterRoutes(r)

	return r
}

func postMetaPayload(t *testing.T, r *gin.Engine, body []byte, sign bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if sign {
		req.Header.Set("X-Hub-Signature-256", signBody(body))
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func textPayload(t *testing.T, from, messageType, body string) []byte {
	t.Helper()
	payload := map[string]interface{}{
		"object": "whatsapp_business_account",
		"entry": []map[string]interface{}{{
			"changes": []map[string]interface{}{{
				"value": map[string]interface{}{
					"messages": []map[string]interface{}{{
						"from": from,
						"id":   "wamid.test",
						"type": messageType,
						"text": map[string]string{"body": body},
					}},
				},
			}},
		}},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func audioPayload(t *testing.T, from string) []byte {
	t.Helper()
	payload := map[string]interface{}{
		"object": "whatsapp_business_account",
		"entry": []map[string]interface{}{{
			"changes": []map[string]interface{}{{
				"value": map[string]interface{}{
					"messages": []map[string]interface{}{{
						"from": from,
						"id":   "wamid.test",
						"type": "audio",
						"audio": map[string]string{
							"id":        "media.test",
							"mime_type": "audio/ogg",
						},
					}},
				},
			}},
		}},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func signBody(body []byte) string {
	mac := hmac.New(sha256.New, []byte(testAppSecret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestHealth(t *testing.T) {
	r := buildTestRouter(t, &mockNLPService{}, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestWebhookVerificationOK(t *testing.T) {
	r := buildTestRouter(t, &mockNLPService{}, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/webhook?hub.mode=subscribe&hub.verify_token=verify-token&hub.challenge=abc123", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "abc123" {
		t.Fatalf("expected challenge body, got %q", w.Body.String())
	}
}

func TestWebhookVerificationForbidden(t *testing.T) {
	r := buildTestRouter(t, &mockNLPService{}, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/webhook?hub.mode=subscribe&hub.verify_token=wrong&hub.challenge=abc123", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestWebhookInvalidSignature(t *testing.T) {
	r := buildTestRouter(t, &mockNLPService{}, nil, nil)
	body := textPayload(t, "5511999999999", "text", "ola")
	w := postMetaPayload(t, r, body, false)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestWebhookUnknownIntentSendsFallback(t *testing.T) {
	sender := &mockSender{}
	r := buildTestRouter(t, &mockNLPService{intent: domain.Intent{Name: domain.IntentUnknown}}, sender, nil)
	body := textPayload(t, "5511999999999", "text", "nao sei o que quero")
	w := postMetaPayload(t, r, body, true)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if sender.to != "5511999999999" {
		t.Errorf("expected sender target, got %q", sender.to)
	}
	if sender.text == "" {
		t.Error("expected non-empty fallback reply")
	}
}

func TestWebhookGreetingSendsSilIntro(t *testing.T) {
	sender := &mockSender{}
	r := buildTestRouter(t, &mockNLPService{intent: domain.Intent{Name: domain.IntentUnknown}}, sender, nil)
	body := textPayload(t, "5511999999999", "text", "Oi")
	w := postMetaPayload(t, r, body, true)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if sender.to != "5511999999999" {
		t.Errorf("expected sender target, got %q", sender.to)
	}
	if !strings.Contains(sender.text, "Eu sou a Sil") || !strings.Contains(sender.text, "Excellence and Quality") {
		t.Errorf("expected Sil intro message, got %q", sender.text)
	}
}

func TestWebhookGetContactsFullFlow(t *testing.T) {
	rdServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/contacts" {
			resp := rdClient.ContactsListResponse{
				Contacts: []rdClient.ContactResponse{
					{ID: "c1", Name: "Joao Silva", Email: "joao@test.com"},
				},
				Total: 1,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
	defer rdServer.Close()

	sender := &mockSender{}
	mockNLP := &mockNLPService{
		intent: domain.Intent{
			Name:       domain.IntentGetContacts,
			Parameters: map[string]string{"name": "Silva"},
		},
		reply: "Encontrei 1 contato: Joao Silva.",
	}

	r := buildTestRouter(t, mockNLP, sender, rdServer)
	body := textPayload(t, "5511999999999", "text", "Quais contatos tem sobrenome Silva?")
	w := postMetaPayload(t, r, body, true)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if sender.text != "Encontrei 1 contato: Joao Silva." {
		t.Errorf("expected formatted reply, got %q", sender.text)
	}
}

func TestWebhookAudioIsTranscribedAndProcessed(t *testing.T) {
	sender := &mockSender{media: []byte("audio-bytes"), mediaMime: "audio/ogg"}
	nlpMock := &mockNLPService{
		intent:          domain.Intent{Name: domain.IntentUnknown},
		transcribedText: "quais contatos tem sobrenome Silva?",
	}
	r := buildTestRouter(t, nlpMock, sender, nil)
	body := audioPayload(t, "5511999999999")
	w := postMetaPayload(t, r, body, true)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if nlpMock.intent.RawText != "quais contatos tem sobrenome Silva?" {
		t.Errorf("expected transcribed text to be parsed, got %q", nlpMock.intent.RawText)
	}
	if sender.text == "" {
		t.Error("expected reply for audio payload")
	}
}

func TestWebhookNLPErrorReturnsUnknown(t *testing.T) {
	sender := &mockSender{}
	r := buildTestRouter(t, &mockNLPService{
		intent: domain.Intent{Name: domain.IntentUnknown},
		err:    errors.New("openai timeout"),
	}, sender, nil)
	body := textPayload(t, "5511999999999", "text", "alguma coisa")
	w := postMetaPayload(t, r, body, true)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 even on NLP error, got %d", w.Code)
	}
	if sender.text == "" {
		t.Error("expected fallback reply")
	}
}

func TestWebhookMetaUsesActiveDealForFollowUp(t *testing.T) {
	rdServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/deals":
			resp := rdClient.DealsListResponse{
				Deals: []rdClient.DealResponse{
					{
						ID:        "d1",
						Name:      "Alpha",
						DealStage: rdClient.DealStageResponse{Name: "Amostra"},
						DealOwner: rdClient.DealUserResponse{Name: "Glauco"},
					},
					{
						ID:        "d2",
						Name:      "Beta",
						DealStage: rdClient.DealStageResponse{Name: "Proposta"},
						DealOwner: rdClient.DealUserResponse{Name: "Glauco"},
					},
				},
				Total: 2,
			}
			if r.URL.Query().Get("name") == "Alpha" {
				resp.Deals = resp.Deals[:1]
				resp.Total = 1
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		case "/deals/d1":
			resp := rdClient.DealResponse{
				ID:        "d1",
				Name:      "Alpha",
				DealStage: rdClient.DealStageResponse{Name: "Amostra"},
				DealOwner: rdClient.DealUserResponse{Name: "Glauco"},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		default:
			t.Fatalf("unexpected RD path: %s", r.URL.Path)
		}
	}))
	defer rdServer.Close()

	sender := &mockSender{}
	mockNLP := &mockNLPService{
		intents: []domain.Intent{
			{Name: domain.IntentGetDeals, Parameters: map[string]string{"owner_name": "Glauco"}},
			{Name: domain.IntentGetDeal, Parameters: map[string]string{}},
		},
		reply: "resultado formatado",
	}
	r := buildTestRouter(t, mockNLP, sender, rdServer)

	w := postMetaPayload(t, r, textPayload(t, "5511999999999", "text", "cards do Glauco"), true)
	if w.Code != http.StatusOK {
		t.Fatalf("expected first webhook 200, got %d", w.Code)
	}

	w = postMetaPayload(t, r, textPayload(t, "5511999999999", "text", "1"), true)
	if w.Code != http.StatusOK {
		t.Fatalf("expected selection webhook 200, got %d", w.Code)
	}
	if !strings.Contains(sender.text, "Selecionei *Alpha*") {
		t.Fatalf("expected active deal selection reply, got %q", sender.text)
	}

	w = postMetaPayload(t, r, textPayload(t, "5511999999999", "text", "me dê mais informações da negociação"), true)
	if w.Code != http.StatusOK {
		t.Fatalf("expected follow-up webhook 200, got %d", w.Code)
	}
	if sender.text != "resultado formatado" {
		t.Fatalf("expected formatted active deal details, got %q", sender.text)
	}
}

func TestWebhookMetaGuidedCreateDealFlow(t *testing.T) {
	sender := &mockSender{}
	mockNLP := &mockNLPService{
		intent: domain.Intent{Name: domain.IntentCreateDeal, Parameters: map[string]string{}},
	}
	r := buildTestRouter(t, mockNLP, sender, nil)

	w := postMetaPayload(t, r, textPayload(t, "5511999999999", "text", "cria uma negociacao"), true)
	if w.Code != http.StatusOK {
		t.Fatalf("expected first webhook 200, got %d", w.Code)
	}
	if sender.text != "Qual e o cliente ou empresa dessa negociacao?" {
		t.Fatalf("expected company question, got %q", sender.text)
	}

	w = postMetaPayload(t, r, textPayload(t, "5511999999999", "text", "Casa do Aroma"), true)
	if w.Code != http.StatusOK {
		t.Fatalf("expected pending webhook 200, got %d", w.Code)
	}
	if sender.text != "Qual produto esta sendo negociado?" {
		t.Fatalf("expected product question, got %q", sender.text)
	}
}

func TestWebhookMetaGuidedScheduledTaskFlow(t *testing.T) {
	sender := &mockSender{}
	mockNLP := &mockNLPService{
		intent: domain.Intent{Name: domain.IntentCreateScheduledTask, Parameters: map[string]string{}},
	}
	r := buildTestRouter(t, mockNLP, sender, nil)

	w := postMetaPayload(t, r, textPayload(t, "5511999999999", "text", "cria uma tarefa"), true)
	if w.Code != http.StatusOK {
		t.Fatalf("expected first webhook 200, got %d", w.Code)
	}
	if sender.text != "Em qual negociacao devo criar essa tarefa?" {
		t.Fatalf("expected deal question, got %q", sender.text)
	}

	w = postMetaPayload(t, r, textPayload(t, "5511999999999", "text", "Casa do Aroma - F200"), true)
	if w.Code != http.StatusOK {
		t.Fatalf("expected pending webhook 200, got %d", w.Code)
	}
	if sender.text != "Qual e o assunto da tarefa?" {
		t.Fatalf("expected subject question, got %q", sender.text)
	}
}
