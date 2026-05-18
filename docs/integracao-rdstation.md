Documento: Integração — RD Station CRM API
Versão: 1.0.0
Data: 26/04/2026
Status: Rascunho
Gerado por: MarfPlanner

---

# Documento de Integração — RD Station CRM API

## 1. Visão Geral da Integração

- **Sistemas envolvidos:** synova-rd-workflow (`client/rdstation`) ↔ RD Station CRM API v1
- **Direção do fluxo:** bidirecional (leitura de contatos, negociações e estágios; escrita de contatos e negociações)
- **Protocolo de comunicação:** REST / HTTPS / JSON
- **Frequência:** por evento — a cada mensagem do usuário que resulte em uma intent de CRM
- **Autenticação:** Bearer Token (API Key permanente do RD Station CRM)

---

## 2. Autenticação e Segurança

- **Mecanismo:** Bearer Token no header `Authorization`
- **Onde a credencial é armazenada:** variável de ambiente `RDSTATION_API_KEY` (arquivo `.env` local, nunca versionado)
- **Rotação:** manual — o token não expira automaticamente; rotação recomendada a cada 90 dias ou em caso de comprometimento
- **Dados transmitidos:** nomes, e-mails, telefones e dados de negociações de clientes — trafegam exclusivamente via HTTPS (TLS 1.2+)
- **PII em logs:** campos `email`, `phone` e `name` de contatos NÃO devem ser registrados em nível INFO ou superior; apenas em DEBUG com flag explícito de depuração ativo

---

## 3. Contratos

### 3.1 GET /contacts — Buscar contatos

```
GET https://crm.rdstation.com/api/v1/contacts

Headers:
  Authorization: Bearer {RDSTATION_API_KEY}

Query params (todos opcionais — ao menos um deve ser fornecido):
  name:  string — filtro por nome (busca parcial, case-insensitive)
  email: string — filtro por e-mail (exato)
  phone: string — filtro por telefone (exato)
  page:  integer — página (padrão: 1)

Response 200:
{
  "contacts": [
    {
      "_id":        "string — ID interno do contato no RD Station",
      "name":       "string",
      "email":      "string",
      "phone":      "string",
      "created_at": "string (ISO 8601)",
      "updated_at": "string (ISO 8601)"
    }
  ],
  "total": "integer — total de registros correspondentes"
}
```

**Observação:** O RD Station retorna no máximo 25 registros por página. Se `total > 25`, o `service/rdstation` deve iterar as páginas ou limitar o retorno ao usuário informando que há mais resultados.

---

### 3.2 POST /contacts — Criar contato

```
POST https://crm.rdstation.com/api/v1/contacts

Headers:
  Authorization: Bearer {RDSTATION_API_KEY}
  Content-Type:  application/json

Request body:
{
  "contact": {
    "name":  "string — obrigatório",
    "email": "string — opcional",
    "phone": "string — opcional"
  }
}

Response 200:
{
  "_id":        "string — ID do contato criado",
  "name":       "string",
  "email":      "string",
  "phone":      "string",
  "created_at": "string (ISO 8601)"
}
```

**Observação:** O RD Station pode retornar 422 se já existir um contato com o mesmo e-mail. O `service/rdstation` deve tratar esse caso e informar o usuário.

---

### 3.3 GET /deals — Buscar negociações

```
GET https://crm.rdstation.com/api/v1/deals

Headers:
  Authorization: Bearer {RDSTATION_API_KEY}

Query params (todos opcionais):
  name:           string  — filtro por nome da negociação (busca parcial)
  deal_stage_id:  string  — filtro por ID do estágio
  win:            boolean — true = ganhas; false = perdidas; omitir = abertas
  page:           integer — página (padrão: 1)

Response 200:
{
  "deals": [
    {
      "_id":  "string — ID da negociação",
      "name": "string",
      "deal_stage": {
        "_id":  "string — ID do estágio atual",
        "name": "string — nome do estágio atual"
      },
      "contacts": [
        {
          "_id":  "string",
          "name": "string"
        }
      ],
      "created_at": "string (ISO 8601)",
      "updated_at": "string (ISO 8601)"
    }
  ],
  "total": "integer"
}
```

---

### 3.4 POST /deals — Criar negociação

```
POST https://crm.rdstation.com/api/v1/deals

Headers:
  Authorization: Bearer {RDSTATION_API_KEY}
  Content-Type:  application/json

Request body:
{
  "deal": {
    "name":           "string — obrigatório",
    "deal_stage_id":  "string — opcional (usa o estágio padrão do funil se omitido)",
    "contacts_attributes": [
      { "_id": "string — ID de um contato existente no RD Station" }
    ]
  }
}

Response 200:
{
  "_id":  "string",
  "name": "string",
  "deal_stage": {
    "_id":  "string",
    "name": "string"
  },
  "created_at": "string (ISO 8601)"
}
```

---

### 3.5 PUT /deals/{id} — Atualizar negociação (incluindo mover estágio)

```
PUT https://crm.rdstation.com/api/v1/deals/{id}

Path params:
  id: string — ID da negociação (obtido via GET /deals)

Headers:
  Authorization: Bearer {RDSTATION_API_KEY}
  Content-Type:  application/json

Request body (mover estágio):
{
  "deal": {
    "deal_stage_id": "string — ID do novo estágio (obtido via GET /deal_stages)"
  }
}

Request body (atualizar nome):
{
  "deal": {
    "name": "string — novo nome da negociação"
  }
}

Response 200:
{
  "_id":  "string",
  "name": "string",
  "deal_stage": {
    "_id":  "string",
    "name": "string"
  },
  "updated_at": "string (ISO 8601)"
}
```

---

### 3.6 GET /deal_stages — Listar estágios dos funis

```
GET https://crm.rdstation.com/api/v1/deal_stages

Headers:
  Authorization: Bearer {RDSTATION_API_KEY}

Response 200:
{
  "deal_stages": [
    {
      "_id":              "string — ID do estágio",
      "name":             "string — nome do estágio",
      "deal_pipeline_id": "string — ID do funil ao qual pertence",
      "created_at":       "string (ISO 8601)"
    }
  ]
}
```

**Observação:** Esta lista deve ser buscada dinamicamente quando o usuário informar um nome de estágio. Não cachear em V1 — o usuário pode renomear estágios na plataforma.

---

### 3.7 Tabela de Erros

| Código HTTP | Significado | Ação esperada no `client/rdstation` |
|---|---|---|
| 400 | Payload inválido (campo ausente ou formato incorreto) | Não retentar — retornar `RDStationError{Code: 400}` ao serviço |
| 401 | Token ausente ou inválido | Não retentar — logar em WARN e retornar erro; verificar `RDSTATION_API_KEY` |
| 404 | Recurso não encontrado | Não retentar — retornar `RDStationError{Code: 404}` |
| 422 | Entidade não processável (ex: e-mail duplicado) | Não retentar — retornar `RDStationError{Code: 422}` com mensagem do body |
| 429 | Rate limit atingido | Aguardar o tempo indicado no header `Retry-After` (ou 60s se ausente) e retentar uma vez |
| 500 | Erro interno do RD Station | Retentar após 2s; se falhar novamente, retornar `RDStationError{Code: 500}` |

**Tipo de erro Go:**

```go
type RDStationError struct {
    Code    int
    Message string
}

func (e *RDStationError) Error() string {
    return fmt.Sprintf("rdstation error %d: %s", e.Code, e.Message)
}
```

---

## 4. Comportamento em Falha

- **Tentativas máximas:** 2 (requisição original + 1 retry)
- **Estratégia de backoff:** fixo de 2s entre tentativas (exceto 429, que respeita `Retry-After`)
- **Timeout por requisição:** 10s
- **Comportamento quando o RD Station está indisponível:**
  - Após 2 tentativas falhas, retornar `RDStationError` para o `service/rdstation`
  - `service/rdstation` repassa para o `service/nlp` que formata mensagem amigável ao usuário
  - Mensagem padrão: `"Não foi possível acessar o RD Station no momento. Tente novamente em alguns instantes."`
- **Circuit breaker:** não implementado na V1 (registrado para V2)
- **Dead letter / fila:** não aplicável — operações são síncronas e o usuário recebe feedback imediato

---

## 5. Ambientes

| Ambiente | URL Base | Observações |
|---|---|---|
| Development | `https://crm.rdstation.com/api/v1` | RD Station não disponibiliza sandbox; usar conta real com dados de teste prefixados com `[TESTE]` |
| Production | `https://crm.rdstation.com/api/v1` | Mesma URL — diferenciado apenas pela chave de API |

**Nota:** Todos os contatos e negociações criados durante desenvolvimento e testes devem usar o prefixo `[TESTE]` no campo `name` para identificação e limpeza posterior.

---

## 6. SLA e Limites

- **Rate limit:** [PENDENTE — verificar com documentação oficial ou suporte do RD Station CRM]
- **Latência p99 esperada:** < 2s por requisição
- **Disponibilidade contratada:** [PENDENTE — verificar SLA do plano contratado]
- **Volumes estimados:** < 100 requisições/dia no uso inicial (uso pessoal por um único usuário)
- **Custo:** incluso no plano contratado do RD Station CRM — sem custo adicional por volume de API

---

## 7. Estratégia de Busca por Nome

Como a API do RD Station não oferece busca fuzzy, o `service/rdstation` deve implementar a seguinte lógica para lidar com nomes parciais ou com erros de digitação leve:

1. Enviar o nome exato informado pelo usuário no parâmetro de filtro
2. Se nenhum resultado for retornado, retornar mensagem ao usuário informando o resultado vazio
3. Se múltiplos resultados forem retornados, listar os primeiros 5 e solicitar especificação ao usuário

Essa lógica se aplica tanto para `GET /contacts?name=` quanto para `GET /deals?name=`.

---

## 8. Histórico de Revisões

| Data | Autor | Descrição |
|---|---|---|
| 26/04/2026 | MarfPlanner | Versão inicial |

---

Gerado por MarfPlanner | 26/04/2026
