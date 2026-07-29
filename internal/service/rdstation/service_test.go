package rdstation_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func TestGetDeals_FiltersByUpdatedRangeInclusive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/deals" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := rdClient.DealsListResponse{
			Deals: []rdClient.DealResponse{
				{ID: "before", Name: "Crossfix antes", UpdatedAt: "2026-06-12T23:59:59Z"},
				{ID: "start", Name: "Crossfix inicio", UpdatedAt: "2026-06-13T00:00:00Z"},
				{ID: "middle", Name: "Crossfix meio", UpdatedAt: "2026-06-20T15:30:00Z"},
				{ID: "end", Name: "Crossfix fim", UpdatedAt: "2026-06-23T23:59:59Z"},
				{ID: "after", Name: "Crossfix depois", UpdatedAt: "2026-06-24T00:00:00Z"},
			},
			Total: 5,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := rdClient.NewWithBaseURL("key", srv.URL)
	svc := rdSvc.New(client)

	deals, err := svc.GetDeals(context.Background(), rdSvc.GetDealsParams{
		Name:          "Crossfix",
		UpdatedAfter:  "2026-06-13",
		UpdatedBefore: "2026-06-23",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotIDs := make([]string, 0, len(deals))
	for _, deal := range deals {
		gotIDs = append(gotIDs, deal.ID)
	}
	want := []string{"end", "middle", "start"}
	if len(gotIDs) != len(want) {
		t.Fatalf("expected ids %v, got %v", want, gotIDs)
	}
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Fatalf("expected ids %v, got %v", want, gotIDs)
		}
	}
}

func TestGetDeals_FiltersByUpdatedRangeAcrossPages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/deals" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("next_page") == "cursor-2" {
			_ = json.NewEncoder(w).Encode(rdClient.DealsListResponse{
				Deals: []rdClient.DealResponse{
					{ID: "inside", Name: "Crossfix pagina 2", UpdatedAt: "2026-06-20T10:00:00Z"},
				},
				Total: 201,
			})
			return
		}

		deals := make([]rdClient.DealResponse, 200)
		for i := range deals {
			deals[i] = rdClient.DealResponse{
				ID:        "before-" + strconv.Itoa(i),
				Name:      "Crossfix fora",
				UpdatedAt: "2026-06-01T10:00:00Z",
			}
		}
		_ = json.NewEncoder(w).Encode(rdClient.DealsListResponse{
			Deals:    deals,
			HasMore:  true,
			NextPage: "cursor-2",
			Total:    201,
		})
	}))
	defer srv.Close()

	client := rdClient.NewWithBaseURL("key", srv.URL)
	svc := rdSvc.New(client)

	deals, err := svc.GetDeals(context.Background(), rdSvc.GetDealsParams{
		Name:          "Crossfix",
		UpdatedAfter:  "2026-06-13",
		UpdatedBefore: "2026-06-23",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deals) != 1 || deals[0].ID != "inside" {
		t.Fatalf("expected paginated deal inside date range, got %#v", deals)
	}
}

func TestGetDeals_OrdersByMostRecentlyUpdated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := rdClient.DealsListResponse{
			Deals: []rdClient.DealResponse{
				{ID: "old", Name: "Antiga", UpdatedAt: "2026-06-10T10:00:00Z"},
				{ID: "empty", Name: "Sem data"},
				{ID: "new", Name: "Recente", UpdatedAt: "2026-06-23T10:00:00Z"},
				{ID: "middle", Name: "Intermediaria", UpdatedAt: "2026-06-20T10:00:00Z"},
			},
			Total: 4,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := rdClient.NewWithBaseURL("key", srv.URL)
	svc := rdSvc.New(client)

	deals, err := svc.GetDeals(context.Background(), rdSvc.GetDealsParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gotIDs := []string{deals[0].ID, deals[1].ID, deals[2].ID, deals[3].ID}
	want := []string{"new", "middle", "old", "empty"}
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Fatalf("expected ids %v, got %v", want, gotIDs)
		}
	}
}

func TestGetDeals_WithoutUpdatedRangeKeepsExistingBehavior(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := rdClient.DealsListResponse{
			Deals: []rdClient.DealResponse{
				{ID: "d1", Name: "Sem data"},
				{ID: "d2", Name: "Com data", UpdatedAt: "2026-06-20T15:30:00Z"},
			},
			Total: 2,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := rdClient.NewWithBaseURL("key", srv.URL)
	svc := rdSvc.New(client)

	deals, err := svc.GetDeals(context.Background(), rdSvc.GetDealsParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deals) != 2 {
		t.Fatalf("expected existing no-date behavior to return 2 deals, got %#v", deals)
	}
}

func TestGetDeals_FiltersByExactCustomerName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/deals" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("name"); got != "" {
			t.Fatalf("expected no deal name query when filtering by customer, got %q", got)
		}
		resp := rdClient.DealsListResponse{
			Deals: []rdClient.DealResponse{
				{ID: "right", Name: "Empresa ABC - Credito", Contacts: []rdClient.DealContactResponse{{ID: "c1", Name: "Empresa ABC"}}},
				{ID: "wrong", Name: "Empresa ABCD - Seguro", Contacts: []rdClient.DealContactResponse{{ID: "c2", Name: "Empresa ABCD"}}},
			},
			Total: 2,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := rdClient.NewWithBaseURL("key", srv.URL)
	svc := rdSvc.New(client)

	deals, err := svc.GetDeals(context.Background(), rdSvc.GetDealsParams{CustomerName: "Empresa ABC"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deals) != 1 || deals[0].ID != "right" {
		t.Fatalf("expected only exact customer deal, got %#v", deals)
	}
}

func TestCreateDeal_AmbiguousStageAcrossPipelinesReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/deals":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(rdClient.DealsListResponse{Deals: nil, Total: 0})
		case "/deal_stages":
			resp := rdClient.DealStageListResponse{
				DealStages: []struct {
					ID             string `json:"_id"`
					Name           string `json:"name"`
					DealPipelineID string `json:"deal_pipeline_id"`
				}{
					{ID: "s1", Name: "Apresentacao", DealPipelineID: "pipeline-a"},
					{ID: "s2", Name: "Apresentacao", DealPipelineID: "pipeline-b"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := rdClient.NewWithBaseURL("key", srv.URL)
	svc := rdSvc.New(client)

	_, err := svc.CreateDeal(context.Background(), rdSvc.CreateDealParams{Name: "ABC - Credito", Stage: "Apresentacao"})
	if err == nil {
		t.Fatal("expected ambiguous stage error")
	}
	var ambiguous *rdSvc.AmbiguousStageError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("expected AmbiguousStageError, got %T: %v", err, err)
	}
	if len(ambiguous.Stages) != 2 {
		t.Fatalf("expected two ambiguous stages, got %#v", ambiguous.Stages)
	}
}

func TestCreateDeal_StageWithPipelineUsesMatchingStage(t *testing.T) {
	var createdStageID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/deal_stages":
			resp := rdClient.DealStageListResponse{
				DealStages: []struct {
					ID             string `json:"_id"`
					Name           string `json:"name"`
					DealPipelineID string `json:"deal_pipeline_id"`
				}{
					{ID: "s1", Name: "Apresentacao", DealPipelineID: "pipeline-a"},
					{ID: "s2", Name: "Apresentacao", DealPipelineID: "pipeline-b"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		case "/deals":
			var payload struct {
				Deal struct {
					DealStageID string `json:"deal_stage_id"`
				} `json:"deal"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			createdStageID = payload.Deal.DealStageID
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(rdClient.DealResponse{
				ID:        "d1",
				Name:      "ABC - Credito",
				DealStage: rdClient.DealStageResponse{ID: createdStageID, Name: "Apresentacao", DealPipelineID: "pipeline-b"},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := rdClient.NewWithBaseURL("key", srv.URL)
	svc := rdSvc.New(client)

	result, err := svc.CreateDeal(context.Background(), rdSvc.CreateDealParams{Name: "ABC - Credito", Stage: "Apresentacao", Pipeline: "pipeline-b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if createdStageID != "s2" || result.Deal.Stage.PipelineID != "pipeline-b" {
		t.Fatalf("expected pipeline-b stage s2, got stage id %q deal %#v", createdStageID, result.Deal)
	}
}

func TestCreateDeal_CreatesFollowUpTaskAfterDeal(t *testing.T) {
	createdDeal := false
	createdTask := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/deals":
			if r.Method == http.MethodGet {
				_ = json.NewEncoder(w).Encode(rdClient.DealsListResponse{Deals: nil, Total: 0})
				return
			}
			createdDeal = true
			_ = json.NewEncoder(w).Encode(rdClient.DealResponse{ID: "d1", Name: "ABC - Credito"})
		case "/tasks":
			if r.Method == http.MethodGet {
				if got := r.URL.Query().Get("deal_id"); got != "d1" {
					t.Fatalf("expected task lookup for created deal, got %q", got)
				}
				_ = json.NewEncoder(w).Encode(rdClient.TasksListResponse{Tasks: nil, Total: 0})
				return
			}
			createdTask = true
			_ = json.NewEncoder(w).Encode(rdClient.TaskResponse{ID: "t1", Subject: "Ligar", Date: "2026-07-30", Hour: "10:00", Deal: rdClient.TaskDealResponse{ID: "d1", Name: "ABC - Credito"}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	svc := rdSvc.New(rdClient.NewWithBaseURL("key", srv.URL))
	result, err := svc.CreateDeal(context.Background(), rdSvc.CreateDealParams{
		Name:            "ABC - Credito",
		FollowUpSubject: "Ligar",
		FollowUpDate:    "2026-07-30",
		FollowUpHour:    "10:00",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !createdDeal || !createdTask || result.Task.ID != "t1" {
		t.Fatalf("expected deal and follow-up task created, result %#v", result)
	}
}

func TestCreateScheduledTask_ReusesExistingEquivalentTask(t *testing.T) {
	postCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/deals":
			_ = json.NewEncoder(w).Encode(rdClient.DealsListResponse{
				Deals: []rdClient.DealResponse{{ID: "d1", Name: "ABC - Credito"}},
				Total: 1,
			})
		case "/tasks":
			if r.Method == http.MethodPost {
				postCount++
				t.Fatalf("did not expect duplicate task post")
			}
			_ = json.NewEncoder(w).Encode(rdClient.TasksListResponse{
				Tasks: []rdClient.TaskResponse{{ID: "t1", Subject: "Ligar", Type: "task", Date: "2026-07-30", Hour: "10:00", Deal: rdClient.TaskDealResponse{ID: "d1", Name: "ABC - Credito"}}},
				Total: 1,
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	svc := rdSvc.New(rdClient.NewWithBaseURL("key", srv.URL))
	task, err := svc.CreateScheduledTask(context.Background(), rdSvc.CreateScheduledTaskParams{
		DealName: "ABC - Credito",
		Subject:  "Ligar",
		Type:     "task",
		Date:     "2026-07-30",
		Hour:     "10:00",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.ID != "t1" || postCount != 0 {
		t.Fatalf("expected existing task reused, got %#v postCount=%d", task, postCount)
	}
}

func TestCreateDealActivity_ResolvesDealByCustomerAndAvoidsDuplicate(t *testing.T) {
	postCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/deals":
			_ = json.NewEncoder(w).Encode(rdClient.DealsListResponse{
				Deals: []rdClient.DealResponse{{
					ID:       "d1",
					Name:     "ABC - Credito",
					Contacts: []rdClient.DealContactResponse{{ID: "c1", Name: "Empresa ABC"}},
				}},
				Total: 1,
			})
		case "/activities":
			if r.Method == http.MethodPost {
				postCount++
				t.Fatalf("did not expect duplicate activity post")
			}
			_ = json.NewEncoder(w).Encode(rdClient.ActivitiesListResponse{
				Activities: []rdClient.ActivityResponse{{ID: "a1", Text: "Cliente aprovou a proposta", Date: "2026-07-29"}},
				Total:      1,
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	svc := rdSvc.New(rdClient.NewWithBaseURL("key", srv.URL))
	activity, err := svc.CreateDealActivity(context.Background(), rdSvc.CreateDealActivityParams{
		Company: "Empresa ABC",
		UserID:  "u1",
		Text:    "Cliente aprovou a proposta",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if activity.ID != "a1" || postCount != 0 {
		t.Fatalf("expected existing activity reused, got %#v postCount=%d", activity, postCount)
	}
}

func TestGetDealSummaryForOwners_GathersDealActivitiesContactsAndOpenTasks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/deals":
			if got := r.URL.Query().Get("name"); got != "Alpha" {
				t.Errorf("expected deal name query Alpha, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(rdClient.DealsListResponse{
				Deals: []rdClient.DealResponse{{
					ID:        "d1",
					Name:      "Alpha",
					DealStage: rdClient.DealStageResponse{ID: "s1", Name: "Amostra"},
					DealOwner: rdClient.DealUserResponse{ID: "u1", Name: "Glauco"},
				}},
				Total: 1,
			})
		case "/deals/d1":
			_ = json.NewEncoder(w).Encode(rdClient.DealResponse{
				ID:        "d1",
				Name:      "Alpha",
				DealStage: rdClient.DealStageResponse{ID: "s1", Name: "Amostra"},
				DealOwner: rdClient.DealUserResponse{ID: "u1", Name: "Glauco"},
				Contacts:  []rdClient.DealContactResponse{{ID: "c1", Name: "Maria"}},
			})
		case "/deals/d1/contacts":
			_ = json.NewEncoder(w).Encode(rdClient.DealContactsListResponse{
				Contacts: []rdClient.DealContactResponse{{ID: "c1", Name: "Maria"}},
				Total:    1,
			})
		case "/activities":
			if got := r.URL.Query().Get("deal_id"); got != "d1" {
				t.Errorf("expected activity deal_id d1, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(rdClient.ActivitiesListResponse{
				Activities: []rdClient.ActivityResponse{{ID: "a1", Text: "Cliente pediu proposta ajustada", Date: "2026-06-30"}},
				Total:      1,
			})
		case "/tasks":
			if got := r.URL.Query().Get("deal_id"); got != "d1" {
				t.Errorf("expected task deal_id d1, got %q", got)
			}
			if got := r.URL.Query().Get("limit"); got != strconv.Itoa(20) {
				t.Errorf("expected limit=20, got %q", got)
			}
			if r.URL.Query().Get("done") == "true" {
				_ = json.NewEncoder(w).Encode(rdClient.TasksListResponse{
					Tasks: []rdClient.TaskResponse{{
						ID:      "t2",
						Subject: "Primeiro contato realizado",
						Date:    "2026-06-30",
						Hour:    "09:00",
						Deal:    rdClient.TaskDealResponse{ID: "d1", Name: "Alpha"},
						Users:   []rdClient.TaskUserResponse{{Name: "Glauco"}},
					}},
					Total: 1,
				})
				return
			}
			if got := r.URL.Query().Get("done"); got != "false" {
				t.Errorf("expected done=false, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(rdClient.TasksListResponse{
				Tasks: []rdClient.TaskResponse{{
					ID:      "t1",
					Subject: "Follow-up comercial",
					Date:    "2026-07-01",
					Hour:    "10:00",
					Deal:    rdClient.TaskDealResponse{ID: "d1", Name: "Alpha"},
					Users:   []rdClient.TaskUserResponse{{Name: "Glauco"}},
				}},
				Total: 1,
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := rdClient.NewWithBaseURL("key", srv.URL)
	svc := rdSvc.New(client)

	summary, err := svc.GetDealSummaryForOwners(context.Background(), "Alpha", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Deal.Name != "Alpha" || summary.Deal.Stage.Name != "Amostra" {
		t.Fatalf("unexpected deal summary: %#v", summary.Deal)
	}
	if len(summary.Contacts) != 1 || summary.Contacts[0].Name != "Maria" {
		t.Fatalf("expected contact Maria, got %#v", summary.Contacts)
	}
	if len(summary.Activities) != 1 || summary.Activities[0].Text != "Cliente pediu proposta ajustada" {
		t.Fatalf("expected activity text, got %#v", summary.Activities)
	}
	if len(summary.OpenTasks) != 1 || summary.OpenTasks[0].Subject != "Follow-up comercial" {
		t.Fatalf("expected open task, got %#v", summary.OpenTasks)
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
