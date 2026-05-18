output "webhook_url" {
  description = "Register this URL in Meta for Developers."
  value       = "${trimsuffix(aws_apigatewayv2_stage.default.invoke_url, "/")}/webhook"
}

output "lambda_arn" {
  description = "Lambda function ARN."
  value       = aws_lambda_function.workflow.arn
}

output "dynamodb_table_name" {
  description = "DynamoDB table used for conversation history."
  value       = aws_dynamodb_table.conversations.name
}
