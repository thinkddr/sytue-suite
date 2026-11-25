// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/Skeleton.tsx

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
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

// BloqueFijo es un `<Skeleton width height>` con las dos medidas de verdad: mide `ancho`
// × `alto` pegado arriba a la izquierda de lo que le den, y NO se estira. Es lo que
// hace la web con un `width` fijo dentro de una celda más grande; Bloque, en cambio, se
// queda con lo que le dé su contenedor, y en la rejilla del mes de Calendar el nombre
// de cada día medía 120 de ancho en vez de los `2rem` de la web. Si no cabe, encoge
// hasta lo que haya (el `min-w-0`).
func (b *BloquesEsqueleto) BloqueFijo(ancho, alto float32) fyne.CanvasObject {
	return b.forma(formaBloque{ancho: ancho, alto: alto}, tema.RadioMD)
}

// Circulo es el `<Skeleton width height circle>`: un círculo de `lado` (`rounded-full`)
// pegado arriba a la izquierda, que tampoco se estira; estirado dejaría de ser un
// círculo, que es lo que les pasaba a las celdas del mes de Calendar (128 × 24 en vez
// del círculo de `1.5rem`).
func (b *BloquesEsqueleto) Circulo(lado float32) fyne.CanvasObject {
	return b.forma(formaBloque{ancho: lado, alto: lado}, lado/2)
}

// BloqueFraccion es el `<Skeleton width="NN%" height>`: ocupa esa parte del ancho que le
// den, pegado a la izquierda, y todo su alto, como `NuevoEsqueletoFraccion`, con el que
// se cambia sin mover un píxel. Las líneas de un párrafo de carga necesitan anchos
// distintos o parecen una tabla.
func (b *BloquesEsqueleto) BloqueFraccion(fraccion, alto float32) fyne.CanvasObject {
	return b.forma(formaBloque{fraccion: fraccion, alto: alto, estirar: true}, tema.RadioMD)
}

// forma pide un bloque y lo envuelve en el layout que le da su forma.
func (b *BloquesEsqueleto) forma(f formaBloque, radio float32) fyne.CanvasObject {
	if f.alto <= 0 {
		f.alto = AltoEsqueleto
	}
	r := b.Bloque(f.ancho, f.alto)
	r.CornerRadius = radio
	return container.New(f, r)
}

// formaBloque coloca un bloque a su medida dentro de lo que le den: a un ancho fijo, a
// una fracción del ancho o, con estirar, a todo el alto.
type formaBloque struct {
	ancho, alto, fraccion float32
	estirar               bool
}

var _ fyne.Layout = formaBloque{}

func (f formaBloque) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(f.ancho, f.alto)
}

func (f formaBloque) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	w, h := min(f.ancho, t.Width), min(f.alto, t.Height)
	if f.ancho <= 0 {
		w = t.Width
		if f.fraccion > 0 && f.fraccion < 1 {
			w = float32(int(t.Width*f.fraccion + 0.5))
		}
	}
	if f.estirar {
		h = t.Height
	}
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(fyne.NewSize(w, h))
	}
}

// Con pone lo que coloca los bloques y devuelve el grupo, para encadenarlo.
func (b *BloquesEsqueleto) Con(contenido fyne.CanvasObject) *BloquesEsqueleto {
	b.contenido = contenido
	return b
}

// AMano es el contenido de un grupo cuyos bloques coloca a mano el renderer de la
// pantalla, como coloca todo lo demás (la portada del Hub, la de CertiPDF): el
// contenido no tiene layout y el grupo se pone sobre el widget entero con Cubrir, así que
// las coordenadas de lo que lleva dentro son las del renderer. Sin `objs`, sus bloques.
func (b *BloquesEsqueleto) AMano(objs ...fyne.CanvasObject) *BloquesEsqueleto {
	if len(objs) == 0 {
		for _, q := range b.bloques {
			objs = append(objs, q)
		}
	}
	b.contenido = container.NewWithoutLayout(objs...)
	return b
}

// Cubrir pone el grupo en el origen del widget que lo contiene, que mide `tam`. Con un
// grupo nil no hace nada: es la carga que no está.
func (b *BloquesEsqueleto) Cubrir(tam fyne.Size) {
	if b == nil {
		return
	}
	b.Move(fyne.NewPos(0, 0))
	b.Resize(tam)
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

// AltoPara es el alto del contenido a ese ancho (lo que pregunta AltoA): envolver una
// rejilla o una pila en el grupo no cambia lo que mide.
func (b *BloquesEsqueleto) AltoPara(ancho float32) float32 {
	if b.contenido == nil {
		return 0
	}
	return AltoA(b.contenido, ancho)
}

// Contenido es lo que coloca los bloques, tal como se puso con Con.
func (b *BloquesEsqueleto) Contenido() fyne.CanvasObject { return b.contenido }

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

func (r *bloquesEsqueletoRenderer) Objects() []fyne.CanvasObject {
	r.b.pintada()
	return r.objetos
}
func (r *bloquesEsqueletoRenderer) Destroy() { r.b.Parar() }

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
