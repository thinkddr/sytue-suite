// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Asistente de programacion («buscar hueco»): a partir de los periodos OCUPADOS
// (free/busy) de uno o varios participantes, deriva los HUECOS libres comunes de
// duracion suficiente dentro de una ventana y una franja laboral, respetando la
// zona horaria (DST correcto). Reusa el free/busy existente, que ya solo expone
// ocupado/libre (nunca detalles de eventos ajenos); aqui solo intersecta.
//
// Modelo: los huecos comunes libres = complemento de la UNION de los ocupados de
// todos los participantes, restringido a la franja laboral de cada dia. La union
// se calcula en tiempo ABSOLUTO (UTC); solo los limites de la franja laboral se
// resuelven en la zona de la consulta (para que «09:00-17:00» sea la hora-de-
// pared local correcta a ambos lados de un cambio de hora).
package domain

import (
	"sort"
	"time"
)

// FreeSlot es un intervalo LIBRE comun candidato para agendar (instantes UTC).
type FreeSlot struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// WorkingHours acota las sugerencias a una franja diaria [StartMin, EndMin)
// (minutos desde la medianoche LOCAL en la zona de la consulta) y, opcionalmente,
// a solo dias laborables (lun-vie). Una franja invalida (fuera de [0,1440] o
// EndMin <= StartMin) se interpreta como dia completo.
type WorkingHours struct {
	StartMin     int
	EndMin       int
	WeekdaysOnly bool
}

// maxSlotDays acota la iteracion por dia: la ventana ya viene limitada por la
// config (MaxFreeBusyWindow), esto es una segunda barrera dura.
const maxSlotDays = 370

// SuggestFreeSlots deriva los huecos LIBRES comunes de duracion >= dur dentro de
// [from, to), restringidos a la franja laboral wh interpretada en loc (DST
// correcto). busy son los periodos OCUPADOS de TODOS los participantes juntos (su
// union se calcula aqui). Devuelve intervalos maximales ordenados por inicio,
// como mucho `limit`. Si loc es nil se usa UTC.
func SuggestFreeSlots(busy []FreeBusyPeriod, from, to time.Time, loc *time.Location, wh WorkingHours, dur time.Duration, limit int) []FreeSlot {
	if loc == nil {
		loc = time.UTC
	}
	from, to = from.UTC(), to.UTC()
	if dur <= 0 || !to.After(from) {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	merged := mergeBusy(busy)

	var out []FreeSlot
	// Itera el dia natural en loc desde el dia de `from` hasta cubrir `to`.
	y, m, d := from.In(loc).Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, loc)
	for i := 0; i < maxSlotDays && day.Before(to); i, day = i+1, day.AddDate(0, 0, 1) {
		if wh.WeekdaysOnly {
			if wd := day.Weekday(); wd == time.Saturday || wd == time.Sunday {
				continue
			}
		}
		ws, we := workWindow(day, wh, loc)
		// Recorta la franja laboral a [from, to] en tiempo absoluto.
		ws, we = clampAbs(ws, we, from, to)
		if !we.After(ws) {
			continue
		}
		for _, slot := range subtractBusy(ws, we, merged) {
			if slot.End.Sub(slot.Start) < dur {
				continue
			}
			out = append(out, slot)
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

type interval struct{ start, end time.Time }

// workWindow devuelve la franja laboral [inicio, fin) de un dia natural (en loc),
// como instantes absolutos. time.Date resuelve la hora-de-pared local al offset
// correcto del dia (DST). Una franja invalida => dia completo [00:00, 24:00) local.
func workWindow(day time.Time, wh WorkingHours, loc *time.Location) (time.Time, time.Time) {
	y, mo, d := day.Date()
	if wh.EndMin <= wh.StartMin || wh.StartMin < 0 || wh.EndMin > 24*60 {
		start := time.Date(y, mo, d, 0, 0, 0, 0, loc)
		return start.UTC(), start.AddDate(0, 0, 1).UTC()
	}
	// hour=0 con minutos=StartMin/EndMin: time.Date normaliza (540 min => 09:00).
	start := time.Date(y, mo, d, 0, wh.StartMin, 0, 0, loc)
	end := time.Date(y, mo, d, 0, wh.EndMin, 0, 0, loc)
	return start.UTC(), end.UTC()
}

// mergeBusy fusiona los periodos ocupados (de posiblemente varios participantes)
// en intervalos [start, end) UTC no solapados, ordenados por inicio.
func mergeBusy(busy []FreeBusyPeriod) []interval {
	if len(busy) == 0 {
		return nil
	}
	ivs := make([]interval, 0, len(busy))
	for _, b := range busy {
		if b.End.After(b.Start) {
			ivs = append(ivs, interval{start: b.Start.UTC(), end: b.End.UTC()})
		}
	}
	if len(ivs) == 0 {
		return nil
	}
	sort.Slice(ivs, func(i, j int) bool { return ivs[i].start.Before(ivs[j].start) })
	out := make([]interval, 0, len(ivs))
	out = append(out, ivs[0])
	for _, iv := range ivs[1:] {
		last := &out[len(out)-1]
		if !iv.start.After(last.end) { // solapa o contiguo => fusiona
			if iv.end.After(last.end) {
				last.end = iv.end
			}
			continue
		}
		out = append(out, iv)
	}
	return out
}

// subtractBusy resta los intervalos ocupados (ordenados y no solapados) de la
// ventana [ws, we), devolviendo los tramos libres. No filtra por duracion.
func subtractBusy(ws, we time.Time, busy []interval) []FreeSlot {
	var out []FreeSlot
	cursor := ws
	for _, b := range busy {
		if !b.start.Before(we) {
			break // el resto de ocupados empieza tras la ventana
		}
		if !b.end.After(cursor) {
			continue // ocupado enteramente antes del cursor
		}
		if b.start.After(cursor) {
			out = append(out, FreeSlot{Start: cursor, End: b.start})
		}
		cursor = b.end
		if !cursor.Before(we) {
			return out
		}
	}
	if cursor.Before(we) {
		out = append(out, FreeSlot{Start: cursor, End: we})
	}
	return out
}

func clampAbs(s, e, lo, hi time.Time) (time.Time, time.Time) {
	if s.Before(lo) {
		s = lo
	}
	if e.After(hi) {
		e = hi
	}
	return s, e
}
