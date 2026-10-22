// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// «Buscar un hueco»: FindTimePanel.tsx, dentro del formulario y bajo las horas, solo en
// un evento con hora. Un botón fantasma lo abre y lo cierra; el panel pide al servicio
// los tramos LIBRES de la persona en una ventana de días y en la franja laboral, y
// ofrece horas de inicio que, al pulsarlas, fijan el evento en ese hueco. El cálculo
// —cruces, franja y horario de verano— es del servicio; aquí solo se pide y se pinta.

// FranjaDesde y FranjaHasta son WORK_START y WORK_END.
const (
	FranjaDesde = "09:00"
	FranjaHasta = "17:00"
)

// VentanasHuecos son WINDOWS: las ventanas de búsqueda, en días.
var VentanasHuecos = []int{7, 14, 30}

// Los topes de `candidateStarts`: cada media hora, cuatro por hueco y dieciséis en total.
const (
	pasoHueco  = 30 * time.Minute
	porHueco   = 4
	maxInicios = 16
)

// IniciosCandidatos es `candidateStarts`: las horas de inicio que caben en cada hueco
// libre, de media en media hora.
func IniciosCandidatos(huecos []api.Hueco, dur time.Duration) []time.Time {
	var out []time.Time
	for _, h := range huecos {
		for t, k := h.Start, 0; !t.Add(dur).After(h.End) && k < porHueco; t, k = t.Add(pasoHueco), k+1 {
			out = append(out, t)
			if len(out) >= maxInicios {
				return out
			}
		}
	}
	return out
}

// MinutosEntre es `durationMinutesOf`: los minutos del evento, o 60 si el fin falta o
// deja menos de un cuarto de hora.
func MinutosEntre(inicio, fin time.Time) int {
	m := fin.Sub(inicio).Minutes()
	if m >= 15 {
		return int(math.Round(m))
	}
	return 60
}

// VentanaHuecos es la ventana que se pide: desde el principio del día del inicio del
// evento —o desde ahora, si ya pasó— hasta `dias` días después de ese principio.
func VentanaHuecos(inicio, ahora time.Time, dias int, loc *time.Location) (desde, hasta time.Time) {
	i := inicio.In(loc)
	dia := time.Date(i.Year(), i.Month(), i.Day(), 0, 0, 0, 0, loc)
	desde = dia
	if ahora.After(dia) {
		desde = ahora
	}
	return desde, dia.AddDate(0, 0, dias)
}

// buscaHueco es el estado del panel mientras vive el formulario.
type buscaHueco struct {
	abierto, cargando, fallo bool
	dias                     int
	lista                    []api.Hueco
	// turno descarta las respuestas viejas (el `stale` del efecto de la web).
	turno      int
	laborables *ui.Conmutador
	ventana    *ui.Selector
	// esqueletos son los dos de «cargando», los mismos mientras dure la carga: el pulso
	// es una animación y se para al terminar, no cuando Fyne recoja los viejos.
	esqueletos []fyne.CanvasObject
}

// cargado termina la carga y para el pulso de los esqueletos.
func (h *buscaHueco) cargado() {
	h.cargando = false
	for _, o := range h.esqueletos {
		if e, ok := o.(*ui.Esqueleto); ok {
			e.Parar()
		}
	}
	h.esqueletos = nil
}

// construirHuecos crea los dos controles del panel una sola vez, para que no pierdan
// el foco cada vez que el formulario se rehace.
func (f *formulario) construirHuecos() {
	p := f.p
	h := &buscaHueco{dias: VentanasHuecos[0]}
	h.laborables = ui.NuevoConmutador(p.t("findTime.weekdaysOnly", nil), func(bool) { f.pedirHuecos() })
	h.laborables.Encendido = true
	opciones := make([]string, len(VentanasHuecos))
	for i, d := range VentanasHuecos {
		opciones[i] = p.t("findTime.windowDays", i18n.Vars{"count": d})
	}
	h.ventana = ui.NuevoSelector(opciones, func(i int) { h.dias = VentanasHuecos[i]; f.pedirHuecos() })
	// `class="h-8"` sobre el Select md: la caja baja a 28 y se queda con `pl-3 pr-9
	// text-base`; en su fila flex, mide lo que su opción más ancha.
	var opcion float32
	for _, o := range opciones {
		opcion = max(opcion, ui.Medir(o, tema.TextoBase, ui.Normal).Width)
	}
	h.ventana.Ancho, h.ventana.Alto = opcion+2*tema.Borde+espacio3+tema.Espacio(9), tema.Espacio(8)
	f.bh = h
}

// alternarHuecos abre o cierra el panel; al abrirlo, pide.
func (f *formulario) alternarHuecos() {
	f.bh.abierto = !f.bh.abierto
	if f.bh.abierto {
		f.pedirHuecos()
		return
	}
	f.bh.cargado()
	f.rehacer()
}

// pedirHuecos lanza la consulta con lo que diga el formulario en este momento.
func (f *formulario) pedirHuecos() {
	h := f.bh
	if h == nil || !h.abierto || f.a.Huecos == nil {
		return
	}
	f.leerFechas()
	loc := f.p.zona()
	desde, hasta := VentanaHuecos(f.e.Inicio, time.Now(), h.dias, loc)
	h.turno++
	turno := h.turno
	h.cargando, h.fallo = true, false
	f.rehacer()
	f.a.Huecos(desde, hasta, MinutosEntre(f.e.Inicio, f.e.Fin), f.e.Zona, h.laborables.Encendido,
		func(l []api.Hueco, err error) {
			if turno != h.turno {
				return
			}
			h.cargado()
			h.fallo, h.lista = err != nil, l
			f.rehacer()
		})
}

// elegirHueco es `onPick`: el evento pasa a ese hueco y el panel se cierra.
func (f *formulario) elegirHueco(inicio time.Time, minutos int) {
	f.e.Inicio, f.e.Fin = inicio, inicio.Add(time.Duration(minutos)*time.Minute)
	f.escribirFechas()
	f.bh.abierto = false
	f.bh.cargado()
	if f.errFin != "" {
		f.errFin, f.fin.Invalida = "", false
	}
	f.rehacer()
}

// bloqueHuecos es el botón y, si está abierto, el panel (`flex flex-col gap-2`).
func (f *formulario) bloqueHuecos() fyne.CanvasObject {
	p, h := f.p, f.bh
	rotulo := p.t("event.findTimeShow", nil)
	if h.abierto {
		rotulo = p.t("event.findTimeHide", nil)
	}
	b := ui.NuevoBoton(rotulo, f.alternarHuecos).Con(ui.Fantasma, ui.TallaSM)
	b.Icono, b.TamIcono = iconos.Buscar, 16
	if !h.abierto {
		return encajar(b)
	}
	return container.New(ui.Columna{Hueco: espacio2}, encajar(b), f.panelHuecos())
}

// panelHuecos es `border-border bg-bg-subtle flex flex-col gap-3 rounded-md border p-3`.
func (f *formulario) panelHuecos() fyne.CanvasObject {
	p, h := f.p, f.bh
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	interior := f.util() - 2*(tema.Borde+espacio3)
	minutos := MinutosEntre(f.e.Inicio, f.e.Fin)

	// Un párrafo sin ancho no sabe cuánto mide: el de su única línea, sin pasar del panel.
	aSuLinea := func(s string, e tema.Escalon, token string, tope float32) *ui.Parrafo {
		return ui.NuevoParrafo(s, e, ui.Normal, token, v).A(min(ui.Medir(s, e, ui.Normal).Width, tope))
	}
	resumen := aSuLinea(p.t("findTime.summary", i18n.Vars{"start": FranjaDesde, "end": FranjaHasta, "duration": minutos}),
		tema.TextoXS, "text-muted", interior)
	// `flex items-center gap-3`: envolver a su ancho justo es una fila centrada en vertical.
	natural := h.laborables.MinSize().Width + espacio3 + h.ventana.MinSize().Width
	controles := container.New(&envolver{Hueco: espacio3, Ancho: natural}, h.laborables, h.ventana)
	filas := []fyne.CanvasObject{container.New(&envolver{Hueco: espacio2, Ancho: interior, Repartir: true}, resumen, controles)}

	inicios := IniciosCandidatos(h.lista, time.Duration(minutos)*time.Minute)
	switch {
	case h.cargando:
		// `<Skeleton height="1.75rem" />` y el mismo al 70 %.
		if h.esqueletos == nil {
			h.esqueletos = []fyne.CanvasObject{ui.NuevoEsqueleto(0, tema.Espacio(7)), ui.NuevoEsqueleto(0, tema.Espacio(7))}
		}
		filas = append(filas, container.New(ui.Columna{Hueco: espacio1p5},
			h.esqueletos[0], container.New(fraccion(0.7), h.esqueletos[1])))
	case h.fallo:
		otra := ui.NuevoBoton(p.t("common.retry", nil), f.pedirHuecos).Con(ui.Secundario, ui.TallaSM)
		filas = append(filas, container.New(&envolver{Hueco: espacio2, Ancho: interior, Repartir: true},
			aSuLinea(p.t("findTime.error", nil), tema.TextoSM, "text-subtle", interior-espacio2-otra.MinSize().Width), otra))
	case len(inicios) == 0:
		filas = append(filas, container.New(layout.NewCustomPaddedLayout(espacio2, espacio2, 0, 0),
			ui.NuevoParrafo(p.t("findTime.empty", nil), tema.TextoSM, ui.Normal, "text-subtle", v).A(interior)))
	default:
		filas = append(filas, f.gruposHuecos(inicios, minutos, interior, v))
	}
	caja := ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioMD)
	return container.New(&conCaja{caja: caja, X: espacio3, Y: espacio3},
		append(caja.Objetos(), container.New(ui.Columna{Hueco: espacio3}, filas...))...)
}

// gruposHuecos agrupa las horas por día natural EN LA ZONA DEL EVENTO: `ul gap-2`, y
// cada día un `sytue-meta font-medium first-letter:uppercase` con sus horas en `flex-wrap gap-1.5`.
func (f *formulario) gruposHuecos(inicios []time.Time, minutos int, interior float32, v tema.Variante) fyne.CanvasObject {
	loc, err := time.LoadLocation(f.e.Zona)
	if err != nil {
		loc = f.p.zona()
	}
	l := f.p.locale()
	var dias []fyne.CanvasObject
	var clave string
	horas := make([]fyne.CanvasObject, 0, len(inicios))
	cerrar := func() {
		if len(horas) == 0 {
			return
		}
		dia := alto(ui.Texto(capitalizar(clave), tema.TextoXS, ui.Medio, tema.Color("text-subtle", v)), tema.TextoXS.Linea)
		dias = append(dias, container.New(ui.Columna{Hueco: espacio1}, dia,
			container.New(&envolver{Hueco: espacio1p5, Ancho: interior}, horas...)))
		horas = nil
	}
	for _, t := range inicios {
		t = t.In(loc)
		if k := i18n.FormatWeekdayDayMonthShort(l, t); k != clave {
			cerrar()
			clave = k
		}
		horas = append(horas, nuevoBotonHueco(i18n.FormatTimeDigits(l, t), func() { f.elegirHueco(t, minutos) }))
	}
	cerrar()
	return container.New(ui.Columna{Hueco: espacio2}, dias...)
}

// fraccion deja a su hijo en esa parte del ancho: el `width="70%"` del segundo Skeleton.
type fraccion float32

func (fr fraccion) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }
func (fr fraccion) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(t.Width*float32(fr), t.Height))
}

// botonHueco es una hora de «buscar hueco». No es un `Button` sino un `<button>` con
// sus clases, más bajo que el `sm`:
//
//	border-border bg-surface text-text rounded-md border px-2.5 py-1 text-sm
//	hover:border-border-strong hover:bg-bg-subtle tabular-nums
//
// `tabular-nums` es ui.TextoTabular: el conformador de Fyne no activa `tnum`, así que
// cada carácter se pinta en la casilla que le da el `tnum` de la fuente, y las horas de un
// día miden lo mismo.
type botonHueco struct {
	widget.BaseWidget
	hora     string
	alPulsar func()

	encima, enfocado bool
}

var (
	_ fyne.Tappable      = (*botonHueco)(nil)
	_ fyne.Focusable     = (*botonHueco)(nil)
	_ desktop.Hoverable  = (*botonHueco)(nil)
	_ desktop.Cursorable = (*botonHueco)(nil)
)

func nuevoBotonHueco(hora string, alPulsar func()) *botonHueco {
	b := &botonHueco{hora: hora, alPulsar: alPulsar}
	b.ExtendBaseWidget(b)
	return b
}

func (b *botonHueco) Tapped(*fyne.PointEvent) {
	if b.alPulsar != nil {
		b.alPulsar()
	}
}

func (b *botonHueco) FocusGained()                   { b.enfocado = true; b.Refresh() }
func (b *botonHueco) FocusLost()                     { b.enfocado = false; b.Refresh() }
func (b *botonHueco) TypedRune(rune)                 {}
func (b *botonHueco) TypedKey(e *fyne.KeyEvent)      { pulsarConTecla(b, e, func() { b.Tapped(nil) }) }
func (b *botonHueco) MouseIn(*desktop.MouseEvent)    { b.encima = true; b.Refresh() }
func (b *botonHueco) MouseMoved(*desktop.MouseEvent) {}
func (b *botonHueco) MouseOut()                      { b.encima = false; b.Refresh() }
func (b *botonHueco) Cursor() desktop.Cursor         { return desktop.PointerCursor }

func (b *botonHueco) AccessibilityLabel() string             { return b.hora }
func (b *botonHueco) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

func (b *botonHueco) MinSize() fyne.Size {
	b.ExtendBaseWidget(b)
	w := ui.MedirTabular(b.hora, tema.TextoSM, ui.Normal)
	alto := tema.TextoSM.Linea + 2*(espacio1+tema.Borde)
	if ui.EsTactil() {
		alto = max(alto, ui.SueloTactilBarra) // `pointer-coarse:min-h-11`
	}
	return fyne.NewSize(w+2*(espacio2p5+tema.Borde), alto)
}

func (b *botonHueco) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &botonHuecoRenderer{
		b:     b,
		caja:  ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD),
		texto: ui.NuevoTextoTabular(b.hora, tema.TextoSM, ui.Normal, tema.Color("text", v)),
		aro:   ui.NuevoAro(tema.RadioMD, ui.SinHueco, v),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.texto.Objetos()...)
	r.Refresh()
	return r
}

type botonHuecoRenderer struct {
	b       *botonHueco
	caja    *ui.Caja
	texto   *ui.TextoTabular
	aro     *ui.Aro
	objetos []fyne.CanvasObject
}

func (r *botonHuecoRenderer) MinSize() fyne.Size           { return r.b.MinSize() }
func (r *botonHuecoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *botonHuecoRenderer) Destroy()                     {}

func (r *botonHuecoRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	r.texto.Colocar(tema.Borde+espacio2p5, (t.Height-tema.TextoSM.Linea)/2, tema.TextoSM)
}

func (r *botonHuecoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, borde := "surface", "border"
	if r.b.encima {
		fondo, borde = "bg-subtle", "border-strong"
	}
	r.caja.Pintar(tema.Color(fondo, v), tema.Color(borde, v))
	r.aro.Pintar(v)
	r.aro.Ver(r.b.enfocado)
	r.texto.Tenir(tema.Color("text", v))
}
