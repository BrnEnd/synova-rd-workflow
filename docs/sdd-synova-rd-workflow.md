Documento: SDD — synova-rd-workflow
Versão: 1.0.0
Data: 26/04/2026
Status: Rascunho
Gerado por: MarfPlanner

---

# SDD — synova-rd-workflow

## 1. Visão Geral

**Objetivo:** Este documento descreve a arquitetura, os contratos de API, os fluxos e o plano de implementação do sistema synova-rd-workflow — um agente conversacional que permite ao usuário consultar e manipular dados do RD Station CRM por meio de mensagens em linguagem natural enviadas via WhatsApp.

**Escopo — dentro:**
- Recebimento de mensagens via interface mockada de WhatsApp (endpoint REST local)
- Interpretação de linguagem natural com LLM (OpenAI GPT-4o com function calling)
- Consulta, criação, atualização e movimentação de cards no RD Station CRM
- Retorno de resposta em linguagem natural
- Armazenamento de histórico de conversa por sessão para manutenção de contexto
- Execução local via Docker Compose

**Escopo — fora:**
- Integração com WhatsApp real (produção) — será avaliada em versão futura
- Interface frontend ou painel web
- Autenticação multi-usuário — credenciais do RD Station são fixas por ambiente
- Deploy em nuvem (AWS, GCP, etc.)
- Integração com RD Station Marketing (apenas CRM)
- Suporte a múltiplas contas simultâneas de RD Station

**Público-alvo:** Desenvolvedor responsável pela implementação do sistema.

**Agente de implementação:** O backend em Go deve ser implementado diretamente (não há agente especializado em Go neste workspace; os padrões descritos neste documento orientam a implementação).

---

## 2. Contexto e Motivação

O RD Station CRM possui interface web funcional, porém o acesso rápido a dados durante atendimentos, reuniões ou deslocamento exige que o usuário navegue até a plataforma, faça login e localize a informação manualmente. O objetivo do synova-rd-workflow é eliminar esse atrito: o usuário envia uma mensagem em linguagem natural e recebe dados ou confirma ações sem abrir o CRM.

O WhatsApp é o canal de preferência por já estar aberto durante a maior parte do dia de trabalho do usuário.

**Restrições atuais:**
- A integração com WhatsApp está em avaliação (Twilio vs Meta Business API direta). A V1 usa um endpoint REST local que simula o canal, com contrato alinhado ao formato que será usado com o WhatsApp real.
- Toda a execução é local via Docker. Sem preocupação com escalabilidade nesta versão.
- Um único usuário e uma única conta do RD Station CRM são atendidos.

---

## 3. Decisões de Arquitetura

### DA-001 — Go como linguagem principal
**Decisão:** Implementar toda a API em Go 1.22+.
**Justificativa:** Performance nativa, tipagem forte, compilação estática, excelente suporte a HTTP concorrente e binário único sem dependência de runtime. Stack definida pelo usuário.
**Alternativas rejeitadas:** Node.js e Python — rejeitados por preferência explícita do usuário.

### DA-002 — Framework HTTP: Gin
**Decisão:** Usar `github.com/gin-gonic/gin` como framework HTTP.
**Justificativa:** Framework maduro, com maior ecossistema e documentação do que alternativas. Middleware de logging, recovery e binding de JSON já incluídos.
**Alternativas rejeitadas:**
- Fiber: menor maturidade de middleware para este caso
- `net/http` puro: verbosidade desnecessária para o escopo atual

### DA-003 — Arquitetura em camadas (handler → service → client)
**Decisão:** Organizar o código em camadas: `handler` → `service` → `client`, com tipos de domínio centralizados em `domain/`.
**Justificativa:** Separação de responsabilidades, facilidade de teste unitário com mocks de interface, e permissão para substituir o client de WhatsApp no futuro sem alterar a lógica de negócio.
**Alternativas rejeitadas:** Estrutura flat — rejeitada por dificultar a substituição do mock e por misturar concerns de infraestrutura com regras de negócio.

### DA-004 — PostgreSQL para histórico de conversa
**Decisão:** Usar PostgreSQL para armazenar sessões e histórico de mensagens.
**Justificativa:** O LLM precisa de contexto das últimas N mensagens para interpretar corretamente referências como "mude o estágio dele" (referindo-se a uma negociação mencionada anteriormente). PostgreSQL é confiável, disponível via Docker e adequado para o volume esperado.
**Alternativas rejeitadas:**
- In-memory: perda de contexto ao reiniciar o serviço
- Redis: dependência adicional sem benefício claro neste escopo

### DA-005 — OpenAI GPT-4o para NLP
**Decisão:** Usar a API da OpenAI (modelo GPT-4o) com function calling para conversão de linguagem natural em intent estruturada.
**Justificativa:** O function calling da OpenAI retorna JSON estruturado com nome de intent e parâmetros sem necessidade de parsing adicional. Qualidade comprovada para português. A abstração via interface permite trocar o provedor no futuro.
**Alternativas rejeitadas:** Modelos locais (Ollama, etc.) — rejeitados por complexidade de setup e qualidade inferior para português.

### DA-006 — Mock do WhatsApp via endpoint REST
**Decisão:** Simular o recebimento de mensagens do WhatsApp com um endpoint `POST /mock/message` que aceita exatamente o mesmo contrato que será usado com o WhatsApp real.
**Justificativa:** Permite desenvolver e testar o fluxo completo sem conta de WhatsApp Business ativa. A migração para o WhatsApp real exige apenas adicionar o handler de webhook oficial sem alterar nada nos serviços internos.
**Alternativas rejeitadas:** CLI interativo — rejeitado por não simular o formato real de mensagem e por dificultar testes automatizados.

---

## 4. Componentes e Responsabilidades

### 4.1 Lista de Componentes

| Componente | Responsabilidade |
|---|---|
| `handler/webhook` | Receber mensagem (mock ou real), validar API key e payload, acionar o pipeline de processamento |
| `service/conversation` | Gerenciar sessão por número de telefone, recuperar histórico, persistir mensagens |
| `service/nlp` | Chamar a OpenAI com histórico + mensagem atual, retornar intent estruturada; formatar resultado em linguagem natural |
| `service/intent_router` | Mapear intent recebida para o use case correto em service/rdstation |
| `service/rdstation` | Orquestrar operações no RD Station: consulta, criação, atualização e movimentação de cards |
| `client/openai` | HTTP client para a API da OpenAI (chat completions com function calling) |
| `client/rdstation` | HTTP client para a API do RD Station CRM |
| `store/conversation` | CRUD de sessões e mensagens no PostgreSQL |
| `domain/` | Tipos de domínio compartilhados: Intent, Message, Session, Contact, Deal, Stage |
| `config/` | Leitura e validação de variáveis de ambiente |

### 4.2 Diagrama de Dependências

```
[WhatsApp / Mock HTTP]
        |
        v
[handler/webhook]
        |
        v
[service/conversation] <---> [store/conversation] <---> [PostgreSQL]
        |
        v
[service/nlp] <---> [client/openai] <---> [OpenAI API]
        |
        v
[service/intent_router]
        |
        v
[service/rdstation] <---> [client/rdstation] <---> [RD Station CRM API]
        |
        v
[service/nlp (formatação de resposta)]
        |
        v
[handler/webhook (retorno ao usuário)]
```

### 4.3 Estrutura de Diretórios

```
synova-rd-workflow/
├── cmd/
│   └── api/
│       └── main.go
├── internal/
│   ├── handler/
│   │   └── webhook/
│   │       └── handler.go
│   ├── service/
│   │   ├── conversation/
│   │   │   └── service.go
│   │   ├── nlp/
│   │   │   └── service.go
│   │   ├── intent_router/
│   │   │   └── router.go
│   │   └── rdstation/
│   │       └── service.go
│   ├── client/
│   │   ├── openai/
│   │   │   └── client.go
│   │   └── rdstation/
│   │       └── client.go
│   ├── store/
│   │   └── conversation/
│   │       └── store.go
│   └── domain/
│       ├── intent.go
│       ├── message.go
│       ├── contact.go
│       └── deal.go
├── config/
│   └── config.go
├── migrations/
│   └── 001_initial.sql
├── docker-compose.yml
├── Dockerfile
├── .env.example
└── go.mod
```

### 4.4 Interfaces Principais

```go
// NLPService — contrato do serviço de NLP
type NLPService interface {
    ParseIntent(ctx context.Context, history []Message, input string) (Intent, error)
    FormatResponse(ctx context.Context, intent Intent, result interface{}) (string, error)
}

// RDStationService — contrato do serviço de CRM
type RDStationService interface {
    GetContacts(ctx context.Context, params GetContactsParams) ([]Contact, error)
    CreateContact(ctx context.Context, params CreateContactParams) (Contact, error)
    GetDeals(ctx context.Context, params GetDealsParams) ([]Deal, error)
    CreateDeal(ctx context.Context, params CreateDealParams) (Deal, error)
    UpdateDeal(ctx context.Context, dealID string, params UpdateDealParams) (Deal, error)
    MoveDealStage(ctx context.Context, dealID string, targetStageName string) (Deal, error)
}

// ConversationStore — contrato do repositório de conversa
type ConversationStore interface {
    GetOrCreateSession(ctx context.Context, phoneNumber string) (Session, error)
    GetRecentMessages(ctx context.Context, sessionID string, limit int) ([]Message, error)
    SaveMessage(ctx context.Context, msg Message) error
}
```

---

## 5. Modelo de Dados

### 5.1 Tabela: sessions

| Coluna | Tipo | Restrição | Descrição |
|---|---|---|---|
| id | UUID | PK, DEFAULT gen_random_uuid() | Identificador da sessão |
| phone_number | VARCHAR(20) | NOT NULL, UNIQUE | Número do WhatsApp (formato E.164: +5511999999999) |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Data de criação |
| updated_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Última atualização |

### 5.2 Tabela: messages

| Coluna | Tipo | Restrição | Descrição |
|---|---|---|---|
| id | UUID | PK, DEFAULT gen_random_uuid() | Identificador da mensagem |
| session_id | UUID | NOT NULL, FK → sessions.id ON DELETE CASCADE | Sessão à qual pertence |
| role | VARCHAR(10) | NOT NULL, CHECK (role IN ('user','assistant')) | Origem da mensagem |
| content | TEXT | NOT NULL | Texto da mensagem |
| intent | VARCHAR(50) | NULL | Intent identificada (preenchido apenas quando role = 'user') |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Data de criação |

**Índices:**
- `idx_messages_session_id` em `messages(session_id)` — cobre a busca de histórico por sessão
- `idx_sessions_phone_number` em `sessions(phone_number)` — cobre o lookup de sessão por telefone

### 5.3 Tipos de Domínio em Go

```go
// domain/intent.go
type Intent struct {
    Name       IntentName
    Parameters map[string]string
    RawText    string
}

type IntentName string

const (
    IntentGetContacts   IntentName = "get_contacts"
    IntentGetDeals      IntentName = "get_deals"
    IntentCreateContact IntentName = "create_contact"
    IntentCreateDeal    IntentName = "create_deal"
    IntentUpdateDeal    IntentName = "update_deal"
    IntentMoveDealStage IntentName = "move_deal_stage"
    IntentUnknown       IntentName = "unknown"
)

// domain/message.go
type Session struct {
    ID          string
    PhoneNumber string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type Message struct {
    ID        string
    SessionID string
    Role      string // "user" | "assistant"
    Content   string
    Intent    string
    CreatedAt time.Time
}

// domain/contact.go
type Contact struct {
    ID    string
    Name  string
    Email string
    Phone string
}

// domain/deal.go
type Deal struct {
    ID        string
    Name      string
    Stage     Stage
    Contacts  []Contact
    CreatedAt time.Time
    UpdatedAt time.Time
}

type Stage struct {
    ID   string
    Name string
}
```

**Parâmetros por intent:**

| Intent | Parâmetros |
|---|---|
| `get_contacts` | `name?`, `email?`, `phone?` |
| `get_deals` | `name?`, `stage?`, `status?` (open/won/lost) |
| `create_contact` | `name*`, `email?`, `phone?`, `company?` |
| `create_deal` | `name*`, `contact_name?`, `stage?` |
| `update_deal` | `deal_name?`, `field*`, `value*` |
| `move_deal_stage` | `deal_name?`, `target_stage*` |

`*` obrigatório | `?` opcional — se campo obrigatório ausente, o LLM deve solicitar ao usuário antes de executar.

---

## 6. Contratos de API

### 6.1 POST /mock/message — Simular mensagem do WhatsApp

**Descrição:** Endpoint que simula o recebimento de uma mensagem do WhatsApp. Em produção, será substituído pelo webhook oficial (Twilio ou Meta).

```
POST /mock/message
Headers:
  Content-Type: application/json
  X-Api-Key: <valor de APP_API_KEY no .env>

Request body:
{
  "from":    "string — número de telefone em formato E.164 (ex: +5511999999999)",
  "message": "string — texto da mensagem em linguagem natural (máx: 4096 chars)"
}

Response 200:
{
  "reply":      "string — resposta em linguagem natural",
  "intent":     "string — intent identificada pelo NLP",
  "session_id": "string (UUID) — identificador da sessão"
}

Response 400:
{
  "error": "string — descrição do problema de validação"
}

Response 401:
{
  "error": "Unauthorized"
}

Response 500:
{
  "error": "string — mensagem de erro interno (sem stack trace)"
}
```

**Exemplos de mensagem (campo `message`):**
- `"Quais contatos temos com o sobrenome Silva?"`
- `"Mostre as negociações abertas no estágio Proposta"`
- `"Crie um contato para Maria Santos, email: maria@empresa.com"`
- `"Mova a negociação Projeto Alpha para o estágio Fechado Ganho"`
- `"Crie uma negociação chamada Expansão 2026 para o contato João da Silva"`

### 6.2 GET /health — Health check

```
GET /health

Response 200:
{
  "status":  "ok",
  "version": "string — versão do binário"
}
```

### 6.3 Contrato interno — Intent (NLP → Router)

O `service/nlp` retorna o seguinte tipo após chamar a OpenAI com function calling:

```json
{
  "intent": "get_contacts",
  "parameters": {
    "name": "Silva"
  }
}
```

A OpenAI é instruída (via system prompt) a retornar sempre um JSON com os campos `intent` e `parameters`. Se não conseguir identificar uma intent válida, retorna `"intent": "unknown"`.

---

## 7. Fluxos Principais

### 7.1 Fluxo Principal — Consulta de contatos

```
1. Usuário envia POST /mock/message:
   { "from": "+5511999999999", "message": "Quais contatos temos com sobrenome Silva?" }

2. handler/webhook valida X-Api-Key e estrutura do payload.

3. service/conversation busca a sessão pelo número de telefone.
   3a. Se não existe: cria nova sessão e persiste.

4. service/conversation recupera as últimas 10 mensagens da sessão (histórico de contexto).

5. service/nlp monta o prompt com system instructions + histórico + mensagem atual.
   Chama POST /chat/completions na OpenAI com function calling.

6. OpenAI retorna:
   { "intent": "get_contacts", "parameters": { "name": "Silva" } }

7. service/nlp persiste a mensagem do usuário com a intent identificada.

8. service/intent_router recebe a Intent e aciona service/rdstation.GetContacts(name="Silva").

9. client/rdstation chama GET /contacts?name=Silva na API do RD Station.

10. RD Station retorna a lista de contatos correspondentes.

11. service/nlp formata a lista em linguagem natural via segunda chamada à OpenAI.
    Exemplo de saída: "Encontrei 3 contatos com sobrenome Silva: João Silva, Maria Silva e Pedro Silva."

12. service/conversation persiste a resposta do assistente.

13. handler/webhook retorna Response 200 com o campo "reply" preenchido.
```

### 7.2 Fluxo Alternativo — Intent desconhecida

```
(acontece no passo 6 do fluxo principal)

6a. OpenAI retorna: { "intent": "unknown" }
6b. service/nlp gera resposta de fallback:
    "Não entendi o que você precisa. Posso consultar contatos, negociações, criar
     contatos ou mover cards no seu RD Station. Como posso ajudar?"
6c. service/conversation persiste a mensagem do usuário e a resposta do assistente.
6d. handler/webhook retorna Response 200 com a mensagem de fallback.
```

### 7.3 Fluxo Alternativo — Erro na API do RD Station

```
(acontece no passo 9 do fluxo principal)

9a. client/rdstation recebe status 4xx ou 5xx.
9b. service/rdstation retorna erro tipado (RDStationError com código e mensagem).
9c. service/nlp formata mensagem amigável:
    "Tive um problema ao acessar o RD Station (código 500). Tente novamente em instantes."
9d. handler/webhook retorna Response 200 com a mensagem de erro.

Nota: o endpoint /mock/message sempre retorna HTTP 200 ao usuário —
erros são comunicados em linguagem natural no campo "reply".
```

### 7.4 Fluxo — Criação de contato

```
1. Usuário envia: "Crie um contato para João da Silva, email joao@empresa.com"

2. handler/webhook valida e aciona o pipeline.

3. service/nlp retorna:
   { "intent": "create_contact", "parameters": { "name": "João da Silva", "email": "joao@empresa.com" } }

4. service/intent_router aciona service/rdstation.CreateContact(name="João da Silva", email="joao@empresa.com").

5. client/rdstation chama POST /contacts no RD Station com o payload.

6. RD Station retorna o contato criado com ID.

7. service/nlp formata a resposta:
   "Contato João da Silva criado com sucesso no RD Station."

8. Resposta retornada ao usuário.
```

### 7.5 Fluxo — Movimentação de card

```
1. Usuário envia: "Mova a negociação Projeto Alpha para Fechado Ganho"

2. service/nlp retorna:
   { "intent": "move_deal_stage", "parameters": { "deal_name": "Projeto Alpha", "target_stage": "Fechado Ganho" } }

3. service/rdstation busca a negociação por nome via GET /deals?name=Projeto Alpha.

4. Se múltiplas negociações encontradas: retornar lista ao usuário solicitando especificação.
   Se nenhuma encontrada: retornar mensagem "Não encontrei nenhuma negociação com esse nome."

5. service/rdstation busca os estágios via GET /deal_stages para localizar o ID do "Fechado Ganho".
   Se estágio não encontrado: listar os estágios disponíveis e solicitar que o usuário escolha.

6. client/rdstation chama PUT /deals/{id} com o novo deal_stage_id.

7. Resposta confirmando:
   "A negociação Projeto Alpha foi movida para o estágio Fechado Ganho."
```

### 7.6 Edge Cases identificados

| Situação | Comportamento esperado |
|---|---|
| Múltiplos contatos/negociações com mesmo nome | Retornar lista com numeração e solicitar especificação ao usuário |
| Estágio informado não encontrado | Listar estágios disponíveis e solicitar que o usuário escolha |
| Campo obrigatório ausente (ex: name em create_contact) | LLM solicita o dado antes de executar a ação |
| Mensagem vazia ou com apenas espaços | Retornar erro 400 antes de acionar o pipeline |
| Histórico com mais de 10 mensagens | Enviar apenas as 10 mais recentes ao LLM (janela deslizante) |

---

## 8. Requisitos Não Funcionais

### 8.1 Performance
- Latência esperada por requisição (p50): < 5s (inclui latência OpenAI + RD Station)
- Latência máxima tolerada (p99): < 10s
- Timeout para chamadas à OpenAI: 15s
- Timeout para chamadas ao RD Station: 10s
- Execução local — sem SLA de disponibilidade formal nesta versão

### 8.2 Segurança
- Autenticação do endpoint mock via header `X-Api-Key` (valor configurado em `APP_API_KEY` no `.env`)
- Todas as credenciais (RD Station, OpenAI, API Key da app) armazenadas exclusivamente em variáveis de ambiente — nunca em código ou em arquivos versionados
- Validação de entrada: campo `from` deve corresponder ao padrão E.164 (`^\+\d{10,15}$`); `message` não pode ser vazio nem exceder 4096 caracteres
- Logs não devem expor tokens, chaves de API ou dados pessoais (PII) de contatos
- `.env` listado no `.gitignore`

### 8.3 Observabilidade
- Logging estruturado em JSON em todos os serviços
- Campos obrigatórios por log entry: `timestamp`, `level`, `service`, `message`
- Campos contextuais quando disponíveis: `session_id`, `intent`, `duration_ms`, `status_code`
- Log de todas as chamadas externas (OpenAI, RD Station) com status code e duração em ms
- Nível de log configurável via variável de ambiente `LOG_LEVEL` (DEBUG, INFO, WARN, ERROR)

### 8.4 Configuração (12-factor)
- Toda configuração via variáveis de ambiente
- Arquivo `.env.example` com todas as variáveis documentadas (obrigatórias e opcionais)
- A aplicação deve falhar imediatamente no startup se variáveis obrigatórias estiverem ausentes

**Variáveis de ambiente obrigatórias:**

| Variável | Descrição |
|---|---|
| `APP_API_KEY` | Chave de autenticação do endpoint mock |
| `DATABASE_URL` | String de conexão PostgreSQL (ex: `postgres://user:pass@localhost:5432/synova_rd_workflow`) |
| `OPENAI_API_KEY` | Chave da API da OpenAI |
| `RDSTATION_API_KEY` | Token da API do RD Station CRM |

**Variáveis opcionais:**

| Variável | Padrão | Descrição |
|---|---|---|
| `PORT` | `8080` | Porta em que a API escuta |
| `LOG_LEVEL` | `INFO` | Nível de log |
| `NLP_CONTEXT_WINDOW` | `10` | Número de mensagens enviadas ao LLM como contexto |
| `OPENAI_MODEL` | `gpt-4o` | Modelo OpenAI a usar |

---

## 9. Dependências Externas

| Dependência | Tipo | Uso |
|---|---|---|
| OpenAI API (GPT-4o) | REST — terceiro cobrado por token | Interpretação de linguagem natural e formatação de respostas |
| RD Station CRM API v1 | REST — terceiro (incluso no plano CRM) | Consulta, criação e atualização de dados do CRM |
| PostgreSQL 16 | Banco de dados relacional (Docker local) | Sessões e histórico de mensagens |

### 9.1 OpenAI API
- Autenticação: Bearer token (`OPENAI_API_KEY`)
- Base URL: `https://api.openai.com/v1`
- Endpoint utilizado: `POST /chat/completions` com `tools` (function calling)
- Estratégia de fallback: se a chamada falhar após 2 tentativas com intervalo de 2s, retornar mensagem genérica de erro ao usuário sem expor detalhes técnicos
- Documento de integração: não gerado separadamente (escopo simples — um único endpoint)

### 9.2 RD Station CRM API
- Autenticação: Bearer token (`RDSTATION_API_KEY`)
- Base URL: `https://crm.rdstation.com/api/v1`
- Documento de integração detalhado: [integracao-rdstation.md](integracao-rdstation.md)

### 9.3 PostgreSQL
- Versão: 16
- Conexão via `DATABASE_URL`
- Migrations versionadas em `/migrations/`
- Driver Go: `github.com/jackc/pgx/v5`

---

## 10. Estratégia de Testes

### 10.1 Testes Unitários (cobertura mínima: 80% nas camadas service/)
- `service/nlp` — mock de `client/openai` via interface; verificar mapeamento correto de intents para cada tipo; verificar fallback para intent `unknown`
- `service/rdstation` — mock de `client/rdstation` via interface; verificar tratamento de erros 4xx e 5xx; verificar busca por nome com múltiplos resultados
- `service/intent_router` — verificar que cada intent mapeia para o handler correto; verificar que intent `unknown` não gera chamada ao RD Station
- `service/conversation` — mock do `store/conversation`; verificar criação de sessão nova; verificar janela deslizante de 10 mensagens

### 10.2 Testes de Integração
- Fluxo completo via `POST /mock/message` usando `httptest.NewServer` com mocks dos servidores da OpenAI e do RD Station
- Verificar persistência de sessão e mensagens no PostgreSQL de teste (container Docker separado via `testcontainers-go`)

### 10.3 Cenários Críticos

| Cenário | Resultado esperado |
|---|---|
| Intent desconhecida | Resposta de fallback clara sem erro 500 |
| RD Station retorna 429 | Retry após 60s ou mensagem informando indisponibilidade temporária |
| OpenAI retorna 500 | Erro tratado, usuário informado com mensagem genérica |
| Sessão nova (primeiro contato) | Sessão criada automaticamente, conversa iniciada normalmente |
| Histórico com > 10 mensagens | Apenas as 10 mais recentes enviadas ao LLM |
| Criação de contato sem campo `name` | LLM solicita o dado antes de acionar o RD Station |
| Múltiplas negociações com mesmo nome na movimentação | Lista de opções retornada ao usuário |
| Payload sem campo `from` | Retorno 400 com mensagem de validação |

### 10.4 Dados de Teste
- Conta real do RD Station CRM com dados de sandbox (contatos e negociações de teste)
- Chave de API da OpenAI de desenvolvimento (mesma conta, custo controlado)
- Banco PostgreSQL local via Docker

---

## 11. Plano de Implementação

| Fase | ID | Componente | Complexidade |
|---|---|---|---|
| 1 — Fundação | F-01 | Estrutura de diretórios, `go.mod`, `.env.example` | Baixa |
| 1 — Fundação | F-02 | `config/config.go` (leitura e validação de env vars) | Baixa |
| 1 — Fundação | F-03 | `domain/` (todos os tipos de domínio) | Baixa |
| 1 — Fundação | F-04 | `Dockerfile` e `docker-compose.yml` | Baixa |
| 2 — Dados | D-01 | `migrations/001_initial.sql` (sessions + messages) | Baixa |
| 2 — Dados | D-02 | `store/conversation` (interface + implementação PostgreSQL) | Baixa |
| 3 — Clients | C-01 | `client/rdstation` (HTTP client com retry e timeout) | Média |
| 3 — Clients | C-02 | `client/openai` (function calling) | Média |
| 4 — Serviços | S-01 | `service/nlp` (parse de intent + formatação de resposta) | Alta |
| 4 — Serviços | S-02 | `service/rdstation` (orquestração de todas as operações) | Alta |
| 4 — Serviços | S-03 | `service/intent_router` | Baixa |
| 4 — Serviços | S-04 | `service/conversation` | Baixa |
| 5 — API | A-01 | `handler/webhook` + rotas Gin + middleware de autenticação | Baixa |
| 6 — Testes | T-01 | Testes unitários dos serviços | Média |
| 6 — Testes | T-02 | Testes de integração do fluxo completo | Média |

**Riscos técnicos identificados:**

| ID | Descrição | Mitigação |
|---|---|---|
| RT-001 | Function calling pode não extrair entidades corretamente em português informal | System prompt com exemplos em pt-BR e instruções explícitas; possibilidade de fallback para parsing manual de campos críticos |
| RT-002 | A API do RD Station CRM não possui sandbox público — testes impactam dados reais | Criar contatos e negociações de teste com prefixo `[TESTE]` identificável; limpar após os testes |
| RT-003 | Documentação da API do RD Station para movimentação de cards é escassa | Validar endpoints reais na fase C-01, antes de implementar S-02; registrar respostas brutas no nível DEBUG |
| RT-004 | Histórico de conversa cresce indefinidamente sem limpeza | Janela deslizante de 10 mensagens resolve o problema de contexto; job de limpeza periódica registrado para V2 |

---

## 12. Glossário

| Termo | Definição |
|---|---|
| Intent | Intenção estruturada extraída de uma mensagem em linguagem natural (ex: `get_contacts`, `move_deal_stage`) |
| Function Calling | Funcionalidade da API da OpenAI que instrui o modelo a retornar um JSON estruturado em vez de texto livre |
| Session | Contexto de conversa associado a um número de telefone específico; persiste entre mensagens |
| Stage (Estágio) | Fase de um funil de vendas no RD Station CRM (ex: Prospecção, Proposta, Fechado Ganho) |
| Deal (Negociação) | Oportunidade de venda no RD Station CRM; representada como um card no kanban do funil |
| Mock | Implementação simulada do canal WhatsApp; permite desenvolvimento sem conta de WhatsApp Business |
| CRM | Customer Relationship Management — sistema de gestão de relacionamento com clientes (neste contexto: RD Station CRM) |
| E.164 | Formato internacional de numeração telefônica: `+` seguido do código do país e número (ex: `+5511999999999`) |
| 12-factor | Metodologia de configuração que exige separação completa entre código e configuração via variáveis de ambiente |

---

## 13. Histórico de Revisões

| Data | Autor | Descrição |
|---|---|---|
| 26/04/2026 | MarfPlanner | Versão inicial |

---

Gerado por MarfPlanner | 26/04/2026
