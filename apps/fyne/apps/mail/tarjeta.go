// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Una tarjeta de mensaje del hilo: `ThreadMessage` de `MessageReader.tsx`.
//
// # Las medidas, de la hoja
//
//	tarjeta    `Card raised={false}`: neutra en claro, borde de 1 y `rounded-lg`
//	cabecera   un botón `flex items-start gap-3 px-5 py-4` con `hover:bg-bg-subtle`
//	           avatar 40 plegada y 48 desplegada · la columna de texto · el galón de 18
//	           con su `mt-1`
//	línea 1    el remitente SIN clase de tamaño —hereda los 14/19,25 del texto raíz, NO
//	           el `text-base` de 12,25— y la fecha en `text-sm`, alineados por la línea
//	           base (`items-baseline gap-3`)
//	línea 2+   desplegada, la dirección (`text-text-muted`) y «Para: …»; plegada, el
//	           extracto (`text-text-subtle`); las tres en `text-sm truncate`
//	cuerpo     `px-5 pt-1 pb-5`, en `text-base leading-relaxed` (12,25 / 19,90625) y
//	           `whitespace-pre-wrap`: los saltos del correo se respetan
//
// # El cuerpo va en TEXTO, y se dice
//
// La web pinta el HTML del correo en un iframe aislado. Aquí no hay motor de HTML —es
// un límite del toolkit, no una decisión de diseño— y se enseña la parte de texto del
// mensaje, o el HTML pasado a texto si no la trae (`apps/nucleo/htmltext`). Pintar a
// medias el marcado de un remitente desconocido sería enseñar algo que no es el
// mensaje. Cuando el correo traía formato, la tarjeta lo AVISA con
// `reader.plainTextOnly`, en la misma caja con la que la web avisa del contenido
// remoto bloqueado: quien lee tiene que saber que no está viendo la maqueta original.
//
// Debajo del cuerpo van los adjuntos y el pie de acciones: ver `adjuntos.go`.
const (
	// LineaCuerpo es `leading-relaxed` (1,625) sobre `text-base`.
	LineaCuerpo = 1.625 * 0.875 * tema.Rem
	// RellenoCuerpoArriba y RellenoCuerpoAbajo son el `pt-1` y el `pb-5` del cuerpo.
	RellenoCuerpoArriba float32 = 1 * 0.25 * tema.Rem
	RellenoCuerpoAbajo  float32 = 5 * 0.25 * tema.Rem
	// maxCuerpo es lo que se reparte en líneas de un cuerpo.
	//
	// ponytail: 64 KiB. Repartir mide palabra a palabra, y un boletín pasado a texto
	// puede traer los 2 MiB que admite `eml`; cortar aquí deja el correo legible sin
	// congelar la ventana. Si alguien necesita leer más, un reparto perezoso por
	// pantallas en vez de todo de golpe.
	maxCuerpo = 64 << 10
)

// El aviso de «sin formato»: la caja de `reader.remoteContentBlocked` de la web
// (`mb-3 flex items-center gap-3 rounded-md border px-3 py-2`, icono de 16).
var (
	margenAvisoX = tema.Espacio(3)
	margenAvisoY = tema.Espacio(2)
	huecoAviso   = tema.Espacio(3)
	bajoAviso    = tema.Espacio(3)
)

// TarjetaMensaje es un mensaje dentro del hilo.
type TarjetaMensaje struct {
	widget.BaseWidget

	L         *Lector
	Mensaje   Mensaje
	Expandida bool

	ancho  float32 // lo pone el hilo al colocarla: el alto depende de él
	encima bool
}

var (
	_ fyne.Widget       = (*TarjetaMensaje)(nil)
	_ fyne.Tappable     = (*TarjetaMensaje)(nil)
	_ desktop.Hoverable = (*TarjetaMensaje)(nil)
)

func nuevaTarjetaMensaje(l *Lector, m Mensaje, expandida bool) *TarjetaMensaje {
	t := &TarjetaMensaje{L: l, Mensaje: m, Expandida: expandida}
	t.ExtendBaseWidget(t)
	return t
}

// Tapped pliega y despliega, que es lo que hace el `<button aria-expanded>` de la
// web. Solo la CABECERA es el botón: pulsar en el cuerpo es para seleccionar texto.
func (t *TarjetaMensaje) Tapped(e *fyne.PointEvent) {
	if e != nil && e.Position.Y > tema.Borde+t.altoCabecera() {
		return
	}
	t.Expandida = !t.Expandida
	if t.Expandida && t.L.AlDesplegar != nil {
		t.L.AlDesplegar(t.Mensaje.ID)
	}
	t.Refresh()
	if p := t.L; p != nil {
		p.Refresh() // el alto cambia: el hilo se recoloca
	}
}

// MouseIn es `hover:bg-bg-subtle`, solo sobre la cabecera.
func (t *TarjetaMensaje) MouseIn(e *desktop.MouseEvent) { t.MouseMoved(e) }

// MouseMoved enciende el velo mientras el puntero está en la cabecera.
func (t *TarjetaMensaje) MouseMoved(e *desktop.MouseEvent) {
	dentro := e.Position.Y <= tema.Borde+t.altoCabecera()
	if dentro != t.encima {
		t.encima = dentro
		t.Refresh()
	}
}

// MouseOut apaga el velo.
func (t *TarjetaMensaje) MouseOut() {
	if t.encima {
		t.encima = false
		t.Refresh()
	}
}

// AccessibilityLabel es el remitente y la fecha, que es lo que lee la cabecera.
func (t *TarjetaMensaje) AccessibilityLabel() string {
	return NombreVisible(t.L.T, t.Mensaje.De) + " — " + FechaLarga(t.L.Locale, t.Mensaje.Recibido)
}

// AccessibilityRole: la cabecera de la tarjeta es un botón que despliega.
func (t *TarjetaMensaje) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleButton
}

func (t *TarjetaMensaje) talla() ui.TallaAvatar {
	if t.Expandida {
		return ui.AvatarLG
	}
	return ui.AvatarMD
}

// lineasCabecera son las líneas de `text-sm` bajo el remitente: el extracto plegada, y
// desplegada la dirección y —si hay destinatarios— el «Para».
func (t *TarjetaMensaje) lineasCabecera() []string {
	m, tr := t.Mensaje, t.L.T
	if !t.Expandida {
		if m.Extracto != "" {
			return []string{m.Extracto}
		}
		return []string{tr.T("list.noPreview", nil)}
	}
	var correo string
	if len(m.De) > 0 {
		correo = m.De[0].Correo
	}
	out := []string{correo}
	if len(m.Para) > 0 {
		out = append(out, tr.T("reader.to", i18n.Vars{"addresses": UnirDirecciones(m.Para)}))
	}
	return out
}

// altoCabecera es el botón: `py-4` más lo que mande entre el avatar y el texto.
func (t *TarjetaMensaje) altoCabecera() float32 {
	texto := tema.TextoRaiz.Linea + float32(len(t.lineasCabecera()))*tema.TextoSM.Linea
	return 2*RellenoTarjeta + max(texto, ui.MedidaAvatar(t.talla()).Height)
}

// anchoUtilCuerpo es lo que le queda al texto del cuerpo dentro del `px-5` y el borde.
func (t *TarjetaMensaje) anchoUtilCuerpo(ancho float32) float32 {
	return max(ancho-2*tema.Borde-2*MargenTarjeta, tema.Rem)
}

// cuerpo decide qué enseña el cuerpo desplegado, en el orden de `MessageBodyView`:
// el cuerpo leído, el extracto mientras llega (o si falló), y si no hay nada, el aviso.
func (t *TarjetaMensaje) cuerpo() (texto string, cargando, conFormato, vacio bool) {
	c, ya := t.L.cuerpoDe(t.Mensaje.ID)
	switch {
	case ya && c.Texto != "":
		return recortar(c.Texto), false, c.ConFormato, false
	case t.Mensaje.Extracto != "":
		return t.Mensaje.Extracto, !ya, false, false
	case !ya:
		return t.L.T.T("reader.loadingBody", nil), true, false, true
	}
	return t.L.T.T("reader.noTextContent", nil), false, false, true
}

func recortar(s string) string {
	if len(s) <= maxCuerpo {
		return s
	}
	return strings.ToValidUTF8(s[:maxCuerpo], "") + "…"
}

// medidaCuerpo es lo que ocupan el aviso de formato y las líneas del cuerpo.
func (t *TarjetaMensaje) medidaCuerpo(ancho float32) (aviso, lineas []string, alto float32) {
	util := t.anchoUtilCuerpo(ancho)
	texto, cargando, conFormato, _ := t.cuerpo()
	if conFormato {
		aviso = ui.Repartir(t.L.T.T("reader.plainTextOnly", nil), tema.TextoSM, ui.Normal,
			util-2*tema.Borde-2*margenAvisoX-TallaGlifoLector-huecoAviso)
		alto += altoAviso(len(aviso)) + bajoAviso
	}
	lineas = repartirPre(texto, tema.TextoBase, ui.Normal, util)
	alto += float32(len(lineas)) * LineaCuerpo
	if cargando && t.Mensaje.Extracto != "" {
		alto += tema.Espacio(3) + esqueletoCuerpo // el `mt-3` y la barra de 0,9rem
	}
	return aviso, lineas, alto
}

// esqueletoCuerpo es el `height="0.9rem"` de la barra que dice «aún carga».
const esqueletoCuerpo = 0.9 * tema.Rem

func altoAviso(lineas int) float32 {
	return 2*tema.Borde + 2*margenAvisoY + max(TallaGlifoLector, float32(max(lineas, 1))*tema.TextoSM.Linea)
}

// MedidaTarjeta es el alto de la tarjeta a un ancho dado. Es la única cuenta: el
// `MinSize()` la llama y el guardián compara contra ella. Hoja.
func MedidaTarjeta(t *TarjetaMensaje, ancho float32) float32 {
	alto := 2*tema.Borde + t.altoCabecera()
	if t.Expandida {
		_, _, c := t.medidaCuerpo(ancho)
		alto += RellenoCuerpoArriba + c + t.altoAdjuntos() + t.altoPie(ancho) + RellenoCuerpoAbajo
	}
	return alto
}

// adjuntos son los que se enseñan y si hay bloque: no lo hay sin adjuntos, ni cuando
// todo lo adjunto son firmas de sobre —«este mensaje trae adjuntos» sería mentira—.
// Mientras el .eml no ha llegado, el bloque dice que los hay, sin lista.
func (t *TarjetaMensaje) adjuntos() (vis []AdjuntoVisible, bloque bool) {
	if !t.Mensaje.ConAdjunto {
		return nil, false
	}
	c, _ := t.L.cuerpoDe(t.Mensaje.ID)
	vis = Visibles(c.Adjuntos)
	return vis, len(c.Adjuntos) == 0 || len(vis) > 0
}

// altoAdjuntos es el bloque entero: `mt-6 border-t pt-4`, el rótulo con su `mb-2` y
// las filas —o la caja de «trae adjuntos»— con su `gap-2`.
func (t *TarjetaMensaje) altoAdjuntos() float32 {
	vis, bloque := t.adjuntos()
	if !bloque {
		return 0
	}
	filas := max(len(vis), 1)
	return aireAdjuntos + tema.Borde + rellenoAdjuntos + tema.TextoXS.Linea + bajoRotuloAdj +
		float32(filas)*AltoEnlaceAdjunto + float32(filas-1)*huecoAdjuntos
}

// anchosPie son los tres del pie: Responder y Reenviar fantasma con su glifo, y el
// enlace del original.
func (t *TarjetaMensaje) anchosPie() []float32 {
	tr := t.L.T
	boton := func(clave string) float32 {
		return ui.MedidaBotonCon(tr.T(clave, nil), ui.TallaSM, 0, 1, false, TallaGlifoLector).Width
	}
	return []float32{boton("reader.reply"), boton("reader.forward"), MedidaEnlaceOriginal(tr.T("reader.downloadOriginal", nil)).Width}
}

// altoPie es el `mt-4` y las líneas en que parte el pie (`flex-wrap gap-2`): en un
// teléfono no caben las tres acciones en una.
func (t *TarjetaMensaje) altoPie(ancho float32) float32 {
	n := float32(lineasFlujo(t.anchosPie(), HuecoBarraLector, t.anchoUtilCuerpo(ancho)))
	lado := ui.MedidaBoton("", ui.TallaSM, 1).Height
	return aireAcciones + n*lado + (n-1)*HuecoBarraLector
}

// CreateRenderer compone la tarjeta. Termina en Refresh (regla 7 de `ui.go`).
func (t *TarjetaMensaje) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &tarjetaRenderer{
		t:         t,
		caja:      ui.NuevaCaja(fondoTarjetaMensaje(v), tema.Color("border", v), tema.RadioLG),
		velo:      canvas.NewRectangle(color.NRGBA{}),
		avatar:    ui.NuevoAvatar(""),
		nombre:    ui.Texto("", tema.TextoRaiz, ui.Normal, tema.Color("text", v)),
		fecha:     ui.Texto("", tema.TextoSM, ui.Normal, tema.Color("text-subtle", v)),
		galon:     iconos.Trazo(iconos.ChevronAbajo, TallaGalon, "text-subtle", v),
		avisoCaja: ui.NuevaCaja(tema.Color("surface-sunken", v), tema.Color("border", v), tema.RadioMD),
		avisoIcon: iconos.Trazo(iconos.Informacion, TallaGlifoLector, "text-subtle", v),
		carga:     ui.NuevoEsqueleto(0, 0),
		fileteAdj: canvas.NewRectangle(tema.Color("border", v)),
	}
	r.pie = t.pie()
	r.Refresh()
	return r
}

// pie es la fila de acciones de UN mensaje: responder y reenviar a ESE, no al último
// del hilo, y bajar el original tal como llegó.
func (t *TarjetaMensaje) pie() *fyne.Container {
	tr := t.L.T
	accion := func(clave, d string, modo Respuesta) *ui.Boton {
		return botonCabecera(tr.T(clave, nil), d, ui.Fantasma, "", false, false, iconos.Ninguno, func() {
			if t.L.AlResponder != nil {
				t.L.AlResponder(t.Mensaje, modo)
			}
		})
	}
	original := nuevoEnlaceOriginal(tr.T("reader.downloadOriginal", nil), tr.T("reader.downloadOriginalHint", nil), func() {
		if t.L.AlOriginal != nil {
			t.L.AlOriginal(t.Mensaje)
		}
	})
	return container.New(flujo{hueco: HuecoBarraLector},
		accion("reader.reply", ResponderD, Responder), accion("reader.forward", ReenviarD, Reenviar), original)
}

type tarjetaRenderer struct {
	t             *TarjetaMensaje
	caja          *ui.Caja
	velo          *canvas.Rectangle
	avatar        *ui.Avatar
	nombre, fecha *canvas.Text
	sub           []*canvas.Text // las líneas de `text-sm` bajo el remitente
	galon         *iconos.Dibujo
	avisoCaja     *ui.Caja
	avisoIcon     *iconos.Dibujo
	aviso         []*canvas.Text
	parrafo       []*canvas.Text
	carga         *ui.Esqueleto
	// Los adjuntos: el filete, el rótulo en versalitas, una fila por adjunto (o la
	// caja de «trae adjuntos») y la clave con la que se sabe si hay que rehacerlos.
	fileteAdj *canvas.Rectangle
	rotuloAdj *ui.Versal
	filasAdj  []filaAdjunto
	// info es la caja de «Este mensaje contiene adjuntos» mientras no hay lista:
	// `inline-flex gap-2 rounded-md border bg-bg-subtle px-3 py-2 text-sm text-muted`
	// con el icono de información de 16.
	info      bool
	infoCaja  *ui.Caja
	infoIcono *iconos.Dibujo
	infoTexto *canvas.Text
	claveAdj  string
	pie       *fyne.Container
	objetos   []fyne.CanvasObject
}

// filaAdjunto es el enlace de un adjunto y, si hay Drive, su «Guardar en Drive».
type filaAdjunto struct {
	enlace *enlaceAdjunto
	drive  *ui.Boton
}

func (r *tarjetaRenderer) MinSize() fyne.Size {
	ancho := r.t.ancho
	if ancho <= 0 {
		ancho = 44 * tema.Rem // el suelo del `clamp(44rem, …)` hasta que el hilo lo coloque
	}
	return fyne.NewSize(2*MargenTarjeta, MedidaTarjeta(r.t, ancho))
}

func (r *tarjetaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *tarjetaRenderer) Destroy()                     {}

func (r *tarjetaRenderer) Layout(tam fyne.Size) {
	t := r.t
	r.caja.Redimensionar(tam)
	cab := t.altoCabecera()
	// El velo del `hover` va dentro del borde y sigue la esquina de la tarjeta: arriba
	// siempre, abajo solo si está plegada (desplegada, debajo viene el cuerpo).
	r.velo.Move(fyne.NewPos(tema.Borde, tema.Borde))
	r.velo.Resize(fyne.NewSize(tam.Width-2*tema.Borde, cab))
	radio := tema.RadioLG - tema.Borde
	r.velo.TopLeftCornerRadius, r.velo.TopRightCornerRadius = radio, radio
	r.velo.BottomLeftCornerRadius, r.velo.BottomRightCornerRadius = 0, 0
	if !t.Expandida {
		r.velo.BottomLeftCornerRadius, r.velo.BottomRightCornerRadius = radio, radio
	}

	lado := ui.MedidaAvatar(t.talla())
	x := tema.Borde + MargenTarjeta
	y := tema.Borde + RellenoTarjeta
	r.avatar.Talla = t.talla()
	r.avatar.Move(fyne.NewPos(x, y))
	r.avatar.Resize(lado)
	x += lado.Width + HuecoTarjetas

	derecha := tam.Width - tema.Borde - MargenTarjeta
	r.galon.Colocar(derecha-TallaGalon, y+tema.Espacio(1), TallaGalon) // el `mt-1`
	derecha -= TallaGalon + HuecoTarjetas

	// Línea 1: `items-baseline gap-3`. La fecha es `shrink-0`; el nombre se trunca.
	peso := r.peso()
	anchoFecha := ui.Medir(r.fecha.Text, tema.TextoSM, peso).Width
	ui.ColocarTexto(r.fecha, tema.TextoSM, derecha-anchoFecha,
		y+lineaBase(tema.TextoRaiz, peso)-lineaBase(tema.TextoSM, peso))
	ui.Truncar(r.nombre, NombreVisible(t.L.T, t.Mensaje.De), tema.TextoRaiz, peso,
		max(derecha-anchoFecha-HuecoTarjetas-x, 0))
	ui.ColocarTexto(r.nombre, tema.TextoRaiz, x, y)
	y += tema.TextoRaiz.Linea
	for i, l := range t.lineasCabecera() {
		if i < len(r.sub) {
			ui.Truncar(r.sub[i], l, tema.TextoSM, ui.Normal, max(derecha-x, 0))
			ui.ColocarTexto(r.sub[i], tema.TextoSM, x, y)
		}
		y += tema.TextoSM.Linea
	}

	if !t.Expandida {
		return
	}
	x = tema.Borde + MargenTarjeta
	y = tema.Borde + cab + RellenoCuerpoArriba
	util := t.anchoUtilCuerpo(tam.Width)
	if len(r.aviso) > 0 {
		alto := altoAviso(len(r.aviso))
		ui.MoverCaja(r.avisoCaja, fyne.NewPos(x, y), fyne.NewSize(util, alto))
		ix := x + tema.Borde + margenAvisoX
		r.avisoIcon.Colocar(ix, y+(alto-TallaGlifoLector)/2, TallaGlifoLector)
		ty := y + (alto-float32(len(r.aviso))*tema.TextoSM.Linea)/2
		for _, l := range r.aviso {
			ui.ColocarTexto(l, tema.TextoSM, ix+TallaGlifoLector+huecoAviso, ty)
			ty += tema.TextoSM.Linea
		}
		y += alto + bajoAviso
	}
	for _, l := range r.parrafo {
		ui.ColocarTexto(l, tema.Escalon{Tam: tema.TextoBase.Tam, Linea: LineaCuerpo}, x, y)
		y += LineaCuerpo
	}
	if r.carga.Visible() {
		r.carga.Move(fyne.NewPos(x, y+tema.Espacio(3)))
		r.carga.Resize(fyne.NewSize(util*0.6, esqueletoCuerpo))
	}
	_, _, altoCuerpo := t.medidaCuerpo(tam.Width)
	y = tema.Borde + cab + RellenoCuerpoArriba + altoCuerpo
	y = r.colocarAdjuntos(x, y, util)
	altoPie := t.altoPie(tam.Width) - aireAcciones
	r.pie.Move(fyne.NewPos(x, y+aireAcciones))
	r.pie.Resize(fyne.NewSize(util, altoPie))
}

// colocarAdjuntos pone el bloque de adjuntos a partir de `y` y devuelve dónde acaba.
func (r *tarjetaRenderer) colocarAdjuntos(x, y, util float32) float32 {
	if !r.fileteAdj.Visible() {
		return y
	}
	y += aireAdjuntos
	r.fileteAdj.Move(fyne.NewPos(x, y))
	r.fileteAdj.Resize(fyne.NewSize(util, tema.Borde))
	y += tema.Borde + rellenoAdjuntos
	if r.rotuloAdj != nil {
		r.rotuloAdj.Colocar(x, y)
	}
	y += tema.TextoXS.Linea + bajoRotuloAdj
	if r.info {
		ancho := 2*tema.Borde + 2*margenXEnlace + TallaGlifoLector + huecoAdjuntos +
			ui.Medir(r.infoTexto.Text, tema.TextoSM, ui.Normal).Width
		ui.MoverCaja(r.infoCaja, fyne.NewPos(x, y), fyne.NewSize(min(ancho, util), AltoEnlaceAdjunto))
		ix := x + tema.Borde + margenXEnlace
		r.infoIcono.Colocar(ix, y+(AltoEnlaceAdjunto-TallaGlifoLector)/2, TallaGlifoLector)
		ui.ColocarTexto(r.infoTexto, tema.TextoSM, ix+TallaGlifoLector+huecoAdjuntos, y+(AltoEnlaceAdjunto-tema.TextoSM.Linea)/2)
		return y + AltoEnlaceAdjunto
	}
	for i, f := range r.filasAdj {
		if i > 0 {
			y += huecoAdjuntos
		}
		resto := util
		if f.drive != nil {
			md := f.drive.MinSize()
			resto -= md.Width + huecoAdjuntos
			f.enlace.ancho = resto
			f.drive.Resize(md)
		}
		f.enlace.ancho = resto
		me := f.enlace.MinSize()
		f.enlace.Move(fyne.NewPos(x, y))
		f.enlace.Resize(me)
		if f.drive != nil {
			f.drive.Move(fyne.NewPos(x+me.Width+huecoAdjuntos, y+(AltoEnlaceAdjunto-f.drive.MinSize().Height)/2))
		}
		y += AltoEnlaceAdjunto
	}
	return y
}

// peso es el 400 → 700 del remitente: el mismo salto que en la lista, porque dentro
// de un hilo largo saber qué mensaje sigue sin abrir es justo lo que se busca.
func (r *tarjetaRenderer) peso() ui.Peso {
	if !r.t.Mensaje.Leido {
		return ui.Negrita
	}
	return ui.Normal
}

// textos ajusta una lista de `canvas.Text` al número de líneas pedido y dice si ha
// cambiado su número (entonces hay que volver a montar los objetos).
func textos(l []*canvas.Text, n int, e tema.Escalon, c color.NRGBA) ([]*canvas.Text, bool) {
	if len(l) == n {
		return l, false
	}
	l = l[:0]
	for range n {
		l = append(l, ui.Texto("", e, ui.Normal, c))
	}
	return l, true
}

// fondoTarjetaMensaje es la superficie de la tarjeta del mensaje: en claro,
// blanco (surface), destacando sobre el fondo (bg) del lector.
func fondoTarjetaMensaje(v tema.Variante) color.NRGBA {
	if v == tema.Claro {
		return tema.Color("surface", v)
	}
	return tema.Color("surface-raised", v)
}

func (r *tarjetaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(fondoTarjetaMensaje(v), tema.Color("border", v))
	r.velo.FillColor = color.NRGBA{}
	if r.t.encima {
		r.velo.FillColor = tema.Color("bg-subtle", v)
	}
	cambio := r.pintarCabecera(v)
	cambio = r.pintarCuerpo(v) || cambio
	cambio = r.pintarAdjuntos(v) || cambio
	if cambio || r.objetos == nil {
		r.montar()
	}
	r.Layout(r.t.Size())
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// pintarCabecera pone remitente, fecha, galón y las líneas de debajo. Dice si ha
// cambiado el número de líneas.
func (r *tarjetaRenderer) pintarCabecera(v tema.Variante) bool {
	t, m := r.t, r.t.Mensaje
	r.avatar.Nombre = NombreVisible(t.L.T, m.De)
	peso := r.peso()
	tinta, tintaFecha := tema.Color("text", v), tema.Color("text-subtle", v)
	if !m.Leido {
		tinta, tintaFecha = tema.Color("primary-text", v), tema.Color("primary-text", v)
	}
	r.nombre.Color, r.nombre.FontSource = tinta, peso.Cara()
	r.fecha.Color, r.fecha.FontSource = tintaFecha, peso.Cara()
	// La fecha larga solo cabe en escritorio: en el móvil aplastaba el nombre del
	// remitente hasta dejarlo en «D…» (`max-md:hidden` / `md:hidden`).
	r.fecha.Text = FechaCorta(t.L.Locale, m.Recibido, t.L.Ahora())
	if t.Expandida && ventana(t, t.ancho) >= ui.Estrecho {
		r.fecha.Text = FechaLarga(t.L.Locale, m.Recibido)
	}
	icono := iconos.ChevronAbajo
	if t.Expandida {
		icono = iconos.ChevronArriba
	}
	r.galon.Cambiar(icono, tema.Color("text-subtle", v))

	var cambio bool
	r.sub, cambio = textos(r.sub, len(t.lineasCabecera()), tema.TextoSM, color.NRGBA{})
	for i := range r.sub {
		// Desplegada, la primera es la dirección en `text-muted`; lo demás, `subtle`.
		r.sub[i].Color = tema.Color("text-subtle", v)
		if t.Expandida && i == 0 {
			r.sub[i].Color = tema.Color("text-muted", v)
		}
	}
	return cambio
}

// pintarCuerpo pone el aviso de formato, las líneas del cuerpo y la barra de carga.
// Dice si ha cambiado el número de objetos.
func (r *tarjetaRenderer) pintarCuerpo(v tema.Variante) bool {
	t := r.t
	var aviso, lineas []string
	var cargando, vacio bool
	if t.Expandida {
		aviso, lineas, _ = t.medidaCuerpo(t.ancho)
		_, cargando, _, vacio = t.cuerpo()
	}
	var c1, c2 bool
	r.aviso, c1 = textos(r.aviso, len(aviso), tema.TextoSM, tema.Color("text-subtle", v))
	for i, l := range aviso {
		r.aviso[i].Text, r.aviso[i].Color = l, tema.Color("text-subtle", v)
	}
	r.avisoCaja.Pintar(tema.Color("surface-sunken", v), tema.Color("border", v))
	r.avisoIcon.Tenir("text-subtle", v)
	r.avisoIcon.Ver(len(aviso) > 0)
	for _, o := range r.avisoCaja.Objetos() {
		verlo(o, len(aviso) > 0)
	}

	tintaCuerpo := tema.Color("text", v)
	if vacio {
		tintaCuerpo = tema.Color("text-subtle", v)
	}
	r.parrafo, c2 = textos(r.parrafo, len(lineas), tema.TextoBase, tintaCuerpo)
	for i, l := range lineas {
		r.parrafo[i].Text, r.parrafo[i].Color = l, tintaCuerpo
	}
	verlo(r.carga, t.Expandida && cargando && t.Mensaje.Extracto != "")
	return c1 || c2
}

// pintarAdjuntos rehace el bloque de adjuntos si ha cambiado lo que enseña. Dice si
// ha cambiado el número de objetos.
func (r *tarjetaRenderer) pintarAdjuntos(v tema.Variante) bool {
	t := r.t
	vis, bloque := t.adjuntos()
	bloque = bloque && t.Expandida
	verlo(r.fileteAdj, bloque)
	verlo(r.pie, t.Expandida)
	r.fileteAdj.FillColor = tema.Color("border", v)
	clave := ""
	if bloque {
		clave = fmt.Sprint(len(vis), t.L.AlGuardarEnDrive != nil)
		for _, a := range vis {
			clave += "|" + a.Nombre + fmt.Sprint(t.L.Guardando[claveAdjunto(t.Mensaje.ID, a.Indice)])
		}
	}
	if clave == r.claveAdj {
		return false
	}
	r.claveAdj = clave
	r.rotuloAdj, r.info, r.filasAdj = nil, false, nil
	if !bloque {
		return true
	}
	tr := t.L.T
	rotulo := tr.T("reader.attachments", nil)
	if len(vis) > 0 {
		rotulo = tr.T("reader.attachmentCount", i18n.Vars{"count": len(vis)})
	}
	r.rotuloAdj = ui.NuevoVersal(rotulo, tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v))
	if len(vis) == 0 {
		r.info = true
		r.infoCaja = ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioMD)
		r.infoIcono = iconos.Trazo(iconos.Informacion, TallaGlifoLector, "text-muted", v)
		r.infoTexto = ui.Texto(tr.T("reader.hasAttachments", nil), tema.TextoSM, ui.Normal, tema.Color("text-muted", v))
		return true
	}
	for _, a := range vis {
		f := filaAdjunto{enlace: nuevoEnlaceAdjunto(nombreAdjunto(a.Adjunto), i18n.FormatBytes(t.L.Locale, float64(len(a.Datos))),
			tr.T("reader.download", i18n.Vars{"name": nombreAdjunto(a.Adjunto)}), func() {
				if t.L.AlDescargarAdjunto != nil {
					t.L.AlDescargarAdjunto(t.Mensaje, a)
				}
			})}
		if t.L.AlGuardarEnDrive != nil {
			f.drive = ui.NuevoBoton(tr.T("reader.saveToDrive", nil), func() { t.L.AlGuardarEnDrive(t.Mensaje, a) }).
				Con(ui.Fantasma, ui.TallaSM)
			f.drive.Inerte = t.L.Guardando[claveAdjunto(t.Mensaje.ID, a.Indice)]
		}
		r.filasAdj = append(r.filasAdj, f)
	}
	return true
}

// montar arma la lista de objetos en el orden en que se pintan.
func (r *tarjetaRenderer) montar() {
	r.objetos = append(r.caja.Objetos(), r.velo, r.avatar, r.nombre, r.fecha, r.galon.Objeto())
	for _, l := range r.sub {
		r.objetos = append(r.objetos, l)
	}
	r.objetos = append(r.objetos, r.avisoCaja.Objetos()...)
	r.objetos = append(r.objetos, r.avisoIcon.Objeto())
	for _, l := range r.aviso {
		r.objetos = append(r.objetos, l)
	}
	for _, l := range r.parrafo {
		r.objetos = append(r.objetos, l)
	}
	r.objetos = append(r.objetos, r.carga, r.fileteAdj)
	if r.rotuloAdj != nil {
		r.objetos = append(r.objetos, r.rotuloAdj.Objetos()...)
	}
	if r.info {
		r.objetos = append(r.objetos, r.infoCaja.Objetos()...)
		r.objetos = append(r.objetos, r.infoIcono.Objeto(), r.infoTexto)
	}
	for _, f := range r.filasAdj {
		r.objetos = append(r.objetos, f.enlace)
		if f.drive != nil {
			r.objetos = append(r.objetos, f.drive)
		}
	}
	r.objetos = append(r.objetos, r.pie)
}

// repartirPre es el reparto de `whitespace-pre-wrap` con `[overflow-wrap:anywhere]`:
// los saltos de línea del correo se RESPETAN —`ui.Repartir` los aplana, que está bien
// para un párrafo de interfaz y mal para un correo, donde un saludo, una firma y una
// lista son líneas—, y una palabra que no cabe entera (una URL) se parte por donde
// haga falta en vez de salirse por el borde.
func repartirPre(texto string, e tema.Escalon, p ui.Peso, cabe float32) []string {
	var out []string
	for _, parrafo := range strings.Split(texto, "\n") {
		if strings.TrimSpace(parrafo) == "" {
			out = append(out, "")
			continue
		}
		for _, l := range ui.Repartir(parrafo, e, p, cabe) {
			out = append(out, partirLarga(l, e, p, cabe)...)
		}
	}
	return out
}

// partirLarga corta por caracteres una línea más ancha que `cabe`: el tramo más largo
// que quepa, y otra vez con lo que sobre.
func partirLarga(l string, e tema.Escalon, p ui.Peso, cabe float32) []string {
	if cabe <= 0 || ui.Medir(l, e, p).Width <= cabe {
		return []string{l}
	}
	var out []string
	rs := []rune(l)
	for len(rs) > 0 {
		lo, hi := 1, len(rs)
		for lo < hi {
			mid := (lo + hi + 1) / 2
			if ui.Medir(string(rs[:mid]), e, p).Width <= cabe {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		out = append(out, string(rs[:lo]))
		rs = rs[lo:]
	}
	return out
}

// UnirDirecciones es `joinAddresses`: nombre si lo hay, y si no la dirección.
func UnirDirecciones(ds []Direccion) string {
	partes := make([]string, 0, len(ds))
	for _, d := range ds {
		if n := espaciosFuera(d.Nombre); n != "" {
			partes = append(partes, n)
			continue
		}
		partes = append(partes, d.Correo)
	}
	return strings.Join(partes, ", ")
}
