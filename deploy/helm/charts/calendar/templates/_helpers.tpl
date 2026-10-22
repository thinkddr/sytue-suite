{{/* Helpers de sytue-calendar. Reusa global.* del umbrella cuando esta disponible. */}}

{{- define "sytue-calendar.name" -}}sytue-calendar{{- end -}}

{{- define "sytue-calendar.fullname" -}}
{{- printf "%s-calendar" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sytue-calendar.component" -}}{{- .Values.component | default "calendar" -}}{{- end -}}

{{- define "sytue-calendar.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{- define "sytue-calendar.domain" -}}
{{- (.Values.global).domain | default .Values.domain | default "sytue.eu" -}}
{{- end -}}

{{- define "sytue-calendar.host" -}}
{{- printf "%s.%s" (.Values.subdomain | default "calendar") (include "sytue-calendar.domain" .) -}}
{{- end -}}

{{- define "sytue-calendar.realm" -}}
{{- ((.Values.global).identity | default dict).realm | default "sytue" -}}
{{- end -}}

{{/* Issuer OIDC: discovery de Sytue ID (id.<domain>). Sin /realms/ (eso era Keycloak, retirado). */}}
{{- define "sytue-calendar.oidcIssuer" -}}
{{- printf "https://id.%s" (include "sytue-calendar.domain" .) -}}
{{- end -}}

{{- define "sytue-calendar.clusterIssuer" -}}
{{- ((.Values.global).tls | default dict).issuer | default (.Values.tls).issuer | default "sytue-letsencrypt-staging" -}}
{{- end -}}

{{- define "sytue-calendar.imageRegistry" -}}
{{- (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" -}}
{{- end -}}

{{- define "sytue-calendar.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-calendar.env" . | quote }}
sytue.eu/stack: go
{{ include "sytue-calendar.selectorLabels" . }}
{{- end -}}

{{- define "sytue-calendar.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-calendar.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: {{ include "sytue-calendar.component" . }}
{{- end -}}
