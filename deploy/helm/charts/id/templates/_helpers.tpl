{{/* Helpers de sytue-id. Reusa global.* del umbrella cuando esta disponible. */}}

{{- define "sytue-id.name" -}}sytue-id{{- end -}}

{{- define "sytue-id.fullname" -}}
{{- printf "%s-id" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sytue-id.component" -}}{{- .Values.component | default "id" -}}{{- end -}}

{{- define "sytue-id.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{- define "sytue-id.domain" -}}
{{- (.Values.global).domain | default .Values.domain | default "sytue.eu" -}}
{{- end -}}

{{- define "sytue-id.host" -}}
{{- printf "%s.%s" (.Values.subdomain | default "id") (include "sytue-id.domain" .) -}}
{{- end -}}

{{- define "sytue-id.realm" -}}
{{- ((.Values.global).identity | default dict).realm | default .Values.config.realm | default "sytue" -}}
{{- end -}}

{{/* Issuer OIDC propio: Sytue ID se emite a si mismo (https://id.<domain>).
     Discovery estandar en /.well-known/openid-configuration. Sin /realms/ (eso era Keycloak, retirado). */}}
{{- define "sytue-id.oidcIssuer" -}}
{{- printf "https://%s" (include "sytue-id.host" .) -}}
{{- end -}}

{{- define "sytue-id.clusterIssuer" -}}
{{- ((.Values.global).tls | default dict).issuer | default (.Values.tls).issuer | default "sytue-letsencrypt-staging" -}}
{{- end -}}

{{- define "sytue-id.imageRegistry" -}}
{{- (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" -}}
{{- end -}}

{{- define "sytue-id.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-id.env" . | quote }}
sytue.eu/stack: go
{{ include "sytue-id.selectorLabels" . }}
{{- end -}}

{{- define "sytue-id.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-id.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: {{ include "sytue-id.component" . }}
{{- end -}}
