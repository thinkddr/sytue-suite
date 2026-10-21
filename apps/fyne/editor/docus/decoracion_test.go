// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image/png"
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
)

// TestLaMarcaYElBordeSePintan: la marca se pone desde la orden de Disposición, se pinta
// como una baldosa por cada 1123 px del papel con su opacidad, y el borde a medio margen.
// Con SYTUE_CAPTURA_DECORACION deja el PNG. Se vio en ROJO sin `pintarDecoracion` en
// `pintar`: no salía ninguna baldosa.
func TestLaMarcaYElBordeSePintan(t *testing.T) {
	f := folioCon(t, `<h1>Propuesta</h1><p>Texto del documento.</p>`)
	f.Mandar(cmdMarcaAgua, valorMarca(rico.SanearMarcaAgua(false, "BORRADOR", "", "", "0.25", "120", "1")))
	f.Mandar(cmdBorde, valorBorde(rico.SanearBorde("solid", "3", "#0A0A0A")))
	if !strings.HasPrefix(f.Serializar(), `<div data-sytue-page-border=""`) || !strings.Contains(f.Serializar(), `data-text="BORRADOR"`) {
		t.Fatalf("las capas no están en el folio: %.200s", f.Serializar())
	}
	baldosas, rayas := 0, 0
	for _, o := range f.pintarDecoracion(nil, fyne.NewPos(0, 0), 1) {
		switch x := o.(type) {
		case *canvas.Image:
			baldosas++
			if x.Translucency < 0.74 || x.Translucency > 0.76 {
				t.Errorf("la baldosa tiene transparencia %.2f y la marca es al 25 %%", x.Translucency)
			}
		case *canvas.Rectangle:
			rayas++
			if x.Position().X < MargenPagina/2-0.01 || x.Position().Y < MargenPagina/2-0.01 {
				t.Errorf("el borde empieza en %v y va a medio margen (%g)", x.Position(), MargenPagina/2)
			}
		}
	}
	if baldosas != 1 || rayas != 4 {
		t.Errorf("una baldosa (una página) y cuatro lados: %d y %d", baldosas, rayas)
	}
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetContent(container.NewStack(f))
	c.Resize(fyne.NewSize(900, 1250))
	img := c.Capture()
	// Lo pintado de verdad: la raya del borde a medio margen del papel, en su tinta.
	ox, oy := f.pag.origen()
	esc := float32(img.Bounds().Dx()) / 900
	px := img.At(int((ox-MargenPagina/2+1)*esc), int((oy+300)*esc))
	if r, g, b, _ := px.RGBA(); r > 0x3000 || g > 0x3000 || b > 0x3000 {
		t.Errorf("en el borde hay %v y tiene que estar la raya negra", px)
	}
	if ruta := os.Getenv("SYTUE_CAPTURA_DECORACION"); ruta != "" {
		fh, err := os.Create(ruta)
		if err != nil {
			t.Fatal(err)
		}
		defer fh.Close()
		_ = png.Encode(fh, img)
	}
	f.Mandar(cmdMarcaAgua, "")
	f.Mandar(cmdBorde, "")
	if got := f.Serializar(); got != `<h1>Propuesta</h1><p>Texto del documento.</p>` {
		t.Errorf("quitar la decoración: %s", got)
	}
}

// TestLasColumnasSeReparten: dos columnas del mismo ancho con el hueco de 2 em; los bloques
// se reparten para que las dos midan parecido, y un clic en la segunda cae en ella.
func TestLasColumnasSeReparten(t *testing.T) {
	f := folioCon(t, `<p>uno</p><p>dos</p><p>tres</p><p>cuatro</p><p>fuera</p>`)
	f.anc, f.cur = punto{0, 0}, punto{3, 0}
	f.Mandar(cmdColumnas, "2")
	if !strings.HasPrefix(f.Serializar(), `<div class="sytue-cols" data-cols="2" style="column-count: 2; column-gap: 2em;"><p>uno</p>`) {
		t.Fatalf("las columnas: %s", f.Serializar())
	}
	w := (f.anchoTexto() - huecoColumnas) / 2
	izq := f.cajas[2].x
	if f.cajas[0].x != 0 || f.cajas[1].x != 0 || izq != w+huecoColumnas || f.cajas[3].x != izq || f.cajas[2].y != f.cajas[0].y {
		t.Errorf("el reparto: %v %v %v %v", f.cajas[0].x, f.cajas[1].x, f.cajas[2].x, f.cajas[3].x)
	}
	if f.cajas[4].y <= f.cajas[1].y+f.cajas[1].alto {
		t.Errorf("lo de fuera va debajo de las columnas: %g", f.cajas[4].y)
	}
	if q := f.enMaqueta(izq+4, f.cajas[2].y+4); q.b != 2 {
		t.Errorf("el clic en la segunda columna cae en el bloque %d", q.b)
	}
	// Desde Disposición, con su selector.
	var sel *editor.SelectorCinta
	for _, p := range piezasDe(nuevoInicio("es").cuerpoDisposicion(f.ctx)) {
		if s, ok := p.(*editor.SelectorCinta); ok && s.Rotulo == "layout.columns.label" {
			sel = s
		}
	}
	if sel == nil {
		t.Fatal("Disposición no tiene «Columnas»")
	}
}

// TestLaPaginaDelDocumento: la página que guarda el documento (un .docx importado: carta
// apaisada, márgenes distintos y papel de color) manda en el folio, y cambiarla en la cinta
// la guarda en él. Antes la página era de la vista: el documento se abría siempre en A4.
func TestLaPaginaDelDocumento(t *testing.T) {
	capa := `<div data-sytue-page="" data-width="1056" data-height="816" data-margins="48 72 96 120" data-color="#FFF2CC" contenteditable="false" aria-hidden="true"></div>`
	f := folioCon(t, capa+`<p>Hola</p>`)
	if ancho, alto := f.Pagina().medidas(); ancho != 1056 || alto != 816 || f.anchoTexto() != 1056-72-120 || f.Pagina().Papel.A == 0 {
		t.Fatalf("la página es %v×%v con %v de texto", ancho, alto, f.anchoTexto())
	}
	if got := f.Serializar(); got != capa+`<p>Hola</p>` {
		t.Errorf("abrir el documento lo cambia: %s", got)
	}
	f.GuardarPagina(Pagina{Margen: 48, Tamano: "a4"})
	if got := f.Serializar(); !strings.HasPrefix(got, `<div data-sytue-page="" data-width="794" data-height="1123" data-margins="48 48 48 48"`) {
		t.Errorf("la página nueva no se guarda: %.200s", got)
	}
	f.Mandar(editor.Deshacer, "")
	if ancho, _ := f.Pagina().medidas(); ancho != 1056 || !strings.HasPrefix(f.Serializar(), capa) {
		t.Errorf("deshacer no devuelve la página de antes: %v de ancho, %.200s", ancho, f.Serializar())
	}
	f.GuardarPagina(PaginaPorDefecto())
	if got := f.Serializar(); strings.Contains(got, "data-sytue-page") {
		t.Errorf("la página por defecto no necesita capa: %.200s", got)
	}
}
