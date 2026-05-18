package conversation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"synova-rd-workflow/internal/domain"
	convSvc "synova-rd-workflow/internal/service/conversation"
)

// --- Mock Store ---

type mockStore struct {
	sessions map[string]domain.Session
	messages []domain.Message
	getErr   error
	saveErr  error
}

func newMockStore() *mockStore {
	return &mockStore{sessions: make(map[string]domain.Session)}
}

func (m *mockStore) GetOrCreateSession(_ context.Context, phone string) (domain.Session, error) {
	if m.getErr != nil {
		return domain.Session{}, m.getErr
	}
	if s, ok := m.sessions[phone]; ok {
		return s, nil
	}
	s := domain.Session{ID: "sess-" + phone, PhoneNumber: phone, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	m.sessions[phone] = s
	return s, nil
}

func (m *mockStore) GetRecentMessages(_ context.Context, sessionID string, limit int) ([]domain.Message, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
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

func (m *mockStore) SaveMessage(_ context.Context, msg domain.Message) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.messages = append(m.messages, msg)
	return nil
}

// --- Tests ---

func TestGetOrCreateSession_NewSession(t *testing.T) {
	store := newMockStore()
	svc := convSvc.New(store, 10)

	sess, err := svc.GetOrCreateSession(context.Background(), "+5511999999999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess.PhoneNumber != "+5511999999999" {
		t.Errorf("expected phone +5511999999999, got %s", sess.PhoneNumber)
	}
}

func TestGetOrCreateSession_ExistingSession(t *testing.T) {
	store := newMockStore()
	svc := convSvc.New(store, 10)

	sess1, _ := svc.GetOrCreateSession(context.Background(), "+5511111111111")
	sess2, _ := svc.GetOrCreateSession(context.Background(), "+5511111111111")

	if sess1.ID != sess2.ID {
		t.Errorf("expected same session ID, got %s and %s", sess1.ID, sess2.ID)
	}
}

func TestGetOrCreateSession_StoreError(t *testing.T) {
	store := newMockStore()
	store.getErr = errors.New("db error")
	svc := convSvc.New(store, 10)

	_, err := svc.GetOrCreateSession(context.Background(), "+5511999999999")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetRecentHistory_SlidingWindow(t *testing.T) {
	store := newMockStore()
	svc := convSvc.New(store, 3) // window = 3

	for i := 0; i < 5; i++ {
		store.messages = append(store.messages, domain.Message{
			SessionID: "sess-1",
			Role:      "user",
			Content:   "msg",
		})
	}

	msgs, err := svc.GetRecentHistory(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("expected 3 messages, got %d", len(msgs))
	}
}

func TestSaveUserMessage_PersistsIntent(t *testing.T) {
	store := newMockStore()
	svc := convSvc.New(store, 10)

	err := svc.SaveUserMessage(context.Background(), "sess-1", "texto", "get_contacts")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(store.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(store.messages))
	}
	if store.messages[0].Intent != "get_contacts" {
		t.Errorf("expected intent get_contacts, got %s", store.messages[0].Intent)
	}
	if store.messages[0].Role != "user" {
		t.Errorf("expected role user, got %s", store.messages[0].Role)
	}
}

func TestSaveAssistantMessage(t *testing.T) {
	store := newMockStore()
	svc := convSvc.New(store, 10)

	_ = svc.SaveAssistantMessage(context.Background(), "sess-1", "resposta")

	if len(store.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(store.messages))
	}
	if store.messages[0].Role != "assistant" {
		t.Errorf("expected role assistant, got %s", store.messages[0].Role)
	}
}
