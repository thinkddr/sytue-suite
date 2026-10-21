// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/ia"
)

// palabraRica es una palabra con su cara y si lleva un espacio delante.
type palabraRica struct {
	texto   string
	cara    fyne.Resource
	espacio bool
}

// ParrafoRico es un `<p>` con `<strong>` y `<em>` dentro —lo que pinta la web con los
// bloques de `bloquesDe` de una respuesta de la IA—, repartido palabra a palabra al
// ancho como la línea de CSS. `base` es la cara del párrafo (Regular, o SemiNegra en un
// título); la negrita es la de `<strong>` (700). La tinta es `text`.
//
// Estaba en CertiPDF (`text-xs`); Drive pinta lo mismo en `text-sm`, y por eso el
// escalón es un parámetro y la pieza vive aquí.
//
// ponytail: una palabra pegada a otra de otro estilo («foo**bar**») se puede partir
// entre las dos; el navegador no lo haría.
func ParrafoRico(trozos []ia.Trozo, base fyne.Resource, esc tema.Escalon, ancho float32) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var ps []palabraRica
	espacio := false
	for _, t := range trozos {
		cara := base
		switch {
		case t.Cursiva:
			cara = tema.Cursiva
		case t.Negrita:
			cara = tema.Negra
		}
		for j, w := range strings.Split(t.Texto, " ") {
			if j > 0 {
				espacio = true
			}
			if w == "" {
				continue
			}
			ps = append(ps, palabraRica{texto: w, cara: cara, espacio: espacio})
			espacio = false
		}
	}
	medir := func(s string, cara fyne.Resource) float32 {
		m, _ := fyne.CurrentApp().Driver().RenderedTextSize(s, esc.Tam, fyne.TextStyle{}, cara)
		return m.Width
	}
	objs := make([]fyne.CanvasObject, 0, len(ps))
	var x, y float32
	for _, p := range ps {
		w := medir(p.texto, p.cara)
		sp := float32(0)
		if p.espacio {
			sp = medir("a a", p.cara) - medir("aa", p.cara)
		}
		if x > 0 && x+sp+w > ancho {
			x, y, sp = 0, y+esc.Linea, 0
		}
		t := canvas.NewText(p.texto, tema.Color("text", v))
		t.TextSize, t.FontSource = esc.Tam, p.cara
		ColocarTexto(t, esc, x+sp, y)
		objs = append(objs, t)
		x += sp + w
	}
	alto := esc.Linea
	if len(ps) > 0 {
		alto = y + esc.Linea
	}
	return container.New(tamFijo{fyne.NewSize(ancho, alto)}, objs...)
}

// tamFijo declara un tamaño y deja a los hijos donde se colocaron al construir.
type tamFijo struct{ tam fyne.Size }

func (f tamFijo) MinSize([]fyne.CanvasObject) fyne.Size { return f.tam }
func (tamFijo) Layout([]fyne.CanvasObject, fyne.Size)   {}
