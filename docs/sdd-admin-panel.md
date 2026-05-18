Documento: SDD — synova-admin
Versão: 1.0.0
Data: 06/05/2026
Status: Rascunho
Gerado por: MarfPlanner

---

# SDD — synova-admin

## 1. Visão Geral

**Objetivo:** Este documento descreve a arquitetura, os contratos de API, os fluxos e o plano de implementação do painel administrativo do synova-rd-workflow — uma interface web que permite gerenciar colaboradores, configurar alertas proativos sobre negociações no RD Station CRM, controlar a allowlist do bot no EvolutionAPI e configurar a autenticação do administrador.

**Escopo — dentro:**
- Autenticação de usuário único administrador com troca de senha obrigatória no primeiro acesso
- Cadastro, edição, ativação e desativação de colaboradores (nome, e-mail, WhatsApp)
- Configuração de alertas: monitoramento de negociações por estágio e tempo, template de mensagem personalizável, destinatários por alerta
- Gerenciamento da allowlist do EvolutionAPI: quais números podem conversar com o bot
- Configuração de destinatários padrão de alertas
- Extensão da API Go existente (`synova-rd-workflow`) com endpoints de administração protegidos por JWT
- Frontend Next.js 15 com shadcn/ui e estética liquid glass

**Escopo — fora:**
- Multi-tenant / múltiplos usuários administradores
- Autenticação OAuth / SSO
- Painel de analytics de conversas do bot
- Gerenciamento de pipelines ou estágios no RD Station (apenas leitura para configuração de alertas)
- Deploy de infra (coberto pelo `terraform/` existente)
- Notificações por e-mail

**Público-alvo:** Desenvolvedor responsável pela implementação do painel administrativo.

**Agente de implementação:**
- Backend Go: MarfCoder
- Frontend Next.js: MarfFrontend

---

## 2. Contexto e Motivação

O synova-rd-workflow já fornece a lógica central de um agente conversacional via WhatsApp + RD Station CRM. Dois problemas operacionais persistem sem o painel admin:

1. **Sem proatividade:** o sistema apenas responde a mensagens; não há mecanismo para enviar alertas quando negociações ficam paradas em determinado estágio por muito tempo.
2. **Sem controle de acesso:** a allowlist do EvolutionAPI (quais números podem usar o bot) só pode ser alterada diretamente via variáveis de ambiente ou chamadas manuais à API do Evolution, sem interface.

O painel admin resolve esses dois problemas e centraliza a configuração operacional do sistema num único lugar.

**Restrições existentes:**
- Somente um usuário administrador existe. Não há necessidade de sistema de registro.
- A API Go do `synova-rd-workflow` já está estruturada com Gin. Os endpoints de admin serão adicionados ao mesmo processo, prefixados com `/admin`.
- O armazenamento de produção usa DynamoDB (conforme `store/conversation/dynamodb.go`). O painel admin usará o mesmo DynamoDB.
- O frontend `admin/` será uma aplicação Next.js separada do `playground/`.

---

## 3. Decisões de Arquitetura

### DA-001 — Extensão da API Go existente (não microserviço separado)
**Decisão:** Os endpoints de admin serão adicionados ao processo Go do `synova-rd-workflow`, agrupados sob o prefixo `/admin` com middleware JWT dedicado.
**Justificativa:** Um processo único elimina a necessidade de novo Dockerfile, novo serviço no docker-compose e novo deploy na AWS. O volume de tráfego admin é mínimo (um único usuário); não há ganho de escala em separar.
**Alternativas rejeitadas:**
- Microserviço Go separado: complexidade operacional desnecessária para escopo de um usuário.
- Next.js API Routes como BFF: adiciona uma camada de indireção sem benefício; aumenta a superfície de ataque.

### DA-002 — JWT (RS256) para autenticação admin
**Decisão:** Emitir JSON Web Tokens assinados com RS256 (par de chaves RSA gerado na inicialização ou via env var) com expiração de 8 horas. O token é armazenado em cookie `httpOnly; Secure; SameSite=Strict`.
**Justificativa:** RS256 permite validar tokens no frontend sem expor a chave privada. Cookie httpOnly elimina o risco de XSS roubar o token. SameSite=Strict previne CSRF.
**Alternativas rejeitadas:**
- HS256 com secret em variável de ambiente: seguro, mas a chave precisa ser compartilhada entre processo de geração e validação; RS256 é mais limpo.
- Sessão em banco: estado extra em DynamoDB para uma operação de baixíssima frequência.
- Bearer token no localStorage: vulnerável a XSS.

### DA-003 — Credenciais do admin via variáveis de ambiente + DynamoDB
**Decisão:** O e-mail inicial e o hash bcrypt da senha inicial são semeados via variáveis de ambiente (`ADMIN_EMAIL`, `ADMIN_INITIAL_PASSWORD_HASH`). Após a troca de senha, o hash atualizado é armazenado na tabela DynamoDB `admin_config`. A flag `must_change_password` é `true` enquanto a senha for a inicial.
**Justificativa:** Para um único usuário, criar uma tabela separada de usuários com registro é over-engineering. O seed via env var garante que o administrador sempre possa recuperar o acesso redefinindo a variável e reiniciando o serviço.
**Alternativas rejeitadas:**
- Apenas env var (sem DynamoDB): impossibilita a troca de senha persistente.
- Tabela `admin_users` completa: desnecessária para um único usuário.

### DA-004 — Scheduler de alertas como goroutine interna
**Decisão:** Um worker goroutine dentro do mesmo processo Go verifica alertas ativos a cada intervalo configurável (`ALERT_CHECK_INTERVAL`, padrão: 30 minutos). Para cada alerta, consulta o RD Station CRM, avalia negociações na condição e dispara mensagens via EvolutionAPI.
**Justificativa:** Para um único usuário administrador com poucos alertas configurados, uma goroutine interna é suficiente. AWS Lambda com EventBridge seria a alternativa de produção escalável — documentada como upgrade de V2.
**Alternativas rejeitadas:**
- AWS EventBridge + Lambda separado: complexidade de deploy e IAM desnecessária para V1.
- Cron externo (systemd timer): dependência de infraestrutura fora do processo Go.

### DA-005 — DynamoDB para persistência dos dados admin
**Decisão:** Usar o mesmo DynamoDB já provisionado para `store/conversation`. Criar novas tabelas: `admin_collaborators`, `admin_alerts`, `admin_allowlist`, `admin_config`.
**Justificativa:** Consistência com a infra existente. Evita adicionar um banco relacional separado só para o admin. O volume de dados admin é pequeno (dezenas de registros), adequado para DynamoDB com leitura por chave primária.
**Alternativas rejeitadas:**
- PostgreSQL separado: dependência nova, não presente na infra atual de produção.
- SSM Parameter Store para configs: adequado para segredos, não para dados estruturados como listas de alertas.

### DA-006 — Next.js 15 App Router com Server Components
**Decisão:** Usar Next.js 15 App Router. Páginas protegidas são Server Components que verificam o cookie JWT no middleware (`middleware.ts`) e redirecionam para `/login` se inválido. Componentes interativos (formulários, tabelas com ações) são Client Components.
**Justificativa:** Server Components reduzem o bundle JavaScript enviado ao cliente. O middleware de rota do Next.js é o lugar canônico para proteger páginas sem lógica duplicada.
**Alternativas rejeitadas:**
- Pages Router: versão anterior, sem os benefícios de Server Components.
- SPA pura (Vite + React): sem proteção server-side de rotas; requer backend separado para servir a aplicação.

### DA-007 — Liquid Glass com Tailwind CSS v4 + backdrop-filter
**Decisão:** Implementar o efeito liquid glass usando `backdrop-blur`, `backdrop-saturate`, `bg-white/10` (ou `bg-black/20`) e `border border-white/20` via Tailwind. Animações com Framer Motion para transições de página e micro-interações.
**Justificativa:** CSS nativo com Tailwind é mais performático do que bibliotecas de efeitos; o efeito glassmorphism é alcançável puramente com `backdrop-filter`.

---

## 4. Componentes e Responsabilidades

### 4.1 Backend — Novos Componentes

| Componente | Responsabilidade |
|---|---|
| `handler/admin/auth_handler.go` | POST /admin/login, POST /admin/logout, POST /admin/change-password |
| `handler/admin/collaborator_handler.go` | CRUD de colaboradores |
| `handler/admin/alert_handler.go` | CRUD de alertas |
| `handler/admin/allowlist_handler.go` | CRUD da allowlist do EvolutionAPI |
| `middleware/admin_auth.go` | Validação JWT RS256 em todas as rotas `/admin` (exceto /admin/login) |
| `service/admin/auth_service.go` | Login, troca de senha, emissão e revogação de JWT |
| `service/admin/collaborator_service.go` | Regras de negócio de colaboradores |
| `service/admin/alert_service.go` | Regras de negócio de alertas, validação de template |
| `service/admin/allowlist_service.go` | Sincronização da allowlist com o EvolutionAPI |
| `service/admin/alert_scheduler.go` | Worker goroutine; avalia alertas e dispara notificações |
| `store/admin/dynamodb.go` | CRUD DynamoDB para as 4 tabelas admin |
| `client/evolution/admin_client.go` | Métodos de gerenciamento de allowlist no EvolutionAPI |

### 4.2 Frontend — Estrutura de Diretórios

```
admin/
├── src/
│   ├── app/
│   │   ├── layout.tsx                   ← layout global (liquid glass bg)
│   │   ├── login/
│   │   │   └── page.tsx                 ← tela de login
│   │   ├── change-password/
│   │   │   └── page.tsx                 ← troca de senha obrigatória (1º acesso)
│   │   └── dashboard/
│   │       ├── layout.tsx               ← sidebar + header protegidos
│   │       ├── page.tsx                 ← dashboard home (resumo)
│   │       ├── collaborators/
│   │       │   ├── page.tsx             ← listagem de colaboradores
│   │       │   └── [id]/page.tsx        ← detalhe / edição
│   │       ├── alerts/
│   │       │   ├── page.tsx             ← listagem de alertas
│   │       │   └── [id]/page.tsx        ← detalhe / edição de alerta
│   │       └── allowlist/
│   │           └── page.tsx             ← gerenciamento da allowlist
│   ├── components/
│   │   ├── ui/                          ← shadcn/ui components customizados
│   │   ├── glass/
│   │   │   ├── GlassCard.tsx            ← card com efeito liquid glass
│   │   │   ├── GlassPanel.tsx           ← painel lateral/modal glass
│   │   │   └── GlassButton.tsx          ← botão com shimmer glass
│   │   ├── layout/
│   │   │   ├── Sidebar.tsx
│   │   │   └── Header.tsx
│   │   └── forms/
│   │       ├── CollaboratorForm.tsx
│   │       ├── AlertForm.tsx
│   │       └── AllowlistForm.tsx
│   ├── lib/
│   │   ├── api.ts                       ← fetch helpers com cookie auth
│   │   ├── auth.ts                      ← funções client-side de auth
│   │   └── utils.ts
│   └── middleware.ts                    ← proteção de rotas Next.js
├── package.json
├── tailwind.config.ts
└── tsconfig.json
```

### 4.3 Diagrama de Dependências

```
[Browser — Next.js 15 Admin Panel]
        |  (HTTPS, cookie JWT)
        v
[handler/admin/*] ← [middleware/admin_auth.go]
        |
        ├──> [service/admin/auth_service]       → DynamoDB (admin_config)
        ├──> [service/admin/collaborator_service] → DynamoDB (admin_collaborators)
        ├──> [service/admin/alert_service]       → DynamoDB (admin_alerts)
        └──> [service/admin/allowlist_service]   → DynamoDB (admin_allowlist)
                                                  → EvolutionAPI (sync allowlist)

[service/admin/alert_scheduler] ← goroutine (ticker)
        |
        ├──> DynamoDB (admin_alerts — leitura de alertas ativos)
        ├──> client/rdstation (busca negociações por estágio)
        └──> client/evolution (envia mensagem WhatsApp)
```

---

## 5. Modelo de Dados

### 5.1 Tabela: `admin_config`

| Atributo | Tipo | Descrição |
|---|---|---|
| `pk` | String | Chave fixa: `"admin"` |
| `email` | String | E-mail do administrador |
| `password_hash` | String | Hash bcrypt (custo 12) da senha atual |
| `must_change_password` | Boolean | `true` enquanto senha for a inicial |
| `updated_at` | String (ISO 8601) | Última atualização |

**Índices:** Chave primária `pk` (único registro).

---

### 5.2 Tabela: `admin_collaborators`

| Atributo | Tipo | Descrição |
|---|---|---|
| `id` | String (UUID v4) | Chave primária |
| `name` | String | Nome completo |
| `email` | String | E-mail (único) |
| `whatsapp` | String (E.164) | Número WhatsApp, ex: `+5511999999999` |
| `active` | Boolean | Colaborador habilitado para receber alertas |
| `created_at` | String (ISO 8601) | Data de cadastro |
| `updated_at` | String (ISO 8601) | Última atualização |

**Índices:**
- Chave primária: `id`
- GSI `email-index`: `email` → para garantir unicidade e busca por e-mail

**Regras de integridade:**
- `email` deve ser único. Verificado via GSI antes de inserir.
- `whatsapp` deve estar em formato E.164. Validado na camada de serviço (regex `^\+[1-9]\d{7,14}$`).
- Soft delete via `active = false`; registros não são excluídos fisicamente enquanto referenciados por alertas.

---

### 5.3 Tabela: `admin_alerts`

| Atributo | Tipo | Descrição |
|---|---|---|
| `id` | String (UUID v4) | Chave primária |
| `name` | String | Nome descritivo do alerta, ex: `"Negociação parada em Proposta"` |
| `deal_stage_id` | String | ID do estágio RD Station monitorado |
| `deal_stage_name` | String | Nome do estágio (cache, para exibição) |
| `time_threshold_hours` | Number | Horas no estágio para disparar o alerta |
| `message_template` | String | Template de mensagem com variáveis (ver seção 6.3) |
| `recipient_ids` | List\<String\> | Lista de IDs de `admin_collaborators` que receberão o alerta |
| `active` | Boolean | Alerta habilitado/desabilitado |
| `last_checked_at` | String (ISO 8601) | Última vez que o scheduler avaliou este alerta |
| `created_at` | String (ISO 8601) | Data de criação |
| `updated_at` | String (ISO 8601) | Última atualização |

**Índices:**
- Chave primária: `id`
- GSI `active-index`: `active` (Boolean projetado como String `"true"/"false"`) → para o scheduler buscar apenas alertas ativos

**Variáveis disponíveis no `message_template`:**
- `{{deal_name}}` — Nome da negociação
- `{{deal_stage}}` — Nome do estágio atual
- `{{days_in_stage}}` — Dias no estágio (calculado pelo scheduler)
- `{{contact_name}}` — Nome do primeiro contato da negociação
- `{{deal_value}}` — Valor da negociação (se disponível)

**Regras de integridade:**
- `time_threshold_hours` > 0.
- `recipient_ids` deve ter pelo menos 1 elemento.
- Quando um colaborador é desativado, ele é removido de `recipient_ids` de todos os alertas que o referenciam (via `alert_service` na atualização do colaborador).

---

### 5.4 Tabela: `admin_allowlist`

| Atributo | Tipo | Descrição |
|---|---|---|
| `id` | String (UUID v4) | Chave primária |
| `phone_number` | String (E.164) | Número autorizado a usar o bot |
| `label` | String | Rótulo descritivo, ex: `"Marco — cel pessoal"` |
| `active` | Boolean | Número habilitado na allowlist |
| `created_at` | String (ISO 8601) | Data de inclusão |
| `updated_at` | String (ISO 8601) | Última atualização |

**Índices:**
- Chave primária: `id`
- GSI `phone-index`: `phone_number` → para verificar unicidade e para o handler de webhook consultar se um número está autorizado

**Regras de integridade:**
- `phone_number` deve ser único.
- Sincronização com EvolutionAPI deve ocorrer a cada alteração (add/remove/toggle) via `allowlist_service`.

---

### 5.5 Tabela: `admin_alert_sent` (deduplicação)

| Atributo | Tipo | Descrição |
|---|---|---|
| `pk` | String | `"{alert_id}#{deal_id}"` |
| `sent_at` | String (ISO 8601) | Quando o alerta foi enviado para este deal |
| `ttl` | Number (Unix timestamp) | TTL DynamoDB — expira em 48h para permitir reenvio |

**Propósito:** Evitar que o scheduler envie o mesmo alerta para o mesmo deal em execuções consecutivas. O TTL de 48h garante que o alerta seja reenviado se a negociação continuar parada após 2 dias.

---

## 6. Contratos de API

Todos os endpoints de admin exigem cookie `session_token` com JWT válido, exceto `POST /admin/login`.

---

### 6.1 POST /admin/login

```
POST /admin/login

Request:
  Content-Type: application/json
  body: {
    "email":    "string — e-mail do administrador",
    "password": "string — senha atual"
  }

Response 200:
  Set-Cookie: session_token=<JWT>; HttpOnly; Secure; SameSite=Strict; Max-Age=28800
  body: {
    "must_change_password": boolean,
    "email": "string"
  }

Response 401:
  { "error": "invalid_credentials" }

Response 429:
  { "error": "too_many_attempts", "retry_after_seconds": 60 }
```

**Notas:**
- Rate limit: máximo 5 tentativas em 60 segundos por IP (middleware Gin).
- Se `must_change_password: true`, o frontend redireciona para `/change-password` antes de permitir acesso ao dashboard.

---

### 6.2 POST /admin/logout

```
POST /admin/logout
Cookie: session_token=<JWT>

Response 200:
  Set-Cookie: session_token=; Max-Age=0
  { "ok": true }
```

---

### 6.3 POST /admin/change-password

```
POST /admin/change-password
Cookie: session_token=<JWT>

Request:
  {
    "current_password": "string",
    "new_password":     "string — mínimo 12 chars, ao menos 1 maiúscula, 1 número, 1 símbolo"
  }

Response 200:
  { "ok": true }

Response 400:
  { "error": "weak_password", "detail": "string com requisitos" }

Response 401:
  { "error": "invalid_current_password" }
```

---

### 6.4 GET /admin/collaborators

```
GET /admin/collaborators?active=true|false|all (padrão: all)

Response 200:
  {
    "collaborators": [
      {
        "id":         "string (UUID)",
        "name":       "string",
        "email":      "string",
        "whatsapp":   "string (E.164)",
        "active":     boolean,
        "created_at": "string (ISO 8601)",
        "updated_at": "string (ISO 8601)"
      }
    ],
    "total": integer
  }
```

---

### 6.5 POST /admin/collaborators

```
POST /admin/collaborators

Request:
  {
    "name":     "string — obrigatório, 2–100 chars",
    "email":    "string — obrigatório, formato e-mail válido",
    "whatsapp": "string — obrigatório, formato E.164"
  }

Response 201:
  { "id": "string (UUID)", "name": "string", "email": "string", "whatsapp": "string", "active": true, "created_at": "string" }

Response 409:
  { "error": "email_already_exists" }

Response 422:
  { "error": "validation_failed", "fields": { "whatsapp": "invalid E.164 format" } }
```

---

### 6.6 PUT /admin/collaborators/:id

```
PUT /admin/collaborators/:id

Request: (mesmos campos do POST; todos opcionais)
  {
    "name":     "string",
    "email":    "string",
    "whatsapp": "string",
    "active":   boolean
  }

Response 200: (objeto colaborador atualizado)

Response 404:
  { "error": "collaborator_not_found" }

Response 409:
  { "error": "email_already_exists" }
```

---

### 6.7 DELETE /admin/collaborators/:id

```
DELETE /admin/collaborators/:id

Response 204: (sem body)

Response 404:
  { "error": "collaborator_not_found" }

Response 409:
  { "error": "collaborator_referenced_by_alerts", "alert_ids": ["string"] }
```

**Nota:** Exclusão física bloqueada se o colaborador for destinatário de algum alerta ativo. Deve-se desativá-lo via PUT ou remover dos alertas primeiro.

---

### 6.8 GET /admin/alerts

```
GET /admin/alerts?active=true|false|all (padrão: all)

Response 200:
  {
    "alerts": [
      {
        "id":                   "string (UUID)",
        "name":                 "string",
        "deal_stage_id":        "string",
        "deal_stage_name":      "string",
        "time_threshold_hours": integer,
        "message_template":     "string",
        "recipient_ids":        ["string"],
        "recipients":           [{ "id": "string", "name": "string", "whatsapp": "string" }],
        "active":               boolean,
        "last_checked_at":      "string (ISO 8601) | null",
        "created_at":           "string (ISO 8601)",
        "updated_at":           "string (ISO 8601)"
      }
    ],
    "total": integer
  }
```

---

### 6.9 POST /admin/alerts

```
POST /admin/alerts

Request:
  {
    "name":                 "string — obrigatório",
    "deal_stage_id":        "string — obrigatório, ID válido do RD Station",
    "time_threshold_hours": integer — obrigatório, > 0",
    "message_template":     "string — obrigatório, máx 1000 chars",
    "recipient_ids":        ["string"] — obrigatório, ao menos 1 ID válido de colaborador"
  }

Response 201: (objeto alerta criado)

Response 422:
  { "error": "validation_failed", "fields": { ... } }

Response 404:
  { "error": "deal_stage_not_found" }
```

---

### 6.10 PUT /admin/alerts/:id

```
PUT /admin/alerts/:id

Request: (mesmos campos do POST; todos opcionais)

Response 200: (objeto alerta atualizado)

Response 404:
  { "error": "alert_not_found" }
```

---

### 6.11 DELETE /admin/alerts/:id

```
DELETE /admin/alerts/:id

Response 204: (sem body)

Response 404:
  { "error": "alert_not_found" }
```

---

### 6.12 POST /admin/alerts/:id/test

```
POST /admin/alerts/:id/test

Dispara o alerta imediatamente (ignora threshold de tempo) para fins de teste.
Envia mensagem de teste para o número WhatsApp do próprio administrador.

Response 200:
  { "deals_matched": integer, "messages_sent": integer }

Response 404:
  { "error": "alert_not_found" }
```

---

### 6.13 GET /admin/allowlist

```
GET /admin/allowlist

Response 200:
  {
    "entries": [
      {
        "id":           "string (UUID)",
        "phone_number": "string (E.164)",
        "label":        "string",
        "active":       boolean,
        "created_at":   "string (ISO 8601)"
      }
    ],
    "total": integer
  }
```

---

### 6.14 POST /admin/allowlist

```
POST /admin/allowlist

Request:
  {
    "phone_number": "string — obrigatório, E.164",
    "label":        "string — opcional, máx 100 chars"
  }

Response 201: (objeto da entrada criada)

Response 409:
  { "error": "phone_number_already_exists" }

Response 422:
  { "error": "validation_failed", "fields": { "phone_number": "invalid E.164 format" } }

Response 502:
  { "error": "evolution_api_sync_failed", "detail": "string" }
```

**Nota:** O `allowlist_service` sincroniza imediatamente com o EvolutionAPI após criar a entrada. Em caso de falha na sincronização, a entrada ainda é persistida no DynamoDB e a tentativa de sync é agendada para a próxima execução do scheduler.

---

### 6.15 PUT /admin/allowlist/:id

```
PUT /admin/allowlist/:id

Request:
  {
    "label":  "string",
    "active": boolean
  }

Response 200: (objeto atualizado)

Response 404:
  { "error": "allowlist_entry_not_found" }
```

---

### 6.16 DELETE /admin/allowlist/:id

```
DELETE /admin/allowlist/:id

Response 204: (sem body)

Response 404:
  { "error": "allowlist_entry_not_found" }

Response 502:
  { "error": "evolution_api_sync_failed", "detail": "string" }
```

---

### 6.17 GET /admin/rd-station/stages

```
GET /admin/rd-station/stages

Retorna os estágios disponíveis no RD Station CRM para preencher o select
no formulário de criação de alertas.

Response 200:
  {
    "stages": [
      { "id": "string", "name": "string", "pipeline_name": "string" }
    ]
  }
```

---

## 7. Fluxos Principais

### 7.1 Fluxo: Primeiro Acesso (troca de senha obrigatória)

```
1. Admin acessa /login
2. Frontend exibe formulário de login
3. Admin submete e-mail + senha inicial (do .env)
4. POST /admin/login → Go valida bcrypt contra ADMIN_INITIAL_PASSWORD_HASH
5. Go emite JWT com claim extra "must_change_password: true"
6. Go persiste must_change_password=true em admin_config (caso ainda não exista)
7. Go seta cookie session_token e retorna { must_change_password: true }
8. Frontend detecta must_change_password: true e redireciona para /change-password
9. Admin digita senha atual + nova senha + confirmação
10. Frontend submete POST /admin/change-password
11. Go valida senha atual com bcrypt
12. Go valida força da nova senha (12+ chars, maiúscula, número, símbolo)
13. Go gera hash bcrypt (custo 12) da nova senha
14. Go atualiza admin_config: { password_hash: <novo_hash>, must_change_password: false }
15. Go retorna { ok: true }
16. Frontend redireciona para /dashboard

Fluxo Alternativo — Nova senha fraca (passo 12):
12a. Go retorna 400 { error: "weak_password", detail: "..." }
12b. Frontend exibe mensagem de erro inline sem redirecionar
```

---

### 7.2 Fluxo: Acesso Normal (login)

```
1. Admin acessa /login
2. Frontend exibe formulário
3. Admin submete e-mail + senha
4. POST /admin/login → Go valida bcrypt
5. Go emite JWT (8h) e seta cookie httpOnly
6. Go retorna { must_change_password: false }
7. Frontend redireciona para /dashboard

Fluxo Alternativo — Credenciais inválidas (passo 4):
4a. Go retorna 401 { error: "invalid_credentials" }
4b. Frontend exibe "E-mail ou senha inválidos" sem revelar qual campo está errado

Fluxo Alternativo — Rate limit atingido (passo 4):
4c. Go retorna 429 { retry_after_seconds: 60 }
4d. Frontend exibe timer de contagem regressiva e desabilita o botão de login
```

---

### 7.3 Fluxo: Proteção de Rotas (middleware Next.js)

```
1. Usuário tenta acessar /dashboard/* sem cookie válido
2. middleware.ts do Next.js intercepta a requisição
3. Middleware decodifica o JWT do cookie (verificação de expiração apenas, sem chamada ao backend)
4. JWT ausente ou expirado → redireciona para /login?redirect=<caminho_original>
5. JWT presente e válido → requisição prossegue
6. must_change_password=true no JWT → redireciona para /change-password
```

---

### 7.4 Fluxo: Cadastro de Colaborador

```
1. Admin navega para /dashboard/collaborators
2. Frontend carrega GET /admin/collaborators → lista colaboradores
3. Admin clica em "Novo Colaborador"
4. Modal GlassPanel abre com CollaboratorForm
5. Admin preenche nome, e-mail, WhatsApp
6. Frontend valida formato E.164 client-side antes de submeter
7. POST /admin/collaborators
8. Go valida campos, verifica unicidade de e-mail via GSI DynamoDB
9. Go persiste em admin_collaborators
10. Go retorna 201 com objeto criado
11. Frontend fecha modal e atualiza lista

Fluxo Alternativo — E-mail duplicado (passo 8):
8a. Go retorna 409 { error: "email_already_exists" }
8b. Frontend exibe erro inline no campo e-mail
```

---

### 7.5 Fluxo: Criação de Alerta

```
1. Admin navega para /dashboard/alerts → "Novo Alerta"
2. Frontend carrega GET /admin/rd-station/stages (para popular select de estágios)
3. Frontend carrega GET /admin/collaborators?active=true (para popular select de destinatários)
4. Admin preenche: nome do alerta, estágio, threshold (ex: 48h), template de mensagem
5. Admin seleciona destinatários (multi-select de colaboradores ativos)
6. Frontend exibe preview da mensagem com variáveis substituídas por valores fictícios
7. POST /admin/alerts
8. Go valida campos e verifica se deal_stage_id existe no RD Station
9. Go persiste em admin_alerts com active=true
10. Go retorna 201
11. Frontend exibe alerta na lista com status "Ativo"

Fluxo Alternativo — Estágio inválido (passo 8):
8a. Go retorna 404 { error: "deal_stage_not_found" }
8b. Frontend exibe erro no campo de estágio
```

---

### 7.6 Fluxo: Execução do Scheduler de Alertas

```
1. Goroutine scheduler acorda a cada ALERT_CHECK_INTERVAL (padrão 30min)
2. Scheduler busca todos os alertas com active=true em admin_alerts
3. Para cada alerta:
   3a. Consulta RD Station: GET /deals?stage_id=<deal_stage_id>
   3b. Filtra negociações com data de entrada no estágio > time_threshold_hours
       (usa deal.updated_at como proxy para "entrou no estágio")
   3c. Para cada negociação elegível:
       - Verifica tabela admin_alert_sent: chave "{alert_id}#{deal_id}"
       - Se já enviado e TTL não expirou: pula (deduplicação)
       - Renderiza message_template substituindo variáveis
       - Para cada recipient_id: busca whatsapp do colaborador em admin_collaborators
       - POST EvolutionAPI: envia mensagem para cada destinatário
       - Persiste em admin_alert_sent com TTL = now + 48h
   3d. Atualiza last_checked_at no alerta
4. Scheduler aguarda próximo tick

Fluxo de Exceção — RD Station indisponível (passo 3a):
3a-err. Log de erro com alerta ID; scheduler continua para o próximo alerta; não aborta o ciclo

Fluxo de Exceção — EvolutionAPI indisponível (envio de mensagem):
env-err. Log de erro com deal ID e destinatário; entrada NÃO é salva em admin_alert_sent
         (para ser re-tentada no próximo ciclo)
```

---

### 7.7 Fluxo: Gerenciamento da Allowlist + Sincronização EvolutionAPI

```
1. Admin navega para /dashboard/allowlist
2. Frontend carrega GET /admin/allowlist → lista entradas
3. Admin clica em "Adicionar Número"
4. Admin preenche número (E.164) e rótulo opcional
5. POST /admin/allowlist
6. Go persiste em admin_allowlist (active=true)
7. Go chama EvolutionAPI: adiciona número à allowlist da instância
8. Se EvolutionAPI retornar 2xx: retorna 201 ao frontend
9. Se EvolutionAPI retornar erro: Go salva DynamoDB com flag sync_pending=true;
   retorna 502 ao frontend com detalhe do erro

Fluxo: Verificação de allowlist durante recebimento de mensagem:
1. Bot recebe mensagem de número X
2. handler/webhook consulta admin_allowlist via GSI phone-index
3. Se phone_number não encontrado ou active=false: retorna sem processar
4. Se active=true: processa normalmente
```

---

## 8. Requisitos Não Funcionais

### 8.1 Performance
- Endpoints admin: latência < 500ms (P99) — tráfego mínimo, um único usuário.
- Scheduler: ciclo completo de verificação < 5 minutos para até 50 alertas configurados e 500 negociações no RD Station. Se ultrapassar, o intervalo do ticker deve ser ajustado.
- Frontend: Time to Interactive (TTI) < 2s em conexão 4G. Bundle JavaScript < 200KB gzipped.

### 8.2 Segurança
- **Autenticação:** JWT RS256, expiração 8h, cookie httpOnly/Secure/SameSite=Strict.
- **Senhas:** bcrypt custo 12. Política mínima: 12 chars, 1 maiúscula, 1 número, 1 símbolo.
- **Rate limiting:** 5 tentativas de login por IP em 60s (middleware Gin com janela deslizante em memória).
- **CORS:** `Access-Control-Allow-Origin` restrito ao domínio do painel admin (`ADMIN_ORIGIN` env var). Sem wildcard.
- **Headers de segurança:** `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Content-Security-Policy` configurado no Next.js middleware.
- **Segredos:** Nenhuma credencial (JWT private key, admin password hash) deve ser logada. Private key RSA carregada de env var `ADMIN_JWT_PRIVATE_KEY` (PEM, base64-encoded).
- **PII:** campos `email`, `whatsapp`, `phone_number` não devem aparecer em logs de nível INFO+.
- **Validação de entrada:** Todos os inputs validados server-side. Campos string com limites de tamanho. `phone_number` validado com regex E.164 rigoroso.

### 8.3 Disponibilidade
- O scheduler é tolerante a falhas de dependências externas: uma falha no RD Station ou EvolutionAPI não derruba o processo; apenas loga o erro e continua o ciclo.
- O processo Go deve reiniciar automaticamente (Docker `restart: unless-stopped` ou ECS task restart policy).

### 8.4 Observabilidade
- Logs estruturados (JSON) em todos os handlers e no scheduler, com campos: `component`, `alert_id`, `deal_id`, `error`.
- Métricas a logar por ciclo do scheduler: `alerts_evaluated`, `deals_matched`, `messages_sent`, `errors`.
- Next.js: erros de fetch (4xx/5xx) devem logar no console com request ID para correlação.

---

## 9. Dependências Externas

### 9.1 RD Station CRM API
- **Uso no admin:** Endpoint `GET /deal_stages` para popular o select de estágios no formulário de alerta; `GET /deals` com filtro por estágio para o scheduler.
- **Autenticação:** Bearer Token existente (`RDSTATION_API_KEY`).
- **Fallback:** Se indisponível durante criação de alerta, a validação do `deal_stage_id` é relaxada (aviso ao usuário, não bloqueio). Se indisponível no scheduler, o ciclo para aquele alerta é pulado com log de erro.

### 9.2 EvolutionAPI
- **Uso no admin:** Sincronização da allowlist (adicionar/remover números); envio de mensagens de alerta.
- **Autenticação:** API Key via header `apikey` (variável `EVOLUTION_API_KEY`).
- **Endpoints usados:**
  - `POST /instance/{instance}/whitelist` — adicionar número à allowlist
  - `DELETE /instance/{instance}/whitelist/{phone}` — remover número
  - `POST /message/sendText/{instance}` — enviar mensagem de texto
- **Contrato esperado:**

```
POST /message/sendText/{instance}
Headers: apikey: {EVOLUTION_API_KEY}
Body: {
  "number":  "string (E.164 sem +, ex: 5511999999999)",
  "text":    "string"
}
Response 201: { "key": { "id": "string" }, "status": "PENDING" }
```

- **Fallback:** Falha na sincronização da allowlist salva flag `sync_pending=true` no DynamoDB. O scheduler verifica entradas com `sync_pending=true` a cada ciclo e re-tenta a sincronização.

### 9.3 DynamoDB (AWS)
- **Tabelas novas:** `admin_config`, `admin_collaborators`, `admin_alerts`, `admin_allowlist`, `admin_alert_sent`.
- **Provisionamento:** On-demand (pay-per-request) — volume admin é mínimo.
- **Fallback:** Sem fallback — DynamoDB é a fonte da verdade. Falha de DynamoDB retorna 503 ao cliente.

---

## 10. Estratégia de Testes

### 10.1 Testes Unitários — Backend Go

| ID | Componente | Cenários obrigatórios |
|---|---|---|
| UT-001 | `service/admin/auth_service` | Login correto; senha errada; must_change_password; troca de senha com senha fraca; troca com senha atual errada |
| UT-002 | `service/admin/collaborator_service` | Criar colaborador válido; e-mail duplicado; whatsapp formato inválido; soft-delete bloqueado por alerta |
| UT-003 | `service/admin/alert_service` | Criar alerta válido; destinatário inexistente; threshold zero; template com variáveis inválidas |
| UT-004 | `service/admin/alert_scheduler` | Nenhum deal elegível; deal elegível + mensagem enviada; deduplicação (mesmo deal em 2 ciclos); RD Station falha; EvolutionAPI falha |
| UT-005 | `service/admin/allowlist_service` | Adicionar número; duplicata; EvolutionAPI falha → sync_pending; re-tentativa de sync |

### 10.2 Testes de Integração — Backend Go

| ID | Cenário |
|---|---|
| IT-001 | `POST /admin/login` → cookie JWT emitido → `GET /admin/collaborators` com cookie → 200 |
| IT-002 | `POST /admin/login` → `POST /admin/change-password` com senha fraca → 400 |
| IT-003 | `GET /admin/collaborators` sem cookie → 401 |
| IT-004 | Ciclo completo do scheduler com DynamoDB real (testcontainers) + mock de RD Station e EvolutionAPI |

### 10.3 Testes de Componente — Frontend Next.js

| ID | Componente | Cenários |
|---|---|---|
| CT-001 | `CollaboratorForm` | Submit válido; erro de e-mail duplicado exibido; validação E.164 client-side |
| CT-002 | `AlertForm` | Preview de mensagem atualiza em tempo real; seleção de múltiplos destinatários; submit sem destinatário exibe erro |
| CT-003 | Login page | Erro de credenciais; contador de rate limit; redirect para change-password |

### 10.4 Critérios de Aceite

- Cobertura mínima: 80% nas funções de `service/admin/*`.
- Nenhum teste deve depender de credenciais reais de RD Station ou EvolutionAPI.
- Todos os fluxos de primeiro acesso e troca de senha devem ter cobertura de teste de integração.

---

## 11. Plano de Implementação

### Fase 1 — Fundação Backend (baixa complexidade)

| ID | Tarefa | Complexidade | Dep |
|---|---|---|---|
| ADM-001 | Criar tabelas DynamoDB no `terraform/main.tf`: `admin_config`, `admin_collaborators`, `admin_alerts`, `admin_allowlist`, `admin_alert_sent` com GSIs | Baixa | — |
| ADM-002 | Adicionar variáveis de ambiente ao `config/config.go`: `ADMIN_EMAIL`, `ADMIN_INITIAL_PASSWORD_HASH`, `ADMIN_JWT_PRIVATE_KEY`, `ADMIN_JWT_PUBLIC_KEY`, `ADMIN_ORIGIN`, `ALERT_CHECK_INTERVAL` | Baixa | — |
| ADM-003 | Implementar `store/admin/dynamodb.go`: interfaces e CRUD para as 4 tabelas admin | Média | ADM-001 |
| ADM-004 | Adicionar domínio: `domain/admin.go` com tipos `Collaborator`, `Alert`, `AllowlistEntry`, `AdminConfig` | Baixa | — |

### Fase 2 — Autenticação (média complexidade)

| ID | Tarefa | Complexidade | Dep |
|---|---|---|---|
| ADM-005 | Implementar `service/admin/auth_service.go`: login, troca de senha, emissão JWT RS256 | Média | ADM-002, ADM-003, ADM-004 |
| ADM-006 | Implementar `middleware/admin_auth.go`: validação JWT, extração de claims, rate limiting por IP | Média | ADM-005 |
| ADM-007 | Implementar `handler/admin/auth_handler.go`: POST /admin/login, /logout, /change-password | Baixa | ADM-005, ADM-006 |
| ADM-008 | Registrar rotas `/admin/*` no `cmd/api/main.go` com grupo Gin protegido pelo middleware | Baixa | ADM-006, ADM-007 |

### Fase 3 — CRUD de Colaboradores e Allowlist (baixa complexidade)

| ID | Tarefa | Complexidade | Dep |
|---|---|---|---|
| ADM-009 | Implementar `service/admin/collaborator_service.go` + `handler/admin/collaborator_handler.go` | Baixa | ADM-003, ADM-004, ADM-008 |
| ADM-010 | Estender `client/evolution/admin_client.go` com métodos de allowlist (whitelist add/remove) | Baixa | — |
| ADM-011 | Implementar `service/admin/allowlist_service.go` + `handler/admin/allowlist_handler.go` | Baixa | ADM-003, ADM-010 |

### Fase 4 — Alertas e Scheduler (alta complexidade)

| ID | Tarefa | Complexidade | Dep |
|---|---|---|---|
| ADM-012 | Implementar `service/admin/alert_service.go` + `handler/admin/alert_handler.go` (CRUD + test endpoint) | Média | ADM-003, ADM-004, ADM-009 |
| ADM-013 | Implementar `service/admin/alert_scheduler.go`: goroutine com ticker, avaliação de alertas, deduplicação via admin_alert_sent, envio via EvolutionAPI | Alta | ADM-003, ADM-010, ADM-012 |
| ADM-014 | Integrar scheduler no startup: `cmd/api/main.go` inicia goroutine do scheduler após inicialização dos serviços | Baixa | ADM-013 |

### Fase 5 — Endpoint auxiliar RD Station

| ID | Tarefa | Complexidade | Dep |
|---|---|---|---|
| ADM-015 | Implementar `GET /admin/rd-station/stages` no handler de alertas (proxy para RD Station) | Baixa | ADM-008 |

### Fase 6 — Frontend Next.js

| ID | Tarefa | Complexidade | Dep |
|---|---|---|---|
| ADM-016 | Scaffold `admin/` Next.js 15: configurar Tailwind v4, shadcn/ui, Framer Motion | Baixa | — |
| ADM-017 | Implementar componentes Glass: `GlassCard`, `GlassPanel`, `GlassButton` com backdrop-blur + Tailwind | Média | ADM-016 |
| ADM-018 | Implementar `middleware.ts` de proteção de rotas + tela de login + troca de senha | Média | ADM-016, ADM-007 |
| ADM-019 | Implementar layout do dashboard: Sidebar glass + Header + estrutura de páginas | Média | ADM-017, ADM-018 |
| ADM-020 | Implementar página de Colaboradores: listagem + modal de criação/edição | Média | ADM-019, ADM-009 |
| ADM-021 | Implementar página de Alertas: listagem + formulário completo com preview de mensagem | Alta | ADM-019, ADM-012, ADM-015 |
| ADM-022 | Implementar página de Allowlist: listagem + formulário + toggle de ativação | Média | ADM-019, ADM-011 |

### Fase 7 — Testes

| ID | Tarefa | Complexidade | Dep |
|---|---|---|---|
| ADM-023 | Testes unitários `service/admin/*` (UT-001 a UT-005) | Média | ADM-005 a ADM-013 |
| ADM-024 | Testes de integração (IT-001 a IT-004) com testcontainers | Média | Toda fase backend |
| ADM-025 | Testes de componente React (CT-001 a CT-003) com Vitest + Testing Library | Média | ADM-020 a ADM-022 |

---

### Riscos Técnicos

| ID | Descrição | Mitigação |
|---|---|---|
| RT-001 | EvolutionAPI pode não ter endpoint de gerenciamento de whitelist estável na versão em uso | Verificar versão da instância antes de ADM-010; se ausente, substituir by bypass: checker de allowlist é feito apenas no DynamoDB, sem sync com EvolutionAPI |
| RT-002 | `deal.updated_at` no RD Station pode refletir edições gerais, não apenas mudança de estágio | Implementar como V1 com nota de limitação; para V2 avaliar webhook do RD Station para rastrear mudanças de estágio com timestamp preciso |
| RT-003 | Rate limiting de login em memória é perdido no restart do processo | Aceitável para V1 (único usuário, baixo risco de ataque); para produção usar Redis ou DynamoDB para estado de rate limit |
| RT-004 | Scheduler concorrente se o intervalo for menor que o tempo de execução do ciclo | Usar `sync.Mutex` ou canal de sinalização para garantir que apenas um ciclo rode por vez |

---

## 12. Glossário

| Termo | Definição |
|---|---|
| **Allowlist** | Lista de números de telefone autorizados a interagir com o bot no EvolutionAPI. Números fora da lista são ignorados. |
| **Alert** | Configuração que define: qual estágio de CRM monitorar, por quanto tempo, qual mensagem enviar e para quem. |
| **Scheduler** | Goroutine Go que acorda periodicamente para avaliar alertas ativos e disparar notificações via WhatsApp. |
| **Liquid Glass** | Estilo visual que usa `backdrop-filter: blur` + transparência + bordas sutis para criar a ilusão de vidro fosco sobre o conteúdo de fundo. |
| **must_change_password** | Flag que indica que a senha do administrador ainda é a inicial e precisa ser trocada antes de acessar o sistema. |
| **TTL (DynamoDB)** | Time-to-Live: atributo numérico Unix timestamp que faz o DynamoDB expirar e excluir automaticamente o item após a data indicada. Usado em `admin_alert_sent` para deduplicação com janela de 48h. |
| **deal_stage_id** | ID interno do RD Station CRM que identifica um estágio de pipeline (coluna no kanban). |
| **E.164** | Formato internacional de número de telefone definido pela ITU-T: `+` seguido de código de país e número local, sem espaços (ex: `+5511999999999`). |
| **JWT RS256** | JSON Web Token assinado com algoritmo RSA-SHA256. A chave privada assina o token; a chave pública valida. Permite verificação sem expor a chave de assinatura. |
| **GlassCard / GlassPanel** | Componentes React customizados construídos sobre shadcn/ui com efeito liquid glass aplicado via Tailwind. |

---

## 13. Histórico de Revisões

| Data | Autor | Descrição |
|---|---|---|
| 06/05/2026 | MarfPlanner | Versão inicial do documento |

---

*Gerado por MarfPlanner | 06/05/2026*
