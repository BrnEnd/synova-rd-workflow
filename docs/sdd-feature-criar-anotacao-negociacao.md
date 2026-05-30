Documento: SDD — Feature: Criar Anotação em Negociação
Versão: 1.0.0
Data: 30/05/2026
Status: Rascunho
Gerado por: GitHub Copilot

---

# SDD — Feature: Criar Anotação em Negociação

## 1. Visão Geral

**Objetivo:** Permitir que colaboradores registrem anotações manuais em negociações do RD Station CRM diretamente pelo WhatsApp, em linguagem natural.

**Escopo — dentro:**
- Novo intent `create_deal_activity` reconhecido pelo NLP
- Criação de anotação via `POST /api/v1/activities` na API do RD Station
- `user_id` derivado automaticamente do `actor.RDStationID` (número de WhatsApp do conversador) — não é um parâmetro conversacional
- Respeito ao filtro de responsável — colaborador só pode anotar em negociações que lhe pertencem

**Escopo — fora:**
- Edição ou exclusão de anotações — a API do RD Station não suporta atualização de atividades após criação
- Consulta de anotações (coberta por SDD separado)

**Público-alvo:** Desenvolvedor responsável pela manutenção do synova-rd-workflow.

---

## 2. Contexto e Motivação

Colaboradores frequentemente precisam registrar o resultado de uma ligação ou reunião enquanto ainda estão em deslocamento. O acesso ao CRM via browser é lento nesse contexto. Com esta feature, o colaborador pode enviar uma mensagem pelo WhatsApp do tipo "anota na negociação Acme que o cliente confirmou a proposta" e o registro é feito imediatamente no RD Station.

### 2.1 Decisão de design: `user_id` não é parâmetro do NLP

O `user_id` (quem criou a anotação) **não deve ser extraído pelo NLP**. Ele vem exclusivamente do `actor.RDStationID`, derivado do número de WhatsApp via `AccessProfiler.AccessProfile(ctx, phone)`. Isso garante que:

1. A anotação é sempre atribuída ao colaborador autenticado que iniciou a conversa.
2. Não há risco de manipulação via texto (ex: usuário fingindo ser outro colaborador).

---

## 3. Fluxo de dados

```
WhatsApp message: "anota na negociação Acme que o cliente confirmou a proposta"
    → handler/evolution: AccessProfiler.AccessProfile(ctx, phone) → Actor{RDStationID, Role, TeamRDUserIDs}
    → NLP (ParseIntent): intent="create_deal_activity", parameters={deal_name: "Acme", text: "cliente confirmou a proposta"}
    → Router.RouteForActor: case IntentCreateDealActivity
    → Service.CreateDealActivity(ctx, CreateDealActivityParams{DealName, UserID: actor.RDStationID, Text, AllowedOwnerID})
        → findSingleDeal(ctx, DealName, AllowedOwnerID) → deal.ID
        → Valida: UserID não pode ser vazio
        → client.CreateActivity(ctx, deal.ID, UserID, Text) → ActivityResponse
    → NLP (FormatResponse): confirmação em linguagem natural
    → WhatsApp response
```

---

## 4. API do RD Station

- **Endpoint:** `POST /api/v1/activities`
- **Body:**
  ```json
  {
    "activity": {
      "deal_id": "<id da negociação>",
      "user_id": "<id do usuário no RD Station>",
      "text":    "<texto da anotação>"
    }
  }
  ```
- **Resposta (201 Created):**
  ```json
  {
    "_id": "abc123",
    "date": "2026-05-30T10:15:00.000Z",
    "deal_id": "def456",
    "text": "cliente confirmou a proposta",
    "user_id": "ghi789"
  }
  ```
- **Restrições:** anotações são imutáveis após criação (não há PATCH/DELETE)
- **Limite de taxa:** compartilhado com os demais endpoints (120 req/min)

---

## 5. Alterações necessárias

### 5.1 `internal/domain/intent.go`

Adicionar nova constante:

```go
IntentCreateDealActivity IntentName = "create_deal_activity"
```

### 5.2 `internal/client/rdstation/client.go`

> **Nota:** os tipos `ActivityResponse` e `ActivitiesListResponse` são adicionados pelo SDD de consulta de anotações. Se esta feature for implementada antes, adicioná-los aqui também.

**Novo método `CreateActivity`:**

```go
// CreateActivity cria uma anotação manual em uma negociação.
func (c *Client) CreateActivity(ctx context.Context, dealID, userID, text string) (ActivityResponse, error) {
    payload := map[string]interface{}{
        "activity": map[string]interface{}{
            "deal_id": dealID,
            "user_id": userID,
            "text":    text,
        },
    }

    data, err := c.do(ctx, http.MethodPost, "/activities", payload)
    if err != nil {
        return ActivityResponse{}, err
    }

    var result ActivityResponse
    if err := json.Unmarshal(data, &result); err != nil {
        return ActivityResponse{}, fmt.Errorf("rdstation CreateActivity unmarshal: %w", err)
    }

    return result, nil
}
```

### 5.3 `internal/domain/` — tipo `Activity`

> Já coberto pelo SDD de consulta de anotações. Se esta feature for implementada antes, adicionar em `internal/domain/deal.go`:

```go
type Activity struct {
    ID   string
    Text string
    Date string
}
```

### 5.4 `internal/service/rdstation/service.go`

**Novo tipo `CreateDealActivityParams`:**

```go
type CreateDealActivityParams struct {
    DealName       string
    UserID         string
    Text           string
    AllowedOwnerID map[string]struct{}
}
```

**Novo método `CreateDealActivity`:**

```go
// CreateDealActivity registra uma anotação manual em uma negociação identificada pelo nome.
func (s *Service) CreateDealActivity(ctx context.Context, params CreateDealActivityParams) (domain.Activity, error) {
    if params.UserID == "" {
        return domain.Activity{}, fmt.Errorf("seu perfil não possui RD Station ID configurado; contate o administrador")
    }

    deal, err := s.findSingleDeal(ctx, params.DealName, params.AllowedOwnerID)
    if err != nil {
        return domain.Activity{}, err
    }

    resp, err := s.client.CreateActivity(ctx, deal.ID, params.UserID, params.Text)
    if err != nil {
        return domain.Activity{}, err
    }

    return domain.Activity{ID: resp.ID, Text: resp.Text, Date: resp.Date}, nil
}
```

### 5.5 `internal/service/intent_router/router.go`

Adicionar case no switch de `RouteForActor`:

```go
case domain.IntentCreateDealActivity:
    return r.rdstation.CreateDealActivity(ctx, rdSvc.CreateDealActivityParams{
        DealName:       p["deal_name"],
        UserID:         actor.RDStationID,
        Text:           p["text"],
        AllowedOwnerID: owners,
    })
```

### 5.6 `internal/service/nlp/service.go`

**`systemPrompt`** — adicionar linha na lista de intents:

```
- create_deal_activity → parâmetros: "deal_name" (OBRIGATÓRIO — negociação onde registrar), "text" (OBRIGATÓRIO — conteúdo da anotação)
```

Adicionar regra crítica:

```
- Para create_deal_activity: extraia exatamente o que o usuário quer registrar como "text"; o responsável pela anotação é determinado automaticamente pelo sistema
```

**`intentTool`** — adicionar `"create_deal_activity"` ao enum:

```go
"enum": []string{
    "get_contacts", "get_deals", "get_deal", "get_deal_contacts",
    "create_contact", "create_deal", "update_deal", "move_deal_stage",
    "delete_deal", "update_contact", "associate_contact_to_deal",
    "create_deal_activity", // novo
    "unknown",
},
```

Adicionar parâmetro `text` no schema de parâmetros:

```go
"text": map[string]interface{}{
    "type":        "string",
    "description": "Conteúdo textual da anotação a registrar na negociação (create_deal_activity)",
},
```

Atualizar descrição de `deal_name` para incluir `create_deal_activity`.

---

## 6. Arquivos alterados

| Arquivo | Tipo de alteração |
|---------|-------------------|
| `internal/domain/intent.go` | Nova constante `IntentCreateDealActivity` |
| `internal/domain/deal.go` | Novo tipo `Activity` (se não adicionado pelo SDD de consulta) |
| `internal/client/rdstation/client.go` | Novo método `CreateActivity`; novos tipos de resposta (se não adicionados pelo SDD de consulta) |
| `internal/service/rdstation/service.go` | Novo tipo `CreateDealActivityParams`; novo método `CreateDealActivity` |
| `internal/service/intent_router/router.go` | Novo case `IntentCreateDealActivity` em `RouteForActor` |
| `internal/service/nlp/service.go` | Atualização do `systemPrompt`, enum e schema do `intentTool` |

---

## 7. Validações

| Condição | Comportamento |
|----------|---------------|
| `actor.RDStationID` vazio | Erro com mensagem: "seu perfil não possui RD Station ID configurado; contate o administrador" |
| Negociação não encontrada | `DealNotFoundError` (já implementado) |
| Múltiplas negociações com mesmo nome | `MultipleDealsError` → fluxo de seleção já implementado |
| `text` vazio | NLP garante extração; service não valida (NLP é a fronteira) |
| `user_id` inválido (rejeitado pelo RD Station) | HTTP 422 → `RDStationError` propagado; mensagem de erro retornada ao usuário |

---

## 8. Testes

### 8.1 Unitários — `internal/service/rdstation/service_test.go`

- Cenário feliz: mock do client verifica que `deal_id`, `user_id` e `text` corretos são enviados ao `CreateActivity`
- `UserID` vazio: retorna erro descritivo sem chamar o client
- Negociação não encontrada: `DealNotFoundError` propagado
- Múltiplas negociações com mesmo nome: `MultipleDealsError` propagado

### 8.2 Unitários — `internal/service/intent_router/router_test.go`

- Verificar que `IntentCreateDealActivity` chama `rdstation.CreateDealActivity` com `UserID = actor.RDStationID`

### 8.3 Integração

- Enviar mensagem "anota na negociação X que Y" e verificar que a atividade aparece no RD Station com o colaborador correto como autor

---

## 9. Comportamento esperado

**Entrada do usuário:**
> "anota na negociação Acme 2026 que o cliente pediu prazo de 30 dias para decidir"

**Resposta do agente:**
> "Anotação registrada na negociação *Acme 2026*: \"o cliente pediu prazo de 30 dias para decidir\"."

**Erro — sem RD Station ID:**
> "Não consegui registrar a anotação. Seu perfil não possui RD Station ID configurado. Contate o administrador."

---

## 10. Riscos e mitigações

| Risco | Probabilidade | Mitigação |
|-------|---------------|-----------|
| Colaborador sem `RDStationID` cadastrado | Médio | Validação no service com mensagem clara; admin panel deve tornar o campo obrigatório |
| Anotação criada com texto errado (NLP) | Baixo | `text` é extraído literalmente do que o usuário escreveu; pouca margem para erro |
| Negociação com nome ambíguo | Média | `MultipleDealsError` → fluxo de seleção já implementado |
| Atividade não é editável após criação | Informativo | Comportamento esperado da API do RD Station; comunicar ao usuário se perguntar sobre edição |
| Limite de taxa da API (120 req/min) | Baixo | `doWithRetry` com `Retry-After` já implementado |
