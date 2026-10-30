// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # La cabecera de vista de Sytue Drive, medida
//
// No es `ui.CabeceraApp` (SectionHeader): esa lleva título y subtítulo y `px-4`, y esta
// lleva la RUTA y `sm:px-6`. En la web tampoco es el mismo componente — está escrita a
// mano en FileBrowser.tsx.
//
// Medido a 1280×800 sobre `corporate.sytue.eu/drive`:
//
//	cabecera   48,5 de alto = `py-2.5` (8,75) × 2 + 30 de contenido + `border-b` (1)
//	canalón    21 desde `sm:` (`sm:px-6`), 14 por debajo (`px-4`)
//	ruta       el primer segmento en x=281 = 260 (carril) + 21
//	buscador   28 × 196, en x=833,25
//	«Nuevo»    28 × 76,25, en x=1036,25  ·  «Subir» 28 × 71, en x=1119,5
//	conmutador 58 × 30, de x=1201 a 1259 — y es ÉL quien manda el alto
//
// Los 30 son la pieza que no se ve: todos los controles miden 28, pero el conmutador de
// vista lleva `border` y con `box-sizing: border-box`… no, al revés: el `div` que lo
// envuelve es `inline-flex` con borde, así que mide 28 + 2 = 30 y estira la línea. De
// ahí salen los 48,5 de la cabecera y los 66,63 de la ruta (centrada en 30, no en 28).
// Calcularlo con 28 da 47,5 y deja toda la pantalla un píxel arriba.
const (
	// AltoControlCromo es el `h-8` de los controles de la cabecera.
	AltoControlCromo float32 = 8 * 0.25 * tema.Rem
	// AltoLineaCabecera es lo que mide la línea de la cabecera: el conmutador con su
	// borde, que es el hijo más alto.
	AltoLineaCabecera float32 = AltoControlCromo + 2*tema.Borde
	// RellenoYCabecera es `py-2.5`.
	RellenoYCabecera float32 = 2.5 * 0.25 * tema.Rem
	// AltoCabeceraVista es el total, con el `border-b` por dentro de la caja.
	AltoCabeceraVista float32 = 2*RellenoYCabecera + AltoLineaCabecera + tema.Borde
	// HuecoCabecera es `gap-x-3` entre la ruta y el grupo de acciones.
	HuecoCabecera float32 = 3 * 0.25 * tema.Rem
	// HuecoAcciones es `gap-2` dentro del grupo.
	HuecoAcciones float32 = 2 * 0.25 * tema.Rem
	// AnchoBuscador es `sm:w-56`.
	AnchoBuscador float32 = 56 * 0.25 * tema.Rem
	// MargenCromo es `ml-1` entre «Subir» y el conmutador.
	MargenCromo float32 = 1 * 0.25 * tema.Rem
)

// CanalonCabecera es `px-4 sm:px-6`, resuelto contra el ancho de la VENTANA porque
// `sm:` es una media query y se mide con el rem inicial del navegador, no con el 14.
func CanalonCabecera(ventana float32) float32 {
	if ventana == 0 || ventana >= ui.CorteSM {
		return 6 * 0.25 * tema.Rem
	}
	return 4 * 0.25 * tema.Rem
}

// ---- El conmutador de vista (lista / cuadrícula) ----

// Es UN widget y no dos botones sueltos porque en la web es un `div` con `border`,
// `divide-x` y `overflow-hidden`: el filete del medio es el borde izquierdo del segundo
// segmento y las esquinas redondeadas son del grupo. Partido en dos, el filete se pinta
// dos veces o ninguna.
//
// Medido: grupo 58 × 30 con `rounded-md` y `border-border-strong`; cada segmento 28×28;
// el activo `bg-surface-sunken text-primary-text` y el apagado `text-text-muted`, que
// al pasar por encima va a `bg-surface-sunken text-text`.
const (
	// LadoSegmento es el `h-8 w-8` de cada mitad.
	LadoSegmento = AltoControlCromo
	// GlifoSegmento es el `size={16}` del icono de dentro.
	GlifoSegmento float32 = 16
)

// MedidaConmutadorVista es lo que ocupa el grupo: dos segmentos y el marco.
func MedidaConmutadorVista() fyne.Size {
	return fyne.NewSize(2*LadoSegmento+2*tema.Borde, LadoSegmento+2*tema.Borde)
}

// ConmutadorVista es el par de botones «vista de lista» / «vista de cuadrícula».
type ConmutadorVista struct {
	widget.BaseWidget

	// Rejilla dice cuál está activo. El cero es la lista, como en la web.
	Rejilla bool
	// NombreLista y NombreRejilla son los `aria-label`, ya traducidos.
	NombreLista, NombreRejilla string
	AlCambiar                  func(rejilla bool)

	encima   int // 0 ninguno, 1 lista, 2 rejilla
	enfocado bool
}

var (
	_ fyne.Widget       = (*ConmutadorVista)(nil)
	_ fyne.Tappable     = (*ConmutadorVista)(nil)
	_ fyne.Focusable    = (*ConmutadorVista)(nil)
	_ desktop.Hoverable = (*ConmutadorVista)(nil)
)

// NuevoConmutadorVista crea el conmutador con los dos nombres ya traducidos.
func NuevoConmutadorVista(lista, rejilla string, alCambiar func(bool)) *ConmutadorVista {
	c := &ConmutadorVista{NombreLista: lista, NombreRejilla: rejilla, AlCambiar: alCambiar}
	c.ExtendBaseWidget(c)
	return c
}

// MinSize es la medida declarada.
func (c *ConmutadorVista) MinSize() fyne.Size {
	c.ExtendBaseWidget(c)
	return MedidaConmutadorVista()
}

// segmentoEn dice sobre qué mitad cae una posición: 1 la lista, 2 la rejilla.
func (c *ConmutadorVista) segmentoEn(x float32) int {
	if x < tema.Borde+LadoSegmento {
		return 1
	}
	return 2
}

// Tapped elige la vista de la mitad pulsada.
func (c *ConmutadorVista) Tapped(e *fyne.PointEvent) {
	if l := fyne.CurrentApp().Driver().CanvasForObject(c); l != nil {
		l.Focus(c)
	}
	c.poner(c.segmentoEn(e.Position.X) == 2)
}

func (c *ConmutadorVista) poner(rejilla bool) {
	if c.Rejilla == rejilla {
		return
	}
	c.Rejilla = rejilla
	c.Refresh()
	if c.AlCambiar != nil {
		c.AlCambiar(rejilla)
	}
}

// MouseIn y MouseMoved: el hover es POR SEGMENTO, así que hace falta la posición.
func (c *ConmutadorVista) MouseIn(e *desktop.MouseEvent) { c.MouseMoved(e) }

// MouseMoved mueve el resalte de una mitad a la otra sin salir del widget.
func (c *ConmutadorVista) MouseMoved(e *desktop.MouseEvent) {
	if s := c.segmentoEn(e.Position.X); s != c.encima {
		c.encima = s
		c.Refresh()
	}
}

// MouseOut apaga el resalte.
func (c *ConmutadorVista) MouseOut() { c.encima = 0; c.Refresh() }

// FocusGained enciende el aro.
func (c *ConmutadorVista) FocusGained() { c.enfocado = true; c.Refresh() }

// FocusLost lo apaga.
func (c *ConmutadorVista) FocusLost() { c.enfocado = false; c.Refresh() }

// TypedRune conmuta con la barra espaciadora.
func (c *ConmutadorVista) TypedRune(r rune) {
	if r == ' ' {
		c.poner(!c.Rejilla)
	}
}

// TypedKey recorre los dos segmentos con las flechas, como un grupo de radios.
func (c *ConmutadorVista) TypedKey(e *fyne.KeyEvent) {
	if ui.SubirEscape(c, e) { // Escape y «atrás» son del diálogo de encima
		return
	}
	switch e.Name { //nolint:exhaustive // las demás teclas son de quien tenga el foco
	case fyne.KeyLeft:
		c.poner(false)
	case fyne.KeyRight:
		c.poner(true)
	case fyne.KeyReturn, fyne.KeyEnter:
		c.poner(!c.Rejilla)
	}
}

// AccessibilityLabel anuncia la vista ACTIVA, que es lo que `aria-pressed` transmite en
// la web. Fyne no tiene forma de decir «pulsado», así que lo dice el rótulo.
func (c *ConmutadorVista) AccessibilityLabel() string {
	if c.Rejilla {
		return c.NombreRejilla
	}
	return c.NombreLista
}

// AccessibilityRole implementa fyne.Accessible.
func (c *ConmutadorVista) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// CreateRenderer compone el marco, el filete del medio y los dos glifos.
func (c *ConmutadorVista) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &conmutadorRenderer{
		c:       c,
		aro:     ui.NuevoAro(tema.RadioMD, ui.SinHueco, v),
		marco:   ui.NuevaCaja(color.NRGBA{}, tema.Color("border-strong", v), tema.RadioMD),
		veloA:   canvas.NewRectangle(color.NRGBA{}),
		veloB:   canvas.NewRectangle(color.NRGBA{}),
		filete:  canvas.NewRectangle(tema.Color("border", v)),
		glifoA:  iconos.Trazo(iconos.Menu, GlifoSegmento, "primary-text", v),
		glifoB:  iconos.Trazo(iconos.Rejilla, GlifoSegmento, "text-muted", v),
		trasero: canvas.NewRectangle(tema.Color("surface", v)),
	}
	r.objetos = append(r.aro.Objetos(), r.trasero)
	r.objetos = append(r.objetos, r.marco.Objetos()...)
	r.objetos = append(r.objetos, r.veloA, r.veloB, r.filete, r.glifoA.Objeto(), r.glifoB.Objeto())
	r.Refresh()
	return r
}

type conmutadorRenderer struct {
	c              *ConmutadorVista
	aro            *ui.Aro
	marco          *ui.Caja
	trasero        *canvas.Rectangle
	veloA, veloB   *canvas.Rectangle
	filete         *canvas.Rectangle
	glifoA, glifoB *iconos.Dibujo
	objetos        []fyne.CanvasObject
}

func (r *conmutadorRenderer) MinSize() fyne.Size           { return MedidaConmutadorVista() }
func (r *conmutadorRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *conmutadorRenderer) Destroy()                     {}

func (r *conmutadorRenderer) Layout(t fyne.Size) {
	r.trasero.Move(fyne.NewPos(0, 0))
	r.trasero.Resize(t)
	r.marco.Redimensionar(t)

	interior := fyne.NewSize(LadoSegmento, t.Height-2*tema.Borde)
	r.veloA.Move(fyne.NewPos(tema.Borde, tema.Borde))
	r.veloA.Resize(interior)
	x := tema.Borde + LadoSegmento
	r.veloB.Move(fyne.NewPos(x, tema.Borde))
	r.veloB.Resize(interior)
	// `divide-x`: el filete del medio es el borde IZQUIERDO del segundo segmento, así
	// que va dentro de sus 28 y no entre los dos.
	r.filete.Move(fyne.NewPos(x, tema.Borde))
	r.filete.Resize(fyne.NewSize(tema.Borde, interior.Height))

	y := (t.Height - GlifoSegmento) / 2
	r.glifoA.Colocar(tema.Borde+(LadoSegmento-GlifoSegmento)/2, y, GlifoSegmento)
	r.glifoB.Colocar(x+(LadoSegmento-GlifoSegmento)/2, y, GlifoSegmento)
	r.aro.Colocar(t)
}

func (r *conmutadorRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	c := r.c
	r.trasero.FillColor = tema.Color("surface", v)
	r.trasero.Refresh()
	r.marco.Pintar(color.NRGBA{}, tema.Color("border-strong", v))
	r.filete.FillColor = tema.Color("border", v)

	// El orden de la web: activo gana, y el hover solo existe en la otra rama.
	pinta := func(velo *canvas.Rectangle, glifo *iconos.Dibujo, activo, encima bool) {
		switch {
		case activo:
			velo.FillColor = tema.Color("surface-sunken", v)
			glifo.Tenir("primary-text", v)
		case encima:
			velo.FillColor = tema.Color("surface-sunken", v)
			glifo.Tenir("text", v)
		default:
			velo.FillColor = color.NRGBA{}
			glifo.Tenir("text-muted", v)
		}
	}
	pinta(r.veloA, r.glifoA, !c.Rejilla, c.encima == 1)
	pinta(r.veloB, r.glifoB, c.Rejilla, c.encima == 2)

	r.aro.Pintar(v)
	r.aro.Ver(c.enfocado)
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// ---- La cabecera de vista ----

// CabeceraVista es la barra de arriba del explorador: la ruta a la izquierda y el
// grupo de acciones a la derecha, sobre `surface` y con su `border-b`.
//
// Es un CONTENEDOR y no un widget: no se pulsa, no se enfoca y no se anuncia; lo
// interactivo son sus hijos, que ya son widgets. La regla del paquete `ui` es elegir por
// comportamiento, y este no tiene ninguno.
type CabeceraVista struct {
	// Ruta son las migas, de la raíz a la carpeta actual.
	Ruta *ui.Migas
	// Acciones van a la derecha, en el orden de la web: buscador, «Nuevo», «Subir» y
	// el conmutador de vista.
	Acciones []fyne.CanvasObject
	// Conmutador va el último y se separa con `ml-1` ADEMÁS del `gap-2` del grupo:
	// 10,5 en total. Medido: «Subir» acaba en 1190,5 y el conmutador empieza en 1201.
	Conmutador fyne.CanvasObject
	// Ventana es el ancho de la VENTANA en px de CSS: decide el canalón (`sm:px-6`).
	Ventana float32
	// AlCambiarAlto lo llama Layout cuando el ancho cambia el número de líneas: quien la
	// apila vuelve a colocarla.
	AlCambiarAlto func()

	ancho float32 // el de la última vez que se colocó
}

var _ fyne.Layout = (*CabeceraVista)(nil)

// Objeto arma la cabecera. Los dos primeros hijos son SIEMPRE el fondo y el filete, y
// es lo único que `Layout` da por hecho de la lista: el resto lo coloca por los campos.
func (c *CabeceraVista) Objeto(v tema.Variante) fyne.CanvasObject {
	hijos := []fyne.CanvasObject{
		canvas.NewRectangle(tema.Color("surface", v)),
		canvas.NewRectangle(tema.Color("border", v)),
	}
	if c.Ruta != nil {
		hijos = append(hijos, c.Ruta)
	}
	hijos = append(hijos, c.Acciones...)
	if c.Conmutador != nil {
		hijos = append(hijos, c.Conmutador)
	}
	return container.New(c, hijos...)
}

// enRejilla dice si la cabecera va en las DOS FILAS de la web por debajo de 768
// (`max-md:grid max-md:grid-cols-[minmax(0,1fr)_auto_auto]` en FileBrowser.tsx): las migas
// y el conmutador de vista arriba; el buscador, «Nuevo» y «Subir» abajo. Era una fila que
// envolvía, y a 360 daba tres.
func (c *CabeceraVista) enRejilla() bool {
	return c.Ventana > 0 && c.Ventana < ui.Estrecho && c.Ruta != nil && c.Conmutador != nil && len(c.Acciones) == 3
}

// AltoCabeceraRejilla es lo que mide en dos filas: la de arriba la estira el conmutador
// con su borde y la de abajo mide lo que sus controles. Medido en la web a 360 y a 390:
// 95.
const AltoCabeceraRejilla float32 = 2*RellenoYCabecera + AltoLineaCabecera + HuecoAcciones + AltoControlCromo + tema.Borde

// columnasRejilla mide las tres columnas: el buscador crece y las otras dos
// conservan «Nuevo» y lo más ancho de «Subir» y el conmutador de vista.
func (c *CabeceraVista) columnasRejilla(ancho float32) [3]float32 {
	col2 := c.Acciones[1].MinSize().Width
	col3 := max(c.Acciones[2].MinSize().Width, c.Conmutador.MinSize().Width)
	col1 := max(ancho-2*CanalonCabecera(c.Ventana)-col2-col3-2*HuecoAcciones, 0)
	return [3]float32{col1, col2, col3}
}

func (c *CabeceraVista) buscadorEnFilaPropia(ancho float32) bool {
	columnas := c.columnasRejilla(ancho)
	// El mínimo del widget solo deja un cuadratín para escribir. El buscador
	// necesita tres para poder leer parte de la consulta junto a su icono.
	return columnas[0] < ui.MedidaEntrada(ui.CampoCromo, 0, true).Width+2*tema.Rem
}

func (c *CabeceraVista) colocarRejilla(ancho float32) {
	canalon := CanalonCabecera(c.Ventana)
	columnas := c.columnasRejilla(ancho)
	col1, col2 := columnas[0], columnas[1]

	// Las migas ocupan las dos primeras columnas (`col-span-2`) y se recortan si no caben.
	y := RellenoYCabecera
	centrarEn(c.Ruta, canalon, y, min(c.Ruta.MinSize().Width, col1+HuecoAcciones+col2), AltoLineaCabecera)
	conmutador := c.Conmutador.MinSize().Width
	centrarEn(c.Conmutador, ancho-canalon-conmutador, y, conmutador, AltoLineaCabecera)

	y += AltoLineaCabecera + HuecoAcciones
	if c.buscadorEnFilaPropia(ancho) {
		// El carril móvil y el área segura pueden dejar menos que el propio
		// relleno del campo. Conservar una columna casi vacía impide escribir.
		centrarEn(c.Acciones[0], canalon, y, max(ancho-2*canalon, 0), AltoControlCromo)
		y += AltoControlCromo + HuecoAcciones
		x := ancho - canalon - columnas[1] - columnas[2] - HuecoAcciones
		for i := 1; i < len(columnas); i++ {
			centrarEn(c.Acciones[i], x, y, columnas[i], AltoControlCromo)
			x += columnas[i] + HuecoAcciones
		}
		return
	}
	x := canalon
	for i, w := range columnas {
		centrarEn(c.Acciones[i], x, y, w, AltoControlCromo)
		x += w + HuecoAcciones
	}
}

// MinSize es el alto de las líneas que ocupa en el ancho de la última vez que se colocó
// (o en el de la ventana, antes de la primera); el ancho lo pone la pantalla.
func (c *CabeceraVista) MinSize([]fyne.CanvasObject) fyne.Size {
	ancho := c.ancho
	if ancho <= 0 {
		ancho = c.Ventana
	}
	return fyne.NewSize(0, c.AltoCon(nil, ancho))
}

// AltoCon permite que ui.Pila mida el salto del buscador antes de colocarlo,
// con el ancho útil de la hoja en vez de conservar el de la ventana anterior.
func (c *CabeceraVista) AltoCon(_ []fyne.CanvasObject, ancho float32) float32 {
	if c.enRejilla() {
		alto := AltoCabeceraRejilla
		if c.buscadorEnFilaPropia(ancho) {
			alto += AltoControlCromo + HuecoAcciones
		}
		return alto
	}
	return AltoCabeceraEn(len(c.lineas(ancho)))
}

// AltoCabeceraEn es lo que mide la cabecera con `n` líneas: el `py-2.5`, las líneas con
// su `gap-y-2` y el `border-b`.
func AltoCabeceraEn(n int) float32 {
	n = max(n, 1)
	return 2*RellenoYCabecera + float32(n)*AltoLineaCabecera + float32(n-1)*HuecoAcciones + tema.Borde
}

// linea es una línea de la cabecera: sus hijos de izquierda a derecha, y si va pegada a
// la derecha (el `ml-auto` del grupo cuando cabe entero).
type linea struct {
	hijos   []fyne.CanvasObject
	derecha bool
}

// lineas reparte la cabecera como el `flex-wrap` de la web: todo en una línea si cabe;
// si no, la ruta arriba y el grupo de acciones debajo, partido en tantas líneas como haga
// falta. Antes no se partía y, a 360, el grupo se salía por la izquierda tapando la ruta:
// «Mi unidad» no se veía (visto en una ventana real el 2026-09-27).
func (c *CabeceraVista) lineas(ancho float32) []linea {
	grupo := append([]fyne.CanvasObject{}, c.Acciones...)
	if c.Conmutador != nil {
		grupo = append(grupo, c.Conmutador)
	}
	hueco := func(i int) float32 { // lo que va delante del hijo i del grupo en su línea
		if i < len(grupo) && grupo[i] == c.Conmutador {
			return HuecoAcciones + MargenCromo
		}
		return HuecoAcciones
	}
	var anchoGrupo float32
	for i, o := range grupo {
		if i > 0 {
			anchoGrupo += hueco(i)
		}
		anchoGrupo += o.MinSize().Width
	}
	libre := ancho - 2*CanalonCabecera(c.Ventana)
	var ruta float32
	if c.Ruta != nil {
		ruta = c.Ruta.MinSize().Width + HuecoCabecera
	}
	if ancho <= 0 || ruta+anchoGrupo <= libre {
		return []linea{{hijos: append(soloRuta(c.Ruta), grupo...), derecha: true}}
	}
	out := []linea{}
	if c.Ruta != nil {
		out = append(out, linea{hijos: soloRuta(c.Ruta)})
	}
	if anchoGrupo <= libre {
		return append(out, linea{hijos: grupo, derecha: true})
	}
	var actual linea
	var usado float32
	for i, o := range grupo {
		w := o.MinSize().Width
		if len(actual.hijos) > 0 && usado+hueco(i)+w > libre {
			out, actual, usado = append(out, actual), linea{}, 0
		}
		if len(actual.hijos) > 0 {
			usado += hueco(i)
		}
		actual.hijos = append(actual.hijos, o)
		usado += w
	}
	return append(out, actual)
}

func soloRuta(r *ui.Migas) []fyne.CanvasObject {
	if r == nil {
		return nil
	}
	return []fyne.CanvasObject{r}
}

// Layout coloca cada línea centrada en su alto de 30: la de una sola pieza, con la ruta a
// la izquierda y las acciones pegadas a la derecha; partida, la ruta arriba y el grupo
// debajo.
func (c *CabeceraVista) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) >= 2 {
		objs[0].Move(fyne.NewPos(0, 0))
		objs[0].Resize(t)
		// El `border-b` va DENTRO de la caja: `box-sizing: border-box`.
		objs[1].Move(fyne.NewPos(0, t.Height-tema.Borde))
		objs[1].Resize(fyne.NewSize(t.Width, tema.Borde))
	}
	if c.enRejilla() {
		c.ancho = t.Width
		c.colocarRejilla(t.Width)
		return
	}
	lineas := c.lineas(t.Width)
	if c.ancho != t.Width {
		antes := len(c.lineas(c.ancho))
		c.ancho = t.Width
		// El alto depende del ancho y `MinSize` no lo recibe: si cambia el número de
		// líneas, el padre tiene que volver a preguntarlo.
		if antes != len(lineas) && c.AlCambiarAlto != nil {
			go fyne.Do(c.AlCambiarAlto)
		}
	}
	for n, l := range lineas {
		arriba := RellenoYCabecera + float32(n)*(AltoLineaCabecera+HuecoAcciones)
		if l.derecha {
			c.colocarADerecha(l, arriba, t.Width)
		} else {
			c.colocarAIzquierda(l, arriba, t.Width)
		}
	}
}

// centrarEnLinea coloca un hijo centrado en el alto de 30 de la línea que empieza en y.
func centrarEnLinea(o fyne.CanvasObject, x, y, ancho float32) {
	centrarEn(o, x, y, ancho, AltoLineaCabecera)
}

// centrarEn coloca un hijo con ese ancho, centrado en una fila de ese alto que empieza
// en y (`items-center`).
func centrarEn(o fyne.CanvasObject, x, y, ancho, fila float32) {
	alto := o.MinSize().Height
	o.Move(fyne.NewPos(x, y+(fila-alto)/2))
	o.Resize(fyne.NewSize(ancho, alto))
}

// colocarAIzquierda es una línea partida: de izquierda a derecha desde el canalón.
func (c *CabeceraVista) colocarAIzquierda(l linea, y, ancho float32) {
	canalon := CanalonCabecera(c.Ventana)
	x := canalon
	for i, o := range l.hijos {
		if i > 0 {
			x += HuecoAcciones
			if o == c.Conmutador {
				x += MargenCromo
			}
		}
		w := o.MinSize().Width
		if o == fyne.CanvasObject(c.Ruta) {
			w = min(w, max(ancho-2*canalon, 0))
		}
		centrarEnLinea(o, x, y, w)
		x += w
	}
}

// colocarADerecha es la línea entera: las acciones pegadas a la derecha y la ruta con
// lo que quede.
func (c *CabeceraVista) colocarADerecha(l linea, y, ancho float32) {
	canalon := CanalonCabecera(c.Ventana)
	derecha := ancho - canalon
	for i := len(l.hijos) - 1; i >= 0; i-- {
		o := l.hijos[i]
		if o == fyne.CanvasObject(c.Ruta) {
			// `grow`: la ruta se lleva lo que quede, y se recorta si no cabe.
			resto := max(derecha-canalon+HuecoAcciones-HuecoCabecera, 0)
			centrarEnLinea(o, canalon, y, min(resto, c.Ruta.MinSize().Width))
			continue
		}
		m := o.MinSize()
		centrarEnLinea(o, derecha-m.Width, y, m.Width)
		derecha -= m.Width + HuecoAcciones
		if o == c.Conmutador {
			derecha -= MargenCromo
		}
	}
}
