#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# LA FIRMA DE LO QUE SE PUBLICA: imágenes (la marca `desplegable/…`) e instaladores
# (`SHA256SUMS`). Un formato y un verificador para los tres sitios que la miran: el
# CI que firma, el piloto que despliega y las apps que se actualizan.
#
#   firma-publicacion.sh firmar <fichero>
#       Escribe <fichero>.sig. Lo llama el CI, desde el entorno `publicacion`.
#   firma-publicacion.sh verificar <clave.pub> <fichero> <fichero.sig>
#   firma-publicacion.sh verificar-marca <clave.pub>            < mensaje de la etiqueta
#       Si verifica, escribe por stdout el cuerpo: lo que se lea de ahí está firmado.
#   firma-publicacion.sh verificar-sumas <clave.pub> <SHA256SUMS> <SHA256SUMS.sig> <vX.Y.Z>
#   firma-publicacion.sh cuerpo-marca                            < mensaje de la etiqueta
#       El cuerpo canónico, sin verificar nada (para firmarlo o para el modo aviso).
#   firma-publicacion.sh sumas <vX.Y.Z> <commit> <directorio>     > SHA256SUMS
#       El SHA256SUMS de los instaladores de escritorio que haya en el directorio.
#
# QUÉ ES LA FIRMA. ECDSA P-256 sobre SHA-256, en ASN.1 DER y en base64, en una
# línea. La produce `cosign sign-blob` —es el campo `messageSignature.signature`
# de su bundle— y se verifica SIN cosign: con openssl aquí, con `crypto/ecdsa` en
# las apps. Comprobado con cosign 3.1.3 el 2026-10-06.
#
# EL INTERRUPTOR ES LA CLAVE PÚBLICA (deploy/firma/publicacion.pub), y es el mismo
# para todos. Mientras no exista, nadie firma y nadie exige: se avisa y se sigue,
# porque la clave privada vive en un entorno protegido de GitHub que todavía no
# existe. En cuanto esté en el repositorio, `firmar` sin poder firmar es ROJO: lo
# publicado no verificaría en ningún sitio.
#
# QUIEN VERIFICA PONE LA CLAVE, y nunca la saca de lo que está verificando: el
# piloto la toma del árbol que ya tiene desplegado, no del tag que va a desplegar.
# Por eso `verificar*` la piden por argumento y `firmar` no.
set -euo pipefail

RAIZ="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PUBLICA="${SYTUE_FIRMA_PUBLICA:-$RAIZ/deploy/firma/publicacion.pub}"

uso() { sed -n '9,18p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//' >&2; exit 2; }

# verificar <clave.pub> <fichero> <firma: fichero .sig o el base64 tal cual>
verificar() {
  local clave="$1" fichero="$2" firma="$3" der
  [ -s "$clave" ] || { echo "no hay clave pública en $clave" >&2; return 1; }
  [ -f "$fichero" ] || { echo "no existe $fichero" >&2; return 1; }
  [ ! -f "$firma" ] || firma="$(tr -d '[:space:]' <"$firma")"
  [ -n "$firma" ] || { echo "la firma está vacía" >&2; return 1; }
  der="$(mktemp)"
  if ! printf '%s' "$firma" | base64 -d >"$der" 2>/dev/null; then
    rm -f "$der"
    echo "la firma no es base64" >&2
    return 1
  fi
  if openssl dgst -sha256 -verify "$clave" -signature "$der" "$fichero" >/dev/null 2>&1; then
    rm -f "$der"
    return 0
  fi
  rm -f "$der"
  echo "la firma no verifica con $clave" >&2
  return 1
}

# El cuerpo canónico de una marca: lo anterior a `firma:`, sin líneas vacías y con
# cada línea acabada en LF. Se reconstruye así en los dos extremos para que no
# importe qué haga git con los saltos del final del mensaje.
# Ninguno de los dos corta la lectura a medias: con pipefail, quien escribe a una
# tubería que ya nadie lee muere por SIGPIPE y el acierto sale como fallo.
cuerpo_marca() { tr -d '\r' | awk '/^firma: /{fin = 1} !fin && NF'; }
firma_marca() { tr -d '\r' | awk '/^firma: / && !ya {sub(/^firma: /, ""); print; ya = 1}'; }
version_de_sumas() { awk '/^# version: / && !ya {sub(/^# version: /, ""); print; ya = 1}' "$1"; }

verificar_marca() {
  local clave="$1" tmp
  tmp="$(mktemp -d)"
  cat >"$tmp/mensaje"
  cuerpo_marca <"$tmp/mensaje" >"$tmp/cuerpo"
  if [ "$(head -1 "$tmp/cuerpo")" != 'sytue-desplegable 1' ]; then
    rm -rf "$tmp"
    echo "la marca no lleva el manifiesto «sytue-desplegable 1»" >&2
    return 1
  fi
  if ! verificar "$clave" "$tmp/cuerpo" "$(firma_marca <"$tmp/mensaje")"; then
    rm -rf "$tmp"
    return 1
  fi
  cat "$tmp/cuerpo"
  rm -rf "$tmp"
}

verificar_sumas() {
  local clave="$1" sumas="$2" firma="$3" version="$4"
  verificar "$clave" "$sumas" "$firma" || return 1
  [ "$(sed -n '1p' "$sumas")" = '# sytue-instaladores 1' ] || {
    echo "$sumas no es un «sytue-instaladores 1»" >&2
    return 1
  }
  # Sin esto, un SHA256SUMS viejo y bien firmado serviría para colar un instalador viejo.
  [ "$(version_de_sumas "$sumas")" = "$version" ] || {
    echo "$sumas es de la versión «$(version_de_sumas "$sumas")», no de $version" >&2
    return 1
  }
}

# El SHA256SUMS de los instaladores: tres comentarios de cabecera (sha256sum -c los
# ignora) y una línea por instalador de escritorio, ordenadas por nombre. Solo los
# instaladores: un .sig, un SBOM o un AAB que haya en la release no se listan.
sumas() {
  local version="$1" commit="$2" dir="$3"
  [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "«${version}» no es vX.Y.Z" >&2; return 1; }
  [[ "$commit" =~ ^[0-9a-f]{40}$ ]] || { echo "«${commit}» no es un commit de 40 hex" >&2; return 1; }
  [ -d "$dir" ] || { echo "no existe $dir" >&2; return 1; }
  local nombres
  nombres="$(cd "$dir" && find . -maxdepth 1 -type f \( -name '*.dmg' -o -name '*-installer.exe' -o -name '*-linux-*.tar.xz' \) \
    -printf '%f\n' | LC_ALL=C sort)"
  [ -n "$nombres" ] || { echo "no hay ningún instalador en $dir" >&2; return 1; }
  printf '# sytue-instaladores 1\n# version: %s\n# commit: %s\n' "$version" "$commit"
  (cd "$dir" && printf '%s\n' "$nombres" | xargs -d '\n' sha256sum)
}

firmar() {
  local fichero="$1" bundle firma
  [ -f "$fichero" ] || { echo "no existe $fichero" >&2; return 1; }
  rm -f "$fichero.sig"
  if [ ! -s "$PUBLICA" ]; then
    echo "::warning title=Publicación sin firmar::no hay clave pública en deploy/firma/publicacion.pub, así que $(basename "$fichero") se publica SIN FIRMA y nadie la exige todavía. Se enciende creando el entorno «publicacion» con su clave y subiendo la pública a esa ruta."
    return 0
  fi
  if [ -z "${SYTUE_FIRMA_CLAVE:-}" ]; then
    echo "::error::hay clave pública en el repositorio y este job no tiene SYTUE_FIRMA_CLAVE: sin firma, el piloto y las apps rechazarían $(basename "$fichero"). ¿Falta el secreto en el entorno «publicacion», o el job no corre en él?" >&2
    return 1
  fi
  for h in cosign jq openssl; do
    command -v "$h" >/dev/null 2>&1 || { echo "::error::falta $h para firmar" >&2; return 1; }
  done
  bundle="$(mktemp)"
  # Mismas banderas que las firmas de release.yml: sin registro de transparencia
  # (saca fuera de la UE qué publicamos y cuándo) y sin la configuración de firma
  # de Sigstore, que con clave propia no existe. `--bundle` es obligatorio en
  # cosign 3: `--output-signature` ya no escribe nada.
  if ! COSIGN_PRIVATE_KEY="$SYTUE_FIRMA_CLAVE" COSIGN_PASSWORD="${SYTUE_FIRMA_PASSWORD:-}" \
    cosign sign-blob --yes --key env://COSIGN_PRIVATE_KEY --tlog-upload=false \
    --use-signing-config=false --bundle "$bundle" "$fichero" >/dev/null; then
    rm -f "$bundle"
    echo "::error::cosign no ha firmado $(basename "$fichero")" >&2
    return 1
  fi
  firma="$(jq -er '.messageSignature.signature' "$bundle")" || firma=""
  rm -f "$bundle"
  [ -n "$firma" ] || { echo "::error::el bundle de cosign no trae messageSignature.signature" >&2; return 1; }
  # SE VERIFICA LO QUE SE ACABA DE FIRMAR, contra la clave del repositorio y con el
  # mismo openssl que usará quien despliegue. Si en el entorno se pegó otra clave, o
  # cosign cambia un día de formato, el rojo sale aquí y no en el piloto.
  verificar "$PUBLICA" "$fichero" "$firma" || {
    echo "::error::la firma recién hecha no verifica con deploy/firma/publicacion.pub: la clave del entorno no es la pareja de la del repositorio" >&2
    return 1
  }
  printf '%s\n' "$firma" >"$fichero.sig"
  echo "firmado: $(basename "$fichero")"
}

case "${1:-}" in
  firmar) [ $# -eq 2 ] || uso; firmar "$2" ;;
  verificar) [ $# -eq 4 ] || uso; verificar "$2" "$3" "$4" ;;
  verificar-marca) [ $# -eq 2 ] || uso; verificar_marca "$2" ;;
  verificar-sumas) [ $# -eq 5 ] || uso; verificar_sumas "$2" "$3" "$4" "$5" ;;
  cuerpo-marca) [ $# -eq 1 ] || uso; cuerpo_marca ;;
  sumas) [ $# -eq 4 ] || uso; sumas "$2" "$3" "$4" ;;
  *) uso ;;
esac
