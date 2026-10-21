// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"image/color"
	"time"

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

// En la web: apps/frontend/apps/rooms/src/components/Composer.tsx (el «!» de prioridad
// y acuse, el reloj de «Enviar más tarde» y las fichas de `OptionChip`) y
// apps/frontend/apps/rooms/src/lib/schedule.ts (los atajos).
//
// Las tres opciones de envío viven tras su puerta y SOLO se ven cuando están activas:
// una ficha sobre la caja que se quita con su aspa. Nada de formulario permanente.

// HoraJornada es `WORK_START_HOUR`: los atajos de «Enviar más tarde» son a las 9:00.
const HoraJornada = 9

// PresetsProgramar es `schedulePresets`: mañana y el próximo lunes a las 9:00 locales.
// El lunes se busca estrictamente hacia delante: un lunes, es el de la semana que viene.
// Con `time.Date` y no sumando horas, que es lo que hace `setHours` al cruzar un cambio
// de hora.
func PresetsProgramar(ahora time.Time) (manana, lunes time.Time) {
	y, m, d := ahora.Date()
	manana = time.Date(y, m, d+1, HoraJornada, 0, 0, 0, ahora.Location())
	n := 1
	for ahora.AddDate(0, 0, n).Weekday() != time.Monday {
		n++
	}
	lunes = time.Date(y, m, d+n, HoraJornada, 0, 0, 0, ahora.Location())
	return manana, lunes
}

// Las medidas del panel, de la hoja de Composer.tsx.
var (
	// AnchoPrioridad es el `w-56` del panel de prioridad y AnchoProgramar el `w-60` del
	// de «Enviar más tarde».
	AnchoPrioridad = tema.Espacio(56)
	AnchoProgramar = tema.Espacio(60)
	// CanalonOpcion es el `px-1.5` de POPOVER_ITEM y POPOVER_OPTION, y RellenoOpcion
	// su `py-1.5`.
	CanalonOpcion = tema.Espacio(1.5)
	RellenoOpcion = tema.Espacio(1.5)
	// GlifoOpcion es el `size={16}` del reloj de cada atajo.
	GlifoOpcion = iconos.TallaSM
)

// FilaEmergente es POPOVER_ITEM: `flex w-full items-center gap-2 rounded-md px-1.5
// py-1.5 text-left text-sm text-text hover:bg-bg-subtle focus-visible:bg-bg-subtle`,
// con un glifo de 16 delante.
type FilaEmergente struct {
	widget.BaseWidget

	Rotulo   string
	Glifo    *iconos.Dibujo
	AlPulsar func()

	encima, enfocado bool
	fondo            *canvas.Rectangle
	texto            *canvas.Text
}

var (
	_ fyne.Widget       = (*FilaEmergente)(nil)
	_ fyne.Tappable     = (*FilaEmergente)(nil)
	_ fyne.Focusable    = (*FilaEmergente)(nil)
	_ desktop.Hoverable = (*FilaEmergente)(nil)
)

// NuevaFilaEmergente crea la fila con su glifo (nil, sin él).
func NuevaFilaEmergente(rotulo string, glifo *iconos.Dibujo, alPulsar func()) *FilaEmergente {
	f := &FilaEmergente{Rotulo: rotulo, Glifo: glifo, AlPulsar: alPulsar}
	f.ExtendBaseWidget(f)
	return f
}

// MedidaFilaEmergente es la fila: `py-1.5` y la caja de línea de `text-sm`.
func MedidaFilaEmergente(rotulo string) fyne.Size {
	return fyne.NewSize(2*CanalonOpcion+GlifoOpcion+tema.Espacio(2)+ui.Medir(rotulo, tema.TextoSM, ui.Normal).Width,
		2*RellenoOpcion+tema.TextoSM.Linea)
}

func (f *FilaEmergente) Tapped(*fyne.PointEvent) {
	if f.AlPulsar != nil {
		f.AlPulsar()
	}
}

func (f *FilaEmergente) FocusGained() { f.enfocado = true; f.Refresh() }
func (f *FilaEmergente) FocusLost()   { f.enfocado = false; f.Refresh() }
func (f *FilaEmergente) TypedRune(r rune) {
	if r == ' ' {
		f.Tapped(nil)
	}
}

func (f *FilaEmergente) TypedKey(k *fyne.KeyEvent) {
	if ui.SubirEscape(f, k) { // Escape y «atrás» son del diálogo de encima
		return
	}
	if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
		f.Tapped(nil)
	}
}
func (f *FilaEmergente) MouseIn(*desktop.MouseEvent)    { f.encima = true; f.Refresh() }
func (f *FilaEmergente) MouseMoved(*desktop.MouseEvent) {}
func (f *FilaEmergente) MouseOut()                      { f.encima = false; f.Refresh() }

// AccessibilityLabel es su rótulo.
func (f *FilaEmergente) AccessibilityLabel() string { return f.Rotulo }

// AccessibilityRole: es un botón.
func (f *FilaEmergente) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

func (f *FilaEmergente) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.fondo = canvas.NewRectangle(color.NRGBA{})
	f.fondo.CornerRadius = tema.RadioMD
	f.texto = ui.Texto(f.Rotulo, tema.TextoSM, ui.Normal, tema.Color("text", v))
	objs := []fyne.CanvasObject{f.fondo, f.texto}
	if f.Glifo != nil {
		objs = append(objs, f.Glifo.Objeto())
	}
	return &filaEmergenteRenderer{f: f, objs: objs}
}

type filaEmergenteRenderer struct {
	f    *FilaEmergente
	objs []fyne.CanvasObject
}

func (r *filaEmergenteRenderer) MinSize() fyne.Size           { return MedidaFilaEmergente(r.f.Rotulo) }
func (r *filaEmergenteRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *filaEmergenteRenderer) Destroy()                     {}

func (r *filaEmergenteRenderer) Layout(t fyne.Size) {
	f := r.f
	f.fondo.Resize(t)
	x := CanalonOpcion
	if f.Glifo != nil {
		g := f.Glifo.Objeto()
		g.Move(fyne.NewPos(x, (t.Height-GlifoOpcion)/2))
		g.Resize(fyne.NewSize(GlifoOpcion, GlifoOpcion))
		x += GlifoOpcion + tema.Espacio(2)
	}
	ui.ColocarTexto(f.texto, tema.TextoSM, x, RellenoOpcion)
}

func (r *filaEmergenteRenderer) Refresh() {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.fondo.FillColor = color.NRGBA{}
	if f.encima || f.enfocado {
		f.fondo.FillColor = tema.Color("bg-subtle", v)
	}
	f.fondo.Refresh()
	f.texto.Color = tema.Color("text", v)
	f.texto.Refresh()
	r.Layout(f.Size())
}

// rotuloEmergente es POPOVER_LABEL: `mb-1 px-1.5 sytue-overline text-text-muted`.
func rotuloEmergente(texto string) fyne.CanvasObject {
	return container.New(ui.Margen{Izq: CanalonOpcion, Abj: tema.Espacio(1)},
		ui.NuevoRotulo(ui.Versalitas(texto), tema.TextoXS, ui.SemiNegrita, "text-muted"))
}

// opcionElegible es POPOVER_OPTION: la casilla o el radio con su etiqueta en `text-sm`,
// en el mismo `px-1.5 py-1.5`.
func opcionElegible(e *ui.Eleccion) fyne.CanvasObject {
	e.Escalon = tema.TextoSM
	return container.New(ui.Margen{Izq: CanalonOpcion, Arr: RellenoOpcion, Der: CanalonOpcion, Abj: RellenoOpcion}, e)
}

// anchoFijo deja el contenido a ese ancho: los `w-56` y `w-60` de los paneles.
type anchoFijo float32

func (a anchoFijo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	alto := float32(0)
	for _, o := range objs {
		alto = max(alto, o.MinSize().Height)
	}
	return fyne.NewSize(float32(a), alto)
}

func (a anchoFijo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.Position{})
		o.Resize(t)
	}
}

// ---- Los dos paneles ----

// abrirPrioridad es el Popover del «!»: la prioridad en tres radios y, tras un pelo, la
// casilla de pedir acuse. Se aplica EN VIVO, sin Aplicar: la ficha que sale sobre la
// caja ya confirma el estado y su aspa lo deshace.
func (c *Compositor) abrirPrioridad(disparador fyne.CanvasObject) {
	t := c.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	radios := make([]*ui.Eleccion, 0, 3)
	filas := make([]fyne.CanvasObject, 0, 6)
	filas = append(filas, rotuloEmergente(t.T("composer.priority", nil)))
	for _, p := range []Prioridad{Normal, Importante, Urgente} {
		r := ui.NuevoRadio(t.T(clavePrioridad[p], nil), nil)
		r.Marcada = c.Prioridad == p
		r.AlCambiar = func(bool) {
			for _, x := range radios {
				x.Marcada = x == r
				x.Refresh()
			}
			c.PonerPrioridad(p)
		}
		radios = append(radios, r)
		filas = append(filas, opcionElegible(r))
	}
	pelo := canvas.NewRectangle(tema.Color("border-subtle", v))
	pelo.SetMinSize(fyne.NewSize(0, tema.Borde))
	acuse := ui.NuevaCasilla(t.T("composer.requestAckLong", nil), nil)
	acuse.Marcada = c.Acuse
	acuse.AlCambiar = func(si bool) {
		acuse.Marcada = si
		acuse.Refresh()
		c.PonerAcuse(si)
	}
	filas = append(filas, container.New(ui.Margen{Arr: tema.Espacio(1.5), Abj: tema.Espacio(1.5)}, pelo), opcionElegible(acuse))
	c.emergente = ui.MostrarPopover(disparador, container.New(anchoFijo(AnchoPrioridad), container.New(ui.Columna{}, filas...)))
}

// clavePrioridad es `priority.${c}`.
var clavePrioridad = [...]string{Normal: "priority.normal", Importante: "priority.important", Urgente: "priority.urgent"}

// abrirProgramar es el Popover del reloj: los dos atajos RELATIVOS y, solo si se pide,
// el control de fecha y hora con su «Listo».
func (c *Compositor) abrirProgramar(disparador fyne.CanvasObject) {
	t := c.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var e *ui.Emergente
	reloj := func() *iconos.Dibujo { return dibujo(glifoReloj, GlifoOpcion, "text", v) }
	manana, lunes := PresetsProgramar(c.ahora())
	filas := make([]fyne.CanvasObject, 0, 3)
	filas = append(filas, rotuloEmergente(t.T("composer.sendLater", nil)))
	for _, p := range []struct {
		clave  string
		cuando time.Time
	}{{"schedule.tomorrow", manana}, {"schedule.monday", lunes}} {
		filas = append(filas, NuevaFilaEmergente(t.T(p.clave, i18n.Vars{"time": i18n.FormatTime(c.Locale, p.cuando)}), reloj(), func() {
			c.PonerProgramado(p.cuando)
			e.Cerrar()
		}))
	}
	col := container.New(ui.Columna{}, filas...)
	var personalizada *FilaEmergente
	personalizada = NuevaFilaEmergente(t.T("composer.customDateTime", nil),
		iconos.Trazo(iconos.Puntos, GlifoOpcion, "text-muted", v), func() {
			col.Remove(personalizada)
			col.Add(c.fechaPersonalizada(func() { e.Cerrar() }))
			e.Recolocar()
		})
	col.Add(personalizada)
	e = ui.MostrarPopover(disparador, container.New(anchoFijo(AnchoProgramar), col))
	c.emergente = e
}

// fechaPersonalizada es el `datetime-local` revelado: «Fecha y hora» en `text-xs
// font-medium text-muted mb-1`, el control, y «Listo» a la derecha, apagado hasta que
// lo escrito sea una fecha futura.
func (c *Compositor) fechaPersonalizada(cerrar func()) fyne.CanvasObject {
	t := c.T
	campo := ui.NuevoCampoFechaHora(ui.CampoSM, c.ProgramarA)
	listo := ui.NuevoBoton(t.T("action.done", nil), nil).Con(ui.Primario, ui.TallaSM)
	valer := func() (time.Time, bool) {
		cuando, ok := campo.Leer(time.Local)
		return cuando, ok && cuando.After(c.ahora())
	}
	confirmar := func(cuando time.Time) {
		if cuando.After(c.ahora()) {
			c.PonerProgramado(cuando)
			cerrar()
		}
	}
	listo.AlPulsar = func() {
		if cuando, ok := valer(); ok {
			confirmar(cuando)
		}
	}
	campo.AlConfirmar = confirmar
	_, ok := valer()
	listo.Inerte = !ok
	for _, x := range []*ui.Entrada{campo.Fecha, campo.Hora} {
		x.AlCambiar(func(string) {
			_, ok := valer()
			listo.Inerte = !ok
			listo.Refresh()
		})
	}
	rotulo := ui.NuevoRotulo(t.T("composer.dateAndTime", nil), tema.TextoXS, ui.Medio, "text-muted")
	return container.New(ui.Margen{Izq: CanalonOpcion, Arr: tema.Espacio(1.5), Der: CanalonOpcion},
		container.New(ui.Columna{Hueco: tema.Espacio(1)}, rotulo, campo.Objeto(),
			container.New(ui.Margen{Arr: tema.Espacio(1)}, container.New(alFinal{}, listo))))
}

// alFinal es el `flex justify-end` de la fila de «Listo»: el hijo a su tamaño y a la
// derecha.
type alFinal struct{}

func (alFinal) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }

func (alFinal) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	m := objs[0].MinSize()
	objs[0].Move(fyne.NewPos(t.Width-m.Width, 0))
	objs[0].Resize(m)
}

// ---- Las fichas: `OptionChip` ----

// FichaOpcion es `OptionChip`: una opción de envío ACTIVA sobre la caja, con su aspa.
// `inline-flex items-center gap-1 rounded-md pr-0.5 pl-2 text-xs font-semibold` y el
// botón de quitar `min-h-[24px] min-w-[24px]` con la cruz de 12.
type FichaOpcion struct {
	widget.BaseWidget

	Rotulo string
	// Fondo y Tinta son los tokens del tono: `warning`/`danger` en la prioridad y
	// `bg-subtle`/`text-muted` en las demás.
	Fondo, Tinta string

	quitar *ui.BotonIcono
	caja   *canvas.Rectangle
	texto  *canvas.Text
}

// NuevaFichaOpcion crea la ficha; `rotuloQuitar` es el nombre de su aspa.
func NuevaFichaOpcion(rotulo, fondo, tinta, rotuloQuitar string, alQuitar func()) *FichaOpcion {
	f := &FichaOpcion{Rotulo: rotulo, Fondo: fondo, Tinta: tinta}
	f.quitar = ui.NuevoBotonIcono(iconos.Cerrar, rotuloQuitar, alQuitar)
	f.quitar.Lado, f.quitar.TamIcono, f.quitar.Radio = LadoQuitarFicha, iconos.TallaXXS, tema.RadioSM
	// `hover:opacity-70` y no un velo: la ficha ya es de color.
	f.quitar.Trazo, f.quitar.TrazoEncima, f.quitar.FondoEncima = tinta, tinta, ""
	f.ExtendBaseWidget(f)
	return f
}

// LadoQuitarFicha es el `min-h-[24px] min-w-[24px]` del aspa.
const LadoQuitarFicha float32 = 24

// MedidaFichaOpcion es `pl-2`, el texto, `gap-1`, el aspa y `pr-0.5`.
func MedidaFichaOpcion(rotulo string) fyne.Size {
	return fyne.NewSize(tema.Espacio(2)+ui.Medir(rotulo, tema.TextoXS, ui.SemiNegrita).Width+tema.Espacio(1)+
		LadoQuitarFicha+tema.Espacio(0.5), LadoQuitarFicha)
}

// AccessibilityLabel es su rótulo.
func (f *FichaOpcion) AccessibilityLabel() string { return f.Rotulo }

// AccessibilityRole: es texto con un botón dentro.
func (f *FichaOpcion) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

func (f *FichaOpcion) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.caja = canvas.NewRectangle(tema.Color(f.Fondo, v))
	f.caja.CornerRadius = tema.RadioMD
	f.texto = ui.Texto(f.Rotulo, tema.TextoXS, ui.SemiNegrita, tema.Color(f.Tinta, v))
	return &fichaRenderer{f: f}
}

type fichaRenderer struct{ f *FichaOpcion }

func (r *fichaRenderer) MinSize() fyne.Size { return MedidaFichaOpcion(r.f.Rotulo) }
func (r *fichaRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.f.caja, r.f.texto, r.f.quitar}
}
func (r *fichaRenderer) Destroy() {}

func (r *fichaRenderer) Layout(t fyne.Size) {
	f := r.f
	f.caja.Resize(t)
	ui.ColocarTexto(f.texto, tema.TextoXS, tema.Espacio(2), (t.Height-tema.TextoXS.Linea)/2)
	f.quitar.Move(fyne.NewPos(t.Width-tema.Espacio(0.5)-LadoQuitarFicha, (t.Height-LadoQuitarFicha)/2))
	f.quitar.Resize(fyne.NewSize(LadoQuitarFicha, LadoQuitarFicha))
}

func (r *fichaRenderer) Refresh() {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.caja.FillColor = tema.Color(f.Fondo, v)
	f.caja.Refresh()
	f.texto.Color = tema.Color(f.Tinta, v)
	f.texto.Refresh()
	r.Layout(f.Size())
}

// HuecoFichas es el `gap-2` de la fila de fichas y AireFichas su `mb-2`.
var (
	HuecoFichas = tema.Espacio(2)
	AireFichas  = tema.Espacio(2)
)

// fichas son las de las opciones activas, en el orden de la web: prioridad, acuse y
// programación.
func (c *Compositor) fichas() []fyne.CanvasObject {
	if c.Sencillo {
		return nil
	}
	t := c.T
	var out []fyne.CanvasObject
	switch c.Prioridad {
	case Importante:
		out = append(out, NuevaFichaOpcion(ui.Versalitas(t.T("priority.important", nil)), "warning-bg", "warning-text",
			t.T("composer.clearPriority", nil), func() { c.PonerPrioridad(Normal) }))
	case Urgente:
		out = append(out, NuevaFichaOpcion(ui.Versalitas(t.T("priority.urgent", nil)), "danger-bg", "danger-text",
			t.T("composer.clearPriority", nil), func() { c.PonerPrioridad(Normal) }))
	case Normal:
	}
	if c.Acuse {
		out = append(out, NuevaFichaOpcion(t.T("composer.requestAck", nil), "bg-subtle", "text-muted",
			t.T("composer.clearAck", nil), func() { c.PonerAcuse(false) }))
	}
	if !c.ProgramarA.IsZero() {
		// `formatSchedule`: la fecha ABSOLUTA y no «mañana», que se vuelve mentira al
		// cruzar la medianoche con la ficha en pantalla.
		out = append(out, NuevaFichaOpcion(t.T("scheduled.sendsAt", i18n.Vars{"when": RotuloProgramado(c.Locale, c.ProgramarA)}),
			"bg-subtle", "text-muted", t.T("composer.clearSchedule", nil), func() { c.PonerProgramado(time.Time{}) }))
	}
	return out
}

// altoFichas es lo que las fichas le suman a la franja: una fila y su `mb-2`, o nada.
//
// ponytail: las fichas no parten línea (`flex-wrap`): las tres juntas miden ~330 y la
// caja más estrecha que se pinta, 328 (360 − 2×16). Si una traducción las pasa, se
// solapan con el borde derecho; se parte cuando alguna lo haga.
func (c *Compositor) altoFichas() float32 {
	if len(c.objFichas) == 0 {
		return 0
	}
	return LadoQuitarFicha + AireFichas
}
