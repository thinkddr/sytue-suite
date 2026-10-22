{{/* Helpers de sytue-rooms. Reusa global.* del umbrella cuando esta disponible. */}}

{{- define "sytue-rooms.name" -}}sytue-rooms{{- end -}}

{{- define "sytue-rooms.fullname" -}}
{{- printf "%s-rooms" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sytue-rooms.component" -}}{{- .Values.component | default "rooms" -}}{{- end -}}

{{- define "sytue-rooms.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{- define "sytue-rooms.domain" -}}
{{- (.Values.global).domain | default .Values.domain | default "sytue.eu" -}}
{{- end -}}

{{- define "sytue-rooms.host" -}}
{{- printf "%s.%s" (.Values.subdomain | default "rooms") (include "sytue-rooms.domain" .) -}}
{{- end -}}

{{- define "sytue-rooms.realm" -}}
{{- ((.Values.global).identity | default dict).realm | default "sytue" -}}
{{- end -}}

{{/* Issuer OIDC: discovery de Sytue ID (id.<domain>). Sin /realms/ (eso era Keycloak, retirado). */}}
{{- define "sytue-rooms.oidcIssuer" -}}
{{- printf "https://id.%s" (include "sytue-rooms.domain" .) -}}
{{- end -}}

{{- define "sytue-rooms.clusterIssuer" -}}
{{- ((.Values.global).tls | default dict).issuer | default (.Values.tls).issuer | default "sytue-letsencrypt-staging" -}}
{{- end -}}

{{- define "sytue-rooms.imageRegistry" -}}
{{- (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" -}}
{{- end -}}

{{- define "sytue-rooms.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-rooms.env" . | quote }}
sytue.eu/stack: go
{{ include "sytue-rooms.selectorLabels" . }}
{{- end -}}

{{- define "sytue-rooms.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-rooms.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: {{ include "sytue-rooms.component" . }}
{{- end -}}
