# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Sytue Chat — imagen multi-stage. El contexto de build es la RAÍZ del repo
# (módulo único git.sytue.eu/sytue/one con go.mod en la raíz). Binario estático
# sobre distroless (rootless), conforme a CONVENTIONS §0.7.

# --- Stage 1: build ---
FROM golang:1.27.4@sha256:08bcb99a38f85a265f9a93d1d3323e2440eec333380f3ea26ee0dec9264bb41c AS build
WORKDIR /src

# Cache de dependencias: copia primero los manifiestos del módulo.
COPY go.mod go.sum ./
# apps/nucleo es un módulo aparte que el raíz usa con `replace` (el motor de fórmulas con
# el que Docs exporta las hojas): sin su go.mod, `go mod download` ni arranca.
COPY apps/nucleo/go.mod apps/nucleo/
RUN go mod download

# Código fuente (módulo completo; el servicio importa platform/pkg/*).
COPY . .

# El núcleo de cliente que Rooms sirve en /api/v1/client-core/ (los WebAssembly de
# apps/nucleo: vault y core para el cifrado E2E, office, mail y pdf, ya precomprimidos
# en brotli y gzip) lo genera UN guion, y este RUN
# es solo su llamada: scripts/publicar-imagen-go.sh, que en la release no ejecuta esta
# etapa, reconoce esta línea y ejecuta el mismo guion antes de cruzar el binario. Una
# receta escrita aquí dentro se la saltaba y la v3.20.3 salió sin núcleo
# (docs/operations.md#client-core-vacio). Cualquier otro RUN en esta etapa hace fallar
# la publicación en vez de perderse.
RUN scripts/client-core-assets.sh services/rooms/internal/http/client_core_assets

ARG VERSION=dev
# CGO desactivado -> binario estático apto para distroless/static.
# `-tags imagen`: sin el núcleo de cliente generado el binario NO compila
# (internal/http/client_core_embed_imagen.go); publicar-imagen-go.sh lee la etiqueta de aquí.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -tags imagen \
    -ldflags "-s -w -X git.sytue.eu/sytue/one/services/rooms/internal/config.Version=${VERSION}" \
    -o /out/rooms ./services/rooms/cmd/rooms

# --- Stage 2: runtime ---
FROM gcr.io/distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
WORKDIR /app
COPY --from=build /out/rooms /app/rooms

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

# Puerto HTTP/WebSocket de Sytue Chat (CONVENTIONS §6).
EXPOSE 8085
ENV PORT=8085

# Liveness probe vía subcomando (no requiere shell ni curl en la imagen).
HEALTHCHECK --interval=15s --timeout=5s --retries=10 --start-period=20s \
    CMD ["/app/rooms", "healthcheck"]

ENTRYPOINT ["/app/rooms"]
