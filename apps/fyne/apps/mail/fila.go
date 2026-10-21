// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Por qué esta fila no es `ui.FilaLista`
//
// `ui.FilaLista` ES el `ListItem` de la suite y está clavado: 33,25 sin subtítulo, 49
// con él y 54 con avatar md. La fila de Sytue Mail mide **75,75** porque su
// «subtítulo» son DOS bloques apilados —el asunto y una tira de 24 donde se relevan el
// extracto y el triaje— más el `gap-0.5` entre ellos. Usar el `ListItem` genérico aquí
// daría 54: 21,75 px menos por fila, o sea una pantalla y media de desvío en cincuenta
// correos.
//
// Es la misma distinción que el paquete `ui` ya hace entre `FilaLista` y `FilaDrive`.
// Lo compartido —avatar, casilla, truncado, colocación del texto— se importa.
//
// # Las medidas, del DOM de producción (1280×800, rem 14)
//
//	li            75,75 × 279, `padding 7px 10.5px`, `border-b` de 1, `border-l-2`
//	delantero     casilla 16 · gap-1 · punto 8 · gap-1 · avatar 40 = 72, y el texto
//	              empieza en x = 260 + 2 + 12 + 72 + 12 = 358 (raíz actual 16)
//	texto         19,25 (remitente) + 15,75 (asunto) + 1,75 (gap-0.5) + 24 (tira)
//	              = 60,75, que ES el alto de contenido: 60,75 + 14 + 1 = 75,75
//	meta          derecha en x = 528,5; fecha `text-xs` (14 de caja) centrada
const (
	// MargenFila es `px-3` y RellenoFila es `py-2`.
	MargenFila  float32 = 3 * 0.25 * tema.Rem
	RellenoFila float32 = 2 * 0.25 * tema.Rem
	// HuecoBloque es el `gap-3` entre delantero, texto y meta.
	HuecoBloque float32 = 3 * 0.25 * tema.Rem
	// HuecoDelantero es el `gap-1` de dentro del delantero (casilla · punto · avatar).
	HuecoDelantero float32 = 1 * 0.25 * tema.Rem
	// PuntoSinLeer es el `h-2 w-2` del punto de no leído.
	PuntoSinLeer float32 = 2 * 0.25 * tema.Rem
	// TiraAcciones es el `h-6 min-h-[24px]` donde se relevan el extracto y el triaje.
	// Es un PÍXEL literal de la hoja (`min-h-[24px]`), no un paso de la escala.
	TiraAcciones float32 = 24
	// HuecoLineas es el `gap-0.5` entre el asunto y la tira.
	HuecoLineas float32 = 0.5 * 0.25 * tema.Rem
	// HuecoMeta es el `gap-1` entre la fecha y el clip.
	HuecoMeta float32 = 1 * 0.25 * tema.Rem
	// TallaClip es el `size={14}` del clip de adjunto, y TallaEstrella el `size={16}`
	// de la estrella que acompaña al remitente.
	TallaClip     float32 = 14
	TallaEstrella float32 = 16
)

// AltoTextoFila es el bloque de tres líneas: es lo que decide el alto de la fila, y no
// el avatar (40) ni la casilla.
var AltoTextoFila = tema.TextoBase.Linea + tema.TextoSM.Linea + HuecoLineas + TiraAcciones

// AltoFilaConversacion es 75,75 MEDIDO en Carril. Es la única cuenta: el `MinSize()` del
// renderer la llama y el guardián compara contra ella Y contra los píxeles. Es una
// función porque el relleno de la fila es de la FORMA (`ui.RellenoYFila`): en Marco la
// fila de `ListItem` mide un píxel más por arriba y por abajo.
func AltoFilaConversacion() float32 {
	return 2*ui.RellenoYFila() + max(AltoTextoFila, ui.MedidaAvatar(ui.AvatarMD).Height) + tema.Borde
}

// Las acciones de triaje de la fila (`RowAction` de MessageList.tsx): botones
// fantasma de SOLO ICONO en talla `xs` —24, el suelo de WCAG 2.5.8—, glifo de 16 y
// `gap-0.5` entre ellos, dentro de la tira de 24 donde se relevan con el extracto.
const (
	HuecoTriaje      float32 = 0.5 * 0.25 * tema.Rem
	TallaGlifoTriaje float32 = 16
	// altoTiraConChips es el `gap-0.5` de la columna más el `mt-0.5` de los chips.
	altoTiraConChips float32 = 2 * HuecoLineas
	// rellenoChipArriba es `py-px`: un píxel literal, no un paso de la escala.
	rellenoChipArriba float32 = 1
)

var (
	LadoBotonTriaje = ui.MedidaBotonCon("", ui.TallaXS, 0, 1, true, TallaGlifoTriaje).Width
	lineaChip       = 1.25 * tema.Texto2XS.Tam // `leading-tight`
	escalonChip     = tema.Escalon{Tam: tema.Texto2XS.Tam, Linea: lineaChip}
)

// Los chips de etiqueta de la fila: `mt-0.5 flex flex-wrap items-center gap-1`, y cada
// uno `rounded-pill border px-1.5 py-px text-2xs leading-tight gap-1 max-w-[10rem]`
// con su punto de `h-1.5 w-1.5`. Hoja: producción no tenía etiquetas puestas el día de
// la medida.
var AltoChip = lineaChip + 2*rellenoChipArriba + 2*tema.Borde

const (
	MargenChip float32 = 1.5 * 0.25 * tema.Rem
	PuntoChip  float32 = 1.5 * 0.25 * tema.Rem
	HuecoChip  float32 = 1 * 0.25 * tema.Rem
	MaxChip    float32 = 10 * tema.Rem
)

// anchoChip es lo que ocupa un chip con su nombre entero, antes del tope.
func anchoChip(nombre string) float32 {
	return 2*tema.Borde + 2*MargenChip + PuntoChip + HuecoChip + ui.Medir(nombre, escalonChip, ui.Normal).Width
}

// ColocarChips reparte los chips con `flex-wrap`: de izquierda a derecha y a la
// línea siguiente cuando no caben. Devuelve la esquina y el ancho de cada uno (ya con
// el `max-w-[10rem]` y el de la columna) y el alto total. Es la ÚNICA cuenta: la usan
// la medida de la fila y su layout.
func ColocarChips(chips []Etiqueta, ancho float32) (pos []fyne.Position, anchos []float32, alto float32) {
	if len(chips) == 0 {
		return nil, nil, 0
	}
	x, y := float32(0), float32(0)
	for i, e := range chips {
		w := min(anchoChip(e.Nombre), MaxChip, max(ancho, 0))
		if i > 0 && x+w > ancho {
			x, y = 0, y+AltoChip+HuecoChip
		}
		pos = append(pos, fyne.NewPos(x, y))
		anchos = append(anchos, w)
		x += w + HuecoChip
	}
	return pos, anchos, y + AltoChip
}

// AltoFilaCon es el alto de una fila con sus chips a ese ancho de columna de texto: la
// fila de siempre y, si hay etiquetas, el `gap-0.5`, el `mt-0.5` y las líneas de chips.
// Sin etiquetas es exactamente AltoFilaConversacion, los 75,75 medidos.
func AltoFilaCon(chips []Etiqueta, anchoTexto float32) float32 {
	if len(chips) == 0 {
		return AltoFilaConversacion()
	}
	_, _, alto := ColocarChips(chips, anchoTexto)
	return AltoFilaConversacion() + altoTiraConChips + alto
}

// AnchoTextoFila es la columna `flex-1 min-w-0` de una fila de ese ancho: lo que dejan
// el delantero, los dos `gap-3` y la meta, que es `shrink-0` y manda.
func AnchoTextoFila(anchoFila float32, fecha string, conAdjunto, sinLeer bool) float32 {
	peso := ui.Normal
	if sinLeer {
		peso = ui.Negrita
	}
	meta := max(ui.Medir(fecha, tema.TextoXS, peso).Width, adjuntoAncho(conAdjunto))
	izquierda := ui.InicioFila() + ui.LadoEleccion + 2*HuecoDelantero + PuntoSinLeer +
		ui.MedidaAvatar(ui.AvatarMD).Width + HuecoBloque
	return max(anchoFila-MargenFila-meta-HuecoBloque-izquierda, 0)
}

// coloresEtiqueta es `LABEL_COLORS` de `components/utils.ts`: TOKEN → color del PUNTO.
// Son hex literales de la app —tienen que coincidir con `domain.LabelColors` del
// backend— y por eso NO salen de `tema.Color`, que es la paleta del cromo.
var coloresEtiqueta = map[string]color.NRGBA{
	"slate": {R: 0x5b, G: 0x6b, B: 0x7b, A: 255},
	"brass": {R: 0x9a, G: 0x7b, B: 0x3f, A: 255},
	"sage":  {R: 0x5f, G: 0x7a, B: 0x5a, A: 255},
	"clay":  {R: 0xa4, G: 0x62, B: 0x3f, A: 255},
	"blue":  {R: 0x3f, G: 0x66, B: 0x99, A: 255},
	"plum":  {R: 0x7a, G: 0x54, B: 0x80, A: 255},
	"teal":  {R: 0x3f, G: 0x7d, B: 0x7a, A: 255},
	"rose":  {R: 0x9c, G: 0x54, B: 0x70, A: 255},
}

// ColorEtiqueta es `labelDot`: el token de color de una etiqueta a su hex. Uno
// desconocido cae a `border-strong`, que es el respaldo que la web escribe en el
// propio `var()`.
func ColorEtiqueta(token string, v tema.Variante) color.NRGBA {
	if c, hay := coloresEtiqueta[token]; hay {
		return c
	}
	return tema.Color("border-strong", v)
}

// FilaConversacion es una fila de la bandeja.
//
// Va envuelta en un `fyne.Widget` y no en un contenedor porque se PULSA, se anuncia y
// tiene hover: sin la cáscara, `walkObjectTree` no le entrega el clic y el lector de
// pantalla dice «Container» en vez del asunto del correo.
type FilaConversacion struct {
	widget.BaseWidget

	// Conv es lo que pinta. Se cambia con Poner, que es lo que llama el reciclado.
	Conv Conversacion
	// Remitente y Fecha llegan YA escritos: el nombre y el formato de fecha dependen
	// del idioma, y el idioma es de la app, no de un widget.
	Remitente, Asunto, Fecha string
	// Abierta es la conversación que el lector está enseñando (`aria-selected`).
	Abierta bool
	// Marcada es la casilla de multiselección.
	Marcada bool
	// ConCasilla la esconde en una vista de solo lectura, donde no hay lote.
	ConCasilla bool
	// SeleccionActiva es que hay ALGUNA fila marcada: entonces las casillas de todas se
	// ven sin pasar por encima (`selectionActive` de la web).
	SeleccionActiva bool
	// SinLeerTexto es el prefijo que solo oye un lector de pantalla («No leído.»).
	SinLeerTexto string
	// T traduce los nombres accesibles del triaje. Sin él no hay triaje.
	T *i18n.Translator
	// Acciones son las del triaje; nil en solo lectura, que no tiene tira.
	Acciones *Acciones
	// Chips son las etiquetas del registro que lleva el hilo (`conversationLabels`).
	Chips []Etiqueta

	AlAbrir  func()
	AlMarcar func()
	// AlDesplazarV es la rueda vertical que cae sobre la tira de triaje: es de la
	// lista, como en el navegador. La pone la bandeja.
	AlDesplazarV func(*fyne.ScrollEvent)

	// dentro cuenta los objetos de la fila que tienen el puntero encima: la fila y
	// sus botones de triaje. Ver `botonTriaje`.
	dentro int
}

var (
	_ fyne.Widget       = (*FilaConversacion)(nil)
	_ fyne.Tappable     = (*FilaConversacion)(nil)
	_ desktop.Hoverable = (*FilaConversacion)(nil)
	_ fyne.Accessible   = (*FilaConversacion)(nil)
)

// NuevaFilaConversacion crea una fila vacía, que es lo que pide el reciclado de
// `widget.List`: se construye una vez y se rellena con Poner.
func NuevaFilaConversacion() *FilaConversacion {
	f := &FilaConversacion{ConCasilla: true}
	f.ExtendBaseWidget(f)
	return f
}

// Poner recicla la fila con otra conversación. Es el `UpdateItem` de la lista.
//
// El hover se suelta SOLO si la fila pasa a ser de otra conversación: la lista la
// vuelve a rellenar también tras un triaje, con el puntero todavía encima, y en la web
// la tira no desaparece por destacar un correo.
func (f *FilaConversacion) Poner(c Conversacion, remitente, asunto, fecha string) {
	if c.HiloID != f.Conv.HiloID {
		f.dentro = 0
	}
	f.Conv, f.Remitente, f.Asunto, f.Fecha = c, remitente, asunto, fecha
	f.Refresh()
}

// Tapped abre la conversación. La casilla NO se reparte aquí: es una `ui.Eleccion` de
// verdad dentro del árbol y Fyne entrega el clic al objeto más profundo, así que
// marcar no abre —que es lo que en la web hace el `stopPropagation`—.
func (f *FilaConversacion) Tapped(*fyne.PointEvent) {
	if f.AlAbrir != nil {
		f.AlAbrir()
	}
}

// MouseIn es `:hover`, que en la web solo se pinta si la fila NO está elegida.
func (f *FilaConversacion) MouseIn(*desktop.MouseEvent) { f.pasar(1) }

// MouseMoved no hace nada: el estado lo fijan la entrada y la salida.
func (f *FilaConversacion) MouseMoved(*desktop.MouseEvent) {}

// MouseOut devuelve la fila a su color de reposo.
func (f *FilaConversacion) MouseOut() { f.pasar(-1) }

// pasar lleva la cuenta del puntero. Fyne entrega el hover a UN solo objeto —el más
// profundo que lo acepte—, así que al pasar de la fila a un botón de su tira la fila
// recibe `MouseOut` y el botón `MouseIn`, en ese orden y antes de pintar. Con un
// booleano la tira se escondía justo cuando el puntero llegaba a ella; con la cuenta,
// la fila sigue «encima» mientras el puntero esté en ella o en cualquiera de sus
// botones, que es el `group-hover` de la web.
func (f *FilaConversacion) pasar(d int) {
	f.dentro = max(f.dentro+d, 0)
	f.Refresh()
}

func (f *FilaConversacion) encima() bool { return f.dentro > 0 }

// AccessibilityLabel es lo que la web anuncia: el prefijo de no leído, el remitente y
// el asunto. Sin el prefijo, un lector de pantalla no distingue leído de no leído,
// porque en pantalla eso lo dice el PESO de la letra y un punto de color.
func (f *FilaConversacion) AccessibilityLabel() string {
	s := ""
	if f.Conv.SinLeer && f.SinLeerTexto != "" {
		s = f.SinLeerTexto + " "
	}
	return s + f.Remitente + " — " + f.Asunto
}

// AccessibilityRole es `option` dentro de un listbox, que es lo que emite la web.
// Fyne 2.8.1 solo conoce cuatro roles y `option` no está: se anuncia como botón.
func (f *FilaConversacion) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleButton
}

// botonTriaje es un `RowAction`: el `ui.Boton` de siempre que además le cuenta a su
// fila cuándo entra y sale el puntero (ver `pasar`). Es extender el widget —la forma
// de Fyne de añadir comportamiento a uno ya hecho—, no copiarlo: se pinta, se mide y se
// enfoca con el código de `ui`.
type botonTriaje struct {
	ui.Boton
	fila *FilaConversacion
	// d es la ruta del glifo y relleno si va lleno (la estrella de un destacado).
	d       string
	relleno bool
	token   string
}

var _ desktop.Hoverable = (*botonTriaje)(nil)

func nuevoBotonTriaje(f *FilaConversacion, d string, relleno bool, token string, alPulsar func()) *botonTriaje {
	b := &botonTriaje{fila: f, d: d, relleno: relleno, token: token}
	b.Variante, b.Talla, b.SoloIcono, b.TamIcono = ui.Fantasma, ui.TallaXS, true, TallaGlifoTriaje
	b.AlPulsar = alPulsar
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if relleno {
		b.Glifo = GlifoRelleno(d, TallaGlifoTriaje, token, v)
	} else {
		b.Glifo = Glifo(d, TallaGlifoTriaje, token, v)
	}
	b.ExtendBaseWidget(b)
	return b
}

// MouseIn le avisa a la fila antes de encender su propio velo.
func (b *botonTriaje) MouseIn(e *desktop.MouseEvent) { b.Boton.MouseIn(e); b.fila.pasar(1) }

// MouseOut, igual al salir.
func (b *botonTriaje) MouseOut() { b.Boton.MouseOut(); b.fila.pasar(-1) }

// CreateRenderer compone la fila con primitivas y widgets de la suite.
func (f *FilaConversacion) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaRenderer{
		f: f,
		// El opaco de debajo no es decoración: dentro de un `widget.List` cada fila va
		// sobre un `listItem` de Fyne que pinta su propio fondo con el tema de serie.
		base:     canvas.NewRectangle(tema.Color("surface", v)),
		velo:     canvas.NewRectangle(color.NRGBA{}),
		galon:    canvas.NewRectangle(color.NRGBA{}),
		filete:   canvas.NewRectangle(tema.Color("border", v)),
		punto:    canvas.NewCircle(color.NRGBA{}),
		avatar:   ui.NuevoAvatar(""),
		nombre:   ui.Texto("", tema.TextoBase, ui.Normal, tema.Color("text", v)),
		asunto:   ui.Texto("", tema.TextoSM, ui.Normal, tema.Color("text-muted", v)),
		extracto: ui.Texto("", tema.TextoSM, ui.Normal, tema.Color("text-subtle", v)),
		fecha:    ui.Texto("", tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
		cuantos:  ui.Texto("", tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
		estrella: GlifoRelleno(EstrellaD, TallaEstrella, "warning-text", v),
		clip:     Glifo(ClipD, TallaClip, "text-subtle", v),
	}
	r.casilla = ui.NuevaCasilla("", func(bool) {
		if f.AlMarcar != nil {
			f.AlMarcar()
		}
	})
	r.casilla.SinEtiqueta = true
	con := func(h func(*Acciones) func(Conversacion)) func() {
		return func() {
			if f.Acciones != nil {
				if accion := h(f.Acciones); accion != nil {
					accion(f.Conv)
				}
			}
		}
	}
	destacar := con(func(a *Acciones) func(Conversacion) { return a.Destacar })
	leido := con(func(a *Acciones) func(Conversacion) { return a.Leido })
	// El orden de la web: estrella, leído, archivar, papelera, spam y «no es spam».
	// Los conmutadores son DOS botones cada uno y se enseña el que toca: el glifo
	// cambia de dibujo (lleno o no, sobre abierto o cerrado) y no solo de color.
	r.triaje = []*botonTriaje{
		nuevoBotonTriaje(f, EstrellaD, false, "text", destacar),
		nuevoBotonTriaje(f, EstrellaD, true, "warning-text", destacar),
		nuevoBotonTriaje(f, LeidoD, false, "text", leido),
		nuevoBotonTriaje(f, SinLeerD, false, "text", leido),
		nuevoBotonTriaje(f, ArchivarD, false, "text", con(func(a *Acciones) func(Conversacion) { return a.Archivar })),
		nuevoBotonTriaje(f, PapeleraD, false, "text", con(func(a *Acciones) func(Conversacion) { return a.Papelera })),
		nuevoBotonTriaje(f, SpamD, false, "text", con(func(a *Acciones) func(Conversacion) { return a.Spam })),
		nuevoBotonTriaje(f, SpamD, false, "text", con(func(a *Acciones) func(Conversacion) { return a.NoSpam })),
	}
	botones := make([]fyne.CanvasObject, 0, len(r.triaje))
	for _, b := range r.triaje {
		botones = append(botones, b)
	}
	r.tira = ui.NuevaTiraDeslizable(LadoBotonTriaje, HuecoTriaje, botones...)
	r.tira.AlDesplazarV = func(e *fyne.ScrollEvent) {
		if f.AlDesplazarV != nil {
			f.AlDesplazarV(e)
		}
	}
	r.montar()
	r.Refresh()
	return r
}

// chipFila es un chip de etiqueta: la píldora, su punto y el nombre.
type chipFila struct {
	caja   *ui.Caja
	punto  *canvas.Circle
	nombre *canvas.Text
}

type filaRenderer struct {
	f                         *FilaConversacion
	base, velo, galon, filete *canvas.Rectangle
	punto                     *canvas.Circle
	casilla                   *ui.Eleccion
	avatar                    *ui.Avatar
	nombre, asunto, extracto  *canvas.Text
	fecha, cuantos            *canvas.Text
	estrella, clip            *iconos.Dibujo
	triaje                    []*botonTriaje
	tira                      *ui.TiraDeslizable
	chips                     []chipFila
	objetos                   []fyne.CanvasObject
}

// montar arma la lista de objetos en el orden en que se pintan. Se rehace cuando
// cambia el número de chips, que es lo único variable de la fila.
func (r *filaRenderer) montar() {
	r.objetos = []fyne.CanvasObject{
		r.base, r.velo, r.galon, r.casilla, r.punto, r.avatar,
		r.nombre, r.cuantos, r.estrella.Objeto(), r.asunto, r.extracto,
		r.fecha, r.clip.Objeto(), r.filete,
	}
	r.objetos = append(r.objetos, r.tira)
	for _, c := range r.chips {
		r.objetos = append(r.objetos, c.caja.Objetos()...)
		r.objetos = append(r.objetos, c.punto, c.nombre)
	}
}

// MinSize es la que decide el alto de TODAS las filas del `widget.List`: la lista toma
// el MinSize del objeto que devuelve su `CreateItem` y lo aplica a todas. Una fila con
// etiquetas crece, y ese alto se lo pone la bandeja con `SetItemHeight`.
func (r *filaRenderer) MinSize() fyne.Size {
	ancho := ui.InicioFila() + MargenFila + ui.LadoEleccion + 2*HuecoDelantero +
		PuntoSinLeer + ui.MedidaAvatar(ui.AvatarMD).Width + 2*HuecoBloque
	return fyne.NewSize(ancho, AltoFilaConversacion())
}

func (r *filaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaRenderer) Destroy()                     {}

func (r *filaRenderer) Layout(t fyne.Size) {
	f := r.f
	r.base.Resize(t)
	r.base.Move(fyne.NewPos(0, 0))
	// El `border-b` va DENTRO de la caja (`box-sizing: border-box`), y el `border-l`
	// ocupa su ancho a la izquierda. El velo y la barra, con la sangría y el radio de
	// la forma.
	r.filete.Resize(fyne.NewSize(t.Width, tema.Borde))
	r.filete.Move(fyne.NewPos(0, t.Height-tema.Borde))
	ui.ColocarVeloFila(r.velo, r.galon, t.Width, t.Height-tema.Borde)

	contenido := t.Height - tema.Borde
	centro := func(alto float32) float32 { return (contenido - alto) / 2 }

	x := ui.InicioFila()
	// La casilla ocupa sitio SIEMPRE aunque no se vea: en la web es `opacity-0`, no
	// `hidden`, y si desapareciera el avatar se movería 21 px al pasar el ratón.
	r.casilla.Resize(fyne.NewSize(ui.LadoEleccion, ui.LadoEleccion))
	r.casilla.Move(fyne.NewPos(x, centro(ui.LadoEleccion)))
	x += ui.LadoEleccion + HuecoDelantero
	r.punto.Resize(fyne.NewSize(PuntoSinLeer, PuntoSinLeer))
	r.punto.Move(fyne.NewPos(x, centro(PuntoSinLeer)))
	x += PuntoSinLeer + HuecoDelantero
	lado := ui.MedidaAvatar(ui.AvatarMD)
	r.avatar.Resize(lado)
	r.avatar.Move(fyne.NewPos(x, centro(lado.Height)))
	x += lado.Width + HuecoBloque

	// La meta es `shrink-0`: nunca encoge, encoge la columna de texto.
	anchoFecha := ui.Medir(f.Fecha, tema.TextoXS, r.peso()).Width
	altoMeta := tema.TextoXS.Linea
	if f.Conv.ConAdjunto {
		altoMeta += HuecoMeta + TallaClip
	}
	yMeta := centro(altoMeta)
	derecha := t.Width - MargenFila
	ui.ColocarTexto(r.fecha, tema.TextoXS, derecha-anchoFecha, yMeta)
	if f.Conv.ConAdjunto {
		// `items-end`: el clip se pega a la derecha, debajo de la fecha.
		r.clip.Colocar(derecha-TallaClip, yMeta+tema.TextoXS.Linea+HuecoMeta, TallaClip)
	}
	derecha -= max(anchoFecha, adjuntoAncho(f.Conv.ConAdjunto)) + HuecoBloque

	// La columna de texto es `flex-1 min-w-0`: lo que queda, y con truncado. Va
	// centrada como bloque (`items-center` del `li`), que solo se nota con chips.
	disponible := max(derecha-x, 0)
	_, _, altoChips := ColocarChips(f.Chips, disponible)
	bloque := AltoTextoFila
	if len(f.Chips) > 0 {
		bloque += altoTiraConChips + altoChips
	}
	y := centro(bloque)
	// El remitente comparte línea con el contador de mensajes del hilo y la estrella,
	// los dos `shrink-0`: se reservan a la derecha y el nombre se trunca con el resto.
	reservado := float32(0)
	if f.Conv.Total > 1 {
		reservado += tema.Espacio(1) + ui.Medir(r.cuantos.Text, tema.TextoXS, ui.Normal).Width
	}
	if f.Conv.Destacada {
		reservado += tema.Espacio(1) + TallaEstrella
	}
	anchoNombre := ui.Truncar(r.nombre, f.Remitente, tema.TextoBase, r.peso(), disponible-reservado)
	ui.ColocarTexto(r.nombre, tema.TextoBase, x, y)
	nx := x + anchoNombre + tema.Espacio(1) // `gap-1` de la línea del remitente
	if f.Conv.Total > 1 {
		ui.ColocarTexto(r.cuantos, tema.TextoXS, nx, y+(tema.TextoBase.Linea-tema.TextoXS.Linea)/2)
		nx += ui.Medir(r.cuantos.Text, tema.TextoXS, ui.Normal).Width + tema.Espacio(1)
	}
	if f.Conv.Destacada {
		r.estrella.Colocar(nx, y+(tema.TextoBase.Linea-TallaEstrella)/2, TallaEstrella)
	}
	y += tema.TextoBase.Linea

	ui.Truncar(r.asunto, f.Asunto, tema.TextoSM, r.peso(), disponible)
	ui.ColocarTexto(r.asunto, tema.TextoSM, x, y)
	y += tema.TextoSM.Linea + HuecoLineas

	// El extracto va CENTRADO en la tira de 24, que es donde se releva con los botones
	// de triaje al pasar por encima.
	ui.Truncar(r.extracto, f.Conv.Ultimo.Extracto, tema.TextoSM, r.peso(), disponible)
	ui.ColocarTexto(r.extracto, tema.TextoSM, x, y+(TiraAcciones-tema.TextoSM.Linea)/2)
	r.colocarTriaje(x, y, disponible)
	y += TiraAcciones

	if len(f.Chips) > 0 {
		r.colocarChips(x, y+altoTiraConChips, disponible)
	}
}

// colocarTriaje pone la tira de triaje: sus botones, de izquierda a derecha con su
// `gap-0.5`, en `max-w-full overflow-x-auto` sin barra. Con seis acciones en un panel
// de 280 el último no cabe: se recorta y se alcanza con el gesto o el tabulador, como
// en la web.
func (r *filaRenderer) colocarTriaje(x, y, disponible float32) {
	r.tira.Move(fyne.NewPos(x, y))
	r.tira.Resize(fyne.NewSize(max(disponible, 0), LadoBotonTriaje))
	r.tira.Refresh()
}

// colocarChips pone los chips con el reparto de ColocarChips.
func (r *filaRenderer) colocarChips(x, y, disponible float32) {
	pos, anchos, _ := ColocarChips(r.f.Chips, disponible)
	for i, c := range r.chips {
		if i >= len(pos) {
			break
		}
		cx, cy, w := x+pos[i].X, y+pos[i].Y, anchos[i]
		ui.MoverCaja(c.caja, fyne.NewPos(cx, cy), fyne.NewSize(w, AltoChip))
		px := cx + tema.Borde + MargenChip
		c.punto.Move(fyne.NewPos(px, cy+(AltoChip-PuntoChip)/2))
		c.punto.Resize(fyne.NewSize(PuntoChip, PuntoChip))
		px += PuntoChip + HuecoChip
		ui.Truncar(c.nombre, r.f.Chips[i].Nombre, escalonChip, ui.Normal, cx+w-tema.Borde-MargenChip-px)
		ui.ColocarTexto(c.nombre, escalonChip, px, cy+tema.Borde+rellenoChipArriba)
	}
}

// adjuntoAncho es lo que la columna de meta reserva por el clip.
func adjuntoAncho(hay bool) float32 {
	if hay {
		return TallaClip
	}
	return 0
}

// peso es el 400 → 700 de la bandeja. NO es el 500 → 600 del `ListItem` genérico:
// Mail lo pisa a propósito con `font-normal`/`font-bold` porque un escalón de 100 a
// 12,25 px no se percibe y la bandeja se leía como un bloque uniforme.
func (r *filaRenderer) peso() ui.Peso {
	if r.f.Conv.SinLeer {
		return ui.Negrita
	}
	return ui.Normal
}

func (r *filaRenderer) Refresh() {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.base.FillColor = tema.Color("surface", v)
	r.filete.FillColor = tema.Color("border", v)

	// El orden de la web: elegida gana, y el hover solo existe en la otra rama. Una
	// fila MARCADA con la casilla se pinta como elegida —sin galón—: con la selección
	// repartida por la lista, la casilla de 14 px no basta para ver qué va a recibir
	// la acción en lote.
	switch {
	case f.Abierta:
		r.velo.FillColor = tema.Color("selected", v)
		r.galon.FillColor = tema.Color("primary", v)
	case f.Marcada:
		r.velo.FillColor = tema.Color("selected", v)
		r.galon.FillColor = color.NRGBA{}
	case f.encima():
		r.velo.FillColor = tema.Color("bg-subtle", v)
		r.galon.FillColor = color.NRGBA{}
	default:
		r.velo.FillColor = color.NRGBA{}
		r.galon.FillColor = color.NRGBA{}
	}

	tinta, tintaSub := tema.Color("text", v), tema.Color("text-muted", v)
	tintaFecha := tema.Color("text-subtle", v)
	if f.Conv.SinLeer {
		tinta, tintaSub = tema.Color("primary-text", v), tema.Color("primary-text", v)
		tintaFecha = tema.Color("primary-text", v)
	}
	cara := r.peso().Cara()
	r.nombre.Text, r.nombre.Color, r.nombre.FontSource = f.Remitente, tinta, cara
	r.asunto.Text, r.asunto.Color, r.asunto.FontSource = f.Asunto, tintaSub, cara
	// El extracto SIEMPRE va en `text-subtle`; lo que hereda del bloque es el PESO.
	r.extracto.Text = f.Conv.Ultimo.Extracto
	if r.extracto.Text == "" && f.T != nil {
		r.extracto.Text = f.T.T("list.noPreview", nil)
	}
	r.extracto.Color, r.extracto.FontSource = tema.Color("text-subtle", v), cara
	r.fecha.Text, r.fecha.Color, r.fecha.FontSource = f.Fecha, tintaFecha, cara
	r.cuantos.Text = ""
	if f.Conv.Total > 1 {
		r.cuantos.Text = numero(f.Conv.Total)
	}
	r.cuantos.Color = tema.Color("text-subtle", v)

	r.punto.FillColor = color.NRGBA{}
	if f.Conv.SinLeer {
		r.punto.FillColor = tema.Color("primary-600", v)
	}
	r.avatar.Nombre = f.Remitente
	r.casilla.Marcada = f.Marcada
	// La casilla es `opacity-0` en reposo: se ve marcada, con una selección en curso,
	// al pasar por encima o con el dedo, que no tiene «encima» (`pointer-coarse`).
	verlo(r.casilla, f.ConCasilla && (f.Marcada || f.SeleccionActiva || f.encima() || esTactil()))
	r.estrella.Ver(f.Conv.Destacada)
	r.clip.Ver(f.Conv.ConAdjunto)
	// El extracto se aparta al pasar por encima AUNQUE no haya tira que poner
	// (`group-hover:opacity-0` no mira si hay acciones): así lo hace la web en un buzón
	// compartido.
	verlo(r.extracto, !f.encima())
	r.pintarTriaje(v)
	if r.pintarChips(v) {
		r.montar()
	}

	for _, o := range r.objetos {
		o.Refresh()
	}
	// El truncado depende del ancho y la fila se recicla con textos distintos, así que
	// hay que repartirlo otra vez con el tamaño que ya tiene.
	if tam := r.f.Size(); tam.Width > 0 {
		r.Layout(tam)
	}
}

// pintarTriaje decide qué botones de la tira se ven y con qué nombre.
func (r *filaRenderer) pintarTriaje(v tema.Variante) {
	f, a := r.f, r.f.Acciones
	hay := func(h func(Conversacion)) bool { return a != nil && h != nil && f.T != nil && f.encima() }
	var ver [8]bool
	var rotulos [8]string
	if a != nil && f.T != nil {
		ver = [8]bool{
			hay(a.Destacar) && !f.Conv.Destacada, hay(a.Destacar) && f.Conv.Destacada,
			hay(a.Leido) && f.Conv.SinLeer, hay(a.Leido) && !f.Conv.SinLeer,
			hay(a.Archivar), hay(a.Papelera), hay(a.Spam), hay(a.NoSpam),
		}
		t := f.T
		rotulos = [8]string{
			t.T("action.star", nil), t.T("action.unstar", nil),
			t.T("action.markRead", nil), t.T("action.markUnread", nil),
			t.T("action.archive", nil), t.T("action.trash", nil),
			t.T("action.spam", nil), t.T("action.notSpam", nil),
		}
	}
	for i, b := range r.triaje {
		b.Rotulo, b.Texto = rotulos[i], rotulos[i]
		b.Glifo.Tenir(b.token, v)
		verlo(b, ver[i])
	}
}

// pintarChips ajusta los chips a las etiquetas de la fila y dice si ha cambiado su
// número (entonces hay que volver a montar los objetos).
func (r *filaRenderer) pintarChips(v tema.Variante) bool {
	cambio := len(r.chips) != len(r.f.Chips)
	for len(r.chips) < len(r.f.Chips) {
		r.chips = append(r.chips, chipFila{
			caja:   ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioPill),
			punto:  canvas.NewCircle(color.NRGBA{}),
			nombre: ui.Texto("", escalonChip, ui.Normal, tema.Color("text-muted", v)),
		})
	}
	r.chips = r.chips[:len(r.f.Chips)]
	for i, c := range r.chips {
		c.caja.Pintar(tema.Color("bg-subtle", v), tema.Color("border", v))
		c.punto.FillColor = ColorEtiqueta(r.f.Chips[i].Color, v)
		c.nombre.Color = tema.Color("text-muted", v)
	}
	return cambio
}

// esTactil es `pointer-coarse:`: un dispositivo sin puntero fino, donde no hay hover.
func esTactil() bool {
	return ui.EsTactil()
}

// ponerEstado deja el estado vacío pedido, devolviendo un `ui.Vacio` NUEVO cuando el
// contenido cambia y el mismo cuando no.
//
// Se rehace y no se muta porque `ui.Vacio` arma sus `Objects()` en CreateRenderer:
// ponerle una acción a uno ya montado deja el botón fuera de su layout y Fyne lo
// pinta en la esquina superior izquierda del panel. Se vio en la captura del lector.
func ponerEstado(viejo *ui.Vacio, titulo, desc string, accion fyne.CanvasObject) *ui.Vacio {
	if viejo != nil && viejo.Titulo == titulo && viejo.Descripcion == desc &&
		(viejo.Accion == nil) == (accion == nil) {
		return viejo
	}
	v := ui.NuevoVacio(titulo, desc)
	v.Accion = accion
	return v
}

// centrarVacio coloca el estado vacío CENTRADO en la caja, que es el `h-full` con
// `justify-center` de la web. `ui.Vacio` se dispone desde arriba —es su mínimo—, así
// que el centrado lo pone quien lo coloca, igual que en la web lo pone el contenedor.
func centrarVacio(v *ui.Vacio, pos fyne.Position, tam fyne.Size) {
	alto := min(v.MedidaVacio(tam.Width), tam.Height)
	v.Move(fyne.NewPos(pos.X, pos.Y+(tam.Height-alto)/2))
	v.Resize(fyne.NewSize(tam.Width, alto))
}

// cambiar sustituye un objeto por otro dentro de la lista del renderer.
func cambiar(objetos []fyne.CanvasObject, viejo, nuevo fyne.CanvasObject) {
	for i, o := range objetos {
		if o == viejo {
			objetos[i] = nuevo
			return
		}
	}
}

func verlo(o fyne.CanvasObject, si bool) {
	if si {
		o.Show()
		return
	}
	o.Hide()
}

// numero escribe un entero pequeño sin pasar por el formateador de idioma: es el
// contador de mensajes de un hilo, que nunca llega al separador de millares.
func numero(n int) string {
	if n <= 0 {
		return ""
	}
	var d [8]byte
	i := len(d)
	for n > 0 && i > 0 {
		i--
		d[i] = byte('0' + n%10)
		n /= 10
	}
	return string(d[i:])
}
