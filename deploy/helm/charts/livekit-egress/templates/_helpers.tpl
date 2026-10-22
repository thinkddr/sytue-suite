{{/* Helpers de sytue-livekit-egress. Reusa global.* del umbrella cuando existe. */}}

{{- define "livekit-egress.name" -}}sytue-livekit-egress{{- end -}}

{{- define "livekit-egress.fullname" -}}
{{- printf "%s-livekit-egress" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "livekit-egress.labels" -}}
app.kubernetes.io/name: {{ include "livekit-egress.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: livekit-egress
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
{{- with .Values.global }}
{{- if .env }}
sytue.eu/env: {{ .env | quote }}
{{- end }}
{{- end }}
{{- end -}}

{{- define "livekit-egress.selectorLabels" -}}
app.kubernetes.io/name: {{ include "livekit-egress.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: livekit-egress
{{- end -}}

{{/* URL WS del SFU. Vacía en values => el del propio release. */}}
{{- define "livekit-egress.wsUrl" -}}
{{- .Values.livekit.wsUrl | default (printf "ws://%s-livekit:7880" .Release.Name) -}}
{{- end -}}

{{/* Imagen. Sin registro global se usa el upstream tal cual (dev/render suelto). */}}
{{- define "livekit-egress.image" -}}
{{- $reg := (.Values.global).imageRegistry | default "" -}}
{{- if and .Values.image.useGlobalRegistry $reg -}}
{{- printf "%s/%s:%s" $reg .Values.image.repository .Values.image.tag -}}
{{- else -}}
{{- printf "%s:%s" .Values.image.repository .Values.image.tag -}}
{{- end -}}
{{- end -}}

{{/*
La credencial de Dragonfly, con el mismo contrato que el SFU: primero lo que
diga el propio chart, y si no lo que ponga el umbrella en `global.cache`. Vacío
significa «cache sin contraseña», y entonces el ConfigMap se monta tal cual.
*/}}
{{- define "livekit-egress.cacheSecret" -}}
{{- ((.Values.redis).passwordSecret).name | default (((.Values.global).cache).secretName) | default "" -}}
{{- end -}}

{{- define "livekit-egress.cacheSecretKey" -}}
{{- ((.Values.redis).passwordSecret).key | default (((.Values.global).cache).passwordKey) | default "dragonfly-password" -}}
{{- end -}}
