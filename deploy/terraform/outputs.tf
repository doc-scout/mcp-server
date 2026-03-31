# Copyright 2026 Leonan Carvalho
# SPDX-License-Identifier: AGPL-3.0-only

output "namespace" {
  description = "Kubernetes namespace where DocScout-MCP was deployed."
  value       = kubernetes_namespace.this.metadata[0].name
}

output "service_name" {
  description = "Kubernetes Service name."
  value       = kubernetes_service.this.metadata[0].name
}

output "service_cluster_ip" {
  description = "ClusterIP assigned to the Service."
  value       = kubernetes_service.this.spec[0].cluster_ip
}

output "ingress_host" {
  description = "Ingress hostname (empty if ingress is disabled)."
  value       = var.ingress_enabled ? var.ingress_host : ""
}

output "deployment_image" {
  description = "Docker image used by the Deployment."
  value       = "${var.image_repository}:${var.image_tag}"
}
