{{/* Helpers de sytue-drive. Reusa global.* del umbrella cuando esta disponible. */}}

{{- define "sytue-drive.name" -}}sytue-drive{{- end -}}

{{- define "sytue-drive.fullname" -}}
{{- printf "%s-drive" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sytue-drive.component" -}}{{- .Values.component | default "drive" -}}{{- end -}}

{{- define "sytue-drive.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{- define "sytue-drive.domain" -}}
{{- (.Values.global).domain | default .Values.domain | default "sytue.eu" -}}
{{- end -}}

{{- define "sytue-drive.host" -}}
{{- printf "%s.%s" (.Values.subdomain | default "drive") (include "sytue-drive.domain" .) -}}
{{- end -}}

{{- define "sytue-drive.realm" -}}
{{- ((.Values.global).identity | default dict).realm | default "sytue" -}}
{{- end -}}

{{/* Issuer OIDC: discovery de Sytue ID (id.<domain>). Sin /realms/ (eso era Keycloak, retirado). */}}
{{- define "sytue-drive.oidcIssuer" -}}
{{- printf "https://id.%s" (include "sytue-drive.domain" .) -}}
{{- end -}}

{{- define "sytue-drive.clusterIssuer" -}}
{{- ((.Values.global).tls | default dict).issuer | default (.Values.tls).issuer | default "sytue-letsencrypt-staging" -}}
{{- end -}}

{{- define "sytue-drive.imageRegistry" -}}
{{- (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" -}}
{{- end -}}

{{- define "sytue-drive.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-drive.env" . | quote }}
sytue.eu/stack: go
{{ include "sytue-drive.selectorLabels" . }}
{{- end -}}

{{- define "sytue-drive.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-drive.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: {{ include "sytue-drive.component" . }}
{{- end -}}
