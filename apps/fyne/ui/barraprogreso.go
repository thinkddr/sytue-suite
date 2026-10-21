// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// paridad-web: apps/frontend/packages/ui/src/components/ProgressBar.tsx

package ui

import (
	"image/color"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// BarraProgreso es ProgressBar.tsx: «cuánto falta de algo que está pasando» (una
// subida, una exportación). NO es el Medidor: el medidor cuenta lo gastado de un tope y
// avisa al acercarse; una subida al 95 % no es una alarma, es casi hecha.
//
// `bg-border h-1.5 rounded-full` y el relleno en `primary` (o `danger` si el proceso
// falló), también redondo.
//
// ponytail: sin `value` la web la anima (indeterminada); aquí la indeterminada pinta un
// tercio quieto, que es lo que la web hace con `prefers-reduced-motion`. Se anima
// cuando una pantalla nativa la necesite.
type BarraProgreso struct {
	widget.BaseWidget

	// Fraccion es lo hecho, de 0 a 1. Negativa: indeterminada.
	Fraccion float64
	// Rotulo es el nombre accesible («Subiendo entrevista.mp4»).
	Rotulo string
	// Peligro es `tone="danger"`.
	Peligro bool
}

var (
	_ fyne.Widget     = (*BarraProgreso)(nil)
	_ fyne.Accessible = (*BarraProgreso)(nil)
)

// AltoBarraProgreso es el `h-1.5`.
var AltoBarraProgreso = tema.Espacio(1.5)

// NuevaBarraProgreso crea la barra.
func NuevaBarraProgreso(rotulo string, fraccion float64) *BarraProgreso {
	b := &BarraProgreso{Rotulo: rotulo, Fraccion: fraccion}
	b.ExtendBaseWidget(b)
	return b
}

// MedidaBarraProgreso es lo que ocupa: todo el ancho y el `h-1.5`.
func MedidaBarraProgreso() fyne.Size { return fyne.NewSize(0, AltoBarraProgreso) }

// AccessibilityRole: Fyne no tiene `progressbar`; se anuncia como contenedor.
func (b *BarraProgreso) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleContainer
}

// AccessibilityLabel es el rótulo con el porcentaje, el `aria-valuetext` de la web.
func (b *BarraProgreso) AccessibilityLabel() string {
	if b.Fraccion < 0 {
		return b.Rotulo
	}
	return b.Rotulo + " " + strconv.Itoa(int(b.hecho()*100+0.5)) + " %"
}

func (b *BarraProgreso) hecho() float64 { return min(max(b.Fraccion, 0), 1) }

// CreateRenderer arma el carril y el relleno. Termina en Refresh (regla 7 de ui.go).
func (b *BarraProgreso) CreateRenderer() fyne.WidgetRenderer {
	r := &barraProgresoRenderer{b: b}
	r.carril = NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioPill)
	r.relleno = NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioPill)
	r.objetos = append(r.carril.Objetos(), r.relleno.Objetos()...)
	r.Refresh()
	return r
}

type barraProgresoRenderer struct {
	b               *BarraProgreso
	carril, relleno *Caja
	objetos         []fyne.CanvasObject
}

func (r *barraProgresoRenderer) MinSize() fyne.Size           { return MedidaBarraProgreso() }
func (r *barraProgresoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *barraProgresoRenderer) Destroy()                     {}

func (r *barraProgresoRenderer) Layout(t fyne.Size) {
	r.carril.Redimensionar(t)
	ancho := t.Width * float32(r.b.hecho())
	if r.b.Fraccion < 0 {
		ancho = t.Width / 3
	}
	MoverCaja(r.relleno, fyne.NewPos(0, 0), fyne.NewSize(ancho, t.Height))
}

func (r *barraProgresoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	tinta := "primary"
	if r.b.Peligro {
		tinta = "danger"
	}
	r.carril.Pintar(tema.Color("border", v), color.NRGBA{})
	r.relleno.Pintar(tema.Color(tinta, v), color.NRGBA{})
	r.Layout(r.b.Size())
}
