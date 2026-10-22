#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# ¿TERMINÓ RENOVATE, O ABANDONÓ A MEDIAS?
#
#   renovate-resultado.sh <registro JSON de Renovate (LOG_FILE)>
#
#   0 = procesó el repositorio entero («done», o «automerged»)
#   1 = abortó, o no se sabe: el registro no existe o no llega al final
#
# Renovate sale con 0 también cuando abandona: el 2026-10-05 un corte contra npm
# lo dejó en `external-host-error`, `dependencias.yml` contó cero actualizaciones
# pendientes y escribió «todo lo que el catálogo declara está en su última
# estable». No había abierto una PR desde la migración (auditoría 2026-10-06,
# CI-12). El código de salida no dice nada; lo dice la línea «Repository
# finished» de su registro, que es lo que se lee aquí.
set -euo pipefail

registro="${1:?uso: renovate-resultado.sh <registro JSON de Renovate>}"
[ -s "$registro" ] || {
  echo "Renovate no dejó registro en $registro: no se sabe qué hizo, y eso no es «todo al día»." >&2
  exit 1
}
# La última: si Renovate procesara más de un repositorio, cada uno escribe la suya.
resultado="$(grep -h '^{' "$registro" |
  jq -Rr 'fromjson? | select(.msg == "Repository finished") | .result // "sin-resultado"' | tail -1)"
case "$resultado" in
  done | automerged)
    echo "Renovate terminó el repositorio: $resultado"
    ;;
  "")
    echo "Renovate no llegó a «Repository finished»: se quedó a medias y no hay veredicto." >&2
    exit 1
    ;;
  *)
    echo "Renovate abandonó el repositorio con «${resultado}»: lo que diga de actualizaciones pendientes no vale." >&2
    exit 1
    ;;
esac
