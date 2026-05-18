Documento: Decomposição de Tarefas — synova-rd-workflow
Versão: 1.0.0
Data: 26/04/2026
Gerado por: MarfPlanner
SDD de referência: sdd-synova-rd-workflow.md v1.0.0

---

# Decomposição de Tarefas — synova-rd-workflow

## Tabela de Tarefas

| ID | Categoria | Componente | Descrição resumida | Complexidade | Dependências |
|---|---|---|---|---|---|
| M-001 | Modelo | `domain/intent.go` | Definir tipo `Intent`, `IntentName` e constantes de intents suportadas | Baixa | — |
| M-002 | Modelo | `domain/message.go` | Definir tipos `Session` e `Message` | Baixa | — |
| M-003 | Modelo | `domain/contact.go` | Definir tipo `Contact` com campos do RD Station | Baixa | — |
| M-004 | Modelo | `domain/deal.go` | Definir tipos `Deal` e `Stage` com campos do RD Station | Baixa | — |
| M-005 | Modelo | `migrations/001_initial.sql` | SQL de criação das tabelas `sessions` e `messages` com índices e constraints | Baixa | — |
| CFG-001 | Config | `config/config.go` | Leitura de todas as variáveis de ambiente; falha no startup se obrigatórias ausentes | Baixa | — |
| INFRA-001 | Infra | `docker-compose.yml` | Definir serviços `api` e `postgres`; variáveis de ambiente; volume para dados | Baixa | CFG-001 |
| INFRA-002 | Infra | `Dockerfile` | Build multi-stage Go: estágio de build + imagem final mínima (distroless ou alpine) | Baixa | — |
| R-001 | Repositório | `store/conversation` | Interface `ConversationStore` + implementação PostgreSQL: `GetOrCreateSession`, `GetRecentMessages`, `SaveMessage` | Baixa | M-002, M-005, CFG-001 |
| INT-001 | Integração | `client/rdstation` | HTTP client com timeout de 10s, retry (2 tentativas, backoff fixo 2s), tratamento de erros tipados (`RDStationError`). Métodos: `GetContacts`, `CreateContact`, `GetDeals`, `CreateDeal`, `UpdateDeal`, `GetDealStages` | Média | M-003, M-004, CFG-001 |
| INT-002 | Integração | `client/openai` | HTTP client para `POST /chat/completions` com function calling. Montar tools definition para cada intent. Timeout de 15s | Média | M-001, CFG-001 |
| UC-001 | Use Case | `service/nlp` (parse) | Montar system prompt com exemplos em pt-BR, enviar histórico + mensagem atual para OpenAI, parsear function call retornada e retornar `Intent` | Alta | INT-002, M-001 |
| UC-002 | Use Case | `service/rdstation` | Orquestrar as 6 operações de CRM: `GetContacts`, `CreateContact`, `GetDeals`, `CreateDeal`, `UpdateDeal`, `MoveDealStage`. Implementar busca por nome com lógica de múltiplos resultados e estágio não encontrado | Alta | INT-001, M-003, M-004 |
| UC-003 | Use Case | `service/intent_router` | Mapear `IntentName` → método correto em `service/rdstation`; retornar erro para `IntentUnknown` sem acionar o CRM | Baixa | UC-002, M-001 |
| UC-004 | Use Case | `service/conversation` | Gerenciar sessão por número de telefone; recuperar janela de 10 mensagens; persistir mensagem do usuário e resposta do assistente | Baixa | R-001, M-002 |
| UC-005 | Use Case | `service/nlp` (formatação) | Segunda chamada à OpenAI para converter resultado estruturado do CRM em linguagem natural; implementar mensagens de fallback para `unknown` e erros | Média | UC-001 |
| API-001 | Endpoint | `POST /mock/message` | Handler Gin: validar `X-Api-Key`, validar payload (from no padrão E.164, message não vazio e ≤ 4096 chars), orquestrar pipeline completo, retornar `reply` | Baixa | UC-001, UC-002, UC-003, UC-004, UC-005 |
| API-002 | Endpoint | `GET /health` | Handler Gin retornando `{ "status": "ok", "version": "..." }` | Baixa | — |
| API-003 | Endpoint | Middleware de autenticação | Middleware Gin que valida `X-Api-Key` em todas as rotas protegidas | Baixa | CFG-001 |
| TEST-001 | Testes | Unitários `service/nlp` | Mock de `client/openai`; cenários: intent conhecida retornada, intent unknown, falha na API da OpenAI | Média | UC-001, UC-005 |
| TEST-002 | Testes | Unitários `service/rdstation` | Mock de `client/rdstation`; cenários: resultado único, múltiplos resultados, não encontrado, erros 4xx e 5xx | Média | UC-002 |
| TEST-003 | Testes | Unitários `service/intent_router` | Verificar mapeamento de cada `IntentName` para o handler correto; verificar que `unknown` não aciona o CRM | Baixa | UC-003 |
| TEST-004 | Testes | Unitários `service/conversation` | Mock de `store/conversation`; verificar criação de sessão nova; verificar janela deslizante de 10 mensagens | Baixa | UC-004 |
| TEST-005 | Testes | Integração `POST /mock/message` | Fluxo completo com `httptest.NewServer` mockando OpenAI e RD Station; PostgreSQL de teste via `testcontainers-go`; cenários: get_contacts, create_contact, move_deal_stage, unknown, erro RD Station | Média | API-001, R-001 |

---

## Sequência de Execução Recomendada

**Fase 1 — Fundação**
M-001, M-002, M-003, M-004, CFG-001, INFRA-001, INFRA-002

**Fase 2 — Dados**
M-005, R-001

**Fase 3 — Clients externos**
INT-001, INT-002

**Fase 4 — Serviços**
UC-001, UC-002, UC-003, UC-004, UC-005

**Fase 5 — API**
API-003, API-001, API-002

**Fase 6 — Testes**
TEST-001, TEST-002, TEST-003, TEST-004, TEST-005

---

## Riscos Técnicos da Implementação

| ID | Descrição | Mitigação |
|---|---|---|
| RT-001 | Function calling pode não extrair entidades corretamente em português informal | System prompt com exemplos variados em pt-BR; fallback para parsing manual de campos obrigatórios ausentes |
| RT-002 | RD Station não disponibiliza sandbox — testes criam dados reais | Prefixar todos os dados de teste com `[TESTE]`; documentar script de limpeza |
| RT-003 | Documentação da API do RD Station para movimentação de cards é escassa | Validar endpoints na fase INT-001 antes de implementar UC-002; registrar responses brutas em nível DEBUG |
| RT-004 | Histórico de conversa cresce sem limite na V1 | Janela deslizante de 10 mensagens resolve o problema de contexto para o LLM; job de limpeza de sessões antigas registrado para V2 |

---

Gerado por MarfPlanner | 26/04/2026
