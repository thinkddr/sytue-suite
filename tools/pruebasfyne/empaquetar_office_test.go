// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// Lo que empaquetar.sh hace distinto para Sytue Office (SYTUE_ARBOL=cmd-local), el
// árbol de apps de la edición local (docs/roadmap.md, «Sytue Office»).

// empaquetarOffice ejecuta `empaquetar.sh args…` sobre el árbol de Sytue Office.
func empaquetarOffice(t *testing.T, args ...string) guiontest.Resultado {
	t.Helper()
	return empaquetarCon(t, []string{"SYTUE_ARBOL=cmd-local"}, args...)
}

// --- 6c. UN SEGUNDO ESQUEMA (esquemas_extra) ---------------------------------------
// Mail de Sytue Office se ofrece para `mailto:` en los cinco sistemas, como candidata y
// sin quitárselo a nadie: en Windows por Capabilities\URLAssociations, NUNCA escribiendo
// `Classes\mailto`. Y un esquema que no lo es (o el de otra app de la suite) falla.
// Visto en rojo quitando el bucle de plist_esquemas y poniendo `Classes\mailto`.
func TestEmpaquetarOfficeMailSeOfreceParaMailtoSinQuitarseloANadie(t *testing.T) {
	if got := empaquetarJunto(empaquetarOffice(t, "esquemas", "linux", "mail")); got != "x-scheme-handler/mailto" {
		t.Errorf("Mail de Sytue Office no declara x-scheme-handler/mailto en su .desktop: %q", got)
	}
	if !empaquetarHayLinea(empaquetarJunto(empaquetarOffice(t, "esquemas", "android", "mail")), "\t\t\t<data android:scheme=\"mailto\" />") {
		t.Error("Mail de Sytue Office no declara mailto en su intent-filter de Android")
	}
	ordenes := empaquetarJunto(empaquetarOffice(t, "esquemas", "apple", "mail"))
	if !empaquetarHayLinea(ordenes, "Add :CFBundleURLTypes:0:CFBundleURLSchemes:0 string sytueoffice-mail") ||
		!empaquetarHayLinea(ordenes, "Add :CFBundleURLTypes:0:CFBundleURLSchemes:1 string mailto") {
		t.Errorf("el Info.plist de Mail de Sytue Office no lleva su esquema y mailto: %s", empaquetarUnaLinea(ordenes))
	}
	reg := empaquetarJunto(empaquetarOffice(t, "esquemas", "windows", "mail"))
	for _, l := range []string{
		`WriteRegStr HKCU "Software\Sytue\eu.sytue.office.mail\Capabilities\URLAssociations" "mailto" "eu.sytue.office.mail.enlace"`,
		`WriteRegStr HKCU "Software\Classes\eu.sytue.office.mail.enlace" "URL Protocol" ""`,
		`WriteRegStr HKCU "Software\RegisteredApplications" "Sytue Mail Office" "Software\Sytue\eu.sytue.office.mail\Capabilities"`,
		`DeleteRegKey HKCU "Software\Classes\eu.sytue.office.mail.enlace"`,
	} {
		if !empaquetarHayLinea(reg, "  "+l) {
			t.Errorf("el instalador de Mail de Sytue Office no lleva «%s»", l)
		}
	}
	if regexp.MustCompile(`(?i)Classes\\mailto`).MatchString(reg) {
		t.Error(`el instalador escribe Classes\mailto: le quitaría mailto al programa de correo de la persona`)
	}
	if got := empaquetarJunto(empaquetar(t, "esquemas", "linux", "mail")); got != "" {
		t.Errorf("Mail de la edición conectada declara esquemas de más: %q", got)
	}
	dir := t.TempDir()
	for _, malo := range []string{"MailTo", "sytue-docs", "https", "mail to"} {
		guiontest.Escribir(t, filepath.Join(dir, "FyneApp.toml"),
			"[Development]\n  esquema = \"sytueoffice-x\"\n  esquemas_extra = \""+malo+"\"\n")
		if r := empaquetar(t, "esquemas", "linux", dir); r.Codigo == 0 {
			t.Errorf("«%s» se ha dado por bueno como esquema extra", malo)
		}
	}
}

// Los formatos nativos de Sytue Office tienen tipo MIME y UTI propios, y cada extensión
// que declara una de sus apps los tiene: si no, el empaquetado de ESA app para en seco en
// todos los sistemas (pasó con el `.zip` de Notepads, que no salía ni en Linux ni en
// Android).
func TestEmpaquetarOfficeCadaExtensionTieneTipoMIMEYUTI(t *testing.T) {
	if !empaquetarHayLinea(empaquetarJunto(empaquetarOffice(t, "tipos", "calcus")), "application/vnd.sytue.calcus+zip") {
		t.Error("Calcus de Sytue Office no declara .calcus")
	}
	if !empaquetarHay(empaquetarJunto(empaquetarOffice(t, "documentos-mac", "docus")), `LSItemContentTypes:0 string eu\.sytue\.office\.docus$`) {
		t.Error("Docus de Sytue Office no declara .docus en macOS")
	}
	d := empaquetarDirs(t)
	entradas, err := os.ReadDir(filepath.Join(d.fyne, "cmd-local"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entradas {
		if !e.IsDir() {
			continue
		}
		if r := empaquetarOffice(t, "tipos", e.Name()); r.Codigo != 0 {
			t.Errorf("%s de Sytue Office declara una extensión sin tipo MIME: %s", e.Name(), r.Todo())
		}
		if r := empaquetarOffice(t, "documentos-mac", e.Name()); r.Codigo != 0 {
			t.Errorf("%s de Sytue Office declara una extensión sin UTI: %s", e.Name(), r.Todo())
		}
	}
}

// --- SYTUE OFFICE EN iOS: su App Group, no la sesión de la suite (F11) ------------
// Las apps de cmd-local se firman en iOS con el App Group de la edición (`group.` +
// ServicioLlavero de espacio.go) y nada más: ni el grupo ni el Keychain de la sesión de
// la suite, que no tienen. Visto en rojo dándoles el grupo de la suite, cambiando el
// ServicioLlavero y quitando la rama de cmd-local de firmar_ipa.
func TestEmpaquetarOfficeEnIOSSoloSuAppGroup(t *testing.T) {
	d := empaquetarDirs(t)
	m := regexp.MustCompile(`(?m)^\s*ServicioLlavero\s*= "(.*)"$`).FindStringSubmatch(empaquetarLeer(empaquetarUnir(d.raiz, "apps/nucleo/local/espacio/espacio.go")))
	if len(m) < 2 || "group."+m[1] != "group.eu.sytue.office" {
		t.Fatalf("el App Group de Sytue Office no es group.eu.sytue.office: %v", m)
	}
	grupo := "group." + m[1]
	for _, app := range empaquetarAppsOffice(t) {
		p, ok := empaquetarPlistDe(t, []string{"SYTUE_ARBOL=cmd-local"}, "derechos-office", app, empaquetarEquipo)
		if !ok {
			t.Errorf("derechos-office %s falla", app)
			continue
		}
		if !empaquetarMismoJSON(p, map[string]any{"com.apple.security.application-groups": []any{grupo}}) {
			t.Errorf("los derechos de iOS de %s (Sytue Office) no son solo su App Group: %v", app, p)
		}
	}
	if r := empaquetarOffice(t, "derechos-office", "docus", "ABCDE12345"); r.Codigo == 0 {
		t.Error("derechos-office acepta otro equipo Apple")
	}
	if r := empaquetar(t, "derechos-office", "docus", empaquetarEquipo); r.Codigo == 0 {
		t.Error("derechos-office acepta una app de la edición conectada")
	}
	if r := empaquetarOffice(t, "derechos-sesion", "docus", empaquetarEquipo, "ios"); r.Codigo == 0 {
		t.Error("la sesión de la suite acepta una app de Sytue Office")
	}
	firmar := empaquetarExtraerFunciones(empaquetarLeer(d.guion), false, "firmar_ipa")
	if !strings.Contains(firmar, `derechos_office "$6"`) ||
		!regexp.MustCompile(`verificar-perfil-grupo.*grupo_office_apple`).MatchString(firmar) ||
		!strings.Contains(firmar, "incorporar_ia_apple") {
		t.Error("firmar_ipa no firma Sytue Office con su App Group (comprobado contra el perfil) y su SytueIA.framework")
	}
	construir := empaquetarUnir(d.fyne, "xcode/ia-sistema/construir.sh")
	if fi, err := os.Stat(construir); err != nil || fi.Mode()&0o111 == 0 {
		t.Error("falta xcode/ia-sistema/construir.sh, o no es ejecutable")
	} else if r := guiontest.Ejecutar(t, guiontest.Opciones{}, "bash", "-n", construir); r.Codigo != 0 {
		t.Errorf("xcode/ia-sistema/construir.sh no es un guion válido: %s", r.Todo())
	}
}

// --- SYTUE OFFICE EN ANDROID: sin la sesión de la suite ----------------------------
// El dex de cmd-local lleva el SesionFirmas de la edición (apagado y sin ningún paquete
// de confianza) y su prueba en la JVM es SesionLocalPrueba; el de la conectada, el
// catálogo revisado y DrivePreferenciaPrueba. Visto en rojo quitando `--edicion local`
// de la rama de cmd-local.
func TestEmpaquetarOfficeEnAndroidSinLaSesionDeLaSuite(t *testing.T) {
	d := empaquetarDirs(t)
	dex := strings.Join(empaquetarLineas(empaquetarLeer(d.guion), `android-sesion\.py" java|prueba_sesion`), "\n")
	for _, l := range []string{
		`android-sesion.py" java "$dex_tmp/generated/eu/sytue/nativo/SesionFirmas.java" --edicion local`,
		`android-sesion.py" java "$dex_tmp/generated/eu/sytue/nativo/SesionFirmas.java"`,
		`if [ "$ARBOL" = cmd-local ]; then prueba_sesion=SesionLocalPrueba; fi`,
	} {
		if !empaquetarHay(dex, regexp.QuoteMeta(l)+`$`) {
			t.Errorf("empaquetar.sh android no da a Sytue Office su SesionFirmas sin sesión y su SesionLocalPrueba (falta «%s»)", l)
		}
	}
	if !empaquetarExiste(empaquetarUnir(d.fyne, "android/prueba/eu/sytue/nativo/SesionLocalPrueba.java")) {
		t.Error("falta la prueba Java de la sesión de Sytue Office")
	}
}

// empaquetarAppsOffice son las apps de cmd-local, por su directorio.
func empaquetarAppsOffice(t *testing.T) []string {
	t.Helper()
	entradas, err := os.ReadDir(filepath.Join(empaquetarDirs(t).fyne, "cmd-local"))
	if err != nil {
		t.Fatal(err)
	}
	var apps []string
	for _, e := range entradas {
		if e.IsDir() && empaquetarExiste(filepath.Join(empaquetarDirs(t).fyne, "cmd-local", e.Name(), "FyneApp.toml")) {
			apps = append(apps, e.Name())
		}
	}
	return apps
}

// SYTUE_PARA_ESTE_EQUIPO=1 deja pasar un binario que exige una glibc más nueva que el suelo
// de la release (Debian 12): es para `sytue-deploy --mode sytue-office`, que instala en el
// mismo equipo que compila. Ningún workflow puede ponerla, o una release saldría con un
// binario que no arranca en Debian 12 ni en Ubuntu 22.04. Visto en rojo poniéndola en el
// env de release-office.yml.
func TestEmpaquetarElSueloDeGlibcSoloSeSaltaFueraDeLaRelease(t *testing.T) {
	d := empaquetarDirs(t)
	guion := empaquetarLeer(d.guion)
	if !strings.Contains(guion, `if [ "${SYTUE_PARA_ESTE_EQUIPO:-}" = 1 ]; then`) {
		t.Fatal("empaquetar.sh ya no tiene la salida de SYTUE_PARA_ESTE_EQUIPO: esta prueba sobra o hay que rehacerla")
	}
	flujos, err := filepath.Glob(filepath.Join(d.raiz, ".github", "workflows", "*.yml"))
	if err != nil || len(flujos) == 0 {
		t.Fatalf("sin workflows que mirar: %v", err)
	}
	for _, f := range flujos {
		if strings.Contains(empaquetarLeer(f), "SYTUE_PARA_ESTE_EQUIPO") {
			t.Errorf("%s pone SYTUE_PARA_ESTE_EQUIPO: una release no se salta el suelo de glibc", filepath.Base(f))
		}
	}
}
