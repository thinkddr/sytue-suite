#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Sytue One — smoke test e2e (extremo a extremo).
# ---------------------------------------------------------------------------
# Verifica, contra una suite `make dev-up` YA levantada (Docker Compose + Bun),
# que el sistema responde de extremo a extremo. NO levanta ni derriba nada:
# asume la pila corriendo bajo *.sytue.localhost (Traefik + TLS mkcert).
#
# Mapa de checks (ver docs/operations.md):
#   1. Salud de CADA servicio Go por su puerto local: /healthz y /readyz.
#   2. OIDC discovery de Sytue ID en /.well-known/openid-configuration
#      SIN /realms/ (IdP propio, no Keycloak) y con issuer coherente.
#   3. Flujo OIDC + PKCE básico contra `id`: authorize con code_challenge S256
#      del client `sytue-one` -> 302 a /login (sin error OAuth). Prueba que el
#      authorization endpoint acepta una petición PKCE bien formada de extremo
#      a extremo (sin necesidad de credenciales interactivas).
#   4. Cada app web: la raíz redirige a su portada y /auth/login lleva a Sytue
#      ID (Traefik, el BFF y el cliente OIDC de la app encajan).
#
# Uso:
#   make e2e                       # objetivo sugerido (ver docs/operations.md)
#   scripts/e2e/smoke.sh           # directo
#   E2E_BASE_DOMAIN=sytue.localhost scripts/e2e/smoke.sh
#   E2E_INSECURE=1 scripts/e2e/smoke.sh   # si el CA mkcert no está instalado
#
# Requisitos: bash, curl, openssl (para PKCE S256). Docker + Bun se asumen ya
# usados por `make dev-up`. 100 % UE, sin secretos: nada sensible en este script.
# ---------------------------------------------------------------------------

set -u -o pipefail

# --- Configuración (parametrizable por entorno) -----------------------------
BASE_DOMAIN="${E2E_BASE_DOMAIN:-sytue.localhost}"
SCHEME="${E2E_SCHEME:-https}"
# Timeout por petición (segundos).
TIMEOUT="${E2E_TIMEOUT:-10}"
# Reintentos y espera entre intentos (para arranque en frío de la pila).
RETRIES="${E2E_RETRIES:-1}"
RETRY_WAIT="${E2E_RETRY_WAIT:-3}"
# Client OIDC público de referencia para el flujo PKCE (sembrado en Sytue ID).
OIDC_CLIENT_ID="${E2E_OIDC_CLIENT_ID:-sytue-one}"
# redirect_uri registrado para ese client en dev (ver seed 0002_seed_clients).
OIDC_REDIRECT_URI="${E2E_OIDC_REDIRECT_URI:-${SCHEME}://one.${BASE_DOMAIN}/auth/callback}"

# TLS mkcert: si el CA local NO está instalado (`mkcert -install`), curl fallará
# la verificación. E2E_INSECURE=1 permite saltarla (solo para dev/*.localhost).
CURL_TLS_OPTS=()
if [ "${E2E_INSECURE:-0}" = "1" ]; then
  CURL_TLS_OPTS+=(--insecure)
fi

# --- Colores (desactivados si no hay TTY o si NO_COLOR) ----------------------
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
  C_GREEN=$'\033[32m'; C_RED=$'\033[31m'; C_YELLOW=$'\033[33m'
  C_BOLD=$'\033[1m'; C_DIM=$'\033[2m'; C_RESET=$'\033[0m'
else
  C_GREEN=""; C_RED=""; C_YELLOW=""; C_BOLD=""; C_DIM=""; C_RESET=""
fi

PASS_COUNT=0
FAIL_COUNT=0
FAILED_CHECKS=()

# --- Utilidades -------------------------------------------------------------
info() { printf '%s→%s %s\n' "$C_DIM" "$C_RESET" "$*"; }

pass() {
  PASS_COUNT=$((PASS_COUNT + 1))
  printf '  %s✔%s %s\n' "$C_GREEN" "$C_RESET" "$1"
}

fail() {
  FAIL_COUNT=$((FAIL_COUNT + 1))
  FAILED_CHECKS+=("$1")
  printf '  %sx%s %s%s%s\n' "$C_RED" "$C_RESET" "$C_RED" "$1" "$C_RESET"
  if [ -n "${2:-}" ]; then
    printf '      %s%s%s\n' "$C_DIM" "$2" "$C_RESET"
  fi
}

section() {
  printf '\n%s%s%s\n' "$C_BOLD" "$1" "$C_RESET"
}

# require CMD... — aborta si falta una herramienta.
require() {
  for c in "$@"; do
    if ! command -v "$c" >/dev/null 2>&1; then
      printf '%sERROR:%s se requiere "%s" y no está en PATH.\n' "$C_RED" "$C_RESET" "$c" >&2
      exit 2
    fi
  done
}

# curl_status URL — imprime el código HTTP (o 000 si no conecta). Reintenta.
curl_status() {
  local url="$1" attempt=1 code
  while :; do
    code=$(curl -s -o /dev/null -w '%{http_code}' \
      --max-time "$TIMEOUT" "${CURL_TLS_OPTS[@]}" "$url" 2>/dev/null)
    if [ "$code" != "000" ] || [ "$attempt" -ge "$RETRIES" ]; then
      break
    fi
    attempt=$((attempt + 1))
    sleep "$RETRY_WAIT"
  done
  printf '%s' "$code"
}

# curl_body URL — imprime el cuerpo (para inspección de JSON). Reintenta ante 000.
curl_body() {
  local url="$1" attempt=1 body code
  while :; do
    body=$(curl -s --max-time "$TIMEOUT" "${CURL_TLS_OPTS[@]}" \
      -w '\n%{http_code}' "$url" 2>/dev/null)
    code="${body##*$'\n'}"
    if [ "$code" != "000" ] || [ "$attempt" -ge "$RETRIES" ]; then
      break
    fi
    attempt=$((attempt + 1))
    sleep "$RETRY_WAIT"
  done
  # Devuelve el cuerpo sin la última línea (el código).
  printf '%s' "${body%$'\n'*}"
}

# base64url STDIN — codificación base64url sin padding (para PKCE).
b64url() {
  openssl base64 -A | tr '+/' '-_' | tr -d '='
}

# ---------------------------------------------------------------------------
# Servicios Go (backend): "nombre|puerto". Se comprueban por su puerto publicado
# en 127.0.0.1 y no por el host público: varios hosts públicos (drive, calendar,
# contacts…) sirven la app web, no el backend, y su /readyz respondía un 302 al
# login. Rooms no aparece en /readyz: sin servidor Matrix (no lo hay en Compose)
# responde «not_ready» a propósito, aunque el servicio esté sano.
# ---------------------------------------------------------------------------
LOCAL_HOST="${E2E_LOCAL_HOST:-127.0.0.1}"
GO_SERVICES=(
  "id|8081" "drive|8082" "docs|8083" "ai|8084" "rooms|8085" "mail|8086"
  "calendar|8087" "contacts|8088" "notes|8089" "cloud-api|8090" "cloud-builder|8091"
  "hub|8092" "analytics-ingest|8094" "analytics-api|8095" "syphotos|8097"
  "syerp|8098" "cast|8099" "pages|8100" "bases|8102" "cloud-controller|8103"
)
SIN_READYZ=" rooms "

# Apps web: "host|base". La raíz del host redirige a su portada (<base>/), que sin
# sesión manda al login del BFF (/auth/login), que a su vez manda a Sytue ID. Es el
# camino que recorre cualquiera que abra la app, y el que se rompía.
WEB_APPS=(
  "one|/one" "admin|/admin" "mail|/mail" "drive|/drive" "docus|/docus"
  "calcus|/calcus" "diapos|/diapos" "certipdf|/certipdf" "calendar|/calendar"
  "contacts|/contacts" "notes|/notes" "rooms|/rooms" "erp|/erp" "cast|/cast"
  "web|/web" "bases|/bases" "cloud|/cloud"
)

# ===========================================================================
# CHECK 1 — Salud de cada servicio Go (/healthz + /readyz por su puerto local).
# ===========================================================================
check_health() {
  section "1) Salud de los servicios Go (/healthz + /readyz en ${LOCAL_HOST})"
  local entry name port base code
  for entry in "${GO_SERVICES[@]}"; do
    name="${entry%%|*}"
    port="${entry##*|}"
    base="http://${LOCAL_HOST}:${port}"
    code=$(curl_status "${base}/healthz")
    if [ "$code" = "200" ]; then
      pass "${name} /healthz -> 200"
    else
      fail "${name} /healthz" "esperado 200, obtenido ${code} (${base}/healthz)"
    fi
    case "$SIN_READYZ" in *" ${name} "*) continue ;; esac
    code=$(curl_status "${base}/readyz")
    if [ "$code" = "200" ]; then
      pass "${name} /readyz -> 200"
    else
      # 503 = servicio vivo pero una dependencia no lista.
      fail "${name} /readyz" "esperado 200, obtenido ${code} (${base}/readyz)"
    fi
  done
}

# ===========================================================================
# CHECK 2 — OIDC discovery de Sytue ID (sin /realms/).
# ===========================================================================
check_oidc_discovery() {
  section "2) OIDC discovery de Sytue ID (/.well-known/openid-configuration)"
  local issuer="${SCHEME}://id.${BASE_DOMAIN}"
  local disco="${issuer}/.well-known/openid-configuration"

  local code
  code=$(curl_status "$disco")
  if [ "$code" != "200" ]; then
    fail "discovery accesible" "esperado 200, obtenido ${code} (${disco})"
    return
  fi
  pass "discovery -> 200"

  # El path NO debe requerir /realms/ (patrón Keycloak, expresamente fuera).
  case "$disco" in
    *"/realms/"*)
      fail "discovery sin /realms/" "la URL contiene /realms/ (patrón Keycloak)"
      ;;
    *)
      pass "discovery sin /realms/ (IdP propio, no Keycloak)"
      ;;
  esac

  local body
  body=$(curl_body "$disco")

  # issuer coherente con el host de Sytue ID.
  if grep -q "\"issuer\"[[:space:]]*:[[:space:]]*\"${issuer}\"" <<<"$body"; then
    pass "issuer = ${issuer}"
  else
    fail "issuer coherente" "no se encontró issuer=\"${issuer}\" en el documento"
  fi

  # Endpoints y capacidades mínimas para OIDC+PKCE (Authorization Code + S256).
  local field
  for field in authorization_endpoint token_endpoint jwks_uri; do
    if grep -q "\"${field}\"" <<<"$body"; then
      pass "discovery expone ${field}"
    else
      fail "discovery expone ${field}" "campo ausente en el documento"
    fi
  done

  if grep -q '"S256"' <<<"$body"; then
    pass "code_challenge_methods_supported incluye S256 (PKCE)"
  else
    fail "PKCE S256 soportado" "no se declara S256 en el documento de discovery"
  fi

  # JWKS accesible (necesario para que cada servicio Go valide los JWT).
  code=$(curl_status "${issuer}/.well-known/jwks.json")
  if [ "$code" = "200" ]; then
    pass "JWKS -> 200"
  else
    fail "JWKS accesible" "esperado 200, obtenido ${code}"
  fi
}

# ===========================================================================
# CHECK 3 — Flujo OIDC + PKCE básico contra Sytue ID.
# Genera code_verifier/challenge (S256) y pide /oauth2/authorize con el client
# sembrado `sytue-one`. Sin sesión SSO, el AS debe redirigir (302) a /login
# CONSERVANDO la petición — y NO devolver un error OAuth (?error=...). Eso
# prueba, de extremo a extremo, que el authorization endpoint acepta una
# petición Authorization Code + PKCE bien formada.
# ===========================================================================
check_oidc_pkce() {
  section "3) Flujo OIDC + PKCE básico (authorize del client ${OIDC_CLIENT_ID})"
  local issuer="${SCHEME}://id.${BASE_DOMAIN}"

  # code_verifier: 43-128 chars base64url (RFC 7636). 32 bytes aleatorios.
  local verifier challenge
  verifier=$(openssl rand 32 | b64url)
  # code_challenge = BASE64URL(SHA256(verifier)).
  challenge=$(printf '%s' "$verifier" | openssl dgst -binary -sha256 | b64url)

  if [ -z "$verifier" ] || [ -z "$challenge" ]; then
    fail "generar PKCE S256" "openssl no produjo verifier/challenge"
    return
  fi
  pass "PKCE S256 generado (verifier ${#verifier} chars)"

  local state nonce authz
  state="e2e-$(openssl rand -hex 8)"
  nonce="e2e-$(openssl rand -hex 8)"

  # Construcción de la query de authorize (urlencode simple de los valores).
  authz="${issuer}/oauth2/authorize"
  authz+="?response_type=code"
  authz+="&client_id=${OIDC_CLIENT_ID}"
  authz+="&redirect_uri=$(url_encode "${OIDC_REDIRECT_URI}")"
  authz+="&scope=$(url_encode 'openid profile email')"
  authz+="&state=${state}"
  authz+="&nonce=${nonce}"
  authz+="&code_challenge=${challenge}"
  authz+="&code_challenge_method=S256"

  # NO seguir redirecciones: queremos inspeccionar el 302 y su Location.
  local headers code location
  headers=$(curl -s -D - -o /dev/null --max-time "$TIMEOUT" \
    "${CURL_TLS_OPTS[@]}" "$authz" 2>/dev/null)
  code=$(printf '%s' "$headers" | awk 'NR==1{print $2}')
  location=$(printf '%s' "$headers" \
    | tr -d '\r' | awk -F': ' 'tolower($1)=="location"{print $2; exit}')

  if [ "$code" = "302" ] || [ "$code" = "303" ] || [ "$code" = "301" ]; then
    pass "authorize -> ${code} (redirección, sin error directo)"
  else
    fail "authorize responde con redirección" "esperado 30x, obtenido ${code:-<none>}"
    return
  fi

  # El destino debe ser el login del AS (no hay sesión) y NO un error OAuth.
  case "$location" in
    *"error="*)
      fail "authorize sin error OAuth" "Location contiene un error: ${location}"
      ;;
    *"/login"*)
      pass "authorize redirige a /login conservando la petición (flujo PKCE aceptado)"
      ;;
    "")
      fail "authorize devuelve Location" "no se recibió cabecera Location"
      ;;
    *)
      # Cualquier otra redirección sin error también es aceptable (p. ej. consent);
      # lo esencial es que la petición PKCE fue ACEPTADA (no rechazada con ?error=).
      pass "authorize redirige sin error OAuth (${location%%\?*})"
      ;;
  esac
}

# url_encode STR — codificación porcentual mínima (segura para query values).
url_encode() {
  local s="$1" out="" c i
  for (( i=0; i<${#s}; i++ )); do
    c="${s:$i:1}"
    case "$c" in
      [a-zA-Z0-9.~_-]) out+="$c" ;;
      *) out+=$(printf '%%%02X' "'$c") ;;
    esac
  done
  printf '%s' "$out"
}

# ===========================================================================
# CHECK 4 — Cada app web: raíz -> portada -> login del BFF -> Sytue ID.
# ===========================================================================
# location URL — cabecera Location de una respuesta (sin seguir redirecciones).
location_of() {
  curl -s -o /dev/null -w '%{http_code} %{redirect_url}' --max-time "$TIMEOUT" \
    "${CURL_TLS_OPTS[@]}" "$1" 2>/dev/null
}

check_web_apps() {
  section "4) Apps web (*.${BASE_DOMAIN}): portada, login y enlace con Sytue ID"
  local entry host base r code dest
  for entry in "${WEB_APPS[@]}"; do
    host="${entry%%|*}.${BASE_DOMAIN}"
    base="${entry##*|}"
    r=$(location_of "${SCHEME}://${host}/")
    code="${r%% *}"
    dest="${r#* }"
    if [ "${code:0:1}" = "3" ] && [ "$dest" = "${SCHEME}://${host}${base}/" ]; then
      pass "${host}/ -> ${base}/"
    else
      fail "${host}/ redirige a su portada" "esperado 30x a ${base}/, obtenido ${r}"
      continue
    fi
    r=$(location_of "${SCHEME}://${host}/auth/login")
    code="${r%% *}"
    dest="${r#* }"
    case "$code|$dest" in
      3*"|${SCHEME}://id.${BASE_DOMAIN}/oauth2/authorize?"*)
        pass "${host}/auth/login -> Sytue ID (el BFF llega al IdP)"
        ;;
      *)
        fail "${host}/auth/login" "esperado 30x a id.${BASE_DOMAIN}/oauth2/authorize, obtenido ${r:0:160}"
        ;;
    esac
  done
}

# ===========================================================================
# Main
# ===========================================================================
main() {
  require curl openssl awk tr

  printf '%sSytue One — smoke e2e%s  (dominio: %s%s%s, esquema: %s)\n' \
    "$C_BOLD" "$C_RESET" "$C_BOLD" "$BASE_DOMAIN" "$C_RESET" "$SCHEME"
  info "Asume 'make dev-up' YA levantado (Docker Compose + Bun). No arranca nada."
  if [ "${E2E_INSECURE:-0}" = "1" ]; then
    printf '  %sTLS: verificación DESACTIVADA (E2E_INSECURE=1)%s\n' "$C_YELLOW" "$C_RESET"
  else
    info "TLS: verificando el CA mkcert. Si falla, ejecuta 'mkcert -install' o usa E2E_INSECURE=1."
  fi

  check_health
  check_oidc_discovery
  check_oidc_pkce
  check_web_apps

  # --- Resumen -------------------------------------------------------------
  section "Resumen"
  local total=$((PASS_COUNT + FAIL_COUNT))
  if [ "$FAIL_COUNT" -eq 0 ]; then
    printf '%s%sOK%s — %d/%d checks pasaron.\n' \
      "$C_BOLD" "$C_GREEN" "$C_RESET" "$PASS_COUNT" "$total"
    exit 0
  fi

  printf '%s%sFALLO%s — %d/%d checks pasaron, %d fallaron:\n' \
    "$C_BOLD" "$C_RED" "$C_RESET" "$PASS_COUNT" "$total" "$FAIL_COUNT"
  local f
  for f in "${FAILED_CHECKS[@]}"; do
    printf '  %s- %s%s\n' "$C_RED" "$f" "$C_RESET"
  done
  exit 1
}

main "$@"
