// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"crypto/md5" //nolint:gosec // G501: los identificadores de generar.py, no seguridad.
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// TestEmpaquetarManifiestoDePrivacidadDeCadaApp: cada una de las quince apps host lleva
// su PrivacyInfo.xcprivacy (generar.py), en la fase de recursos de su target de Xcode y en
// el .app que arma la CLI (empaquetar.sh), antes de firmar. Declara las cuatro API de
// motivo requerido que usa el Go, el fork de Fyne, GLFW y SQLite —fechas de ficheros,
// hora desde el arranque, espacio en disco y UserDefaults—, el motivo de los ficheros que
// elige la persona solo en las apps que los abren, ningún rastreo, y los mismos datos que
// su etiqueta «App Privacy» (appstore/<app>/privacidad.txt).
//
// VISTO EN ROJO quitando el manifiesto de la fase de recursos de una app y la copia de
// empaquetar.sh.
func TestEmpaquetarManifiestoDePrivacidadDeCadaApp(t *testing.T) {
	d := empaquetarDirs(t)
	proyecto := guiontest.Leer(t, empaquetarUnir(d.fyne, "xcode/Sytue.xcodeproj/project.pbxproj"))
	tomls, err := filepath.Glob(empaquetarUnir(d.fyne, "cmd/*/FyneApp.toml"))
	if err != nil {
		t.Fatal(err)
	}
	apps := 0
	for _, toml := range tomls {
		app := filepath.Base(filepath.Dir(toml))
		if app == "demo" {
			continue
		}
		apps++
		manifiesto, err := empaquetarPlist(t, guiontest.Leer(t, empaquetarUnir(d.fyne, "xcode", app, "PrivacyInfo.xcprivacy")))
		if err != nil {
			t.Fatalf("%s: %v", app, err)
		}
		if manifiesto["NSPrivacyTracking"] != false || len(manifiesto["NSPrivacyTrackingDomains"].([]any)) != 0 {
			t.Errorf("%s: declara rastreo: %v", app, manifiesto)
		}
		motivos := map[string][]string{}
		for _, a := range manifiesto["NSPrivacyAccessedAPITypes"].([]any) {
			api := a.(map[string]any)
			for _, m := range api["NSPrivacyAccessedAPITypeReasons"].([]any) {
				motivos[api["NSPrivacyAccessedAPIType"].(string)] = append(motivos[api["NSPrivacyAccessedAPIType"].(string)], m.(string))
			}
		}
		texto := guiontest.Leer(t, toml)
		eligeFicheros := regexp.MustCompile(`(?m)^\s*(ficheros|archivos)\s*=\s*"si"|^\s*extensiones\s*=\s*"[^"]+"`).MatchString(texto)
		quiero := map[string][]string{
			"NSPrivacyAccessedAPICategoryDiskSpace":      {"E174.1"},
			"NSPrivacyAccessedAPICategoryFileTimestamp":  {"C617.1"},
			"NSPrivacyAccessedAPICategorySystemBootTime": {"35F9.1"},
			"NSPrivacyAccessedAPICategoryUserDefaults":   {"CA92.1"},
		}
		if eligeFicheros {
			quiero["NSPrivacyAccessedAPICategoryFileTimestamp"] = []string{"3B52.1", "C617.1"}
		}
		if !empaquetarMismoJSON(motivos, quiero) {
			t.Errorf("%s: API de motivo requerido %v; quiero %v", app, motivos, quiero)
		}
		var datos []string
		for _, c := range manifiesto["NSPrivacyCollectedDataTypes"].([]any) {
			dato := c.(map[string]any)
			if dato["NSPrivacyCollectedDataTypeTracking"] != false || dato["NSPrivacyCollectedDataTypeLinked"] != true {
				t.Errorf("%s: %v", app, dato)
			}
			datos = append(datos, dato["NSPrivacyCollectedDataType"].(string))
		}
		etiqueta := guiontest.Leer(t, empaquetarUnir(d.fyne, "appstore", app, "privacidad.txt"))
		if filas := strings.Count(etiqueta, "| App Functionality |"); filas != len(datos) || !slices.Contains(datos, "NSPrivacyCollectedDataTypeUserID") {
			t.Errorf("%s: %d datos en el manifiesto y %d en su etiqueta de la tienda: %v", app, len(datos), filas, datos)
		}
		// En la fase de recursos de su target: Xcode lo pone donde Apple lo busca. Los
		// identificadores son los de generar.py (`ident`).
		recursos := empaquetarObjetoXcode(proyecto, empaquetarIdent("recursos", app))
		fichero := empaquetarObjetoXcode(proyecto, empaquetarIdent("bf", app, "privacidad"))
		if !strings.Contains(recursos, "isa = PBXResourcesBuildPhase;") ||
			!strings.Contains(recursos, empaquetarIdent("bf", app, "privacidad")) ||
			!strings.Contains(fichero, "fileRef = "+empaquetarIdent("ref", app+"/privacidad")+";") {
			t.Errorf("%s: el manifiesto no está en la fase de recursos de su target:\n%s\n%s", app, recursos, fichero)
		}
	}
	if apps != 15 {
		t.Fatalf("%d apps host; son quince", apps)
	}
	// Una referencia y un fichero de compilación por app (y el de las extensiones de los
	// widgets, aparte).
	if n := strings.Count(proyecto, "path = PrivacyInfo.xcprivacy;"); n != 16 {
		t.Errorf("%d referencias a PrivacyInfo.xcprivacy en el proyecto; son 15 apps y los widgets", n)
	}
	guion := guiontest.Leer(t, d.guion)
	ruta := `manifiesto="$FYNE/xcode/$app/PrivacyInfo.xcprivacy"`
	copia := `cp "$manifiesto" "$recursos/PrivacyInfo.xcprivacy" || exit 1`
	i, ipa := strings.Index(guion, copia), strings.Index(guion, `[ -z "$ipa_abierto" ] || firmar_ipa`)
	if !strings.Contains(guion, ruta) || i < 0 || ipa < 0 || i > ipa {
		t.Error("empaquetar.sh no pone el manifiesto en el .app de la CLI antes de firmar")
	}
	// Un tag anterior a los manifiestos se empaqueta sin él y con un aviso: release-desktop
	// despachado desde main usa este guion con las fuentes del tag, y un `cp` sin
	// comprobar tumbaba la entrega de v3.25.1 (2026-11-25).
	if !strings.Contains(guion, `if [ -f "$manifiesto" ]; then`) || !strings.Contains(guion, "::warning::$app: este tag no trae PrivacyInfo.xcprivacy") {
		t.Error("empaquetar.sh tiene que avisar, no abortar, con un tag que aún no trae el manifiesto")
	}
}

// empaquetarIdent es `ident` de generar.py: los identificadores estables del proyecto.
func empaquetarIdent(partes ...string) string {
	suma := md5.Sum([]byte(strings.Join(partes, "/")))
	return strings.ToUpper(hex.EncodeToString(suma[:])[:24])
}

// empaquetarObjetoXcode es el objeto `id` del project.pbxproj, tal como lo escribe
// generar.py (`\t\t<id> = {` … `\t\t};`), o vacío.
func empaquetarObjetoXcode(proyecto, id string) string {
	_, resto, ok := strings.Cut(proyecto, "\n\t\t"+id+" = {\n")
	if !ok {
		return ""
	}
	objeto, _, _ := strings.Cut(resto, "\n\t\t};\n")
	return objeto
}

// TestEmpaquetarNingunaAPIDeMotivoSinDeclarar: el manifiesto declara lo que usa el código
// de fuera (Go, Fyne, GLFW, SQLite); el Swift y el Objective-C propios de las apps no usan
// ninguna API de motivo requerido de las que no declara. Una nueva —p. ej. el espacio
// libre con NSFileManager, la hora de arranque con systemUptime o el teclado activo—
// obliga a revisar API_PRIVACIDAD de generar.py.
func TestEmpaquetarNingunaAPIDeMotivoSinDeclarar(t *testing.T) {
	d := empaquetarDirs(t)
	prohibidas := regexp.MustCompile(`systemUptime|activeInputModes|volumeAvailableCapacity|NSFileSystemFreeSize|attributesOfFileSystemForPath|attributesOfItemAtPath|NSURLCreationDateKey|NSURLContentModificationDateKey|getattrlist`)
	for _, raiz := range []string{d.fyne, empaquetarUnir(d.raiz, "apps", "nucleo")} {
		err := filepath.WalkDir(raiz, func(ruta string, e fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if e.IsDir() && (e.Name() == "testdata" || e.Name() == "node_modules" || e.Name() == ".deps") {
				return filepath.SkipDir
			}
			ext := filepath.Ext(ruta)
			if e.IsDir() || strings.HasSuffix(ruta, "_test.go") || (ext != ".go" && ext != ".m" && ext != ".swift" && ext != ".c") {
				return nil
			}
			crudo, err := os.ReadFile(ruta)
			if err != nil {
				return err
			}
			if m := prohibidas.Find(crudo); m != nil {
				t.Errorf("%s usa %s: revisa el manifiesto de privacidad (generar.py, API_PRIVACIDAD)", ruta, m)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
