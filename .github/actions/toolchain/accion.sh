#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Deja en el runner las herramientas que su imagen NO trae.
#
#   .github/actions/toolchain/accion.sh go bun helm crane envsubst jq trivy docker
#
# POR QUÉ EXISTE. El CI se escribió contra una imagen de runner propia
# (deploy/sytue-git/runner/Containerfile) que traía Go, Bun y el resto ya
# dentro; por eso `ci-go.yml` fija `GOTOOLCHAIN=local` y falla a propósito si
# la versión no es la que pide `go.mod`. En git.gay el runner es COMPARTIDO y
# su imagen (Ubuntu 24.04) trae git, curl, jq, gcc, node y python3, pero NO Go
# ni Bun. Sin esto, todo job de Go muere en la primera orden con «command not
# found» y el gate de toolchain no llega ni a ejecutarse.
#
# Lo mismo pasaba, sin que nadie lo hubiera atado, con TRES herramientas más que
# la imagen propia sí traía (ver deploy/sytue-git/runner/Containerfile) y que
# ningún job instalaba: `helm`, `crane` y el módulo `yaml` de Python. El síntoma
# no era un «command not found» claro sino cuatro comprobaciones en rojo —
# alertas-test, helm-render-test, helm-so-test y
# release-workflow-test— y el espejo del catálogo abortando con «crane no está
# en la imagen del runner; reconstruye con build-runner-image.sh», que manda
# reconstruir una imagen de un runner que ya no existe.
#
# LA VERSIÓN Y EL SHA SON LOS MISMOS que los de la imagen propia, y salen del
# MISMO fichero (`versions.env`): si se sube una versión ahí, sube en los dos
# sitios o el gate de coherencia lo canta. Duplicar el número aquí habría
# creado exactamente la deriva que ese gate existe para impedir.
#
# Se verifica el sha256 SIEMPRE, también en la descarga de origen. Es lo único
# que garantiza qué binario acaba ejecutando el CI, y es el requisito que pide
# el gate «Toda descarga de herramienta se verifica por SHA-256».
#
# Idempotente: si la herramienta ya está con la versión correcta no hace nada,
# así esto sigue siendo correcto el día que se vuelva a un runner propio con
# todo dentro de la imagen.
set -euo pipefail

RAIZ="${GITHUB_WORKSPACE:-$PWD}"
VERSIONES="$RAIZ/deploy/sytue-git/versions.env"
if [ ! -f "$VERSIONES" ]; then
  echo "✗ no encuentro $VERSIONES: sin él no hay versión fijada que instalar." >&2
  exit 1
fi
set -a
# shellcheck source=/dev/null
. "$VERSIONES"
set +a

# Se instala en /usr/local y se anuncia por $GITHUB_PATH: exportar PATH en un
# `run` NO llega al paso siguiente —costó los veinte trabajos de la #309—.
anunciar() {
  [ -n "${GITHUB_PATH:-}" ] && echo "$1" >> "$GITHUB_PATH"
  export PATH="$1:$PATH"
}

# Las imágenes oficiales de Debian aún pueden traer sus repositorios por HTTP.
# El pool de CI no admite depender del puerto 80: el 2026-10-01 el mirror
# respondió por DNS pero agotó la conexión y APT acabó diciendo, de forma poco
# útil, que no existían las cabeceras de escritorio. Cambiar SOLO los orígenes
# Debian a HTTPS mantiene el mismo repositorio y metadatos firmados; los tres
# intentos cubren un corte transitorio sin ocultar una caída persistente.
apt_actualizar() {
  local intento
  find /etc/apt -type f \( -name '*.list' -o -name '*.sources' \) -exec \
    sed -i 's|http://deb.debian.org|https://deb.debian.org|g' {} +
  for intento in 1 2 3; do
    if apt-get -o Acquire::Retries=3 -o Acquire::http::Timeout=30 -o Acquire::https::Timeout=30 update -qq; then
      return 0
    fi
    if [ "$intento" -lt 3 ]; then
      echo "apt-get update falló (intento $intento/3); se reintenta." >&2
      sleep "$((intento * 3))"
    fi
  done
  echo "✗ apt-get update falló tras tres intentos; no se instalan paquetes con índices obsoletos." >&2
  return 1
}

# The archive downloads need the same bounded retries as index refresh. APT
# still verifies the distribution metadata and returns persistent failures.
apt_instalar() {
  DEBIAN_FRONTEND=noninteractive apt-get -o Acquire::Retries=3 \
    -o Acquire::http::Timeout=30 -o Acquire::https::Timeout=30 install "$@"
}


# `sha256sum` es de coreutils y en macOS NO existe: allí la orden equivalente es
# `shasum -a 256`, que acepta el mismo formato «<sha>  <fichero>» y el mismo `-c`.
# Sin esto, todo `bajar_verificado` en un Mac muere con «command not found» DESPUÉS
# de haber descargado el fichero, y el error apunta a la comprobación en vez de a
# que falta la herramienta.
. "$RAIZ/.github/actions/toolchain/sha256.sh"

# La descarga en sí —reintentos, suelo de velocidad y reanudación, con el porqué
# de cada número— es la misma que usa `bajar-herramienta.sh` y vive en un sitio.
# shellcheck source=/dev/null
. "$RAIZ/deploy/sytue-git/bin/descarga-reanudable.sh"

# CADA HERRAMIENTA SE BAJA UNA VEZ POR RUNNER, NO UNA VEZ POR JOB. Los runners
# montan una caché del host (`/var/cache/sytue-ci/descargas`, ver .github/RUNNERS.md)
# y aquí se guarda cada descarga con su sha256 por nombre. El 2026-11-08 `quality`
# murió sin ejecutar nada: Bun desde GitHub a menos de 100 KB/s, tres tandas de
# reanudación y código 28, igual que con Go el 2026-10-05. Con la caché, un origen
# lento solo cuesta la primera vez. El sha se comprueba SIEMPRE, también al sacar de
# la caché: una entrada corrupta se tira y se vuelve a bajar. Sin el montaje (el Mac,
# Xcode Cloud) todo sigue como antes.
DESCARGAS="${SYTUE_CACHE_DESCARGAS:-/github/home/.cache/sytue-descargas}"
bajar_verificado() {  # <url> <sha256> <destino>
  local cache="${DESCARGAS:-}" guardada
  if [ -z "$cache" ] || [ ! -d "$cache" ]; then
    descargar_reanudable "$3" "$1" || return
    comprobar_sha256 "$2" "$3"
    return
  fi
  guardada="$cache/$2"
  if [ -s "$guardada" ] && cp "$guardada" "$3" && comprobar_sha256 "$2" "$3"; then
    echo "  ${1##*/}: de la caché del runner, sin descargar"
    return 0
  fi
  [ ! -e "$guardada" ] || rm -f "$guardada"
  descargar_reanudable "$3" "$1" || return
  comprobar_sha256 "$2" "$3" || return
  if [ -w "$cache" ]; then
    # Primero a un temporal y luego `mv`: dos jobs a la vez no dejan media entrada.
    cp "$3" "$guardada.$$" && mv -f "$guardada.$$" "$guardada" || rm -f "$guardada.$$"
  fi
  return 0
}

# «<os> <arch>» con los nombres que usan go.dev y sigstore, que por suerte
# coinciden. Está aparte porque lo necesitan Go y cosign, y una segunda copia de
# un `case` sobre `uname` es una segunda copia que se queda atrás.
os_arch() {
  local os arch
  case "$(uname -s)" in
    Linux)  os=linux ;;
    Darwin) os=darwin ;;
    *) echo "✗ sistema no soportado: $(uname -s)" >&2; return 1 ;;
  esac
  case "$(uname -m)" in
    x86_64|amd64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *) echo "✗ arquitectura no soportada: $(uname -m)" >&2; return 1 ;;
  esac
  printf '%s %s' "$os" "$arch"
}

# El sha de versions.env para <PREFIJO>_<OS>_<ARCH>_SHA256. Componer el nombre en
# vez de listarlos hace que añadir una plataforma sea una línea allí y ninguna
# aquí.
sha_de() {  # <PREFIJO> <os> <arch>
  local var sha
  var="$(printf '%s_%s_%s_SHA256' "$1" "$2" "$3" | tr '[:lower:]' '[:upper:]')"
  sha="${!var:-}"
  [ -n "$sha" ] || {
    echo "✗ versions.env no declara $var: sin sha no se instala en $2/$3." >&2
    return 1
  }
  printf '%s' "$sha"
}

tarball_go() {  # imprime «<fichero> <sha256>»
  local v="$1" os arch sha
  read -r os arch <<<"$(os_arch)"
  sha="$(sha_de GO "$os" "$arch")" || return 1
  printf 'go%s.%s-%s.tar.gz %s' "$v" "$os" "$arch" "$sha"
}

tarball_bun() {  # imprime «<fichero> <directorio> <sha256>»
  local os arch nombre_arch sha
  read -r os arch <<<"$(os_arch)"
  case "$arch" in
    amd64) nombre_arch=x64 ;;
    arm64) nombre_arch=aarch64 ;;
  esac
  sha="$(sha_de BUN "$os" "$arch")" || return 1
  printf 'bun-%s-%s.zip bun-%s-%s %s' "$os" "$nombre_arch" "$os" "$nombre_arch" "$sha"
}

# cosign. Lo trae la imagen del runner propio (deploy/sytue-git/runner/
# Containerfile) y por eso ningún job lo instalaba; la VM de macOS de Codemagic
# no, y el build de v2.13.19 compiló los tres bundles antes de morir al publicar
# con «cosign: command not found» —exit 127—.
#
# Misma versión y misma verificación por sha256 que en la imagen: lo que firma
# los instaladores que se sirven al público no puede salir de una descarga sin
# comprobar.
instalar_cosign() {
  local quiere="${COSIGN_VERSION:?falta COSIGN_VERSION en versions.env}"
  if command -v cosign >/dev/null 2>&1 &&
     grep -q "$quiere" <<<"$(cosign version 2>/dev/null)"; then
    echo "cosign $quiere ya está en el runner: no se descarga"
    return 0
  fi
  local os arch sha destino
  read -r os arch <<<"$(os_arch)"
  sha="$(sha_de COSIGN "$os" "$arch")" || return 1
  echo "instalando cosign $quiere (cosign-${os}-${arch}; la imagen del runner no lo trae)"
  bajar_verificado \
    "https://github.com/sigstore/cosign/releases/download/v${quiere}/cosign-${os}-${arch}" \
    "$sha" /tmp/cosign
  # Igual que Go: donde se pueda escribir sin privilegios. En macOS /usr/local es
  # de root.
  if [ -w /usr/local/bin ]; then destino=/usr/local/bin; else destino="$HOME/bin"; fi
  # `mkdir -p` + `install` SIN `-D`, que es un flag de GNU coreutils y en macOS no
  # existe: el `install` de BSD lo rechaza escupiendo su modo de empleo, y el
  # binario se queda sin copiar. Las demás herramientas de este fichero usan
  # `install -D` y siguen bien porque solo se instalan en el runner de Linux;
  # cosign es la primera que corre también en un Mac.
  mkdir -p "$destino"
  install -m 0755 /tmp/cosign "$destino/cosign"
  rm -f /tmp/cosign
  anunciar "$destino"
  cosign version 2>/dev/null | grep -i gitversion || true
}

# El tarball de Go depende de la plataforma, y desde que los clientes de macOS se
# construyen en Codemagic esto ya no corre solo en Linux.
#
# NO se usa Homebrew, que sería una línea: `brew install go` trae la versión que a
# Homebrew le parezca, sin sha fijado y sin coincidir con la de `versions.env`. El
# gate `toolchain-check` existe precisamente para que esa versión no derive, y el
# CI la verifica por sha256 en todas partes: que el build de macOS salga de otro
# proveedor no es razón para bajar el listón.
instalar_go() {
  local quiere="${GO_VERSION:?falta GO_VERSION en versions.env}"
  if command -v go >/dev/null 2>&1 && [ "$(go env GOVERSION 2>/dev/null)" = "go$quiere" ]; then
    echo "go$quiere ya está en el runner: no se descarga"
    return 0
  fi
  local fichero sha datos_tarball
  datos_tarball="$(tarball_go "$quiere")" || return 1
  read -r fichero sha <<<"$datos_tarball"
  echo "instalando Go $quiere ($fichero; la imagen del runner no lo trae)"
  bajar_verificado "https://go.dev/dl/${fichero}" "$sha" /tmp/go.tar.gz
  # DONDE SE PUEDA ESCRIBIR SIN PRIVILEGIOS, y no siempre en /usr/local.
  #
  # En el runner de Linux el job corre como root y /usr/local es suyo, así que
  # sigue yendo ahí. En una VM de macOS NO: /usr/local es de root:wheel —
  # comprobado en un Mac ARM, `drwxr-xr-x root wheel`— y ahí instalar exigiría
  # `sudo`. Que el CI de un tercero lo conceda sin contraseña es una suposición,
  # y cada suposición equivocada cuesta un build entero del cupo mensual de
  # Codemagic. $HOME siempre es escribible y no necesita permisos especiales.
  #
  # `codemagic.yaml` pone las DOS rutas en el PATH por eso, sin tener que saber
  # cuál se usó.
  local base
  if [ -w /usr/local ]; then base=/usr/local; else base="$HOME/.sytue-toolchain"; fi
  mkdir -p "$base"
  rm -rf "$base/go"
  tar -C "$base" -xzf /tmp/go.tar.gz
  rm -f /tmp/go.tar.gz
  anunciar "$base/go/bin"
  go version
}

# LA CACHÉ DE COMPILACIÓN COMPARTIDA SOLO SIRVE SI ES NUESTRA (2026-10-02).
# En el frontal `go-build` se monta en todos los jobs y el contenedor escribe como
# sytue-runner. El 2026-10-01 algo con root de verdad compiló contra ella y dejó
# 3330 entradas `root 644`. Go reescribe el índice `-a` de una entrada cada vez
# que la vuelve a guardar, y con una ajena el `open` da «permission denied»:
# `go build` y `go test` lo toleran, pero `go list -export` sale con 1 y la salida
# vacía, go/packages se lo traga en modo export y golangci-lint acaba diciendo
# «no go files to analyze: running `go mod tidy` may solve the problem», que no
# tiene nada que ver. Aquí solo se AVISA, con el arreglo: devolverlas al dueño se
# hace en el host, que es donde están los permisos.
avisar_cache_go_ajena() {
  local cache ajenas
  cache="$(go env GOCACHE 2>/dev/null)"
  [ -d "$cache" ] || return 0
  ajenas="$(find "$cache" -type f ! -user "$(id -u)" 2>/dev/null | wc -l)" || true
  [ "$((ajenas + 0))" -eq 0 ] ||
    echo "::warning::caché de Go con $((ajenas)) entradas de otro usuario: golangci-lint dirá «no go files to analyze». En el host: chown -R sytue-runner: /var/cache/sytue-ci"
}

instalar_bun() {
  local quiere="${BUN_VERSION:?falta BUN_VERSION en versions.env}"
  if command -v bun >/dev/null 2>&1 && [ "$(bun --version 2>/dev/null)" = "$quiere" ]; then
    echo "bun $quiere ya está en el runner: no se descarga"
    return 0
  fi
  local fichero directorio sha datos_tarball destino
  datos_tarball="$(tarball_bun)" || return 1
  read -r fichero directorio sha <<<"$datos_tarball"
  echo "instalando Bun $quiere ($fichero; la imagen del runner no lo trae)"
  bajar_verificado \
    "https://github.com/oven-sh/bun/releases/download/bun-v${quiere}/${fichero}" \
    "$sha" \
    /tmp/bun.zip
  rm -rf /tmp/bun
  unzip -q /tmp/bun.zip -d /tmp/bun
  if [ -w /usr/local/bin ]; then destino=/usr/local/bin; else destino="$HOME/bin"; fi
  mkdir -p "$destino"
  install -m 0755 "/tmp/bun/$directorio/bun" "$destino/bun"
  ln -sf "$destino/bun" "$destino/bunx"
  rm -rf /tmp/bun /tmp/bun.zip
  anunciar "$destino"
  bun --version
}

# govulncheck y syft se daban por venidos en la imagen del runner. La que usa
# este runner es git.gay/gitgay/docker_images:24.04 y no los trae, así que los
# jobs morían con «command not found» DESPUÉS de haber hecho su trabajo: el de
# osv-scanner llegó a imprimir «0 vulnerabilities» y aun así salió con 127.
instalar_govulncheck() {
  local quiere="${GOVULNCHECK_VERSION:?falta GOVULNCHECK_VERSION en versions.env}"
  local actual
  if command -v govulncheck >/dev/null 2>&1 &&
    actual="$(govulncheck -version 2>/dev/null)" &&
    grep -Fxq "Scanner: govulncheck@v${quiere}" <<<"$actual"; then
    echo "govulncheck $quiere ya está en el runner: no se instala"
    return 0
  fi
  command -v go >/dev/null 2>&1 || instalar_go
  echo "instalando govulncheck $quiere (la imagen del runner no lo trae)"
  # /usr/local/bin y NO /opt/gotools/bin: ese está en el PATH de NUESTRA imagen de
  # runner, no en la de esta. Instalar ahí es instalar donde nadie lo encuentra.
  GOBIN=/usr/local/bin go install "golang.org/x/vuln/cmd/govulncheck@v${quiere}"
  anunciar /usr/local/bin
  # SE COMPRUEBA QUE HAYA QUEDADO LLAMABLE. `go install` puede terminar bien y
  # dejar el binario donde el PATH no lo ve —GOBIN heredado del entorno, GOFLAGS,
  # una imagen con otro PATH—: entonces el fallo sale tres pasos más adelante
  # como un `exitcode 127` sin nombre, que es lo que pasó el 2026-09-05. Aquí se
  # ve en el sitio donde está la causa. `-version` además dice con qué Go se
  # compiló, que es lo que decide si podrá analizar estas fuentes.
  command -v govulncheck >/dev/null 2>&1 || {
    echo "✗ govulncheck se instaló pero no está en el PATH." >&2
    echo "  GOBIN=$(go env GOBIN) GOPATH=$(go env GOPATH) PATH=$PATH" >&2
    exit 1
  }
  # `sed -n` y NO `| head -3`: head cierra la tubería al leer su tercera línea,
  # el proceso de origen recibe SIGPIPE y, con `set -o pipefail`, eso tumba la
  # instalación entera. Pasó: el job salió con 141 —128+13, SIGPIPE— justo
  # después de imprimir la versión. sed lee hasta el final.
  actual="$(govulncheck -version 2>&1)"
  grep -Fxq "Scanner: govulncheck@v${quiere}" <<<"$actual" || {
    echo "✗ govulncheck instalado no coincide con $quiere." >&2
    return 1
  }
  sed -n '1,3p' <<<"$actual"
}

instalar_go_licenses() {
  local quiere="${GO_LICENSES_VERSION:?falta GO_LICENSES_VERSION en versions.env}"
  command -v go >/dev/null 2>&1 || instalar_go
  GOBIN=/usr/local/bin go install "github.com/google/go-licenses/v2@v${quiere}"
  anunciar /usr/local/bin
  local version
  version="$(go version -m /usr/local/bin/go-licenses |
    awk '$1 == "mod" && $2 == "github.com/google/go-licenses/v2" { print $3 }')"
  [ "$version" = "v$quiere" ] || {
    echo "✗ go-licenses instalado no coincide con $quiere." >&2
    return 1
  }
}

instalar_syft() {
  local quiere="${SYFT_VERSION:?falta SYFT_VERSION en versions.env}"
  if command -v syft >/dev/null 2>&1 && grep -q "$quiere" <<<"$(syft version 2>/dev/null)"; then
    echo "syft $quiere ya está en el runner: no se descarga"
    return 0
  fi
  local os arch sha
  read -r os arch <<<"$(os_arch)"
  [ "$os" = linux ] || { echo "✗ syft solo se instala desde esta acción en Linux." >&2; return 1; }
  sha="$(sha_de SYFT "$os" "$arch")" || return 1
  echo "instalando syft $quiere (linux-$arch; la imagen del runner no lo trae)"
  bajar_verificado \
    "https://github.com/anchore/syft/releases/download/v${quiere}/syft_${quiere}_${os}_${arch}.tar.gz" \
    "$sha" \
    /tmp/syft.tar.gz
  tar -C /tmp -xzf /tmp/syft.tar.gz syft
  install -D -m 0755 /tmp/syft /usr/local/bin/syft
  rm -f /tmp/syft.tar.gz /tmp/syft
  syft version
}

instalar_helm() {
  local quiere="${HELM_VERSION:?falta HELM_VERSION en versions.env}"
  if command -v helm >/dev/null 2>&1 && grep -q "^$quiere" <<<"$(helm version --short 2>/dev/null)"; then
    echo "helm $quiere ya está en el runner: no se descarga"
    return 0
  fi
  local os arch sha
  read -r os arch <<<"$(os_arch)"
  [ "$os" = linux ] || { echo "✗ Helm solo se instala desde esta acción en Linux." >&2; return 1; }
  sha="$(sha_de HELM "$os" "$arch")" || return 1
  echo "instalando Helm $quiere (linux-$arch; la imagen del runner no lo trae)"
  bajar_verificado \
    "https://get.helm.sh/helm-${quiere}-${os}-${arch}.tar.gz" \
    "$sha" \
    /tmp/helm.tar.gz
  tar -C /tmp -xzf /tmp/helm.tar.gz
  install -D -m 0755 "/tmp/${os}-${arch}/helm" /usr/local/bin/helm
  rm -rf /tmp/helm.tar.gz "/tmp/${os}-${arch}"
  helm version --short
}

# envsubst, del paquete gettext-base. No es una herramienta que se descargue con
# su SHA: es un binario de la distribución, y va por apt igual que iba PyYAML.
# La imagen genérica de git.gay no lo trae y el gate del piloto lo necesita para
# renderizar su values.
instalar_envsubst() {
  if command -v envsubst >/dev/null 2>&1; then
    echo "envsubst ya está en el runner: no se instala"
    return 0
  fi
  echo "instalando gettext-base (la imagen del runner no trae envsubst)"
  apt_actualizar
  apt_instalar -y -qq --no-install-recommends gettext-base
  envsubst --version | sed -n '1p'
}

# jq lo necesitan los gates de despliegue y de App Store. El de Debian 12 es 1.6:
# se usa la versión estable fijada en versions.env y el binario oficial verificado,
# también si la imagen trae uno anterior. Los nombres de macOS son «macos», no darwin.
instalar_jq() {
  local quiere="${JQ_VERSION:?falta JQ_VERSION en versions.env}"
  if command -v jq >/dev/null 2>&1 && [ "$(jq --version 2>/dev/null)" = "jq-$quiere" ]; then
    echo "jq $quiere ya está en el runner: no se descarga"
    return 0
  fi
  local os arch sha plataforma destino
  read -r os arch <<<"$(os_arch)"
  sha="$(sha_de JQ "$os" "$arch")" || return 1
  case "$os" in linux) plataforma=linux ;; darwin) plataforma=macos ;; esac
  echo "instalando jq $quiere ($plataforma-$arch)"
  bajar_verificado \
    "https://github.com/jqlang/jq/releases/download/jq-${quiere}/jq-${plataforma}-${arch}" \
    "$sha" /tmp/jq
  if [ -w /usr/local/bin ]; then destino=/usr/local/bin; else destino="$HOME/bin"; fi
  mkdir -p "$destino"
  install -m 0755 /tmp/jq "$destino/jq"
  rm -f /tmp/jq
  anunciar "$destino"
  [ "$(jq --version)" = "jq-$quiere" ] || { echo "✗ jq instalado no coincide con $quiere." >&2; return 1; }
  jq --version
}

# El JDK para FIRMAR el AAB de Android (jarsigner, keytool) y ejecutar bundletool. En
# el Mac de Codemagic ya viene; en el runner (node:lts, Debian 12) se toma de Debian.
instalar_jdk() {
  # macOS trae /usr/bin/java, javac y keytool aunque no haya ningún JDK: son avisos que
  # piden instalar uno. El que vale es el que encuentra `java_home`; sin él, se dice aquí
  # (en el Mac del runner no se instala nada por su cuenta, como Xcode).
  if [ "$(uname -s)" = Darwin ]; then
    local jdk
    jdk="$(/usr/libexec/java_home -v 17+ 2>/dev/null)" || {
      echo "✗ el Mac del runner no tiene un JDK 17 o posterior (/usr/libexec/java_home -v 17+)." >&2
      return 1
    }
    export JAVA_HOME="$jdk"
    [ -z "${GITHUB_ENV:-}" ] || printf 'JAVA_HOME=%s\n' "$jdk" >>"$GITHUB_ENV"
    anunciar "$jdk/bin"
    return 0
  fi
  if command -v jarsigner >/dev/null 2>&1 && command -v keytool >/dev/null 2>&1; then
    echo "el JDK ya está en el runner: no se instala"
    return 0
  fi
  echo "instalando el JDK (la imagen del runner no lo trae)"
  apt_actualizar
  apt_instalar -y -qq --no-install-recommends openjdk-17-jdk-headless
  jarsigner -help 2>&1 | head -1 || true
}

# bundletool es un .jar: se deja al lado un `bundletool` que lo llama con java.
instalar_bundletool() {
  local quiere="${BUNDLETOOL_VERSION:?falta BUNDLETOOL_VERSION en versions.env}" destino
  if command -v bundletool >/dev/null 2>&1 && [ "$(bundletool version 2>/dev/null)" = "$quiere" ]; then
    echo "bundletool $quiere ya está: no se descarga"
    return 0
  fi
  if [ -w /usr/local/bin ]; then destino=/usr/local/bin; else destino="$HOME/bin"; fi
  mkdir -p "$destino"
  bajar_verificado \
    "https://github.com/google/bundletool/releases/download/${quiere}/bundletool-all-${quiere}.jar" \
    "${BUNDLETOOL_SHA256:?falta BUNDLETOOL_SHA256 en versions.env}" "$destino/bundletool-$quiere.jar"
  printf '#!/bin/sh\nexec java -jar "%s" "$@"\n' "$destino/bundletool-$quiere.jar" >"$destino/bundletool"
  chmod 0755 "$destino/bundletool"
  anunciar "$destino"
  bundletool version
}

# EL SDK DE ANDROID EN UN RUNNER ARM64. Google no publica NDK ni build-tools para Linux
# ARM64, así que se instalan los de x86_64 y corren por qemu-user: el binfmt del frontal
# lleva la bandera F y sirve también dentro de los contenedores. Les hace falta el glibc
# de amd64 (multiarch); Go sigue siendo el nativo, solo el clang del NDK va emulado. Deja
# ANDROID_HOME y ANDROID_NDK_HOME en $GITHUB_ENV.
#
# En el Mac del runner (`android-emulador.yml`) son las de macOS, con su sha propio, y el
# SDK se queda en la caché del usuario del runner entre un job y otro.
instalar_android_sdk() {
  local sistema plataforma=linux sdk_defecto=/opt/android-sdk sha_ct="${ANDROID_CMDLINE_TOOLS_SHA256:-}"
  sistema="$(uname -s)"
  if [ "$sistema" = Darwin ]; then
    plataforma=mac sdk_defecto="$HOME/.cache/sytue-ci/android-sdk" sha_ct="${ANDROID_CMDLINE_TOOLS_MAC_SHA256:-}"
  fi
  local sdk="${ANDROID_HOME:-$sdk_defecto}" ct="${ANDROID_CMDLINE_TOOLS_VERSION:?falta ANDROID_CMDLINE_TOOLS_VERSION en versions.env}"
  local ndk="${ANDROID_NDK_VERSION:?falta ANDROID_NDK_VERSION en versions.env}" bt="${ANDROID_BUILD_TOOLS_VERSION:?falta ANDROID_BUILD_TOOLS_VERSION en versions.env}"
  local api zip
  api="$(sed -n 's/^ANDROID_TARGET_SDK=//p' "$RAIZ/apps/fyne/scripts/empaquetar.sh")"
  # Dentro de un contenedor binfmt_misc no está montado aunque el anfitrión lo tenga: lo
  # que vale es que el clang del NDK ARRANQUE, y eso se prueba abajo.
  if [ "$sistema" = Linux ] && [ "$(uname -m)" != x86_64 ]; then
    case " $(dpkg --print-foreign-architectures | tr '\n' ' ') " in *" amd64 "*) ;; *) dpkg --add-architecture amd64 ;; esac
    apt_actualizar
    apt_instalar -y -qq --no-install-recommends \
      libc6:amd64 libstdc++6:amd64 zlib1g:amd64 >/dev/null
  fi
  instalar_zip
  if [ ! -x "$sdk/cmdline-tools/latest/bin/sdkmanager" ]; then
    zip="$(mktemp -d)/cmdline-tools.zip"
    bajar_verificado "https://dl.google.com/android/repository/commandlinetools-${plataforma}-${ct}_latest.zip" \
      "${sha_ct:?falta el sha256 de las cmdline-tools de $plataforma en versions.env}" "$zip"
    mkdir -p "$sdk/cmdline-tools"
    unzip -q "$zip" -d "$sdk/cmdline-tools"
    rm -rf "$sdk/cmdline-tools/latest" && mv "$sdk/cmdline-tools/cmdline-tools" "$sdk/cmdline-tools/latest"
  fi
  # Aceptar las licencias es la condición de Google para bajar sus paquetes.
  yes | "$sdk/cmdline-tools/latest/bin/sdkmanager" --sdk_root="$sdk" --licenses >/dev/null || true
  "$sdk/cmdline-tools/latest/bin/sdkmanager" --sdk_root="$sdk" \
    "ndk;$ndk" "platforms;android-${api:?empaquetar.sh no fija ANDROID_TARGET_SDK}" "build-tools;$bt" >/dev/null
  local prebuilt=linux-x86_64 version
  [ "$sistema" != Darwin ] || prebuilt=darwin-x86_64
  local clang="$sdk/ndk/$ndk/toolchains/llvm/prebuilt/$prebuilt/bin/clang"
  version="$("$clang" --version 2>&1)" || {
    echo "✗ el clang x86_64 del NDK no arranca en $(uname -m): el anfitrión necesita qemu-user en binfmt con la bandera F. $version" >&2
    return 1
  }
  echo "${version%%$'\n'*}"
  export ANDROID_HOME="$sdk" ANDROID_NDK_HOME="$sdk/ndk/$ndk"
  if [ -n "${GITHUB_ENV:-}" ]; then
    printf 'ANDROID_HOME=%s\nANDROID_NDK_HOME=%s\n' "$ANDROID_HOME" "$ANDROID_NDK_HOME" >> "$GITHUB_ENV"
  fi
}

# EL EMULADOR DE ANDROID, para `android-emulador.yml`: adb, el Android Emulator y la
# imagen del sistema con Google APIs de la API a la que apuntan las apps
# (ANDROID_TARGET_SDK de empaquetar.sh), de la arquitectura del anfitrión. Va después
# de `android-sdk`, que deja el sdkmanager. Google publica el emulador para Linux
# x86_64, macOS y Windows, NO para Linux ARM64: en el frontal (`sytue-arm64`) se dice
# aquí y no al arrancar un emulador que no existe.
instalar_android_emulador() {
  local sdk="${ANDROID_HOME:?falta ANDROID_HOME: primero accion.sh android-sdk}" api abi
  case "$(uname -s)/$(uname -m)" in
    Darwin/arm64) abi=arm64-v8a ;;
    Linux/x86_64 | Darwin/x86_64) abi=x86_64 ;;
    *) echo "✗ Google no publica el Android Emulator para $(uname -s)/$(uname -m): el job va en el Mac (sytue-macos)." >&2
       return 1 ;;
  esac
  api="$(sed -n 's/^ANDROID_TARGET_SDK=//p' "$RAIZ/apps/fyne/scripts/empaquetar.sh")"
  "$sdk/cmdline-tools/latest/bin/sdkmanager" --sdk_root="$sdk" \
    "platform-tools" "emulator" "system-images;android-${api:?empaquetar.sh no fija ANDROID_TARGET_SDK};google_apis;$abi" >/dev/null
  anunciar "$sdk/platform-tools"
  anunciar "$sdk/emulator"
  printf 'ANDROID_EMULADOR_IMAGEN=%s\n' "system-images;android-$api;google_apis;$abi" >>"${GITHUB_ENV:-/dev/null}"
  export ANDROID_EMULADOR_IMAGEN="system-images;android-$api;google_apis;$abi"
  emulator -version 2>/dev/null | head -1 || true
}

# jsign es un .jar, como bundletool: se deja al lado un `jsign` que lo llama con java (el
# perfil `jdk`). Firma los .exe de Windows contra Azure Artifact Signing.
instalar_jsign() {
  local quiere="${JSIGN_VERSION:?falta JSIGN_VERSION en versions.env}" destino
  if command -v jsign >/dev/null 2>&1 && grep -qs "jsign-$quiere.jar" "$(command -v jsign)"; then
    echo "jsign $quiere ya está: no se descarga"
    return 0
  fi
  if [ -w /usr/local/bin ]; then destino=/usr/local/bin; else destino="$HOME/bin"; fi
  mkdir -p "$destino"
  bajar_verificado \
    "https://github.com/ebourg/jsign/releases/download/${quiere}/jsign-${quiere}.jar" \
    "${JSIGN_SHA256:?falta JSIGN_SHA256 en versions.env}" "$destino/jsign-$quiere.jar"
  printf '#!/bin/sh\nexec java -jar "%s" "$@"\n' "$destino/jsign-$quiere.jar" >"$destino/jsign"
  chmod 0755 "$destino/jsign"
  anunciar "$destino"
}

# osslsigncode VERIFICA las firmas Authenticode (jsign no sabe). Del sistema, como jq.
# Con curl y openssl: firmar-windows.sh baja las CRL de Microsoft y las pasa a PEM.
instalar_osslsigncode() {
  if command -v osslsigncode >/dev/null 2>&1 && command -v openssl >/dev/null 2>&1 &&
    command -v curl >/dev/null 2>&1; then
    echo "osslsigncode, openssl y curl ya están en el runner: no se instalan"
    return 0
  fi
  apt_actualizar
  apt_instalar -y -qq --no-install-recommends \
    osslsigncode openssl curl ca-certificates
  osslsigncode --version 2>&1 | head -1 || true
}

# El cliente Docker va separado del daemon: los jobs solo reciben el socket
# Unix del proxy restringido del runner. Instalar docker.io aquí aporta la CLI
# sin arrancar ni conceder un daemon dentro del contenedor efímero.
instalar_docker() {
  if command -v docker >/dev/null 2>&1; then
    echo "docker ya está en el runner: no se instala"
    return 0
  fi
  [ "$(uname -s)" = Linux ] || { echo "✗ docker solo se instala desde esta acción en Linux." >&2; return 1; }
  echo "instalando cliente Docker (el endpoint lo aporta el proxy restringido)"
  apt_actualizar
  apt_instalar -y -qq --no-install-recommends docker.io
  docker --version
}

# Trivy publica tarballs distintos para amd64 y ARM64. Elegir siempre el
# histórico `Linux-64bit` descargaba correctamente en los runners ARM64, pero
# fallaba al ejecutar con exit 126. La selección y el SHA viven aquí, junto con
# las demás herramientas que se descargan de forma verificable.
instalar_trivy() {
  local quiere="${TRIVY_VERSION:?falta TRIVY_VERSION en versions.env}"
  if command -v trivy >/dev/null 2>&1 && grep -q "$quiere" <<<"$(trivy --version 2>/dev/null)"; then
    echo "trivy $quiere ya está en el runner: no se descarga"
    return 0
  fi
  local os arch sha sufijo
  read -r os arch <<<"$(os_arch)"
  [ "$os" = linux ] || { echo "✗ trivy solo se instala desde esta acción en Linux." >&2; return 1; }
  sha="$(sha_de TRIVY "$os" "$arch")" || return 1
  case "$arch" in
    amd64) sufijo=64bit ;;
    arm64) sufijo=ARM64 ;;
    *) echo "✗ arquitectura no soportada para Trivy: $arch" >&2; return 1 ;;
  esac
  echo "instalando trivy $quiere (Linux-$sufijo; la imagen del runner no lo trae)"
  "$RAIZ/deploy/sytue-git/bin/bajar-herramienta.sh" trivy "$quiere" \
    "trivy_${quiere}_Linux-${sufijo}.tar.gz" "$sha" \
    "https://github.com/aquasecurity/trivy/releases/download/v${quiere}/trivy_${quiere}_Linux-${sufijo}.tar.gz" \
    /tmp/trivy.tar.gz
  tar -C /tmp -xzf /tmp/trivy.tar.gz trivy
  install -D -m 0755 /tmp/trivy /usr/local/bin/trivy
  rm -f /tmp/trivy /tmp/trivy.tar.gz
  trivy --version
}

# Lo que las apps de Fyne necesitan para compilar y empaquetar Linux y Windows, que va
# por apt igual que envsubst: son paquetes de la distribución, no binarios con SHA. El
# runner compartido de git.gay corre como uid 0, así que aquí se pueden instalar (la
# imagen propia, deploy/sytue-git/runner/Containerfile, no podía y los traía dentro).
#
# Solo tiene sentido en Linux: en macOS no hay apt, y el llamador no debe pedirlo ahí.
#
# Las apps Fyne en Linux piden CGO y las cabeceras de X11 y EGL. Este perfil
# instala las dependencias que comparten CI y los empaquetados nativos.
#
# `libgtk-3-dev` y `libwebkit2gtk-4.1-dev` eran de Wails y salieron el 2026-09-26: ni
# `apps/fyne` ni `apps/nucleo` enlazan GTK ni WebKit (ningún módulo de sus go.mod los
# pide). NSIS y mingw se quedan: los usa `apps/fyne/scripts/empaquetar.sh windows`.
# libpipewire y libspa: solo cabeceras, para compartir la pantalla en Wayland
# (apps/fyne/llamada/camara/pipewire_linux.go abre la biblioteca con dlopen).
FYNE_PKGCONFIG="x11 x11-xcb xkbcommon xkbcommon-x11 xcursor xfixes egl glesv2 vulkan libpipewire-0.3 libspa-0.2"
# Los paquetes que dan esas cabeceras (y gcc para el CGO): los comparten `escritorio` y
# `ventana`, que solo añaden lo suyo encima.
FYNE_APT_DEV="pkg-config gcc libgl1-mesa-dev xorg-dev libwayland-dev libxkbcommon-dev \
libx11-dev libx11-xcb-dev libxkbcommon-x11-dev libxcursor-dev libxfixes-dev \
libegl1-mesa-dev libgles2-mesa-dev libvulkan-dev libffi-dev libpipewire-0.3-dev libspa-0.2-dev"
instalar_escritorio() {
  local faltan=()
  command -v pkg-config >/dev/null 2>&1 || faltan+=(pkg-config)
  command -v makensis   >/dev/null 2>&1 || faltan+=(nsis)
  command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1 || faltan+=(gcc-mingw-w64-x86-64)
  # 7z saca el Mesa de reserva de Windows (preparar_mesa en empaquetar.sh). Faltó en
  # v3.8.3 y solo se vio en la release: ci-desktop pide este mismo perfil, así que
  # desde aquí lo descubre él. Debian lo instala como `7zz`; empaquetar.sh acepta los tres.
  command -v 7z >/dev/null 2>&1 || command -v 7za >/dev/null 2>&1 || command -v 7zz >/dev/null 2>&1 || faltan+=(7zip)
  # El bus privado de la prueba del portal de Wayland (camara/portal_linux_test.go).
  command -v dbus-daemon >/dev/null 2>&1 || faltan+=(dbus-daemon)
  # La edición admite las familias del sistema: la prueba de Georgia necesita
  # resolver una sustituta incluso en una imagen de CI sin fuentes preinstaladas.
  command -v fc-match >/dev/null 2>&1 || faltan+=(fontconfig fonts-dejavu-core)
  # Las cabeceras no dejan binario en el PATH, así que se preguntan a pkg-config.
  # Se comprueba DESPUÉS de tenerlo: sin pkg-config la respuesta no vale nada.
  # shellcheck disable=SC2086  # FYNE_PKGCONFIG es una lista: se quiere partida en palabras.
  if [ ${#faltan[@]} -eq 0 ] && pkg-config --exists gl wayland-client $FYNE_PKGCONFIG 2>/dev/null; then
    echo "las dependencias de escritorio ya están en el runner: no se instalan"
    return 0
  fi
  echo "instalando dependencias de escritorio (la imagen del runner no las trae)"
  apt_actualizar
  # shellcheck disable=SC2086  # FYNE_APT_DEV es una lista: se quiere partida en palabras.
  apt_instalar -y -qq --no-install-recommends $FYNE_APT_DEV \
    nsis gcc-mingw-w64-x86-64 dbus-daemon 7zip fontconfig fonts-dejavu-core
  # QUE EL FALLO SALGA AQUÍ Y NO A LOS DIEZ MINUTOS. Sin esto, un paquete que
  # cambie de nombre aguas arriba deja el job compilando hasta morir en el mismo
  # `pkg-config` de siempre, que es el error que no dice nada.
  pkg-config --exists gl wayland-client || {
    echo "✗ gl o wayland-client siguen sin verse tras instalarlos." >&2
    echo "  ¿cambió el nombre del paquete en la distribución del runner?" >&2
    exit 1
  }
  # shellcheck disable=SC2086
  pkg-config --exists $FYNE_PKGCONFIG || {
    echo "✗ faltan cabeceras de Fyne (X11/EGL) tras instalarlas: de «$FYNE_PKGCONFIG» no están todas." >&2
    echo "  ¿cambió el nombre de algún paquete en la distribución del runner?" >&2
    exit 1
  }
  command -v makensis >/dev/null 2>&1 || {
    echo "✗ nsis se instaló pero makensis no está en el PATH." >&2; exit 1; }
  command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1 || {
    echo "✗ gcc-mingw-w64-x86-64 se instaló pero x86_64-w64-mingw32-gcc no está en el PATH." >&2; exit 1; }
  command -v 7z >/dev/null 2>&1 || command -v 7za >/dev/null 2>&1 || command -v 7zz >/dev/null 2>&1 || {
    echo "✗ 7zip se instaló pero ni 7z, ni 7za, ni 7zz están en el PATH." >&2; exit 1; }
  echo "pkg-config $(pkg-config --version) · $(makensis -VERSION) · $(x86_64-w64-mingw32-gcc -dumpversion)"
}

# Los bundles de Linux se publican en amd64. La máquina de runners es ARM64,
# pero no se puede cambiar GOARCH sin más: Fyne en Linux usa CGO y el gcc nativo
# ARM rechaza el flag `-m64` que Go le pasa para el
# destino amd64. Este segundo perfil conserva las dependencias nativas que
# necesita NSIS y añade el compilador cruzado con las bibliotecas X11/EGL DEL
# destino (y las de Wayland/OpenGL que pedía Fyne: ver el ponytail de arriba).
#
# No se instala desde `escritorio` a secas: ci-desktop valida nativamente la
# shell y no debe cargar multiarch ni convertir por accidente su build en
# uno cruzado. Solo release-desktop pide explícitamente este perfil.
instalar_escritorio_amd64() {
  instalar_escritorio

  case "$(uname -m)" in
    x86_64|amd64)
      # El compilador y las bibliotecas ya son amd64 nativos.
      return 0
      ;;
    aarch64|arm64)
      ;;
    *)
      echo "✗ no sé preparar escritorio amd64 desde $(uname -m)." >&2
      return 1
      ;;
  esac

  echo "instalando sysroot y compilador cruzado Linux/amd64 para Fyne"
  dpkg --add-architecture amd64
  apt_actualizar
  apt_instalar -y -qq --no-install-recommends \
    gcc-x86-64-linux-gnu libc6-dev-amd64-cross \
    libgl1-mesa-dev:amd64 libegl1-mesa-dev:amd64 \
    libwayland-dev:amd64 libxkbcommon-dev:amd64 \
    libx11-dev:amd64 libxcursor-dev:amd64 libxrandr-dev:amd64 \
    libxinerama-dev:amd64 libxi-dev:amd64 libxxf86vm-dev:amd64 \
    libx11-xcb-dev:amd64 libxkbcommon-x11-dev:amd64 libxfixes-dev:amd64 \
    libgles2-mesa-dev:amd64 libvulkan-dev:amd64 libffi-dev:amd64 \
    libpipewire-0.3-dev:amd64 libspa-0.2-dev:amd64

  command -v x86_64-linux-gnu-gcc >/dev/null 2>&1 || {
    echo "✗ falta x86_64-linux-gnu-gcc tras instalar el toolchain cruzado." >&2
    return 1
  }
  PKG_CONFIG_PATH='' \
    PKG_CONFIG_LIBDIR=/usr/lib/x86_64-linux-gnu/pkgconfig:/usr/share/pkgconfig \
    pkg-config --exists gl wayland-client || {
      echo "✗ el pkg-config amd64 no encuentra OpenGL o Wayland." >&2
      return 1
    }
  # shellcheck disable=SC2086
  PKG_CONFIG_PATH='' \
    PKG_CONFIG_LIBDIR=/usr/lib/x86_64-linux-gnu/pkgconfig:/usr/share/pkgconfig \
    pkg-config --exists $FYNE_PKGCONFIG || {
      echo "✗ el pkg-config amd64 no encuentra las cabeceras de Fyne: de «$FYNE_PKGCONFIG» no están todas." >&2
      return 1
    }
}

# LA VENTANA REAL: compilar las apps de Fyne para Linux y ABRIRLAS en una ventana de
# verdad, con el GL de Mesa y no el pintor de software de los tests
# (apps/fyne/scripts/ventana-real.sh, que corre `ventana-real.yml`). Las cabeceras de
# `escritorio` sin NSIS ni mingw, que son del instalador de Windows, y lo que el guion
# pide para correr: Xvfb con GLX y el llvmpipe de Mesa, xdotool para pulsar, xwd e
# ImageMagick para capturar, glxinfo para dejar dicho qué GL pintó, y un D-Bus con
# gnome-keyring, que es donde la sesión de la suite guarda su token.
VENTANA_ORDENES="Xvfb xdpyinfo xdotool xwd convert glxinfo dbus-run-session gnome-keyring-daemon"
instalar_ventana() {
  [ "$(uname -s)" = Linux ] || { echo "✗ la ventana real se prepara en el contenedor Linux del CI." >&2; return 1; }
  local faltan=() h
  for h in $VENTANA_ORDENES; do
    command -v "$h" >/dev/null 2>&1 || faltan+=("$h")
  done
  # shellcheck disable=SC2086  # FYNE_PKGCONFIG es una lista: se quiere partida en palabras.
  if [ ${#faltan[@]} -eq 0 ] && command -v pkg-config >/dev/null 2>&1 &&
     pkg-config --exists gl $FYNE_PKGCONFIG 2>/dev/null; then
    echo "lo de la ventana real ya está en el runner: no se instala"
    return 0
  fi
  echo "instalando lo de la ventana real (la imagen del runner no lo trae)"
  apt_actualizar
  # shellcheck disable=SC2086  # FYNE_APT_DEV es una lista: se quiere partida en palabras.
  apt_instalar -y -qq --no-install-recommends $FYNE_APT_DEV \
    xvfb x11-utils x11-apps xdotool imagemagick mesa-utils libgl1-mesa-dri \
    dbus-daemon gnome-keyring fontconfig fonts-dejavu-core
  # Que el fallo salga aquí, con su nombre, y no a mitad del recorrido.
  for h in $VENTANA_ORDENES; do
    command -v "$h" >/dev/null 2>&1 || { echo "✗ $h sigue sin estar tras instalarlo." >&2; return 1; }
  done
  # shellcheck disable=SC2086
  pkg-config --exists gl $FYNE_PKGCONFIG || {
    echo "✗ faltan cabeceras de Fyne tras instalarlas: de «gl $FYNE_PKGCONFIG» no están todas." >&2
    return 1
  }
}

# golangci-lint. El job `lint` de ci-go lo invoca dando por hecho que está en la
# imagen del runner —lo estaba en la propia— y en la de git.gay no. Moría en
# segundos con «command not found», y eso tenía un efecto que no se veía: con
# `if: always()` llegaba igual al paso de guardar la caché y RESERVABA la clave
# `go-<hash>` sin tener nada que guardar, dejándola inservible para el
# `build-test` que veinte minutos después sí tenía los 9 GB de módulos. Por eso
# la caché de Go no acertó nunca.
instalar_golangci_lint() {
  local quiere="${GOLANGCI_LINT_VERSION:?falta GOLANGCI_LINT_VERSION en versions.env}"
  if command -v golangci-lint >/dev/null 2>&1 && grep -q "$quiere" <<<"$(golangci-lint version 2>/dev/null)"; then
    echo "golangci-lint $quiere ya está en el runner: no se instala"
    return 0
  fi
  command -v go >/dev/null 2>&1 || instalar_go
  echo "instalando golangci-lint $quiere (la imagen del runner no lo trae)"
  GOBIN=/usr/local/bin go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v${quiere}"
  command -v golangci-lint >/dev/null 2>&1 || {
    echo "✗ golangci-lint se instaló pero no está en el PATH." >&2; exit 1; }
  golangci-lint version
}

# osv-scanner. Lo piden dos jobs de security y hasta ahora cada uno se lo
# buscaba por su cuenta: el de dependencias lo bajaba con la versión y el sha
# copiados dentro del propio workflow, y el del SBOM lo llamaba a pelo dando por
# hecho que la imagen del runner lo trae. En la compartida no está.
instalar_osv_scanner() {
  local quiere="${OSV_SCANNER_VERSION:?falta OSV_SCANNER_VERSION en versions.env}"
  if command -v osv-scanner >/dev/null 2>&1 && grep -q "$quiere" <<<"$(osv-scanner --version 2>/dev/null)"; then
    echo "osv-scanner $quiere ya está en el runner: no se descarga"
    return 0
  fi
  local os arch sha
  read -r os arch <<<"$(os_arch)"
  [ "$os" = linux ] || { echo "✗ osv-scanner solo se instala desde esta acción en Linux." >&2; return 1; }
  sha="$(sha_de OSV_SCANNER "$os" "$arch")" || return 1
  echo "instalando osv-scanner $quiere (linux-$arch; la imagen del runner no lo trae)"
  bajar_verificado \
    "https://github.com/google/osv-scanner/releases/download/v${quiere}/osv-scanner_${os}_${arch}" \
    "$sha" \
    /tmp/osv-scanner
  install -D -m 0755 /tmp/osv-scanner /usr/local/bin/osv-scanner
  osv-scanner --version
}

# crane se compila con Go, como en la imagen propia: no publica un tarball con
# sha en versions.env, y `go install` con la versión fijada da el mismo binario
# reproducible sin añadir un hash más que mantener a mano.
# Solo el CLIENTE de BuildKit: el demonio es el del frontal, compartido por los
# jobs a través de su propio socket (BUILDKIT_HOST), no del de Docker (ADR 0066).
instalar_buildctl() {
  local quiere="${BUILDKIT_VERSION:?falta BUILDKIT_VERSION en versions.env}"
  if command -v buildctl >/dev/null 2>&1 && grep -q "v$quiere " <<<"$(buildctl --version 2>/dev/null) "; then
    echo "buildctl $quiere ya está en el runner: no se descarga"
    return 0
  fi
  local os arch sha fichero
  read -r os arch <<<"$(os_arch)"
  [ "$os" = linux ] || { echo "✗ buildctl solo se instala desde esta acción en Linux." >&2; return 1; }
  sha="$(sha_de BUILDKIT "$os" "$arch")" || return 1
  fichero="buildkit-v${quiere}.linux-${arch}.tar.gz"
  echo "instalando buildctl $quiere (el cliente; el BuildKit es el del frontal)"
  "$RAIZ/deploy/sytue-git/bin/bajar-herramienta.sh" buildkit "$quiere" "$fichero" "$sha" \
    "https://github.com/moby/buildkit/releases/download/v${quiere}/${fichero}" /tmp/buildkit.tar.gz
  tar -C /tmp -xzf /tmp/buildkit.tar.gz bin/buildctl
  install -D -m 0755 /tmp/bin/buildctl /usr/local/bin/buildctl
  rm -rf /tmp/bin /tmp/buildkit.tar.gz
  buildctl --version
}

# Cliente del repositorio estable de Debian del contenedor del job. psql usa
# el protocolo de PostgreSQL; para el SQL del bootstrap no se necesita el
# binario de servidor ni las herramientas de backup de su misma versión mayor.
instalar_psql() {
  if command -v psql >/dev/null 2>&1; then
    psql --version
    return 0
  fi
  apt_actualizar
  apt_instalar -y --no-install-recommends postgresql-client
  psql --version
}

# Debian separa ensurepip de Python: el guardián del altool real necesita un
# venv del job, igual que la firma de Apple, sin instalar pip en el sistema.
instalar_python_venv() {
  if python3 -c 'import ensurepip, venv' >/dev/null 2>&1; then
    return 0
  fi
  apt_actualizar
  apt-get install -y --no-install-recommends python3-venv
}

# El ejecutor de pasos de Codemagic (.github/actions/apple/codemagic-pasos.py) lee el
# YAML con Psych de Ruby, que el Mac trae de serie. Su guardián corre en Linux, en un
# contenedor sin Ruby: sin esto, ci-desktop salía en rojo con «No such file: ruby».
instalar_ruby() {
  if command -v ruby >/dev/null 2>&1; then
    return 0
  fi
  apt_actualizar
  apt-get install -y --no-install-recommends ruby
  ruby --version
}

instalar_zip() {
  if command -v zip >/dev/null 2>&1 && command -v unzip >/dev/null 2>&1; then
    return 0
  fi
  apt_actualizar
  apt_instalar -y -qq --no-install-recommends zip unzip
}

# La imagen fijada del CI es Debian 12: su paquete imagemagick sólo expone
# convert (IM6). Se compila la fuente estable verificada en un prefijo del job,
# sin cambiar de imagen ni sustituir las herramientas de la distribución.
instalar_imagemagick() {
  local quiere="${IMAGEMAGICK_VERSION:?falta IMAGEMAGICK_VERSION en versions.env}"
  local base="$RAIZ/.deps/imagemagick/$quiere"
  if [ -x "$base/bin/magick" ] && grep -q "ImageMagick $quiere " <<<"$("$base/bin/magick" -version)"; then
    anunciar "$base/bin"
    return 0
  fi
  [ "$(uname -s)" = Linux ] || { echo "✗ ImageMagick se prepara en el contenedor Linux del CI." >&2; return 1; }
  if ! command -v make >/dev/null 2>&1 || ! command -v gcc >/dev/null 2>&1 ||
     ! command -v xz >/dev/null 2>&1 || ! command -v pkg-config >/dev/null 2>&1 ||
     ! pkg-config --exists libpng librsvg-2.0; then
    apt_actualizar
    apt_instalar -y -qq --no-install-recommends \
      build-essential xz-utils pkg-config libpng-dev librsvg2-dev
  fi
  (
    trabajo="$(mktemp -d)"
    trap 'rm -rf "$trabajo"' EXIT
    bajar_verificado \
      "https://github.com/ImageMagick/ImageMagick/releases/download/${quiere}/ImageMagick-${quiere}.tar.xz" \
      "${IMAGEMAGICK_SOURCE_SHA256:?falta IMAGEMAGICK_SOURCE_SHA256 en versions.env}" "$trabajo/fuente.tar.xz"
    tar -C "$trabajo" -xf "$trabajo/fuente.tar.xz"
    cd "$trabajo/ImageMagick-$quiere"
    ./configure --prefix="$base" --disable-shared --disable-openmp --disable-docs \
      --without-magick-plus-plus --without-perl --without-x --with-rsvg=yes --with-png=yes
    make -j2
    make install
  )
  anunciar "$base/bin"
  "$base/bin/magick" -version
}

instalar_crane() {
  local quiere="${CRANE_VERSION:?falta CRANE_VERSION en versions.env}"
  if command -v crane >/dev/null 2>&1 && grep -q "$quiere" <<<"$(crane version 2>/dev/null)"; then
    echo "crane $quiere ya está en el runner: no se descarga"
    return 0
  fi
  command -v go >/dev/null 2>&1 || instalar_go
  echo "instalando crane $quiere (la imagen del runner no lo trae)"
  GOBIN=/usr/local/bin go install "github.com/google/go-containerregistry/cmd/crane@v${quiere}"
  crane version
}

[ "$#" -gt 0 ] || { echo "uso: accion.sh [go] [bun] [helm] [crane] [buildctl] [envsubst] [jq] [trivy] [docker] [syft] [govulncheck] [osv-scanner] [golangci-lint] [escritorio] [escritorio-amd64] [ventana] [cosign] [jdk] [bundletool] [android-sdk] [android-emulador] [jsign] [osslsigncode] [psql] [python-venv] [ruby] [zip] [imagemagick]" >&2; exit 2; }

# `/usr/local/bin` SE ANUNCIA SIEMPRE, aunque no se instale nada ahí en esta
# llamada. Varias herramientas van a ese directorio con `GOBIN=/usr/local/bin go
# install`, y el PATH del contenedor de act NO lo trae: se instalaban bien, la
# comprobación de la propia función las encontraba —mismo proceso, mismo PATH— y
# el paso SIGUIENTE moría con «exitcode 127: command not found» sin decir qué
# comando faltaba. Exportar PATH dentro de un `run` no llega al siguiente; lo que
# cruza es $GITHUB_PATH, y de eso se ocupa `anunciar`.
anunciar /usr/local/bin
for h in "$@"; do
  case "$h" in
    go)     instalar_go; avisar_cache_go_ajena
      if [ -f "$RAIZ/apps/fyne/scripts/preparar-livekit.sh" ]; then
        if ! command -v patch >/dev/null || ! command -v unzip >/dev/null; then
          if [ "$(uname -s)" = Linux ]; then apt_actualizar; apt-get install -y --no-install-recommends patch unzip; fi
        fi
        bash "$RAIZ/apps/fyne/scripts/preparar-livekit.sh"
      fi ;;
    bun)    instalar_bun ;;
    helm)   instalar_helm ;;
    crane)  instalar_crane ;;
    buildctl) instalar_buildctl ;;
    govulncheck) instalar_govulncheck ;;
    go-licenses) instalar_go_licenses ;;
    golangci-lint) instalar_golangci_lint ;;
    syft)   instalar_syft ;;
    osv-scanner) instalar_osv_scanner ;;
    envsubst) instalar_envsubst ;;
    jq)     instalar_jq ;;
    zip)    instalar_zip ;;
    imagemagick) instalar_imagemagick ;;
    psql)   instalar_psql ;;
    python-venv) instalar_python_venv ;;
    ruby)   instalar_ruby ;;
    trivy)  instalar_trivy ;;
    docker) instalar_docker ;;
    escritorio) instalar_escritorio ;;
    escritorio-amd64) instalar_escritorio_amd64 ;;
    ventana) instalar_ventana ;;
    cosign) instalar_cosign ;;
    jdk)    instalar_jdk ;;
    bundletool) instalar_bundletool ;;
    android-sdk) instalar_android_sdk ;;
    android-emulador) instalar_android_emulador ;;
    jsign)  instalar_jsign ;;
    osslsigncode) instalar_osslsigncode ;;
    *)      echo "✗ herramienta desconocida: $h" >&2; exit 2 ;;
  esac
done
