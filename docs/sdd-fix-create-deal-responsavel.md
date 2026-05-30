Documento: SDD — Fix: create_deal sem responsável
Versão: 1.0.0
Data: 30/05/2026
Status: Rascunho
Gerado por: GitHub Copilot

---

# SDD — Fix: create_deal não atribui responsável pelo conversador

## 1. Visão Geral

**Objetivo:** Corrigir o bug em que negociações criadas via WhatsApp são atribuídas ao dono do token de autenticação do RD Station, e não ao colaborador que está conversando com o agente.

**Escopo — dentro:**
- Propagação de `user_id` (RD Station ID do colaborador) no payload de criação de negociação
- Ajuste nas três camadas: client, service e router

**Escopo — fora:**
- Alteração no fluxo de autenticação do token do RD Station
- Alteração no cadastro de colaboradores no admin panel

**Público-alvo:** Desenvolvedor responsável pela manutenção do synova-rd-workflow.

---

## 2. Diagnóstico

### 2.1 Comportamento atual

Quando o colaborador solicita a criação de uma negociação via WhatsApp, o sistema executa `POST /api/v1/deals` na API do RD Station **sem enviar o campo `user_id`**. O RD Station interpreta a ausência do campo como "atribuir ao dono do token de autenticação", resultando na negociação sendo criada sempre no nome do mesmo usuário técnico — independente de quem está conversando com o agente.

### 2.2 Causa raiz

O campo `user_id` nunca foi incorporado ao fluxo de criação de negociação em nenhuma das camadas:

| Camada | Arquivo | Problema |
|--------|---------|----------|
| Router | `internal/service/intent_router/router.go` | `IntentCreateDeal` não passa `actor.RDStationID` para o service |
| Service | `internal/service/rdstation/service.go` | `CreateDealParams` não possui campo `UserID` |
| Client | `internal/client/rdstation/client.go` | `CreateDealParams` não possui `UserID`; payload HTTP não inclui `"user_id"` |

### 2.3 Infraestrutura existente que resolve o problema

O sistema já possui o mecanismo correto para obter o RD Station ID do conversador:

```
WhatsApp phone number
    → AccessProfiler.AccessProfile(ctx, phone)
    → domain.AccessProfile { RDStationID string }
    → intentRouter.Actor { RDStationID string }
```

O campo `actor.RDStationID` chega corretamente preenchido em `RouteForActor`. O único problema é que ele não é repassado no `case domain.IntentCreateDeal`.

---

## 3. Solução

### 3.1 Decisão de design

O `user_id` **não deve ser extraído pelo NLP** (não é um parâmetro conversacional). Ele vem exclusivamente do `actor.RDStationID`, derivado do número de WhatsApp via `AccessProfile`. Isso garante que o responsável é sempre o colaborador autenticado que iniciou a conversa, sem risco de manipulação via mensagem de texto.

### 3.2 Tratamento de `RDStationID` vazio

Se `actor.RDStationID` estiver vazio (colaborador sem ID do RD Station configurado no admin panel), o campo `user_id` não é enviado e o RD Station usa o dono do token como fallback. O service deve emitir um log de warning neste caso — não um erro, pois a negociação ainda é criada com sucesso.

---

## 4. Alterações necessárias

### 4.1 `internal/client/rdstation/client.go`

**Tipo `CreateDealParams`** — adicionar campo `UserID`:

```go
// Antes
type CreateDealParams struct {
    Name        string
    DealStageID string
    ContactIDs  []string
}

// Depois
type CreateDealParams struct {
    Name        string
    DealStageID string
    ContactIDs  []string
    UserID      string
}
```

**Função `CreateDeal`** — incluir `user_id` no payload quando presente:

```go
// Dentro de CreateDeal(), no bloco de montagem do map `deal`:
if params.UserID != "" {
    deal["user_id"] = params.UserID
}
```

### 4.2 `internal/service/rdstation/service.go`

**Tipo `CreateDealParams`** — adicionar campo `UserID`:

```go
// Antes
type CreateDealParams struct {
    Name        string
    ContactName string
    Stage       string
}

// Depois
type CreateDealParams struct {
    Name        string
    ContactName string
    Stage       string
    UserID      string
}
```

**Função `CreateDeal`** — repassar `UserID` ao client:

```go
// Antes
clientParams := rdClient.CreateDealParams{Name: params.Name}

// Depois
clientParams := rdClient.CreateDealParams{
    Name:   params.Name,
    UserID: params.UserID,
}
```

### 4.3 `internal/service/intent_router/router.go`

**`case domain.IntentCreateDeal`** — passar `actor.RDStationID`:

```go
// Antes
case domain.IntentCreateDeal:
    return r.rdstation.CreateDeal(ctx, rdSvc.CreateDealParams{
        Name:        p["name"],
        ContactName: p["contact_name"],
        Stage:       p["stage"],
    })

// Depois
case domain.IntentCreateDeal:
    return r.rdstation.CreateDeal(ctx, rdSvc.CreateDealParams{
        Name:        p["name"],
        ContactName: p["contact_name"],
        Stage:       p["stage"],
        UserID:      actor.RDStationID,
    })
```

---

## 5. Arquivos alterados

| Arquivo | Tipo de alteração |
|---------|-------------------|
| `internal/client/rdstation/client.go` | Adicionar campo e condicional no payload |
| `internal/service/rdstation/service.go` | Adicionar campo e repasse ao client |
| `internal/service/intent_router/router.go` | Passar `actor.RDStationID` |

---

## 6. Testes

### 6.1 Unitários

- `internal/service/rdstation/service_test.go`: mock do client deve verificar que `UserID` é corretamente repassado ao `CreateDeal` do client quando fornecido.
- Cenário: `UserID` vazio — verificar que o campo não é incluído no payload (para não sobrescrever o comportamento padrão da API).

### 6.2 Integração

- Criar negociação e verificar que o campo `user.id` (ou `owner.id`) no `DealResponse` corresponde ao `RDStationID` do colaborador que fez a requisição.

---

## 7. Pré-requisito operacional

Cada colaborador deve ter o campo **RD Station ID** preenchido no admin panel (tela de colaboradores). Sem esse cadastro, `actor.RDStationID` chegará vazio e a negociação continuará sendo atribuída ao dono do token.

---

## 8. Riscos e mitigações

| Risco | Probabilidade | Mitigação |
|-------|---------------|-----------|
| Colaborador sem `RDStationID` cadastrado | Médio | Log de warning; negociação é criada com responsável fallback; admin panel deve tornar o campo obrigatório |
| ID inválido rejeitado pelo RD Station (HTTP 422) | Baixo | Erro propagado naturalmente pelo `doWithRetry`; mensagem de erro retornada ao usuário |
