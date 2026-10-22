{{/* Helpers de sytue-syerp. Reusa global.* del umbrella cuando esta disponible. */}}

{{- define "sytue-syerp.name" -}}sytue-syerp{{- end -}}

{{- define "sytue-syerp.fullname" -}}
{{- printf "%s-syerp" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sytue-syerp.component" -}}{{- .Values.component | default "syerp" -}}{{- end -}}

{{- define "sytue-syerp.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{- define "sytue-syerp.domain" -}}
{{- (.Values.global).domain | default .Values.domain | default "sytue.eu" -}}
{{- end -}}

{{- define "sytue-syerp.host" -}}
{{- printf "%s.%s" (.Values.subdomain | default "syerp") (include "sytue-syerp.domain" .) -}}
{{- end -}}

{{- define "sytue-syerp.realm" -}}
{{- ((.Values.global).identity | default dict).realm | default "sytue" -}}
{{- end -}}

{{/* Issuer OIDC: discovery de Sytue ID (id.<domain>). Sin /realms/ (eso era Keycloak, retirado). */}}
{{- define "sytue-syerp.oidcIssuer" -}}
{{- printf "https://id.%s" (include "sytue-syerp.domain" .) -}}
{{- end -}}

{{- define "sytue-syerp.clusterIssuer" -}}
{{- ((.Values.global).tls | default dict).issuer | default (.Values.tls).issuer | default "sytue-letsencrypt-staging" -}}
{{- end -}}

{{- define "sytue-syerp.imageRegistry" -}}
{{- (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" -}}
{{- end -}}

{{- define "sytue-syerp.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-syerp.env" . | quote }}
sytue.eu/stack: go
{{ include "sytue-syerp.selectorLabels" . }}
{{- end -}}

{{- define "sytue-syerp.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-syerp.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: {{ include "sytue-syerp.component" . }}
{{- end -}}
