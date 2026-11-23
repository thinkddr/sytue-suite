// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

// empaquetarCurlDePlay es la API de Google Play de mentira del original: la misma para
// sondear y para producción, y solo persiste un track al hacer commit. Apunta cada
// petición (método y URL) en $PLAY_FALSO_ROOT/peticiones y cada OAuth en
// autenticaciones; PLAY_ESCENARIO elige la respuesta.
const empaquetarCurlDePlay = `#!/bin/bash
raiz="$PLAY_FALSO_ROOT"
printf '%s\n' "$*" >>"$raiz/argv"
salida=/dev/stdout w="" metodo=GET cuerpo="" codigo=200
while [ $# -gt 0 ]; do
  case "$1" in
  -o) salida="$2"; shift ;;
  -w) w="$2"; shift ;;
  -X) metodo="$2"; shift ;;
  --data | --data-binary)
    cuerpo="$2"; shift
    [ "$cuerpo" != @- ] || cuerpo="$(cat)"
    ;;
  esac
  url="$1"; shift
done
mkdir -p "$raiz/tracks" "$raiz/pendientes" "$raiz/internos"
paquete="${url#*/applications/}"; paquete="${paquete%%/*}"
if [ "${url#*oauth2}" != "$url" ]; then
  echo OAuth >>"$raiz/autenticaciones"
else
  printf '%s %s\n' "$metodo" "$url" >>"$raiz/peticiones"
fi
# Una sola fuente para el track editable y el resumen fuera de la edición. Los
# escenarios pin-* reproducen el borrador real anterior, no el tag que se sube.
produccion_actual() {
  if [ -f "$raiz/tracks/$paquete.json" ]; then
    cat "$raiz/tracks/$paquete.json"
    return
  fi
  case "${PLAY_ESCENARIO:-}:$paquete" in
  estable:eu.sytue.rooms) echo '{"track":"production","releases":[{"versionCodes":["3012004"],"status":"completed"}]}' ;;
  otro-borrador:eu.sytue.rooms) echo '{"track":"production","releases":[{"versionCodes":["3012005"],"status":"draft"}]}' ;;
  mismo:*) echo '{"track":"production","releases":[{"name":"Conservar","versionCodes":["3012004"],"status":"draft","releaseNotes":[{"language":"es-ES","text":"Notas revisadas"}]}]}' ;;
  pin-*:*)
    base='{"track":"production","releases":[{"name":"v3.12.24","versionCodes":["3012024"],"status":"draft","releaseNotes":[{"language":"es-ES","text":"Notas revisadas"},{"language":"en-US","text":"Reviewed notes"}],"inAppUpdatePriority":3}]}'
    case "${PLAY_ESCENARIO:-}:$paquete" in
    pin-parcial:eu.sytue.bases|pin-parcial:eu.sytue.calcus|pin-parcial:eu.sytue.calendar|pin-parcial:eu.sytue.cast|pin-parcial:eu.sytue.certipdf|pin-parcial:eu.sytue.contacts|pin-ya-completed-*:*)
      jq '.releases[0] |= (.name="v3.14.0" | .versionCodes=["3014000"] | .status="completed")' <<<"$base"
      return
      ;;
    esac
    if [ "$paquete" != eu.sytue.rooms ]; then echo "$base"; return; fi
    case "$PLAY_ESCENARIO" in
    pin-otro) jq '.releases[0].versionCodes=["3012023"]' <<<"$base" ;;
    pin-mezclado) jq '.releases[0].versionCodes=["3012024","3014000"]' <<<"$base" ;;
    pin-futuro) jq '.releases[0].versionCodes=["3014001"]' <<<"$base" ;;
    pin-completed) jq '.releases[0].status="completed"' <<<"$base" ;;
    pin-inProgress) jq '.releases[0].status="inProgress"' <<<"$base" ;;
    pin-halted) jq '.releases[0].status="halted"' <<<"$base" ;;
    pin-dos) jq '.releases += [.releases[0]]' <<<"$base" ;;
    pin-campo-desconocido) jq '.releases[0].futuro="no adivinar"' <<<"$base" ;;
    pin-countryTargeting) jq '.releases[0].countryTargeting={countries:["ES"]}' <<<"$base" ;;
    pin-userFraction) jq '.releases[0].userFraction=0.1' <<<"$base" ;;
    pin-notas-invalidas) jq '.releases[0].releaseNotes=[{language:"es-ES",text:42}]' <<<"$base" ;;
    pin-prioridad-invalida) jq '.releases[0].inAppUpdatePriority=6' <<<"$base" ;;
    *) echo "$base" ;;
    esac
    ;;
  *) echo '{"track":"production","releases":[]}' ;;
  esac
}
respuesta='{}'
case "$url" in
*oauth2*) respuesta='{"access_token":"TOKEN-QUE-NO-SE-VE"}' ;;
*/generatedApks/*)
  case "${PLAY_ESCENARIO:-}" in
  sin-huellas) respuesta='{"generatedApks":[]}' ;;
  huella-rota) respuesta='{"generatedApks":[{"certificateSha256Hash":"no-es-un-certificado"}]}' ;;
  falta-docus)
    if [ "$paquete" = eu.sytue.docus ]; then
      codigo=404; respuesta='{"error":{"code":404,"status":"NOT_FOUND","message":"App Bundle version not found"}}'
    else
      respuesta='{"generatedApks":[{"certificateSha256Hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}'
    fi
    ;;
  huellas-base64) respuesta='{"generatedApks":[{"certificateSha256Hash":"//////////////////////////////////////////8="},{"certificateSha256Hash":"__________________________________________8"},{"certificateSha256Hash":"FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF"}]}' ;;
  *) respuesta='{"generatedApks":[{"certificateSha256Hash":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},{"certificateSha256Hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},{"certificateSha256Hash":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}]}' ;;
  esac
  ;;
*/edits) respuesta='{"id":"e1"}' ;;
*/bundles)
  if [ "${PLAY_ESCENARIO:-}:$paquete" = interno-falta:eu.sytue.rooms ]; then
    respuesta='{"bundles":[]}'
  elif [[ "${PLAY_ESCENARIO:-}" == interno-coexistente* ]]; then
    respuesta='{"bundles":[{"versionCode":3014001}]}'
  else
    respuesta='{"bundles":[{"versionCode":3013004}]}'
  fi
  ;;
*/testers/internal) respuesta='{"googleGroups":["testers-privados@example.com"]}' ;;
*/tracks/internal)
  if [ "$metodo" = PUT ]; then
    if [ "${PLAY_ESCENARIO:-}:$paquete" = interno-rechazado:eu.sytue.rooms ]; then
      codigo=400; respuesta='{"error":{"code":400,"message":"Only releases with status draft may be created on draft app"}}'
    else
      printf '%s' "$cuerpo" >"$raiz/pendientes/$paquete-interno.json"
    fi
  else
  case "${PLAY_ESCENARIO:-}:$paquete" in
  interno-par-*:*)
    respuesta='{"track":"internal","releases":[{"name":"Borrador","versionCodes":["3013004"],"status":"draft","releaseNotes":[{"language":"es-ES","text":"Notas del borrador"}],"countryTargeting":{"countries":["ES"]},"inAppUpdatePriority":3},{"name":"Activo","versionCodes":["3013003"],"status":"completed","releaseNotes":[{"language":"es-ES","text":"Notas del activo"}],"countryTargeting":{"countries":["DE"]},"inAppUpdatePriority":1}]}'
    case "${PLAY_ESCENARIO:-}:$paquete" in
    interno-par-activo-primero:*) respuesta="$(jq '.releases |= reverse' <<<"$respuesta")" ;;
    interno-par-dos-drafts:eu.sytue.rooms) respuesta="$(jq '.releases[1].status="draft"' <<<"$respuesta")" ;;
    interno-par-dos-completed:eu.sytue.rooms) respuesta="$(jq '.releases[0].status="completed"' <<<"$respuesta")" ;;
    interno-par-parcial:eu.sytue.rooms) respuesta="$(jq '.releases[1].status="inProgress" | .releases[1].userFraction=0.5' <<<"$respuesta")" ;;
    interno-par-otro-tag:eu.sytue.rooms) respuesta="$(jq '.releases[0].versionCodes=["3013005"]' <<<"$respuesta")" ;;
    interno-par-mezclado:eu.sytue.rooms) respuesta="$(jq '.releases[0].versionCodes=["3013004","3013005"]' <<<"$respuesta")" ;;
    interno-par-igual:eu.sytue.rooms) respuesta="$(jq '.releases[1].versionCodes=["3013004"]' <<<"$respuesta")" ;;
    interno-par-futuro:eu.sytue.rooms) respuesta="$(jq '.releases[1].versionCodes=["3013005"]' <<<"$respuesta")" ;;
    interno-par-duplicado:eu.sytue.rooms) respuesta="$(jq '.releases[1].versionCodes=["3013003","3013003"]' <<<"$respuesta")" ;;
    interno-par-texto:eu.sytue.rooms) respuesta="$(jq '.releases[1].versionCodes=["no-numerico"]' <<<"$respuesta")" ;;
    interno-par-vacio:eu.sytue.rooms) respuesta="$(jq '.releases[1].versionCodes=[]' <<<"$respuesta")" ;;
    interno-par-fraccion:eu.sytue.rooms) respuesta="$(jq '.releases[0].userFraction=0.5' <<<"$respuesta")" ;;
    interno-par-halted:eu.sytue.rooms) respuesta="$(jq '.releases[1].status="halted"' <<<"$respuesta")" ;;
    interno-par-tres:eu.sytue.rooms) respuesta="$(jq '.releases += [.releases[1]]' <<<"$respuesta")" ;;
    esac
    ;;
  interno-vacio:*) respuesta='{"track":"internal","releases":[]}' ;;
  interno-coexistente*:*)
    respuesta='{"track":"internal","releases":[{"versionCodes":["3014001"],"status":"draft","name":"NO-MOSTRAR","releaseNotes":[{"language":"es-ES","text":"NO-MOSTRAR"}],"inAppUpdatePriority":2},{"versionCodes":["3014000"],"status":"completed","name":"NO-MOSTRAR"}]}'
    case "$PLAY_ESCENARIO" in
    interno-coexistente-inverso) respuesta="$(jq '.releases |= reverse' <<<"$respuesta")" ;;
    interno-coexistente-duplicado) respuesta="$(jq '.releases[1].versionCodes=["3014001"]' <<<"$respuesta")" ;;
    interno-coexistente-mixto) respuesta="$(jq '.releases[0].versionCodes += ["3014000"]' <<<"$respuesta")" ;;
    interno-coexistente-ausente) respuesta="$(jq '.releases[0].versionCodes=["3014002"]' <<<"$respuesta")" ;;
    interno-coexistente-ajeno-draft) respuesta="$(jq '.releases[1].status="draft"' <<<"$respuesta")" ;;
    interno-coexistente-ajeno-nuevo) respuesta="$(jq '.releases[1].versionCodes=["3014002"]' <<<"$respuesta")" ;;
    interno-coexistente-ajeno-mixto) respuesta="$(jq '.releases[1].versionCodes += ["3013999"]' <<<"$respuesta")" ;;
    interno-coexistente-ajeno-numero) respuesta="$(jq '.releases[1].versionCodes=[3014000]' <<<"$respuesta")" ;;
    interno-coexistente-ajeno-enorme) respuesta="$(jq '.releases[1].versionCodes=["999999999999999999999999999999"]' <<<"$respuesta")" ;;
    interno-coexistente-desconocido) respuesta="$(jq '.releases[0].status="future"' <<<"$respuesta")" ;;
    interno-coexistente-fraccion) respuesta="$(jq '.releases[0].userFraction=0.5' <<<"$respuesta")" ;;
    interno-coexistente-tres) respuesta="$(jq '.releases += [{versionCodes:["3013999"],status:"completed"}]' <<<"$respuesta")" ;;
    esac
    ;;
  interno-track-invalido:eu.sytue.rooms) respuesta='{"track":"qa","releases":[{"versionCodes":["3013004"],"status":"draft","name":"NO-MOSTRAR","releaseNotes":[{"language":"es-ES","text":"NO-MOSTRAR"}]}]}' ;;
  interno-multiples:eu.sytue.rooms) respuesta='{"track":"internal","releases":[{"versionCodes":["3013003"],"status":"draft","name":"NO-MOSTRAR"},{"versionCodes":["3013004"],"status":"draft"}]}' ;;
  interno-parcial:eu.sytue.rooms) respuesta='{"track":"internal","releases":[{"versionCodes":["3013004"],"status":"inProgress","userFraction":0.5,"name":"NO-MOSTRAR"}]}' ;;
  interno-completado:*) respuesta='{"track":"internal","releases":[{"versionCodes":["3013004"],"status":"completed"}]}' ;;
  interno-anterior:*) respuesta='{"track":"internal","releases":[{"name":"Anterior","versionCodes":["3013003"],"status":"completed","releaseNotes":[{"language":"es-ES","text":"Notas revisadas"}],"countryTargeting":{"countries":["ES"]},"inAppUpdatePriority":2}]}' ;;
  interno-*:*) respuesta='{"track":"internal","releases":[{"name":"Conservar","versionCodes":["3013004"],"status":"draft","releaseNotes":[{"language":"es-ES","text":"Notas revisadas"}],"countryTargeting":{"countries":["ES"]},"inAppUpdatePriority":2}]}' ;;
  vacio:eu.sytue.rooms) respuesta='{"track":"internal","releases":[]}' ;;
  nuevo:eu.sytue.rooms) respuesta='{"track":"internal","releases":[{"versionCodes":["3012005"],"status":"draft"}]}' ;;
  mezclado:eu.sytue.rooms) respuesta='{"track":"internal","releases":[{"versionCodes":["3012004","3012005"],"status":"draft"}]}' ;;
  objetivo-v5:*) respuesta='{"track":"internal","releases":[{"versionCodes":["3012005"],"status":"draft"}]}' ;;
  pin-interno-mal:eu.sytue.rooms) respuesta='{"track":"internal","releases":[{"versionCodes":["3012024"],"status":"draft"}]}' ;;
  pin-*:*) respuesta='{"track":"internal","releases":[{"versionCodes":["3014000"],"status":"draft"}]}' ;;
  *) respuesta='{"track":"internal","releases":[{"versionCodes":["3012004"],"status":"draft"}]}' ;;
  esac
  fi
  ;;
*/tracks/production/releases)
  respuesta="$(produccion_actual | jq '{releases:[.releases[]? | {track:"production",activeArtifacts:[.versionCodes[] | {versionCode:tonumber}],releaseLifecycleState:(if .status == "draft" then "RELEASE_LIFECYCLE_STATE_DRAFT" else "RELEASE_LIFECYCLE_STATE_PUBLISHED" end)}]}')"
  case "${PLAY_ESCENARIO:-}" in
  pin-ya-completed-in-review) respuesta="$(jq '.releases[0].releaseLifecycleState="RELEASE_LIFECYCLE_STATE_IN_REVIEW"' <<<"$respuesta")" ;;
  pin-ya-completed-approved) respuesta="$(jq '.releases[0].releaseLifecycleState="RELEASE_LIFECYCLE_STATE_APPROVED_NOT_PUBLISHED"' <<<"$respuesta")" ;;
  esac
  if [ "$paquete" = eu.sytue.rooms ]; then
    case "${PLAY_ESCENARIO:-}" in
    pin-ciclo-in-review) respuesta="$(jq '.releases[0].releaseLifecycleState="RELEASE_LIFECYCLE_STATE_IN_REVIEW"' <<<"$respuesta")" ;;
    pin-ciclo-approved) respuesta="$(jq '.releases[0].releaseLifecycleState="RELEASE_LIFECYCLE_STATE_APPROVED_NOT_PUBLISHED"' <<<"$respuesta")" ;;
    pin-ciclo-desconocido) respuesta="$(jq '.releases[0].releaseLifecycleState="RELEASE_LIFECYCLE_STATE_FUTURE"' <<<"$respuesta")" ;;
    pin-ciclo-ausente) respuesta="$(jq 'del(.releases[0].releaseLifecycleState)' <<<"$respuesta")" ;;
    pin-ciclo-vacio) respuesta='{"releases":[]}' ;;
    pin-ciclo-doble) respuesta="$(jq '.releases += [.releases[0]]' <<<"$respuesta")" ;;
    pin-ciclo-track) respuesta="$(jq '.releases[0].track="internal"' <<<"$respuesta")" ;;
    pin-ciclo-version) respuesta="$(jq '.releases[0].activeArtifacts=[{versionCode:3012023}]' <<<"$respuesta")" ;;
    pin-ciclo-mixto) respuesta="$(jq '.releases[0].activeArtifacts += [{versionCode:3014000}]' <<<"$respuesta")" ;;
    pin-ciclo-texto) respuesta="$(jq '.releases[0].activeArtifacts[0].versionCode |= tostring' <<<"$respuesta")" ;;
    pin-ciclo-campo) respuesta="$(jq '.releases[0].futuro="no adivinar"' <<<"$respuesta")" ;;
    pin-ciclo-error) codigo=503; respuesta='{"error":{"code":503,"message":"Consulta de revisión no disponible"}}' ;;
    pin-ya-completed-draft) respuesta="$(jq '.releases[0].releaseLifecycleState="RELEASE_LIFECYCLE_STATE_DRAFT"' <<<"$respuesta")" ;;
    pin-ya-completed-not-approved) respuesta="$(jq '.releases[0].releaseLifecycleState="RELEASE_LIFECYCLE_STATE_NOT_APPROVED"' <<<"$respuesta")" ;;
    pin-ya-completed-not-sent) respuesta="$(jq '.releases[0].releaseLifecycleState="RELEASE_LIFECYCLE_STATE_NOT_SENT_FOR_REVIEW"' <<<"$respuesta")" ;;
    pin-ya-completed-unknown) respuesta="$(jq '.releases[0].releaseLifecycleState="RELEASE_LIFECYCLE_STATE_UNSPECIFIED"' <<<"$respuesta")" ;;
    pin-ya-completed-version) respuesta="$(jq '.releases[0].activeArtifacts=[{versionCode:3012024}]' <<<"$respuesta")" ;;
    vacio-con-resumen) respuesta='{"releases":[{"track":"production","activeArtifacts":[{"versionCode":3012004}],"releaseLifecycleState":"RELEASE_LIFECYCLE_STATE_DRAFT"}]}' ;;
    esac
  fi
  ;;
*/tracks/production)
  if [ "$metodo" = PUT ]; then
    printf '%s' "$cuerpo" >"$raiz/pendientes/$paquete.json"
  else
    respuesta="$(produccion_actual)"
  fi
  ;;
*:commit*)
  if [ "${PLAY_ESCENARIO:-}" = rechazar-commit ]; then
    codigo=400; respuesta='{"error":{"code":400,"message":"No se admite este commit"}}'
  elif [ -f "$raiz/pendientes/$paquete.json" ]; then
    mv "$raiz/pendientes/$paquete.json" "$raiz/tracks/$paquete.json"
  elif [ -f "$raiz/pendientes/$paquete-interno.json" ]; then
    mv "$raiz/pendientes/$paquete-interno.json" "$raiz/internos/$paquete.json"
  fi
  ;;
*) [ "$metodo" != DELETE ] || rm -f "$raiz/pendientes/$paquete.json" "$raiz/pendientes/$paquete-interno.json" ;;
esac
printf '%s\n' "$respuesta" >"$salida"
[ -z "$w" ] || printf '%s' "$codigo"
exit 0
`
