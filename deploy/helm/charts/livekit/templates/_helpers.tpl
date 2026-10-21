{{- define "livekit.name" -}}livekit{{- end -}}
{{- define "livekit.fullname" -}}{{- printf "%s-livekit" .Release.Name | trunc 63 | trimSuffix "-" -}}{{- end -}}
{{- define "livekit.env" -}}{{- (.Values.global).env | default .Values.env | default "base" -}}{{- end -}}
{{- define "livekit.domain" -}}{{- (.Values.global).domain | default .Values.domain | default "sytue.eu" -}}{{- end -}}
{{- define "livekit.host" -}}{{- printf "%s.%s" (.Values.rooms.subdomain | default "rooms-sfu") (include "livekit.domain" .) -}}{{- end -}}
{{/*
HOST DE SEÑALIZACIÓN, separado del host de MEDIA. Por defecto son el mismo, que
es como estaba y como debe seguir siendo en un clúster con IP pública propia.

Se separan cuando el servidor está detrás de un router doméstico. Entonces son
dos caminos incompatibles bajo un solo nombre DNS: la señalización es WebSocket
sobre HTTPS y cabe por un túnel; los medios son UDP y necesitan la IP real. Un
registro A no proxied sirve para el media, pero deja la señalización fuera del
túnel sin ganar nada a cambio —y si el ISP bloquea el 443 entrante, no entra—.
Medido el 2026-09-06 desde ocho nodos externos: ningún puerto de la IP pública
de este piloto era alcanzable, y por eso las salas se creaban y se cerraban
vacías: el navegador nunca llegaba a abrir el WebSocket.
*/}}
{{- define "livekit.signalHost" -}}{{- .Values.ingress.signalHost | default (include "livekit.host" .) -}}{{- end -}}
{{/*
Secreto TLS de la señalización. Vacío = el de siempre. Se separa para que
apuntar la señalización a otro nombre NO reescriba el certificado del host de
media: cert-manager emite desde la anotación del Ingress y guarda en el
`secretName` que ahí se declare, así que compartirlo dejaría al host de media
sin su certificado sin que nada lo avisara.
*/}}
{{- define "livekit.signalTlsSecret" -}}{{- .Values.ingress.signalTlsSecret | default .Values.ingress.tlsSecret -}}{{- end -}}
{{- define "livekit.turnDomain" -}}{{- .Values.turn.domain | default (printf "turn.%s" (include "livekit.host" .)) -}}{{- end -}}
{{- define "livekit.clusterIssuer" -}}{{- ((.Values.global).tls | default dict).issuer | default (.Values.tls).issuer | default "sytue-letsencrypt-staging" -}}{{- end -}}
{{- define "livekit.imageRegistry" -}}{{- (.Values.global).imageRegistry | default "" -}}{{- end -}}

{{/*
Imagen del SFU. Producción activa image.useGlobalRegistry para usar el mirror
firmado en Scaleway; desarrollo puede tirar de la imagen oficial UPSTREAM.
*/}}
{{- define "livekit.image" -}}
{{- $reg := include "livekit.imageRegistry" . -}}
{{- if and .Values.image.useGlobalRegistry $reg -}}
{{- printf "%s/%s:%s" $reg .Values.image.repository .Values.image.tag -}}
{{- else -}}
{{- printf "%s:%s" .Values.image.repository .Values.image.tag -}}
{{- end -}}
{{- end -}}

{{- define "livekit.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "livekit.env" . | quote }}
{{ include "livekit.selectorLabels" . }}
{{- end -}}

{{- define "livekit.selectorLabels" -}}
app.kubernetes.io/name: {{ include "livekit.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: rooms
{{- end -}}

{{/*
Secret compartido de la cache. Sale del `global` del umbrella —el mismo que leen
Dragonfly y los siete servicios— y se puede forzar por valor propio. Vacío ⇒ sin
contraseña, que es como estaba antes del CVE-2026-62357.
*/}}
{{- define "livekit.cacheSecret" -}}
{{- .Values.redis.passwordSecret.name | default (((.Values.global).cache).secretName) | default "" -}}
{{- end -}}

{{- define "livekit.cacheSecretKey" -}}
{{- .Values.redis.passwordSecret.key | default (((.Values.global).cache).passwordKey) | default "dragonfly-password" -}}
{{- end -}}
