# =============================================================================
# RDS MySQL Module — Shopping Cart Persistence Layer
# =============================================================================
# MySQL 8.0 on db.t3.micro (Free Tier eligible)
# Private placement in default VPC, accessible only from ECS security group
# =============================================================================

# DB Subnet Group — requires ≥2 subnets in different AZs
resource "aws_db_subnet_group" "mysql" {
  name       = "${var.service_name}-db-subnet-group"
  subnet_ids = var.subnet_ids

  tags = {
    Name = "${var.service_name}-db-subnet-group"
  }
}

# Security Group — only ECS tasks can reach port 3306
resource "aws_security_group" "mysql" {
  name        = "${var.service_name}-mysql-sg"
  description = "Allow MySQL access from ECS tasks only"
  vpc_id      = var.vpc_id

  ingress {
    description     = "MySQL from ECS"
    from_port       = 3306
    to_port         = 3306
    protocol        = "tcp"
    security_groups = [var.ecs_security_group_id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "${var.service_name}-mysql-sg"
  }
}

# Custom parameter group for connection tuning
resource "aws_db_parameter_group" "mysql" {
  name   = "${var.service_name}-mysql-params"
  family = "mysql8.0"

  parameter {
    name  = "max_connections"
    value = "150"
  }

  parameter {
    name  = "innodb_lock_wait_timeout"
    value = "5"
  }

  tags = {
    Name = "${var.service_name}-mysql-params"
  }
}

# RDS MySQL Instance
resource "aws_db_instance" "mysql" {
  identifier     = "${var.service_name}-mysql"
  engine         = "mysql"
  engine_version = "8.0"
  instance_class = "db.t3.micro"

  # Storage
  allocated_storage     = 20
  max_allocated_storage = 100
  storage_type          = "gp2"
  storage_encrypted     = false

  # Database config
  db_name  = var.db_name
  username = var.db_username
  password = var.db_password
  port     = 3306

  # Network — same VPC as ECS, not publicly accessible
  db_subnet_group_name   = aws_db_subnet_group.mysql.name
  vpc_security_group_ids = [aws_security_group.mysql.id]
  publicly_accessible    = false
  multi_az               = false

  # Assignment-specific: skip snapshot, no deletion protection
  skip_final_snapshot    = true
  deletion_protection    = false
  backup_retention_period = 0

  # Disable enhanced monitoring (not needed for assignment)
  monitoring_interval          = 0
  performance_insights_enabled = false

  parameter_group_name = aws_db_parameter_group.mysql.name

  tags = {
    Name = "${var.service_name}-mysql"
  }
}
