// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Recurrencia iCalendar (RFC 5545 3.3.10 RECUR / 3.8.5 RRULE,RDATE,EXDATE).
//
// Clean-room: implementacion original del parseo y la expansion de reglas de
// recurrencia desde la especificacion publica RFC 5545. Soporta el subconjunto
// de uso comun (FREQ, INTERVAL, COUNT, UNTIL, BYMONTH, BYMONTHDAY, BYDAY, WKST)
// suficiente para calendarios personales y de equipo, con una cota dura de
// instancias para acotar reglas infinitas.
package domain

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// maxExpandInstances acota la expansion de cualquier serie (defensa frente a
// reglas infinitas/abusivas). Una ventana de consulta razonable nunca lo alcanza.
const maxExpandInstances = 10000

// Frequency es la FREQ de una RRULE.
type Frequency string

const (
	FreqDaily   Frequency = "DAILY"
	FreqWeekly  Frequency = "WEEKLY"
	FreqMonthly Frequency = "MONTHLY"
	FreqYearly  Frequency = "YEARLY"
)

// weekdayOffset es un BYDAY con desplazamiento ordinal opcional (p. ej. 2MO,
// -1SU). Offset==0 significa "todas las ocurrencias del dia".
type weekdayOffset struct {
	weekday time.Weekday
	offset  int
}

// RRule es una regla de recurrencia parseada (RFC 5545 RECUR).
type RRule struct {
	Freq       Frequency
	Interval   int
	Count      int        // 0 = sin limite por conteo
	Until      *time.Time // nil = sin limite por fecha (UTC)
	ByMonth    []int      // 1..12
	ByMonthDay []int      // 1..31 o -31..-1
	ByDay      []weekdayOffset
	WeekStart  time.Weekday
	raw        string
}

// String devuelve la representacion canonica RRULE (sin el prefijo "RRULE:").
func (r RRule) String() string { return r.raw }

// ParseRRule parsea una linea RRULE de iCalendar. Acepta con o sin el prefijo
// "RRULE:". Valida FREQ obligatoria y los rangos de los componentes BY*.
//
//nolint:gocognit,gocyclo // parser de RRULE (RFC 5545 §3.3.10): un case por parámetro de la gramática.
func ParseRRule(s string) (RRule, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return RRule{}, ErrInvalidRRULE
	}
	if i := strings.IndexByte(raw, ':'); i >= 0 && strings.EqualFold(raw[:i], "RRULE") {
		raw = raw[i+1:]
	}

	r := RRule{Interval: 1, WeekStart: time.Monday, raw: "RRULE:" + raw}
	hasFreq := false

	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return RRule{}, fmt.Errorf("%w: componente %q", ErrInvalidRRULE, part)
		}
		key, val := strings.ToUpper(strings.TrimSpace(kv[0])), strings.TrimSpace(kv[1])

		switch key {
		case "FREQ":
			f := Frequency(strings.ToUpper(val))
			switch f {
			case FreqDaily, FreqWeekly, FreqMonthly, FreqYearly:
				r.Freq = f
				hasFreq = true
			default:
				return RRule{}, fmt.Errorf("%w: FREQ %q no soportada", ErrInvalidRRULE, val)
			}
		case "INTERVAL":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				return RRule{}, fmt.Errorf("%w: INTERVAL %q", ErrInvalidRRULE, val)
			}
			r.Interval = n
		case "COUNT":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				return RRule{}, fmt.Errorf("%w: COUNT %q", ErrInvalidRRULE, val)
			}
			r.Count = n
		case "UNTIL":
			t, err := parseICalTime(val)
			if err != nil {
				return RRule{}, fmt.Errorf("%w: UNTIL %q", ErrInvalidRRULE, val)
			}
			u := t.UTC()
			r.Until = &u
		case "BYMONTH":
			months, err := parseIntList(val, 1, 12, false)
			if err != nil {
				return RRule{}, fmt.Errorf("%w: BYMONTH %q", ErrInvalidRRULE, val)
			}
			r.ByMonth = months
		case "BYMONTHDAY":
			days, err := parseIntList(val, 1, 31, true)
			if err != nil {
				return RRule{}, fmt.Errorf("%w: BYMONTHDAY %q", ErrInvalidRRULE, val)
			}
			r.ByMonthDay = days
		case "BYDAY":
			wds, err := parseByDay(val)
			if err != nil {
				return RRule{}, fmt.Errorf("%w: BYDAY %q", ErrInvalidRRULE, val)
			}
			r.ByDay = wds
		case "WKST":
			wd, ok := parseWeekday(val)
			if !ok {
				return RRule{}, fmt.Errorf("%w: WKST %q", ErrInvalidRRULE, val)
			}
			r.WeekStart = wd
		default:
			// Componentes no soportados (BYHOUR, BYSETPOS, ...) se ignoran de
			// forma conservadora: la expansion sigue siendo un superconjunto
			// seguro para free/busy.
		}
	}
	if !hasFreq {
		return RRule{}, fmt.Errorf("%w: falta FREQ", ErrInvalidRRULE)
	}
	if r.Count > 0 && r.Until != nil {
		return RRule{}, fmt.Errorf("%w: COUNT y UNTIL son mutuamente excluyentes", ErrInvalidRRULE)
	}
	return r, nil
}

// Expand genera los DTSTART de cada instancia de la serie cuyo inicio cae en
// [windowStart, windowEnd). `start` es el DTSTART del evento maestro (UTC).
// rdate/exdate son fechas adicionales/excluidas (RFC 5545 3.8.5.2/.1).
//
// Devuelve las fechas de inicio ordenadas. Aplica la cota maxExpandInstances.
//
//nolint:gocognit // expansión de recurrencias: la casuística FREQ/BYDAY/COUNT/UNTIL es inherente a la RFC 5545.
func (r RRule) Expand(start time.Time, rdate, exdate []time.Time, windowStart, windowEnd time.Time) []time.Time {
	// start conserva su ZONA (la del evento vía DTSTART;TZID): las ocurrencias se
	// generan a la HORA-DE-PARED local en esa zona (time.Date en loc) y solo se
	// normalizan a UTC al emitirlas (add). Así la hora local se preserva a ambos
	// lados de un cambio DST y el instante UTC se desplaza ±1 h. Un start en UTC
	// (all-day/flotante y los tests) reproduce el comportamiento naíf anterior.
	excluded := make(map[int64]struct{}, len(exdate))
	for _, ex := range exdate {
		excluded[ex.UTC().Unix()] = struct{}{}
	}

	seen := make(map[int64]struct{})
	var out []time.Time
	add := func(t time.Time) {
		t = t.UTC()
		key := t.Unix()
		if _, dup := seen[key]; dup {
			return
		}
		if _, ex := excluded[key]; ex {
			return
		}
		seen[key] = struct{}{}
		if !t.Before(windowStart) && t.Before(windowEnd) {
			out = append(out, t)
		}
	}

	count := 0
	for cur := start; ; {
		if r.Until != nil && cur.After(*r.Until) {
			break
		}
		// Genera los candidatos del periodo actual (dia/semana/mes/anyo) que
		// satisfagan los filtros BY*.
		for _, inst := range r.instancesIn(cur, start) {
			if inst.Before(start) {
				continue
			}
			if r.Until != nil && inst.After(*r.Until) {
				continue
			}
			count++
			if count > maxExpandInstances {
				goto done
			}
			add(inst)
			if r.Count > 0 && count >= r.Count {
				goto done
			}
		}
		next := r.advance(cur)
		if !next.After(cur) { // proteccion frente a no-avance
			break
		}
		cur = next
		if cur.After(windowEnd) && r.Count == 0 {
			break // sin COUNT, no hace falta seguir mas alla de la ventana
		}
	}

done:
	// Fechas explicitas RDATE dentro de la ventana (no cuentan para COUNT).
	for _, rd := range rdate {
		add(rd.UTC())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

// instancesIn devuelve los inicios candidatos del periodo que contiene `cur`,
// aplicando BYMONTH/BYMONTHDAY/BYDAY. Conserva la hora del DTSTART.
func (r RRule) instancesIn(cur, start time.Time) []time.Time {
	hh, mm, ss := start.Clock()
	ns := start.Nanosecond()
	loc := start.Location() // zona del evento: fija la hora-de-pared, el offset lo pone time.Date

	atClock := func(y int, mo time.Month, d int) time.Time {
		return time.Date(y, mo, d, hh, mm, ss, ns, loc)
	}

	switch r.Freq {
	case FreqDaily:
		t := atClock(cur.Year(), cur.Month(), cur.Day())
		if r.matchesFilters(t) {
			return []time.Time{t}
		}
		return nil

	case FreqWeekly:
		// Semana que contiene a cur, comenzando en WeekStart.
		weekStart := startOfWeek(cur, r.WeekStart)
		var res []time.Time
		days := r.ByDay
		if len(days) == 0 {
			days = []weekdayOffset{{weekday: start.Weekday()}}
		}
		for _, wd := range days {
			offset := (int(wd.weekday) - int(weekStart.Weekday()) + 7) % 7
			d := weekStart.AddDate(0, 0, offset)
			t := atClock(d.Year(), d.Month(), d.Day())
			if r.byMonthOK(t) {
				res = append(res, t)
			}
		}
		return res

	case FreqMonthly:
		return r.monthlyInstances(cur, atClock, start)

	case FreqYearly:
		// Para YEARLY, BYMONTH restringe el/los mes(es); por defecto el mes del
		// DTSTART. Dentro de cada mes se aplican BYMONTHDAY/BYDAY.
		months := r.ByMonth
		if len(months) == 0 {
			months = []int{int(start.Month())}
		}
		var res []time.Time
		for _, m := range months {
			ref := time.Date(cur.Year(), time.Month(m), 1, 0, 0, 0, 0, cur.Location())
			res = append(res, r.monthlyInstances(ref, atClock, start)...)
		}
		return res
	}
	return nil
}

// monthlyInstances expande un mes concreto (el de ref) segun BYMONTHDAY/BYDAY.
func (r RRule) monthlyInstances(ref time.Time, atClock func(int, time.Month, int) time.Time, start time.Time) []time.Time {
	year, month := ref.Year(), ref.Month()
	last := daysInMonth(year, month)

	var res []time.Time
	switch {
	case len(r.ByMonthDay) > 0:
		for _, md := range r.ByMonthDay {
			d := md
			if d < 0 {
				d = last + d + 1
			}
			if d < 1 || d > last {
				continue
			}
			t := atClock(year, month, d)
			if r.byMonthOK(t) {
				res = append(res, t)
			}
		}
	case len(r.ByDay) > 0:
		for _, wd := range r.ByDay {
			for _, d := range monthDaysForWeekday(year, month, wd) {
				t := atClock(year, month, d)
				if r.byMonthOK(t) {
					res = append(res, t)
				}
			}
		}
	default:
		// Sin BYDAY/BYMONTHDAY: mismo dia-del-mes que el DTSTART.
		d := start.Day()
		if d <= last {
			t := atClock(year, month, d)
			if r.byMonthOK(t) {
				res = append(res, t)
			}
		}
	}
	sort.Slice(res, func(i, j int) bool { return res[i].Before(res[j]) })
	return res
}

// matchesFilters aplica BYMONTH/BYMONTHDAY/BYDAY a un instante (uso DAILY).
func (r RRule) matchesFilters(t time.Time) bool {
	if !r.byMonthOK(t) {
		return false
	}
	if len(r.ByMonthDay) > 0 && !intInList(t.Day(), r.ByMonthDay, daysInMonth(t.Year(), t.Month())) {
		return false
	}
	if len(r.ByDay) > 0 {
		ok := false
		for _, wd := range r.ByDay {
			if wd.weekday == t.Weekday() && wd.offset == 0 {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func (r RRule) byMonthOK(t time.Time) bool {
	if len(r.ByMonth) == 0 {
		return true
	}
	for _, m := range r.ByMonth {
		if time.Month(m) == t.Month() {
			return true
		}
	}
	return false
}

// advance mueve `cur` al inicio del siguiente periodo segun FREQ e INTERVAL.
func (r RRule) advance(cur time.Time) time.Time {
	n := r.Interval
	if n < 1 {
		n = 1
	}
	switch r.Freq {
	case FreqDaily:
		return cur.AddDate(0, 0, n)
	case FreqWeekly:
		return cur.AddDate(0, 0, 7*n)
	case FreqMonthly:
		return firstOfMonth(cur).AddDate(0, n, 0)
	case FreqYearly:
		return time.Date(cur.Year()+n, cur.Month(), 1, 0, 0, 0, 0, cur.Location())
	}
	return cur.AddDate(0, 0, 1)
}

// --- Helpers de calendario ---------------------------------------------------

func startOfWeek(t time.Time, wkst time.Weekday) time.Time {
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	offset := (int(day.Weekday()) - int(wkst) + 7) % 7
	return day.AddDate(0, 0, -offset)
}

func firstOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// monthDaysForWeekday devuelve los dias del mes que cumplen un BYDAY (con o sin
// ordinal). Ordinal positivo: la n-esima ocurrencia; negativo: desde el final.
func monthDaysForWeekday(year int, month time.Month, wd weekdayOffset) []int {
	last := daysInMonth(year, month)
	var matches []int
	for d := 1; d <= last; d++ {
		if time.Date(year, month, d, 0, 0, 0, 0, time.UTC).Weekday() == wd.weekday {
			matches = append(matches, d)
		}
	}
	if wd.offset == 0 {
		return matches
	}
	idx := wd.offset
	if idx > 0 {
		if idx <= len(matches) {
			return []int{matches[idx-1]}
		}
		return nil
	}
	// offset negativo
	if -idx <= len(matches) {
		return []int{matches[len(matches)+idx]}
	}
	return nil
}

func intInList(day int, list []int, last int) bool {
	for _, v := range list {
		d := v
		if d < 0 {
			d = last + d + 1
		}
		if d == day {
			return true
		}
	}
	return false
}

// --- Parseo de componentes ---------------------------------------------------

func parseIntList(s string, lo, hi int, allowNeg bool) ([]int, error) {
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return nil, err
		}
		mag := n
		if mag < 0 {
			if !allowNeg {
				return nil, fmt.Errorf("valor negativo no permitido: %d", n)
			}
			mag = -mag
		}
		if mag < lo || mag > hi {
			return nil, fmt.Errorf("valor fuera de rango: %d", n)
		}
		out = append(out, n)
	}
	return out, nil
}

var weekdayCodes = map[string]time.Weekday{
	"SU": time.Sunday, "MO": time.Monday, "TU": time.Tuesday, "WE": time.Wednesday,
	"TH": time.Thursday, "FR": time.Friday, "SA": time.Saturday,
}

func parseWeekday(s string) (time.Weekday, bool) {
	wd, ok := weekdayCodes[strings.ToUpper(strings.TrimSpace(s))]
	return wd, ok
}

// parseByDay parsea una lista BYDAY: "MO,WE,FR" o "2MO,-1SU".
func parseByDay(s string) ([]weekdayOffset, error) {
	parts := strings.Split(s, ",")
	out := make([]weekdayOffset, 0, len(parts))
	for _, p := range parts {
		p = strings.ToUpper(strings.TrimSpace(p))
		if len(p) < 2 {
			return nil, fmt.Errorf("BYDAY invalido: %q", p)
		}
		code := p[len(p)-2:]
		wd, ok := weekdayCodes[code]
		if !ok {
			return nil, fmt.Errorf("dia de la semana invalido: %q", p)
		}
		offset := 0
		if prefix := p[:len(p)-2]; prefix != "" {
			n, err := strconv.Atoi(prefix)
			if err != nil {
				return nil, fmt.Errorf("ordinal BYDAY invalido: %q", p)
			}
			offset = n
		}
		out = append(out, weekdayOffset{weekday: wd, offset: offset})
	}
	return out, nil
}

// parseICalTime parsea un DATE o DATE-TIME de iCalendar (RFC 5545 3.3.4/3.3.5)
// en sus formas basicas: "YYYYMMDD", "YYYYMMDDTHHMMSS" (local/flotante) y
// "YYYYMMDDTHHMMSSZ" (UTC). Devuelve UTC.
func parseICalTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	switch {
	case len(s) == 8: // DATE
		return time.Parse("20060102", s)
	case len(s) == 15: // DATE-TIME flotante/local
		return time.Parse("20060102T150405", s)
	case len(s) == 16 && strings.HasSuffix(s, "Z"): // DATE-TIME UTC
		return time.Parse("20060102T150405Z", s)
	default:
		return time.Time{}, fmt.Errorf("formato de fecha/hora iCalendar invalido: %q", s)
	}
}

// RecurrenceEnd calcula el ultimo instante de FIN alcanzable por la serie, o nil
// si es infinita (sin COUNT/UNTIL). Sirve para acotar consultas por rango.
func (r RRule) RecurrenceEnd(start time.Time, duration time.Duration) *time.Time {
	if r.Until != nil {
		end := r.Until.Add(duration)
		return &end
	}
	if r.Count > 0 {
		// Ventana amplia: COUNT siempre termina; expandimos hasta agotarlo.
		far := start.AddDate(100, 0, 0)
		insts := r.Expand(start, nil, nil, start, far)
		if len(insts) == 0 {
			e := start.Add(duration)
			return &e
		}
		end := insts[len(insts)-1].Add(duration)
		return &end
	}
	return nil // infinita
}
