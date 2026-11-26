// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"archive/zip"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
	"git.sytue.eu/sytue/one/apps/nucleo/local/fototeca"
	"git.sytue.eu/sytue/one/apps/nucleo/local/traer"
)

// textosEnCapas son los textos de lo que hay abierto encima de la ventana (diálogos).
func textosEnCapas(w fyne.Window) string {
	var out []string
	var recorrer func(o fyne.CanvasObject)
	recorrer = func(o fyne.CanvasObject) {
		if o == nil || !o.Visible() {
			return
		}
		switch x := o.(type) {
		case *widget.Label:
			out = append(out, x.Text)
		case *canvas.Text:
			out = append(out, x.Text)
		case *fyne.Container:
			for _, h := range x.Objects {
				recorrer(h)
			}
			return
		}
		if wd, ok := o.(fyne.Widget); ok {
			for _, h := range test.WidgetRenderer(wd).Objects() {
				recorrer(h)
			}
		}
	}
	for _, c := range w.Canvas().Overlays().List() {
		recorrer(c)
	}
	return strings.Join(out, "\n")
}

func zipDeSytue(t *testing.T) string {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), "sytue-one-2026-11-20.zip")
	f, err := os.Create(ruta)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for nombre, datos := range map[string]string{
		"sytue-one-2026-11-20/calendar/calendars/Casa.ics": "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//ES\r\nBEGIN:VEVENT\r\nUID:u1\r\nDTSTAMP:20261120T100000Z\r\nDTSTART:20261201T090000Z\r\nDTEND:20261201T100000Z\r\nSUMMARY:Visita\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
		"sytue-one-2026-11-20/account/profile.json":        `{}`,
		"sytue-one-2026-11-20/summary.json":                `[]`,
	} {
		w, err := zw.Create(nombre)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(datos)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return ruta
}

// HU-07: con Calendar abierta se pide cerrarla; cerrada, se dice qué trae y se trae, y
// al acabar se dice cuánto ha entrado y dónde está lo demás.
func TestTraerDesdeSytuePideCerrarYDiceComoHaIdo(t *testing.T) {
	test.NewTempApp(t)
	w := test.NewWindow(nil)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(1000, 700))
	datos := t.TempDir()
	abiertasAhora := []string{"calendar"}
	tr := &Traedor{
		W: w, Idioma: "es",
		Destino:  traer.Destino{Datos: datos, Sujeto: "yo", Persona: biblioteca.Persona{ID: "yo"}},
		Abiertas: func([]string) ([]string, bool) { return abiertasAhora, true },
		Traer:    traer.Traer,
	}
	zipRuta := zipDeSytue(t)

	cerrarCapas := func() {
		for _, c := range w.Canvas().Overlays().List() {
			w.Canvas().Overlays().Remove(c)
		}
	}
	tr.Mirar(zipRuta)
	if got := textosEnCapas(w); !strings.Contains(got, "Cierra antes las apps") || !strings.Contains(got, "Calendar") {
		t.Fatalf("con Calendar abierta:\n%s", got)
	}
	cerrarCapas()

	abiertasAhora = nil
	tr.Mirar(zipRuta)
	if got := textosEnCapas(w); !strings.Contains(got, "Este fichero trae: account, Calendar.") || !strings.Contains(got, "No se abre ninguna conexión") {
		t.Fatalf("confirmación:\n%s", got)
	}
	cerrarCapas()

	inf, err := traer.Traer(context.Background(), zipRuta, tr.Destino)
	if err != nil {
		t.Fatal(err)
	}
	resumen := tr.Resumen(inf, nil)
	for _, quiero := range []string{"Calendar: 1", "2 ficheros se han dejado tal cual en " + filepath.Join(datos, "Drive", "Traído de Sytue "+time.Now().Format("2006-01-02"))} {
		if !strings.Contains(resumen, quiero) {
			t.Errorf("falta %q en el resumen:\n%s", quiero, resumen)
		}
	}
	// Otra vez: el evento ya estaba, y lo dice de Calendar.
	inf, err = traer.Traer(context.Background(), zipRuta, tr.Destino)
	if err != nil {
		t.Fatal(err)
	}
	resumen = tr.Resumen(inf, nil)
	if !strings.Contains(resumen, "1 ya estaban aquí de otra vez y no se han repetido (Calendar: 1).") || strings.Contains(resumen, "Calendar: 1\n") {
		t.Errorf("la segunda vez:\n%s", resumen)
	}

	// Un ZIP que no es de Sytue lo dice.
	otro := filepath.Join(t.TempDir(), "otro.zip")
	if err := os.WriteFile(otro, []byte("no es un zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	tr.Mirar(otro)
	if got := textosEnCapas(w); got == "" {
		t.Fatal("un fichero que no es un ZIP no dice nada")
	}
}

// HU-07: «Traer» escribe también en Photos, así que con Photos abierta pide cerrarla; y
// al acabar dice cuántas fotos y notas ya estaban de otra vez y no se han repetido.
func TestTraerPideCerrarPhotosYDiceLoQueYaEstaba(t *testing.T) {
	test.NewTempApp(t)
	w := test.NewWindow(nil)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(1000, 700))
	var pedidas []string
	tr := &Traedor{
		W: w, Idioma: "es",
		Destino: traer.Destino{Datos: t.TempDir(), Sujeto: "yo", Persona: biblioteca.Persona{ID: "yo"}},
		Abiertas: func(cs []string) ([]string, bool) {
			pedidas = cs
			return []string{"photos"}, true
		},
	}
	tr.Mirar(zipDeSytue(t))
	if got := textosEnCapas(w); !strings.Contains(got, "Cierra antes las apps") || !strings.Contains(got, "Photos") {
		t.Fatalf("con Photos abierta:\n%s", got)
	}
	if !slices.Contains(pedidas, "photos") || !slices.Contains(pedidas, "notes") {
		t.Errorf("no se mira si Photos y Notepads están abiertas: %v", pedidas)
	}

	resumen := tr.Resumen(traer.Informe{Partes: []traer.Parte{
		{Producto: "notes", Importados: 1, Repetidos: 4},
		{Producto: "photos", Repetidos: 3},
	}}, nil)
	for _, quiero := range []string{"Notepads: 1", "7 ya estaban aquí de otra vez y no se han repetido (Notepads: 4 · Photos: 3)."} {
		if !strings.Contains(resumen, quiero) {
			t.Errorf("falta %q en el resumen:\n%s", quiero, resumen)
		}
	}
	if strings.Contains(tr.Resumen(traer.Informe{Partes: []traer.Parte{{Producto: "photos", Repetidos: 3}}}, nil), "No había nada") {
		t.Error("con todo repetido dice que no había nada")
	}
}

// HU-07: al traer otra vez, el resumen dice de cada app cuánto ya estaba, cuánto se ha
// puesto al día, cuánto había cambiado también aquí y cuánto se quitó aquí y no vuelve;
// en inglés también. Si solo hay de eso, no dice que no había nada.
func TestElResumenDiceLoDeCadaAppAlTraerOtraVez(t *testing.T) {
	inf := traer.Informe{Partes: []traer.Parte{
		{Producto: "calendar", Importados: 1, Repetidos: 1, Actualizados: 2, Conservados: 1, Borrados: 1},
		{Producto: "contacts", Repetidos: 2, Actualizados: 1, Conservados: 1},
		{Producto: "docs", Actualizados: 1, Conservados: 1},
		{Producto: "mail", Repetidos: 2},
	}}
	for idioma, quiero := range map[string][]string{
		"es": {
			"Calendar: 1",
			"5 ya estaban aquí de otra vez y no se han repetido (Calendar: 1 · Contacts: 2 · Mail: 2).",
			"4 habían cambiado en Sytue y, como aquí no se habían tocado, se han puesto al día (Calendar: 2 · Contacts: 1 · Docus, Calcus, Diapos: 1).",
			"3 habían cambiado aquí y también en Sytue: lo de aquí se queda como estaba y lo de Sytue se ha dejado aparte (Calendar: 1 · Contacts: 1 · Docus, Calcus, Diapos: 1).",
			"1 que quitaste aquí no se han vuelto a traer (Calendar: 1).",
		},
		"en": {"4 had changed in Sytue and, as they hadn't been touched here, were brought up to date (Calendar: 2 · Contacts: 1 · Docus, Calcus, Diapos: 1)."},
	} {
		tr := &Traedor{Idioma: i18n.Locale(idioma)}
		resumen := tr.Resumen(inf, nil)
		for _, q := range quiero {
			if !strings.Contains(resumen, q) {
				t.Errorf("%s: falta %q en el resumen:\n%s", idioma, q, resumen)
			}
		}
	}
	tr := &Traedor{Idioma: "es"}
	if got := tr.Resumen(traer.Informe{Partes: []traer.Parte{{Producto: "docs", Conservados: 1}}}, nil); strings.Contains(got, "No había nada") {
		t.Errorf("con solo lo cambiado aquí dice que no había nada:\n%s", got)
	}
}

// zipDeFotos es una copia de «Tus datos» con los originales y las tablas de Fotos de
// ejemplo de `nucleo/local/traer` (las escribe el exportador del servicio): fotos, un
// vídeo WebM, una AVIF y una HEIC.
func zipDeFotos(t *testing.T) string {
	t.Helper()
	base := filepath.Join("..", "..", "..", "nucleo", "local", "traer", "testdata", "tusdatos")
	const raiz = "sytue-one-2026-11-20/"
	ficheros := map[string]string{}
	err := filepath.WalkDir(filepath.Join(base, "drive"), func(ruta string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(filepath.Join(base, "drive"), ruta)
		ficheros[raiz+"drive/files/"+filepath.ToSlash(rel)] = ruta
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"photos", "albums"} {
		ficheros[raiz+"photos/"+n+".json"] = filepath.Join(base, "photos", n+".json")
	}
	ruta := filepath.Join(t.TempDir(), "sytue-one-2026-11-20.zip")
	f, err := os.Create(ruta)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for nombre, origen := range ficheros {
		datos, err := os.ReadFile(origen)
		if err != nil {
			t.Fatal(err)
		}
		w, err := zw.Create(nombre)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(datos); err != nil {
			t.Fatal(err)
		}
	}
	if err := errors.Join(zw.Close(), f.Close()); err != nil {
		t.Fatal(err)
	}
	return ruta
}

// HU-07: con lo que lleva el Hub —los formatos de Photos (`local/imagenes`) y su medidor
// de vídeos (`local/videos`)—, la AVIF y el vídeo de la copia entran en Photos como
// desde la propia Photos; la HEIC, sin conversor, se queda en Drive y el resumen dice por
// qué y cómo arreglarlo. Traer otra vez la copia no copia nada a Drive y lo dice.
func TestTraerLlevaAPhotosLosVideosYLasAVIF(t *testing.T) {
	test.NewTempApp(t)
	ctx := t.Context()
	zipRuta := zipDeFotos(t)
	datos := t.TempDir()
	tr := &Traedor{Idioma: "es", Destino: traer.Destino{Datos: datos, Sujeto: "yo", Persona: biblioteca.Persona{ID: "yo"}, Video: medirVideo}}
	inf, err := traer.Traer(ctx, zipRuta, tr.Destino)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := fototeca.Abrir(ctx, filepath.Join(datos, "Datos", "photos"), "yo", fototeca.Opciones{})
	if err != nil {
		t.Fatal(err)
	}
	linea, _, err := repo.Timeline(ctx, "", 50, false, "")
	repo.Close()
	if err != nil {
		t.Fatal(err)
	}
	por := map[string]api.Photo{}
	for _, f := range linea {
		por[f.Name] = f
	}
	if flor := por["flor.avif"]; flor.ContentType != "image/avif" || flor.Externo == "" || flor.Description != "Una flor" {
		t.Errorf("la AVIF no ha entrado en Photos: %+v", flor)
	}
	if paseo := por["paseo.webm"]; paseo.DurationMs < 2000 || paseo.Width != 320 || paseo.Height != 180 {
		t.Errorf("el vídeo no se ha medido como en Photos: %+v", paseo)
	}
	if _, ok := por["IMG_0001.jpg"]; ok || len(linea) != 6 {
		t.Errorf("fotos: %+v", linea)
	}
	resumen := tr.Resumen(inf, nil)
	for _, quiero := range []string{
		"Photos: 6", "1 fotos o vídeos se han quedado en Drive: Photos no los sabe abrir en este equipo.",
		"hace falta ffmpeg o libheif",
	} {
		if !strings.Contains(resumen, quiero) {
			t.Errorf("falta %q en el resumen:\n%s", quiero, resumen)
		}
	}

	otra, err := traer.Traer(ctx, zipRuta, tr.Destino)
	if err != nil {
		t.Fatal(err)
	}
	resumen = tr.Resumen(otra, nil)
	for _, quiero := range []string{"6 ya estaban aquí de otra vez", "ficheros ya estaban en Drive de otra vez y no se han vuelto a copiar."} {
		if !strings.Contains(resumen, quiero) {
			t.Errorf("falta %q en el resumen de la segunda vez:\n%s", quiero, resumen)
		}
	}
	if strings.Contains(resumen, "se han dejado tal cual") || strings.Contains(resumen, "No había nada") {
		t.Errorf("la segunda vez:\n%s", resumen)
	}
}
