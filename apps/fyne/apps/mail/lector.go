// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El lector (`MessageReader.tsx`): la columna de la derecha.
//
// # Qué está medido y qué no
//
// El ESTADO VACÍO sí: es lo que producción enseñaba en el barrido del 2026-09-21, con
// el título en `sytue-h4` en y = 441,5, la descripción en dos líneas en y = 474,75 y
// el botón de 94 × 28 en y = 530,75, todo centrado en x = 910. Lo pinta `ui.Vacio`,
// que ya clava esa composición (`gap-3` más el `mt-2` de la acción).
//
// El hilo abierto NO está medido, y aquí se dice: abrir una conversación marca el
// correo como leído, o sea que escribe en producción, y la sonda de medidas no
// escribe. Todos los números del hilo salen de las CLASES de `MessageReader.tsx`
// —origen «hoja»— y llevan su clase escrita al lado. Quien pueda medirlo contra un
// buzón de pruebas, que lo ate.
const (
	// MargenLector es el `px-6` de la cabecera, RellenoLector su `py-5` y
	// AireAcciones el `mt-4` sobre la fila de botones. Hoja.
	MargenLector  float32 = 6 * 0.25 * tema.Rem
	RellenoLector float32 = 5 * 0.25 * tema.Rem
	AireAcciones  float32 = 4 * 0.25 * tema.Rem
	// HuecoAcciones es el `gap-2` entre los botones de la cabecera y entre el asunto
	// y la cuenta de mensajes. Hoja.
	HuecoAcciones float32 = 2 * 0.25 * tema.Rem
	// MargenTarjeta es el `px-5` de una tarjeta de mensaje y RellenoTarjeta su `py-4`.
	// Hoja.
	MargenTarjeta  float32 = 5 * 0.25 * tema.Rem
	RellenoTarjeta float32 = 4 * 0.25 * tema.Rem
	// HuecoTarjetas es el `gap-3` entre mensajes del hilo y también el `gap-3` entre
	// el avatar, el texto y el galón de dentro de la cabecera. Hoja.
	HuecoTarjetas float32 = 3 * 0.25 * tema.Rem
	// CuerpoLector es el `py-6` del hilo. Hoja.
	CuerpoLector float32 = 6 * 0.25 * tema.Rem
	// TallaGlifoLector es el `size = 16` por defecto de `<Glyph>`, y TallaGalon el
	// `size={18}` del galón que pliega una tarjeta.
	TallaGlifoLector float32 = 16
	TallaGalon       float32 = 18
)

// Lector es la columna de la derecha en sus seis estados.
type Lector struct {
	widget.BaseWidget

	T      *i18n.Translator
	Locale i18n.Locale
	Ahora  func() time.Time

	// Conversacion es el hilo abierto. Vacía (sin mensajes) es el estado «elige una».
	Conversacion Conversacion
	Estado       Estado
	// SoloLectura es un buzón compartido o la falta de red: se quedan Responder,
	// Responder a todos y Reenviar, que solo abren un borrador.
	SoloLectura bool
	// Cuerpo dice lo que ya se ha leído de un mensaje; `false` es que todavía no ha
	// llegado. Lo pone el Buzón: el lector no habla con el servicio.
	Cuerpo func(id string) (Cuerpo, bool)

	// Acciones son las del triaje, compartidas con la lista.
	Acciones *Acciones
	// Baja dice que el último mensaje es de una lista y ofrece darse de baja
	// (List-Unsubscribe); DandoDeBaja, que la petición está en camino.
	Baja, DandoDeBaja bool
	AlDarseDeBaja     func(Mensaje)
	// AlCrearEvento es «Crear evento»: el intent del Hub que abre Sytue Calendar con
	// el evento ya relleno. Nil, no hay botón. CreandoEvento es la petición en camino.
	AlCrearEvento func(Mensaje)
	CreandoEvento bool
	// Los adjuntos y el pie de cada mensaje: descargar, guardar en Drive (nil: no se
	// ofrece) y el original. Guardando son los adjuntos en camino a Drive, por
	// `claveAdjunto`: su botón se apaga mientras tanto.
	AlDescargarAdjunto func(Mensaje, AdjuntoVisible)
	AlGuardarEnDrive   func(Mensaje, AdjuntoVisible)
	AlOriginal         func(Mensaje)
	Guardando          map[string]bool

	AlResponder  func(Mensaje, Respuesta)
	AlRedactar   func()
	AlReintentar func()
	// AlDesplegar pide el cuerpo de un mensaje al desplegar su tarjeta: la web lo baja
	// bajo demanda (`getMessageBody` al expandir), y aquí igual.
	AlDesplegar func(id string)
}

// Respuesta es el modo con el que se abre el redactor.
type Respuesta uint8

// Los tres modos de `ReplyMode`.
const (
	Responder Respuesta = iota
	ResponderATodos
	Reenviar
)

var _ fyne.Widget = (*Lector)(nil)

// NuevoLector crea la columna.
func NuevoLector(t *i18n.Translator, l i18n.Locale) *Lector {
	x := &Lector{T: t, Locale: l, Ahora: time.Now}
	x.ExtendBaseWidget(x)
	return x
}

// AccessibilityLabel es el asunto del hilo, que es el encabezado de la vista.
func (x *Lector) AccessibilityLabel() string {
	if len(x.Conversacion.Mensajes) == 0 {
		return x.T.T("reader.emptyTitle", nil)
	}
	return x.asunto()
}

// AccessibilityRole implementa fyne.Accessible.
func (x *Lector) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

func (x *Lector) asunto() string {
	if s := x.Conversacion.Ultimo.Asunto; s != "" {
		return s
	}
	return x.T.T("common.noSubject", nil)
}

// abierto dice si hay hilo que pintar.
func (x *Lector) abierto() bool {
	return x.Estado == Listo && len(x.Conversacion.Mensajes) > 0
}

func (x *Lector) cuerpoDe(id string) (Cuerpo, bool) {
	if x.Cuerpo == nil {
		return Cuerpo{}, false
	}
	return x.Cuerpo(id)
}

// tintaBoton es el token del rótulo de cada variante, que es el color del glifo.
func tintaBoton(v ui.Variante) string {
	if v == ui.Primario || v == ui.Peligro {
		return "on-primary"
	}
	return "text"
}

// CreateRenderer compone cabecera, hilo y estado vacío.
func (x *Lector) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &lectorRenderer{
		x:      x,
		fondo:  canvas.NewRectangle(tema.Color("bg", v)),
		banda:  canvas.NewRectangle(tema.Color("surface", v)),
		filete: canvas.NewRectangle(tema.Color("border", v)),
		cuenta: ui.NuevoRotulo("", tema.TextoSM, ui.Normal, "text-subtle"),
		barra:  container.NewWithoutLayout(),
	}

	r.hilo = &hiloLayout{}
	r.pila = container.New(r.hilo)
	r.desplaza = container.NewVScroll(r.pila)
	r.vacio = ui.NuevoVacio("", "")
	r.Refresh()
	return r
}

func (x *Lector) responder(m Respuesta) {
	if x.AlResponder == nil || len(x.Conversacion.Mensajes) == 0 {
		return
	}
	x.AlResponder(x.Conversacion.Mensajes[0], m)
}

type lectorRenderer struct {
	x                    *Lector
	fondo, banda, filete *canvas.Rectangle
	titulo               []*canvas.Text
	cuenta               *ui.Rotulo
	// barra son los botones de la cabecera y chips las etiquetas del hilo (nil sin
	// ninguna); se rehacen solo cuando cambia su clave (ver `claveBarra`).
	barra, chips *fyne.Container
	clave        string
	hilo         *hiloLayout
	pila         *fyne.Container
	desplaza     *container.Scroll
	vacio        *ui.Vacio
	objetos      []fyne.CanvasObject
	// hiloID y nHilo dicen qué tarjetas hay montadas: rehacerlas cuesta, y solo
	// cambian cuando cambia el hilo.
	hiloID string
	nHilo  int
	ancho  float32
}

// lineasAsunto reparte el asunto. El `h2` de la web PARTE en líneas
// (`[overflow-wrap:anywhere]`, `min-w-0 flex-1`) y no trunca: un asunto largo se lee
// entero en la cabecera del hilo, que es justo donde se va a buscar.
func (r *lectorRenderer) lineasAsunto(ancho float32) []string {
	cabe := ancho - 2*MargenLector
	if r.cuenta.Visible() {
		cabe -= ui.MedidaRotulo(r.cuenta).Width + HuecoAcciones
	}
	return repartirPre(r.x.asunto(), ui.EscalonTitulo(3), ui.PesoTitulo(3), cabe)
}

// AltoCabeceraLector es el alto de la banda del asunto: `py-5`, las líneas del
// `sytue-h3`, el `mt-4`, las filas de la barra, y con etiquetas el `mt-3` y sus filas
// de chips; el `border-b` va dentro. Hoja.
func AltoCabeceraLector(lineas int, barra, chips float32) float32 {
	alto := 2*RellenoLector + float32(max(lineas, 1))*ui.EscalonTitulo(3).Linea + AireAcciones + barra + tema.Borde
	if chips > 0 {
		alto += AireChipsLector + chips
	}
	return alto
}

func (r *lectorRenderer) MinSize() fyne.Size {
	return fyne.NewSize(2*MargenLector, AltoCabeceraLector(1, ui.MedidaBoton("", ui.TallaSM, 1).Height, 0))
}

// altos son los de la barra y los chips repartidos a lo que les deja el `px-6`.
func (r *lectorRenderer) altos(ancho float32) (barra, chips float32) {
	util := max(ancho-2*MargenLector, 0)
	barra = altoFlujo(r.barra, util)
	if r.chips != nil {
		chips = altoFlujo(r.chips, util)
	}
	return barra, chips
}

func (r *lectorRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *lectorRenderer) Destroy()                     {}

func (r *lectorRenderer) Layout(t fyne.Size) {
	r.ancho = t.Width
	r.fondo.Resize(t)
	if !r.x.abierto() {
		centrarVacio(r.vacio, fyne.NewPos(0, 0), t)
		return
	}
	lineas := r.lineasAsunto(t.Width)
	r.ponerTitulo(lineas)
	altoBarra, altoChips := r.altos(t.Width)
	alto := AltoCabeceraLector(len(lineas), altoBarra, altoChips)
	r.banda.Move(fyne.NewPos(0, 0))
	r.banda.Resize(fyne.NewSize(t.Width, alto))
	r.filete.Move(fyne.NewPos(0, alto-tema.Borde))
	r.filete.Resize(fyne.NewSize(t.Width, tema.Borde))

	for i, l := range r.titulo {
		ui.ColocarTexto(l, ui.EscalonTitulo(3), MargenLector, RellenoLector+float32(i)*ui.EscalonTitulo(3).Linea)
	}
	if r.cuenta.Visible() {
		// `items-baseline`: la cuenta se apoya en la línea base de la PRIMERA línea
		// del asunto, no en el fondo de su caja.
		m := ui.MedidaRotulo(r.cuenta)
		r.cuenta.Resize(m)
		r.cuenta.Move(fyne.NewPos(t.Width-MargenLector-m.Width,
			RellenoLector+lineaBase(ui.EscalonTitulo(3), ui.PesoTitulo(3))-lineaBase(tema.TextoSM, ui.Normal)))
	}

	util := max(t.Width-2*MargenLector, 0)
	y := RellenoLector + float32(len(lineas))*ui.EscalonTitulo(3).Linea + AireAcciones
	r.barra.Move(fyne.NewPos(MargenLector, y))
	r.barra.Resize(fyne.NewSize(util, altoBarra))
	if r.chips != nil {
		r.chips.Move(fyne.NewPos(MargenLector, y+altoBarra+AireChipsLector))
		r.chips.Resize(fyne.NewSize(util, altoChips))
	}

	// El `py-6 px-6` va DENTRO del desplazamiento (`overflow-y-auto` con su relleno):
	// el hilo se desliza hasta el borde de la cabecera, no hasta 21 px antes.
	r.hilo.margen, r.hilo.ancho = margenHilo(ventana(r.x, t.Width)), t.Width
	r.desplaza.Move(fyne.NewPos(0, alto))
	r.desplaza.Resize(fyne.NewSize(t.Width, max(t.Height-alto, 0)))
	r.pila.Refresh()
}

// ponerTitulo deja un `canvas.Text` por línea del asunto.
func (r *lectorRenderer) ponerTitulo(lineas []string) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if len(lineas) != len(r.titulo) {
		r.titulo = r.titulo[:0]
		for range lineas {
			r.titulo = append(r.titulo, ui.Texto("", ui.EscalonTitulo(3), ui.PesoTitulo(3), tema.Color("text", v)))
		}
		r.montar()
	}
	for i, l := range lineas {
		r.titulo[i].Text = l
		r.titulo[i].Color = tema.Color("text", v)
		r.titulo[i].Refresh()
	}
}

func (r *lectorRenderer) montar() {
	r.objetos = []fyne.CanvasObject{r.fondo, r.banda, r.filete}
	for _, l := range r.titulo {
		r.objetos = append(r.objetos, l)
	}
	r.objetos = append(r.objetos, r.cuenta, r.barra)
	if r.chips != nil {
		r.objetos = append(r.objetos, r.chips)
	}
	r.objetos = append(r.objetos, r.desplaza, r.vacio)
}

// ponerBarra rehace la barra y los chips si ha cambiado lo que los define.
func (r *lectorRenderer) ponerBarra() {
	x := r.x
	angosta := estrecha(x, r.ancho)
	if k := x.claveBarra(angosta); k != r.clave || r.objetos == nil {
		r.clave = k
		r.barra = x.barra(angosta)
		r.chips = x.chips()
		r.montar()
	}
}

func (r *lectorRenderer) Refresh() {
	x := r.x
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = tema.Color("bg", v)
	r.banda.FillColor = tema.Color("surface", v)
	r.filete.FillColor = tema.Color("border", v)

	abierto := x.abierto()
	if abierto {
		r.ponerBarra()
	}
	for _, o := range []fyne.CanvasObject{r.banda, r.filete, r.desplaza, r.barra} {
		verlo(o, abierto)
	}
	if r.chips != nil {
		verlo(r.chips, abierto)
	}
	for _, l := range r.titulo {
		verlo(l, abierto)
	}
	verlo(r.cuenta, abierto && len(x.Conversacion.Mensajes) > 1)
	verlo(r.vacio, !abierto)

	if abierto {
		r.cuenta.Texto = x.T.T("reader.messageCount", i18n.Vars{"count": len(x.Conversacion.Mensajes)})
		r.rehacerHilo()
	} else {
		r.ponerVacio()
	}
	if r.objetos == nil {
		r.montar()
	}
	for _, o := range r.objetos {
		o.Refresh()
	}
	for _, o := range r.pila.Objects {
		o.Refresh()
	}
	r.Layout(x.Size())
}

// rehacerHilo monta las tarjetas del hilo en orden de LECTURA (antiguo → reciente),
// que es el revés de la lista, y expande solo el último. Con el mismo hilo y los
// mismos mensajes se conservan: plegar una tarjeta no se deshace porque llegue el
// cuerpo de otra.
func (r *lectorRenderer) rehacerHilo() {
	c := r.x.Conversacion
	if r.hiloID == c.HiloID && r.nHilo == len(c.Mensajes) {
		for i, o := range r.pila.Objects {
			if t, ok := o.(*TarjetaMensaje); ok {
				t.Mensaje = c.Mensajes[len(c.Mensajes)-1-i] // el triaje optimista cambia el sobre
			}
		}
		return
	}
	r.hiloID, r.nHilo = c.HiloID, len(c.Mensajes)
	r.pila.Objects = nil
	for i := len(c.Mensajes) - 1; i >= 0; i-- {
		r.pila.Add(nuevaTarjetaMensaje(r.x, c.Mensajes[i], i == 0))
	}
	r.desplaza.ScrollToTop()
}

func (r *lectorRenderer) ponerVacio() {
	x, t := r.x, r.x.T
	var titulo, desc string
	var accion fyne.CanvasObject
	switch x.Estado {
	case Error:
		titulo = t.T("list.errorTitle", nil)
		desc = t.T("list.errorDescription", nil)
	case SinConexion:
		titulo = t.T("offline.status", nil)
		desc = t.T("offline.description", nil)
	case SinPermiso:
		titulo = t.T("forbidden.title", nil)
		desc = t.T("forbidden.description", nil)
	case Cargando:
		// Cargando SOLO es «cargando» si hay un hilo elegido. Sin hilo, lo que toca
		// es «elige una conversación»: el cero del tipo es Cargando, y sin esto la
		// pantalla recién abierta decía «cargando el mensaje…» para siempre.
		if len(x.Conversacion.Mensajes) > 0 {
			titulo = t.T("reader.loadingBody", nil)
			break
		}
		fallthrough
	case Vacio, Listo:
		// MEDIDO: es el estado que producción enseñaba. Título en `sytue-h4`,
		// descripción de dos líneas y el botón de 94 × 28 debajo.
		titulo = t.T("reader.emptyTitle", nil)
		desc = t.T("reader.emptyDescription", nil)
		if !x.SoloLectura {
			b := ui.NuevoBoton(t.T("nav.compose", nil), x.AlRedactar).Con(ui.Primario, ui.TallaSM)
			b.Icono = iconos.Mas
			accion = b
		}
	}
	if nuevo := ponerEstado(r.vacio, titulo, desc, accion); nuevo != r.vacio {
		cambiar(r.objetos, r.vacio, nuevo)
		r.vacio = nuevo
	}
}

// ventana es el ancho de la VENTANA, que es lo que miden `xl:` y `2xl:` (con el rem de
// 16 del navegador, no el de la suite). Sin lienzo todavía, el del propio lector.
func ventana(o fyne.CanvasObject, respaldo float32) float32 {
	if c := fyne.CurrentApp().Driver().CanvasForObject(o); c != nil && c.Size().Width > 0 {
		return c.Size().Width
	}
	return respaldo
}

// margenHilo es `px-6 py-6 xl:px-8 2xl:px-10`: los cortes van en px de ventana (1280 y
// 1536), no en el rem de la suite.
func margenHilo(ventana float32) (horizontal float32) {
	switch {
	case ventana >= 1536:
		return tema.Espacio(10)
	case ventana >= 1280:
		return tema.Espacio(8)
	}
	return tema.Espacio(6)
}

// AnchoHilo es `max-w-[min(100%,clamp(44rem,92cqw,88rem))]` centrado con `mx-auto`: el
// hilo crece con el panel hasta 88rem y no baja de 44rem salvo que no quepa. `util` es
// el ancho de dentro del relleno, que es contra lo que se miden el 100 % y los `cqw`.
func AnchoHilo(util float32) float32 {
	return min(util, max(44*tema.Rem, min(0.92*util, 88*tema.Rem)))
}

// hiloLayout apila las tarjetas con su `gap-3`, dentro del relleno del desplazamiento
// y al ancho de AnchoHilo, centrado.
//
// `ancho` es el del desplazamiento y lo fija el lector ANTES de que nadie pregunte el
// mínimo: el alto de una tarjeta depende del ancho al que se reparte su cuerpo, y el
// `Scroll` pregunta el mínimo del contenido antes de colocarlo.
type hiloLayout struct{ margen, ancho float32 }

func (h *hiloLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	ancho := AnchoHilo(max(h.ancho-2*h.margen, 0))
	alto := 2 * CuerpoLector
	for i, o := range objs {
		if tj, ok := o.(*TarjetaMensaje); ok && ancho > 0 {
			tj.ancho = ancho
		}
		if i > 0 {
			alto += HuecoTarjetas
		}
		alto += o.MinSize().Height
	}
	return fyne.NewSize(2*h.margen, alto)
}

func (h *hiloLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	util := max(t.Width-2*h.margen, 0)
	ancho := AnchoHilo(util)
	x := h.margen + (util-ancho)/2
	y := CuerpoLector
	for _, o := range objs {
		if tj, ok := o.(*TarjetaMensaje); ok {
			tj.ancho = ancho // el alto depende del ancho: el cuerpo se reparte en líneas
		}
		m := o.MinSize()
		o.Move(fyne.NewPos(x, y))
		o.Resize(fyne.NewSize(ancho, m.Height))
		y += m.Height + HuecoTarjetas
	}
}

// lineaBase es dónde cae la línea base DENTRO de la caja de línea de CSS de un
// escalón, la misma que pinta `ui.ColocarTexto` (la de Chromium). Sirve para
// `items-baseline`.
func lineaBase(e tema.Escalon, p ui.Peso) float32 {
	return ui.LineaBase(e, p)
}
