#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Lanza la auditoría contra PRODUCCIÓN con la cuenta de pruebas, sacando la
# contraseña y el TOTP del llavero de macOS.
#
# Existe para que la credencial no se escriba nunca en una línea de comando: lo
# que se teclea queda en el historial del shell, en la salida de `ps` mientras
# corre y en el log de cualquier agente que lo ejecute. Aquí solo viaja por
# variables de entorno del proceso hijo.
#
#   ./auditar-prod.sh [app...]          # sin args = todas
#
# EN UN SERVIDOR (sin llavero) la credencial entra por entorno. Y el fichero que
# la guarda se escribe con `%q`, que no es un detalle de estilo:
#
#   umask 077
#   printf 'SYTUE_AUDIT_PASS=%q\n' "$pass" >> ~/.config/sytue/audit.env
#   set -a; . ~/.config/sytue/audit.env; set +a
#
# Sin `%q`, un valor con un carácter especial parte la línea al hacer `source` y
# el shell intenta EJECUTAR el resto: la credencial acaba impresa en el error, en
# el log y en la terminal. Pasó el 2026-09-11 con este mismo fichero, y antes ya
# había pasado por otra vía —ver la nota sobre los prefijos de asignación más
# abajo—. Es el mismo fallo dos veces: un valor sin escapar termina donde se lee.
#
# Alta de la credencial (una vez, y la teclea una PERSONA — el `-w` sin valor la
# pide por stdin, sin eco):
#   security add-generic-password -a ops@sytue.com -s sytue-audit-pass -w
#   security add-generic-password -a ops@sytue.com -s sytue-audit-totp -w
set -euo pipefail

# ESTE GUION ARRANCA UN CHROMIUM AQUÍ, y la regla de la casa es que toda prueba
# que ejecute algo corre en la nube: lo que decide es dónde arranca el navegador,
# no contra qué URL apunta, así que apuntar a producción no lo convierte en
# «probar contra producción». La vía correcta cuesta lo mismo:
#
#     ./auditar-en-cloud.sh [app...]      # el barrido corre en un runner de Scaleway
#
# Se avisa y se sigue, en vez de bloquear: hay un caso legítimo —depurar el
# propio detector, donde ver el navegador es el trabajo— y ahí obligar a dar un
# rodeo por el CI en cada iteración no ayuda a nadie. `SYTUE_AUDIT_LOCAL=1` calla
# el aviso para ese caso.
if [ "${SYTUE_AUDIT_LOCAL:-0}" != "1" ]; then
  echo "⚠ Esto levanta Chromium EN ESTE EQUIPO. Para lanzarlo en Scaleway:" >&2
  echo "    ./auditar-en-cloud.sh ${*:-}" >&2
  echo "  (SYTUE_AUDIT_LOCAL=1 silencia este aviso)" >&2
  echo >&2
fi

CUENTA="${SYTUE_AUDIT_EMAIL:-ops@sytue.com}"
AQUI="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

leer() { security find-generic-password -s "$1" -a "$CUENTA" -w 2>/dev/null || true; }

# EL ENTORNO MANDA SOBRE EL LLAVERO, y no es una comodidad: `security` solo
# existe en macOS, así que sin esto el barrido únicamente se podía lanzar desde
# un portátil —que es justo donde la regla de la casa dice que NO debe correr un
# Chromium—. Con la variable puesta, el mismo guion vale en el servidor.
PASS="${SYTUE_AUDIT_PASS:-$(leer sytue-audit-pass)}"
if [ -z "$PASS" ]; then
  echo "No hay credencial para $CUENTA." >&2
  echo "  En macOS:    security add-generic-password -a $CUENTA -s sytue-audit-pass -w" >&2
  echo "  En servidor: exporta SYTUE_AUDIT_PASS (ver la cabecera de este guion)" >&2
  exit 2
fi
# El segundo factor es opcional: si la cuenta no lo tiene, no hay entrada.
TOTP="${SYTUE_AUDIT_TOTP:-$(leer sytue-audit-totp)}"

cd "$AQUI"

# Las variables se EXPORTAN una a una, no como prefijos del comando.
#
# Antes el segundo factor iba así:  ${TOTP:+SYTUE_AUDIT_TOTP="$TOTP"} \
# y eso está roto de una forma que solo se ve cuando la cuenta SÍ tiene TOTP: un
# prefijo de asignación tiene que ser visible para bash ANTES de expandir, y esa
# expansión ocurre después, así que el resultado se interpreta como el NOMBRE DEL
# COMANDO. El guion moría con «SYTUE_AUDIT_TOTP=…: command not found» y, al
# hacerlo, IMPRIMÍA LA SEMILLA en la terminal — justo lo que este guion existe
# para evitar (ver la cabecera: la credencial no se escribe en ninguna línea de
# comando ni acaba en el historial).
#
# Con `export` condicional no hay expansión que interpretar y el valor no aparece
# en ningún mensaje, falle lo que falle.
export SYTUE_BASE_DOMAIN="${SYTUE_BASE_DOMAIN:-sytue.eu}"
export SYTUE_AUDIT_EMAIL="$CUENTA"
export SYTUE_AUDIT_PASS="$PASS"
[ -n "$TOTP" ] && export SYTUE_AUDIT_TOTP="$TOTP"

exec bun run mobile-audit.mjs "$@"
