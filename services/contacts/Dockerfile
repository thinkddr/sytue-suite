# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Sytue Contacts — imagen multi-stage. El contexto de build es la RAÍZ del repo
# (módulo único git.sytue.eu/sytue/one con go.mod en la raíz). Binario estático
# sobre distroless (rootless), conforme a CONVENTIONS §0.7.

# --- Stage 1: build ---
FROM golang:1.27.3@sha256:f24164268fc1e741bbdc0066dbadcf1c8426c5a8489f0729c02c3905a53032fd AS build
WORKDIR /src

# Cache de dependencias: copia primero los manifiestos del módulo.
COPY go.mod go.sum ./
# apps/nucleo es un módulo aparte que el raíz usa con `replace` (el motor de fórmulas con
# el que Docs exporta las hojas): sin su go.mod, `go mod download` ni arranca.
COPY apps/nucleo/go.mod apps/nucleo/
RUN go mod download

# Código fuente (módulo completo; el servicio importa platform/pkg/*).
COPY . .

ARG VERSION=dev
# CGO desactivado -> binario estático apto para distroless/static.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags "-s -w -X git.sytue.eu/sytue/one/services/contacts/internal/config.Version=${VERSION}" \
    -o /out/contacts ./services/contacts/cmd/contacts

# --- Stage 2: runtime ---
FROM gcr.io/distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
WORKDIR /app
COPY --from=build /out/contacts /app/contacts

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

# Usuario no-root (distroless nonroot = uid 65532).
USER nonroot:nonroot

# Puerto HTTP/CardDAV de Sytue Contacts (CONVENTIONS §6).
EXPOSE 8088
ENV PORT=8088

# Liveness probe vía subcomando (no requiere shell ni curl en la imagen).
HEALTHCHECK --interval=15s --timeout=5s --retries=10 --start-period=20s \
    CMD ["/app/contacts", "healthcheck"]

ENTRYPOINT ["/app/contacts"]
