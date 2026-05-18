terraform {
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

locals {
  app_name       = "synova-rd-workflow"
  table_name     = "synova-rd-workflow-conversations"
  lambda_zip     = abspath("${path.module}/${var.function_zip}")
  lambda_log_arn = "${aws_cloudwatch_log_group.lambda.arn}:*"
}

resource "aws_dynamodb_table" "conversations" {
  name         = local.table_name
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "PK"
  range_key    = "SK"

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
  memory_size      = 256
  role             = aws_iam_role.lambda.arn

  environment {
    variables = {
      WHATSAPP_ACCESS_TOKEN    = var.whatsapp_access_token
      WHATSAPP_PHONE_NUMBER_ID = var.whatsapp_phone_number_id
      WHATSAPP_VERIFY_TOKEN    = var.whatsapp_verify_token
      WHATSAPP_APP_SECRET      = var.whatsapp_app_secret
      OPENAI_API_KEY           = var.openai_api_key
      OPENAI_MODEL             = var.openai_model
      RDSTATION_TOKEN          = var.rdstation_token
      DYNAMODB_TABLE_NAME      = aws_dynamodb_table.conversations.name
      AWS_REGION_APP           = var.aws_region
      LOG_LEVEL                = var.log_level
      NLP_CONTEXT_WINDOW       = tostring(var.nlp_context_window)
    }
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

resource "aws_apigatewayv2_stage" "default" {
  api_id      = aws_apigatewayv2_api.webhook.id
  name        = "$default"
  auto_deploy = true
}

resource "aws_lambda_permission" "api_gateway" {
  statement_id  = "AllowAPIGatewayInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.workflow.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.webhook.execution_arn}/*/*"
}
