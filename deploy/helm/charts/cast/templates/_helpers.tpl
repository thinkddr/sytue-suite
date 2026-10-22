{{/* Helpers de sytue-cast. Reusa global.* del umbrella cuando esta disponible. */}}

{{- define "sytue-cast.name" -}}sytue-cast{{- end -}}

{{- define "sytue-cast.fullname" -}}
{{- printf "%s-cast" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* El segundo Deployment: el transcodificador (mismo binario, subcomando `transcoder`). */}}
{{- define "sytue-cast.transcoderFullname" -}}
{{- printf "%s-cast-transcoder" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sytue-cast.component" -}}{{- .Values.component | default "cast" -}}{{- end -}}

{{- define "sytue-cast.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{- define "sytue-cast.domain" -}}
{{- (.Values.global).domain | default .Values.domain | default "sytue.eu" -}}
{{- end -}}

{{- define "sytue-cast.host" -}}
{{- printf "%s.%s" (.Values.subdomain | default "cast") (include "sytue-cast.domain" .) -}}
{{- end -}}

{{- define "sytue-cast.realm" -}}
{{- ((.Values.global).identity | default dict).realm | default "sytue" -}}
{{- end -}}

{{/* Issuer OIDC: discovery de Sytue ID (id.<domain>). Sin /realms/ (eso era Keycloak, retirado). */}}
{{- define "sytue-cast.oidcIssuer" -}}
{{- printf "https://id.%s" (include "sytue-cast.domain" .) -}}
{{- end -}}

{{- define "sytue-cast.clusterIssuer" -}}
{{- ((.Values.global).tls | default dict).issuer | default (.Values.tls).issuer | default "sytue-letsencrypt-staging" -}}
{{- end -}}

{{- define "sytue-cast.imageRegistry" -}}
{{- (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" -}}
{{- end -}}

{{- define "sytue-cast.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-cast.env" . | quote }}
sytue.eu/stack: go
{{ include "sytue-cast.selectorLabels" . }}
{{- end -}}

{{- define "sytue-cast.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-cast.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: {{ include "sytue-cast.component" . }}
{{- end -}}

{{/* Etiquetas del transcodificador: mismo name/instance, OTRO component, para que
     el Service, el HPA y el PDB del servicio HTTP no lo seleccionen. */}}
{{- define "sytue-cast.transcoderLabels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-cast.env" . | quote }}
sytue.eu/stack: go
{{ include "sytue-cast.transcoderSelectorLabels" . }}
{{- end -}}

{{- define "sytue-cast.transcoderSelectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-cast.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: {{ printf "%s-transcoder" (include "sytue-cast.component" .) }}
{{- end -}}

{{/* Las variables que comparten los DOS Deployments (servicio y transcodificador):
     el mismo binario lee la misma configuración (services/cast/internal/config), y
     escribirlas dos veces es la copia que diverge (CONVENTIONS §13.2). Lo que solo
     necesita uno —OIDC, el chat, la escucha HTTP— va en su plantilla. */}}
{{- define "sytue-cast.envComun" -}}
{{- $globalObjectStorage := ((.Values.global).objectStorage | default dict) -}}
{{- $objectStorage := .Values.objectStorage | default dict -}}
{{- $remote := ((.Values.global).objectStorageRemote | default dict) -}}
{{- $remoteSecret := $remote.existingSecret | default "sytue-object-storage-remote" -}}
{{- $objectStorageSecret := $objectStorage.existingSecret | default $globalObjectStorage.existingSecret | default "sytue-object-storage" -}}
{{- $objectStorageRegion := $objectStorage.region | default $globalObjectStorage.region | default "fr-par" -}}
{{- $objectStorageUseSSL := true -}}
{{- if hasKey $globalObjectStorage "useSSL" -}}
  {{- $objectStorageUseSSL = $globalObjectStorage.useSSL -}}
{{- end -}}
{{- if hasKey $objectStorage "useSSL" -}}
  {{- $objectStorageUseSSL = $objectStorage.useSSL -}}
{{- end -}}
{{- include "sytue.logCollectorEnv" . }}
- name: LOG_LEVEL
  value: {{ .Values.config.logLevel | quote }}
- name: PUBLIC_URL
  value: "https://{{ include "sytue-cast.host" . }}"
# ----- Postgres EXTERNO (Scaleway RDB; unica fuente de verdad) -----
- name: DATABASE_URL
  valueFrom:
    secretKeyRef:
      name: {{ .Values.postgresql.existingSecret }}
      key: {{ .Values.postgresql.dsnKey }}
# Pool pgx POR replica (bajo en prod: PgBouncer multiplexa hacia la RDB).
- name: DB_MAX_CONNS
  value: {{ .Values.config.dbMaxConns | quote }}
# DSN DIRECTO a la RDB (bypass del pooler) para estado de sesion: advisory
# locks de migraciones y el FOR UPDATE SKIP LOCKED de la cola (platform/pkg/pg).
# Opcional: sin PgBouncer (staging/dev) no existe y se usa DATABASE_URL.
- name: DATABASE_DIRECT_URL
  valueFrom:
    secretKeyRef:
      name: {{ .Values.postgresql.existingSecret }}
      key: {{ .Values.postgresql.dsnDirectKey }}
      optional: true
# ----- Sytue Drive: dueño del contador de la cuota del plan (CAST-ENCARGO §8) -----
# Vacio = se deriva del Release, como en Photos. Sin puerto: el Service expone el 80.
- name: DRIVE_URL
  value: {{ .Values.config.driveUrl | default (printf "http://%s-drive.%s.svc.cluster.local" .Release.Name .Release.Namespace) | quote }}
# ----- Sytue ID por dentro: quien es «@ana» al compartir con personas (people:resolve) -----
- name: ID_URL
  value: {{ .Values.config.idUrl | default (printf "http://%s-id.%s.svc.cluster.local" .Release.Name .Release.Namespace) | quote }}
# El secreto compartido con Drive: con el Cast declara su uso en
# `PUT /internal/quota/external/cast`. El MISMO valor vive en el Secret de Drive
# (clave `cast-internal-secret`). Fuera de dev, sin el, el binario no arranca.
- name: CAST_INTERNAL_SECRET
  valueFrom:
    secretKeyRef:
      name: {{ .Values.existingSecret }}
      key: cast-internal-secret
# Firma los enlaces de /media (HMAC, 6 h). Fuera de dev, sin ella, no arranca.
- name: MEDIA_SIGNING_KEY
  valueFrom:
    secretKeyRef:
      name: {{ .Values.existingSecret }}
      key: media-signing-key
# ----- Object Storage S3 (Scaleway fr-par, UE): el bucket de Cast -----
# Original, HLS, WebM y posteres, con prefijo por organizacion. No es el de Drive:
# la cuota se cuenta alli, los bytes viven aqui.
- name: S3_ENDPOINT
  valueFrom:
    secretKeyRef:
      name: {{ $objectStorageSecret }}
      key: endpoint
- name: S3_USE_SSL
  value: {{ $objectStorageUseSSL | quote }}
- name: S3_REGION
  value: {{ $objectStorageRegion | quote }}
- name: S3_BUCKET
  value: {{ .Values.config.s3Bucket | quote }}
- name: S3_ACCESS_KEY
  valueFrom:
    secretKeyRef:
      name: {{ $objectStorageSecret }}
      key: access-key
- name: S3_SECRET_KEY
  valueFrom:
    secretKeyRef:
      name: {{ $objectStorageSecret }}
      key: secret-key
{{- if $remote.enabled }}
# ----- Almacén remoto: todo lo nuevo a Scaleway One Zone (ADR 0038) -----
- name: S3_REMOTE_ENDPOINT
  valueFrom:
    secretKeyRef:
      name: {{ $remoteSecret }}
      key: endpoint
- name: S3_REMOTE_REGION
  value: {{ $remote.region | default "fr-par" | quote }}
- name: S3_REMOTE_STORAGE_CLASS
  value: {{ $remote.storageClass | default "ONEZONE_IA" | quote }}
- name: S3_REMOTE_BUCKET
  valueFrom:
    secretKeyRef:
      name: {{ $remoteSecret }}
      key: bucket
- name: S3_REMOTE_ACCESS_KEY
  valueFrom:
    secretKeyRef:
      name: {{ $remoteSecret }}
      key: access-key
- name: S3_REMOTE_SECRET_KEY
  valueFrom:
    secretKeyRef:
      name: {{ $remoteSecret }}
      key: secret-key
- name: S3_LOCAL_TENANTS
  value: {{ $remote.localTenants | default "" | quote }}
{{- end }}
{{- end -}}

{{/* La videollamada: el SFU LiveKit, el MISMO que Rooms (Secret sytue-livekit-keys).
     Lo necesitan los dos Deployments: el servicio para tokens, ingress y egress; el
     transcodificador porque el binario carga UNA configuracion y estas claves son
     obligatorias fuera de dev en cuanto el paquete de directo las exija. */}}
{{- define "sytue-cast.envLiveKit" -}}
# ----- Directo: el SFU LiveKit (la otra mitad de Rooms), con Ingress y Egress -----
- name: LIVEKIT_URL
  value: {{ .Values.config.livekitUrl | default (printf "wss://rooms.%s" (include "sytue-cast.domain" .)) | quote }}
- name: LIVEKIT_HOST
  value: {{ .Values.config.livekitApiHost | default (printf "http://%s-livekit:7880" .Release.Name) | quote }}
- name: LIVEKIT_API_KEY
  valueFrom:
    secretKeyRef:
      name: sytue-livekit-keys
      key: api-key
- name: LIVEKIT_API_SECRET
  valueFrom:
    secretKeyRef:
      name: sytue-livekit-keys
      key: api-secret
# Sondeo de respaldo del webhook (ListIngress/ListRooms/ListParticipants). 0 = apagado.
- name: LIVEKIT_POLL_INTERVAL
  value: {{ .Values.config.pollInterval | quote }}
{{- if .Values.config.recordingEndpoint }}
# El grabador puede vivir fuera del cluster: otro endpoint hacia el MISMO S3.
# No cambia el bucket ni las credenciales que viajan en la peticion de grabacion.
- name: RECORDING_ENDPOINT
  value: {{ .Values.config.recordingEndpoint | quote }}
{{- end }}
# Lo que se le ensena a quien emite con OBS: rtmp://cast-ingest.<dominio>/live.
# El ingress vive en la maquina de medios (deploy/cast-media/), no en el cluster.
- name: CAST_INGEST_RTMP_URL
  value: {{ .Values.config.ingestRtmpUrl | default (printf "rtmp://cast-ingest.%s/live" (include "sytue-cast.domain" .)) | quote }}
{{- end -}}
