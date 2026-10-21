{{/* Helpers de sytue-mail. Reusa global.* del umbrella cuando esta disponible. */}}

{{- define "sytue-mail.name" -}}sytue-mail{{- end -}}

{{- define "sytue-mail.fullname" -}}
{{- printf "%s-mail" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sytue-mail.component" -}}{{- .Values.component | default "mail" -}}{{- end -}}

{{- define "sytue-mail.env" -}}
{{- (.Values.global).env | default .Values.env | default "base" -}}
{{- end -}}

{{- define "sytue-mail.domain" -}}
{{- (.Values.global).domain | default .Values.domain | default "sytue.eu" -}}
{{- end -}}

{{- define "sytue-mail.host" -}}
{{- printf "%s.%s" (.Values.subdomain | default "mail") (include "sytue-mail.domain" .) -}}
{{- end -}}

{{/*
Nombre del MTA: el que el servidor anuncia en EHLO y estampa en Received.
Es OTRO que el del webmail y por eso tiene su propio helper. mail.<dominio> es
HTTPS por el Ingress; quien recibe correo es mx.<dominio>, que es adonde apunta
el registro MX y lo que dice el PTR de su IP dedicada.

La distinción no es cosmética: los grandes receptores comprueban que el nombre
del EHLO, el PTR de la IP que se conecta y la resolución directa de ese nombre
cierren el círculo (FCrDNS). Anunciando mail.<dominio> desde una IP cuyo inverso
dice mx.<dominio>, el círculo no cierra y el correo se queda en spam o lo
rechazan de plano — con un 250 nuestro y ninguna pista de por qué no llegó.
*/}}
{{- define "sytue-mail.mtaHost" -}}
{{- printf "%s.%s" (.Values.config.mxHost | default "mx") (include "sytue-mail.domain" .) -}}
{{- end -}}

{{- define "sytue-mail.realm" -}}
{{- ((.Values.global).identity | default dict).realm | default "sytue" -}}
{{- end -}}

{{/* Issuer OIDC: discovery de Sytue ID (id.<domain>). Sin /realms/ (eso era Keycloak, retirado). */}}
{{- define "sytue-mail.oidcIssuer" -}}
{{- printf "https://id.%s" (include "sytue-mail.domain" .) -}}
{{- end -}}

{{- define "sytue-mail.clusterIssuer" -}}
{{- ((.Values.global).tls | default dict).issuer | default (.Values.tls).issuer | default "sytue-letsencrypt-staging" -}}
{{- end -}}

{{- define "sytue-mail.imageRegistry" -}}
{{- (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" -}}
{{- end -}}

{{- define "sytue-mail.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: sytue-one
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
sytue.eu/env: {{ include "sytue-mail.env" . | quote }}
sytue.eu/stack: go
{{ include "sytue-mail.selectorLabels" . }}
{{- end -}}

{{- define "sytue-mail.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sytue-mail.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: {{ include "sytue-mail.component" . }}
{{- end -}}
