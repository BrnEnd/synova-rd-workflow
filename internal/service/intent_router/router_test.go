package intent_router_test

import (
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
