{{/* Helpers de sytue-ai. Reusa global.* del umbrella cuando está disponible. */}}

{{- define "sytue-ai.name" -}}sytue-ai{{- end -}}

{{- define "sytue-ai.fullname" -}}
{{- printf "%s-ai" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sytue-ai.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{- define "sytue-ai.domain" -}}
{{- (.Values.global).domain | default .Values.domain | default "sytue.eu" -}}
{{- end -}}

{{/* Issuer de cert-manager para el ingress: el mismo que usan los otros charts
     (global.tls.issuer, que el overlay por entorno fija). */}}
{{- define "sytue-ai.clusterIssuer" -}}
{{- ((.Values.global).tls | default dict).issuer | default (.Values.tls).issuer | default "sytue-letsencrypt-staging" -}}
{{- end -}}

{{/* Issuer OIDC: discovery de Sytue ID (id.<domain>). */}}
{{- define "sytue-ai.oidcIssuer" -}}
{{- printf "https://id.%s" (include "sytue-ai.domain" .) -}}
{{- end -}}

{{- define "sytue-ai.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-ai.env" . | quote }}
sytue.eu/stack: go
{{ include "sytue-ai.selectorLabels" . }}
{{- end -}}

{{- define "sytue-ai.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-ai.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: ai
{{- end -}}

{{- define "sytue-ai.imageRegistry" -}}
{{- (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" -}}
{{- end -}}
