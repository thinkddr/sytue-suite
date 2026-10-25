#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
set -euo pipefail
AQUI="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
temporal="$(mktemp -d)"
trap 'rm -rf "$temporal"' EXIT
mkdir -p "$temporal/bin"
export APPLE_SDK_CASOS="$temporal/casos" APPLE_SDK_CANARIO="$temporal/ejecutado"
cat >"$temporal/bin/xcrun" <<'SH'
#!/usr/bin/env bash
case "$1" in
otool)
  case "$2" in
  -L)
    puente='@rpath/SytueWidgetBridge.framework/Versions/A/SytueWidgetBridge'
    [[ "$3" != *ios* ]] || puente='@rpath/SytueWidgetBridge.framework/SytueWidgetBridge'
    [ "${APPLE_SDK_PUENTE_ABSOLUTO:-no}" != si ] || puente="${puente/@rpath/\/Library\/Frameworks}"
    echo "$puente (compatibility version 1.0.0)"
    if [ "${APPLE_SDK_ALIAS_SWIFT:-no}" = si ] && [[ "$3" == *ios* ]]; then echo '@rpath/libswiftCoreMedia.dylib'; fi ;;
  -l)
    ruta='@executable_path/../Frameworks'
    [[ "$3" != *ios* ]] || ruta='@executable_path/Frameworks'
    printf 'Load command 1\n cmd LC_RPATH\n path %s (offset 12)\n' "$ruta" ;;
  -hv) printf 'Mach header\nmagic cputype cpusubtype caps filetype ncmds sizeofcmds flags\nMH_MAGIC_64 ARM64 ALL 0x00 EXECUTE 10 4000 0x00\n' ;;
  *) exit 1 ;;
  esac
  exit 0 ;;
vtool)
  printf 'Load command 1\n cmd LC_BUILD_VERSION\n platform IOS\n minos %s\n sdk 27.1\n' "$APPLE_SDK_MINIMUM"
  exit 0 ;;
esac
case "$3" in
--show-sdk-path) echo "/sdk/$2" ;;
-f) echo "/bin/$4" ;;
*) exit 1 ;;
esac
SH
cat >"$temporal/bin/go" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s|%s|%s|%s|%s|%s\n' "$GOOS" "$GOARCH" "$CGO_CFLAGS" "$CGO_LDFLAGS" "$*" "${CGO_LDFLAGS_ALLOW:-}" >>"$APPLE_SDK_CASOS"
# Revisa la fuente que se entregaría a cada compilador, antes de que el trap la quite.
for argumento in "$@"; do fuente="$argumento"; done
python3 - "$fuente/main.go" <<'PYTHON'
import pathlib, re, sys
fuente = pathlib.Path(sys.argv[1]).read_text()
assert '"git.sytue.eu/sytue/one/apps/nucleo/sistema"' in fuente, "el SDK no compilaría el puente Objective-C de FileProvider"
assert "runtime.KeepAlive([]any{" in fuente, "las referencias de FileProvider deben llegar al enlace"
for nombre in ("ArchivosDisponible", "GuardarCredencialArchivos", "HayCredencialArchivos", "RegistrarArchivos", "AbrirArchivos", "OlvidarArchivos"):
    assert "sistema." + nombre + "," in fuente, "falta la referencia de FileProvider: " + nombre
    assert not re.search(r"sistema\." + nombre + r"\s*\(", fuente), "el canario no debe invocar APIs de FileProvider"
PYTHON
while [ "$1" != -o ]; do shift; done
shift
# If the verifier ever runs the output, the canary records forbidden execution.
printf '#!/usr/bin/env bash\ntouch "$APPLE_SDK_CANARIO"\n' >"$1"
chmod +x "$1"
SH
chmod +x "$temporal/bin/"*
PATH="$temporal/bin:$PATH" RUNNER_TEMP="$temporal" bash "$AQUI/apple-sesion-sdk.sh"
[ ! -e "$APPLE_SDK_CANARIO" ] || { echo 'not ok - el gate ejecutó código que puede acceder al Keychain' >&2; exit 1; }
python3 - "$APPLE_SDK_CASOS" "$AQUI/../xcode/Sytue.xcconfig" <<'PY'
import pathlib, re, sys
casos = pathlib.Path(sys.argv[1]).read_text().splitlines()
config = pathlib.Path(sys.argv[2]).read_text()
mac = re.search(r'^MACOSX_DEPLOYMENT_TARGET = ([0-9.]+)$', config, re.M).group(1)
ios = re.search(r'^IPHONEOS_DEPLOYMENT_TARGET = ([0-9.]+)$', config, re.M).group(1)
assert len(casos) == 5, casos
for arch, clangarch in [('arm64', 'arm64'), ('amd64', 'x86_64')]:
    for tags in ['goolm', 'tienda,goolm']:
        matches = [c for c in casos if c.startswith('darwin|' + arch + '|') and '-tags ' + tags + ' ' in c]
        assert len(matches) == 1, matches
        assert '-arch ' + clangarch in matches[0] and '-mmacosx-version-min=' + mac in matches[0]
device = [c for c in casos if c.startswith('ios|arm64|')]
assert len(device) == 1 and '-miphoneos-version-min=' + ios in device[0]
assert '-buildmode=c-archive' in device[0] and '-tags goolm ' in device[0]
assert all('|build ' in c for c in casos), casos
assert all(c.endswith('|') for c in casos), casos
print('ok - los bindings Apple se compilan para cinco variantes sin ejecutar sus resultados')
PY

# Cuando existen los productos Swift, se enlaza también cada host Go real. Las
# salidas siguen siendo canarios: ninguna verificación puede ejecutarlas.
mkdir -p "$temporal/widgets/ios/productos/SytueWidgetBridge.framework" "$temporal/widgets/macos/productos/SytueWidgetBridge.framework"
export APPLE_SDK_MINIMUM
APPLE_SDK_MINIMUM="$(sed -n 's/^IPHONEOS_DEPLOYMENT_TARGET = \([0-9.]*\)$/\1/p' "$AQUI/../xcode/Sytue.xcconfig")"
: >"$APPLE_SDK_CASOS"
PATH="$temporal/bin:$PATH" RUNNER_TEMP="$temporal" SYTUE_WIDGET_PRODUCTOS_ROOT="$temporal/widgets" \
  bash "$AQUI/apple-sesion-sdk.sh"
[ ! -e "$APPLE_SDK_CANARIO" ] || { echo 'not ok - el gate ejecutó un host enlazado' >&2; exit 1; }
python3 - "$APPLE_SDK_CASOS" <<'PY'
import pathlib, sys
casos = pathlib.Path(sys.argv[1]).read_text().splitlines()
assert len(casos) == 6, casos
assert all('-Wl,-needed_framework,SytueWidgetBridge' in c for c in casos), casos
assert all('|-isysroot ' in c and ' -F "' in c for c in casos), casos
assert all(c.endswith('|^-Wl,(-needed_framework,SytueWidgetBridge|-rpath,@executable_path/(\\.\\./)?Frameworks)$') for c in casos), casos
ios = [c for c in casos if c.startswith('ios|')]
assert len(ios) == 2 and sum('-buildmode=c-archive' in c for c in ios) == 1, ios
print('ok - cuatro hosts Mac y un host iOS enlazan WidgetKit sin ejecutarse')
PY
if PATH="$temporal/bin:$PATH" RUNNER_TEMP="$temporal" SYTUE_WIDGET_PRODUCTOS_ROOT="$temporal/widgets" \
  APPLE_SDK_ALIAS_SWIFT=si bash "$AQUI/apple-sesion-sdk.sh" >"$temporal/alias.log" 2>&1; then
  echo 'not ok - el host iOS acepta un alias Swift sin soporte antes de firmar' >&2
  exit 1
fi
grep -q 'enlaza Swift por @rpath' "$temporal/alias.log" || { cat "$temporal/alias.log" >&2; exit 1; }
[ ! -e "$APPLE_SDK_CANARIO" ] || { echo 'not ok - el caso negativo ejecutó un host' >&2; exit 1; }
if PATH="$temporal/bin:$PATH" RUNNER_TEMP="$temporal" SYTUE_WIDGET_PRODUCTOS_ROOT="$temporal/widgets" \
  APPLE_SDK_PUENTE_ABSOLUTO=si bash "$AQUI/apple-sesion-sdk.sh" >"$temporal/absoluto.log" 2>&1; then
  echo 'not ok - el host carga el framework desde una ruta externa al bundle' >&2
  exit 1
fi
grep -q 'el host Go no carga' "$temporal/absoluto.log" || { cat "$temporal/absoluto.log" >&2; exit 1; }
