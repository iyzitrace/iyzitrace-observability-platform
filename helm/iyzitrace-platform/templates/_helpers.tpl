{{/*
Expand the name of the chart.
*/}}
{{- define "iyzitrace.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "iyzitrace.fullname" -}}
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
Create chart name and version as used by the chart label.
*/}}
{{- define "iyzitrace.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "iyzitrace.labels" -}}
helm.sh/chart: {{ include "iyzitrace.chart" . }}
{{ include "iyzitrace.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: iyzitrace-platform
{{- end }}

{{/*
Selector labels
*/}}
{{- define "iyzitrace.selectorLabels" -}}
app.kubernetes.io/name: {{ include "iyzitrace.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Component labels — call with (dict "context" $ "component" "name")
*/}}
{{- define "iyzitrace.componentLabels" -}}
{{ include "iyzitrace.labels" .context }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/*
Component selector labels
*/}}
{{- define "iyzitrace.componentSelectorLabels" -}}
{{ include "iyzitrace.selectorLabels" .context }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/*
S3 credentials secret name
*/}}
{{- define "iyzitrace.s3SecretName" -}}
{{- if .Values.global.s3SecretName -}}
{{ .Values.global.s3SecretName }}
{{- else -}}
{{ include "iyzitrace.fullname" . }}-s3-credentials
{{- end -}}
{{- end }}

{{/*
TLS secret name
*/}}
{{- define "iyzitrace.tlsSecretName" -}}
{{- if .Values.global.tlsSecretName -}}
{{ .Values.global.tlsSecretName }}
{{- else -}}
{{ include "iyzitrace.fullname" . }}-tls
{{- end -}}
{{- end }}

{{/*
SeaweedFS internal endpoint
*/}}
{{- define "iyzitrace.seaweedfsEndpoint" -}}
{{- $seaweedName := default "seaweedfs" .Values.seaweedfs.fullnameOverride -}}
{{ printf "%s-s3:8333" $seaweedName }}
{{- end }}

{{/*
Tempo internal endpoint (distributed: distributor)
*/}}
{{- define "iyzitrace.tempoEndpoint" -}}
{{- $tempoName := default "tempo" .Values.tempo.fullnameOverride -}}
{{ printf "%s-distributor:4317" $tempoName }}
{{- end }}

{{/*
Tempo query endpoint
*/}}
{{- define "iyzitrace.tempoQueryEndpoint" -}}
{{- $tempoName := default "tempo" .Values.tempo.fullnameOverride -}}
{{ printf "%s-query-frontend:3200" $tempoName }}
{{- end }}

{{/*
Tempo OTLP/HTTP endpoint
*/}}
{{- define "iyzitrace.tempoHttpEndpoint" -}}
{{- $tempoName := default "tempo" .Values.tempo.fullnameOverride -}}
{{ printf "%s-distributor:4318" $tempoName }}
{{- end }}

{{/*
Loki internal endpoint
*/}}
{{- define "iyzitrace.lokiEndpoint" -}}
{{- $lokiName := default "loki" .Values.loki.fullnameOverride -}}
{{ printf "%s-gateway:80" $lokiName }}
{{- end }}

{{/*
Prometheus internal endpoint
*/}}
{{- define "iyzitrace.prometheusServiceName" -}}
{{- $promValues := .Values.prometheus | default dict -}}
{{- $serverValues := $promValues.server | default dict -}}
{{- $serverFullname := $serverValues.fullnameOverride | default "" -}}
{{- if $serverFullname -}}
{{- $serverFullname -}}
{{- else -}}
{{- $name := $promValues.nameOverride | default "prometheus" -}}
{{- $serverName := $serverValues.name | default "server" -}}
{{- if contains $name .Release.Name -}}
{{- printf "%s-%s" .Release.Name $serverName -}}
{{- else -}}
{{- printf "%s-%s-%s" .Release.Name $name $serverName -}}
{{- end -}}
{{- end -}}
{{- end }}

{{- define "iyzitrace.prometheusEndpoint" -}}
{{ printf "%s:80" (include "iyzitrace.prometheusServiceName" .) }}
{{- end }}

{{/*
Thanos Query internal endpoint
*/}}
{{- define "iyzitrace.thanosQueryEndpoint" -}}
{{- $thanosName := default "thanos" .Values.thanos.fullnameOverride -}}
{{ printf "%s-query-frontend:9090" $thanosName }}
{{- end }}

{{/*
Service names for custom components
*/}}
{{- define "iyzitrace.lawrenceName" -}}
{{ include "iyzitrace.fullname" . }}-lawrence
{{- end }}

{{- define "iyzitrace.authServiceName" -}}
{{ include "iyzitrace.fullname" . }}-auth-service
{{- end }}

{{- define "iyzitrace.inventoryServiceName" -}}
{{ include "iyzitrace.fullname" . }}-inventory-service
{{- end }}

{{- define "iyzitrace.nginxName" -}}
{{ include "iyzitrace.fullname" . }}-nginx
{{- end }}

{{/*
SeaweedFS master endpoint
*/}}
{{- define "iyzitrace.seaweedfsMasterEndpoint" -}}
{{- $seaweedName := default "seaweedfs" .Values.seaweedfs.fullnameOverride -}}
{{ printf "%s-master:9333" $seaweedName }}
{{- end }}

{{/*
OTel collector service endpoints
*/}}
{{- define "iyzitrace.traceCollectorName" -}}
iyzitrace-trace-enrichment
{{- end }}

{{- define "iyzitrace.metricCollectorName" -}}
iyzitrace-metric-enrichment
{{- end }}

{{- define "iyzitrace.logCollectorName" -}}
iyzitrace-log-enrichment
{{- end }}

{{- define "iyzitrace.logCollectorExtensionEndpoint" -}}
{{ printf "%s-collector-extension:13133" (include "iyzitrace.logCollectorName" .) }}
{{- end }}

{{- define "iyzitrace.logCollectorMonitoringEndpoint" -}}
{{ printf "%s-collector-monitoring:8888" (include "iyzitrace.logCollectorName" .) }}
{{- end }}

{{- define "iyzitrace.metricCollectorExtensionEndpoint" -}}
{{ printf "%s-collector-extension:13133" (include "iyzitrace.metricCollectorName" .) }}
{{- end }}

{{- define "iyzitrace.metricCollectorMonitoringEndpoint" -}}
{{ printf "%s-collector-monitoring:8888" (include "iyzitrace.metricCollectorName" .) }}
{{- end }}

{{- define "iyzitrace.traceCollectorExtensionEndpoint" -}}
{{ printf "%s-collector-extension:13133" (include "iyzitrace.traceCollectorName" .) }}
{{- end }}

{{- define "iyzitrace.traceCollectorMonitoringEndpoint" -}}
{{ printf "%s-collector-monitoring:8888" (include "iyzitrace.traceCollectorName" .) }}
{{- end }}
