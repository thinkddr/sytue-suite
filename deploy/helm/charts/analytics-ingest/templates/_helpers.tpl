{{/* Helpers de sytue-analytics-ingest. Reusa global.* del umbrella cuando está disponible. */}}

{{- define "sytue-analytics-ingest.name" -}}sytue-analytics-ingest{{- end -}}

{{- define "sytue-analytics-ingest.fullname" -}}
{{- printf "%s-analytics-ingest" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Entorno y dominio efectivos (caen a valores locales si no hay global) */}}
{{- define "sytue-analytics-ingest.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{- define "sytue-analytics-ingest.domain" -}}
{{- (.Values.global).domain | default .Values.domain | default "sytue.eu" -}}
{{- end -}}

{{- define "sytue-analytics-ingest.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-analytics-ingest.env" . | quote }}
sytue.eu/stack: go
{{ include "sytue-analytics-ingest.selectorLabels" . }}
{{- end -}}

{{- define "sytue-analytics-ingest.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-analytics-ingest.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: analytics-ingest
{{- end -}}

{{- define "sytue-analytics-ingest.imageRegistry" -}}
{{- (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" -}}
{{- end -}}
