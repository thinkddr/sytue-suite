// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Las medidas de Dropdown.tsx, en px de CSS.
const (
	// MinAnchoMenu es `min-w-[220px]`.
	MinAnchoMenu float32 = 220
	// MargenVentana es `MARGEN_VENTANA`: el aire mínimo entre el panel y el borde de
	// la ventana. Es también el `max-w-[calc(100vw-16px)]`, que son estos 8 por dos.
	MargenVentana float32 = 8
	// SeparacionMenu es el hueco entre el disparador y el panel (`d.bottom + 6`).
	SeparacionMenu float32 = 6
	// AltoMinimoMenu es `ALTO_MINIMO`: por debajo de esto un panel ya no se puede usar, y
	// antes de bajar de ahí tapa su disparador.
	AltoMinimoMenu float32 = 120
)

var (
	rellenoPanelMenu = tema.Espacio(1) // p-1 del panel: 3,5
	huecoElemento    = tema.Espacio(2) // gap-2 entre el icono y el rótulo: 7
	// Las dos densidades de MenuItem. `md` es el cromo de la aplicación; `sm`, el
	// cromo denso de los editores.
	margenElemento = [...]fyne.Size{
		// px-3 py-2 text-base
		{Width: tema.Espacio(3), Height: tema.Espacio(2)},
		// px-2.5 py-1.5 text-sm
		{Width: tema.Espacio(2.5), Height: tema.Espacio(1.5)},
	}
	escalonElemento = [...]tema.Escalon{TextoBaseMenu, tema.TextoSM}
)

// TextoBaseMenu es el `text-base` de un elemento de menú. Con nombre propio porque es
// el escalón del que sale su alto de 33,25, y así el guardián lo puede nombrar.
var TextoBaseMenu = tema.TextoBase

// Menu es el panel de Dropdown.tsx: la lista de acciones que cuelga de un disparador.
//
// # Por qué esta pieza es la que más se echó de menos
//
// La web la usa en ONCE aplicaciones y el port anterior no la tenía. No es que faltara
// un adorno: sin menú, los controles que van dentro se quedaban fuera, y eso se mide —
// la cabecera del panel de Contactos daba 42 px donde la web da 76, porque las acciones
// que el menú recoge no estaban.
//
// # El panel va a una capa, y no es un adorno
//
// En la web va a un portal porque cualquier ancestro con `transform`, `filter`,
// `opacity` o `will-change` le crea contexto de apilamiento y las filas siguientes se
// pintan encima —medido en producción sobre Drive, con el panel opaco y `z-index:
// 1000`—. En Fyne pasa lo mismo por otro camino: un hijo se pinta dentro de su padre y
// no puede salirse de él. Por eso va a `Overlays()`, que es el equivalente del portal.
//
// Y por eso también sale gratis lo demás: un menú abierto DENTRO de un diálogo se apila
// encima de él, así que Escape se lleva primero el menú, que es lo que espera cualquiera.
type Menu struct {
	widget.BaseWidget
	// Elementos son *ElementoMenu y *SeparadorMenu, en orden.
	Elementos []fyne.CanvasObject
	// AlInicio es `align='start'`: el panel se alinea con el borde IZQUIERDO del
	// disparador. El valor cero es `align='end'`, que es el de la web.
	AlInicio bool
	// Arriba es `side='top'`: desplegar hacia arriba aunque abajo quepa. El valor
	// cero es `bottom`; y si abajo no cabe y arriba sí, se vuelca solo.
	Arriba bool
	// Rotulo es el nombre accesible del menú (`aria-label`).
	Rotulo string
	// Ancho fija el ancho del panel: los menús que la web escribe con uno propio (el
	// «…» de un mensaje de Sytue Rooms mide 232, `MENU_W`). Cero: el natural, con su
	// mínimo de 220.
	Ancho float32
	// Ancla sitúa el panel respecto a otro objeto (el contenedor del botón Archivo).
	// El disparador sigue siendo quien lo abre y recupera el foco con Escape y Tab.
	// Nil: la geometría también se toma del disparador.
	Ancla fyne.CanvasObject
	// Separacion cambia el hueco vertical cuando el menú propio usa otro CSS
	// (Archivo usa mt-1). Cero conserva los seis píxeles de Dropdown.tsx.
	Separacion float32
	// ContenidoAjustable entrega todo el alto visible a un único contenido que
	// gestiona su propio desplazamiento (el selector de emoji). Los menús de
	// acciones conservan sus filas naturales y su desplazamiento habitual.
	ContenidoAjustable bool

	disparador fyne.CanvasObject
	ventana    fyne.Size
	previo     fyne.Focusable
	capa       *capa
	// desplazado es lo que el contenido ha subido dentro del panel cuando no cabe en su
	// hueco: el `scrollTop` del panel de la web.
	desplazado float32
}

var (
	_ fyne.Widget     = (*Menu)(nil)
	_ fyne.Tappable   = (*Menu)(nil)
	_ fyne.Scrollable = (*Menu)(nil)
	_ Descartable     = (*Menu)(nil)
	_ fyne.Accessible = (*Menu)(nil)
)

// NuevoMenu crea el menú con sus elementos.
func NuevoMenu(elementos ...fyne.CanvasObject) *Menu {
	m := &Menu{Elementos: elementos}
	m.ExtendBaseWidget(m)
	for _, e := range elementos {
		if it, es := e.(*ElementoMenu); es {
			it.menu = m
		}
	}
	return m
}

// AccessibilityLabel es el `aria-label` del menú.
func (m *Menu) AccessibilityLabel() string { return m.Rotulo }

// AccessibilityRole: un menú es un contenedor de acciones.
func (m *Menu) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// MedidaMenu es lo que va a ocupar el panel. Como en el resto del paquete, es la ÚNICA
// cuenta: el renderer la llama y los guardianes comparan contra ella y contra los
// píxeles.
//
// `ventana` de cero es «sin ventana todavía»: no se aplica el `max-w`.
func MedidaMenu(m *Menu, ventana fyne.Size) fyne.Size {
	var ancho, alto float32
	hoja := enHoja(ventana)
	for _, e := range m.Elementos {
		s := e.MinSize()
		ancho = max(ancho, s.Width)
		alto += altoElementoEnHoja(e, s.Height, hoja)
	}
	if hoja {
		// La hoja del teléfono: a todo el ancho, con el `p-2` y la zona segura.
		return fyne.NewSize(ventana.Width, alto+2*rellenoHojaMenu+tema.Borde)
	}
	marco := 2 * (rellenoPanelMenu + tema.Borde)
	ancho = max(ancho+marco, MinAnchoMenu)
	if m.Ancho > 0 {
		ancho = m.Ancho // el ancho fijo de un menú propio (`MENU_W` de Sytue Rooms)
	}
	if ventana.Width > 0 {
		// `max-w-[calc(100vw-16px)]`. Es condición del clamp: sin él, apartar del
		// borde izquierdo un panel más ancho que la ventana lo sacaría por el derecho.
		ancho = min(ancho, ventana.Width-2*MargenVentana)
	}
	return fyne.NewSize(max(ancho, 0), alto+marco)
}

// rellenoHojaMenu es el `p-2` del menú como hoja del teléfono (`HOJA_MENU` de Dropdown.tsx).
var rellenoHojaMenu = tema.Espacio(2)

// FraccionAltoHojaMenu es su `max-h-[80dvh]`.
const FraccionAltoHojaMenu float32 = 0.8

// altoElementoEnHoja es el `[&_[role=menuitem]]:min-h-11` de la hoja: cada acción, 44
// (los separadores, lo suyo). Fuera de la hoja, lo que mida.
func altoElementoEnHoja(e fyne.CanvasObject, alto float32, hoja bool) float32 {
	if _, es := e.(*ElementoMenu); es && hoja {
		return max(alto, SueloTactilBarra)
	}
	return alto
}

// EncajarMenu es `encajarPanel` de Dropdown.tsx, entera y sin lienzo de por medio para
// que se pueda probar con números: dónde va el panel y cuánto puede medir de alto.
//
// EL PANEL NO TAPA SU DISPARADOR. Se queda en el hueco de un lado —abajo, salvo que no
// quepa y arriba haya más sitio— y lo que no entra ahí se desplaza POR DENTRO. Antes el
// alto máximo era el de la ventana y el panel se subía para caber: el menú de cuenta, al
// crecer, se plantaba encima de su propio botón y ya no se podía cerrar pulsando donde
// se abrió (web, 2026-10-05).
//
//   - **Arriba si no cabe abajo**, sin que el llamante lo pida: un menú cerca del pie
//     de la ventana —el de «más opciones» de una videollamada— se desplegaba hacia
//     abajo y quedaba medio fuera.
//   - **El hueco manda sobre el alto**, con un suelo de AltoMinimoMenu: con menos que
//     eso en el lado elegido no hay hueco que respetar, y el panel se queda dentro de la
//     ventana, que es lo que resuelve el tope.
//   - **El tope**, que aparta del borde 8 px por los cuatro lados. Hace falta porque
//     un menú ancho —el centro de notificaciones mide 330— colgado de un disparador
//     que no está en el borde arranca en coordenada negativa, y lo que se recorta NO
//     se alcanza desplazando.
//
// `panel` es la caja tal como se pintaría SIN el tope de alto.
func EncajarMenu(disparador fyne.Position, tamDisparador, panel, ventana fyne.Size,
	alInicio, arriba bool,
) (pos fyne.Position, altoMaximo float32) {
	return encajarMenu(disparador, tamDisparador, panel, ventana, alInicio, arriba, SeparacionMenu)
}

func encajarMenu(disparador fyne.Position, tamDisparador, panel, ventana fyne.Size,
	alInicio, arriba bool, separacion float32,
) (pos fyne.Position, altoMaximo float32) {
	sitioAbajo := ventana.Height - (disparador.Y + tamDisparador.Height) - separacion - MargenVentana
	sitioArriba := disparador.Y - separacion - MargenVentana
	sube := arriba || (panel.Height > sitioAbajo && sitioArriba > sitioAbajo)

	altoMaximo = max(sitioAbajo, AltoMinimoMenu)
	if sube {
		altoMaximo = max(sitioArriba, AltoMinimoMenu)
	}
	// En apaisado o con teclado, ni siquiera el suelo de 120 puede caber.
	// La zona segura manda también sobre ese mínimo.
	altoMaximo = min(altoMaximo, max(ventana.Height-2*MargenVentana, 0))
	alto := min(panel.Height, altoMaximo)
	y := disparador.Y + tamDisparador.Height + separacion
	if sube {
		y = disparador.Y - separacion - alto
	}
	x := disparador.X + tamDisparador.Width - panel.Width // align='end'
	if alInicio {
		x = disparador.X
	}
	tope := func(v, tope float32) float32 {
		return max(MargenVentana, min(v, tope-MargenVentana))
	}
	return fyne.NewPos(tope(x, ventana.Width-panel.Width), tope(y, ventana.Height-alto)), altoMaximo
}

// Mostrar despliega el menú colgando de `disparador`, que tiene que estar ya montado.
//
// El fondo de la capa es un velo TRANSPARENTE: no se ve y recoge el clic de fuera, que
// es lo que en la web hace el `mousedown` en `document`.
func (m *Menu) Mostrar(disparador fyne.CanvasObject) {
	c := lienzoDe(disparador)
	if c == nil || m.capa != nil {
		return
	}
	m.disparador = disparador
	// En el teléfono el menú es una HOJA inferior, como los diálogos (Dropdown.tsx,
	// `useEnHoja`): el velo de los diálogos detrás, que también cierra al pulsarlo.
	hoja := enHoja(c.Size())
	velo := nuevoVeloCruzado(m.Descartar)
	if hoja {
		velo = NuevoVelo(true, m.Descartar)
	}
	m.capa = nuevaCapa(velo, m, func(ventana fyne.Size) (fyne.Position, fyne.Size) {
		m.ventana = ventana
		tam := MedidaMenu(m, ventana)
		if enHoja(ventana) {
			visible := min(tam.Height, FraccionAltoHojaMenu*ventana.Height)
			m.desplazado = min(m.desplazado, max(tam.Height-visible, 0))
			return fyne.NewPos(0, ventana.Height-visible), fyne.NewSize(ventana.Width, visible)
		}
		ancla := disparador
		if m.Ancla != nil {
			ancla = m.Ancla
		}
		abs := fyne.CurrentApp().Driver().AbsolutePositionForObject(ancla)
		separacion := float32(SeparacionMenu)
		if m.Separacion > 0 {
			separacion = m.Separacion
		}
		// Lo que no cabe en el hueco se desplaza por dentro: el panel mide el hueco.
		pos, tope := encajarMenu(abs, ancla.Size(), tam, ventana, m.AlInicio, m.Arriba, separacion)
		visible := min(tam.Height, tope)
		m.desplazado = min(m.desplazado, max(tam.Height-visible, 0))
		return pos, fyne.NewSize(tam.Width, visible)
	})
	// `shadow-[var(--shadow-md)]` de Dropdown.tsx, con su `rounded-lg`. La hoja no lleva.
	if !hoja {
		m.capa.sombra = NuevaSombra(tema.SombraMD, tema.RadioLG)
	}
	m.capa.alDescartar = m.Descartar
	m.desplazado = 0
	m.previo = mostrar(c, m.capa)
	// `menuRef.current?.querySelector('[role="menuitem"]')?.focus()`.
	if e := m.elemento(0); e != nil {
		// `:focus-visible` hereda la modalidad: abierto con el ratón —nada tenía el foco,
		// que en Fyne solo llega por teclado o por código— la opción lo recibe sin fondo
		// ni anillo; con el teclado, con los dos. Medido en Chromium el 2026-10-05, y es
		// la misma regla que el aspa de un diálogo (`Boton.enfocarSinAro`).
		e.sinAro = m.previo == nil
		enfocar(c, e)
	}
	// `sytue-pop-in`: 150 ms desde `translateY(-4px) scale(.98)` y transparente.
	m.capa.entrar(c, DuracionPop, -DesplazamientoPop, EscalaEntrada, 0.5)
}

// Reemplazar cambia lo que lleva el menú, esté abierto o no, y lo vuelve a medir y a
// colocar. Es lo que hace la campana cuando llega un aviso con el panel desplegado:
// asignar `Elementos` y refrescar no bastaba, porque el renderer se quedaba con la
// lista de objetos y el tamaño del momento de abrir.
func (m *Menu) Reemplazar(elementos ...fyne.CanvasObject) {
	m.Elementos = elementos
	for _, e := range elementos {
		if it, es := e.(*ElementoMenu); es {
			it.menu = m
		}
	}
	m.Refresh()
	if m.capa != nil {
		m.capa.Refresh() // vuelve a pedir MedidaMenu y recoloca el panel
	}
}

// Abierto dice si el menú está desplegado.
//
// No vale `Visible()`: un widget que no se ha escondido nunca lo contesta `true`
// aunque no esté en ninguna pantalla, así que un disparador que lo usara para
// alternar se quedaría siempre en la rama de «ciérralo». Lo que de verdad dice si
// está puesto es su capa.
func (m *Menu) Abierto() bool { return m.capa != nil }

// Cerrar recoge el menú y devuelve el foco al disparador, como hace la web al cerrar
// con Escape.
func (m *Menu) Cerrar() {
	if m.capa == nil {
		return
	}
	ocultar(lienzoDe(m.capa), m.capa, m.previo)
	m.capa, m.previo = nil, nil
	m.disparador.Refresh() // el estado aria-expanded del disparador ya está cerrado
}

// Descartar es Escape y el clic fuera: un menú no pregunta, se cierra.
func (m *Menu) Descartar() { m.Cerrar() }

// cerrarConEscape es el Escape de Dropdown.tsx: cierra y lleva el foco al DISPARADOR
// (`triggerRef…querySelector('button,a,[tabindex]')?.focus()`), no a lo que lo tuviera
// antes de abrir.
func (m *Menu) cerrarConEscape() {
	if f, es := m.disparador.(fyne.Focusable); es && m.capa != nil {
		m.previo = f
	}
	m.Cerrar()
}

// salirConTab es el Tab que se va del menú por un extremo. En la web el panel es un
// portal al final del `<body>` y el foco sigue por el documento con el menú abierto; en
// Fyne una capa encima se queda el teclado entero, así que salir es cerrarlo. El foco va
// a lo siguiente (o lo anterior) al DISPARADOR, que es donde lo pone el patrón de menú de
// WAI-ARIA.
//
// ponytail: la web lo dejaba en lo que haya tras el portal —el principio del documento—
// con el menú aún abierto; aquí se cierra y sigue desde el disparador. Es la única lectura
// nativa de «el Tab sale del menú».
func (m *Menu) salirConTab(atras bool) {
	c := lienzoDe(m)
	disparador, _ := m.disparador.(fyne.Focusable)
	m.Cerrar()
	if c == nil || disparador == nil {
		return
	}
	enfocar(c, disparador)
	// Con el cajón abierto, el gestor de foco del fork recorre solo el cajón (FocusScope).
	if atras {
		c.FocusPrevious()
		return
	}
	c.FocusNext()
}

// conMayusculas dice si Mayúsculas está apretada: en Fyne el Tab que recibe un control
// no trae los modificadores, se preguntan al driver de escritorio.
func conMayusculas() bool {
	d, es := fyne.CurrentApp().Driver().(desktop.Driver)
	return es && d.CurrentKeyModifiers()&fyne.KeyModifierShift != 0
}

// Tapped se traga el clic que cae en el panel y no en un elemento, para que no lo
// recoja el velo de debajo y cierre el menú.
func (m *Menu) Tapped(*fyne.PointEvent) {}

// elemento devuelve el i-ésimo ELEMENTO pulsable, saltándose separadores e inertes,
// que es lo que hace `querySelectorAll('[role="menuitem"]:not([disabled])')`.
func (m *Menu) elemento(i int) *ElementoMenu {
	lista := m.pulsables()
	if i < 0 || i >= len(lista) {
		return nil
	}
	return lista[i]
}

func (m *Menu) pulsables() []*ElementoMenu {
	var out []*ElementoMenu
	for _, e := range m.Elementos {
		if it, es := e.(*ElementoMenu); es && !it.Inerte {
			out = append(out, it)
		}
	}
	return out
}

// mover es la navegación con flechas: circula, como en la web.
func (m *Menu) mover(desde *ElementoMenu, paso int) {
	lista := m.pulsables()
	if len(lista) == 0 {
		return
	}
	i := 0
	for j, e := range lista {
		if e == desde {
			i = j
			break
		}
	}
	siguiente := lista[((i+paso)%len(lista)+len(lista))%len(lista)]
	if c := lienzoDe(m); c != nil {
		enfocar(c, siguiente)
	}
	m.revelar(siguiente)
}

// Scrolled desplaza el contenido por dentro del panel, hasta el final: es su
// `overflow-y-auto`. Con el contenido entero a la vista no hace nada.
func (m *Menu) Scrolled(ev *fyne.ScrollEvent) { m.desplazar(m.desplazado - ev.Scrolled.DY) }

// desplazar pone el desplazamiento, acotado a lo que hay.
func (m *Menu) desplazar(a float32) {
	if m.ContenidoAjustable && len(m.Elementos) == 1 {
		return // el único desplazador está dentro del contenido, también con el dedo
	}
	a = max(min(a, MedidaMenu(m, m.ventana).Height-m.Size().Height), 0)
	if a != m.desplazado {
		m.desplazado = a
		m.Refresh()
	}
}

// revelar trae a la vista el elemento que acaba de recibir el foco, que es lo que hace
// el navegador con `focus()` dentro de un contenedor que se desplaza.
//
// ponytail: solo los elementos del menú, que es a lo que llevan las flechas. Un control
// suelto (el grupo de la apariencia, el selector de idioma) al que se llega con el
// tabulador no se revela; se alcanza con la rueda.
func (m *Menu) revelar(e *ElementoMenu) {
	dentro := tema.Borde + rellenoPanelMenu
	arriba, abajo := e.Position().Y, e.Position().Y+e.Size().Height
	switch {
	case arriba < dentro:
		m.desplazar(m.desplazado - (dentro - arriba))
	case abajo > m.Size().Height-dentro:
		m.desplazar(m.desplazado + abajo - (m.Size().Height - dentro))
	}
}

// CreateRenderer construye el panel: caja con borde y esquina de 8, y los elementos uno
// debajo de otro dentro del `p-1`.
func (m *Menu) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &menuRenderer{
		m:     m,
		caja:  NuevaCaja(tema.Color("menu-surface", v), tema.Color("border", v), tema.RadioLG),
		marco: canvas.NewRectangle(color.NRGBA{}),
		aviso: nuevoMasAbajo(tema.RadioLG - tema.Borde),
	}
	// El borde, OTRA VEZ y encima de todo: lo que se desplaza pasa por debajo de él,
	// como en la web, donde el recorte es el de la caja de relleno.
	r.marco.StrokeWidth, r.marco.CornerRadius = tema.Borde, tema.RadioLG
	r.componer()
	return r
}

type menuRenderer struct {
	m       *Menu
	caja    *Caja
	marco   *canvas.Rectangle
	aviso   *masAbajo
	objetos []fyne.CanvasObject
}

// componer ordena lo que se pinta: la caja, los elementos, el aviso de que queda más
// abajo y el borde.
func (r *menuRenderer) componer() {
	// Concat: sin tocar el array de la caja.
	r.objetos = slices.Concat(r.caja.Objetos(), r.m.Elementos, r.aviso.objetos(), []fyne.CanvasObject{r.marco})
}

// IsClip es el recorte del panel: un rótulo largo no se sale por el borde redondeado.
func (r *menuRenderer) IsClip() {}

func (r *menuRenderer) MinSize() fyne.Size { return MedidaMenu(r.m, r.m.ventana) }

func (r *menuRenderer) Objects() []fyne.CanvasObject { return r.objetos }

func (r *menuRenderer) Destroy() {}

// Layout apila los elementos. Cada uno es `w-full`; el separador, además, se come el
// `p-1` con su `-mx-1` y llega de borde a borde.
func (r *menuRenderer) Layout(t fyne.Size) {
	hoja := enHoja(r.m.ventana)
	radio := tema.RadioLG
	if hoja {
		radio = tema.Forma("radius-card")
	}
	r.caja.CambiarRadio(radio)
	r.marco.CornerRadius = radio
	abajo := radio
	if hoja {
		abajo = esquinaRecta
	}
	for _, o := range append(r.caja.Objetos(), r.marco) {
		if rect, es := o.(*canvas.Rectangle); es {
			rect.BottomLeftCornerRadius, rect.BottomRightCornerRadius = abajo, abajo
		}
	}
	if hoja {
		// Sin filetes a los lados ni abajo: la caja y el borde, 1 px más anchos por cada
		// lado y más altos; el recorte del panel se queda con lo de dentro.
		b := tema.Borde
		r.caja.Redimensionar(fyne.NewSize(t.Width+2*b, t.Height+b))
		for _, o := range r.caja.Objetos() {
			o.Move(o.Position().SubtractXY(b, 0))
		}
		r.marco.Move(fyne.NewPos(-b, 0))
		r.marco.Resize(fyne.NewSize(t.Width+2*b, t.Height+b))
	} else {
		r.caja.Redimensionar(t)
		r.marco.Move(fyne.Position{})
		r.marco.Resize(t)
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	propio := r.m.ContenidoAjustable && len(r.m.Elementos) == 1
	r.aviso.colocar(t, !propio && QuedaMasAbajo(MedidaMenu(r.m, r.m.ventana).Height, r.m.desplazado, t.Height), v)
	dentro := tema.Borde + rellenoPanelMenu
	if hoja {
		dentro = tema.Borde + rellenoHojaMenu
	}
	y := dentro - r.m.desplazado
	for _, e := range r.m.Elementos {
		alto := altoElementoEnHoja(e, e.MinSize().Height, hoja)
		if propio {
			alto = max(t.Height-2*dentro, 0)
		}
		if _, es := e.(*SeparadorMenu); es {
			e.Move(fyne.NewPos(tema.Borde, y))
			e.Resize(fyne.NewSize(t.Width-2*tema.Borde, alto))
		} else {
			e.Move(fyne.NewPos(dentro, y))
			e.Resize(fyne.NewSize(t.Width-2*dentro, alto))
		}
		y += alto
	}
}

func (r *menuRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(tema.Color("menu-surface", v), tema.Color("border", v))
	r.marco.StrokeColor = tema.Color("border", v)
	r.componer()
	r.Layout(r.m.Size())
}

// ElementoMenu es MenuItem.tsx: una acción del menú.
type ElementoMenu struct {
	widget.BaseWidget
	Texto string
	// Icono es el glifo del catálogo. `iconos.Ninguno`, que es el cero: sin icono.
	Icono iconos.Icono
	// TamIcono en px de CSS. Cero: 16, que es el que pasan doce de los catorce
	// `MenuItem` con icono de la suite. MenuItem.tsx no impone ninguno.
	TamIcono float32
	// Peligro es el ítem destructivo (cerrar sesión, eliminar): tiñe también su
	// fondo al pasar por encima, no solo el rótulo.
	Peligro bool
	// Denso es `size='sm'`: el cromo denso de los editores. El valor cero es `md`.
	Denso bool
	// Inerte es `disabled`: al 50 %, sin puntero y fuera de la navegación por flechas.
	Inerte   bool
	AlPulsar func()
	// Punto es un token de color: en vez del glifo, un círculo de 8,75 en su hueco.
	// Es lo que la hoja pone donde el icono en las cuatro presencias del menú de
	// cuenta (`<span class="h-2.5 w-2.5 rounded-full" style="background: presence-…">`).
	// Vacío: no hay punto.
	Punto string
	// PuntoColor es el mismo punto con un color que es un DATO y no un token: el del
	// estado de una etiqueta en el filtro de Notepads (`style={{ background: s.color }}`).
	// Gana a `Punto` si los dos están puestos.
	PuntoColor color.NRGBA
	// MantieneAbierto es el ítem que NO recoge el menú: en Dropdown.tsx el menú solo se
	// cierra con Escape, con el clic de fuera o cuando el ítem llama a `close()`. Lo que
	// abre otra cosa lo cierra (el valor cero); lo que cambia un estado que se ve en el
	// propio menú —la presencia del menú de cuenta, con su tic— lo deja abierto.
	MantieneAbierto bool
	// TamTic es el `size` del tic del ítem marcado. Cero: los 16 de Dropdown.tsx; el
	// menú de fondos de Sytue Photos lo pone a 14.
	TamTic float32
	// Marcada pone un tic al final, que es el `{presence === state ? <Icon check/>}`
	// de la web: el estado activo de una lista de opciones dentro de un menú.
	Marcada bool
	// Actual es `aria-current` con `bg-menu-selected`: la opción vigente marcada por el
	// FONDO y no por un tic (la presencia elegida del menú de estado de Sytue Rooms).
	Actual bool
	// Glifo sustituye a Icono cuando el dibujo NO es del catálogo compartido: los
	// glifos propios de una app (el triaje de Sytue Mail) o el punto de color de una
	// etiqueta, que en la web van como hijos del `MenuItem` y no en `icon`. Como en
	// `Boton`, NO se tiñe: lo pinta quien lo construye, que es quien sabe su color.
	// Ocupa `TamIcono` y su hueco, igual que un icono.
	Glifo *iconos.Dibujo
	// TintaTic es el token del tic de Marcada. Vacío: el del rótulo, que es el de la
	// presencia; las etiquetas de Mail lo piden en `primary-text`.
	TintaTic string

	menu                      *Menu
	encima, pulsado, enfocado bool
	// sinAro es el foco SIN `:focus-visible`: se lo dio el código al abrir el menú con
	// el ratón. La primera tecla lo enciende.
	sinAro bool
}

// focoVisible es `:focus-visible`: el foco llegó con el teclado, o por código tras él.
func (e *ElementoMenu) focoVisible() bool { return e.enfocado && !e.sinAro }

var (
	_ fyne.Widget       = (*ElementoMenu)(nil)
	_ fyne.Tappable     = (*ElementoMenu)(nil)
	_ fyne.Focusable    = (*ElementoMenu)(nil)
	_ fyne.Tabbable     = (*ElementoMenu)(nil)
	_ desktop.Hoverable = (*ElementoMenu)(nil)
)

// AcceptsTab: el Tab SALE del menú desde su último ítem (y Mayúsculas+Tab desde el
// primero), como en la web, donde los ítems son `<button>` corrientes y el panel no
// atrapa el foco. En medio, el Tab va de ítem en ítem, que es lo que ya hace el gestor
// de foco de la capa; solo en los extremos hay que decidir, porque la capa da la vuelta.
func (e *ElementoMenu) AcceptsTab() bool {
	if e.menu == nil {
		return false
	}
	lista := e.menu.pulsables()
	if len(lista) == 0 {
		return false
	}
	if conMayusculas() {
		return lista[0] == e
	}
	return lista[len(lista)-1] == e
}

// NuevoElementoMenu crea la acción.
func NuevoElementoMenu(texto string, icono iconos.Icono, alPulsar func()) *ElementoMenu {
	e := &ElementoMenu{Texto: texto, Icono: icono, AlPulsar: alPulsar}
	e.ExtendBaseWidget(e)
	return e
}

func (e *ElementoMenu) densidad() int {
	if e.Denso {
		return 1
	}
	return 0
}

func (e *ElementoMenu) tamIcono() float32 {
	if e.TamIcono > 0 {
		return e.TamIcono
	}
	return iconos.TallaSM
}

// MedidaElementoMenu es lo que ocupa una acción: `px-3 py-2 text-base` en la densidad
// normal (33,25 de alto) y `px-2.5 py-1.5 text-sm` en la densa (26,25).
//
// El ancho es el NATURAL. En el panel cada elemento es `w-full` y se estira, pero el
// natural es el que decide cuánto mide el panel.
func MedidaElementoMenu(e *ElementoMenu) fyne.Size {
	m, esc := margenElemento[e.densidad()], escalonElemento[e.densidad()]
	ancho := Medir(e.Texto, esc, Normal).Width
	if e.Icono != iconos.Ninguno || e.Glifo != nil || e.conPunto() {
		// El hueco del punto es el del icono: la web le pone `w-[18px]` a propósito,
		// «para alinear el punto con los iconos del menú».
		ancho += e.tamIcono() + huecoElemento
	}
	if e.Marcada {
		ancho += e.tamTic() + huecoElemento
	}
	// Con el dedo, 44 como un botón (`pointer-coarse:min-h-[max(44px,2.75rem)]` de
	// MenuItem): medían 33 y 26.
	return fyne.NewSize(ancho+2*m.Width, sueloTactil(esc.Linea+2*m.Height))
}

// TamTicMenu es el `<Icon name="check" size={16} />` del ítem marcado, y LadoPuntoMenu
// el `h-2.5 w-2.5` del punto de presencia.
const (
	TamTicMenu    float32 = 16
	LadoPuntoMenu         = 2.5 * 0.25 * tema.Rem
)

func (e *ElementoMenu) colores(v tema.Variante) (fondo, texto, icono color.NRGBA) {
	texto, icono = tema.Color("text", v), tema.Color("text-muted", v)
	encima, activo := "menu-hover", "menu-selected"
	if e.Peligro {
		texto, icono = tema.Color("danger-text", v), tema.Color("danger-text", v)
		encima = "danger-bg"
	}
	switch {
	case e.Inerte: // `disabled:` congela el reposo antes de bajar la opacidad
	case e.pulsado:
		fondo = tema.Color(activo, v)
	case e.encima || e.focoVisible(): // `hover:` y `focus-visible:` pintan igual
		fondo = tema.Color(encima, v)
	case e.Actual:
		fondo = tema.Color("menu-selected", v)
	}
	return atenuar(fondo, e.Inerte), atenuar(texto, e.Inerte), atenuar(icono, e.Inerte)
}

// CreateRenderer construye la acción: caja sin borde, glifo y rótulo.
func (e *ElementoMenu) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, texto, icono := e.colores(v)
	r := &elementoRenderer{
		e: e,
		// El aro global de theme.css (`:where(:focus-visible)`: 2 px de `focus-ring`,
		// pegado y con la esquina del ítem), que `focus-visible:outline-none` no quita
		// porque es `box-shadow`. Abierto: el fondo del ítem entra en 150 ms y, mientras,
		// dejaría ver un aro relleno por el centro.
		aro:    NuevoAro(tema.RadioMD, SinHueco, v).Abierto(),
		caja:   NuevaCaja(fondo, color.NRGBA{}, tema.RadioMD),
		rotulo: Texto(e.Texto, escalonElemento[e.densidad()], Normal, texto),
	}
	r.fondo.hacia(fondo, r.pintarFondo)
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	// El hueco del glifo se RESERVA siempre (ver MedidaElementoMenu) y se pinta solo
	// si hay quien lo dibuje: así el ancho del menú no cambia el día que lleguen.
	switch {
	case e.Glifo != nil:
		r.ico, r.propio = e.Glifo, true
		r.objetos = append(r.objetos, r.ico.Objeto())
	case e.Icono != iconos.Ninguno:
		r.ico = iconos.TrazoConTinta(e.Icono, e.tamIcono(), icono)
		r.objetos = append(r.objetos, r.ico.Objeto())
	}
	if e.conPunto() {
		r.punto = canvas.NewRectangle(e.colorPunto(tema.De(fyne.CurrentApp().Settings().ThemeVariant())))
		r.punto.CornerRadius = LadoPuntoMenu / 2
		r.objetos = append(r.objetos, r.punto)
	}
	if e.Marcada {
		r.tic = iconos.TrazoConTinta(iconos.Tic, e.tamTic(), r.tintaTic(texto))
		r.objetos = append(r.objetos, r.tic.Objeto())
	}
	r.objetos = append(r.objetos, r.rotulo)
	return r
}

// Tapped ejecuta la acción y recoge el menú, salvo con `MantieneAbierto`.
func (e *ElementoMenu) Tapped(*fyne.PointEvent) {
	if e.Inerte {
		return
	}
	if e.menu != nil && !e.MantieneAbierto {
		e.menu.Cerrar()
	}
	if e.AlPulsar != nil {
		e.AlPulsar()
	}
}

// MouseIn y MouseOut son `:hover`.
func (e *ElementoMenu) MouseIn(*desktop.MouseEvent) {
	if e.Inerte {
		return
	}
	e.encima = true
	e.Refresh()
}

// MouseMoved no hace nada.
func (e *ElementoMenu) MouseMoved(*desktop.MouseEvent) {}

// MouseOut devuelve la acción a su color de reposo.
func (e *ElementoMenu) MouseOut() { e.encima = false; e.Refresh() }

// MouseDown y MouseUp son `active:bg-border`.
func (e *ElementoMenu) MouseDown(*desktop.MouseEvent) {
	if !e.Inerte {
		e.pulsado = true
		e.Refresh()
	}
}

func (e *ElementoMenu) MouseUp(*desktop.MouseEvent) {
	if e.pulsado {
		e.pulsado = false
		e.Refresh()
	}
}

// FocusGained pinta el ítem como al pasar por encima —el `focus-visible:bg-…` de la
// hoja— y le pone el anillo, que es lo que lo distingue del que está bajo el puntero.
func (e *ElementoMenu) FocusGained() { e.enfocado = true; e.Refresh() }

// FocusLost lo devuelve al reposo.
func (e *ElementoMenu) FocusLost() { e.enfocado, e.sinAro = false, false; e.Refresh() }

// TypedRune ignora el texto.
func (e *ElementoMenu) TypedRune(rune) {}

// TypedKey es la navegación del menú: flechas que circulan, Intro y espacio que
// activan, y Escape que recoge.
//
// Escape se atiende AQUÍ y no en el panel porque el foco está en el ítem, y en Fyne la
// tecla es de quien tiene el foco: no hay fase de captura. Ver `Descartar`.
func (e *ElementoMenu) TypedKey(ev *fyne.KeyEvent) {
	if e.sinAro { // la primera tecla enciende el foco visible
		e.sinAro = false
		e.Refresh()
	}
	switch ev.Name {
	case fyne.KeyDown:
		if e.menu != nil {
			e.menu.mover(e, 1)
		}
	case fyne.KeyUp:
		if e.menu != nil {
			e.menu.mover(e, -1)
		}
	case fyne.KeyReturn, fyne.KeyEnter, fyne.KeySpace:
		e.Tapped(nil)
	case fyne.KeyEscape, mobile.KeyBack:
		if e.menu != nil {
			e.menu.cerrarConEscape()
		}
	case fyne.KeyTab:
		if e.menu != nil {
			e.menu.salirConTab(conMayusculas())
		}
	default:
		// Las demás teclas no son del menú y se dejan pasar. El `default` es
		// obligatorio: `exhaustive` cuenta las setenta y pico de `fyne.KeyName`.
	}
}

type elementoRenderer struct {
	e        *ElementoMenu
	aro      *Aro
	caja     *Caja
	fondo    transicion // `transition-colors duration-150` de MenuItem
	ico, tic *iconos.Dibujo
	// propio dice que el glifo es el `Glifo` de quien llama y no se tiñe.
	propio  bool
	punto   *canvas.Rectangle
	rotulo  *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *elementoRenderer) MinSize() fyne.Size { return MedidaElementoMenu(r.e) }

func (r *elementoRenderer) Objects() []fyne.CanvasObject { return r.objetos }

func (r *elementoRenderer) Destroy() {}

// Layout coloca el glifo y el rótulo: `items-center gap-2`, pegados a la izquierda.
func (r *elementoRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	m, esc := margenElemento[r.e.densidad()], escalonElemento[r.e.densidad()]
	x := m.Width
	if r.ico != nil {
		d := r.e.tamIcono()
		r.ico.Colocar(x, (t.Height-d)/2, d)
		x += d + huecoElemento
	}
	if r.punto != nil {
		// Centrado en el hueco del icono, que es lo que hace el `w-[18px]` de la web.
		d := r.e.tamIcono()
		r.punto.Move(fyne.NewPos(x+(d-LadoPuntoMenu)/2, (t.Height-LadoPuntoMenu)/2))
		r.punto.Resize(fyne.NewSize(LadoPuntoMenu, LadoPuntoMenu))
		x += d + huecoElemento
	}
	ColocarTexto(r.rotulo, esc, x, (t.Height-esc.Linea)/2)
	if r.tic != nil {
		// Al final de la fila estirada, que es donde lo deja el `flex-1` del rótulo.
		tic := r.e.tamTic()
		r.tic.Colocar(t.Width-m.Width-tic, (t.Height-tic)/2, tic)
	}
}

func (r *elementoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, texto, icono := r.e.colores(v)
	r.fondo.hacia(fondo, r.pintarFondo)
	r.aro.Pintar(v)
	r.aro.Ver(r.e.focoVisible())
	r.rotulo.Text = r.e.Texto
	r.rotulo.Color = texto
	r.rotulo.Refresh()
	if r.ico != nil && !r.propio {
		r.ico.TenirCon(icono)
	}
	if r.punto != nil {
		r.punto.FillColor = r.e.colorPunto(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
		r.punto.Refresh()
	}
	if r.tic != nil {
		r.tic.TenirCon(r.tintaTic(texto))
	}
}

func (r *elementoRenderer) pintarFondo(c color.NRGBA) { r.caja.Pintar(c, color.NRGBA{}) }

// tintaTic es el color del tic: el de TintaTic, o el del rótulo si no hay.
func (r *elementoRenderer) tintaTic(texto color.NRGBA) color.NRGBA {
	if r.e.TintaTic == "" {
		return texto
	}
	return atenuar(tema.Color(r.e.TintaTic, tema.De(fyne.CurrentApp().Settings().ThemeVariant())), r.e.Inerte)
}

// SeparadorMenu es MenuSeparator.tsx: `-mx-1 my-1 h-px bg-border-subtle`. El `-mx-1` se
// come el `p-1` del panel, así que la línea llega de borde a borde.
type SeparadorMenu struct {
	widget.BaseWidget
}

var _ fyne.Widget = (*SeparadorMenu)(nil)

// NuevoSeparadorMenu crea el separador.
func NuevoSeparadorMenu() *SeparadorMenu {
	s := &SeparadorMenu{}
	s.ExtendBaseWidget(s)
	return s
}

// AltoSeparadorMenu es lo que ocupa: la línea de un píxel más el `my-1` por arriba y
// por abajo.
func AltoSeparadorMenu() float32 { return tema.Borde + 2*tema.Espacio(1) }

// CreateRenderer construye la línea.
func (s *SeparadorMenu) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return &separadorRenderer{linea: canvas.NewRectangle(BordeSobre("border-subtle", "menu-surface", v))}
}

type separadorRenderer struct{ linea *canvas.Rectangle }

func (r *separadorRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, AltoSeparadorMenu())
}

func (r *separadorRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.linea}
}

func (r *separadorRenderer) Destroy() {}

func (r *separadorRenderer) Layout(t fyne.Size) {
	r.linea.Move(fyne.NewPos(0, tema.Espacio(1)))
	r.linea.Resize(fyne.NewSize(t.Width, tema.Borde))
}

func (r *separadorRenderer) Refresh() {
	r.linea.FillColor = BordeSobre("border-subtle", "menu-surface", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r.linea.Refresh()
}

// conPunto dice si el ítem lleva punto en vez de glifo.
func (e *ElementoMenu) conPunto() bool { return e.Punto != "" || e.PuntoColor.A > 0 }

// colorPunto es el del dato si lo hay y, si no, el del token.
func (e *ElementoMenu) colorPunto(v tema.Variante) color.NRGBA {
	if e.PuntoColor.A > 0 {
		return e.PuntoColor
	}
	return tema.Color(e.Punto, v)
}

// tamTic es el lado del tic: el suyo o el de Dropdown.tsx.
func (e *ElementoMenu) tamTic() float32 {
	if e.TamTic > 0 {
		return e.TamTic
	}
	return TamTicMenu
}
