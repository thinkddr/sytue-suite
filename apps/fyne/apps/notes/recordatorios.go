// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// Paridad web: el recordatorio de una nota —RecordatorioModal.tsx, `RecordatorioEnFila`
// de NoteCard.tsx y client/recordatorios.ts—. Cuándo (fecha y hora, o el día entero) y
// si repite; la «flexible», N días después de completarla, solo en una tarea.
//
// El aviso llega por el Hub: no es una alarma del sistema y aquí no se promete.

// Repeticiones son las del selector, en el orden de REPETICIONES de la web. «custom» es
// una RRULE que no es de estas: se enseña y se respeta al guardar.
var Repeticiones = []string{"none", "daily", "weekdays", "weekly", "monthly", "flexible"}

var rruleDe = map[string]string{
	"daily":    "FREQ=DAILY",
	"weekdays": "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR",
	"weekly":   "FREQ=WEEKLY",
	"monthly":  "FREQ=MONTHLY",
}

// HoraTodoElDia es la hora a la que avisa uno de todo el día: las 9 de su día.
const HoraTodoElDia = 9

// RepeticionDe es qué repetición es un recordatorio guardado.
func RepeticionDe(r api.NoteReminder) string {
	if r.AfterDays > 0 {
		return "flexible"
	}
	if r.RRule == "" {
		return "none"
	}
	for k, v := range rruleDe {
		if v == r.RRule {
			return k
		}
	}
	return "custom"
}

// EntradaRecordatorio es lo que se manda al servicio; false si no vale (la flexible
// pide de 1 a 365 días). `original` es la RRULE de una «custom», que se conserva.
func EntradaRecordatorio(cuando time.Time, todoElDia bool, rep string, dias int, original string) (api.NoteReminderInput, bool) {
	if cuando.IsZero() || (rep == "flexible" && (dias < 1 || dias > 365)) {
		return api.NoteReminderInput{}, false
	}
	if todoElDia {
		cuando = time.Date(cuando.Year(), cuando.Month(), cuando.Day(), HoraTodoElDia, 0, 0, 0, cuando.Location())
	}
	in := api.NoteReminderInput{DueAt: cuando.UTC(), AllDay: todoElDia, RRule: rruleDe[rep], TZID: sistema.ZonaIANA(cuando.Location())}
	switch rep {
	case "custom":
		in.RRule = original
	case "flexible":
		in.AfterDays = dias
	}
	return in, true
}

// RotuloRecordatorio es el rótulo corto del indicador: el día y, si no es de todo el
// día, la hora (`rotuloRecordatorio` de la web).
func RotuloRecordatorio(loc i18n.Locale, r api.NoteReminder) string {
	d := r.DueAt.In(time.Local)
	dia := i18n.FormatDayMonth(loc, d)
	if r.AllDay {
		return dia
	}
	return dia + ", " + i18n.FormatTimeDigits(loc, d)
}

// indicadorRecordatorio es `RecordatorioEnFila`: la campana, la fecha corta y la marca
// de que repite, en `text-xs`; en rojo si ya pasó. Con `alPulsar`, se pulsa (la tarjeta).
func indicadorRecordatorio(tr *i18n.Translator, r api.NoteReminder, alPulsar func(), v tema.Variante) fyne.CanvasObject {
	tinta := "text-muted"
	if r.DueAt.Before(time.Now()) {
		tinta = "danger-text"
	}
	cuando := RotuloRecordatorio(tr.Locale(), r)
	piezas := []fyne.CanvasObject{
		Icono(iconos.Campana, 12, tinta, v),
		lineaTok(cuando, tema.TextoXS, ui.Normal, tinta, v),
	}
	rotulo := tr.T("reminder.at", i18n.Vars{"when": cuando})
	if r.Repite() {
		piezas = append(piezas, Icono(iconos.Rotar, 11, tinta, v))
		rotulo += " · " + tr.T("reminder.repeats", nil)
	}
	fila := container.New(ui.Fila{Hueco: 1 * 0.25 * tema.Rem}, piezas...)
	if alPulsar == nil {
		return fila
	}
	o := NuevaOpcion(rotulo, fila, alPulsar)
	o.Radio, o.PX = tema.RadioSM, 1*0.25*tema.Rem
	return o
}

// ponerRecordatorios guarda los míos y se los da al lienzo y al árbol.
func (p *Pantalla) ponerRecordatorios(m map[string]api.NoteReminder) {
	p.recordatorios = m
	p.lienzo.Recordatorios, p.arbol.Recordatorios = m, m
}

// cargarRecordatorios pide los míos. Sin ellos la app sigue: solo falta el indicador.
func (p *Pantalla) cargarRecordatorios() {
	if p.svc == nil || !p.hayRed() {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	rs, err := p.svc.ListReminders(ctx)
	if err != nil {
		return
	}
	m := make(map[string]api.NoteReminder, len(rs))
	for _, r := range rs {
		m[r.NoteID] = r
	}
	enHilo(func() { p.ponerRecordatorios(m); p.Repintar() })
}

// guardarRecordatorio pone (con `in`) o quita (nil) mi recordatorio en una nota.
func (p *Pantalla) guardarRecordatorio(nota string, in *api.NoteReminderInput) error {
	ctx, cancelar := context.WithTimeout(context.Background(), p.Plazo)
	defer cancelar()
	var r api.NoteReminder
	var err error
	if in != nil {
		r, err = p.svc.SetReminder(ctx, nota, *in)
	} else {
		err = p.svc.DeleteReminder(ctx, nota)
	}
	enHilo(func() {
		if err != nil {
			p.fallo("reminder.failed", err)
			return
		}
		m := make(map[string]api.NoteReminder, len(p.recordatorios)+1)
		for k, x := range p.recordatorios {
			m[k] = x
		}
		if in != nil {
			m[nota] = r
		} else {
			delete(m, nota)
		}
		p.ponerRecordatorios(m)
		p.Repintar()
	})
	return err
}

// dialogoRecordatorio es RecordatorioModal.tsx: cuándo, «todo el día», repetir y, en la
// flexible, los días. Parte de lo guardado o de mañana a las 9.
func (p *Pantalla) dialogoRecordatorio(n Nota) {
	t := p.tr.T
	actual, tiene := p.recordatorios[n.ID]
	cuando := time.Now().AddDate(0, 0, 1)
	cuando = time.Date(cuando.Year(), cuando.Month(), cuando.Day(), 9, 0, 0, 0, time.Local)
	rep, dias := "none", 1
	if tiene {
		cuando, rep = actual.DueAt.In(time.Local), RepeticionDe(actual)
		dias = max(actual.AfterDays, 1)
	}
	opciones := slices.DeleteFunc(slices.Clone(Repeticiones), func(r string) bool {
		return r == "flexible" && n.Tipo != "todo" && rep != "flexible"
	})
	if rep == "custom" {
		opciones = append(opciones, "custom")
	}
	rotulos := make([]string, len(opciones))
	for i, r := range opciones {
		rotulos[i] = t("reminder.repeat."+r, nil)
	}

	// El ancho de dentro de un diálogo `sm` en esta ventana: en un teléfono no caben los
	// 400 del panel, y el `w-full` de los campos se mide con lo que queda.
	fecha := ui.NuevoCampoFechaHora(ui.CampoMD, cuando)
	// Como el control nativo, a todo el ancho: la hora con un respiro y la fecha el resto.
	fecha.Hora.Ancho += tema.Espacio(2)
	filaFecha := container.New(ui.FilaAuto{Hueco: ui.HuecoFechaHora, Derecha: 2, Partir: true, SaltoFila: ui.HuecoFechaHora, Elastico: 1}, fecha.Fecha, fecha.Hora)
	todoElDia := ui.NuevoConmutador(t("reminder.allDay", nil), nil)
	todoElDia.Encendido = tiene && actual.AllDay
	selector := ui.NuevoSelector(rotulos, nil)
	selector.Elegida = max(slices.Index(opciones, rep), 0)
	selector.Ancho = 0
	entradaDias := ui.NuevaEntrada("1")
	entradaDias.PonerTexto(strconv.Itoa(dias))
	entradaDias.Ancho = 6 * tema.Rem
	grupoDias := grupoCampo(t("reminder.days", nil), entradaDias, p.variante())
	errorFecha := ui.LineaError(t("reminder.invalid", nil))
	errorFecha.Hide()
	ver := func() {
		if todoElDia.Encendido {
			fecha.Hora.Hide()
			filaFecha.Objects = []fyne.CanvasObject{fecha.Fecha}
		} else {
			fecha.Hora.Show()
			filaFecha.Objects = []fyne.CanvasObject{fecha.Fecha, fecha.Hora}
		}
		filaFecha.Refresh()
		if opciones[selector.Elegida] == "flexible" {
			grupoDias.Show()
		} else {
			grupoDias.Hide()
		}
	}
	ver()

	v := p.variante()
	cuerpo := container.New(ui.Columna{Hueco: 4 * 0.25 * tema.Rem},
		grupoCampo(t("reminder.when", nil), container.New(ui.Columna{Hueco: 1 * 0.25 * tema.Rem}, filaFecha, errorFecha), v),
		container.New(ui.Fila{}, todoElDia),
		grupoCampo(t("reminder.repeat", nil), selector, v),
		grupoDias,
		ui.NuevoParrafo(t("reminder.hint", nil), tema.TextoSM, ui.Normal, "text-muted", v),
	)

	var d *ui.Dialogo
	guardar := ui.NuevoBoton(t("common.save", nil), func() {
		var dia time.Time
		var ok bool
		if todoElDia.Encendido {
			var vacia bool
			dia, vacia, ok = ui.LeerFecha(fecha.Fecha.Texto(), time.Local)
			ok = ok && !vacia
		} else {
			dia, ok = fecha.Leer(time.Local)
		}
		nDias, _ := strconv.Atoi(strings.TrimSpace(entradaDias.Texto()))
		in, vale := EntradaRecordatorio(dia, todoElDia.Encendido, opciones[selector.Elegida], nDias, actual.RRule)
		if !ok || !vale {
			errorFecha.Show()
			d.Refresh()
			return
		}
		d.Cerrar()
		lanzar(func() error { return p.guardarRecordatorio(n.ID, &in) })
	}).Con(ui.Primario, ui.TallaMD)
	cancelar := ui.NuevoBoton(t("common.cancel", nil), func() { d.Cerrar() }).Con(ui.Fantasma, ui.TallaMD)
	var quitar fyne.CanvasObject
	if tiene {
		quitar = ui.NuevoBoton(t("reminder.remove", nil), func() {
			d.Cerrar()
			lanzar(func() error { return p.guardarRecordatorio(n.ID, nil) })
		}).Con(ui.Fantasma, ui.TallaMD)
	}
	acciones := sinNil([]fyne.CanvasObject{quitar})
	derecha := len(acciones)
	acciones = append(acciones, cancelar, guardar)
	// «Quitar» a la izquierda en escritorio; en el teléfono, sola arriba y Cancelar |
	// Guardar debajo (`ui.PieAcciones`).
	pie := container.New(ui.PieAcciones{Izquierda: derecha}, acciones...)
	todoElDia.AlCambiar = func(bool) { ver(); d.Refresh() }
	selector.AlElegir = func(int) { ver(); d.Refresh() }
	d = ui.NuevoDialogo(t("reminder.title", nil), cuerpo).Con(ui.DialogoSM, pie)
	d.Foco = fecha.Fecha
	p.mostrar(d)
}
