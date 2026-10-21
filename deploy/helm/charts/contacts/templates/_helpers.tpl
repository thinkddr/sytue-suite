{{/* Helpers de sytue-contacts. Reusa global.* del umbrella cuando esta disponible. */}}

{{- define "sytue-contacts.name" -}}sytue-contacts{{- end -}}

{{- define "sytue-contacts.fullname" -}}
{{- printf "%s-contacts" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sytue-contacts.component" -}}{{- .Values.component | default "contacts" -}}{{- end -}}

{{- define "sytue-contacts.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{- define "sytue-contacts.domain" -}}
{{- (.Values.global).domain | default .Values.domain | default "sytue.eu" -}}
{{- end -}}

{{- define "sytue-contacts.host" -}}
{{- printf "%s.%s" (.Values.subdomain | default "contacts") (include "sytue-contacts.domain" .) -}}
{{- end -}}

{{- define "sytue-contacts.realm" -}}
{{- ((.Values.global).identity | default dict).realm | default "sytue" -}}
{{- end -}}

{{/* Issuer OIDC: discovery de Sytue ID (id.<domain>). Sin /realms/ (eso era Keycloak, retirado). */}}
{{- define "sytue-contacts.oidcIssuer" -}}
{{- printf "https://id.%s" (include "sytue-contacts.domain" .) -}}
{{- end -}}

{{- define "sytue-contacts.clusterIssuer" -}}
{{- ((.Values.global).tls | default dict).issuer | default (.Values.tls).issuer | default "sytue-letsencrypt-staging" -}}
{{- end -}}

{{- define "sytue-contacts.imageRegistry" -}}
{{- (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" -}}
{{- end -}}

{{- define "sytue-contacts.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-contacts.env" . | quote }}
sytue.eu/stack: go
{{ include "sytue-contacts.selectorLabels" . }}
{{- end -}}

{{- define "sytue-contacts.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-contacts.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: {{ include "sytue-contacts.component" . }}
{{- end -}}
