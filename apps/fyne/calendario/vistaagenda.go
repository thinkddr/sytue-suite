// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image/color"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// La vista AGENDA: `AgendaView` y `AgendaRow` de CalendarGrid.tsx. Los números salen de
// sus clases con el rem de 14:
//
//	buscador          `border-b p-2`, campo `h-9`                          31,5 + 14 + 1
//	día (li)          `flex gap-3 px-3 py-2.5`, `divide-y` entre días
//	columna de fecha  `w-14 pt-0.5 text-center`: overline, `text-lg`, meta  49 × 52,5
//	fila              `px-2 py-1.5` con `border` transparente              28,25
//	  punto 8 · hora `w-20 text-xs` · título `text-sm` · lugar `sytue-meta` (≥ sm)
const (
	// AnchoColumnaDia es el `w-14` de la columna de fecha.
	AnchoColumnaDia = 14 * 0.25 * tema.Rem
	// AnchoHoraAgenda es el `w-20` de la hora de una fila.
	AnchoHoraAgenda = 20 * 0.25 * tema.Rem
	// MaxLugarAgenda es el `max-w-[38%]` del lugar.
	MaxLugarAgenda = 0.38
	// CorteSM es el `sm:` de la hoja: 40rem con el rem del NAVEGADOR (16), medido sobre
	// la VENTANA, no sobre el contenido. Por debajo, el lugar no se pinta.
	CorteSM float32 = 640
)

var (
	espacio3 = tema.Espacio(3) // px-3, gap-3
	espacio1 = tema.Espacio(1) // gap-1 entre filas
)

// AltoFilaAgenda es la medida declarada de una fila, y la única cuenta.
func AltoFilaAgenda() float32 {
	return 2*(tema.Borde+espacio1p5) + max(tema.TextoSM.Linea, tema.TextoXS.Linea, LadoPunto)
}

// AltoColumnaDia es lo que ocupa la columna de fecha: `pt-0.5`, la overline, el número
// en `text-lg` y el mes en `sytue-meta`.
func AltoColumnaDia() float32 {
	return espacio0p5 + tema.TextoXS.Linea + tema.TextoLG.Linea + tema.TextoXS.Linea
}

// grupoDia son las ocurrencias de un mismo día, en orden.
type grupoDia struct {
	dia    time.Time
	dentro []Ocurrencia
}

// porDia agrupa por día natural, en orden cronológico y con el filtro del buscador. Un
// evento en curso desde ANTES de la ventana se ancla al primer día, para no encabezar
// la agenda con una fecha pasada: es lo que hace `AgendaView`.
func (p *Pantalla) porDia(consulta string) []grupoDia {
	loc := p.zona()
	inicio := inicioDelDia(p.Cursor, loc)
	q := buscar.Plegar(strings.TrimSpace(consulta))
	var grupos []grupoDia
	for _, o := range p.Eventos { // ya vienen ordenadas por inicio
		if q != "" && !strings.Contains(buscar.Plegar(o.Event.Summary+" "+o.Event.Location+" "+o.Event.Description), q) {
			continue
		}
		d := o.Day(loc)
		if d.Before(inicio) {
			d = inicio
		}
		if n := len(grupos); n > 0 && mismoDia(grupos[n-1].dia, d) {
			grupos[n-1].dentro = append(grupos[n-1].dentro, o)
			continue
		}
		grupos = append(grupos, grupoDia{dia: d, dentro: []Ocurrencia{o}})
	}
	return grupos
}

// vistaAgenda monta el buscador y, debajo, la lista. El buscador filtra EN CALIENTE lo
// que ya está cargado —la web lo dice: el índice del servicio queda para cuando haga
// falta escala—, así que al teclear se rehace solo la lista y el campo no pierde el foco.
func (p *Pantalla) vistaAgenda() fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// El `<input>` de la web va escrito a mano: `h-9 rounded-md border border-border px-3
	// text-sm`, no el `Input` de Form.tsx (`text-base`, `border-strong`). Es un
	// `ui.CampoPropio`: el `py` sale de centrar la línea de `text-sm` (15,75) en los 31,5
	// del `h-9` con su borde.
	buscador := ui.NuevaEntrada(p.t("search.events", nil)).ConPropia(ui.CampoPropio{
		MargenX: tema.Espacio(3), MargenY: (tema.Espacio(9) - 2*tema.Borde - tema.TextoSM.Linea) / 2,
		Texto: tema.TextoSM, Borde: "border",
	})
	buscador.PonerTexto(p.consultaAgenda)
	lista := container.NewStack(p.listaAgenda())
	buscador.AlCambiar(func(s string) {
		p.consultaAgenda = s
		lista.Objects = []fyne.CanvasObject{p.listaAgenda()}
		lista.Refresh()
	})
	altoBuscador := 2*espacio2 + ui.AltoCampo(ui.CampoMD) + tema.Borde
	cabecera := container.New(&conFilete{M: espacio2}, canvas.NewRectangle(tema.Color("border", v)), buscador)
	return container.NewStack(canvas.NewRectangle(tema.Color("surface", v)),
		container.New(&arriba{Alto: altoBuscador}, cabecera, lista))
}

// listaAgenda es la lista de días o, sin ninguno, el vacío con su acción.
func (p *Pantalla) listaAgenda() fyne.CanvasObject {
	grupos := p.porDia(p.consultaAgenda)
	if len(grupos) == 0 {
		options := p.Refresco
		options.Arriba = func() bool { return true }
		return ui.RefrescarAlTirar(p.agendaVacia(strings.TrimSpace(p.consultaAgenda) != ""), &options)
	}
	filas := make([]fyne.CanvasObject, 0, len(grupos))
	for i, g := range grupos {
		filas = append(filas, p.nuevoGrupoAgenda(g, i > 0))
	}
	scroll, lista := desplazable(container.New(ui.Columna{}, filas...))
	options := p.Refresco
	options.Arriba = func() bool { return scroll.Offset.Y <= 0.5 }
	return ui.RefrescarAlTirar(lista, &options)
}

// agendaVacia es el vacío de la agenda, que NO es `EmptyState`: un texto `text-sm
// text-text-subtle` y, si no hay búsqueda en curso, el botón que la llena, en columna
// centrada con `gap-3 p-8`. Un vacío que solo dice «no hay nada» deja sin salida.
func (p *Pantalla) agendaVacia(buscando bool) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	clave := "cal.noUpcoming"
	if buscando {
		clave = "agenda.noMatches"
	}
	hijos := []fyne.CanvasObject{ui.Texto(p.t(clave, nil), tema.TextoSM, ui.Normal, tema.Color("text-subtle", v))}
	if !buscando {
		b := ui.NuevoBoton(p.t("nav.createEvent", nil), p.crearHoy).Con(ui.Secundario, ui.TallaSM).ConIcono(iconos.Mas, 16)
		b.Inerte = p.SinConexion
		hijos = append(hijos, b)
	}
	return container.New(&columnaCentrada{Hueco: espacio3}, hijos...)
}

// columnaCentrada es `flex flex-col items-center justify-center gap-3`: los hijos a su
// medida, centrados en los dos ejes. Los textos se colocan por su caja de LÍNEA.
type columnaCentrada struct{ Hueco float32 }

func (c *columnaCentrada) medida(o fyne.CanvasObject) fyne.Size {
	if t, ok := o.(*canvas.Text); ok {
		return ui.Medir(t.Text, tema.TextoSM, ui.Normal)
	}
	return o.MinSize()
}

func (c *columnaCentrada) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	for i, o := range objs {
		m := c.medida(o)
		ancho = max(ancho, m.Width)
		if i > 0 {
			alto += c.Hueco
		}
		alto += m.Height
	}
	return fyne.NewSize(ancho, alto)
}

func (c *columnaCentrada) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	y := (t.Height - c.MinSize(objs).Height) / 2
	for _, o := range objs {
		m := c.medida(o)
		x := (t.Width - m.Width) / 2
		if tx, ok := o.(*canvas.Text); ok {
			ui.ColocarTexto(tx, tema.TextoSM, x, y)
		} else {
			o.Move(fyne.NewPos(x, y))
			o.Resize(m)
		}
		y += m.Height + c.Hueco
	}
}

// conFilete es `p-2 border-b`: el PRIMER hijo es el filete, que va pegado abajo y
// DENTRO de la caja; los demás llevan el mismo margen por los cuatro lados.
type conFilete struct{ M float32 }

func (c *conFilete) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var m fyne.Size
	for _, o := range objs[1:] {
		m = m.Max(o.MinSize())
	}
	return fyne.NewSize(m.Width+2*c.M, m.Height+2*c.M+tema.Borde)
}

func (c *conFilete) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, t.Height-tema.Borde))
	objs[0].Resize(fyne.NewSize(t.Width, tema.Borde))
	for _, o := range objs[1:] {
		o.Move(fyne.NewPos(c.M, c.M))
		o.Resize(fyne.NewSize(max(t.Width-2*c.M, 0), max(t.Height-2*c.M-tema.Borde, 0)))
	}
}

// grupoAgenda es un `<li>` de la agenda: la columna de fecha y las filas del día.
type grupoAgenda struct {
	widget.BaseWidget
	p     *Pantalla
	g     grupoDia
	filas []*filaAgenda
	// filete es el `divide-y`: todos los días menos el primero llevan su borde arriba.
	filete bool
}

func (p *Pantalla) nuevoGrupoAgenda(g grupoDia, filete bool) *grupoAgenda {
	w := &grupoAgenda{p: p, g: g, filete: filete}
	for _, o := range g.dentro {
		w.filas = append(w.filas, p.nuevaFilaAgenda(o))
	}
	w.ExtendBaseWidget(w)
	return w
}

// MedidaGrupoAgenda es lo que ocupa un día con `n` eventos: el filete si lo lleva, el
// `py-2.5` y lo más alto entre la columna de fecha y las filas con su `gap-1`.
func MedidaGrupoAgenda(n int, filete bool) float32 {
	filas := float32(n)*AltoFilaAgenda() + float32(max(n-1, 0))*espacio1
	alto := 2*espacio2p5 + max(AltoColumnaDia(), filas)
	if filete {
		alto += tema.Borde
	}
	return alto
}

func (w *grupoAgenda) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	l := w.p.locale()
	dias := i18n.WeekdaysShort(l)
	tinta := "text"
	if esHoy(w.g.dia) {
		tinta = "primary-text"
	}
	r := &grupoAgendaRenderer{
		w:      w,
		linea:  canvas.NewRectangle(tema.Color("border", v)),
		semana: ui.NuevoVersal(dias[(int(w.g.dia.Weekday())+6)%7], tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v)),
		numero: ui.Texto(strconv.Itoa(w.g.dia.Day()), tema.TextoLG, ui.SemiNegrita, tema.Color(tinta, v)),
		mes:    ui.Texto(i18n.FormatMonthShort(l, w.g.dia), tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
	}
	r.linea.Hidden = !w.filete
	r.objetos = append([]fyne.CanvasObject{r.linea}, r.semana.Objetos()...)
	r.objetos = append(r.objetos, r.numero, r.mes)
	for _, f := range w.filas {
		r.objetos = append(r.objetos, f)
	}
	return r
}

type grupoAgendaRenderer struct {
	w           *grupoAgenda
	linea       *canvas.Rectangle
	semana      *ui.Versal
	numero, mes *canvas.Text
	objetos     []fyne.CanvasObject
}

func (r *grupoAgendaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, MedidaGrupoAgenda(len(r.w.filas), r.w.filete))
}
func (r *grupoAgendaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *grupoAgendaRenderer) Destroy()                     {}

func (r *grupoAgendaRenderer) Layout(t fyne.Size) {
	arriba := float32(0)
	if r.w.filete {
		r.linea.Move(fyne.NewPos(0, 0))
		r.linea.Resize(fyne.NewSize(t.Width, tema.Borde))
		arriba = tema.Borde
	}
	y := arriba + espacio2p5
	x := espacio3
	centrar := func(ancho float32) float32 { return x + (AnchoColumnaDia-ancho)/2 }
	yc := y + espacio0p5
	r.semana.Colocar(centrar(r.semana.Ancho()), yc)
	yc += tema.TextoXS.Linea
	ui.ColocarTexto(r.numero, tema.TextoLG, centrar(ui.Medir(r.numero.Text, tema.TextoLG, ui.SemiNegrita).Width), yc)
	yc += tema.TextoLG.Linea
	ui.ColocarTexto(r.mes, tema.TextoXS, centrar(ui.Medir(r.mes.Text, tema.TextoXS, ui.Normal).Width), yc)

	fx := x + AnchoColumnaDia + espacio3
	ancho := max(t.Width-fx-espacio3, 0)
	for _, f := range r.w.filas {
		f.Move(fyne.NewPos(fx, y))
		f.Resize(fyne.NewSize(ancho, AltoFilaAgenda()))
		y += AltoFilaAgenda() + espacio1
	}
}

func (r *grupoAgendaRenderer) Refresh() {
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// filaAgenda es `AgendaRow`: punto del color del evento, hora (o «todo el día»), título
// y, con la ventana de `sm` para arriba, el lugar. Pulsarla abre la vista rápida.
type filaAgenda struct {
	widget.BaseWidget
	p *Pantalla
	o Ocurrencia

	encima, enfocada bool
}

var (
	_ fyne.Tappable     = (*filaAgenda)(nil)
	_ fyne.Focusable    = (*filaAgenda)(nil)
	_ desktop.Hoverable = (*filaAgenda)(nil)
)

func (p *Pantalla) nuevaFilaAgenda(o Ocurrencia) *filaAgenda {
	f := &filaAgenda{p: p, o: o}
	f.ExtendBaseWidget(f)
	return f
}

func (f *filaAgenda) Tapped(*fyne.PointEvent) {
	if f.p.AlAbrirEvento != nil {
		f.p.AlAbrirEvento(f.o, f)
	}
}
func (f *filaAgenda) FocusGained()                   { f.enfocada = true; f.Refresh() }
func (f *filaAgenda) FocusLost()                     { f.enfocada = false; f.Refresh() }
func (f *filaAgenda) TypedRune(rune)                 {}
func (f *filaAgenda) MouseIn(*desktop.MouseEvent)    { f.encima = true; f.Refresh() }
func (f *filaAgenda) MouseMoved(*desktop.MouseEvent) {}
func (f *filaAgenda) MouseOut()                      { f.encima = false; f.Refresh() }
func (f *filaAgenda) TypedKey(e *fyne.KeyEvent) {
	if ui.SubirEscape(f, e) { // Escape y «atrás» son del diálogo de encima
		return
	}
	if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter || e.Name == fyne.KeySpace {
		f.Tapped(nil)
	}
}

// AccessibilityLabel es el `title` de la web: el resumen.
func (f *filaAgenda) AccessibilityLabel() string { return f.titulo() }

// AccessibilityRole: `role="button"`.
func (f *filaAgenda) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

func (f *filaAgenda) titulo() string { return resumenDe(f.o, f.p.t("event.untitled", nil)) }

func (f *filaAgenda) hora() string {
	if f.o.Event.AllDay {
		return f.p.t("event.allDay", nil)
	}
	return i18n.FormatTimeDigits(f.p.locale(), f.o.Start.In(f.p.zona()))
}

func (f *filaAgenda) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaAgendaRenderer{
		f:      f,
		caja:   ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioMD),
		punto:  canvas.NewRectangle(f.o.Color),
		hora:   ui.Texto(f.hora(), tema.TextoXS, ui.Medio, tema.Color("text-muted", v)),
		titulo: ui.Texto(f.titulo(), tema.TextoSM, ui.Normal, tema.Color("text", v)),
		lugar:  ui.Texto(f.o.Event.Location, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
		aro:    ui.NuevoAro(tema.RadioMD, 0, v),
	}
	r.punto.CornerRadius = LadoPunto / 2
	r.objetos = append(r.caja.Objetos(), r.punto, r.hora, r.titulo, r.lugar)
	r.objetos = append(r.objetos, r.aro.Objetos()...)
	r.Refresh()
	return r
}

type filaAgendaRenderer struct {
	f                   *filaAgenda
	caja                *ui.Caja
	punto               *canvas.Rectangle
	hora, titulo, lugar *canvas.Text
	aro                 *ui.Aro
	objetos             []fyne.CanvasObject
}

func (r *filaAgendaRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, AltoFilaAgenda()) }
func (r *filaAgendaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaAgendaRenderer) Destroy()                     {}

func (r *filaAgendaRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	centro := t.Height / 2
	x := tema.Borde + espacio2
	r.punto.Move(fyne.NewPos(x, centro-LadoPunto/2))
	r.punto.Resize(fyne.NewSize(LadoPunto, LadoPunto))
	x += LadoPunto + HuecoFilaCalendario
	ui.Truncar(r.hora, r.f.hora(), tema.TextoXS, ui.Medio, AnchoHoraAgenda)
	ui.ColocarTexto(r.hora, tema.TextoXS, x, centro-tema.TextoXS.Linea/2)
	x += AnchoHoraAgenda + HuecoFilaCalendario

	der := t.Width - tema.Borde - espacio2
	util := max(t.Width-2*(tema.Borde+espacio2), 0)
	r.lugar.Hidden = r.f.o.Event.Location == "" || ventanaDe(r.f) < CorteSM
	if !r.lugar.Hidden {
		ancho := ui.Truncar(r.lugar, r.f.o.Event.Location, tema.TextoXS, ui.Normal, min(MaxLugarAgenda*util, max(der-x-HuecoFilaCalendario, 0)))
		ui.ColocarTexto(r.lugar, tema.TextoXS, der-ancho, centro-tema.TextoXS.Linea/2)
		der -= ancho + HuecoFilaCalendario
	}
	ui.Truncar(r.titulo, r.f.titulo(), tema.TextoSM, ui.Normal, max(der-x, 0))
	ui.ColocarTexto(r.titulo, tema.TextoSM, x, centro-tema.TextoSM.Linea/2)
	r.aro.Colocar(t)
}

// ventanaDe es el ancho de la ventana donde está montado el widget, que es lo que miran
// los cortes `sm:` de la hoja. Sin ventana —recién construido—, cero.
func ventanaDe(o fyne.CanvasObject) float32 {
	if c := fyne.CurrentApp().Driver().CanvasForObject(o); c != nil {
		return c.Size().Width
	}
	return 0
}

// Refresh pinta el `hover:border-border hover:bg-bg-subtle` y el foco.
func (r *filaAgendaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if r.f.encima {
		r.caja.Pintar(tema.Color("bg-subtle", v), tema.Color("border", v))
	} else {
		r.caja.Pintar(color.NRGBA{}, color.NRGBA{})
	}
	r.punto.FillColor = r.f.o.Color
	r.hora.Color = tema.Color("text-muted", v)
	r.titulo.Color = tema.Color("text", v)
	r.lugar.Color = tema.Color("text-subtle", v)
	r.aro.Ver(r.f.enfocada)
	for _, o := range []fyne.CanvasObject{r.punto, r.hora, r.titulo, r.lugar} {
		o.Refresh()
	}
}
