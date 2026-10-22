// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Disponibilidad (free/busy). Expande las series recurrentes en instancias y
// fusiona los periodos ocupados (RFC 5545 FREEBUSY / RFC 4791 free-busy-query).
package domain

import (
	"sort"
	"time"
)

// Instance es una ocurrencia concreta de un evento (maestro, override o
// expansion de recurrencia) acotada a una ventana temporal.
type Instance struct {
	Event Event     `json:"event"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	// IsOverride indica que la instancia procede de un evento override
	// (RECURRENCE-ID) y no de la expansion de la regla maestra.
	IsOverride bool `json:"is_override,omitempty"`
}

// ExpandEvents expande una lista de eventos (maestros + overrides) en instancias
// concretas que solapan [windowStart, windowEnd). Aplica RRULE/RDATE/EXDATE del
// maestro y sustituye las instancias por su override cuando coincide el
// RECURRENCE-ID (RFC 5545 3.8.4.4). Devuelve las instancias ordenadas por inicio.
func ExpandEvents(events []Event, windowStart, windowEnd time.Time) []Instance {
	// Indexa overrides por UID + RECURRENCE-ID (UTC).
	overrides := map[string]map[int64]Event{}
	for _, e := range events {
		if e.RecurrenceID == nil {
			continue
		}
		m := overrides[e.UID]
		if m == nil {
			m = map[int64]Event{}
			overrides[e.UID] = m
		}
		m[e.RecurrenceID.UTC().Unix()] = e
	}

	var out []Instance
	for _, e := range events {
		if e.RecurrenceID != nil {
			// El override se inserta al expandir su maestro; si solapa la
			// ventana, se incluye explicitamente para cubrir overrides movidos.
			if overlaps(e.DTStart, e.DTEnd, windowStart, windowEnd) {
				out = append(out, Instance{Event: e, Start: e.DTStart, End: e.DTEnd, IsOverride: true})
			}
			continue
		}

		dur := e.Duration()
		if !e.IsRecurring() {
			if overlaps(e.DTStart, e.DTEnd, windowStart, windowEnd) {
				out = append(out, Instance{Event: e, Start: e.DTStart, End: e.DTEnd})
			}
			continue
		}

		// Serie: expande los inicios solapando un margen de duracion para captar
		// instancias que arrancan antes de la ventana pero la solapan.
		starts := expandStarts(e, windowStart.Add(-dur), windowEnd)
		ovr := overrides[e.UID]
		for _, st := range starts {
			if o, ok := ovr[st.UTC().Unix()]; ok {
				// Esta instancia fue redefinida por un override (ya incluido
				// arriba); no la dupliques aqui.
				_ = o
				continue
			}
			end := st.Add(dur)
			if !overlaps(st, end, windowStart, windowEnd) {
				continue
			}
			inst := e
			inst.DTStart, inst.DTEnd = st, end
			out = append(out, Instance{Event: inst, Start: st, End: end})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out
}

// expandStarts devuelve los inicios de las instancias de un evento recurrente.
// Combina la expansion de la RRULE (si existe) con las RDATE, restando EXDATE.
func expandStarts(e Event, windowStart, windowEnd time.Time) []time.Time {
	if e.RRule == "" {
		// Solo RDATE: el maestro mas las fechas explicitas, menos EXDATE.
		excluded := map[int64]struct{}{}
		for _, ex := range e.ExDate {
			excluded[ex.UTC().Unix()] = struct{}{}
		}
		seen := map[int64]struct{}{}
		var out []time.Time
		add := func(t time.Time) {
			t = t.UTC()
			if _, ex := excluded[t.Unix()]; ex {
				return
			}
			if _, dup := seen[t.Unix()]; dup {
				return
			}
			seen[t.Unix()] = struct{}{}
			if !t.Before(windowStart) && t.Before(windowEnd) {
				out = append(out, t)
			}
		}
		add(e.DTStart)
		for _, rd := range e.RDate {
			add(rd)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
		return out
	}
	rule, err := ParseRRule(e.RRule)
	if err != nil {
		// Regla invalida persistida: degrada a evento puntual (no rompe la query).
		if !e.DTStart.Before(windowStart) && e.DTStart.Before(windowEnd) {
			return []time.Time{e.DTStart}
		}
		return nil
	}
	// DTStart en la zona del evento (TZID; UTC/flotante si es all-day): Expand
	// preserva la hora-de-pared local a ambos lados de un cambio DST.
	loc := EventLocation(e.TZID, e.AllDay)
	return rule.Expand(e.DTStart.In(loc), e.RDate, e.ExDate, windowStart, windowEnd)
}

// FreeBusy fusiona las instancias OPAQUE (que cuentan como ocupado) en periodos
// de disponibilidad no solapados dentro de [windowStart, windowEnd). Las
// instancias TENTATIVE se marcan BUSY-TENTATIVE; el resto, BUSY.
func FreeBusy(instances []Instance, windowStart, windowEnd time.Time) []FreeBusyPeriod {
	type seg struct {
		start, end time.Time
		tentative  bool
	}
	segs := make([]seg, 0, len(instances))
	for _, inst := range instances {
		if !inst.Event.CountsAsBusy() {
			continue
		}
		s, en := clamp(inst.Start, inst.End, windowStart, windowEnd)
		if !en.After(s) {
			continue
		}
		segs = append(segs, seg{start: s, end: en, tentative: inst.Event.Status == StatusTentative})
	}
	if len(segs) == 0 {
		return nil
	}
	sort.Slice(segs, func(i, j int) bool { return segs[i].start.Before(segs[j].start) })

	periods := make([]FreeBusyPeriod, 0, len(segs))
	cur := segs[0]
	for _, s := range segs[1:] {
		if !s.start.After(cur.end) { // solapa o es contiguo => fusiona
			if s.end.After(cur.end) {
				cur.end = s.end
			}
			cur.tentative = cur.tentative && s.tentative
			continue
		}
		periods = append(periods, toPeriod(cur.start, cur.end, cur.tentative))
		cur = s
	}
	periods = append(periods, toPeriod(cur.start, cur.end, cur.tentative))
	return periods
}

func toPeriod(start, end time.Time, tentative bool) FreeBusyPeriod {
	t := FBBusy
	if tentative {
		t = FBBusyTentative
	}
	return FreeBusyPeriod{Start: start.UTC(), End: end.UTC(), Type: t}
}

func overlaps(aStart, aEnd, bStart, bEnd time.Time) bool {
	return aStart.Before(bEnd) && bStart.Before(aEnd)
}

func clamp(start, end, lo, hi time.Time) (time.Time, time.Time) {
	if start.Before(lo) {
		start = lo
	}
	if end.After(hi) {
		end = hi
	}
	return start, end
}
