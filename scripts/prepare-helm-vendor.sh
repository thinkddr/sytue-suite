#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos

# Materializa charts upstream desde archivos versionados y verificados. No hay
# acceso de red: el tgz oficial forma parte del repositorio y su hash está fijado.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly ROOT_DIR
readonly ARCHIVE="$ROOT_DIR/deploy/helm/upstream/traefik-41.1.0.tgz"
readonly EXPECTED_SHA256="0b4144b78251a1039913852991d9252419f35e69536043aa2be97d6226e3e7c3"
readonly VENDOR_ROOT="$ROOT_DIR/deploy/helm/.vendor"
readonly TARGET="$VENDOR_ROOT/traefik-41.1.0"
readonly MARKER="$TARGET/.sytue-archive-sha256"
# Parches propios sobre el chart oficial. Van aquí y no editando `.vendor/`
# porque ese directorio es material REGENERABLE —está en .gitignore— y cualquiera
# que lo retire y vuelva a ejecutar esto perdería el cambio sin enterarse.
readonly PATCH_DIR="$ROOT_DIR/deploy/helm/upstream/parches"

[ -f "$ARCHIVE" ] || {
  echo "falta el chart vendorizado: $ARCHIVE" >&2
  exit 1
}
actual_sha256="$(shasum -a 256 "$ARCHIVE" | awk '{print $1}')"
[ "$actual_sha256" = "$EXPECTED_SHA256" ] || {
  echo "checksum inválido para $ARCHIVE" >&2
  exit 1
}

# La huella incluye los PARCHES, no solo el archivo: al cambiar uno hay que
# rehacer el árbol, y sin esto el marcador seguiría coincidiendo y el parche
# nuevo no se aplicaría nunca.
huella_esperada="$EXPECTED_SHA256"
if [ -d "$PATCH_DIR" ]; then
  parches_sha="$(cat "$PATCH_DIR"/*.patch 2>/dev/null | shasum -a 256 | awk '{print $1}')"
  huella_esperada="$EXPECTED_SHA256+$parches_sha"
fi

if [ -f "$MARKER" ] && [ "$(cat "$MARKER")" = "$huella_esperada" ]; then
  exit 0
fi
# HUELLA DISTINTA: SE REHACE EL ÁRBOL, no se pide ayuda a un humano.
#
# Antes se abortaba aquí con «existe pero no coincide con el archivo fijado;
# retíralo y reintenta», y eso convierte CUALQUIER cambio de parche en un
# despliegue que no puede terminar solo —justo el caso en el que hay que
# revendorizar—. El 2026-09-05 dejó el despliegue de v2.13.1 en punto muerto
# después de tocar un parche de Traefik; con el actualizador automático
# encendido habría sido un fallo cada diez minutos sin nadie que lo leyera.
#
# Borrarlo es seguro: este directorio es material regenerable —está en
# .gitignore— y sale entero del archivo fijado por sha256 más los parches, que
# es lo que la huella acaba de comprobar.
#
# Se retira SOLO lo que escribió este script, reconocible por su marcador. Un
# directorio sin marcador no es nuestro y sigue abortando: puede haberlo puesto
# alguien a mano, y tirarlo sin avisar sería peor que parar.
if [ -e "$TARGET" ]; then
  [ -f "$MARKER" ] || {
    echo "$TARGET existe y NO lo generó este script (le falta el marcador);" >&2
    echo "  retíralo a mano si de verdad sobra, y reintenta" >&2
    exit 1
  }
  rm -rf "$TARGET"
fi

mkdir -p "$VENDOR_ROOT"
staging="$(mktemp -d "$VENDOR_ROOT/.traefik-41.1.0.XXXXXX")"
tar -xzf "$ARCHIVE" -C "$staging" --strip-components=1

# Los parches se aplican en orden y NINGUNO puede fallar: un chart a medio
# parchear es peor que uno sin parchear, porque parece el bueno.
shopt -s nullglob
for parche in "$PATCH_DIR"/*.patch; do
  echo "aplicando $(basename "$parche")"
  patch -s -p1 -d "$staging" <"$parche" || {
    echo "el parche $(basename "$parche") no aplica sobre el chart $EXPECTED_SHA256" >&2
    echo "  (¿se ha subido la versión del chart sin rehacer el parche?)" >&2
    rm -rf "$staging"
    exit 1
  }
done
shopt -u nullglob

printf '%s\n' "$huella_esperada" >"$staging/.sytue-archive-sha256"
mv "$staging" "$TARGET"
