{{/*
Copyright 2026 Leonan Carvalho
SPDX-License-Identifier: AGPL-3.0-only
*/}}

{{/*
Expand the name of the chart.
*/}}
{{- define "docscout-mcp.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this
(by the DNS naming spec). If the release name contains the chart name it will
be used as the full name.
*/}}
{{- define "docscout-mcp.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart label (used in the "helm.sh/chart" label).
*/}}
{{- define "docscout-mcp.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels applied to all resources.
*/}}
{{- define "docscout-mcp.labels" -}}
helm.sh/chart: {{ include "docscout-mcp.chart" . }}
{{ include "docscout-mcp.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels — used in matchLabels and pod template labels.
*/}}
{{- define "docscout-mcp.selectorLabels" -}}
app.kubernetes.io/name: {{ include "docscout-mcp.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Name of the ConfigMap.
*/}}
{{- define "docscout-mcp.configmapName" -}}
{{- printf "%s-config" (include "docscout-mcp.fullname" .) }}
{{- end }}

{{/*
Name of the Secret.
*/}}
{{- define "docscout-mcp.secretName" -}}
{{- printf "%s-secrets" (include "docscout-mcp.fullname" .) }}
{{- end }}

{{/*
Name of the PersistentVolumeClaim.
*/}}
{{- define "docscout-mcp.pvcName" -}}
{{- printf "%s-data" (include "docscout-mcp.fullname" .) }}
{{- end }}
