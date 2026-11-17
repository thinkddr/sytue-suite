#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# PUBLICA UNA IMAGEN DE TERCEROS RECONSTRUIDA (deploy/upstream-reconstruidas).
#
#   DESTINO=ghcr.io/thinkddr scripts/publicar-upstream-reconstruida.sh <nombre>
#
# Construye las plataformas declaradas en el catálogo con el BuildKit del CI,
# publica el índice en `$DESTINO/upstream-<nombre>:<etiqueta>`, comprueba que es
# un índice multiarquitectura, lo escanea con trivy con los MISMOS criterios que
# el gate de `security`, y añade a `$SALIDA` la línea lista para
# deploy/production/upstream-images.txt.
#
# Sale con 1 si la imagen publicada conserva hallazgos CRITICAL/HIGH con arreglo:
# una reconstrucción que no quita lo que venía a quitar no debe entrar en el
# catálogo sin que alguien lo mire. La imagen queda publicada igualmente (la
# etiqueta es inmutable), y el informe dice qué falta.
set -euo pipefail

nombre="${1:?uso: publicar-upstream-reconstruida.sh <nombre>}"
: "${DESTINO:?falta DESTINO (p. ej. ghcr.io/thinkddr)}"
SALIDA="${SALIDA:-/dev/stdout}"
raiz="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dir="$raiz/deploy/upstream-reconstruidas/$nombre"
catalogo="$raiz/deploy/upstream-reconstruidas/catalogo.txt"

[ -f "$dir/Dockerfile" ] || { echo "no existe $dir/Dockerfile" >&2; exit 2; }
linea="$(awk -v n="$nombre" '$1 == n { print; exit }' "$catalogo")"
[ -n "$linea" ] || { echo "$nombre no está en $catalogo" >&2; exit 2; }
read -r _ etiqueta plataformas _ <<<"$linea"
plataformas="${plataformas:-linux/amd64,linux/arm64}"
case "$etiqueta" in
  *-sytue.[0-9]*) ;;
  *) echo "etiqueta «$etiqueta» sin el sufijo -sytue.<n>: no se distinguiría de la de upstream" >&2; exit 2 ;;
esac

ref="$DESTINO/upstream-$nombre:$etiqueta"
# INMUTABLE. Ver la cabecera del catálogo.
if crane digest "$ref" >/dev/null 2>&1; then
  if [ "${REUTILIZAR_PUBLICADA:-}" = 1 ]; then
    echo "$ref ya está publicada: se reutiliza tal cual (REUTILIZAR_PUBLICADA=1)"
  else
    echo "$ref YA EXISTE y la etiqueta es inmutable: sube el -sytue.<n> en $catalogo" >&2
    exit 1
  fi
else
  echo "── construyendo $ref ($plataformas)"
  "$raiz/scripts/construir-imagen.sh" --platform "$plataformas" \
    -f "$dir/Dockerfile" --push "$ref" "$dir"
fi

digest="$(crane digest "$ref")"
tipo="$(crane manifest "$ref@$digest" | jq -r .mediaType)"
case "$tipo" in
  *manifest.list.v2+json|*image.index.v1+json) ;;
  *) echo "$ref@$digest es $tipo y no un índice: el catálogo exige el índice multiarquitectura" >&2; exit 1 ;;
esac
for p in ${plataformas//,/ }; do
  crane digest --platform "$p" "$ref@$digest" >/dev/null ||
    { echo "$ref@$digest no trae $p" >&2; exit 1; }
done

# El MISMO escaneo que hará trivy-upstream sobre esta línea, para enterarse aquí
# y no en el nocturno. Por plataforma: un `apt-get upgrade` emulado que fallara
# en una sola arquitectura dejaría la otra con el paquete viejo.
fallos=0
for p in ${plataformas//,/ }; do
  echo "── trivy $ref@$digest ($p)"
  trivy image "$ref@$digest" --platform "$p" \
    --ignorefile "$raiz/.trivyignore.yaml" \
    --scanners vuln --severity CRITICAL,HIGH --ignore-unfixed \
    --exit-code 1 --format table || fallos=1
done

origen="$(sed -n 's/^# *reconstruida-de: *//p' "$dir/Dockerfile" | head -1)"
[ -n "$origen" ] || { echo "falta la línea «# reconstruida-de: <imagen>:<tag>» en $dir/Dockerfile" >&2; exit 2; }
printf '%s %s %s %s reconstruida-de=%s\n' "$nombre" "$etiqueta" "$DESTINO/upstream-$nombre" "$digest" "$origen" >>"$SALIDA"
[ "$fallos" -eq 0 ] || echo "::warning::$nombre: la reconstrucción conserva hallazgos con arreglo (ver arriba)"
exit "$fallos"
