// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestElCampoPropioMideSusClases: un `<input class="border-border rounded-md border px-2
// py-1 text-xs">` escrito a mano (el de los paneles de CertiPDF) mide 16 de línea, 8 de
// `py-1` y 2 de borde —26— y empieza a escribir en 1 + 8. Su borde es `border-border`,
// no el `border-strong` de Form.tsx: es el que se pinta, y con la talla sería el otro.
func TestElCampoPropioMideSusClases(t *testing.T) {
	const m, altoWeb, textoEnX = 10, 26, 1 + 8

	e := NuevaEntrada("").ConPropia(CampoPropio{
		MargenX: tema.Espacio(2), MargenY: tema.Espacio(1), Texto: tema.TextoXS, Borde: "border",
	})
	e.Ancho = 200
	e.PonerTexto("Hxg")
	declarada := e.MinSize()
	casi(t, "el alto de `py-1 text-xs` con borde", declarada.Height, altoWeb)

	r, _ := e.CreateRenderer().(*campoRenderer)
	r.Layout(declarada)
	casi(t, "el texto empieza en x", e.env.Position().X, textoEnX)

	img := colocado(t, e, declarada, m)
	x0, x1, y0, y1 := caja(img, tema.Color("border", tema.Claro))
	mismaCaja(t, "el campo propio", declarada, x0, x1, y0, y1)
}

// TestElSelectorPropioEsElNativoDeChromium: el `<select>` del estado de un comentario
// (`border-border rounded border px-1 py-0.5 text-2xs`, con la flecha del navegador)
// conserva aquí el alto nativo medido de 21 y, de ancho, la opción más ancha con
// su relleno, el borde y los 20,59 de la flecha. La medida histórica de «Descartado»
// (49,41 a 9,625px de letra) escala a 56,47 con text-2xs de 11px: ancho total 87,06.
func TestElSelectorPropioEsElNativoDeChromium(t *testing.T) {
	s := NuevoSelector([]string{"Abierto", "Descartado"}, nil)
	s.Propio = &SelectorPropio{
		Campo: CampoPropio{MargenX: tema.Espacio(1), MargenY: tema.Espacio(0.5), Texto: tema.Texto2XS, Borde: "border", Radio: 0.25 * tema.Rem},
		Alto:  AltoSelectNativo2XS,
	}
	m := s.MinSize()
	casi(t, "el alto del select nativo", m.Height, 21)
	// La opción más ancha con su relleno, el borde y la flecha, REDONDEADO: Chromium da
	// al `<select>` un ancho entero: 87 donde la suma de las cotas externas da 87,06.
	if m.Width != 87 {
		t.Errorf("el select mide %g de ancho; la suma de las cotas de Chromium redondea a 87", m.Width)
	}
	// La flecha y el hueco del texto son del navegador: el texto 4 px más allá del
	// relleno (1 + 4 + 4) y la flecha, de 9,31 de ancho, con su tinta centrada a 9 del
	// borde derecho (de 4,375 a 13,69). Medido a 8 aumentos, igual con `text-2xs`,
	// `text-xs` y `text-sm`; a 1 aumento salía 9,25.
	r, _ := s.CreateRenderer().(*selectorRenderer)
	r.Layout(m)
	casi(t, "el texto del select empieza en x", r.rotulo.Position().X, 9)
	o := r.chevron.Objeto()
	casi(t, "el centro de la flecha", o.Position().X+anchoFlechaNativa/2, m.Width-9.03)
}

// TestLaMarcaDeUnCampoSinClaseEsLaDePreflight: un `<input>` escrito a mano sin clase
// `placeholder:` pinta su marca de agua con la de preflight de Tailwind, la tinta del
// texto al 50 %, y no con `text-text-subtle`: medido en el diálogo «Organizar» de
// CertiPDF, el «1,3,5-8» sale más claro que un texto tenue.
func TestLaMarcaDeUnCampoSinClaseEsLaDePreflight(t *testing.T) {
	sin := temaEditor{}
	con := temaEditor{marcaMitad: true}
	if got := sin.marca(tema.Claro); got != tema.Color("text-subtle", tema.Claro) {
		t.Errorf("la marca de Form.tsx es %v; tiene que ser `text-subtle`", got)
	}
	quiero := tema.Color("text", tema.Claro)
	quiero.A /= 2
	if got := con.marca(tema.Claro); got != quiero {
		t.Errorf("la marca de preflight es %v; tiene que ser el texto al 50 %% (%v)", got, quiero)
	}
	e := NuevaEntrada("x").ConPropia(CampoPropio{Texto: tema.TextoXS, MarcaDelTexto: true})
	if !e.temaEd.marcaMitad {
		t.Error("ConPropia no le pasa MarcaDelTexto al editor")
	}
}

// TestLasCarasQueNoSonPeso: `italic` y `font-mono` se piden como un peso más y llegan a
// todo lo que mide y pinta por Peso —Texto, Medir, Parrafo—. La mono lo es DE VERDAD
// (las íes ocupan lo que las emes), y un campo propio con `Mono` escribe con ella.
func TestLasCarasQueNoSonPeso(t *testing.T) {
	if Cursiva.Cara() != tema.Cursiva || Mono.Cara() != tema.Mono {
		t.Fatalf("caras: cursiva %v, mono %v", Cursiva.Cara().Name(), Mono.Cara().Name())
	}
	if c := Texto("Sin mensaje", tema.TextoSM, Cursiva, tema.Color("text", tema.Claro)); c.FontSource != tema.Cursiva {
		t.Errorf("ui.Texto en cursiva pinta con %v", c.FontSource.Name())
	}
	finas, anchas := Medir("iiiiiiii", tema.TextoXS, Mono).Width, Medir("MMMMMMMM", tema.TextoXS, Mono).Width
	if finas != anchas || finas == Medir("iiiiiiii", tema.TextoXS, Normal).Width {
		t.Errorf("la mono no es mono: «i» %g, «M» %g", finas, anchas)
	}
	p := NuevoParrafo("Sin mensaje", tema.TextoSM, Cursiva, "text-subtle", tema.Claro).A(200)
	r, _ := p.CreateRenderer().(*parrafoRenderer)
	r.Layout(fyne.NewSize(200, 20))
	if len(r.lineas) != 1 || r.lineas[0].FontSource != tema.Cursiva {
		t.Errorf("el párrafo en cursiva no pinta en cursiva")
	}

	a := NuevaAreaTexto("")
	a.ConPropia(CampoPropio{MargenX: tema.Espacio(3), MargenY: tema.Espacio(2), Texto: tema.TextoXS, Mono: true})
	if !a.editor.TextStyle.Monospace || a.temaEd.cara() != tema.Mono {
		t.Error("el área con `Mono` no escribe en mono")
	}
	if (tema.Sytue{}).Font(fyne.TextStyle{Monospace: true}) != tema.Mono {
		t.Error("el tema no resuelve la monoespaciada a JetBrains Mono")
	}
}

// El título de EditorDemo, Chromium remoto 2026-10-04: h28, text-sm13,
// font-semibold600, px-1.5=6, fondo transparente y radio4. La cara del
// editor y la que mide su alto deben coincidir; CampoSM normal sigue normal.
func TestElCampoPropioConPesoUsaLaCaraDelTituloWeb(t *testing.T) {
	e := NuevaEntrada("Título").ConPropia(CampoPropio{
		MargenX: 5, MargenY: 4, Texto: tema.TextoSM,
		Peso: SemiNegrita, Radio: tema.RadioSM, Plano: true, FondoActivo: "bg-subtle",
	})
	e.Ancho = 200
	e.Radio = tema.RadioSM
	e.PonerTexto("Informe trimestral")
	if e.MinSize().Height != 28 || e.radio() != 4 || e.temaEd.Font(fyne.TextStyle{}) != tema.SemiNegra || e.temaEd.cara() != tema.SemiNegra {
		t.Errorf("el título: alto %g, cara %s; Chromium usa 28 y Figtree 600", e.MinSize().Height, e.temaEd.cara().Name())
	}
	r := test.WidgetRenderer(e).(*campoRenderer)
	r.Layout(e.MinSize())
	casi(t, "px-1.5 sin borde visible", e.env.Position().X, 6)
	if r.caja.relleno.A != 0 || r.caja.borde.A != 0 {
		t.Error("el título en reposo no es transparente/sin borde")
	}
	e.FocusGained()
	if r.caja.relleno != tema.Color("bg-subtle", tema.Claro) || !r.aro.Objetos()[0].Visible() {
		t.Error("focus:bg-bg-subtle no conserva el aro de foco")
	}
	if e.MinSize().Height != 28 || e.env.Position().X != 6 {
		t.Error("el foco ha cambiado la geometría del título")
	}
	e.FocusLost()
	if r.caja.relleno.A != 0 || r.aro.Objetos()[0].Visible() {
		t.Error("al salir del foco el título sigue activado")
	}
	normal := NuevaEntrada("")
	if normal.temaEd.Font(fyne.TextStyle{}) != tema.Regular {
		t.Error("Peso cero ha cambiado la cara del campo normal")
	}
}
