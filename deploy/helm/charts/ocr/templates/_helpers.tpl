{{/* Helpers de sytue-ocr. Reusa global.* del umbrella cuando está disponible. */}}

{{- define "sytue-ocr.name" -}}sytue-ocr{{- end -}}

{{- define "sytue-ocr.fullname" -}}
{{- printf "%s-ocr" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sytue-ocr.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{- define "sytue-ocr.domain" -}}
{{- (.Values.global).domain | default .Values.domain | default "sytue.eu" -}}
{{- end -}}

{{/* Issuer OIDC: discovery de Sytue ID (id.<domain>). */}}
{{- define "sytue-ocr.oidcIssuer" -}}
{{- printf "https://id.%s" (include "sytue-ocr.domain" .) -}}
{{- end -}}

{{- define "sytue-ocr.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-ocr.env" . | quote }}
sytue.eu/stack: go
{{ include "sytue-ocr.selectorLabels" . }}
{{- end -}}

{{- define "sytue-ocr.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-ocr.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: ocr
{{- end -}}

{{- define "sytue-ocr.imageRegistry" -}}
{{- (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" -}}
{{- end -}}
