#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# GENERA EL NÚCLEO DE CLIENTE QUE ROOMS SIRVE EN /api/v1/client-core/: los
# WebAssembly de apps/nucleo, cada uno con el `wasm_exec.js` y la licencia de LA
# MISMA toolchain que lo compiló (un runtime de otra versión de Go no arranca ese
# wasm, y el manifiesto publica los dos hashes juntos).
#
#   <directorio>/                core   ← apps/nucleo/cmd/pqc-wasm  (núcleo completo: Matrix, Rooms)
#   <directorio>/vault/          vault  ← apps/nucleo/cmd/vault-wasm (núcleo de bóveda, ADR 0086)
#   <directorio>/office/         office ← apps/nucleo/cmd/office-wasm
#   <directorio>/mail/           mail   ← apps/nucleo/cmd/mail-wasm
#   <directorio>/pdf/            pdf    ← apps/nucleo/cmd/pdf-wasm  (PDF de la bóveda, ADR 0086)
#
# cada uno con core.wasm, wasm_exec.js y GO-LICENSE, y core.wasm y wasm_exec.js
# también PRECOMPRIMIDOS: `.br` (brotli 11) y `.gz` (gzip 9) al lado, que Rooms sirve
# según el Accept-Encoding (tools/precomprimir). Brotli 11 de ~90 MB de wasm tarda un
# par de minutos en paralelo; se paga aquí una vez y no en cada descarga en Traefik.
#
# UNA SOLA RECETA, Y POR QUÉ. Esto vivía como un RUN en la etapa de compilación de
# services/rooms/Dockerfile, y la release NO construye los servicios con su
# Dockerfile: scripts/publicar-imagen-go.sh cruza el binario con `go build` y monta
# la imagen con `crane`, leyendo del Dockerfile solo la etapa de runtime. Así que
# el RUN no se ejecutaba nunca, el `//go:embed client_core_assets` se llevaba un
# directorio vacío y la v3.20.3 salió con Rooms contestando 503 «client core
# unavailable»: el cifrado de extremo a extremo de la web no se podía configurar
# (docs/operations.md#client-core-vacio). Ahora el Dockerfile llama a ESTE guion y
# publicar-imagen-go.sh lo reconoce en el Dockerfile y lo ejecuta antes de cruzar
# el binario. El build local (deploy/compose) y el del piloto (build-images.sh)
# pasan por el Dockerfile, así que lo heredan sin más.
#
# Los wasm no dependen de la arquitectura de destino del servidor: se generan una
# vez y valen para todas las imágenes.
#
# USO (desde cualquier sitio; el directorio relativo se toma del actual):
#   scripts/client-core-assets.sh services/rooms/internal/http/client_core_assets
set -euo pipefail

destino="${1:?uso: client-core-assets.sh <directorio>}"
raiz="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mkdir -p "$destino"
destino="$(cd "$destino" && pwd)"

# `go env GOROOT` DENTRO de apps/nucleo, que es donde se compila: con
# GOTOOLCHAIN=auto la toolchain la elige el go.mod del directorio, y el runtime
# tiene que ser el de la que compiló el wasm, no el de la que haya en el PATH.
cd "$raiz/apps/nucleo"
goroot="$(go env GOROOT)"

declare -a comprimir=()
for par in core:pqc-wasm vault:vault-wasm office:office-wasm mail:mail-wasm pdf:pdf-wasm; do
  modulo="${par%%:*}"
  dir="$destino"
  [ "$modulo" = core ] || dir="$destino/$modulo"
  mkdir -p "$dir"
  # -buildvcs=false: en el Dockerfile no hay .git (lo excluye .dockerignore) y en el
  # CI sí; sin esto el wasm saldría distinto según dónde se genere, y el hash del
  # manifiesto con él.
  CGO_ENABLED=0 GOOS=js GOARCH=wasm go build -trimpath -buildvcs=false -ldflags "-s -w" \
    -o "$dir/core.wasm" "./cmd/${par#*:}"
  cp "$goroot/lib/wasm/wasm_exec.js" "$dir/wasm_exec.js"
  cp "$goroot/LICENSE" "$dir/GO-LICENSE"
  # Una variante de una generación anterior junto a un wasm nuevo no casaría con su
  # huella: se borran antes de regenerarlas (Rooms además descarta la que no case).
  rm -f "$dir"/core.wasm.br "$dir"/core.wasm.gz "$dir"/wasm_exec.js.br "$dir"/wasm_exec.js.gz
  comprimir+=("$dir/core.wasm" "$dir/wasm_exec.js")
  echo "client core $modulo: $(wc -c <"$dir/core.wasm" | tr -d ' ') bytes en $dir"
done

# Desde la raíz: la herramienta es del módulo raíz, que es el que tiene brotli.
cd "$raiz"
go run ./tools/precomprimir "${comprimir[@]}"
