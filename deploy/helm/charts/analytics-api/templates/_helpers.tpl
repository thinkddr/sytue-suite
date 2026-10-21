{{/* Helpers de sytue-analytics-api. Reusa global.* del umbrella cuando está disponible. */}}

{{- define "sytue-analytics-api.name" -}}sytue-analytics-api{{- end -}}

{{- define "sytue-analytics-api.fullname" -}}
{{- printf "%s-analytics-api" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Entorno y dominio efectivos (caen a valores locales si no hay global) */}}
{{- define "sytue-analytics-api.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{- define "sytue-analytics-api.domain" -}}
{{- (.Values.global).domain | default .Values.domain | default "sytue.eu" -}}
{{- end -}}

{{/* Issuer OIDC: discovery de Sytue ID (id.<domain>). Sin /realms/ (eso era Keycloak, retirado). */}}
{{- define "sytue-analytics-api.oidcIssuer" -}}
{{- printf "https://id.%s" (include "sytue-analytics-api.domain" .) -}}
{{- end -}}

{{/* Origenes del navegador: el panel de analitica es una seccion de cloud-dashboard
     (cloud.<domain>) y de Sytue Admin. Overridable por config.corsAllowedOrigins. */}}
{{- define "sytue-analytics-api.corsAllowedOrigins" -}}
{{- $d := include "sytue-analytics-api.domain" . -}}
{{- .Values.config.corsAllowedOrigins | default (printf "https://cloud.%s,https://admin.%s" $d $d) -}}
{{- end -}}

{{- define "sytue-analytics-api.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-analytics-api.env" . | quote }}
sytue.eu/stack: go
{{ include "sytue-analytics-api.selectorLabels" . }}
{{- end -}}

{{- define "sytue-analytics-api.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-analytics-api.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: analytics-api
{{- end -}}

{{- define "sytue-analytics-api.imageRegistry" -}}
{{- (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" -}}
{{- end -}}
