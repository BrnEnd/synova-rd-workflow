package rdstation_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	rdClient "synova-rd-workflow/internal/client/rdstation"
	rdSvc "synova-rd-workflow/internal/service/rdstation"
)

func TestGetContacts_ReturnsMappedContacts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/contacts" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := rdClient.ContactsListResponse{
			Contacts: []rdClient.ContactResponse{
				{ID: "c1", Name: "João Silva", Email: "joao@test.com"},
			},
			Total: 1,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := rdClient.NewWithBaseURL("test-key", srv.URL)
	svc := rdSvc.New(client)

	contacts, err := svc.GetContacts(context.Background(), rdSvc.GetContactsParams{Name: "Silva"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(contacts))
	}
	if contacts[0].Name != "João Silva" {
		t.Errorf("expected João Silva, got %s", contacts[0].Name)
	}
}

func TestGetContacts_4xxError_NotRetried(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := rdClient.NewWithBaseURL("bad-key", srv.URL)
	svc := rdSvc.New(client)

	_, err := svc.GetContacts(context.Background(), rdSvc.GetContactsParams{Name: "test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if callCount != 1 {
		t.Errorf("expected 1 call (no retry), got %d", callCount)
	}
}

func TestGetDeals_FiltersByOwnerName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/deals" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := rdClient.DealsListResponse{
			Deals: []rdClient.DealResponse{
				{ID: "d1", Name: "SILICA", DealOwner: rdClient.DealUserResponse{Name: "Glauco de Oliveira"}},
				{ID: "d2", Name: "PORTIFOLIO", DealOwner: rdClient.DealUserResponse{Name: "Renato Silva"}},
			},
			Total: 2,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := rdClient.NewWithBaseURL("key", srv.URL)
	svc := rdSvc.New(client)

	deals, err := svc.GetDeals(context.Background(), rdSvc.GetDealsParams{OwnerName: "Glauco"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deals) != 1 {
		t.Fatalf("expected 1 deal, got %d", len(deals))
	}
	if deals[0].Owner.Name != "Glauco de Oliveira" {
		t.Errorf("expected Glauco de Oliveira, got %s", deals[0].Owner.Name)
	}
}

func TestMoveDealStage_MultipleDeals_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/deals":
			resp := rdClient.DealsListResponse{
				Deals: []rdClient.DealResponse{
					{ID: "d1", Name: "Projeto Alpha"},
					{ID: "d2", Name: "Projeto Alpha"},
				},
				Total: 2,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		case "/deal_stages":
			resp := rdClient.DealStageListResponse{
				DealStages: []struct {
					ID             string `json:"_id"`
					Name           string `json:"name"`
					DealPipelineID string `json:"deal_pipeline_id"`
				}{{ID: "s1", Name: "Fechado Ganho"}},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
	defer srv.Close()

	client := rdClient.NewWithBaseURL("key", srv.URL)
	svc := rdSvc.New(client)

	_, err := svc.MoveDealStage(context.Background(), "Projeto Alpha", "Fechado Ganho")
	if err == nil {
		t.Fatal("expected MultipleDealsError, got nil")
	}

	var multiErr *rdSvc.MultipleDealsError
	if !isMultipleDealsError(err, &multiErr) {
		t.Errorf("expected MultipleDealsError, got %T: %v", err, err)
	}
}

func isMultipleDealsError(err error, target **rdSvc.MultipleDealsError) bool {
	if mde, ok := err.(*rdSvc.MultipleDealsError); ok {
		*target = mde
		return true
	}
	return false
}

func TestMoveDealStage_StageNotFound_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/deals":
			resp := rdClient.DealsListResponse{
				Deals: []rdClient.DealResponse{
					{ID: "d1", Name: "Projeto Alpha"},
				},
				Total: 1,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		case "/deal_stages":
			resp := rdClient.DealStageListResponse{
				DealStages: []struct {
					ID             string `json:"_id"`
					Name           string `json:"name"`
					DealPipelineID string `json:"deal_pipeline_id"`
				}{{ID: "s1", Name: "Proposta"}},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
	defer srv.Close()

	client := rdClient.NewWithBaseURL("key", srv.URL)
	svc := rdSvc.New(client)

	_, err := svc.MoveDealStage(context.Background(), "Projeto Alpha", "Estagio Inexistente")
	if err == nil {
		t.Fatal("expected StageNotFoundError, got nil")
	}

	if _, ok := err.(*rdSvc.StageNotFoundError); !ok {
		t.Errorf("expected StageNotFoundError, got %T: %v", err, err)
	}
}
