// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// El bloque «Recordatorios» de EventModal.tsx, el último del formulario:
//
//	<FieldGroup label="Recordatorios">
//	  <ul class="mb-2 flex flex-col gap-2">  una ReminderRow por recordatorio
//	  (o <p class="text-text-subtle mb-2 text-sm">Sin recordatorios</p>)
//	  <Button variant="secondary" size="sm" leadingIcon={plus 16}>Añadir recordatorio</Button>
//
// Cada fila es el selector con los diez de siempre y «Personalizado…» (`flex-1`), que abre
// la cantidad (`w-20`) y la unidad (a su medida), y el aspa de quitar (`h-8 w-8`, rojo al
// pasar); «Añadir» ocupa el ancho entero, que es lo que hace el `flex-col` del FieldGroup
// con un botón. Medido en el frontal con la fila tal y como la deja el PR que la arregla
// en la web (las clases iban al `<select>` y no a su envoltorio): 481 el selector de un
// preset; 275 + 70 + 122 el de un personalizado, y 28 el aspa, en un diálogo de 560. Viajan
// al servicio como `reminders`, en minutos antes del inicio; los VALARM que la web
// tampoco edita (correo, hora fija) no se enseñan y el servicio los conserva.

// PresetsRecordatorio es `REMINDER_PRESETS` de lib/ical.ts: minutos antes del inicio.
var PresetsRecordatorio = []int{0, 5, 10, 15, 30, 60, 120, 1440, 2880, 10080}

// Las unidades del «Personalizado…», en el orden de sus `<option>`: `UNIT_FACTOR`.
var (
	factoresUnidad = []int{1, 60, 1440}
	clavesUnidad   = []string{"event.unitMinutes", "event.unitHours", "event.unitDays"}
)

// minutosCustom es lo que pone la web al pasar un preset a «Personalizado…».
const minutosCustom = 45

// EtiquetaRecordatorio es `reminderLabel`: «En el momento del evento», «10 minutos
// antes», «1 día antes».
func (p *Pantalla) EtiquetaRecordatorio(minutos int) string {
	switch {
	case minutos == 0:
		return p.t("reminder.atTime", nil)
	case minutos%10080 == 0:
		return p.t("reminder.weeks", i18n.Vars{"count": minutos / 10080})
	case minutos%1440 == 0:
		return p.t("reminder.days", i18n.Vars{"count": minutos / 1440})
	case minutos%60 == 0:
		return p.t("reminder.hours", i18n.Vars{"count": minutos / 60})
	}
	return p.t("reminder.minutes", i18n.Vars{"count": minutos})
}

// Descomponer es `decompose`: la cantidad y la unidad (índice de factoresUnidad).
func Descomponer(m int) (cantidad, unidad int) {
	switch {
	case m > 0 && m%1440 == 0:
		return m / 1440, 2
	case m > 0 && m%60 == 0:
		return m / 60, 1
	}
	return m, 0
}

// controlesRecordatorio son los controles de una ReminderRow. Se crean una vez por fila y
// sobreviven a `rehacer`, para que teclear la cantidad no pierda el foco.
type controlesRecordatorio struct {
	eleccion, unidad *ui.Selector
	cantidad         *ui.Entrada
	quitar           *ui.BotonIcono
}

// anchoSelectorNatural es lo que mide un `<Select>` md sin `w-*` en una fila flex: su
// opción más ancha más `pl-3 pr-9` y el borde.
func anchoSelectorNatural(opciones []string) float32 {
	var ancho float32
	for _, o := range opciones {
		ancho = max(ancho, ui.Medir(o, tema.TextoBase, ui.Normal).Width)
	}
	return ancho + 2*tema.Borde + espacio3 + tema.Espacio(9)
}

// recordatorios son los minutos de la edición; nil si todavía no se saben.
func (f *formulario) recordatorios() []int {
	if f.e.Recordatorios == nil {
		return nil
	}
	return *f.e.Recordatorios
}

// construirRecordatorios rehace los controles de las filas desde los minutos: al
// abrir, al leerlos del servicio, al añadir y al quitar (las filas van por índice, como
// el `key={i}` de la web).
func (f *formulario) construirRecordatorios() {
	p := f.p
	f.filasRec = nil
	for i, m := range f.recordatorios() {
		opciones := make([]string, 0, len(PresetsRecordatorio)+1)
		for _, pm := range PresetsRecordatorio {
			opciones = append(opciones, p.EtiquetaRecordatorio(pm))
		}
		opciones = append(opciones, p.t("event.reminderCustom", nil))
		fr := &controlesRecordatorio{}
		fr.eleccion = ui.NuevoSelector(opciones, func(j int) {
			actual := f.recordatorios()[i]
			if j == len(PresetsRecordatorio) { // «Personalizado…»
				if slices.Contains(PresetsRecordatorio, actual) {
					actual = minutosCustom
				}
				f.ponerRecordatorio(i, actual)
				return
			}
			f.ponerRecordatorio(i, PresetsRecordatorio[j])
		})
		fr.eleccion.Elegida = len(PresetsRecordatorio)
		if j := slices.Index(PresetsRecordatorio, m); j >= 0 {
			fr.eleccion.Elegida = j
		}

		cantidad, unidad := Descomponer(m)
		fr.cantidad = ui.NuevaEntrada("")
		fr.cantidad.Ancho = tema.Espacio(20) // `w-20`
		fr.cantidad.PonerTexto(strconv.Itoa(cantidad))
		fr.cantidad.AlCambiar(func(s string) {
			n, err := strconv.Atoi(strings.TrimSpace(s))
			if err != nil || n < 0 {
				n = 0 // `Math.max(0, Number(v) || 0)`
			}
			_, u := Descomponer(f.recordatorios()[i])
			f.ponerRecordatorio(i, n*factoresUnidad[u])
		})
		unidades := make([]string, len(clavesUnidad))
		for k, c := range clavesUnidad {
			unidades[k] = p.t(c, nil)
		}
		fr.unidad = ui.NuevoSelector(unidades, func(k int) {
			c, _ := Descomponer(f.recordatorios()[i])
			f.ponerRecordatorio(i, c*factoresUnidad[k])
		})
		fr.unidad.Ancho = anchoSelectorNatural(unidades)
		fr.unidad.Elegida = unidad

		fr.quitar = ui.NuevoBotonIcono(iconos.Cerrar, p.t("event.reminderRemove", i18n.Vars{"index": i + 1}), func() {
			l := slices.Delete(slices.Clone(f.recordatorios()), i, i+1)
			f.e.Recordatorios = &l
			f.construirRecordatorios()
			f.rehacer()
		})
		// `grid h-8 w-8 rounded-md text-text-muted hover:bg-danger-bg hover:text-danger-text`.
		fr.quitar.Lado, fr.quitar.TamIcono, fr.quitar.Radio = ui.LadoIconoLG, iconos.TallaSM, tema.Forma("radius-control")
		fr.quitar.TrazoEncima, fr.quitar.FondoEncima = "danger-text", "danger-bg"
		if ui.EsTactil() {
			fr.quitar.Lado = ui.SueloTactilBarra // `pointer-coarse:h-11 w-11`
		}
		f.filasRec = append(f.filasRec, fr)
	}
	if f.e.SoloLectura {
		f.recordatoriosInertes()
	}
}

// ponerRecordatorio es `setReminderAt`. La fila solo se rehace si pasa de preset a
// personalizado o al revés: es lo que hace la web, que al teclear «5» en minutos enseña
// ya «5 minutos antes» en el selector.
func (f *formulario) ponerRecordatorio(i, m int) {
	l := slices.Clone(f.recordatorios())
	antes := slices.Contains(PresetsRecordatorio, l[i])
	l[i] = m
	f.e.Recordatorios = &l
	fr := f.filasRec[i]
	switch ahora := slices.Contains(PresetsRecordatorio, m); {
	case antes != ahora:
		f.construirRecordatorios()
		f.rehacer()
	case ahora:
		fr.eleccion.Elegida = slices.Index(PresetsRecordatorio, m)
		fr.eleccion.Refresh()
	default:
		// La cantidad y la unidad son `decompose(minutes)`, como el `value` controlado de
		// la web: 180 minutos pasan a «3» horas.
		c, u := Descomponer(m)
		if fr.unidad.Elegida != u {
			fr.unidad.Elegida = u
			fr.unidad.Refresh()
		}
		if t := strconv.Itoa(c); strings.TrimSpace(fr.cantidad.Texto()) != t {
			fr.cantidad.PonerTexto(t)
		}
	}
}

// anadirRecordatorio es `addReminder`: diez minutos antes.
func (f *formulario) anadirRecordatorio() {
	l := append(slices.Clone(f.recordatorios()), 10)
	f.e.Recordatorios = &l
	f.construirRecordatorios()
	f.rehacer()
}

// recordatoriosInertes apaga el bloque: el `<fieldset disabled>` del modo consulta.
func (f *formulario) recordatoriosInertes() {
	for _, fr := range f.filasRec {
		fr.eleccion.Inerte(true)
		fr.unidad.Inerte(true)
		fr.cantidad.Inerte(true)
		fr.quitar.Inerte = true
	}
	if f.anadirRec != nil {
		f.anadirRec.Inerte = true
	}
}

// bloqueRecordatorios es el FieldGroup entero. Sin minutos que enseñar —se están
// leyendo del servicio, o no se pudieron leer— no se pinta: el guardado los deja como
// estaban.
func (f *formulario) bloqueRecordatorios() fyne.CanvasObject {
	if f.e.Recordatorios == nil {
		return nil
	}
	p := f.p
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var arriba fyne.CanvasObject
	if len(f.filasRec) == 0 {
		arriba = ui.NuevoParrafo(p.t("event.remindersNone", nil), tema.TextoSM, ui.Normal, "text-subtle", v)
	} else {
		filas := make([]fyne.CanvasObject, len(f.filasRec))
		for i, fr := range f.filasRec {
			switch {
			case fr.eleccion.Elegida != len(PresetsRecordatorio):
				filas[i] = container.New(filaElastica{Hueco: espacio2}, fr.eleccion, fr.quitar)
			case f.ventana() < ui.Estrecho:
				// `flex-wrap` con el selector en `max-md:basis-full`: entero arriba, y
				// cantidad, unidad (`max-md:flex-1`) y quitar debajo. En una línea, a
				// «Personalizado» le quedaban 66 px y se leía «Pe…».
				filas[i] = container.New(ui.Columna{Hueco: espacio2}, fr.eleccion,
					container.New(filaElastica{Hueco: espacio2, Elastico: 1}, fr.cantidad, fr.unidad, fr.quitar))
			default:
				filas[i] = container.New(filaElastica{Hueco: espacio2}, fr.eleccion, fr.cantidad, fr.unidad, fr.quitar)
			}
		}
		arriba = container.New(ui.Columna{Hueco: espacio2}, filas...)
	}
	// Entre la lista (o el párrafo) y el botón, su `mb-2` más el `gap-1.5` del FieldGroup.
	return ui.GrupoCampo(p.t("event.reminders", nil), false,
		container.New(ui.Columna{Hueco: espacio2 + tema.Espacio(1.5)}, arriba, f.anadirRec), "", "")
}

// filaElastica es `flex items-center gap-*` con UN hijo en `min-w-0 flex-1` (el de
// índice `Elastico`): se queda con lo que no ocupan los demás, que van a su medida y
// centrados en el alto. Con un índice que no existe (-1) no estira a nadie: es la fila
// `inline-flex items-center` a su medida.
type filaElastica struct {
	Hueco    float32
	Elastico int
}

func (c filaElastica) elastico(objs []fyne.CanvasObject) bool {
	return c.Elastico >= 0 && c.Elastico < len(objs)
}

func (c filaElastica) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := ui.Fila{Hueco: c.Hueco}.MinSize(objs)
	if c.elastico(objs) {
		m.Width -= objs[c.Elastico].MinSize().Width // `min-w-0`
	}
	return m
}

func (c filaElastica) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	resto := c.MinSize(objs).Width
	x := float32(0)
	for i, o := range objs {
		m := o.MinSize()
		if i == c.Elastico {
			m.Width = max(t.Width-resto, 0)
		}
		o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		o.Resize(m)
		x += m.Width + c.Hueco
	}
}
