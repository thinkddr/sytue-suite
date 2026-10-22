// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/AdminShell.tsx
// paridad-web: apps/frontend/packages/ui/src/components/AppShell.tsx
// paridad-web: apps/frontend/packages/ui/src/components/ResizablePanel.tsx

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// El armazón de pantalla de la suite (AppShell.tsx): menú, panel de lista y contenido,
// en ese orden. Medido en `mail/bandeja.ancho.json` a 1280×800:
//
//	menú      0 → 260      (260, con su filete dentro)
//	panel   260 → 540      (280 = `md:w-80`, y el `section` de dentro mide 279:
//	                        el `border-r` se lo come)
//	contenido 540 → 1280   (740, lo que sobra)
//
// La barra de la suite (`BarraSuite`) va FUERA del armazón, encima: la monta cada `main`
// con `Montar`, así que el armazón empieza en y = 0 de lo que queda debajo.
//
// El panel crece por escalones con la ventana, como en la hoja: 280 hasta 1535, 336
// (`2xl:w-96`) desde 1536 y 364 (`min-[1920px]:w-[26rem]`) desde 1920. Más ancho antes
// dejaría el detalle en 640 px en un portátil de 1280.
const (
	// Estrecho es el `md` de la hoja: por debajo, el menú es un cajón y la lista y el
	// detalle se turnan la pantalla.
	Estrecho float32 = 768
	// AnchoPanelBase es `md:w-80` con el rem de 14.
	AnchoPanelBase = 20 * tema.Rem
	// AnchoPanel2XL es `2xl:w-96`, desde 1536.
	AnchoPanel2XL = 24 * tema.Rem
	// AnchoPanelAncho es `min-[1920px]:w-[26rem]`.
	AnchoPanelAncho = 26 * tema.Rem
	// AnchoPanelMinimo es el ancho mínimo del panel de lista (180 px, como en la web).
	AnchoPanelMinimo float32 = 180
	// AnchoSeparadorPanel es el área de agarre del separador (6 px, como en la web).
	AnchoSeparadorPanel float32 = 6
	// AnchoContenidoMinimo es el espacio mínimo reservado para el detalle (280 px, como en la web).
	AnchoContenidoMinimo float32 = 280
)

// AnchoPanel es el escalón que le toca al panel de lista con esa anchura de ventana.
func AnchoPanel(ventana float32) float32 {
	switch {
	case ventana >= 1920:
		return AnchoPanelAncho
	case ventana >= 1536: // el `2xl` de la hoja, que va en px y no en el rem de la suite
		return AnchoPanel2XL
	}
	return AnchoPanelBase
}

// MargenContenido es el `p-6` del contenido restringido.
var MargenContenido = tema.Espacio(6)

// Armazon es AppShell: el marco de la ventana.
type Armazon struct {
	pulsable
	sinParada // el marco no es una parada de Tab

	// Menu es el carril de la izquierda. Puede ser nil (pantallas sin navegación).
	Menu fyne.CanvasObject
	// Panel es la lista de las apps de bandeja (Mail, Chat). Puede ser nil.
	Panel fyne.CanvasObject
	// Contenido es el detalle. Se lleva lo que sobra.
	Contenido fyne.CanvasObject
	// BarraMovil va sobre las columnas cuando la ventana es estrecha: el botón de menú
	// y el de volver. La pone la app, porque son botones y los botones son de otra
	// pieza. nil: no hay barra.
	BarraMovil fyne.CanvasObject
	// Restringido acota el contenido al ancho canónico (1280) con `p-6`, centrado.
	Restringido bool
	// DetalleAbierto solo cuenta con Panel y ventana estrecha: `true` enseña el
	// detalle y `false` la lista. Sin él, en estrecho el detalle sería inalcanzable.
	DetalleAbierto bool
	// CajonAbierto saca el menú por encima del contenido en ventana estrecha, con su
	// velo, que es lo que hace el `drawer` de la web.
	CajonAbierto bool
	// Titulo es el de la barra móvil que pone el propio armazón cuando la app no trae
	// la suya (el `title` de AppShell.tsx).
	Titulo string
	// AlCajon avisa cuando el cajón se abre o se cierra DESDE el armazón —el velo, su
	// barra móvil, la ventana que se ensancha—, para quien guarde ese estado. Solo debe
	// guardarlo: llega también desde Layout.
	AlCajon func(abierto bool)
	// AlVolver es el `onBack` de la barra móvil que pone el armazón: sin él, en
	// estrecho y con el detalle abierto no hay botón de volver, como en la web.
	AlVolver func()
	// SinBarraMovil es para la app que lleva su propio botón del cajón dentro del
	// contenido, como el ERP (`#erp-cajon-boton`, «el único mando de navegación que hay
	// por debajo de 768 px»): sin esto salían dos hamburguesas, la de la barra del
	// armazón y la suya (visto en una ventana real el 2026-09-27).
	SinBarraMovil bool
	// Carril es el carril de ICONOS de la app que lleva el menú en un cajón también en
	// ancho, como el ERP (`.erp-rail` y `#erp-cajon` de ErpApp.tsx): en ancho va a la
	// izquierda, y la hoja empieza donde él acaba; por debajo de 768 no se pinta
	// (`display: none`). Con él, el Menu es SIEMPRE cajón, y conserva su fondo.
	Carril fyne.CanvasObject
	// AnchoPanelPersonalizado es el ancho manual del panel (si > 0).
	// Si es 0, usa AnchoPanel(ventana) o el guardado en preferencias.
	AnchoPanelPersonalizado float32
	// ClavePreferenciaPanel es la clave en preferencias donde se persiste el ancho
	// del panel (por ejemplo "sytue.mail.panel-width").
	ClavePreferenciaPanel string

	barra      *BarraMovil // la del armazón, cuando la app no trae ninguna
	anchoPanel float32
	separador  *separadorPanel
	// entrada es lo que le falta al cajón para llegar a su sitio: `sytue-drawer-in`, de
	// `translateX(-100%)` a cero en 180 ms.
	entrada float32
}

func (a *Armazon) anchoPanelActual() float32 {
	if a.anchoPanel > 0 {
		return a.anchoPanel
	}
	if a.AnchoPanelPersonalizado > 0 {
		return a.AnchoPanelPersonalizado
	}
	if a.ClavePreferenciaPanel != "" {
		if p := fyne.CurrentApp().Preferences(); p != nil {
			v := float32(p.FloatWithFallback(a.ClavePreferenciaPanel, 0))
			if v >= AnchoPanelMinimo {
				return v
			}
		}
	}
	return AnchoPanel(a.Size().Width)
}

func (a *Armazon) anchoMaximoPanel() float32 {
	t := a.Size()
	separa := a.separacionHoja(t.Width < Estrecho)
	disponible := t.Width - separa
	if a.Menu != nil && (t.Width >= Estrecho && a.Carril == nil) {
		disponible -= a.Menu.MinSize().Width
	} else if a.Carril != nil && t.Width >= Estrecho {
		disponible -= a.Carril.MinSize().Width
	}
	return max(AnchoPanelMinimo, disponible-AnchoContenidoMinimo)
}

func (a *Armazon) fijarAnchoPanel(ancho float32) {
	minimo := AnchoPanelMinimo
	maximo := a.anchoMaximoPanel()
	if ancho < minimo {
		ancho = minimo
	}
	if ancho > maximo {
		ancho = maximo
	}
	a.anchoPanel = ancho
	a.AnchoPanelPersonalizado = ancho
	a.Refresh()
}

func (a *Armazon) guardarAnchoPreferencia() {
	if a.ClavePreferenciaPanel == "" {
		return
	}
	if p := fyne.CurrentApp().Preferences(); p != nil {
		p.SetFloat(a.ClavePreferenciaPanel, float64(a.anchoPanelActual()))
	}
}

func (a *Armazon) restablecerAnchoPanel() {
	a.anchoPanel = AnchoPanel(a.Size().Width)
	a.AnchoPanelPersonalizado = 0
	if a.ClavePreferenciaPanel != "" {
		if p := fyne.CurrentApp().Preferences(); p != nil {
			p.RemoveValue(a.ClavePreferenciaPanel)
		}
	}
	a.Refresh()
}

func (a *Armazon) separadorPanel() *separadorPanel {
	if a.separador == nil {
		a.separador = nuevoSeparadorPanel(a)
	}
	return a.separador
}

// DuracionCajon es `sytue-drawer-in`.
const DuracionCajon = 180 * time.Millisecond

// desplazamientoCajon es lo que le queda al cajón de `sytue-drawer-in`: cero si el menú
// no es cajón o ya ha entrado.
func (a *Armazon) desplazamientoCajon(cajon bool) float32 {
	if cajon {
		return a.entrada
	}
	return 0
}

// entrarCajon arranca `sytue-drawer-in`: el cajón entra desde la izquierda, todo su ancho.
// El velo no se mueve: en la web lleva `sytue-fade-in` y aparece en su sitio.
func (a *Armazon) entrarCajon() {
	if a.Menu == nil || SinMovimiento {
		return
	}
	ancho := a.Menu.MinSize().Width
	anim := fyne.NewAnimation(DuracionCajon, func(p float32) {
		a.entrada = -ancho * (1 - p)
		if a.CajonAbierto {
			a.Menu.Move(fyne.NewPos(a.entrada, 0))
		}
	})
	anim.Curve = CurvaEstandar
	anim.Start()
}

// CerrarCajon cierra el cajón. Es lo que la web hace al activar algo del menú en el
// teléfono —el cajón tapa la pantalla y lo pulsado se abría detrás—, y aquí lo llama la
// app desde lo que se activa en su menú, porque Fyne no deja escuchar desde el
// contenedor los toques de los hijos.
//
// ponytail: la web lo hace sola con un delegador en el `<aside>`; aquí cada app lo
// llama en sus destinos (Contactos ya lo hace con `enMenu`). Y el teclado del cajón
// (`useNavDrawer`: foco dentro, Tab atrapado, Escape cierra, foco de vuelta al botón)
// no está: el cajón no es una capa de Fyne, que es lo que daría las tres cosas.
func (a *Armazon) CerrarCajon() { a.cambiarCajon(false) }

// AbrirCajon lo abre, con su entrada y su teclado: es el botón de menú que la app lleva
// fuera de la barra móvil del armazón (el del carril de iconos del ERP).
func (a *Armazon) AbrirCajon() { a.cambiarCajon(true) }

func (a *Armazon) cambiarCajon(abierto bool) {
	if a.CajonAbierto == abierto {
		return
	}
	a.CajonAbierto = abierto
	if a.AlCajon != nil {
		a.AlCajon(abierto)
	}
	a.Refresh()
	a.tecladoCajon(abierto)
	if abierto {
		a.entrarCajon()
	}
}

// # El teclado del cajón (`useNavDrawer`)
//
// Al abrirlo, el foco entra en el menú; Escape lo cierra; el Tab da vueltas DENTRO; y al
// cerrarlo el foco vuelve al botón que lo abrió. El cajón no es una capa de Fyne —la
// barra de la suite tiene que seguir viva encima, como en la web—: el armazón se apunta
// como cajón abierto de su ventana, `Descartar` lo cierra si no hay capa encima, y el Tab
// lo encierra el gestor de foco del fork (FocusScope), sea cual sea el control.
var cajones porLienzo[Armazon]

// FocusScope es el gestor de foco POR SUBÁRBOL del fork de Fyne (desde
// `v2.8.1-sytue.5`): mientras el cajón está abierto, el Tab de la ventana solo recorre el
// menú, sea cual sea el control —un `widget.Entry` o un widget propio de una app que no
// dice sus hijos—, y la barra de la suite sigue viva encima para el puntero.
func (a *Armazon) FocusScope() fyne.CanvasObject {
	if a.CajonAbierto && a.Menu != nil && a.Menu.Visible() {
		return a.Menu
	}
	return nil
}

// cajonDe es el armazón con el cajón abierto en esa ventana, si lo hay.
func cajonDe(c fyne.Canvas) *Armazon { return cajones.de(c) }

func (a *Armazon) tecladoCajon(abierto bool) {
	c := lienzoDe(a)
	if c == nil {
		return
	}
	if !abierto {
		cajones.quitar(c)
		if b, es := a.barraMovil().(*BarraMovil); es && b.boton != nil {
			enfocar(c, b.boton) // `document.getElementById(toggleId)?.focus()`
		}
		return
	}
	cajones.poner(c, a)
	asegurarTeclas(c)
	switch m := a.Menu.(type) {
	case *Lateral:
		m.dar(c, m) // el menú, que no es parada: el Tab sigue a su primera fila
	case fyne.Focusable:
		enfocar(c, m)
	}
}

// conHijos es la pieza del lateral que dice qué controles lleva dentro.
type conHijos interface{ hijosDeCajon() []fyne.CanvasObject }

// barraMovil es la barra que se coloca arriba en estrecho: la de la app si la trae y,
// si no, la del armazón, que es lo que hace AppShell.tsx por su cuenta en cuanto hay
// menú o panel. Sin ella, en un teléfono no había forma de abrir el menú en Mail,
// Notepads, Drive, Calendar ni el ERP.
func (a *Armazon) barraMovil() fyne.CanvasObject {
	if a.BarraMovil != nil {
		return a.BarraMovil
	}
	if a.SinBarraMovil || (a.Menu == nil && a.Panel == nil) {
		return nil
	}
	if a.barra == nil {
		a.barra = NuevaBarraMovil()
	}
	b := a.barra
	b.Titulo = a.Titulo
	b.EnDetalle = a.Panel != nil && a.DetalleAbierto && a.AlVolver != nil
	b.CajonAbierto = a.CajonAbierto
	b.RotuloAbrir, b.RotuloCerrar, b.RotuloVolver =
		textoComun("ui.openNav"), textoComun("ui.closeNav"), textoComun("ui.backToList")
	b.AlMenu = nil
	if a.Menu != nil {
		b.AlMenu = func() { a.cambiarCajon(!a.CajonAbierto) }
	}
	b.AlVolver = func() {
		a.DetalleAbierto = false
		if a.AlVolver != nil {
			a.AlVolver()
		}
		a.Refresh()
	}
	return b
}

// volverAlPanel usa la misma acción que el botón visible, también después de
// girar el teléfono. Las apps que aportan su barra conservan sus callbacks.
func (a *Armazon) volverAlPanel() bool {
	b, ok := a.barraMovil().(*BarraMovil)
	if !ok || !b.EnDetalle || b.AlVolver == nil {
		return false
	}
	b.AlVolver()
	return true
}

var _ fyne.Widget = (*Armazon)(nil)

// NuevoArmazon crea el marco con su contenido.
func NuevoArmazon(contenido fyne.CanvasObject) *Armazon {
	a := &Armazon{Contenido: contenido}
	a.Nombre = "contenido"
	a.ExtendBaseWidget(a)
	return a
}

// AccessibilityRole: el marco agrupa.
func (a *Armazon) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// MedidaArmazon es el mínimo del marco: en la pantalla ocupa la ventana entera.
func MedidaArmazon(a *Armazon) fyne.Size {
	ancho := float32(0)
	switch {
	case a.Carril != nil: // el menú es un cajón: lo que ocupa sitio es el carril
		ancho += a.Carril.MinSize().Width
	case a.Menu != nil:
		ancho += a.Menu.MinSize().Width
	}
	if a.Panel != nil {
		ancho += AnchoPanelBase
	}
	return fyne.NewSize(ancho, 0)
}

// CreateRenderer construye el marco: fondo, velo del cajón y las tres columnas.
func (a *Armazon) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &armazonRenderer{
		a:     a,
		fondo: NuevoFondoSuite(v),
		// El velo del cajón SE PULSA y lo cierra, como el `onClick` del de AppShell.
		// Era un rectángulo y el toque lo atravesaba hasta el contenido de debajo.
		velo:  NuevoVelo(true, func() { a.cambiarCajon(false) }),
		base:  canvas.NewRectangle(color.NRGBA{}),
		marco: nuevoMarco(),
	}
	if a.Panel != nil && a.Contenido != nil {
		r.separador = a.separadorPanel()
	}
	r.rehacer()
	return r
}

type armazonRenderer struct {
	a     *Armazon
	fondo *FondoSuite
	velo  *Velo
	// base es el suelo OPACO del cajón. El carril del traslúcido deja ver un punto de lo
	// que tiene debajo, que en escritorio es el fondo ambiental y en el cajón, el
	// contenido: el texto se colaba entre los rótulos del menú. La web le pone el mismo
	// color sobre `neutral-50` (`nav.bg-sidebar` por debajo de 768, appearance.css).
	base *canvas.Rectangle
	// marco es la forma «Marco»: el color continuo detrás del menú y la hoja en la que
	// van el panel y el contenido (ver marco.go). En Carril no se ve.
	marco     *marco
	separador *separadorPanel
	objetos   []fyne.CanvasObject
}

// rehacer arma la lista de objetos en el orden en que se pintan: el menú va el ÚLTIMO
// cuando es cajón, para que quede por encima del velo.
func (r *armazonRenderer) rehacer() {
	a := r.a
	r.objetos = append([]fyne.CanvasObject{r.fondo}, r.marco.detras()...)
	for _, o := range []fyne.CanvasObject{a.barraMovil(), a.Panel, a.Contenido} {
		if o != nil {
			r.objetos = append(r.objetos, o)
		}
	}
	if a.Panel != nil && a.Contenido != nil {
		if r.separador == nil {
			r.separador = a.separadorPanel()
		}
		r.objetos = append(r.objetos, r.separador)
	}
	// Las máscaras de la hoja y su sombra, DESPUÉS de lo que va dentro de ella y antes
	// del menú, que va fuera.
	r.objetos = append(r.objetos, r.marco.delante()...)
	if a.Carril != nil { // bajo el velo: el cajón abierto lo tapa, como en la web
		r.objetos = append(r.objetos, a.Carril)
	}
	if a.Menu != nil {
		r.objetos = append(r.objetos, r.velo, r.base, a.Menu)
	}
}

// separacionHoja es lo que la hoja de Marco se separa del borde de la ventana
// (`--sytue-sheet-gap`): nada en Carril, y nada en estrecho, donde va a sangre.
func (a *Armazon) separacionHoja(estrecha bool) float32 {
	if estrecha || !tema.EsMarco() {
		return 0
	}
	return tema.Forma("sheet-gap")
}

// colocarMarco pinta la forma Marco: el color continuo detrás de todo y la hoja desde
// `x` —donde acaba el menú— hasta lo que se separa del borde. En Carril lo quita y deja
// el lienzo de siempre. Y le dice al menú si deja ver el marco, que ya es de su color:
// en el cajón —el del teléfono y el de quien lleva carril de iconos— conserva su fondo.
func (r *armazonRenderer) colocarMarco(t fyne.Size, x, arriba, separa float32, estrecha bool) {
	enMarco := tema.EsMarco()
	sinFondo := enMarco && !estrecha && r.a.Carril == nil
	if l := lateralEn(r.a.Menu); l != nil && l.SinFondo != sinFondo {
		l.SinFondo = sinFondo
		l.Refresh()
	}
	if !enMarco {
		r.fondo.Show()
		r.marco.esconder()
		return
	}
	r.fondo.Hide()
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	hoja := fyne.NewSize(max(t.Width-x-separa, 0), max(t.Height-arriba-separa, 0))
	r.marco.colocar(fyne.NewPos(0, 0), t, fyne.NewPos(x, arriba), hoja, estrecha, v)
}

// ponerBase coloca el suelo opaco bajo el cajón y lo enseña solo cuando hace falta: con
// el cajón abierto y un carril que deja ver lo de debajo (el del traslúcido).
func (r *armazonRenderer) ponerBase(conCajon bool) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	esconder(r.base, !conCajon || tema.Color("sidebar-bg", v).A == 0xff)
	r.base.FillColor = tema.Color("neutral-50", v)
	r.base.Move(r.a.Menu.Position())
	r.base.Resize(r.a.Menu.Size())
}

func (r *armazonRenderer) MinSize() fyne.Size           { return MedidaArmazon(r.a) }
func (r *armazonRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *armazonRenderer) Destroy()                     {}

// Layout reparte la ventana. Ancha: menú, panel y contenido en fila. Estrecha: el menú
// es cajón sobre velo, y panel y contenido se turnan.
func (r *armazonRenderer) Layout(t fyne.Size) {
	a := r.a
	r.fondo.Resize(t)
	estrecha := t.Width < Estrecho
	// En Marco el panel y el contenido van en la HOJA, que se separa del borde derecho y
	// del de abajo (el `margin` de `.sytue-sheet`) y, sin menú a su izquierda, también de ese.
	// En estrecho va a sangre, como en la web: sin separación y solo con las esquinas
	// de arriba.
	separa := a.separacionHoja(estrecha)

	// Con carril de iconos el menú es cajón a cualquier ancho.
	esCajon := estrecha || a.Carril != nil
	// Al pasar a ancho el cajón se cierra solo, que es lo que hace `useNavDrawer`: si
	// no, al volver a estrecho aparecía abierto sin que nadie lo hubiera pedido.
	if !esCajon && a.CajonAbierto {
		a.CajonAbierto = false
		if a.AlCajon != nil {
			a.AlCajon(false)
		}
	}
	arriba := float32(0)
	if bm := a.barraMovil(); bm != nil {
		if b, es := bm.(*BarraMovil); es {
			b.ConLista = a.Panel != nil
			b.CajonAbierto = a.CajonAbierto
			b.Refresh()
		}
		h := bm.MinSize().Height
		esconder(bm, !estrecha)
		if estrecha {
			bm.Move(fyne.NewPos(0, 0))
			bm.Resize(fyne.NewSize(t.Width, h))
			arriba = h
		}
	}
	alto := max(t.Height-arriba-separa, 0)

	x := r.colocarNavegacion(t, separa, estrecha, esCajon)

	r.colocarMarco(t, x, arriba, separa, estrecha)

	verDetalle := a.Panel == nil || !estrecha || a.DetalleAbierto
	x = r.colocarPanel(t, x, arriba, alto, estrecha, verDetalle)
	if a.Contenido == nil {
		return
	}
	esconder(a.Contenido, !verDetalle)
	ancho := max(t.Width-x-separa, 0)
	if !a.Restringido {
		a.Contenido.Move(fyne.NewPos(x, arriba))
		a.Contenido.Resize(fyne.NewSize(ancho, alto))
		return
	}
	// `mx-auto w-full p-6` con `max-width: var(--container-content)`.
	util := min(ancho-2*MargenContenido, tema.AnchoContenido)
	a.Contenido.Move(fyne.NewPos(x+(ancho-util)/2, arriba+MargenContenido))
	a.Contenido.Resize(fyne.NewSize(max(util, 0), max(alto-2*MargenContenido, 0)))
}

// colocarPanel posiciona el panel de lista y su separador arrastrable, devolviendo
// la coordenada X donde empieza el detalle.
func (r *armazonRenderer) colocarPanel(t fyne.Size, x, arriba, alto float32, estrecha, verDetalle bool) float32 {
	a := r.a
	if a.Panel == nil {
		if r.separador != nil {
			esconder(r.separador, true)
		}
		return x
	}
	ancho := a.anchoPanelActual()
	if estrecha {
		ancho = t.Width
	} else {
		maximo := a.anchoMaximoPanel()
		if ancho > maximo {
			ancho = maximo
		}
		if ancho < AnchoPanelMinimo {
			ancho = AnchoPanelMinimo
		}
	}
	esconder(a.Panel, estrecha && verDetalle)
	a.Panel.Move(fyne.NewPos(x, arriba))
	a.Panel.Resize(fyne.NewSize(ancho, alto))
	if !estrecha {
		if r.separador != nil {
			esconder(r.separador, a.Contenido == nil)
			if a.Contenido != nil {
				r.separador.Move(fyne.NewPos(x+ancho-AnchoSeparadorPanel/2, arriba))
				r.separador.Resize(fyne.NewSize(AnchoSeparadorPanel, alto))
			}
		}
		return x + ancho
	}
	if r.separador != nil {
		esconder(r.separador, true)
	}
	return x
}

// colocarNavegacion pone el carril de iconos y el menú —fijo a la izquierda o cajón sobre
// su velo— y devuelve dónde empieza lo que va a su derecha.
func (r *armazonRenderer) colocarNavegacion(t fyne.Size, separa float32, estrecha, esCajon bool) float32 {
	a := r.a
	x := separa
	if a.Carril != nil {
		esconder(a.Carril, estrecha)
		if !estrecha {
			x = a.Carril.MinSize().Width
			a.Carril.Move(fyne.NewPos(0, 0))
			a.Carril.Resize(fyne.NewSize(x, t.Height))
		}
	}
	if a.Menu == nil {
		return x
	}
	ancho := a.Menu.MinSize().Width
	conCajon := esCajon && a.CajonAbierto
	visible := !esCajon || a.CajonAbierto
	esconder(a.Menu, !visible)
	esconder(r.velo, !conCajon)
	// El cajón y su velo arrancan ARRIBA, justo bajo la barra de la suite, y tapan
	// la barra móvil, como en AppShell.tsx (`BAJO_LA_TOPBAR`): con 38,5 de desfase
	// la barra quedaba viva y sin velo entre la de la suite y el cajón.
	if visible {
		a.Menu.Move(fyne.NewPos(a.desplazamientoCajon(conCajon), 0))
		a.Menu.Resize(fyne.NewSize(ancho, t.Height))
	}
	r.ponerBase(conCajon)
	if !esCajon {
		return ancho
	}
	// El cajón tapa el contenido, pero el contenido sigue ocupando su sitio: al
	// cerrarlo no se recoloca nada.
	r.velo.Move(fyne.NewPos(0, 0))
	r.velo.Resize(t)
	return x
}

// esconder es el `hidden` de la web: un objeto invisible no se pinta y tampoco recibe
// el puntero, que es lo que hace falta con el cajón cerrado.
func esconder(o fyne.CanvasObject, oculto bool) {
	if oculto {
		o.Hide()
		return
	}
	o.Show()
}

// Refresh repinta fondo y velo, y REHACE la disposición.
//
// Lo segundo hace falta y no es una precaución: abrir el detalle o el cajón no cambia
// el tamaño del marco, y Fyne solo llama a Layout cuando el tamaño cambia. Sin esto,
// pulsar un correo en el teléfono no enseña nada.
func (r *armazonRenderer) Refresh() {
	// Y REHACE la lista de objetos: quien cambia el menú, el panel o el contenido
	// después del primer pintado —Mail reconstruye su lateral con cada dato que llega—
	// dejaba el nuevo fuera de `Objects()`, y un objeto que no está ahí no se pinta: el
	// carril se quedaba en blanco.
	r.rehacer()
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.PonerVariante(v)
	r.velo.Refresh()
	if r.separador != nil {
		r.separador.Refresh()
	}
	r.Layout(r.a.Size())
}
