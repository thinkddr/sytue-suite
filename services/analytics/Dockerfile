# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Sytue Analytics — imagen multi-stage con LOS DOS binarios (ADR 0026 §5). El
# contexto de build es la RAÍZ del repo (módulo único git.sytue.eu/sytue/one).
# Binarios estáticos sobre distroless (rootless), conforme a CONVENTIONS §0.7.
#
# Una sola imagen y dos procesos: el ENTRYPOINT por defecto es el colector y el
# despliegue de la API lo sobreescribe con /app/analytics-api. Son procesos
# separados —que es lo que importa para el radio de explosión—, no imágenes.

# --- Stage 1: build ---
FROM golang:1.27.3@sha256:f24164268fc1e741bbdc0066dbadcf1c8426c5a8489f0729c02c3905a53032fd AS build
WORKDIR /src

COPY go.mod go.sum ./
# apps/nucleo es un módulo aparte que el raíz usa con `replace` (el motor de fórmulas con
# el que Docs exporta las hojas): sin su go.mod, `go mod download` ni arranca.
COPY apps/nucleo/go.mod apps/nucleo/
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags "-s -w -X git.sytue.eu/sytue/one/services/analytics/internal/config.Version=${VERSION}" \
    -o /out/ ./services/analytics/cmd/analytics-ingest ./services/analytics/cmd/analytics-api

# País sin guardar IP: la tabla de rangos se EMPOTRA EN LA IMAGEN, no en git (son
# ~4,5 MB que cambian cada mes; el repo más grande no ayuda a nadie). Sin ella el
# servicio arranca igual y todo el tráfico queda como país "ZZ".
#
# DB-IP IP-to-Country Lite — CC BY 4.0 — https://db-ip.com (ver NOTICE).
ARG GEO_MONTH
RUN set -eu; \
    mkdir -p /out/data; \
    m="${GEO_MONTH:-$(date -u +%Y-%m)}"; \
    prev="$(date -u -d '1 month ago' +%Y-%m)"; \
    for mm in "$m" "$prev"; do \
        if curl -fsSL --retry 3 --max-time 120 \
            -o /out/data/dbip-country-lite.csv.gz \
            "https://download.db-ip.com/free/dbip-country-lite-$mm.csv.gz"; then \
            echo "DB-IP $mm descargado"; exit 0; \
        fi; \
    done; \
    rm -f /out/data/dbip-country-lite.csv.gz; \
    echo "AVISO: sin base DB-IP; el desglose por país quedará en ZZ"

# --- Stage 2: runtime ---
FROM gcr.io/distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
WORKDIR /app
COPY --from=build /out/analytics-ingest /app/analytics-ingest
COPY --from=build /out/analytics-api /app/analytics-api
COPY --from=build /out/data /app/data

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
COPY services/analytics/NOTICE /app/NOTICE

USER nonroot:nonroot

# Colector 8094, API de lectura 8095 (CONVENTIONS §6).
EXPOSE 8094 8095
ENV PORT=8094

HEALTHCHECK --interval=15s --timeout=5s --retries=10 --start-period=20s \
    CMD ["/app/analytics-ingest", "healthcheck"]

ENTRYPOINT ["/app/analytics-ingest"]
