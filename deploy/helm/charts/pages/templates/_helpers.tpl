{{- define "sytue-pages.fullname" -}}{{ printf "%s-pages" .Release.Name | trunc 63 | trimSuffix "-" }}{{- end -}}
{{- define "sytue-pages.sitiosFullname" -}}{{ printf "%s-pages-sitios" .Release.Name | trunc 63 | trimSuffix "-" }}{{- end -}}
{{- define "sytue-pages.domain" -}}{{ (.Values.global).domain | default "sytue.eu" }}{{- end -}}
{{- define "sytue-pages.env" -}}{{ (.Values.global).env | default "base" }}{{- end -}}
{{- define "sytue-pages.registry" -}}{{ (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" }}{{- end -}}
{{- define "sytue-pages.labels" -}}
app.kubernetes.io/name: sytue-pages
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: sytue-one
sytue.eu/env: {{ include "sytue-pages.env" . | quote }}
sytue.eu/stack: go
{{- end -}}
{{- define "sytue-pages.selector" -}}
app.kubernetes.io/name: sytue-pages
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
