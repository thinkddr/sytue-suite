#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# XCODE CLOUD, NADA MÁS CLONAR. Lo ejecuta él solo antes de cada acción (archivar para iOS,
# archivar para macOS…), cada una en su máquina, y deja lo que la imagen de Apple no trae y
# la fase «Compilar el Go» de Sytue.xcodeproj necesita:
#
#   1. version.xcconfig: CFBundleShortVersionString del tag (tienda-v3.12.0 o v3.12.0 →
#      3.12.0) y
#      CFBundleVersion, el número de build de Xcode Cloud (CI_BUILD_NUMBER).
#      El cifrado: exento de documentación sin Francia, o el código aprobado de la app.
#   2. Go, el de deploy/sytue-git/versions.env verificado por sha256 (accion.sh, lo mismo
#      que el CI y Codemagic), y la CLI de Fyne fijada.
#   3. Los módulos de la app del esquema, AQUÍ y no en la fase de Xcode: los forks de Fyne
#      son PRIVADOS (github.com/thinkddr/{fyne,typesetting,glfw}) y aquí están las
#      credenciales.
#
# Sin secretos en el repositorio: a los forks privados (typesetting y glfw; fyne es público)
# entra Xcode Cloud con su acceso a GitHub, una vez aprobados en cada producto; la variable
# SECRETA SYTUE_GIT_FORKS_TOKEN del flujo es la alternativa. Ver «Xcode Cloud» en
# docs/operations.md.
set -euo pipefail

cd "${CI_PRIMARY_REPOSITORY_PATH:?esto lo ejecuta Xcode Cloud}"
app="${CI_XCODE_SCHEME:?}"

preparar_cifrado() {
  # FALSE sin código: algoritmos estándar y distribución sin Francia, no «sin cifrado».
  # Revisar esta exención antes de habilitar Francia o incorporar algoritmos propios.
  # El código del workflow de ESTA app se usa solo si Apple aprobó sus documentos:
  # TRUE sin código válido tumba la subida con 90592. Nunca hay un código por defecto.
  python3 - <<'PY'
import os, pathlib, plistlib, sys

uso = "ITSAppUsesNonExemptEncryption"
codigo = "ITSEncryptionExportComplianceCode"
valor = os.environ.get("SYTUE_APP_ENCRYPTION_EXPORT_CODE", "")

def codigo_no_vacio(valor):
    return (isinstance(valor, str) and bool(valor.strip()) and valor == valor.strip()
            and valor != "[]" and not any(ord(c) < 32 for c in valor))

if valor and not codigo_no_vacio(valor):
    sys.exit("error: SYTUE_APP_ENCRYPTION_EXPORT_CODE no contiene un código; se omite o se aporta el aprobado por Apple")

plists = []
for plataforma in ("ios", "macos"):
    ruta = pathlib.Path("apps/fyne/xcode/Info-%s.plist" % plataforma)
    with ruta.open("rb") as fichero:
        info = plistlib.load(fichero)
    exento = info.get(uso) is False and codigo not in info
    documentado = info.get(uso) is True and codigo_no_vacio(info.get(codigo))
    if not (exento or documentado):
        sys.exit("error: %s requiere FALSE sin código o el par TRUE + código no vacío; no se sube a Apple" % ruta)
    plists.append((ruta, info))

# Validar las dos plataformas antes de escribir cualquiera de ellas.
for ruta, info in plists:
    nuevo = dict(info)
    if valor:
        nuevo[uso] = True
        nuevo[codigo] = valor
    if nuevo != info:
        ruta.write_bytes(plistlib.dumps(nuevo, sort_keys=False))
print("Cifrado: código aportado por el workflow" if valor else "Cifrado: metadatos de exportación validados")
PY
}

# El guardián prueba los plists sin instalar Go, archivar ni subir un build.
if [ "${1:-}" = --solo-cifrado ]; then
  preparar_cifrado
  exit 0
fi

# 1. LA VERSIÓN. Las ramas manuales TAMBIÉN suben a App Store Connect. Apple las clona
# sin tags y con poco historial: git describe no ve la release de la que salen. Se leen
# las referencias remotas y se amplía SOLO el historial del commit que se está probando,
# sin cambiar HEAD ni traer la historia de todos los tags. Un tag de otra rama o una RC
# no puede convertirse en la versión de esta app.
version_de_rama() {
  local version refs profundidad
  version="$(git describe --tags --abbrev=0 --match 'v[0-9]*.[0-9]*.[0-9]*' --exclude '*[-+]*' 2>/dev/null || true)"
  if [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    printf '%s\n' "$version"
    return 0
  fi
  refs="$(git ls-remote --tags origin 'refs/tags/v*')" || return 1
  for profundidad in 0 32 128 512; do
    if [ "$profundidad" -gt 0 ]; then
      [ "$(git rev-parse --is-shallow-repository)" = true ] || break
      git fetch --quiet --deepen="$profundidad" --no-tags origin "$(git rev-parse HEAD)" || return 1
    fi
    version="$(printf '%s\n' "$refs" | python3 -c '
import re, subprocess, sys
tags = {}
for line in sys.stdin:
    fields = line.split()
    if len(fields) != 2:
        continue
    sha, ref = fields
    match = re.fullmatch(r"refs/tags/v([0-9]+)\.([0-9]+)\.([0-9]+)(\^\{\})?", ref)
    if match:
        key = tuple(map(int, match.groups()[:3]))
        # Los tags anotados apuntan primero al objeto tag: su ^{} es el commit.
        if key not in tags or match.group(4):
            tags[key] = sha
ancestros = set(subprocess.check_output(["git", "rev-list", "HEAD"], text=True).split())
alcanzables = [version for version, sha in tags.items() if sha in ancestros]
if alcanzables:
    print("v" + ".".join(map(str, max(alcanzables))))
')" || return 1
    if [ -n "$version" ]; then
      # Lo necesita también empaquetar.sh para la versión que incrusta en el Go.
      git fetch --quiet --depth=1 --no-tags origin "refs/tags/$version:refs/tags/$version" || return 1
      printf '%s\n' "$version"
      return 0
    fi
  done
  return 1
}

version="${CI_TAG:-}"
if [ -z "$version" ]; then
  version="$(version_de_rama)" || {
    # Alternativa explícita para un checkout al que Apple no deje hacer fetch. La pone
    # quien encarga el build, desde la release publicada; nunca FyneApp.toml (0.1.0).
    version="${SYTUE_APP_VERSION:-}"
  }
fi
# Los flujos arrancan con `tienda-vX.Y.Z`, no con cada release: 30 subidas por tag agotan
# el cupo diario de la cuenta de Apple. Un `vX.Y.Z` vale para un build lanzado a mano.
version="${version#tienda-}"
version="${version#v}"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || [ "$version" = 0.0.0 ]; then
  echo "error: no se puede resolver una versión estable de este commit; no se sube 0.0.0." >&2
  echo "  Usa un tag tienda-vX.Y.Z, permite fetch al repositorio o declara SYTUE_APP_VERSION desde la release publicada." >&2
  exit 1
fi
printf 'MARKETING_VERSION = %s\nCURRENT_PROJECT_VERSION = %s\n' "$version" "${CI_BUILD_NUMBER:?}" \
  >apps/fyne/xcode/version.xcconfig
echo "$app $version ($CI_BUILD_NUMBER) para ${CI_PRODUCT_PLATFORM:-?}"
# El guardián prueba la resolución con repositorios shallow sin instalar ni compilar Go.
[ "${1:-}" != --solo-version ] || exit 0
preparar_cifrado

# 2. Go y la CLI de Fyne.
bash .github/actions/toolchain/accion.sh go
export PATH="/usr/local/go/bin:$HOME/.sytue-toolchain/go/bin:$HOME/go/bin:$PATH"
export GOTOOLCHAIN=local GOPRIVATE='github.com/thinkddr/*'
# Con el token, forks.sh lo prueba contra GitHub y deja a git leyendo los forks con él. Sin
# él queda el acceso que Xcode Cloud tenga a esos repositorios (repositorios adicionales).
if [ -n "${SYTUE_GIT_FORKS_TOKEN:-}" ]; then
  bash .github/actions/toolchain/forks.sh
fi
go install "fyne.io/tools/cmd/fyne@v$(sed -n 's/^FYNE_TOOLS_VERSION=//p' deploy/sytue-git/versions.env)"

# 3. Los módulos que compila esta app en esta plataforma (los de macOS, con `tienda`).
goos=ios
[ "${CI_PRODUCT_PLATFORM:-}" != macOS ] || goos=darwin
(cd apps/fyne && GOOS="$goos" GOARCH=arm64 CGO_ENABLED=1 go list -deps -tags tienda,goolm "./cmd/$app" >/dev/null) || {
  echo "error: no se bajan los módulos de apps/fyne/cmd/$app. Si es «could not read Username»" >&2
  echo "  o un 401, 403 o 404 de github.com/thinkddr: App Store Connect → Xcode Cloud → $app →" >&2
  echo "  Ajustes → Repositorios → «Conceder» en los que pide (o SYTUE_GIT_FORKS_TOKEN, secreta)." >&2
  exit 1
}
