// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
// paridad-web: apps/frontend/packages/ui/src/components/Tag.tsx

package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Etiqueta es `Tag.tsx`: el chip de un recurso —el grupo de un contacto, la etiqueta de
// un hilo, un invitado— con punto de color y aspa opcionales.
//
//	chip   `rounded-pill border bg-bg-subtle py-0.5 text-sm`, `px-2.5` / `ps-2.5 pe-1`
//	punto  `h-2 w-2 rounded-full`
//	texto  `min-w-0 truncate max-w-48`, `text-text` o `text-text-muted`
//	aspa   `IconButton sm tone=subtle -my-1` con `<Icon close size={12}>`
//
// Hasta ahora Calendario, Contactos y Mail se dibujaban la suya, y la de Contactos con un
// aspa de 20 px: por debajo de los 24 de WCAG 2.5.8 y sin suelo táctil. Aquí el aspa es un
// BotonIcono de talla sm —24 px de diana, 44 con el dedo—, y el `-my-1` deja que la diana
// sobresalga del chip en vez de engordarlo: el alto del chip sigue siendo el de su texto.
type Etiqueta struct {
	widget.BaseWidget

	// Texto es lo que dice la etiqueta. Se trunca con «…» en Tope.
	Texto string
	// Punto es el color del punto de la izquierda. Nil: sin punto. Es decoración: el
	// significado lo lleva el texto.
	Punto color.Color
	// Apagada es `tone="muted"`: la tinta del texto en `text-muted`.
	Apagada bool
	// Densa es `size="xs"`: `text-xs` y `gap-1.5`, el de las etiquetas de un hilo.
	Densa bool
	// Tope es el `max-w-*` del texto. Cero es el `max-w-48` del componente.
	Tope float32

	aspa  *BotonIcono
	anexo fyne.CanvasObject
}

// Las medidas de Tag.tsx con el rem de 16.
const (
	// TopeEtiqueta es el `max-w-48` del texto.
	TopeEtiqueta = 48 * 0.25 * tema.Rem
	// LadoPuntoEtiqueta es el `h-2 w-2` del punto.
	LadoPuntoEtiqueta = 2 * 0.25 * tema.Rem
	// GlifoAspaEtiqueta es el `<Icon size={12}>` del aspa.
	GlifoAspaEtiqueta float32 = 12

	izqEtiqueta    = 2.5 * 0.25 * tema.Rem // `ps-2.5`
	derAspa        = 0.25 * tema.Rem       // `pe-1` cuando hay aspa
	vertEtiqueta   = 0.5 * 0.25 * tema.Rem // `py-0.5`
	huecoEtiqueta  = 0.25 * tema.Rem       // `gap-1`
	huecoDensa     = 1.5 * 0.25 * tema.Rem // `gap-1.5`
	margenAspaVert = 0.25 * tema.Rem       // el `-my-1` del aspa
)

// NuevaEtiqueta crea el chip sin aspa: el que se pinta cuando no se puede escribir.
func NuevaEtiqueta(texto string) *Etiqueta {
	e := &Etiqueta{Texto: texto}
	e.ExtendBaseWidget(e)
	return e
}

// ConAspa le pone el aspa que la quita. `rotulo` es el nombre accesible, YA traducido, y
// debe nombrar la etiqueta («Quitar la etiqueta Trabajo»): con cinco aspas en fila, cinco
// «Quitar» iguales no dicen cuál es cuál.
func (e *Etiqueta) ConAspa(rotulo string, alQuitar func()) *Etiqueta {
	e.aspa = &BotonIcono{
		Icono: iconos.Cerrar, Lado: LadoIconoSM, TamIcono: GlifoAspaEtiqueta,
		Radio: tema.Forma("radius-control"),
		// El tono `subtle` de IconButton.tsx: apagada en reposo, y el velo `border`
		// porque el chip ya está en `bg-subtle`.
		Trazo: "text-muted", TrazoEncima: "text", FondoEncima: "border",
		Rotulo: rotulo, AlPulsar: alQuitar,
	}
	e.aspa.ExtendBaseWidget(e.aspa)
	e.Refresh()
	return e
}

// ConAnexo es el `trailing` de Tag.tsx: lo que acompaña al texto sin ser parte de él (la
// Insignia con la respuesta de un invitado), entre el texto y el aspa. No se trunca: si
// falta sitio, el que se recorta es el texto.
func (e *Etiqueta) ConAnexo(o fyne.CanvasObject) *Etiqueta {
	e.anexo = o
	e.Refresh()
	return e
}

// Anexo devuelve lo que se puso con ConAnexo, o nil.
func (e *Etiqueta) Anexo() fyne.CanvasObject { return e.anexo }

// Aspa devuelve el botón que la quita, o nil si no lo tiene: para apagarlo mientras se
// guarda el cambio anterior (`removeDisabled`) y para los guardianes.
func (e *Etiqueta) Aspa() *BotonIcono { return e.aspa }

// AccessibilityLabel es el texto entero, sin truncar: el `title` de la web.
func (e *Etiqueta) AccessibilityLabel() string { return e.Texto }

// LadoAspaEtiqueta es la diana del aspa: los 24 de `min-h-[24px]` y, con el dedo, los 44
// de `pointer-coarse:min-h-[max(44px,2.75rem)]`.
func LadoAspaEtiqueta() float32 { return SueloTactil(LadoIconoSM) }

// MedidaEtiqueta es lo que ocupa un chip. El alto es el de la línea con su `py-0.5` y su
// borde; el aspa solo cuenta con lo que le queda tras el `-my-1`, que con ratón (24 − 8)
// es menos que la línea de `text-sm` (18): el chip mide lo mismo con aspa que sin ella.
func MedidaEtiqueta(texto string, conPunto, conAspa, densa bool, tope float32) fyne.Size {
	escalon, hueco := tema.TextoSM, float32(huecoEtiqueta)
	if densa {
		escalon, hueco = tema.TextoXS, huecoDensa
	}
	if tope <= 0 {
		tope = TopeEtiqueta
	}
	ancho := 2*tema.Borde + izqEtiqueta + min(Medir(texto, escalon, Normal).Width, tope)
	alto := escalon.Linea
	if conPunto {
		ancho += LadoPuntoEtiqueta + hueco
	}
	if conAspa {
		lado := LadoAspaEtiqueta()
		ancho += hueco + lado + derAspa
		alto = max(alto, lado-2*margenAspaVert)
	} else {
		ancho += izqEtiqueta
	}
	return fyne.NewSize(ancho, alto+2*vertEtiqueta+2*tema.Borde)
}

func (e *Etiqueta) escalon() (tema.Escalon, float32) {
	if e.Densa {
		return tema.TextoXS, huecoDensa
	}
	return tema.TextoSM, huecoEtiqueta
}

func (e *Etiqueta) tope() float32 {
	if e.Tope > 0 {
		return e.Tope
	}
	return TopeEtiqueta
}

// CreateRenderer arma la píldora, el punto, el texto y el aspa.
func (e *Etiqueta) CreateRenderer() fyne.WidgetRenderer {
	e.ExtendBaseWidget(e)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	escalon, _ := e.escalon()
	r := &etiquetaRenderer{
		e:     e,
		caja:  NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), 999),
		punto: canvas.NewCircle(color.NRGBA{}),
		texto: Texto(e.Texto, escalon, Normal, tema.Color("text", v)),
	}
	r.Refresh()
	return r
}

type etiquetaRenderer struct {
	e     *Etiqueta
	caja  *Caja
	punto *canvas.Circle
	texto *canvas.Text
}

func (r *etiquetaRenderer) MinSize() fyne.Size {
	m := MedidaEtiqueta(r.e.Texto, r.e.Punto != nil, r.e.aspa != nil, r.e.Densa, r.e.Tope)
	if a := r.e.anexo; a != nil {
		_, hueco := r.e.escalon()
		am := a.MinSize()
		m.Width += am.Width + hueco
		m.Height = max(m.Height, am.Height+2*vertEtiqueta+2*tema.Borde)
	}
	return m
}

func (r *etiquetaRenderer) Objects() []fyne.CanvasObject {
	objs := append(r.caja.Objetos(), r.punto, r.texto)
	if r.e.anexo != nil {
		objs = append(objs, r.e.anexo)
	}
	if r.e.aspa != nil {
		objs = append(objs, r.e.aspa)
	}
	return objs
}

func (r *etiquetaRenderer) Destroy() {}

func (r *etiquetaRenderer) Layout(t fyne.Size) {
	escalon, hueco := r.e.escalon()
	r.caja.Redimensionar(t)
	x := tema.Borde + izqEtiqueta
	r.punto.Hidden = r.e.Punto == nil
	if r.e.Punto != nil {
		r.punto.Move(fyne.NewPos(x, (t.Height-LadoPuntoEtiqueta)/2))
		r.punto.Resize(fyne.NewSize(LadoPuntoEtiqueta, LadoPuntoEtiqueta))
		x += LadoPuntoEtiqueta + hueco
	}
	// Lo que cabe es lo que deja la caja asignada, no solo el tope: en una fila estrecha
	// el chip encoge (`min-w-0`) y el texto se trunca antes.
	der := tema.Borde + izqEtiqueta
	if a := r.e.aspa; a != nil {
		lado := LadoAspaEtiqueta()
		der = tema.Borde + derAspa + lado + hueco
		a.Lado = lado
		a.Move(fyne.NewPos(t.Width-tema.Borde-derAspa-lado, (t.Height-lado)/2))
		a.Resize(fyne.NewSize(lado, lado))
	}
	anchoAnexo := float32(0)
	if a := r.e.anexo; a != nil {
		anchoAnexo = a.MinSize().Width + hueco
	}
	w := Truncar(r.texto, r.e.Texto, escalon, Normal, max(min(r.e.tope(), t.Width-x-der-anchoAnexo), 0))
	ColocarTexto(r.texto, escalon, x, (t.Height-escalon.Linea)/2)
	if a := r.e.anexo; a != nil {
		am := a.MinSize()
		a.Move(fyne.NewPos(x+w+hueco, (t.Height-am.Height)/2))
		a.Resize(am)
	}
}

func (r *etiquetaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	escalon, _ := r.e.escalon()
	r.caja.Pintar(tema.Color("bg-subtle", v), tema.Color("border", v))
	tinta := "text"
	if r.e.Apagada {
		tinta = "text-muted"
	}
	r.texto.Color = tema.Color(tinta, v)
	r.texto.TextSize = escalon.Tam
	if r.e.Punto != nil {
		r.punto.FillColor = r.e.Punto
	}
	r.Layout(r.e.Size())
	r.punto.Refresh()
	r.texto.Refresh()
	if r.e.anexo != nil {
		r.e.anexo.Refresh()
	}
	if r.e.aspa != nil {
		r.e.aspa.Refresh()
	}
}
