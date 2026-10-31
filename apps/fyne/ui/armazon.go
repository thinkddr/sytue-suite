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
	// MargenCajonMovil deja siempre una franja de velo visible y pulsable a la derecha
	// del cajón en pantallas estrechas (56 px = 3,5 rem).
	MargenCajonMovil float32 = 56
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
	// CajonAbierto abre el menú como cajón superpuesto en móvil (o con carril de iconos).
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
	// SinBarraMovil es para la app que en Carril lleva su propio botón del cajón dentro del
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
	// entrada es lo que le falta al cajón para llegar a su sitio (`<= 0`): `sytue-drawer-in`,
	// de `translateX(-100%)` a cero en 180 ms, el cierre inverso y el arrastre táctil.
	entrada          float32
	arrastrandoCajon bool
	cerrandoCajon    bool
	r                *armazonRenderer
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

func (a *Armazon) anchoHoja() float32 {
	menu, carril := float32(0), float32(0)
	if a.Menu != nil {
		menu = a.Menu.MinSize().Width
	}
	if a.Carril != nil {
		carril = a.Carril.MinSize().Width
	}
	return AnchoHoja(a.Size().Width, menu, carril)
}

func (a *Armazon) anchoMaximoPanel() float32 {
	return max(AnchoPanelMinimo, a.anchoHoja()-AnchoContenidoMinimo)
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

// anchoCajon acota el ancho del menú cuando sale como cajón en teléfono para conservar
// siempre velo libre a la derecha (`MargenCajonMovil`).
func (a *Armazon) anchoCajon(anchoVentana float32, estrecha bool) float32 {
	if a.Menu == nil {
		return 0
	}
	ancho := a.Menu.MinSize().Width
	if estrecha && anchoVentana > 0 {
		ancho = min(ancho, max(anchoVentana-MargenCajonMovil, 0))
	}
	return ancho
}

// desplazamientoCajon es lo que le queda al cajón para su posición de reposo: cero si el
// menú no es cajón o ya está completamente abierto.
func (a *Armazon) desplazamientoCajon(cajon bool) float32 {
	if cajon {
		return a.entrada
	}
	return 0
}

// progresoCajon devuelve la fracción visible del cajón (`0..1`) para sincronizar la
// opacidad del velo y la atenuación de botones flotantes.
func (a *Armazon) progresoCajon() float32 {
	if !a.CajonAbierto && !a.arrastrandoCajon && !a.cerrandoCajon {
		return 0
	}
	t := a.Size()
	ancho := a.anchoCajon(t.Width, t.Width < Estrecho)
	if ancho <= 0 {
		if a.CajonAbierto {
			return 1
		}
		return 0
	}
	posX := min(max(a.entrada, -ancho), 0)
	return min(max((ancho+posX)/ancho, 0), 1)
}

func (a *Armazon) moverCajonEnVivo() {
	r := a.r
	if r == nil || a.Menu == nil {
		return
	}
	t := a.Size()
	estrecha := t.Width < Estrecho
	ancho := a.anchoCajon(t.Width, estrecha)
	if ancho <= 0 {
		return
	}
	posX := min(max(a.entrada, -ancho), 0)
	progreso := min(max((ancho+posX)/ancho, 0), 1)
	a.Menu.Move(fyne.NewPos(posX, 0))
	r.ponerBase(true)
	r.colocarAdornosCajon(posX, ancho, t.Height, progreso, true)
	a.refrescarFlotantes()
}

func (a *Armazon) refrescarFlotantes() {
	c := lienzoDe(a)
	if c == nil {
		return
	}
	refrescarFlotantesEn(c.Content())
}

func refrescarFlotantesEn(o fyne.CanvasObject) {
	switch v := o.(type) {
	case nil:
	case *BotonFlotante:
		v.Refresh()
	case *raizGestos:
		refrescarFlotantesEn(v.content)
	case *fyne.Container:
		for _, h := range v.Objects {
			refrescarFlotantesEn(h)
		}
	}
}

// animarHaciaAbierto desliza el cajón desde `desde` (`<= 0`) hasta `0`.
func (a *Armazon) animarHaciaAbierto(desde float32) {
	t := a.Size()
	ancho := a.anchoCajon(t.Width, t.Width < Estrecho)
	if a.Menu == nil || SinMovimiento || lienzoDe(a) == nil || ancho <= 0 || desde >= 0 {
		a.entrada = 0
		a.Refresh()
		a.refrescarFlotantes()
		return
	}
	a.entrada = desde
	a.Refresh()
	a.refrescarFlotantes()
	anim := fyne.NewAnimation(DuracionCajon, func(p float32) {
		if !a.CajonAbierto || a.arrastrandoCajon {
			return
		}
		if p >= 1 {
			a.entrada = 0
			a.moverCajonEnVivo()
			return
		}
		a.entrada = desde * (1 - p)
		a.moverCajonEnVivo()
	})
	anim.Curve = CurvaEstandar
	anim.Start()
}

// animarHaciaCerrado desliza el cajón desde `desde` (`<= 0`) hasta `-ancho` y lo oculta.
func (a *Armazon) animarHaciaCerrado(desde float32) {
	t := a.Size()
	ancho := a.anchoCajon(t.Width, t.Width < Estrecho)
	if a.Menu == nil || SinMovimiento || lienzoDe(a) == nil || ancho <= 0 || desde <= -ancho {
		a.cerrandoCajon = false
		a.entrada = 0
		a.Refresh()
		a.refrescarFlotantes()
		return
	}
	a.cerrandoCajon = true
	a.entrada = desde
	a.Refresh()
	a.refrescarFlotantes()
	anim := fyne.NewAnimation(DuracionCajon, func(p float32) {
		if a.CajonAbierto || a.arrastrandoCajon {
			return
		}
		if p >= 1 {
			a.cerrandoCajon = false
			a.entrada = 0
			a.Refresh()
			a.refrescarFlotantes()
			return
		}
		a.entrada = desde + (-ancho-desde)*p
		a.moverCajonEnVivo()
	})
	anim.Curve = CurvaEstandar
	anim.Start()
}

// arrastrarAperturaCajon sigue el dedo desde el borde izquierdo mientras abre el cajón.
func (a *Armazon) arrastrarAperturaCajon(dx float32) {
	t := a.Size()
	ancho := a.anchoCajon(t.Width, t.Width < Estrecho)
	if a.Menu == nil || ancho <= 0 {
		return
	}
	visibles := min(max(dx, 0), ancho)
	antes := a.arrastrandoCajon
	a.arrastrandoCajon = true
	a.cerrandoCajon = false
	a.entrada = visibles - ancho
	if !antes {
		a.Refresh()
		a.refrescarFlotantes()
		return
	}
	a.moverCajonEnVivo()
}

// arrastrarCierreCajon sigue el dedo hacia la izquierda mientras recoge el cajón abierto.
func (a *Armazon) arrastrarCierreCajon(dx float32) {
	t := a.Size()
	ancho := a.anchoCajon(t.Width, t.Width < Estrecho)
	if a.Menu == nil || ancho <= 0 {
		return
	}
	desplaza := min(max(dx, -ancho), 0)
	antes := a.arrastrandoCajon
	a.arrastrandoCajon = true
	a.entrada = desplaza
	if !antes {
		a.Refresh()
		a.refrescarFlotantes()
		return
	}
	a.moverCajonEnVivo()
}

// cancelarArrastreCajon devuelve el cajón a su estado previo cuando el gesto no llega al
// umbral o se interrumpe.
func (a *Armazon) cancelarArrastreCajon() {
	if !a.arrastrandoCajon {
		return
	}
	desde := a.entrada
	a.arrastrandoCajon = false
	if a.CajonAbierto {
		a.animarHaciaAbierto(desde)
		return
	}
	a.animarHaciaCerrado(desde)
}

// CerrarCajon cierra el cajón con transición suave y devuelve el foco si procede.
func (a *Armazon) CerrarCajon() { a.cambiarCajon(false) }

// AbrirCajon lo abre, con su entrada y su teclado: es el botón de menú que la app lleva
// fuera de la barra móvil del armazón (el del carril de iconos del ERP o Rooms).
func (a *Armazon) AbrirCajon() { a.cambiarCajon(true) }

// cerrarDesdeMando cierra el cajón respetando el mando de la barra móvil propia de la
// app cuando existe (Mail, Notepads, Calendar, Contacts), para mantener sincronizado su
// estado interno.
func (a *Armazon) cerrarDesdeMando() {
	if !a.CajonAbierto {
		if a.arrastrandoCajon {
			a.cancelarArrastreCajon()
		}
		return
	}
	if a.BarraMovil != nil {
		if b, ok := a.BarraMovil.(*BarraMovil); ok && b.CajonAbierto && b.AlMenu != nil {
			b.AlMenu()
			if !a.CajonAbierto {
				if a.arrastrandoCajon {
					desde := a.entrada
					a.arrastrandoCajon = false
					a.animarHaciaCerrado(desde)
				}
				if c := lienzoDe(a); c != nil && cajones.de(c) == a {
					a.tecladoCajon(false)
				}
				if a.AlCajon != nil {
					a.AlCajon(false)
				}
				return
			}
		}
	}
	a.CerrarCajon()
}

func (a *Armazon) cambiarCajon(abierto bool) {
	if a.CajonAbierto == abierto && !a.arrastrandoCajon && !a.cerrandoCajon {
		return
	}
	t := a.Size()
	ancho := a.anchoCajon(t.Width, t.Width < Estrecho)
	desde := a.entrada
	enCurso := a.arrastrandoCajon || a.cerrandoCajon
	a.arrastrandoCajon = false
	a.cerrandoCajon = false
	a.CajonAbierto = abierto
	if a.AlCajon != nil {
		a.AlCajon(abierto)
	}
	if abierto {
		if !enCurso {
			desde = -ancho
		}
		a.animarHaciaAbierto(desde)
		if c := lienzoDe(a); c != nil && cajones.de(c) != a {
			a.tecladoCajon(true)
		}
		return
	}
	if !enCurso {
		desde = 0
	}
	a.animarHaciaCerrado(desde)
	if c := lienzoDe(a); c != nil && cajones.de(c) == a {
		a.tecladoCajon(false)
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
	if a.AlVolver != nil {
		b.AlVolver = func() {
			a.DetalleAbierto = false
			if a.AlVolver != nil {
				a.AlVolver()
			}
			a.Refresh()
		}
	} else {
		b.AlVolver = nil
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
	sombra := NuevaSombra(tema.SombraXL, 0).SinInterior()
	sombra.Objeto().Hide()
	borde := canvas.NewRectangle(color.NRGBA{})
	borde.Hidden = true
	r := &armazonRenderer{
		a:     a,
		fondo: NuevoFondoSuite(v),
		// El velo del cajón SE PULSA y lo cierra, como el `onClick` del de AppShell.
		// Era un rectángulo y el toque lo atravesaba hasta el contenido de debajo.
		velo:        NuevoVelo(true, a.cerrarDesdeMando),
		sombraCajon: sombra,
		base:        canvas.NewRectangle(color.NRGBA{}),
		bordeCajon:  borde,
		marco:       nuevoMarco(),
	}
	a.r = r
	if a.Panel != nil && a.Contenido != nil {
		r.separador = a.separadorPanel()
	}
	r.rehacer()
	return r
}

type armazonRenderer struct {
	a           *Armazon
	fondo       *FondoSuite
	velo        *Velo
	sombraCajon *Sombra
	// base es el suelo OPACO del cajón. El carril del traslúcido deja ver un punto de lo
	// que tiene debajo, que en escritorio es el fondo ambiental y en el cajón, el
	// contenido: el texto se colaba entre los rótulos del menú. La web le pone el mismo
	// color sobre `neutral-50` (`nav.bg-sidebar` por debajo de 768, appearance.css).
	base       *canvas.Rectangle
	bordeCajon *canvas.Rectangle
	// marco es la forma «Marco»: el color continuo detrás del menú y la hoja en la que
	// van el panel y el contenido (ver marco.go). En Carril no se ve.
	marco     *marco
	separador *separadorPanel
	objetos   []fyne.CanvasObject
}

// rehacer compone el árbol visual en orden de profundidad: marco y hoja abajo, contenido
// en medio, y el cajón con su velo, sombra y filete por encima.
func (r *armazonRenderer) rehacer() {
	a := r.a
	r.objetos = append([]fyne.CanvasObject{r.fondo}, r.marco.detras()...)
	if barra := a.barraMovil(); barra != nil {
		r.objetos = append(r.objetos, barra)
	}
	for _, o := range []fyne.CanvasObject{a.Panel, a.Contenido} {
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
		r.objetos = append(r.objetos, r.velo, r.sombraCajon.Objeto(), r.base, a.Menu, r.bordeCajon)
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

// Layout reparte la ventana. Ancha: menú, panel y contenido en fila. Estrecha: la barra
// móvil va arriba a todo el ancho, panel y contenido se turnan debajo, y el menú sale
// como cajón elevado sobre su velo.
func (r *armazonRenderer) Layout(t fyne.Size) {
	defer func() { sincronizarExclusionMenuMovil(lienzoDe(r.a)) }()
	a := r.a
	defer a.enlazarCajonMovil()
	r.fondo.Resize(t)
	estrecha := t.Width < Estrecho
	// En Marco el panel y el contenido van en la HOJA, que se separa del borde derecho y
	// del de abajo (el `margin` de `.sytue-sheet`) y, sin menú a su izquierda, también de ese.
	// En estrecho llega al borde y conserva las cuatro esquinas.
	separa := a.separacionHoja(estrecha)

	esCajon := estrecha || a.Carril != nil
	// Al pasar a ancho el cajón se cierra solo, que es lo que hace `useNavDrawer`: si
	// no, al volver a estrecho aparecía abierto sin que nadie lo hubiera pedido.
	if !esCajon && (a.CajonAbierto || a.arrastrandoCajon || a.cerrandoCajon) {
		estabaAbierto := a.CajonAbierto
		a.CajonAbierto = false
		a.arrastrandoCajon = false
		a.cerrandoCajon = false
		a.entrada = 0
		if estabaAbierto && a.AlCajon != nil {
			a.AlCajon(false)
		}
	}
	arriba := float32(0)
	if bm := a.barraMovil(); bm != nil {
		if b, es := bm.(*BarraMovil); es {
			b.ConLista = a.Panel != nil
			b.listaVisible = a.Panel != nil && !estrecha
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

// Las barras que configura cada app también pueden cambiar CajonAbierto y
// refrescar directamente. Comparten el mismo Escape, foco inicial, retorno y
// atenuación de botones flotantes.
func (a *Armazon) enlazarCajonMovil() {
	c := lienzoDe(a)
	if c == nil {
		return
	}
	esCajon := a.Size().Width < Estrecho || a.Carril != nil
	registrado := cajones.de(c) == a
	if esCajon && a.CajonAbierto && !registrado {
		a.tecladoCajon(true)
		a.refrescarFlotantes()
	} else if registrado && (!a.CajonAbierto || !esCajon) {
		a.tecladoCajon(false)
		a.refrescarFlotantes()
	}
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
		ancho = AcotarAnchoPanel(ancho, a.anchoHoja())
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
	ancho := a.anchoCajon(t.Width, estrecha)
	conCajon := esCajon && (a.CajonAbierto || a.arrastrandoCajon || a.cerrandoCajon)
	visible := !esCajon || conCajon
	esconder(a.Menu, !visible)
	esconder(r.velo, !conCajon)
	// El cajón y su velo arrancan ARRIBA, justo bajo la barra de la suite, y tapan
	// la barra móvil, como en AppShell.tsx (`BAJO_LA_TOPBAR`): con desfase la barra
	// quedaría viva y sin velo entre la de la suite y el cajón.
	posX := float32(0)
	if visible {
		if conCajon {
			posX = min(max(a.desplazamientoCajon(true), -ancho), 0)
		}
		a.Menu.Move(fyne.NewPos(posX, 0))
		a.Menu.Resize(fyne.NewSize(ancho, t.Height))
	}
	r.ponerBase(conCajon)
	progreso := float32(0)
	if conCajon && ancho > 0 {
		progreso = min(max((ancho+posX)/ancho, 0), 1)
	}
	r.colocarAdornosCajon(posX, ancho, t.Height, progreso, conCajon)
	if !esCajon {
		return ancho
	}
	// El cajón tapa el contenido, pero el contenido sigue ocupando su sitio: al
	// cerrarlo no se recoloca nada.
	r.velo.Move(fyne.NewPos(0, 0))
	r.velo.Resize(t)
	return x
}

func (r *armazonRenderer) colocarAdornosCajon(posX, ancho, alto, progreso float32, conCajon bool) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	activo := conCajon && progreso > 0
	if activo {
		r.sombraCajon.Objeto().Show()
		r.sombraCajon.Ajustar(fyne.NewPos(posX, 0), fyne.NewSize(ancho, alto), v)
	} else {
		r.sombraCajon.Objeto().Hide()
	}
	conBorde := activo && ancho >= tema.Borde
	esconder(r.bordeCajon, !conBorde)
	if conBorde {
		r.bordeCajon.FillColor = BordeSobre("sidebar-border", TokenCarril(), v)
		r.bordeCajon.Move(fyne.NewPos(posX+ancho-tema.Borde, 0))
		r.bordeCajon.Resize(fyne.NewSize(tema.Borde, alto))
		r.bordeCajon.Refresh()
	}
	if conCajon {
		r.velo.PonerOpacidad(progreso)
	}
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
