// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// FilaPulsable es el `<button type="button" class="flex min-h-11 w-full text-left
// rounded-md px-2 py-1 hover:bg-bg-subtle sytue-focus-ring">` que lleva DENTRO lo que se
// le dé: una fila de lista que se pulsa entera y cuyo contenido no es un rótulo (la hora
// y el título de un capítulo de Cast, por ejemplo).
//
// Existe porque un `*fyne.Container` no recibe el foco ni se anuncia por su nombre (ver
// `pulsable`), y FilaLista tiene su propia tipografía de lista con título y subtítulo.
// El contenido se coloca con el relleno y centrado en vertical; no recibe eventos, así
// que lo que haya dentro es decorado.
type FilaPulsable struct {
	pulsable

	// Contenido es lo que se pinta dentro.
	Contenido fyne.CanvasObject
	// AltoMinimo es el `min-h-*`; 0 vale `min-h-11` (2,75 rem). Con el dedo nunca baja
	// de 44 px.
	AltoMinimo float32
	// SinRelleno quita el `px-2 py-1` y alinea el contenido arriba: el contenido llega
	// al borde, como la tarjeta entera que es un botón (la de una galería, con su
	// portada a sangre).
	SinRelleno bool
	// Radio es el `rounded-*` del fondo y del aro; 0 vale `rounded-md`.
	Radio float32
}

var _ fyne.Widget = (*FilaPulsable)(nil)

// Relleno de la fila: `px-2 py-1`.
var (
	rellenoFilaPulsableX = tema.Espacio(2)
	rellenoFilaPulsableY = tema.Espacio(1)
)

// NuevaFilaPulsable crea la fila. `nombre` es su nombre accesible, ya traducido: lo
// que leería el lector de pantalla del texto del botón.
func NuevaFilaPulsable(nombre string, contenido fyne.CanvasObject, alPulsar func()) *FilaPulsable {
	f := &FilaPulsable{Contenido: contenido}
	f.Nombre, f.AlPulsar = nombre, alPulsar
	f.ExtendBaseWidget(f)
	return f
}

// AccessibilityLabel es el nombre que se le dio.
func (f *FilaPulsable) AccessibilityLabel() string { return f.Nombre }

// AccessibilityRole: es un botón.
func (f *FilaPulsable) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// relleno es el `px-2 py-1`, o nada con SinRelleno.
func (f *FilaPulsable) relleno() (x, y float32) {
	if f.SinRelleno {
		return 0, 0
	}
	return rellenoFilaPulsableX, rellenoFilaPulsableY
}

func (f *FilaPulsable) radio() float32 {
	if f.Radio > 0 {
		return f.Radio
	}
	return tema.RadioMD
}

func (f *FilaPulsable) altoMinimo() float32 {
	alto := f.AltoMinimo
	if alto <= 0 {
		alto = 2.75 * tema.Rem
	}
	return SueloTactil(alto)
}

// MedidaFilaPulsable es el contenido con su relleno, y como poco el alto mínimo.
func MedidaFilaPulsable(f *FilaPulsable) fyne.Size {
	var m fyne.Size
	if f.Contenido != nil {
		m = f.Contenido.MinSize()
	}
	x, y := f.relleno()
	return fyne.NewSize(m.Width+2*x, max(m.Height+2*y, f.altoMinimo()))
}

// CreateRenderer compone el aro, el fondo del `:hover` y el contenido.
func (f *FilaPulsable) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaPulsableRenderer{
		f:    f,
		aro:  NuevoAro(f.radio(), SinHueco, v).Abierto(),
		caja: NuevaCaja(color.NRGBA{}, color.NRGBA{}, f.radio()),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	if f.Contenido != nil {
		r.objetos = append(r.objetos, f.Contenido)
	}
	r.Refresh()
	return r
}

type filaPulsableRenderer struct {
	f       *FilaPulsable
	aro     *Aro
	caja    *Caja
	objetos []fyne.CanvasObject
}

func (r *filaPulsableRenderer) MinSize() fyne.Size           { return MedidaFilaPulsable(r.f) }
func (r *filaPulsableRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaPulsableRenderer) Destroy()                     {}

// Layout pone el contenido dentro del relleno, a todo el ancho y centrado en vertical
// (`items-baseline` de una sola línea).
func (r *filaPulsableRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	c := r.f.Contenido
	if c == nil {
		return
	}
	x, y := r.f.relleno()
	ancho := max(t.Width-2*x, 0)
	alto := min(AltoA(c, ancho), max(t.Height-2*y, 0))
	arriba := (t.Height - alto) / 2
	if r.f.SinRelleno {
		// Una tarjeta entera se alinea arriba: en una fila de la rejilla todas miden
		// lo que la más alta, y la portada no debe bajar en las demás.
		arriba = 0
	}
	c.Move(fyne.NewPos(x, arriba))
	c.Resize(fyne.NewSize(ancho, alto))
}

// Refresh pinta `hover:bg-bg-subtle` (también al pulsar) y el aro con el foco.
func (r *filaPulsableRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := color.NRGBA{}
	if r.f.encima || r.f.pulsado {
		fondo = tema.Color("bg-subtle", v)
	}
	r.aro.Ver(r.f.enfocado)
	r.aro.Pintar(v)
	r.caja.Pintar(fondo, color.NRGBA{})
	r.Layout(r.f.Size())
	if r.f.Contenido != nil {
		r.f.Contenido.Refresh()
	}
}
