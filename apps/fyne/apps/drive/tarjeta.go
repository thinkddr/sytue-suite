// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"image"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// # La tarjeta de la vista de cuadrícula, y dónde va su sombra
//
// La clase de la web es
// `group border-border bg-surface hover:border-border-strong relative flex cursor-pointer
// flex-col overflow-hidden rounded-lg border transition hover:shadow-[var(--shadow-sm)]`,
// con una previsualización de proporción 4/3 arriba y un pie de `border-t px-3 py-2`
// con el nombre (`text-base font-medium`) y la meta (`text-xs`, con `mt-0.5`).
//
// **La sombra va DENTRO del renderer de la tarjeta, la primera, y no reserva sitio en
// el layout.** Es la pregunta que dejó abierta quien portó `ui.Sombra`, y para esta
// vista la respuesta sale de la propia hoja: `box-shadow` no participa en el layout de
// CSS, así que reservarle hueco en la rejilla movería las tarjetas 10 px y rompería el
// reparto de columnas, que está medido. Se pinta fuera de la caja igual que hace
// `ui.Aro` con el anillo de foco —Fyne no recorta los objetos de un renderer a la caja
// de su widget—, y el contenedor de la rejilla es un `*fyne.Container`, que tampoco
// recorta.
//
// Lo que eso deja abierto, y conviene saberlo antes de copiarlo: el orden de pintado es
// el de los hijos, así que la sombra de una tarjeta queda TAPADA por la tarjeta de su
// derecha y por la de abajo. En la web pasa lo mismo mientras nadie ponga `z-index`, y
// como aquí la sombra solo aparece al pasar por encima, se ve donde importa —el borde
// izquierdo y el superior de la tarjeta señalada— y el `gap-4` (14) da de sobra para el
// alcance del `shadow-sm`. Una tarjeta con sombra EN REPOSO (`ui.Tarjeta`, `shadow-xs`)
// sí querría que el padre reordenara o que la rejilla pintara las sombras en una pasada
// aparte.
var RadioTarjeta = tema.RadioLG

const (
	// RadioTarjeta es `rounded-lg`.
	// ProporcionPrevia es el `aspect-[4/3]` de la previsualización.
	ProporcionPrevia float32 = 3.0 / 4.0
	// MargenPieTarjeta es `px-3` y RellenoPieTarjeta es `py-2`.
	MargenPieTarjeta  float32 = 3 * 0.25 * tema.Rem
	RellenoPieTarjeta float32 = 2 * 0.25 * tema.Rem
	// SeparaMeta es el `mt-0.5` entre el nombre y la meta.
	SeparaMeta float32 = 0.5 * 0.25 * tema.Rem
	// TeselaPrevia es el `size={48}` del icono de tipo dentro de la previsualización.
	TeselaPrevia float32 = 48

	// La tesela de la FRANJA de carpetas (`franja` en FileGrid): sin previsualización,
	// con el icono en un `CardIcon` (`h-8 w-8 rounded-md`, glifo de 18) con `mt-3 ml-3`.
	MargenChipFranja float32 = 3 * 0.25 * tema.Rem
	LadoChipFranja   float32 = 8 * 0.25 * tema.Rem
	GlifoFranja      float32 = 18
	// CasillaFranja es el `right-10` de la casilla, que en la franja va a la derecha,
	// junto al menú: la esquina izquierda es del icono.
	CasillaFranja float32 = 10 * 0.25 * tema.Rem
)

// AltoTeselaFranja es lo que mide una tesela de la franja: el icono con su `mt-3` y el
// pie SIN su `border-t`, más el borde. 102 con el rem de 16, que es lo medido en la web.
var AltoTeselaFranja = 2*tema.Borde + MargenChipFranja + LadoChipFranja + AltoPieTarjeta - tema.Borde

// AltoPieTarjeta es lo que ocupa el pie: `py-2` a cada lado, la caja de línea del
// nombre, el `mt-0.5` y la de la meta, más el `border-t` por arriba.
var AltoPieTarjeta = tema.Borde + 2*RellenoPieTarjeta + tema.TextoBase.Linea + SeparaMeta + tema.TextoXS.Linea

// MedidaTarjetaDrive es lo que ocupa una tarjeta a un ancho de columna dado: la
// previsualización con su proporción más el pie, y el borde por los cuatro lados.
func MedidaTarjetaDrive(ancho float32) fyne.Size {
	interior := max(ancho-2*tema.Borde, 0)
	return fyne.NewSize(ancho, 2*tema.Borde+interior*ProporcionPrevia+AltoPieTarjeta)
}

// TarjetaDrive es una tesela de la vista de cuadrícula.
type TarjetaDrive struct {
	widget.BaseWidget

	Nombre    string
	Extension string
	Carpeta   bool
	// Meta es el pie ya escrito: «Carpeta», o «1,2 MB · 6 sept 2026».
	Meta string
	// Seleccionada tiñe el borde, como `bg-selected` en la lista: `border-primary
	// ring-1 ring-primary`, que se ve como un borde de dos.
	Seleccionada bool
	// Destino es la carpeta sobre la que se está soltando lo arrastrado: `border-primary
	// ring-primary ring-2`.
	Destino bool
	AlAbrir func()
	// AlMarcar pone la casilla de la esquina (`top-1.5 left-1.5`); nil, no hay casilla.
	// Se ve si está marcada, si hay selección en la rejilla (SeleccionActiva), al pasar
	// por encima y siempre en un puntero grueso (Grueso).
	AlMarcar        func()
	SeleccionActiva bool
	// AlMenu es el «⋯» de la otra esquina (`top-1.5 right-1.5`), colgado de su caja.
	AlMenu func(disparador fyne.CanvasObject)
	Grueso bool
	// Miniatura es la de una imagen, ya descodificada: se pinta con `object-cover` en
	// la previsualización en vez de la tesela. nil, la tesela.
	Miniatura image.Image
	// Rotulo es el `aria-label` («Abrir la carpeta X»); vacío, el nombre.
	Rotulo string
	// Tono es el color de grupo de una CARPETA, por turno (`cardTone` en
	// FileBrowser.tsx): en la forma Marco la tarjeta es de color pleno, y lo de dentro
	// toma la tinta de ese relleno (`.sytue-card-tone` de appearance.css). En Carril no
	// pinta nada. Vacío, la tarjeta blanca de siempre.
	Tono ui.TonoTarjeta
	// Franja es la tesela de la franja de carpetas que va sobre la LISTA en la forma
	// Marco: baja, sin previsualización, con el icono en su chip y la casilla a la
	// derecha. Es la misma tarjeta, como en la web (`FileGrid` con `franja`).
	Franja bool

	encima, encimaMas bool
	enfocado          bool
	// yo es quien recibe el foco al pulsar: la propia tarjeta, o la que la embebe para
	// arrastrarse (`tarjetaArrastrable`), que es la que está de verdad en el lienzo.
	yo enfocable
	// ancho es el último ancho colocado, para que MinSize sepa el alto.
	ancho float32
	r     *tarjetaRenderer
}

var (
	_ fyne.Widget       = (*TarjetaDrive)(nil)
	_ fyne.Tappable     = (*TarjetaDrive)(nil)
	_ fyne.Focusable    = (*TarjetaDrive)(nil)
	_ desktop.Hoverable = (*TarjetaDrive)(nil)
)

// NuevaTarjetaDrive crea la tesela de un elemento.
func NuevaTarjetaDrive(nombre, extension string, carpeta bool) *TarjetaDrive {
	t := &TarjetaDrive{Nombre: nombre, Extension: extension, Carpeta: carpeta}
	t.ExtendBaseWidget(t)
	return t
}

// MinSize declara el alto a partir del ancho que la rejilla le dio la última vez. Con
// 0 todavía, se declara el de una columna mínima, que es lo que la rejilla usa para
// contar filas antes de colocar.
func (t *TarjetaDrive) MinSize() fyne.Size {
	t.ExtendBaseWidget(t)
	ancho := t.ancho
	if ancho <= 0 {
		ancho = 9 * tema.Rem
	}
	if t.Franja {
		return fyne.NewSize(ancho, AltoTeselaFranja)
	}
	return MedidaTarjetaDrive(ancho)
}

// Tapped abre el elemento, salvo en el «⋯», que abre su menú sin abrir la tarjeta. La
// casilla se la queda la `Eleccion`, que está más adentro.
func (t *TarjetaDrive) Tapped(e *fyne.PointEvent) {
	if e != nil && t.enMas(e.Position) && t.verMas() {
		if t.r != nil {
			t.AlMenu(t.r.zonaMas)
		}
		return
	}
	var yo enfocable = t
	if t.yo != nil {
		yo = t.yo
	}
	if c := fyne.CurrentApp().Driver().CanvasForObject(yo); c != nil {
		c.Focus(yo)
	}
	if t.AlAbrir != nil {
		t.AlAbrir()
	}
}

// enfocable es lo que puede recibir el foco al pulsar: un objeto del lienzo que además
// se enfoca.
type enfocable interface {
	fyne.CanvasObject
	fyne.Focusable
}

// grosorHalo es lo que sobresale el anillo: `ring-2` sobre la carpeta de destino,
// `ring-1` en la seleccionada, y nada en reposo.
func (t *TarjetaDrive) grosorHalo() float32 {
	switch {
	case t.Destino:
		return 2
	case t.Seleccionada:
		return 1
	}
	return 0
}

// MargenEsquina es el `top-1.5`/`left-1.5`/`right-1.5` de la casilla y del «⋯».
const MargenEsquina float32 = 1.5 * 0.25 * tema.Rem

// cajaMasTarjeta es dónde cae el «⋯»: 28 × 28 en la esquina de arriba a la derecha.
func cajaMasTarjeta(t fyne.Size) (fyne.Position, fyne.Size) {
	return fyne.NewPos(t.Width-MargenEsquina-ui.LadoMasFila, MargenEsquina), fyne.NewSize(ui.LadoMasFila, ui.LadoMasFila)
}

func (t *TarjetaDrive) enMas(p fyne.Position) bool {
	if t.AlMenu == nil {
		return false
	}
	o, m := cajaMasTarjeta(t.Size())
	return p.X >= o.X && p.X < o.X+m.Width && p.Y >= o.Y && p.Y < o.Y+m.Height
}

// verEsquinas dice si la casilla y el «⋯» se ven.
func (t *TarjetaDrive) verCasilla() bool {
	return t.AlMarcar != nil && (t.Seleccionada || t.SeleccionActiva || t.encima || t.Grueso)
}
func (t *TarjetaDrive) verMas() bool { return t.AlMenu != nil && (t.encima || t.Grueso) }

// MouseIn enciende el hover: borde fuerte y `shadow-sm`, y destapa las esquinas.
func (t *TarjetaDrive) MouseIn(e *desktop.MouseEvent) {
	t.encima, t.encimaMas = true, t.enMas(e.Position)
	t.Refresh()
}

// MouseMoved enciende el velo del «⋯» al pasar por encima de él.
func (t *TarjetaDrive) MouseMoved(e *desktop.MouseEvent) {
	if en := t.enMas(e.Position); en != t.encimaMas {
		t.encimaMas = en
		t.Refresh()
	}
}

// MouseOut lo apaga.
func (t *TarjetaDrive) MouseOut() { t.encima, t.encimaMas = false, false; t.Refresh() }

// FocusGained enciende el aro.
func (t *TarjetaDrive) FocusGained() { t.enfocado = true; t.Refresh() }

// FocusLost lo apaga.
func (t *TarjetaDrive) FocusLost() { t.enfocado = false; t.Refresh() }

// TypedRune abre con la barra espaciadora.
func (t *TarjetaDrive) TypedRune(r rune) {
	if r == ' ' {
		t.Tapped(nil)
	}
}

// TypedKey abre con Intro.
func (t *TarjetaDrive) TypedKey(e *fyne.KeyEvent) {
	if ui.SubirEscape(t, e) { // Escape y «atrás» son del diálogo de encima
		return
	}
	if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
		t.Tapped(nil)
	}
}

// AccessibilityLabel es el `aria-label` de la tarjeta, o su nombre.
func (t *TarjetaDrive) AccessibilityLabel() string {
	if t.Rotulo != "" {
		return t.Rotulo
	}
	return t.Nombre
}

// AccessibilityRole implementa fyne.Accessible.
func (t *TarjetaDrive) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// CreateRenderer compone la tarjeta: sombra, caja, previsualización, filete y pie.
func (t *TarjetaDrive) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &tarjetaRenderer{
		t:      t,
		sombra: ui.NuevaSombra(tema.SombraSM, RadioTarjeta),
		aro:    ui.NuevoAro(RadioTarjeta, ui.SinHueco, v),
		caja:   ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), RadioTarjeta),
		previa: canvas.NewRectangle(tema.Color("surface-sunken", v)),
		filete: canvas.NewRectangle(tema.Color("border", v)),
		tesela: ui.NuevaTeselaTipo(t.Extension, t.Carpeta, TeselaPrevia),
		nombre: ui.Texto(t.Nombre, tema.TextoBase, ui.Medio, tema.Color("text", v)),
		meta:   ui.Texto(t.Meta, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
	}
	// `overflow-hidden rounded-lg`: la previsualización sigue las esquinas de arriba de la
	// tarjeta. Con el gris sobre el lienzo no se notaba; con una carpeta de color pleno,
	// sus picos asomaban fuera del redondeo (visto en ventana real).
	r.previa.TopLeftCornerRadius, r.previa.TopRightCornerRadius = RadioTarjeta-tema.Borde, RadioTarjeta-tema.Borde
	if t.Franja {
		// En la franja lo que era la previsualización es el chip del icono.
		r.previa.TopLeftCornerRadius, r.previa.TopRightCornerRadius, r.previa.CornerRadius = 0, 0, tema.RadioMD
		r.carpeta = iconos.Trazo(iconos.DriveCarpeta, GlifoFranja, "text", v)
	}
	// El `ring-1` de la seleccionada y el `ring-2` del destino: un `box-shadow` que
	// SOBRESALE del borde, pintado como un rectángulo lleno DETRÁS de la caja opaca, igual
	// que el aro de foco relleno. (Iba por dentro con una `ui.Caja` sin borde, que no
	// pinta el borde nunca: la seleccionada salía con un filete y no con dos.)
	r.halo = canvas.NewRectangle(color.NRGBA{})
	// La sombra va la PRIMERA: se pinta detrás de todo y sobresale de la caja.
	r.objetos = []fyne.CanvasObject{r.sombra.Objeto(), r.halo}
	r.objetos = append(r.objetos, r.aro.Objetos()...)
	r.objetos = append(r.objetos, r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.previa)
	if t.Franja {
		r.objetos = append(r.objetos, r.carpeta.Objeto(), r.nombre, r.meta)
	} else {
		r.objetos = append(r.objetos, r.tesela.Objetos()...)
		if t.Miniatura != nil {
			r.imagen = canvas.NewImageFromImage(t.Miniatura)
			r.imagen.FillMode = canvas.ImageFillCover
			r.objetos = append(r.objetos, r.imagen)
		}
		r.objetos = append(r.objetos, r.filete, r.nombre, r.meta)
	}
	if t.AlMarcar != nil {
		r.casilla = ui.NuevaCasilla(t.Nombre, func(bool) { t.AlMarcar() })
		r.casilla.SinEtiqueta, r.casilla.Marcada = true, t.Seleccionada
		r.objetos = append(r.objetos, r.casilla)
	}
	if t.AlMenu != nil {
		r.zonaMas = canvas.NewRectangle(color.NRGBA{})
		r.zonaMas.CornerRadius = tema.RadioMD
		r.mas = glifoCon(glifoMas, iconos.TallaSM, tema.Color("text-muted", v))
		r.objetos = append(r.objetos, r.zonaMas, r.mas.Objeto())
	}
	t.r = r
	r.Refresh()
	return r
}

type tarjetaRenderer struct {
	t              *TarjetaDrive
	sombra         *ui.Sombra
	aro            *ui.Aro
	caja           *ui.Caja
	halo           *canvas.Rectangle
	previa, filete *canvas.Rectangle
	tesela         *ui.TeselaTipo
	carpeta        *iconos.Dibujo // el glifo del chip, solo en la franja
	imagen         *canvas.Image
	nombre, meta   *canvas.Text
	casilla        *ui.Eleccion
	zonaMas        *canvas.Rectangle
	mas            *iconos.Dibujo
	objetos        []fyne.CanvasObject
}

func (r *tarjetaRenderer) MinSize() fyne.Size           { return r.t.MinSize() }
func (r *tarjetaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *tarjetaRenderer) Destroy()                     {}

func (r *tarjetaRenderer) Layout(t fyne.Size) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.t.ancho = t.Width
	r.caja.Redimensionar(t)
	r.aro.Colocar(t)
	if r.t.encima {
		r.sombra.Ajustar(fyne.NewPos(0, 0), t, v)
	}

	interior := max(t.Width-2*tema.Borde, 0)
	altoPrevia := interior * ProporcionPrevia
	if r.t.Franja {
		// El chip del icono, con su `mt-3 ml-3`; el pie empieza justo debajo.
		o := tema.Borde + MargenChipFranja
		r.previa.Move(fyne.NewPos(o, o))
		r.previa.Resize(fyne.NewSize(LadoChipFranja, LadoChipFranja))
		r.carpeta.Colocar(o+(LadoChipFranja-GlifoFranja)/2, o+(LadoChipFranja-GlifoFranja)/2, GlifoFranja)
		altoPrevia = MargenChipFranja + LadoChipFranja - tema.Borde // el pie no lleva `border-t`
	} else {
		r.previa.Move(fyne.NewPos(tema.Borde, tema.Borde))
		r.previa.Resize(fyne.NewSize(interior, altoPrevia))
		r.tesela.Colocar(tema.Borde+(interior-TeselaPrevia)/2, tema.Borde+(altoPrevia-TeselaPrevia)/2)
	}
	if r.imagen != nil {
		r.imagen.Move(fyne.NewPos(tema.Borde, tema.Borde))
		r.imagen.Resize(fyne.NewSize(interior, altoPrevia))
	}
	r.colocarHalo(t)
	if r.casilla != nil {
		m := ui.MedidaEleccion("")
		x := MargenEsquina
		if r.t.Franja {
			x = t.Width - CasillaFranja - m.Width
		}
		r.casilla.Move(fyne.NewPos(x, MargenEsquina))
		r.casilla.Resize(m)
	}
	if r.zonaMas != nil {
		o, m := cajaMasTarjeta(t)
		r.zonaMas.Move(o)
		r.zonaMas.Resize(m)
		r.mas.Colocar(o.X+(m.Width-iconos.TallaSM)/2, o.Y+(m.Height-iconos.TallaSM)/2, iconos.TallaSM)
	}

	y := tema.Borde + altoPrevia
	r.filete.Move(fyne.NewPos(tema.Borde, y))
	r.filete.Resize(fyne.NewSize(interior, tema.Borde))

	x := tema.Borde + MargenPieTarjeta
	cabe := max(interior-2*MargenPieTarjeta, 0)
	y += tema.Borde + RellenoPieTarjeta
	ui.Truncar(r.nombre, r.t.Nombre, tema.TextoBase, ui.Medio, cabe)
	ui.ColocarTexto(r.nombre, tema.TextoBase, x, y)
	y += tema.TextoBase.Linea + SeparaMeta
	ui.Truncar(r.meta, r.t.Meta, tema.TextoXS, ui.Normal, cabe)
	ui.ColocarTexto(r.meta, tema.TextoXS, x, y)
}

// tintasTarjeta son los colores de una tarjeta.
type tintasTarjeta struct {
	fondo, texto, apoyo, hundido, velo, filete, mas color.NRGBA
	// pleno: es la tarjeta de recurso de color pleno.
	pleno bool
}

// tintas son los de la superficie blanca o, para una carpeta en la forma Marco, los de
// la tarjeta de recurso de color pleno: el relleno de su tono y, sobre él, las
// reasignaciones de `.sytue-card-tone` —el texto y el de apoyo en la tinta del relleno,
// el panel hundido y el velo a su 14 % y el filete a su 22 %—.
func (t *TarjetaDrive) tintas(v tema.Variante) tintasTarjeta {
	if t.Tono == "" || !tema.EsMarco() {
		return tintasTarjeta{
			fondo: tema.Color("surface", v), texto: tema.Color("text", v), apoyo: tema.Color("text-subtle", v),
			hundido: tema.Color("surface-sunken", v), velo: tema.Color("bg-subtle", v),
			filete: tema.Color("border", v), mas: tema.Color("text-muted", v),
		}
	}
	fondo, tinta := ui.TonoSolido(t.Tono, v)
	velo := tokens.Aplanar(tinta, 0.14, fondo)
	return tintasTarjeta{
		fondo: fondo, texto: tinta, apoyo: tinta, hundido: velo, velo: velo,
		filete: tokens.Aplanar(tinta, 0.22, fondo), mas: tinta, pleno: true,
	}
}

// colocarHalo pone el anillo alrededor de la caja con el grosor de ahora.
func (r *tarjetaRenderer) colocarHalo(t fyne.Size) {
	g := r.t.grosorHalo()
	r.halo.CornerRadius = RadioTarjeta + g
	r.halo.Move(fyne.NewPos(-g, -g))
	r.halo.Resize(fyne.NewSize(t.Width+2*g, t.Height+2*g))
}

func (r *tarjetaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	borde := tema.Color("border", v)
	if r.t.encima { // `hover:border-border-strong`
		borde = tema.Color("border-strong", v)
	}
	if r.t.Seleccionada || r.t.Destino {
		borde = tema.Color("primary", v)
	}
	c := r.t.tintas(v)
	if c.pleno { // `border-color: transparent`: el de selección es el halo
		borde = c.fondo
	}
	r.caja.Pintar(c.fondo, borde)
	r.halo.FillColor = color.NRGBA{}
	if r.t.grosorHalo() > 0 {
		r.halo.FillColor = tema.Color("primary", v)
	}
	r.colocarHalo(r.t.Size()) // el grosor cambia al entrar o salir un arrastre
	if r.casilla != nil {
		r.casilla.Marcada = r.t.Seleccionada
		r.casilla.Hidden = !r.t.verCasilla()
	}
	if r.zonaMas != nil {
		r.zonaMas.FillColor = color.NRGBA{}
		mas := c.mas
		if r.t.encimaMas {
			r.zonaMas.FillColor, mas = c.velo, c.texto
		}
		r.mas.TenirCon(mas)
		r.mas.Ver(r.t.verMas())
	}
	r.previa.FillColor = c.hundido
	r.filete.FillColor = c.filete
	r.tesela.Poner(r.t.Extension, r.t.Carpeta, v)
	if c.pleno {
		r.tesela.SobreTono(c.hundido, c.texto)
	}
	if r.t.Franja {
		// `CardIcon` dentro de la tarjeta de recurso: un velo del 22 % de su tinta, y el
		// relleno del glifo a su 30 %.
		chip := tokens.Aplanar(c.texto, 0.22, c.fondo)
		r.previa.FillColor = chip
		r.carpeta.TenirCon(c.texto)
		r.carpeta.Acento(tokens.Aplanar(c.texto, 0.30, chip))
	}
	if r.imagen != nil {
		// Con miniatura, la tesela no se pinta: una imagen con transparencias la
		// enseñaría por detrás.
		for _, o := range r.tesela.Objetos() {
			o.Hide()
		}
	}
	r.nombre.Color = c.texto
	r.meta.Color = c.apoyo
	r.aro.Pintar(v)
	r.aro.Ver(r.t.enfocado)
	// La sombra solo existe al pasar por encima (`hover:shadow-[var(--shadow-sm)]`).
	if r.t.encima {
		r.sombra.Objeto().Show()
	} else {
		r.sombra.Objeto().Hide()
	}
	for _, o := range r.objetos {
		o.Refresh()
	}
}
