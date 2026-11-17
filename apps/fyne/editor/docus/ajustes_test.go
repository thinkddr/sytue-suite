// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image/png"
	"math"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"
)

// ajustesDe monta el panel «Ajustes» del folio y lo devuelve ya leído.
func ajustesDe(f *Folio) *panelAjustes {
	f.PanelAjustes(nil)
	return f.ajustes
}

func (p *panelAjustes) paso(clave string) *pasoAjuste {
	for _, s := range p.pasos {
		if s.clave == clave {
			return s
		}
	}
	return nil
}

// teclear escribe un valor en la entrada de un paso y lo confirma con Intro, como la web
// con su `change`.
func teclear(s *pasoAjuste, texto string) {
	s.entrada.PonerTexto(texto)
	s.entrada.AlTeclear(&fyne.KeyEvent{Name: fyne.KeyReturn})
}

// TestAjustesEspacioDespuesEnTodaLaSeleccion: «Espacio después» a 12 pt escribe
// `margin-bottom:16px` en TODOS los bloques de la selección, remaqueta y cuenta como un
// cambio que guardar; deshacer lo quita de un paso.
func TestAjustesEspacioDespuesEnTodaLaSeleccion(t *testing.T) {
	const antes = "<p>uno</p><p>dos</p><p>tres</p>"
	f := folioCon(t, antes)
	f.mover(punto{0, 1}, false)
	f.mover(punto{1, 2}, true)
	p := ajustesDe(f)
	cambios := f.Cambios()
	teclear(p.paso("settings.spacing.after"), "12")
	if got := f.Serializar(); got != `<p style="margin-bottom: 16px;">uno</p><p style="margin-bottom: 16px;">dos</p><p>tres</p>` {
		t.Fatalf("12 pt después: %s", got)
	}
	if f.bloques[0].Abajo != 16 || f.bloques[1].Abajo != 16 {
		t.Errorf("la maqueta no se ha rehecho con el espaciado: %v, %v", f.bloques[0].Abajo, f.bloques[1].Abajo)
	}
	if f.Cambios() == cambios {
		t.Error("el ajuste no cuenta como cambio: no se guardaría")
	}
	if got := p.paso("settings.spacing.after").entrada.Texto(); got != "12" {
		t.Errorf("la entrada tras aplicar dice %q", got)
	}
	f.deshacer()
	if got := f.Serializar(); got != antes {
		t.Errorf("deshacer: %s", got)
	}
}

// TestAjustesLeeLoMismoQueLaWeb: para el mismo HTML, el panel enseña lo que la web saca
// de `getComputedStyle` (px a cm y a pt, el interlineado como múltiplo).
func TestAjustesLeeLoMismoQueLaWeb(t *testing.T) {
	f := folioCon(t, `<p style="margin-top: 8px; margin-left: 37.8px; margin-right: 75.6px; text-indent: -18.9px; line-height: 2;">x</p>`)
	f.mover(punto{0, 0}, false)
	p := ajustesDe(f)
	quiere := map[string]string{
		"settings.spacing.before":     "6",
		"settings.spacing.lineHeight": "2",
		"settings.indent.left":        "1",
		"settings.indent.right":       "2",
		"settings.indent.firstLine":   "-0.5",
	}
	for clave, v := range quiere {
		if got := p.paso(clave).entrada.Texto(); got != v {
			t.Errorf("%s = %q, la web enseña %q", clave, got, v)
		}
	}
	if p.pista.Visible() {
		t.Error("con un párrafo bajo el cursor no se enseña la pista de «selecciona un párrafo»")
	}
}

// TestAjustesPasosYTopes: −/+ saltan su paso y no salen de los topes de la web.
func TestAjustesPasosYTopes(t *testing.T) {
	f := folioCon(t, "<p>x</p>")
	f.mover(punto{0, 0}, false)
	p := ajustesDe(f)
	test.Tap(p.paso("settings.indent.left").mas)
	if got := f.Serializar(); got != `<p style="margin-left: 9.45px;">x</p>` {
		t.Errorf("+0,25 cm: %s", got)
	}
	primera := p.paso("settings.indent.firstLine")
	teclear(primera, "-9")
	if got := primera.entrada.Texto(); got != "-5" {
		t.Errorf("la primera línea se acota a −5 cm y dice %q", got)
	}
	teclear(primera, "abc")
	if got := primera.entrada.Texto(); got != "-5" {
		t.Errorf("lo que no es un número vuelve al valor: %q", got)
	}
	if v := float64(f.bloques[0].Base.Sangria1); math.Abs(v+5*pxPorCm) > 0.01 {
		t.Errorf("sangría de primera línea = %v px", v)
	}
}

// TestAjustesDeTablaComoLaWeb: con el cursor en una tabla salen sus tres opciones, y cada
// una escribe el HTML que escribe `SeccionTabla`; el documento sigue siendo editable.
func TestAjustesDeTablaComoLaWeb(t *testing.T) {
	const antes = `<table><tbody><tr><td>a</td><td>b</td></tr><tr><td>c</td></tr><tr><td>e</td></tr></tbody></table>`
	f := folioCon(t, antes)
	f.mover(punto{0, 0}, false)
	p := ajustesDe(f)
	if !p.enTabla || p.cuerpo.Objects[0] != p.tabla {
		t.Fatal("con el cursor en una celda el panel enseña la sección de tabla")
	}
	if !p.bordes.Encendido || p.cabecera.Encendido || p.anchoAut.Encendido {
		t.Errorf("estado inicial: bordes %v, cabecera %v, ancho %v", p.bordes.Encendido, p.cabecera.Encendido, p.anchoAut.Encendido)
	}
	test.Tap(p.cabecera)
	const bandas = `<table data-cabecera-alternada=""><tbody>` +
		`<tr style="background-color: rgb(226, 232, 240);"><td style="font-weight: 600;">a</td><td style="font-weight: 600;">b</td></tr>` +
		`<tr><td>c</td></tr><tr style="background-color: rgb(241, 245, 249);"><td>e</td></tr></tbody></table>`
	if got := f.Serializar(); got != bandas {
		t.Errorf("cabecera alternada:\n got %s\nwant %s", got, bandas)
	}
	if !p.cabecera.Encendido {
		t.Error("tras aplicarla, la cabecera alternada se relee encendida")
	}
	if f.bloques[0].Celda.Fondo.A == 0 || f.bloques[2].Celda.Fondo.A != 0 || f.bloques[3].Celda.Fondo.A == 0 {
		t.Error("las celdas no pintan el fondo de su fila")
	}
	test.Tap(p.cabecera)
	test.Tap(p.bordes)
	test.Tap(p.anchoAut)
	const sinBordes = `<table style="table-layout: auto; width: auto;"><tbody><tr><td style="border: none;">a</td><td style="border: none;">b</td></tr>` +
		`<tr><td style="border: none;">c</td></tr><tr><td style="border: none;">e</td></tr></tbody></table>`
	if got := f.Serializar(); got != sinBordes {
		t.Errorf("sin bordes y ancho automático:\n got %s\nwant %s", got, sinBordes)
	}
	if !f.doc.Editable() || p.bordes.Encendido || !p.anchoAut.Encendido {
		t.Errorf("editable %v, bordes %v, ancho %v", f.doc.Editable(), p.bordes.Encendido, p.anchoAut.Encendido)
	}
	for range 4 {
		f.deshacer()
	}
	if got := f.Serializar(); got != antes {
		t.Errorf("deshacer los cuatro: %s", got)
	}
}

// TestAjustesEnSoloLecturaApagaTodo: sin permiso de edición nada del panel responde.
func TestAjustesEnSoloLecturaApagaTodo(t *testing.T) {
	f := folioCon(t, `<p>x</p><table><tbody><tr><td>a</td></tr></tbody></table>`)
	f.ctx.SoloLectura = true
	f.mover(punto{0, 0}, false)
	p := ajustesDe(f)
	s := p.paso("settings.spacing.after")
	if !s.mas.Disabled() || !s.menos.Disabled() {
		t.Error("en solo lectura los −/+ están apagados")
	}
	teclear(s, "12")
	if got := f.Serializar(); got != `<p>x</p><table><tbody><tr><td>a</td></tr></tbody></table>` {
		t.Errorf("en solo lectura no se escribe: %s", got)
	}
	f.mover(punto{1, 0}, false)
	p.refrescar()
	if !p.enTabla || !p.bordes.Disabled() || !p.cabecera.Disabled() || !p.anchoAut.Disabled() {
		t.Error("en solo lectura los conmutadores de tabla están apagados")
	}
}

// TestElPanelAjustesSePinta pinta el panel a su ancho (`--editor-panel-w`, 288 px) y
// comprueba que la sección de párrafo cabe sin desbordar. Con SYTUE_CAPTURA_AJUSTES
// escribe el PNG para mirarlo.
func TestElPanelAjustesSePinta(t *testing.T) {
	f := folioCon(t, "<p>x</p>")
	f.mover(punto{0, 0}, false)
	o := f.PanelAjustes(traductorPlay(t))
	if ancho := f.ajustes.parrafo.MinSize().Width; ancho > 288 {
		t.Errorf("la sección de párrafo pide %v px y el panel mide 288", ancho)
	}
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetContent(o)
	c.SetScale(1)
	c.Resize(fyne.NewSize(288, 420))
	img := c.Capture()
	if ruta := os.Getenv("SYTUE_CAPTURA_AJUSTES"); ruta != "" {
		w, err := os.Create(ruta)
		if err != nil {
			t.Fatal(err)
		}
		defer w.Close()
		if err := png.Encode(w, img); err != nil {
			t.Fatal(err)
		}
	}
}
