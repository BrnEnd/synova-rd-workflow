output "webhook_url" {
  description = "Meta WhatsApp webhook URL, if Meta Cloud API is used."
  value       = "${trimsuffix(aws_apigatewayv2_stage.default.invoke_url, "/")}/webhook"
}

output "api_base_url" {
  description = "HTTP API base URL."
  value       = local.api_base_url
}

output "evolution_webhook_url" {
  description = "Register this URL in Evolution API webhooks."
  value       = "${local.api_base_url}/evolution/webhook"
}

output "admin_panel_url" {
  description = "Admin panel URL."
  value       = local.effective_admin_origin
}

output "evolution_url" {
  description = "Evolution API public URL."
  value       = local.effective_evolution_url
}

output "evolution_public_ip" {
  description = "Elastic IP attached to the Evolution ECS host."
  value       = aws_eip.evolution.public_ip
}

output "evolution_api_key" {
  description = "Evolution API key used by the Lambda integration."
  value       = local.effective_evolution_key
  sensitive   = true
}

output "lambda_arn" {
  description = "Lambda function ARN."
  value       = aws_lambda_function.workflow.arn
}

output "dynamodb_table_name" {
  description = "DynamoDB table used for conversation history."
  value       = aws_dynamodb_table.conversations.name
}
