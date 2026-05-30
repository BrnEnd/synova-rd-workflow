Documento: SDD — Feature: Consulta de Anotações de Negociação
Versão: 1.0.0
Data: 30/05/2026
Status: Rascunho
Gerado por: GitHub Copilot

---

# SDD — Feature: Consulta de Anotações de Negociação

## 1. Visão Geral

**Objetivo:** Permitir que colaboradores consultem as anotações manuais registradas em uma negociação do RD Station CRM diretamente pelo WhatsApp, em linguagem natural.

**Escopo — dentro:**
- Novo intent `get_deal_activities` reconhecido pelo NLP
- Consulta de anotações via `GET /api/v1/activities?deal_id=` na API do RD Station
- Resposta formatada em linguagem natural com lista de anotações (texto + data)
- Respeito ao filtro de responsável (`AllowedOwnerID`) — colaborador só acessa negociações que lhe pertencem

**Escopo — fora:**
- Consulta de eventos automáticos de sistema (mudanças de estágio, etc.) — a API do RD Station não expõe esse histórico
- Paginação completa — retorna as 20 anotações mais recentes
- Criação de anotações (coberta por SDD separado)

**Público-alvo:** Desenvolvedor responsável pela manutenção do synova-rd-workflow.

---

## 2. Contexto e Motivação

A API do RD Station CRM **não expõe histórico automático de eventos** (mudanças de estágio, e-mails enviados, etc.) via endpoint público. O único histórico consultável são as **atividades manuais** (`/api/v1/activities`) — anotações criadas por colaboradores.

Quando um colaborador pergunta "quais são as anotações da negociação X?", o sistema atualmente retorna erro de intent desconhecida. Esta feature preenche essa lacuna.

---

## 3. Fluxo de dados

```
WhatsApp message: "me mostra as anotações da negociação Acme"
    → handler/evolution: AccessProfiler.AccessProfile(ctx, phone) → Actor{RDStationID, Role, TeamRDUserIDs}
    → NLP (ParseIntent): intent="get_deal_activities", parameters={deal_name: "Acme"}
    → Router.RouteForActor: case IntentGetDealActivities
    → Service.GetDealActivities(ctx, "Acme", allowedOwnerIDs)
        → findSingleDeal(ctx, "Acme", allowedOwnerIDs) → deal.ID
        → client.GetActivities(ctx, deal.ID) → []ActivityResponse
    → NLP (FormatResponse): formata lista em linguagem natural
    → WhatsApp response
```

### 3.1 Filtro de acesso

O campo `AllowedOwnerID` é construído a partir de `actor.RDStationID` e `actor.TeamRDUserIDs` (mesma lógica de `allowedOwnerIDs()` já existente no router). A função `findSingleDeal` valida que a negociação pertence ao colaborador antes de consultar as atividades.

---

## 4. API do RD Station

- **Endpoint:** `GET /api/v1/activities`
- **Parâmetros:** `deal_id=<id>` (obrigatório), `limit=20`
- **Autenticação:** `?token=<RDSTATION_TOKEN>` (padrão do client)
- **Resposta:**
  ```json
  {
    "activities": [
      {
        "_id": "abc123",
        "date": "2026-05-28T14:30:00.000Z",
        "deal_id": "def456",
        "text": "Cliente confirmou interesse na proposta",
        "user_id": "ghi789"
      }
    ],
    "total": 1
  }
  ```
- **Limite de taxa:** compartilhado com os demais endpoints (120 req/min)

---

## 5. Alterações necessárias

### 5.1 `internal/domain/intent.go`

Adicionar nova constante de intent:

```go
IntentGetDealActivities IntentName = "get_deal_activities"
```

### 5.2 `internal/client/rdstation/client.go`

**Novos tipos de resposta:**

```go
type ActivityResponse struct {
    ID     string `json:"_id"`
    Date   string `json:"date"`
    DealID string `json:"deal_id"`
    Text   string `json:"text"`
    UserID string `json:"user_id"`
}

type ActivitiesListResponse struct {
    Activities []ActivityResponse `json:"activities"`
    Total      int                `json:"total"`
}
```

**Novo método `GetActivities`:**

```go
// GetActivities retorna as anotações manuais de uma negociação.
func (c *Client) GetActivities(ctx context.Context, dealID string) ([]ActivityResponse, error) {
    q := url.Values{}
    q.Set("deal_id", dealID)
    q.Set("limit", "20")

    data, err := c.do(ctx, http.MethodGet, "/activities?"+q.Encode(), nil)
    if err != nil {
        return nil, err
    }

    var result ActivitiesListResponse
    if err := json.Unmarshal(data, &result); err != nil {
        return nil, fmt.Errorf("rdstation GetActivities unmarshal: %w", err)
    }

    return result.Activities, nil
}
```

### 5.3 `internal/domain/` — novo tipo `Activity`

Adicionar em `internal/domain/deal.go` (ou em novo arquivo `activity.go`):

```go
type Activity struct {
    ID   string
    Text string
    Date string
}
```

### 5.4 `internal/service/rdstation/service.go`

**Novo método `GetDealActivities`:**

```go
// GetDealActivities retorna as anotações de uma negociação identificada pelo nome.
func (s *Service) GetDealActivities(ctx context.Context, dealName string, allowedOwnerID map[string]struct{}) ([]domain.Activity, error) {
    deal, err := s.findSingleDeal(ctx, dealName, allowedOwnerID)
    if err != nil {
        return nil, err
    }

    resp, err := s.client.GetActivities(ctx, deal.ID)
    if err != nil {
        return nil, err
    }

    activities := make([]domain.Activity, len(resp))
    for i, a := range resp {
        activities[i] = domain.Activity{ID: a.ID, Text: a.Text, Date: a.Date}
    }
    return activities, nil
}
```

### 5.5 `internal/service/intent_router/router.go`

Adicionar case no switch de `RouteForActor`:

```go
case domain.IntentGetDealActivities:
    return r.rdstation.GetDealActivities(ctx, p["deal_name"], owners)
```

Adicionar ao switch de `ResolveDealSelection` (para o fluxo de desambiguação de deals):

```go
case domain.IntentGetDealActivities:
    return r.rdstation.GetDealActivitiesByID(ctx, deal.ID)
```

> `GetDealActivitiesByID` é uma variação interna que recebe o ID diretamente, evitando nova busca.

### 5.6 `internal/service/nlp/service.go`

**`systemPrompt`** — adicionar linha na lista de intents:

```
- get_deal_activities → parâmetro OBRIGATÓRIO: "deal_name" (nome da negociação cujas anotações serão consultadas)
```

**`intentTool`** — adicionar `"get_deal_activities"` ao enum:

```go
"enum": []string{
    "get_contacts", "get_deals", "get_deal", "get_deal_contacts",
    "create_contact", "create_deal", "update_deal", "move_deal_stage",
    "delete_deal", "update_contact", "associate_contact_to_deal",
    "get_deal_activities", // novo
    "unknown",
},
```

Atualizar descrição do parâmetro `deal_name` para incluir `get_deal_activities`.

---

## 6. Arquivos alterados

| Arquivo | Tipo de alteração |
|---------|-------------------|
| `internal/domain/intent.go` | Nova constante `IntentGetDealActivities` |
| `internal/domain/deal.go` | Novo tipo `Activity` (ou novo `activity.go`) |
| `internal/client/rdstation/client.go` | Novos tipos `ActivityResponse`, `ActivitiesListResponse`; novo método `GetActivities` |
| `internal/service/rdstation/service.go` | Novo método `GetDealActivities` |
| `internal/service/intent_router/router.go` | Novo case `IntentGetDealActivities` em `RouteForActor` e `ResolveDealSelection` |
| `internal/service/nlp/service.go` | Atualização do `systemPrompt` e schema do `intentTool` |

---

## 7. Testes

### 7.1 Unitários — `internal/service/rdstation/service_test.go`

- Cenário feliz: mock retorna lista de atividades → verificar mapeamento para `[]domain.Activity`
- Negociação não encontrada: `DealNotFoundError` propagado
- Múltiplas negociações com mesmo nome: `MultipleDealsError` → fluxo de seleção existente
- Lista vazia: retornar slice vazio sem erro

### 7.2 Unitários — `internal/service/intent_router/router_test.go`

- Verificar que `IntentGetDealActivities` chama `rdstation.GetDealActivities` com os parâmetros corretos

### 7.3 Integração

- Enviar mensagem "quais são as anotações da negociação X?" e verificar resposta formatada

---

## 8. Comportamento esperado

**Entrada do usuário:**
> "me mostra as anotações da negociação Acme 2026"

**Resposta do agente:**
> "Encontrei 2 anotações na negociação *Acme 2026*:
>
> 1. **28/05/2026** — Cliente confirmou interesse na proposta
> 2. **15/05/2026** — Reunião agendada para apresentação"

**Sem anotações:**
> "A negociação *Acme 2026* não possui anotações registradas."

---

## 9. Riscos e mitigações

| Risco | Probabilidade | Mitigação |
|-------|---------------|-----------|
| Negociação com nome ambíguo | Média | `MultipleDealsError` → fluxo de seleção já implementado |
| Negociação não pertencente ao colaborador | Baixa | `AllowedOwnerID` filtra na busca da deal antes de consultar atividades |
| Limite de taxa da API (120 req/min) | Baixa | `doWithRetry` com `Retry-After` já implementado no client |
| Lista muito longa de anotações | Baixa | `limit=20` garante resposta razoável; paginação pode ser adicionada futuramente |
