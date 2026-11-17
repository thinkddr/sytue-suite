// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Cómo se pintan los BLOQUES ATÓMICOS de la web ([rico.Atomico]): el dibujo con su PNG de
// respaldo, a lo ancho del folio como su SVG en la web (`width: 100%`), y lo que no tiene
// respaldo —un dibujo guardado sin él, el informe de Power BI— como la tarjeta de la
// leyenda estática del informe (`.sytue-embed-fallback`): su título y que se edita en la
// web. Se seleccionan, se mueven y se borran enteros, como una forma.

// altoEmbed es `--embed-h` del bloque del informe.
const altoEmbed = 460

// altoTarjeta es el de la tarjeta de un dibujo del que no se sabe la proporción, y
// altoTarjetaMin lo mínimo para que quepan sus dos líneas con el `padding: 24px`.
const (
	altoTarjeta    = 160
	altoTarjetaMin = 96
)

// respaldoDibujo es el PNG de respaldo ya descodificado, o nil si no lleva, si pasa del
// tope de imagen de la web (MaxImagen) o si no se puede leer. Lo descodifica imagenDe, con
// la barrera de píxeles de las imágenes del folio.
func respaldoDibujo(a *rico.Atomico) image.Image {
	if a.Clase != rico.ClaseDibujo || a.Respaldo == "" || len(a.Respaldo) > MaxImagen {
		return nil
	}
	img := imagenDe(a.Respaldo)
	if img == nil || img.Bounds().Dx() <= 0 || img.Bounds().Dy() <= 0 {
		return nil
	}
	return img
}

// medidaAtomico es su caja en un ancho: los dos van a lo ancho del folio.
func medidaAtomico(a *rico.Atomico, ancho float32) (w, h float32) {
	if a.Clase == rico.ClaseEmbed {
		return ancho, altoEmbed
	}
	prop := float32(a.Proporcion)
	img := respaldoDibujo(a)
	if prop == 0 && img != nil {
		// la del PNG, acotada como la de la escena: un respaldo de 1 × 10 000 no hace un
		// bloque de un kilómetro
		prop = float32(rico.ProporcionAcotada(float64(img.Bounds().Dx()), float64(img.Bounds().Dy())))
	}
	switch {
	case prop == 0:
		return ancho, altoTarjeta
	case img == nil:
		return ancho, max(ancho*prop, altoTarjetaMin)
	}
	return ancho, ancho * prop
}

// pintarAtomico pinta el respaldo del dibujo estirado a su caja o, sin él, la tarjeta.
func (f *Folio) pintarAtomico(objs []fyne.CanvasObject, c caja, ox, oy, z float32, usadas map[string]*canvas.Image) []fyne.CanvasObject {
	a := c.b.Ilustracion.Atomico
	w, h := medidaAtomico(a, c.ancho)
	pos := fyne.NewPos(ox+c.izq*z, oy+c.y*z)
	if img := respaldoDibujo(a); img != nil {
		ci := f.reutilizar(usadas, "dibujo\x00"+a.Respaldo+"\x00"+strconv.Itoa(int(w*z))+"x"+strconv.Itoa(int(h*z)), func() image.Image { return img })
		ci.Move(pos)
		ci.Resize(fyne.NewSize(w*z, h*z))
		return append(objs, ci)
	}
	return append(objs, f.tarjetaAtomico(a, pos, w*z, h*z, z)...)
}

// tarjetaAtomico es `.sytue-embed-fallback` dentro del marco del bloque: borde de 1 px,
// `radius-lg`, el fondo hundido y, centrados con `gap: 4px`, el título en seminegrita, la
// referencia del informe en monoespaciada y la nota. El papel es blanco en los dos temas:
// los colores, los del claro.
func (f *Folio) tarjetaAtomico(a *rico.Atomico, pos fyne.Position, w, h, z float32) []fyne.CanvasObject {
	v := tema.Claro
	marco := canvas.NewRectangle(tema.Color("surface-sunken", v))
	marco.StrokeColor, marco.StrokeWidth = tema.Color("border", v), max(z, 1)
	marco.CornerRadius = tema.RadioLG * z
	marco.Move(pos)
	marco.Resize(fyne.NewSize(w, h))

	type linea struct {
		texto string
		e     tema.Escalon
		peso  ui.Peso
		alfa  float32
	}
	escala := func(e tema.Escalon) tema.Escalon { return tema.Escalon{Tam: e.Tam * z, Linea: e.Linea * z} }
	titulo, nota := a.Titulo, f.traducir("embed.editOnWeb")
	var lineas []linea
	if a.Clase == rico.ClaseEmbed {
		lineas = append(lineas, linea{f.traducir("embed.powerbi.title"), escala(tema.TextoSM), ui.SemiNegrita, 1})
		if titulo != "" {
			lineas = append(lineas, linea{titulo, escala(tema.TextoXS), ui.Mono, 0.85}) // `opacity: .85`
		}
	} else {
		if titulo == "" {
			titulo = f.traducir("drawing.title")
		}
		lineas = append(lineas, linea{titulo, escala(tema.TextoSM), ui.SemiNegrita, 1})
	}
	lineas = append(lineas, linea{nota, escala(tema.TextoXS), ui.Normal, 0.7}) // `opacity: .7`

	objs := make([]fyne.CanvasObject, 0, 1+len(lineas))
	objs = append(objs, marco)
	hueco := 4 * z
	alto := -hueco
	for _, l := range lineas {
		alto += l.e.Linea + hueco
	}
	tinta := tema.Color("text-muted", v)
	cabe := max(w-2*24*z, 1) // `padding: 24px`
	y := pos.Y + (h-alto)/2
	for _, l := range lineas {
		texto := l.texto
		if partes := ui.Repartir(texto, l.e, l.peso, cabe); len(partes) > 0 {
			texto = partes[0] // una línea: lo que no cabe se corta, la tarjeta no crece
		}
		col := tinta
		col.A = uint8(float32(col.A) * l.alfa)
		t := ui.Texto(texto, l.e, l.peso, col)
		ui.ColocarTexto(t, l.e, pos.X+(w-ui.Medir(texto, l.e, l.peso).Width)/2, y)
		objs = append(objs, t)
		y += l.e.Linea + hueco
	}
	return objs
}
