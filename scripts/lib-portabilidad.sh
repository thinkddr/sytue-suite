# shellcheck shell=bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Lo que hace falta para que un guion del CI se pueda EJECUTAR también en el
# portátil. No es comodidad: los gates de pre-push ejecutan estos guiones, y un
# gate que solo pasa en Linux acaba normalizando el `git push --no-verify`, que
# es peor que no tener gate.
#
# Se importa, no se copia: lo usan `publicar-imagen-go.sh` y
# `publicar-imagen-frontend.sh`, que llevan siete `timeout` cada uno.

# `timeout` es de GNU coreutils y el macOS de serie no lo trae. Se define SOLO
# si falta, así que en Linux —donde corre el CI de verdad— sigue usándose el
# binario y esto no cambia nada.
#
# `perl -e alarm` y no un watchdog en bash: un watchdog mata el subshell pero
# deja vivo al hijo, que es justo lo que se quería acotar. Es el mismo recurso
# que ya usa el guardián del entorno efímero, por el mismo motivo. La alarma
# sobrevive al `exec` —POSIX conserva los temporizadores pendientes—, y SIGALRM
# sin manejador mata el proceso, que es el comportamiento que se busca.
if ! command -v timeout >/dev/null 2>&1; then
  timeout() {
    local segundos="$1"
    shift
    perl -e 'alarm shift; exec @ARGV' "$segundos" "$@"
  }
fi

# reintentar ejecuta un mandato que habla con una dependencia remota. Un error
# transitorio del registro no invalida una imagen que ya se construyó; se prueba
# de nuevo con una espera acotada y se devuelve el fallo real si persiste.
reintentar() { # <intentos> <espera en segundos> <mandato...>
  local intentos="$1" espera="$2" intento estado
  shift 2
  for ((intento = 1; intento <= intentos; intento++)); do
    if "$@"; then
      return 0
    else
      estado=$?
    fi
    [ "$intento" -eq "$intentos" ] && return "$estado"
    echo "${0##*/}: $* falló (intento $intento/$intentos); reintentando en ${espera}s" >&2
    sleep "$espera"
  done
}
