// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// Traducción de apps/fyne/scripts/apple-sesion-sdk-test.sh: apple-sesion-sdk.sh
// (el real) compila los bindings de Apple para cinco variantes con xcrun y go
// de mentira, y NUNCA ejecuta lo que compila: los binarios de salida son
// canarios que dejan rastro si alguien los ejecuta (podrían tocar el Keychain).

const appleSDKXcrun = `case "$1" in
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
esac`

// El go de mentira anota cada compilación y guarda una copia de la fuente que
// recibiría el compilador (antes de que el trap del guion la borre), para que
// la prueba la revise.
const appleSDKGo = `set -euo pipefail
printf '%s|%s|%s|%s|%s|%s\n' "$GOOS" "$GOARCH" "$CGO_CFLAGS" "$CGO_LDFLAGS" "$*" "${CGO_LDFLAGS_ALLOW:-}" >>"$APPLE_SDK_CASOS"
for argumento in "$@"; do fuente="$argumento"; done
n="$(find "$APPLE_SDK_FUENTES" -type f | wc -l)"
cp "$fuente/main.go" "$APPLE_SDK_FUENTES/$n.go"
while [ "$1" != -o ]; do shift; done
shift
# Si el verificador ejecutara alguna vez la salida, el canario deja constancia.
printf '#!/usr/bin/env bash\ntouch "$APPLE_SDK_CANARIO"\n' >"$1"
chmod +x "$1"`

type appleSDK struct {
	temporal, casos, canario, fuentes string
	env                               []string
}

func appleNuevoSDK(t *testing.T) *appleSDK {
	t.Helper()
	temporal := t.TempDir()
	s := &appleSDK{
		temporal: temporal, casos: filepath.Join(temporal, "casos"), canario: filepath.Join(temporal, "ejecutado"),
		fuentes: filepath.Join(temporal, "fuentes"),
	}
	if err := os.MkdirAll(s.fuentes, 0o750); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(temporal, "bin")
	guiontest.Doble(t, bin, "xcrun", appleSDKXcrun)
	guiontest.Doble(t, bin, "go", appleSDKGo)
	s.env = []string{
		guiontest.ConPATH(bin), "RUNNER_TEMP=" + temporal, "APPLE_SDK_CASOS=" + s.casos,
		"APPLE_SDK_CANARIO=" + s.canario, "APPLE_SDK_FUENTES=" + s.fuentes,
	}
	return s
}

func (s *appleSDK) ejecutar(t *testing.T, env ...string) guiontest.Resultado {
	t.Helper()
	return guiontest.Guion(t, guiontest.Opciones{Env: append(append([]string{}, s.env...), env...)}, "apps/fyne/scripts/apple-sesion-sdk.sh")
}

func (s *appleSDK) sinCanario(t *testing.T, que string) {
	t.Helper()
	if _, err := os.Stat(s.canario); err == nil {
		t.Fatalf("not ok - %s", que)
	}
}

func (s *appleSDK) leerCasos(t *testing.T) []string {
	t.Helper()
	texto := strings.TrimRight(guiontest.Leer(t, s.casos), "\n")
	if texto == "" {
		return nil
	}
	return strings.Split(texto, "\n")
}

// revisarFuentes: lo que comprobaba el python del go de mentira en cada compilación.
func (s *appleSDK) revisarFuentes(t *testing.T) {
	t.Helper()
	copias, err := filepath.Glob(filepath.Join(s.fuentes, "*.go"))
	if err != nil || len(copias) == 0 {
		t.Fatalf("el go de mentira no recibió fuentes (%v)", err)
	}
	sort.Strings(copias)
	for _, c := range copias {
		fuente := guiontest.Leer(t, c)
		if !strings.Contains(fuente, `"git.sytue.eu/sytue/one/apps/nucleo/sistema"`) {
			t.Errorf("%s: el SDK no compilaría el puente Objective-C de FileProvider", filepath.Base(c))
		}
		if !strings.Contains(fuente, "runtime.KeepAlive([]any{") {
			t.Errorf("%s: las referencias de FileProvider deben llegar al enlace", filepath.Base(c))
		}
		for _, nombre := range []string{"ArchivosDisponible", "GuardarCredencialArchivos", "HayCredencialArchivos", "RegistrarArchivos", "AbrirArchivos", "OlvidarArchivos"} {
			if !strings.Contains(fuente, "sistema."+nombre+",") {
				t.Errorf("%s: falta la referencia de FileProvider: %s", filepath.Base(c), nombre)
			}
			if regexp.MustCompile(`sistema\.` + nombre + `\s*\(`).MatchString(fuente) {
				t.Errorf("%s: el canario no debe invocar APIs de FileProvider: %s", filepath.Base(c), nombre)
			}
		}
		// La galería de Photos y el selector de Fotos: el Objective-C de PhotoKit,
		// ImageIO, Network y PHPicker solo lo compila un SDK de Apple. Referencias,
		// nunca llamadas.
		for _, ref := range []string{
			"carrete.Disponible", "carrete.EstadoPermiso", "carrete.PedirPermiso", "carrete.Listar", "carrete.Abrir",
			"carrete.Guardar", "carrete.Convertir", "carrete.RedMedida", "ficheros.ElegirImagenes", "sistema.SeguirEnFondo",
		} {
			if !strings.Contains(fuente, ref+",") {
				t.Errorf("%s: falta la referencia de la galería: %s", filepath.Base(c), ref)
			}
			if regexp.MustCompile(regexp.QuoteMeta(ref) + `\s*\(`).MatchString(fuente) {
				t.Errorf("%s: el canario no debe invocar la galería: %s", filepath.Base(c), ref)
			}
		}
	}
}

func appleXcconfig(t *testing.T, clave string) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^` + clave + ` = ([0-9.]+)$`).FindStringSubmatch(guiontest.Leer(t, "apps/fyne/xcode/Sytue.xcconfig"))
	if m == nil {
		t.Fatalf("Sytue.xcconfig sin %s", clave)
	}
	return m[1]
}

func appleFiltrar(casos []string, f func(string) bool) []string {
	var out []string
	for _, c := range casos {
		if f(c) {
			out = append(out, c)
		}
	}
	return out
}

func TestAppleSesionSDKLosBindingsSeCompilanParaCincoVariantesSinEjecutarSusResultados(t *testing.T) {
	s := appleNuevoSDK(t)
	if r := s.ejecutar(t); r.Codigo != 0 {
		t.Fatalf("apple-sesion-sdk.sh: código %d\n%s", r.Codigo, r.Todo())
	}
	s.sinCanario(t, "el gate ejecutó código que puede acceder al Keychain")
	s.revisarFuentes(t)
	mac, ios := appleXcconfig(t, "MACOSX_DEPLOYMENT_TARGET"), appleXcconfig(t, "IPHONEOS_DEPLOYMENT_TARGET")
	casos := s.leerCasos(t)
	if len(casos) != 5 {
		t.Fatalf("se esperaban 5 compilaciones: %q", casos)
	}
	for _, a := range [][2]string{{"arm64", "arm64"}, {"amd64", "x86_64"}} {
		for _, tags := range []string{"goolm", "tienda,goolm"} {
			m := appleFiltrar(casos, func(c string) bool {
				return strings.HasPrefix(c, "darwin|"+a[0]+"|") && strings.Contains(c, "-tags "+tags+" ")
			})
			if len(m) != 1 {
				t.Errorf("darwin %s %s: %q", a[0], tags, m)
				continue
			}
			if !strings.Contains(m[0], "-arch "+a[1]) || !strings.Contains(m[0], "-mmacosx-version-min="+mac) {
				t.Errorf("darwin %s %s sin -arch %s o mínimo %s: %s", a[0], tags, a[1], mac, m[0])
			}
		}
	}
	dispositivo := appleFiltrar(casos, func(c string) bool { return strings.HasPrefix(c, "ios|arm64|") })
	if len(dispositivo) != 1 || !strings.Contains(dispositivo[0], "-miphoneos-version-min="+ios) ||
		!strings.Contains(dispositivo[0], "-buildmode=c-archive") || !strings.Contains(dispositivo[0], "-tags goolm ") {
		t.Errorf("iOS: %q", dispositivo)
	}
	for _, c := range casos {
		if !strings.Contains(c, "|build ") || !strings.HasSuffix(c, "|") {
			t.Errorf("compilación inesperada (sin build o con CGO_LDFLAGS_ALLOW): %s", c)
		}
	}
}

// appleSDKConWidgets: con los productos Swift presentes se enlaza también cada
// host Go real. Las salidas siguen siendo canarios.
func appleSDKConWidgets(t *testing.T) (*appleSDK, []string) {
	t.Helper()
	s := appleNuevoSDK(t)
	for _, p := range []string{"ios", "macos"} {
		if err := os.MkdirAll(filepath.Join(s.temporal, "widgets", p, "productos", "SytueWidgetBridge.framework"), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	env := []string{
		"SYTUE_WIDGET_PRODUCTOS_ROOT=" + filepath.Join(s.temporal, "widgets"),
		"APPLE_SDK_MINIMUM=" + appleXcconfig(t, "IPHONEOS_DEPLOYMENT_TARGET"),
	}
	return s, env
}

func TestAppleSesionSDKCuatroHostsMacYUnHostIOSEnlazanWidgetKitSinEjecutarse(t *testing.T) {
	s, env := appleSDKConWidgets(t)
	if r := s.ejecutar(t, env...); r.Codigo != 0 {
		t.Fatalf("apple-sesion-sdk.sh: código %d\n%s", r.Codigo, r.Todo())
	}
	s.sinCanario(t, "el gate ejecutó un host enlazado")
	s.revisarFuentes(t)
	casos := s.leerCasos(t)
	if len(casos) != 6 {
		t.Fatalf("se esperaban 6 compilaciones: %q", casos)
	}
	permiso := `|^-Wl,(-needed_framework,SytueWidgetBridge|-rpath,@executable_path/(\.\./)?Frameworks)$`
	for _, c := range casos {
		if !strings.Contains(c, "-Wl,-needed_framework,SytueWidgetBridge") || !strings.Contains(c, "|-isysroot ") ||
			!strings.Contains(c, ` -F "`) || !strings.HasSuffix(c, permiso) {
			t.Errorf("host sin el enlace de WidgetKit o sin su permiso finito: %s", c)
		}
	}
	ios := appleFiltrar(casos, func(c string) bool { return strings.HasPrefix(c, "ios|") })
	if len(ios) != 2 || len(appleFiltrar(ios, func(c string) bool { return strings.Contains(c, "-buildmode=c-archive") })) != 1 {
		t.Errorf("iOS: %q", ios)
	}
}

func TestAppleSesionSDKElHostIOSRechazaUnAliasSwiftSinSoporteAntesDeFirmar(t *testing.T) {
	s, env := appleSDKConWidgets(t)
	r := s.ejecutar(t, append(env, "APPLE_SDK_ALIAS_SWIFT=si")...)
	if r.Codigo == 0 {
		t.Fatal("not ok - el host iOS acepta un alias Swift sin soporte antes de firmar")
	}
	if !strings.Contains(r.Todo(), "enlaza Swift por @rpath") {
		t.Errorf("falta el motivo:\n%s", r.Todo())
	}
	s.sinCanario(t, "el caso negativo ejecutó un host")
}

func TestAppleSesionSDKElHostNoCargaElFrameworkDesdeUnaRutaExternaAlBundle(t *testing.T) {
	s, env := appleSDKConWidgets(t)
	r := s.ejecutar(t, append(env, "APPLE_SDK_PUENTE_ABSOLUTO=si")...)
	if r.Codigo == 0 {
		t.Fatal("not ok - el host carga el framework desde una ruta externa al bundle")
	}
	if !strings.Contains(r.Todo(), "el host Go no carga") {
		t.Errorf("falta el motivo:\n%s", r.Todo())
	}
}
