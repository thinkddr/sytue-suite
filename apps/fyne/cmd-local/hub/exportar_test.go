// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
	"git.sytue.eu/sytue/one/apps/nucleo/local/exportar"
	"git.sytue.eu/sytue/one/apps/nucleo/local/traer"
)

// «Exportar todo»: con alguna app abierta se pide cerrarla y no se elige carpeta; sin
// saberlo, se pide a la persona antes; cerradas, se elige la carpeta, se exporta en una
// nueva con la fecha y se dice qué apps han quedado allí.
func TestExportarTodoPideCerrarYDiceDondeQueda(t *testing.T) {
	test.NewTempApp(t)
	w := test.NewWindow(nil)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(1000, 700))
	datos := t.TempDir()
	// Lo de Calendar, con «Traer desde Sytue» (el camino inverso).
	if _, err := traer.Traer(context.Background(), zipDeSytue(t), traer.Destino{Datos: datos, Sujeto: "yo", Persona: biblioteca.Persona{ID: "yo"}}); err != nil {
		t.Fatal(err)
	}
	abiertasAhora, sabido := []string{"calendar", "notes"}, true
	var pidio []string
	var elegida func(string)
	x := &Exportador{
		W: w, Idioma: "es",
		Origen:   exportar.Origen{Datos: datos, Sujeto: "yo", Persona: biblioteca.Persona{ID: "yo"}},
		Abiertas: func(cs []string) ([]string, bool) { pidio = cs; return abiertasAhora, sabido },
		Elegir:   func(_ string, listo func(string)) { elegida = listo },
		Exportar: exportar.Todo,
		Ahora:    func() time.Time { return time.Date(2026, 11, 25, 10, 30, 0, 0, time.UTC) },
	}
	cerrarCapas := func() {
		for _, c := range w.Canvas().Overlays().List() {
			w.Canvas().Overlays().Remove(c)
		}
	}

	x.Empezar()
	if got := textosEnCapas(w); !strings.Contains(got, "Cierra antes las apps") || !strings.Contains(got, "Calendar, Notepads") {
		t.Fatalf("con apps abiertas:\n%s", got)
	}
	if elegida != nil {
		t.Fatal("con apps abiertas se ha pedido la carpeta")
	}
	// Se pregunta por todas las que exportan, y solo por ellas.
	for _, c := range []string{"calendar", "contacts", "docus", "calcus", "diapos", "notes", "photos", "bases"} {
		if !slices.Contains(pidio, c) {
			t.Errorf("no se mira si %s está abierta: %v", c, pidio)
		}
	}
	if slices.Contains(pidio, "mail") || slices.Contains(pidio, "drive") {
		t.Errorf("se pide cerrar apps que no exportan: %v", pidio)
	}
	cerrarCapas()

	// Sin poder saberlo, se pide a la persona antes de elegir.
	abiertasAhora, sabido = nil, false
	x.Empezar()
	if got := textosEnCapas(w); !strings.Contains(got, "Mientras se exporta tienen que estar cerradas") {
		t.Fatalf("sin saberlo:\n%s", got)
	}
	if elegida != nil {
		t.Fatal("se ha pedido la carpeta sin confirmar")
	}
	cerrarCapas()

	// Cerradas: la carpeta y, dentro, «Sytue Office <fecha>/Calendar/Casa.ics».
	sabido = true
	x.Empezar()
	if elegida == nil {
		t.Fatal("no se ha pedido la carpeta")
	}
	base := t.TempDir()
	partes, err := x.Exportar(context.Background(), x.Origen, filepath.Join(base, "Sytue Office 2026-11-25 10.30"), nombreApp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(base, "Sytue Office 2026-11-25 10.30", "Calendar", "Casa.ics")); err != nil {
		t.Fatal(err)
	}
	resumen, hecho := x.Resumen("/ahí", partes, nil)
	if !hecho || resumen != "Lo de Calendar está en /ahí, en formatos que abren otros programas." {
		t.Fatal(resumen)
	}
	// Nada que exportar, y lo que no se pudo.
	if r, hecho := x.Resumen("/ahí", nil, nil); hecho || r != "No había nada que exportar en este equipo." {
		t.Fatal(r)
	}
	r, _ := x.Resumen("/ahí", []exportar.Parte{{Producto: "photos", Ficheros: 3, Fallidos: 2}}, errors.New("roto"))
	if !strings.Contains(r, "Photos: 2 no se han podido copiar") || !strings.HasSuffix(r, "roto") {
		t.Fatal(r)
	}
}

// El flujo entero, con la espera y el aviso del final: la carpeta con la fecha se crea
// dentro de la elegida y, sin nada que exportar, no se queda vacía.
func TestExportarTodoSinNadaNoDejaCarpeta(t *testing.T) {
	test.NewTempApp(t)
	w := test.NewWindow(nil)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(1000, 700))
	base := t.TempDir()
	hecho := make(chan struct{})
	x := &Exportador{
		W: w, Idioma: "es",
		Origen:   exportar.Origen{Datos: t.TempDir(), Sujeto: "yo"},
		Abiertas: func([]string) ([]string, bool) { return nil, true },
		Elegir:   func(_ string, listo func(string)) { listo(base) },
		Exportar: func(ctx context.Context, o exportar.Origen, destino string, carpeta func(string) string) ([]exportar.Parte, error) {
			defer close(hecho)
			if filepath.Dir(destino) != base || !strings.HasPrefix(filepath.Base(destino), "Sytue Office ") {
				t.Errorf("destino: %s", destino)
			}
			return exportar.Todo(ctx, o, destino, carpeta)
		},
		Ahora: time.Now,
	}
	x.Empezar()
	<-hecho
	for range 50 {
		if strings.Contains(textosEnCapas(w), "No había nada que exportar") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := textosEnCapas(w); !strings.Contains(got, "No había nada que exportar") {
		t.Fatalf("aviso:\n%s", got)
	}
	if es, _ := os.ReadDir(base); len(es) != 0 {
		t.Fatalf("ha dejado una carpeta vacía: %v", es)
	}
}
