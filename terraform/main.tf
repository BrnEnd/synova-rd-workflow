terraform {
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
    null = {
      source  = "hashicorp/null"
      version = "~> 3.2"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

data "aws_caller_identity" "current" {}

locals {
  app_name                              = "synova-rd-workflow"
  table_name                            = "synova-rd-workflow-conversations"
  lambda_zip                            = abspath("${path.module}/${var.function_zip}")
  lambda_log_arn                        = "${aws_cloudwatch_log_group.lambda.arn}:*"
  effective_evolution_url               = var.evolution_base_url != "" ? trimsuffix(var.evolution_base_url, "/") : "http://${aws_eip.evolution.public_ip}"
  effective_evolution_key               = var.evolution_api_key != "" ? var.evolution_api_key : random_password.evolution_api_key.result
  effective_evolution_postgres_password = var.evolution_postgres_password != "" ? var.evolution_postgres_password : random_password.evolution_postgres_password.result
  effective_admin_origin                = var.admin_origin != "" ? trimsuffix(var.admin_origin, "/") : "http://${aws_eip.evolution.public_ip}:3002"
  api_base_url                          = trimsuffix(aws_apigatewayv2_stage.default.invoke_url, "/")
  ecr_registry                          = "${data.aws_caller_identity.current.account_id}.dkr.ecr.${var.aws_region}.amazonaws.com"
  admin_source_hash = sha256(join("", [
    for file in fileset("${path.module}/../admin", "**") :
    filesha256("${path.module}/../admin/${file}")
    if !startswith(file, "node_modules/") && !startswith(file, ".next/")
  ]))
  lambda_environment = merge(
    {
      OPENAI_API_KEY       = var.openai_api_key
      OPENAI_MODEL         = var.openai_model
      RDSTATION_TOKEN      = var.rdstation_token
      DYNAMODB_TABLE_NAME  = aws_dynamodb_table.conversations.name
      AWS_REGION_APP       = var.aws_region
      LOG_LEVEL            = var.log_level
      NLP_CONTEXT_WINDOW   = tostring(var.nlp_context_window)
      ADMIN_EMAIL          = var.admin_email
      ADMIN_ORIGIN         = local.effective_admin_origin
      ADMIN_COOKIE_SECURE  = tostring(var.admin_cookie_secure)
      ALERT_CHECK_INTERVAL = var.alert_check_interval
      EVOLUTION_SEND_DELAY = var.evolution_send_delay
      EVOLUTION_INSTANCE   = var.evolution_instance
      EVOLUTION_BASE_URL   = local.effective_evolution_url
      EVOLUTION_API_KEY    = local.effective_evolution_key
    },
    var.whatsapp_access_token != "" ? { WHATSAPP_ACCESS_TOKEN = var.whatsapp_access_token } : {},
    var.whatsapp_phone_number_id != "" ? { WHATSAPP_PHONE_NUMBER_ID = var.whatsapp_phone_number_id } : {},
    var.whatsapp_verify_token != "" ? { WHATSAPP_VERIFY_TOKEN = var.whatsapp_verify_token } : {},
    var.whatsapp_app_secret != "" ? { WHATSAPP_APP_SECRET = var.whatsapp_app_secret } : {},
    var.evolution_allowed_numbers != "" ? { EVOLUTION_ALLOWED_NUMBERS = var.evolution_allowed_numbers } : {}
  )
}

resource "random_password" "evolution_api_key" {
  length  = 32
  special = false
}

resource "random_password" "evolution_postgres_password" {
  length  = 32
  special = false
}

resource "aws_dynamodb_table" "conversations" {
  name           = local.table_name
  billing_mode   = "PROVISIONED"
  read_capacity  = var.dynamodb_read_capacity
  write_capacity = var.dynamodb_write_capacity
  hash_key       = "PK"
  range_key      = "SK"

  attribute {
    name = "PK"
    type = "S"
  }

  attribute {
    name = "SK"
    type = "S"
  }

  ttl {
    attribute_name = "ttl"
    enabled        = true
  }
}

resource "aws_cloudwatch_log_group" "lambda" {
  name              = "/aws/lambda/${local.app_name}"
  retention_in_days = 7
}

resource "aws_iam_role" "lambda" {
  name = "${local.app_name}-lambda-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Service = "lambda.amazonaws.com"
      }
      Action = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy" "lambda" {
  name = "${local.app_name}-lambda-policy"
  role = aws_iam_role.lambda.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "dynamodb:GetItem",
          "dynamodb:PutItem",
          "dynamodb:Query"
        ]
        Resource = aws_dynamodb_table.conversations.arn
      },
      {
        Effect = "Allow"
        Action = [
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Resource = local.lambda_log_arn
      }
    ]
  })
}

resource "aws_lambda_function" "workflow" {
  function_name    = local.app_name
  filename         = local.lambda_zip
  source_code_hash = filebase64sha256(local.lambda_zip)
  handler          = "bootstrap"
  runtime          = "provided.al2023"
  architectures    = ["arm64"]
  timeout          = 15
  memory_size      = var.lambda_memory_size
  role             = aws_iam_role.lambda.arn

  environment {
    variables = local.lambda_environment
  }

  depends_on = [
    aws_cloudwatch_log_group.lambda,
    aws_iam_role_policy.lambda
  ]
}

resource "aws_apigatewayv2_api" "webhook" {
  name          = "${local.app_name}-webhook"
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

resource "aws_apigatewayv2_route" "default" {
  api_id    = aws_apigatewayv2_api.webhook.id
  route_key = "$default"
  target    = "integrations/${aws_apigatewayv2_integration.lambda.id}"
}

resource "aws_apigatewayv2_stage" "default" {
  api_id      = aws_apigatewayv2_api.webhook.id
  name        = "$default"
  auto_deploy = true

  default_route_settings {
    throttling_burst_limit = var.api_throttle_burst_limit
    throttling_rate_limit  = var.api_throttle_rate_limit
  }
}

resource "aws_lambda_permission" "api_gateway" {
  statement_id  = "AllowAPIGatewayInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.workflow.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.webhook.execution_arn}/*/*"
}
