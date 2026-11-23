// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// Traducción de apps/fyne/scripts/apple-keychain-test.sh: E/S REAL del Keychain
// de macOS con dos apps firmadas (hub y rooms) que comparten grupo de llavero.
// Solo nombres de cuenta sintéticos y aleatorios; sin instalar la app, sin
// registrarla en LaunchServices, sin distribuir y sin leer producción.
//
// Solo corre dentro de apps/fyne/scripts/apple-keychain-ci.sh, que prepara el
// llavero efímero, la identidad Developer ID (SYTUE_KEYCHAIN_QA_IDENTITY) y los
// perfiles existentes (SYTUE_PERFILES_DIR). Fuera de ahí se salta; con la
// identidad preparada exige, como el original, el runner de CI y macOS.

const appleKeychainPlist = `import pathlib,plistlib,sys,tomllib
metadata=tomllib.loads(pathlib.Path(sys.argv[1]).read_text())
with open(sys.argv[2],'wb') as out:
 plistlib.dump({'CFBundleIdentifier':metadata['Details']['ID'],'CFBundleExecutable':'probe',
  'CFBundleName':'Sytue Keychain QA','CFBundlePackageType':'APPL',
  'CFBundleURLTypes':[{'CFBundleURLSchemes':[metadata['Development']['esquema']]}]},out)
`

func appleKeychainDebe(t *testing.T, que string, r guiontest.Resultado) guiontest.Resultado {
	t.Helper()
	if r.Codigo != 0 {
		t.Fatalf("%s: código %d\n%s", que, r.Codigo, r.Todo())
	}
	return r
}

func TestAppleKeychainDosAppsFirmadasCompartenElLlaveroReal(t *testing.T) {
	identidad := os.Getenv("SYTUE_KEYCHAIN_QA_IDENTITY")
	if identidad == "" {
		t.Skip("falta SYTUE_KEYCHAIN_QA_IDENTITY: esta prueba solo corre en el job de macOS que la prepara (apple-keychain-ci.sh)")
	}
	runnerTemp := os.Getenv("RUNNER_TEMP")
	if runnerTemp == "" {
		t.Fatal("RUNNER_TEMP: CI runner required")
	}
	if os.Getenv("SYTUE_PERFILES_DIR") == "" {
		t.Fatal("SYTUE_PERFILES_DIR: prepared existing profiles required")
	}
	if runtime.GOOS != "darwin" {
		t.Fatal("::error::signed Keychain QA requires macOS")
	}
	raiz := guiontest.Raiz(t)
	scripts := filepath.Join(raiz, "apps", "fyne", "scripts")
	qa, err := os.MkdirTemp(runnerTemp, "sytue-keychain.")
	if err != nil {
		t.Fatal(err)
	}
	nonce := appleKeychainDebe(t, "openssl rand", guiontest.Ejecutar(t, guiontest.Opciones{}, "openssl", "rand", "-hex", "32")).Texto()
	llavero := appleKeychainDebe(t, "llavero-efimero", guiontest.Guion(t, guiontest.Opciones{}, "apps/fyne/scripts/apple-firma.sh", "llavero-efimero")).Texto()
	t.Cleanup(func() {
		for _, app := range []string{"a.app", "b.app"} {
			sonda := filepath.Join(qa, app, "Contents", "MacOS", "probe")
			if _, err := os.Stat(sonda); err == nil {
				_ = guiontest.Ejecutar(t, guiontest.Opciones{}, sonda, "clean", nonce)
			}
		}
		_ = os.RemoveAll(qa)
	})
	appleKeychainDebe(t, "compilar la sonda", guiontest.Ejecutar(t, guiontest.Opciones{Dir: filepath.Join(raiz, "apps", "nucleo")},
		"go", "build", "-tags", "tienda,apple_keychain_qa,goolm", "-trimpath", "-o", filepath.Join(qa, "probe"), "./auth/testdata/apple-keychain"))
	m := regexp.MustCompile(`(?m)^const GrupoSuite = "([^.]*)\.`).FindStringSubmatch(guiontest.Leer(t, "apps/nucleo/ajustes/grupo_apple.go"))
	if m == nil {
		t.Fatal("no se encuentra el equipo en GrupoSuite")
	}
	equipo := m[1]
	for _, par := range [][2]string{{"a", "hub"}, {"b", "rooms"}} {
		slot, producto := par[0], par[1]
		bundle := filepath.Join(qa, slot+".app")
		appleKeychainDebe(t, "copiar la sonda", guiontest.Ejecutar(t, guiontest.Opciones{}, "bash", "-c",
			`mkdir -p "$1/Contents/MacOS" && cp "$2" "$1/Contents/MacOS/probe"`, "bash", bundle, filepath.Join(qa, "probe")))
		plist := filepath.Join(bundle, "Contents", "Info.plist")
		appleKeychainDebe(t, "Info.plist", guiontest.Ejecutar(t, guiontest.Opciones{Entrada: appleKeychainPlist}, "python3", "-",
			filepath.Join(raiz, "apps", "fyne", "cmd", producto, "FyneApp.toml"), plist))
		appid := appleKeychainDebe(t, "PlistBuddy", guiontest.Ejecutar(t, guiontest.Opciones{}, "/usr/libexec/PlistBuddy", "-c", "Print :CFBundleIdentifier", plist)).Texto()
		derechos := appleKeychainDebe(t, "derechos-sesion", guiontest.Guion(t, guiontest.Opciones{}, filepath.Join(scripts, "empaquetar.sh"),
			"derechos-sesion", producto, equipo, "developerid"))
		derechosPlist := filepath.Join(qa, slot+".plist")
		guiontest.Escribir(t, derechosPlist, derechos.Salida)
		perfil := appleKeychainDebe(t, "perfil-ruta", guiontest.Guion(t, guiontest.Opciones{}, filepath.Join(scripts, "apple-firma.sh"),
			"perfil-ruta", appid, "developerid")).Texto()
		appleKeychainDebe(t, "embedded.provisionprofile", guiontest.Ejecutar(t, guiontest.Opciones{}, "cp", perfil, filepath.Join(bundle, "Contents", "embedded.provisionprofile")))
		appleKeychainDebe(t, "codesign", guiontest.Ejecutar(t, guiontest.Opciones{}, "codesign", "--force", "--options", "runtime", "--timestamp=none",
			"--keychain", llavero, "--sign", identidad, "--entitlements", derechosPlist, bundle))
		appleKeychainDebe(t, "codesign --verify", guiontest.Ejecutar(t, guiontest.Opciones{}, "codesign", "--verify", "--strict", bundle))
	}
	for _, paso := range [][2]string{{"a", "seed-a"}, {"b", "check-b"}, {"a", "migrate-a"}, {"b", "after-b"}} {
		appleKeychainDebe(t, paso[1], guiontest.Ejecutar(t, guiontest.Opciones{}, filepath.Join(qa, paso[0]+".app", "Contents", "MacOS", "probe"), paso[1], nonce))
	}
	t.Log("KEYCHAIN_QA_REAL_SIGNED_TWO_APPS_PASS")
}
