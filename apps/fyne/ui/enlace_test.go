// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestElEnlaceMideSuDianaYSeSubrayaAlPasar: la medida es el rótulo con la sangría
// —los 26 px de diana del Hub—, el subrayado solo aparece con el puntero encima, y el
// rol anunciado es el que toca: `link` para un `<a>`, `button` para el de Contactos.
func TestElEnlaceMideSuDianaYSeSubrayaAlPasar(t *testing.T) {
	e := NuevoEnlace("Ver el calendario", tema.TextoSM, SemiNegrita, "primary-text", nil)
	e.Sangria = 1 * 0.25 * tema.Rem
	m := MedidaEnlace(e)
	if e.MinSize() != m {
		t.Errorf("MinSize = %v y MedidaEnlace = %v: tienen que ser la misma cuenta", e.MinSize(), m)
	}
	if quiero := tema.TextoSM.Linea + 2*4; m.Height != quiero {
		t.Errorf("alto %v, se esperaba %v (la caja de `text-sm` con `py-1`)", m.Height, quiero)
	}
	if w := Medir("Ver el calendario", tema.TextoSM, SemiNegrita).Width; m.Width != w {
		t.Errorf("ancho %v, se esperaba el del rótulo: %v", m.Width, w)
	}

	e.Resize(m)
	r := test.WidgetRenderer(e)
	texto := func() *canvas.Text {
		for _, o := range r.Objects() {
			if c, ok := o.(*canvas.Text); ok {
				return c
			}
		}
		t.Fatal("el enlace no pinta ningún texto")
		return nil
	}
	if texto().TextStyle.Underline {
		t.Error("en reposo el enlace va sin subrayar: el subrayado es del `hover`")
	}
	e.MouseIn(nil)
	if !texto().TextStyle.Underline {
		t.Error("con el puntero encima el enlace tiene que subrayarse (`hover:underline`)")
	}
	e.MouseOut()
	if texto().TextStyle.Underline {
		t.Error("al salir el puntero el subrayado tiene que irse")
	}

	// `Subrayado` es el `underline` de siempre (el «Reintentar» de un aviso de error):
	// subrayado también en reposo.
	e.Subrayado = true
	e.Refresh()
	if !texto().TextStyle.Underline {
		t.Error("con Subrayado el enlace va subrayado también en reposo")
	}
	e.Subrayado = false
	e.Refresh()

	// `Liso` es un `<a>` sin clases: tampoco se subraya al pasar.
	e.Liso = true
	e.MouseIn(nil)
	if texto().TextStyle.Underline {
		t.Error("un enlace Liso no se subraya ni con el puntero encima")
	}
	e.MouseOut()
	e.Liso = false

	if e.AccessibilityRole() != fyne.AccessibleRoleLink {
		t.Errorf("rol %q: un `<a>` se anuncia como enlace", e.AccessibilityRole())
	}
	e.Boton = true
	if e.AccessibilityRole() != fyne.AccessibleRoleButton {
		t.Errorf("rol %q: el `<button>` con pinta de enlace se anuncia como botón", e.AccessibilityRole())
	}
}

// TestElAnilloDelEnlaceAbrazaElRotulo: un `<a>` en línea mide su rótulo aunque la
// columna que lo contiene se estire —el «Reintentar» de Contactos va en una
// `ui.Columna`, que estira—, así que el anillo de foco acaba donde acaba el texto y no
// en el borde de la columna.
func TestElAnilloDelEnlaceAbrazaElRotulo(t *testing.T) {
	e := NuevoEnlace("Reintentar", tema.TextoSM, SemiNegrita, "sidebar-fg", nil)
	w := MedidaEnlace(e).Width
	e.Move(fyne.NewPos(4, 4))
	e.Resize(fyne.NewSize(w+60, MedidaEnlace(e).Height))
	e.FocusGained()
	img := pinta(t, w+70, MedidaEnlace(e).Height+8, e)
	fuera := px(img, int((4+w+30)*escala), int((4+MedidaEnlace(e).Height/2)*escala))
	if blanco := (color.NRGBA{R: 255, G: 255, B: 255, A: 255}); fuera != blanco {
		t.Errorf("a 30 px del final del rótulo hay %v y tendría que verse el fondo: el anillo "+
			"se estira con la columna en vez de abrazar el rótulo", fuera)
	}
}
