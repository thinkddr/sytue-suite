// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/formula/analisis"
)

// libroRelleno: nombres con sus iniciales, dos sin rellenar; B3 va en negrita.
const libroRelleno = `{"active":0,"sheets":[{"name":"H","cells":{"A1":"Nombre","B1":"Iniciales",` +
	`"A2":"Ana García","B2":"AG","A3":"Luis Pérez","B3":"<b></b>","A4":"Marta Ruiz","A5":"Pedro"}}]}`

// TestRellenarConIA: el panel manda lo que pide `PedirRelleno` (los vectores de la web),
// enseña lo propuesto sin escribirlo, descarta una fórmula y, al confirmar, escribe
// conservando el formato de la celda. Con `SYTUE_CAPTURA` deja el panel con la
// propuesta a ancho de escritorio y de móvil.
func TestRellenarConIA(t *testing.T) {
	for _, tam := range []fyne.Size{fyne.NewSize(1280, 800), fyne.NewSize(360, 740)} {
		lz := NuevoLienzo(NuevoLibro())
		lz.T = nil
		if !lz.Cargar(libroRelleno) {
			t.Fatal("el libro se abre en solo lectura")
		}
		var instruccion, contexto string
		lz.IA = func(_ context.Context, i, c string) (string, error) {
			instruccion, contexto = i, c
			return "```\n3\tLP\n4\t=HYPERLINK(A4)\n5\t«P»\n```", nil
		}
		lz.lanzar = func(f func()) { f() }
		lz.Rejilla.Seleccionar(0, 0, false)
		lz.Rejilla.Seleccionar(1, 4, true)
		ctx := &editor.Contexto{Tipo: editor.Hoja, Lienzo: lz}
		cuerpo := lz.cuerpoFormulas(ctx)
		w := test.NewTempWindow(t, container.NewBorder(cuerpo, nil, nil, nil, lz))
		w.Resize(tam)
		b := botonDeCinta(cuerpo, "data.aiFill")
		if b == nil {
			t.Fatal("con servicio de IA, Fórmulas no tiene «Rellenar con IA»")
		}
		b.Tapped(nil)
		pulsar := func(rotulo string) {
			for _, o := range todos(w.Canvas().Overlays().Top()) {
				if b, ok := o.(*editor.BotonPequeno); ok && b.Texto == rotulo && !b.Inerte {
					b.Tapped(nil)
					return
				}
			}
			t.Fatalf("no hay «%s» pulsable", rotulo)
		}
		pulsar("data.aiFill.propose")
		p, _ := analisis.PedirRelleno([]string{"Nombre", "Iniciales"},
			[][]string{{"Ana García", "AG"}, {"Luis Pérez", ""}, {"Marta Ruiz", ""}, {"Pedro", ""}}, 2, 1, "")
		if instruccion != p.Instruccion || contexto != p.Contexto {
			t.Fatalf("se pidió\n%q\n%q", instruccion, contexto)
		}
		if lz.Libro.Crudo(1, 2) != "" {
			t.Fatal("se ha escrito antes de confirmar")
		}
		for _, o := range todos(w.Canvas().Overlays().Top()) {
			if s, ok := o.(*editor.SelectorCinta); ok && s.Size().Width != anchoSelectorCampo {
				t.Errorf("el selector de columna mide %v y en la web %v", s.Size().Width, anchoSelectorCampo)
			}
		}
		if tam.Width == 1280 {
			guarda(t, w.Canvas().Capture())
		} else if d := os.Getenv("SYTUE_CAPTURA"); d != "" {
			t.Setenv("SYTUE_CAPTURA", strings.TrimSuffix(d, ".png")+"-360.png")
			guarda(t, w.Canvas().Capture())
		}
		pulsar("data.aiFill.write")
		if lz.Libro.HTML(1, 2) != "<b>LP</b>" || lz.Libro.Crudo(1, 4) != "P" || lz.Libro.Crudo(1, 3) != "" {
			t.Errorf("tras escribir: B3 %q, B4 %q, B5 %q", lz.Libro.HTML(1, 2), lz.Libro.Crudo(1, 3), lz.Libro.Crudo(1, 4))
		}
		if !lz.Libro.PuedeDeshacer() {
			t.Error("lo escrito no se puede deshacer")
		}
	}
}

// TestSinServicioNoHayRellenoYLosFallosSeDicen: sin IA el botón no sale; un servicio
// sin modelo se dice con su texto; lo que llega de una consulta ya olvidada (se cambió
// la indicación) no se enseña; y `fillcells` no escribe fórmulas ni lo que no es texto.
func TestSinServicioNoHayRellenoYLosFallosSeDicen(t *testing.T) {
	lz := NuevoLienzo(NuevoLibro())
	lz.T = nil
	lz.Cargar(libroRelleno)
	ctx := &editor.Contexto{Tipo: editor.Hoja, Lienzo: lz}
	if botonDeCinta(lz.cuerpoFormulas(ctx), "data.aiFill") != nil {
		t.Error("sin servicio de IA sale el botón")
	}
	if got := claveErrorIA(&api.Error{Status: http.StatusServiceUnavailable, Body: `{"error":{"code":"ai_not_configured"}}`}); got != "ai.error.notConfigured" {
		t.Errorf("sin modelo se dice %q", got)
	}
	if got := claveErrorIA(context.DeadlineExceeded); got != "ai.error.timeout" {
		t.Errorf("fuera de plazo se dice %q", got)
	}

	var tarde func()
	lz.IA = func(context.Context, string, string) (string, error) { return "3\tLP", nil }
	lz.lanzar = func(f func()) { tarde = f }
	lz.Rejilla.Seleccionar(0, 0, false)
	lz.Rejilla.Seleccionar(1, 4, true)
	cuerpo := lz.cuerpoFormulas(ctx)
	w := test.NewTempWindow(t, container.NewBorder(cuerpo, nil, nil, nil, lz))
	w.Resize(fyne.NewSize(1280, 800))
	botonDeCinta(cuerpo, "data.aiFill").Tapped(nil)
	var pista *ui.Entrada
	for _, o := range todos(w.Canvas().Overlays().Top()) {
		if b, ok := o.(*editor.BotonPequeno); ok && b.Texto == "data.aiFill.propose" {
			b.Tapped(nil)
		}
		if e, ok := o.(*ui.Entrada); ok {
			pista = e // la segunda: la indicación
		}
	}
	if tarde == nil || pista == nil {
		t.Fatal("no se ha lanzado la consulta")
	}
	pista.PonerTexto("en mayúsculas")
	tarde()
	for _, o := range todos(w.Canvas().Overlays().Top()) {
		if b, ok := o.(*editor.BotonPequeno); ok && (b.Texto == "data.aiFill.write" || b.Texto == "data.aiFill.busy") {
			t.Errorf("tras cambiar la indicación sigue «%s»", b.Texto)
		}
	}

	lz.Mandar(CmdRellenar, `{"values":{"B3":"=1+1","B4":7}}`)
	if lz.Libro.Crudo(1, 2) != "" || lz.Libro.Crudo(1, 3) != "" {
		t.Error("fillcells escribe una fórmula o algo que no es texto")
	}
}
