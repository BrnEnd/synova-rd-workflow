package nlp

import (
	"context"
	"strings"
	"testing"

	"synova-rd-workflow/internal/domain"
)

func TestGetDealSummaryPromptIncludesGuardrails(t *testing.T) {
	t.Setenv("DEAL_SUMMARY_PROMPT_TEMPLATE", "")

	prompt := getDealSummaryPrompt()

	for _, want := range []string{
		"Não suponha o que vai acontecer depois.",
		"Não crie \"próximo passo provável\".",
		"Use as tarefas abertas apenas para informar o que está agendado no RD, com assunto, data, horário e responsável disponíveis.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q, got:\n%s", want, prompt)
		}
	}
}

func TestGetDealSummaryPromptAppendsGuardrailsToCustomTemplate(t *testing.T) {
	t.Setenv("DEAL_SUMMARY_PROMPT_TEMPLATE", "Template customizado do cliente.")

	prompt := getDealSummaryPrompt()

	if !strings.Contains(prompt, "Template customizado do cliente.") {
		t.Fatalf("expected custom template, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Não suponha o que vai acontecer depois.") {
		t.Fatalf("expected guardrails in custom template, got:\n%s", prompt)
	}
}

func TestFormatResponseGetDealsListsAllItemsWithStableFormat(t *testing.T) {
	svc := &Service{}
	deals := make([]domain.Deal, 12)
	for i := range deals {
		deals[i] = domain.Deal{
			Name:     "Negociação " + string(rune('A'+i)),
			Stage:    domain.Stage{Name: "Amostra"},
			Owner:    domain.DealOwner{Name: "Glauco"},
			Contacts: []domain.Contact{{Name: "Maria"}},
		}
	}

	got, err := svc.FormatResponse(context.Background(), domain.Intent{Name: domain.IntentGetDeals}, deals)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Count(got, "Nome: Negociação ") != 12 {
		t.Fatalf("expected all 12 deals, got:\n%s", got)
	}
	for _, want := range []string{"Nome: Negociação A", "Etapa: Amostra", "Contato: Maria", "Responsavel: Glauco"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in response, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "*Negociação A*") {
		t.Fatalf("expected stable deal fields, got:\n%s", got)
	}
}

func TestFormatResponseGetDealsLimitsAtThirtyItems(t *testing.T) {
	svc := &Service{}
	deals := make([]domain.Deal, 35)
	for i := range deals {
		deals[i] = domain.Deal{
			Name:  "Negociação " + string(rune('A'+i)),
			Stage: domain.Stage{Name: "Amostra"},
			Owner: domain.DealOwner{Name: "Glauco"},
		}
	}

	got, err := svc.FormatResponse(context.Background(), domain.Intent{Name: domain.IntentGetDeals}, deals)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Count(got, "Nome: Negociação ") != 30 {
		t.Fatalf("expected only 30 displayed deals, got:\n%s", got)
	}
	if strings.Contains(got, "31.") {
		t.Fatalf("did not expect item 31 in response:\n%s", got)
	}
	if !strings.Contains(got, "Mostrei as 30 negociações mais recentes. Existem mais 5 negociações que não foram exibidas por limitação de tamanho da resposta.") {
		t.Fatalf("expected size limitation warning, got:\n%s", got)
	}
}

func TestFormatResponseContactsUsesStableFormat(t *testing.T) {
	svc := &Service{}
	contacts := []domain.Contact{{Name: "Maria", Email: "maria@example.com", Phone: "11999999999"}}

	got, err := svc.FormatResponse(context.Background(), domain.Intent{Name: domain.IntentGetContacts}, contacts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{"1. *Maria*", "E-mail: maria@example.com", "Telefone: 11999999999"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in response, got:\n%s", want, got)
		}
	}
}
