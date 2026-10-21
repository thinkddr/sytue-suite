{{/* Helpers de sytue-pgbouncer. Reusa global.* del umbrella cuando esta disponible. */}}

{{- define "sytue-pgbouncer.name" -}}sytue-pgbouncer{{- end -}}

{{- define "sytue-pgbouncer.fullname" -}}
{{- printf "%s-pgbouncer" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Nombre DNS estable del Service. Los DSN se materializan antes de instalar Helm,
por lo que no pueden depender del nombre elegido para la release.
*/}}
{{- define "sytue-pgbouncer.serviceName" -}}
{{- .Values.service.name | default "pgbouncer" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sytue-pgbouncer.component" -}}{{- .Values.component | default "pgbouncer" -}}{{- end -}}

{{- define "sytue-pgbouncer.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{/*
Imagen comunitaria servida desde el mirror de Scaleway cuando
image.useGlobalRegistry=true (producción).
*/}}
{{- define "sytue-pgbouncer.image" -}}
{{- $reg := (.Values.global).imageRegistry | default "" -}}
{{- if and .Values.image.useGlobalRegistry $reg -}}
{{- printf "%s/%s:%s" $reg .Values.image.repository .Values.image.tag -}}
{{- else -}}
{{- printf "%s:%s" .Values.image.repository .Values.image.tag -}}
{{- end -}}
{{- end -}}

{{- define "sytue-pgbouncer.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-pgbouncer.env" . | quote }}
{{ include "sytue-pgbouncer.selectorLabels" . }}
{{- end -}}

{{- define "sytue-pgbouncer.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-pgbouncer.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: {{ include "sytue-pgbouncer.component" . }}
{{- end -}}
