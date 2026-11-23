// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// Un `go` de prueba: la herramienta de metadatos y `go build` apuntan lo que reciben;
// lo demás (`go version -m`, de arquitectura_de) va al Go de verdad.
const empaquetarGoDePrueba = `#!/usr/bin/env bash
set -euo pipefail
case "$1" in
-C)
  # go -C <fyne> run ./herramientas/metadatos -dir <app> -salida <fichero>
  printf '%s\n' "$@" >"$IOS_FIXTURE/metadatos.argv"
  printf '%s|%s\n' "${GOOS:-}" "${CC:-}" >"$IOS_FIXTURE/metadatos.entorno"
  [ "${IOS_FALLA_METADATOS:-no}" = no ] || exit 81
  salida=""; while [ "$#" -gt 0 ]; do [ "$1" = -salida ] && salida="$2"; shift; done
  echo 'package main // metadatos de prueba' >"$salida"
  echo "${IOS_ETIQUETAS:-}" ;;
build)
  [ -f fyne_metadata_init.go ] || exit 82
  printf '%s\n' "$@" >"$IOS_FIXTURE/build.argv"
  printf '%s\n' "$GOOS" "$GOARCH" "$CGO_ENABLED" "$CGO_CFLAGS" "$CGO_CXXFLAGS" "$CGO_LDFLAGS" "$GOFLAGS" >"$IOS_FIXTURE/entorno"
  [ "${IOS_FALLA_BUILD:-no}" = no ] || exit 92
  salida=""; while [ "$#" -gt 0 ]; do [ "$1" = -o ] && salida="$2"; shift; done
  cp "$IOS_BINARIO" "$salida" ;;
# ` + "`go version -m`" + ` (arquitectura_de) y lo demás, el Go de verdad.
*) exec "$GO_REAL" "$@" ;;
esac
`

// Un `xcrun` de prueba con el SDK, lipo, vtool y otool que mide verificar_ios_paquete.
const empaquetarXcrunDePrueba = `#!/usr/bin/env bash
set -euo pipefail
case "$1" in
--sdk)
  [ "$2" = iphoneos ] || exit 93
  case "$3" in
  --show-sdk-path) echo /sdk/iphoneos ;;
  -f) echo "/sdk/$4" ;;
  *) exit 94 ;;
  esac ;;
lipo)
  if [ "$2" = -archs ]; then echo "${IOS_LIPO_ARCHS:-arm64}"
  else cp "$IOS_BINARIO" "$6"; fi ;;
vtool)
  if [ "${IOS_MACHO_LEGACY:-no}" = si ]; then
    printf 'Load command 1\n      cmd LC_VERSION_MIN_IPHONEOS\n  version %s\n      sdk 27.1\n' "${IOS_MINIMO_REAL:-13.0}"
  else
    printf 'Load command 1\n      cmd LC_BUILD_VERSION\n  cmdsize 32\n platform IOS\n    minos %s\n      sdk 27.1\n   ntools 1\n     tool LD\n  version 1222.1\n' "${IOS_MINIMO_REAL:-13.0}"
  fi ;;
otool)
  if [ "$2" = -hv ]; then
    printf 'Mach header\n      magic cputype cpusubtype caps filetype ncmds sizeofcmds flags\nMH_MAGIC_64 ARM64 ALL 0x00 %s 37 4000 NOUNDEFS\n' "${IOS_TIPO_MACHO:-EXECUTE}"
  elif [ "${IOS_ALIAS_SWIFT:-no}" = si ]; then
    echo '@rpath/libswiftCoreMedia.dylib (compatibility version 1.0.0)'
  else
    echo '/System/Library/Frameworks/CoreMedia.framework/CoreMedia (compatibility version 1.0.0)'
  fi ;;
*) exit 95 ;;
esac
`

// empaquetarIOS es el fixture de compilar_ios y verificar_ios_paquete.
type empaquetarIOS struct {
	t                  *testing.T
	dir, arm64, goReal string
}

func (f empaquetarIOS) env(extra ...string) []string {
	return append([]string{
		"GO_REAL=" + f.goReal, empaquetarConPATH(empaquetarUnir(f.dir, "bin")), "FYNE=" + empaquetarUnir(f.dir, "fyne"),
		"IOS_FIXTURE=" + f.dir, "IOS_BINARIO=" + f.arm64,
	}, extra...)
}

// compilar: el Simulador y la fase de Xcode, compilar_ios desde el directorio de la app.
func (f empaquetarIOS) compilar(extra ...string) guiontest.Resultado {
	f.t.Helper()
	for _, n := range []string{"metadatos.argv", "build.argv", "entorno", "salida.bin"} {
		_ = os.Remove(empaquetarUnir(f.dir, n))
	}
	return guiontest.Bash(f.t, guiontest.Opciones{Env: f.env(extra...)},
		`set -euo pipefail; source "$1/funciones.sh"; cd "$1/fuentes"; compilar_ios iphoneos "$1/salida.bin"`, f.dir)
}

// verificar: el paquete de TestFlight, lo que deja `fyne package` (o el FAT que se dé).
func (f empaquetarIOS) verificar(ejecutable string, extra ...string) guiontest.Resultado {
	f.t.Helper()
	empaquetarCopiar(f.t, ejecutable, empaquetarUnir(f.dir, "Sytue Rooms.app/main"))
	return guiontest.Bash(f.t, guiontest.Opciones{Env: f.env(extra...)},
		`set -euo pipefail; source "$1/funciones.sh"; verificar_ios_paquete "$1/Sytue Rooms.app"`, f.dir)
}

// empaquetarCopiar es `cp`: el contenido y el modo del original.
func empaquetarCopiar(t *testing.T, de, a string) {
	t.Helper()
	info, err := os.Stat(de)
	if err != nil {
		t.Fatal(err)
	}
	guiontest.Escribir(t, a, guiontest.Leer(t, de))
	if err := os.Chmod(a, info.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
}

func (f empaquetarIOS) leer(n string) string { return empaquetarLeer(empaquetarUnir(f.dir, n)) }

func empaquetarUltimas2(r guiontest.Resultado) string {
	l := strings.Split(strings.TrimRight(r.Todo(), "\n"), "\n")
	return strings.Join(l[max(0, len(l)-2):], "\n")
}

// iOS POR EL CAMINO OFICIAL (2026-11-06): fyne.io/tools 1.7.4 no admite `fyne build`
// para iOS. El paquete de TestFlight lo compila `fyne package` y aquí solo se verifica
// (verificar_ios_paquete); el Simulador y la fase de Xcode compilan con `go build` y los
// metadatos de herramientas/metadatos (compilar_ios). Se ejecutan las funciones reales
// con un `go`, un `xcrun` y un SDK de prueba; el mínimo del fixture es 16 para detectar
// un 15 copiado.
func empaquetarFixtureIOS(t *testing.T) empaquetarIOS {
	t.Helper()
	d := empaquetarDirs(t)
	_, _, arm64 := empaquetarBinarios(t)
	goReal, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	f := empaquetarIOS{t: t, dir: empaquetarUnir(t.TempDir(), "ios-enlace"), arm64: arm64, goReal: goReal}
	guiontest.Escribir(t, empaquetarUnir(f.dir, "fyne/xcode/Sytue.xcconfig"), "IPHONEOS_DEPLOYMENT_TARGET = 16.0\n")
	guiontest.Escribir(t, empaquetarUnir(f.dir, "fuentes/FyneApp.toml"), "[Details]\nName = \"Sytue Rooms\"\nID = \"eu.sytue.rooms\"\n")
	empaquetarEscribirPlist(t, empaquetarUnir(f.dir, "Sytue Rooms.app/Info.plist"), map[string]any{"CFBundleExecutable": "main"}, false)
	empaquetarEjecutable(t, empaquetarUnir(f.dir, "bin/go"), empaquetarGoDePrueba)
	empaquetarEjecutable(t, empaquetarUnir(f.dir, "bin/xcrun"), empaquetarXcrunDePrueba)
	// Sin copia de implementación: se cargan las funciones del empaquetador de verdad.
	funciones := empaquetarExtraerFunciones(guiontest.Leer(t, d.guion), false, "metadato", "binario_de", "arquitectura_de", "verificar_arquitectura",
		"minimo_ios", "enlace_widgets_apple", "permiso_enlace_widgets_apple", "compilar_ios", "verificar_minimo_ios", "verificar_ios_paquete")
	guiontest.Escribir(t, empaquetarUnir(f.dir, "funciones.sh"), funciones)
	if !empaquetarHay(funciones, `^compilar_ios\(\) \{`) || !empaquetarHay(funciones, `^verificar_ios_paquete\(\) \{`) {
		t.Fatal("la prueba no encuentra compilar_ios o verificar_ios_paquete en empaquetar.sh")
	}
	return f
}

func TestEmpaquetarCompilarIOS(t *testing.T) {
	f := empaquetarFixtureIOS(t)
	metadatos := empaquetarUnir(f.dir, "fuentes/fyne_metadata_init.go")

	if r := f.compilar("GOOS=linux"); r.Codigo != 0 {
		t.Errorf("compilar_ios no compila el binario suelto de iOS: %s", empaquetarUltimas2(r))
	}
	empaquetarComprobarCompilacionIOS(t, f)

	if r := f.compilar("IOS_ETIQUETAS=release,migrated_fynedo"); r.Codigo != 0 || !empaquetarHayLinea(f.leer("build.argv"), "goolm,release,migrated_fynedo") {
		t.Error("compilar_ios no pasa a go build las etiquetas que daba fyne build")
	}
	if r := f.compilar("IOS_FALLA_METADATOS=si"); r.Codigo == 0 {
		t.Error("compilar_ios compila sin metadatos cuando la herramienta falla")
	}
	if empaquetarExiste(empaquetarUnir(f.dir, "build.argv")) {
		t.Error("compilar_ios llega a go build sin metadatos")
	}
	if r := f.compilar("IOS_FALLA_BUILD=si"); r.Codigo == 0 {
		t.Error("compilar_ios sigue aunque falle go build")
	}
	if empaquetarExiste(metadatos) {
		t.Error("un go build fallido deja fyne_metadata_init.go en el árbol")
	}
	guiontest.Escribir(t, metadatos, "package main // de otro\n")
	if r := f.compilar(); r.Codigo == 0 {
		t.Error("compilar_ios pisa un fyne_metadata_init.go que no es suyo")
	}
	if !strings.Contains(empaquetarLeer(metadatos), "de otro") {
		t.Error("compilar_ios borró un fyne_metadata_init.go ajeno")
	}
	_ = os.Remove(metadatos)
	r := guiontest.Bash(t, guiontest.Opciones{Env: []string{empaquetarConPATH(empaquetarUnir(f.dir, "bin")), "FYNE=" + empaquetarUnir(f.dir, "fyne")}},
		`source "$1/funciones.sh"; cd "$1/fuentes"; compilar_ios iphoneos "$1/x" --release`, f.dir)
	if r.Codigo == 0 {
		t.Error("compilar_ios acepta opciones de fyne build que ya no usa")
	}

	// El puente de WidgetKit se enlaza aunque la ruta de sus productos lleve espacios.
	productos := empaquetarUnir(f.dir, "productos con espacios")
	if err := os.MkdirAll(empaquetarUnir(productos, "SytueWidgetBridge.framework"), 0o750); err != nil {
		t.Fatal(err)
	}
	if r := f.compilar("SYTUE_WIDGET_PRODUCTOS=" + productos); r.Codigo != 0 {
		t.Errorf("iOS no enlaza el puente WidgetKit: %s", empaquetarUltimas2(r))
	}
	if l := strings.Split(f.leer("entorno"), "\n"); len(l) < 6 ||
		!strings.Contains(l[5], `-F "`) || !strings.Contains(l[5], `productos con espacios"`) ||
		!strings.Contains(l[5], "-Wl,-needed_framework,SytueWidgetBridge") || !strings.Contains(l[5], "-Wl,-rpath,@executable_path/Frameworks") {
		t.Errorf("el enlace Go descarta el puente público o no puede encontrarlo: %q", l)
	}
	if err := os.Remove(empaquetarUnir(productos, "SytueWidgetBridge.framework")); err != nil {
		t.Fatal(err)
	}
	if r := f.compilar("SYTUE_WIDGET_PRODUCTOS=" + productos); r.Codigo == 0 {
		t.Error("iOS compila un host con WidgetKit sin haber construido su framework")
	}
}

// El binario suelto de iOS conserva sus metadatos, su mínimo y su destino.
func empaquetarComprobarCompilacionIOS(t *testing.T, f empaquetarIOS) {
	t.Helper()
	mal := func(detalle any) {
		t.Errorf("el binario suelto de iOS perdería sus metadatos, su mínimo o su destino: %v", detalle)
	}
	meta := strings.Split(strings.TrimRight(f.leer("metadatos.argv"), "\n"), "\n")
	valor := func(clave string) string {
		if i := slices.Index(meta, clave); i >= 0 && i+1 < len(meta) {
			return meta[i+1]
		}
		return ""
	}
	if len(meta) < 4 || !slices.Equal(meta[:4], []string{"-C", empaquetarUnir(f.dir, "fyne"), "run", "./herramientas/metadatos"}) {
		mal(meta)
	}
	if valor("-dir") != empaquetarUnir(f.dir, "fuentes") || valor("-salida") != empaquetarUnir(f.dir, "fuentes", "fyne_metadata_init.go") {
		mal(meta)
	}
	// La herramienta corre con el Go del anfitrión, no con el entorno de iOS.
	if strings.TrimSpace(f.leer("metadatos.entorno")) != "|" {
		mal(f.leer("metadatos.entorno"))
	}
	args := strings.Split(strings.TrimRight(f.leer("build.argv"), "\n"), "\n")
	if !slices.Equal(args, []string{"build", "-tags", "goolm", "-o", empaquetarUnir(f.dir, "salida.bin"), "."}) {
		mal(args)
	}
	env := strings.Split(strings.TrimRight(f.leer("entorno"), "\n"), "\n")
	if len(env) < 7 || !slices.Equal(env[:3], []string{"ios", "arm64", "1"}) {
		mal(env)
		return
	}
	for _, flags := range env[3:6] {
		if !strings.Contains(flags, "-miphoneos-version-min=16.0") || !strings.Contains(flags, "-arch arm64") {
			mal(env[3:6])
		}
	}
	if !strings.Contains(env[6], "-trimpath") {
		mal(env[6])
	}
	if empaquetarExiste(empaquetarUnir(f.dir, "fuentes", "fyne_metadata_init.go")) {
		mal("el fichero de metadatos se queda en el árbol")
	}
}

// El paquete de TestFlight: el ejecutable de `fyne package`, enlazado con iOS 13.0 por la
// CLI. Se admite un mínimo entre 13.0 y el canónico (16 en el fixture), nunca más alto.
func TestEmpaquetarVerificarPaqueteIOS(t *testing.T) {
	f := empaquetarFixtureIOS(t)
	if r := f.verificar(f.arm64); r.Codigo != 0 {
		t.Errorf("verificar_ios_paquete rechaza el ejecutable de fyne package: %s", empaquetarUltimas2(r))
	}
	if r := f.verificar(f.arm64, "IOS_MACHO_LEGACY=si"); r.Codigo != 0 {
		t.Error("iOS rechaza el mínimo válido de LC_VERSION_MIN_IPHONEOS")
	}
	if r := f.verificar(f.arm64, "IOS_MINIMO_REAL=16.0"); r.Codigo != 0 {
		t.Error("iOS rechaza un mínimo igual al canónico")
	}
	for _, minimo := range []string{"17.0", "7.0"} {
		r := f.verificar(f.arm64, "IOS_MINIMO_REAL="+minimo)
		switch {
		case r.Codigo == 0:
			t.Errorf("iOS publica un Mach-O con mínimo %s (fuera de 13.0..16.0)", minimo)
		case !strings.Contains(r.Todo(), "mínimo fuera de 13.0..16.0"):
			t.Errorf("el rechazo del mínimo %s no explica qué se ha medido", minimo)
		}
	}
	for _, tipo := range []string{"OBJECT", "DYLIB"} {
		r := f.verificar(f.arm64, "IOS_TIPO_MACHO="+tipo)
		switch {
		case r.Codigo == 0:
			t.Errorf("iOS acepta un Mach-O %s como ejecutable de la app", tipo)
		case !strings.Contains(r.Todo(), "no es un ejecutable MH_EXECUTE"):
			t.Errorf("iOS no comprueba el tipo Mach-O %s", tipo)
		}
	}
	r := f.verificar(f.arm64, "IOS_ALIAS_SWIFT=si")
	switch {
	case r.Codigo == 0:
		t.Error("iOS permite publicar el alias Swift CoreMedia sin soporte Swift")
	case !strings.Contains(r.Todo(), "enlaza Swift por @rpath"):
		t.Error("el rechazo del alias Swift no se ha ejecutado")
	}
	// El FAT de una sola arquitectura que deja la CLI se adelgaza; uno con más, no pasa.
	fat := empaquetarUnir(f.dir, "fat")
	guiontest.Escribir(t, fat, "\xca\xfe\xba\xbefat")
	if r := f.verificar(fat); r.Codigo != 0 {
		t.Errorf("verificar_ios_paquete no acepta el FAT arm64 de fyne package: %s", empaquetarUltimas2(r))
	} else if f.leer("Sytue Rooms.app/main") != empaquetarLeer(f.arm64) {
		t.Error("verificar_ios_paquete no deja el ejecutable en su arm64")
	}
	if r := f.verificar(fat, "IOS_LIPO_ARCHS=arm64 x86_64"); r.Codigo == 0 {
		t.Error("verificar_ios_paquete publica un FAT con algo más que arm64")
	}
}
