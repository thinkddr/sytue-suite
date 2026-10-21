{{/* Helpers de sytue-docs. Reusa global.* del umbrella cuando esta disponible. */}}

{{- define "sytue-docs.name" -}}sytue-docs{{- end -}}

{{- define "sytue-docs.fullname" -}}
{{- printf "%s-docs" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sytue-docs.component" -}}{{- .Values.component | default "docs" -}}{{- end -}}

{{- define "sytue-docs.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{- define "sytue-docs.domain" -}}
{{- (.Values.global).domain | default .Values.domain | default "sytue.eu" -}}
{{- end -}}

{{- define "sytue-docs.host" -}}
{{- printf "%s.%s" (.Values.subdomain | default "docs") (include "sytue-docs.domain" .) -}}
{{- end -}}

{{- define "sytue-docs.realm" -}}
{{- ((.Values.global).identity | default dict).realm | default "sytue" -}}
{{- end -}}

{{/* Issuer OIDC: discovery de Sytue ID (id.<domain>). Sin /realms/ (eso era Keycloak, retirado). */}}
{{- define "sytue-docs.oidcIssuer" -}}
{{- printf "https://id.%s" (include "sytue-docs.domain" .) -}}
{{- end -}}

{{- define "sytue-docs.clusterIssuer" -}}
{{- ((.Values.global).tls | default dict).issuer | default (.Values.tls).issuer | default "sytue-letsencrypt-staging" -}}
{{- end -}}

{{- define "sytue-docs.imageRegistry" -}}
{{- (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" -}}
{{- end -}}

{{- define "sytue-docs.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-docs.env" . | quote }}
sytue.eu/stack: go
{{ include "sytue-docs.selectorLabels" . }}
{{- end -}}

{{- define "sytue-docs.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-docs.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: {{ include "sytue-docs.component" . }}
{{- end -}}
