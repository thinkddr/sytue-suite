#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Las capturas de la ficha de la App Store de cada app (apps/fyne/appstore/README.md):
#
#   capturas.sh <dir>     copia a <app>/capturas/ las elegidas de <dir>/<clase>/<app>/, que es
#                         lo que deja el recorrido con SYTUE_CAPTURAS_PLAY=<dir>/<clase>/<app>
#                         y la SYTUE_CAPTURAS_MEDIDA de esa clase (iphone, ipad, mac)
#   capturas.sh --seleccion <clase> <app>  imprime solo la selección para el CI
#
# No cambia ni un píxel: el recorrido ya escribe PNG RGB sin alfa (la ventana es opaca) a
# la medida exacta, y appstore-publicar.py --seco es rojo si no. Si hay ImageMagick, solo
# los vuelve a comprimir sin pérdida (de 33 a 24 MB las 138 del 2026-10-12).
set -euo pipefail
AQUI="$(cd "$(dirname "$0")" && pwd)"
dir="${1:?uso: $0 <dir con iphone/ ipad/ mac/>}"
im="$(command -v magick || command -v convert || true)"

# Las capturas de cada app y clase, por el número que les pone el recorrido
# (NNN-lo_pulsado.png), en el orden de la ficha. Se eligen mirando lo que deja el recorrido:
# la pantalla principal primero, y nada con un error o un «cargando» a la vista. El número
# cambia cuando cambia la app: tras regenerar, se vuelven a mirar.
elegidas() { # <clase> <app>
  case "$1/$2" in
  iphone/bases) echo 001 005 015 019 ;;
  iphone/calcus) echo 072 001 ;;
  iphone/calendar) echo 001 034 054 062 ;;
  iphone/cast) echo 001 008 011 032 ;;
  iphone/certipdf) echo 019 020 023 ;;
  iphone/contacts) echo 001 004 002 003 ;;
  iphone/diapos) echo 002 013 ;;
  iphone/docus) echo 005 013 001 ;;
  iphone/drive) echo 001 007 013 ;;
  iphone/erp) echo 001 014 003 036 ;;
  iphone/hub) echo 001 002 ;;
  iphone/mail) echo 001 003 014 002 ;;
  iphone/notes) echo 001 044 040 041 ;;
  iphone/photos) echo 003 014 036 ;;
  iphone/rooms) echo 001 004 002 003 ;;
  # iPad y Mac: revisadas visualmente el 2026-10-14 antes de publicar la ficha.
  # Se excluyen errores del servicio falso, estados aún cargando y texto de prueba.
  ipad/bases) echo 001 003 ;;
  ipad/calcus) echo 068 001 ;;
  ipad/calendar) echo 001 102 101 096 ;;
  ipad/cast) echo 001 014 019 043 ;;
  ipad/certipdf) echo 040 041 044 ;;
  ipad/contacts) echo 001 ;;
  ipad/diapos) echo 002 013 ;;
  ipad/docus) echo 005 013 001 ;;
  ipad/drive) echo 001 014 004 007 ;;
  ipad/erp) echo 001 ;;
  ipad/hub) echo 001 002 ;;
  ipad/mail) echo 011 022 010 001 ;;
  ipad/notes) echo 001 ;;
  ipad/photos) echo 012 024 ;;
  ipad/rooms) echo 001 024 008 ;;
  # En el Mac la numeración se corre uno donde aparece una «inicio-desplazada» de más.
  mac/bases) echo 001 003 ;;
  mac/calcus) echo 069 001 ;;
  mac/calendar) echo 001 103 102 096 ;;
  mac/cast) echo 001 016 022 ;;
  mac/certipdf) echo 040 041 044 ;;
  mac/contacts) echo 001 ;;
  mac/diapos) echo 002 013 ;;
  mac/docus) echo 005 014 001 ;;
  mac/drive) echo 001 014 004 007 ;;
  mac/erp) echo 001 ;;
  mac/hub) echo 001 002 ;;
  mac/mail) echo 011 022 010 001 ;;
  mac/notes) echo 001 ;;
  mac/photos) echo 003 014 ;;
  mac/rooms) echo 001 024 008 ;;
  *) echo "capturas.sh: no hay capturas elegidas para $1/$2" >&2; return 1 ;;
  esac
}

if [ "$dir" = --seleccion ]; then
  elegidas "${2:?falta clase}" "${3:?falta app}"
  exit 0
fi

for app in "$AQUI"/*/; do
  app="$(basename "$app")"
  destino="$AQUI/$app/capturas"
  mkdir -p "$destino"
  for clase in iphone ipad mac; do
    lista="$(elegidas "$clase" "$app")"
    rm -f "$destino/$clase"-*.png
    i=0
    for n in $lista; do
      i=$((i + 1))
      origen="$(find "$dir/$clase/$app" -maxdepth 1 -name "$n-*.png" | sort | sed -n 1p)"
      [ -n "$origen" ] || { echo "capturas.sh: falta $dir/$clase/$app/$n-*.png" >&2; exit 1; }
      cp "$origen" "$destino/$clase-$i.png"
      [ -z "$im" ] || "$im" "$destino/$clase-$i.png" -strip -define png:compression-level=9 \
        -define png:compression-strategy=1 "PNG24:$destino/$clase-$i.png"
    done
  done
done
echo "capturas.sh: copiadas"
