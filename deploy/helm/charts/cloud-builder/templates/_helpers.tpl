{{- define "sytue-cloud-builder.name" -}}sytue-cloud-builder{{- end -}}

{{- define "sytue-cloud-builder.fullname" -}}
{{- printf "%s-cloud-builder" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sytue-cloud-builder.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{- define "sytue-cloud-builder.imageRegistry" -}}
{{- (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" -}}
{{- end -}}

{{- define "sytue-cloud-builder.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-cloud-builder.env" . | quote }}
sytue.eu/stack: go
{{ include "sytue-cloud-builder.selectorLabels" . }}
{{- end -}}

{{- define "sytue-cloud-builder.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-cloud-builder.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: cloud-builder
{{- end -}}
