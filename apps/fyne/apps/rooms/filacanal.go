// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// FilaCanal es `ChannelRow.tsx`: un canal o un directo en el carril.
//
// No es `ui.FilaLateral`, y la diferencia se ve a la primera: aquella es el destino de
// navegación de las seis apps; esta lleva el no leído en NEGRITA —el contador solo no
// se ve al recorrer una lista larga—, se atenúa entera al silenciarla, saca la estrella
// de favorito al pasar por encima y deja su pastilla FUERA del botón, pegada al borde
// con `pr-1`. Medido en `canal.ancho.json`: el botón de «#Aleatorio», con 10 sin leer,
// mide 206,5 de ancho, y el de «#equipo», sin ninguno, 227,5 (los 231 menos el `pr-1`).
//
//	fila       `relative flex items-center rounded-md pr-1`, 35 de alto
//	botón      `flex-1 gap-2 px-2.5 py-2 text-base`, `font-semibold` con no leídos
//	activa     `bg-sidebar-active` y la barra de 1,75 de `indicator` a la izquierda
//	pastilla   `h-[18px] min-w-[18px] px-1 text-xs font-bold`, `bg-primary-600`
//	estrella   `h-6 w-6 rounded`, glifo de 16: fija si es favorito, si no al pasar
//	menú       el «…», igual que la estrella, al pasar y siempre con el dedo
type FilaCanal struct {
	ui.Pulsable

	Canal Canal
	// Marca es lo que va en la caja de 21: el «#», el candado, el avatar o la «@».
	Marca fyne.CanvasObject
	// Activa es `aria-current="page"`.
	Activa bool

	AlFavorito func()
	// AlMenu abre el menú «…» de la fila colgando de su disparador; nil, no hay «…».
	AlMenu func(disparador fyne.CanvasObject)

	estrella *estrellaCanal
	menu     *estrellaCanal
	aro      *ui.Aro
	caja     *ui.Caja
	barra    *canvas.Rectangle
	rotulo   *canvas.Text
	pastilla *ui.Caja
	numero   *ui.TextoTabular
	vivo     *canvas.Circle
	fuera    *iconos.Dibujo
	objetos  []fyne.CanvasObject
}

var _ fyne.Widget = (*FilaCanal)(nil)

// Medidas de la fila de canal, en px de CSS. HOJA salvo el alto y los dos anchos de
// botón, que son MEDIDOS.
var (
	// MargenDerFila es el `pr-1` de la fila.
	MargenDerFila = tema.Espacio(1)
	// HuecoBotonFila es el `gap-2` del botón; su `px` es de la forma (ui.RellenoNav).
	HuecoBotonFila = tema.Espacio(2)
	// RellenoFilaCanal es su `py-2`.
	RellenoFilaCanal = tema.Espacio(2)
	// HuecoCola es el `gap-0.5` de la cola: pastilla, estrella y menú.
	HuecoCola = tema.Espacio(0.5)
	// AltoPastilla es el `h-[18px] min-w-[18px]` y CanalonPastilla su `px-1`.
	AltoPastilla    float32 = 18
	CanalonPastilla         = tema.Espacio(1)
	// LadoEstrella es el `h-6 w-6` de la estrella y GlifoEstrella su `size={16}`.
	LadoEstrella  = tema.Espacio(6)
	GlifoEstrella = iconos.TallaSM
	// PuntoVivo es el `h-1.5 w-1.5` de «hay una llamada». GlifoFuera es el edificio
	// de una conversación de otra organización, de 14.
	PuntoVivo  = tema.Espacio(1.5)
	GlifoFuera = iconos.TallaXS
)

// NuevaFilaCanal crea la fila. El nombre accesible es el del canal.
func NuevaFilaCanal(t *i18n.Translator, c Canal, marca fyne.CanvasObject, activa bool,
	alPulsar, alFavorito func(),
) *FilaCanal {
	f := &FilaCanal{}
	f.montar(t, c, marca, activa, alPulsar, alFavorito)
	f.ExtendBaseWidget(f)
	return f
}

func (f *FilaCanal) montar(t *i18n.Translator, c Canal, marca fyne.CanvasObject, activa bool, alPulsar, alFavorito func()) {
	f.Canal, f.Marca, f.Activa, f.AlFavorito = c, marca, activa, alFavorito
	f.Nombre, f.AlPulsar = c.Nombre, alPulsar
	f.estrella = nuevaEstrella(t, c.Favorito, f)
	f.menu = nuevoMenuFila(t, f)
}

// FilaCanalArrastrable es la fila que se ARRASTRA a otra sección del carril: el DnD de
// la web (`draggable`), que mueve el canal a esa categoría o a su sección por defecto.
// Es otro tipo y no un campo porque ser `fyne.Draggable` es estático: con el dedo, la
// fila no se arrastra —el HTML5 DnD de la web tampoco—, y así el carril se sigue
// desplazando al deslizar sobre ella.
type FilaCanalArrastrable struct {
	FilaCanal
	// AlArrastrar recibe el punto (absoluto) bajo el puntero y AlSoltar se llama al
	// acabar.
	AlArrastrar func(fyne.Position)
	AlSoltar    func()
}

var _ fyne.Draggable = (*FilaCanalArrastrable)(nil)

// NuevaFilaCanalArrastrable es NuevaFilaCanal que se deja arrastrar.
func NuevaFilaCanalArrastrable(t *i18n.Translator, c Canal, marca fyne.CanvasObject, activa bool,
	alPulsar, alFavorito func(),
) *FilaCanalArrastrable {
	f := &FilaCanalArrastrable{}
	f.montar(t, c, marca, activa, alPulsar, alFavorito)
	f.ExtendBaseWidget(f)
	return f
}

// Dragged sigue al puntero.
func (f *FilaCanalArrastrable) Dragged(e *fyne.DragEvent) {
	if f.AlArrastrar != nil {
		f.AlArrastrar(e.AbsolutePosition)
	}
}

// DragEnd suelta el canal donde esté el puntero.
func (f *FilaCanalArrastrable) DragEnd() {
	if f.AlSoltar != nil {
		f.AlSoltar()
	}
}

// sinLeer dice si la fila se lee como no leída: con algo pendiente y sin silenciar.
func (f *FilaCanal) sinLeer() bool { return f.Canal.SinLeer > 0 && !f.Canal.Silenciado }

// peso es el del nombre, como en ChannelRow.tsx: semi-negrita con algo sin leer y, si
// no, el de «estás aquí» de la forma en el canal activo.
func (f *FilaCanal) peso() ui.Peso {
	switch {
	case f.sinLeer():
		return ui.SemiNegrita
	case f.Activa:
		return ui.PesoNavActivo()
	}
	return ui.Medio
}

// encimaDe es el `group-hover` y `group-focus-within`: el puntero o el foco en la fila
// o en uno de sus botones.
func (f *FilaCanal) encimaDe() bool {
	return f.Encima() || f.Enfocado() || f.estrella.Encima() || f.menu.Encima()
}

// conEstrella dice si la estrella está a la vista: fija si es favorito, y al pasar por
// encima (o con el foco dentro) si no.
func (f *FilaCanal) conEstrella() bool { return f.Canal.Favorito || f.encimaDe() }

// conMenu dice si el «…» está a la vista: al pasar por encima y, con el dedo, siempre
// (`pointer-coarse:flex`: sin él, en un móvil no había forma de llegar al menú).
func (f *FilaCanal) conMenu() bool {
	return f.AlMenu != nil && (f.encimaDe() || ui.EsTactil())
}

// conPastilla es el `group-hover:hidden` de la pastilla: al pasar por encima se van
// los números y salen las acciones, para no solaparse en la barra estrecha.
func (f *FilaCanal) conPastilla() bool { return f.Canal.SinLeer > 0 && !f.encimaDe() }

// MedidaPastilla es lo que ocupa la pastilla de no leídos. Es la única cuenta.
func MedidaPastilla(n int) fyne.Size {
	ancho := ui.MedirTabular(Contador(n), tema.TextoXS, ui.Negrita) + 2*CanalonPastilla
	return fyne.NewSize(max(ancho, AltoPastilla), AltoPastilla)
}

// anchoCola es lo que se lleva la cola a la derecha del botón.
func (f *FilaCanal) anchoCola() float32 {
	ancho, n := float32(0), 0
	if f.conPastilla() {
		ancho += MedidaPastilla(f.Canal.SinLeer).Width
		n++
	}
	if f.conEstrella() {
		ancho += LadoEstrella
		n++
	}
	if f.conMenu() {
		ancho += LadoEstrella
		n++
	}
	if n > 1 {
		ancho += float32(n-1) * HuecoCola
	}
	return ancho
}

// MedidaFilaCanal es lo que ocupa: 35 de alto (el icono de 21 manda sobre la caja de
// línea de 19,25). De ancho se estira a lo que le den.
func MedidaFilaCanal() fyne.Size {
	return fyne.NewSize(0, CajaIconoFila+2*RellenoFilaCanal)
}

// AnchoBotonFila es lo que mide el botón de la fila dentro de un ancho dado: lo que
// sobra tras el `pr-1` y la cola. Es lo que midió el inspector: 206,5 y 227,5.
func (f *FilaCanal) AnchoBotonFila(ancho float32) float32 {
	return max(ancho-MargenDerFila-f.anchoCola(), 0)
}

// CreateRenderer compone la fila.
func (f *FilaCanal) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.aro = ui.NuevoAroCon("sidebar-focus", tema.Forma("radius-nav"), ui.SinHueco, v)
	f.caja = ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.Forma("radius-nav"))
	f.rotulo = ui.Texto(f.Canal.Nombre, tema.TextoBase, f.peso(), color.NRGBA{})
	f.objetos = append(f.aro.Objetos(), f.caja.Objetos()...)
	if f.Activa {
		f.barra = canvas.NewRectangle(color.NRGBA{})
		f.barra.CornerRadius = tema.Espacio(0.5) / 2
		f.objetos = append(f.objetos, f.barra)
	}
	if f.Marca != nil {
		f.objetos = append(f.objetos, f.Marca)
	}
	f.objetos = append(f.objetos, f.rotulo)
	if f.Canal.Externo {
		f.fuera = iconos.Trazo(iconos.Edificio, GlifoFuera, "sidebar-fg-muted", v)
		f.objetos = append(f.objetos, f.fuera.Objeto())
	}
	if f.Canal.EnLlamada {
		f.vivo = canvas.NewCircle(color.NRGBA{})
		f.objetos = append(f.objetos, f.vivo)
	}
	if f.Canal.SinLeer > 0 {
		f.pastilla = ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, AltoPastilla/2)
		f.numero = ui.NuevoTextoTabular(Contador(f.Canal.SinLeer), tema.TextoXS, ui.Negrita, color.NRGBA{})
		f.objetos = append(f.objetos, f.pastilla.Objetos()...)
		f.objetos = append(f.objetos, f.numero.Objetos()...)
	}
	f.objetos = append(f.objetos, f.estrella, f.menu)
	r := &filaCanalRenderer{f: f}
	r.Refresh()
	return r
}

type filaCanalRenderer struct{ f *FilaCanal }

func (r *filaCanalRenderer) MinSize() fyne.Size           { return MedidaFilaCanal() }
func (r *filaCanalRenderer) Objects() []fyne.CanvasObject { return r.f.objetos }
func (r *filaCanalRenderer) Destroy()                     {}

// Layout coloca el botón a la izquierda y la cola a la derecha, todo centrado en
// vertical (`items-center`).
func (r *filaCanalRenderer) Layout(t fyne.Size) {
	f := r.f
	f.aro.Colocar(t)
	f.caja.Redimensionar(t)
	if f.barra != nil {
		// `before:top-1.5 before:bottom-1.5 before:left-0
		// before:w-[min(2px,var(--sytue-nav-indicator))]`: de la forma. En Marco no la
		// hay —la fila activa ya es una píldora—.
		m, ancho := tema.Espacio(1.5), min(2, ui.AnchoSenal())
		f.barra.Move(fyne.NewPos(0, m))
		f.barra.Resize(fyne.NewSize(ancho, max(t.Height-2*m, 0)))
		f.barra.Hidden = ancho <= 0
	}

	// La cola, desde la derecha: `pr-1`, y luego menú, estrella y pastilla.
	der := t.Width - MargenDerFila
	for _, b := range []struct {
		boton *estrellaCanal
		ver   bool
	}{{f.menu, f.conMenu()}, {f.estrella, f.conEstrella()}} {
		b.boton.Hidden = !b.ver
		if b.ver {
			der -= LadoEstrella
			b.boton.Move(fyne.NewPos(der, (t.Height-LadoEstrella)/2))
			b.boton.Resize(fyne.NewSize(LadoEstrella, LadoEstrella))
			der -= HuecoCola
		}
	}
	if f.pastilla != nil {
		ver := f.conPastilla()
		for _, o := range append(f.pastilla.Objetos(), f.numero.Objetos()...) {
			if ver {
				o.Show()
			} else {
				o.Hide()
			}
		}
		if ver {
			m := MedidaPastilla(f.Canal.SinLeer)
			der -= m.Width
			ui.MoverCaja(f.pastilla, fyne.NewPos(der, (t.Height-m.Height)/2), m)
			ancho := f.numero.Ancho()
			f.numero.Colocar(
				der+(m.Width-ancho)/2, (t.Height-tema.TextoXS.Tam)/2, tema.Escalon{Tam: tema.TextoXS.Tam, Linea: tema.TextoXS.Tam})
		}
	}

	// El botón: `px-2.5`, la marca en su caja de 21, el rótulo que se trunca y, detrás,
	// el edificio y el punto vivo, que van DENTRO del botón para leerse con el nombre.
	// `px-[var(--sytue-nav-pad)]`: el mismo relleno, de la forma, que un ítem del menú.
	fin := f.AnchoBotonFila(t.Width) - ui.RellenoNav()
	x := ui.RellenoNav()
	if f.Marca != nil {
		// `grid place-items-center`: la marca va centrada en su caja con SU tamaño —el
		// candado de 14, el avatar `xs` de 24, que asoma 1,5 por cada lado igual que en
		// la web—.
		m := f.Marca.MinSize()
		f.Marca.Move(fyne.NewPos(x+(CajaIconoFila-m.Width)/2, (t.Height-m.Height)/2))
		f.Marca.Resize(m)
		x += CajaIconoFila + HuecoBotonFila
	}
	if f.vivo != nil {
		fin -= PuntoVivo
		f.vivo.Move(fyne.NewPos(fin, (t.Height-PuntoVivo)/2))
		f.vivo.Resize(fyne.NewSize(PuntoVivo, PuntoVivo))
		fin -= HuecoBotonFila
	}
	if f.fuera != nil {
		fin -= GlifoFuera
		f.fuera.Colocar(fin, (t.Height-GlifoFuera)/2, GlifoFuera)
		fin -= HuecoBotonFila
	}
	ui.Truncar(f.rotulo, f.Canal.Nombre, tema.TextoBase, f.peso(), max(fin-x, 0))
	ui.ColocarTexto(f.rotulo, tema.TextoBase, x, (t.Height-tema.TextoBase.Linea)/2)
}

// Refresh repinta según el estado. Silenciada, TODO baja al 60 % (`opacity-60`).
func (r *filaCanalRenderer) Refresh() {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// `rounded-nav`: el radio de la fila es de la forma, que puede cambiar con la app
	// abierta.
	f.caja.CambiarRadio(tema.Forma("radius-nav"))
	f.aro.CambiarRadio(tema.Forma("radius-nav"))
	velo := float32(1)
	if f.Canal.Silenciado {
		velo = 0.6
	}
	con := func(token string, parte float32) color.NRGBA { return atenuado(token, v, parte*velo) }

	fondo := color.NRGBA{}
	switch {
	case f.Activa:
		fondo = con("sidebar-active-bg", 1)
	case f.Encima() || f.estrella.Encima() || f.menu.Encima():
		fondo = con("sidebar-hover-bg", 1)
	}
	f.caja.Pintar(fondo, color.NRGBA{})
	f.aro.Ver(f.Enfocado())
	f.aro.Pintar(v)
	if f.barra != nil {
		f.barra.FillColor = con("sidebar-indicator", 1)
		f.barra.Refresh()
	}
	tinta := "sidebar-fg"
	if f.Activa || f.sinLeer() {
		tinta = "sidebar-active-fg"
	}
	f.rotulo.FontSource = f.peso().Cara()
	f.rotulo.Color = con(tinta, 1)
	f.rotulo.Refresh()
	if f.vivo != nil {
		f.vivo.FillColor = con("success", 1)
		f.vivo.Refresh()
	}
	if f.fuera != nil {
		f.fuera.TenirCon(con("sidebar-fg-muted", 1))
	}
	if f.pastilla != nil {
		// El `<Badge count>` dentro del carril: el primario de allí y su rótulo.
		relleno, cifra := con("sidebar-primary", 1), con("sidebar-on-primary", 1)
		if f.Canal.Silenciado {
			relleno, cifra = con("sidebar-active-bg", 0.5), con("sidebar-fg-muted", 1)
		}
		f.pastilla.Pintar(relleno, color.NRGBA{})
		f.numero.Tenir(cifra)
	}
	f.estrella.velo, f.menu.velo = velo, velo
	f.estrella.Refresh()
	f.menu.Refresh()
	r.Layout(f.Size())
}

// ---- La estrella de favorito ----

// estrellaCanal es el botón de la estrella de la fila: `SIDEBAR_TRIGGER` —`h-6 w-6
// rounded text-sidebar-fg-muted hover:bg-sidebar-active/60 hover:text-sidebar-fg`— y
// `text-indicator` si ya es favorito. Se anuncia con PALABRAS («Añadir a favoritos» /
// «Quitar de favoritos»), que es su `aria-label`: `aria-pressed` solo no dice cuál.
type estrellaCanal struct {
	ui.Pulsable

	// menu lo hace el «…» de la fila: el mismo botón con el glifo de `MoreGlyph`.
	menu     bool
	favorito bool
	fila     *FilaCanal
	velo     float32

	caja  *ui.Caja
	aro   *ui.Aro
	glifo *iconos.Dibujo
}

func nuevaEstrella(t *i18n.Translator, favorito bool, fila *FilaCanal) *estrellaCanal {
	e := &estrellaCanal{favorito: favorito, fila: fila, velo: 1}
	e.Nombre = t.T("channel.favourite", nil)
	if favorito {
		e.Nombre = t.T("channel.unfavourite", nil)
	}
	e.AlPulsar = func() {
		if fila.AlFavorito != nil {
			fila.AlFavorito()
		}
	}
	e.ExtendBaseWidget(e)
	return e
}

// nuevoMenuFila es el «…» de la fila, que abre el menú del canal. Se anuncia con
// «Acciones de #canal» (`channel.actions`).
func nuevoMenuFila(t *i18n.Translator, fila *FilaCanal) *estrellaCanal {
	e := &estrellaCanal{menu: true, fila: fila, velo: 1}
	e.Nombre = t.T("channel.actions", i18n.Vars{"name": fila.Canal.Nombre})
	e.AlPulsar = func() {
		if fila.AlMenu != nil {
			fila.AlMenu(e)
		}
	}
	e.ExtendBaseWidget(e)
	return e
}

// MouseIn y MouseOut repintan también la FILA: pasar de la fila a la estrella le quita
// el `hover` a la fila, y sin esto la estrella se escondería justo al llegarle el ratón.
func (e *estrellaCanal) MouseIn(ev *desktop.MouseEvent) {
	e.Pulsable.MouseIn(ev)
	e.fila.Refresh()
}

func (e *estrellaCanal) MouseOut() {
	e.Pulsable.MouseOut()
	e.fila.Refresh()
}

func (e *estrellaCanal) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	e.caja = ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioSM)
	e.aro = ui.NuevoAroCon("chrome-focus", tema.RadioSM, ui.SinHueco, v)
	switch {
	case e.menu:
		e.glifo = dibujo(glifoMas, GlifoEstrella, "sidebar-fg-muted", v)
	case e.favorito:
		e.glifo = iconos.TrazoDeRutaRelleno(rutaEstrella, GlifoEstrella, tema.Color("indicator", v))
	default:
		e.glifo = iconos.TrazoDeRuta(rutaEstrella, GlifoEstrella, tema.Color("sidebar-fg-muted", v))
	}
	objs := append(e.aro.Objetos(), e.caja.Objetos()...)
	r := &estrellaRenderer{e: e, objetos: append(objs, e.glifo.Objeto())}
	r.Refresh()
	return r
}

type estrellaRenderer struct {
	e       *estrellaCanal
	objetos []fyne.CanvasObject
}

func (r *estrellaRenderer) MinSize() fyne.Size           { return fyne.NewSize(LadoEstrella, LadoEstrella) }
func (r *estrellaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *estrellaRenderer) Destroy()                     {}

func (r *estrellaRenderer) Layout(t fyne.Size) {
	r.e.aro.Colocar(t)
	r.e.caja.Redimensionar(t)
	r.e.glifo.Colocar((t.Width-GlifoEstrella)/2, (t.Height-GlifoEstrella)/2, GlifoEstrella)
}

func (r *estrellaRenderer) Refresh() {
	e := r.e
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := color.NRGBA{}
	tinta := "sidebar-fg-muted"
	if e.favorito {
		tinta = "indicator"
	}
	if e.Encima() {
		fondo = atenuado("sidebar-active-bg", v, 0.6*e.velo)
		if !e.favorito {
			tinta = "sidebar-fg"
		}
	}
	e.caja.Pintar(fondo, color.NRGBA{})
	e.aro.Ver(e.Enfocado())
	e.aro.Pintar(v)
	e.glifo.TenirCon(atenuado(tinta, v, e.velo))
	r.Layout(e.Size())
}
