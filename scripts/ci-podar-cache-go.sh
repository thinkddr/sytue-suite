#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# PONE TOPE A LA CACHÉ DE COMPILACIÓN DE GO QUE COMPARTEN LOS TRES RUNNERS.
#
#   ci-podar-cache-go.sh [raiz=/var/cache/sytue-ci] [tope_gb=12] [dias=7]
#
# POR QUÉ EXISTE. Desde que `/var/cache/sytue-ci/go-build` y `go-mod` se montan
# en todos los contenedores de job (ver .github/RUNNERS.md), la caché ya no se
# tira a cada ejecución: crece. Y crece sin techo. Medido en el piloto, que lleva
# meses compilando este mismo módulo: `~/.cache/go-build` = 28 GB con 143 604
# ficheros. El disco del frontal está al 76 % (137 de 180 GB) y ahí abajo también
# viven Pangolin y el SFU de Rooms EN PRODUCCIÓN: una caché sin tope no deja un
# CI lento, deja el frontal lleno. Ya pasó el 2026-09-20 —«la caché de módulos de
# Go del CI, envenenada por un disco lleno»—, donde un `go mod download` a medias
# dejó un zip truncado que luego se sirvió a todos los jobs.
#
# LA PODA DE GO NO BASTA, y se comprobó: Go recorta solo las entradas sin usar en
# 5 días (deja su marca en `trim.txt`, y en el piloto había corrido ese mismo
# día). Aun así eran 28 GB, porque en 5 días de compilar cabe mucho. El recorte
# por antigüedad es de Go; el TOPE no lo pone nadie, y es este guion.
#
# ───────────────────────────── POR QUÉ ES SEGURO MIENTRAS SE CONSTRUYE ────────
# Un job dura como mucho 3 h (`timeout: 3h` en los tres `config.yml`), y aquí no
# se borra NADA cuyo atime o mtime esté dentro de las últimas 24 h. Así que un
# fichero que una construcción en curso haya tocado —leído o escrito— no es
# candidato, por definición y con holgura de 8×. La raíz va montada con
# `relatime` (comprobado con `findmnt`), que es justo lo que hace que leer un
# fichero con atime de más de 24 h lo actualice: es la garantía en la que se
# apoya esto, no una suposición.
#
# Esa propiedad es la que evita repetir el incidente del 2026-09-21, en el que
# una limpieza global le borró las capas a un job a media construcción. Aun así
# el temporizador dispara de madrugada (ver el instalador): las dos cosas, porque
# la hora tranquila es una costumbre y el atime es una garantía.
#
# ───────────────────────────── QUÉ SE PODA Y QUÉ NO ───────────────────────────
# `go-build` es direccionable por contenido: cada fichero es independiente y
# borrar uno de más solo provoca que se recompile ese paquete. Se poda por
# fichero, sin riesgo.
#
# `go-mod` NO. Un módulo extraído es un ÁRBOL y Go no comprueba que esté
# completo: media carpeta borrada no se detecta, se lee, y el fallo sale luego en
# otro sitio —que es la forma exacta del envenenamiento del 2026-09-20—. Y
# además no hace falta: está acotado por `go.sum` y se midió en 209 MB para las
# imágenes de la suite, 2,1 GB en un puesto con meses de trabajo encima. Así que
# aquí solo se MIDE y se avisa.
# ponytail: techo conocido. El día que `go-mod` pase de su aviso, lo correcto no
# es borrar ficheros sueltos sino carpetas `<módulo>@<versión>` enteras por
# antigüedad; hasta entonces sería código sin un problema que resolver.
set -uo pipefail

raiz="${1:-${SYTUE_CI_CACHE_RAIZ:-/var/cache/sytue-ci}}"
tope_gb="${2:-${SYTUE_CI_CACHE_TOPE_GB:-12}}"
dias="${3:-${SYTUE_CI_CACHE_DIAS:-7}}"

construccion="$raiz/go-build"
modulos="$raiz/go-mod"
tope=$((tope_gb * 1024 * 1024 * 1024))
# El suelo de protección, en minutos, que es lo que entiende `find -amin`.
# `-atime +0` sería «más de 24 h» en días enteros y redondea hacia donde no
# queremos; en minutos se dice lo que se quiere decir.
protegido=1440

tam() { du -sb "$1" 2>/dev/null | cut -f1; }
gb() { awk -v b="${1:-0}" 'BEGIN { printf "%.1f", b / 1073741824 }'; }

[ -d "$construccion" ] || { echo "· no hay $construccion: nada que podar"; exit 0; }

antes="$(tam "$construccion")"
echo "── go-build: $(gb "$antes") GB antes (tope ${tope_gb} GB, antigüedad ${dias} d)"

# 1. POR ANTIGÜEDAD. `-amin +$protegido` sobra aquí —$dias es mucho mayor que un
#    día—, pero se deja escrito para que la regla se lea en los dos sitios y para
#    que bajar `dias` a 1 por una urgencia no se coma la protección sin querer.
find "$construccion" -type f -atime "+$dias" -amin "+$protegido" -delete 2>/dev/null

# 2. POR TAMAÑO, si aún sobra. Se borra lo menos usado primero, que es lo que
#    menos va a hacer falta luego, y se para en cuanto se entra en el tope.
ahora="$(tam "$construccion")"
if [ "${ahora:-0}" -gt "$tope" ]; then
  echo "── sigue en $(gb "$ahora") GB: se borra lo menos usado hasta bajar del tope"
  sobra=$((ahora - tope))
  # `-amin +1440` NO es decorativo en este paso: aquí sí se llega a ficheros
  # recientes, y es el único cerrojo que impide robarle el suelo a un job vivo.
  find "$construccion" -type f -amin "+$protegido" -printf '%A@\t%s\t%p\0' 2>/dev/null |
    sort -zn |
    while IFS=$'\t' read -r -d '' _ bytes ruta; do
      [ "$sobra" -gt 0 ] || break
      rm -f "$ruta" 2>/dev/null && sobra=$((sobra - bytes))
    done
fi

# Go guarda en trim.txt cuándo recortó por última vez; si lo borramos, vuelve a
# recortar antes de tiempo. Cuesta un fichero de 11 bytes dejarlo en su sitio.
find "$construccion" -mindepth 1 -type d -empty -delete 2>/dev/null

despues="$(tam "$construccion")"
echo "── go-build: $(gb "$despues") GB después (liberados $(gb $((antes - despues))) GB)"

if [ -d "$modulos" ]; then
  m="$(tam "$modulos")"
  echo "── go-mod: $(gb "$m") GB (no se poda; ver la cabecera)"
  if [ "${m:-0}" -gt $((5 * 1024 * 1024 * 1024)) ]; then
    echo "AVISO: go-mod pasa de 5 GB. Con los runners parados: rm -rf $modulos" >&2
  fi
fi

# Nunca falla: limpiar no es el trabajo, y un fallo aquí no puede parecerse a un
# CI roto. Es la misma regla que ci-podar-docker.sh.
exit 0
