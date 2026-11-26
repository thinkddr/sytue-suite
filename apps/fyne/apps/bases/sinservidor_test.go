// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"net/http/httptest"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestSinServidorNoOfreceCompartirNiAbreElTiempoReal: en la edición local la base se
// abre igual, pero sin «Compartir», sin la actividad en el menú y sin pedir el flujo de
// eventos ni latir (que sin servidor solo diría «se ha perdido la conexión").
func TestSinServidorNoOfreceCompartirNiAbreElTiempoReal(t *testing.T) {
	f := nuevoFalso(1)
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	tr, err := Traductor("es")
	if err != nil {
		t.Fatal(err)
	}
	w := test.NewWindow(nil)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	p := NuevaPantalla(tr, api.NewBases(srv.URL, nil, nil), w)
	p.fondo = func(g func()) { g() }
	p.SinServidor = true
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	p.Abrir("b1")

	if p.abierta == nil || p.abierta.rt != nil {
		t.Fatal("sin servidor no se sigue la base en tiempo real")
	}
	if boton(w, "Compartir") != nil {
		t.Error("sin servidor se ofrece «Compartir»")
	}
	if got := f.peticiones("GET /bases/b1/events"); len(got) != 0 {
		t.Errorf("sin servidor se pidió el flujo de eventos: %q", got)
	}
	// Con el valor de siempre, la conectada lo sigue ofreciendo.
	p2, w2 := abrir(t, nuevoFalso(1))
	if boton(w2, "Compartir") == nil {
		t.Error("la conectada ha dejado de ofrecer «Compartir»")
	}
	p2.dejarDeSeguir(p2.abierta)
}

// tiposDeVista son los tipos que ofrece el diálogo «Añadir vista», y lo cierra.
func tiposDeVista(t *testing.T, p *Pantalla, w fyne.Window) []string {
	t.Helper()
	p.dialogoDeVista(nil)
	var tipos []string
	enCapas(w, func(o fyne.CanvasObject) {
		if s, ok := o.(*ui.Selector); ok && s.Rotulo == "Tipo" {
			tipos = slices.Clone(s.Opciones)
		}
	})
	for _, capa := range w.Canvas().Overlays().List() {
		w.Canvas().Overlays().Remove(capa)
	}
	return tipos
}

// B-05 y B-06: con SinFormularios no se ofrece la vista de formulario (sus enlaces
// públicos piden un servicio) y con ActividadLocal la actividad vuelve al menú aunque no
// haya servidor. Con los valores de siempre, la conectada sigue igual.
func TestSinFormulariosYActividadLocal(t *testing.T) {
	f := nuevoFalso(1)
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	tr, err := Traductor("es")
	if err != nil {
		t.Fatal(err)
	}
	w := test.NewWindow(nil)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	p := NuevaPantalla(tr, api.NewBases(srv.URL, nil, nil), w)
	p.fondo = func(g func()) { g() }
	p.SinServidor, p.SinFormularios = true, true
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	p.Abrir("b1")

	if got := tiposDeVista(t, p, w); len(got) == 0 || slices.Contains(got, "Formulario") {
		t.Errorf("sin formularios se ofrecen los tipos %q", got)
	}
	nombre := p.abierta.Datos.Name
	if got := elementosDe(t, w, nombre); slices.Contains(got, "Actividad") {
		t.Errorf("sin servidor ni actividad local se ofrece la actividad: %q", got)
	}
	p.ActividadLocal = true
	p.Repintar()
	if got := elementosDe(t, w, nombre); !slices.Contains(got, "Actividad") {
		t.Errorf("con actividad local no se ofrece la actividad: %q", got)
	}

	p2, w2 := abrir(t, nuevoFalso(1))
	defer p2.dejarDeSeguir(p2.abierta)
	if got := tiposDeVista(t, p2, w2); !slices.Contains(got, "Formulario") {
		t.Errorf("la conectada ha dejado de ofrecer el formulario: %q", got)
	}
}
