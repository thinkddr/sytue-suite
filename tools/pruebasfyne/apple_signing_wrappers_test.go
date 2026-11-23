// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// Traducción de apps/fyne/scripts/apple_signing_wrappers-test.py: ejercita los
// argumentos de los envoltorios de firma de producción
// (.github/actions/apple/*-existing-only.sh) con fronteras nativas sintéticas.

type appleEnvoltorios struct {
	raiz, nativo, propio string
	env                  []string
}

// appleEjecutable escribe un fichero ejecutable (0700, como el original).
func appleEjecutable(t *testing.T, ruta, contenido string) {
	t.Helper()
	guiontest.Escribir(t, ruta, contenido)
	if err := os.Chmod(ruta, 0o700); err != nil { //nolint:gosec // G302: el envoltorio copiado tiene que ser ejecutable.
		t.Fatal(err)
	}
}

func appleNuevosEnvoltorios(t *testing.T) *appleEnvoltorios {
	t.Helper()
	raiz := t.TempDir()
	e := &appleEnvoltorios{raiz: raiz, nativo: filepath.Join(raiz, "native")}
	for _, nombre := range []string{"xcrun", "codesign", "productsign", "productbuild", "security"} {
		guiontest.Doble(t, e.nativo, nombre, `exec python3 -c 'import json,sys; print(json.dumps(sys.argv[1:]))' "$@"`)
	}
	ayudante := filepath.Join(raiz, "helper.py")
	guiontest.Escribir(t, ayudante, "import os\nprint(os.environ['OWN_FIXTURE'])\n")
	e.propio = filepath.Join(raiz, `own space $value".keychain-db`)
	e.env = []string{
		"SYTUE_APPLE_KEYCHAIN_HELPER=" + ayudante, "OWN_FIXTURE=" + e.propio,
		"FAKE_TOOL_DIR=" + e.nativo, "SYTUE_APPLE_JOB_DIR=" + raiz,
	}
	// Solo se sustituye la frontera exec nativa en una copia privada de prueba.
	// Producción no tiene override ni flag de prueba que redirija los nativos.
	xcrun := guiontest.Leer(t, ".github/actions/apple/xcrun-existing-only.sh")
	appleEjecutable(t, filepath.Join(raiz, "xcrun"), strings.ReplaceAll(xcrun, "/usr/bin/xcrun", filepath.Join(e.nativo, "xcrun")))
	firma := strings.ReplaceAll(guiontest.Leer(t, ".github/actions/apple/sign-existing-only.sh"), `"/usr/bin/$tool"`, `"$FAKE_TOOL_DIR/$tool"`)
	for _, nombre := range []string{"codesign", "productsign", "productbuild"} {
		appleEjecutable(t, filepath.Join(raiz, nombre), firma)
	}
	security := guiontest.Leer(t, ".github/actions/apple/security-existing-only.sh")
	appleEjecutable(t, filepath.Join(raiz, "security"), strings.ReplaceAll(security, "/usr/bin/security", filepath.Join(e.nativo, "security")))
	return e
}

func (e *appleEnvoltorios) ejecutar(t *testing.T, nombre string, args ...string) guiontest.Resultado {
	t.Helper()
	return e.bash(t, filepath.Join(e.raiz, nombre), args...)
}

func (e *appleEnvoltorios) bash(t *testing.T, ruta string, args ...string) guiontest.Resultado {
	t.Helper()
	return guiontest.Ejecutar(t, guiontest.Opciones{Dir: e.raiz, Env: e.env}, "/bin/bash", append([]string{ruta}, args...)...)
}

func appleArgv(t *testing.T, r guiontest.Resultado) []string {
	t.Helper()
	var argv []string
	if err := json.Unmarshal([]byte(r.Salida), &argv); err != nil {
		t.Fatalf("la frontera nativa no recibió argv (código %d): %q\n%s", r.Codigo, r.Salida, r.Error)
	}
	return argv
}

func appleMismoArgv(t *testing.T, que string, r guiontest.Resultado, quiero ...string) {
	t.Helper()
	if r.Codigo != 0 {
		t.Fatalf("%s: código %d\n%s", que, r.Codigo, r.Error)
	}
	if got := appleArgv(t, r); !reflect.DeepEqual(got, quiero) {
		t.Errorf("%s:\n  argv %q\n  quiero %q", que, got, quiero)
	}
}

// appleBloqueado: código 64 y nada en stdout (ninguna herramienta nativa llamada).
func appleBloqueado(t *testing.T, que string, r guiontest.Resultado) {
	t.Helper()
	if r.Codigo != 64 || r.Salida != "" {
		t.Errorf("%s: se esperaba el rechazo 64 sin salida; código %d, salida %q", que, r.Codigo, r.Salida)
	}
}

func TestAppleEnvoltoriosLosFirmantesDirectosSoloUsanElLlaveroPropioYConservanArgumentos(t *testing.T) {
	e := appleNuevosEnvoltorios(t)
	for _, nombre := range []string{"codesign", "productsign", "productbuild"} {
		t.Run(nombre, func(t *testing.T) {
			r := e.ejecutar(t, nombre, "--sign", "existing identity", "a space.pkg")
			appleMismoArgv(t, nombre, r, "--keychain", e.propio, "--sign", "existing identity", "a space.pkg")
			r = e.ejecutar(t, nombre, "--sign", "existing", "--keychain", "/fixture/personal")
			if r.Codigo == 0 || r.Salida != "" {
				t.Errorf("un --keychain ajeno se aceptó: código %d, salida %q", r.Codigo, r.Salida)
			}
		})
	}
}

func TestAppleEnvoltoriosElAlcancePropioNoSeDuplicaYLaInspeccionSeConserva(t *testing.T) {
	e := appleNuevosEnvoltorios(t)
	argv := appleArgv(t, e.ejecutar(t, "codesign", "-s", "existing", "--keychain", e.propio, "app"))
	n := 0
	for _, a := range argv {
		if a == "--keychain" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("--keychain aparece %d veces: %q", n, argv)
	}
	appleMismoArgv(t, "codesign --verify", e.ejecutar(t, "codesign", "--verify", "--strict", "app"), "--verify", "--strict", "app")
}

func TestAppleEnvoltoriosLasOpcionesCortasPegadasYCombinadasDeCodesignTienenAlcance(t *testing.T) {
	e := appleNuevosEnvoltorios(t)
	for _, args := range [][]string{{"-sexisting identity", "app"}, {"-fs", "existing identity", "app"}, {"-fvsexisting identity", "app"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			appleMismoArgv(t, "firma", e.ejecutar(t, "codesign", args...), append([]string{"--keychain", e.propio}, args...)...)
			appleBloqueado(t, "con --keychain ajeno", e.ejecutar(t, "codesign", append(args, "--keychain", "/fixture/personal")...))
		})
	}
}

// appleCodesignDesdeShell repite lo que hace Xcode: pasa OTHER_CODE_SIGN_FLAGS
// por un shell. shlex por sí solo no deshace el escape de un `$` literal entre
// comillas dobles.
func appleCodesignDesdeShell(t *testing.T, e *appleEnvoltorios, banderas string) guiontest.Resultado {
	t.Helper()
	return guiontest.Ejecutar(t, guiontest.Opciones{Dir: e.raiz, Env: e.env}, "/bin/bash", "-c",
		"exec '"+filepath.Join(e.nativo, "codesign")+"' "+banderas)
}

func TestAppleEnvoltoriosXcodeQuitaActualizacionesRechazaRegistroYConservaOtrasBanderas(t *testing.T) {
	e := appleNuevosEnvoltorios(t)
	r := e.ejecutar(t, "xcrun", "--sdk", "iphoneos", "xcodebuild", "-archive", "-allowProvisioningUpdates",
		"OTHER_CODE_SIGN_FLAGS=--timestamp --options runtime")
	if r.Codigo != 0 {
		t.Fatalf("código %d\n%s", r.Codigo, r.Error)
	}
	argv := appleArgv(t, r)
	if len(argv) < 4 || !reflect.DeepEqual(argv[:4], []string{"--sdk", "iphoneos", "xcodebuild", "-archive"}) {
		t.Errorf("cabecera de argv inesperada: %q", argv)
	}
	var banderas []string
	for _, a := range argv {
		if strings.HasPrefix(a, "OTHER_CODE_SIGN_FLAGS=") {
			banderas = append(banderas, a)
		}
	}
	if len(banderas) != 1 {
		t.Fatalf("se esperaba un único OTHER_CODE_SIGN_FLAGS: %q", argv)
	}
	_, valor, _ := strings.Cut(banderas[0], "=")
	appleMismoArgv(t, "codesign desde el shell de Xcode", appleCodesignDesdeShell(t, e, valor),
		"--keychain", e.propio, "--timestamp", "--options", "runtime")
	for _, bandera := range []string{"-allowProvisioningDeviceRegistration", "-allowProvisioningDeviceRegistration=YES"} {
		appleBloqueado(t, bandera, e.ejecutar(t, "xcrun", "xcodebuild", bandera))
	}
	appleBloqueado(t, "OTHER_CODE_SIGN_FLAGS con --keychain ajeno",
		e.ejecutar(t, "xcrun", "xcodebuild", "OTHER_CODE_SIGN_FLAGS=--keychain /fixture/personal"))
}

func TestAppleEnvoltoriosLasOperacionesXcrunSinFirmaPasanTalCual(t *testing.T) {
	e := appleNuevosEnvoltorios(t)
	appleMismoArgv(t, "stapler", e.ejecutar(t, "xcrun", "stapler", "validate", "a space.dmg"), "stapler", "validate", "a space.dmg")
}

func TestAppleEnvoltoriosLasConsultasDeSDKPasanSinHerramienta(t *testing.T) {
	e := appleNuevosEnvoltorios(t)
	for _, bandera := range []string{"--show-sdk-path", "-show-sdk-path", "--show-sdk-version", "-show-sdk-version"} {
		for _, prefijo := range [][]string{{"--sdk", "iphoneos"}, {"-sdk", "macosx"}} {
			t.Run(bandera+" "+strings.Join(prefijo, " "), func(t *testing.T) {
				args := append(append([]string{}, prefijo...), bandera)
				appleMismoArgv(t, "consulta", e.ejecutar(t, "xcrun", args...), args...)
				appleBloqueado(t, "consulta seguida de codesign", e.ejecutar(t, "xcrun", append(args, "codesign", "--sign", "existing")...))
			})
		}
	}
}

func TestAppleEnvoltoriosXcrunLlevaFirmantesYFindALosEnvoltoriosPropios(t *testing.T) {
	e := appleNuevosEnvoltorios(t)
	for _, nombre := range []string{"codesign", "productsign", "productbuild"} {
		t.Run(nombre, func(t *testing.T) {
			// El envoltorio resuelve su propio directorio con `cd -P`; en macOS
			// ($TMPDIR bajo el enlace /var -> /private/var) da la ruta física.
			envoltorio, err := filepath.EvalSymlinks(filepath.Join(e.raiz, nombre))
			if err != nil {
				t.Fatal(err)
			}
			for _, bandera := range []string{"--find", "-f"} {
				r := e.ejecutar(t, "xcrun", "--sdk", "macosx", bandera, nombre)
				if r.Codigo != 0 || r.Salida != envoltorio+"\n" {
					t.Errorf("xcrun %s %s: código %d, salida %q, quiero %q\n%s", bandera, nombre, r.Codigo, r.Salida, envoltorio, r.Error)
				}
			}
			args := []string{"--sign", "existing", "--", "a space.app"}
			for _, prefijo := range [][]string{{}, {"--sdk", "macosx"}, {"--"}} {
				r := e.ejecutar(t, "xcrun", append(append(append([]string{}, prefijo...), nombre), args...)...)
				appleMismoArgv(t, "despacho", r, append(append(append([]string{}, prefijo...), envoltorio), args...)...)
			}
			// Se sigue exactamente el ejecutable que eligió xcrun/find.
			appleMismoArgv(t, "envoltorio elegido", e.bash(t, envoltorio, args...), append([]string{"--keychain", e.propio}, args...)...)
			appleBloqueado(t, "envoltorio con --keychain ajeno",
				e.bash(t, envoltorio, "--sign", "existing", "--keychain", "/fixture/personal", "--", "app"))
		})
	}
}

func TestAppleEnvoltoriosElAlcanceDeXcodePrecedeAUnSeparadorExistente(t *testing.T) {
	e := appleNuevosEnvoltorios(t)
	r := e.ejecutar(t, "xcrun", "xcodebuild", "OTHER_CODE_SIGN_FLAGS=--timestamp --")
	if r.Codigo != 0 {
		t.Fatalf("código %d\n%s", r.Codigo, r.Error)
	}
	argv := appleArgv(t, r)
	if len(argv) < 2 {
		t.Fatalf("argv corto: %q", argv)
	}
	_, banderas, _ := strings.Cut(argv[1], "=")
	appleMismoArgv(t, "codesign desde el shell de Xcode", appleCodesignDesdeShell(t, e, banderas),
		"--keychain", e.propio, "--timestamp", "--")
	appleBloqueado(t, "xcrun --find xcodebuild", e.ejecutar(t, "xcrun", "--find", "xcodebuild"))
}

func TestAppleEnvoltoriosLaBusquedaSinAlcanceDeFyneQuedaEnElLlaveroPropioYLaPersonalSeRechaza(t *testing.T) {
	e := appleNuevosEnvoltorios(t)
	appleMismoArgv(t, "find-certificate de Fyne", e.ejecutar(t, "security", "find-certificate", "-c", "Apple Distribution: existing", "-p"),
		"find-certificate", "-c", "Apple Distribution: existing", "-p", e.propio)
	// Formas exactas de los lectores de identidad y perfiles del repo. Fyne
	// Tools solo usa la búsqueda sin alcance de arriba.
	for _, args := range [][]string{
		{"find-identity", "-v", "-p", "codesigning"},
		{"find-identity", "-p", "codesigning", e.propio},
		{"find-identity", "-v", e.propio},
		{"find-certificate", "-a", "-Z", "-p", e.propio},
		{"show-keychain-info", e.propio},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			quiero := args
			if args[len(args)-1] != e.propio {
				quiero = append(append([]string{}, args...), e.propio)
			}
			appleMismoArgv(t, "lector", e.ejecutar(t, "security", args...), quiero...)
		})
	}
	for _, args := range [][]string{
		{"find-certificate", "-p", "/fixture/personal.keychain-db"},
		{"list-keychains", "-d", "user", "-s", e.propio},
		{"default-keychain", "-s", e.propio},
		{"import", "/fixture/private.p12"},
		{"add-trusted-cert", "/fixture/cert"},
		{"cms", "-D", "-i", "/fixture/profile.mobileprovision"},
	} {
		t.Run("bloquea "+args[0], func(t *testing.T) { appleBloqueado(t, args[0], e.ejecutar(t, "security", args...)) })
	}
}

func TestAppleEnvoltoriosLaPoliticaNativaConservaElAlcanceYRechazaOtraAncla(t *testing.T) {
	e := appleNuevosEnvoltorios(t)
	cert := filepath.Join(e.raiz, "leaf.pem")
	guiontest.Escribir(t, cert, "public certificate fixture")
	r := e.ejecutar(t, "security", "verify-cert", "-L", "-p", "codeSign", "-c", cert, "-k", e.propio)
	if r.Codigo != 0 {
		t.Errorf("verify-cert en el llavero propio: código %d\n%s", r.Codigo, r.Error)
	}
	for _, alcance := range [][]string{{}, {"-k", "/fixture/personal"}, {"-k", e.propio, "-r", "/fixture/alternate-root"}} {
		args := append([]string{"verify-cert", "-L", "-p", "codeSign", "-c", cert}, alcance...)
		appleBloqueado(t, strings.Join(alcance, " "), e.ejecutar(t, "security", args...))
	}
}
