// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// El buscador de eventos: SearchPanel.tsx, un `Modal size="lg"` que pregunta al servicio
// (`/events/search`) con una espera de 250 ms entre teclas, desde dos letras, de veinte
// en veinte con «Cargar más». Pulsar un resultado lleva el calendario a su DÍA, donde el
// evento se ve y se abre con su vista rápida.
const (
	// PaginaBusqueda es `PAGE` y MinBusqueda `MIN`.
	PaginaBusqueda = 20
	MinBusqueda    = 2
	// EsperaBusqueda es el `DEBOUNCE_MS`.
	EsperaBusqueda = 250 * time.Millisecond
)

// PeriodosBusqueda es `SEARCH_PERIODS` de lib/search.ts, en su orden.
var PeriodosBusqueda = []string{"any", "upcoming", "past", "month", "year"}

// rangoPeriodo es `periodRange`: el tramo [desde, hasta) de un periodo, con `ahora` en
// la zona de quien mira (mes y año empiezan a SUS 00:00). Un extremo a cero no acota.
func rangoPeriodo(periodo string, ahora time.Time) (desde, hasta time.Time) {
	switch periodo {
	case "upcoming":
		return ahora, time.Time{}
	case "past":
		return time.Time{}, ahora
	case "month":
		ini := time.Date(ahora.Year(), ahora.Month(), 1, 0, 0, 0, 0, ahora.Location())
		return ini, ini.AddDate(0, 1, 0)
	case "year":
		ini := time.Date(ahora.Year(), 1, 1, 0, 0, 0, 0, ahora.Location())
		return ini, ini.AddDate(1, 0, 0)
	}
	return time.Time{}, time.Time{}
}

// propiosConID son los calendarios que se pueden filtrar: los PROPIOS con id del
// servicio. La búsqueda aísla por dueño, así que un compartido nunca daría resultados.
func (p *Pantalla) propiosConID() []api.Calendar {
	var out []api.Calendar
	for _, c := range p.Calendarios {
		if c.ID != "" {
			out = append(out, c)
		}
	}
	return out
}

// filtrosBusqueda son los filtros de calendario y periodo que van bajo el campo (`grid
// grid-cols-2 gap-2`, `<Select size="sm">`): la fila, y lo que piden en cada momento.
// Cambiarlos llama a `alCambiar`, que relanza la búsqueda. `filtro` lee los selectores:
// se llama en el hilo de la UI.
func (p *Pantalla) filtrosBusqueda(alCambiar func()) (fila fyne.CanvasObject, filtro func() api.FiltroBusqueda) {
	propios := p.propiosConID()
	opcionesCal := make([]string, 0, len(propios)+1)
	opcionesCal = append(opcionesCal, p.t("search.allCalendars", nil))
	for _, c := range propios {
		opcionesCal = append(opcionesCal, c.Title())
	}
	opcionesPer := make([]string, len(PeriodosBusqueda))
	for i, x := range PeriodosBusqueda {
		opcionesPer[i] = p.t("search.period."+x, nil)
	}
	calSel := ui.NuevoSelector(opcionesCal, func(int) { alCambiar() }).Con(ui.CampoSM)
	calSel.Rotulo = p.t("event.calendar", nil)
	perSel := ui.NuevoSelector(opcionesPer, func(int) { alCambiar() }).Con(ui.CampoSM)
	perSel.Rotulo = p.t("search.period", nil)
	filtro = func() api.FiltroBusqueda {
		var f api.FiltroBusqueda
		if i := calSel.Elegida - 1; i >= 0 && i < len(propios) {
			f.Calendario = propios[i].ID
		}
		if i := perSel.Elegida; i >= 0 && i < len(PeriodosBusqueda) {
			f.Desde, f.Hasta = rangoPeriodo(PeriodosBusqueda[i], time.Now().In(p.zona()))
		}
		return f
	}
	return container.New(&dosColumnas{Hueco: espacio2}, calSel, perSel), filtro
}

// abrirBusqueda abre el buscador. `alAbrir` lleva al día de un resultado.
func (a *App) abrirBusqueda() {
	lienzo := a.lienzo()
	if lienzo == nil {
		return
	}
	p := a.P
	campo := ui.NuevaEntrada(p.t("search.placeholder", nil))
	campo.Icono = iconos.Buscar
	var buscar func(string)
	filtros, filtro := p.filtrosBusqueda(func() { buscar(strings.TrimSpace(campo.Texto())) })
	zona := container.New(&minAlto{Alto: tema.Espacio(32)}) // `min-h-[8rem]`
	var d *ui.Dialogo
	var (
		resultados []api.Event
		cursor     string
		// consulta son los filtros con los que se pidió la primera página, con su
		// tramo ya fijado: «Cargar más» pide la siguiente de ESA consulta. Recalcular
		// «Próximos» con la hora de ahora movería `from` bajo el mismo cursor.
		consulta     api.FiltroBusqueda
		secuencia    int
		cargando     bool
		fallo        bool
		temporizador *time.Timer
		pintar       func()
	)
	pintar = func() {
		zona.Objects = []fyne.CanvasObject{p.cuerpoBusqueda(strings.TrimSpace(campo.Texto()), resultados, cursor, cargando, fallo, func(ev api.Event) {
			d.Cerrar()
			a.irAlDia(inicioResultado(ev, p.zona()))
		}, func() {
			// «Cargar más»: misma búsqueda; si el término cambia, se descarta.
			sec := secuencia
			// El cursor se copia en el hilo de la UI, que es quien lo escribe.
			q, f, desde := strings.TrimSpace(campo.Texto()), consulta, cursor
			a.lanzar(func() {
				ctx, parar := context.WithTimeout(context.Background(), plazo)
				defer parar()
				b, err := a.Cal.SearchEventsFiltrada(ctx, q, f, PaginaBusqueda, desde)
				a.enUI(func() {
					if sec != secuencia {
						return
					}
					if err != nil {
						fallo = true
					} else {
						resultados, cursor = append(resultados, b.Events...), b.Siguiente
					}
					pintar()
				})
			})
		}, func() { campo.PonerTexto(campo.Texto() + " ") })}
		zona.Refresh()
		if arriba := lienzo.Overlays().Top(); arriba != nil {
			arriba.Refresh()
		}
	}
	buscar = func(q string) {
		secuencia++
		sec := secuencia
		if temporizador != nil {
			temporizador.Stop()
		}
		if len([]rune(q)) < MinBusqueda || a.Cal == nil {
			resultados, cursor, fallo, cargando = nil, "", false, false
			pintar()
			return
		}
		cargando, fallo = true, false
		pintar()
		f := filtro() // en el hilo de la UI: los selectores son suyos
		consulta = f
		temporizador = time.AfterFunc(EsperaBusqueda, func() {
			a.lanzar(func() {
				ctx, parar := context.WithTimeout(context.Background(), plazo)
				defer parar()
				b, err := a.Cal.SearchEventsFiltrada(ctx, q, f, PaginaBusqueda, "")
				a.enUI(func() {
					if sec != secuencia {
						return // llegó una búsqueda más nueva
					}
					cargando = false
					fallo = err != nil
					resultados, cursor = b.Events, b.Siguiente
					pintar()
				})
			})
		})
	}
	campo.AlCambiar(func(s string) { buscar(strings.TrimSpace(s)) })
	pintar()
	d = ui.NuevoDialogo(p.t("search.title", nil), container.New(ui.Columna{Hueco: espacio3}, campo, filtros, zona)).Con(ui.DialogoLG, nil)
	d.RotuloCerrar = p.t("common.close", nil)
	d.Foco = campo
	d.AlCerrar = func() {
		secuencia++ // una respuesta que llegue ya no pinta nada
		if temporizador != nil {
			temporizador.Stop()
		}
		d.Cerrar()
	}
	d.Mostrar(lienzo)
}

// irAlDia es `onOpenResult`: el calendario va al día y a la vista de día.
func (a *App) irAlDia(dia time.Time) {
	a.cambiar(func() {
		a.P.Cursor = dia
		a.P.Vista = VistaDia
	})
}

// inicioResultado es `startDate`: un día completo por su FECHA, sin que la zona lo mueva.
func inicioResultado(ev api.Event, loc *time.Location) time.Time {
	if ev.AllDay {
		u := ev.DTStart.UTC()
		return time.Date(u.Year(), u.Month(), u.Day(), 12, 0, 0, 0, loc)
	}
	return ev.DTStart.In(loc)
}

// cuandoResultado es `whenLabel` de lib/search.ts.
func (p *Pantalla) cuandoResultado(ev api.Event) string {
	if ev.AllDay {
		fin := time.Time{}
		if !ev.DTEnd.IsZero() {
			fin = inicioResultado(api.Event{AllDay: true, DTStart: ev.DTEnd}, p.zona())
		}
		return p.Cuando(inicioResultado(ev, p.zona()), fin, true)
	}
	fin := time.Time{}
	if !ev.DTEnd.IsZero() {
		fin = ev.DTEnd.In(p.zona())
	}
	return p.Cuando(ev.DTStart.In(p.zona()), fin, false)
}

// calendarioDe busca un calendario por su id entre los cargados.
func (p *Pantalla) calendarioDe(id string) (api.Calendar, bool) {
	for _, c := range append(append([]api.Calendar(nil), p.Calendarios...), p.Compartidos...) {
		if c.ID == id {
			return c, true
		}
	}
	return api.Calendar{}, false
}

// cuerpoBusqueda es la región viva bajo el campo: la pista de dos letras, la rueda, el
// error con «Reintentar», el «sin resultados» o la lista.
func (p *Pantalla) cuerpoBusqueda(q string, res []api.Event, cursor string, cargando, fallo bool, abrir func(api.Event), mas, reintentar func()) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	centrada := func(s, token string, y float32) fyne.CanvasObject {
		par := ui.NuevoParrafo(s, tema.TextoSM, ui.Normal, token, v)
		return container.New(&centrado{Y: y}, par)
	}
	switch {
	case len([]rune(q)) < MinBusqueda:
		return centrada(p.t("search.minChars", i18n.Vars{"count": MinBusqueda}), "text-subtle", tema.Espacio(8))
	case cargando:
		fila := container.New(ui.Fila{Hueco: espacio2}, ui.NuevaRueda(ui.LadoRueda, "text-muted", v),
			alto(ui.Texto(p.t("search.searching", nil), tema.TextoSM, ui.Normal, tema.Color("text-muted", v)), ui.LadoRueda))
		return container.New(&centrado{Y: tema.Espacio(10)}, fila)
	case fallo:
		return container.New(&centrado{Y: tema.Espacio(8)}, container.New(ui.Columna{Hueco: espacio2, Encaja: true},
			ui.NuevoParrafo(p.t("search.error", nil), tema.TextoSM, ui.Normal, "text-muted", v),
			ui.NuevoBoton(p.t("common.retry", nil), reintentar).Con(ui.Secundario, ui.TallaSM)))
	case len(res) == 0:
		return centrada(p.t("search.noResults", i18n.Vars{"term": q}), "text-subtle", tema.Espacio(10))
	}
	filas := make([]fyne.CanvasObject, 0, len(res)+1)
	for _, ev := range res {
		filas = append(filas, p.nuevoResultado(ev, func() { abrir(ev) }))
	}
	if cursor != "" {
		b := ui.NuevoBoton(p.t("search.loadMore", nil), mas).Con(ui.Secundario, ui.TallaSM)
		filas = append(filas, container.New(&sangria{Izq: espacio2p5, Arr: espacio1p5}, encajar(b)))
	}
	return container.New(ui.Columna{Hueco: espacio0p5}, filas...)
}

// resultado es un `<button>` de la lista:
//
//	hover:bg-bg-subtle flex w-full items-start gap-2.5 rounded-md px-2.5 py-2
//	punto `mt-1 h-2.5 w-2.5` del color del calendario · título `text-sm font-semibold
//	truncate` con la insignia «Serie» · cuándo `text-xs text-muted mt-0.5` · fragmento
//	y calendario `sytue-meta mt-0.5 truncate`
type resultado struct {
	widget.BaseWidget
	contenido fyne.CanvasObject
	fondo     *canvas.Rectangle
	alPulsar  func()
	nombre    string
	encima    bool
}

var (
	_ fyne.Tappable     = (*resultado)(nil)
	_ desktop.Hoverable = (*resultado)(nil)
)

func (p *Pantalla) nuevoResultado(ev api.Event, alPulsar func()) *resultado {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cal, hay := p.calendarioDe(ev.CalendarID)
	tinta := colorDe("#2787d8") // `var(--sytue-primary-500)` si no hay calendario
	if hay {
		tinta = colorDe(cal.Color)
	}
	titulo := ev.Summary
	if titulo == "" {
		titulo = p.t("event.untitled", nil)
	}
	ancho := AnchoUtilLG - 2*espacio2p5 - tema.Espacio(2.5) - espacio2p5
	cabeza := []fyne.CanvasObject{ui.NuevoParrafo(titulo, tema.TextoSM, ui.SemiNegrita, "text", v).A(ancho).Recortado(1)}
	if ev.Recurring() {
		cabeza = append(cabeza, ui.NuevaInsignia(p.t("search.series", nil), ui.InsigniaNeutra))
	}
	lineas := []fyne.CanvasObject{
		container.New(&filaTitulo{}, cabeza...),
		container.New(&margenArriba{Arr: espacio0p5}, textoXS(p.cuandoResultado(ev), "text-muted", ancho, 1)),
	}
	if frag := strings.TrimSpace(firstNonEmpty(ev.Location, ev.Description)); frag != "" {
		lineas = append(lineas, container.New(&margenArriba{Arr: espacio0p5}, textoXS(frag, "text-subtle", ancho, 1)))
	}
	if hay {
		lineas = append(lineas, container.New(&margenArriba{Arr: espacio0p5}, textoXS(cal.Title(), "text-subtle", ancho, 1)))
	}
	r := &resultado{
		contenido: container.New(&filaResultado{}, puntoDe(tinta, ladoPuntoRapida), container.New(ui.Columna{}, lineas...)),
		fondo:     canvas.NewRectangle(color.NRGBA{}),
		alPulsar:  alPulsar,
		nombre:    titulo,
	}
	r.fondo.CornerRadius = tema.RadioMD
	r.ExtendBaseWidget(r)
	return r
}

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if strings.TrimSpace(x) != "" {
			return x
		}
	}
	return ""
}

// AnchoUtilLG es lo que queda dentro del diálogo `lg`: 720 menos bordes y `px-6`.
var AnchoUtilLG = 720 - 2*tema.Borde - 2*tema.Espacio(6)

func (r *resultado) Tapped(*fyne.PointEvent) {
	if r.alPulsar != nil {
		r.alPulsar()
	}
}
func (r *resultado) MouseIn(*desktop.MouseEvent)    { r.encima = true; r.Refresh() }
func (r *resultado) MouseMoved(*desktop.MouseEvent) {}
func (r *resultado) MouseOut()                      { r.encima = false; r.Refresh() }
func (r *resultado) Cursor() desktop.Cursor         { return desktop.PointerCursor }
func (r *resultado) AccessibilityLabel() string     { return r.nombre }

func (r *resultado) MinSize() fyne.Size {
	return r.contenido.MinSize().AddWidthHeight(2*espacio2p5, 2*espacio2)
}

func (r *resultado) CreateRenderer() fyne.WidgetRenderer {
	return &resultadoRenderer{r: r}
}

type resultadoRenderer struct{ r *resultado }

func (x *resultadoRenderer) MinSize() fyne.Size { return x.r.MinSize() }
func (x *resultadoRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{x.r.fondo, x.r.contenido}
}
func (x *resultadoRenderer) Destroy() {}
func (x *resultadoRenderer) Layout(t fyne.Size) {
	x.r.fondo.Resize(t)
	x.r.contenido.Move(fyne.NewPos(espacio2p5, espacio2))
	x.r.contenido.Resize(fyne.NewSize(max(t.Width-2*espacio2p5, 0), max(t.Height-2*espacio2, 0)))
}

func (x *resultadoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	x.r.fondo.FillColor = color.NRGBA{}
	if x.r.encima {
		x.r.fondo.FillColor = tema.Color("bg-subtle", v)
	}
	x.r.fondo.Refresh()
}

// filaResultado es `flex items-start gap-2.5` con el punto `mt-1`.
type filaResultado struct{}

func (filaResultado) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[1].MinSize()
	return fyne.NewSize(ladoPuntoRapida+espacio2p5+m.Width, max(m.Height, tema.Espacio(1)+ladoPuntoRapida))
}

func (filaResultado) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, tema.Espacio(1)))
	objs[0].Resize(fyne.NewSize(ladoPuntoRapida, ladoPuntoRapida))
	x := ladoPuntoRapida + espacio2p5
	objs[1].Move(fyne.NewPos(x, 0))
	objs[1].Resize(fyne.NewSize(max(t.Width-x, 0), objs[1].MinSize().Height))
}

// filaTitulo es `flex items-center gap-2`: el título se lleva lo que sobra y la
// insignia va a su medida al final.
type filaTitulo struct{}

func (filaTitulo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return ui.Fila{Hueco: espacio2}.MinSize(objs)
}

func (filaTitulo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	alto := float32(0)
	for _, o := range objs {
		alto = max(alto, o.MinSize().Height)
	}
	der := t.Width
	if len(objs) > 1 {
		m := objs[1].MinSize()
		der -= m.Width
		objs[1].Move(fyne.NewPos(der, (alto-m.Height)/2))
		objs[1].Resize(m)
		der -= espacio2
	}
	m := objs[0].MinSize()
	objs[0].Move(fyne.NewPos(0, (alto-m.Height)/2))
	objs[0].Resize(fyne.NewSize(max(der, 0), m.Height))
}

// minAlto es `min-h-*`: el hijo con, como poco, ese alto.
type minAlto struct{ Alto float32 }

func (m *minAlto) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) == 0 {
		return fyne.NewSize(0, m.Alto)
	}
	s := objs[0].MinSize()
	return fyne.NewSize(s.Width, max(s.Height, m.Alto))
}

func (m *minAlto) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
}
