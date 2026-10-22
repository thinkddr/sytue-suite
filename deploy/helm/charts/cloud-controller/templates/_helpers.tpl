{{- define "sytue-cloud-controller.name" -}}sytue-cloud-controller{{- end -}}

{{- define "sytue-cloud-controller.fullname" -}}
{{- printf "%s-cloud-controller" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sytue-cloud-controller.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{- define "sytue-cloud-controller.imageRegistry" -}}
{{- (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" -}}
{{- end -}}

{{/* ANALYTICS_INGEST_HOST: nombre DNS interno del colector de Sytue Analytics (ADR
     0026). Por defecto el Service del propio release —el namespace es el del release,
     no uno adivinado—; override para despliegues donde el colector vive en otro sitio. */}}
{{- define "sytue-cloud-controller.analyticsIngestHost" -}}
{{- .Values.config.analytics.ingestHost | default (printf "%s-analytics-ingest.%s.svc.cluster.local" .Release.Name .Release.Namespace) -}}
{{- end -}}

{{/* CERT_ISSUER: sigue al ClusterIssuer de la plataforma (global.tls.issuer) salvo override. */}}
{{- define "sytue-cloud-controller.certIssuer" -}}
{{- .Values.config.certIssuer | default (((.Values.global).tls | default dict).issuer) | default "sytue-letsencrypt-staging" -}}
{{- end -}}

{{- define "sytue-cloud-controller.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-cloud-controller.env" . | quote }}
sytue.eu/stack: go
{{ include "sytue-cloud-controller.selectorLabels" . }}
{{- end -}}

{{- define "sytue-cloud-controller.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-cloud-controller.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: cloud-controller
{{- end -}}
