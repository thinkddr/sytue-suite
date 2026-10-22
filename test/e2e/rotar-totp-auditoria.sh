#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Rota el segundo factor de la cuenta de auditoría y deja la semilla NUEVA en los
# tres sitios donde hace falta: el llavero de macOS, `~/.config/sytue/audit.env` del
# piloto (las sondas de Playwright) y el secreto del repositorio. Sirve también para
# el PRIMER alta: sin factor, al pedir el código se pulsa Intro.
#
#   ./rotar-totp-auditoria.sh
#
# POR QUÉ UN GUION Y NO CUATRO `curl` PEGADOS A MANO. La semilla no puede
# aparecer en una línea de comando —queda en el historial del shell, en la salida
# de `ps` mientras corre y en el log de cualquier agente— y el alta no se puede
# partir: entre `enroll` y `confirm` la cuenta se queda con el factor viejo
# desactivado y el nuevo sin confirmar, o sea SIN segundo factor. Aquí va todo
# seguido y la semilla solo viaja por variables del proceso.
#
# Lo que hace falta tener a mano:
#   · la contraseña de ops@sytue.com en el llavero (`sytue-audit-pass`);
#   · la app de autenticación con el factor ACTUAL, para probar quién eres;
#   · `gh` con sesión en GitHub, para subir el secreto al repositorio;
#   · `ssh piloto` por clave (otro alias con SYTUE_PILOTO=…; vacío, no lo toca).
set -euo pipefail

CUENTA="${SYTUE_AUDIT_EMAIL:-ops@sytue.com}"
IDP="${SYTUE_IDP_URL:-https://id.sytue.eu}"
# El repositorio en GitHub, la forja desde el 2026-09-27 (antes, git.gay).
REPO="${SYTUE_GIT_REPO:-thinkddr/sytue-one}"
PILOTO="${SYTUE_PILOTO-piloto}"
GALLETAS="$(mktemp)"
trap 'rm -f "$GALLETAS"' EXIT

PASS="$(security find-generic-password -s sytue-audit-pass -a "$CUENTA" -w 2>/dev/null || true)"
if [ -z "$PASS" ]; then
  echo "No hay contraseña en el llavero para $CUENTA." >&2
  echo "  security add-generic-password -a $CUENTA -s sytue-audit-pass -w" >&2
  exit 2
fi

# El código ACTUAL lo teclea una persona: es la prueba de identidad que exige
# desactivar el factor, y no está en ningún sitio del que se pueda leer.
#
# Se pide en blanco a propósito: si una pasada anterior dejó el alta a medias, la
# cuenta ya NO tiene segundo factor y no hay código que dar. En ese caso se deja
# vacío y el guion sigue —abajo detecta el alta pendiente y la completa—.
printf 'Código de 6 dígitos de tu app (Intro si el alta quedó a medias, para %s): ' "$CUENTA" >&2
read -r -s CODIGO_ACTUAL
echo >&2

api() { # api <ruta> [campo=valor ...]
  local ruta="$1"; shift
  # `-X POST` EXPLÍCITO, y no por gusto: `curl` solo cambia a POST cuando hay
  # datos, así que una llamada SIN campos —`enroll`, que no lleva ninguno— salía
  # como GET. El IdP contestaba con la página y el guion seguía como si el alta
  # se hubiera hecho. Eso es lo que dejó la cuenta sin segundo factor: el
  # `disable` sí llevaba datos y sí se ejecutó, y el `enroll` que debía
  # sustituirlo nunca llegó a ocurrir.
  #
  # Los campos van por la ENTRADA ESTÁNDAR, no como argumentos de `curl`: la
  # contraseña y los códigos se verían en `ps` mientras corre. `printf` es del
  # propio bash y no crea proceso.
  { [ $# -eq 0 ] || printf '%s\0' "$@"; } |
    python3 -c 'import sys, urllib.parse as u
print(u.urlencode([tuple(c.split("=", 1)) for c in sys.stdin.read().split("\0") if c]), end="")' |
    curl -sS -b "$GALLETAS" -c "$GALLETAS" -o /dev/null -w '%{http_code}' \
      --max-time 30 -X POST -H 'content-type: application/x-www-form-urlencoded' \
      --data-binary @- "$IDP$ruta"
}

echo "· entrando…" >&2
api /login/ "email=$CUENTA" "password=$PASS" "return_to=/account" >/dev/null

# LEE LA SEMILLA PENDIENTE ANTES DE TOCAR NADA, y esto es lo que hace el guion
# repetible. Si una pasada anterior se paró entre `enroll` y `confirm`, la cuenta
# se quedó SIN segundo factor y con un alta a medias: volver a empezar desde
# `disable` fallaría —no hay nada que retirar— y el guion moriría dejándola igual
# de expuesta. Con el alta pendiente ya hay semilla, así que solo falta
# confirmarla, y ese es justo el paso que arregla el estado.
cuenta() { curl -sS -b "$GALLETAS" --max-time 30 "$IDP/account"; }

# EL ESTADO SE MIRA, NO SE SUPONE, y por no hacerlo la primera versión dejó la
# cuenta a medias. El IdP los distingue por el formulario que ofrece:
#   confirm → alta pendiente (hay semilla, falta activarla)
#   disable → factor activo (hay que retirarlo antes)
#   enroll  → sin factor (ni activo ni pendiente)
# La semilla sale del `otpauth://`, que es inequívoco: la otra copia va agrupada
# de cuatro en cuatro para poder teclearla y habría que quitarle los espacios.
PAGINA="$(cuenta)"
# El separador NO se busca, y por eso fallaba: en el HTML el `&` viene escapado
# como `&amp;`, de modo que delante de `secret=` hay un «;». Y `q.Encode()` de Go
# ordena los parámetros alfabéticamente, así que `secret` va el ÚLTIMO
# (algorithm, digits, issuer, period, secret) y no justo detrás del «?». Como
# `secret=` aparece una sola vez en la URI, basta con anclarse a él.
semilla_de() { grep -o 'otpauth://[^<"]*' | head -1 | sed -n 's/.*secret=\([A-Z2-7]*\).*/\1/p'; }

if printf '%s' "$PAGINA" | grep -q 'action="/account/mfa/totp/confirm"'; then
  echo "· alta a medias: se completa esa en vez de empezar de cero." >&2
  echo "  (la cuenta está SIN segundo factor hasta que se confirme)" >&2
  SEMILLA="$(printf '%s' "$PAGINA" | semilla_de)"

elif printf '%s' "$PAGINA" | grep -q 'action="/account/mfa/totp/disable"'; then
  [ -n "$CODIGO_ACTUAL" ] || {
    echo "La cuenta tiene el segundo factor ACTIVO: hace falta un código para retirarlo." >&2
    exit 2
  }
  # El segundo paso es OTRA petición: el login con MFA no termina en el primer POST.
  mfa="$(api /login/mfa "code=$CODIGO_ACTUAL")"
  case "$mfa" in
    200|302|303) ;;
    *) echo "El IdP rechazó el código ($mfa). Prueba con el siguiente, que rota cada 30 s." >&2; exit 1 ;;
  esac
  echo "· retirando el factor actual…" >&2
  api /account/mfa/totp/disable "code=$CODIGO_ACTUAL" "password=$PASS" >/dev/null
  echo "· dando de alta uno nuevo…" >&2
  api /account/mfa/totp/enroll >/dev/null
  SEMILLA="$(cuenta | semilla_de)"

else
  # Sin factor: es el estado en el que queda una rotación interrumpida, y NO hay
  # nada que retirar. Pedir un código aquí sería pedir algo que no existe.
  echo "· la cuenta no tiene segundo factor: se da de alta uno…" >&2
  api /account/mfa/totp/enroll >/dev/null
  SEMILLA="$(cuenta | semilla_de)"
fi
if [ -z "$SEMILLA" ]; then
  echo "No se encontró la semilla en /account. El alta puede haberse quedado a medias:" >&2
  echo "  abre $IDP/account y termina de configurar el segundo factor a mano." >&2
  exit 1
fi

# Se confirma con un código generado de la semilla NUEVA. Sin este paso el alta
# queda pendiente y la cuenta se queda sin segundo factor.
echo "· confirmando…" >&2
CODIGO_NUEVO="$(SEMILLA="$SEMILLA" python3 - <<'PY'
import base64, hmac, hashlib, os, struct, time
s = os.environ['SEMILLA']
clave = base64.b32decode(s + '=' * (-len(s) % 8))
h = hmac.new(clave, struct.pack('>Q', int(time.time()) // 30), hashlib.sha1).digest()
o = h[-1] & 0x0F
print(f"{(int.from_bytes(h[o:o+4], 'big') & 0x7FFFFFFF) % 1000000:06d}")
PY
)"
conf="$(api /account/mfa/totp/confirm "code=$CODIGO_NUEVO")"
case "$conf" in
  200|302|303) ;;
  *) echo "El IdP no aceptó la confirmación ($conf). Termina en $IDP/account." >&2; exit 1 ;;
esac

echo "· guardando en el llavero…" >&2
security delete-generic-password -a "$CUENTA" -s sytue-audit-totp >/dev/null 2>&1 || true
# Por `security -i`, que lee la orden de la entrada estándar: con `-w "$SEMILLA"`
# la semilla iba en los argumentos, visible en `ps`.
printf 'add-generic-password -a "%s" -s sytue-audit-totp -w "%s"\n' "$CUENTA" "$SEMILLA" | security -i

if [ -n "$PILOTO" ]; then
  echo "· guardándola en el piloto ($PILOTO:~/.config/sytue/audit.env)…" >&2
  # Sustituye la línea de SYTUE_AUDIT_TOTP y deja el resto del fichero como estaba.
  # La semilla viaja por la entrada estándar del ssh, no en la orden remota.
  if ! printf 'SYTUE_AUDIT_TOTP=%s\n' "$SEMILLA" | ssh "$PILOTO" 'set -e; umask 077
      f=~/.config/sytue/audit.env; mkdir -p "${f%/*}"; touch "$f"
      { grep -v "^SYTUE_AUDIT_TOTP=" "$f" || true; cat; } >"$f.nuevo"; mv "$f.nuevo" "$f"'; then
    echo "  ⚠ no se pudo escribir en el piloto: la semilla está en el llavero, no allí." >&2
    echo "    Cópiala desde el llavero sin pegarla en ningún chat (docs/operations.md#org-de-pruebas)." >&2
    exit 1
  fi
fi

echo "· subiendo el secreto del repositorio…" >&2
# Con `gh`, que cifra el valor con la clave pública del repositorio (GitHub no
# acepta el secreto en claro) y lo lee por la entrada estándar: la semilla no va
# en ningún argumento.
if ! command -v gh >/dev/null || ! printf '%s' "$SEMILLA" | gh secret set SYTUE_AUDIT_TOTP -R "$REPO" >/dev/null; then
  echo "  ⚠ no se pudo subir: la semilla está en el llavero, no en el repositorio." >&2
  echo "    Dala de alta a mano: gh secret set SYTUE_AUDIT_TOTP -R $REPO" >&2
  exit 1
fi
echo "✓ rotada. Llavero, piloto y secreto del repositorio al día." >&2

echo >&2
echo "Siguiente paso: vuelve a escanear el QR en tu app de autenticación." >&2
echo "  $IDP/account  ·  el factor viejo ya no vale." >&2
