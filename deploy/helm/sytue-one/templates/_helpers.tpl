{{/*
============================================================================
 Helpers comunes del umbrella chart Sytue One.
 Define nombres, labels obligatorias (app.kubernetes.io/part-of=sytue-one,
 sytue.eu/env) y utilidades de dominio/TLS reutilizadas por las plantillas.
============================================================================
*/}}

{{/* Nombre base del release */}}
{{- define "sytue-one.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Nombre completamente cualificado */}}
{{- define "sytue-one.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name (include "sytue-one.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{/*
Labels obligatorias por contrato (CONVENTIONS.md §2).
Uso: {{ include "sytue-one.labels" (dict "context" . "component" "hub") | nindent 4 }}
*/}}
{{- define "sytue-one.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .context.Chart.Name .context.Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .context.Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .context.Chart.AppVersion | quote }}
sytue.eu/env: {{ .context.Values.global.env | quote }}
{{- if .component }}
{{ include "sytue-one.selectorLabels" (dict "context" .context "component" .component) }}
{{- end }}
{{- end -}}

{{/* Selector labels (estables, no incluyen version) */}}
{{- define "sytue-one.selectorLabels" -}}
app.kubernetes.io/name: {{ printf "%s-%s" (include "sytue-one.name" .context) .component | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/instance: {{ .context.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{/* Dominio raíz efectivo (p.ej. sytue.eu). */}}
{{- define "sytue-one.domain" -}}
{{- .Values.global.domain | required "global.domain es obligatorio" -}}
{{- end -}}

{{/* Construye un FQDN para un subdominio: ej. (include "sytue-one.host" (dict "context" . "sub" "hub")) -> hub.sytue.eu */}}
{{- define "sytue-one.host" -}}
{{- if .sub -}}
{{- printf "%s.%s" .sub (include "sytue-one.domain" .context) -}}
{{- else -}}
{{- include "sytue-one.domain" .context -}}
{{- end -}}
{{- end -}}

{{/* Nombre del ClusterIssuer de cert-manager según entorno */}}
{{- define "sytue-one.clusterIssuer" -}}
{{- .Values.global.tls.issuer | default (printf "sytue-letsencrypt-%s" .Values.global.env) -}}
{{- end -}}

{{/* Anotaciones de Ingress comunes (TLS + clase) */}}
{{- define "sytue-one.ingressAnnotations" -}}
cert-manager.io/cluster-issuer: {{ include "sytue-one.clusterIssuer" . | quote }}
{{- with .Values.global.ingress.annotations }}
{{ toYaml . }}
{{- end }}
{{- end -}}

{{/* Issuer de Sytue ID (servicio Go) — base OIDC de toda la suite.
     https://id.<domain>, discovery en /.well-known/openid-configuration. Sin /realms/ (eso era Keycloak, retirado). */}}
{{- define "sytue-one.oidcIssuer" -}}
{{- printf "https://%s" (include "sytue-one.host" (dict "context" . "sub" "id")) -}}
{{- end -}}

{{/* Origen privado de Sytue ID para tráfico servidor-a-servidor. El issuer
     sigue siendo la URL pública: cambiarlo rompería la validación del claim iss. */}}
{{- define "sytue.oidcInternalBaseUrl" -}}
{{- $identity := ((.Values.global).identity | default dict) -}}
{{- $identity.internalBaseUrl | default (printf "http://%s-id" .Release.Name) -}}
{{- end -}}

{{/* JWKS privado: evita depender de DNS/TLS/ingress para arrancar los servicios
     que validan tokens emitidos por el ID del mismo release. */}}
{{- define "sytue.oidcJwksUrl" -}}
{{- $identity := ((.Values.global).identity | default dict) -}}
{{- $identity.jwksUrl | default (printf "%s/.well-known/jwks.json" (include "sytue.oidcInternalBaseUrl" .)) -}}
{{- end -}}

{{/* ServiceAccount name */}}
{{- define "sytue-one.serviceAccountName" -}}
{{- printf "%s-%s" (include "sytue-one.fullname" .context) .component | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
============================================================================
 Runtime Go atado a los LIMITES del pod (coste + rendimiento a escala).
 SIN dependencias ni codigo Go: el runtime ya lee GOMAXPROCS/GOMEMLIMIT del
 entorno. En k8s, sin esto, Go veria los cores del NODO -> exceso de Ps,
 context-switching y throttling CFS (latencia + coste). Aqui los derivamos de
 la cuota del pod. Helpers COMPARTIDOS por todos los subcharts de servicio Go
 (Helm comparte los named templates entre umbrella y subcharts en el render).
============================================================================
*/}}

{{/*
sytue.gomemlimit — soft-limit del GC de Go (~85% de un limite de memoria k8s).
Regla: GOMEMLIMIT < limits.memory, para que el GC sea agresivo ANTES del
OOM-kill duro del cgroup. Acepta Mi/Gi (las unidades de los values). Uso:
  {{ include "sytue.gomemlimit" "512Mi" }}   ->  435MiB
*/}}
{{- define "sytue.gomemlimit" -}}
{{- $s := . | toString -}}
{{- $mib := 0.0 -}}
{{- if hasSuffix "Gi" $s -}}
{{- $mib = mulf (trimSuffix "Gi" $s | float64) 1024.0 -}}
{{- else if hasSuffix "Mi" $s -}}
{{- $mib = trimSuffix "Mi" $s | float64 -}}
{{- else -}}
{{- fail (printf "sytue.gomemlimit: limite de memoria %q no soportado (usa Mi o Gi)" $s) -}}
{{- end -}}
{{- printf "%dMiB" (mulf $mib 0.85 | int) -}}
{{- end -}}

{{/*
sytue.goRuntimeEnv — bloque env comun a los servicios Go. Deriva el runtime de
los limites del contenedor (requiere resources.limits.{cpu,memory}):
  GOMAXPROCS = techo(limits.cpu) via downward API. divisor "1" redondea AL ALZA
               a un entero -> nº de Ps == cuota de CPU, sin throttling CFS.
  GOMEMLIMIT = ~85% de limits.memory (ver sytue.gomemlimit).
Uso (nindent al nivel de los items de env, normalmente 12):
  {{- include "sytue.goRuntimeEnv" (dict "container" "mail" "memLimit" .Values.resources.limits.memory) | nindent 12 }}
*/}}
{{- define "sytue.goRuntimeEnv" -}}
- name: GOMAXPROCS
  valueFrom:
    resourceFieldRef:
      containerName: {{ .container }}
      resource: limits.cpu
      divisor: "1"
- name: GOMEMLIMIT
  value: {{ include "sytue.gomemlimit" .memLimit | quote }}
{{- end -}}

{{/*
sytue.logCollectorEnv — a donde manda cada servicio los avisos y errores que
captura (platform/pkg/log). La tabla `service_logs` y la pantalla «que esta
fallando» viven en Sytue ID, asi que el resto se los envia por HTTP DENTRO del
cluster; sin esto solo se veia lo del propio IdP y los otros catorce escribian
a stdout para nadie.

El Secret es `optional: true` A PROPOSITO: si `02b-secretos.sh` no ha corrido,
la variable no se pone, el sumidero no se engancha y el servicio arranca y sirve
igual. Es la degradacion honesta —no capturar se ve en la pantalla; capturar
contra un destino que no autentica seria un error nuevo cada dos segundos—.

Uso (nindent al nivel de los items de env, normalmente 12), con el contexto
completo porque necesita .Release:
  {{- include "sytue.logCollectorEnv" . | nindent 12 }}
*/}}
{{- define "sytue.logCollectorEnv" -}}
- name: SYTUE_LOG_COLLECTOR_URL
  value: {{ printf "http://%s-id/service-logs" .Release.Name | quote }}
- name: SYTUE_LOG_COLLECTOR_TOKEN
  valueFrom:
    secretKeyRef:
      name: sytue-log-ingest
      key: token
      optional: true
{{- end -}}

{{/*
sytue.correoEnv — el buzón con el que un servicio manda correo (platform/pkg/config
Loader.Correo: SMTP_ADDR, SMTP_FROM, SMTP_TLS_SERVER_NAME, SMTP_USER, SMTP_PASSWORD,
con el prefijo de variables del servicio). Es el de «X pide editar» de Docs, Drive,
Notepads y Mail: docs@ para Docs y la cuenta común avisos@ para los otros tres.

Lee `.Values.config.smtp`: `addr` (vacío: <release>-mail-mx:587), `from` (sin él no se
pone nada y no sale correo), `tlsServerName`, `user` (en claro; vacío: `smtp-user` de
existingSecret) y `passwordSecret` (el Secret con `smtp-password`, p. ej. el que genera
`secretosInternos`; vacío: existingSecret).

Uso (nindent 12):
  {{- include "sytue.correoEnv" (dict "root" . "prefijo" "SYTUE_DOCS_") | nindent 12 }}
*/}}
{{- define "sytue.correoEnv" -}}
{{- $v := .root.Values -}}
{{- $smtp := $v.config.smtp | default dict -}}
{{- if $smtp.from }}
- name: {{ .prefijo }}SMTP_ADDR
  value: {{ $smtp.addr | default (printf "%s-mail-mx:587" .root.Release.Name) | quote }}
- name: {{ .prefijo }}SMTP_FROM
  value: {{ $smtp.from | quote }}
- name: {{ .prefijo }}SMTP_TLS_SERVER_NAME
  value: {{ $smtp.tlsServerName | default "" | quote }}
- name: {{ .prefijo }}SMTP_USER
{{- if $smtp.user }}
  value: {{ $smtp.user | quote }}
{{- else }}
  valueFrom:
    secretKeyRef:
      name: {{ $v.existingSecret }}
      key: smtp-user
      optional: true
{{- end }}
- name: {{ .prefijo }}SMTP_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ $smtp.passwordSecret | default $v.existingSecret }}
      key: smtp-password
      optional: true
{{- end }}
{{- end -}}

{{/*
sytue.correoEgress — el puerto del relé de correo de fuera del clúster
(`config.smtp.addr`), para el `ports:` de la regla de salida a internet de la
NetworkPolicy. Sin dirección no pone nada: el MSA de Sytue Mail es intra-suite.

Uso (nindent al nivel de los items de ports, normalmente 8):
  {{- include "sytue.correoEgress" . | nindent 8 }}
*/}}
{{- define "sytue.correoEgress" -}}
{{- with (.Values.config.smtp).addr }}
- protocol: TCP
  port: {{ splitList ":" . | last | int }}
{{- end }}
{{- end -}}

{{/* One configuration and credential for both ends of document call revocation. */}}
{{- define "sytue.documentCallsEnv" -}}
{{- $calls := ((.root.Values.global).documentCalls | default dict) -}}
{{- if not .omitEnabled }}
- name: {{ .prefix }}_ENCRYPTED_DOCUMENT_CALLS
  value: {{ $calls.enabled | default false | quote }}
{{- end }}
- name: {{ .prefix }}_{{ .secretSuffix }}
  valueFrom:
    secretKeyRef:
      name: {{ $calls.secretName | default "sytue-docs-rooms-access" }}
      key: {{ $calls.secretKey | default "secret" }}
      optional: {{ not ($calls.enabled | default false) }}
{{- end -}}
