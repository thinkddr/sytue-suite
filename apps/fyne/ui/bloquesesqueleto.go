// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/Skeleton.tsx

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// BloquesEsqueleto son los bloques de carga de algo que NO es una lista de filas —la
// rejilla del mes de Calendar, el hueco de las salas de un evento—, colocados por el
// layout que ya tiene la pantalla y latiendo A UNA, con un único «Cargando».
//
// Es la pareja de FilasEsqueleto para lo que no son filas. Con un `Esqueleto` por bloque
// la rejilla del mes eran 49 animaciones que pulsaban cada una a su aire y ninguna
// anunciaba la carga; en la web todos los `.sytue-skeleton` arrancan juntos y el
// contenedor es un solo `role="status"`.
//
// Cada Bloque es un rectángulo que pide lo mismo que pediría `NuevoEsqueleto(ancho,
// alto)` y, como él, se queda con lo que le dé su contenedor: cambiar uno por otro no
// mueve ni un píxel.
//
//	b := ui.NuevosBloquesEsqueleto()
//	b.Con(container.New(ui.Margen{…}, b.Bloque(2*tema.Rem, 0.75*tema.Rem)))
type BloquesEsqueleto struct {
	widget.BaseWidget

	// Rotulo es el `aria-label` del contenedor; vacío es «Cargando» (`ui.loading`).
	Rotulo string
	// Tinte es el token de los bloques; vacío es `bg-subtle` (SobreCromo).
	Tinte string

	contenido fyne.CanvasObject
	bloques   []*canvas.Rectangle

	latidoEsqueleto
}

var (
	_ fyne.Widget     = (*BloquesEsqueleto)(nil)
	_ fyne.Accessible = (*BloquesEsqueleto)(nil)
)

// NuevosBloquesEsqueleto crea el grupo, vacío: los bloques se piden con Bloque y se
// colocan con Con.
func NuevosBloquesEsqueleto() *BloquesEsqueleto {
	b := &BloquesEsqueleto{}
	b.ExtendBaseWidget(b)
	return b
}

// Bloque es un `<Skeleton width height>` del grupo: pide `ancho` × `alto` (cero de ancho
// es el `100%`, cero de alto el `1rem`) con `rounded-md`, y late con los demás.
func (b *BloquesEsqueleto) Bloque(ancho, alto float32) *canvas.Rectangle {
	if alto <= 0 {
		alto = AltoEsqueleto
	}
	r := canvas.NewRectangle(color.NRGBA{})
	r.CornerRadius = tema.RadioMD
	r.SetMinSize(fyne.NewSize(ancho, alto))
	b.bloques = append(b.bloques, r)
	return r
}

// Con pone lo que coloca los bloques y devuelve el grupo, para encadenarlo.
func (b *BloquesEsqueleto) Con(contenido fyne.CanvasObject) *BloquesEsqueleto {
	b.contenido = contenido
	return b
}

// ConRotulo cambia el «Cargando» por el de la pantalla («Cargando el calendario»), ya
// traducido, y devuelve el grupo.
func (b *BloquesEsqueleto) ConRotulo(rotulo string) *BloquesEsqueleto {
	b.Rotulo = rotulo
	return b
}

// SobreCromo tiñe los bloques para el carril oscuro y devuelve el grupo.
func (b *BloquesEsqueleto) SobreCromo() *BloquesEsqueleto {
	b.Tinte = "sidebar-hover-bg"
	return b
}

// Bloques son los rectángulos del grupo, en el orden en que se pidieron.
func (b *BloquesEsqueleto) Bloques() []*canvas.Rectangle { return b.bloques }

// AccessibilityLabel es la región `role="status"`: se dice una vez para todo el grupo.
func (b *BloquesEsqueleto) AccessibilityLabel() string {
	if b.Rotulo != "" {
		return b.Rotulo
	}
	return textoComun("ui.loading")
}

// AccessibilityRole: como FilasEsqueleto, Fyne no tiene «status» y llega el texto.
func (b *BloquesEsqueleto) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleText
}

// CreateRenderer tiñe los bloques y arranca el latido.
func (b *BloquesEsqueleto) CreateRenderer() fyne.WidgetRenderer {
	r := &bloquesEsqueletoRenderer{b: b}
	if b.contenido != nil {
		r.objetos = []fyne.CanvasObject{b.contenido}
	}
	b.latir(r.tenir)
	r.tenir()
	return r
}

type bloquesEsqueletoRenderer struct {
	b       *BloquesEsqueleto
	objetos []fyne.CanvasObject
}

func (r *bloquesEsqueletoRenderer) MinSize() fyne.Size {
	if r.b.contenido == nil {
		return fyne.Size{}
	}
	return r.b.contenido.MinSize()
}

func (r *bloquesEsqueletoRenderer) Layout(t fyne.Size) {
	if c := r.b.contenido; c != nil {
		c.Move(fyne.NewPos(0, 0))
		c.Resize(t)
	}
}

func (r *bloquesEsqueletoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *bloquesEsqueletoRenderer) Destroy()                     { r.b.Parar() }

// tenir pinta todos los bloques con el instante del latido.
func (r *bloquesEsqueletoRenderer) tenir() {
	c := r.b.tinta(r.b.Tinte)
	for _, q := range r.b.bloques {
		q.FillColor = c
		q.Refresh()
	}
}

// Refresh vuelve a tenir (el tema puede haber cambiado) y recoloca.
func (r *bloquesEsqueletoRenderer) Refresh() {
	r.tenir()
	r.Layout(r.b.Size())
}
