{{- define "sytue-cloud-api.name" -}}sytue-cloud-api{{- end -}}

{{- define "sytue-cloud-api.fullname" -}}
{{- printf "%s-cloud-api" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sytue-cloud-api.runtimeAccessConfigMap" -}}
{{- printf "%s-runtime-access" (include "sytue-cloud-api.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sytue-cloud-api.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{- define "sytue-cloud-api.domain" -}}
{{- (.Values.global).domain | default .Values.domain | default "sytue.eu" -}}
{{- end -}}

{{- define "sytue-cloud-api.host" -}}
{{- printf "%s.%s" (.Values.subdomain | default "cloud") (include "sytue-cloud-api.domain" .) -}}
{{- end -}}

{{/* Issuer OIDC: discovery de Sytue ID (id.<domain>). Sin /realms/ (eso era Keycloak, retirado). */}}
{{- define "sytue-cloud-api.oidcIssuer" -}}
{{- printf "https://id.%s" (include "sytue-cloud-api.domain" .) -}}
{{- end -}}

{{- define "sytue-cloud-api.clusterIssuer" -}}
{{- ((.Values.global).tls | default dict).issuer | default (.Values.tls).issuer | default "sytue-letsencrypt-staging" -}}
{{- end -}}

{{- define "sytue-cloud-api.imageRegistry" -}}
{{- (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" -}}
{{- end -}}

{{- define "sytue-cloud-api.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-cloud-api.env" . | quote }}
sytue.eu/stack: go
{{ include "sytue-cloud-api.selectorLabels" . }}
{{- end -}}

{{- define "sytue-cloud-api.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-cloud-api.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: cloud-api
{{- end -}}
