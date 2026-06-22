data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

data "aws_ssm_parameter" "ecs_ami" {
  name = "/aws/service/ecs/optimized-ami/amazon-linux-2/recommended/image_id"
}

resource "aws_cloudwatch_log_group" "evolution" {
  name              = "/ecs/${local.app_name}-evolution"
  retention_in_days = 7
}

resource "aws_security_group" "evolution" {
  name        = "${local.app_name}-evolution-sg"
  description = "Public HTTP access for the admin panel."
  vpc_id      = data.aws_vpc.default.id

  ingress {
    description = "Admin panel HTTP"
    from_port   = 3002
    to_port     = 3002
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    description = "Outbound access"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_ecs_cluster" "evolution" {
  name = "${local.app_name}-evolution"
}

resource "aws_iam_role" "ecs_instance" {
  name = "${local.app_name}-ecs-instance-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Service = "ec2.amazonaws.com"
      }
      Action = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "ecs_instance" {
  role       = aws_iam_role.ecs_instance.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonEC2ContainerServiceforEC2Role"
}

resource "aws_iam_instance_profile" "ecs_instance" {
  name = "${local.app_name}-ecs-instance-profile"
  role = aws_iam_role.ecs_instance.name
}

resource "aws_iam_role" "ecs_task_execution" {
  name = "${local.app_name}-ecs-task-execution-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Service = "ecs-tasks.amazonaws.com"
      }
      Action = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "ecs_task_execution" {
  role       = aws_iam_role.ecs_task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_instance" "evolution" {
  ami                         = data.aws_ssm_parameter.ecs_ami.value
  instance_type               = var.evolution_ecs_instance_type
  subnet_id                   = data.aws_subnets.default.ids[0]
  vpc_security_group_ids      = [aws_security_group.evolution.id]
  iam_instance_profile        = aws_iam_instance_profile.ecs_instance.name
  associate_public_ip_address = true

  user_data = <<-EOF
    #!/bin/bash
    echo "ECS_CLUSTER=${aws_ecs_cluster.evolution.name}" >> /etc/ecs/ecs.config
  EOF

  root_block_device {
    volume_size = var.evolution_root_volume_size
    volume_type = "gp3"
    encrypted   = true
  }

  tags = {
    Name = "${local.app_name}-evolution"
  }

  depends_on = [aws_iam_role_policy_attachment.ecs_instance]
}

resource "aws_eip" "evolution" {
  domain   = "vpc"
  instance = aws_instance.evolution.id

  tags = {
    Name = "${local.app_name}-evolution"
  }
}

resource "aws_ecr_repository" "admin" {
  name         = "${local.app_name}-admin"
  force_delete = true

  image_scanning_configuration {
    scan_on_push = true
  }
}

resource "aws_ecr_lifecycle_policy" "admin" {
  repository = aws_ecr_repository.admin.name

  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Keep only the latest admin image."
      selection = {
        tagStatus   = "any"
        countType   = "imageCountMoreThan"
        countNumber = 1
      }
      action = {
        type = "expire"
      }
    }]
  })
}

resource "null_resource" "admin_image" {
  triggers = {
    source_hash = local.admin_source_hash
    api_url     = local.api_base_url
    image_url   = aws_ecr_repository.admin.repository_url
  }

  provisioner "local-exec" {
    interpreter = ["PowerShell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command"]
    command     = <<-EOT
      $ErrorActionPreference = "Stop"
      aws ecr get-login-password --region ${var.aws_region} | docker login --username AWS --password-stdin ${local.ecr_registry}
      docker build --build-arg NEXT_PUBLIC_API_URL="" --build-arg ADMIN_API_INTERNAL_URL="${local.api_base_url}" -t "${aws_ecr_repository.admin.repository_url}:latest" "${abspath("${path.module}/../admin")}"
      docker push "${aws_ecr_repository.admin.repository_url}:latest"
    EOT
  }

  depends_on = [aws_ecr_lifecycle_policy.admin]
}

resource "aws_ecs_task_definition" "evolution" {
  family                   = "${local.app_name}-evolution"
  requires_compatibilities = ["EC2"]
  network_mode             = "bridge"
  cpu                      = "256"
  memory                   = "256"
  execution_role_arn       = aws_iam_role.ecs_task_execution.arn

  container_definitions = jsonencode([
    {
      name              = "admin"
      image             = "${aws_ecr_repository.admin.repository_url}:latest"
      essential         = true
      memoryReservation = 128
      portMappings = [
        { containerPort = 3002, hostPort = 3002, protocol = "tcp" }
      ]
      environment = [
        { name = "NODE_ENV", value = "production" },
        { name = "PORT", value = "3002" },
        { name = "ADMIN_API_INTERNAL_URL", value = local.api_base_url }
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.evolution.name
          awslogs-region        = var.aws_region
          awslogs-stream-prefix = "admin"
        }
      }
    }
  ])

  depends_on = [
    null_resource.admin_image,
    aws_iam_role_policy_attachment.ecs_task_execution
  ]
}

resource "aws_ecs_service" "evolution" {
  name                               = "${local.app_name}-evolution"
  cluster                            = aws_ecs_cluster.evolution.id
  task_definition                    = aws_ecs_task_definition.evolution.arn
  desired_count                      = 1
  launch_type                        = "EC2"
  deployment_minimum_healthy_percent = 0
  deployment_maximum_percent         = 100

  depends_on = [
    aws_instance.evolution,
    aws_eip.evolution
  ]
}
