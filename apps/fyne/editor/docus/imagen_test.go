// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	imgpng "image/png"
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/driver/software"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
)

// pngLleno es una imagen de un solo color, para verla en las capturas.
func pngLleno(t *testing.T, w, h int, c color.NRGBA) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	var b bytes.Buffer
	if err := imgpng.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

const largo = "Texto que rodea a la imagen y que sigue durante varias líneas para ver dónde vuelve al margen: " +
	"una, dos, tres y cuatro líneas de un párrafo corriente del documento."

// TestElTextoRodeaALaImagen: una imagen «cuadrado» flota a la izquierda con su hueco de 14
// a la derecha y abajo, y las líneas —de este párrafo y del de debajo— se apartan mientras
// la tienen al lado; el párrafo que solo la lleva no mide nada. A la derecha, las líneas se
// acortan por la derecha. Se vio en ROJO sin apartar las líneas: la primera empezaba en 0,
// debajo de la imagen.
func TestElTextoRodeaALaImagen(t *testing.T) {
	src := pngLleno(t, 10, 5, color.NRGBA{R: 200, A: 255})
	img := `<img src="` + src + `" data-wrap="square" data-w="100" data-h="50" data-sytue-image="">`
	f := folioCon(t, `<p>`+img+`</p><p>`+largo+` `+largo+`</p>`)
	if f.cajas[0].alto != 0 {
		t.Errorf("el párrafo con solo la imagen mide %g", f.cajas[0].alto)
	}
	ims := f.imagenes()
	if len(ims) != 1 || ims[0].x != 0 || ims[0].y != 0 || ims[0].w != 100 || ims[0].h != 50 {
		t.Fatalf("la imagen que flota: %+v", ims)
	}
	c := f.cajas[1]
	if c.y != 8 || len(c.lineas) < 4 {
		t.Fatalf("el párrafo de debajo: en %g con %d líneas", c.y, len(c.lineas))
	}
	for _, l := range c.lineas {
		quiero := float32(0)
		if c.y+l.y < 50+14 {
			quiero = 100 + 14
		}
		if l.x0 != quiero {
			t.Errorf("la línea en %g empieza en %g y tiene que empezar en %g", c.y+l.y, l.x0, quiero)
		}
	}
	derecha := strings.Replace(img, `data-wrap="square"`, `data-wrap="square" data-align="right"`, 1)
	f = folioCon(t, `<p>`+derecha+largo+`</p>`)
	l := f.cajas[0].lineas[0]
	var ult pieza
	for _, p := range l.piezas {
		if strings.TrimSpace(p.texto) != "" {
			ult = p // el espacio del final cuelga: no cuenta
		}
	}
	if l.x0 != 0 || ult.x+ult.ancho > f.anchoTexto()-114+0.01 {
		t.Errorf("a la derecha: la línea va de %g a %g y la imagen empieza en %g", l.x0, ult.x+ult.ancho, f.anchoTexto()-114)
	}
	if im := f.imagenes()[0]; im.x != f.anchoTexto()-100 {
		t.Errorf("la imagen a la derecha está en %g y va en %g (tras el hueco de 14)", im.x, f.anchoTexto()-100)
	}
}

// TestLaImagenRecortada: lo que se ve de una imagen recortada ocupa su sitio —el resto se
// come con márgenes negativos— y se pinta el trozo del fichero que queda; «Arriba y abajo»
// va debajo de todo lo que flota (`clear: both`).
func TestLaImagenRecortada(t *testing.T) {
	src := pngLleno(t, 30, 20, color.NRGBA{G: 200, A: 255})
	f := folioCon(t, `<p>a<img src="`+src+`" data-w="300" data-h="200" data-crop="0,0.1,0.25,0" data-sytue-image="">b</p>`)
	var p pieza
	for _, x := range f.cajas[0].lineas[0].piezas {
		if x.img != nil {
			p = x
		}
	}
	if p.ancho != 270 || p.alto != 150 {
		t.Errorf("lo que se ve de la recortada mide %g × %g y en la web 270 × 150", p.ancho, p.alto)
	}
	if r := imagenRecortada(p.img).Bounds(); r.Dx() != 27 || r.Dy() != 15 {
		t.Errorf("el trozo del fichero es %v y tiene que ser 27 × 15", r)
	}
	flota := `<img src="` + src + `" data-wrap="square" data-w="100" data-h="80" data-sytue-image="">`
	bloque := `<img src="` + src + `" data-wrap="topBottom" data-w="50" data-h="20" data-sytue-image="">`
	f = folioCon(t, `<p>`+flota+`x`+bloque+`</p>`)
	for _, l := range f.cajas[0].lineas {
		for _, pz := range l.piezas {
			if pz.img != nil && pz.img.Ajuste == "topBottom" && l.y < 80+14 {
				t.Errorf("la de arriba y abajo empieza en %g, por encima de lo que flota (hasta %d)", l.y, 80+14)
			}
		}
	}
}

// TestSeleccionarYTocarLaImagen: un clic en la imagen la selecciona y la cinta enseña su
// grupo; su tirador de abajo a la derecha la agranda sin deformarla, en un paso de
// deshacer; cambiar el ajuste y quitarla van por la cinta; y un clic fuera la suelta.
func TestSeleccionarYTocarLaImagen(t *testing.T) {
	src := pngLleno(t, 40, 20, color.NRGBA{B: 200, A: 255})
	f := folioCon(t, `<p>a<img src="`+src+`">b</p>`)
	c := software.NewCanvas()
	c.SetContent(container.NewStack(f))
	c.Resize(fyne.NewSize(1000, 900))
	ox, oy := f.pag.origen()
	im := f.imagenes()[0]
	en := func(x, y float32) fyne.Position { return fyne.NewPos(ox+x, oy+y) }
	f.pag.MouseDown(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: en(im.x+5, im.y+5)}, Button: desktop.MouseButtonPrimary})
	if !f.ImagenSeleccionada() || f.cur != (punto{0, 2}) {
		t.Fatalf("el clic no la selecciona: %v, cursor %v", f.sel, f.cur)
	}
	grupo := false
	for _, p := range piezasDe(nuevoInicio("es").cuerpoInsertar(f.ctx)) {
		if s, ok := p.(*editor.SelectorCinta); ok && s.Rotulo == "image.wrap" {
			grupo = s.Valor == "inline"
		}
	}
	if !grupo {
		t.Error("con la imagen seleccionada, Insertar enseña su ajuste («En línea»)")
	}
	// El tirador de abajo a la derecha, 60 px a la derecha: 100 × 50.
	esq := en(im.x+im.w, im.y+im.h)
	f.pag.MouseDown(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: esq}, Button: desktop.MouseButtonPrimary})
	f.pag.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: esq.AddXY(60, 0)}})
	f.pag.DragEnd()
	if got := f.Serializar(); !strings.Contains(got, `data-w="100" data-h="50" data-sytue-image="" contenteditable="false"`) {
		t.Errorf("el tirador: %s", got[min(len(got), 300):])
	}
	f.deshacer()
	if strings.Contains(f.Serializar(), "data-w") {
		t.Error("agrandarla se deshace de un paso")
	}
	f.rehacer()
	f.sel = &punto{0, 1}
	f.Mandar(cmdAjusteImagen, "square")
	if !strings.Contains(f.Serializar(), `data-wrap="square"`) {
		t.Errorf("el ajuste: %.200s", f.Serializar())
	}
	f.sel = &punto{0, 1}
	f.pag.MouseDown(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: en(400, 400)}, Button: desktop.MouseButtonPrimary})
	if f.ImagenSeleccionada() {
		t.Error("un clic fuera suelta la imagen")
	}
	f.sel = &punto{0, 1}
	f.Mandar(cmdEliminarImagen, "")
	if got := f.Serializar(); got != `<p>ab</p>` {
		t.Errorf("eliminarla: %s", got)
	}
}

// TestLaImagenQueFlotaSePinta deja la captura de una imagen con el texto alrededor, con
// SYTUE_CAPTURA_IMAGEN, y la pinta seleccionada: un token inventado entraría en pánico.
func TestLaImagenQueFlotaSePinta(t *testing.T) {
	src := pngLleno(t, 30, 20, color.NRGBA{R: 30, G: 110, B: 180, A: 255})
	img := `<img src="` + src + `" data-wrap="square" data-w="180" data-h="120" data-crop="0.1,0,0,0.1" data-sytue-image="">`
	f := folioCon(t, `<h2>Rodear</h2><p>`+img+largo+`</p><p>`+largo+`</p>`)
	f.sel = &f.imagenes()[0].q
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetContent(container.NewStack(f))
	c.Resize(fyne.NewSize(900, 700))
	img0 := c.Capture()
	if ruta := os.Getenv("SYTUE_CAPTURA_IMAGEN"); ruta != "" {
		fh, err := os.Create(ruta)
		if err != nil {
			t.Fatal(err)
		}
		defer fh.Close()
		_ = imgpng.Encode(fh, img0)
	}
}
