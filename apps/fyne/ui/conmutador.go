// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// El carril y el pomo de `Switch` en Form.tsx, en píxeles con el rem de 14.
var (
	// AnchoCarril es el `w-9` del carril.
	AnchoCarril = tema.Espacio(9)
	// AltoCarril es su `h-5`.
	AltoCarril = tema.Espacio(5)
	// LadoPomo es el `h-4 w-4` del pomo.
	LadoPomo = tema.Espacio(4)
	// MargenPomo es el `top-0.5 left-0.5` que lo separa del carril.
	MargenPomo = tema.Espacio(0.5)
	// RecorridoPomo es el `translate-x-4` que hace al encender.
	RecorridoPomo = tema.Espacio(4)
	// HuecoConmutador es el `gap-2.5` hasta la etiqueta. Es medio paso MÁS que el de la
	// casilla: el carril es más ancho y con `gap-2` la etiqueta se le pegaba.
	HuecoConmutador = tema.Espacio(2.5)
)

// MedidaConmutador es lo que ocupa el conmutador con su etiqueta.
//
// El alto es el de la caja de línea de `text-base` y no el del carril: el `<label>` es
// un `inline-flex items-center text-base`, igual que en la casilla.
//
// El contorno de un píxel del carril apagado NO cuenta: es `ring-1`, o sea un
// `box-shadow`, y una sombra no ocupa sitio en el layout. Se pinta por fuera de estos
// 31,5 × 17,5, como el aro de foco.
func MedidaConmutador(etiqueta string) fyne.Size {
	return medidaConmutador(etiqueta, tema.TextoBase)
}

func medidaConmutador(etiqueta string, e tema.Escalon) fyne.Size {
	ancho := AnchoCarril
	if etiqueta != "" {
		ancho += HuecoConmutador + Medir(etiqueta, e, Normal).Width
	}
	return fyne.NewSize(ancho, max(AltoCarril, e.Linea))
}

// Conmutador es `Switch` de Form.tsx: el carril con el pomo y su etiqueta.
type Conmutador struct {
	widget.BaseWidget
	Etiqueta  string
	Encendido bool
	// Escalon es el tamaño de la etiqueta cuando quien lo usa se lo cambia, como el
	// `class="text-sm"` de la autorespuesta de Mail. Vacío: el `text-base` del label.
	Escalon tema.Escalon
	// AnchoEtiqueta limita el reparto de la etiqueta. Cero conserva su ancho natural.
	AnchoEtiqueta float32
	AlCambiar     func(bool)

	inerte, enfocado bool
}

var (
	_ fyne.Widget      = (*Conmutador)(nil)
	_ fyne.Tappable    = (*Conmutador)(nil)
	_ fyne.Focusable   = (*Conmutador)(nil)
	_ fyne.Accessible  = (*Conmutador)(nil)
	_ fyne.Disableable = (*Conmutador)(nil)
)

// NuevoConmutador crea un conmutador apagado.
func NuevoConmutador(etiqueta string, alCambiar func(bool)) *Conmutador {
	c := &Conmutador{Etiqueta: etiqueta, AlCambiar: alCambiar}
	c.ExtendBaseWidget(c)
	return c
}

// MinSize es la medida declarada.
func (c *Conmutador) MinSize() fyne.Size {
	c.ExtendBaseWidget(c)
	if c.AnchoEtiqueta > 0 && c.Etiqueta != "" {
		w := min(Medir(c.Etiqueta, c.escalon(), Normal).Width, c.AnchoEtiqueta)
		ancho := AnchoCarril + HuecoConmutador + w
		return fyne.NewSize(ancho, c.AltoPara(ancho))
	}
	m := medidaConmutador(c.Etiqueta, c.escalon())
	if c.Etiqueta != "" {
		m.Height = sueloTactil(m.Height) // con el dedo, 44, como en Form.tsx
	}
	return m
}

// AltoPara mide la etiqueta optativamente envuelta al ancho útil actual.
func (c *Conmutador) AltoPara(ancho float32) float32 {
	if c.AnchoEtiqueta <= 0 || c.Etiqueta == "" {
		return medidaConmutador(c.Etiqueta, c.escalon()).Height
	}
	w := max(min(c.AnchoEtiqueta, ancho-AnchoCarril-HuecoConmutador), 1)
	return sueloTactil(max(AltoCarril, float32(len(Repartir(c.Etiqueta, c.escalon(), Normal, w)))*c.escalon().Linea))
}

func (c *Conmutador) escalon() tema.Escalon {
	if c.Escalon.Tam == 0 {
		return tema.TextoBase
	}
	return c.Escalon
}

// Tapped enciende y apaga.
func (c *Conmutador) Tapped(*fyne.PointEvent) {
	if c.inerte {
		return
	}
	if lienzo := fyne.CurrentApp().Driver().CanvasForObject(c); lienzo != nil {
		enfocar(lienzo, c)
	}
	c.Encendido = !c.Encendido
	c.Refresh()
	if c.AlCambiar != nil {
		c.AlCambiar(c.Encendido)
	}
}

// Disable es `disabled`.
func (c *Conmutador) Disable() { c.inerte = true; c.Refresh() }

// Enable lo devuelve a la vida.
func (c *Conmutador) Enable() { c.inerte = false; c.Refresh() }

// Disabled dice si está inerte.
func (c *Conmutador) Disabled() bool { return c.inerte }

// FocusGained enciende el aro.
func (c *Conmutador) FocusGained() { c.enfocado = true; c.Refresh() }

// FocusLost lo apaga.
func (c *Conmutador) FocusLost() { c.enfocado = false; c.Refresh() }

// TypedRune conmuta con la barra espaciadora.
func (c *Conmutador) TypedRune(r rune) {
	if r == ' ' {
		c.Tapped(nil)
	}
}

// TypedKey no atiende ninguna otra tecla.
func (c *Conmutador) TypedKey(k *fyne.KeyEvent) { SubirEscape(c, k) }

// AccessibilityLabel es el rótulo que se anuncia.
func (c *Conmutador) AccessibilityLabel() string { return c.Etiqueta }

// AccessibilityRole es «button», que es lo más cerca que se puede estar.
//
// Fyne 2.8 solo conoce cuatro roles —button, container, link y text—, así que el
// `role="switch"` que el componente de la web pone a mano no tiene equivalente: un
// lector de pantalla anunciará «botón» y no «conmutador, activado». Es un techo del
// toolkit, no de este widget.
func (c *Conmutador) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// CreateRenderer compone el contorno, el carril, el pomo y la etiqueta.
func (c *Conmutador) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &conmutadorRenderer{
		c:        c,
		aro:      NuevoAro(tema.RadioPill, SinHueco, v),
		contorno: canvas.NewRectangle(tema.Color("neutral-500", v)),
		carril:   canvas.NewRectangle(tema.Color("bg-subtle", v)),
		pomo:     canvas.NewCircle(tema.Color("surface", v)),
		// `shadow-[var(--shadow-xs)]` del pomo: medida en producción a ×2, la sombra deja
		// sobre el carril dos o tres filas más oscuras bajo el pomo (#e7e4e1 sobre
		// #f1ede9 apagado), y sin ella el pomo apagado se funde con el carril.
		sombra: NuevaSombra(tema.SombraXS, LadoPomo/2),
	}
	r.contorno.CornerRadius = tema.RadioPill
	r.carril.CornerRadius = tema.RadioPill
	r.objetos = append(r.aro.Objetos(), r.contorno, r.carril, r.sombra.Objeto(), r.pomo)
	if c.Etiqueta != "" {
		r.texto = Texto(c.Etiqueta, c.escalon(), Normal, tema.Color("text", v))
		r.parrafo = NuevoParrafo(c.Etiqueta, c.escalon(), Normal, "text", v)
		r.objetos = append(r.objetos, r.texto, r.parrafo)
	}
	r.Refresh()
	return r
}

type conmutadorRenderer struct {
	c        *Conmutador
	aro      *Aro
	contorno *canvas.Rectangle
	carril   *canvas.Rectangle
	pomo     *canvas.Circle
	sombra   *Sombra
	texto    *canvas.Text
	parrafo  *Parrafo
	objetos  []fyne.CanvasObject
}

func (r *conmutadorRenderer) MinSize() fyne.Size           { return r.c.MinSize() }
func (r *conmutadorRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *conmutadorRenderer) Destroy()                     {}

func (r *conmutadorRenderer) Layout(t fyne.Size) {
	y := (t.Height - AltoCarril) / 2 // `items-center`
	// `ring-1` es una sombra de un píxel POR FUERA del carril, no un borde por dentro:
	// el carril sigue midiendo 31,5 × 17,5 y el contorno se pinta alrededor.
	r.contorno.Move(fyne.NewPos(-tema.Borde, y-tema.Borde))
	r.contorno.Resize(fyne.NewSize(AnchoCarril+2*tema.Borde, AltoCarril+2*tema.Borde))
	r.carril.Move(fyne.NewPos(0, y))
	r.carril.Resize(fyne.NewSize(AnchoCarril, AltoCarril))
	r.aro.ColocarEn(fyne.NewPos(0, y), fyne.NewSize(AnchoCarril, AltoCarril))

	x := MargenPomo
	if r.c.Encendido {
		x += RecorridoPomo
	}
	r.pomo.Move(fyne.NewPos(x, y+MargenPomo))
	r.pomo.Resize(fyne.NewSize(LadoPomo, LadoPomo))
	r.sombra.Ajustar(fyne.NewPos(x, y+MargenPomo), fyne.NewSize(LadoPomo, LadoPomo), tema.De(fyne.CurrentApp().Settings().ThemeVariant()))

	if r.parrafo != nil && r.c.AnchoEtiqueta > 0 {
		w := max(min(t.Width-AnchoCarril-HuecoConmutador, r.c.AnchoEtiqueta), 1)
		h := AltoParrafo(r.parrafo, w)
		r.parrafo.Move(fyne.NewPos(AnchoCarril+HuecoConmutador, (t.Height-h)/2))
		r.parrafo.Resize(fyne.NewSize(w, h))
	}
	if r.texto != nil {
		e := r.c.escalon()
		ColocarTexto(r.texto, e, AnchoCarril+HuecoConmutador, (t.Height-e.Linea)/2)
	}
}

func (r *conmutadorRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// El carril APAGADO lleva el contraste en el CONTORNO y no en el relleno, que es
	// como lo resuelven Apple y GitHub: un carril relleno de gris oscuro parecería
	// encendido y el estado se leería al revés. Encendido, el contorno desaparece.
	carril, contorno := tema.Color("bg-subtle", v), tema.Color("neutral-500", v)
	if r.c.Encendido {
		carril, contorno = tema.Color("primary", v), color.NRGBA{}
	}
	r.carril.FillColor = atenuar(carril, r.c.inerte)
	r.carril.Refresh()
	r.contorno.FillColor = atenuar(contorno, r.c.inerte)
	r.contorno.Refresh()
	// El pomo sale de `surface` y no de blanco: en tema oscuro el blanco fijo no
	// invertía y quedaba un círculo blanco sobre un carril oscuro.
	r.pomo.FillColor = atenuar(tema.Color("surface", v), r.c.inerte)
	r.pomo.Refresh()
	// El `opacity-50` del grupo también apaga la sombra.
	r.sombra.img.Translucency = 0
	if r.c.inerte {
		r.sombra.img.Translucency = 0.5
	}
	r.aro.Pintar(v)
	r.aro.Ver(r.c.enfocado)
	if r.texto != nil {
		r.texto.Text = r.c.Etiqueta
		r.texto.TextSize = r.c.escalon().Tam
		r.texto.Color = atenuar(tema.Color("text", v), r.c.inerte)
		r.texto.Refresh()
		ver(r.texto, r.c.AnchoEtiqueta <= 0)
	}
	if r.parrafo != nil {
		r.parrafo.Texto, r.parrafo.Escalon = r.c.Etiqueta, r.c.escalon()
		r.parrafo.Tinta = atenuar(tema.Color("text", v), r.c.inerte)
		r.parrafo.Ancho = r.c.AnchoEtiqueta
		r.parrafo.Refresh()
		ver(r.parrafo, r.c.AnchoEtiqueta > 0)
	}
	r.Layout(r.c.Size())
}
