#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# En el servidor SSR COMPILADO —o sea, en producción— `import.meta.env` queda
# CONGELADO en el build. El bundle desplegado lo enseña sin ambigüedad:
#
#     loadConnections(Object.assign(
#       { ASSETS_PREFIX: void 0, BASE_URL: "/", DEV: false, MODE: "production",
#         PROD: true, SITE: "https://admin.sytue.eu", SSR: true },
#       { _: "/Users/ddr/.npm-global/bin/bun" }))
#
# Ahí no está ninguna variable del contenedor; el segundo argumento es un resto de
# la máquina donde se compiló. La única fuente viva es `process.env`, y `serverEnv`
# funde las dos dando prioridad al runtime.
#
# POR QUÉ UN GATE Y NO SOLO UN COMENTARIO. Este fallo apareció CUATRO veces en el
# mismo día y en cuatro sitios distintos, y nunca falla de forma visible: la
# variable sale vacía y gana el `??` de respaldo. En producción esos respaldos
# suelen acertar por casualidad —`https://docs.sytue.eu` ES el valor bueno—, así
# que el error solo se manifiesta en staging, en el efímero o en cualquier otro
# dominio, y allí apunta a PRODUCCIÓN. Es el mismo accidente que el chart ya
# documenta para `SYTUE_ROOMS_WS_URL`: staging entregaba un token de staging
# contra el WebSocket de prod y el tiempo real se quedaba mudo sin un solo error.
#
# El caso que costó una pantalla entera: la de conexiones con terceros leía así el
# token de operador, lo veía vacío, concluía que esa superficie no gestionaba las
# conexiones y ofrecía un enlace… a sí misma. Un bucle sin salida.
set -euo pipefail
cd "$(dirname "$0")/.."

raiz="apps/frontend"
[ -d "$raiz" ] || { echo "no encuentro $raiz"; exit 1; }

# LAS APPS ESTÁTICAS QUEDAN FUERA, y no es una excepción de conveniencia: en una
# app `output: 'static'` no hay proceso servidor que pueda tener `process.env`, así
# que el env del build es la ÚNICA fuente posible y leer `PUBLIC_*` de ahí es lo
# correcto. Se detecta leyendo su `astro.config` en vez de listar nombres, para que
# una app nueva entre sola y para que el día que `www` pase a SSR el gate empiece a
# vigilarla sin que nadie se acuerde.
estaticas=""
for cfg in "$raiz"/apps/*/astro.config.*; do
  [ -f "$cfg" ] || continue
  if grep -qE "^[[:space:]]*output:[[:space:]]*'static'" "$cfg"; then
    estaticas="$estaticas $(basename "$(dirname "$cfg")")"
  fi
done

# Dos idiomas, que son los que se han dado de verdad:
#   1) `const env = import.meta.env;`  y luego `env.SYTUE_…`
#   2) `import.meta.env.SYTUE_…` / `(import.meta.env as …).SYTUE_…` directo
#
# El idioma 1 se ancla al FINAL de la expresión (`;` o un `as …;`) a propósito: sin
# el ancla, `const isDev = import.meta.env.DEV === true` también coincidía, y `DEV`,
# `MODE`, `PROD` y `SSR` sí son constantes del build —leerlas de ahí es su uso
# legítimo—.
#
# `grep -P` no existe en macOS, así que todo va con -E.
fallos=""

# Descarta una coincidencia si viene de una app estática.
de_app_estatica() {
  local ruta="${1%%:*}" app
  app="$(printf '%s\n' "$ruta" | sed -n "s|^$raiz/apps/\([^/]*\)/.*|\1|p")"
  [ -n "$app" ] || return 1
  case " $estaticas " in *" $app "*) return 0 ;; esac
  return 1
}

# --- idioma 1 -----------------------------------------------------------------
# Se acepta si la MISMA línea pasa por serverEnv.
while IFS= read -r linea; do
  [ -n "$linea" ] || continue
  case "$linea" in *serverEnv*) continue ;; esac
  de_app_estatica "$linea" && continue
  fallos="$fallos$linea"$'\n'
done <<EOF
$(grep -rnE 'const [A-Za-z_]+ = import\.meta\.env( as [^;]*)?;' "$raiz/apps" "$raiz/packages" \
    --include='*.ts' --include='*.tsx' --include='*.astro' 2>/dev/null || true)
EOF

# --- idioma 2 -----------------------------------------------------------------
# Lectura directa de una variable de la casa desde el env de build.
while IFS= read -r linea; do
  [ -n "$linea" ] || continue
  case "$linea" in *serverEnv*) continue ;; esac
  de_app_estatica "$linea" && continue
  fallos="$fallos$linea"$'\n'
done <<EOF
$(grep -rnE 'import\.meta\.env[^;]*\)?\.(PUBLIC_)?SYTUE_[A-Z_]+' "$raiz/apps" "$raiz/packages" \
    --include='*.ts' --include='*.tsx' --include='*.astro' 2>/dev/null || true)
EOF

if [ -n "$fallos" ]; then
  echo "FALLO: se lee el entorno del BUILD donde hace falta el de RUNTIME." >&2
  echo "$fallos" >&2
  echo "       Usa serverEnv(import.meta.env) de @sytue/auth/server: funde el env" >&2
  echo "       de build con process.env y el runtime gana. Sin eso la variable sale" >&2
  echo "       vacía en producción y gana el valor por defecto, que suele acertar" >&2
  echo "       en prod por casualidad y apuntar a prod desde staging." >&2
  exit 1
fi
echo "OK: nadie lee variables SYTUE_* del entorno congelado del build."
