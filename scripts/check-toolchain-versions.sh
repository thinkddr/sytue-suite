#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

fail() {
  echo "toolchain: $*" >&2
  exit 1
}

# Los runners mínimos de CI no incluyen necesariamente ripgrep. Se usa cuando
# está disponible y se conserva un fallback portable para que este gate pueda
# ejecutarse antes de instalar cualquier toolchain.
search_q() {
  if command -v rg >/dev/null 2>&1; then
    rg -q -- "$@"
  else
    local pattern="$1"
    shift
    grep -ERq -- "$pattern" "$@"
  fi
}

search_n() {
  if command -v rg >/dev/null 2>&1; then
    rg -n -- "$@"
  else
    local pattern="$1"
    shift
    grep -ERn -- "$pattern" "$@"
  fi
}

# ¿La versión instalada vale? VALE LA FIJADA O CUALQUIERA POSTERIOR. Lo que el
# repositorio declara es un MÍNIMO, no una talla única: una herramienta más nueva
# construye lo mismo, y bloquear a quien va por delante solo invita a desinstalar
# para poder empujar.
#
# El SHA-256 de `versions.env` sigue siendo exacto, y no es contradicción: ahí se
# fija QUÉ BINARIO descarga el CI —un hash no admite «o superior»—. Esto es otra
# pregunta: si lo que ya hay en una máquina sirve.
#
# `sort -V` compara versiones de verdad: con `<` de shell, 1.10.0 sería menor que
# 1.9.0 porque compara texto.
version_suficiente() {  # <lo-que-hay> <lo-que-se-pide>
  [ "$1" = "$2" ] && return 0
  [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | head -1)" = "$2" ]
}

NODE_VERSION="$(tr -d '[:space:]' < .node-version)"
GO_VERSION="$(awk '$1 == "go" { print $2; exit }' go.mod)"
BUN_VERSION="$(jq -r '.packageManager | sub("^bun@"; "")' apps/frontend/package.json)"
# `engines.bun` es el SUELO y `packageManager` la versión exacta con
# la que se resolvió el lockfile. Se leía y no se comparaba con nada, así que las
# dos podían separarse en silencio: un `packageManager` por debajo de su propio
# mínimo declarado es una instalación que el repositorio dice no admitir.
BUN_ENGINE="$(jq -r '.engines.bun' apps/frontend/package.json)"
[ "$BUN_ENGINE" = ">=$BUN_VERSION" ] \
  || fail "engines.bun ('$BUN_ENGINE') debe ser '>=$BUN_VERSION', el de packageManager"
# ASTRO ES UN RANGO, no un número clavado: `>=X.Y.Z <MAJOR+1`. Misma forma que
# `engines.node` («>=26.8.1 <27») en las apps de móvil, y por lo mismo: una
# versión posterior de la misma serie construye igual y no hay motivo para
# obligar a tocar el catálogo cada vez que sale un parche. El techo NO sobra: sin
# él, un major de Astro entraría solo en la próxima resolución.
#
# La reproducibilidad no la da esto, la da `bun.lock`, que sí fija la versión
# exacta que se instaló —hoy 7.2.9— con su hash.
ASTRO_RANGO="$(jq -r '.catalog.astro' apps/frontend/package.json)"
[[ "$ASTRO_RANGO" =~ ^\>=([0-9]+)\.([0-9]+)\.([0-9]+)\ \<([0-9]+)$ ]] \
  || fail "catalog.astro debe ser un rango '>=X.Y.Z <MAYOR+1' (es '$ASTRO_RANGO')"
ASTRO_VERSION="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.${BASH_REMATCH[3]}"
[ "${BASH_REMATCH[4]}" = "$(( BASH_REMATCH[1] + 1 ))" ] \
  || fail "el techo de catalog.astro debe ser el major siguiente a ${BASH_REMATCH[1]} (es ${BASH_REMATCH[4]})"
NODE_TYPES_VERSION="$(jq -r '.catalog["@types/node"]' apps/frontend/package.json)"
LHCI_VERSION="$(jq -r '.catalog["@lhci/cli"]' apps/frontend/package.json)"

for version in "$NODE_VERSION" "$GO_VERSION" "$BUN_VERSION" "$ASTRO_VERSION" "$NODE_TYPES_VERSION" "$LHCI_VERSION"; do
  [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "versión estable inválida: $version"
done
# `packageManager` fija la versión con la que se generan los lockfiles —corepack
# no entiende rangos— y `engines` declara el MÍNIMO que sirve para trabajar.
#
# EN LOS TRES PAQUETES QUE FIJAN BUN, no solo en el del frontal: los lockfiles de
# `apps/mobile` son los que rompieron el CI el 2026-09-05 y su package.json no
# decía en ninguna parte qué versión hacía falta. Quien solo abre esa carpeta no
# tiene por qué ir a leer el del frontal para enterarse.
for paquete in $(git ls-files '*package.json' | grep -v node_modules); do
  pm="$(jq -r '.packageManager // empty' "$paquete")"
  case "$pm" in bun@*) ;; *) continue ;; esac
  fijada="${pm#bun@}"
  [ "$fijada" = "$BUN_VERSION" ] || fail "$paquete fija bun $fijada y el resto $BUN_VERSION"
  minimo="$(jq -r '.engines.bun // empty' "$paquete")"
  [ "$minimo" = ">=$BUN_VERSION" ] || fail "$paquete debe declarar engines.bun \">=$BUN_VERSION\" (tiene \"$minimo\")"
done

search_q '^go [0-9]+\.[0-9]+\.[0-9]+$' go.mod || fail "go.mod debe fijar la versión exacta de Go"
# Los workflows viven en `.github/workflows` (GitHub Actions desde el 2026-09-27),
# pero la toolchain NO la ponen las acciones de setup de GitHub —no hay
# `go-version:` ni `node-version:` que comprobar— sino `.github/actions/toolchain/
# accion.sh`, con la versión y el sha256 de `versions.env`, dentro de un contenedor
# fijado por digest. Es lo que hacía la imagen del runner de la forja propia.
#
# Lo que sí se conserva —y es lo que de verdad protegía— es que ningún workflow
# duplique una versión a mano ni la descargue por una referencia mutable: eso
# convierte una release reproducible en «lo que hubiera ese día». Sin excepciones:
# la CLI de Fyne, que fue la última, se fija en versions.env desde el 2026-09-23.
if search_n '^[[:space:]]+(go-version|node-version):' .github/workflows; then
  fail "los workflows no pueden duplicar versiones: las fija la imagen del runner"
fi
# El `^[^#]*` de delante no es adorno: sin él, el gate se dispara cuando un
# COMENTARIO menciona la referencia mutable —por ejemplo, uno que explique por qué
# no se usa—. Eso es un falso positivo que empuja a describir la regla con rodeos
# en vez de escribirla claro donde toca leerla.
mutables="$(search_n '^[^#]*(@latest|:latest|version:[[:space:]]*latest)' .github/workflows || true)"
if [ -n "$mutables" ]; then
  printf '%s\n' "$mutables" >&2
  fail "los workflows no pueden descargar herramientas mediante referencias mutables latest"
fi
# Y que la coherencia de la toolchain se compruebe en el CI, que es lo que
# sustituye a los `*-version-file` de las acciones de setup.
search_q 'make toolchain-check' .github/workflows || \
  fail "ningún workflow verifica la coherencia de la toolchain"

while IFS= read -r dockerfile; do
  if search_q '^FROM golang:' "$dockerfile" && \
    ! search_q "^FROM golang:${GO_VERSION}@sha256:[0-9a-f]{64} AS build$" "$dockerfile"; then
    fail "$dockerfile no fija Go ${GO_VERSION} por digest"
  fi
done < <(git ls-files 'services/**/Dockerfile*')

search_q "^FROM oven/bun:${BUN_VERSION}-alpine@sha256:[0-9a-f]{64} AS base$" \
  apps/frontend/Dockerfile || fail "el Dockerfile frontend no fija Bun ${BUN_VERSION} por digest"
search_q "FROM oven/bun:${BUN_VERSION}@sha256:[0-9a-f]{64} AS build" \
  services/cloud/internal/builder/detect.go || fail "Sytue DevX no genera builds con Bun ${BUN_VERSION}"
search_q "FROM golang:${GO_VERSION}@sha256:[0-9a-f]{64} AS build" \
  services/cloud/internal/builder/detect.go || fail "Sytue DevX no genera builds con Go ${GO_VERSION}"

while IFS= read -r package; do
  bad="$(jq -r '
    ["astro", "@astrojs/check", "@astrojs/node", "@astrojs/preact", "@astrojs/sitemap",
     "eslint-plugin-astro", "prettier-plugin-astro", "@types/node", "@lhci/cli"] as $central
    | ((.dependencies // {}) + (.devDependencies // {})) as $deps
    | [$central[] as $name | ($deps[$name]? // empty)]
    | map(select(. != "catalog:")) | .[]
  ' "$package")"
  [ -z "$bad" ] || fail "$package elude el catálogo central: $bad"
done < <({ printf '%s\n' apps/frontend/package.json; find apps/frontend/apps apps/frontend/packages -name package.json -type f; } | sort)

search_q 'apps/frontend/node_modules/.bin/lhci' .github/workflows/ci-frontend.yml || \
  fail "Lighthouse CI debe ejecutarse desde el lockfile (@lhci/cli ${LHCI_VERSION})"

for package in apps/mobile/suite/package.json apps/mobile/rooms/package.json; do
  [ "$(jq -r '.engines.node' "$package")" = ">=${NODE_VERSION} <27" ] || \
    fail "$package no exige Node ${NODE_VERSION}"
  [ "$(jq -r '.devDependencies["@types/node"]' "$package")" = "$NODE_TYPES_VERSION" ] || \
    fail "$package no usa @types/node ${NODE_TYPES_VERSION}"
done

# --- Las DOS copias de la toolchain del runner ------------------------------
#
# `versions.env` y `runner/Containerfile` declaran lo mismo dos veces: el primero
# es lo que leen los scripts de la forja (el tag de la imagen sale de ahí), el
# segundo es lo que de verdad se descarga y se verifica al construirla. Nada las
# comparaba, y el 2026-08-14 la subida a Go 1.26.6 dejó el número nuevo junto al
# sha256 VIEJO en los dos ficheros. Eso no rompe nada visible: la imagen ya estaba
# publicada, así que el fallo espera a la próxima reconstrucción y sale como un
# `sha256sum -c` en mitad de un `podman build`, lejos del commit que lo causó.
#
# Se comparan los pares, no solo Go: el mismo descuido cabe en Node, Bun o cosign.
env_val() { awk -F= -v k="$1" '$1 == k { print $2; exit }' deploy/sytue-git/versions.env; }
arg_val() { awk -F= -v k="ARG $1" '$1 == k { print $2; exit }' deploy/sytue-git/runner/Containerfile; }

# Y LAS ACCIONES DE GITHUB, FIJADAS POR SHA DE COMMIT y con el SHA que declara
# versions.env: `actions/checkout@v7` se mueve sola y cambia lo que ejecuta el
# CI sin que cambie el repositorio, que es la misma regla que las imágenes por
# digest. Una acción que no esté prevista aquí se añade con su variable, no se
# cuela con una etiqueta.
comprobar_usos_workflows() {
  local directorio="$1" flujo linea accion var sha destino
  for flujo in "$directorio"/.github/workflows/*.yml "$directorio"/.github/workflows/*.yaml; do
    [ -f "$flujo" ] || continue
    while IFS= read -r linea; do
      # La clave es la inicial: un «uses:» dentro del comentario no puede
      # convertir una acción remota en una llamada local aparentemente válida.
      accion="$(sed -E 's/[[:space:]]+#.*$//; s/^[[:space:]]*(- )?uses:[[:space:]]*//; s/[[:space:]]+$//' <<<"$linea")"
      case "$accion" in
        \"*\"|\'*\') accion="${accion:1:${#accion}-2}" ;;
      esac
      if [[ "$accion" == ./* ]]; then
        # Un workflow local usa el mismo commit que su padre, sin @ref. No es
        # una acción remota ni una excepción a los SHA fijados en versions.env.
        [[ "$accion" =~ ^\./\.github/workflows/[A-Za-z0-9][A-Za-z0-9._-]*\.ya?ml$ ]] || \
          fail "ruta de workflow local no permitida: $accion"
        destino="$directorio/${accion#./}"
        [ -f "$destino" ] && [ ! -L "$destino" ] && \
          [ ! -L "$directorio/.github" ] && [ ! -L "$directorio/.github/workflows" ] || \
          fail "el workflow local no existe o es un enlace simbólico: $accion"
        git -C "$directorio" ls-files --error-unmatch -- "${accion#./}" >/dev/null 2>&1 || \
          fail "el workflow local no está versionado en este commit: $accion"
        # yamlq es el parser compartido: rechaza YAML inválido y claves dobles.
        # GitHub acepta workflow_call como evento escalar, lista o mapa.
        if ! go run "$ROOT/tools/yamlq" "$destino" | jq -se '
          length == 1 and (.[0].on |
            if type == "object" then has("workflow_call") and
              (.workflow_call | type == "null" or type == "object")
            elif type == "array" then all(.[]; type == "string") and
              index("workflow_call") != null
            else . == "workflow_call" end)
        ' >/dev/null; then
          fail "el workflow local debe declarar un workflow_call válido: $accion"
        fi
        continue
      fi
      case "$accion" in
        actions/checkout@*) var=ACTION_CHECKOUT_SHA ;;
        actions/upload-artifact@*) var=ACTION_UPLOAD_ARTIFACT_SHA ;;
        actions/cache/restore@*|actions/cache/save@*) var=ACTION_CACHE_SHA ;;
        *) fail "acción no prevista en los workflows: $accion (fíjala en versions.env y aquí)" ;;
      esac
      sha="${accion##*@}"
      [[ "$sha" =~ ^[0-9a-f]{40}$ ]] || fail "$accion no va fijada por SHA de commit"
      [ "$sha" = "$(env_val "$var")" ] || \
        fail "$accion no coincide con $var=$(env_val "$var") de versions.env"
    done < <(grep -hE '^[[:space:]]*(- )?uses:' "$flujo")
  done
}
comprobar_usos_workflows "$ROOT"

[ "$(env_val GO_VERSION)" = "$GO_VERSION" ] || \
  fail "versions.env fija Go $(env_val GO_VERSION) y go.mod pide ${GO_VERSION}"

# `.node-version` es la CUARTA copia, y decide los `engines` de los package.json.
# Estaba en 26.5.1 mientras el runner instalaba 26.7.0: no rompía nada porque
# «>=26.5.1» se cumple con 26.7.0, pero es la misma divergencia silenciosa de
# siempre —dos números para lo mismo, sin nadie mirando— y el día que se separen de
# verdad el síntoma será un fallo de runtime en el runner, no un gate en rojo.
[ "$(env_val NODE_VERSION)" = "$NODE_VERSION" ] || \
  fail "versions.env fija Node $(env_val NODE_VERSION) y .node-version dice ${NODE_VERSION}"

while read -r clave arg; do
  [ -n "$clave" ] || continue
  esperado="$(env_val "$clave")"
  tiene="$(arg_val "$arg")"
  [ -n "$esperado" ] || fail "versions.env ya no declara $clave"
  [ -n "$tiene" ] || fail "el Containerfile del runner ya no declara ARG $arg"
  [ "$esperado" = "$tiene" ] || \
    fail "$clave y ARG $arg divergen: versions.env dice '$esperado' y el Containerfile '$tiene'"
done <<'PARES'
GO_LINUX_AMD64_SHA256 GO_SHA256
NODE_VERSION NODE_VERSION
NODE_LINUX_X64_SHA256 NODE_SHA256
BUN_VERSION BUN_VERSION
BUN_LINUX_X64_SHA256 BUN_SHA256
COSIGN_VERSION COSIGN_VERSION
COSIGN_LINUX_AMD64_SHA256 COSIGN_SHA256
SYFT_VERSION SYFT_VERSION
SYFT_LINUX_AMD64_SHA256 SYFT_SHA256
HELM_VERSION HELM_VERSION
HELM_SHA256 HELM_SHA256
CRANE_VERSION CRANE_VERSION
GOLANGCI_LINT_VERSION GOLANGCI_LINT_VERSION
GOVULNCHECK_VERSION GOVULNCHECK_VERSION
TRIVY_VERSION TRIVY_VERSION
TRIVY_LINUX_AMD64_SHA256 TRIVY_SHA256
OSV_SCANNER_VERSION OSV_SCANNER_VERSION
OSV_SCANNER_SHA256 OSV_SCANNER_SHA256
PARES

# `docker` no identifica una CPU concreta: git.gay puede asignar el mismo job a
# cualquiera de los dos workers ARM64 propios o al runner x86. Las descargas de
# la acción compartida han de tener checksum para ambas arquitecturas; si falta
# uno el error aparece tarde, solo cuando la cola elige ese runner.
while read -r clave; do
  [ -n "$clave" ] || continue
  valor="$(env_val "$clave")"
  [[ "$valor" =~ ^[0-9a-f]{64}$ ]] || \
    fail "versions.env debe declarar un SHA-256 válido para $clave"
done <<'SHAS_MULTIPLATAFORMA'
GO_LINUX_AMD64_SHA256
GO_LINUX_ARM64_SHA256
BUN_LINUX_AMD64_SHA256
BUN_LINUX_ARM64_SHA256
BUN_DARWIN_AMD64_SHA256
BUN_DARWIN_ARM64_SHA256
COSIGN_LINUX_AMD64_SHA256
COSIGN_LINUX_ARM64_SHA256
SYFT_LINUX_AMD64_SHA256
SYFT_LINUX_ARM64_SHA256
TRIVY_LINUX_AMD64_SHA256
TRIVY_LINUX_ARM64_SHA256
HELM_LINUX_AMD64_SHA256
HELM_LINUX_ARM64_SHA256
OSV_SCANNER_LINUX_AMD64_SHA256
OSV_SCANNER_LINUX_ARM64_SHA256
SHAS_MULTIPLATAFORMA

# --- Trivy y osv-scanner: sus copias viven en el WORKFLOW -------------------
#
# Estas dos no están en `versions.env`: cada job de `security.yml` fija su propia
# versión y su propio sha256 justo antes de decidir si usa la de la imagen del
# runner o se la descarga. Trivy aparece CUATRO veces. Nadie comparaba esas copias
# con el `ARG` del Containerfile.
#
# Qué pasa cuando se separan, y no es un fallo ruidoso: el `command -v` comprueba
# que la versión de la imagen coincida con la del job, así que al no coincidir se
# descarga la otra. Todo sigue verde, cada job se baja un binario de internet en
# cada ejecución, y la herramienta que escanea deja de ser la que se auditó al
# construir la imagen.
#
# Se asocia cada número a su herramienta por la URL de descarga, no por cercanía:
# `VER` es un nombre genérico y en el mismo fichero lo usa más de una.
comprobar_workflow() {
  local nombre="$1" arg="$2" patron="$3" esperado observadas
  esperado="$(arg_val "$arg")"
  [ -n "$esperado" ] || fail "el Containerfile del runner ya no declara ARG $arg"
  observadas="$(awk -v pat="$patron" '
    /^[[:space:]]*(VER|OSV_VERSION)=/ { v = $0; sub(/.*=/, "", v); gsub(/[[:space:]]/, "", v) }
    $0 ~ pat && v != "" { print v }
  ' .github/workflows/security.yml | sort -u)"
  [ -n "$observadas" ] || fail "security.yml ya no descarga $nombre"
  [ "$observadas" = "$esperado" ] || \
    fail "$nombre: el Containerfile fija $esperado y security.yml usa $(echo "$observadas" | tr '\n' ' ')"
}
# Trivy se instala desde la acción común: así las cuatro familias de jobs no
# pueden volver a elegir cada una un tarball amd64 en un runner ARM64. La versión
# del Containerfile se conserva como referencia de la imagen propia retirada.
[ "$(env_val TRIVY_VERSION)" = "$(arg_val TRIVY_VERSION)" ] || \
  fail "TRIVY_VERSION diverge entre versions.env y el Containerfile"
search_q 'accion\.sh.*trivy' .github/workflows/security.yml || \
  fail "security.yml no instala Trivy mediante la acción multiplataforma"
# osv-scanner YA NO SE COMPARA ASÍ: desde el 2026-09-05 su versión no se copia
# dentro del workflow, se lee de `versions.env` con un `awk`, así que el par
# OSV_SCANNER_VERSION/SHA256 lo cubre la comparación de arriba —la de
# versions.env contra el Containerfile— y esta miraría una expresión de shell.
# Lo que sí se comprueba es que el workflow siga LEYÉNDOLO de ahí y no vuelva a
# escribir el número a mano.
search_q 'OSV_SCANNER_VERSION.*versions\.env' .github/workflows/security.yml \
  || fail "security.yml ya no lee la versión de osv-scanner de versions.env"
grep -qE '^[[:space:]]*OSV_VERSION=[0-9]' .github/workflows/security.yml \
  && fail "security.yml vuelve a fijar a mano la versión de osv-scanner"

# --- Y la TERCERA copia: la del puesto que despliega ------------------------
#
# `deploy/production/lib-tooling.sh` exige su propia versión de cosign y no la
# comparaba nadie con las otras dos. El día que alguien suba cosign en
# `versions.env` y en el Containerfile y se olvide de esta, el CI seguirá VERDE:
# el fallo espera al `00-preflight.sh` de un despliegue y sale como «cosign
# v3.1.4 no coincide con la versión segura fijada (v3.1.3)», con el despliegue
# detenido. Es el mismo patrón que el sha256 viejo de Go: el descuido no falla
# donde se comete, sino lejos y más tarde.
#
# Lleva prefijo `v` y las otras dos no, así que se normaliza antes de comparar:
# sin eso, comparar 'v3.1.3' con '3.1.3' daría un falso positivo permanente y el
# gate acabaría desactivado por ruidoso.
requerida="$(awk -F'"' '/^readonly SYTUE_REQUIRED_COSIGN_VERSION=/ { print $2; exit }' deploy/production/lib-tooling.sh)"
[ -n "$requerida" ] || \
  fail "lib-tooling.sh ya no declara SYTUE_REQUIRED_COSIGN_VERSION"
[ "${requerida#v}" = "$(env_val COSIGN_VERSION)" ] || \
  fail "lib-tooling.sh exige cosign ${requerida} y versions.env fija $(env_val COSIGN_VERSION)"

# --- Y el Go que de verdad se está ejecutando -------------------------------
#
# Todo lo de arriba compara FICHEROS entre sí, así que puede decir «OK» mientras
# el `go` instalado es otro. Es lo que pasó el 2026-08-14: el toolchain subió a
# 1.26.6 por seis CVE de la biblioteca estándar —go.mod, Containerfile del runner
# y versions.env, los tres a la vez— pero la imagen del runner no se reconstruyó.
# Con `GOTOOLCHAIN=local` no hay rescate: los runners siguieron con 1.26.5 y se
# cayó TODO lo que compila Go —ci-go, ci-integration, el lighthouse del frontend
# y cuatro jobs de security, incluido govulncheck, que dejó de analizar nada
# justo después del commit que entró para tapar CVE—. Siete jobs en rojo, la
# misma línea enterrada en cada log, y este gate diciendo «toolchain OK».
#
# Se pide la fijada O UNA POSTERIOR: quien va por delante compila lo mismo, y una
# versión más nueva no es un problema que haya que resolver desinstalando. Lo que
# no vale es ir por detrás, que es cuando faltan características del lenguaje.
#
# En CI es un fallo, para que el mensaje salga UNA vez y diga qué hacer. En local
# solo avisa: quien tenga otra versión sigue pudiendo empujar, porque su Go no es
# el que construye nada.
go_real="$(go version 2>/dev/null | awk '{print $3}' | sed 's/^go//')"
if [ -n "$go_real" ] && ! version_suficiente "$go_real" "$GO_VERSION"; then
  aviso="el 'go' instalado es ${go_real} y el repo exige ${GO_VERSION} o superior"
  if [ -n "${CI:-}${GITHUB_ACTIONS:-}" ]; then
    echo "toolchain: ${aviso}." >&2
    echo "  La imagen del runner se quedó atrás. Reconstruirla y publicarla:" >&2
    echo "    SCW_REGISTRY_PUSH_PASSWORD=… deploy/sytue-git/bin/build-runner-image.sh" >&2
    exit 1
  fi
  echo "! toolchain: ${aviso} (aviso local; en CI esto rompe el build)" >&2
fi

# LO MISMO CON BUN, y por un motivo distinto al de Go: no es que compile peor, es
# que REESCRIBE los lockfiles. Bun 1.4 cambió el formato —`lockfileVersion` 1 → 3,
# con los overrides anidados dentro— así que un `bun install` con 1.3 devuelve los
# `bun.lock` al formato viejo, y el CI, que corre la versión fijada, los rechaza
# con «lockfile had changes, but lockfile is frozen». Pasó el 2026-09-05 con los
# dos lockfiles de móvil y costó un job de seguridad entero en rojo.
#
# La fijada o una POSTERIOR, igual que Go: bun 1.4.3 escribe el mismo formato que
# 1.4.2 y no hay motivo para frenar a quien lo tenga. Ir por DEBAJO sí rompe.
#
# Mismo reparto que arriba: en CI rompe, en local avisa. Lo que SÍ se bloquea en
# local es empujar un `bun.lock` escrito por una versión anterior a la fijada, y
# eso lo hace el hook de pre-push, que es donde se sabe qué lleva el empujón.
bun_real="$(bun --version 2>/dev/null | tr -d '[:space:]')"
if [ -n "$bun_real" ] && ! version_suficiente "$bun_real" "$BUN_VERSION"; then
  aviso="el 'bun' instalado es ${bun_real} y el repo exige ${BUN_VERSION} o superior"
  if [ -n "${CI:-}${GITHUB_ACTIONS:-}" ]; then
    echo "toolchain: ${aviso}." >&2
    echo "  Lo instala .github/actions/toolchain/accion.sh con el SHA de versions.env." >&2
    exit 1
  fi
  echo "! toolchain: ${aviso} (aviso local; NO regeneres lockfiles con esa versión)" >&2
fi

echo "toolchain OK: Node ${NODE_VERSION} o superior · Go ${GO_VERSION} o superior · Bun ${BUN_VERSION} o superior · Astro ${ASTRO_RANGO}"
