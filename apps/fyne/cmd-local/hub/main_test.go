// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
	"git.sytue.eu/sytue/one/apps/nucleo/local/indice"
)

// metadatos lee `[Development] <clave>` de cada `cmd-local/<app>/FyneApp.toml`.
func metadatos(t *testing.T, clave string) map[string]string {
	t.Helper()
	tomls, _ := filepath.Glob(filepath.Join("..", "*", "FyneApp.toml"))
	if len(tomls) < 10 {
		t.Fatalf("solo %d apps en cmd-local", len(tomls))
	}
	re := regexp.MustCompile(`(?m)^\s*` + clave + `\s*=\s*"([^"]*)"`)
	out := map[string]string{}
	for _, f := range tomls {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		v := ""
		if m := re.FindSubmatch(raw); m != nil {
			v = string(m[1])
		}
		out[filepath.Base(filepath.Dir(f))] = v
	}
	return out
}

// TestElCatalogoEsElDeCmdLocal: cada app de `cmd-local` tiene tesela en el Hub, con el
// esquema que declara su FyneApp.toml. Una app que falta aquí no se podría lanzar.
func TestElCatalogoEsElDeCmdLocal(t *testing.T) {
	esquemas := metadatos(t, "esquema")
	if esquemas["hub"] != "sytueoffice-one" {
		t.Fatalf("el esquema del Hub es %q", esquemas["hub"])
	}
	for app, esquema := range esquemas {
		if app == "hub" {
			continue
		}
		i := slices.IndexFunc(catalogo, func(a App) bool { return a.Carpeta == app })
		if i < 0 {
			t.Errorf("cmd-local/%s no está en el catálogo del Hub", app)
			continue
		}
		if quiero := "sytueoffice-" + catalogo[i].Producto; esquema != quiero {
			t.Errorf("cmd-local/%s declara el esquema %q y el Hub lo abre con %q", app, esquema, quiero)
		}
	}
	if b := metadatos(t, "bundle")["hub"]; b != "office" {
		t.Errorf("el Hub va en el bundle %q, no en el de escritorio de la edición", b)
	}
}

// TestCadaFicheroVaALaAppQueLoDeclara: la tabla de `indice` (qué app abre un fichero
// de Drive) es la de las extensiones de cada FyneApp.toml.
func TestCadaFicheroVaALaAppQueLoDeclara(t *testing.T) {
	producto := map[string]string{}
	for _, a := range catalogo {
		producto[a.Carpeta] = a.Producto
	}
	quien := map[string][]string{}
	for app, exts := range metadatos(t, "extensiones") {
		for _, e := range strings.Split(exts, ",") {
			if e = strings.TrimSpace(e); e != "" {
				quien[e] = append(quien[e], producto[app])
			}
		}
	}
	for ext, apps := range quien {
		if ext == "zip" || ext == "markdown" {
			continue // un volcado de cuadernos: no se manda a Notepads cualquier zip
		}
		p, ok := indice.ProductoDeFichero("x." + ext)
		if !ok || !slices.Contains(apps, p) {
			t.Errorf(".%s lo abre %q según indice, y lo declaran %q", ext, p, apps)
		}
	}
}

func ejecutable(t *testing.T, ruta string, modo os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(ruta), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ruta, []byte("#!/bin/sh\n"), modo); err != nil {
		t.Fatal(err)
	}
}

func carpetas(is []Instalada) []string {
	out := make([]string, 0, len(is))
	for _, i := range is {
		out = append(out, i.Carpeta)
	}
	return out
}

func TestInstaladasSonLasHermanasDelHub(t *testing.T) {
	dir := t.TempDir()
	hubExe := filepath.Join(dir, "sytue-office-hub")
	ejecutable(t, hubExe, 0o755)
	ejecutable(t, filepath.Join(dir, "sytue-office-docus"), 0o755)
	ejecutable(t, filepath.Join(dir, "sytue-office-mail"), 0o755)
	ejecutable(t, filepath.Join(dir, "sytue-office-notes"), 0o644) // sin permiso de ejecución
	ejecutable(t, filepath.Join(dir, "sytue-docus"), 0o755)        // la conectada
	ejecutable(t, filepath.Join(dir, "sytue-office-inventada"), 0o755)
	if got := carpetas(Instaladas(hubExe, "linux")); !slices.Equal(got, []string{"mail", "docus"}) {
		t.Fatalf("en Linux = %q", got)
	}

	win := t.TempDir()
	ejecutable(t, filepath.Join(win, "sytue-office-hub.exe"), 0o644)
	ejecutable(t, filepath.Join(win, "sytue-office-calcus.exe"), 0o644)
	if got := carpetas(Instaladas(filepath.Join(win, "sytue-office-hub.exe"), "windows")); !slices.Equal(got, []string{"calcus"}) {
		t.Fatalf("en Windows = %q", got)
	}

	mac := t.TempDir()
	hubMac := filepath.Join(mac, "Sytue Office.app", "Contents", "MacOS", "sytue-office-hub")
	ejecutable(t, hubMac, 0o755)
	ejecutable(t, filepath.Join(mac, "Sytue Diapos Office.app", "Contents", "MacOS", "sytue-office-diapos"), 0o755)
	ejecutable(t, filepath.Join(mac, "Otra.app", "Contents", "MacOS", "otra"), 0o755)
	got := Instaladas(hubMac, "darwin")
	if len(got) != 1 || got[0].Carpeta != "diapos" || got[0].Destino != filepath.Join(mac, "Sytue Diapos Office.app") {
		t.Fatalf("en macOS = %+v", got)
	}
	if Instaladas(filepath.Join(mac, "suelto"), "darwin") != nil || Instaladas("", "linux") != nil {
		t.Fatal("fuera de un .app, o sin ejecutable, no hay hermanas")
	}
}

func TestLaOrdenNoPasaPorUnInterpreteNiAceptaOtrosEnlaces(t *testing.T) {
	i := Instalada{App{"docus", "docs", 0}, "/opt/sytue-office/sytue-office-docus"}
	if got, err := Orden(i, "sytueoffice-docs:/abrir?id=1", "linux"); err != nil || !slices.Equal(got, []string{i.Destino, "sytueoffice-docs:/abrir?id=1"}) {
		t.Fatal(got, err)
	}
	if got, _ := Orden(i, "", "windows"); !slices.Equal(got, []string{i.Destino}) {
		t.Fatal(got)
	}
	m := Instalada{App{"docus", "docs", 0}, "/Applications/Sytue Docus Office.app"}
	if got, _ := Orden(m, "sytueoffice-docs:/abrir", "darwin"); !slices.Equal(got, []string{"open", "-a", m.Destino, "--args", "sytueoffice-docs:/abrir"}) {
		t.Fatal(got)
	}
	if _, err := Orden(i, "https://ejemplo.invalid", "linux"); err == nil {
		t.Fatal("ha aceptado un enlace ajeno")
	}
	if _, err := Orden(i, "", "android"); !errors.Is(err, ErrNoLanza) {
		t.Fatal("en el móvil no se lanza:", err)
	}
}

// TestLaPortadaAbreCadaCosaEnSuApp: lo reciente sale de los almacenes; un documento se
// abre en Docus con su enlace, un fichero sin app de la edición con el visor del sistema
// y lo que no tiene su app instalada lo dice.
func TestLaPortadaAbreCadaCosaEnSuApp(t *testing.T) {
	test.NewTempApp(t)
	datos := t.TempDir()
	bib, err := biblioteca.Abrir(filepath.Join(datos, "Documentos"), "docs", biblioteca.Persona{ID: "yo"})
	if err != nil {
		t.Fatal(err)
	}
	doc, _, err := bib.CreateDocumentFromSource(t.Context(), api.DocText, "Carta al banco", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(datos, "Drive"), 0o700); err != nil {
		t.Fatal(err)
	}
	zip := filepath.Join(datos, "Drive", "banco.zip")
	if err := os.WriteFile(zip, []byte("PK"), 0o600); err != nil {
		t.Fatal(err)
	}

	tr, err := i18n.New(i18n.Locale("es"), i18n.Common())
	if err != nil {
		t.Fatal(err)
	}
	docus := Instalada{catalogo[slices.IndexFunc(catalogo, func(a App) bool { return a.Carpeta == "docus" })], "/x/sytue-office-docus"}
	p := NuevaPortada(tr, datos, []Instalada{docus})
	var lanzado, abierto, aviso string
	p.Lanzar = func(i Instalada, enlace string) error { lanzado = i.Carpeta + " " + enlace; return nil }
	p.Fichero = func(r string) error { abierto = r; return nil }
	p.Aviso = func(titulo, _ string) { aviso = titulo }
	p.cargar()
	p.pintar()

	rotulo, es := p.Elementos()
	if rotulo != "Reciente" || len(es) != 2 {
		t.Fatalf("%s: %+v", rotulo, es)
	}
	p.Buscar("CARTA")
	rotulo, es = p.Elementos()
	if rotulo != "Resultados" || len(es) != 1 || es[0].ID != doc.ID {
		t.Fatalf("%s: %+v", rotulo, es)
	}
	p.Abrir(es[0])
	if lanzado != "docus sytueoffice-docs:/abrir?clase=documento&id="+doc.ID {
		t.Fatalf("lanzado = %q", lanzado)
	}
	p.Buscar("banco")
	_, es = p.Elementos()
	if len(es) != 2 {
		t.Fatalf("banco = %+v", es)
	}
	for _, e := range es {
		if e.Clase == indice.Fichero {
			p.Abrir(e)
		}
	}
	if abierto != zip {
		t.Fatalf("el zip se ha abierto con %q", abierto)
	}
	p.Abrir(indice.Elemento{Producto: "notes", Clase: indice.Nota, ID: "n"})
	if aviso != "Notepads no está instalada" {
		t.Fatalf("aviso = %q", aviso)
	}
	p.Buscar("")
	if r, _ := p.Elementos(); r != "Reciente" {
		t.Fatal("vaciar la caja no vuelve a lo reciente")
	}
}
