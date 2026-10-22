{{/* Helpers de sytue-integrations. Reusa global.* del umbrella cuando está disponible. */}}

{{- define "sytue-integrations.name" -}}sytue-integrations{{- end -}}

{{- define "sytue-integrations.fullname" -}}
{{- printf "%s-integrations" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Entorno y dominio efectivos (caen a valores locales si no hay global) */}}
{{- define "sytue-integrations.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{- define "sytue-integrations.domain" -}}
{{- (.Values.global).domain | default .Values.domain | default "sytue.eu" -}}
{{- end -}}

{{- define "sytue-integrations.host" -}}
{{- printf "%s.%s" (.Values.subdomain | default "integrations") (include "sytue-integrations.domain" .) -}}
{{- end -}}

{{/*
Adónde se devuelve al admin al volver del consentimiento OAuth.

Lleva el PREFIJO, no solo el host: la consola se sirve en `admin.<dominio>/admin`
—`console.` es un host legado que responde 301— y el servicio compone
`<consoleURL>/<sección>`. Sin el prefijo saldría `admin.<dominio>/migrations`, que
no existe: un 404 al final de un flujo que había ido bien.

Sin esto, `CONSOLE_URL` no se pasaba en absoluto y el servicio caía a su valor por
defecto de desarrollo: al terminar el consentimiento en producción, el navegador
acababa en `console.sytue.localhost` con ERR_CONNECTION_REFUSED.
*/}}
{{- define "sytue-integrations.consoleURL" -}}
{{- $sub := .Values.console.subdomain | default "admin" -}}
{{- $path := .Values.console.path | default "/admin" -}}
{{- printf "https://%s.%s%s" $sub (include "sytue-integrations.domain" .) $path -}}
{{- end -}}

{{/* Issuer OIDC: discovery de Sytue ID (id.<domain>). Sin /realms/ (eso era Keycloak, retirado). */}}
{{- define "sytue-integrations.oidcIssuer" -}}
{{- printf "https://id.%s" (include "sytue-integrations.domain" .) -}}
{{- end -}}

{{- define "sytue-integrations.clusterIssuer" -}}
{{- ((.Values.global).tls | default dict).issuer | default (.Values.tls).issuer | default "sytue-letsencrypt-staging" -}}
{{- end -}}

{{- define "sytue-integrations.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-integrations.env" . | quote }}
sytue.eu/stack: go
{{ include "sytue-integrations.selectorLabels" . }}
{{- end -}}

{{- define "sytue-integrations.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-integrations.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: integrations
{{- end -}}

{{- define "sytue-integrations.imageRegistry" -}}
{{- (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" -}}
{{- end -}}
