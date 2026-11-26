#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# LA ÚNICA FORMA DE BAJAR UNA HERRAMIENTA DEL CI. Se carga con `.`; la usan
# `.github/actions/toolchain/accion.sh` y `bajar-herramienta.sh`, que hasta el
# 2026-10-05 llevaban cada uno su `curl` con sus números, y el segundo se había
# quedado con los que al primero ya le habían fallado tres veces.
#
#   descargar_reanudable <destino> <url> [opciones de curl…]
#
# Deja el fichero en <destino> o devuelve el código de curl. NO comprueba el
# sha256: eso lo hace quien llama, que es quien sabe cuál toca. Las opciones de
# más van DESPUÉS de las de aquí, así que la última gana: `--retry 0` las quita.
#
# CADA NÚMERO SALE DE UN ROJO QUE PARECÍA DEL CÓDIGO.
#
# `--retry 3` se agotaba en SIETE SEGUNDOS: el backoff de curl es 1+2+4, así
# que un origen que devuelve 504 durante un cuarto de minuto tumbaba el job.
# Pasó el 2026-09-21: `scripts de producción` y `osv-scanner` murieron los dos
# bajando Bun, con cuatro 504 en quince segundos.
#
# Con 6 reintentos el backoff llega a 1+2+4+8+16+32 = 63 s, y `--retry-max-time`
# lo corta a 180 para que un origen caído de verdad no deje el job colgado cinco
# minutos por descarga. `--retry-all-errors` hace falta porque curl NO reintenta
# por sí solo los fallos de conexión y de TLS, que es la otra forma en que esto
# se cae.
#
# `--speed-limit`/`--speed-time` para la tercera: una descarga que no falla pero
# se ATASCA. El 2026-09-27 la de Go 1.27.1 llevaba 1 MB de 67 a los cinco
# minutos; `--max-time 300` la cortó, pero ese único intento ya había gastado
# más que `--retry-max-time 180` y no quedó reintento. Por debajo de 100 KB/s
# durante 30 s se aborta con el mismo código 28, y ESE sí cabe en la ventana.
#
# Y la cuarta: LENTA SIN LLEGAR A ATASCARSE. El 2026-10-05, con los tres runners
# ocupados y las imágenes de una release publicándose, la de Go iba a ~130 KB/s:
# por encima del suelo, así que nadie la abortó, y a los 300 s llevaba 39 MB de
# 67. Corte, ningún reintento, y `scripts de producción` en rojo sin haber
# ejecutado un solo test. Los reintentos de curl no sirven aquí —tiran lo
# descargado y empiezan de cero, al mismo ritmo—, así que un 28 se REANUDA con
# `-C -` en otro curl: tres tandas de 300 s al suelo de 100 KB/s son 90 MB, y el
# tarball de Go —lo mayor que se ha visto cortado— pesa 67. Solo el 28: un 404
# o un origen caído siguen fallando igual de pronto que antes.
#
# Lo prueba .github/tests/descarga-reanudable-test.sh.
descargar_reanudable() {
  local destino="$1" url="$2" tanda codigo=0
  shift 2
  # Con `-C -` lo que hubiera en el destino se tomaría por media descarga de ESTA
  # dirección: un resto de otro origen acabaría pegado delante de lo nuevo.
  rm -f "$destino"
  for tanda in 1 2 3; do
    [ "$tanda" -eq 1 ] || echo "  descarga cortada por tiempo: tanda $tanda de 3, se reanuda donde iba" >&2
    codigo=0
    curl -fsSL -C - --max-time 300 --speed-limit 102400 --speed-time 30 \
      --retry 6 --retry-max-time 180 --retry-all-errors --retry-connrefused \
      "$@" -o "$destino" "$url" || codigo=$?
    [ "$codigo" -eq 28 ] || break
  done
  return "$codigo"
}
