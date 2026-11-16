// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestLaCitaSeSeparaComoEnLaWeb recorre testdata/cita.json, que recorre también
// utils.test.ts.
//
// VISTO EN ROJO (2026-11-09): sin la coma opcional no se reconocía la cabecera de
// Gmail en inglés; sin el espacio de no separación, la de Apple Mail en francés; sin
// el «no queda nada nuevo», un mensaje que era solo cita salía vacío. Deshecho.
func TestLaCitaSeSeparaComoEnLaWeb(t *testing.T) {
	crudo, err := os.ReadFile("testdata/cita.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Casos []struct{ Nombre, Texto, Cuerpo, Cita string }
	}
	if err := json.Unmarshal(crudo, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Casos) < 10 {
		t.Fatalf("solo hay %d casos", len(v.Casos))
	}
	for _, c := range v.Casos {
		cuerpo, cita := separarCita(c.Texto)
		if cuerpo != c.Cuerpo || cita != c.Cita {
			t.Errorf("%s:\n cuerpo %q\n   cita %q\nquiero\n cuerpo %q\n   cita %q", c.Nombre, cuerpo, cita, c.Cuerpo, c.Cita)
		}
	}
}

// TestLaCitaReconoceLaCabeceraDeCadaIdioma: la atribución que escribe la propia
// respuesta de Sytue, en cualquiera de los idiomas del catálogo, abre la cita.
func TestLaCitaReconoceLaCabeceraDeCadaIdioma(t *testing.T) {
	if n := len(atribuciones()); n < 20 {
		t.Fatalf("solo hay %d atribuciones; el catálogo trae una por idioma", n)
	}
	for _, l := range []i18n.Locale{"es", "en", "fr", "de", "hu", "fi", "el", "bg", "mt"} {
		tr, err := Traductor(l)
		if err != nil {
			t.Fatal(err)
		}
		cabecera := tr.T("reader.quoteHeader", i18n.Vars{"date": "5 ene 2026, 10:00", "name": "Ana Ruiz"})
		if cuerpo, _ := separarCita("Vale.\n\n" + cabecera + "\n> Hola"); cuerpo != "Vale." {
			t.Errorf("%s: %q no abre la cita; lo nuevo sale %q", l, cabecera, cuerpo)
		}
	}
}

// TestLaTarjetaPliegaLaCita: una respuesta enseña lo nuevo y el botón; el botón
// despliega el texto íntegro (y la tarjeta crece), y plegarla lo vuelve a esconder.
// Un mensaje que es solo cita no lleva botón.
func TestLaTarjetaPliegaLaCita(t *testing.T) {
	tr := traductor(t)
	l := NuevoLector(tr, "es")
	texto := "Perfecto, lo miro.\n\nEl 5 ene 2026, Ana Ruiz escribió:\n> ¿Lo revisas?\n> Ana"
	l.Cuerpo = func(string) (Cuerpo, bool) { return Cuerpo{Texto: texto}, true }
	tj := nuevaTarjetaMensaje(l, MensajeDe(api.MailMessage{ID: "m1", From: "ana@ejemplo.eu"}), true)
	tj.ancho = 600
	w := test.NewWindow(tj)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(600, 600))
	r := test.WidgetRenderer(tj).(*tarjetaRenderer)

	visto := func() string {
		var b strings.Builder
		for _, p := range append(slices.Clone(r.parrafo), r.citadas...) {
			b.WriteString(p.Text + "\n")
		}
		return b.String()
	}
	if s := visto(); !strings.Contains(s, "Perfecto, lo miro.") || strings.Contains(s, "¿Lo revisas?") {
		t.Fatalf("plegada, la tarjeta pinta %q", s)
	}
	if !r.botonCita.Visible() || r.botonCita.Texto != tr.T("reader.showQuoted", nil) || r.botonCita.IconoDetras != iconos.ChevronAbajo {
		t.Fatalf("el botón de la cita: visible %v, %q", r.botonCita.Visible(), r.botonCita.Texto)
	}
	plegada := MedidaTarjeta(tj, 600)

	fyne.DoAndWait(func() { test.Tap(r.botonCita) })
	s := visto()
	for _, linea := range strings.Split(texto, "\n") {
		if strings.TrimSpace(linea) != "" && !strings.Contains(s, linea) {
			t.Errorf("desplegada falta %q; pinta %q", linea, s)
		}
	}
	if r.botonCita.Texto != tr.T("reader.hideQuoted", nil) || r.botonCita.IconoDetras != iconos.ChevronArriba {
		t.Errorf("desplegada, el botón dice %q", r.botonCita.Texto)
	}
	if d := MedidaTarjeta(tj, 600); d <= plegada {
		t.Errorf("desplegar la cita no hace crecer la tarjeta: %v → %v", plegada, d)
	}

	fyne.DoAndWait(func() { test.Tap(r.botonCita) })
	if s := visto(); strings.Contains(s, "¿Lo revisas?") {
		t.Errorf("plegada otra vez, pinta %q", s)
	}
	if MedidaTarjeta(tj, 600) != plegada {
		t.Error("plegar la cita no devuelve la tarjeta a su alto")
	}

	solo := "El 5 ene 2026, Ana Ruiz escribió:\n> Hola"
	l.Cuerpo = func(string) (Cuerpo, bool) { return Cuerpo{Texto: solo}, true }
	fyne.DoAndWait(tj.Refresh)
	if r.botonCita.Visible() || !strings.Contains(visto(), "> Hola") {
		t.Errorf("un mensaje que es solo cita se pliega: botón %v, pinta %q", r.botonCita.Visible(), visto())
	}
}
