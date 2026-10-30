// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// BotonIcono es `IconButton.tsx`: el botón CUADRADO de solo icono del CROMO DENSO
// —carriles, cabeceras de panel, barras de estado, filas de lista—, más los pocos
// cuadrados que la web escribe a mano fuera de ese componente.
//
// # Por qué no es `Boton` con `IconoSolo`, y cuándo usar cada uno
//
// Son dos componentes distintos en la web y por un motivo medido, que está escrito en
// el comentario de `IconButton.tsx`: `Button iconOnly` no baja de `h-8` —32 px con la raíz de 16— porque su suelo es el objetivo táctil de una
// ACCIÓN PRINCIPAL. El cromo del editor necesita 24 y 28, y por eso existe el otro.
//
// La regla para elegir, sin pensarlo dos veces:
//
//   - Una acción de la pantalla que resulta que no lleva texto (el aspa de un diálogo,
//     «Redactar» sin rótulo, el triaje de una fila, la barra de lote) → `Boton` con
//     `IconoSolo`. Es lo que usan MessageList.tsx, BatchActionBar.tsx y Modal.tsx.
//   - Un botón del CROMO: 24 / 28 / 32 px, esquina `rounded-control`, sin relleno en
//     reposo y con `aria-pressed` si conmuta → este.
//
// # Las tres tallas
//
// `LadoIconoSM` (24) es el suelo de WCAG 2.5.8 y no hay talla menor: la hoja lo escribe
// en px (`min-h-[24px]`) justamente para que se cumpla con cualquier raíz. Las otras dos
// salen de `h-7` y `h-8` con el rem de 16.
//
// Los colores van por TOKEN y no por un enumerado de tonos, porque los tres `TONE` de
// la web (`default`, `danger`, `header`) son exactamente eso —tres juegos de tokens— y
// hay además dos usos a mano que no son ninguno de los tres (el aspa de Toast.tsx). Un
// enumerado obligaría a añadirle una cuarta rama a cada uno.
type BotonIcono struct {
	widget.BaseWidget
	// Icono es el glifo del catálogo compartido. Obligatorio salvo con Glifo.
	Icono iconos.Icono
	// Glifo sustituye a Icono cuando el dibujo NO es del catálogo compartido: los
	// glifos propios de una app, que en la web tampoco están en `Icon.tsx` (los doce
	// de Sytue Rooms, la estrella de Contacts). Se arma con `iconos.TrazoDeRuta` o
	// `iconos.TrazoDeCuerpo` y gana a `Icono` si los dos están puestos.
	//
	// A diferencia del de `Boton`, ESTE SÍ se tiñe: un botón de cromo cambia el color
	// del glifo al pasar el puntero, y ese es todo su estado de reposo.
	Glifo *iconos.Dibujo
	// Lado es la caja pulsable: una de las tres constantes `LadoIcono*`.
	Lado float32
	// TamIcono es el lado del glifo dentro de la caja.
	TamIcono float32
	// Radio es la esquina: el `rounded-control` compartido por todos los tonos.
	Radio float32
	// Trazo y TrazoEncima son los tokens del glifo. TrazoEncima vacío: el mismo en
	// reposo y al pasar por encima.
	Trazo, TrazoEncima string
	// FondoEncima y FondoActivo son los velos. Vacíos: sin fondo en ningún estado, que
	// es el `ghost` de la web en reposo.
	FondoEncima, FondoActivo string
	// FondoPulsado es el relleno de `aria-pressed` (`bg-selected`, o `chrome-active`
	// sobre la banda). Vacío: el botón no conmuta.
	FondoPulsado string
	// Pulsado es el `pressed` de la web: estado de conmutador. El color por sí solo no
	// comunica estado (WCAG 1.4.1), así que quien lo use pone también una pista de
	// texto —el `title`— con el estado.
	Pulsado bool
	// Inerte es `disabled`: ni puntero, ni pulsación, ni foco, y el glifo al 40 %
	// (`disabled:opacity-40 disabled:pointer-events-none` de la hoja). Un botón que
	// no se puede usar y se ve igual es una diana que falla sin decir por qué.
	Inerte bool
	// Rotulo es el nombre accesible, YA traducido. Obligatorio: un icono no dice nada
	// a quien no lo ve.
	Rotulo   string
	AlPulsar func()
	// AlEnfocar permite que una tira desplazable revele el control al tabular.
	AlEnfocar   func()
	mostrarFoco func()
	// TokenAro es el color del anillo de foco. Vacío es `focus-ring`, el azul de la
	// hoja; sobre la banda de tinta lo pone `SobreBanda` en `chrome-focus`.
	TokenAro string

	encima, pulsado, enfocado bool
	pista                     *Pista // la ayuda que lo envuelve, para abrirla con el teclado
	// alEncima avisa al padre que tenga que saber del `:hover` (una fila del menú con su
	// pliegue dentro).
	alEncima func(bool)
	// fueraDelTab aparta el botón del tabulador mientras diga que sí, sin apagarlo: el
	// aspa de un aviso PRESTADO a un diálogo (ver `Aviso.prestado`).
	fueraDelTab func() bool
}

// Disabled es `fyne.Disableable`, que es lo único que el gestor de foco de Fyne mira
// para saltarse un control al tabular: un botón `disabled` no es parada en el navegador,
// y aquí lo era. Y el aspa de un aviso prestado tampoco (ver `fueraDelTab`).
func (b *BotonIcono) Disabled() bool {
	return b.Inerte || (b.fueraDelTab != nil && b.fueraDelTab())
}

// Enable enciende el botón.
func (b *BotonIcono) Enable() { b.Inerte = false; b.Refresh() }

// Disable lo apaga.
func (b *BotonIcono) Disable() { b.Inerte = true; b.Refresh() }

func (b *BotonIcono) avisarEncima(f func(bool)) { b.alEncima = f }

func (b *BotonIcono) ponerPista(p *Pista) { b.pista = p }

// Las tres tallas de IconButton.tsx, con el rem de 16.
const (
	// LadoIconoSM es `h-6 w-6 min-h-[24px] min-w-[24px]`: 21 subidos al suelo de WCAG.
	LadoIconoSM float32 = 24
	// LadoIconoMD es `h-7 w-7`: 24,5, no los 28 que dice su comentario en la web
	// —ese comentario da por hecha una raíz de 16—.
	LadoIconoMD = 7 * 0.25 * tema.Rem
	// LadoIconoLG es `h-8 w-8`: 28.
	LadoIconoLG = 8 * 0.25 * tema.Rem
)

var (
	_ fyne.Widget       = (*BotonIcono)(nil)
	_ fyne.Tappable     = (*BotonIcono)(nil)
	_ fyne.Focusable    = (*BotonIcono)(nil)
	_ fyne.Accessible   = (*BotonIcono)(nil)
	_ fyne.Disableable  = (*BotonIcono)(nil)
	_ desktop.Hoverable = (*BotonIcono)(nil)
)

// NuevoBotonIcono crea el del cromo corriente: talla md, esquina `rounded-control`, glifo
// azul con velo de selección al pasar y velo de superficie, que es el tono `default`.
func NuevoBotonIcono(i iconos.Icono, rotulo string, alPulsar func()) *BotonIcono {
	b := &BotonIcono{
		Icono: i, Lado: LadoIconoMD, TamIcono: iconos.TallaSM, Radio: tema.Forma("radius-control"),
		Trazo: "primary-text", TrazoEncima: "primary-text", FondoEncima: "selected",
		FondoPulsado: "selected", Rotulo: rotulo, AlPulsar: alPulsar,
	}
	b.ExtendBaseWidget(b)
	return b
}

// SobreBanda es el tono `header` de la web: sobre la banda de tinta los velos son los
// del cromo, porque un `hover:bg-bg-subtle` ahí pinta un velo claro y deja el glifo a
// 1,1:1 — el defecto que ya costó una ronda entera de auditoría.
func (b *BotonIcono) SobreBanda() *BotonIcono {
	b.Trazo, b.TrazoEncima = "primary-text", ""
	b.FondoEncima, b.FondoActivo, b.FondoPulsado = "chrome-hover", "chrome-active", "chrome-active"
	b.Radio = tema.Forma("radius-control") // todos los tonos comparten la esquina del botón
	// Y el anillo BLANCO, que es lo que pide la hoja para todo lo que va sobre tinta
	// (`.sytue-focus-ring-chrome` en TopBarButton y
	// en Sidebar): el azul de `focus-ring` sobre el cromo no llega a 3:1, que es el
	// mínimo de WCAG 1.4.11 para un indicador de foco.
	b.TokenAro = "chrome-focus"
	return b
}

// AccessibilityLabel es el `aria-label`. Un botón que solo lleva un glifo no tiene texto
// que leer, así que sin esto el lector de pantalla anuncia «botón» y ya.
func (b *BotonIcono) AccessibilityLabel() string { return b.Rotulo }

// AccessibilityRole dice que es un botón.
func (b *BotonIcono) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// MedidaBotonIcono es lo que ocupa: cuadrado, como su `h-N w-N`.
func MedidaBotonIcono(lado float32) fyne.Size { return fyne.NewSize(lado, lado) }

func (b *BotonIcono) colores(v tema.Variante) (fondo, trazo color.NRGBA) {
	trazo = tema.Color(b.Trazo, v)
	if b.Inerte {
		// `disabled:` congela el reposo y baja la opacidad; no hay hover ni pulsado.
		return color.NRGBA{}, atenuarCon(trazo, 0.4)
	}
	if b.encima && b.TrazoEncima != "" {
		trazo = tema.Color(b.TrazoEncima, v)
	}
	if b.Pulsado && b.FondoPulsado == "selected" {
		trazo = tema.Color("text", v)
	}
	switch {
	case b.pulsado && b.FondoActivo != "":
		fondo = tema.Color(b.FondoActivo, v)
	case b.encima && b.FondoEncima != "":
		fondo = tema.Color(b.FondoEncima, v)
	case b.Pulsado && b.FondoPulsado != "":
		fondo = tema.Color(b.FondoPulsado, v)
	}
	return fondo, trazo
}

// CreateRenderer construye la geometría: el aro, una caja sin borde y el glifo centrado.
func (b *BotonIcono) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, trazo := b.colores(v)
	// `sytue-focus-ring`, el PEGADO: es el que la hoja le pone a IconButton, y no el
	// de hueco que llevan los botones de acción.
	token := b.TokenAro
	if token == "" {
		token = "focus-ring"
	}
	// Sin fondo en reposo: el aro va abierto (punto 3 de `Aro`).
	r := &botonIconoRenderer{
		b: b, icono: b.Icono, glifo: b.Glifo,
		aro: NuevoAroCon(token, b.Radio, SinHueco, v).Abierto(), caja: NuevaCaja(fondo, color.NRGBA{}, b.Radio),
	}
	// Con un token `sidebar-*` el botón va dentro del carril y su glifo lleva el relleno
	// ámbar de allí, como cualquier otro icono del menú.
	r.ico = b.dibujo(trazo).DeToken(b.Trazo)
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.ico.Objeto())
	return r
}

// dibujo es el glifo del botón: el propio de la app si lo hay, y si no el del catálogo.
func (b *BotonIcono) dibujo(trazo color.NRGBA) *iconos.Dibujo {
	if b.Glifo != nil {
		b.Glifo.TenirCon(trazo)
		return b.Glifo
	}
	return iconos.TrazoConTinta(b.Icono, b.TamIcono, trazo)
}

// Tapped es `onClick`.
func (b *BotonIcono) Tapped(*fyne.PointEvent) {
	if b.Inerte {
		return
	}
	if c := fyne.CurrentApp().Driver().CanvasForObject(b); c != nil {
		enfocar(c, b)
	}
	if b.AlPulsar != nil {
		b.AlPulsar()
	}
}

// MouseIn y MouseOut son `:hover`.
func (b *BotonIcono) MouseIn(*desktop.MouseEvent) {
	// La pista sale también en un botón apagado, como el `title` de uno `disabled`: es
	// donde se dice POR QUÉ lo está.
	b.pista.alPuntero(true)
	if b.Inerte {
		return
	}
	b.encima = true
	if b.alEncima != nil {
		b.alEncima(true)
	}
	b.Refresh()
}

// MouseMoved no hace nada: el estado ya lo fijan MouseIn y MouseOut.
func (b *BotonIcono) MouseMoved(*desktop.MouseEvent) {}

// MouseOut devuelve el botón a su color de reposo.
func (b *BotonIcono) MouseOut() {
	b.encima = false
	if b.alEncima != nil {
		b.alEncima(false)
	}
	b.pista.alPuntero(false)
	b.Refresh()
}

// MouseDown y MouseUp son `:active`.
func (b *BotonIcono) MouseDown(*desktop.MouseEvent) {
	if !b.Inerte {
		b.pulsado = true
		b.Refresh()
	}
}

func (b *BotonIcono) MouseUp(*desktop.MouseEvent) {
	if b.pulsado {
		b.pulsado = false
		b.Refresh()
	}
}

// FocusGained enciende el aro.
func (b *BotonIcono) FocusGained() {
	if b.Inerte {
		return
	}
	b.enfocado = true
	b.pista.alFoco(true)
	b.Refresh()
	if b.AlEnfocar != nil {
		b.AlEnfocar()
	}
	if b.mostrarFoco != nil {
		b.mostrarFoco()
	}
}

func (b *BotonIcono) revelarAlFoco(f func()) { b.mostrarFoco = f }

// FocusLost lo apaga.
func (b *BotonIcono) FocusLost() { b.enfocado = false; b.pista.alFoco(false); b.Refresh() }

// TypedRune pulsa con la barra espaciadora, como un `<button>`.
func (b *BotonIcono) TypedRune(r rune) {
	if r == ' ' {
		b.Tapped(nil)
	}
}

// TypedKey pulsa con Intro y sube Escape a la capa de encima.
func (b *BotonIcono) TypedKey(e *fyne.KeyEvent) {
	if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
		b.Tapped(nil)
		return
	}
	if EsCierre(e) {
		b.pista.alEscape()
	}
	SubirEscape(b, e)
}

type botonIconoRenderer struct {
	b       *BotonIcono
	aro     *Aro
	caja    *Caja
	ico     *iconos.Dibujo
	icono   iconos.Icono
	glifo   *iconos.Dibujo
	objetos []fyne.CanvasObject
}

func (r *botonIconoRenderer) MinSize() fyne.Size {
	return MedidaBotonIcono(r.b.Lado)
}

func (r *botonIconoRenderer) Objects() []fyne.CanvasObject { return r.objetos }

func (r *botonIconoRenderer) Destroy() {}

func (r *botonIconoRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	d := r.b.TamIcono
	r.ico.Colocar((t.Width-d)/2, (t.Height-d)/2, d)
}

// Refresh repinta. El color del glifo va DENTRO de su SVG, así que cambiarlo es
// `Tenir`: el `Dibujo` rehace su mapa de bits solo si el color de verdad ha cambiado.
func (r *botonIconoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, trazo := r.b.colores(v)
	if r.icono != r.b.Icono || r.glifo != r.b.Glifo {
		// La navegación móvil cambia de menú a «volver» sin crear otro botón.
		// Cambiar solo el campo dejaba el SVG anterior en pantalla.
		r.icono, r.glifo = r.b.Icono, r.b.Glifo
		r.ico = r.b.dibujo(trazo).DeToken(r.b.Trazo)
		r.objetos[len(r.objetos)-1] = r.ico.Objeto()
	}
	r.caja.CambiarRadio(r.b.Radio) // el de los botones de la barra es de la forma
	r.aro.CambiarRadio(r.b.Radio)
	r.caja.Pintar(fondo, color.NRGBA{})
	token := r.b.TokenAro
	if token == "" {
		token = "focus-ring"
	}
	r.aro.CambiarToken(token)
	r.aro.Pintar(v)
	r.aro.Ver(r.b.enfocado)
	r.ico.DeToken(r.b.Trazo).TenirCon(trazo)
	r.Layout(r.b.Size())
}
