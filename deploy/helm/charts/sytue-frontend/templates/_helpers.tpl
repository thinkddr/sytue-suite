{{- define "sytue-frontend.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{- define "sytue-frontend.domain" -}}
{{- (.Values.global).domain | default .Values.domain | default "sytue.eu" -}}
{{- end -}}

{{- define "sytue-frontend.imageRegistry" -}}
{{- (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" -}}
{{- end -}}

{{- define "sytue-frontend.clusterIssuer" -}}
{{- ((.Values.global).tls | default dict).issuer | default (.Values.tls).issuer | default "sytue-letsencrypt-staging" -}}
{{- end -}}

{{/* Issuer OIDC: discovery de Sytue ID (id.<domain>). Sin /realms/ (eso era Keycloak, retirado). */}}
{{- define "sytue-frontend.oidcIssuer" -}}
{{- printf "https://id.%s" (include "sytue-frontend.domain" .) -}}
{{- end -}}

{{/* Las operaciones confidenciales del BFF (discovery/token/JWKS) van por el
     Service interno; las redirecciones del navegador conservan el issuer público. */}}
{{- define "sytue-frontend.oidcInternalBaseUrl" -}}
{{- $identity := ((.Values.global).identity | default dict) -}}
{{- $identity.internalBaseUrl | default (printf "http://%s-id" .Release.Name) -}}
{{- end -}}

{{/* SYTUE_ANALYTICS_API_URL: la API de LECTURA de Sytue Analytics es interna y no
     tiene Ingress (ADR 0026 §6), asi que se resuelve por el Service del release.
     Vacia => la seccion de analitica del panel no aparece: es el interruptor de la
     funcion. Override para despliegues donde Analytics vive en otro release. */}}
{{- define "sytue-frontend.analyticsApiUrl" -}}
{{- $a := (.Values.analytics | default dict) -}}
{{- if and (hasKey $a "enabled") (eq (toString $a.enabled) "false") -}}
{{- else -}}
{{- $a.apiUrl | default (printf "http://%s-analytics-api.%s.svc.cluster.local" .Release.Name .Release.Namespace) -}}
{{- end -}}
{{- end -}}

{{/* URL INTERNA del control-plane de Sytue Cloud.

     Sin esta variable, `cloudBaseUrl()` del panel caía a su valor por defecto
     —https://cloud.sytue.eu—, que es LA PROPIA SUPERFICIE WEB: el BFF se pedía
     los proyectos a sí mismo, no encontraba la ruta y trataba el 404 como
     «no hay control-plane», así que el panel se quedaba en modo demostración
     para siempre con el servicio corriendo al lado.

     Va al Service interno del mismo release, como analytics: es tráfico
     servidor-a-servidor y no tiene por qué salir a dar la vuelta por el ingress. */}}
{{- define "sytue-frontend.cloudApiUrl" -}}
{{- $c := (.Values.cloud | default dict) -}}
{{- $c.apiUrl | default (printf "http://%s-cloud-api.%s.svc.cluster.local" .Release.Name .Release.Namespace) -}}
{{- end -}}

{{/* fullname para una app concreta. Uso: (dict "ctx" $ "app" "portal") */}}
{{- define "sytue-frontend.appFullname" -}}
{{- printf "%s-fe-%s" .ctx.Release.Name .app | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* labels para una app concreta */}}
{{- define "sytue-frontend.appLabels" -}}
helm.sh/chart: {{ printf "%s-%s" .ctx.Chart.Name .ctx.Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .ctx.Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .ctx.Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-frontend.env" .ctx | quote }}
sytue.eu/stack: astro
{{ include "sytue-frontend.appSelectorLabels" . }}
{{- end -}}

{{- define "sytue-frontend.appSelectorLabels" -}}
app.kubernetes.io/name: {{ printf "sytue-fe-%s" .app }}
app.kubernetes.io/instance: {{ .ctx.Release.Name }}
app.kubernetes.io/component: frontend
sytue.eu/surface: {{ .app }}
{{- end -}}

{{/* DIRECCIONES DE LOS SERVICIOS DE LA SUITE, derivadas del dominio del despliegue.

     Sin ellas, cada BFF cae al valor por defecto que lleva escrito el código, y ese
     valor es `https://<algo>.sytue.eu`: en producción acierta por casualidad —el
     dominio coincide— y en STAGING apunta a PRODUCCIÓN. O sea que staging no servía
     para validar nada, y una prueba en staging escribía en los datos de los clientes.
     Es el mismo patrón que dejó `global.docsHost` mandando el Drive de staging a
     pedirle el catálogo WOPI a la producción de Docs.

     Se emiten TODAS para TODA app, y no un juego por superficie: la relación real es
     «esta suite vive en este dominio», y una matriz app→variables se desincroniza al
     primer BFF nuevo. Una variable de más en un proceso que no la lee no cuesta nada;
     una de menos apunta a otro despliegue.

     Solo el PRIMER nombre de cada cadena `??` del código: los demás (SYTUE_CHAT_API_URL,
     SYTUE_MEET_API_URL, SYTUE_DRIVE_BASE_URL, SYTUE_ADMIN_API_URL) son respaldos
     históricos y se resuelven con el de delante. `SYTUE_OIDC_ISSUER`,
     `SYTUE_CLOUD_API_URL` y `SYTUE_ANALYTICS_API_URL`
     tienen su propio bloque y NO se repiten aquí.

     Uso: {{ include "sytue-frontend.serviceUrls" $root | nindent 12 }} */}}
{{- define "sytue-frontend.serviceUrls" -}}
{{- $d := include "sytue-frontend.domain" . -}}
- name: PUBLIC_SYTUE_HUB_BASE_URL
  value: "https://hub.{{ $d }}"
- name: SYTUE_HUB_BASE_URL
  value: "https://hub.{{ $d }}"
- name: SYTUE_HUB_URL
  value: "https://hub.{{ $d }}"
- name: PUBLIC_SYTUE_DRIVE_BASE_URL
  value: "https://drive.{{ $d }}"
- name: SYTUE_DRIVE_API_URL
  value: "https://drive.{{ $d }}"
- name: PUBLIC_SYTUE_DOCS_BASE_URL
  value: "https://docs.{{ $d }}"
- name: SYTUE_CALENDAR_BASE_URL
  value: "https://calendar.{{ $d }}"
- name: SYTUE_CONTACTS_BASE_URL
  value: "https://contacts.{{ $d }}"
- name: SYTUE_MAIL_API_URL
  value: "https://mail.{{ $d }}"
{{/* Las dos de arriba son para el NAVEGADOR. Estas son para el SERVIDOR.

     El BFF de cumplimiento (retenciones legales y eDiscovery) llama a Mail y a
     Drive desde dentro del clúster, y lo hacía por su nombre público. Medido
     contra producción el 2026-08-14: las dos ramas del panel devolvían 502 —que
     en ese código significa «la llamada no llegó a ser HTTP», porque un 401 o un
     403 se propagan con su propio código—. Las rutas existen y los servicios
     responden desde fuera; lo que no funciona es salir al ingress y volver a
     entrar.

     Es el mismo patrón que ya usan `SYTUE_INTEGRATIONS_INTERNAL_URL`,
     `cloudApiUrl` y `analyticsApiUrl`: para hablar entre procesos se usa el
     Service del clúster, y el nombre público se reserva para lo que abre el
     navegador. */}}
- name: SYTUE_MAIL_INTERNAL_URL
  value: "http://{{ .Release.Name }}-mail.{{ .Release.Namespace }}.svc.cluster.local"
- name: SYTUE_DRIVE_INTERNAL_URL
  value: "http://{{ .Release.Name }}-drive.{{ .Release.Namespace }}.svc.cluster.local"
{{/* Y Docs, que hasta ahora no tenía interna. La pide el BFF de DRIVE, no el de
     Docs: las operaciones de página sobre un PDF las hace el motor de Sytue PDF,
     que vive en Docs, mientras el fichero está en Drive; el BFF de Drive hace de
     puente para que el token no salga de ahí. Esa llamada iba por el nombre
     público y es la siguiente que habría caído. */}}
- name: SYTUE_DOCS_INTERNAL_URL
  value: "http://{{ .Release.Name }}-docs.{{ .Release.Namespace }}.svc.cluster.local"
{{/* El asistente de almacenamiento de Drive inicia y termina el consentimiento
     con Integrations desde el BFF, también en las cuentas personales sin Admin.
     La URL interna es compartida; el token de operador sigue solo en plataforma. */}}
- name: SYTUE_INTEGRATIONS_INTERNAL_URL
  value: "http://{{ .Release.Name }}-integrations.{{ .Release.Namespace }}.svc.cluster.local"
{{/* Y Sytue ID y el Hub, por lo mismo y con el mismo síntoma.

     Medido el 2026-09-10 desde el contenedor de la consola: `getaddrinfo
     ETIMEOUT id.sytue.eu`. La consola devolvía 502 en `/admin/api/admin/invitations`
     y en `/admin/api/admin/users/bulk`; por el Service interno la misma ruta
     contesta 401. Con el Hub pasa igual, y afecta además a los intents de Mail y
     Drive y al listado de aplicaciones del portal.

     El ISSUER NO ENTRA AQUÍ y no es un olvido: `SYTUE_OIDC_ISSUER` es lo que se
     compara con el `iss` de cada token, así que tiene que seguir siendo el
     público. Lo interno es a dónde se LLAMA, no quién firma.

     `SYTUE_HUB_URL` y `PUBLIC_SYTUE_HUB_BASE_URL` tampoco se tocan: viajan al
     navegador —la búsqueda global de la shell y su `preconnect`— y ahí el
     Service del clúster no existe. */}}
- name: SYTUE_ID_INTERNAL_URL
  value: "http://{{ .Release.Name }}-id.{{ .Release.Namespace }}.svc.cluster.local"
- name: SYTUE_HUB_INTERNAL_URL
  value: "http://{{ .Release.Name }}-hub.{{ .Release.Namespace }}.svc.cluster.local"
{{- /* Y Notes, desde que su traza de auditoría entra en la vista unificada de la
     Consola: es la quinta fuente del fan-out, y la pide el BFF desde dentro del
     clúster igual que las otras cuatro. */}}
- name: SYTUE_NOTES_INTERNAL_URL
  value: "http://{{ .Release.Name }}-notes.{{ .Release.Namespace }}.svc.cluster.local"
{{- /* Y Rooms, por la misma razón: la administración de canales la pide el BFF
     desde dentro del clúster. `SYTUE_ROOMS_API_URL` de abajo es el nombre
     PÚBLICO y se queda para lo que abre el navegador. */}}
- name: SYTUE_ROOMS_INTERNAL_URL
  value: "http://{{ .Release.Name }}-rooms.{{ .Release.Namespace }}.svc.cluster.local"
{{- /* Y Calendar y Contactos, las dos que faltaban del juego. Sus BFF —CalDAV y
     CardDAV— salían por el nombre público y desde un pod eso no resuelve: 502 en
     todo lo que pidiera la agenda o la libreta. Es el mismo fallo que ya tuvieron
     Mail y Drive en agosto, y Notepads, Rooms, ID y el Hub después. */}}
- name: SYTUE_CALENDAR_INTERNAL_URL
  value: "http://{{ .Release.Name }}-calendar.{{ .Release.Namespace }}.svc.cluster.local"
- name: SYTUE_CONTACTS_INTERNAL_URL
  value: "http://{{ .Release.Name }}-contacts.{{ .Release.Namespace }}.svc.cluster.local"
- name: SYTUE_ROOMS_API_URL
  value: "https://rooms.{{ $d }}"
- name: SYTUE_ROOMS_PUBLIC_URL
  value: "https://rooms.{{ $d }}"
# El WebSocket del chat. No es un `??` de respaldo como los de arriba: `/api/ws-ticket`
# devuelve ESTA dirección al navegador junto al token de la sesión, así que sin ella
# staging entregaba un token de staging apuntando a `wss://rooms.sytue.eu/ws` —el de
# PRODUCCIÓN—, el token no valida allí y el tiempo real se queda mudo sin un error que
# lo explique. La ruta la sirve services/rooms (`r.Get("/ws", …)`).
- name: SYTUE_ROOMS_WS_URL
  value: "wss://rooms.{{ $d }}/ws"
- name: SYTUE_INTEGRATIONS_BASE_URL
  value: "https://{{ .Values.integrationsSubdomain }}.{{ $d }}"
{{- /* Sytue Notepads. Faltaban las DOS y por eso la referencia bidireccional con
     Calendar no funcionaba fuera de dev: la ficha de un evento pide sus notas
     vinculadas con `SYTUE_NOTES_API_URL ?? PUBLIC_SYTUE_NOTES_BASE_URL`, y sin
     ninguna de las dos el BFF sale por su rama de degradación y devuelve la lista
     vacía. Estaban solo en docker-compose.yml, así que en local iba y desplegado
     no, sin un solo error que lo dijera —la degradación es deliberada, para que la
     ficha no se rompa si Notepads no está, y eso mismo tapaba el fallo—.

     La PÚBLICA es la que se le enseña a quien mira («abrir en Notepads»), así que
     va al nombre público. La de API la consume el BFF y aquí se dejó también
     pública, «porque lo que se arregla es que exista, no de qué lado del ingress
     entra». ESO ERA EL FALLO: el 2026-09-10 Notepads devolvía 502 en
     `/notes/api/notebooks` y `/notes/api/tags` por exactamente eso. El BFF ya
     prefiere `SYTUE_NOTES_INTERNAL_URL`; estas dos se quedan de respaldo. */}}
- name: PUBLIC_SYTUE_NOTES_BASE_URL
  value: "https://notes.{{ $d }}"
- name: SYTUE_NOTES_API_URL
  value: "https://notes.{{ $d }}"
{{- /* Sytue Fotos. Faltaba, y sin ella la aplicación no arranca: su proxy
     (`apps/photos/src/pages/api/[...ruta].ts`) cae al respaldo
     `http://syphotos:8097`, que es el nombre del servicio en docker-compose y NO
     existe en Kubernetes —aquí es `<release>-syphotos`, y en el 80—. El fallo se
     ve como «Error: Sytue Fotos respondió 500» en la galería, y en el log del
     frontal como `getaddrinfo ENOTFOUND syphotos`: el 500 lo genera el propio
     BFF al no poder resolver el nombre, así que en el servicio de Fotos no queda
     ni rastro de la petición y parece que el caído es él.

     INTERNA y no pública, a diferencia de la de Notepads de arriba: este proxy
     reenvía la ruta tal cual, así que no hay motivo para salir al ingress y
     volver a entrar. Es el mismo patrón que `SYTUE_MAIL_INTERNAL_URL` y
     compañía. Sin puerto: todos los Services de la suite exponen el 80. */}}
- name: SYTUE_PHOTOS_API_URL
  value: "http://{{ .Release.Name }}-syphotos.{{ .Release.Namespace }}.svc.cluster.local"
{{- /* SyERP. Igual que la de Fotos y por el mismo motivo: su proxy
     (`apps/erp/src/pages/api/[...ruta].ts`) reenvía la ruta tal cual al servicio
     Go, así que no hay razón para salir al ingress y volver a entrar. Sin ella
     el BFF cae al respaldo `https://erp.sytue.eu`, que desde un pod no resuelve
     y se ve como un 502 en TODA la aplicación —el catálogo es la primera
     petición que hace, así que no llega a pintarse ni la barra lateral—.

     Sin puerto: todos los Services de la suite exponen el 80. */}}
- name: SYTUE_SYERP_INTERNAL_URL
  value: "http://{{ .Release.Name }}-syerp.{{ .Release.Namespace }}.svc.cluster.local"
{{- /* Sytue Cast. INTERNA como la de Fotos y por el mismo motivo: su proxy
     (`apps/cast/src/pages/api/[...ruta].ts`) reenvía la ruta tal cual al
     servicio Go, y su respaldo `http://cast:8099` es el nombre del compose, que
     en Kubernetes no resuelve. Por aquí pasan también `/public/*` (portada y
     canal sin sesión), que a propósito NO están en el Ingress del servicio.
     Sin puerto: todos los Services de la suite exponen el 80. */}}
- name: SYTUE_CAST_API_URL
  value: "http://{{ .Release.Name }}-cast.{{ .Release.Namespace }}.svc.cluster.local"
{{- /* El WebSocket del chat del directo. PÚBLICA y no interna, como la de Rooms
     y por el mismo motivo: el BFF se la devuelve al NAVEGADOR junto al token de
     la sesión, y ahí el Service del clúster no existe. La ruta la sirve
     services/cast (`/ws/videos/{id}/chat`) y la publica su Ingress bajo `/ws`. */}}
- name: SYTUE_CAST_WS_URL
  value: "wss://cast.{{ $d }}"
{{- /* Pages se resuelve por el Service interno para el BFF; los visitantes de
     sitios publicados van al Deployment aislado `pages sitios`. */}}
- name: SYTUE_PAGES_API_URL
  value: "http://{{ .Release.Name }}-pages.{{ .Release.Namespace }}.svc.cluster.local"
{{- /* Sytue Bases. Por DENTRO: los formularios públicos (`/_bases/v1/forms`) no salen
     por el Ingress del servicio (solo `/api/v1`), así que el BFF ha de
     llegar por el Service (su respaldo `http://bases:8102` es el nombre del compose). */}}
- name: SYTUE_BASES_API_URL
  value: "http://{{ .Release.Name }}-bases.{{ .Release.Namespace }}.svc.cluster.local"
{{- /* Sytue AI. Es la variable que APAGA O ENCIENDE el asistente: el BFF
     (`packages/office/src/pages/api/ai.ts`) devuelve 503 `ai_not_configured` si no la
     ve, y el panel se oculta solo. Por eso no lleva respaldo: ausente significa
     «no hay IA», que es un estado legítimo y no una avería.

     Interna, como la de Fotos: el BFF reenvía a un servicio que no tiene ingress
     —a propósito, porque por él pasa el texto de los documentos—, así que salir
     al ingress y volver a entrar no solo sobra: no funcionaría. */}}
{{- if (.Values.global).ai }}
{{- if (.Values.global).ai.enabled }}
- name: SYTUE_AI_URL
  value: "http://{{ .Release.Name }}-ai.{{ .Release.Namespace }}.svc.cluster.local"
{{- end }}
{{- end }}
{{- end -}}
