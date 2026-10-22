{{- define "tenant.slug" -}}
{{- required "tenant.slug es obligatorio (a-z0-9-)" .Values.tenant.slug -}}
{{- end -}}

{{- define "tenant.namespace" -}}
{{- printf "tenant-%s" (include "tenant.slug" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "tenant.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
sytue.eu/env: {{ .Values.tenant.env | quote }}
sytue.eu/stack: go
sytue.eu/tenant: {{ include "tenant.slug" . | quote }}
{{- end -}}
