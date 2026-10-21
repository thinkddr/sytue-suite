#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Prueba que el informe que se publica se puede LEER.
#
# POR QUÉ. El cuerpo iba en base64 y las incidencias eran un churro que había que
# decodificar a mano para saber qué había fallado. Lo que se comprueba aquí es
# justo eso: que lo que sale por el cable es JSON válido, que el log va en claro y
# que no lleva los códigos de color del terminal.
#
# Con un `curl` de mentira delante, que en vez de publicar guarda lo que le pasan.
set -uo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUION="$DIR/accion.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fallos=0
comprobar() {
  if [ "$2" = "$3" ]; then echo "ok - $1"; else
    echo "not ok - $1" >&2; echo "    esperaba: $2" >&2; echo "    obtuvo:   $3" >&2
    fallos=$((fallos + 1))
  fi
}

# Un log como los de verdad: con colores, comillas y una barra invertida.
printf '%s\n' \
  'compilando…' \
  "$(printf '\033[93mwarning\033[0m: algo')" \
  'error TS2741: falta "erp" en C:\ruta' > "$TMP/log.txt"

cat > "$TMP/curl" <<'FALSO'
#!/usr/bin/env bash
# Guarda el JSON que le pasan por --data @fichero y dice que fue bien.
for i in $(seq 1 $#); do
  if [ "${!i}" = "--data" ]; then j=$((i+1)); cp "${!j#@}" "$SALIDA"; fi
done
exit 0
FALSO
chmod +x "$TMP/curl"

SALIDA="$TMP/enviado.json" PATH="$TMP:$PATH" \
  GITHUB_TOKEN=x GITHUB_API_URL=https://api.ejemplo.test \
  GITHUB_REPOSITORY=org/repo GITHUB_WORKFLOW=ci GITHUB_REF_NAME=main \
  GITHUB_SHA=abcdef1234567890 GITHUB_RUN_ID=7 GITHUB_SERVER_URL=https://ejemplo.test \
  bash "$GUION" "$TMP/log.txt" "fallo quality · main · abcdef1" >/dev/null 2>&1

[ -s "$TMP/enviado.json" ] || { echo "not ok - no se envió nada" >&2; exit 1; }

comprobar "lo que se envía es JSON válido" "sí" \
  "$(python3 -c 'import json,sys; json.load(open(sys.argv[1])); print("sí")' "$TMP/enviado.json" 2>/dev/null || echo "no")"

cuerpo="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("body",""))' "$TMP/enviado.json")"

comprobar "el log va EN CLARO, no en base64" "sí" \
  "$(grep -q 'error TS2741' <<<"$cuerpo" && echo "sí" || echo "no")"
comprobar "no quedan códigos de color" "sí" \
  "$(grep -q $'\033' <<<"$cuerpo" && echo "no" || echo "sí")"
comprobar "las comillas y la barra sobreviven" "sí" \
  "$(grep -qF 'falta "erp" en C:\ruta' <<<"$cuerpo" && echo "sí" || echo "no")"
comprobar "dice dónde falló" "sí" \
  "$(grep -q 'ejemplo.test/org/repo/actions/runs/7' <<<"$cuerpo" && echo "sí" || echo "no")"

titulo="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("title",""))' "$TMP/enviado.json")"
comprobar "el título es el que se pidió" "fallo quality · main · abcdef1" "$titulo"

[ "$fallos" -eq 0 ] || exit 1
echo "# todo bien"
