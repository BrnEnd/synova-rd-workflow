package nlp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	openaiClient "synova-rd-workflow/internal/client/openai"
	"synova-rd-workflow/internal/domain"
)

// ServiceInterface defines the NLP contract for dependency injection and testing.
type ServiceInterface interface {
	ParseIntent(ctx context.Context, history []domain.Message, input string) (domain.Intent, error)
	FormatResponse(ctx context.Context, intent domain.Intent, result interface{}) (string, error)
	TranscribeAudio(ctx context.Context, audio []byte, filename string) (string, error)
}

const systemPrompt = `Você é um assistente especializado em CRM. Analise a mensagem do usuário e identifique a intenção.
Sempre use a ferramenta "identify_intent" para retornar a intenção estruturada com os parâmetros corretos.

Intents disponíveis e seus parâmetros obrigatórios/opcionais:

- get_contacts   → parâmetros opcionais: "name", "email", "phone"
- get_deals      → parâmetros opcionais: "name", "stage", "status" (status: "open", "won" ou "lost"), "owner_name" (nome do responsável/dono/vendedor), "updated_after" (data mínima de atualização YYYY-MM-DD), "updated_before" (data máxima de atualização YYYY-MM-DD)
- get_deal       → parâmetro OBRIGATÓRIO: "deal_name" (nome da negociação específica mencionada pelo usuário)
- get_deal_summary → parâmetro OBRIGATÓRIO: "deal_name" (resumo executivo/detalhado da negociação)
- get_deal_contacts → parâmetro OBRIGATÓRIO: "deal_name" (nome da negociação)
- create_contact → parâmetros: "name" (obrigatório), "email", "phone", "company" (opcionais)
- create_deal    → parâmetros: "name" (nome da nova negociação), "company" (cliente/empresa), "product" (produto), "contact_name" (contato responsável no cliente), "stage" (etapa), "owner_name" (vendedor/responsável), "notes" (observações)
- update_deal    → parâmetros: "deal_name" (OBRIGATÓRIO), "field" (ex: "name" ou "stage"), "value" (novo valor)
- move_deal_stage → parâmetros: "deal_name" (OBRIGATÓRIO), "target_stage" (OBRIGATÓRIO — nome do estágio destino)
- delete_deal    -> parametro OBRIGATORIO: "deal_name" (quando o usuario pedir para apagar/excluir/deletar uma negociacao)
- update_contact → parâmetros: "contact_name" (OBRIGATÓRIO — nome do contato), "field" (OBRIGATÓRIO — ex: "email", "phone", "name"), "value" (OBRIGATÓRIO — novo valor)
- associate_contact_to_deal → parâmetros: "deal_name" (OBRIGATÓRIO), "contact_name" (OBRIGATÓRIO)
- get_deal_activities → parâmetro OBRIGATÓRIO: "deal_name" (nome da negociação cujas anotações serão consultadas)
- create_deal_activity → parâmetros: "deal_name" (OBRIGATÓRIO — negociação onde registrar), "text" (OBRIGATÓRIO — conteúdo da anotação)
- unknown        → quando não for possível identificar a intenção

- get_scheduled_tasks -> sem parametros; usado quando o usuario pedir tarefas agendadas, alertas agendados, pendencias agendadas ou o que esta pendente nos alertas
- create_scheduled_task -> parametros: "deal_name" (negociacao), "subject" (assunto da tarefa), "date" (AAAA-MM-DD), "hour" (HH:MM), "type" (call, email, meeting, task, lunch, visit, whatsapp), "owner_name" (responsavel opcional), "notes" (observacoes opcionais)

REGRAS CRÍTICAS:
- Para get_deal e get_deal_contacts: sempre use a chave "deal_name" com o nome da negociação
- Para get_deal_summary: use quando o usuário pedir "mais detalhes", "mais informações", "resumo", "contexto" ou "situação atual" de uma negociação. Use "deal_name" com o nome da negociação; se o usuário disser "essa", "esse card", "dela" ou similar, infira pelo histórico.
- Para get_deals: quando o usuário pedir negociações "do", "da", "responsável", "dono", "vendedor" ou "atribuídas a" uma pessoa, use "owner_name"; não use "name" para o nome da pessoa
- Para get_deals: quando o usuário mencionar um intervalo de datas de atualização (ex: "atualizadas entre 13/06 e 23/06", "dos últimos 10 dias", "desde 01/06"), extraia as datas nos parâmetros "updated_after" e "updated_before" no formato YYYY-MM-DD. Use o ano corrente se o usuário não informar o ano. Se o usuário disser "últimos N dias", calcule updated_after como hoje menos N dias.
- Para create_deal: "card" significa negociação. Se o usuário informar cliente/empresa e produto, preencha "company" e "product"; se não houver "name", monte "name" como "Cliente - Produto". Se informar vendedor/responsável, use "owner_name". Qualquer detalhe adicional que não caiba nos campos estruturados deve ir em "notes".
- Para update_contact: "contact_name" é o nome do contato a editar; "field" é o campo (email/phone/name); "value" é o novo conteúdo
- Para associate_contact_to_deal: "deal_name" é a negociação destino; "contact_name" é o contato a associar
- Para get_deal_activities: sempre use "deal_name" com o nome da negociação cujas anotações serão listadas
- Para create_deal_activity: extraia exatamente o que o usuário quer registrar como "text"; o responsável pela anotação é determinado automaticamente pelo sistema
- Nunca deixe parâmetros obrigatórios vazios — se o usuário mencionou um nome, extraia-o
- Use o histórico da conversa para inferir nomes de negociações ou contatos quando o usuário usar pronomes como "essa", "esse", "desse"
- Responda apenas com a chamada de ferramenta — nunca com texto livre nesta etapa.`

const defaultDealSummaryPrompt = `Resuma a negociação abaixo de forma clara, executiva e com bom nível de detalhe factual.

Inclua:
- contexto da negociação
- histórico registrado do que já aconteceu
- última interação/anotação relevante
- situação atual com base na etapa e nos registros
- tarefas ou follow-ups agendados no RD, quando existirem, sem interpretar o objetivo delas

Regras:
- Não responda apenas repetindo cliente, vendedor, datas e fase.
- Use as atividades/anotações para reconstruir a linha do tempo e o andamento da negociação.
- Se uma anotação indicar um fato concluído, como homologação, pedido realizado, proposta enviada ou material enviado, descreva esse fato com clareza.
- Use as tarefas abertas apenas para informar o que está agendado no RD, com assunto, data, horário e responsável disponíveis.
- Se houver poucas anotações, não diga genericamente que "não há outros registros" quando já existir informação útil; apenas limite o resumo aos registros disponíveis.
- Não invente fatos que não estejam nos dados.
- Escreva em português brasileiro, em 2 a 4 parágrafos curtos.
- Comece com "*Resumo da negociação:*".`

const dealSummaryGuardrails = `Guard rails obrigatórios:
- Não suponha o que vai acontecer depois.
- Não crie "próximo passo provável".
- Não explique a finalidade de uma tarefa agendada a menos que isso esteja escrito explicitamente na própria tarefa ou anotação.
- Não use termos como "provavelmente", "deve", "deverá", "tende a", "espera-se", "aguarda-se" ou equivalentes para prever ações futuras.
- Se houver tarefas abertas, apenas liste ou descreva o que está agendado no RD com data, horário, assunto e responsável disponíveis.
- Diferencie fatos registrados de agenda: fatos vêm de negociação/anotações; agenda vem de tarefas abertas.
- Você pode explicar o significado comercial de fatos já registrados, mas nunca transformar uma tarefa aberta em previsão do que acontecerá.
- Quando não houver informação suficiente, diga que não há registro suficiente em vez de completar lacunas.`

const maxDealsPerResponse = 30

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
						"get_deal_summary", "create_contact", "create_deal", "update_deal", "move_deal_stage",
						"delete_deal", "update_contact", "associate_contact_to_deal",
						"get_deal_activities", "create_deal_activity", "get_scheduled_tasks", "create_scheduled_task", "unknown",
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
							"description": "Nome de uma negociação EXISTENTE no CRM. Usar em: get_deal, get_deal_summary, get_deal_contacts, update_deal, move_deal_stage, get_deal_activities, create_deal_activity.",
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
							"description": "Empresa/cliente do contato (create_contact) ou cliente da nova negociação (create_deal)",
						},
						"product": map[string]interface{}{
							"type":        "string",
							"description": "Produto relacionado à nova negociação/card (create_deal). Se houver cliente e produto, o nome da negociação deve ser Cliente - Produto.",
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
						"updated_after": map[string]interface{}{
							"type":        "string",
							"description": "Data mínima de atualização das negociações no formato YYYY-MM-DD (get_deals)",
						},
						"updated_before": map[string]interface{}{
							"type":        "string",
							"description": "Data máxima de atualização das negociações no formato YYYY-MM-DD (get_deals)",
						},
						"contact_name": map[string]interface{}{
							"type":        "string",
							"description": "Nome do contato responsável no cliente a associar à nova negociação (create_deal), a editar (update_contact) ou a vincular a uma negociação (associate_contact_to_deal)",
						},
						"notes": map[string]interface{}{
							"type":        "string",
							"description": "Observações livres para registrar na negociação quando o usuário disser qualquer detalhe adicional.",
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
						"subject": map[string]interface{}{
							"type":        "string",
							"description": "Assunto da tarefa a criar no RD Station (create_scheduled_task)",
						},
						"date": map[string]interface{}{
							"type":        "string",
							"description": "Data da tarefa no formato AAAA-MM-DD (create_scheduled_task)",
						},
						"hour": map[string]interface{}{
							"type":        "string",
							"description": "Horario da tarefa no formato HH:MM (create_scheduled_task)",
						},
						"type": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"call", "email", "meeting", "task", "lunch", "visit", "whatsapp"},
							"description": "Tipo da tarefa no RD Station (create_scheduled_task)",
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
	client            *openaiClient.Client
	dealSummaryPrompt string
}

// New returns a new NLP Service.
func New(client *openaiClient.Client) *Service {
	return &Service{
		client:            client,
		dealSummaryPrompt: getDealSummaryPrompt(),
	}
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
	if intent.Name == domain.IntentGetDealSummary {
		return s.formatDealSummary(ctx, intent, result)
	}
	if formatted, ok := formatStableResponse(intent.Name, result); ok {
		return formatted, nil
	}

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

func formatStableResponse(intent domain.IntentName, result interface{}) (string, bool) {
	switch intent {
	case domain.IntentGetDeals:
		deals, ok := result.([]domain.Deal)
		if !ok {
			return "", false
		}
		return formatDealsList(deals), true
	case domain.IntentGetDeal:
		deal, ok := result.(domain.Deal)
		if !ok {
			return "", false
		}
		return formatDealDetails(deal), true
	case domain.IntentGetContacts, domain.IntentGetDealContacts:
		contacts, ok := result.([]domain.Contact)
		if !ok {
			return "", false
		}
		return formatContactsList(contacts), true
	case domain.IntentGetDealActivities:
		activities, ok := result.([]domain.Activity)
		if !ok {
			return "", false
		}
		return formatActivitiesList(activities), true
	default:
		return "", false
	}
}

func formatDealsList(deals []domain.Deal) string {
	if len(deals) == 0 {
		return "Não encontrei negociações com os filtros informados."
	}
	lines := []string{fmt.Sprintf("Encontrei %d %s:", len(deals), pluralize(len(deals), "negociação", "negociações"))}
	displayDeals := deals
	if len(displayDeals) > maxDealsPerResponse {
		displayDeals = displayDeals[:maxDealsPerResponse]
	}
	for i, deal := range displayDeals {
		lines = append(lines,
			fmt.Sprintf("%d.", i+1),
			"Nome: "+valueOrFallback(deal.Name, "Sem nome"),
			"Etapa: "+valueOrFallback(deal.Stage.Name, "Não informada"),
			"Contato: "+valueOrFallback(firstDealContactName(deal), "Não informado"),
			"Responsavel: "+valueOrFallback(deal.Owner.Name, "Não informado"),
		)
	}
	if len(deals) > maxDealsPerResponse {
		lines = append(lines, fmt.Sprintf("Mostrei as %d negociações mais recentes. Existem mais %d negociações que não foram exibidas por limitação de tamanho da resposta.", maxDealsPerResponse, len(deals)-maxDealsPerResponse))
	}
	return strings.Join(lines, "\n")
}

func firstDealContactName(deal domain.Deal) string {
	for _, contact := range deal.Contacts {
		if strings.TrimSpace(contact.Name) != "" {
			return contact.Name
		}
	}
	return ""
}

func formatDealDetails(deal domain.Deal) string {
	lines := []string{
		fmt.Sprintf("*%s*", valueOrFallback(deal.Name, "Negociação sem nome")),
		"Etapa: " + valueOrFallback(deal.Stage.Name, "Não informada"),
		"Responsável: " + valueOrFallback(deal.Owner.Name, "Não informado"),
	}
	if len(deal.Contacts) == 0 {
		lines = append(lines, "Contatos vinculados: nenhum contato informado.")
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "Contatos vinculados:")
	for i, contact := range deal.Contacts {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, valueOrFallback(contact.Name, "Contato sem nome")))
	}
	return strings.Join(lines, "\n")
}

func formatContactsList(contacts []domain.Contact) string {
	if len(contacts) == 0 {
		return "Não encontrei contatos com os filtros informados."
	}
	lines := []string{fmt.Sprintf("Encontrei %d %s:", len(contacts), pluralize(len(contacts), "contato", "contatos"))}
	for i, contact := range contacts {
		lines = append(lines,
			fmt.Sprintf("%d. *%s*", i+1, valueOrFallback(contact.Name, "Contato sem nome")),
			"E-mail: "+valueOrFallback(contact.Email, "Não informado"),
			"Telefone: "+valueOrFallback(contact.Phone, "Não informado"),
		)
	}
	return strings.Join(lines, "\n")
}

func formatActivitiesList(activities []domain.Activity) string {
	if len(activities) == 0 {
		return "A negociação não possui anotações registradas."
	}
	lines := []string{fmt.Sprintf("Encontrei %d %s:", len(activities), pluralize(len(activities), "anotação", "anotações"))}
	for i, activity := range activities {
		lines = append(lines,
			fmt.Sprintf("%d. Data: %s", i+1, formatDisplayDate(activity.Date)),
			"Texto: "+valueOrFallback(activity.Text, "Sem texto informado"),
		)
	}
	return strings.Join(lines, "\n")
}

func valueOrFallback(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func pluralize(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

func formatDisplayDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Não informada"
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("02/01/2006")
		}
	}
	return value
}

func (s *Service) formatDealSummary(ctx context.Context, intent domain.Intent, result interface{}) (string, error) {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("nlp.formatDealSummary marshal result: %w", err)
	}

	prompt := fmt.Sprintf(`Pedido do usuário: "%s"

%s

Dados da negociação (JSON):
%s`,
		intent.RawText,
		s.dealSummaryPrompt,
		string(resultJSON),
	)

	resp, err := s.client.ChatCompletion(ctx, openaiClient.ChatCompletionRequest{
		Messages: []openaiClient.ChatMessage{
			{Role: "system", Content: "Você é um assistente de CRM que escreve resumos executivos de negociações comerciais para WhatsApp."},
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		return "", fmt.Errorf("nlp.formatDealSummary openai: %w", err)
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == nil {
		return "", fmt.Errorf("nlp.formatDealSummary: empty response from OpenAI")
	}

	return *resp.Choices[0].Message.Content, nil
}

func getDealSummaryPrompt() string {
	if custom := strings.TrimSpace(os.Getenv("DEAL_SUMMARY_PROMPT_TEMPLATE")); custom != "" {
		return custom + "\n\n" + dealSummaryGuardrails
	}
	return defaultDealSummaryPrompt + "\n\n" + dealSummaryGuardrails
}

func (s *Service) TranscribeAudio(ctx context.Context, audio []byte, filename string) (string, error) {
	return s.client.TranscribeAudio(ctx, audio, filename)
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
