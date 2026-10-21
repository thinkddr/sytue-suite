# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Sytue OCR — imagen multi-stage. El contexto de build es la RAÍZ del repo
# (módulo único git.sytue.eu/sytue/one con go.mod en la raíz).
#
# ES LA ÚNICA IMAGEN DE LA SUITE QUE NO ES DISTROLESS, y el motivo está aquí
# arriba para que nadie lo "arregle": el motor es Tesseract 5, un binario en C
# con sus datos de idioma, y distroless no tiene gestor de paquetes con el que
# instalarlo. Copiar el binario y adivinar sus bibliotecas compartidas produce
# una imagen que arranca y falla al abrir el primer TIFF.
#
# A cambio, se sostiene todo lo demás: base fijada por DIGEST, sin recomendados,
# sin cachés de apt, proceso no-root y ninguna llamada saliente en tiempo de
# ejecución — el modelo viaja dentro.

# --- Stage 1: build del binario Go ---
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
    -ldflags "-s -w -X git.sytue.eu/sytue/one/services/ocr/internal/config.Version=${VERSION}" \
    -o /out/ocr ./services/ocr/cmd/ocr

# --- Stage 2: runtime con el motor ---
#
# EL DIGEST ES EL DEL ÍNDICE MULTIARCO, no el del manifiesto de amd64.
#
# La etiqueta `docker` la atienden runners de DOS arquitecturas (los tres
# propios de `sytue-rooms-media` son ARM64; los compartidos de git.gay, amd64),
# y cuál te toca es una tirada de dados. Con el digest de amd64, en un runner
# ARM64 la etapa de runtime arranca y el primer `RUN` muere con «exec /bin/sh:
# exec format error» —medido el 2026-09-21 en el job 383016 de la ejecución
# 2456, que dejó el barrido nocturno de trivy en rojo sin haber escaneado la
# imagen—. Era el ÚNICO `FROM` fijado del repositorio que no apuntaba a un
# índice: golang, oven/bun y distroless ya lo hacen.
#
# En amd64 no cambia ni un byte: `sha256:f3034a6e…`, el digest que había aquí,
# es exactamente el miembro `linux/amd64` de este índice.
FROM debian:bookworm-slim@sha256:3783cc01769c7b2b1b83a5c5ad96c815348e28ed7da68e2e3687004faa906251 AS runtime

# Tesseract 5 y los datos de los CINCO idiomas de producto (CONVENTIONS §13.1).
# `osd` detecta orientación y escritura: sin él, una página escaneada del revés
# se reconoce como ruido en vez de rotarse.
#
# --no-install-recommends es obligatorio, no cosmético: sin él entran decenas de
# paquetes de escritorio que multiplican por cuatro el tamaño y la superficie.
#
# EL `upgrade` NO SOBRA, y fijar un digest más nuevo no lo sustituye. Este es el
# único servicio de la suite sobre Debian —los demás son distroless, que no lleva
# ni bibliotecas del sistema— y por eso es el único que arrastra los CVE de la
# base. El nocturno de trivy lo tumbó el 2026-09-13 con dos HIGH de `libpcre2-8-0`
# (CVE-2026-86145, escritura fuera de límites, y CVE-2026-89161, corrupción de
# memoria en `pcre2_jit_match`).
#
# Lo que se comprobó antes de escribir esto: la imagen `debian:bookworm-slim` MÁS
# RECIENTE sigue trayendo `10.42-1`, y el arreglo `10.42-1+deb12u1` solo está en el
# repositorio. O sea que actualizar el digest no arregla nada: Debian publica el
# parche en apt y no reconstruye la imagen. El `upgrade` es lo que lo trae, y de
# paso cubre el siguiente CVE de la base sin que haya que volver aquí.
ARG SYTUE_SECURITY_REFRESH=2026-10-01
RUN : "$SYTUE_SECURITY_REFRESH" \
 && apt-get update \
 && apt-get upgrade -y --no-install-recommends \
 && apt-get install -y --no-install-recommends \
      tesseract-ocr \
      tesseract-ocr-osd \
      tesseract-ocr-spa \
      tesseract-ocr-eng \
      tesseract-ocr-fra \
      tesseract-ocr-deu \
      tesseract-ocr-ita \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=build /out/ocr /app/ocr

ARG VERSION
ARG COMMIT
ENV SYTUE_BUILD_VERSION=${VERSION}
ENV SYTUE_BUILD_COMMIT=${COMMIT}

# Usuario no-root con el MISMO uid que usa distroless en el resto de la suite, para
# que las políticas de PodSecurity y los `runAsUser` del chart valgan igual aquí.
RUN groupadd --gid 65532 nonroot \
 && useradd --uid 65532 --gid 65532 --no-create-home --shell /usr/sbin/nologin nonroot
USER 65532:65532

# Puerto HTTP de Sytue OCR (CONVENTIONS §6: siguiente hueco tras la API de Analytics 8095).
EXPOSE 8096
ENV PORT=8096

# El sistema de ficheros del contenedor va de solo lectura desde el chart, así
# que el temporal de cada página va a un emptyDir montado en /tmp.
ENV TMPDIR=/tmp

HEALTHCHECK --interval=15s --timeout=5s --retries=10 --start-period=20s \
    CMD ["/app/ocr", "healthcheck"]

ENTRYPOINT ["/app/ocr"]
