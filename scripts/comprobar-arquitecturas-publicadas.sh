#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# LO PUBLICADO TIENE QUE PODER ARRANCAR EN SU DESTINO, imagen por imagen.
#
# POR QUÉ EXISTE. Los Dockerfiles no fijan plataforma, así que cada imagen sale
# con la arquitectura del runner que la construyó. Cuando los runners se mudaron
# a `sytue-rooms-media` —ARM64— la tubería empezó a publicar arm64 para un
# servidor que es x86_64, y una imagen ajena NO falla al descargarse: falla al
# ARRANCAR, con `exec format error`, lejísimos de su causa. Medido el 2026-09-21
# en `sytue-id`: v3.1.13 arm64, v3.2.0 amd64, v3.2.2 y v3.2.3 arm64 — el
# servidor llevaba cuatro versiones sin poder actualizarse y nada salía en rojo.
#
# QUÉ CAMBIÓ AQUÍ. Antes esto miraba DOS imágenes de muestra contra UNA
# arquitectura esperada para todas. Ya no vale: desde que cada imagen se publica
# solo para la arquitectura de su destino, «la esperada» depende de la imagen.
# Una muestra de dos deja trece frontends sin mirar. Se comprueban TODAS, y cada una
# contra lo que dice `scripts/arquitecturas-imagen.sh`, que es el mismo fichero
# del que sale lo que se construye: si los dos no coinciden, es que algo se
# publicó mal, no que la lista esté desactualizada.
#
# SOBRAR NO ES FALLAR. Una arquitectura de más no rompe nada en producción —solo
# se pagó por construirla—, así que sale como aviso. Lo que tumba la release es
# que FALTE la que el destino necesita, porque la marca `desplegable/` es lo que
# hace que el servidor se la descargue.
#
# USO (en el job `cerrar` de imagenes-produccion.yml):
#   DESTINO=ghcr.io/thinkddr TAG=v3.3.8 PERFIL=piloto \
#   SYTUE_GIT_USER=… SYTUE_GIT_PACKAGES_TOKEN=… \
#     scripts/comprobar-arquitecturas-publicadas.sh
#
# ponytail: `SYTUE_ARQ_PUBLICADAS=<fichero>` («imagen<TAB>arqs» por línea)
# sustituye la consulta al registro. Es la costura que deja probar este guardián
# —incluido verlo en ROJO— sin registro ni red; ver
# deploy/production/tests/arquitecturas-por-destino-test.sh.
set -euo pipefail

: "${DESTINO:?falta DESTINO (p. ej. ghcr.io/thinkddr)}"
: "${TAG:?falta TAG}"

raiz="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

ACEPTA='application/vnd.oci.image.index.v1+json,application/vnd.oci.image.manifest.v1+json,application/vnd.docker.distribution.manifest.list.v2+json,application/vnd.docker.distribution.manifest.v2+json'

# Credencial para leer <imagen> en el registro, en forma de cabecera. El protocolo
# de distribución de Docker contesta 401 con `WWW-Authenticate: Bearer realm=…`, y
# el token se pide a ese `realm` con usuario y contraseña: así lo exige ghcr.io,
# que no acepta Basic en `/v2/` (el registro de git.gay sí lo aceptaba, y por eso
# esto era un `curl -u` directo). Si el registro no pide Bearer, se sigue en Basic.
cabecera() { # <imagen>
  local v2="https://${DESTINO%%/*}/v2/${DESTINO#*/}/$1" reto realm service token
  reto="$(curl -sS --max-time 60 -o /dev/null -D - "$v2/manifests/$TAG" 2>/dev/null |
    tr -d '\r' | sed -n 's/^[Ww][Ww][Ww]-[Aa]uthenticate: *Bearer //p' | head -1)" || true
  if [ -z "$reto" ]; then
    printf 'Authorization: Basic %s' "$(printf '%s:%s' "$SYTUE_GIT_USER" "$SYTUE_GIT_PACKAGES_TOKEN" | base64 | tr -d '\n')"
    return
  fi
  realm="$(sed -n 's/.*realm="\([^"]*\)".*/\1/p' <<<"$reto")"
  service="$(sed -n 's/.*service="\([^"]*\)".*/\1/p' <<<"$reto")"
  token="$(curl -sS --fail --max-time 60 -u "$SYTUE_GIT_USER:$SYTUE_GIT_PACKAGES_TOKEN" -G "$realm" \
    --data-urlencode "service=$service" --data-urlencode "scope=repository:${DESTINO#*/}/$1:pull" |
    bun -e 'const j = JSON.parse(await Bun.stdin.text()); process.stdout.write(j.token || j.access_token || "")')" || return 1
  [ -n "$token" ] || return 1
  printf 'Authorization: Bearer %s' "$token"
}

publicadas() { # <imagen> → «amd64,arm64»; sale != 0 si no se pudo leer
  if [ -n "${SYTUE_ARQ_PUBLICADAS:-}" ]; then
    awk -F'\t' -v i="$1" '$1 == i { print $2; hay = 1 } END { exit !hay }' \
      "$SYTUE_ARQ_PUBLICADAS"
    return
  fi
  local v2="https://${DESTINO%%/*}/v2/${DESTINO#*/}/$1" salida auth
  auth="$(cabecera "$1")" || return 1
  curl -sS --fail --max-time 60 -H "$auth" \
    -H "Accept: $ACEPTA" "$v2/manifests/$TAG" -o "$tmp/manifiesto.json" || return 1
  # EL DIGEST DE LO QUE SE ACABA DE LEER, para la marca `desplegable` (auditoría
  # 2026-10-06, CI-01): es la sha256 del manifiesto tal como lo sirve el registro,
  # o sea el `imagen@sha256:…` que resuelve hoy esa etiqueta. Se apunta en la misma
  # pasada que comprueba la arquitectura para no leer el registro dos veces.
  [ -z "${SYTUE_DIGESTS:-}" ] ||
    printf 'sha256:%s  %s:%s\n' "$(sha256sum "$tmp/manifiesto.json" | cut -d' ' -f1)" "$1" "$TAG" >>"$SYTUE_DIGESTS"
  # Un índice multiarquitectura ya trae las plataformas; uno simple obliga a
  # bajar su config, que es donde vive `architecture`.
  salida="$(bun -e '
    const m = await Bun.file(Bun.env.SYTUE_ARQ_MANIFIESTO).json();
    if (m.manifests) console.log("LISTA " + m.manifests.map((x) => x.platform?.architecture ?? "?").join(","));
    else console.log("CONFIG " + (m.config?.digest ?? ""));' 2>/dev/null)" || return 1
  case "$salida" in
    "LISTA "*) printf '%s\n' "${salida#LISTA }" ;;
    "CONFIG sha256:"*)
      curl -sSL --fail --max-time 60 -H "$auth" \
        "$v2/blobs/${salida#CONFIG }" -o "$tmp/config.json" || return 1
      bun -e 'console.log((await Bun.file(Bun.env.SYTUE_ARQ_CONFIG).json()).architecture ?? "")' ;;
    *) return 1 ;;
  esac
}
export SYTUE_ARQ_MANIFIESTO="$tmp/manifiesto.json" SYTUE_ARQ_CONFIG="$tmp/config.json"

fallos=0
avisos=0
total=0
while IFS=$'\t' read -r _tipo imagen esperadas _ref; do
  total=$((total + 1))
  if ! arqs="$(publicadas "$imagen")" || [ -z "$arqs" ]; then
    echo "  ✗ $imagen:$TAG — no se pudo leer su manifiesto en el registro" >&2
    fallos=$((fallos + 1))
    continue
  fi
  faltan=""
  for e in $esperadas; do
    case ",$arqs," in *",$e,"*) ;; *) faltan="$faltan $e" ;; esac
  done
  sobran=""
  for a in ${arqs//,/ }; do
    case " $esperadas " in *" $a "*) ;; *) sobran="$sobran $a" ;; esac
  done
  if [ -n "$faltan" ]; then
    echo "  ✗ $imagen publicado para «$arqs»; su destino necesita:$faltan" >&2
    fallos=$((fallos + 1))
  elif [ -n "$sobran" ]; then
    echo "  ⚠ $imagen publicado para «$arqs»: le sobra:$sobran (se pagó por construirla y no la despliega nadie)"
    avisos=$((avisos + 1))
  else
    echo "  ✓ $imagen → $arqs"
  fi
done < <("$raiz/scripts/arquitecturas-imagen.sh")

# Un catálogo que no devuelve nada dejaría este guardián en verde sin haber
# mirado una sola imagen, que es peor que no tenerlo.
[ "$total" -gt 0 ] ||
  { echo "el catálogo salió vacío: este guardián no ha comprobado NADA" >&2; exit 1; }

[ "$avisos" -eq 0 ] ||
  echo "$avisos imágenes traen arquitecturas que no usa ningún destino: mira scripts/arquitecturas-imagen.sh."

if [ "$fallos" -gt 0 ]; then
  echo "" >&2
  if [ "$fallos" -eq 1 ]; then
    echo "1 imagen no sirve en el destino al que va." >&2
  else
    echo "$fallos imágenes no sirven en el destino al que van." >&2
  fi
  echo "  Las imágenes ESTÁN subidas, pero NO se marca la versión como desplegable:" >&2
  echo "  el destino se las descargaría y los pods morirían con «exec format error»." >&2
  echo "  Los runners construyen en su propia arquitectura y los Dockerfiles no la" >&2
  echo "  fijan, así que esto pasa sin avisar cada vez que cambia la flota." >&2
  echo "  Quién necesita qué está en scripts/arquitecturas-imagen.sh, con la medida." >&2
  exit 1
fi
echo "✓ las $total imágenes de $TAG arrancan en su destino"
