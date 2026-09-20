terraform {
  required_version = ">= 1.5.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

variable "region" {
  type    = string
  default = "eu-central-1"
}

variable "test_id" {
  type = string
}

variable "generation" {
  type = string
}

variable "autoscaling_service_linked_role_name" {
  type    = string
  default = "AWSServiceRoleForAutoScaling_Convergenci"
}

variable "instance_warmup_seconds" {
  type    = number
  default = 1
}

provider "aws" {
  region = var.region
}

data "aws_vpc" "default" {
  default = true
}

data "aws_caller_identity" "current" {}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

data "aws_ami" "amazon_linux" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["al2023-ami-*-x86_64"]
  }

  filter {
    name   = "root-device-type"
    values = ["ebs"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

resource "aws_launch_template" "app" {
  name_prefix   = "${var.test_id}-"
  image_id      = data.aws_ami.amazon_linux.id
  instance_type = "t3.micro"

  tags = {
    "project-scope"       = "convergenci"
    "convergenci-test-id" = var.test_id
    Name                  = "${var.test_id}-launch-template"
  }

  user_data = base64encode(<<-EOF
    #!/bin/sh
    echo "convergenci test ${var.test_id} generation ${var.generation}" >/etc/convergenci-generation
    sleep ${var.instance_warmup_seconds}
  EOF
  )

  tag_specifications {
    resource_type = "instance"

    tags = {
      "project-scope"       = "convergenci"
      Name                  = "${var.test_id}-instance"
      "convergenci-test-id" = var.test_id
    }
  }
}

resource "aws_autoscaling_group" "app" {
  name                      = "${var.test_id}-asg"
  min_size                  = 1
  max_size                  = 1
  desired_capacity          = 1
  default_instance_warmup   = var.instance_warmup_seconds
  health_check_grace_period = 10
  vpc_zone_identifier       = [data.aws_subnets.default.ids[0]]
  service_linked_role_arn   = "arn:aws:iam::${data.aws_caller_identity.current.account_id}:role/aws-service-role/autoscaling.amazonaws.com/${var.autoscaling_service_linked_role_name}"
  wait_for_capacity_timeout = "2m"

  launch_template {
    id      = aws_launch_template.app.id
    version = aws_launch_template.app.latest_version
  }

  instance_refresh {
    strategy = "Rolling"

    preferences {
      min_healthy_percentage = 0
      instance_warmup        = var.instance_warmup_seconds
      skip_matching          = false
    }
  }

  tag {
    key                 = "convergenci_rotation"
    value               = var.generation
    propagate_at_launch = true
  }

  tag {
    key                 = "convergenci-test-id"
    value               = var.test_id
    propagate_at_launch = true
  }

  tag {
    key                 = "project-scope"
    value               = "convergenci"
    propagate_at_launch = true
  }
}

output "asg_name" {
  value = aws_autoscaling_group.app.name
}
