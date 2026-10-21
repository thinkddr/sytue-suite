{{/* Helpers de sytue-notes. Reusa global.* del umbrella cuando esta disponible. */}}

{{- define "sytue-notes.name" -}}sytue-notes{{- end -}}

{{- define "sytue-notes.fullname" -}}
{{- printf "%s-notes" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sytue-notes.component" -}}{{- .Values.component | default "notes" -}}{{- end -}}

{{- define "sytue-notes.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{- define "sytue-notes.domain" -}}
{{- (.Values.global).domain | default .Values.domain | default "sytue.eu" -}}
{{- end -}}

{{- define "sytue-notes.host" -}}
{{- printf "%s.%s" (.Values.subdomain | default "notes") (include "sytue-notes.domain" .) -}}
{{- end -}}

{{- define "sytue-notes.realm" -}}
{{- ((.Values.global).identity | default dict).realm | default "sytue" -}}
{{- end -}}

{{/* Issuer OIDC: discovery de Sytue ID (id.<domain>). Sin /realms/ (eso era Keycloak, retirado). */}}
{{- define "sytue-notes.oidcIssuer" -}}
{{- printf "https://id.%s" (include "sytue-notes.domain" .) -}}
{{- end -}}

{{- define "sytue-notes.clusterIssuer" -}}
{{- ((.Values.global).tls | default dict).issuer | default (.Values.tls).issuer | default "sytue-letsencrypt-staging" -}}
{{- end -}}

{{- define "sytue-notes.imageRegistry" -}}
{{- (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" -}}
{{- end -}}

{{- define "sytue-notes.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-notes.env" . | quote }}
sytue.eu/stack: go
{{ include "sytue-notes.selectorLabels" . }}
{{- end -}}

{{- define "sytue-notes.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-notes.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: {{ include "sytue-notes.component" . }}
{{- end -}}
