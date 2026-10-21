#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# QUÉ ARQUITECTURA NECESITA CADA IMAGEN, en UN solo sitio.
#
# POR QUÉ EXISTE. Hasta ahora todo se publicaba para las dos y la mitad no la
# usaba nadie. No es un detalle de coste: los trece frontends hacen un
# `docker build` COMPLETO por arquitectura —57 pasos con `bun install` y `astro
# build`—, y tres de esos a la vez en los 6 vCPU del runner es lo que mató la
# publicación de la v3.3.7 con `context deadline exceeded` contra el demonio.
#
# QUIÉN NECESITA QUÉ, medido el 2026-09-21 y no supuesto:
#
#   $ kubectl get nodes -o wide            # el piloto, un solo nodo
#     sytue-server  Ready  control-plane  v1.36.2+k3s1  Ubuntu 26.04.1  (amd64)
#   $ ssh sytue-rooms-media uname -m       # el frontal, donde vive sytue.com
#     aarch64
#
# Y lo que corre en cada uno:
#
#   - PILOTO (amd64): los frontends Astro+Preact y todos los servicios Go.
#   - FRONTAL (arm64): el segundo juego de frontends Astro construido con
#     `DOMINIO=sytue.com` (tipo `fecom`). El backend y los datos siguen en el
#     piloto y se alcanzan por sus nombres públicos.
#
# USO:
#   scripts/arquitecturas-imagen.sh sytue-id      # → amd64
#   scripts/arquitecturas-imagen.sh               # → el catálogo entero:
#                                                 #   tipo \t imagen \t arqs \t ref
set -euo pipefail

raiz="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PERFIL="${PERFIL:-piloto}"

case "$(uname -m)" in
  aarch64 | arm64) nativa=arm64 ;;
  *) nativa=amd64 ;;
esac

arquitecturas_de() { echo amd64; }

if [ $# -gt 0 ]; then
  arquitecturas_de "$1"
  exit 0
fi

# ------------------------------------------------- el catálogo, de un solo sitio
# Sale de `build-images.sh`, que es el que usa el servidor, por lo mismo de
# siempre: dos listas se separan en cuanto alguien añade un servicio y el
# síntoma es un pod en ImagePullBackOff que nadie relaciona con este fichero.
bi="$raiz/deploy/helm/bin/build-images.sh"
[ -f "$bi" ] || { echo "no encuentro $bi" >&2; exit 1; }

# En el perfil piloto no se despliegan —y por tanto no se publican— las de
# Sytue Cloud. El filtro es el mismo que `solo_con_cloud` allí.
sin_cloud() { [[ "$1" =~ ^sytue-(cloud-api|cloud-builder|cloud-controller)$ ]]; }

declare -a lineas=()
while read -r nombre dockerfile; do
  [ "$PERFIL" = piloto ] && sin_cloud "$nombre" && continue
  lineas+=("$(printf 'go\t%s\t%s\t%s' "$nombre" "$(arquitecturas_de "$nombre")" "$dockerfile")")
done < <(grep -oE '^build_go [a-z0-9-]+ [^ ]+' "$bi" | awk '{print $2" "$3}')

# `platform` no sale de esta lista y no es un olvido: reutiliza la imagen de
# `console`, así que no tiene imagen propia que publicar.
apps="$(grep -oE '^for app in [a-z0-9 -]+' "$bi" | head -1 | sed 's/^for app in //')"
for app in $apps; do
  [ "$PERFIL" = piloto ] && [ "$app" = cloud-dashboard ] && continue
  nombre="sytue-frontend-$app"
  lineas+=("$(printf 'fe\t%s\t%s\t%s' "$nombre" "$(arquitecturas_de "$nombre")" "$app")")
done

# ------------------------------------------------- el segundo juego: sytue.com
#
# `sytue.com` sirve LA MISMA suite Astro que `sytue.eu` (ADR 0049) desde el
# FRONTAL, con datos y backend compartidos: lo único que se duplica es el
# frontend. Y hay que duplicarlo porque los frontends Astro hornean las URL
# públicas al construir: la misma imagen servida en `.com` enseñaría la suite
# ahí y hablaría con `.eu`.
#
# ARM64 Y NO amd64, que es lo contrario del juego de arriba: estas van al
# frontal (`uname -m` → aarch64), no al piloto. Sale gratis en tiempo: los
# runners son ARM, así que este juego es el NATIVO —sin `bun install --cpu` ni
# `crane rebase`— mientras que el de `.eu` es el cruzado.
#
# Se lista aquí y no en el flujo para que lo vea el guardián de `cerrar`
# (`comprobar-arquitecturas-publicadas.sh`): una imagen de este juego publicada
# con la arquitectura del otro no falla al descargarse, falla al arrancar.
if [ "${SYTUE_JUEGO_COM:-si}" = si ]; then
  for app in $apps; do
    [ "$PERFIL" = piloto ] && [ "$app" = cloud-dashboard ] && continue
    lineas+=("$(printf 'fecom\t%s\t%s\t%s' "sytue-frontend-$app-com" arm64 "$app")")
  done
fi

# UN CATÁLOGO VACÍO NO FALLA, SOLO NO SUBE NADA: si un día alguien reindenta
# `build_go` o reescribe ese `for`, el grep deja de casar y la release publicaría
# cero imágenes en verde. Aquí se dice, que es lo que no pasaba antes.
[ "${#lineas[@]}" -ge 20 ] || {
  echo "el catálogo salió con ${#lineas[@]} imágenes, y eso no puede ser: revisa los grep sobre $bi" >&2
  exit 1
}
printf '%s\n' "${lineas[@]}"
