# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Sytue Calendar (:8087) — servicio Go propio: CalDAV (RFC 4791) + API REST.
# Build multi-stage: golang:1.x -> gcr.io/distroless/static (rootless, sin shell).
# El contexto de build es la RAIZ del repo (modulo unico git.sytue.eu/sytue/one),
# segun deploy/compose (x-go-build.context=../..).
# syntax=docker/dockerfile:1

# ---- Build ----
FROM golang:1.27.3@sha256:f24164268fc1e741bbdc0066dbadcf1c8426c5a8489f0729c02c3905a53032fd AS build
WORKDIR /src

# Cache de dependencias: copia primero go.mod/go.sum del modulo raiz.
COPY go.mod go.sum ./
# apps/nucleo es un módulo aparte que el raíz usa con `replace` (el motor de fórmulas con
# el que Docs exporta las hojas): sin su go.mod, `go mod download` ni arranca.
COPY apps/nucleo/go.mod apps/nucleo/
RUN go mod download

# Copia el resto del modulo (platform/ + services/).
COPY . .

# Compila el binario estatico del servicio (CGO desactivado -> distroless/static).
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
        -o /out/calendar ./services/calendar/cmd/calendar

# ---- Runtime ----
FROM gcr.io/distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
WORKDIR /app

# Copia el binario; corre como usuario no-root (distroless nonroot = uid 65532).
COPY --from=build /out/calendar /app/calendar

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

USER nonroot:nonroot
EXPOSE 8087

# Healthcheck del compose: "/app/calendar healthcheck" (sin shell en distroless).
ENTRYPOINT ["/app/calendar"]
