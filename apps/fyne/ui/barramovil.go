// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// BarraMovil es la barra que AppShell.tsx pone bajo la topbar por debajo de 768: la
// ÚNICA vía de abrir el menú —y de volver del detalle a la lista— en un teléfono. Va en
// `Armazon.BarraMovil`, que ya la coloca y la esconde en ancho.
//
//	barra  `flex h-11 items-center gap-1.5 border-b bg-surface px-4`       hoja
//	botón  `ghost sm iconOnly` con el `menu` de 18, o el `chevronLeft` de  hoja
//	       18 si hay detalle abierto que cerrar
//	título `text-sm font-semibold text-text truncate`; con lista, SOLO en   hoja
//	       el detalle (con la lista a la vista, la lista trae el suyo y se leía
//	       dos veces). Sin lista, siempre.
//	       Sin menú y sin detalle que cerrar, no hay botón.
//
// Los `px-4` son el mismo canalón que la topbar, para que la hamburguesa caiga en la
// vertical de la marca de arriba.
type BarraMovil struct {
	widget.BaseWidget

	// Titulo se enseña siempre sin lista (`ConLista`) y, con lista, solo con EnDetalle.
	Titulo string
	// ConLista es que el armazón tiene panel de lista: lo pone `Armazon`.
	ConLista bool
	// EnDetalle cambia el menú por «volver a la lista».
	EnDetalle bool
	// CajonAbierto cambia el nombre accesible del botón de menú.
	CajonAbierto bool
	// Los tres nombres accesibles, YA traducidos: `ui.openNav`, `ui.closeNav` y
	// `ui.backToList`.
	RotuloAbrir, RotuloCerrar, RotuloVolver string
	AlMenu, AlVolver                        func()

	boton *BotonIcono // el de menú o volver: adonde vuelve el foco al cerrar el cajón
}

var _ fyne.Widget = (*BarraMovil)(nil)

// AltoBarraMovil es el `h-11`: 38,5 con el rem de 14.
var AltoBarraMovil = tema.Espacio(11)

var (
	canalonBarraMovil = tema.Espacio(4)   // px-4
	huecoBarraMovil   = tema.Espacio(1.5) // gap-1.5
)

// NuevaBarraMovil crea la barra.
func NuevaBarraMovil() *BarraMovil {
	b := &BarraMovil{}
	b.ExtendBaseWidget(b)
	return b
}

// MedidaBarraMovil es lo que ocupa: su alto, con el filete dentro.
func MedidaBarraMovil() fyne.Size { return fyne.NewSize(0, AltoBarraMovil) }

// CreateRenderer compone fondo, filete, botón y título. Termina en Refresh (regla 7).
func (b *BarraMovil) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	boton := NuevoBotonIcono(iconos.Menu, "", nil)
	boton.Lado = MedidaBotonCon("", TallaSM, 0, 0, true, iconos.TallaMD).Width
	boton.TamIcono = iconos.TallaMD
	b.boton = boton
	r := &barraMovilRenderer{
		b:      b,
		fondo:  canvas.NewRectangle(tema.Color("surface", v)),
		filete: canvas.NewRectangle(tema.Color("border", v)),
		boton:  boton,
		titulo: Texto("", tema.TextoSM, SemiNegrita, tema.Color("text", v)),
	}
	r.Refresh()
	return r
}

type barraMovilRenderer struct {
	b             *BarraMovil
	fondo, filete *canvas.Rectangle
	boton         *BotonIcono
	titulo        *canvas.Text
}

func (r *barraMovilRenderer) MinSize() fyne.Size { return MedidaBarraMovil() }
func (r *barraMovilRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.fondo, r.filete, r.boton, r.titulo}
}
func (r *barraMovilRenderer) Destroy() {}

func (r *barraMovilRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	r.filete.Move(fyne.NewPos(0, t.Height-tema.Borde))
	r.filete.Resize(fyne.NewSize(t.Width, tema.Borde))
	linea := t.Height - tema.Borde
	x := canalonBarraMovil
	if r.boton.Visible() {
		lado := r.boton.MinSize()
		r.boton.Move(fyne.NewPos(canalonBarraMovil, (linea-lado.Height)/2))
		r.boton.Resize(lado)
		x += lado.Width + huecoBarraMovil
	}
	Truncar(r.titulo, r.tituloVisible(), tema.TextoSM, SemiNegrita, max(t.Width-canalonBarraMovil-x, 0))
	ColocarTexto(r.titulo, tema.TextoSM, x, (linea-tema.TextoSM.Linea)/2)
}

func (r *barraMovilRenderer) tituloVisible() string {
	if r.b.EnDetalle || !r.b.ConLista {
		return r.b.Titulo
	}
	return ""
}

func (r *barraMovilRenderer) Refresh() {
	b := r.b
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = tema.Color("surface", v)
	r.filete.FillColor = tema.Color("border", v)
	// En Marco esta fila prolonga la barra de la suite. El fondo lo pinta el
	// armazón, con el mismo degradado horizontal; la hoja empieza debajo.
	r.filete.Hidden = tema.EsMarco()
	r.titulo.Color = tema.Color("text", v)
	r.boton.Trazo, r.boton.TrazoEncima = "primary-text", "primary-text"
	r.boton.FondoEncima, r.boton.FondoActivo, r.boton.TokenAro = "selected", "selected", "focus-ring"
	r.fondo.Show()
	if tema.EsMarco() {
		r.fondo.Hide()
		r.titulo.Color = tema.Color("sidebar-fg", v)
		r.boton.Trazo, r.boton.TrazoEncima = "sidebar-fg", "sidebar-fg"
		r.boton.FondoEncima, r.boton.FondoActivo, r.boton.TokenAro = "sidebar-active-bg", "sidebar-active-bg", "sidebar-focus"
	}
	r.boton.Show()
	switch {
	case b.EnDetalle:
		r.boton.Icono, r.boton.Rotulo, r.boton.AlPulsar = iconos.ChevronIzquierda, b.RotuloVolver, b.AlVolver
	case b.AlMenu == nil:
		r.boton.Hide() // ni menú ni detalle: AppShell no pinta botón
	case b.CajonAbierto:
		r.boton.Icono, r.boton.Rotulo, r.boton.AlPulsar = iconos.Menu, b.RotuloCerrar, b.AlMenu
	default:
		r.boton.Icono, r.boton.Rotulo, r.boton.AlPulsar = iconos.Menu, b.RotuloAbrir, b.AlMenu
	}
	r.fondo.Refresh()
	r.filete.Refresh()
	r.boton.Refresh()
	r.Layout(b.Size())
	r.titulo.Refresh()
}

// ---- El botón flotante ----

// BotonFlotante es MobileFab.tsx: la acción principal al alcance del pulgar en el
// teléfono, donde el menú lateral es un cajón cerrado. Cuadrado de `h-14` en `primary`,
// con la esquina `rounded-control`, el icono de 24 y la sombra `lg`, a `right-4` y
// `bottom-4` (o el borde seguro, si es mayor). Solo se ve por debajo de 768
// (`md:hidden`): lo coloca `CapaFlotante`.
type BotonFlotante struct {
	Pulsable
	Icono iconos.Icono
}

// LadoBotonFlotante es el `h-14 w-14`: 49 con el rem de 14.
var LadoBotonFlotante = tema.Espacio(14)

// MargenBotonFlotante es el `right-4` y el `bottom-[max(1rem,…)]`.
var MargenBotonFlotante = tema.Espacio(4)

// NuevoBotonFlotante crea el botón con su nombre accesible y su icono.
func NuevoBotonFlotante(rotulo string, icono iconos.Icono, alPulsar func()) *BotonFlotante {
	b := &BotonFlotante{Icono: icono}
	b.Nombre, b.AlPulsar = rotulo, alPulsar
	b.ExtendBaseWidget(b)
	return b
}

// CreateRenderer compone sombra, caja, icono y anillo. Termina en Refresh.
func (b *BotonFlotante) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &flotanteRenderer{
		b:      b,
		sombra: NuevaSombra(tema.SombraLG, tema.Forma("radius-control")),
		fondo:  canvas.NewRectangle(tema.Color("primary", v)),
		glifo:  iconos.Trazo(b.Icono, iconos.TallaXL, "on-primary", v),
		aro:    NuevoAro(tema.Forma("radius-control"), SinHueco, v),
	}
	r.Refresh()
	return r
}

type flotanteRenderer struct {
	b      *BotonFlotante
	sombra *Sombra
	fondo  *canvas.Rectangle
	glifo  *iconos.Dibujo
	aro    *Aro
}

func (r *flotanteRenderer) MinSize() fyne.Size {
	return fyne.NewSize(LadoBotonFlotante, LadoBotonFlotante)
}

func (r *flotanteRenderer) Objects() []fyne.CanvasObject {
	return append([]fyne.CanvasObject{r.sombra.Objeto()}, r.aro.Antes(r.fondo, r.glifo.Objeto())...)
}
func (r *flotanteRenderer) Destroy() {}

func (r *flotanteRenderer) Layout(t fyne.Size) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.sombra.Ajustar(fyne.NewPos(0, 0), t, v)
	r.fondo.Resize(t)
	r.aro.Colocar(t)
	r.glifo.Colocar((t.Width-iconos.TallaXL)/2, (t.Height-iconos.TallaXL)/2, iconos.TallaXL)
}

func (r *flotanteRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	radio := tema.Forma("radius-control")
	r.fondo.CornerRadius = radio
	r.sombra.CambiarRadio(radio)
	r.aro.CambiarRadio(radio)
	switch {
	case r.b.Encima():
		r.fondo.FillColor = tema.Color("primary-hover", v)
	default:
		r.fondo.FillColor = tema.Color("primary", v)
	}
	r.glifo.Tenir("on-primary", v)
	r.aro.Pintar(v)
	r.aro.Ver(r.b.Enfocado())
	r.fondo.Refresh()
	r.Layout(r.b.Size())
}

// CapaFlotante es el `fixed right-4 bottom-4 md:hidden` del botón flotante: lo pega a
// la esquina de abajo a la derecha de lo que cubre, y lo esconde en ancho (≥ 768).
type CapaFlotante struct{}

var _ fyne.Layout = CapaFlotante{}

// MinSize no pide nada: la capa va encima de la pantalla y no la empuja.
func (CapaFlotante) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

// Layout coloca el botón en la esquina, o lo esconde.
func (CapaFlotante) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		esconder(o, t.Width >= Estrecho)
		m := o.MinSize()
		o.Move(fyne.NewPos(t.Width-MargenBotonFlotante-m.Width, t.Height-MargenBotonFlotante-m.Height))
		o.Resize(m)
	}
}
