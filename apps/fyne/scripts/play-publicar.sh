#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Sube a Google Play (Developer API v3) la ficha de cada app de apps/fyne/play/ y,
# si se le da un directorio de AAB firmados, el AAB a la pista «internal», o promueve
# los quince AAB ya presentes en internal a producción con una confirmación explícita.
# Ver «Google Play: ficha y subida por la API» en docs/operations.md.
#
#   play-publicar.sh --seco [app...]          valida textos, imágenes y CSV; no llama a nadie
#   play-publicar.sh --sondear [app...]       ¿existe la app y nos deja editarla? (insert + delete)
#   play-publicar.sh --huellas --tag vX.Y.Z [app...]
#                                           solo GET: certificados públicos finales de Play
#   play-publicar.sh --interno --tag vX.Y.Z --estado sondear|completed [app...]
#                                           consulta o completa internal sin subir otro AAB
#   play-publicar.sh [--aab DIR] [app...]     ficha + imágenes + seguridad de los datos [+ AAB]
#   play-publicar.sh --produccion --tag vX.Y.Z --estado draft|completed [--borrador-anterior vA.B.C]
#                                           internal → production de las quince apps
#
# Credenciales: PLAY_CREDENCIALES es la RUTA de la clave JSON de la cuenta de servicio
# (el workflow la escribe en $RUNNER_TEMP y la borra con un trap). La clave privada y el
# token nunca van en argv ni al log: openssl firma leyendo un fichero, y curl lee la
# cabecera Authorization de otro (`-H @fichero`).
#
# Secuencia por app: edits.insert → details.patch → listings.update (cada idioma) →
# images.deleteall + images.upload → [bundles.upload → tracks.update internal] →
# edits.commit → dataSafety (va fuera de la edición: la API no la ata a ninguna).
set -euo pipefail

AQUI="$(cd "$(dirname "$0")" && pwd)"
# PLAY_DIR solo lo cambia el guardián (empaquetar-test.sh), para verlo en rojo.
PLAY="${PLAY_DIR:-$AQUI/../play}"
API=https://androidpublisher.googleapis.com/androidpublisher/v3/applications
SUBIDA=https://androidpublisher.googleapis.com/upload/androidpublisher/v3/applications
# El idioma por defecto de la ficha: las imágenes van solo a él y Play las usa para los demás.
IDIOMA_BASE=es-ES
# Estado de la release en internal. Una app que aún no se ha publicado nunca solo admite
# «draft» (la API rechaza «completed» con «Only releases with status draft may be created
# on draft app»); el dueño la lanza desde la consola.
PLAY_ESTADO="${PLAY_ESTADO:-draft}"

modo=publicar aab="" tag="" estado="" borrador_anterior="" version_anterior=""
while [ $# -gt 0 ]; do
  case "$1" in
  --seco) modo=seco ;;
  --sondear) modo=sondear ;;
  --huellas) modo=huellas ;;
  --interno) modo=interno ;;
  --produccion) modo=produccion ;;
  --aab) aab="$2"; shift ;;
  --tag) tag="${2:?--tag necesita vX.Y.Z}"; shift ;;
  --estado) estado="${2:?--estado necesita un valor explícito}"; shift ;;
  --borrador-anterior)
    [ -z "$borrador_anterior" ] || { echo 'play-publicar: --borrador-anterior no se puede repetir' >&2; exit 2; }
    borrador_anterior="${2:?--borrador-anterior necesita vX.Y.Z}"; shift ;;
  -*) echo "uso: $0 [--seco|--sondear|--huellas --tag vX.Y.Z|--interno --tag vX.Y.Z --estado sondear|completed|--produccion --tag vX.Y.Z --estado draft|completed [--borrador-anterior vA.B.C]] [--aab DIR] [app...]" >&2; exit 2 ;;
  *) break ;;
  esac
  shift
done
apps=("$@")
# Las quince de Play: cada cmd/ salvo la demostración.
[ ${#apps[@]} -gt 0 ] || mapfile -t apps < <(cd "$AQUI/../cmd" && for d in */; do [ "$d" != demo/ ] && echo "${d%/}"; done)
# El paquete es el ID de FyneApp.toml, el mismo que firma release-mobile.yml.
paquete_de() { sed -n 's/^[[:space:]]*ID[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' "$AQUI/../cmd/$1/FyneApp.toml" | head -1; }
[ -z "$borrador_anterior" ] || [ "$modo" = produccion ] || {
  echo 'play-publicar: --borrador-anterior sólo se admite con --produccion' >&2; exit 2;
}

# Producción no admite atajos ni una publicación parcial: solo la llamada manual del
# workflow, con la frase de confirmación exacta, puede promover las quince apps. Los AAB
# se suben antes a internal por release-mobile.yml/play-publicar.yml y aquí se reutilizan.
if [ "$modo" = produccion ]; then
  [ -z "$aab" ] || { echo "play-publicar: --produccion no acepta --aab; promueve el AAB ya verificado en internal" >&2; exit 2; }
  [ "${PLAY_CONFIRMAR_PRODUCCION:-}" = "PUBLICAR_15_APPS" ] || {
    echo "play-publicar: producción requiere PLAY_CONFIRMAR_PRODUCCION=PUBLICAR_15_APPS" >&2
    exit 2
  }
  mapfile -t esperadas < <(cd "$AQUI/../cmd" && for d in */; do [ "$d" != demo/ ] && echo "${d%/}"; done)
  if [ "${#esperadas[@]}" -ne 15 ] || [ "$(printf '%s\n' "${apps[@]}" | sort)" != "$(printf '%s\n' "${esperadas[@]}" | sort)" ]; then
    echo "play-publicar: producción debe incluir exactamente las quince apps" >&2
    exit 2
  fi
  case "$estado" in draft | completed) ;; *) echo "play-publicar: producción requiere --estado draft|completed explícito" >&2; exit 2 ;; esac
  [[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "play-publicar: producción requiere --tag vX.Y.Z" >&2; exit 2; }
  # La misma fórmula que firma el manifiesto Android; no elegimos «el último» de otra sesión.
  version_esperada="$(bash "$AQUI/empaquetar.sh" version-code "$tag")" || exit 2
  if [ -n "$borrador_anterior" ]; then
    [[ "$borrador_anterior" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'play-publicar: --borrador-anterior necesita vX.Y.Z' >&2; exit 2; }
    version_anterior="$(bash "$AQUI/empaquetar.sh" version-code "$borrador_anterior")" || exit 2
    [ "$version_anterior" -lt "$version_esperada" ] || {
      echo 'play-publicar: el borrador anterior debe tener un versionCode menor al tag solicitado' >&2; exit 2;
    }
  fi
elif [ "$modo" = huellas ] || [ "$modo" = interno ]; then
  [ -z "$aab" ] || { echo "play-publicar: --$modo no acepta --aab" >&2; exit 2; }
  if [ "$modo" = huellas ]; then
    [ -z "$estado" ] || { echo 'play-publicar: --huellas no acepta --estado' >&2; exit 2; }
  else
    case "$estado" in sondear | completed) ;; *) echo 'play-publicar: --interno requiere --estado sondear|completed' >&2; exit 2 ;; esac
    [ "$(printf '%s\n' "${apps[@]}" | sort -u | wc -l)" -eq "${#apps[@]}" ] || {
      echo 'play-publicar: --interno no acepta apps repetidas' >&2; exit 2;
    }
  fi
  [[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "play-publicar: --$modo requiere --tag vX.Y.Z" >&2; exit 2; }
  version_esperada="$(bash "$AQUI/empaquetar.sh" version-code "$tag")" || exit 2
  for app in "${apps[@]}"; do
    [[ "$app" =~ ^[a-z]+$ ]] && [ "$app" != demo ] && [ -f "$AQUI/../cmd/$app/FyneApp.toml" ] || {
      echo "play-publicar: --$modo solo admite apps publicables de la suite" >&2; exit 2;
    }
  done
elif [ -n "$tag$estado" ]; then
  echo "play-publicar: --tag y --estado solo se usan con --produccion" >&2
  exit 2
fi

command -v python3 >/dev/null || { echo "::error::falta python3" >&2; exit 1; }
error() { echo "::error::$*" >&2; fallos=$((fallos + 1)); }
fallos=0

# ---------------------------------------------------------------- validación (--seco)

# png_medida <fichero> → «ancho alto tipo»: el IHDR va en los bytes 16-25. Tipo 6 = RGBA
# de 8 bits, que es el «PNG de 32 bits» que Play pide para el icono.
png_medida() {
  local b
  read -r -a b <<<"$(od -An -tu1 -j16 -N10 "$1")"
  echo "$(((b[0] << 24) | (b[1] << 16) | (b[2] << 8) | b[3])) $(((b[4] << 24) | (b[5] << 16) | (b[6] << 8) | b[7])) ${b[8]}-${b[9]}"
}

# largo en caracteres (no en bytes): Play cuenta caracteres.
largo() { python3 -c 'import sys; print(len(open(sys.argv[1], encoding="utf-8").read().rstrip("\n")), end="")' "$1"; }

validar() {
  local app="$1" d="$PLAY/$1" idioma n f m
  [ -n "$(paquete_de "$app")" ] || { error "$app: cmd/$app/FyneApp.toml sin ID"; return; }
  [ -d "$d/$IDIOMA_BASE" ] || error "$app: falta la ficha en $IDIOMA_BASE, el idioma por defecto"
  for idioma in "$d"/*-* "$d"/??; do
    [ -d "$idioma" ] || continue
    for f in title:30 short_description:80 full_description:4000; do
      [ -s "$idioma/${f%%:*}.txt" ] || { error "$app/${idioma##*/}: falta ${f%%:*}.txt"; continue; }
      n="$(largo "$idioma/${f%%:*}.txt")"
      [ "$n" -le "${f#*:}" ] || error "$app/${idioma##*/}/${f%%:*}.txt: $n caracteres (máximo ${f#*:})"
    done
  done
  m="$(png_medida "$d/graficos/icono.png" 2>/dev/null || true)"
  [ "$m" = "512 512 8-6" ] || error "$app: graficos/icono.png es «$m» y tiene que ser 512×512 RGBA de 8 bits"
  m="$(png_medida "$d/graficos/destacado.png" 2>/dev/null || true)"
  [ "${m% *}" = "1024 500" ] || error "$app: graficos/destacado.png es «$m» y tiene que ser 1024×500"
  n=0
  for f in "$d"/graficos/telefono-*.png; do
    [ -f "$f" ] || continue
    n=$((n + 1))
    m="$(png_medida "$f")"
    [ "${m% *}" = "1080 1920" ] || error "$app: ${f##*/} es «$m» y tiene que ser 1080×1920"
  done
  [ "$n" -ge 2 ] && [ "$n" -le 8 ] || error "$app: $n capturas de teléfono (Play pide de 2 a 8)"
  [ -s "$d/seguridad-datos.csv" ] || error "$app: falta seguridad-datos.csv"
  grep -q '^Question ID (machine readable),Response ID (machine readable),Response value,Answer requirement,Human-friendly question label' <<<"$(head -1 "$d/seguridad-datos.csv")" ||
    error "$app: seguridad-datos.csv no tiene la cabecera de la exportación de Play"
}

case "$modo" in sondear | produccion | huellas | interno) ;; *) for app in "${apps[@]}"; do validar "$app"; done ;; esac
[ "$fallos" -eq 0 ] || { echo "play-publicar: $fallos errores de validación" >&2; exit 1; }
if [ "$modo" = seco ]; then
  echo "play-publicar --seco: ${#apps[@]} apps válidas (${apps[*]})"
  exit 0
fi

# ---------------------------------------------------------------- token OAuth (JWT)

: "${PLAY_CREDENCIALES:?PLAY_CREDENCIALES tiene que ser la ruta de la clave JSON de la cuenta de servicio}"
for h in curl jq openssl; do command -v "$h" >/dev/null || { echo "::error::falta $h" >&2; exit 1; }; done
umask 077
TMP="$(mktemp -d "${RUNNER_TEMP:-/tmp}/play-XXXXXX")"
ediciones=()
limpiar() {
  local resultado=$? e
  for e in "${ediciones[@]}"; do
    [ -z "$e" ] || api DELETE "$e" >/dev/null || true
  done
  rm -rf "$TMP"
  exit "$resultado"
}
trap limpiar EXIT
jq -r .private_key "$PLAY_CREDENCIALES" >"$TMP/clave.pem"
cuenta="$(jq -r .client_email "$PLAY_CREDENCIALES")"
b64() { openssl base64 -A | tr '+/' '-_' | tr -d '='; }
ahora="$(date +%s)"
firmar="$(printf '{"alg":"RS256","typ":"JWT"}' | b64).$(jq -cn --arg c "$cuenta" --argjson t "$ahora" \
  '{iss:$c, scope:"https://www.googleapis.com/auth/androidpublisher", aud:"https://oauth2.googleapis.com/token", iat:$t, exp:($t+3600)}' | b64)"
jwt="$firmar.$(printf '%s' "$firmar" | openssl dgst -sha256 -sign "$TMP/clave.pem" -binary | b64)"
rm -f "$TMP/clave.pem"
# El JWT va por stdin (--data @-), no en argv.
respuesta="$(printf 'grant_type=urn%%3Aietf%%3Aparams%%3Aoauth%%3Agrant-type%%3Ajwt-bearer&assertion=%s' "$jwt" |
  curl -sS --max-time 60 --data @- https://oauth2.googleapis.com/token)"
unset jwt
token="$(jq -r '.access_token // empty' <<<"$respuesta")"
[ -n "$token" ] || { echo "::error::Google no dio token: $(jq -c '{error, error_description}' <<<"$respuesta")" >&2; exit 1; }
printf 'Authorization: Bearer %s\n' "$token" >"$TMP/cabecera"
unset token respuesta

# api MÉTODO URL [curl...] → imprime el cuerpo; devuelve 1 (con el error de Google en
# stderr) si el código no es 2xx.
api() {
  local metodo="$1" url="$2" codigo
  shift 2
  codigo="$(curl -sS --max-time 600 -o "$TMP/r" -w '%{http_code}' -X "$metodo" -H @"$TMP/cabecera" "$@" "$url")" || codigo=000
  if [ "${codigo:0:1}" != 2 ]; then
    echo "HTTP $codigo $metodo ${url#https://androidpublisher.googleapis.com}: $(jq -c '.error | {code, status, message}' "$TMP/r" 2>/dev/null || head -c 300 "$TMP/r")" >&2
    return 1
  fi
  cat "$TMP/r"
}
json() { api "$@" -H 'Content-Type: application/json'; }

abrir_edicion() { json POST "$API/$1/edits" --data '{}' | jq -er '.id | select(type == "string" and length > 0)'; }

# Completar una prueba interna reutiliza el bundle ya subido. La consulta sólo
# abre ediciones temporales que el trap borra: no cambia pistas, fichas ni testers.
preparar_interno() {
  local app="$1" paquete edit e grupos
  paquete="$(paquete_de "$app")"
  edit="$(abrir_edicion "$paquete")" || return 1
  e="$API/$paquete/edits/$edit"
  ediciones+=("$e")
  api GET "$e/bundles" >"$TMP/$app-bundles.json" || return 1
  jq -e --arg v "$version_esperada" 'any(.bundles[]?; (.versionCode | tostring) == $v)' \
    "$TMP/$app-bundles.json" >/dev/null || {
    echo "✗ $app: Play no contiene el bundle $version_esperada ($tag); no se modifica internal" >&2
    return 1
  }
  api GET "$e/tracks/internal" >"$TMP/$app-internal.json" || return 1
  jq -e '.track == "internal" and ((.releases // []) | type == "array" and length <= 1)
    and all(.releases[]?; (.status == "draft" or .status == "completed"))' \
    "$TMP/$app-internal.json" >/dev/null || {
    echo "✗ $app: internal contiene varias releases o un rollout parcial; requiere revisión" >&2
    return 1
  }
  # La API sólo cuenta Google Groups; las listas de correos de Console no son
  # visibles. No se imprimen nombres ni se invita/cambia a ningún probador.
  grupos="$(api GET "$e/testers/internal" | jq -er '(.googleGroups // []) | length')" || return 1
  jq -c --arg a "$app" --arg v "$version_esperada" --argjson g "$grupos" \
    '{app:$a, expectedVersionCode:$v, internal:[.releases[]? | {versionCodes,status}], googleGroups:$g,
      emailListsVisibleViaAPI:false}' "$TMP/$app-internal.json"
}

interno() {
  local app i e
  local malas=()
  for app in "${apps[@]}"; do preparar_interno "$app" || malas+=("$app"); done
  [ ${#malas[@]} -eq 0 ] || {
    echo "::error::preflight internal sin terminar: ${malas[*]}; ninguna pista modificada" >&2
    return 1
  }
  [ "$estado" != sondear ] || return 0
  for i in "${!apps[@]}"; do
    app="${apps[$i]}" e="${ediciones[$i]}"
    if jq -e --arg v "$version_esperada" '.releases | length == 1 and
      .[0].status == "completed" and (.[0].versionCodes | map(tostring) | unique) == [$v]' \
      "$TMP/$app-internal.json" >/dev/null; then
      echo "✓ $app: internal $tag ($version_esperada) ya está completed; sin cambios"
      continue
    fi
    # Conserva notas, prioridad y targeting de la pista; sólo sustituye la versión
    # y su estado. No se toca production ni ninguna configuración de probadores.
    if ! jq --arg v "$version_esperada" --arg t "$tag" '{track:"internal", releases:[
      ((.releases[0] // {}) | .name=$t | .versionCodes=[$v] | .status="completed" | del(.userFraction))]}' \
      "$TMP/$app-internal.json" | json PUT "$e/tracks/internal" --data @- >/dev/null; then
      malas+=("$app")
      continue
    fi
    if ! json POST "$e:commit?changesInReviewBehavior=ERROR_IF_IN_REVIEW" --data '{}' >/dev/null; then
      malas+=("$app")
      continue
    fi
    ediciones[$i]=""
    echo "✓ $app: internal $tag ($version_esperada) completed; probadores existentes conservados"
  done
  [ ${#malas[@]} -eq 0 ] || {
    echo "::error::Play no completó internal: ${malas[*]}; revisar los motivos de Google" >&2
    return 1
  }
}

if [ "$modo" = interno ]; then
  interno
  exit $?
fi

# La firma del AAB es la clave de subida. Para el broker se necesitan las firmas
# que Google entrega a los dispositivos, incluidas las claves que haya rotado.
# Este modo no abre ediciones, descarga APK ni cambia pistas o fichas.
if [ "$modo" = huellas ]; then
  malas=()
  for app in "${apps[@]}"; do
    paquete="$(paquete_de "$app")"
    [[ "$paquete" =~ ^eu\.sytue\.[a-z]+$ ]] || { echo 'play-publicar: paquete no admitido' >&2; exit 2; }
    if ! api GET "$API/$paquete/generatedApks/$version_esperada" >"$TMP/huellas.json"; then
      malas+=("$app")
      continue
    fi
    python3 - "$TMP/huellas.json" "$paquete" "$version_esperada" <<'PY' || malas+=("$app")
import base64, binascii, json, re, sys
def huella(valor):
    if not isinstance(valor, str):
        raise ValueError('falta la huella')
    if re.fullmatch(r'[a-fA-F0-9]{64}|(?:[a-fA-F0-9]{2}:){31}[a-fA-F0-9]{2}', valor):
        datos = bytes.fromhex(valor.replace(':', ''))
    elif re.fullmatch(r'[A-Za-z0-9+/_-]{43}=?', valor):
        datos = base64.b64decode(valor.rstrip('=').translate(str.maketrans('-_', '+/')) + '=', validate=True)
    else:
        raise ValueError('formato de huella no admitido')
    if len(datos) != 32:
        raise ValueError('la huella no tiene 32 bytes')
    return datos.hex()
try:
    with open(sys.argv[1]) as archivo:
        grupos = json.load(archivo)['generatedApks']
    if not isinstance(grupos, list) or not grupos:
        raise ValueError('Play no ha generado APK de esta versión')
    certificados = sorted({huella(g['certificateSha256Hash']) for g in grupos})
except (OSError, ValueError, KeyError, TypeError, binascii.Error) as error:
    sys.exit('play-publicar: certificados finales inválidos para ' + sys.argv[2] + ': ' + str(error))
print(json.dumps({'package_name':sys.argv[2], 'version_code':int(sys.argv[3]), 'certificate_sha256':certificados}, separators=(',', ':')))
PY
  done
  [ ${#malas[@]} -eq 0 ] || { echo "::error::catálogo incompleto; faltan certificados finales de: ${malas[*]}" >&2; exit 1; }
  exit 0
fi

# Producción tiene dos fases: leer las quince antes del primer PUT/commit. Las ediciones
# abiertas se descartan al fallar; un AAB ausente/distinto en la última app deja cero releases.
preparar_produccion() {
  local app="$1" paquete edit e
  paquete="$(paquete_de "$app")"
  edit="$(abrir_edicion "$paquete")" || { echo "✗ $app ($paquete): no se pudo abrir una edición" >&2; return 1; }
  e="$API/$paquete/edits/$edit"
  ediciones+=("$e")
  api GET "$e/tracks/internal" >"$TMP/$app-internal.json" || return 1
  jq -e --arg v "$version_esperada" '
    .track == "internal" and ([.releases[]?.versionCodes[]? | tostring] | unique) == [$v]
  ' "$TMP/$app-internal.json" >/dev/null || {
    echo "✗ $app ($paquete): internal debe contener exclusivamente versionCode $version_esperada ($tag)" >&2
    return 1
  }
  api GET "$e/tracks/production" >"$TMP/$app-production.json" || return 1
  jq -e --arg v "$version_esperada" --arg anterior "$version_anterior" --arg estado "$estado" '
    .track == "production" and (keys - ["track","releases"] | length == 0) and
    ((has("releases") | not) or (.releases | type == "array")) and
    ((.releases // []) | type == "array" and (length == 0 or (length == 1 and
      (.[0] | type == "object" and
        (keys - ["name","versionCodes","status","releaseNotes","inAppUpdatePriority"] | length == 0) and
        ((.status == "draft" and
          (.versionCodes == [$v] or ($anterior != "" and .versionCodes == [$anterior]))) or
          (.status == "completed" and $estado == "completed" and .versionCodes == [$v])) and
        ((has("name") | not) or (.name | type == "string")) and
        ((has("releaseNotes") | not) or (.releaseNotes | type == "array")) and
        all(.releaseNotes[]?; type == "object" and
          (keys - ["language","text"] | length == 0) and
          (.language | type == "string") and (.text | type == "string")) and
        ((has("inAppUpdatePriority") | not) or
          (.inAppUpdatePriority | type == "number" and floor == . and . >= 0 and . <= 5))
      ))))
  ' "$TMP/$app-production.json" >/dev/null || {
    jq -c --arg a "$app" '{app:$a, production:[(.releases // [])[] | {versionCodes,status}]}' \
      "$TMP/$app-production.json" >&2
    echo "✗ $app ($paquete): production no coincide con el borrador autorizado ni con el tag ya enviado; no se sobrescribe" >&2
    return 1
  }
  # El estado de tracks no distingue una revisión de un borrador. La consulta
  # pública de ciclo de vida debe corroborar el mismo artefacto antes de escribir.
  api GET "$API/$paquete/tracks/production/releases" >"$TMP/$app-lifecycle.json" || return 1
  jq -e --slurpfile track "$TMP/$app-production.json" '
    (.releases // []) as $summaries | ($track[0].releases // []) as $planned |
    type == "object" and (keys - ["releases"] | length == 0) and
    ((has("releases") | not) or (.releases | type == "array")) and
    ($summaries | type == "array") and
    (if ($planned | length) == 0 then ($summaries | length) == 0 else
      ($summaries | length) == 1 and ($summaries[0] |
        (keys - ["releaseName","track","activeArtifacts","releaseLifecycleState"] | length == 0) and
        ((has("releaseName") | not) or (.releaseName | type == "string")) and
        .track == "production" and
        (if $planned[0].status == "draft" then
          .releaseLifecycleState == "RELEASE_LIFECYCLE_STATE_DRAFT"
        else
          (.releaseLifecycleState == "RELEASE_LIFECYCLE_STATE_IN_REVIEW" or
            .releaseLifecycleState == "RELEASE_LIFECYCLE_STATE_APPROVED_NOT_PUBLISHED" or
            .releaseLifecycleState == "RELEASE_LIFECYCLE_STATE_PUBLISHED")
        end) and
        (.activeArtifacts | type == "array" and length == 1) and
        all(.activeArtifacts[]; (keys - ["versionCode"] | length == 0) and
          (.versionCode | type == "number" and floor == .)) and
        ([.activeArtifacts[].versionCode | tostring] == $planned[0].versionCodes))
    end)
  ' "$TMP/$app-lifecycle.json" >/dev/null || {
    jq -c --arg a "$app" '{app:$a, releases:[(.releases // [])[] | {track,activeArtifacts:[.activeArtifacts[]? | {versionCode}],releaseLifecycleState}]}' \
      "$TMP/$app-lifecycle.json" >&2 || true
    echo "✗ $app ($paquete): ciclo de publicación ausente, distinto o no editable; no se modifica production" >&2
    return 1
  }
  echo "· $app ($paquete): comprobados internal $version_esperada y production"
}

produccion() {
  local i app e consulta='changesInReviewBehavior=ERROR_IF_IN_REVIEW'
  malas=()
  for app in "${apps[@]}"; do preparar_produccion "$app" || malas+=("$app"); done
  [ ${#malas[@]} -eq 0 ] || { echo "::error::preflight sin terminar: ${malas[*]}; ninguna release modificada" >&2; return 1; }
  [ "$estado" != draft ] || consulta="$consulta&changesNotSentForReview=true"
  for i in "${!apps[@]}"; do
    app="${apps[$i]}" e="${ediciones[$i]}"
    if jq -e --arg v "$version_esperada" '(.releases // []) | length == 1 and
      .[0].status == "completed" and .[0].versionCodes == [$v]' "$TMP/$app-production.json" >/dev/null; then
      echo "✓ $app: production $tag ($version_esperada) ya enviada; ciclo $(jq -r '.releases[0].releaseLifecycleState' "$TMP/$app-lifecycle.json"); sin cambios"
      continue
    fi
    if [ "$estado" = draft ] && jq -e --arg v "$version_esperada" '(.releases // []) | length == 1 and .[0].versionCodes == [$v]' "$TMP/$app-production.json" >/dev/null; then
      echo "✓ $app: borrador production $tag ($version_esperada) ya preparado; sin envío a revisión"
      continue
    fi
    # Sólo cambia la versión del borrador anterior expresamente autorizado. Al
    # completar el mismo tag conserva su nombre; nunca toca los países de la pista.
    jq --arg v "$version_esperada" --arg t "$tag" --arg s "$estado" '
      .releases = (if (.releases // [] | length) == 0 then [{name:$t, versionCodes:[$v], status:$s}]
        else [.releases[0] | if .versionCodes == [$v] then .status = $s
          else .name = $t | .versionCodes = [$v] | .status = $s end] end)
    ' "$TMP/$app-production.json" | json PUT "$e/tracks/production" --data @- >/dev/null || return 1
    if ! json POST "$e:commit?$consulta" --data '{}' >/dev/null; then
      if [ "$estado" = draft ] && grep -q 'changesNotSentForReview must not be set' "$TMP/r"; then
        echo "· $app: la cuenta no usa publicación gestionada; confirmando sin changesNotSentForReview" >&2
        consulta='changesInReviewBehavior=ERROR_IF_IN_REVIEW'
        json POST "$e:commit?$consulta" --data '{}' >/dev/null || return 1
      else
        return 1
      fi
    fi
    ediciones[$i]=""
    if [ "$estado" = draft ]; then
      echo "✓ $app: borrador production $tag ($version_esperada) preparado; sin envío a revisión ni publicación pública"
    else
      echo "✓ $app: production $tag ($version_esperada) enviada a revisión de Play; disponibilidad pública pendiente de Google"
    fi
  done
}

if [ "$modo" = produccion ]; then
  produccion
  exit $?
fi

# ---------------------------------------------------------------- por app

publicar() {
  local app="$1" d="$PLAY/$1" paquete edit idioma img tipo codigo
  paquete="$(paquete_de "$app")"
  edit="$(abrir_edicion "$paquete")" || { echo "✗ $app ($paquete): no se pudo abrir una edición" >&2; return 1; }
  if [ "$modo" = sondear ]; then
    api DELETE "$API/$paquete/edits/$edit" >/dev/null || true
    echo "✓ $app ($paquete): existe y la cuenta de servicio puede editarla"
    return 0
  fi
  local e="$API/$paquete/edits/$edit"
  # edits.details.patch no admite updateMask (400 «Unknown name updateMask»): el PATCH ya solo
  # cambia los campos que van en el cuerpo.
  # Play no deja hacer commit de una edición sin correo de contacto público (403 «This app
  # does not have a contact email address set»). Decidido por el dueño el 26/09.
  jq -n --arg w https://sytue.eu --arg c "${PLAY_CORREO_CONTACTO:-admin@sytue.eu}" --arg l "$IDIOMA_BASE" \
    '{contactWebsite:$w, contactEmail:$c, defaultLanguage:$l}' |
    json PATCH "$e/details" --data @- >/dev/null || return 1
  for idioma in "$d"/*/; do
    idioma="${idioma%/}"
    idioma="${idioma##*/}"
    [ -f "$d/$idioma/title.txt" ] || continue
    jq -n --arg l "$idioma" --rawfile t "$d/$idioma/title.txt" --rawfile s "$d/$idioma/short_description.txt" \
      --rawfile f "$d/$idioma/full_description.txt" \
      '{language:$l, title:($t|rtrimstr("\n")), shortDescription:($s|rtrimstr("\n")), fullDescription:($f|rtrimstr("\n"))}' |
      json PUT "$e/listings/$idioma" --data @- >/dev/null || return 1
  done
  for tipo in icon:icono featureGraphic:destacado phoneScreenshots:telefono-; do
    api DELETE "$e/listings/$IDIOMA_BASE/${tipo%%:*}" >/dev/null || return 1
    for img in "$d"/graficos/"${tipo#*:}"*.png; do
      api POST "$SUBIDA/$paquete/edits/$edit/listings/$IDIOMA_BASE/${tipo%%:*}?uploadType=media" \
        -H 'Content-Type: image/png' --data-binary @"$img" >/dev/null || return 1
    done
  done
  local aab_ya=""
  if [ -n "$aab" ]; then
    if codigo="$(api POST "$SUBIDA/$paquete/edits/$edit/bundles?uploadType=media" \
      -H 'Content-Type: application/octet-stream' --data-binary @"$aab/sytue-$app-android.aab" | jq -r .versionCode)"; then
      jq -n --arg c "$codigo" --arg s "$PLAY_ESTADO" '{track:"internal", releases:[{versionCodes:[$c], status:$s}]}' |
        json PUT "$e/tracks/internal" --data @- >/dev/null || return 1
    elif grep -q 'has already been used' "$TMP/r"; then
      # Ese versionCode ya lo tiene Play: el AAB de este tag se subió en una pasada anterior.
      # Volver a lanzar el flujo tiene que poder terminar la ficha y la seguridad de los
      # datos, no quedarse aquí para siempre (26/09: doce apps se pararon así).
      aab_ya=1
      echo "· $app: el AAB de este tag ya estaba en Play; no se vuelve a subir" >&2
    else
      return 1
    fi
  fi
  json POST "$e:commit" --data '{}' >/dev/null || return 1
  jq -n --rawfile c "$d/seguridad-datos.csv" '{safetyLabels:$c}' |
    json POST "$API/$paquete/dataSafety" --data @- >/dev/null || { echo "✗ $app: la ficha quedó subida, la seguridad de los datos no" >&2; return 1; }
  if [ -n "$aab_ya" ]; then
    echo "✓ $app ($paquete): ficha, imágenes y seguridad de los datos (el AAB ya estaba)"
  else
    echo "✓ $app ($paquete): ficha, imágenes y seguridad de los datos${aab:+, y el AAB $codigo en internal ($PLAY_ESTADO)}"
  fi
}

malas=()
for app in "${apps[@]}"; do publicar "$app" || malas+=("$app"); done
[ ${#malas[@]} -eq 0 ] || { echo "::error::sin terminar: ${malas[*]}" >&2; exit 1; }
