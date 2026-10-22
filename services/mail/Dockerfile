# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Sytue Mail — imagen multi-stage. Compila el binario estático del servidor de
# correo propio (SMTP/IMAP/JMAP) y lo ejecuta sobre distroless static (rootless),
# conforme a docs/CONVENTIONS.md §0.7, §13. Sin motores de correo de terceros AGPL.

# --- build ---
FROM golang:1.27.3@sha256:f24164268fc1e741bbdc0066dbadcf1c8426c5a8489f0729c02c3905a53032fd AS build
WORKDIR /src

# Cachear dependencias del módulo único (go.mod en la raíz del repo).
COPY go.mod go.sum ./
# apps/nucleo es un módulo aparte que el raíz usa con `replace` (el motor de fórmulas con
# el que Docs exporta las hojas): sin su go.mod, `go mod download` ni arranca.
COPY apps/nucleo/go.mod apps/nucleo/
RUN go mod download

# Copiar el resto del monorepo (módulo único git.sytue.eu/sytue/one).
COPY . .

ENV CGO_ENABLED=0 GOOS=linux
# Binario estático y reproducible; sin símbolos de depuración. La versión se
# inyecta en runtime vía la variable de entorno VERSION.
RUN go build -trimpath -ldflags="-s -w" -o /out/mail ./services/mail/cmd/mail

# --- runtime ---
FROM gcr.io/distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
WORKDIR /app
COPY --from=build /out/mail /app/mail

# Qué release y qué commit son ESTA imagen. Se hornean aquí, no los pone el
# chart: la pregunta que responde la consola de plataforma es «qué está
# corriendo», y un valor puesto por Helm contesta «qué creía Helm que iba a
# correr» — que no es lo mismo cuando un despliegue va a medias, cuando se
# actualiza con `--reuse-values` o cuando el tag de versión se ha movido.
#
# Como ENV y no en ldflags: el binario sigue siendo reproducible byte a byte.
# Los lee platform/pkg/config (versión) y platform/pkg/httpx (commit, en
# /healthz), sin prefijo de servicio: es el mismo dato para los once.
ARG VERSION
ARG COMMIT
ENV SYTUE_BUILD_VERSION=${VERSION}
ENV SYTUE_BUILD_COMMIT=${COMMIT}

# Puertos canónicos de Sytue Mail (docs/CONVENTIONS.md §6):
#   8086 HTTP/JMAP · 25 SMTP MX · 465 SMTPS · 587 Submission · 993 IMAPS
EXPOSE 8086 25 465 587 993
USER nonroot:nonroot
HEALTHCHECK --interval=15s --timeout=5s --retries=10 --start-period=20s \
    CMD ["/app/mail", "healthcheck"]
ENTRYPOINT ["/app/mail"]
