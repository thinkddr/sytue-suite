# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Sytue Cloud builder (worker de build Kaniko) — imagen multi-stage. Contexto de
# build = RAÍZ del repo. Binario estático sobre distroless (rootless).
# Nota: el builder NO ejecuta Kaniko dentro de esta imagen; programa Jobs de
# Kaniko en Kapsule (rootless) y observa su resultado.

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
    -ldflags "-s -w -X git.sytue.eu/sytue/one/services/cloud/internal/config.Version=${VERSION}" \
    -o /out/cloud-builder ./services/cloud/cmd/cloud-builder

# --- Stage 2: runtime ---
FROM gcr.io/distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
WORKDIR /app
COPY --from=build /out/cloud-builder /app/cloud-builder

USER nonroot:nonroot

# Puerto de salud del builder (CONVENTIONS §6).
EXPOSE 8091
ENV PORT=8091

HEALTHCHECK --interval=15s --timeout=5s --retries=10 --start-period=20s \
    CMD ["/app/cloud-builder", "healthcheck"]

ENTRYPOINT ["/app/cloud-builder"]
