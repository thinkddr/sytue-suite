{{/* Helpers de sytue-dragonfly. Reusa global.* del umbrella cuando esta disponible. */}}

{{- define "sytue-dragonfly.name" -}}sytue-dragonfly{{- end -}}

{{- define "sytue-dragonfly.fullname" -}}
{{- printf "%s-dragonfly" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Nombre DNS estable del Service, independiente del nombre de la release: los
servicios lo reciben en DRAGONFLY_URL como `redis://dragonfly:6379`, igual que en
el compose de desarrollo, para que la URL sea la misma en los dos sitios.
*/}}
{{- define "sytue-dragonfly.serviceName" -}}
{{- .Values.service.name | default "dragonfly" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sytue-dragonfly.component" -}}{{- .Values.component | default "dragonfly" -}}{{- end -}}

{{- define "sytue-dragonfly.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{/*
Imagen desde el mirror privado cuando image.useGlobalRegistry=true (produccion):
una dependencia de terceros que se descarga de internet en cada arranque es una
dependencia de que internet funcione.
*/}}
{{- define "sytue-dragonfly.image" -}}
{{- $reg := (.Values.global).imageRegistry | default "" -}}
{{- $ref := "" -}}
{{- if and .Values.image.useGlobalRegistry $reg -}}
{{- $ref = printf "%s/%s" $reg .Values.image.repository -}}
{{- else -}}
{{- $ref = .Values.image.repository -}}
{{- end -}}
{{- if .Values.image.digest -}}
{{/*
Con digest se referencia `repo:tag@sha256:…`. El tag se conserva porque es lo
único legible en un `kubectl get pod` —«v1.40.1» dice algo, un sha no—, pero lo
que RESUELVE es el digest: si el tag se moviera por detrás, el pod sigue
arrancando exactamente la misma imagen.

Hace falta cuando se tira del origen sin pasar por el mirror (ver
deploy/production/upstream-directo.txt): ahí el digest es lo único que queda
garantizando qué se ejecuta, porque no hay copia propia ni firma de Sytue.
*/}}
{{- printf "%s:%s@%s" $ref .Values.image.tag .Values.image.digest -}}
{{- else -}}
{{- printf "%s:%s" $ref .Values.image.tag -}}
{{- end -}}
{{- end -}}

{{- define "sytue-dragonfly.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-dragonfly.env" . | quote }}
{{ include "sytue-dragonfly.selectorLabels" . }}
{{- end -}}

{{- define "sytue-dragonfly.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-dragonfly.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: {{ include "sytue-dragonfly.component" . }}
{{- end -}}
