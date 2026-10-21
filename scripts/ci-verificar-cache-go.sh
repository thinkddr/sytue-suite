#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# VERIFICA LA CACHÉ DE MÓDULOS DE GO QUE COMPARTEN LOS TRES RUNNERS, Y QUITA LO ROTO.
#
#   ci-verificar-cache-go.sh [--seco] [--todo] [raiz=/var/cache/sytue-ci/go-mod]
#
#   0 = todo bien, o lo roto se ha quitado (el siguiente job lo vuelve a bajar)
#   1 = hay algo roto que NO se ha quitado (con --seco, o porque no se pudo)
#
# POR QUÉ. Un disco lleno ya envenenó esta caché una vez (6c6d15084): un zip
# truncado que todo job posterior volvía a leer —«zip: not a valid zip file»— y
# ci-go en rojo en sus cuatro jobs sin que el código tuviera nada que ver. Go
# verifica al usar, y FALLA; no repara. Y como la caché solo la montan los
# runners del frontal, el mismo commit cae en casa y pasa en el pool de git.gay:
# un rojo que parece aleatorio y no lo es.
#
# QUÉ MIRA, por módulo (`cache/download/<m>/@v/<v>.zip` y su árbol `<m>@<v>`):
#   · Que el zip acabe en su directorio central. Uno truncado no lo tiene. Son
#     4 bytes con `od`: el frontal no trae `unzip`.
#   · Que el árbol extraído —lo que compilan los jobs— dé el hash de su
#     `.ziphash`, con el algoritmo de Go (dirhash «h1»: sha256 de las líneas
#     «<sha256>  <m>@<v>/<fichero>» en orden de bytes). Un árbol a medias, un
#     fichero truncado o un `.ziphash` roto salen aquí.
#
# CUÁNDO. Cada hora, lo escrito desde la ronda anterior: es exactamente lo que un
# ENOSPC puede dejar a medias, y así no depende de que el journal haya podido
# apuntar el ENOSPC, que con el disco lleno es de lo primero que falla. Y una vez
# al día, todo (o con --todo).
#
# QUÉ QUITA. Solo el módulo roto —su zip, su `.ziphash` y su árbol— y con el MISMO
# cerrojo (`<v>.lock`, flock) que toma Go al bajarlo y extraerlo: nunca pisa a un
# job que lo esté escribiendo. El `.mod` y el `.info` se quedan; esos los valida
# go.sum en cada uso. Nada de `go clean -modcache`: tiraría 2,9 GB y los forks
# privados, que solo se vuelven a bajar desde los jobs que llaman a forks.sh.
set -uo pipefail

SECO=0
TODO=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    --seco) SECO=1 ;;
    --todo) TODO=1 ;;
    *) break ;;
  esac
  shift
done
RAIZ="${1:-${SYTUE_CI_CACHE_RAIZ:-/var/cache/sytue-ci}/go-mod}"
ESTADO="${SYTUE_CI_ESTADO:-/var/lib/sytue-ci}"
MARCA="$ESTADO/verificar-cache-go.ronda"     # inicio de la última ronda buena
DIARIA="$ESTADO/verificar-cache-go.completa" # la última vez que se miró todo

[ -d "$RAIZ/cache/download" ] || { echo "· no hay $RAIZ/cache/download: nada que verificar"; exit 0; }
mkdir -p "$ESTADO"
inicio="$(mktemp "$ESTADO/.ronda.XXXXXX")"
trap 'rm -f "$inicio"' EXIT

completa="$TODO"
[ -e "$MARCA" ] || completa=1
[ -n "$(find "$DIARIA" -mmin -1380 2>/dev/null)" ] || completa=1

# Go escribe las mayúsculas de rutas y versiones como «!minúscula»; el hash usa
# la ruta de verdad.
decodificar() { sed -E 's/!([a-z])/\U\1/g' <<<"$1"; }

# dirhash.HashDir(dir, prefijo, dirhash.Hash1), en shell.
h1_arbol() {
  local lineas hex
  lineas="$(cd "$1" && find . -type f -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum)" || return 1
  [ -n "$lineas" ] || return 1
  # «<sha256>  ./f» → «<sha256>  <m>@<v>/f»: el nombre, tal cual lo hashea Go.
  lineas="${lineas//  .\//"  $2/"}"
  hex="$(printf '%s\n' "$lineas" | sha256sum | cut -d' ' -f1)" || return 1
  # El resumen va en base64 de los bytes, no del hexadecimal.
  # shellcheck disable=SC2001 # una sustitución por cada par: no cabe en ${//}
  printf 'h1:%s' "$(printf '%b' "$(sed 's/../\\x&/g' <<<"$hex")" | base64)"
}

zip_entero() {
  local tam
  tam="$(stat -c %s "$1")" || return 1
  [ "$tam" -ge 22 ] && [ "$(od -An -tx1 -j $((tam - 22)) -N4 "$1" | tr -d ' \n')" = 504b0506 ]
}

vistos=0
rotos=0
quitados=0
while IFS= read -r -d '' zip; do
  base="${zip%.zip}"
  v="${base##*/}"
  m="${base%/@v/*}"
  m="${m#"$RAIZ"/cache/download/}"
  arbol="$RAIZ/$m@$v"
  if [ "$completa" = 0 ]; then
    [ "$zip" -nt "$MARCA" ] || [ "$base.ziphash" -nt "$MARCA" ] || [ "$arbol" -nt "$MARCA" ] || continue
  fi
  vistos=$((vistos + 1))

  exec {fd}>>"$base.lock" || continue
  if ! flock -w 60 "$fd"; then
    echo "· $m@$v: lo tiene un job desde hace más de 60 s; en la siguiente ronda"
    exec {fd}>&-
    continue
  fi
  motivo=""
  # Con .partial, Go lo está extrayendo o lo rehará él solo la próxima vez.
  if [ ! -e "$base.partial" ]; then
    if ! zip_entero "$zip"; then
      motivo="zip truncado"
    elif [ -d "$arbol" ] && [ -s "$base.ziphash" ]; then
      # No poder LEERLO no es que esté roto: se deja y se dice.
      if h="$(h1_arbol "$arbol" "$(decodificar "$m@$v")")"; then
        [ "$h" = "$(<"$base.ziphash")" ] || motivo="el árbol no da el hash de su .ziphash"
      else
        echo "· $m@$v: no se ha podido leer el árbol; se deja" >&2
      fi
    fi
  fi
  if [ -n "$motivo" ]; then
    rotos=$((rotos + 1))
    if [ "$SECO" = 1 ]; then
      echo "✗ $m@$v: $motivo (--seco: se deja)" >&2
    else
      chmod -R u+w "$arbol" 2>/dev/null
      rm -rf "$arbol"
      rm -f "$zip" "$base.ziphash"
      if [ -e "$zip" ] || [ -e "$arbol" ]; then
        echo "✗ $m@$v: $motivo, y NO se ha podido quitar" >&2
      else
        quitados=$((quitados + 1))
        echo "✓ $m@$v: $motivo → quitado; el siguiente job que lo pida lo vuelve a bajar"
      fi
    fi
  fi
  exec {fd}>&-
done < <(find "$RAIZ/cache/download" -type f -name '*.zip' -print0)

alcance="lo escrito desde la ronda anterior"
[ "$completa" = 1 ] && alcance="toda la caché"
echo "── go-mod ($alcance): $vistos módulos, $rotos rotos, $quitados quitados"

[ "$rotos" -eq "$quitados" ] || exit 1
mv -f "$inicio" "$MARCA"
[ "$completa" = 1 ] && touch "$DIARIA"
exit 0
