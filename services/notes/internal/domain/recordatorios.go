// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/teambition/rrule-go"
)

// Reminder es el recordatorio de UNA persona sobre una nota (0014).
type Reminder struct {
	NoteID     uuid.UUID `json:"note_id"`
	NotebookID uuid.UUID `json:"notebook_id"`
	// DueAt es el instante en UTC; TZID, la zona en que se calcula la repetición.
	DueAt  time.Time `json:"due_at"`
	AllDay bool      `json:"all_day"`
	// RRule es una RRULE de RFC 5545 sin DTSTART: el DTSTART es DTStart.
	RRule string `json:"rrule,omitempty"`
	// AfterDays es la repetición «flexible»: N días después de completar la tarea.
	AfterDays int    `json:"after_days,omitempty"`
	TZID      string `json:"tzid"`
	// DTStart es el primer vencimiento, del que cuenta la RRULE; no cambia al avanzar
	// DueAt. Cero: se cuenta desde DueAt.
	DTStart time.Time `json:"-"`
}

// ReminderInput es lo que llega al poner un recordatorio.
type ReminderInput struct {
	DueAt     time.Time `json:"due_at"`
	AllDay    bool      `json:"all_day"`
	RRule     string    `json:"rrule"`
	AfterDays int       `json:"after_days"`
	TZID      string    `json:"tzid"`
}

// Normalize valida un recordatorio: zona que exista, una sola forma de repetir, y una
// RRULE que se pueda calcular y que no repita más de una vez al día —cada hora sería
// llenar la campana de alguien, y nadie lo ha pedido—. Por eso tampoco BYHOUR,
// BYMINUTE ni BYSECOND: la hora es la del vencimiento, y con ellos un FREQ=DAILY
// sonaba cada pocos minutos.
func (in *ReminderInput) Normalize() error {
	in.RRule = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(in.RRule), "RRULE:"))
	in.TZID = strings.TrimSpace(in.TZID)
	if in.TZID == "" {
		in.TZID = "UTC"
	}
	if in.DueAt.IsZero() || len(in.TZID) > 64 || in.AfterDays < 0 || in.AfterDays > 365 ||
		(in.RRule != "" && in.AfterDays > 0) || len(in.RRule) > 200 {
		return ErrInvalidInput
	}
	if _, err := time.LoadLocation(in.TZID); err != nil {
		return ErrInvalidInput
	}
	if in.RRule != "" {
		if strings.ContainsAny(in.RRule, "\r\n") || strings.Contains(strings.ToUpper(in.RRule), "DTSTART") {
			return ErrInvalidInput
		}
		opt, err := rrule.StrToROption(in.RRule)
		if err != nil || opt.Freq > rrule.DAILY || len(opt.Byhour) > 0 || len(opt.Byminute) > 0 || len(opt.Bysecond) > 0 {
			return ErrInvalidInput
		}
	}
	in.DueAt = in.DueAt.UTC().Truncate(time.Minute)
	return nil
}

// Repite dice si el recordatorio vuelve a sonar.
func (r Reminder) Repite() bool { return r.RRule != "" || r.AfterDays > 0 }

// Siguiente es el próximo vencimiento de un recordatorio que repite, estrictamente
// después de `ahora` y del vencimiento actual. false: no repite, o su RRULE se agotó
// (COUNT o UNTIL).
//
// Se calcula en la zona de quien lo puso: «cada lunes a las 9» son las 9 de su lunes
// también al cambiar la hora. La flexible cuenta N días desde HOY en esa zona, a la
// misma hora del reloj que tenía.
//
// «Cada mes» puesto un día que no todos los meses tienen (29, 30 o 31) cae el último
// del mes que no lo tiene, y vuelve a su día en el siguiente: RFC 5545 se SALTA esos
// meses, y quien eligió «cada mes» no se entera de que en cinco no le avisa.
func Siguiente(r Reminder, ahora time.Time) (time.Time, bool) {
	loc, err := time.LoadLocation(r.TZID)
	if err != nil {
		loc = time.UTC
	}
	due := r.DueAt.In(loc)
	switch {
	case r.AfterDays > 0:
		hoy := ahora.In(loc)
		return time.Date(hoy.Year(), hoy.Month(), hoy.Day()+r.AfterDays, due.Hour(), due.Minute(), 0, 0, loc).UTC(), true
	case r.RRule != "":
		opt, err := rrule.StrToROption(r.RRule)
		if err != nil {
			return time.Time{}, false
		}
		inicio := r.DTStart
		if inicio.IsZero() {
			inicio = r.DueAt
		}
		opt.Dtstart = inicio.In(loc)
		if dia := opt.Dtstart.Day(); dia > 28 && opt.Freq == rrule.MONTHLY && len(opt.Bymonthday) == 0 &&
			len(opt.Byweekday) == 0 && len(opt.Bysetpos) == 0 && len(opt.Bymonth) == 0 &&
			len(opt.Byyearday) == 0 && len(opt.Byweekno) == 0 {
			opt.Bymonthday, opt.Bysetpos = []int{dia, -1}, []int{1}
		}
		rr, err := rrule.NewRRule(*opt)
		if err != nil {
			return time.Time{}, false
		}
		desde := due
		if ahora.After(desde) {
			desde = ahora
		}
		sig := rr.After(desde, false)
		return sig.UTC(), !sig.IsZero()
	}
	return time.Time{}, false
}
