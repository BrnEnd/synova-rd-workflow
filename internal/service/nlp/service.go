package nlp

import (
	"context"
	"encoding/json"
	"fmt"

	openaiClient "synova-rd-workflow/internal/client/openai"
	"synova-rd-workflow/internal/domain"
)

// ServiceInterface defines the NLP contract for dependency injection and testing.
type ServiceInterface interface {
	ParseIntent(ctx context.Context, history []domain.Message, input string) (domain.Intent, error)
	FormatResponse(ctx context.Context, intent domain.Intent, result interface{}) (string, error)
}

const systemPrompt = `Você é um assistente especializado em CRM. Analise a mensagem do usuário e identifique a intenção.
Sempre use a ferramenta "identify_intent" para retornar a intenção estruturada com os parâmetros corretos.

Intents disponíveis e seus parâmetros obrigatórios/opcionais:

- get_contacts   → parâmetros opcionais: "name", "email", "phone"
- get_deals      → parâmetros opcionais: "name", "stage", "status" (status: "open", "won" ou "lost"), "owner_name" (nome do responsável/dono/vendedor)
- get_deal       → parâmetro OBRIGATÓRIO: "deal_name" (nome da negociação específica mencionada pelo usuário)
- get_deal_contacts → parâmetro OBRIGATÓRIO: "deal_name" (nome da negociação)
- create_contact → parâmetros: "name" (obrigatório), "email", "phone", "company" (opcionais)
- create_deal    → parâmetros: "name" (OBRIGATÓRIO — nome da nova negociação), "contact_name" (opcional), "stage" (opcional)
- update_deal    → parâmetros: "deal_name" (OBRIGATÓRIO), "field" (ex: "name" ou "stage"), "value" (novo valor)
- move_deal_stage → parâmetros: "deal_name" (OBRIGATÓRIO), "target_stage" (OBRIGATÓRIO — nome do estágio destino)
- delete_deal    -> parametro OBRIGATORIO: "deal_name" (quando o usuario pedir para apagar/excluir/deletar uma negociacao)
- update_contact → parâmetros: "contact_name" (OBRIGATÓRIO — nome do contato), "field" (OBRIGATÓRIO — ex: "email", "phone", "name"), "value" (OBRIGATÓRIO — novo valor)
- associate_contact_to_deal → parâmetros: "deal_name" (OBRIGATÓRIO), "contact_name" (OBRIGATÓRIO)
- get_deal_activities → parâmetro OBRIGATÓRIO: "deal_name" (nome da negociação cujas anotações serão consultadas)
- create_deal_activity → parâmetros: "deal_name" (OBRIGATÓRIO — negociação onde registrar), "text" (OBRIGATÓRIO — conteúdo da anotação)
- unknown        → quando não for possível identificar a intenção

REGRAS CRÍTICAS:
- Para get_deal e get_deal_contacts: sempre use a chave "deal_name" com o nome da negociação
- Para get_deals: quando o usuário pedir negociações "do", "da", "responsável", "dono", "vendedor" ou "atribuídas a" uma pessoa, use "owner_name"; não use "name" para o nome da pessoa
- Para create_deal: sempre use a chave "name" com o nome da nova negociação
- Para update_contact: "contact_name" é o nome do contato a editar; "field" é o campo (email/phone/name); "value" é o novo conteúdo
- Para associate_contact_to_deal: "deal_name" é a negociação destino; "contact_name" é o contato a associar
- Para get_deal_activities: sempre use "deal_name" com o nome da negociação cujas anotações serão listadas
- Para create_deal_activity: extraia exatamente o que o usuário quer registrar como "text"; o responsável pela anotação é determinado automaticamente pelo sistema
- Nunca deixe parâmetros obrigatórios vazios — se o usuário mencionou um nome, extraia-o
- Use o histórico da conversa para inferir nomes de negociações ou contatos quando o usuário usar pronomes como "essa", "esse", "desse"
- Responda apenas com a chamada de ferramenta — nunca com texto livre nesta etapa.`

var intentTool = openaiClient.Tool{
	Type: "function",
	Function: openaiClient.Function{
		Name:        "identify_intent",
		Description: "Identifica a intenção do usuário e extrai os parâmetros necessários para executar a ação no CRM.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"intent": map[string]interface{}{
					"type": "string",
					"enum": []string{
						"get_contacts", "get_deals", "get_deal", "get_deal_contacts",
						"create_contact", "create_deal", "update_deal", "move_deal_stage",
						"delete_deal", "update_contact", "associate_contact_to_deal",
						"get_deal_activities", "create_deal_activity", "unknown",
					},
					"description": "A intenção identificada na mensagem",
				},
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Nome do contato (get_contacts, create_contact) OU nome da NOVA negociação a criar (create_deal). NÃO usar para buscar negociações existentes.",
						},
						"deal_name": map[string]interface{}{
							"type":        "string",
							"description": "Nome de uma negociação EXISTENTE no CRM. Usar em: get_deal, get_deal_contacts, update_deal, move_deal_stage, get_deal_activities, create_deal_activity.",
						},
						"email": map[string]interface{}{
							"type":        "string",
							"description": "E-mail do contato (get_contacts, create_contact)",
						},
						"phone": map[string]interface{}{
							"type":        "string",
							"description": "Telefone do contato (get_contacts, create_contact)",
						},
						"company": map[string]interface{}{
							"type":        "string",
							"description": "Empresa do contato (create_contact)",
						},
						"stage": map[string]interface{}{
							"type":        "string",
							"description": "Nome do estágio para filtrar ou atribuir em get_deals ou create_deal",
						},
						"target_stage": map[string]interface{}{
							"type":        "string",
							"description": "Nome do estágio destino para mover a negociação (move_deal_stage)",
						},
						"status": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"open", "won", "lost"},
							"description": "Status da negociação para filtrar (get_deals)",
						},
						"owner_name": map[string]interface{}{
							"type":        "string",
							"description": "Nome do responsável/dono/vendedor da negociação. Usar em get_deals quando o usuário pedir negociações de uma pessoa, ex: Glauco ou Glauco de Oliveira.",
						},
						"contact_name": map[string]interface{}{
							"type":        "string",
							"description": "Nome do contato a associar à nova negociação (create_deal), a editar (update_contact) ou a vincular a uma negociação (associate_contact_to_deal)",
						},
						"field": map[string]interface{}{
							"type":        "string",
							"description": "Campo a atualizar em update_deal: 'name' (renomear) ou 'stage' (mover estágio)",
						},
						"value": map[string]interface{}{
							"type":        "string",
							"description": "Novo valor para o campo informado em update_deal",
						},
						"text": map[string]interface{}{
							"type":        "string",
							"description": "Conteúdo textual da anotação a registrar na negociação (create_deal_activity)",
						},
					},
					"additionalProperties": false,
				},
			},
			"required": []string{"intent", "parameters"},
		},
	},
}

// Service handles natural language processing via OpenAI.
type Service struct {
	client *openaiClient.Client
}

// New returns a new NLP Service.
func New(client *openaiClient.Client) *Service {
	return &Service{client: client}
}

// ParseIntent calls OpenAI with conversation history and the user's message,
// returning a structured Intent.
func (s *Service) ParseIntent(ctx context.Context, history []domain.Message, input string) (domain.Intent, error) {
	messages := buildMessages(history, input)

	resp, err := s.client.ChatCompletion(ctx, openaiClient.ChatCompletionRequest{
		Messages:   messages,
		Tools:      []openaiClient.Tool{intentTool},
		ToolChoice: map[string]interface{}{"type": "function", "function": map[string]string{"name": "identify_intent"}},
	})
	if err != nil {
		return domain.Intent{Name: domain.IntentUnknown, RawText: input}, fmt.Errorf("nlp.ParseIntent openai: %w", err)
	}

	if len(resp.Choices) == 0 || len(resp.Choices[0].Message.ToolCalls) == 0 {
		return domain.Intent{Name: domain.IntentUnknown, RawText: input}, nil
	}

	args := resp.Choices[0].Message.ToolCalls[0].Function.Arguments

	var parsed struct {
		Intent     string            `json:"intent"`
		Parameters map[string]string `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(args), &parsed); err != nil {
		return domain.Intent{Name: domain.IntentUnknown, RawText: input}, fmt.Errorf("nlp.ParseIntent unmarshal: %w", err)
	}

	if parsed.Parameters == nil {
		parsed.Parameters = map[string]string{}
	}

	return domain.Intent{
		Name:       domain.IntentName(parsed.Intent),
		Parameters: parsed.Parameters,
		RawText:    input,
	}, nil
}

// FormatResponse calls OpenAI to generate a natural language response
// based on the intent result data.
func (s *Service) FormatResponse(ctx context.Context, intent domain.Intent, result interface{}) (string, error) {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("nlp.FormatResponse marshal result: %w", err)
	}

	prompt := fmt.Sprintf(`O usuário pediu: "%s"
Intent identificada: %s
Resultado da operação no CRM (JSON): %s

Responda ao usuário em português brasileiro de forma clara, objetiva e amigável.
Se o resultado for uma lista, enumere os itens de forma legível.
Se o resultado indicar um erro, informe o usuário de forma gentil sem expor detalhes técnicos.`,
		intent.RawText,
		string(intent.Name),
		string(resultJSON),
	)

	resp, err := s.client.ChatCompletion(ctx, openaiClient.ChatCompletionRequest{
		Messages: []openaiClient.ChatMessage{
			{Role: "system", Content: "Você é um assistente que formata respostas de CRM para o usuário final."},
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		return "", fmt.Errorf("nlp.FormatResponse openai: %w", err)
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == nil {
		return "", fmt.Errorf("nlp.FormatResponse: empty response from OpenAI")
	}

	return *resp.Choices[0].Message.Content, nil
}

// FallbackResponse returns the standard message for unknown intents.
func FallbackResponse() string {
	return "Não entendi o que você precisa. Posso consultar e criar contatos e negociações, mover cards, atualizar contatos e associar contatos a negociações no seu RD Station. Como posso ajudar?"
}

// ErrorResponse returns a user-friendly error message.
func ErrorResponse(service string) string {
	return fmt.Sprintf("Não foi possível acessar o %s no momento. Tente novamente em alguns instantes.", service)
}

func buildMessages(history []domain.Message, current string) []openaiClient.ChatMessage {
	msgs := make([]openaiClient.ChatMessage, 0, len(history)+2)
	msgs = append(msgs, openaiClient.ChatMessage{Role: "system", Content: systemPrompt})

	for _, m := range history {
		msgs = append(msgs, openaiClient.ChatMessage{Role: m.Role, Content: m.Content})
	}

	msgs = append(msgs, openaiClient.ChatMessage{Role: "user", Content: current})
	return msgs
}
