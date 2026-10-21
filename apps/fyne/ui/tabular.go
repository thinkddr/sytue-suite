// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"bytes"
	"image/color"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// # `tabular-nums`
//
// El conformador de Fyne no activa rasgos OpenType, así que `tabular-nums` no existe en un
// `canvas.Text`. Aquí se pide el `tnum` al MISMO conformador (harfbuzz de go-text) solo para
// saber cuánto avanza cada cifra, y se pinta cada carácter en su casilla.
//
// Medido en producción (2026-09-25): en Figtree SemiBold a 10,5 px la cifra tabular avanza
// 6,609 —el contador «12» mide 20,219 de ancho y «99+» 26,828—. Calendar se escribía la suya
// con el ancho del dígito MÁS ANCHO (el 0, 6,766), que no es el `tnum` de la fuente: cada
// cifra salía 0,16 px larga.

var carasTnum sync.Map // fyne.Resource → *font.Face

func caraTnum(p Peso) *font.Face {
	r := p.cara()
	if f, ok := carasTnum.Load(r); ok {
		c, _ := f.(*font.Face)
		return c
	}
	f, err := font.ParseTTF(bytes.NewReader(r.Content()))
	if err != nil {
		return nil
	}
	carasTnum.Store(r, f)
	return f
}

// AvancesTabulares es lo que avanza cada carácter de `s` con `tabular-nums`, en px de CSS.
// Sin cara (no debería pasar: son las cinco de la suite) cae a los avances proporcionales.
func AvancesTabulares(s string, e tema.Escalon, p Peso) []float32 {
	letras := []rune(s)
	av := make([]float32, len(letras))
	cara := caraTnum(p)
	if cara == nil || len(letras) == 0 {
		for i, l := range letras {
			av[i] = Medir(string(l), e, p).Width
		}
		return av
	}
	out := (&shaping.HarfbuzzShaper{}).Shape(shaping.Input{
		Text: letras, RunStart: 0, RunEnd: len(letras),
		Direction: di.DirectionLTR, Face: cara, Size: fixed.Int26_6(e.Tam * 64),
		Script: language.Latin, Language: language.NewLanguage("es"),
		FontFeatures: []shaping.FontFeature{{Tag: ot.MustNewTag("tnum"), Value: 1}},
	})
	for _, g := range out.Glyphs {
		if i := g.TextIndex(); i < len(av) {
			av[i] += float32(g.Advance) / 64
		}
	}
	return av
}

// MedirTabular es el ancho de `s` con `tabular-nums`.
func MedirTabular(s string, e tema.Escalon, p Peso) float32 {
	var w float32
	for _, a := range AvancesTabulares(s, e, p) {
		w += a
	}
	return w
}

// TextoTabular es un texto con `tabular-nums`: un `canvas.Text` por carácter, cada uno
// centrado en la casilla que le da el `tnum` de la fuente.
//
// ponytail: se pinta el glifo PROPORCIONAL centrado en la casilla tabular, no el glifo `tnum`
// (Fyne no lo sabe pedir), y se pierde el kerning entre caracteres; en Figtree las dos cifras
// son el mismo dibujo, así que la diferencia es el reparto de los márgenes, por debajo de
// medio píxel. Si Fyne llega a pasar rasgos al conformador, esto es un `canvas.Text` y sobra.
type TextoTabular struct {
	e       tema.Escalon
	peso    Peso
	letras  []*canvas.Text
	casas   []float32
	espacio float32
}

// NuevoTextoTabular crea el texto.
func NuevoTextoTabular(s string, e tema.Escalon, p Peso, tinta color.Color) *TextoTabular {
	t := &TextoTabular{e: e, peso: p, casas: AvancesTabulares(s, e, p)}
	for _, r := range s {
		t.letras = append(t.letras, Texto(string(r), e, p, tinta))
	}
	return t
}

// ConEspaciado añade `letter-spacing` en em a cada avance, incluido el último,
// como CSS. Sin llamarlo conserva los avances tabulares de la fuente.
func (t *TextoTabular) ConEspaciado(em float32) *TextoTabular {
	t.espacio = em * t.e.Tam
	return t
}

// Objetos son los `canvas.Text`, uno por carácter.
func (t *TextoTabular) Objetos() []fyne.CanvasObject {
	out := make([]fyne.CanvasObject, len(t.letras))
	for i, l := range t.letras {
		out[i] = l
	}
	return out
}

// Ancho es MedirTabular del texto.
func (t *TextoTabular) Ancho() float32 {
	var w float32
	for _, c := range t.casas {
		w += c
	}
	return w + float32(len(t.casas))*t.espacio
}

// Colocar pone la caja de línea del texto con su esquina en (x, y), con el escalón `linea`
// (el del texto, o uno con otra caja de línea, como el `leading-none` del contador).
func (t *TextoTabular) Colocar(x, y float32, linea tema.Escalon) {
	for i, l := range t.letras {
		w := Medir(l.Text, t.e, t.peso).Width
		ColocarTexto(l, linea, x+(t.casas[i]-w)/2, y)
		x += t.casas[i] + t.espacio
	}
}

// Tenir cambia la tinta de todos los caracteres.
func (t *TextoTabular) Tenir(c color.Color) {
	for _, l := range t.letras {
		l.Color = c
		l.Refresh()
	}
}

// Objeto agrupa los glifos con el ancho conformado y su caja de línea. Permite
// colocar el texto tabular como una pieza sin duplicar su conformador o layout.
func (t *TextoTabular) Objeto() *fyne.Container {
	return container.New(t, t.Objetos()...)
}

// MinSize es el contrato de fyne.Layout del grupo tabular.
func (t *TextoTabular) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(t.Ancho(), t.e.Linea)
}

// Layout coloca los mismos glifos de Colocar dentro de la caja de línea del grupo.
func (t *TextoTabular) Layout([]fyne.CanvasObject, fyne.Size) {
	t.Colocar(0, 0, t.e)
}
