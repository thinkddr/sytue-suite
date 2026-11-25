// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"
	"image/color"
	"slices"
	"strconv"
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
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// El centro de recordatorios: ReminderCenter.tsx. Una campana en la cabecera con su
// contador y, al pulsarla, un panel con los recordatorios vencidos —cada uno con su hora,
// «descartar» y el salto a su día—. La App sondea cada minuto, con espera doble ante un
// error hasta diez minutos, y anuncia con un aviso cada recordatorio NUEVO una sola vez.
//
//	campana  border-chrome-border text-header-fg h-8 w-8 rounded-md border
//	         hover:bg-chrome-hover · `<Badge count>` en `-top-1 -right-1`
//	panel    border-border bg-surface-raised absolute right-0 mt-2 w-80 rounded-lg
//	         border shadow-md · cabecera `border-b px-3 py-2` · cuerpo `max-h-96`
//	fila     hover:bg-bg-subtle flex items-start gap-2 px-3 py-2.5, título `text-sm
//	         font-medium truncate` y `sytue-meta` con «Hoy · 09:30»
//
// La campana va en `border-chrome-border` y `text-header-fg`, los tokens de la banda de
// cromo, sobre la cabecera de SUPERFICIE: en claro, blanco sobre blanco. Es lo que pinta
// producción (medido: el cuarto botón de 28 × 28 de la cabecera, con su borde al 22 % de
// blanco), y la paridad lo copia; en oscuro sí se ve.
//
// ponytail: sin la Notification nativa del navegador ni su «Activar avisos». En nativo
// los avisos del sistema los lleva el agente de las funciones del sistema operativo.
const (
	// IntervaloRecordatorios es `BASE_INTERVAL` y TopeRecordatorios `MAX_INTERVAL`.
	IntervaloRecordatorios = time.Minute
	TopeRecordatorios      = 10 * time.Minute
	// preferenciaAnunciados es la gemela del `localStorage` de la web: los ids ya
	// anunciados, para no repetir el aviso al volver a abrir la app.
	preferenciaAnunciados = "calendar.recordatorios.anunciados"
	topeAnunciados        = 300
)

var (
	// AnchoPanelRecordatorios es el `w-80`.
	AnchoPanelRecordatorios = tema.Espacio(80)
	// AltoMaxRecordatorios es el `max-h-96`.
	AltoMaxRecordatorios = tema.Espacio(96)
)

// sondearRecordatorios es el `poll` de la web; se reprograma solo.
func (a *App) sondearRecordatorios(vida context.Context) {
	if a.Cal == nil {
		return
	}
	espera := IntervaloRecordatorios
	for {
		ctx, parar := context.WithTimeout(vida, plazo)
		lista, err := a.Cal.Reminders(ctx)
		parar()
		a.enUI(func() { a.llegaronRecordatorios(lista, err) })
		if err != nil {
			espera = min(2*espera, TopeRecordatorios)
		} else {
			espera = IntervaloRecordatorios
		}
		t := time.NewTimer(espera)
		select {
		case <-vida.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}

// llegaronRecordatorios aplica una ronda del sondeo: quita los descartados, anuncia los
// nuevos y repinta la campana (y el panel, si está abierto).
//
// Una ronda que no cambia nada —lo normal: el sondeo es de cada minuto— no repinta. Antes
// rehacía la pantalla entera cada minuto aunque la campana dijera lo mismo.
func (a *App) llegaronRecordatorios(lista []api.Reminder, err error) {
	cambia := a.recCargando || a.recError != (err != nil)
	a.recCargando = false
	a.recError = err != nil
	if err == nil {
		frescos := lista[:0:0]
		for _, r := range lista {
			if !a.recDescartados[r.ID] {
				frescos = append(frescos, r)
			}
		}
		cambia = cambia || !slices.Equal(frescos, a.P.Recordatorios)
		a.P.Recordatorios = frescos
		a.anunciar(frescos)
	}
	// A medianoche, «hoy» cambia de día aunque la campana diga lo mismo.
	if !cambia && inicioDelDia(a.P.ahora(), a.P.zona()).Equal(a.diaPintado) {
		return
	}
	a.pintarPanelRecordatorios()
	a.pintar()
}

// anunciar saca un aviso por cada recordatorio que aún no se anunció, con «Ver».
func (a *App) anunciar(lista []api.Reminder) {
	prefs := fyne.CurrentApp().Preferences()
	ya := map[string]bool{}
	anunciados := prefs.StringList(preferenciaAnunciados)
	for _, id := range anunciados {
		ya[id] = true
	}
	for _, r := range lista {
		if ya[r.ID] {
			continue
		}
		ya[r.ID] = true
		anunciados = append(anunciados, r.ID)
		titulo := r.Summary
		if titulo == "" {
			titulo = a.P.t("reminders.fallbackTitle", nil)
		}
		cuerpo := a.P.t("reminders.body", nil)
		if !r.DTStart.IsZero() {
			cuerpo = a.P.t("reminders.bodyAt", i18n.Vars{"time": i18n.FormatTimeDigits(a.P.locale(), r.DTStart.In(a.P.zona()))})
		}
		aviso := ui.NuevoAviso(ui.AvisoInfo, titulo, cuerpo)
		aviso.RotuloAccion, aviso.AlPulsarAccion = a.P.t("reminders.view", nil), func() { a.saltarA(r) }
		a.avisos.Notificar(aviso)
	}
	if len(anunciados) > topeAnunciados {
		anunciados = anunciados[len(anunciados)-topeAnunciados:]
	}
	prefs.SetStringList(preferenciaAnunciados, anunciados)
}

// saltarA es `deepLink`: el calendario al día del evento, en la vista de día.
func (a *App) saltarA(r api.Reminder) {
	if a.recPanel != nil {
		a.recPanel.Cerrar()
		a.recPanel = nil
	}
	// Desde el cajón del teléfono, el enlace es un destino: cierra el cajón como
	// cualquier otro (en la web, el enlace no lleva `data-mantiene-nav`).
	a.P.cajonAbierto = false
	if r.DTStart.IsZero() {
		a.pintar()
		return
	}
	a.irAlDia(r.DTStart.In(a.P.zona()))
}

// descartar es `dismiss`: fuera de la lista YA, y el servicio lo apunta.
func (a *App) descartar(r api.Reminder) {
	a.recDescartados[r.ID] = true
	resto := a.P.Recordatorios[:0:0]
	for _, x := range a.P.Recordatorios {
		if x.ID != r.ID {
			resto = append(resto, x)
		}
	}
	a.P.Recordatorios = resto
	a.pintarPanelRecordatorios()
	a.pintar()
	a.escribir(func(ctx context.Context) error { return a.Cal.DismissReminder(ctx, r.ID) }, func(err error) {
		if err != nil {
			a.avisar(ui.AvisoPeligro, "toast.reminderDismissFailed")
		}
	})
}

// PosponerMinutos es lo que se ofrece al posponer: `POSPONER_MINUTOS` de la web.
var PosponerMinutos = []int{5, 10, 30, 60}

// alternarPosponer abre o cierra las opciones de «Posponer» de un recordatorio: una fila
// a la vez, como en la web.
func (a *App) alternarPosponer(r api.Reminder) {
	if a.P.recPosponiendo == r.ID {
		a.P.recPosponiendo = ""
	} else {
		a.P.recPosponiendo = r.ID
	}
	a.pintarPanelRecordatorios()
	a.pintar()
}

// posponer es `snooze`: fuera de la lista ya y de los anunciados (al volver a vencer se
// avisa otra vez, con el mismo id); el servicio lo rearma `minutos` después. Si falla,
// se vuelve a sondear y reaparece.
func (a *App) posponer(r api.Reminder, minutos int) {
	a.P.recPosponiendo = ""
	resto := a.P.Recordatorios[:0:0]
	for _, x := range a.P.Recordatorios {
		if x.ID != r.ID {
			resto = append(resto, x)
		}
	}
	a.P.Recordatorios = resto
	prefs := fyne.CurrentApp().Preferences()
	prefs.SetStringList(preferenciaAnunciados, slices.DeleteFunc(prefs.StringList(preferenciaAnunciados), func(id string) bool { return id == r.ID }))
	a.pintarPanelRecordatorios()
	a.pintar()
	var vuelve time.Time
	a.escribir(func(ctx context.Context) error {
		var err error
		vuelve, err = a.Cal.SnoozeReminder(ctx, r.ID, minutos)
		return err
	}, func(err error) {
		if err != nil {
			a.avisar(ui.AvisoPeligro, "toast.reminderSnoozeFailed")
			// Una ronda suelta del sondeo (no otro bucle): lo devuelve a la lista.
			a.lanzar(func(vida context.Context) {
				ctx, parar := context.WithTimeout(vida, plazo)
				defer parar()
				lista, err := a.Cal.Reminders(ctx)
				a.enUI(func() { a.llegaronRecordatorios(lista, err) })
			})
			return
		}
		titulo := a.P.t("reminders.snoozed", i18n.Vars{"time": i18n.FormatTimeDigits(a.P.locale(), vuelve.In(a.P.zona()))})
		a.avisos.Notificar(ui.NuevoAviso(ui.AvisoExito, titulo, ""))
	})
}

// abrirRecordatorios despliega el panel colgando de la campana, pegado a su borde
// derecho y acotado a la ventana.
func (a *App) abrirRecordatorios(campana fyne.CanvasObject) {
	// En el mes del teléfono no hay campana: es la rama del cajón, que se despliega en el
	// sitio sin cerrarlo.
	if a.P.compacta() {
		a.P.recEnCajon = !a.P.recEnCajon
		a.pintar()
		return
	}
	lienzo := a.lienzo()
	if lienzo == nil {
		return
	}
	if a.recPanel != nil {
		a.recPanel.Cerrar()
		a.recPanel = nil
		return
	}
	a.recContenido = container.NewStack()
	a.recPanel = &ui.Emergente{} // para que el primer pintado ya rellene el contenido
	a.pintarPanelRecordatorios()
	e := ui.MostrarEmergenteEn(lienzo, container.New(&anchoFijo{Ancho: AnchoPanelRecordatorios - 2*tema.Borde}, a.recContenido),
		0, func(ventana, tam fyne.Size) (fyne.Position, fyne.Size) {
			tam.Width = min(tam.Width, ventana.Width-2*MargenVistaRapida) // `max-w-[calc(100vw-16px)]`
			pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(campana)
			x := pos.X + campana.Size().Width - tam.Width // `right-0`
			x = max(MargenVistaRapida, min(x, ventana.Width-MargenVistaRapida-tam.Width))
			return fyne.NewPos(x, pos.Y+campana.Size().Height+espacio2), tam
		})
	e.AlRecogerse = func() {
		if a.recPanel == e {
			a.recPanel = nil
		}
	}
	a.recPanel = e
}

// pintarPanelRecordatorios rehace lo de dentro del panel si está abierto.
func (a *App) pintarPanelRecordatorios() {
	if a.recPanel == nil || a.recContenido == nil {
		return
	}
	a.recContenido.Objects = []fyne.CanvasObject{a.P.cuerpoRecordatorios(a.recCargando, a.recError, func() {
		a.recPanel.Cerrar()
		a.recPanel = nil
	}, a.saltarA, a.descartar)}
	a.recContenido.Refresh()
	if c := a.lienzo(); c != nil {
		if arriba := c.Overlays().Top(); arriba != nil {
			arriba.Refresh()
		}
	}
}

// cuerpoRecordatorios es el panel por dentro: cabecera, y la carga, el error, el vacío
// o la lista.
func (p *Pantalla) cuerpoRecordatorios(cargando, fallo bool, cerrar func(), saltar, descartar func(api.Reminder)) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	aspa := botonAspa(p.t("common.close", nil), cerrar)
	cabecera := container.New(&cabeceraPanel{}, canvas.NewRectangle(tema.Color("border", v)),
		ui.Texto(p.t("reminders.title", nil), tema.TextoSM, ui.SemiNegrita, tema.Color("text", v)), aspa)
	return container.New(ui.Columna{}, cabecera, p.listaRecordatorios(cargando, fallo, saltar, descartar))
}

// listaRecordatorios es lo de dentro del panel: la carga, el error, el vacío o la lista.
// La comparten el panel de la campana y la rama del cajón del teléfono.
func (p *Pantalla) listaRecordatorios(cargando, fallo bool, saltar, descartar func(api.Reminder)) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var cuerpo fyne.CanvasObject
	lista := p.Recordatorios
	switch {
	case cargando:
		cuerpo = container.New(&centrado{Y: tema.Espacio(8)}, ui.NuevaRueda(ui.LadoRueda, "text-muted", v))
	case fallo && len(lista) == 0:
		cuerpo = container.New(&centrado{Y: tema.Espacio(8)}, ui.NuevoParrafo(p.t("reminders.loadError", nil), tema.TextoSM, ui.Normal, "text-muted", v))
	case len(lista) == 0:
		campana := iconos.Trazo(iconos.Campana, 24, "text-subtle", v).Objeto()
		cuerpo = container.New(&centrado{Y: tema.Espacio(8)}, container.New(ui.Columna{Hueco: tema.Espacio(1), Encaja: true},
			campana, ui.NuevoParrafo(p.t("reminders.empty", nil), tema.TextoSM, ui.Normal, "text-muted", v)))
	default:
		filas := make([]fyne.CanvasObject, 0, len(lista))
		for i, r := range lista {
			f := p.nuevaFilaRecordatorio(r, i > 0, func() { saltar(r) }, func() { descartar(r) })
			if p.alternarRec != nil && p.posponerRec != nil {
				f.ponerPosponer(p, r, p.recPosponiendo == r.ID)
			}
			filas = append(filas, f)
		}
		col := container.New(ui.Columna{}, filas...)
		_, d := desplazable(col)
		cuerpo = container.New(&altoMaximo{Alto: AltoMaxRecordatorios, Hijo: col}, d)
	}
	return cuerpo
}

// botonAspa es el `<Button variant="ghost" size="sm" iconOnly>` con el aspa de 16.
func botonAspa(rotulo string, alPulsar func()) *ui.Boton {
	b := ui.NuevoBoton("", alPulsar).Con(ui.Fantasma, ui.TallaSM)
	b.SoloIcono, b.Icono, b.TamIcono, b.Rotulo = true, iconos.Cerrar, 16, rotulo
	return b
}

// cabeceraPanel es `flex items-center justify-between border-b px-3 py-2`: el filete,
// el título y el aspa.
type cabeceraPanel struct{}

func (cabeceraPanel) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, max(objs[2].MinSize().Height, tema.TextoSM.Linea)+2*espacio2+tema.Borde)
}

func (cabeceraPanel) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, t.Height-tema.Borde))
	objs[0].Resize(fyne.NewSize(t.Width, tema.Borde))
	fila := t.Height - tema.Borde
	if tx, ok := objs[1].(*canvas.Text); ok {
		ui.ColocarTexto(tx, tema.TextoSM, espacio3, (fila-tema.TextoSM.Linea)/2)
	}
	m := objs[2].MinSize()
	objs[2].Move(fyne.NewPos(t.Width-espacio3-m.Width, (fila-m.Height)/2))
	objs[2].Resize(m)
}

// altoMaximo es `max-h-*` con desplazamiento: el alto de su contenido hasta el tope.
type altoMaximo struct {
	Alto float32
	Hijo fyne.CanvasObject
}

func (m *altoMaximo) MinSize([]fyne.CanvasObject) fyne.Size {
	s := m.Hijo.MinSize()
	return fyne.NewSize(s.Width, min(s.Height, m.Alto))
}

func (m *altoMaximo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
}

// cuandoRecordatorio es `reminderWhen`: «Hoy · 09:30» o «3 de julio · 09:30».
func (p *Pantalla) cuandoRecordatorio(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	t = t.In(p.zona())
	dia := i18n.FormatDayMonthLong(p.locale(), t)
	if esHoy(t) {
		dia = p.t("reminders.today", nil)
	}
	return p.t("reminders.when", i18n.Vars{"day": dia, "time": i18n.FormatTimeDigits(p.locale(), t)})
}

// filaRecordatorio es un `<li>` de la lista.
type filaRecordatorio struct {
	widget.BaseWidget
	titulo, cuando *canvas.Text
	aspa           *ui.Boton
	// reloj abre «Posponer»; opciones son sus botones, debajo del título (nil cerradas).
	reloj       *ui.Boton
	opciones    fyne.CanvasObject
	filete      *canvas.Rectangle
	fondo       *canvas.Rectangle
	tituloTexto string
	alPulsar    func()
	encima      bool
}

var (
	_ fyne.Tappable     = (*filaRecordatorio)(nil)
	_ desktop.Hoverable = (*filaRecordatorio)(nil)
)

func (p *Pantalla) nuevaFilaRecordatorio(r api.Reminder, filete bool, saltar, descartar func()) *filaRecordatorio {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	titulo := r.Summary
	if titulo == "" {
		titulo = p.t("event.untitled", nil)
	}
	nombre := r.Summary
	if nombre == "" {
		nombre = p.t("reminders.eventFallback", nil)
	}
	f := &filaRecordatorio{
		titulo:      ui.Texto(titulo, tema.TextoSM, ui.Medio, tema.Color("text", v)),
		cuando:      ui.Texto(p.cuandoRecordatorio(r.DTStart), tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
		aspa:        botonAspa(p.t("reminders.dismiss", i18n.Vars{"title": nombre}), descartar),
		filete:      canvas.NewRectangle(tema.Color("border", v)),
		fondo:       canvas.NewRectangle(color.NRGBA{}),
		tituloTexto: titulo,
		alPulsar:    saltar,
	}
	f.filete.Hidden = !filete
	f.ExtendBaseWidget(f)
	return f
}

// ponerPosponer añade el reloj de «Posponer» y, abiertas, sus opciones: «5 min»,
// «10 min», «30 min» y «1 h», con las etiquetas de duración de los ajustes.
func (f *filaRecordatorio) ponerPosponer(p *Pantalla, r api.Reminder, abiertas bool) {
	nombre := r.Summary
	if nombre == "" {
		nombre = p.t("reminders.eventFallback", nil)
	}
	f.reloj = ui.NuevoBoton("", func() { p.alternarRec(r) }).Con(ui.Fantasma, ui.TallaSM)
	f.reloj.SoloIcono, f.reloj.Icono, f.reloj.TamIcono = true, iconos.Reloj, 16
	f.reloj.Rotulo = p.t("reminders.snooze", i18n.Vars{"title": nombre})
	if !abiertas {
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	hijos := []fyne.CanvasObject{ui.Texto(p.t("reminders.snoozeFor", nil), tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))}
	for _, m := range PosponerMinutos {
		hijos = append(hijos, ui.NuevoBoton(p.etiquetaDuracion(m), func() { p.posponerRec(r, m) }).Con(ui.Secundario, ui.TallaSM))
	}
	f.opciones = container.New(ui.Fila{Hueco: tema.Espacio(1.5)}, hijos...)
}

func (f *filaRecordatorio) Tapped(*fyne.PointEvent) {
	if f.alPulsar != nil {
		f.alPulsar()
	}
}
func (f *filaRecordatorio) MouseIn(*desktop.MouseEvent)    { f.encima = true; f.Refresh() }
func (f *filaRecordatorio) MouseMoved(*desktop.MouseEvent) {}
func (f *filaRecordatorio) MouseOut()                      { f.encima = false; f.Refresh() }
func (f *filaRecordatorio) AccessibilityLabel() string     { return f.tituloTexto }

func (f *filaRecordatorio) MinSize() fyne.Size {
	alto := 2*espacio2p5 + max(tema.TextoSM.Linea+tema.TextoXS.Linea, espacio0p5+f.aspa.MinSize().Height)
	if f.opciones != nil {
		alto += espacio2 + f.opciones.MinSize().Height
	}
	return fyne.NewSize(0, alto)
}

func (f *filaRecordatorio) CreateRenderer() fyne.WidgetRenderer {
	r := &filaRecRenderer{f: f}
	r.Refresh()
	return r
}

type filaRecRenderer struct{ f *filaRecordatorio }

func (r *filaRecRenderer) MinSize() fyne.Size { return r.f.MinSize() }
func (r *filaRecRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.f.fondo, r.f.filete, r.f.titulo, r.f.cuando, r.f.aspa}
	if r.f.reloj != nil {
		objs = append(objs, r.f.reloj)
	}
	if r.f.opciones != nil {
		objs = append(objs, r.f.opciones)
	}
	return objs
}
func (r *filaRecRenderer) Destroy() {}

func (r *filaRecRenderer) Layout(t fyne.Size) {
	f := r.f
	f.fondo.Resize(t)
	f.filete.Resize(fyne.NewSize(t.Width, tema.Borde))
	a := f.aspa.MinSize()
	f.aspa.Move(fyne.NewPos(t.Width-espacio3-a.Width, espacio2p5+espacio0p5))
	f.aspa.Resize(a)
	cabe := max(t.Width-2*espacio3-a.Width-espacio2, 0)
	if f.reloj != nil {
		// El reloj a la izquierda del aspa, con el mismo `gap-2`.
		rl := f.reloj.MinSize()
		f.reloj.Move(fyne.NewPos(t.Width-espacio3-a.Width-espacio2-rl.Width, espacio2p5+espacio0p5))
		f.reloj.Resize(rl)
		cabe = max(cabe-rl.Width-espacio2, 0)
	}
	if f.opciones != nil {
		y := espacio2p5 + max(tema.TextoSM.Linea+tema.TextoXS.Linea, espacio0p5+a.Height) + espacio2
		f.opciones.Move(fyne.NewPos(espacio3, y))
		f.opciones.Resize(fyne.NewSize(max(t.Width-2*espacio3, 0), f.opciones.MinSize().Height))
	}
	ui.Truncar(f.titulo, f.tituloTexto, tema.TextoSM, ui.Medio, cabe)
	ui.ColocarTexto(f.titulo, tema.TextoSM, espacio3, espacio2p5)
	ui.ColocarTexto(f.cuando, tema.TextoXS, espacio3, espacio2p5+tema.TextoSM.Linea)
}

func (r *filaRecRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.f.fondo.FillColor = color.NRGBA{}
	if r.f.encima {
		r.f.fondo.FillColor = tema.Color("bg-subtle", v)
	}
	r.f.fondo.Refresh()
}

// ---- La campana de la cabecera ----

// botonCampana es la campana con su contador.
type botonCampana struct {
	widget.BaseWidget
	n        int
	nombre   string
	alPulsar func(fyne.CanvasObject)
	encima   bool
	enfocado bool
}

var (
	_ fyne.Tappable     = (*botonCampana)(nil)
	_ fyne.Focusable    = (*botonCampana)(nil)
	_ desktop.Hoverable = (*botonCampana)(nil)
)

func (p *Pantalla) nuevaCampana() *botonCampana {
	n := len(p.Recordatorios)
	nombre := p.t("reminders.bell", nil)
	if n > 0 {
		nombre = p.t("reminders.bellCount", i18n.Vars{"count": n})
	}
	b := &botonCampana{n: n, nombre: nombre, alPulsar: p.AlVerRecordatorios}
	b.ExtendBaseWidget(b)
	return b
}

func (b *botonCampana) Tapped(*fyne.PointEvent) {
	if b.alPulsar != nil {
		b.alPulsar(b)
	}
}
func (b *botonCampana) FocusGained()                   { b.enfocado = true; b.Refresh() }
func (b *botonCampana) FocusLost()                     { b.enfocado = false; b.Refresh() }
func (b *botonCampana) TypedRune(rune)                 {}
func (b *botonCampana) TypedKey(e *fyne.KeyEvent)      { pulsarConTecla(b, e, func() { b.Tapped(nil) }) }
func (b *botonCampana) MouseIn(*desktop.MouseEvent)    { b.encima = true; b.Refresh() }
func (b *botonCampana) MouseMoved(*desktop.MouseEvent) {}
func (b *botonCampana) MouseOut()                      { b.encima = false; b.Refresh() }

func (b *botonCampana) AccessibilityLabel() string             { return b.nombre }
func (b *botonCampana) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }
func (b *botonCampana) MinSize() fyne.Size                     { return fyne.NewSize(LadoBotonIcono, LadoBotonIcono) }

func (b *botonCampana) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &campanaRenderer{
		b:     b,
		caja:  ui.NuevaCaja(color.NRGBA{}, tema.Color("chrome-border", v), tema.Forma("radius-control")),
		icono: iconos.Trazo(iconos.Campana, 16, "header-fg", v),
		aro:   ui.NuevoAro(tema.Forma("radius-control"), 0, v),
	}
	r.objetos = r.aro.Antes(append(r.caja.Objetos(), r.icono.Objeto())...)
	if b.n > 0 {
		texto := strconv.Itoa(b.n)
		if b.n > 99 {
			texto = "99+"
		}
		r.contador = ui.NuevoContador(texto)
		r.objetos = append(r.objetos, r.contador)
	}
	r.Refresh()
	return r
}

type campanaRenderer struct {
	b        *botonCampana
	caja     *ui.Caja
	icono    *iconos.Dibujo
	contador *ui.Contador
	aro      *ui.Aro
	objetos  []fyne.CanvasObject
}

func (r *campanaRenderer) MinSize() fyne.Size           { return r.b.MinSize() }
func (r *campanaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *campanaRenderer) Destroy()                     {}

func (r *campanaRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	r.icono.Colocar((t.Width-16)/2, (t.Height-16)/2, 16)
	if r.contador != nil {
		// `absolute -top-1 -right-1`.
		m := r.contador.MinSize()
		r.contador.Move(fyne.NewPos(t.Width+tema.Espacio(1)-m.Width, -tema.Espacio(1)))
		r.contador.Resize(m)
	}
	r.aro.Colocar(t)
}

func (r *campanaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := color.NRGBA{}
	if r.b.encima {
		fondo = tema.Color("chrome-hover", v)
	}
	// La caja pinta el borde como un rectángulo lleno por DEBAJO del relleno: con el
	// relleno transparente, se compone el borde sobre la superficie de la cabecera.
	sup := tema.Color("surface", v)
	r.caja.Pintar(componer(fondo, sup), componer(tema.Color("chrome-border", v), sup))
	r.icono.Tenir("header-fg", v) // el modo puede cambiar con la app abierta
	r.aro.Ver(r.b.enfocado)
}

// componer pinta `c` (con su alfa) sobre el fondo opaco `f`.
func componer(c, f color.NRGBA) color.NRGBA { return tokens.SobreSRGB(c, f) }
