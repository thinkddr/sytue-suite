// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"archive/zip"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// --- 10. El Simulador de iOS, sin cuenta de Apple.
// Para repetir el barrido de iOS: un .app que el Simulador lanza sin Team ID ni perfil.
// Su Info.plist lo escribe empaquetar.sh y aquí se lee con plistlib: el identificador es
// el de la tienda y el ejecutable el que compila el guion, o el Simulador instala y no
// abre. Visto en rojo cambiando el CFBundleExecutable y mandando iossimulator por la
// rama de `ios`.
func TestEmpaquetarPlistDelSimulador(t *testing.T) {
	d := empaquetarDirs(t)
	for _, a := range []string{"rooms", "hub"} {
		id := empaquetarID(t, empaquetarUnir(d.fyne, "cmd", a, "FyneApp.toml"))
		p, err := empaquetarPlist(t, empaquetar(t, "plist-simulador", a).Todo())
		switch {
		case err != nil:
			t.Errorf("el Info.plist del Simulador de %s no vale: %s", a, empaquetarUltima(err.Error()))
		case p["CFBundleIdentifier"] != id:
			t.Errorf("el Info.plist del Simulador de %s no vale: %v", a, p["CFBundleIdentifier"])
		case p["CFBundleExecutable"] != "sytue-"+a:
			t.Errorf("el Info.plist del Simulador de %s no vale: %v", a, p["CFBundleExecutable"])
		case p["LSRequiresIPhoneOS"] != true:
			t.Errorf("el Info.plist del Simulador de %s no vale: LSRequiresIPhoneOS %v", a, p["LSRequiresIPhoneOS"])
		}
	}
}

// Fuera de un Mac tiene que fallar por lo suyo —no hay SDK del Simulador— y no por la
// rama de `ios`, que pide la cuenta. En un Mac compilaría de verdad, y eso no va aquí.
func TestEmpaquetarIOSSimuladorVaPorSuRama(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("en un Mac compilaría de verdad, y eso no va aquí")
	}
	// Con una CLI de mentira: la de verdad no hace falta para llegar a esa rama.
	cli := t.TempDir()
	empaquetarEjecutable(t, empaquetarUnir(cli, "fyne"), "#!/bin/sh\nexit 0\n")
	salida := empaquetarJunto(empaquetarCon(t, []string{empaquetarConPATH(cli), "SYTUE_IOS_PROFILE="}, "iossimulator", "demo"))
	if !strings.Contains(salida, "el Simulador de iOS solo existe en un Mac") {
		t.Errorf("empaquetar.sh iossimulator no va por su rama (¿pide la cuenta de Apple?): %s", empaquetarPrimera(salida))
	}
}

// El proyecto temporal que genera la CLI de Fyne también recibe el mínimo de iOS, y su
// xcodebuild no firma.
func TestEmpaquetarIOSMinimoYXcodebuildSinFirmar(t *testing.T) {
	// Una CLI de prueba observa el entorno justo antes de compilar y termina ahí: no
	// necesita un Mac, certificados ni SDK. Sin el xcconfig, Xcode 27 rechaza iOS 9.
	cli := t.TempDir()
	empaquetarEjecutable(t, empaquetarUnir(cli, "uname"), "#!/bin/sh\ncase \"$1\" in -s) echo Darwin ;; -m) echo arm64 ;; esac\n")
	empaquetarEjecutable(t, empaquetarUnir(cli, "xcodebuild"), "#!/bin/sh\nexit 99\n")
	empaquetarEjecutable(t, empaquetarUnir(cli, "fyne"), `#!/bin/sh
[ "$1" = package ] || exit 99
[ -f "${XCODE_XCCONFIG_FILE:-}" ] || exit 98
[ "$(cat "$XCODE_XCCONFIG_FILE")" = 'IPHONEOS_DEPLOYMENT_TARGET = 15.0' ] || exit 97
echo minimo-ios-15-recibido
exit 71
`)
	env := make([]string, 0, 5)
	env = append(env, empaquetarConPATH(cli), "GOPRIVATE=github.com/thinkddr/*", "SYTUE_IOS_PROFILE=perfil-de-prueba", "SYTUE_IOS_IDENTIDAD=identidad-de-prueba")
	salida := empaquetarJunto(empaquetarCon(t, env, "ios", "demo"))
	if !empaquetarHayLinea(salida, "minimo-ios-15-recibido") {
		t.Errorf("el empaquetado iOS no pasa su mínimo a Xcode: %s", empaquetarUltima(salida))
	}

	// En el Mac propio el llavero efímero NO entra en `security list-keychains`, así que
	// `xcodebuild` de la CLI de Fyne debe construir el envoltorio `.app` SIN firmar
	// (`CODE_SIGNING_ALLOWED=NO`, sin `DEVELOPMENT_TEAM=VHC5QW7BZ3`) y `firmar_ipa` embebe
	// el perfil `0644`, deriva los derechos base (`application-identifier`, `team-identifier`,
	// `get-task-allow=false`, `beta-reports-active=true`) y firma con `codesign`.
	empaquetarEjecutable(t, empaquetarUnir(cli, "xcrun"), "#!/usr/bin/env bash\nprintf '%s\\n' \"$@\" >\"$XCRUN_ARGV\"\nexit 72\n")
	empaquetarEjecutable(t, empaquetarUnir(cli, "fyne"), "#!/usr/bin/env bash\n[ \"$1\" = package ] || exit 99\n"+
		"exec xcrun xcodebuild -configuration Release -project /tmp/main.xcodeproj -allowProvisioningUpdates DEVELOPMENT_TEAM=VHC5QW7BZ3\n")
	argv := empaquetarUnir(t.TempDir(), "cli-ios-xcrun.argv")
	empaquetarCon(t, append(env, "XCRUN_ARGV="+argv), "ios", "demo")
	a := empaquetarLeer(argv)
	if a == "" || empaquetarHayLinea(a, "DEVELOPMENT_TEAM=VHC5QW7BZ3") || !empaquetarHayLinea(a, "CODE_SIGNING_ALLOWED=NO") ||
		!empaquetarHayLinea(a, "CODE_SIGNING_REQUIRED=NO") || !empaquetarHayLinea(a, "DEVELOPMENT_TEAM=") {
		t.Errorf("empaquetar.sh ios deja que xcodebuild de la CLI pida el certificado en list-keychains:\n%s", a)
	}
}

// El zip de reserva del original, por si el sistema no trae `zip`.
const empaquetarZipDeReserva = `#!/usr/bin/env python3
import os, sys, zipfile
args = [a for a in sys.argv[1:] if not a.startswith('-')]
destino, raices = args[0], args[1:]
with zipfile.ZipFile(destino, 'w', zipfile.ZIP_DEFLATED) as zf:
    for raiz in raices:
        for base, dirs, ficheros in os.walk(raiz):
            dirs.sort()
            ficheros.sort()
            st = os.lstat(base)
            info = zipfile.ZipInfo(base.rstrip('/') + '/')
            info.external_attr = ((st.st_mode & 0xFFFF) << 16) | 0x10
            zf.writestr(info, b'')
            for nombre in ficheros:
                ruta = os.path.join(base, nombre)
                st = os.lstat(ruta)
                info = zipfile.ZipInfo(ruta)
                info.external_attr = (st.st_mode & 0xFFFF) << 16
                with open(ruta, 'rb') as f:
                    zf.writestr(info, f.read())
`

// `firmar_ipa` sobre un `.app` sin firmar salido de `xcodebuild` (`CODE_SIGNING_ALLOWED=NO`)
// y con `Info.plist` en `0600` como escribe `fyne`: copia el perfil en `0644`, extrae los
// derechos base + `beta-reports-active` + sesión + `associated-domains`, normaliza permisos
// y produce un `.ipa` verificable.
func TestEmpaquetarFirmarIPASinFirmaPrevia(t *testing.T) {
	d := empaquetarDirs(t)
	ipa := empaquetarUnir(t.TempDir(), "ipa-sin-firma")
	app := empaquetarUnir(ipa, "abierto/Payload/Sytue Rooms.app")
	for _, dir := range []string{"bin", "aqui", "raiz/apps/nucleo/ajustes", "abierto/Payload/Sytue Rooms.app", "fuentes/rooms", "dist"} {
		if err := os.MkdirAll(empaquetarUnir(ipa, dir), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	guiontest.Escribir(t, empaquetarUnir(ipa, "raiz/apps/nucleo/ajustes/grupo_apple.go"),
		"package ajustes\nconst GrupoSuite = \"VHC5QW7BZ3.eu.sytue.suite\"\nconst GrupoContenedorSuite = \"group.eu.sytue.suite\"\n")
	guiontest.Escribir(t, empaquetarUnir(ipa, "fuentes/rooms/FyneApp.toml"),
		"[Details]\nName = \"Sytue Rooms\"\nID = \"eu.sytue.rooms\"\nVersion = \"3.15.3\"\nBuild = 56\n[Development]\nesquema = \"sytue-rooms\"\nenlaces = \"rooms\"\n")
	empaquetarEscribirPlist(t, empaquetarUnir(app, "Info.plist"), map[string]any{"CFBundleExecutable": "main"}, false)
	perfil := empaquetarUnir(ipa, "perfil.mobileprovision")
	empaquetarEscribirPlist(t, perfil, map[string]any{
		"TeamIdentifier": []string{"VHC5QW7BZ3"},
		"Platform":       []string{"iOS"},
		"Entitlements": map[string]any{
			"application-identifier":                 "VHC5QW7BZ3.eu.sytue.rooms",
			"com.apple.developer.team-identifier":    "VHC5QW7BZ3",
			"get-task-allow":                         false,
			"beta-reports-active":                    true,
			"com.apple.security.application-groups":  []string{"group.eu.sytue.suite"},
			"keychain-access-groups":                 []string{"VHC5QW7BZ3.*"},
			"com.apple.developer.associated-domains": "*",
		},
	}, false)
	for _, f := range []string{empaquetarUnir(app, "Info.plist"), perfil} {
		if err := os.Chmod(f, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	guiontest.Escribir(t, empaquetarUnir(app, "main"), "macho-inicial")
	empaquetarEjecutable(t, empaquetarUnir(ipa, "aqui/apple-firma.sh"), `#!/usr/bin/env bash
set -euo pipefail
case "$1" in
perfil-ruta)
  [ "$2 $3" = "eu.sytue.rooms ios" ] || exit 1
  printf '%s\n' "$IPA_SIN_FIRMA/perfil.mobileprovision" ;;
verificar-perfil-grupo)
  [ -f "$2" ] && [ "$3 $4" = "eu.sytue.rooms ios" ] ;;
*) exit 1 ;;
esac
`)
	empaquetarEjecutable(t, empaquetarUnir(ipa, "aqui/apple_perfil.py"), `#!/usr/bin/env python3
import pathlib, sys
assert sys.argv[1] == 'decodificar' and sys.argv[3] == 'ios'
sys.stdout.buffer.write(pathlib.Path(sys.argv[2]).read_bytes())
`)
	empaquetarEjecutable(t, empaquetarUnir(ipa, "bin/codesign"), `#!/usr/bin/env bash
set -euo pipefail
if [ "$1" = -d ]; then exit 1; fi
if [ "$1" = --verify ]; then exit 0; fi
while [ "$#" -gt 0 ]; do
  if [ "$1" = --entitlements ]; then cp "$2" "$IPA_SIN_FIRMA/firmados.plist"; break; fi
  shift
done
`)
	if _, err := exec.LookPath("zip"); err != nil {
		empaquetarEjecutable(t, empaquetarUnir(ipa, "bin/zip"), empaquetarZipDeReserva)
	}
	funciones := empaquetarExtraerFunciones(guiontest.Leer(t, d.guion), true, "metadato", "grupos_sesion_apple", "derechos_sesion", "verificar_permisos_apple", "firmar_ipa")
	funciones += "verificar_ios_paquete() { :; }\nincorporar_widgets_apple() { mkdir -p \"$1/../../SwiftSupport/iphoneos\"; }\n"
	guiontest.Escribir(t, empaquetarUnir(ipa, "funciones.sh"), funciones)
	for _, dir := range []string{empaquetarUnir(ipa, "abierto"), empaquetarUnir(ipa, "abierto/Payload")} {
		if err := os.Chmod(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	r := guiontest.Bash(t, guiontest.Opciones{Env: []string{
		empaquetarConPATH(empaquetarUnir(ipa, "bin")), "IPA_SIN_FIRMA=" + ipa,
		"RAIZ=" + empaquetarUnir(ipa, "raiz"), "AQUI=" + empaquetarUnir(ipa, "aqui"),
		"SYTUE_IOS_IDENTIDAD=Apple Distribution: Prueba (VHC5QW7BZ3)", "version=3.15.3", "build=56",
	}},
		`set -euo pipefail; source "$1/funciones.sh"; firmar_ipa "$1/abierto" "$1/abierto/Payload/Sytue Rooms.app" "$1/dist/sytue-rooms-ios.ipa" eu.sytue.rooms rooms.sytue.eu "$1/fuentes/rooms"`, ipa)
	if r.Codigo != 0 {
		l := strings.Split(strings.TrimRight(r.Todo(), "\n"), "\n")
		t.Fatalf("firmar_ipa rechaza un .app sin firma previa de xcodebuild: %s", strings.Join(l[max(0, len(l)-2):], "\n"))
	}
	derechos, err := empaquetarPlist(t, empaquetarLeer(empaquetarUnir(ipa, "firmados.plist")))
	if err != nil || !empaquetarIgualJSON(derechos, `{
		"application-identifier": "VHC5QW7BZ3.eu.sytue.rooms",
		"com.apple.developer.team-identifier": "VHC5QW7BZ3",
		"get-task-allow": false,
		"beta-reports-active": true,
		"com.apple.security.application-groups": ["group.eu.sytue.suite"],
		"keychain-access-groups": ["VHC5QW7BZ3.eu.sytue.rooms", "VHC5QW7BZ3.eu.sytue.suite"],
		"com.apple.developer.associated-domains": ["applinks:rooms.sytue.eu"]}`) {
		t.Errorf("firmar_ipa no graba los derechos o permisos esperados en el IPA: %v %v", derechos, err)
	}
	empaquetarComprobarIPA(t, empaquetarUnir(ipa, "dist/sytue-rooms-ios.ipa"))
}

// empaquetarComprobarIPA: lleva el perfil embebido, no lleva SwiftSupport, y todo es
// legible (y los directorios atravesables) por cualquiera.
func empaquetarComprobarIPA(t *testing.T, ruta string) {
	t.Helper()
	zr, err := zip.OpenReader(ruta)
	if err != nil {
		t.Fatalf("firmar_ipa no graba los derechos o permisos esperados en el IPA: %v", err)
	}
	defer func() { _ = zr.Close() }()
	nombres := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		nombres = append(nombres, f.Name)
		modo := (f.ExternalAttrs >> 16) & 0o777
		minimo := uint32(0o444)
		if strings.HasSuffix(f.Name, "/") {
			minimo = 0o555
		}
		if modo&minimo != minimo {
			t.Errorf("firmar_ipa no graba los derechos o permisos esperados en el IPA: %s %o", f.Name, modo)
		}
	}
	if !slices.Contains(nombres, "Payload/Sytue Rooms.app/embedded.mobileprovision") {
		t.Errorf("firmar_ipa no graba los derechos o permisos esperados en el IPA: %v", nombres)
	}
	for _, n := range nombres {
		if strings.HasPrefix(n, "SwiftSupport") {
			t.Errorf("firmar_ipa no graba los derechos o permisos esperados en el IPA: %v", nombres)
		}
	}
}
