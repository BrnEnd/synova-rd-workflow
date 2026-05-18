Documento: Integração — WhatsApp (Mock e Especificação do Canal Real)
Versão: 1.0.0
Data: 26/04/2026
Status: Rascunho
Gerado por: MarfPlanner

---

# Documento de Integração — WhatsApp

## 1. Visão Geral

Este documento descreve o contrato do canal de entrada de mensagens do synova-rd-workflow. Na V1, o canal é simulado por um endpoint REST local. O contrato foi desenhado para que a migração para o WhatsApp real exija apenas a adição de um novo handler de entrada sem alterar nenhuma camada interna do sistema.

- **Sistemas envolvidos:** WhatsApp (mock ou real) → synova-rd-workflow `handler/webhook`
- **Direção do fluxo:** bidirecional (recebimento de mensagem + envio de resposta)
- **Protocolo V1 (mock):** REST / HTTP local
- **Protocolo futuro (real):** webhook HTTPS — Meta Business API ou Twilio WhatsApp API
- **Frequência:** por evento (cada mensagem do usuário)

---

## 2. V1 — Mock via Endpoint REST Local

O mock simula o canal WhatsApp por meio de um endpoint HTTP que pode ser chamado via `curl`, Postman ou qualquer cliente HTTP. Ele aceita exatamente o mesmo contrato que será usado com o WhatsApp real, garantindo que nenhuma camada interna precise ser alterada na migração.

### 2.1 Endpoint do Mock

```
POST /mock/message

Headers:
  Content-Type: application/json
  X-Api-Key:    <valor de APP_API_KEY no .env>

Request body:
{
  "from":    "string — número do remetente em formato E.164 (ex: +5511999999999)",
  "message": "string — texto da mensagem (máx: 4096 chars)"
}

Response 200:
{
  "reply":      "string — resposta em linguagem natural gerada pelo sistema",
  "intent":     "string — intent identificada pelo NLP",
  "session_id": "string (UUID) — identificador da sessão do usuário"
}

Response 400:
{
  "error": "string — descrição do erro de validação"
}

Response 401:
{
  "error": "Unauthorized"
}
```

### 2.2 Autenticação do Mock

- Mecanismo: header `X-Api-Key`
- Valor: variável de ambiente `APP_API_KEY`
- Requisições sem o header ou com valor incorreto retornam HTTP 401

### 2.3 Exemplos de Uso (curl)

**Consultar contatos:**
```bash
curl -X POST http://localhost:8080/mock/message \
  -H "Content-Type: application/json" \
  -H "X-Api-Key: minha-chave-local" \
  -d '{"from": "+5511999999999", "message": "Quais contatos temos com sobrenome Silva?"}'
```

**Criar negociação:**
```bash
curl -X POST http://localhost:8080/mock/message \
  -H "Content-Type: application/json" \
  -H "X-Api-Key: minha-chave-local" \
  -d '{"from": "+5511999999999", "message": "Crie uma negociação chamada Expansão 2026 para o contato Maria Santos"}'
```

**Mover card:**
```bash
curl -X POST http://localhost:8080/mock/message \
  -H "Content-Type: application/json" \
  -H "X-Api-Key: minha-chave-local" \
  -d '{"from": "+5511999999999", "message": "Mova a negociação Projeto Alpha para o estágio Fechado Ganho"}'
```

---

## 3. V2 — Especificação do Canal Real (referência para migração)

Esta seção documenta o contrato esperado para a integração com o WhatsApp real. Deve ser revisada quando a decisão entre Meta Business API e Twilio for tomada.

### 3.1 Opções Avaliadas

| Provedor | Modelo | Observações |
|---|---|---|
| Meta Business API (direta) | Webhook oficial da Meta | Requer conta Business verificada, número dedicado e aprovação de templates |
| Twilio WhatsApp API | Wrapper REST sobre a Meta API | Mais simples de integrar; custo adicional por mensagem; sandbox disponível para testes |

**Decisão pendente:** [PENDENTE — avaliar custo, velocidade de aprovação e complexidade de setup]

### 3.2 Contrato Esperado do Webhook Real (Meta)

Quando o WhatsApp real for integrado, o payload de entrada será no formato do webhook da Meta:

```json
{
  "object": "whatsapp_business_account",
  "entry": [
    {
      "changes": [
        {
          "value": {
            "messages": [
              {
                "from": "string — número do remetente (sem +, ex: 5511999999999)",
                "text": {
                  "body": "string — texto da mensagem"
                }
              }
            ]
          }
        }
      ]
    }
  ]
}
```

**Adaptação necessária na migração:**
- Criar `handler/whatsapp_real` que extrai `from` e `body` do payload da Meta
- Normalizar `from` para formato E.164 (adicionar `+`)
- Acionar `service/conversation` e o pipeline com os mesmos parâmetros do mock
- Enviar a resposta de volta via API de envio da Meta (POST para a Messaging API)
- Nenhuma camada abaixo do handler precisa ser alterada

### 3.3 Verificação do Webhook (Meta)

A Meta exige que o endpoint de webhook responda a uma requisição de verificação GET com o token configurado no painel:

```
GET /webhook?hub.mode=subscribe&hub.verify_token={WEBHOOK_VERIFY_TOKEN}&hub.challenge={challenge}

Response 200: {challenge} (texto puro)
```

Este handler deve ser implementado quando a integração real for ativada. A variável `WEBHOOK_VERIFY_TOKEN` deve ser adicionada ao `.env`.

---

## 4. Comportamento em Falha

### V1 (mock)
- Erros internos retornam HTTP 500 com JSON `{ "error": "..." }`
- Não há retry no lado do cliente — o usuário pode reenviar a mensagem

### V2 (real)
- A Meta realiza retentativas automáticas do webhook se o servidor retornar status diferente de 200
- O handler real deve responder 200 imediatamente após receber a mensagem e processar de forma assíncrona (ou síncronamente dentro do timeout da Meta de ~20s)
- Se o processamento exceder 20s, implementar fila interna ou resposta assíncrona com callback

---

## 5. Decisão Pendente: Provedor WhatsApp Real

| Item | Status |
|---|---|
| Avaliação Twilio vs Meta direta | PENDENTE |
| Definição de número de WhatsApp Business | PENDENTE |
| Aprovação de conta Business na Meta | PENDENTE |
| Criação de templates de mensagem (se necessário) | PENDENTE |

Quando a decisão for tomada, este documento deve ser atualizado com o provedor escolhido, URL do webhook, variáveis de ambiente adicionais e contrato completo de envio de resposta.

---

## 6. Histórico de Revisões

| Data | Autor | Descrição |
|---|---|---|
| 26/04/2026 | MarfPlanner | Versão inicial — mock documentado; V2 como especificação de referência |

---

Gerado por MarfPlanner | 26/04/2026
