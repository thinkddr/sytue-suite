#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# ¿El tar que se le va a dar a trivy está ENTERO? Se comprueba ANTES de
# invocarlo, para que el fallo diga qué mirar y no sea un error del escáner
# sobre formatos de imagen.
#
# DOS COSAS, Y LA SEGUNDA COSTÓ UNA RELEASE EN ROJO:
#
# 1. QUE LLEVE `manifest.json`. Trivy lee un tar en formato Docker —con su
#    `manifest.json` dentro— o un DIRECTORIO OCI, pero no un *tar* OCI. Con el
#    exportador anterior fallaba con «file manifest.json not found in tar» y las
#    once imágenes de los servicios llevaron sin mirarse de verdad desde que
#    existe el job: no es que salieran limpias, es que el escáner nunca llegaba
#    a abrirlas.
#
# 2. QUE ESTÉN TODOS LOS BLOBS QUE ESE MANIFIESTO REFERENCIA. El 2026-09-21
#    `contacts` salió con su `manifest.json` puesto, `docker save` devolvió 0 y
#    el guardián de entonces —que solo miraba el punto 1— lo dio por bueno; a
#    trivy le faltaba un blob y el job murió con «file blobs/sha256/b10ea080…
#    not found in tar». Un guardián que solo mira el manifiesto deja pasar
#    exactamente ese fallo.
#
# Y si el manifiesto no declara ni un blob reconocible, también se falla: un
# guardián que no encuentra nada que comprobar y sale en verde no protege nada.
#
# uso: comprobar-tar-docker.sh <fichero.tar>
set -uo pipefail

tar_img="${1:-}"
[ -n "$tar_img" ] || { echo "uso: $0 <fichero.tar>" >&2; exit 2; }

[ -s "$tar_img" ] || {
  echo "✗ $tar_img está vacío o no existe: el export de la imagen falló en silencio" >&2
  exit 1
}

entradas="$(mktemp)"
trap 'rm -f "$entradas"' EXIT
tar -tf "$tar_img" 2>/dev/null | sort -u > "$entradas"

grep -qx 'manifest.json' "$entradas" || {
  echo "✗ $tar_img no lleva manifest.json: no es un tar en formato Docker y trivy no lo abrirá." >&2
  echo "  Se produce con 'scripts/construir-imagen.sh --tar' (exportador type=docker)." >&2
  echo "  Un export OCI aquí deja el escaneo en nada." >&2
  exit 1
}

# Los DOS repartos que produce `docker save`: el clásico (`<hash>/layer.tar` y
# `<hash>.json`) y el del content store de containerd (`blobs/sha256/<hash>`),
# que es el que sale en el runner desde que lleva Docker 29 con
# `driver-type: io.containerd.snapshotter.v1`.
referencias="$(tar -xOf "$tar_img" manifest.json 2>/dev/null |
  grep -oE 'blobs/sha256/[0-9a-f]{64}|[0-9a-f]{64}(/layer\.tar|\.json)' | sort -u)"

[ -n "$referencias" ] || {
  echo "✗ $tar_img: manifest.json no declara ni un blob reconocible." >&2
  echo "  O el formato cambió, o el manifiesto está trunco. Sin blobs que comprobar," >&2
  echo "  este guardián no estaría comprobando nada." >&2
  exit 1
}

ausentes=""
while IFS= read -r referencia; do
  [ -n "$referencia" ] || continue
  grep -qxF "$referencia" "$entradas" || ausentes="$ausentes $referencia"
done <<< "$referencias"

[ -z "$ausentes" ] || {
  echo "✗ $tar_img referencia blobs que NO están dentro del tar:" >&2
  for ausente in $ausentes; do echo "    $ausente" >&2; done
  echo "  El export salió a medias: el exportador devolvió 0 pero faltan capas." >&2
  echo "  Con el builder clásico pasaba cuando OTRO job borraba imágenes mientras" >&2
  echo "  este construía; con BuildKit (ADR 0066) no debería: mira su journal" >&2
  echo "  (journalctl -u sytue-buildkit-ci) y el espacio de /srv/ci." >&2
  exit 1
}

echo "· $tar_img íntegro: $(printf '%s\n' "$referencias" | wc -l | tr -d ' ') blobs referenciados, todos presentes"
