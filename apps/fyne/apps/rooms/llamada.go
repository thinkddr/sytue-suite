// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"image"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El cromo de la llamada: la tarjeta del historial y la ANTESALA. La llamada conectada
// es videollamada.go, y los medios, `apps/fyne/llamada` (ADR 0046, enmendado el
// 2026-09-25: voz y vídeo en Go puro, tarjeta de sonido con miniaudio).
//
// El hueco del vídeo de la antesala sigue sin previa: la captura de cámara es la pieza
// que falta (docs/roadmap.md, «Llamadas nativas»). Está medido y reservado
// —`aspect-video`, la misma proporción que la web— para que el layout no se mueva
// cuando llegue: ver `HuecoVideo`.

// ---- La tarjeta de llamada del historial ----

// Medidas de la tarjeta. HOJA salvo donde se diga: el canal medido tenía llamadas
// TERMINADAS (la variante de línea), no la destacada.
var (
	// CanalonTarjeta es el `mx-2`, AireTarjeta el `mt-2`, y RellenoTarjeta /
	// RellenoLinea los `py-2` y `py-1` de cada variante.
	CanalonTarjeta = tema.Espacio(2)
	AireTarjeta    = tema.Espacio(2)
	RellenoTarjeta = tema.Espacio(2)
	RellenoLinea   = tema.Espacio(1)
	// SangriaTarjeta es su `px-3` y HuecoTarjeta su `gap-3`.
	SangriaTarjeta = tema.Espacio(3)
	HuecoTarjeta   = tema.Espacio(3)
	// DiscoDestacado es el `h-8 w-8` de la llamada abierta y DiscoLinea el `h-6 w-6`
	// de una terminada.
	DiscoDestacado = tema.Espacio(8)
	DiscoLinea     = tema.Espacio(6)
	// GlifoTarjeta es el `size={16}` de la cámara.
	GlifoTarjeta = iconos.TallaSM
)

// TarjetaLlamada es la fila de sistema de una llamada.
//
// Solo la llamada ABIERTA es una tarjeta con caja; las terminadas —y los «inició» de
// días pasados— son una línea sin borde. Es el comentario literal de la web, y la
// razón está medida en el uso: un canal con reuniones diarias era una pila de cajas
// iguales en la que la única que importa, la que se puede unir, no destacaba.
type TarjetaLlamada struct {
	widget.BaseWidget

	T       *i18n.Translator
	Locale  i18n.Locale
	Mensaje Mensaje
	// Destacada es la última llamada y además abierta: la única con «Unirse».
	Destacada bool

	AlEntrar func(Mensaje)

	caja        *ui.Caja
	disco       *canvas.Circle
	glifo       *iconos.Dibujo
	texto, hora *canvas.Text
	entrar      *ui.Boton
	objetos     []fyne.CanvasObject
}

var _ fyne.Widget = (*TarjetaLlamada)(nil)

// NuevaTarjetaLlamada crea la fila.
func NuevaTarjetaLlamada(t *i18n.Translator, l i18n.Locale, m Mensaje, destacada bool,
	alEntrar func(Mensaje),
) *TarjetaLlamada {
	// Una llamada ya terminada no se puede «destacar»: la sala está cerrada.
	c := &TarjetaLlamada{
		T: t, Locale: l, Mensaje: m,
		Destacada: destacada && !m.Terminada, AlEntrar: alEntrar,
	}
	c.ExtendBaseWidget(c)
	return c
}

// AccessibilityLabel es el texto de la tarjeta con su hora.
func (c *TarjetaLlamada) AccessibilityLabel() string {
	return RotuloLlamada(c.T, c.Mensaje) + ", " + RotuloHora(c.Locale, c.Mensaje.Creado)
}

// AccessibilityRole implementa fyne.Accessible.
func (c *TarjetaLlamada) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// MedidaTarjetaLlamada es lo que ocupa. Es la única cuenta.
func MedidaTarjetaLlamada(destacada bool) fyne.Size {
	alto := AireTarjeta + 2*RellenoLinea + DiscoLinea
	if destacada {
		alto = AireTarjeta + 2*RellenoTarjeta + 2*tema.Borde + DiscoDestacado
	}
	// El bloque de texto son dos líneas —el qué y la hora— y puede pasar al disco.
	alto = max(alto, AireTarjeta+2*RellenoLinea+tema.TextoSM.Linea+escalonMeta.Linea)
	return fyne.NewSize(0, alto)
}

func (c *TarjetaLlamada) lado() float32 {
	if c.Destacada {
		return DiscoDestacado
	}
	return DiscoLinea
}

func (c *TarjetaLlamada) tokens() (disco, tintaGlifo, tintaTexto string) {
	if c.Destacada {
		return "selected", "primary-text", "text"
	}
	return "border", "text-muted", "text-muted"
}

// CreateRenderer compone la tarjeta.
func (c *TarjetaLlamada) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	disco, tintaGlifo, tintaTexto := c.tokens()

	borde := color.NRGBA{}
	fondo := color.NRGBA{}
	if c.Destacada {
		borde, fondo = tema.Color("border", v), tema.Color("bg-subtle", v)
	}
	c.caja = ui.NuevaCaja(fondo, borde, tema.RadioMD)
	c.disco = canvas.NewCircle(tema.Color(disco, v))
	cuerpo := glifoVideo
	if c.Mensaje.Terminada {
		cuerpo = glifoLlamadaFin
	}
	c.glifo = iconos.TrazoDeCuerpo(cuerpo, GlifoTarjeta, tema.Color(tintaGlifo, v))
	c.texto = ui.Texto(RotuloLlamada(c.T, c.Mensaje), tema.TextoSM, ui.Normal,
		tema.Color(tintaTexto, v))
	c.hora = ui.Texto(RotuloHora(c.Locale, c.Mensaje.Creado), escalonMeta, ui.Normal,
		tema.Color("text-subtle", v))

	c.objetos = append(c.caja.Objetos(), c.disco, c.glifo.Objeto(), c.texto, c.hora)
	if c.Destacada && c.AlEntrar != nil {
		c.entrar = ui.NuevoBoton(c.T.T("action.join", nil),
			func() { c.AlEntrar(c.Mensaje) }).Con(ui.Secundario, ui.TallaSM)
		c.objetos = append(c.objetos, c.entrar)
	}
	r := &tarjetaRenderer{c: c}
	r.Refresh()
	return r
}

type tarjetaRenderer struct{ c *TarjetaLlamada }

func (r *tarjetaRenderer) MinSize() fyne.Size {
	return MedidaTarjetaLlamada(r.c.Destacada)
}
func (r *tarjetaRenderer) Objects() []fyne.CanvasObject { return r.c.objetos }
func (r *tarjetaRenderer) Destroy()                     {}

// Layout coloca el disco, el bloque de texto y, si la hay, la acción de entrar.
func (r *tarjetaRenderer) Layout(t fyne.Size) {
	c := r.c
	arriba := AireTarjeta
	alto := max(t.Height-arriba, 0)
	ui.MoverCaja(c.caja, fyne.NewPos(CanalonTarjeta, arriba),
		fyne.NewSize(max(t.Width-2*CanalonTarjeta, 0), alto))

	lado := c.lado()
	x := CanalonTarjeta + SangriaTarjeta
	c.disco.Move(fyne.NewPos(x, arriba+(alto-lado)/2))
	c.disco.Resize(fyne.NewSize(lado, lado))
	c.glifo.Colocar(x+(lado-GlifoTarjeta)/2, arriba+(alto-GlifoTarjeta)/2, GlifoTarjeta)
	x += lado + HuecoTarjeta

	der := t.Width - CanalonTarjeta - SangriaTarjeta
	if c.entrar != nil {
		m := c.entrar.MinSize()
		der -= m.Width
		c.entrar.Move(fyne.NewPos(der, arriba+(alto-m.Height)/2))
		c.entrar.Resize(m)
		der -= HuecoTarjeta
	}

	bloque := tema.TextoSM.Linea + escalonMeta.Linea
	y := arriba + (alto-bloque)/2
	cabe := max(der-x, 0)
	ui.Truncar(c.texto, RotuloLlamada(c.T, c.Mensaje), tema.TextoSM, ui.Normal, cabe)
	ui.ColocarTexto(c.texto, tema.TextoSM, x, y)
	ui.ColocarTexto(c.hora, escalonMeta, x, y+tema.TextoSM.Linea)
}

// Refresh repinta con los colores del tema actual.
func (r *tarjetaRenderer) Refresh() {
	c := r.c
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	disco, tintaGlifo, tintaTexto := c.tokens()
	borde, fondo := color.NRGBA{}, color.NRGBA{}
	if c.Destacada {
		borde, fondo = tema.Color("border", v), tema.Color("bg-subtle", v)
	}
	c.caja.Pintar(fondo, borde)
	c.disco.FillColor = tema.Color(disco, v)
	c.disco.Refresh()
	c.glifo.TenirCon(tema.Color(tintaGlifo, v))
	c.texto.Color = tema.Color(tintaTexto, v)
	c.texto.Refresh()
	c.hora.Color = tema.Color("text-subtle", v)
	c.hora.Refresh()
	r.Layout(c.Size())
}

// ---- El hueco del vídeo ----

// Medidas del hueco. HOJA (`PreJoin.tsx`): `aspect-video rounded-lg bg-neutral-900`.
const (
	// ProporcionVideo es el `aspect-video` de la web: 16:9.
	ProporcionVideo = 16.0 / 9.0
)

// HuecoVideo es el sitio RESERVADO para la previa de la cámara en la antesala. Hoy no
// pinta vídeo porque la captura no está (ver el comentario de cabecera).
//
// Existe y no es un `canvas.Rectangle` suelto por dos motivos:
//
//   - **La geometría tiene que estar desde hoy.** El hueco es `aspect-video`, y si no
//     se reserva ahora el layout salta el día que el vídeo llegue —que es exactamente
//     el problema que el esqueleto de una lista resuelve—.
//   - **Sin cámara hay que decir algo.** La web pinta «Cámara desactivada» centrado
//     sobre el mismo rectángulo, y ese es el estado que esta pieza sí sabe pintar
//     entero.
//
// Con Imagen, pinta la previa de la cámara encima del rectángulo (`object-cover` en la
// web: se recorta, no se deja franja).
type HuecoVideo struct {
	widget.BaseWidget

	// Rotulo es lo que se dice cuando no hay imagen: «Cámara desactivada», «Sin
	// permiso de cámara». Ya traducido.
	Rotulo string
	// Imagen es el último fotograma de la previa; nil, el rótulo.
	Imagen image.Image

	fondo   *canvas.Rectangle
	texto   *canvas.Text
	previa  *canvas.Image
	objetos []fyne.CanvasObject
}

// PonerImagen cambia el fotograma de la previa sin rehacer nada.
func (h *HuecoVideo) PonerImagen(img image.Image) {
	h.Imagen = img
	if h.previa == nil {
		return
	}
	h.previa.Image = img
	h.previa.Refresh()
	h.Refresh()
}

var _ fyne.Widget = (*HuecoVideo)(nil)

// NuevoHuecoVideo crea el hueco con el rótulo de su estado.
func NuevoHuecoVideo(rotulo string) *HuecoVideo {
	h := &HuecoVideo{Rotulo: rotulo}
	h.ExtendBaseWidget(h)
	return h
}

// AccessibilityLabel es el rótulo: quien no ve el rectángulo tiene que saber por qué
// no hay imagen.
func (h *HuecoVideo) AccessibilityLabel() string { return h.Rotulo }

// AccessibilityRole implementa fyne.Accessible.
func (h *HuecoVideo) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// MedidaHuecoVideo es la caja 16:9 de un ancho dado. Es la única cuenta.
func MedidaHuecoVideo(ancho float32) fyne.Size {
	return fyne.NewSize(ancho, ancho/ProporcionVideo)
}

// CreateRenderer compone el rectángulo y su rótulo.
func (h *HuecoVideo) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	h.fondo = canvas.NewRectangle(tema.Color("neutral-900", v))
	h.fondo.CornerRadius = tema.RadioLG
	h.texto = ui.Texto(h.Rotulo, tema.TextoSM, ui.Normal, tema.Color("neutral-400", v))
	h.previa = canvas.NewImageFromImage(h.Imagen)
	h.previa.FillMode = canvas.ImageFillCover
	h.previa.CornerRadius = tema.RadioLG
	h.objetos = []fyne.CanvasObject{h.fondo, h.texto, h.previa}
	r := &huecoRenderer{h: h}
	r.Refresh()
	return r
}

type huecoRenderer struct{ h *HuecoVideo }

func (r *huecoRenderer) MinSize() fyne.Size {
	return MedidaHuecoVideo(AnchoAntesalaVideo)
}
func (r *huecoRenderer) Objects() []fyne.CanvasObject { return r.h.objetos }
func (r *huecoRenderer) Destroy()                     {}

func (r *huecoRenderer) Layout(t fyne.Size) {
	r.h.fondo.Resize(t)
	r.h.previa.Resize(t)
	ancho := ui.Medir(r.h.Rotulo, tema.TextoSM, ui.Normal).Width
	ui.ColocarTexto(r.h.texto, tema.TextoSM, (t.Width-ancho)/2, (t.Height-tema.TextoSM.Linea)/2)
}

func (r *huecoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.h.fondo.FillColor = tema.Color("neutral-900", v)
	r.h.fondo.Refresh()
	r.h.texto.Text = r.h.Rotulo
	r.h.texto.Color = tema.Color("neutral-400", v)
	r.h.texto.Refresh()
	r.h.previa.Hidden = r.h.Imagen == nil
	r.Layout(r.h.Size())
}

// ---- La antesala ----

// Medidas de la antesala. HOJA (`PreJoin.tsx`): la pantalla no la montaba el barrido,
// porque para medirla hay que estar en una llamada.
var (
	// AnchoAntesala es el `max-w-3xl` del bloque centrado: 48rem con el rem de 14.
	AnchoAntesala float32 = 48 * tema.Rem
	// AnchoAntesalaVideo es la columna `3fr` de su `md:grid-cols-[3fr_2fr]` con el
	// `gap-4` en medio.
	AnchoAntesalaVideo = (AnchoAntesala - tema.Espacio(4)) * 3 / 5
	// CanalonAntesala es su `p-4` y HuecoAntesala el `gap-4`/`gap-3` de las rejillas.
	CanalonAntesala = tema.Espacio(4)
	HuecoAntesala   = tema.Espacio(4)
	HuecoColumna    = tema.Espacio(3)
)

// Antesala es el `PreJoin`: lo que se ve antes de entrar en una llamada.
//
// Los interruptores son BOTONES CON TEXTO y no iconos, y eso es de la web: el rótulo
// dice el ESTADO con el que se entra —«Micrófono activado»— y no la acción. Un icono de
// micrófono tachado no distingue «está apagado» de «apágalo».
//
// ponytail: sin el medidor de nivel que sube al hablar y sin los tres selectores de
// dispositivo: se entra con el micrófono y el altavoz por omisión del sistema. Van en
// el roadmap («Llamadas nativas»).
type Antesala struct {
	widget.BaseWidget

	T *i18n.Translator

	// Titulo es el nombre de la sala.
	Titulo string
	// Micro y Camara son el estado con el que se va a entrar.
	Micro, Camara bool
	// Denegado es el permiso rechazado por el sistema: se dice sobre el hueco, que es
	// donde la web lo pone.
	Denegado bool
	// Entrando es «Entrar» pulsado y esperando al servicio y al SFU.
	Entrando bool
	// Aviso es lo que se dice bajo el subtítulo —la puerta echada, «ya lo sabe», no se
	// pudo conectar—; con RotuloAccion, lleva su botón («Pedir entrar»).
	Aviso        string
	RotuloAccion string
	AlAccion     func()

	AlCambiarMicro  func()
	AlCambiarCamara func()
	// SinCamara es no tener cómo abrirla: el interruptor se queda apagado e inerte.
	SinCamara bool
	// Nivel es el del micrófono entre 0 y 1: la barra que sube al hablar, que dice que
	// el micro CAPTA antes de entrar (la mitad de los «no se me oye» de una reunión).
	Nivel float32
	// Micros, Camaras y Altavoces son los dispositivos que se pueden elegir, con el
	// elegido de cada uno (vacío: el del sistema). Sin altavoces no sale su selector,
	// como en la web cuando el navegador no sabe cambiarlo.
	Micros, Camaras, Altavoces    []Opcion
	MicroID, CamaraID, AltavozID  string
	AlElegirMicro, AlElegirCamara func(id string)
	AlElegirAltavoz               func(id string)

	hueco        *HuecoVideo
	previa       image.Image
	barraNivel   *canvas.Rectangle
	anchoMedidor float32
	AlEntrar     func()
	AlCancelar   func()

	fondo   *canvas.Rectangle
	cuerpo  *fyne.Container
	objetos []fyne.CanvasObject
}

var _ fyne.Widget = (*Antesala)(nil)

// NuevaAntesala crea la antesala con su traductor.
func NuevaAntesala(t *i18n.Translator, titulo string) *Antesala {
	a := &Antesala{T: t, Titulo: titulo, Micro: true, Camara: true}
	a.ExtendBaseWidget(a)
	return a
}

// AccessibilityLabel es el nombre de la sala.
func (a *Antesala) AccessibilityLabel() string { return a.Titulo }

// PonerPrevia cambia el fotograma de la previa de la cámara (nil, ninguna).
func (a *Antesala) PonerPrevia(img image.Image) {
	a.previa = img
	if a.hueco != nil {
		a.hueco.PonerImagen(img)
	}
}

// Previa es el fotograma de la previa, para los guardianes.
func (a *Antesala) Previa() image.Image { return a.previa }

// AccessibilityRole implementa fyne.Accessible.
func (a *Antesala) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// rotuloHueco es lo que dice el hueco del vídeo según el estado.
func (a *Antesala) rotuloHueco() string {
	switch {
	case a.Denegado:
		return a.T.T("meet.prejoin.denied", nil)
	case !a.Camara:
		return a.T.T("meet.prejoin.cameraOff", nil)
	}
	// Con la cámara pedida y el permiso dado, lo que falta es la captura: se dice, no
	// se finge una previa que no hay.
	return a.T.T("meet.connecting", nil)
}

// CreateRenderer compone la antesala.
func (a *Antesala) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	a.fondo = canvas.NewRectangle(tema.Color("neutral-1000", v))
	a.componer()
	r := &antesalaRenderer{a: a}
	r.Refresh()
	return r
}

// componer rehace el bloque: los rótulos de los interruptores, el aviso y «Entrar»
// cambian con el estado.
func (a *Antesala) componer() {
	t := a.T
	titulo := ui.NuevoRotulo(a.Titulo, tema.TextoLG, ui.SemiNegrita, "neutral-0")
	sub := ui.NuevoRotulo(t.T("meet.prejoin.subtitle", nil), tema.TextoSM, ui.Normal, "neutral-400")

	claveMicro, claveCamara := "meet.prejoin.micOff", "meet.prejoin.camOff"
	varMicro, varCamara := ui.Peligro, ui.Peligro
	if a.Micro {
		claveMicro, varMicro = "meet.prejoin.micOn", ui.Secundario
	}
	if a.Camara {
		claveCamara, varCamara = "meet.prejoin.camOn", ui.Secundario
	}
	botonCamara := ui.NuevoBoton(t.T(claveCamara, nil), a.AlCambiarCamara).Con(varCamara, ui.TallaSM)
	botonCamara.Inerte = a.SinCamara
	interruptores := container.New(ui.Fila{Hueco: tema.Espacio(2)},
		ui.NuevoBoton(t.T(claveMicro, nil), a.AlCambiarMicro).Con(varMicro, ui.TallaSM),
		botonCamara,
	)
	unirse := ui.NuevoBoton(t.T("meet.prejoin.join", nil), a.AlEntrar).Con(ui.Primario, ui.TallaMD)
	unirse.Cargando, unirse.Inerte = a.Entrando, a.Entrando
	pie := container.New(ui.Fila{Hueco: tema.Espacio(2)},
		ui.NuevoBoton(t.T("meet.prejoin.cancel", nil), a.AlCancelar).Con(ui.Secundario, ui.TallaMD),
		unirse,
	)
	filas := []fyne.CanvasObject{titulo, sub}
	if a.Aviso != "" {
		aviso := []fyne.CanvasObject{ui.NuevoRotulo(a.Aviso, tema.TextoSM, ui.Normal, "neutral-0")}
		if a.RotuloAccion != "" {
			aviso = append(aviso, ui.NuevoBoton(a.RotuloAccion, a.AlAccion).Con(ui.Secundario, ui.TallaSM))
		}
		filas = append(filas, container.New(ui.Fila{Hueco: tema.Espacio(2)}, aviso...))
	}
	a.hueco = NuevoHuecoVideo(a.rotuloHueco())
	a.hueco.Imagen = a.previa
	// La columna de la derecha: interruptores, el medidor y los selectores (`flex
	// flex-col gap-3`).
	derecha := []fyne.CanvasObject{interruptores, a.medidor()}
	derecha = append(derecha,
		SelectorDispositivo(t, t.T("meet.prejoin.microphone", nil), a.Micros, a.MicroID, "neutral-300", a.AlElegirMicro),
		SelectorDispositivo(t, t.T("meet.prejoin.camera", nil), a.Camaras, a.CamaraID, "neutral-300", a.AlElegirCamara))
	if len(a.Altavoces) > 0 {
		derecha = append(derecha,
			SelectorDispositivo(t, t.T("meet.prejoin.speaker", nil), a.Altavoces, a.AltavozID, "neutral-300", a.AlElegirAltavoz))
	}
	rejilla := container.New(&rejillaAntesala{}, a.hueco, container.New(ui.Columna{Hueco: HuecoColumna}, derecha...))
	filas = append(filas, rejilla, container.New(alFinal{}, pie))
	a.cuerpo = container.New(ui.Columna{Hueco: HuecoColumna}, filas...)
	a.objetos = []fyne.CanvasObject{a.fondo, a.cuerpo}
}

// Medidas del medidor: `h-1.5 rounded-full bg-neutral-800` con la barra `bg-primary`.
var AltoMedidor = tema.Espacio(1.5)

// medidor es la barra del nivel del micrófono. Decorativa para el lector de pantalla,
// como el `aria-hidden` de la web: el dato útil ya lo da el estado del micrófono.
func (a *Antesala) medidor() fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := canvas.NewRectangle(tema.Color("neutral-800", v))
	fondo.CornerRadius = AltoMedidor / 2
	barra := canvas.NewRectangle(tema.Color("primary", v))
	barra.CornerRadius = AltoMedidor / 2
	a.barraNivel = barra
	return container.New(&capaMedidor{a: a}, fondo, barra)
}

// PonerNivel mueve la barra sin rehacer la antesala: llega 15 veces por segundo.
func (a *Antesala) PonerNivel(n float32) {
	a.Nivel = min(max(n, 0), 1)
	if a.barraNivel != nil {
		a.barraNivel.Resize(fyne.NewSize(a.anchoMedidor*a.Nivel, AltoMedidor))
		a.barraNivel.Refresh()
	}
}

// capaMedidor coloca el fondo a todo el ancho y la barra a su parte.
type capaMedidor struct{ a *Antesala }

func (c *capaMedidor) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, AltoMedidor) }
func (c *capaMedidor) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	c.a.anchoMedidor = t.Width
	objs[0].Resize(fyne.NewSize(t.Width, AltoMedidor))
	objs[1].Resize(fyne.NewSize(t.Width*c.a.Nivel, AltoMedidor))
}

// rejillaAntesala es `grid gap-4 md:grid-cols-[3fr_2fr]`: el vídeo a la izquierda y la
// columna a la derecha desde `md`; por debajo, uno encima del otro.
type rejillaAntesala struct{}

func (r *rejillaAntesala) columnas(ancho float32) (float32, float32, bool) {
	if ancho < ui.Estrecho-2*CanalonAntesala {
		return ancho, ancho, false
	}
	izq := (ancho - HuecoAntesala) * 3 / 5
	return izq, ancho - HuecoAntesala - izq, true
}

func (r *rejillaAntesala) MinSize(objs []fyne.CanvasObject) fyne.Size {
	hv, hd := MedidaHuecoVideo(AnchoAntesalaVideo).Height, objs[1].MinSize().Height
	if _, _, lado := r.columnas(AnchoAntesala); lado {
		return fyne.NewSize(0, max(hv, hd))
	}
	return fyne.NewSize(0, hv+HuecoAntesala+hd)
}

func (r *rejillaAntesala) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	izq, der, lado := r.columnas(t.Width)
	hv := MedidaHuecoVideo(izq).Height
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(izq, hv))
	if lado {
		objs[1].Move(fyne.NewPos(izq+HuecoAntesala, 0))
		objs[1].Resize(fyne.NewSize(der, objs[1].MinSize().Height))
		return
	}
	objs[1].Move(fyne.NewPos(0, hv+HuecoAntesala))
	objs[1].Resize(fyne.NewSize(der, objs[1].MinSize().Height))
}

type antesalaRenderer struct{ a *Antesala }

// MinSize es el bloque centrado con su `p-4`.
func (r *antesalaRenderer) MinSize() fyne.Size {
	m := r.a.cuerpo.MinSize()
	return fyne.NewSize(AnchoAntesala+2*CanalonAntesala, m.Height+2*CanalonAntesala)
}

func (r *antesalaRenderer) Objects() []fyne.CanvasObject { return r.a.objetos }
func (r *antesalaRenderer) Destroy()                     {}

// Layout centra el bloque en los dos ejes (`grid place-items-center p-4`).
func (r *antesalaRenderer) Layout(t fyne.Size) {
	a := r.a
	a.fondo.Resize(t)
	ancho := min(AnchoAntesala, max(t.Width-2*CanalonAntesala, 0))
	alto := a.cuerpo.MinSize().Height
	a.cuerpo.Move(fyne.NewPos((t.Width-ancho)/2, max((t.Height-alto)/2, CanalonAntesala)))
	a.cuerpo.Resize(fyne.NewSize(ancho, alto))
}

// Refresh repinta el fondo y rehace la disposición.
func (r *antesalaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.a.componer()
	r.a.fondo.FillColor = tema.Color("neutral-1000", v)
	r.a.fondo.Refresh()
	r.Layout(r.a.Size())
}
