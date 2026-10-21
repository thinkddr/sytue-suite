{{/* Helpers de sytue-hub. Reusa global.* del umbrella cuando está disponible. */}}

{{- define "sytue-hub.name" -}}sytue-hub{{- end -}}

{{- define "sytue-hub.fullname" -}}
{{- printf "%s-hub" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Entorno y dominio efectivos (caen a valores locales si no hay global) */}}
{{- define "sytue-hub.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{- define "sytue-hub.domain" -}}
{{- (.Values.global).domain | default .Values.domain | default "sytue.eu" -}}
{{- end -}}

{{- define "sytue-hub.host" -}}
{{- printf "%s.%s" (.Values.subdomain | default "hub") (include "sytue-hub.domain" .) -}}
{{- end -}}

{{/* Issuer OIDC: discovery de Sytue ID (id.<domain>). Sin /realms/ (eso era Keycloak, retirado). */}}
{{- define "sytue-hub.oidcIssuer" -}}
{{- printf "https://id.%s" (include "sytue-hub.domain" .) -}}
{{- end -}}

{{- define "sytue-hub.clusterIssuer" -}}
{{- ((.Values.global).tls | default dict).issuer | default (.Values.tls).issuer | default "sytue-letsencrypt-staging" -}}
{{- end -}}

{{- define "sytue-hub.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-hub.env" . | quote }}
sytue.eu/stack: go
{{ include "sytue-hub.selectorLabels" . }}
{{- end -}}

{{- define "sytue-hub.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-hub.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: hub
{{- end -}}

{{- define "sytue-hub.imageRegistry" -}}
{{- (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" -}}
{{- end -}}
