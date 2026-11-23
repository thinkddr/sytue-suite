// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"archive/zip"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// android-widgets.py añade los widgets a los recursos Android realmente
// compilados, sin construir las apps Go. Las pruebas de recursos compilan con
// el aapt2 y el android.jar del SDK que les pasa empaquetar.sh (SYTUE_AAPT2 y
// SYTUE_ANDROID_JAR); sin ellos no pueden ejercitar el guion y se saltan. Las de
// los componentes con la app cerrada no necesitan aapt2: corren en cualquier sitio.

const androidWidgetsCargar = `
import argparse, xml.etree.ElementTree as ET
W = cargar("widgets", SCRIPTS / "android-widgets.py")
`

// androidWidgetsRecursos es el escenario de ResourceTests: un APK ya extraído
// con sus artefactos y el manifiesto que dejó Fyne.
type androidWidgetsRecursos struct {
	root, apk, manifest, app, aapt2, jar string
}

const androidWidgetsManifiesto = `<manifest xmlns:android="http://schemas.android.com/apk/res/android"
            package="eu.sytue.mail" android:versionCode="77" android:versionName="9.1.2">
            <uses-permission android:name="android.permission.INTERNET" />
            <application android:label="Sytue Mail" android:debuggable="false">
            <activity android:name="org.golang.app.GoNativeActivity" android:exported="true"
                android:launchMode="singleTask">
                <meta-data android:name="android.app.lib_name" android:value="Sytue_Mail" />
                <intent-filter><action android:name="android.intent.action.VIEW" />
                <data android:scheme="sytue-mail" /></intent-filter></activity>
            <service android:name="eu.sytue.nativo.ExistingService" android:exported="false" />
            <provider android:name="eu.sytue.nativo.ExistingProvider"
                android:authorities="eu.sytue.mail.existing" android:exported="false" />
            </application></manifest>`

var androidWidgetsArtefactos = []string{"classes.dex", "lib/arm64-v8a/libSytue_Mail.so", "assets/existing.txt"}

func androidWidgetsMontar(t *testing.T) *androidWidgetsRecursos {
	t.Helper()
	aapt2, jar := os.Getenv("SYTUE_AAPT2"), os.Getenv("SYTUE_ANDROID_JAR")
	if aapt2 == "" || jar == "" {
		t.Skip("faltan SYTUE_AAPT2 y SYTUE_ANDROID_JAR (el aapt2 y el android.jar del SDK que pasa empaquetar.sh): la prueba no puede compilar recursos")
	}
	root := t.TempDir()
	r := &androidWidgetsRecursos{
		root: root, apk: filepath.Join(root, "apk"), manifest: filepath.Join(root, "AndroidManifest.xml"),
		app: filepath.Join(guiontest.Raiz(t), "apps", "fyne", "cmd", "mail"), aapt2: aapt2, jar: jar,
	}
	guiontest.Escribir(t, r.manifest, androidWidgetsManifiesto)
	for _, nombre := range androidWidgetsArtefactos {
		guiontest.Escribir(t, filepath.Join(r.apk, nombre), "existing artifact")
	}
	return r
}

func (r *androidWidgetsRecursos) icono(t *testing.T, relativo string) {
	t.Helper()
	guiontest.Escribir(t, filepath.Join(r.apk, "res", relativo), guiontest.Leer(t, filepath.Join(r.app, "Icon.png")))
}

// preparar llama a prepare() del guion con los argumentos de empaquetar.sh y
// devuelve la excepción, si la hubo.
func (r *androidWidgetsRecursos) preparar(t *testing.T) *pyRestoExcepcion {
	t.Helper()
	var e *pyRestoExcepcion
	pyRestoCorrer(t, androidWidgetsCargar+`
P = pathlib.Path
args = argparse.Namespace(appdir=P(ENTRADA["app"]), apk_root=P(ENTRADA["apk"]), manifest=P(ENTRADA["manifest"]),
    aapt2=P(ENTRADA["aapt2"]), android_jar=P(ENTRADA["jar"]), min_sdk=24, target_sdk=36)
devolver(fallo(W.prepare, args))
`, &e, pyRestoConEntrada(t, map[string]string{"app": r.app, "apk": r.apk, "manifest": r.manifest, "aapt2": r.aapt2, "jar": r.jar}))
	return e
}

// volcar empaqueta el APK extraído y devuelve `aapt2 dump xmltree` de una entrada.
func (r *androidWidgetsRecursos) volcar(t *testing.T, entrada string) string {
	t.Helper()
	ruta := filepath.Join(r.root, "resources.apk")
	f, err := os.Create(ruta) //nolint:gosec // G304: fichero del directorio temporal.
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	err = filepath.WalkDir(r.apk, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(r.apk, p)
		if err != nil {
			return err
		}
		w, err := z.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(p) //nolint:gosec // G304: fichero del directorio temporal.
		if err != nil {
			return err
		}
		_, err = w.Write(raw)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	res := guiontest.Ejecutar(t, guiontest.Opciones{}, r.aapt2, "dump", "xmltree", ruta, "--file", entrada)
	if res.Codigo != 0 {
		t.Fatalf("aapt2 dump %s falló (código %d)\n%s", entrada, res.Codigo, res.Todo())
	}
	return res.Salida
}

func (r *androidWidgetsRecursos) verificar(t *testing.T) {
	t.Helper()
	if e := r.preparar(t); e != nil {
		t.Fatalf("prepare falló: %v %s", e.Tipos, e.Mensaje)
	}
	dump := r.volcar(t, "AndroidManifest.xml")
	for _, valor := range []string{
		"eu.sytue.nativo.Widgets", "android.appwidget.provider", "sytue-mail",
		"Sytue_Mail", "ExistingService", "ExistingProvider", "eu.sytue.mail.existing",
		"eu.sytue.nativo.RefrescoWidgets", "android.permission.BIND_JOB_SERVICE",
		"versionCode(0x0101021b)=77", "minSdkVersion(0x0101020c)=24", "targetSdkVersion(0x01010270)=36",
	} {
		if !strings.Contains(dump, valor) {
			t.Fatalf("el manifiesto compilado no contiene %s\n%s", valor, dump)
		}
	}
	if !regexp.MustCompile(`resource\(0x01010025\)=@0x7f[0-9a-f]+`).MatchString(dump) {
		t.Fatalf("el manifiesto no enlaza el recurso del widget\n%s", dump)
	}
	if info := r.volcar(t, "res/xml/sytue_widget_info.xml"); !regexp.MustCompile(`initialLayout\(0x01010251\)=@0x7f[0-9a-f]+`).MatchString(info) {
		t.Fatalf("sytue_widget_info.xml sin initialLayout\n%s", info)
	}
	if layout := r.volcar(t, "res/layout/sytue_widget.xml"); !strings.Contains(layout, "TextView") {
		t.Fatalf("el layout del widget no tiene TextView\n%s", layout)
	}
	for _, nombre := range androidWidgetsArtefactos {
		if guiontest.Leer(t, filepath.Join(r.apk, nombre)) != "existing artifact" {
			t.Fatalf("%s cambió", nombre)
		}
	}
	var config struct {
		Actions []string                  `json:"actions"`
		Locales map[string]map[string]any `json:"locales"`
	}
	if err := json.Unmarshal([]byte(guiontest.Leer(t, filepath.Join(r.apk, "assets", "sytue-widget.json"))), &config); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(config.Actions, []string{"open", "compose", "search"}) {
		t.Fatalf("acciones %v", config.Actions)
	}
	if config.Locales["es"]["open"] != "Abrir" {
		t.Fatalf("es.open = %v", config.Locales["es"]["open"])
	}
	if _, ok := config.Locales["en"]["unread"].(map[string]any); !ok {
		t.Fatalf("en.unread no es un diccionario: %v", config.Locales["en"]["unread"])
	}
	if len(config.Locales) != 26 {
		t.Fatalf("%d idiomas, no 26", len(config.Locales))
	}
}

func TestAndroidWidgetsRecursosYManifiestoAntiguosSobreviven(t *testing.T) {
	r := androidWidgetsMontar(t)
	r.icono(t, "mipmap-xxxhdpi/icon.png")
	r.verificar(t)
}

func TestAndroidWidgetsLosParchesBinariosExistentesAceptanElManifiestoEnlazado(t *testing.T) {
	r := androidWidgetsMontar(t)
	r.icono(t, "mipmap-xxxhdpi-v4/icon.png")
	if e := r.preparar(t); e != nil {
		t.Fatalf("prepare falló: %v %s", e.Tipos, e.Mensaje)
	}
	guion := filepath.Join(guiontest.Raiz(t), "apps", "fyne", "scripts", "android-binario.py")
	manifiesto := filepath.Join(r.apk, "AndroidManifest.xml")
	for _, orden := range [][]string{
		{"min-sdk", manifiesto, "24"},
		{"target-sdk", manifiesto, "36"},
		{"servicio-en-primer-plano", manifiesto, "eu.sytue.nativo.ExistingService", "0x20"},
		{"imagen-en-imagen", manifiesto},
	} {
		if res := guiontest.Ejecutar(t, guiontest.Opciones{}, "python3", append([]string{guion}, orden...)...); res.Codigo != 0 {
			t.Fatalf("android-binario.py %v falló (código %d)\n%s", orden, res.Codigo, res.Error)
		}
	}
	dump := r.volcar(t, "AndroidManifest.xml")
	for _, s := range []string{"foregroundServiceType(0x01010599)=0x00000020", "supportsPictureInPicture(0x010104f7)=true", "eu.sytue.nativo.Widgets"} {
		if !strings.Contains(dump, s) {
			t.Fatalf("el manifiesto parcheado no contiene %s\n%s", s, dump)
		}
	}
}

func TestAndroidWidgetsCapasAdaptativasYMonocromaSobreviven(t *testing.T) {
	r := androidWidgetsMontar(t)
	for _, capa := range []string{"background", "foreground", "monochrome"} {
		r.icono(t, "mipmap-xxxhdpi/ic_launcher_"+capa+".png")
	}
	for _, version := range []string{"26", "33"} {
		for _, nombre := range []string{"ic_launcher", "ic_launcher_round"} {
			guiontest.Escribir(t, filepath.Join(r.apk, "res/mipmap-anydpi-v"+version, nombre+".xml"), "compiled XML")
		}
	}
	r.verificar(t)
	if !strings.Contains(r.volcar(t, "res/mipmap-anydpi-v33/ic_launcher.xml"), "monochrome") {
		t.Fatal("el icono adaptativo v33 perdió la capa monocroma")
	}
	if strings.Contains(r.volcar(t, "res/mipmap-anydpi-v26/ic_launcher.xml"), "monochrome") {
		t.Fatal("el icono adaptativo v26 lleva una capa monocroma")
	}
}

func TestAndroidWidgetsRecursosInesperadosNoSeBorranEnSilencio(t *testing.T) {
	r := androidWidgetsMontar(t)
	r.icono(t, "mipmap-xxxhdpi/icon.png")
	importante := filepath.Join(r.apk, "res", "raw", "important.txt")
	guiontest.Escribir(t, importante, "must survive")
	pyRestoLanza(t, r.preparar(t), "ValueError", "recurso Fyne nuevo")
	if guiontest.Leer(t, importante) != "must survive" {
		t.Fatal("se borró un recurso que no se conocía")
	}
}

// Las dos siguientes no usan aapt2: en el original solo lo exigían por compartir
// el setUp de la clase.

func TestAndroidWidgetsLaDemoSeEmpaquetaSinWidgets(t *testing.T) {
	var nulo bool
	pyRestoCorrer(t, androidWidgetsCargar+`
args = type("Args", (), {"appdir": W.FYNE / "cmd" / "demo"})()
devolver(W.prepare(args) is None)
`, &nulo)
	if !nulo {
		t.Fatal("prepare de la demo devolvió algo: la demo no lleva widgets")
	}
}

func TestAndroidWidgetsElCatalogoUsaLosMetadatosDeCadaAppNativa(t *testing.T) {
	var configs []struct {
		ID      string                       `json:"id"`
		Scheme  string                       `json:"scheme"`
		Product string                       `json:"product"`
		Actions []string                     `json:"actions"`
		Locales map[string]map[string]string `json:"locales"`
	}
	pyRestoCorrer(t, androidWidgetsCargar+`
salida = []
for path in (W.FYNE / "cmd").glob("*/FyneApp.toml"):
    if path.parent.name == "demo":
        continue
    c = W.metadata(path.parent)
    salida.append({"id": c["id"], "scheme": c["scheme"], "product": c["product"], "actions": c["actions"],
                   "locales": {l: {k: v for k, v in e.items() if isinstance(v, str)} for l, e in c["locales"].items()}})
devolver(salida)
`, &configs)
	vistos := map[string]bool{}
	for _, c := range configs {
		if vistos[c.ID] {
			t.Fatalf("id repetido: %s", c.ID)
		}
		vistos[c.ID] = true
		if c.Scheme != "sytue-"+c.Product {
			t.Fatalf("%s: esquema %s y producto %s", c.ID, c.Scheme, c.Product)
		}
		if !slices.Contains(c.Actions, "search") {
			t.Fatalf("%s sin la acción search: %v", c.ID, c.Actions)
		}
		for idioma, etiquetas := range c.Locales {
			for _, k := range []string{"search", "calendars", "address_books"} {
				if etiquetas[k] == "" {
					t.Fatalf("%s/%s: falta la etiqueta %s", c.ID, idioma, k)
				}
			}
			if strings.Count(etiquetas["updated_at"], "{time}") != 1 {
				t.Fatalf("%s/%s: updated_at %q no lleva un {time}", c.ID, idioma, etiquetas["updated_at"])
			}
		}
	}
	if len(vistos) != 15 {
		t.Fatalf("%d apps con widget, no 15", len(vistos))
	}
}

// --- Los componentes del widget con la app cerrada, sin aapt2 ---------------

const androidWidgetsFondo = androidWidgetsCargar + `
root = ET.fromstring('''<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="eu.sytue.mail"><uses-permission android:name="android.permission.INTERNET" />
    <application android:label="Sytue Mail" /></manifest>''')
app = root.find("application")

def named(nodo, tag, name):
    return None if nodo is None else nodo.find(tag + "[@" + W.ATTR + "name='" + name + "']")

def atributos(nodo):
    return None if nodo is None else {k.replace(W.ATTR, ""): v for k, v in nodo.attrib.items()}

def observar():
    servicio = named(app, "service", "eu.sytue.nativo.RefrescoWidgets")
    permisos = root.findall("uses-permission")
    return {
        "servicio": atributos(servicio),
        "meta": atributos(named(servicio, "meta-data", "eu.sytue.widgets.go")),
        "relevo": atributos(named(app, "provider", "eu.sytue.nativo.RelevoWidgets")),
        "red": atributos(named(root, "uses-permission", "android.permission.ACCESS_NETWORK_STATE")),
        "permisos": [e.get(W.ATTR + "name") for e in permisos],
        "permisos_antes_de_application": list(root).index(permisos[-1]) < list(root).index(app),
    }
`

type androidWidgetsObservado struct {
	Servicio map[string]string `json:"servicio"`
	Meta     map[string]string `json:"meta"`
	Relevo   map[string]string `json:"relevo"`
	Red      map[string]string `json:"red"`
	Permisos []string          `json:"permisos"`
	Antes    bool              `json:"permisos_antes_de_application"`
	Segunda  *pyRestoExcepcion `json:"segunda"`
}

func TestAndroidWidgetsFondoSinGoSoloElTrabajoQueRepinta(t *testing.T) {
	var r androidWidgetsObservado
	pyRestoCorrer(t, androidWidgetsFondo+`
W.fondo(root, app, "eu.sytue.mail", go=False)
devolver(observar())
`, &r)
	if r.Servicio == nil {
		t.Fatal("falta el servicio RefrescoWidgets")
	}
	if r.Servicio["permission"] != "android.permission.BIND_JOB_SERVICE" || r.Servicio["exported"] != "false" {
		t.Fatalf("servicio %v", r.Servicio)
	}
	if r.Meta["value"] != "false" {
		t.Fatalf("eu.sytue.widgets.go = %v, no false", r.Meta)
	}
	if r.Relevo != nil {
		t.Fatalf("sin Go no hay relevo y está: %v", r.Relevo)
	}
	if r.Red != nil {
		t.Fatal("sin Go no hace falta ACCESS_NETWORK_STATE y se pidió")
	}
}

func TestAndroidWidgetsFondoConGoElRelevoYElPermisoDeRed(t *testing.T) {
	var r androidWidgetsObservado
	pyRestoCorrer(t, androidWidgetsFondo+`
W.fondo(root, app, "eu.sytue.mail", go=True)
devolver(observar())
`, &r)
	if r.Meta["value"] != "true" {
		t.Fatalf("eu.sytue.widgets.go = %v, no true", r.Meta)
	}
	if r.Relevo["authorities"] != "eu.sytue.mail.sytue.widgets" || r.Relevo["exported"] != "true" {
		t.Fatalf("relevo %v", r.Relevo)
	}
	if !reflect.DeepEqual(r.Permisos, []string{"android.permission.INTERNET", "android.permission.ACCESS_NETWORK_STATE"}) {
		t.Fatalf("permisos %v", r.Permisos)
	}
	if !r.Antes {
		t.Fatal("el último uses-permission va detrás de <application>")
	}
}

func TestAndroidWidgetsFondoLosComponentesNoSeDeclaranDosVeces(t *testing.T) {
	var r androidWidgetsObservado
	pyRestoCorrer(t, androidWidgetsFondo+`
W.fondo(root, app, "eu.sytue.mail", go=True)
devolver({"segunda": fallo(W.fondo, root, app, "eu.sytue.mail", go=True)})
`, &r)
	pyRestoLanza(t, r.Segunda, "ValueError", "duplicado")
}

func TestAndroidWidgetsFondoGoEncendidoPorDecisionDelDueno(t *testing.T) {
	// Encendido por decisión del dueño (2026-11-07), aún sin la prueba del runbook en un
	// teléfono. Apagarlo es un commit revisado: esta prueba cambia con él.
	var encendido any
	pyRestoCorrer(t, androidWidgetsCargar+"\ndevolver(W.FONDO_GO)\n", &encendido)
	if encendido != true {
		t.Fatalf("FONDO_GO = %v, no True", encendido)
	}
}
