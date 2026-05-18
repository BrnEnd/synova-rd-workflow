# Synova RD Workflow

[![Go](https://img.shields.io/badge/Go-1.24-00ADD8.svg)](https://go.dev/)
[![Next.js](https://img.shields.io/badge/Next.js-15-000000.svg)](https://nextjs.org/)
[![AWS Lambda](https://img.shields.io/badge/AWS-Lambda-FF9900.svg)](https://aws.amazon.com/lambda/)
[![Terraform](https://img.shields.io/badge/Terraform-1.6+-844FBA.svg)](https://www.terraform.io/)

Backend em Go e painel administrativo em Next.js para atendimento via WhatsApp, consulta ao RD Station CRM, gerenciamento de allowlist e alertas operacionais.

## Visao Geral

O projeto integra mensagens recebidas pelo WhatsApp ou Evolution API com uma camada de NLP, consulta dados no RD Station CRM e persiste contexto operacional em DynamoDB. O painel administrativo permite configurar colaboradores, alertas, allowlist e credenciais de acesso.

## Estrutura

```text
.
|-- cmd/                    # Entrypoints da API, Lambda e utilitarios
|-- config/                 # Carregamento de variaveis de ambiente
|-- internal/               # Dominio, handlers, clientes, servicos e stores
|-- migrations/             # Artefatos de evolucao local
|-- terraform/              # Infraestrutura AWS
|-- admin/                  # Painel administrativo Next.js
|-- docs/                   # Documentacao tecnica e contratos
```

## Requisitos

- Go 1.24 ou superior
- Node.js 20 ou superior
- Docker e Docker Compose
- Terraform 1.6 ou superior, para deploy AWS

## Configuracao

Crie o arquivo de ambiente local a partir do exemplo:

```bash
cp .env.example .env
```

Preencha as credenciais reais somente no `.env` local ou no gerenciador de segredos da infraestrutura. Arquivos `.env`, `.tfvars`, estados Terraform, pacotes ZIP, dependencias instaladas e builds locais estao ignorados pelo Git.

## Execucao Local

Suba API, DynamoDB Local, Evolution API e painel admin:

```bash
docker compose -f docker-compose.dev.yml up --build
```

Servicos principais:

- API: `http://localhost:8080`
- Health check: `http://localhost:8080/health`
- Painel admin: `http://localhost:3002`
- Evolution API: `http://localhost:8081`

## Regras de Acesso da Sil

O bot responde a saudacao `Oi` com a mensagem institucional da Sil para numeros autorizados. Numeros fora da allowlist sao ignorados pelo webhook.

Perfis disponiveis:

- `director`: diretoria, tambem chamado internamente de The God. Pode consultar e operar todos os negocios.
- `supervisor`: consulta e opera os proprios negocios e os negocios da equipe.
- `seller`: vendedor PJ ou PF. Consulta e opera somente negocios sob sua responsabilidade.

A equipe do supervisor e definida no painel admin em `Colaboradores`: cada vendedor deve ter o campo `Supervisor` apontando para o colaborador supervisor, por exemplo Renato. O filtro de negocios usa o `ID do usuario no RD Station` cadastrado em cada colaborador e compara com o responsavel da negociacao no RD.

Exclusao de negociacoes nao e executada pelo bot. Quando o usuario pedir para apagar/excluir/deletar um negocio, o bot informa que a exclusao deve seguir o fluxo de aprovacao do RD Station; usuarios fora da diretoria recebem bloqueio de permissao.

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

Os testes de integracao podem depender de servicos locais e variaveis de ambiente configuradas.

## Deploy

Gere o pacote Lambda:

```bash
make build-lambda
```

Execute o Terraform usando variaveis seguras fora do Git:

```bash
terraform -chdir=terraform init
terraform -chdir=terraform apply
```

## Seguranca

- Nao commitar `.env`, `terraform.tfvars`, `terraform.tfstate`, arquivos ZIP de build ou chaves privadas.
- Rotacionar qualquer credencial que ja tenha sido exposta em arquivos locais antes desta limpeza.
- Usar variaveis de ambiente ou secret manager no ambiente de producao.
- Revisar a allowlist de numeros autorizados antes de ativar o envio de mensagens.
