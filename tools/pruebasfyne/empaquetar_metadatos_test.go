// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// --- 5. Cada app empaquetable declara sus metadatos.
// `FyneApp.toml` es lo que hace que un directorio de `cmd/` entre en la cadena, y
// lo que lleva el identificador de paquete. Un `cmd/` sin él se queda fuera en
// silencio, que es como se publica una release a la que le falta una app.
func TestEmpaquetarCadaAppDeclaraSusMetadatos(t *testing.T) {
	d := empaquetarDirs(t)
	for _, app := range empaquetarApps(t) {
		dir := empaquetarUnir(d.fyne, "cmd", app)
		toml := empaquetarUnir(dir, "FyneApp.toml")
		if !empaquetarExiste(toml) {
			t.Errorf("apps/fyne/cmd/%s no tiene FyneApp.toml: la cadena de empaquetado lo ignora sin decir nada", app)
			continue
		}
		texto := guiontest.Leer(t, toml)
		if !empaquetarHay(texto, `^\s*ID\s*=\s*"eu\.sytue\.`) {
			t.Errorf("apps/fyne/cmd/%s no declara un ID eu.sytue.*: no se puede cambiar una vez subido a una tienda", app)
		}
		icono := strings.Join(empaquetarSed(texto, `^[[:space:]]*Icon[[:space:]]*=[[:space:]]*"(.*)"[[:space:]]*$`), "\n")
		if icono == "" {
			t.Errorf("apps/fyne/cmd/%s no declara Icon: fyne package se niega a empaquetar sin icono", app)
			continue
		}
		if info, err := os.Stat(empaquetarUnir(dir, icono)); err != nil || !info.Mode().IsRegular() {
			t.Errorf("apps/fyne/cmd/%s apunta a un icono que no existe (%s)", app, icono)
		}
	}
}

// --- 6. «Abrir con»: lo que se declara al sistema es lo que la app abre.
// Docus se ofrece para documentos y plantillas con su tipo MIME, que es lo que leen el
// `.desktop` y el `intent-filter`.
func TestEmpaquetarTiposMIME(t *testing.T) {
	casos := []struct{ app, quiero, mensaje string }{
		{"docus", "application/vnd.openxmlformats-officedocument.wordprocessingml.document\n" +
			"application/vnd.openxmlformats-officedocument.wordprocessingml.template\n" +
			"application/vnd.oasis.opendocument.text\n" +
			"application/vnd.oasis.opendocument.text-template\n" +
			"application/vnd.apple.pages", "empaquetar.sh tipos docus no da los cinco tipos de Docus"},
		{"calcus", "application/vnd.apple.numbers\n" +
			"application/vnd.oasis.opendocument.spreadsheet\n" +
			"application/vnd.oasis.opendocument.spreadsheet-template\n" +
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet\n" +
			"application/vnd.openxmlformats-officedocument.spreadsheetml.template", "Calcus no declara los tipos MIME de documentos y plantillas"},
		{"diapos", "application/vnd.apple.keynote\n" +
			"application/vnd.oasis.opendocument.presentation\n" +
			"application/vnd.oasis.opendocument.presentation-template\n" +
			"application/vnd.openxmlformats-officedocument.presentationml.template\n" +
			"application/vnd.openxmlformats-officedocument.presentationml.presentation", "Diapos no declara los tipos MIME de documentos y plantillas"},
		// CertiPDF abre PDF: sin el tipo estándar, Linux y Android no lo registran en
		// «Abrir con» y el empaquetado debe detectarlo antes de publicar un artefacto roto.
		{"certipdf", "application/pdf", "empaquetar.sh tipos certipdf no da application/pdf"},
	}
	for _, c := range casos {
		if got := empaquetarJunto(empaquetar(t, "tipos", c.app)); got != c.quiero {
			t.Errorf("%s: %s", c.mensaje, empaquetarUnaLinea(got))
		}
	}
	// Y una extensión sin tipo conocido FALLA: saltársela dejaría la app fuera de «Abrir
	// con» para ese formato sin que nadie se entere.
	if r := empaquetar(t, "tipos", empaquetarAppRara(t)); r.Codigo == 0 {
		t.Error("una extensión sin tipo MIME conocido se ha dado por buena")
	}
}

// empaquetarAppRara es una app con una extensión que no tiene tipo ni UTI.
func empaquetarAppRara(t *testing.T) string {
	t.Helper()
	rara := empaquetarUnir(t.TempDir(), "rara")
	guiontest.Escribir(t, empaquetarUnir(rara, "FyneApp.toml"), "[Development]\n  extensiones = \"docx,zzz\"\n")
	return rara
}

// --- 6b. Y POR DEFECTO: macOS.
// Cada extensión es un tipo de documento del Info.plist con su UTI y con
// `LSHandlerRank Owner`; sin eso el Finder no la ofrece ni deja ponerla por defecto. Una
// extensión sin UTI conocido falla, como sin tipo MIME. Visto en rojo quitando `pptx` de
// `uti_de` y la línea de LSHandlerRank.
func TestEmpaquetarDocumentosMac(t *testing.T) {
	for _, par := range []string{
		"calcus:org.oasis-open.opendocument.spreadsheet", "calcus:org.openxmlformats.spreadsheetml.sheet",
		"docus:org.oasis-open.opendocument.text", "docus:org.openxmlformats.wordprocessingml.document",
		"diapos:org.oasis-open.opendocument.presentation", "diapos:org.openxmlformats.presentationml.presentation",
		"calcus:org.oasis-open.opendocument.spreadsheet-template", "calcus:org.openxmlformats.spreadsheetml.template",
		"docus:org.oasis-open.opendocument.text-template", "docus:org.openxmlformats.wordprocessingml.template",
		"diapos:org.oasis-open.opendocument.presentation-template", "diapos:org.openxmlformats.presentationml.template",
		"certipdf:com.adobe.pdf",
	} {
		a, uti, _ := strings.Cut(par, ":")
		r := empaquetar(t, "documentos-mac", a)
		ordenes := empaquetarJunto(r)
		if r.Codigo != 0 {
			t.Errorf("empaquetar.sh documentos-mac %s falla: %s", a, empaquetarUltima(ordenes))
			continue
		}
		if !empaquetarHay(ordenes, `^Add :CFBundleDocumentTypes:[0-9]+:LSItemContentTypes:0 string `+uti+`$`) {
			t.Errorf("el Info.plist de %s no declara el tipo %s", a, uti)
		}
	}
	if n := empaquetarCuenta(empaquetarJunto(empaquetar(t, "documentos-mac", "diapos")), `LSHandlerRank string Owner$`); n != 4 {
		t.Errorf("los tipos de Diapos no van con LSHandlerRank Owner: no podría ser la predeterminada (%d)", n)
	}
	// Los de iWork se abren convertidos y no se guardan en su formato: se declaran para leer
	// y sin quitarle el sitio a Pages, Numbers o Keynote.
	for _, par := range []string{"docus:com.apple.iwork.pages.sffpages", "calcus:com.apple.iwork.numbers.sffnumbers", "diapos:com.apple.iwork.keynote.sffkey"} {
		a, uti, _ := strings.Cut(par, ":")
		ordenes := empaquetarJunto(empaquetar(t, "documentos-mac", a))
		var n []string
		for _, l := range empaquetarLineas(ordenes, `LSItemContentTypes:0 string `+uti+`$`) {
			n = append(n, regexp.MustCompile(`^Add :CFBundleDocumentTypes:(\d+):.*`).ReplaceAllString(l, "$1"))
		}
		indice := strings.Join(n, "\n")
		if indice == "" ||
			!empaquetarHayLinea(ordenes, "Add :CFBundleDocumentTypes:"+indice+":CFBundleTypeRole string Viewer") ||
			!empaquetarHayLinea(ordenes, "Add :CFBundleDocumentTypes:"+indice+":LSHandlerRank string Alternate") {
			t.Errorf("%s no declara %s como Viewer y Alternate", a, uti)
		}
	}
	if strings.Contains(empaquetarJunto(empaquetar(t, "documentos-mac", "certipdf")), "UTImportedTypeDeclarations") {
		t.Error("CertiPDF importa com.adobe.pdf, que es del sistema")
	}
	if empaquetarJunto(empaquetar(t, "documentos-mac", "hub")) != "" {
		t.Error("el Hub declara tipos de documento sin abrir ninguno")
	}
	if r := empaquetar(t, "documentos-mac", empaquetarAppRara(t)); r.Codigo == 0 {
		t.Error("una extensión sin UTI conocido se ha dado por buena en macOS")
	}
}

// Windows: además de «Abrir con», Capabilities\FileAssociations y RegisteredApplications,
// que es lo que la pone en «Aplicaciones predeterminadas», y el desinstalador lo quita.
// Visto en rojo quitando la línea de RegisteredApplications del desinstalador.
func TestEmpaquetarRegistroWindows(t *testing.T) {
	reg := empaquetarJunto(empaquetar(t, "registro-windows", "calcus"))
	for _, l := range []string{
		`WriteRegStr HKCU "Software\Sytue\eu.sytue.calcus\Capabilities\FileAssociations" ".xlsx" "eu.sytue.calcus.documento"`,
		`WriteRegStr HKCU "Software\Sytue\eu.sytue.calcus\Capabilities\FileAssociations" ".ods" "eu.sytue.calcus.documento"`,
		`WriteRegStr HKCU "Software\Sytue\eu.sytue.calcus\Capabilities\FileAssociations" ".ots" "eu.sytue.calcus.documento"`,
		`WriteRegStr HKCU "Software\Sytue\eu.sytue.calcus\Capabilities\FileAssociations" ".xltx" "eu.sytue.calcus.documento"`,
		`WriteRegStr HKCU "Software\RegisteredApplications" "Sytue Calcus" "Software\Sytue\eu.sytue.calcus\Capabilities"`,
		`WriteRegStr HKCU "Software\Sytue\eu.sytue.calcus\Capabilities" "ApplicationName" "Sytue Calcus"`,
		`System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'`,
		`DeleteRegValue HKCU "Software\RegisteredApplications" "Sytue Calcus"`,
		`DeleteRegKey HKCU "Software\Sytue\eu.sytue.calcus"`,
	} {
		if !empaquetarHayLinea(reg, "  "+l) {
			t.Errorf("el instalador de Windows no lleva «%s»", l)
		}
	}
}

// Windows: el OpenGL de reserva (Mesa) va en `$INSTDIR\mesa` y NUNCA junto a los .exe,
// donde Windows lo cargaría antes que el controlador de la tarjeta en TODOS los equipos.
// Se mira dónde deja cada `File` sus DLL, siguiendo el último SetOutPath; y que al
// desinstalar la carpeta se va. Visto en rojo poniendo `SetOutPath "$INSTDIR"` en nsis_mesa.
func TestEmpaquetarMesaWindows(t *testing.T) {
	mesa := empaquetarJunto(empaquetar(t, "mesa-windows", "/m"))
	// awk '/^  SetOutPath / { d = $2 } /^  File .*\.dll"$/ && d != "\"$INSTDIR\\mesa\"" { print }'
	destino := ""
	dll := regexp.MustCompile(`^ {2}File .*\.dll"$`)
	var fuera []string
	for l := range strings.SplitSeq(mesa, "\n") {
		if strings.HasPrefix(l, "  SetOutPath ") {
			if c := strings.Fields(l); len(c) >= 2 {
				destino = c[1]
			}
		}
		if dll.MatchString(l) && destino != `"$INSTDIR\mesa"` {
			fuera = append(fuera, l)
		}
	}
	if len(fuera) > 0 {
		t.Errorf("el instalador de Windows deja una DLL de Mesa fuera de $INSTDIR\\mesa: %s", strings.Join(fuera, "\n"))
	}
	for _, f := range []string{"opengl32.dll", "libgallium_wgl.dll", "LICENCIAS.txt"} {
		if !empaquetarHayLinea(mesa, `  File "/m/`+f+`"`) {
			t.Errorf("el instalador de Windows no lleva %s del Mesa de reserva", f)
		}
	}
	if !empaquetarHayLinea(mesa, `  RMDir "$INSTDIR\mesa"`) {
		t.Error("el desinstalador de Windows no quita la carpeta mesa")
	}
}

// --- 7. Los bundles de escritorio son los que decidió el dueño.
// En escritorio se publica un instalador por bundle y ninguno por app. Qué app va en
// cuál lo dice su FyneApp.toml (`bundle`); una app de la suite que no lo diga saldría
// en un bundle suyo, que es justo lo que se quitó, así que aquí es rojo.
func TestEmpaquetarBundlesDeEscritorio(t *testing.T) {
	d := empaquetarDirs(t)
	for _, app := range empaquetarApps(t) {
		if app == "demo" {
			continue
		}
		if !empaquetarHay(empaquetarLeer(empaquetarUnir(d.fyne, "cmd", app, "FyneApp.toml")), `^[[:space:]]*bundle[[:space:]]*=[[:space:]]*"[a-z0-9]+"`) {
			t.Errorf("apps/fyne/cmd/%s no declara en qué bundle de escritorio va (bundle = \"…\")", app)
		}
	}
	quiero := "demo demo\ndrive drive\none bases calcus calendar cast certipdf contacts diapos docus erp hub mail notes photos rooms"
	if got := empaquetarJunto(empaquetar(t, "paquetes")); got != quiero {
		t.Errorf("los bundles no son «Sytue One» (Hub y las de trabajo) y «Sytue Drive»: %s", strings.ReplaceAll(got, "\n", "|"))
	}
	// Lo que se publica: dos instaladores por sistema, con el nombre que leen el Hub, el
	// portal y traer-instaladores.sh; la demo no.
	for _, p := range []string{"linux:linux-amd64.tar.xz", "windows:windows-amd64-installer.exe", "macos:macos-universal.dmg"} {
		so, sufijo, _ := strings.Cut(p, ":")
		if got := empaquetarJunto(empaquetar(t, "publicables", so)); got != "sytue-drive-"+sufijo+"\nsytue-one-"+sufijo {
			t.Errorf("empaquetar.sh publicables %s no da los dos bundles: %s", so, empaquetarUnaLinea(got))
		}
	}
	// Y con directorio, falla si falta uno: es lo que impide publicar una release coja.
	dist := t.TempDir()
	guiontest.Escribir(t, empaquetarUnir(dist, "sytue-one-linux-amd64.tar.xz"), "")
	r := empaquetar(t, "publicables", "linux", dist)
	switch {
	case r.Codigo == 0:
		t.Error("publicables da por completa una salida sin el bundle de Drive")
	case !strings.Contains(r.Todo(), "sytue-drive-linux-amd64.tar.xz"):
		t.Errorf("publicables falla sin decir qué falta: %s", empaquetarJunto(r))
	}
}
