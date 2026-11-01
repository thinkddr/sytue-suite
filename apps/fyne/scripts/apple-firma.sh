#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# LA FIRMA DE APPLE, fuera del yaml para que tenga test (deploy/production/tests/
# apple-firma-test.sh). Corre en el Mac propio (runner sytue-macos, job macos de
# release-desktop.yml); ver «Apple: firma, notarización
# y TestFlight» en docs/operations.md.
#
#   apple-firma.sh directorio-xcode        # SDK completo para DEVELOPER_DIR del job
#   apple-firma.sh credenciales            # ¿están la clave de API y la del certificado?
#   apple-firma.sh preparar macos          # certificado al llavero; imprime la identidad
#   apple-firma.sh sondear-macos            # firma mínima real; no compila ni publica
#   apple-firma.sh sondear-notaria <UUID>  # consulta el envío existente; no lo reenvía
#   apple-firma.sh notarizar <dmg>...      # firma el .dmg, lo notariza, lo grapa y lo verifica
#   apple-firma.sh verificar <dmg>...      # rojo si alguno no está notarizado y grapado
#   apple-firma.sh preparar ios <id>...    # Apple Distribution y un perfil de App Store por App ID
#   apple-firma.sh perfil <id>             # nombre del perfil de App Store instalado de ese App ID
#   apple-firma.sh subir <ipa|pkg>...      # a App Store Connect: TestFlight, pruebas INTERNAS
#   apple-firma.sh preparar macappstore <id>...  # distribución, instalador y un perfil de Mac App Store por App ID
#   apple-firma.sh perfil-mac <id>         # ruta del perfil de Mac App Store instalado de ese App ID
#   apple-firma.sh perfil-ruta <id> <ios|macappstore|developerid> # ruta para app o extensión
#
# DE DÓNDE SALE CADA SECRETO, y ninguno persistente pasa por argv ni por el log:
#   - APP_STORE_CONNECT_ISSUER_ID, _KEY_IDENTIFIER y _PRIVATE_KEY son secretos de
#     GitHub Actions que el job pasa en su `env:`.
#   - CERTIFICATE_PRIVATE_KEY es la clave privada RSA de NUESTROS certificados, otro
#     secreto de GitHub Actions. Es SIEMPRE LA MISMA: con una clave nueva
#     en cada build Apple emitiría un certificado nuevo cada vez, y los limita (cinco
#     Developer ID, tres de distribución). Aquí sólo se reutilizan certificados y
#     perfiles ya emitidos con esa clave. El Apple Distribution sirve también para
#     las apps de la Mac App Store; el de instalador debe existir por separado.
#     La ausencia de cualquier recurso es un error: no se crea ni se revoca.
#
# LO QUE AQUÍ NO SE HACE NUNCA: registrar un App ID (los quince los registró el dueño;
# uno que falte es rojo, no un alta), pedir la revisión beta de TestFlight ni enviar
# nada a la revisión del App Store. Eso lo decide una persona.
#
# Bash 3.2 (el de macOS): sin mapfile, sin `case` dentro de `$(…)`.
set -euo pipefail

mal() {
  echo "::error::$*" >&2
  exit 1
}

# Devuelve el SDK completo para DEVELOPER_DIR del proceso, sin cambiar el
# xcode-select de la persona ni acceder a certificados o credenciales.
directorio_xcode() {
  local directorio="${DEVELOPER_DIR:-}" app
  if [ -z "$directorio" ]; then
    app="$(ls -d /Applications/Xcode*.app 2>/dev/null | sort | tail -1)" || app=""
    [ -n "$app" ] || mal 'no hay Xcode.app en /Applications: hace falta el SDK completo de Apple'
    directorio="$app/Contents/Developer"
  fi
  [ -x "$directorio/usr/bin/xcodebuild" ] || mal 'DEVELOPER_DIR no contiene Xcode completo (las Command Line Tools no bastan)'
  printf '%s\n' "$directorio"
}

credenciales_api() {
  local v faltan=0
  for v in APP_STORE_CONNECT_ISSUER_ID APP_STORE_CONNECT_KEY_IDENTIFIER APP_STORE_CONNECT_PRIVATE_KEY; do
    [ -n "${!v:-}" ] || {
      echo "::error::falta $v: es un secreto de GitHub Actions (APP_STORE_CONNECT_*, CERTIFICATE_PRIVATE_KEY) que el job pasa en su env: (docs/operations.md, «Apple: firma, notarización y TestFlight»)." >&2
      faltan=1
    }
  done
  return "$faltan"
}

credenciales() {
  local faltan=0
  credenciales_api || faltan=1
  [ -n "${CERTIFICATE_PRIVATE_KEY:-}" ] || {
    echo "::error::falta CERTIFICATE_PRIVATE_KEY: la clave privada de los certificados, secreto de GitHub Actions (APP_STORE_CONNECT_*, CERTIFICATE_PRIVATE_KEY; docs/operations.md, «Apple: firma, notarización y TestFlight»)." >&2
    faltan=1
  }
  return "$faltan"
}

# La identidad del llavero cuyo nombre empieza por <patrón>, o rojo con qué hacer.
# Con un tercer argumento se miran TODAS las identidades y no solo las de firmar código:
# la de instalador no es de esa política y `-p codesigning` no la enseña.
identidad() { # <patrón> <qué falta si no está> [todas]
  local nombre llavero candidato certificado evaluar=no politica_nativa=codeSign politica=(-p codesigning)
  llavero="$(llavero_efimero)" || exit 1
  [ -z "${3:-}" ] || politica=()
  nombre="$(security find-identity -v ${politica[@]+"${politica[@]}"} "$llavero" | sed -n "s/.*\"\($1: [^\"]*\)\".*/\1/p" | sed -n 1p)" || nombre=""
  # En el Mac actual, el clasificador legado de find-identity -v omite
  # identidades cuya evaluación nativa sí es válida. Aceptar
  # sólo un candidato inequívoco del llavero propio y verificar su cadena sin AIA.
  case "$1:${3:-}" in
    'Developer ID Application:' | 'Apple Distribution:') evaluar=si ;;
    '3rd Party Mac Developer Installer:todas' | 'Mac Installer Distribution:todas') evaluar=si; politica_nativa=basic ;;
  esac
  if [ -z "$nombre" ] && [ "$evaluar" = si ]; then
    candidato="$(security find-identity ${politica[@]+"${politica[@]}"} "$llavero" | sed -n "s/.*\"\($1: [^\"]*\)\".*/\1/p")" || candidato=""
    if [ -n "$candidato" ] && [ "$(printf '%s\n' "$candidato" | wc -l | tr -d ' ')" = 1 ]; then
      certificado="$(mktemp "$CERTS/identity.XXXXXXXX")" || exit 1
      if security find-certificate -c "$candidato" -p "$llavero" >"$certificado" &&
        security verify-cert -L -p "$politica_nativa" -c "$certificado" -k "$llavero" >/dev/null 2>&1; then
        nombre="$candidato"
      fi
      rm -f -- "$certificado"
    fi
  fi
  if [ -z "$nombre" ]; then
    [ "$1" != 'Developer ID Application' ] || diagnostico_developer_id >&2 || true
    mal "no hay ninguna identidad válida «$1» en el llavero. $2"
  fi
  printf '%s\n' "$nombre"
}

# El DER público se espeja en la plomería que el workflow superpone al tag. No se
# descarga al compilar ni se cambian las decisiones de confianza del sistema.
# Origen: https://www.apple.com/appleca/AppleIncRootCertificate.cer
#         https://www.apple.com/certificateauthority/DeveloperIDCA.cer
#         https://www.apple.com/certificateauthority/DeveloperIDG2CA.cer
llavero_efimero() {
  python3 "$(dirname "${BASH_SOURCE[0]}")/apple_llavero.py" ruta ||
    mal 'no se pudo validar el llavero exclusivo de este job'
}

llavero_operacion() {
  python3 "$(dirname "${BASH_SOURCE[0]}")/apple_llavero.py" "$@"
}

# intermedios <fichero.cer>...: los certificados PÚBLICOS de Apple que cierran la cadena,
# comprobados contra su huella y añadidos solo al llavero efímero. Developer ID pide los
# suyos y Apple Distribution el WWDR G3: un Mac sin Xcode reciente solo trae el WWDR
# antiguo, caducado en 2023, y la identidad no sale válida (v3.13.2, Mac propio).
intermedios() {
  local dir f archivos=()
  dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)/.github/actions/apple"
  for f in "$@"; do archivos+=("$dir/$f"); done
  llavero_operacion publicos "${archivos[@]}" ||
    mal 'no se pudo añadir el intermedio público fijado al llavero exclusivo'
}

intermedio_developer_id() { intermedios AppleRootCA.cer DeveloperIDCA.cer DeveloperIDG2CA.cer; }

# Los .p12 que baja la CLI llevan la CLAVE: van a un temporal propio y se borran al
# salir. Su sitio por defecto es la carpeta de Xcode de la persona, que en un Mac propio
# no se vacía (v3.13.2 dejó allí quince).
certificados_temporales() {
  umask 077
  [ -n "${SYTUE_APPLE_JOB_DIR:-}" ] || mal 'falta el contexto privado de firma del job'
  llavero_efimero >/dev/null || exit 1
  export SYTUE_PERFILES_DIR="$SYTUE_APPLE_JOB_DIR/perfiles-apple"
  CERTS="$(mktemp -d "$SYTUE_APPLE_JOB_DIR/certificates.XXXXXXXX")" || mal 'no se pudo crear el directorio temporal de certificados'
  trap 'rm -rf "$CERTS"' EXIT
}

# Solo certificados públicos y resultados de validación, nunca claves. Sin -v se
# ven también las identidades no válidas; -L evita que una descarga AIA oculte una
# cadena incompleta. El fallo del diagnóstico no convierte la identidad en válida.
diagnostico_developer_id() (
  local llavero certificado
  llavero="$(llavero_efimero)" || return 0
  security find-identity -p codesigning "$llavero" || true
  certificado="$(mktemp "$SYTUE_APPLE_JOB_DIR/diagnostic.XXXXXXXX")" || return 0
  trap 'rm -f "$certificado"' EXIT
  if security find-certificate -c 'Developer ID Application' -p "$llavero" >"$certificado"; then
    openssl x509 -in "$certificado" -noout -subject -issuer -dates -serial -fingerprint -sha256 || true
    security verify-cert -L -v -p codeSign -c "$certificado" -k "$llavero" || true
  fi
)

# El certificado «Mac Installer Distribution» sale en el llavero como «3rd Party Mac
# Developer Installer: …» (es lo que busca la documentación de codemagic-cli-tools). Se acepta
# también con el nombre del tipo, por si Apple lo emite así: no hay un Mac donde mirarlo.
instalador() { # <qué falta si no está>
  (identidad '3rd Party Mac Developer Installer' '' todas) 2>/dev/null ||
    identidad 'Mac Installer Distribution' "$1" todas
}

# preparar macos: el Developer ID Application que casa con CERTIFICATE_PRIVATE_KEY, al
# llavero. Solo `list`: si no hay ninguno emitido con esa clave, es rojo y no se crea.
preparar() {
  case "${1:-}" in
  macos)
    credenciales || exit 1
    certificados_temporales
    intermedio_developer_id
    llavero_operacion cli certificates list --type DEVELOPER_ID_APPLICATION DEVELOPER_ID_APPLICATION_G2 \
      --certificates-dir "$CERTS" --certificate-key @env:CERTIFICATE_PRIVATE_KEY --save >&2
    llavero_operacion importar "$CERTS" codesign >&2
    identidad 'Developer ID Application' 'Revisa la cadena de confianza, las fechas y la asociación con la clave en el diagnóstico anterior. Aquí no se emiten ni revocan certificados.'
    ;;
  ios | macappstore)
    local destino="$1" plataforma=IOS tipo=IOS_APP_STORE
    local herramientas=(codesign productsign)
    [ "$destino" = ios ] || { plataforma=MAC_OS && tipo=MAC_APP_STORE; }
    # productbuild firma el .pkg. El importador solo autoriza codesign/productsign
    # por defecto: omitirlo deja el job esperando un diálogo de llavero (v3.13.3).
    [ "$destino" != macappstore ] || herramientas+=(productbuild)
    shift
    [ "$#" -gt 0 ] || mal "preparar $destino: falta al menos un App ID"
    credenciales || exit 1
    certificados_temporales
    intermedios AppleRootCA.cer AppleWWDRCAG3.cer
    local id registrados
    for id in "$@"; do
      # Se exige el App ID exacto existente antes de solicitar su material de firma.
      registrados="$(app-store-connect bundle-ids list --bundle-id-identifier "$id" --strict-match-identifier --json)" ||
        mal "no se pudo preguntar a Apple por el App ID $id"
      grep -q "\"$id\"" <<<"$registrados" ||
        mal "el App ID $id no está registrado en la cuenta de Apple (Certificates, Identifiers & Profiles → Identifiers): aquí no se da de alta."
      # El perfil de tienda de ESTE App ID y el Apple Distribution de nuestra clave:
      # sólo se reutilizan: sin --create ni --delete-stale-profiles, falta = error.
      llavero_operacion cli fetch-signing-files "$id" --platform "$plataforma" --type "$tipo" \
        --strict-match-identifier --certificates-dir "$CERTS" --certificate-key @env:CERTIFICATE_PRIVATE_KEY >&2 ||
        mal "no se pudo traer el perfil de tienda ($tipo) de $id"
      # Y que el de Mac está donde `empaquetar.sh` lo va a buscar: aquí, en el primer
      # minuto, y no después de compilar.
      [ "$destino" != macappstore ] || perfil "$id" mac >/dev/null
    done
    if [ "$destino" = macappstore ]; then
      # El instalador debe existir, ser válido y corresponder a la clave fija.
      llavero_operacion cli certificates list --type MAC_INSTALLER_DISTRIBUTION \
        --certificates-dir "$CERTS" --certificate-key @env:CERTIFICATE_PRIVATE_KEY --save >&2 ||
        mal 'no se pudo consultar el Mac Installer Distribution existente'
      llavero_operacion importar "$CERTS" "${herramientas[@]}" >&2
      (instalador '') >/dev/null 2>&1 ||
        mal 'falta un Mac Installer Distribution existente y válido para la clave fija; aquí no se crea ni revoca'
    fi
    llavero_operacion importar "$CERTS" "${herramientas[@]}" >&2
    identidad 'Apple Distribution' 'fetch-signing-files no dejó ningún certificado Apple Distribution: mira arriba qué contestó Apple.'
    # En la Mac App Store, DOS líneas: la identidad de la app y, debajo, la del instalador.
    [ "$destino" != macappstore ] ||
      instalador 'no hay certificado Mac Installer Distribution en el llavero: mira arriba qué contestó Apple.'
    ;;
  *) mal "uso: $0 preparar macos | preparar ios <App ID>... | preparar macappstore <App ID>..." ;;
  esac
}

# perfil <App ID>: el NOMBRE del perfil de App Store instalado para ese App ID, que es
# lo que pide `fyne package --profile`. Uno por app: el más reciente sin caducar, de
# los que no llevan dispositivos (ad hoc, desarrollo) ni son de empresa.
#
# perfil-mac <App ID>: lo mismo para la Mac App Store, pero da la RUTA del fichero, que
# es lo que hay que copiar dentro del .app (`Contents/embedded.provisionprofile`). Los
# perfiles de Mac dicen `Platform: OSX` y su App ID va en `com.apple.application-identifier`;
# se miran con cualquiera de las dos extensiones, por si la CLI los guarda con la de iOS.
# Los de Developer ID (`ProvisionsAllDevices`) y los de desarrollo quedan fuera igual.
perfil() { # <App ID> [mac|developerid] [ruta]
  local id="${1:?uso: $0 perfil <App ID>}" mac="${2:-}" formato="${3:-}" dir f elegido=""
  local dirs=() plataforma=ios
  [ -z "$mac" ] || plataforma=macappstore
  [ "$mac" != developerid ] || plataforma=developerid
  if [ -n "${SYTUE_PERFILES_DIR:-}" ]; then
    dirs=("$SYTUE_PERFILES_DIR")
  else
    dirs=(
      "$HOME/Library/Developer/Xcode/UserData/Provisioning Profiles"
      "$HOME/Library/MobileDevice/Provisioning Profiles"
    )
  fi
  for dir in "${dirs[@]}"; do
    [ -d "$dir" ] || continue
    elegido="$(for f in "$dir"/*.mobileprovision "$dir"/*.provisionprofile; do
      # Los de iOS, solo con su extensión (como siempre); los de Mac, con las dos.
      [ -n "$mac" ] || [ "${f##*.}" = mobileprovision ] || continue
      [ -f "$f" ] || continue
      python3 "$(dirname "${BASH_SOURCE[0]}")/apple_perfil.py" decodificar "$f" "$plataforma" 2>/dev/null | python3 -c '
import datetime, plistlib, sys
try:
    p = plistlib.loads(sys.stdin.buffer.read())
except Exception:
    sys.exit(0)
d = p.get("Entitlements", {})
appid = d.get("application-identifier") or d.get("com.apple.application-identifier") or ""
depura = d.get("get-task-allow") or d.get("com.apple.security.get-task-allow")
if (appid.split(".", 1)[-1] == sys.argv[1]
        and (("OSX" in p.get("Platform", [])) if sys.argv[3] else ("iOS" in p.get("Platform", []) or "IOS" in p.get("Platform", [])))
        and "ProvisionedDevices" not in p
        and bool(p.get("ProvisionsAllDevices")) == (sys.argv[3] == "developerid")
        and not depura and p["ExpirationDate"] > datetime.datetime.now()):
    print(p["CreationDate"].isoformat(), p["Name"], sys.argv[2], sep="\t")
' "$id" "$f" "$mac" || true
    done | sort | tail -1 | cut -f"$([ -z "$mac$formato" ] && echo 2 || echo 3)")"
    [ -z "$elegido" ] || break
  done
  [ -n "$elegido" ] || [ "$mac" != developerid ] ||
    mal "no hay ningún perfil Developer ID (MAC_APP_DIRECT) instalado para $id: configúralo con su App Group autorizado antes de empaquetar."
  [ -n "$elegido" ] || mal "no hay ningún perfil de tienda instalado para ${id} en «${dirs[*]}»: ¿pasó «apple-firma.sh preparar $([ -z "$mac" ] && echo ios || echo macappstore) ${id}»?"
  printf '%s\n' "$elegido"
}

# El perfil autoriza el grupo, además de identificar la app. Se comparte entre
# hosts de SSO y extensiones de widgets, que no reciben el grupo del Keychain.
verificar_perfil_grupo() { # <ruta> <App ID> <ios|macappstore|developerid> <grupo> [protección] [llavero] [derechos.plist]
  python3 "$(dirname "${BASH_SOURCE[0]}")/apple_perfil.py" verificar "$@"
}

# altool (ContentDelivery) se ha colgado 3 h 36 min sin escribir ni salir (41166113491,
# sytue-diapos-macos-tienda.pkg). Se acota por grupo de procesos y solo se reintenta una
# vez cuando vence el reloj, nunca ante un rechazo real de Apple.
publicar_acotado() { # <ipa|pkg>
  local paquete="$1" limite="${SYTUE_APPLE_PUBLISH_TIMEOUT_SEC:-900}"
  python3 - "$limite" app-store-connect publish --path "$paquete" <<'PY'
import os, signal, subprocess, sys
limite = float(sys.argv[1])
if limite <= 0:
    sys.exit(2)
orden = sys.argv[2:]
for intento in (1, 2):
    proc = subprocess.Popen(orden, stdout=sys.stderr, stderr=sys.stderr, start_new_session=True)
    try:
        sys.exit(proc.wait(timeout=limite))
    except subprocess.TimeoutExpired:
        try:
            os.killpg(proc.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        proc.wait()
        print(f"::warning::tiempo agotado ({limite:g}s) subiendo {orden[-1]} (intento {intento}/2)", file=sys.stderr)
sys.exit(124)
PY
}

# subir <ipa|pkg>...: a App Store Connect (los .pkg son los de la Mac App Store). SIN --testflight (que pide la revisión beta para
# probadores externos) y SIN --app-store: el build queda en TestFlight para los grupos
# INTERNOS y nada más. Se intentan todos y se dice cuáles no entraron: una app sin
# ficha en App Store Connect no deja a las otras catorce sin subir.
subir() {
  local ipa fallidos=""
  [ "$#" -gt 0 ] || mal 'subir: no hay ningún .ipa ni .pkg'
  credenciales || exit 1
  for ipa in "$@"; do
    [ -f "$ipa" ] || mal "$ipa no existe"
    if publicar_acotado "$ipa"; then
      echo "✓ $ipa: en App Store Connect (TestFlight, pruebas internas)" >&2
    else
      fallidos="$fallidos $(basename "$ipa")"
    fi
  done
  [ -z "$fallidos" ] ||
    mal "App Store Connect no aceptó:$fallidos. Arriba, qué contestó; lo habitual es que la app aún no tenga ficha (App Store Connect → Apps → +) o que ese número de build ya esté subido."
}

verificar() { # <dmg>...
  local dmg salida
  [ "$#" -gt 0 ] || mal 'verificar: no hay ningún .dmg que mirar'
  for dmg in "$@"; do
    [ -f "$dmg" ] || mal "$dmg no existe"
    xcrun stapler validate "$dmg" >&2 || mal "$dmg no lleva grapado el ticket de notarización: no se publica."
    # Lo que hará Gatekeeper al abrirlo. Sin tubería hacia `grep -q` (ci-sin-azar-test.sh).
    salida="$(spctl -a -t open --context context:primary-signature -vv "$dmg" 2>&1)" ||
      mal "Gatekeeper rechaza $dmg: $salida"
    grep -q 'source=Notarized Developer ID' <<<"$salida" ||
      mal "$dmg no figura como «Notarized Developer ID»: $salida"
    echo "✓ $dmg: notarizado, grapado y aceptado por Gatekeeper" >&2
  done
}

# La misma normalización de @file:/@env: que usa el cliente de App Store Connect.
# El llamador conserva el temporal hasta terminar y lo elimina con su trap.
clave_notaria() { # <directorio temporal privado>
  python3 "$(dirname "${BASH_SOURCE[0]}")/apple_clave.py" "$1/AuthKey.p8" ||
    mal 'no se pudo preparar la clave de notarización'
}

sondear_notaria() ( # <UUID>, solo lectura de la solicitud ya enviada
  local id="${1:-}" clave salida estado
  [[ "$#" = 1 && "$id" =~ ^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$ ]] ||
    mal 'sondear-notaria requiere el UUID de una solicitud existente'
  credenciales_api || exit 1
  umask 077
  clave="$(mktemp -d)"
  trap 'rm -rf "$clave"' EXIT
  clave_notaria "$clave"
  salida="$(xcrun notarytool info "$id" --key "$clave/AuthKey.p8" \
    --key-id "$APP_STORE_CONNECT_KEY_IDENTIFIER" --issuer "$APP_STORE_CONNECT_ISSUER_ID" \
    --output-format json)" || mal 'no se pudo consultar la solicitud de notarización existente'
  # No imprimir respuestas arbitrarias: solo el estado conocido y el ID que se pidió.
  estado="$(python3 -c '
import json, sys
try:
    r = json.load(sys.stdin)
    if r.get("id", "").lower() != sys.argv[1].lower() or r.get("status") not in ("Accepted", "In Progress", "Invalid"):
        raise ValueError()
    print(r["status"])
except (ValueError, TypeError, AttributeError):
    sys.exit(1)
' "$id" <<<"$salida")" || mal 'notarytool info no devolvió el ID y un estado de notarización conocidos'
  if [ "$estado" = Invalid ]; then
    xcrun notarytool log "$id" --key "$clave/AuthKey.p8" \
      --key-id "$APP_STORE_CONNECT_KEY_IDENTIFIER" --issuer "$APP_STORE_CONNECT_ISSUER_ID" >&2 ||
      mal 'Apple rechazó la solicitud y no se pudo leer su informe'
  fi
  python3 - "$id" "$estado" <<'PY'
import json, sys
print("SYTUE_NOTARIA_ESTADO=" + json.dumps({"id": sys.argv[1], "status": sys.argv[2],
    "accepted": sys.argv[2] == "Accepted", "stapled": False, "artifacts": False}))
PY
)

notarizar() { # <dmg>...
  local dmg clave salida id estado
  [ "$#" -gt 0 ] || mal 'notarizar: no hay ningún .dmg'
  credenciales || exit 1
  : "${SYTUE_MACOS_IDENTIDAD:?falta SYTUE_MACOS_IDENTIDAD (la da «apple-firma.sh preparar macos»)}"
  # notarytool solo lee la clave de API de un FICHERO: en un temporal 600 que se borra.
  umask 077
  clave="$(mktemp -d)"
  # shellcheck disable=SC2064 # la ruta se fija ahora, a propósito
  trap "rm -rf '$clave'" EXIT
  clave_notaria "$clave"
  for dmg in "$@"; do
    # Los .app de dentro ya van firmados (empaquetar.sh, runtime endurecido); el .dmg
    # también, o `spctl -t open` no tiene firma primaria que mirar.
    codesign -f -s "$SYTUE_MACOS_IDENTIDAD" --timestamp "$dmg" >&2
    if ! salida="$(xcrun notarytool submit "$dmg" --key "$clave/AuthKey.p8" \
      --key-id "$APP_STORE_CONNECT_KEY_IDENTIFIER" --issuer "$APP_STORE_CONNECT_ISSUER_ID" \
      --wait --timeout 45m --output-format json 2>&1)"; then
      # notarytool escribe el JSON del timeout en stderr. El ID ya existe: no
      # confundir el fin de nuestra espera con un fallo de subida ni reenviarlo.
      id="$(sed -n 's/.*"id" *: *"\([^"]*\)".*/\1/p' <<<"$salida" | sed -n 1p)"
      if [[ "$id" =~ ^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$ ]] &&
        grep -q 'Timeout of' <<<"$salida"; then
        mal "notarytool agotó la espera de la solicitud $id: consultar ese mismo UUID con sondear-notaria; no reenviar. $salida"
      fi
      mal "notarytool no pudo completar la notarización de $dmg: $salida"
    fi
    id="$(sed -n 's/.*"id" *: *"\([^"]*\)".*/\1/p' <<<"$salida" | sed -n 1p)"
    estado="$(sed -n 's/.*"status" *: *"\([^"]*\)".*/\1/p' <<<"$salida" | sed -n 1p)"
    # `--wait` sale 0 también con «Invalid»: lo que manda es el estado.
    if [ "$estado" != Accepted ]; then
      [ -z "$id" ] || xcrun notarytool log "$id" --key "$clave/AuthKey.p8" \
        --key-id "$APP_STORE_CONNECT_KEY_IDENTIFIER" --issuer "$APP_STORE_CONNECT_ISSUER_ID" >&2 || true
      mal "Apple no notarizó $dmg (estado «${estado:-desconocido}»): arriba, el informe."
    fi
    xcrun stapler staple "$dmg" >&2
  done
  verificar "$@"
}

orden="${1:-}"
[ "$#" -eq 0 ] || shift
case "$orden" in
directorio-xcode) directorio_xcode ;;
credenciales) credenciales ;;
preparar) preparar "$@" ;;
sondear-macos)
  llavero_sonda="$(llavero_efimero)" || exit 1
  python3 "$(dirname "${BASH_SOURCE[0]}")/apple_firma_sonda.py" "$llavero_sonda" "${SYTUE_MACOS_IDENTIDAD:?falta identidad de preparar macos}"
  ;;
sondear-notaria) sondear_notaria "$@" ;;
notarizar) notarizar "$@" ;;
verificar) verificar "$@" ;;
perfil) perfil "${1:-}" ;;
perfil-mac) perfil "${1:-}" mac ;;
perfil-ruta)
  case "${2:-}" in
  ios) perfil "${1:-}" "" ruta ;;
  macappstore) perfil "${1:-}" mac ruta ;;
  developerid) perfil "${1:-}" developerid ruta ;;
  *) mal 'uso: perfil-ruta <App ID> <ios|macappstore|developerid>' ;;
  esac ;;
instalar-perfil-ios)
  [ "$#" = 1 ] || mal 'uso: instalar-perfil-ios <App ID>'
  perfil_instalar="$(perfil "$1" "" ruta)" || exit 1
  llavero_operacion instalar-perfil "$perfil_instalar" "$1"
  ;;
verificar-perfil-grupo) verificar_perfil_grupo "$@" ;;
llavero-efimero) llavero_efimero ;;
subir) subir "$@" ;;
*)
  echo "uso: $0 {credenciales|llavero-efimero|preparar macos|preparar ios <App ID>...|preparar macappstore <App ID>...|sondear-macos|sondear-notaria <UUID>|notarizar <dmg>...|verificar <dmg>...|perfil <App ID>|perfil-mac <App ID>|perfil-ruta <App ID> <ios|macappstore|developerid>|verificar-perfil-grupo <ruta> <AppID> <plataforma> <grupo> [protección] [llavero] [derechos.plist]|subir <ipa|pkg>...}" >&2
  exit 2
  ;;
esac
