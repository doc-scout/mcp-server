# Copyright 2026 Leonan Carvalho
# SPDX-License-Identifier: AGPL-3.0-only
#
# DocScout-MCP — Kubernetes Terraform module
#
# Deploys DocScout-MCP to any Kubernetes cluster using the official
# hashicorp/kubernetes Terraform provider. Configure your kubeconfig
# (or in-cluster auth) before running `terraform apply`.
#
# Usage:
#   terraform init
#   terraform apply \
#     -var="github_token=github_pat_..." \
#     -var="github_org=my-org"

terraform {
  required_version = ">= 1.5"

  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.27"
    }
  }
}

locals {
  app_name  = "docscout-mcp"
  image     = "${var.image_repository}:${var.image_tag}"

  common_labels = {
    "app.kubernetes.io/name"       = local.app_name
    "app.kubernetes.io/managed-by" = "terraform"
  }
}

# ── Namespace ─────────────────────────────────────────────────────────────────

resource "kubernetes_namespace" "this" {
  metadata {
    name   = var.namespace
    labels = local.common_labels
  }
}

# ── Secret ────────────────────────────────────────────────────────────────────

resource "kubernetes_secret" "this" {
  metadata {
    name      = "${local.app_name}-secrets"
    namespace = kubernetes_namespace.this.metadata[0].name
    labels    = local.common_labels
  }

  data = {
    GITHUB_TOKEN           = var.github_token
    MCP_HTTP_BEARER_TOKEN  = var.mcp_http_bearer_token
    GITHUB_WEBHOOK_SECRET  = var.github_webhook_secret
  }
}

# ── ConfigMap ─────────────────────────────────────────────────────────────────

resource "kubernetes_config_map" "this" {
  metadata {
    name      = "${local.app_name}-config"
    namespace = kubernetes_namespace.this.metadata[0].name
    labels    = local.common_labels
  }

  data = {
    GITHUB_ORG       = var.github_org
    SCAN_INTERVAL    = var.scan_interval
    HTTP_ADDR        = ":8080"
    DATABASE_URL     = var.database_url
    SCAN_CONTENT     = tostring(var.scan_content)
    MAX_CONTENT_SIZE = tostring(var.max_content_size)
    SCAN_FILES       = var.scan_files
    SCAN_DIRS        = var.scan_dirs
    EXTRA_REPOS      = var.extra_repos
    REPO_TOPICS      = var.repo_topics
    REPO_REGEX       = var.repo_regex
  }
}

# ── PersistentVolumeClaim ─────────────────────────────────────────────────────

resource "kubernetes_persistent_volume_claim" "this" {
  metadata {
    name      = "${local.app_name}-data"
    namespace = kubernetes_namespace.this.metadata[0].name
    labels    = local.common_labels
  }

  spec {
    access_modes       = ["ReadWriteOnce"]
    storage_class_name = var.pvc_storage_class != "" ? var.pvc_storage_class : null

    resources {
      requests = {
        storage = var.pvc_size
      }
    }
  }
}

# ── Deployment ────────────────────────────────────────────────────────────────

resource "kubernetes_deployment" "this" {
  metadata {
    name      = local.app_name
    namespace = kubernetes_namespace.this.metadata[0].name
    labels    = local.common_labels
  }

  spec {
    replicas = 1

    selector {
      match_labels = {
        "app.kubernetes.io/name" = local.app_name
      }
    }

    template {
      metadata {
        labels = local.common_labels
      }

      spec {
        security_context {
          run_as_non_root = true
          run_as_user     = 1000
        }

        container {
          name              = local.app_name
          image             = local.image
          image_pull_policy = "Always"

          port {
            container_port = 8080
            protocol       = "TCP"
          }

          env_from {
            config_map_ref {
              name = kubernetes_config_map.this.metadata[0].name
            }
          }

          env_from {
            secret_ref {
              name = kubernetes_secret.this.metadata[0].name
            }
          }

          volume_mount {
            name       = "data"
            mount_path = "/data"
          }

          resources {
            requests = {
              cpu    = var.resources_requests_cpu
              memory = var.resources_requests_memory
            }
            limits = {
              cpu    = var.resources_limits_cpu
              memory = var.resources_limits_memory
            }
          }

          liveness_probe {
            http_get {
              path = "/metrics"
              port = 8080
            }
            initial_delay_seconds = 15
            period_seconds        = 30
            timeout_seconds       = 5
            failure_threshold     = 3
          }

          readiness_probe {
            http_get {
              path = "/metrics"
              port = 8080
            }
            initial_delay_seconds = 10
            period_seconds        = 10
            timeout_seconds       = 3
            failure_threshold     = 3
          }
        }

        volume {
          name = "data"
          persistent_volume_claim {
            claim_name = kubernetes_persistent_volume_claim.this.metadata[0].name
          }
        }
      }
    }
  }
}

# ── Service ───────────────────────────────────────────────────────────────────

resource "kubernetes_service" "this" {
  metadata {
    name      = local.app_name
    namespace = kubernetes_namespace.this.metadata[0].name
    labels    = local.common_labels
  }

  spec {
    type = var.service_type

    selector = {
      "app.kubernetes.io/name" = local.app_name
    }

    port {
      port        = 80
      target_port = 8080
      protocol    = "TCP"
    }
  }
}

# ── Ingress (optional) ────────────────────────────────────────────────────────

resource "kubernetes_ingress_v1" "this" {
  count = var.ingress_enabled ? 1 : 0

  metadata {
    name      = local.app_name
    namespace = kubernetes_namespace.this.metadata[0].name
    labels    = local.common_labels
    annotations = {
      "nginx.ingress.kubernetes.io/proxy-read-timeout" = "3600"
      "nginx.ingress.kubernetes.io/proxy-send-timeout" = "3600"
    }
  }

  spec {
    ingress_class_name = var.ingress_class_name

    rule {
      host = var.ingress_host

      http {
        path {
          path      = "/"
          path_type = "Prefix"

          backend {
            service {
              name = kubernetes_service.this.metadata[0].name
              port {
                number = 80
              }
            }
          }
        }
      }
    }
  }
}
