// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package indice

import (
	"path/filepath"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/calendar/event"
	calendarical "git.sytue.eu/sytue/one/apps/nucleo/calendar/ical"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/avisos"
)

// # Los recordatorios con las apps cerradas (HU-06)
//
// En macOS, Windows, iOS y Android Calendar y Notepads dejan sus recordatorios
// programados EN EL SISTEMA, que avisa aunque la app esté cerrada (`local/avisos`). En
// Linux no hay quien los guarde: cada app los avisa con un reloj de su proceso, y
// cerrada no avisa nadie. Por eso el Hub residente lee los mismos recordatorios de sus
// almacenes y los avisa él cuando la app dueña no está abierta (PLAN §5.12).
//
// Son los mismos que programa cada app, con la misma clave, el mismo texto y el mismo
// enlace: los de Calendar salen de los VALARM de cada evento, en cada repetición de los
// próximos 30 días; los de Notepads, el próximo de cada nota.
//
// Cada aviso se forma con `avisos.DeEvento` y `avisos.DeNota`, las mismas que usan las
// apps; lo que cambia es de dónde salen las ocurrencias (aquí, de leer el almacén).

// Avisos son los recordatorios de los próximos `avisos.Ventana` de Calendar y Notepads,
// por origen (`calendar`, `notes`), y las fuentes que no se han podido leer.
func Avisos(datos string, ahora time.Time) (map[string][]avisos.Aviso, map[string]error) {
	out := map[string][]avisos.Aviso{}
	fallos := map[string]error{}
	if filas, err := almacen.Leer(filepath.Join(datos, "Datos", "calendar")); err != nil {
		fallos["calendar"] = err
	} else {
		out["calendar"] = avisosDeCalendario(leerCalendario(filas), ahora)
	}
	if filas, err := almacen.Leer(filepath.Join(datos, "Datos", "notes")); err != nil {
		fallos["notes"] = err
	} else {
		out["notes"] = avisosDeNotas(leerNotas(filas))
	}
	return out, fallos
}

// avisosDeCalendario son los recordatorios de todas las ocurrencias de la ventana (con
// una semana más por delante, para el que avisa días antes), como `cmd-local/calendar`.
func avisosDeCalendario(c calendarioLeido, ahora time.Time) []avisos.Aviso {
	hasta := ahora.Add(avisos.Ventana)
	minutos := map[string][]int{} // por objeto: leer el VCALENDAR cuesta, una vez
	var out []avisos.Aviso
	for _, i := range event.ExpandEvents(c.eventos, ahora, hasta.Add(7*24*time.Hour)) {
		obj := i.Event.ObjectID.String()
		ms, ok := minutos[obj]
		if !ok {
			ms = calendarical.Recordatorios(c.datos[obj])
			minutos[obj] = ms
		}
		for _, m := range ms {
			out = append(out, avisoDeOcurrencia(i, m))
		}
	}
	return out
}

func avisoDeOcurrencia(i event.Instance, minutos int) avisos.Aviso {
	return avisos.DeEvento(i.Event.ID.String(), i.Event.Summary, i.Event.Location, i.Start, i.Event.AllDay, minutos)
}

// avisosDeNotas es el próximo de cada nota, como `cmd-local/notes`.
func avisosDeNotas(n notasLeidas) []avisos.Aviso {
	out := make([]avisos.Aviso, 0, len(n.recordatorios))
	for _, r := range n.recordatorios {
		out = append(out, avisos.DeNota(r.NoteID, n.notas[r.NoteID].Title, r.DueAt, r.AllDay))
	}
	return out
}
