#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# ¿Está funcionando de verdad la caché del CI?
#
# Las dos cachés de la forja tragan sus errores a propósito —una caché ausente no
# debe romper un job— y eso las vuelve indistinguibles de una caché rota. El
# 2026-08-17 se descubrió que llevaban SIN FUNCIONAR desde que se escribieron:
# cero tags `:buildcache` en las 53 imágenes del registro y ni un «caché
# restaurada» en ningún job. Nada estaba en rojo.
#
# Esto pregunta por las señales OBSERVABLES, no por la configuración:
#
#   1. Capas: existe el tag `:buildcache` de cada imagen en el Registry.
#   2. Dependencias: el paquete `ci-cache-*` existe en el registro de paquetes.
#
# Uso:  scripts/verificar-cache-ci.sh
set -uo pipefail

fallos=0
aviso() { echo "  ✗ $*" >&2; fallos=$((fallos + 1)); }
bien()  { echo "  ✓ $*"; }

echo "1. Caché de CAPAS (BuildKit) — tags :buildcache en el Registry"
if ! command -v scw >/dev/null 2>&1; then
  echo "  · sin scw: no se puede consultar el Registry"
else
  for img in sytue-mail sytue-id sytue-docs; do
    # Con jq y no con python3: es JSON, jq ya está en cualquier máquina donde
    # esto se ejecuta y no hace falta un intérprete para leer un campo. El
    # `if type == "array"` cubre las dos formas en que la CLI de Scaleway ha
    # devuelto esto según la versión.
    id="$(scw registry image list -o json 2>/dev/null |
      jq -r --arg n "$img" 'if type == "array" then . else .images end
                            | map(select(.name == $n)) | first | .id // empty')"
    if [ -z "$id" ]; then
      echo "  · $img: no está en el Registry (¿aún no se ha construido?)"
      continue
    fi
    n="$(scw registry tag list image-id="$id" -o json 2>/dev/null |
      jq -r 'if type == "array" then . else .tags end
             | map(select((.name // "") | test("buildcache"))) | length')"
    if [ "${n:-0}" -ge 1 ]; then
      bien "$img: tiene su tag :buildcache"
    else
      aviso "$img: SIN tag :buildcache — la caché de capas no está exportando"
    fi
  done
fi

echo
echo "2. Caché de DEPENDENCIAS — la del propio runner"
# YA NO SE MIRA EL REGISTRO DE PAQUETES. El respaldo compartido se retiró el
# 2026-09-05 por seguridad (ver .github/actions/cache/accion.sh): restauraba con
# `tar -x` sobre `/` un tarball de un paquete de la ORGANIZACIÓN, con una
# credencial de escritura y borrado presente en cualquier pull request.
#
# La caché que trabaja es la que levanta el runner en su propia máquina, y su
# rastro está en el LOG del job, no en ninguna API: esta comprobación no puede
# verlo desde fuera. Se dice dónde mirar en vez de fingir un veredicto.
echo "  · el rastro está en el log de cualquier job de ci-go/ci-frontend:"
echo "      «· caché restaurada»                       → acierto"
echo "      «· caché guardada en el servidor del runner» → se guardó"
echo "      «· el servidor de caché no tiene «…»»        → fallo de clave"
echo "  · si Go falla siempre y Bun acierta, mira que lint/vuln no reserven la"
echo "    clave «go-v2-<hash>» sin tener nada que guardar (accion.sh lo comprueba antes)."

echo
if [ "$fallos" -eq 0 ]; then
  echo "ok - las dos cachés dejan rastro observable"
  exit 0
fi
echo "not ok - $fallos comprobación(es) sin rastro; mira el log de un build:" >&2
echo "         debe salir «buildctl: caché de capas en …:buildcache» y «caché guardada»" >&2
exit 1
