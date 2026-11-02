#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# ¿TERMINÓ RENOVATE, O ABANDONÓ A MEDIAS?
#
#   renovate-resultado.sh <registro JSON de Renovate (LOG_FILE)> [full|lookup]
#   renovate-resultado.sh --propuestas <registro JSON de Renovate (LOG_FILE)>
#
#   0 = procesó el repositorio entero, o completó la consulta solicitada
#   1 = abortó, o no se sabe: el registro no existe o no llega al final
#   --propuestas sólo extrae un array sanitizado; no acredita haber terminado.
#
# Renovate sale con 0 también cuando abandona: el 2026-10-05 un corte contra npm
# lo dejó en `external-host-error`, `dependencias.yml` contó cero actualizaciones
# pendientes y escribió «todo lo que el catálogo declara está en su última
# estable». No había abierto una PR desde la migración (auditoría 2026-10-06,
# CI-12). El código de salida no dice nada; lo dice la línea «Repository
# finished» de su registro, que es lo que se lee aquí.
set -euo pipefail

accion=comprobar
if [ "${1:-}" = "--propuestas" ]; then
  accion=propuestas
  shift
fi
registro="${1:?uso: renovate-resultado.sh <registro JSON de Renovate> [full|lookup]}"
modo="${2:-full}"
[ -s "$registro" ] || {
  echo "Renovate no dejó registro en $registro: no se sabe qué hizo, y eso no es «todo al día»." >&2
  exit 1
}
if [ "$accion" = propuestas ]; then
  # Sólo nombres y versiones/propuestas: jamás se exporta el JSON de Renovate,
  # que contiene también configuración y credenciales de registros. En el log
  # de lookup depName está en el padre y newValue dentro de updates[]. Otras
  # fases escriben ambos en el mismo objeto; se admiten las dos formas.
  jq -Rn '
    def texto_seguro:
      if type != "string" then null
      elif test("[[:cntrl:]?#]") or test("://[^/]*@") then null else . end;
    [inputs | fromjson? | .. | objects
     | select(.depName? | type == "string") | . as $dep
     | (.updates[]?, .) as $propuesta
     | select($propuesta.newValue? | type == "string")
     | {depName: ($dep.depName | texto_seguro),
        currentValue: ($dep.currentValue // $dep.currentVersion // null | texto_seguro),
        currentDigest: ($dep.currentDigest // null | texto_seguro),
        newValue: ($propuesta.newValue | texto_seguro),
        newVersion: ($propuesta.newVersion // null | texto_seguro),
        newDigest: ($propuesta.newDigest // null | texto_seguro),
        updateType: ($propuesta.updateType // null | texto_seguro),
        datasource: ($dep.datasource // null | texto_seguro)}
     | select(.depName != null and .newValue != null)]
    | unique_by([.depName, .currentValue, .currentDigest, .newValue, .newVersion, .newDigest, .updateType, .datasource])
  ' "$registro"
  exit 0
fi
case "$modo" in
  full) ;;
  lookup)
    # Renovate 44.190.2 no asigna result al completar dryRun=lookup (también
    # en GitHub): se salta update/finalize. No basta con aceptar su ausencia.
    # Exigimos extracción, consultas terminadas y cierre SIN resultado de aborto,
    # en ese orden y dentro del mismo repositorio/contexto. El marcador de
    # consultas se escribe a nivel debug, sólo al resolver todos los gestores.
    if jq -Rne '
      reduce (inputs | fromjson? | select(type == "object")) as $linea
        ({};
         if $linea.msg == "Repository started" then
           {repository: $linea.repository, logContext: $linea.logContext,
            extraido: false, consultado: false, completo: false}
         elif (.repository | type) == "string" and
              (.logContext | type) == "string" and
              $linea.repository == .repository and $linea.logContext == .logContext then
           if $linea.msg == "Dependency extraction complete" then .extraido = true
           elif $linea.msg == "Package releases lookups complete" then .consultado = .extraido
           elif $linea.msg == "Repository finished" then
             .completo = (.consultado and ($linea | has("result") | not))
           else . end
         else . end)
      | .completo == true
    ' "$registro" >/dev/null; then
      echo "Renovate terminó la consulta de versiones sin actualizar el repositorio."
      exit 0
    fi
    echo "Renovate no acreditó extracción, consultas completas y cierre sin aborto del mismo repositorio." >&2
    exit 1
    ;;
  *)
    echo "Modo de resultado de Renovate no admitido: $modo" >&2
    exit 1
    ;;
esac
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
