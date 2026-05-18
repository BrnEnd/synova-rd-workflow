Documento: SDD — WhatsApp Real Integration (V2)
Versão: 2.0.0
Data: 04/05/2026
Status: Rascunho
Gerado por: GitHub Copilot
SDD base: sdd-synova-rd-workflow.md v1.0.0

---

# SDD — WhatsApp Real Integration (synova-rd-workflow V2)

## 1. Visão Geral

**Objetivo:** Este documento descreve as mudanças de arquitetura, os novos componentes, os contratos de API e o plano de infraestrutura necessários para substituir o endpoint mock `/mock/message` pela integração real com o WhatsApp Cloud API da Meta, com deploy serverless em AWS totalmente dentro do Free Tier permanente.

**Escopo — dentro:**
- Recebimento de mensagens reais do WhatsApp via webhook da Meta (Cloud API)
- Verificação de assinatura HMAC-SHA256 do webhook
- Envio de respostas de volta ao usuário via Meta Cloud API
- Remoção completa do endpoint mock `/mock/message`
- Substituição de PostgreSQL por DynamoDB para histórico de conversa
- Provisionamento de infraestrutura AWS Free Tier com Terraform: Lambda, API Gateway HTTP API, DynamoDB, IAM, CloudWatch
- Atualização de configuração e variáveis de ambiente

**Escopo — fora:**
- Templates de mensagem aprovados pela Meta (HSM) — necessário apenas para mensagens iniciadas pelo negócio
- Suporte a múltiplos tipos de mensagem (áudio, imagem, documentos) — apenas texto nesta versão
- Múltiplas contas WhatsApp Business simultâneas
- Sistema de refresh automático de Access Token (token permanente é responsabilidade do operador)
- VPC, NAT Gateway, Load Balancer — não são Free Tier e são desnecessários
- EC2, RDS, ElastiCache — fora do escopo desta versão

**Público-alvo:** Desenvolvedor responsável pela implementação.

**Documento de referência:** `sdd-synova-rd-workflow.md` v1.0.0 — os componentes de serviço (NLP, intent_router, rdstation service, conversation service) permanecem inalterados. As mudanças são no entrypoint, no canal de entrada/saída, na store e na infraestrutura.

---

## 2. Contexto e Motivação

O synova-rd-workflow V1 opera com um endpoint mock (`POST /mock/message`) que simula o WhatsApp. Isso exige que o usuário faça chamadas manuais via curl/Postman, o que contradiz o objetivo principal do sistema: permitir consultas ao RD Station CRM diretamente pelo WhatsApp, sem atrito.

Esta V2 remove o mock e conecta o sistema ao canal real, tornando-o operacional para uso diário. O usuário já possui:
- Número de teste da Meta: `+1 555 636 8630`
- Phone Number ID: `000000000000000`
- WhatsApp Business Account ID: `1406386851586660`
- Access Token temporário (expira em ~24h — ver seção 8 para instruções de token permanente)

**Por que Lambda + DynamoDB (e não EC2 + PostgreSQL):**
A abordagem com EC2 t2.micro tem **750 horas/mês** de Free Tier válidas apenas por **12 meses** e somente para contas novas. Lambda e DynamoDB têm Free Tier **permanente** (sem expiração), tornando o sistema gratuito indefinidamente para o volume esperado (uso pessoal por um único usuário).

**Restrições desta versão:**
- Access token temporário exige rotação manual a cada ~24h durante fase de testes. Deve ser convertido para System User Token antes do uso continuado.
- Apenas mensagens de texto são suportadas.
- O processamento ocorre de forma síncrona dentro do timeout da Meta (20s); o pipeline NLP + RD Station cabe confortavelmente nesse limite.

---

## 3. Decisões de Arquitetura

### DA-007 — Meta Cloud API direta (sem Twilio)
**Decisão:** Integrar diretamente com a Meta Cloud API.
**Justificativa:** O usuário já possui credenciais ativas (Phone Number ID, Access Token). Elimina custo por mensagem do Twilio.
**Alternativas rejeitadas:** Twilio — custo por mensagem; intermediário desnecessário.

### DA-008 — AWS Lambda como runtime da aplicação
**Decisão:** Compilar o Go para Lambda (`provided.al2023`, arquitetura `arm64`) e expor via API Gateway HTTP API.
**Justificativa:**
- Free Tier permanente: 1 milhão de invocações/mês + 400.000 GB-segundos/mês — para uso pessoal nunca será ultrapassado.
- API Gateway HTTP API fornece HTTPS nativo — elimina necessidade de Nginx, Let's Encrypt, Certbot, EC2 e qualquer servidor permanente.
- Zero infraestrutura para gerenciar (sem patches de SO, sem monitoramento de disco).
- Cold start de Go em Lambda arm64: ~200-500ms — aceitável, pois ocorre apenas na primeira chamada após idle.
**Alternativas rejeitadas:**
- EC2 t2.micro: Free Tier expira em 12 meses; requer gerenciamento de SO, Nginx, SSL.
- App Runner / ECS Fargate: custo fora do Free Tier.

### DA-009 — DynamoDB para histórico de conversa (substitui PostgreSQL)
**Decisão:** Usar DynamoDB com design de tabela única (single-table) para sessões e mensagens.
**Justificativa:**
- Free Tier permanente: 25 GB de armazenamento + 25 WCU + 25 RCU/mês — mais do que suficiente para uso pessoal.
- Lambda sem VPC acessa DynamoDB via endpoint público da AWS sem custo adicional.
- TTL nativo do DynamoDB expira mensagens antigas automaticamente (mantém dados dentro do Free Tier).
**Alternativas rejeitadas:**
- PostgreSQL em RDS Free Tier: expira em 12 meses; Lambda em VPC requer NAT Gateway (tem custo).
- PostgreSQL em EC2: mesmo problema do EC2 acima.

### DA-010 — API Gateway HTTP API (não REST API)
**Decisão:** Usar API Gateway **HTTP API**.
**Justificativa:** HTTP API tem custo ~70% menor que REST API e Free Tier de 1 milhão de chamadas/mês. Para uso pessoal, praticamente gratuito. Suporta integração Lambda proxy nativa.
**Alternativas rejeitadas:** REST API — mais caro sem benefício adicional para este caso.

### DA-011 — Processamento síncrono dentro do timeout de 20s da Meta
**Decisão:** Processar a mensagem de forma síncrona na invocação Lambda.
**Justificativa:** O pipeline (OpenAI ~5-7s + RD Station ~1-3s + DynamoDB ~50ms) consome menos de 15s. O timeout da Lambda será configurado em 15s. Processamento assíncrono adicionaria complexidade desnecessária.
**Alternativas rejeitadas:** Resposta assíncrona com SQS — complexidade sem necessidade para o volume atual.

### DA-012 — Validação de assinatura X-Hub-Signature-256
**Decisão:** Validar HMAC-SHA256 com o App Secret da Meta em toda requisição POST.
**Justificativa:** Requisito de segurança — garante que apenas a Meta pode disparar o pipeline.
**Alternativas rejeitadas:** Apenas verify token — insuficiente (exposto na URL de verificação GET).

### DA-013 — Remoção completa do endpoint mock e autenticação X-Api-Key
**Decisão:** Remover `POST /mock/message`, o middleware `X-Api-Key` e a variável `APP_API_KEY`.
**Justificativa:** O canal real torna o mock obsoleto. A autenticação HMAC-SHA256 da Meta substitui o `X-Api-Key`.

### DA-014 — Lambda arm64 (Graviton2)
**Decisão:** Compilar para `GOARCH=arm64` e usar runtime `provided.al2023` em `arm64`.
**Justificativa:** ~20% mais rápido e mais barato que x86_64 — ainda dentro do mesmo Free Tier.

---

## 4. Componentes e Responsabilidades

### 4.1 Componentes Novos ou Alterados

| Componente | Status | Responsabilidade |
|---|---|---|
| `cmd/lambda/main.go` | **Novo** (substitui `cmd/api/main.go`) | Entrypoint Lambda: inicializa dependências uma vez (fora do handler para reutilização entre invocações quentes), registra handler com aws-lambda-go-api-proxy |
| `handler/whatsapp/handler.go` | **Novo** (substitui `handler/webhook/handler.go`) | Receber evento do API Gateway; rotear GET (verificação) e POST (mensagens); validar HMAC-SHA256; extrair from + body do payload Meta; acionar pipeline; chamar `client/whatsapp` para enviar resposta |
| `client/whatsapp/client.go` | **Novo** | HTTP client para Meta Cloud API: `SendTextMessage(ctx, to, text)` via `POST /messages` |
| `store/conversation/dynamodb.go` | **Novo** (substitui `postgres.go`) | Implementação DynamoDB da interface `ConversationStore`: `GetOrCreateSession`, `GetRecentMessages`, `SaveMessage` usando single-table design com TTL |
| `config/config.go` | **Alterado** | Substituir `APP_API_KEY`, `POSTGRES_*` por `WHATSAPP_ACCESS_TOKEN`, `WHATSAPP_PHONE_NUMBER_ID`, `WHATSAPP_VERIFY_TOKEN`, `WHATSAPP_APP_SECRET`, `DYNAMODB_TABLE_NAME`, `AWS_REGION` |
| `terraform/` | **Novo** | Infraestrutura AWS: Lambda, API Gateway HTTP API, DynamoDB, IAM roles, CloudWatch Log Group |

### 4.2 Componentes Removidos

| Componente | Motivo |
|---|---|
| `cmd/api/main.go` | Substituído por `cmd/lambda/main.go` |
| `handler/webhook/handler.go` | Substituído por `handler/whatsapp/handler.go` |
| `store/conversation/postgres.go` | Substituído por `store/conversation/dynamodb.go` |
| `migrations/` | DynamoDB não usa SQL migrations — tabela criada pelo Terraform |
| `docker-compose.yml` (deploy) | Sem servidor permanente; Docker Compose fica apenas para dev local com DynamoDB Local |

### 4.3 Componentes Inalterados

`service/conversation`, `service/nlp`, `service/intent_router`, `service/rdstation`, `client/openai`, `client/rdstation`, `domain/*`, `store/conversation/store.go` (interface)

### 4.4 Diagrama de Dependências

```
[Usuário WhatsApp]
       │ mensagem de texto
       ▼
[Meta Cloud API]
       │ HTTPS POST/GET
       ▼
[API Gateway HTTP API]  ← URL gerada automaticamente com HTTPS nativo
       │ evento proxy
       ▼
[AWS Lambda — Go arm64 / provided.al2023]
  └─ [handler/whatsapp]
       ├─ valida HMAC-SHA256 (X-Hub-Signature-256)
       ├─ extrai from + body
       └─ aciona pipeline:
              │
              ▼
     [service/conversation] ←→ [store/conversation/dynamodb] ←→ [DynamoDB]
              │
              ▼
      [service/nlp] ←→ [client/openai] ←→ [OpenAI API]
              │
              ▼
     [service/intent_router]
              │
              ▼
     [service/rdstation] ←→ [client/rdstation] ←→ [RD Station CRM API]
              │
              ▼
     [service/nlp (formatação)]
              │
              ▼
     [client/whatsapp] ──→ [Meta Cloud API (POST /messages)]
                                       │
                                       ▼
                              [Usuário WhatsApp] ← resposta
```

### 4.5 Estrutura de Diretórios Atualizada

```
synova-rd-workflow/
├── cmd/
│   └── lambda/
│       └── main.go                          (novo — entrypoint Lambda)
├── config/
│   └── config.go                            (alterado — novas vars, sem Postgres)
├── internal/
│   ├── handler/
│   │   └── whatsapp/
│   │       └── handler.go                   (novo — substitui handler/webhook/)
│   ├── client/
│   │   ├── whatsapp/
│   │   │   └── client.go                    (novo)
│   │   ├── openai/                          (inalterado)
│   │   └── rdstation/                       (inalterado)
│   ├── store/
│   │   └── conversation/
│   │       ├── store.go                     (inalterado — interface)
│   │       └── dynamodb.go                  (novo — substitui postgres.go)
│   └── ...                                  (demais pacotes inalterados)
├── terraform/
│   ├── main.tf                              (novo)
│   ├── variables.tf                         (novo)
│   └── outputs.tf                           (novo)
├── docker-compose.dev.yml                   (novo — DynamoDB Local para dev)
├── Makefile                                 (alterado — build-lambda, deploy)
├── .env.example                             (alterado)
└── go.mod                                   (alterado — aws-lambda-go, aws-sdk-go-v2)
```

### 4.6 Interfaces

```go
// client/whatsapp/client.go
type WhatsAppClient interface {
    SendTextMessage(ctx context.Context, to string, text string) error
}

// store/conversation/store.go (inalterado)
type ConversationStore interface {
    GetOrCreateSession(ctx context.Context, phoneNumber string) (domain.Session, error)
    GetRecentMessages(ctx context.Context, sessionID string, limit int) ([]domain.Message, error)
    SaveMessage(ctx context.Context, msg domain.Message) error
}
```

### 4.7 Entrypoint Lambda

```go
// cmd/lambda/main.go
func main() {
    // Inicializado uma vez — reutilizado entre invocações quentes (warm start)
    cfg  := config.Load()
    deps := wire.InitDependencies(cfg)
    h    := whatsapp.NewHandler(deps)

    // aws-lambda-go-api-proxy adapta http.Handler para APIGatewayV2HTTPRequest
    lambda.Start(httpadapter.NewV2(h.Router()).ProxyWithContext)
}
```

---

## 5. Modelo de Dados — DynamoDB

### 5.1 Design da Tabela (Single-Table)

**Nome:** `synova-rd-workflow-conversations`
**Billing mode:** `PAY_PER_REQUEST` — sem capacidade provisionada fixa; paga por operação real; permanece dentro do Free Tier para uso pessoal.

| Atributo | Tipo DynamoDB | Descrição |
|---|---|---|
| `PK` | S (String) | Chave de partição |
| `SK` | S (String) | Chave de ordenação |
| `ttl` | N (Number) | Unix timestamp de expiração (TTL nativo) |

**Padrões de acesso:**

| Padrão | PK | SK | Operação DynamoDB |
|---|---|---|---|
| Criar/obter sessão | `PHONE#<e164>` | `SESSION` | `GetItem` / `PutItem` |
| Salvar mensagem | `PHONE#<e164>` | `MSG#<timestamp_ms>#<uuid>` | `PutItem` |
| Últimas N mensagens | `PHONE#<e164>` | `begins_with("MSG#")`, desc, Limit=N | `Query` |

### 5.2 Itens da Tabela

**Item de sessão:**
```json
{
  "PK":          "PHONE#+5511999999999",
  "SK":          "SESSION",
  "session_id":  "uuid-v4",
  "phone":       "+5511999999999",
  "created_at":  "2026-05-04T10:00:00Z",
  "updated_at":  "2026-05-04T10:00:00Z"
}
```

**Item de mensagem:**
```json
{
  "PK":         "PHONE#+5511999999999",
  "SK":         "MSG#1746352800000#uuid-v4",
  "message_id": "uuid-v4",
  "role":       "user",
  "content":    "Quais negociações abertas temos?",
  "intent":     "get_deals",
  "created_at": "2026-05-04T10:00:00Z",
  "ttl":        1751536800
}
```

### 5.3 Política de TTL

- **Mensagens:** expiram automaticamente após **30 dias** (TTL nativo — gratuito, não consome WCU)
- **Sessões:** não expiram (atualizadas a cada mensagem)
- **Objetivo:** manter uso dentro do Free Tier (25 GB) indefinidamente

---

## 6. Contratos de API / Interfaces

### 6.1 GET /webhook — Verificação do Webhook (Meta → Lambda)

```
GET /webhook?hub.mode=subscribe&hub.verify_token={WHATSAPP_VERIFY_TOKEN}&hub.challenge={string}

Sucesso: hub.mode == "subscribe" AND hub.verify_token == WHATSAPP_VERIFY_TOKEN

Response 200 (text/plain):
  {hub.challenge}

Response 403:
  { "error": "Forbidden" }
```

### 6.2 POST /webhook — Recebimento de Mensagem (Meta → Lambda)

```
POST /webhook
Headers:
  Content-Type:        application/json
  X-Hub-Signature-256: sha256={hmac_hex}

Request body (mensagem de texto):
{
  "object": "whatsapp_business_account",
  "entry": [{
    "id": "1406386851586660",
    "changes": [{
      "value": {
        "messaging_product": "whatsapp",
        "metadata": {
          "display_phone_number": "15556368630",
          "phone_number_id":      "000000000000000"
        },
        "contacts": [{ "profile": { "name": "string" }, "wa_id": "string" }],
        "messages": [{
          "from":      "string — sem + (ex: 5511999999999)",
          "id":        "wamid.XXX",
          "timestamp": "unix_timestamp",
          "type":      "text",
          "text":      { "body": "string" }
        }]
      },
      "field": "messages"
    }]
  }]
}

Response 200 (obrigatório — Meta retenta se não receber 200):
  {}

Casos ignorados (retornam 200 sem processar):
  - type != "text"
  - Payload de status (sem campo "messages")
  - "messages" array vazio
```

**Validação de assinatura:**
```
esperado = "sha256=" + hex(HMAC-SHA256(WHATSAPP_APP_SECRET, raw_body_bytes))
comparar com X-Hub-Signature-256 usando hmac.Equal (tempo constante — previne timing attack)
se diferente → HTTP 401, encerrar
```

### 6.3 Meta Messages API — Envio de Resposta (Lambda → Meta)

```
POST https://graph.facebook.com/v20.0/000000000000000/messages
Headers:
  Authorization: Bearer {WHATSAPP_ACCESS_TOKEN}
  Content-Type:  application/json

Body:
{
  "messaging_product": "whatsapp",
  "to":   "5511999999999",
  "type": "text",
  "text": { "body": "string — até 4096 chars" }
}

Response 200:
{
  "messaging_product": "whatsapp",
  "contacts": [{ "input": "...", "wa_id": "..." }],
  "messages": [{ "id": "wamid.XXX" }]
}

Erros relevantes:
  HTTP 400, error.code 190   → token inválido ou expirado
  HTTP 400, error.code 131030 → número de destino inválido
```

### 6.4 Terraform Output — URL do Webhook

```
webhook_url = "https://{api-id}.execute-api.{region}.amazonaws.com/webhook"
```

Esta URL é registrada no painel Meta for Developers após `terraform apply`.

---

## 7. Fluxos Principais

### 7.1 Fluxo de Verificação do Webhook (uma única vez na configuração)

```
1. Operador executa `terraform apply` → obtém output webhook_url.
2. Operador acessa Meta for Developers → WhatsApp → Configuration → Webhooks.
3. Registra webhook_url + WHATSAPP_VERIFY_TOKEN.
4. Meta envia GET {webhook_url}?hub.mode=subscribe&hub.verify_token=XXX&hub.challenge=YYY.
5. Lambda invocado via API Gateway.
6. handler/whatsapp.VerifyWebhook:
   6a. Verifica hub.mode == "subscribe"
   6b. Verifica hub.verify_token == WHATSAPP_VERIFY_TOKEN
   6c. Retorna HTTP 200 com body = hub.challenge (text/plain)
   6d. Se inválido → HTTP 403
7. Meta confirma webhook ativo. Mensagens reais passam a ser entregues.
```

### 7.2 Fluxo Principal — Mensagem de Texto (caminho feliz)

```
1. Usuário envia mensagem para +1 555 636 8630 no WhatsApp.

2. Meta Cloud API → POST {webhook_url} com payload + X-Hub-Signature-256.

3. API Gateway → invoca Lambda com APIGatewayV2HTTPRequest.

4. handler/whatsapp.HandleMessage:
   4a. Lê raw body bytes (antes de decode) para validação HMAC.
   4b. Valida X-Hub-Signature-256 com HMAC-SHA256(WHATSAPP_APP_SECRET, raw_body).
   4c. Se inválido → HTTP 401, encerra.
   4d. Decode JSON.
   4e. Verifica messages[0].type == "text". Se ausente/diferente → HTTP 200, encerra.
   4f. Extrai from ("5511999999999") → normaliza para E.164 ("+5511999999999").
   4g. Extrai body = messages[0].text.body

5. service/conversation.GetOrCreateSession(ctx, "+5511999999999")
   → DynamoDB GetItem PK="PHONE#+55..." SK="SESSION"
   → Se não existe: PutItem criando sessão

6. service/conversation.GetRecentMessages(ctx, sessionID, 10)
   → DynamoDB Query PK="PHONE#+55..." SK begins_with "MSG#", desc, Limit=10
   → Reverter para ordem cronológica (contexto LLM)

7. service/nlp.ParseIntent(ctx, history, body)
   → OpenAI GPT-4o function calling → Intent{Name, Parameters}

8. service/conversation.SaveMessage(ctx, Message{role:"user", intent:intent.Name, ...})
   → DynamoDB PutItem SK="MSG#<ms>#<uuid>", ttl=now+30d

9. service/intent_router.Route(ctx, intent)
   → service/rdstation executa operação → resultado estruturado

10. service/nlp.FormatResponse(ctx, intent, resultado)
    → resposta em linguagem natural pt-BR (≤ 4096 chars)

11. service/conversation.SaveMessage(ctx, Message{role:"assistant", ...})

12. client/whatsapp.SendTextMessage(ctx, "5511999999999", resposta)
    → POST graph.facebook.com/v20.0/000000000000000/messages

13. handler retorna HTTP 200 (body vazio) → API Gateway → Meta confirma entrega.

14. Usuário recebe resposta no WhatsApp.
```

### 7.3 Fluxo de Exceção — Assinatura Inválida

```
1. POST com X-Hub-Signature-256 incorreto ou ausente.
2. handler → HTTP 401 imediatamente, sem processar.
3. Log: WARN "webhook signature validation failed", request_id
4. Meta descarta o evento (não retenta em respostas 4xx).
```

### 7.4 Fluxo de Exceção — Mensagem Não-Texto

```
1. POST com type != "text" (áudio, imagem, etc.).
2. Log: INFO "unsupported message type: {type}, ignoring"
3. handler → HTTP 200 sem enviar resposta ao usuário.
   (HTTP 200 obrigatório para evitar retentativas da Meta)
```

### 7.5 Fluxo de Exceção — Erro no Pipeline

```
1. Falha em qualquer etapa (OpenAI timeout, RD Station 5xx, DynamoDB erro).
2. Log: ERROR com detalhes + Lambda request ID.
3. client/whatsapp.SendTextMessage envia mensagem de fallback:
   "Desculpe, não consegui processar sua mensagem agora. Tente novamente em instantes."
4. handler → HTTP 200 (evita retentativa da Meta).
```

### 7.6 Fluxo de Exceção — Access Token Expirado

```
1. client/whatsapp recebe HTTP 400 com error.code == 190.
2. Log: ERROR "whatsapp access token expired — rotate WHATSAPP_ACCESS_TOKEN and redeploy"
3. Sem fallback possível (canal de envio quebrado).
4. Operador: gerar novo token → atualizar var na Lambda → redeploy.
```

### 7.7 Edge Cases

| Caso | Comportamento |
|---|---|
| Webhook duplicado (Meta reenvia mesmo wamid) | V2 não implementa deduplicação. Mitigação V3: armazenar wamid no DynamoDB com TTL 5min. |
| Resposta > 4096 chars | `service/nlp` trunca antes de `SendTextMessage`. |
| Payload de status (read/delivery receipts) | Sem campo `messages[]` → HTTP 200 sem processar. |
| DynamoDB throttle (> Free Tier) | Improvável para uso pessoal com `PAY_PER_REQUEST`; se ocorrer, erro → fallback padrão. |
| Cold start Lambda | ≤ 500ms para Go arm64; dentro dos 20s de timeout da Meta. |

---

## 8. Requisitos Não Funcionais

### Performance
- **Latência E2E (P95):** ≤ 15s (dentro do timeout de 20s da Meta)
- **Lambda timeout:** 15s
- **Componentes mais lentos:** OpenAI GPT-4o (~5-7s); RD Station (~1-3s)
- **DynamoDB:** < 5ms por operação (mesma região AWS)
- **Cold start Lambda:** ≤ 500ms (Go arm64)

### Segurança
- **HMAC-SHA256:** validar `X-Hub-Signature-256` em toda requisição POST com `hmac.Equal` (tempo constante — previne timing attack)
- **Segredos:** `WHATSAPP_APP_SECRET`, `WHATSAPP_ACCESS_TOKEN`, `WHATSAPP_VERIFY_TOKEN` — nunca logados, armazenados apenas como env vars da Lambda
- **IAM Least Privilege:** role da Lambda com somente `dynamodb:GetItem`, `dynamodb:PutItem`, `dynamodb:Query` na tabela específica + `logs:CreateLogStream`, `logs:PutLogEvents`
- **HTTPS:** nativo do API Gateway (certificado gerenciado pela AWS) — sem custo
- **Segredos no Terraform:** não commitar `terraform.tfvars`; adicionar ao `.gitignore`

#### Rotação do Access Token
O token temporário expira em ~24h. Para token permanente:
1. Meta for Developers → Business Settings → System Users
2. Criar System User com papel Admin
3. Gerar token permanente associado ao System User
4. Atualizar variável de ambiente na Lambda:
   ```bash
   aws lambda update-function-configuration \
     --function-name synova-rd-workflow \
     --environment "Variables={WHATSAPP_ACCESS_TOKEN=novo_token,...}"
   ```
   Ou via `terraform apply` com `terraform.tfvars` atualizado.

### Disponibilidade
- Lambda: SLA 99.95% (AWS gerenciado)
- DynamoDB: SLA 99.999% (AWS gerenciado)
- API Gateway: SLA 99.95% (AWS gerenciado)
- Meta realiza retentativas de webhook por 7 dias em caso de falha (status != 200)

### Observabilidade
Logs estruturados (JSON) → CloudWatch Logs automaticamente:

```json
{
  "level":       "info",
  "timestamp":   "2026-05-04T10:00:00Z",
  "request_id":  "lambda-request-id",
  "component":   "handler/whatsapp",
  "from":        "+5511999999999",
  "wamid":       "wamid.HBgL...",
  "intent":      "get_deals",
  "latency_ms":  7432
}
```

Nunca logar: `access_token`, `app_secret`, `verify_token`, conteúdo de mensagens em INFO+.

---

## 9. Dependências Externas

### 9.1 AWS Lambda

| Item | Detalhe |
|---|---|
| Free Tier | 1M invocações/mês + 400K GB-s/mês — **permanente** |
| Timeout | 15s |
| Memória | 256 MB |
| Runtime | `provided.al2023` + `arm64` |

### 9.2 AWS API Gateway HTTP API

| Item | Detalhe |
|---|---|
| Free Tier | 1M chamadas/mês — **primeiros 12 meses** |
| Após 12 meses | $1.00/milhão — centavos/mês para uso pessoal |
| HTTPS | Nativo — sem custo adicional |

### 9.3 AWS DynamoDB

| Item | Detalhe |
|---|---|
| Free Tier | 25 GB + 25 WCU + 25 RCU — **permanente** |
| Billing mode | `PAY_PER_REQUEST` — sem throttle, paga pelo uso real |
| TTL | Ativado em coluna `ttl` — expiração grátis |

### 9.4 AWS CloudWatch Logs

| Item | Detalhe |
|---|---|
| Free Tier | 5 GB de ingestão/mês — primeiros 12 meses |
| Retenção | 7 dias (configurar no Terraform para limitar custo) |

### 9.5 Meta Cloud API

| Item | Detalhe |
|---|---|
| Base URL | `https://graph.facebook.com/v20.0` |
| Endpoint envio | `POST /000000000000000/messages` |
| Rate limit | 250 req/s por número |
| Timeout client | 10s |
| Custo | Gratuito para mensagens de resposta (sessão iniciada pelo usuário) |

### 9.6 OpenAI API e RD Station CRM API

Inalterados — ver `sdd-synova-rd-workflow.md` v1.0.0, seção 9.

---

## 10. Estratégia de Testes

### 10.1 Testes Unitários

| ID | Componente | Cenários |
|---|---|---|
| UT-WA-001 | `handler/whatsapp` | GET verification OK; GET token errado → 403; POST assinatura válida; POST assinatura inválida → 401; POST type=text processado; POST type=audio → 200 sem processar; POST sem messages → 200; POST status/receipt → 200 |
| UT-WA-002 | `client/whatsapp` | Envio com sucesso (mock HTTP 200); Token expirado (mock HTTP 400 code 190); Timeout (mock > 10s) |
| UT-DDB-001 | `store/conversation/dynamodb` | GetOrCreateSession nova sessão; GetOrCreateSession sessão existente; GetRecentMessages em ordem cronológica correta; SaveMessage com TTL=now+30d; Erro DynamoDB propagado |

### 10.2 Desenvolvimento Local com DynamoDB Local

```yaml
# docker-compose.dev.yml
services:
  dynamodb-local:
    image: amazon/dynamodb-local:latest
    ports:
      - "8000:8000"
    command: "-jar DynamoDBLocal.jar -sharedDb -inMemory"

  app:
    build: .
    environment:
      DYNAMODB_ENDPOINT: http://dynamodb-local:8000
      AWS_REGION: us-east-1
      AWS_ACCESS_KEY_ID: dummy
      AWS_SECRET_ACCESS_KEY: dummy
      # demais variáveis via .env
    depends_on:
      - dynamodb-local
```

O `config.go` aceita `DYNAMODB_ENDPOINT` para sobrescrever o endpoint AWS (vazio = produção).

### 10.3 Teste E2E

| Cenário | Ação | Esperado |
|---|---|---|
| Consulta de contato | Enviar "Quais contatos temos com sobrenome Silva?" | Lista de contatos no WhatsApp |
| Criar negociação | Enviar "Crie uma negociação Teste V2 para Maria Santos" | Confirmação + negociação no RD Station |
| Intent desconhecida | Enviar "Qual é a previsão do tempo?" | Fallback educado |
| Continuidade | Enviar "E o email dela?" após consulta | Sistema usa histórico do DynamoDB |
| Não-texto | Enviar áudio | Sem resposta |

### 10.4 Critérios de Aceite

- [ ] `terraform apply` conclui sem erros; `webhook_url` no output
- [ ] Webhook verificado com sucesso no painel Meta for Developers
- [ ] Mensagem real respondida no WhatsApp em < 15s
- [ ] Assinatura inválida → HTTP 401 (UT-WA-001)
- [ ] Token expirado → ERROR log com instrução de rotação
- [ ] Payload não-texto → HTTP 200, sem mensagem enviada
- [ ] `go test ./...` passa com todos os testes unitários

---

## 11. Plano de Implementação

### Fase 1 — Backend

| ID | Tarefa | Componente | Complexidade | Dependência |
|---|---|---|---|---|
| WA-001 | Criar `client/whatsapp/client.go` com `SendTextMessage` | client | Baixa | — |
| WA-002 | Criar `handler/whatsapp/handler.go`: GET verification + POST HMAC + pipeline | handler | Média | WA-001 |
| WA-003 | Atualizar `config/config.go`: vars WhatsApp + DynamoDB; remover Postgres + APP_API_KEY | config | Baixa | — |
| WA-004 | Adicionar ao `go.mod`: `aws-lambda-go`, `aws-lambda-go-api-proxy`, `aws-sdk-go-v2/dynamodb` | go.mod | Baixa | — |
| WA-005 | Criar `store/conversation/dynamodb.go` (substituir postgres.go) | store | Média | WA-003, WA-004 |
| WA-006 | Criar `cmd/lambda/main.go` (entrypoint Lambda com httpadapter) | cmd | Baixa | WA-002, WA-003, WA-005 |
| WA-007 | Atualizar `Makefile`: target `build-lambda` (`GOOS=linux GOARCH=arm64`), `zip`, `deploy` | Makefile | Baixa | WA-006 |
| WA-008 | Criar `docker-compose.dev.yml` com DynamoDB Local | dev | Baixa | WA-003 |
| WA-009 | Escrever testes unitários: UT-WA-001, UT-WA-002, UT-DDB-001 | testes | Média | WA-001, WA-002, WA-005 |

### Fase 2 — Infraestrutura Terraform

| ID | Tarefa | Componente | Complexidade | Dependência |
|---|---|---|---|---|
| TF-001 | Criar `terraform/variables.tf` | terraform | Baixa | — |
| TF-002 | `terraform/main.tf`: DynamoDB table (single-table, TTL, PAY_PER_REQUEST) | terraform | Baixa | TF-001 |
| TF-003 | `terraform/main.tf`: IAM role + policy mínima (DynamoDB + CloudWatch Logs) | terraform | Baixa | TF-002 |
| TF-004 | `terraform/main.tf`: Lambda function (provided.al2023, arm64, timeout=15, memory=256) | terraform | Média | TF-003 |
| TF-005 | `terraform/main.tf`: API Gateway HTTP API + integração Lambda proxy + rotas GET/POST /webhook + stage $default | terraform | Média | TF-004 |
| TF-006 | `terraform/main.tf`: CloudWatch Log Group (retention=7d) + Lambda permission para API Gateway | terraform | Baixa | TF-004, TF-005 |
| TF-007 | Criar `terraform/outputs.tf`: webhook_url, lambda_arn, dynamodb_table_name | terraform | Baixa | TF-005 |

### Fase 3 — Deploy e Configuração

| ID | Tarefa | Complexidade | Dependência |
|---|---|---|---|
| DP-001 | `make build-lambda` → gera `function.zip` com binário arm64 | Baixa | Fase 1 |
| DP-002 | `terraform apply -var-file=terraform.tfvars` | Baixa | Fase 2, DP-001 |
| DP-003 | Registrar `webhook_url` + `WHATSAPP_VERIFY_TOKEN` no painel Meta for Developers | Baixa | DP-002 |
| DP-004 | Verificar webhook no painel Meta ("Verify and Save") | Baixa | DP-003 |
| DP-005 | Teste E2E: enviar mensagem real e validar resposta no WhatsApp | Média | DP-004 |

### Riscos Técnicos

| ID | Risco | Probabilidade | Mitigação |
|---|---|---|---|
| RT-001 | Access token temporário expira durante testes | Alta | Gerar System User Token antes do deploy definitivo (ver seção 8) |
| RT-002 | Cold start Lambda > 20s de timeout da Meta | Muito Baixa | Go arm64 cold start ≤ 500ms; pipeline ≤ 14s; total < 15s |
| RT-003 | DynamoDB throttle | Muito Baixa | `PAY_PER_REQUEST` não tem throttle de capacidade provisionada |
| RT-004 | API Gateway Free Tier expira após 12 meses | Baixa | Custo residual: ~$0.01/mês para uso pessoal |
| RT-005 | Segredos expostos no Terraform state | Média | Não commitar `terraform.tfvars`; considerar AWS SSM Parameter Store para V3 |

---

## 12. Variáveis de Ambiente

### `.env.example` atualizado

```env
# Aplicação (usado apenas em dev local)
APP_PORT=8080

# WhatsApp Cloud API (Meta)
WHATSAPP_ACCESS_TOKEN=          # Bearer token — obrigatório
WHATSAPP_PHONE_NUMBER_ID=000000000000000
WHATSAPP_VERIFY_TOKEN=          # String aleatória ≥ 32 chars
WHATSAPP_APP_SECRET=            # App Secret: Meta for Developers → App Settings → Basic

# OpenAI
OPENAI_API_KEY=

# RD Station CRM
RDSTATION_CLIENT_ID=
RDSTATION_CLIENT_SECRET=
RDSTATION_REFRESH_TOKEN=

# DynamoDB
DYNAMODB_TABLE_NAME=synova-rd-workflow-conversations
AWS_REGION=us-east-1
DYNAMODB_ENDPOINT=              # vazio em produção; http://localhost:8000 em dev local

# Observabilidade
LOG_LEVEL=info
```

**Variáveis removidas:** `APP_API_KEY`, `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`

### `terraform.tfvars` (não commitado — `.gitignore`)

```hcl
aws_region               = "us-east-1"
whatsapp_access_token    = "..."
whatsapp_phone_number_id = "000000000000000"
whatsapp_verify_token    = "..."
whatsapp_app_secret      = "..."
openai_api_key           = "..."
rdstation_client_id      = "..."
rdstation_client_secret  = "..."
rdstation_refresh_token  = "..."
```

---

## 13. Infraestrutura Terraform — Esboço Completo

```hcl
# terraform/main.tf

provider "aws" {
  region = var.aws_region
}

# ─── DynamoDB ───────────────────────────────────────────────────────────────

resource "aws_dynamodb_table" "conversations" {
  name         = "synova-rd-workflow-conversations"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "PK"
  range_key    = "SK"

  attribute { name = "PK" type = "S" }
  attribute { name = "SK" type = "S" }

  ttl {
    attribute_name = "ttl"
    enabled        = true
  }
}

# ─── IAM ────────────────────────────────────────────────────────────────────

resource "aws_iam_role" "lambda" {
  name = "synova-rd-workflow-lambda-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "lambda.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy" "lambda" {
  name = "synova-rd-workflow-lambda-policy"
  role = aws_iam_role.lambda.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Action   = ["dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:Query"]
        Resource = aws_dynamodb_table.conversations.arn
      },
      {
        Effect   = "Allow"
        Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
        Resource = "${aws_cloudwatch_log_group.lambda.arn}:*"
      }
    ]
  })
}

# ─── CloudWatch ─────────────────────────────────────────────────────────────

resource "aws_cloudwatch_log_group" "lambda" {
  name              = "/aws/lambda/synova-rd-workflow"
  retention_in_days = 7
}

# ─── Lambda ─────────────────────────────────────────────────────────────────

resource "aws_lambda_function" "workflow" {
  function_name = "synova-rd-workflow"
  filename      = "${path.module}/../function.zip"  # gerado por `make build-lambda`
  handler       = "bootstrap"
  runtime       = "provided.al2023"
  architectures = ["arm64"]
  timeout       = 15
  memory_size   = 256
  role          = aws_iam_role.lambda.arn

  environment {
    variables = {
      WHATSAPP_ACCESS_TOKEN    = var.whatsapp_access_token
      WHATSAPP_PHONE_NUMBER_ID = var.whatsapp_phone_number_id
      WHATSAPP_VERIFY_TOKEN    = var.whatsapp_verify_token
      WHATSAPP_APP_SECRET      = var.whatsapp_app_secret
      OPENAI_API_KEY           = var.openai_api_key
      RDSTATION_CLIENT_ID      = var.rdstation_client_id
      RDSTATION_CLIENT_SECRET  = var.rdstation_client_secret
      RDSTATION_REFRESH_TOKEN  = var.rdstation_refresh_token
      DYNAMODB_TABLE_NAME      = aws_dynamodb_table.conversations.name
      AWS_REGION_APP           = var.aws_region
      LOG_LEVEL                = "info"
    }
  }

  depends_on = [aws_cloudwatch_log_group.lambda]
}

# ─── API Gateway HTTP API ────────────────────────────────────────────────────

resource "aws_apigatewayv2_api" "webhook" {
  name          = "synova-rd-workflow-webhook"
  protocol_type = "HTTP"
}

resource "aws_apigatewayv2_integration" "lambda" {
  api_id                 = aws_apigatewayv2_api.webhook.id
  integration_type       = "AWS_PROXY"
  integration_uri        = aws_lambda_function.workflow.invoke_arn
  payload_format_version = "2.0"
}

resource "aws_apigatewayv2_route" "get_webhook" {
  api_id    = aws_apigatewayv2_api.webhook.id
  route_key = "GET /webhook"
  target    = "integrations/${aws_apigatewayv2_integration.lambda.id}"
}

resource "aws_apigatewayv2_route" "post_webhook" {
  api_id    = aws_apigatewayv2_api.webhook.id
  route_key = "POST /webhook"
  target    = "integrations/${aws_apigatewayv2_integration.lambda.id}"
}

resource "aws_apigatewayv2_stage" "default" {
  api_id      = aws_apigatewayv2_api.webhook.id
  name        = "$default"
  auto_deploy = true
}

resource "aws_lambda_permission" "api_gw" {
  statement_id  = "AllowAPIGatewayInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.workflow.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.webhook.execution_arn}/*/*"
}
```

---

## 14. Resumo do Free Tier

| Serviço | Free Tier | Tipo | Uso estimado/mês |
|---|---|---|---|
| **Lambda** | 1M invocações + 400K GB-s | **Permanente** | < 1.000 invocações |
| **DynamoDB** | 25 GB + 25 WCU + 25 RCU | **Permanente** | < 1 GB, < 1 WCU, < 1 RCU |
| **API Gateway HTTP API** | 1M chamadas | 12 meses | < 1.000 chamadas |
| **CloudWatch Logs** | 5 GB ingestão | 12 meses | < 100 MB |
| **Total estimado após 12 meses** | API GW: ~$0.001/mês | — | Essencialmente gratuito |

---

## 15. Glossário

| Termo | Definição |
|---|---|
| Meta Cloud API | API oficial da Meta para integração com WhatsApp Business |
| Phone Number ID | ID interno Meta do número (`000000000000000`) |
| WhatsApp Business Account ID | ID da conta Meta (`1406386851586660`) |
| wamid | WhatsApp Message ID — ID único de cada mensagem |
| Verify Token | String secreta para verificação inicial do webhook |
| App Secret | Segredo da app Meta para HMAC-SHA256 |
| Access Token | Bearer token para a Graph API |
| System User Token | Access token permanente via Business Manager |
| HMAC-SHA256 | Algoritmo de autenticação de mensagem dos webhooks |
| E.164 | Formato de telefone com `+` e código de país |
| Single-Table Design | Padrão DynamoDB com múltiplas entidades em uma tabela usando prefixos PK/SK |
| PAY_PER_REQUEST | Billing DynamoDB por operação real — sem capacidade provisionada fixa |
| TTL (DynamoDB) | Expiração automática de itens sem consumir WCU |
| provided.al2023 | Runtime Lambda customizado para Go 1.18+ (Amazon Linux 2023) |
| arm64 / Graviton2 | Arquitetura AWS (~20% mais eficiente que x86_64) |
| API Gateway HTTP API | Versão leve do API Gateway (~70% mais barata que REST API) |
| Cold Start | Inicialização do container Lambda após idle (≤ 500ms para Go arm64) |
| httpadapter | Pacote `aws-lambda-go-api-proxy` que adapta `http.Handler` para eventos Lambda |

---

## 16. Histórico de Revisões

| Data | Versão | Autor | Descrição |
|---|---|---|---|
| 04/05/2026 | 1.0.0 | GitHub Copilot | Versão inicial — integração Meta Cloud API com EC2 + PostgreSQL + Nginx |
| 04/05/2026 | 2.0.0 | GitHub Copilot | Substituição de EC2/PostgreSQL/Nginx por Lambda/DynamoDB/API Gateway — 100% Free Tier permanente |

---

Gerado por GitHub Copilot | 04/05/2026
