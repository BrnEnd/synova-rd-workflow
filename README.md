# Synova RD Workflow

[![Go](https://img.shields.io/badge/Go-1.24-00ADD8.svg)](https://go.dev/)
[![Next.js](https://img.shields.io/badge/Next.js-15-000000.svg)](https://nextjs.org/)
[![AWS Lambda](https://img.shields.io/badge/AWS-Lambda-FF9900.svg)](https://aws.amazon.com/lambda/)
[![Terraform](https://img.shields.io/badge/Terraform-1.6+-844FBA.svg)](https://www.terraform.io/)

Backend em Go e painel administrativo em Next.js para atendimento via WhatsApp, consulta ao RD Station CRM, gerenciamento de allowlist, perfis de acesso e alertas operacionais.

## Patch Notes

### 2026-06-11 - Criacao de tarefas no RD Station

- O fluxo de criacao de tarefas agora remove valores padrao inferidos pelo NLP quando o usuario nao informou assunto, data ou horario explicitamente.
- Na etapa de confirmacao, o usuario pode corrigir assunto, data, horario ou negociacao sem reiniciar a conversa.
- Datas relativas como `hoje`, `amanha` e `agora` usam o fuso `America/Sao_Paulo`.
- Horarios em formatos como `14h`, `14h30` e `14:30` sao normalizados antes da criacao no RD Station.
- A resposta de confirmacao ficou mais segura: se o usuario nao responder claramente `sim` ou `nao`, o bot pede uma confirmacao ou orienta como ajustar os campos.

## O Que Este Projeto Faz

`synova-rd-workflow` e o stack principal do bot da Silmax/Synova. Ele conecta uma instancia WhatsApp real pela Evolution API, recebe mensagens dos usuarios, interpreta a intencao com OpenAI, executa consultas ou operacoes no RD Station CRM e responde pelo WhatsApp.

Tambem inclui um painel administrativo para cadastrar colaboradores, liberar numeros na allowlist, acompanhar a conexao do WhatsApp e configurar alertas.

## Fluxo Principal

```text
Usuario no WhatsApp
  -> Evolution API
  -> POST /evolution/webhook
  -> Lambda / backend Go
  -> validacao de allowlist e perfil
  -> OpenAI identifica a intencao
  -> RD Station CRM executa a consulta/acao
  -> OpenAI formata a resposta
  -> Evolution API envia mensagem
  -> Usuario recebe no WhatsApp
```

## Estrutura Do Projeto

```text
.
|-- cmd/
|   |-- api/                 # API HTTP local usada em desenvolvimento
|   |-- lambda/              # Entrypoint AWS Lambda usado em producao
|   |-- genkey/              # Utilitario para gerar chave JWT do admin
|   `-- admin-test-alert/    # Utilitario para testar alertas
|-- config/                  # Leitura e validacao de variaveis de ambiente
|-- internal/
|   |-- client/              # Clientes OpenAI, RD Station, Evolution e WhatsApp Meta
|   |-- domain/              # Tipos de dominio: intent, deal, contact, admin, session
|   |-- handler/             # Rotas HTTP/webhooks/admin
|   |-- middleware/          # Auth, CORS e seguranca do admin
|   |-- service/             # Regras de negocio e orquestracao
|   `-- store/               # Persistencia em DynamoDB
|-- admin/                   # Painel administrativo Next.js
|-- docs/                    # Documentos tecnicos e contratos
|-- migrations/              # Artefatos auxiliares de evolucao local
|-- terraform/               # Infraestrutura AWS
|-- docker-compose.dev.yml   # Ambiente local completo
|-- Dockerfile               # Build do backend Go
|-- Makefile                 # Comandos de build, teste e deploy
`-- .env.example             # Modelo de configuracao local
```

## Servicos E Responsabilidades

| Servico / modulo | Responsabilidade |
| --- | --- |
| `cmd/lambda` | Inicializa dependencias e adapta o Gin para AWS Lambda/API Gateway. |
| `cmd/api` | Roda a mesma aplicacao como servidor HTTP local. |
| `handler/evolution` | Recebe eventos da Evolution API, extrai mensagens de texto, aplica allowlist, chama NLP/RD e envia resposta. |
| `handler/whatsapp` | Handler alternativo para WhatsApp Cloud API da Meta. |
| `handler/admin` | Rotas do painel admin: login, colaboradores, allowlist, alertas, status/QR Code WhatsApp. |
| `service/nlp` | Contrato com OpenAI: identifica intents e formata respostas finais. |
| `service/intent_router` | Decide qual operacao do CRM executar para cada intent. |
| `service/rdstation` | Regras de consulta e escrita no RD Station, incluindo filtro por responsavel/perfil. |
| `client/rdstation` | Cliente HTTP baixo nivel da API RD Station CRM. |
| `client/evolution` | Envio de mensagens, status de conexao, QR Code e chamadas administrativas da Evolution API. |
| `service/admin` | Cadastro de colaboradores, allowlist, perfis, alertas e regras de autorizacao. |
| `store/admin` | Persistencia admin em DynamoDB usando PKs como `ADMIN#COLLABORATORS` e `ADMIN#ALLOWLIST`. |
| `service/conversation` | Gerencia sessao e historico recente por telefone. |
| `store/conversation` | Persiste sessoes e mensagens no DynamoDB com TTL. |
| `admin/` | UI Next.js do painel administrativo. |
| `terraform/` | Cria recursos AWS: Lambda, API Gateway, DynamoDB, CloudWatch, ECS/EC2 da Evolution API e rede. |

## Intencoes Do Bot

O NLP retorna intents estruturadas. As principais sao:

| Intent | Uso |
| --- | --- |
| `get_deals` | Lista negociacoes por nome, etapa, status ou responsavel. |
| `get_deal` | Busca detalhes de uma negociacao especifica. |
| `get_deal_contacts` | Lista contatos de uma negociacao. |
| `get_contacts` | Consulta contatos. |
| `create_contact` | Cria contato. |
| `create_deal` | Cria negociacao. |
| `update_deal` | Atualiza dados de uma negociacao. |
| `move_deal_stage` | Move negociacao de etapa. |
| `update_contact` | Atualiza contato. |
| `associate_contact_to_deal` | Vincula contato a negociacao. |
| `delete_deal` | Nao exclui; orienta o usuario a seguir o fluxo de aprovacao do RD Station. |

## Regras De Acesso

O bot so processa mensagens de telefones ativos na allowlist. O cadastro de colaborador sozinho nao libera acesso.

Perfis:

| Perfil | Permissao |
| --- | --- |
| `director` | Pode consultar e operar todos os negocios disponiveis para o token do RD Station. |
| `supervisor` | Pode consultar e operar os proprios negocios e os negocios da equipe. |
| `seller` | Pode consultar e operar somente negocios sob sua responsabilidade. |

A relacao de equipe e definida no painel admin: vendedores apontam para um colaborador supervisor. O filtro de acesso usa o `rdstation_id` cadastrado nos colaboradores e compara com o responsavel da negociacao no RD Station.

## Variaveis Importantes

| Variavel | Descricao |
| --- | --- |
| `OPENAI_API_KEY` | Chave da OpenAI usada pelo NLP. |
| `OPENAI_MODEL` | Modelo usado para identificar intents e formatar respostas. |
| `RDSTATION_TOKEN` | Token da API RD Station CRM. |
| `DYNAMODB_TABLE_NAME` | Tabela unica para conversas e dados admin. |
| `EVOLUTION_BASE_URL` | URL base da Evolution API. |
| `EVOLUTION_API_KEY` | Chave administrativa da Evolution API. |
| `EVOLUTION_INSTANCE` | Nome da instancia WhatsApp na Evolution. |
| `EVOLUTION_SEND_DELAY` | Atraso artificial antes de enviar mensagens; em Lambda deve ficar `0s`. |
| `ADMIN_EMAIL` | E-mail inicial do admin. |
| `ADMIN_JWT_PRIVATE_KEY` | Chave privada base64 para assinar sessoes do admin. |

Veja `.env.example` para a lista completa.

## Execucao Local

```bash
cp .env.example .env
docker compose -f docker-compose.dev.yml up --build
```

Servicos locais:

| URL | Servico |
| --- | --- |
| `http://localhost:8080/health` | Backend Go |
| `http://localhost:3002` | Painel admin |
| `http://localhost:8081` | Evolution API |
| `http://localhost:8000` | DynamoDB Local |

## Desenvolvimento

Backend:

```bash
go mod download
go run ./cmd/api
```

Painel administrativo:

```bash
cd admin
npm install
npm run dev
```

## Testes

```bash
go test ./...
```

Para testes focados:

```bash
go test ./internal/service/rdstation ./internal/service/intent_router ./internal/handler/evolution
```

## Deploy

Gere o pacote da Lambda:

```bash
make build-lambda
```

Depois aplique a infraestrutura:

```bash
terraform -chdir=terraform init
terraform -chdir=terraform apply
```

Tambem e possivel atualizar apenas o codigo da Lambda com o `function.zip` gerado:

```bash
aws lambda update-function-code --function-name synova-rd-workflow --zip-file fileb://function.zip
```

## Observabilidade

Logs principais em producao:

| Log group | Conteudo |
| --- | --- |
| `/aws/lambda/synova-rd-workflow` | Requisicoes do backend, webhooks, erros de NLP/RD/Evolution e timeouts. |
| `/ecs/synova-rd-workflow-evolution` | Logs da Evolution API, Postgres e Redis. |

Mensagens comuns:

| Log | Significado |
| --- | --- |
| `evolution sender not allowed` | Telefone nao esta ativo na allowlist. |
| `evolution webhook ignored` | Evento nao e mensagem de usuario ou e mensagem enviada pelo proprio bot. |
| `evolution duplicate user message ignored` | Retentativa/duplicidade recente foi descartada. |
| `evolution message completed` | Mensagem processada e resposta enviada. |
| `context deadline exceeded` | A operacao passou do timeout do contexto/Lambda. |

## Seguranca

- Nao commitar `.env`, `terraform.tfvars`, `terraform.tfstate`, `function.zip`, chaves privadas ou tokens.
- Rotacionar qualquer credencial que tenha sido exposta.
- Manter `EVOLUTION_SEND_DELAY=0s` em Lambda para evitar timeouts.
- Revisar a allowlist antes de liberar usuarios finais.
- Preferir alterar segredos por variaveis de ambiente/secret manager, nao por arquivos versionados.
