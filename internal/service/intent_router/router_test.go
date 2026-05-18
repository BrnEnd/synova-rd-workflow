package intent_router_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	rdClient "synova-rd-workflow/internal/client/rdstation"
	"synova-rd-workflow/internal/domain"
	intentRouter "synova-rd-workflow/internal/service/intent_router"
	rdSvc "synova-rd-workflow/internal/service/rdstation"
)

func TestNewRouter_NotNil(t *testing.T) {
	svc := rdSvc.New(rdClient.New("key"))
	r := intentRouter.New(svc)
	if r == nil {
		t.Error("expected non-nil router")
	}
}

func TestIntentNames_AllDefined(t *testing.T) {
	intents := []domain.IntentName{
		domain.IntentGetContacts,
		domain.IntentGetDeals,
		domain.IntentCreateContact,
		domain.IntentCreateDeal,
		domain.IntentUpdateDeal,
		domain.IntentMoveDealStage,
		domain.IntentUnknown,
	}
	for _, i := range intents {
		if i == "" {
			t.Error("found empty intent name")
		}
	}
}

func TestRDSvcParams_Compile(_ *testing.T) {
	_ = rdSvc.GetContactsParams{Name: "test", Email: "e@e.com", Phone: "+551199"}
	_ = rdSvc.CreateContactParams{Name: "Test", Email: "e@e.com"}
	_ = rdSvc.GetDealsParams{Name: "deal", Stage: "Proposta", Status: "open"}
	_ = rdSvc.CreateDealParams{Name: "deal", ContactName: "João", Stage: "Proposta"}
	_ = rdSvc.UpdateDealParams{DealName: "deal", Field: "name", Value: "novo"}
}

func TestIntent_UnknownName(t *testing.T) {
	intent := domain.Intent{Name: domain.IntentUnknown, RawText: "blah"}
	if intent.Name != "unknown" {
		t.Errorf("expected 'unknown', got '%s'", intent.Name)
	}
}

func TestSellerOnlySeesOwnDeals(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/deals" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		resp := rdClient.DealsListResponse{
			Deals: []rdClient.DealResponse{
				{ID: "d1", Name: "Negocio proprio", DealOwner: rdClient.DealUserResponse{ID: "rd-seller"}},
				{ID: "d2", Name: "Negocio de outro", DealOwner: rdClient.DealUserResponse{ID: "rd-other"}},
			},
			Total: 2,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	router := intentRouter.New(rdSvc.New(rdClient.NewWithBaseURL("token", srv.URL)))
	result, err := router.RouteForActor(context.Background(), domain.Intent{
		Name:       domain.IntentGetDeals,
		Parameters: map[string]string{},
	}, intentRouter.Actor{Role: "seller", RDStationID: "rd-seller"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	deals, ok := result.([]domain.Deal)
	if !ok {
		t.Fatalf("expected []domain.Deal, got %T", result)
	}
	if len(deals) != 1 || deals[0].ID != "d1" {
		t.Fatalf("expected only seller deal, got %#v", deals)
	}
}

func TestSupervisorSeesTeamDeals(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := rdClient.DealsListResponse{
			Deals: []rdClient.DealResponse{
				{ID: "d1", Name: "Supervisor", DealOwner: rdClient.DealUserResponse{ID: "rd-renato"}},
				{ID: "d2", Name: "Vendedor PJ", DealOwner: rdClient.DealUserResponse{ID: "rd-pj"}},
				{ID: "d3", Name: "Fora da equipe", DealOwner: rdClient.DealUserResponse{ID: "rd-other"}},
			},
			Total: 3,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	router := intentRouter.New(rdSvc.New(rdClient.NewWithBaseURL("token", srv.URL)))
	result, err := router.RouteForActor(context.Background(), domain.Intent{
		Name:       domain.IntentGetDeals,
		Parameters: map[string]string{},
	}, intentRouter.Actor{Role: "supervisor", RDStationID: "rd-renato", TeamRDUserIDs: []string{"rd-pj"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	deals := result.([]domain.Deal)
	if len(deals) != 2 {
		t.Fatalf("expected supervisor and team deals, got %#v", deals)
	}
}
