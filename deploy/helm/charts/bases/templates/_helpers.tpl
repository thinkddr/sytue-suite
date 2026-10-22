{{- define "sytue-bases.fullname" -}}{{ printf "%s-bases" .Release.Name | trunc 63 | trimSuffix "-" }}{{- end -}}
{{- define "sytue-bases.domain" -}}{{ (.Values.global).domain | default "sytue.eu" }}{{- end -}}
{{- define "sytue-bases.host" -}}{{ printf "%s.%s" (.Values.subdomain | default "bases") (include "sytue-bases.domain" .) }}{{- end -}}
{{- define "sytue-bases.clusterIssuer" -}}{{ ((.Values.global).tls | default dict).issuer | default "sytue-letsencrypt-staging" }}{{- end -}}
{{- define "sytue-bases.env" -}}{{ (.Values.global).env | default "base" }}{{- end -}}
{{- define "sytue-bases.registry" -}}{{ (.Values.global).imageRegistry | default .Values.image.registry | default "rg.fr-par.scw.cloud/sytue" }}{{- end -}}
{{- define "sytue-bases.labels" -}}
app.kubernetes.io/name: sytue-bases
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: sytue-one
sytue.eu/env: {{ include "sytue-bases.env" . | quote }}
sytue.eu/stack: go
{{- end -}}
{{- define "sytue-bases.selector" -}}
app.kubernetes.io/name: sytue-bases
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
