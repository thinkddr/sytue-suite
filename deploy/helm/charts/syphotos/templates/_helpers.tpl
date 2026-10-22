{{/* Helpers de sytue-syphotos. Reusa global.* del umbrella cuando esta disponible. */}}

{{- define "sytue-syphotos.name" -}}sytue-syphotos{{- end -}}

{{- define "sytue-syphotos.fullname" -}}
{{- printf "%s-syphotos" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sytue-syphotos.component" -}}{{- .Values.component | default "syphotos" -}}{{- end -}}

{{- define "sytue-syphotos.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{- define "sytue-syphotos.domain" -}}
{{- (.Values.global).domain | default .Values.domain | default "sytue.eu" -}}
{{- end -}}

{{- define "sytue-syphotos.host" -}}
{{- printf "%s.%s" (.Values.subdomain | default "syphotos") (include "sytue-syphotos.domain" .) -}}
{{- end -}}

{{- define "sytue-syphotos.realm" -}}
{{- ((.Values.global).identity | default dict).realm | default "sytue" -}}
{{- end -}}

{{/* Issuer OIDC: discovery de Sytue ID (id.<domain>). Sin /realms/ (eso era Keycloak, retirado). */}}
{{- define "sytue-syphotos.oidcIssuer" -}}
{{- printf "https://id.%s" (include "sytue-syphotos.domain" .) -}}
{{- end -}}

{{- define "sytue-syphotos.clusterIssuer" -}}
{{- ((.Values.global).tls | default dict).issuer | default (.Values.tls).issuer | default "sytue-letsencrypt-staging" -}}
{{- end -}}

{{- define "sytue-syphotos.imageRegistry" -}}
{{- (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" -}}
{{- end -}}

{{- define "sytue-syphotos.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-syphotos.env" . | quote }}
sytue.eu/stack: go
{{ include "sytue-syphotos.selectorLabels" . }}
{{- end -}}

{{- define "sytue-syphotos.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-syphotos.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: {{ include "sytue-syphotos.component" . }}
{{- end -}}
