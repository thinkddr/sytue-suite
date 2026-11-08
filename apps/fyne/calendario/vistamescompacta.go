// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image/color"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/agenda"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// La pareja en la web es apps/frontend/apps/calendar/src/components/MesCompacto.tsx.
//
// EL MES COMPACTO DEL TELÉFONO: la vista de mes con la VENTANA por debajo de `sm:`
// (`ui.CorteSM`, 640). La rejilla de escritorio encogida eran seis semanas de celdas de
// ~55 con píldoras ilegibles; aquí, como el calendario del teléfono: las semanas que tocan
// el mes con su número ISO, cada día con su número y hasta tres puntos de color, el día
// elegido (que es el CURSOR de la pantalla, no otro estado) relleno del primario, un asa
// y, debajo, la lista de ese día. Los datos los da mescompacto.go; aquí solo se pintan.
//
// EL ASA cambia entre mes, semana y agenda, que son la MISMA vista que marca el selector
// del cajón (la posición sale de `p.Vista`; no hay otro estado). Arrastrarla mueve la hoja
// con el dedo, sin saltos, y al soltar se ajusta a lo que dice `PosicionAlSoltar`
// (asames.go, con su pareja en la web); un toque alterna mes ↔ semana (de la agenda, a la
// semana); ↑/↓ y Inicio/Fin también. En la semana la rejilla se pliega a la fila del
// cursor; en la agenda desaparece y debajo va la agenda de siempre (`vistaAgenda`). Cada
// cambio reconstruye la pantalla y la hoja nueva se anima desde el alto de la vieja,
// salvo con `ui.SinMovimiento` (el `prefers-reduced-motion` de la web).
//
// Tocar un día lo ELIGE (no crea: crear es el «+» de la barra, sobre el día elegido).
// Teclado: ←→ un día, ↑↓ una semana, Inicio/Fin lunes/domingo, RePág/AvPág un mes
// (recortando el día); el foco sigue a la selección. Con el dedo, deslizar sobre los DÍAS
// cambia de mes (plegado, de semana) con `ui.DecidirDeslizamiento`; la columna de semana
// no desliza. Los 24 px del borde izquierdo son además el gesto de abrir el menú de toda
// app Fyne (`bordeMenuMovil`, gestosmoviles.go), que la raíz captura antes: caen dentro de
// esa columna, así que los dos gestos no se pisan.
//
// La geometría no depende de los datos: mientras carga, la hoja es la misma sin puntos y
// la lista enseña su esqueleto.

// Medidas de la captura del dueño (iPhone de 390) redondeadas a la rejilla de 4, con la
// clase de la web al lado.
const (
	AnchoColSemana     float32 = 44 // w-11 / `2.75rem` de la plantilla de columnas
	AltoDiasCompacta   float32 = 24 // h-6, la fila de iniciales
	AltoSemanaCompacta float32 = 40 // h-10, cada semana
	LadoNumeroCompacto float32 = 28 // h-7 w-7, la caja del número (y del elegido)
	LadoPastillaSemana float32 = 22 // h-5.5 w-5.5, la pastilla del número de semana
	LadoPuntoCompacto  float32 = 5  // h-1.25 w-1.25, cada punto
	HuecoPuntos        float32 = 3  // gap-[3px]
	AltoAsaMes         float32 = 32 // h-8, el botón del asa
	AnchoBarritaAsa    float32 = 36 // w-9
	AltoBarritaAsa     float32 = 4  // h-1
	LadoPuntoFila      float32 = 10 // h-2.5 w-2.5, el punto de una fila de la lista
)

// DuracionHoja es el `duration-200 ease-out` con que la hoja llega a su posición.
const DuracionHoja = 200 * time.Millisecond

// relojAsa es el reloj del gesto del asa. Variable por los guardianes: el driver de
// pruebas entrega los arrastres todos en el mismo instante, que serían un lanzamiento.
var relojAsa = time.Now

// empezarAnimacion arranca la animación de la hoja. Variable por los guardianes: el driver
// de pruebas la acaba en cuanto empieza, y así se puede mirar a medio camino.
var empezarAnimacion = func(a *fyne.Animation) { a.Start() }

var (
	// arribaNumero es el `pt-0.5` del botón del día; arribaPastilla el `pt-[5px]` que
	// centra la pastilla con el número, y derPastilla su `pr-1.5`.
	arribaNumero         = tema.Espacio(0.5)
	arribaPastilla       = float32(5)
	derPastilla          = tema.Espacio(1.5)
	espacio4             = tema.Espacio(4) // px-4 / pt-4 de la lista
	altoMinFilaDia       = tema.Espacio(12)
	arribaPuntoFila      = float32(5)      // mt-[5px]
	ladoGlifoHoja        = tema.Espacio(5) // h-5 w-5 del glifo de «Hoy»
	columnasDiasCompacta = ui.Rejilla{Fijas: 7}
)

// conDedo es `pointerType !== 'mouse'`. Variable por los guardianes: el driver de pruebas
// es de escritorio y aquí hay que medir también el teléfono.
var conDedo = ui.EsTactil

// compacta dice si la vista va en su forma de teléfono: VENTANA < 640 y mes, semana o
// agenda, que son las tres posiciones del asa. El día sigue siendo el de siempre, y por
// encima de 640 la semana es la rejilla horaria.
func (p *Pantalla) compacta() bool {
	_, conAsa := PosicionDeVista(p.Vista)
	return conAsa && p.Ventana > 0 && p.Ventana < ui.CorteSM
}

// posicion es la del asa: la vista.
func (p *Pantalla) posicion() PosicionAsa {
	pos, _ := PosicionDeVista(p.Vista)
	return pos
}

// irAPosicion es lo que pide el asa (tocar, teclado o soltar): cambiar de vista.
func (p *Pantalla) irAPosicion(pos PosicionAsa) {
	if pos != p.posicion() {
		p.cambiarVista(VistaDePosicion(pos))
	}
}

// ahora es `new Date()`: el reloj de la pantalla, o el del sistema.
func (p *Pantalla) ahora() time.Time {
	if p.Ahora != nil {
		return p.Ahora().In(p.zona())
	}
	return time.Now().In(p.zona())
}

// AnchoTelefonoSupuesto es el ancho que se supone en un teléfono en vertical mientras
// la ventana no se ha medido: el de la captura del dueño (un iPhone de 390).
const AnchoTelefonoSupuesto float32 = 390

// VentanaInicial es el ancho con el que la pantalla pinta la PRIMERA vez, antes de que
// el armazón mida la ventana en su primer Layout (`AlMedir`). Sin sembrarlo, `Ventana`
// era 0 y el primer pintado en un teléfono era el de escritorio —rejilla de seis
// semanas, cabecera y botón flotante— hasta que `AlRehacer` saltaba al mes compacto. En
// un dispositivo móvil en vertical se supone un teléfono, y el primer pintado ya es el
// compacto; la medida solo corrige (una tableta en vertical, de 768, salta al de
// escritorio en ese primer Layout). Fuera de eso, 0: sin medir no es compacto. Es la
// pareja de las clases `max-sm:hidden` de la web, que tampoco enseñan la forma de
// escritorio por debajo de 640 antes de que `useEstrecha` resuelva.
func VentanaInicial(movil, vertical bool) float32 {
	if movil && vertical {
		return AnchoTelefonoSupuesto
	}
	return 0
}

// medirVentana es el `AlMedir` del armazón: guarda el ancho y, si cruza el corte del mes
// compacto, pide reconstruir FUERA del Layout.
func (p *Pantalla) medirVentana(ancho float32) {
	antes := p.compacta()
	p.Ventana = ancho
	if p.compacta() != antes && p.AlRehacer != nil {
		p.AlRehacer()
	}
}

// pendientesMenu es lo que hay que mirar dentro del cajón: recordatorios vencidos e
// invitaciones sin contestar. En el teléfono ya no se ven en ninguna cabecera.
func (p *Pantalla) pendientesMenu() int { return len(p.Recordatorios) + p.Invitaciones }

// accionesBarraCompacta son «Hoy» con forma de hoja y «+» sobre el día elegido.
func (p *Pantalla) accionesBarraCompacta() []fyne.CanvasObject {
	l, hoy, cursor := p.locale(), p.ahora(), p.Cursor.In(p.zona())
	h := nuevoBotonHoyHoja(hoy.Day(), p.t("grid.todayLabel", i18n.Vars{"date": i18n.FormatDayMonthLong(l, hoy)}), func() {
		if p.AlIrHoy != nil {
			p.AlIrHoy()
		}
	})
	mas := ui.NuevoBotonBarraMovil(iconos.Mas, p.t("nav.createEventOn", i18n.Vars{"date": i18n.FormatDayMonthLong(l, cursor)}), p.crearHoy)
	mas.TamIcono = 20 // `<Icon name="plus" size={20} />`
	mas.Inerte = p.SinConexion
	return []fyne.CanvasObject{h, mas}
}

// vistaMesCompacta es la vista entera: la hoja de la rejilla y, debajo, la lista del día
// (o el panel de error en su sitio), desplazable y con tirar para refrescar.
func (p *Pantalla) vistaMesCompacta() fyne.CanvasObject {
	cargando := p.Estado == agenda.Cargando
	eventos := p.Eventos
	if cargando {
		eventos = nil // la misma hoja, sin puntos
	}
	m := nuevoMesCompacto(p, eventos)
	enAgenda := p.posicion() == PosAgenda
	var abajo fyne.CanvasObject
	switch p.Estado {
	case agenda.Error, agenda.SinPermiso, agenda.SinConexion:
		enAgenda = false // el panel va bajo la hoja, desplazable, como en el mes
		abajo = container.New(ui.Margen{Izq: tema.Espacio(6), Arr: tema.Espacio(6), Der: tema.Espacio(6), Abj: tema.Espacio(6)},
			agenda.Panel(p.Estado, p.textosDeEstado(), nil))
	case agenda.Cargando:
		abajo = p.listaDia(true)
		if enAgenda {
			abajo = esqueletoAgendaCompacta()
		}
	case agenda.Exito, agenda.Vacia:
		abajo = p.listaDia(false)
		if enAgenda {
			// La agenda de siempre, con su buscador, su desplazamiento y su tirar para
			// refrescar: el asa se queda arriba.
			abajo = p.vistaAgenda()
		}
		if p.Recargando {
			abajo = container.NewStack(abajo, nuevaCortina())
		}
	}
	sombra := ui.NuevaSombra(tema.SombraSM, 0)
	cuerpo := container.New(&hojaYLista{sombra: sombra}, sombra.Objeto(), m, abajo)
	fondo := canvas.NewRectangle(tema.Color("surface", tema.De(fyne.CurrentApp().Settings().ThemeVariant())))
	if enAgenda {
		m.alCambiarAlto = cuerpo.Refresh
		m.arrancar()
		return container.NewStack(fondo, cuerpo)
	}
	desplazar, vista := desplazable(container.NewStack(fondo, cuerpo))
	m.desplazar = desplazar
	m.alCambiarAlto = func() {
		cuerpo.Refresh()
		desplazar.Refresh()
	}
	m.arrancar()
	refresco := p.Refresco
	refresco.Arriba = func() bool { return desplazar.Offset.Y <= 0.5 }
	return ui.RefrescarAlTirar(vista, &refresco)
}

// esqueletoAgendaCompacta es la agenda del teléfono mientras carga: tres filas como las
// de la lista del día (`pt-2` y `EsqueletoListaDia` en la web).
func esqueletoAgendaCompacta() fyne.CanvasObject {
	filas := make([]fyne.CanvasObject, 0, 3)
	for range 3 {
		filas = append(filas, esqueletoFilaDia())
	}
	return container.New(ui.Margen{Arr: espacio2}, container.New(ui.Columna{}, filas...))
}

// hojaYLista pone la hoja arriba con su sombra debajo (`shadow-sm` y `z-10`) y la lista
// a continuación. objs[0] es la sombra, objs[1] la hoja y objs[2] lo de abajo.
type hojaYLista struct{ sombra *ui.Sombra }

func (h *hojaYLista) MinSize(objs []fyne.CanvasObject) fyne.Size {
	a, b := objs[1].MinSize(), objs[2].MinSize()
	return fyne.NewSize(max(a.Width, b.Width), a.Height+b.Height)
}

func (h *hojaYLista) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	alto := objs[1].MinSize().Height
	objs[1].Move(fyne.NewPos(0, 0))
	objs[1].Resize(fyne.NewSize(t.Width, alto))
	h.sombra.Ajustar(fyne.NewPos(0, 0), fyne.NewSize(t.Width, alto), tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	objs[2].Move(fyne.NewPos(0, alto))
	objs[2].Resize(fyne.NewSize(t.Width, max(t.Height-alto, objs[2].MinSize().Height)))
}

// listaDia es la sección de debajo: la fecha completa y los eventos del día elegido, el
// vacío, o tres filas de esqueleto mientras carga.
func (p *Pantalla) listaDia(cargando bool) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// En la zona de la pantalla, como la rejilla (el cursor puede venir en otra).
	titulo := renglon(i18n.FormatFullDate(p.locale(), p.Cursor.In(p.zona())), tema.TextoMD, ui.SemiNegrita, "text",
		ui.Margen{Izq: espacio4, Arr: espacio4, Der: espacio4, Abj: espacio2})
	filas := []fyne.CanvasObject{titulo}
	switch dia := EventosDelDia(p.Cursor, p.zona(), p.Eventos); {
	case cargando:
		for range 3 {
			filas = append(filas, esqueletoFilaDia())
		}
	case len(dia) == 0:
		filas = append(filas, renglon(p.t("day.noEvents", nil), tema.TextoSM, ui.Normal, "text-muted",
			ui.Margen{Izq: espacio4, Arr: espacio2, Der: espacio4, Abj: espacio2}))
	default:
		for _, f := range dia {
			filas = append(filas, p.nuevaFilaEventoDia(f, v))
		}
	}
	return container.New(ui.Margen{Abj: espacio4}, container.New(ui.Columna{}, filas...))
}

// esqueletoFilaDia es una fila de la lista sin datos: el punto y las dos líneas.
func esqueletoFilaDia() fyne.CanvasObject {
	return container.New(&filaEsqueleto{},
		ui.NuevoEsqueletoRedondo(LadoPuntoFila),
		agenda.Bloque(5*tema.Rem, 0.875*tema.Rem, false),
		agenda.Bloque(8*tema.Rem, tema.Rem, false))
}

// filaEsqueleto coloca el esqueleto con la geometría de `filaEventoDia`.
type filaEsqueleto struct{}

func (filaEsqueleto) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, altoMinFilaDia) }

func (filaEsqueleto) Layout(objs []fyne.CanvasObject, _ fyne.Size) {
	objs[0].Move(fyne.NewPos(espacio4, espacio2+arribaPuntoFila+1))
	objs[0].Resize(fyne.NewSize(LadoPuntoFila, LadoPuntoFila))
	x := espacio4 + LadoPuntoFila + espacio3
	objs[1].Move(fyne.NewPos(x, espacio2))
	objs[1].Resize(objs[1].MinSize())
	objs[2].Move(fyne.NewPos(x, espacio2+0.875*tema.Rem+espacio1p5))
	objs[2].Resize(objs[2].MinSize())
}

// cuandoFila es la primera línea de una fila: «Todo el día», «14:00 – 15:00», solo la
// hora sin fin, o las dos fechas si cruza de día.
func (p *Pantalla) cuandoFila(f FilaDia) string {
	l := p.locale()
	switch {
	case f.TodoElDia:
		return p.t("event.allDay", nil)
	case f.Cruza:
		return p.t("when.rangeAcross", i18n.Vars{
			"startDate": i18n.FormatDayMonthLong(l, f.Inicio), "startTime": i18n.FormatTimeDigits(l, f.Inicio),
			"endDate": i18n.FormatDayMonthLong(l, f.Fin), "endTime": i18n.FormatTimeDigits(l, f.Fin),
		})
	case !f.Fin.IsZero() && f.Fin.After(f.Inicio):
		return p.t("when.timeRange", i18n.Vars{"start": i18n.FormatTimeDigits(l, f.Inicio), "end": i18n.FormatTimeDigits(l, f.Fin)})
	}
	return i18n.FormatTimeDigits(l, f.Inicio)
}

// renglon es el `<h2 class="px-4 pt-4 pb-2">` y el `<p>` del vacío: un `ui.Rotulo`
// truncado con su margen.
func renglon(texto string, e tema.Escalon, peso ui.Peso, token string, m ui.Margen) fyne.CanvasObject {
	r := ui.NuevoRotulo(texto, e, peso, token)
	r.Truncar = true
	return container.New(m, r)
}

// ---- mesCompacto: la hoja ----

// mesCompacto es la hoja de la rejilla: iniciales de los días, las semanas con su
// pastilla y sus días, y el asa. Recibe el deslizar de mes.
//
// Siempre lleva las semanas del MES, para que la hoja pueda desplegarse con el dedo desde
// la semana o la agenda sin saltos; fuera del mes, la fila del cursor es la semana plegada
// (con los días del mes vecino). Su alto es el de la posición, el que lleva el dedo o el
// del fotograma de la animación, y RECORTA lo que queda fuera: es `fyne.Scrollable` solo
// para eso (el pintor recorta a los Scrollable) y la rueda la pasa al desplazable.
type mesCompacto struct {
	widget.BaseWidget

	p          *Pantalla
	semanas    []SemanaCompacta
	filaCursor int
	dias       []*diaCompacto // los días con fecha, por fila
	pastis     []*pastillaSemana
	asa        *asaMes
	// desplazar es el desplazable de la vista: un arrastre VERTICAL sobre la rejilla se
	// le devuelve, que es el `touch-pan-y` de la web. nil en la agenda, que desplaza sola.
	desplazar *container.Scroll
	// alCambiarAlto recoloca lo de alrededor cuando la hoja cambia de alto.
	alCambiarAlto func()

	// El alto: arrastrando, lo recorrido; animando, el del fotograma.
	arrastrando bool
	dyAsa       float32
	animando    bool
	altoAnim    float32

	// El gesto en curso: lo recorrido, desde cuándo y qué es.
	dx, dy       float32
	desde        time.Time
	descartado   bool // empezó en la columna de semana, o con el ratón
	haciaDesplaz bool // es vertical: va al desplazable
}

var (
	_ fyne.Widget     = (*mesCompacto)(nil)
	_ fyne.Draggable  = (*mesCompacto)(nil)
	_ fyne.Accessible = (*mesCompacto)(nil)
	_ fyne.Scrollable = (*mesCompacto)(nil)
)

func nuevoMesCompacto(p *Pantalla, eventos []Ocurrencia) *mesCompacto {
	m := &mesCompacto{p: p}
	hoy := p.ahora()
	pos := p.posicion()
	m.semanas = SemanasCompactas(p.Cursor, hoy, p.zona(), eventos)
	cursor := inicioDelDia(p.Cursor, p.zona())
	for i, w := range m.semanas {
		for _, d := range w.Dias {
			if !d.Hueco && mismoDia(d.Fecha, cursor) {
				m.filaCursor = i
			}
		}
	}
	if pos != PosMes {
		m.semanas[m.filaCursor] = SemanaPlegada(p.Cursor, hoy, p.zona(), eventos)
	}
	p.diaElegido = nil
	for _, w := range m.semanas {
		m.pastis = append(m.pastis, nuevaPastillaSemana(w.Numero, p.t("grid.weekNumber", i18n.Vars{"week": w.Numero})))
		for _, d := range w.Dias {
			if d.Hueco {
				m.dias = append(m.dias, nil)
				continue
			}
			dc := nuevoDiaCompacto(p, d)
			// En la agenda la rejilla no está: no hay día al que devolver el foco.
			if d.Elegido && pos != PosAgenda {
				p.diaElegido = dc
			}
			m.dias = append(m.dias, dc)
		}
	}
	m.asa = nuevaAsaMes(p, m)
	p.asaMes = m.asa
	m.ExtendBaseWidget(m)
	return m
}

// altoReposo es el alto de la hoja (sin el asa) en la posición de la vista.
func (m *mesCompacto) altoReposo() float32 {
	return AlturaHoja(m.p.posicion(), len(m.semanas))
}

// alto es el de ahora: el del dedo, el del fotograma o el de reposo.
func (m *mesCompacto) alto() float32 {
	switch {
	case m.arrastrando:
		return AlturaArrastre(m.p.posicion(), m.dyAsa, len(m.semanas))
	case m.animando:
		return m.altoAnim
	}
	return m.altoReposo()
}

// quieta es en reposo: ni arrastrando ni animando. Solo entonces se esconde lo que no se
// ve (el `inert` de la web), para que el foco no caiga en una semana recortada.
func (m *mesCompacto) quieta() bool { return !m.arrastrando && !m.animando }

// arrancar anima la hoja desde el alto del pintado anterior hasta el de su posición. Sin
// movimiento, o sin pintado anterior, sale ya en su sitio.
func (m *mesCompacto) arrancar() {
	p := m.p
	if p.animHoja != nil {
		p.animHoja.Stop()
		p.animHoja = nil
	}
	desde, hasta := p.altoHoja, m.altoReposo()
	if !p.hayAltoHoja || desde == hasta || ui.SinMovimiento {
		m.anotarAlto()
		return
	}
	m.animando, m.altoAnim = true, desde
	a := fyne.NewAnimation(DuracionHoja, func(f float32) { m.fotograma(desde, hasta, f) })
	a.Curve = fyne.AnimationEaseOut
	p.animHoja = a
	empezarAnimacion(a)
}

// fotograma es un paso de la animación: `f` va de 0 a 1.
func (m *mesCompacto) fotograma(desde, hasta, f float32) {
	m.altoAnim = desde + (hasta-desde)*f
	if f >= 1 {
		m.animando = false
		if m.p.animHoja != nil {
			m.p.animHoja = nil
		}
	}
	m.reajustar()
}

// anotarAlto guarda el alto de ahora: desde él se anima la hoja del pintado siguiente.
func (m *mesCompacto) anotarAlto() {
	m.p.altoHoja, m.p.hayAltoHoja = m.alto(), true
}

// reajustar repinta la hoja con su alto nuevo y recoloca lo de alrededor.
func (m *mesCompacto) reajustar() {
	m.anotarAlto()
	m.Refresh()
	if m.alCambiarAlto != nil {
		m.alCambiarAlto()
	}
}

// AccessibilityLabel: la rejilla se anuncia con el mes que enseña.
func (m *mesCompacto) AccessibilityLabel() string {
	return i18n.FormatMonthYear(m.p.locale(), m.p.Cursor.In(m.p.zona()))
}

// AccessibilityRole: es un grupo de días, no un control.
func (m *mesCompacto) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleContainer
}

// Scrolled pasa la rueda al desplazable: la hoja solo es Scrollable para recortar.
func (m *mesCompacto) Scrolled(e *fyne.ScrollEvent) {
	if m.desplazar != nil {
		m.desplazar.Scrolled(e)
	}
}

// Dragged sigue al dedo. Lo que empieza en la columna de semana, o con el ratón, no
// cuenta; lo que es más vertical que horizontal es desplazar la página.
func (m *mesCompacto) Dragged(e *fyne.DragEvent) {
	if m.desde.IsZero() {
		m.desde = time.Now()
		inicio := e.Position.X - e.Dragged.DX
		m.descartado = !conDedo() || inicio < AnchoColSemana
		m.haciaDesplaz = m.descartado
	}
	m.dx += e.Dragged.DX
	m.dy += e.Dragged.DY
	if !m.descartado && !m.haciaDesplaz && abs32(m.dy) > ui.DominioDeslizamiento*abs32(m.dx) {
		m.haciaDesplaz = true
	}
	if m.haciaDesplaz {
		if d, es := any(m.desplazar).(fyne.Draggable); es && m.desplazar != nil {
			d.Dragged(e)
		}
	}
}

// DragEnd decide: un mes (en la semana, una semana) hacia donde se deslizó.
func (m *mesCompacto) DragEnd() {
	descartado, haciaDesplaz := m.descartado, m.haciaDesplaz
	dx, dy, desde := m.dx, m.dy, m.desde
	m.dx, m.dy, m.desde, m.descartado, m.haciaDesplaz = 0, 0, time.Time{}, false, false
	if haciaDesplaz {
		if d, es := any(m.desplazar).(fyne.Draggable); es && m.desplazar != nil {
			d.DragEnd()
		}
	}
	if descartado || desde.IsZero() {
		return
	}
	paso := ui.DecidirDeslizamiento(dx, dy, max(m.Size().Width-AnchoColSemana, 0), float64(time.Since(desde).Milliseconds()))
	if paso == 0 {
		return
	}
	m.p.moverCompacto(paso)
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// moverCompacto es un paso del deslizar: un mes en el mes, una semana en la semana.
func (p *Pantalla) moverCompacto(paso int) {
	if p.Vista == VistaSemana {
		if p.AlElegirDia != nil {
			p.AlElegirDia(p.Cursor.AddDate(0, 0, 7*paso))
		}
		return
	}
	if p.AlDesplazarMes != nil {
		p.AlDesplazarMes(paso)
	}
}

// MinSize es la geometría de la posición (o del dedo): no depende de los datos.
func (m *mesCompacto) MinSize() fyne.Size {
	return fyne.NewSize(AnchoColSemana, m.alto()+AltoAsaMes)
}

func (m *mesCompacto) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &mesCompactoRenderer{
		m: m, fondo: canvas.NewRectangle(tema.Color("surface", v)),
		fondoDias: canvas.NewRectangle(tema.Color("surface", v)),
		fondoAsa:  canvas.NewRectangle(tema.Color("surface", v)),
	}
	// Las semanas van ANTES que la fila de iniciales, que lleva su fondo: al plegarse,
	// las que no tocan suben y pasan por debajo de ella, como el `z-[1]` de la web.
	r.objetos = []fyne.CanvasObject{r.fondo}
	for _, p := range m.pastis {
		r.objetos = append(r.objetos, p)
	}
	for _, d := range m.dias {
		if d != nil {
			r.objetos = append(r.objetos, d)
		}
	}
	r.objetos = append(r.objetos, r.fondoDias)
	for _, n := range i18n.WeekdaysShort(m.p.locale()) {
		t := ui.Texto(n, tema.TextoSM, ui.Normal, tema.Color("text-subtle", v))
		r.iniciales = append(r.iniciales, t)
		r.objetos = append(r.objetos, t)
	}
	// El asa lleva fondo: lo que el recorte deja asomar por debajo de la hoja (la semana
	// a medio plegar) no se ve a través de ella. En la web el asa está fuera del recorte.
	r.objetos = append(r.objetos, r.fondoAsa, m.asa)
	return r
}

type mesCompactoRenderer struct {
	m                          *mesCompacto
	fondo, fondoDias, fondoAsa *canvas.Rectangle
	iniciales                  []*canvas.Text
	objetos                    []fyne.CanvasObject
}

func (r *mesCompactoRenderer) MinSize() fyne.Size           { return r.m.MinSize() }
func (r *mesCompactoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *mesCompactoRenderer) Destroy()                     {}

// x0 y ancho de la columna c de días, sobre lo que deja la columna de semana.
func columnaCompacta(ancho float32, c int) (x, w float32) {
	util := max(ancho-AnchoColSemana, 0)
	return AnchoColSemana + columnasDiasCompacta.X(util, c), columnasDiasCompacta.AnchoColumna(util)
}

// Layout coloca la hoja al alto de ahora con `GeometriaHoja`: las semanas suben bajo las
// iniciales al plegarse y, por debajo de la semana, sube todo. El asa va justo debajo.
func (r *mesCompactoRenderer) Layout(t fyne.Size) {
	m := r.m
	h := m.alto()
	pos, quieta := m.p.posicion(), m.quieta()
	plegado, subida := GeometriaHoja(h, len(m.semanas), m.filaCursor)
	r.fondo.Resize(t) // también bajo el asa: debajo va la sombra de la hoja
	verTodo := !quieta || pos == PosMes
	verHoja := !quieta || pos != PosAgenda
	r.fondoDias.Move(fyne.NewPos(0, -subida))
	r.fondoDias.Resize(fyne.NewSize(t.Width, AltoDiasCompacta))
	r.fondoDias.Hidden = !verHoja
	for c, txt := range r.iniciales {
		x, w := columnaCompacta(t.Width, c)
		n := ui.Medir(txt.Text, tema.TextoSM, ui.Normal).Width
		ui.ColocarTexto(txt, tema.TextoSM, x+(w-n)/2, (AltoDiasCompacta-tema.TextoSM.Linea)/2-subida)
		txt.Hidden = !verHoja
	}
	i := 0
	for w := range m.semanas {
		ver := verHoja && (verTodo || w == m.filaCursor)
		y := AltoDiasCompacta + float32(w)*AltoSemanaCompacta - plegado - subida
		p := m.pastis[w]
		p.Move(fyne.NewPos(AnchoColSemana-derPastilla-LadoPastillaSemana, y+arribaPastilla))
		p.Resize(fyne.NewSize(LadoPastillaSemana, LadoPastillaSemana))
		mostrar(p, ver)
		for c := range 7 {
			d := m.dias[i]
			i++
			if d == nil {
				continue
			}
			x, ancho := columnaCompacta(t.Width, c)
			d.Move(fyne.NewPos(x, y))
			d.Resize(fyne.NewSize(ancho, AltoSemanaCompacta))
			mostrar(d, ver)
		}
	}
	r.fondoAsa.Move(fyne.NewPos(0, h))
	r.fondoAsa.Resize(fyne.NewSize(t.Width, AltoAsaMes))
	m.asa.Move(fyne.NewPos(0, h))
	m.asa.Resize(fyne.NewSize(t.Width, AltoAsaMes))
}

// mostrar enseña o esconde sin repintar si ya estaba así.
func mostrar(o fyne.CanvasObject, ver bool) {
	if ver && !o.Visible() {
		o.Show()
	} else if !ver && o.Visible() {
		o.Hide()
	}
}

func (r *mesCompactoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = tema.Color("surface", v)
	r.fondoDias.FillColor = tema.Color("surface", v)
	r.fondoAsa.FillColor = tema.Color("surface", v)
	for _, t := range r.iniciales {
		t.Color = tema.Color("text-subtle", v)
	}
	r.Layout(r.m.Size())
	canvas.Refresh(r.m)
}

// ---- pastillaSemana ----

// pastillaSemana es el número ISO de la semana: `bg-bg-subtle text-text-subtle text-2xs
// font-medium rounded-sm`. Widget solo para tener nombre accesible («Semana 45»).
type pastillaSemana struct {
	widget.BaseWidget
	n      int
	nombre string
}

func nuevaPastillaSemana(n int, nombre string) *pastillaSemana {
	p := &pastillaSemana{n: n, nombre: nombre}
	p.ExtendBaseWidget(p)
	return p
}

func (p *pastillaSemana) AccessibilityLabel() string             { return p.nombre }
func (p *pastillaSemana) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

func (p *pastillaSemana) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &pastillaRenderer{
		p: p, fondo: canvas.NewRectangle(tema.Color("bg-subtle", v)),
		txt: ui.Texto(strconv.Itoa(p.n), tema.Texto2XS, ui.Medio, tema.Color("text-subtle", v)),
	}
	r.fondo.CornerRadius = tema.RadioSM
	return r
}

type pastillaRenderer struct {
	p     *pastillaSemana
	fondo *canvas.Rectangle
	txt   *canvas.Text
}

func (r *pastillaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(LadoPastillaSemana, LadoPastillaSemana)
}

func (r *pastillaRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.fondo, r.txt}
}
func (r *pastillaRenderer) Destroy() {}
func (r *pastillaRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	n := ui.Medir(r.txt.Text, tema.Texto2XS, ui.Medio).Width
	ui.ColocarTexto(r.txt, tema.Texto2XS, (t.Width-n)/2, (t.Height-tema.Texto2XS.Linea)/2)
}

func (r *pastillaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = tema.Color("bg-subtle", v)
	r.txt.Color = tema.Color("text-subtle", v)
	r.Layout(r.p.Size())
	r.fondo.Refresh()
	r.txt.Refresh()
}

// ---- diaCompacto ----

// diaCompacto es un día: un botón con su número y sus puntos. Elegido, la caja del
// número va rellena del primario; hoy sin elegir, el número en `primary-text` y
// semi-negrita; de otro mes (plegado), en `text-subtle`.
//
// Solo el elegido está en el recorrido del tabulador (el tabindex itinerante de la web):
// los demás dicen `Disabled()` sin estar inertes, como `Boton.EnElRecorrido`.
type diaCompacto struct {
	ui.Pulsable
	p *Pantalla
	d DiaCompacto
}

var (
	_ fyne.Widget      = (*diaCompacto)(nil)
	_ fyne.Disableable = (*diaCompacto)(nil)
	_ fyne.Focusable   = (*diaCompacto)(nil)
)

func nuevoDiaCompacto(p *Pantalla, d DiaCompacto) *diaCompacto {
	dc := &diaCompacto{p: p, d: d}
	fecha := d.Fecha
	dc.AlPulsar = func() {
		if p.AlElegirDia != nil {
			p.AlElegirDia(fecha)
		}
	}
	dc.ExtendBaseWidget(dc)
	return dc
}

// Disabled saca del tabulador a los no elegidos. Siguen pulsándose.
func (d *diaCompacto) Disabled() bool { return !d.d.Elegido }

// Enable y Disable completan `fyne.Disableable`; la selección la decide el cursor.
func (d *diaCompacto) Enable()  {}
func (d *diaCompacto) Disable() {}

// AccessibilityLabel: «viernes, 6 de noviembre» y, si toca, «Hoy, …», «…, 2 eventos» y
// «…, seleccionado». Fyne no tiene `aria-current` ni `aria-pressed`, así que hoy y la
// selección van en el nombre.
func (d *diaCompacto) AccessibilityLabel() string {
	p := d.p
	n := i18n.FormatWeekdayDayMonth(p.locale(), d.d.Fecha)
	if d.d.Hoy {
		n = p.t("grid.todayLabel", i18n.Vars{"date": n})
	}
	if d.d.Eventos > 0 {
		n = p.t("grid.dayEvents", i18n.Vars{"date": n, "count": d.d.Eventos})
	}
	if d.d.Elegido {
		n = p.t("grid.daySelected", i18n.Vars{"label": n})
	}
	return n
}

// TypedKey mueve la selección con las flechas, Inicio/Fin y RePág/AvPág; lo demás es
// lo de un botón.
//
// Con modificador no llega aquí, y por eso no hace falta el `if (e.altKey || e.ctrlKey ||
// e.metaKey) return` de la web: el driver de escritorio (y el de wasm, que es el mismo
// window.go) convierte Ctrl/Alt/Super + tecla en un `desktop.CustomShortcut` y, si el
// enfocado no es `fyne.Shortcutable` —el día no lo es—, se lo da al lienzo y NO llama a
// TypedKey. Mayúsculas sola sí llega, como en la web, que tampoco la filtra. Lo vigila
// TestUnDiaNoEsShortcutable.
func (d *diaCompacto) TypedKey(e *fyne.KeyEvent) {
	c := d.d.Fecha
	lunes := LunesDe(c)
	var destino time.Time
	switch e.Name {
	case fyne.KeyLeft:
		destino = c.AddDate(0, 0, -1)
	case fyne.KeyRight:
		destino = c.AddDate(0, 0, 1)
	case fyne.KeyUp:
		destino = c.AddDate(0, 0, -7)
	case fyne.KeyDown:
		destino = c.AddDate(0, 0, 7)
	case fyne.KeyHome:
		destino = lunes
	case fyne.KeyEnd:
		destino = lunes.AddDate(0, 0, 6)
	case fyne.KeyPageUp:
		destino = MoverMes(c, -1)
	case fyne.KeyPageDown:
		destino = MoverMes(c, 1)
	default:
		d.Pulsable.TypedKey(e)
		return
	}
	if d.p.AlElegirDia != nil {
		d.p.AlElegirDia(destino)
	}
}

// TypedRune activa con Espacio y deja SUBIR las demás letras a la ventana: con un día
// enfocado siguen valiendo los atajos de la app (`t`, `n`, `w`…), como en la web.
func (d *diaCompacto) TypedRune(r rune) {
	if r == ' ' {
		d.Pulsable.TypedRune(r)
		return
	}
	ui.SubirRuna(d.Lienzo(), r)
}

func (d *diaCompacto) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	peso := ui.Normal
	if d.d.Hoy {
		peso = ui.SemiNegrita
	}
	r := &diaCompactoRenderer{
		d:      d,
		peso:   peso,
		aro:    ui.NuevoAro(tema.RadioMD, ui.SinHueco, v).Abierto(),
		velo:   canvas.NewRectangle(color.NRGBA{}),
		caja:   canvas.NewRectangle(color.NRGBA{}),
		numero: ui.Texto(strconv.Itoa(d.d.Fecha.Day()), tema.TextoMD, peso, tema.Color("text", v)),
	}
	r.velo.CornerRadius, r.caja.CornerRadius = tema.RadioMD, tema.RadioMD
	for _, c := range d.d.Puntos {
		r.puntos = append(r.puntos, canvas.NewCircle(c))
	}
	r.objetos = r.aro.Antes(r.velo, r.caja, r.numero)
	for _, c := range r.puntos {
		r.objetos = append(r.objetos, c)
	}
	r.Refresh()
	return r
}

type diaCompactoRenderer struct {
	d          *diaCompacto
	peso       ui.Peso
	aro        *ui.Aro
	velo, caja *canvas.Rectangle
	numero     *canvas.Text
	puntos     []*canvas.Circle
	objetos    []fyne.CanvasObject
}

func (r *diaCompactoRenderer) MinSize() fyne.Size {
	return fyne.NewSize(LadoNumeroCompacto, AltoSemanaCompacta)
}
func (r *diaCompactoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *diaCompactoRenderer) Destroy()                     {}

func (r *diaCompactoRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.velo.Resize(t)
	x := (t.Width - LadoNumeroCompacto) / 2
	r.caja.Move(fyne.NewPos(x, arribaNumero))
	r.caja.Resize(fyne.NewSize(LadoNumeroCompacto, LadoNumeroCompacto))
	n := ui.Medir(r.numero.Text, tema.TextoMD, r.peso).Width
	ui.ColocarTexto(r.numero, tema.TextoMD, (t.Width-n)/2, arribaNumero+(LadoNumeroCompacto-tema.TextoMD.Linea)/2)
	// Los puntos: `mt-0.5 flex gap-[3px]`, centrados bajo la caja.
	k := float32(len(r.puntos))
	ancho := k*LadoPuntoCompacto + max(k-1, 0)*HuecoPuntos
	px := (t.Width - ancho) / 2
	y := arribaNumero + LadoNumeroCompacto + arribaNumero
	for _, c := range r.puntos {
		c.Move(fyne.NewPos(px, y))
		c.Resize(fyne.NewSize(LadoPuntoCompacto, LadoPuntoCompacto))
		px += LadoPuntoCompacto + HuecoPuntos
	}
}

func (r *diaCompactoRenderer) Refresh() {
	d := r.d.d
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.velo.FillColor = color.NRGBA{}
	if r.d.Encima() && !d.Elegido {
		r.velo.FillColor = tema.Color("bg-subtle", v)
	}
	r.caja.FillColor = color.NRGBA{}
	switch {
	case d.Elegido:
		r.caja.FillColor = tema.Color("primary", v)
		r.numero.Color = tema.Color("on-primary", v)
	case d.Hoy:
		r.numero.Color = tema.Color("primary-text", v)
	case d.OtroMes:
		r.numero.Color = tema.Color("text-subtle", v)
	default:
		r.numero.Color = tema.Color("text", v)
	}
	r.aro.Pintar(v)
	r.aro.Ver(r.d.Enfocado())
	r.Layout(r.d.Size())
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// ---- asaMes ----

// asaMes es el asa (grabber): cambia entre mes, semana y agenda. Se arrastra (la hoja
// sigue al dedo y al soltar se ajusta con `PosicionAlSoltar`), se toca (mes ↔ semana; de
// la agenda, a la semana) o se maneja con el teclado: ↑/→ un paso hacia la agenda, ↓/← hacia
// el mes, Inicio al mes y Fin a la agenda, como el `slider` de la web. Fyne no tiene
// `aria-valuetext`: el nombre dice la vista de ahora y lo que hace («Semana. Cambiar entre
// mes, semana y agenda»).
type asaMes struct {
	ui.Pulsable
	m *mesCompacto

	// El gesto en curso: lo recorrido, desde cuándo y por dónde fue.
	dy       float32
	inicio   time.Time
	muestras []MuestraAsa
}

var (
	_ fyne.Draggable = (*asaMes)(nil)
	_ fyne.Focusable = (*asaMes)(nil)
)

func nuevaAsaMes(p *Pantalla, m *mesCompacto) *asaMes {
	a := &asaMes{m: m}
	vista := p.t("view.month", nil)
	switch p.posicion() {
	case PosSemana:
		vista = p.t("view.week", nil)
	case PosAgenda:
		vista = p.t("view.agenda", nil)
	case PosMes:
	}
	a.Nombre = p.t("grid.handleState", i18n.Vars{"view": vista})
	a.AlPulsar = func() { p.irAPosicion(PosicionAlTocar(p.posicion())) }
	a.ExtendBaseWidget(a)
	return a
}

// TypedKey: las flechas y Inicio/Fin mueven el asa; Intro y Espacio son el toque.
func (a *asaMes) TypedKey(e *fyne.KeyEvent) {
	p := a.m.p
	pos := p.posicion()
	switch e.Name {
	case fyne.KeyUp, fyne.KeyRight:
		p.irAPosicion(PosicionTrasPaso(pos, 1))
	case fyne.KeyDown, fyne.KeyLeft:
		p.irAPosicion(PosicionTrasPaso(pos, -1))
	case fyne.KeyHome:
		p.irAPosicion(PosMes)
	case fyne.KeyEnd:
		p.irAPosicion(PosAgenda)
	default:
		a.Pulsable.TypedKey(e)
	}
}

// Dragged lleva la hoja con el dedo. Hasta pasar `RecorridoToque` la hoja no se mueve:
// un temblor no es un arrastre.
func (a *asaMes) Dragged(e *fyne.DragEvent) {
	ahora := relojAsa()
	if a.inicio.IsZero() {
		a.inicio, a.dy, a.muestras = ahora, 0, []MuestraAsa{{0, 0}}
	}
	a.dy += e.Dragged.DY
	a.muestras = append(a.muestras, MuestraAsa{ahora.Sub(a.inicio), a.dy})
	if len(a.muestras) > 32 {
		a.muestras = a.muestras[1:]
	}
	m := a.m
	if !m.arrastrando && abs32(a.dy) < RecorridoToque {
		return
	}
	if m.p.animHoja != nil {
		m.p.animHoja.Stop()
		m.p.animHoja = nil
	}
	m.animando, m.arrastrando, m.dyAsa = false, true, a.dy
	m.reajustar()
}

// DragEnd suelta: la hoja va adonde dice el modelo, animada desde donde la dejó el dedo.
func (a *asaMes) DragEnd() {
	m, p := a.m, a.m.p
	dy, v := a.dy, VelocidadFinal(a.muestras)
	a.dy, a.inicio, a.muestras = 0, time.Time{}, nil
	if !m.arrastrando {
		return // un temblor: ni arrastre ni toque
	}
	nueva := PosicionAlSoltar(p.posicion(), dy, v, len(m.semanas))
	m.anotarAlto() // la hoja nueva se anima desde aquí
	m.arrastrando = false
	if nueva != p.posicion() {
		p.irAPosicion(nueva)
		return
	}
	// Se queda: vuelve a su sitio animada.
	m.arrancar()
	m.reajustar()
}

func (a *asaMes) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &asaRenderer{a: a, aro: ui.NuevoAro(tema.RadioMD, ui.SinHueco, v).Abierto(), barrita: canvas.NewRectangle(tema.Color("border-strong", v))}
	r.barrita.CornerRadius = AltoBarritaAsa / 2
	r.Refresh()
	return r
}

type asaRenderer struct {
	a       *asaMes
	aro     *ui.Aro
	barrita *canvas.Rectangle
}

func (r *asaRenderer) MinSize() fyne.Size { return fyne.NewSize(AnchoBarritaAsa, AltoAsaMes) }
func (r *asaRenderer) Objects() []fyne.CanvasObject {
	return r.aro.Antes(r.barrita)
}
func (r *asaRenderer) Destroy() {}
func (r *asaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.barrita.Move(fyne.NewPos((t.Width-AnchoBarritaAsa)/2, (t.Height-AltoBarritaAsa)/2))
	r.barrita.Resize(fyne.NewSize(AnchoBarritaAsa, AltoBarritaAsa))
}

func (r *asaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.barrita.FillColor = tema.Color("border-strong", v)
	r.aro.Pintar(v)
	r.aro.Ver(r.a.Enfocado())
	r.Layout(r.a.Size())
	r.barrita.Refresh()
}

// ---- filaEventoDia ----

// filaEventoDia es un evento de la lista: punto de 10, la hora en `text-sm text-muted`
// y el título en `text-md text`, truncado. Pulsarla abre el evento como siempre.
type filaEventoDia struct {
	ui.Pulsable
	color          color.NRGBA
	cuando, titulo string
}

func (p *Pantalla) nuevaFilaEventoDia(f FilaDia, _ tema.Variante) *filaEventoDia {
	fila := &filaEventoDia{color: colorPunto(f.O), cuando: p.cuandoFila(f), titulo: resumenDe(f.O, p.t("event.untitled", nil))}
	// «Tenis, 14:00 – 15:00»: el separador es del catálogo (`grid.blockAria`, el nombre
	// de un bloque de la semana), no una coma escrita aquí.
	fila.Nombre = p.t("grid.blockAria", i18n.Vars{"title": fila.titulo, "time": fila.cuando})
	o := f.O
	fila.AlPulsar = func() {
		if p.AlAbrirEvento != nil {
			p.AlAbrirEvento(o, fila)
		}
	}
	fila.ExtendBaseWidget(fila)
	return fila
}

func (f *filaEventoDia) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaEventoRenderer{
		f:      f,
		aro:    ui.NuevoAro(0, ui.SinHueco, v).Abierto(),
		velo:   canvas.NewRectangle(color.NRGBA{}),
		punto:  canvas.NewCircle(f.color),
		cuando: ui.Texto(f.cuando, tema.TextoSM, ui.Normal, tema.Color("text-muted", v)),
		titulo: ui.Texto(f.titulo, tema.TextoMD, ui.Normal, tema.Color("text", v)),
	}
	r.Refresh()
	return r
}

type filaEventoRenderer struct {
	f              *filaEventoDia
	aro            *ui.Aro
	velo           *canvas.Rectangle
	punto          *canvas.Circle
	cuando, titulo *canvas.Text
}

func (r *filaEventoRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, max(altoMinFilaDia, 2*espacio2+tema.TextoSM.Linea+tema.TextoMD.Linea))
}

func (r *filaEventoRenderer) Objects() []fyne.CanvasObject {
	return r.aro.Antes(r.velo, r.punto, r.cuando, r.titulo)
}
func (r *filaEventoRenderer) Destroy() {}
func (r *filaEventoRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.velo.Resize(t)
	r.punto.Move(fyne.NewPos(espacio4, espacio2+arribaPuntoFila))
	r.punto.Resize(fyne.NewSize(LadoPuntoFila, LadoPuntoFila))
	x := espacio4 + LadoPuntoFila + espacio3
	cabe := max(t.Width-x-espacio4, 0)
	ui.Truncar(r.cuando, r.f.cuando, tema.TextoSM, ui.Normal, cabe)
	ui.ColocarTexto(r.cuando, tema.TextoSM, x, espacio2)
	ui.Truncar(r.titulo, r.f.titulo, tema.TextoMD, ui.Normal, cabe)
	ui.ColocarTexto(r.titulo, tema.TextoMD, x, espacio2+tema.TextoSM.Linea)
}

func (r *filaEventoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.velo.FillColor = color.NRGBA{}
	if r.f.Encima() {
		r.velo.FillColor = tema.Color("bg-subtle", v)
	}
	r.cuando.Color = tema.Color("text-muted", v)
	r.titulo.Color = tema.Color("text", v)
	r.aro.Pintar(v)
	r.aro.Ver(r.f.Enfocado())
	r.Layout(r.f.Size())
	for _, o := range r.Objects() {
		o.Refresh()
	}
}

// ---- botonHoyHoja ----

// botonHoyHoja es «Hoy» en la barra: un botón de la barra (embebe `ui.BotonIcono`, así
// toma sus tonos, su foco y su pulsación) cuyo glifo es una hoja de calendario de 20 con
// borde de 1,5 y, dentro, el número del día de HOY en `text-2xs font-semibold`. El
// nombre accesible lleva ese número dentro («Hoy, 6 de noviembre»: WCAG 2.5.3).
type botonHoyHoja struct {
	ui.BotonIcono
	dia int
}

// rutaHoja es el contorno de la hoja: un cuadrado redondeado que llena la caja de 24 de
// los iconos menos medio trazo, con la esquina de `rounded-sm`.
const rutaHoja = "M5 .9h14a4.1 4.1 0 0 1 4.1 4.1v14a4.1 4.1 0 0 1-4.1 4.1H5a4.1 4.1 0 0 1-4.1-4.1V5A4.1 4.1 0 0 1 5 .9z"

func nuevoBotonHoyHoja(dia int, nombre string, alPulsar func()) *botonHoyHoja {
	b := &botonHoyHoja{dia: dia}
	// Los mismos campos que `ui.NuevoBotonBarraMovil`; los tonos los pone la barra.
	b.Lado, b.Radio = ui.LadoBotonBarraMovil(), tema.Forma("radius-control")
	// Hueca: la hoja es un borde (`border-[1.5px] border-current`), no una forma con el
	// relleno de acento de los iconos, que taparía el número.
	b.Glifo = iconos.TrazoDeRuta(rutaHoja, ladoGlifoHoja, color.NRGBA{}).Acento(color.NRGBA{})
	b.TamIcono = ladoGlifoHoja
	b.Trazo, b.TrazoEncima, b.FondoEncima = "primary-text", "primary-text", "selected"
	b.Rotulo, b.AlPulsar = nombre, alPulsar
	b.ExtendBaseWidget(b)
	return b
}

func (b *botonHoyHoja) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	base := b.BotonIcono.CreateRenderer()
	r := &hojaRenderer{
		WidgetRenderer: base, b: b,
		numero: ui.Texto(strconv.Itoa(b.dia), tema.Texto2XS, ui.SemiNegrita, tema.Color(b.Trazo, v)),
	}
	r.Refresh()
	return r
}

type hojaRenderer struct {
	fyne.WidgetRenderer
	b      *botonHoyHoja
	numero *canvas.Text
}

func (r *hojaRenderer) Objects() []fyne.CanvasObject {
	return append(append([]fyne.CanvasObject(nil), r.WidgetRenderer.Objects()...), r.numero)
}

func (r *hojaRenderer) Layout(t fyne.Size) {
	r.WidgetRenderer.Layout(t)
	n := ui.Medir(r.numero.Text, tema.Texto2XS, ui.SemiNegrita).Width
	ui.ColocarTexto(r.numero, tema.Texto2XS, (t.Width-n)/2, (t.Height-tema.Texto2XS.Linea)/2)
}

func (r *hojaRenderer) Refresh() {
	r.WidgetRenderer.Refresh()
	r.numero.Color = tema.Color(r.b.Trazo, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r.Layout(r.b.Size())
	r.numero.Refresh()
}
