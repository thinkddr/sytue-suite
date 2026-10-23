// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Programar el envío y los envíos programados
//
// Es el mismo diferido que el «deshacer», con otra espera: el correo queda en la cola
// hasta la hora elegida y hasta entonces se ve en «Programados», se reprograma o se
// cancela.

// campoCuando es la fecha y la hora. En la web es UN `<input type="datetime-local">`
// cuyo selector pinta el navegador; aquí son los dos campos de la suite, fecha y hora,
// que es como se teclea en Fyne (`ui.CampoFecha`, `ui.CampoHora`).
type campoCuando struct {
	fecha, hora *ui.Entrada
	fila        *fyne.Container
}

func nuevoCampoCuando(t time.Time) *campoCuando {
	c := &campoCuando{fecha: ui.CampoFecha(ui.CampoMD), hora: ui.CampoHora(ui.CampoMD)}
	c.fecha.PonerTexto(ui.EscribirFecha(t))
	c.hora.PonerTexto(ui.EscribirHora(t))
	c.fecha.Ancho, c.hora.Ancho = 8*ui.AltoCampo(ui.CampoMD)/2, 5*ui.AltoCampo(ui.CampoMD)/2
	c.fila = container.New(ui.Fila{Hueco: huecoFilaCab}, c.fecha, c.hora)
	return c
}

func (c *campoCuando) objeto() fyne.CanvasObject { return c.fila }

// leer devuelve la hora elegida y si vale.
func (c *campoCuando) leer() (time.Time, bool) {
	dia, vacia, ok := ui.LeerFecha(c.fecha.Texto(), time.Local)
	if vacia || !ok {
		return time.Time{}, false
	}
	h, m, vacia, ok := ui.LeerHora(c.hora.Texto())
	if vacia || !ok {
		return time.Time{}, false
	}
	return dia.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute), true
}

// programar es `handleSchedule`: la hora tiene que ser futura, el mensaje tiene que
// poder enviarse, y al programarse se cierra el redactor diciendo cuándo saldrá.
func (b *Buzon) programar(rd *redaccion, cuando time.Time, ok bool) {
	if !b.redaccionVigente(rd) || rd.enviando || rd.guardando {
		return
	}
	if !ok || !cuando.After(time.Now()) {
		b.avisar(ui.AvisoPeligro, "compose.scheduleFuture", nil)
		return
	}
	br := rd.r.B
	if len(direccionesLaxas(br.Para)) == 0 {
		rd.r.PonerError(b.P.T.T("compose.toRequired", nil))
		return
	}
	de, s, crudo, err := b.componer(br, false)
	if err != nil {
		b.avisarEnvio(err)
		return
	}
	if rd.pararAuto != nil {
		rd.pararAuto()
	}
	rd.enviando = true
	rd.r.BotonProgramar.Cargando = true
	rd.r.BotonProgramar.Refresh()
	b.pintarBotonesRedactor(rd)
	pedir(b, func(ctx context.Context) (api.MailEnvio, error) {
		return b.S.Schedule(ctx, de, s.Destinatarios(), crudo, cuando)
	}, func(_ api.MailEnvio, err error) {
		rd.enviando = false
		rd.r.BotonProgramar.Cargando = false
		rd.r.BotonProgramar.Refresh()
		if err != nil {
			b.pintarBotonesRedactor(rd)
			b.avisar(ui.AvisoPeligro, "compose.scheduleFailed", nil)
			return
		}
		if id := br.ID; id != "" {
			ctx := b.ctx
			b.Lanzar(func() {
				if ctx.Err() == nil {
					_ = b.S.Delete(ctx, id)
				}
			})
		}
		b.cerrar(rd)
		b.alGuardarBorrador()
		b.P.Avisar(ui.AvisoExito, b.P.T.T("compose.scheduled", nil), FechaLarga(b.P.Locale, cuando))
	})
}

// ---- «Programados» (`ScheduledModal.tsx`) ----

// verProgramados abre la lista de los envíos que todavía no han salido, con su hora,
// para cambiarla o cancelar el envío.
func (b *Buzon) verProgramados() {
	lienzo := b.P.Lienzo()
	if lienzo == nil {
		return
	}
	t := b.P.T
	lista := &listaProgramados{b: b, ancho: AnchoCuerpoDialogo(ui.DialogoLG, lienzo.Size().Width)}
	lista.caja = container.New(ui.Columna{})
	var d *ui.Dialogo
	cerrar := ui.NuevoBoton(t.T("common.close", nil), func() { d.Cerrar() }).Con(ui.Secundario, ui.TallaMD)
	d = ui.NuevoDialogo(t.T("scheduled.title", nil), lista.caja).Con(ui.DialogoLG, cerrar)
	d.RotuloCerrar = t.T("ui.close", nil)
	lista.d = d
	d.Mostrar(lienzo)
	lista.cargar()
}

// listaProgramados es el estado del diálogo: cargando, error, vacío o la lista, y la
// fila que se está editando.
type listaProgramados struct {
	b        *Buzon
	d        *ui.Dialogo
	caja     *fyne.Container
	ancho    float32
	items    []api.MailScheduled
	editando string
	ocupado  string
}

func (l *listaProgramados) cargar() {
	l.pintar(Cargando)
	pedir(l.b, l.b.S.Scheduled, func(xs []api.MailScheduled, err error) {
		if err != nil {
			l.pintar(Error)
			return
		}
		l.items = xs
		l.pintar(Listo)
	})
}

// pintar rehace el cuerpo en el estado pedido y vuelve a medir el diálogo.
func (l *listaProgramados) pintar(e Estado) {
	t := l.b.P.T
	var objs []fyne.CanvasObject
	switch {
	case e == Cargando:
		// `flex items-center gap-2 py-6` con la rueda de 16 y «Cargando…».
		rueda := ui.NuevaRueda(16, "text-subtle", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
		rueda.Rotulo = t.T("scheduled.loading", nil)
		objs = append(objs, container.New(ui.Margen{Arr: tema.Espacio(6), Abj: tema.Espacio(6)},
			container.New(ui.Fila{Hueco: tema.Espacio(2)}, rueda,
				ui.NuevoRotulo(t.T("common.loading", nil), tema.TextoSM, ui.Normal, "text-subtle"))))
	case e == Error:
		objs = append(objs, container.New(ui.Columna{Hueco: tema.Espacio(2)},
			ui.NuevoRotulo(t.T("scheduled.loadError", nil), tema.TextoSM, ui.Normal, "text"),
			ui.NuevoBoton(t.T("common.retry", nil), l.cargar).Con(ui.Secundario, ui.TallaSM)))
	case len(l.items) == 0:
		objs = append(objs, container.New(ui.Margen{Arr: tema.Espacio(6), Abj: tema.Espacio(6)},
			ui.NuevoParrafo(t.T("scheduled.empty", nil), tema.TextoSM, ui.Normal, "text-subtle",
				tema.De(fyne.CurrentApp().Settings().ThemeVariant())).A(l.ancho)))
	default:
		filas := make([]fyne.CanvasObject, 0, len(l.items))
		for _, x := range l.items {
			filas = append(filas, l.fila(x))
		}
		objs = append(objs, enCajaConFiletes(filas))
	}
	l.caja.Objects = objs
	l.caja.Refresh()
	if l.d != nil {
		l.d.Recolocar() // el diálogo se vuelve a medir con su contenido nuevo
	}
}

// fila es un programado: asunto, destinatarios y hora, con «Editar hora» y
// «Cancelar»; en edición, el campo de la hora nueva debajo.
func (l *listaProgramados) fila(x api.MailScheduled) fyne.CanvasObject {
	t, loc := l.b.P.T, l.b.P.Locale
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	asunto := x.Subject
	if asunto == "" {
		asunto = t.T("common.noSubject", nil)
	}
	// Asunto y destinatarios `truncate`, como la web: en una sola línea con «…». Con
	// rótulos de ancho fijo se pintaban por debajo de «Editar hora» y fuera del panel.
	texto := container.New(ui.Columna{},
		ui.NuevoParrafo(asunto, tema.TextoSM, ui.Medio, "text", v).Recortado(1),
		ui.NuevoParrafo(t.T("scheduled.recipients", i18n.Vars{"addresses": joinComa(x.Recipients)}), tema.TextoXS, ui.Normal, "text-subtle", v).Recortado(1),
		container.New(ui.Fila{Hueco: tema.Espacio(1.5)},
			iconos.Trazo(iconos.Campana, 14, "text-subtle", v).Objeto(),
			ui.NuevoRotulo(FechaLarga(loc, x.ReleaseAt), tema.TextoXS, ui.Normal, "text-subtle")))
	var cabeza []fyne.CanvasObject
	cabeza = append(cabeza, texto)
	if l.editando != x.ID {
		editar := ui.NuevoBoton(t.T("scheduled.editTime", nil), func() { l.editando = x.ID; l.pintar(Listo) }).
			Con(ui.Fantasma, ui.TallaSM)
		editar.Inerte = l.ocupado == x.ID
		cancelar := ui.NuevoBoton(t.T("common.cancel", nil), func() { l.cancelar(x.ID) }).Con(ui.Fantasma, ui.TallaSM)
		cancelar.Cargando = l.ocupado == x.ID
		cabeza = append(cabeza, container.New(ui.Fila{Hueco: tema.Espacio(2)}, editar, cancelar))
	}
	partes := []fyne.CanvasObject{container.New(&extremos{}, cabeza...)}
	if enTelefono(l.b) && len(cabeza) == 2 {
		// `max-md:flex-col`: el mensaje entero arriba y sus acciones debajo, a la derecha.
		partes[0] = container.New(ui.Columna{Hueco: tema.Espacio(2)}, texto,
			container.New(&extremos{}, layout.NewSpacer(), cabeza[1]))
	}
	if l.editando == x.ID {
		cuando := nuevoCampoCuando(x.ReleaseAt.Local())
		guardar := ui.NuevoBoton(t.T("scheduled.saveTime", nil), func() { l.reprogramar(x.ID, cuando) }).
			Con(ui.Primario, ui.TallaSM)
		guardar.Cargando = l.ocupado == x.ID
		partes = append(partes, container.New(ui.Columna{Hueco: tema.Espacio(3)},
			canvasFilete(v),
			ui.GrupoCampo(t.T("scheduled.newDateTime", nil), false, cuando.objeto(), "", ""),
			container.New(ui.Fila{Hueco: tema.Espacio(2)},
				ui.NuevoBoton(t.T("common.discard", nil), func() { l.editando = ""; l.pintar(Listo) }).Con(ui.Fantasma, ui.TallaSM),
				guardar)))
	}
	return container.New(ui.Margen{Izq: tema.Espacio(3), Der: tema.Espacio(3), Arr: tema.Espacio(3), Abj: tema.Espacio(3)},
		container.New(ui.Columna{Hueco: tema.Espacio(2)}, partes...))
}

func (l *listaProgramados) cancelar(id string) {
	l.ocupado = id
	l.pintar(Listo)
	pedir(l.b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, l.b.S.CancelOutbound(ctx, id)
	}, func(_ struct{}, err error) {
		l.ocupado = ""
		if err != nil {
			l.b.avisar(ui.AvisoPeligro, "scheduled.cancelGone", nil)
			l.cargar()
			return
		}
		for i, x := range l.items {
			if x.ID == id {
				l.items = append(l.items[:i:i], l.items[i+1:]...)
				break
			}
		}
		l.b.avisar(ui.AvisoExito, "scheduled.canceled", nil)
		l.pintar(Listo)
	})
}

func (l *listaProgramados) reprogramar(id string, c *campoCuando) {
	cuando, ok := c.leer()
	if !ok || !cuando.After(time.Now()) {
		l.b.avisar(ui.AvisoPeligro, "compose.scheduleFuture", nil)
		return
	}
	l.ocupado = id
	l.pintar(Listo)
	pedir(l.b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, l.b.S.Reschedule(ctx, id, cuando)
	}, func(_ struct{}, err error) {
		l.ocupado = ""
		if err != nil {
			l.b.avisar(ui.AvisoPeligro, "scheduled.rescheduleGone", nil)
			l.cargar()
			return
		}
		for i := range l.items {
			if l.items[i].ID == id {
				l.items[i].ReleaseAt = cuando
			}
		}
		slices.SortFunc(l.items, func(a, x api.MailScheduled) int { return a.ReleaseAt.Compare(x.ReleaseAt) })
		l.editando = ""
		l.b.avisar(ui.AvisoExito, "scheduled.timeUpdated", nil)
		l.pintar(Listo)
	})
}

func joinComa(xs []string) string { return strings.Join(xs, ", ") }

// extremos es `flex items-start justify-between gap-3`: el primero a la izquierda y el
// último pegado a la derecha, arriba los dos. El primero se queda con lo que sobra
// (`min-w-0 flex-1`). Con `hueco` otro `gap`, y con `centro`, `items-center`.
type extremos struct {
	hueco  float32
	centro bool
	// desborda es para un primero que no encoge (un botón): si no cabe, lo de la
	// derecha se sale en vez de pisarlo.
	desborda bool
}

func (e extremos) gap() float32 {
	if e.hueco == 0 {
		return tema.Espacio(3)
	}
	return e.hueco
}

func (e extremos) MinSize(objs []fyne.CanvasObject) fyne.Size {
	w, h := float32(0), float32(0)
	for i, o := range objs {
		m := o.MinSize()
		if i > 0 {
			w += e.gap()
		}
		w += m.Width
		h = max(h, m.Height)
	}
	return fyne.NewSize(w, h)
}

func (e extremos) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) == 0 {
		return
	}
	y := func(m fyne.Size) float32 {
		if e.centro {
			return (t.Height - m.Height) / 2
		}
		return 0
	}
	x := t.Width
	for i := len(objs) - 1; i >= 1; i-- {
		m := objs[i].MinSize()
		x -= m.Width
		objs[i].Move(fyne.NewPos(x, y(m)))
		objs[i].Resize(m)
		x -= e.gap()
	}
	m := objs[0].MinSize()
	objs[0].Move(fyne.NewPos(0, y(m)))
	objs[0].Resize(fyne.NewSize(max(x, 0), m.Height))
	// Con `desborda`, sin sitio el `ms-auto` se queda en cero: lo de la derecha va
	// detrás del primero y se sale por el borde, en vez de montarse encima.
	if e.desborda && x < m.Width {
		for i, xi := 1, m.Width+e.gap(); i < len(objs); i++ {
			objs[i].Move(fyne.NewPos(xi, objs[i].Position().Y))
			xi += objs[i].MinSize().Width + e.gap()
		}
	}
}

// canvasFilete es el `border-t` de un bloque: una línea de un píxel.
func canvasFilete(v tema.Variante) fyne.CanvasObject {
	f := canvas.NewRectangle(tema.Color("border", v))
	f.SetMinSize(fyne.NewSize(0, tema.Borde))
	return f
}

// enCajaConFiletes es `divide-y border rounded-md`: las filas en una caja con borde y
// un filete entre cada dos.
func enCajaConFiletes(filas []fyne.CanvasObject) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	objs := make([]fyne.CanvasObject, 0, 2*len(filas))
	for i, f := range filas {
		if i > 0 {
			objs = append(objs, canvasFilete(v))
		}
		objs = append(objs, f)
	}
	caja := ui.NuevaCaja(tema.Color("surface-raised", v), tema.Color("border", v), tema.RadioMD)
	dentro := container.New(ui.Columna{}, objs...)
	return container.New(&enCaja{caja: caja}, append(caja.Objetos(), dentro)...)
}

// enCaja coloca la caja a todo el tamaño y el contenido un borde por dentro: con
// `box-sizing: border-box` el borde ocupa sitio. El último hijo es el contenido.
type enCaja struct{ caja *ui.Caja }

func (e *enCaja) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[len(objs)-1].MinSize()
	return fyne.NewSize(m.Width+2*tema.Borde, m.Height+2*tema.Borde)
}

func (e *enCaja) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	e.caja.Redimensionar(t)
	d := objs[len(objs)-1]
	d.Move(fyne.NewPos(tema.Borde, tema.Borde))
	d.Resize(fyne.NewSize(t.Width-2*tema.Borde, t.Height-2*tema.Borde))
}
