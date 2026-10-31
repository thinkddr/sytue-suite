// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

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

// paridad-web: apps/frontend/apps/rooms/src/pages/index.astro
// paridad-web: apps/frontend/apps/rooms/src/pages/channels/[channelId].astro
//
// Las DOS pantallas son la misma: `/rooms` redirige al último canal
// (`/rooms/channels/<id>`), así que la portada y el canal son el mismo árbol con y sin
// canal elegido. Está medido: el barrido las capturó por separado y salieron idénticas
// salvo el menú de presencia abierto en una.

// Pantalla es Sytue Rooms entero: el carril, la cabecera, el historial y la caja de
// escribir dentro del armazón de la suite.
//
// Es un ESTADO, no una vista: la app lo cambia y llama a Refrescar. Todo lo que hay
// aquí son datos y punteros a funciones.
//
// La TOPBAR no está, y no es un olvido: es una de las dos exclusiones de paridad de
// `docs/CONVENTIONS.md` §13.6 —existe solo en la web y en nativo la sustituye el cromo
// de la ventana—. Por eso el armazón empieza en y = 0 y no en 56.
type Pantalla struct {
	T      *i18n.Translator
	Locale i18n.Locale

	Lateral *Lateral
	Canal   *VistaCanal
	// Avisos es la pila de avisos flotantes (`toast` de la web): «Texto copiado»…
	Avisos *ui.PilaAvisos

	armazon *ui.Armazon
	raiz    fyne.CanvasObject
	pagina  fyne.CanvasObject
}

// VistaCanal es la columna de la derecha: cabecera, historial y caja de escribir. Se
// llama así y no `Canal` porque ese nombre ya es el DATO de un canal.
//
// Sin canal abierto pinta el estado vacío ENTERO —«Elige un canal»— y esconde las tres
// piezas: una cabecera sin nombre y una caja donde no se puede escribir es peor que no
// enseñar nada, porque parece que la aplicación está rota.
type VistaCanal struct {
	widget.BaseWidget

	Cabecera   *Cabecera
	Historial  *Historial
	Compositor *Compositor
	// Vacio es lo que se ve sin canal abierto.
	Vacio *ui.Vacio
	// Escribiendo son los nombres de quien está escribiendo en el canal abierto.
	Escribiendo []string
	// EstadoCanales y HayCanales deciden QUÉ vacío se ve sin canal abierto: no es lo
	// mismo «cargando», «no se han podido cargar», «aún no tienes ninguno» —que es
	// donde empieza una organización recién creada, y sin salida no tiene ninguna— y
	// «elige uno».
	EstadoCanales Estado
	HayCanales    bool

	AlReintentar func()
	AlCrearCanal func()
	AlExplorar   func()

	// Derecho es el panel abierto a la derecha (el hilo, la búsqueda…), o nil. En
	// ventana ancha convive con el canal y mide `--rooms-rhs-width`; en estrecha tapa
	// el canal entero, que es el `max-md:hidden` de la web.
	Derecho fyne.CanvasObject
	// Ventana es el ancho de la VENTANA: los breakpoints de la hoja (`md`, `sm`) miden
	// la ventana, no esta columna.
	Ventana func() float32
	// ViendoHistorial enseña la barra de «Ir a mensajes recientes» encima de la línea
	// de «escribiendo», tras saltar a un mensaje viejo.
	ViendoHistorial bool
	AlIrAlFinal     func()

	T           *i18n.Translator
	alFinal     *BarraRecientes
	fondo       *canvas.Rectangle
	escribiendo *canvas.Text
	objetos     []fyne.CanvasObject
	sinCanal    bool
}

// AltoEscribiendo es la fila de «X está escribiendo…» entre el historial y la caja de
// escribir: `sytue-meta h-5 shrink-0 truncate px-4 italic`. Tiene su alto SIEMPRE,
// haya alguien escribiendo o no, que es lo que la web hace para que el historial no
// salte cada vez que alguien pulsa una tecla. MEDIDO en `canal.ancho.png`: el filete
// del compositor cae en y = 703, 17,5 por debajo del final del historial.
var AltoEscribiendo = tema.Espacio(5)

var _ fyne.Widget = (*VistaCanal)(nil)

// NuevaPantalla monta las dos columnas con su traductor.
func NuevaPantalla(t *i18n.Translator, l i18n.Locale) *Pantalla {
	p := &Pantalla{
		T:      t,
		Locale: l,
		Lateral: &Lateral{
			T: t, Locale: l, EstadoCanales: Cargando,
			AlElegirCanal: func(Canal) {}, AlPlegarSeccion: func(Seccion) {},
			AlAbrirPanel: func(string) {},
		},
	}
	p.Canal = nuevoCanal(t, l)
	p.armazon = ui.NuevoArmazon(p.Canal)
	// La hamburguesa es la de la cabecera del canal, como en ChatApp.tsx: con la barra
	// móvil del armazón salían dos, una encima de otra (visto en ventana real a 360).
	p.armazon.SinBarraMovil = true
	p.Avisos = ui.NuevaPilaAvisos()
	p.raiz = container.NewStack(p.armazon, p.Avisos)
	p.Canal.Ventana = func() float32 { return p.armazon.Size().Width }
	// La hamburguesa saca el carril como cajón, y tocar el velo lo recoge.
	//
	// ponytail: el toque llega al armazón sin su posición, así que un toque en un hueco
	// SIN control del propio cajón también lo recoge; la web solo lo hace en el velo.
	// Se afina cuando `ui.Armazon` entregue el punto del toque.
	p.Canal.Cabecera.AlMenu = func() {
		p.armazon.AbrirCajon()
	}
	p.armazon.AlPulsar = func() {
		if p.armazon.CajonAbierto && !p.Canal.sinCanal {
			p.armazon.CerrarCajon()
		}
	}
	p.armazon.Menu = p.Lateral.Construir()
	return p
}

func nuevoCanal(t *i18n.Translator, l i18n.Locale) *VistaCanal {
	c := &VistaCanal{
		Cabecera:   NuevaCabecera(t, l),
		Historial:  NuevoHistorial(t, l),
		Compositor: NuevoCompositor(t, l),
		Vacio:      ui.NuevoVacio(t.T("empty.pickChannel", nil), t.T("empty.pickChannelHelp", nil)),
		T:          t,
		sinCanal:   true,
	}
	c.ExtendBaseWidget(c)
	// Las fichas de las opciones cambian el alto de la caja, y con él el del historial.
	c.Compositor.AlCambiarAlto = c.Refresh
	return c
}

// Objeto es lo que se le da a la ventana.
func (p *Pantalla) Objeto() fyne.CanvasObject { return p.raiz }

// Avisar saca un aviso flotante con ese tono y título.
func (p *Pantalla) Avisar(tono ui.TonoAviso, titulo string) {
	p.Avisos.Notificar(ui.NuevoAviso(tono, titulo, ""))
}

// Abrir enseña un canal: es la única costura entre el carril y la columna derecha, y
// por eso está aquí y no dentro de ninguna de las dos.
func (p *Pantalla) Abrir(c Canal) {
	p.Lateral.CanalID = c.ID
	p.Canal.sinCanal = false
	p.Canal.Cabecera.Canal = c
	p.Canal.Historial.NombreCanal = c.Nombre
	p.Canal.Compositor.NombreCanal = c.Nombre
	p.DetalleAbierto(true)
	p.Refrescar()
}

// DetalleAbierto es el turno de lista y detalle en ventana estrecha: con un canal
// abierto se ve la conversación, y sin él el carril. Sin esto, en un teléfono el canal
// que pulsas no aparece por ninguna parte.
func (p *Pantalla) DetalleAbierto(si bool) {
	p.armazon.DetalleAbierto = si
	p.armazon.CajonAbierto = !si
}

// SinCanal deja la pantalla en su estado de «elige un canal», que es lo que la web
// enseña cuando todavía no hay ninguno abierto.
func (p *Pantalla) SinCanal() {
	p.Lateral.CanalID = ""
	p.Canal.sinCanal = true
	p.DetalleAbierto(false)
	p.Refrescar()
}

// PonerDerecho abre un panel a la derecha del canal, o lo cierra con nil.
func (p *Pantalla) PonerDerecho(o fyne.CanvasObject) {
	p.Canal.Derecho = o
	p.Canal.Refresh()
}

// PonerPagina tapa la sala con otra página de la app —«Compartir en Rooms», que en la
// web es su propia dirección (`/share`)—; nil la retira.
func (p *Pantalla) PonerPagina(o fyne.CanvasObject) {
	objs := []fyne.CanvasObject{p.armazon}
	if o != nil {
		objs = append(objs, o)
	}
	objs = append(objs, p.Avisos)
	p.pagina = o
	p.raiz.(*fyne.Container).Objects = objs //nolint:errcheck,forcetypeassert // Raíz se crea como contenedor en NuevaPantalla.
	p.raiz.Refresh()
}

// Pagina es la página que tapa la sala, o nil.
func (p *Pantalla) Pagina() fyne.CanvasObject { return p.pagina }

// Reloj fija el reloj con el que se escriben «Hoy» y «Ayer». El guardián lo congela
// para no depender de la hora.
func (p *Pantalla) Reloj(f func() time.Time) { p.Canal.Historial.Ahora = f }

// Refrescar vuelve a montar el carril —que se construye a partir de datos— y repinta
// las dos columnas.
func (p *Pantalla) Refrescar() {
	l := p.Lateral
	p.Canal.EstadoCanales, p.Canal.HayCanales = l.EstadoCanales, len(l.Canales) > 0
	p.Canal.AlReintentar, p.Canal.AlCrearCanal, p.Canal.AlExplorar = l.AlReintentar, l.AlCrearCanal, l.AlExplorar
	p.armazon.Menu = p.Lateral.Construir()
	p.armazon.Refresh()
	p.Canal.Refresh()
}

// vacioSinCanal es el `EmptyState` de la columna sin canal, en sus cuatro casos, con
// las ilustraciones y los botones de `ChatApp.tsx`.
func (c *VistaCanal) vacioSinCanal(v tema.Variante) *ui.Vacio {
	t := c.T
	switch {
	case c.EstadoCanales == Error:
		x := ui.NuevoVacio(t.T("empty.channelsError", nil), t.T("error.checkConnection", nil))
		x.Ilustracion = iconos.Trazo(iconos.Peligro, LadoIconoEstado, "danger-text", v).Objeto()
		x.Accion = ui.NuevoBoton(t.T("action.retry", nil), c.AlReintentar).Con(ui.Secundario, ui.TallaSM)
		return x
	case c.EstadoCanales == Cargando:
		x := ui.NuevoVacio(t.T("empty.loadingChannels", nil), t.T("empty.loadingChannelsHelp", nil))
		x.Ilustracion = ui.NuevaRueda(LadoRuedaVacio, "text-subtle", v)
		return x
	case !c.HayCanales:
		x := ui.NuevoVacio(t.T("empty.noChannels", nil), t.T("empty.noChannelsHelp", nil))
		x.Ilustracion = iconos.Pintar(iconos.VacioChat, v).A(LadoIlustracion).Objeto()
		x.Accion = container.New(ui.Fila{Hueco: tema.Espacio(2)},
			ui.NuevoBoton(t.T("sidebar.createChannel", nil), c.AlCrearCanal).Con(ui.Primario, ui.TallaSM),
			ui.NuevoBoton(t.T("sidebar.browseChannels", nil), c.AlExplorar).Con(ui.Secundario, ui.TallaSM))
		return x
	}
	x := ui.NuevoVacio(t.T("empty.pickChannel", nil), t.T("empty.pickChannelHelp", nil))
	x.Ilustracion = iconos.Pintar(iconos.VacioChat, v).A(LadoIlustracion).Objeto()
	return x
}

// estrecha es la ventana por debajo de `md` (768).
func (c *VistaCanal) estrecha() bool { return c.Ventana != nil && c.Ventana() < ui.Estrecho }

// LadoRuedaVacio es el `<Spinner size={24} />` del vacío de «cargando canales». HOJA.
var LadoRuedaVacio float32 = 24

// AccessibilityLabel es el nombre del canal abierto.
func (c *VistaCanal) AccessibilityLabel() string { return c.Cabecera.Canal.Nombre }

// AccessibilityRole implementa fyne.Accessible.
func (c *VistaCanal) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// CreateRenderer compone la columna.
func (c *VistaCanal) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	c.fondo = canvas.NewRectangle(tema.Color("surface", v))
	c.escribiendo = canvas.NewText("", tema.Color("text-subtle", v))
	c.escribiendo.TextSize = tema.TextoXS.Tam
	c.escribiendo.FontSource = tema.Cursiva
	c.alFinal = NuevaBarraRecientes(c.T.T("message.goToRecent", nil), func() {
		if c.AlIrAlFinal != nil {
			c.AlIrAlFinal()
		}
	})
	c.objetos = []fyne.CanvasObject{c.fondo, c.Cabecera, c.Historial, c.escribiendo, c.Compositor, c.Vacio}
	r := &canalRenderer{c: c}
	r.flujo = columnaConversacion{minimo: r.altoMinimo, colocar: r.colocarContenido}
	r.Refresh()
	return r
}

type canalRenderer struct {
	c     *VistaCanal
	flujo columnaConversacion
}

// MinSize permite reducir el viewport: las franjas conservan su altura dentro del
// scroll cuando hace falta, en lugar de imponerla a la ventana móvil.
func (r *canalRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, AltoCabecera)
}

func (r *canalRenderer) Objects() []fyne.CanvasObject {
	if !r.flujo.desbordando {
		return r.c.objetos
	}
	objetos := []fyne.CanvasObject{r.c.fondo, r.flujo.desborde}
	if r.c.Derecho != nil {
		objetos = append(objetos, r.c.Derecho)
	}
	return objetos
}
func (r *canalRenderer) Destroy() {}

func (r *canalRenderer) altoMinimo(ancho float32) float32 {
	alto := AltoCabecera + AltoFilaMensaje(false) + AltoEscribiendo + r.c.Compositor.medidaPara(ancho).Height
	if r.c.ViendoHistorial {
		alto += MedidaBarraRecientes().Height
	}
	return alto
}

// Layout reparte la columna: la cabecera arriba, la caja de escribir abajo y el
// historial con lo que sobre (`min-h-0 flex-1`). Con un panel a la derecha, el canal
// le deja su ancho, o se esconde entero si la ventana es estrecha.
func (r *canalRenderer) Layout(t fyne.Size) {
	c := r.c
	c.fondo.Resize(t)
	tapado := false
	if d := c.Derecho; d != nil {
		d.Show()
		if c.estrecha() {
			tapado = true
			d.Move(fyne.NewPos(0, 0))
			d.Resize(t)
		} else {
			ancho := min(AnchoPanelDerecho, t.Width)
			d.Move(fyne.NewPos(t.Width-ancho, 0))
			d.Resize(fyne.NewSize(ancho, t.Height))
			t = fyne.NewSize(t.Width-ancho, t.Height)
		}
	}
	for _, o := range []fyne.CanvasObject{c.Cabecera, c.Historial, c.alFinal, c.escribiendo, c.Compositor} {
		if c.sinCanal || tapado || (o == c.alFinal && !c.ViendoHistorial) {
			o.Hide()
			continue
		}
		o.Show()
	}
	if tapado {
		r.flujo.desbordando = false
		c.Vacio.Hide()
		return
	}
	if c.sinCanal {
		r.flujo.desbordando = false
		if c.estrecha() && c.Cabecera.AlMenu != nil {
			if c.Vacio.Accion == nil {
				c.Vacio.Accion = ui.NuevoBoton(c.T.T("sidebar.openChannels", nil), c.Cabecera.AlMenu).Con(ui.Secundario, ui.TallaSM)
				c.Vacio.Refresh()
			}
		} else if c.Vacio.Accion != nil {
			c.Vacio.Accion = nil
			c.Vacio.Refresh()
		}
		c.Vacio.Show()
		c.Vacio.Move(fyne.NewPos(0, 0))
		c.Vacio.Resize(t)
		return
	}
	c.Vacio.Hide()
	if estrecha, menuVisible := t.Width < ui.Estrecho, !c.estrecha(); estrecha != c.Cabecera.Estrecha || menuVisible != c.Cabecera.menuYaVisible {
		c.Cabecera.Estrecha = estrecha
		c.Cabecera.menuYaVisible = menuVisible
		c.Cabecera.Refresh()
	}
	r.flujo.repartir(t, []fyne.CanvasObject{c.Cabecera, c.Historial, c.alFinal, c.escribiendo, c.Compositor})
}

func (r *canalRenderer) colocarContenido(t fyne.Size) {
	c := r.c
	c.Cabecera.Move(fyne.NewPos(0, 0))
	c.Cabecera.Resize(fyne.NewSize(t.Width, AltoCabecera))
	alto := c.Compositor.medidaPara(t.Width).Height
	yCaja := max(t.Height-alto, AltoCabecera+AltoEscribiendo)
	c.Compositor.Move(fyne.NewPos(0, yCaja))
	c.Compositor.Resize(fyne.NewSize(t.Width, alto))
	// La línea de «escribiendo», con su `px-4` y truncada. El texto va arriba del
	// `h-5`: es un bloque con una sola caja de línea de 14 dentro.
	yLinea := yCaja - AltoEscribiendo
	ui.TruncarCara(c.escribiendo, RotuloEscribiendo(c.T, c.Escribiendo), tema.TextoXS, tema.Cursiva,
		max(t.Width-2*CanalonHistorial, 0))
	ui.ColocarTexto(c.escribiendo, tema.TextoXS, CanalonHistorial, yLinea)
	fin := yLinea
	if c.ViendoHistorial {
		fin -= MedidaBarraRecientes().Height
		c.alFinal.Move(fyne.NewPos(0, fin))
		c.alFinal.Resize(fyne.NewSize(t.Width, MedidaBarraRecientes().Height))
	}
	c.Historial.Move(fyne.NewPos(0, AltoCabecera))
	c.Historial.Resize(fyne.NewSize(t.Width, max(fin-AltoCabecera, 0)))
}

// Refresh repinta y REHACE la disposición: abrir un canal no cambia el tamaño de la
// columna, y Fyne solo llama a Layout cuando el tamaño cambia.
func (r *canalRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if r.c.sinCanal {
		r.c.Vacio = r.c.vacioSinCanal(v)
	}
	r.c.objetos = []fyne.CanvasObject{r.c.fondo, r.c.Cabecera, r.c.Historial, r.c.alFinal, r.c.escribiendo, r.c.Compositor, r.c.Vacio}
	if r.c.Derecho != nil {
		r.c.objetos = append(r.c.objetos, r.c.Derecho)
	}
	r.c.fondo.FillColor = tema.Color("surface", v)
	r.c.fondo.Refresh()
	r.c.escribiendo.Color = tema.Color("text-subtle", v)
	r.c.escribiendo.Refresh()
	// La cabecera y la caja de escribir llevan el nombre del canal: cambiar de canal
	// no cambia su tamaño, y sin esto seguirían diciendo el del anterior.
	if !r.c.sinCanal {
		r.c.Cabecera.Refresh()
		r.c.Compositor.Refresh()
	}
	r.Layout(r.c.Size())
}
