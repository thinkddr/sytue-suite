// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"
	"time"
)

func TestSiguienteRecordatorio(t *testing.T) {
	madrid, _ := time.LoadLocation("Europe/Madrid")
	en := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04", s, madrid)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	casos := []struct {
		nombre      string
		r           Reminder
		ahora, sale string // "" = no hay siguiente
	}{
		{"diaria", Reminder{RRule: "FREQ=DAILY"}, "2026-10-01 09:30", "2026-10-02 09:00"},
		{"antes de vencer, la de hoy no cuenta", Reminder{RRule: "FREQ=DAILY"}, "2026-10-01 08:00", "2026-10-02 09:00"},
		{"tres días tarde salta a la próxima", Reminder{RRule: "FREQ=DAILY"}, "2026-10-04 10:00", "2026-10-05 09:00"},
		// El 2026-10-02 es viernes: lo siguiente laborable es el lunes.
		{"laborables", Reminder{RRule: "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR"}, "2026-10-02 10:00", "2026-10-05 09:00"},
		{"semanal", Reminder{RRule: "FREQ=WEEKLY"}, "2026-10-01 10:00", "2026-10-08 09:00"},
		{"mensual", Reminder{RRule: "FREQ=MONTHLY"}, "2026-10-01 10:00", "2026-11-01 09:00"},
		// El 25 de octubre de 2026 Madrid pasa de UTC+2 a UTC+1: sigue sonando a las 9 de allí.
		{"cambio de hora", Reminder{RRule: "FREQ=DAILY"}, "2026-10-25 09:30", "2026-10-26 09:00"},
		{"flexible: N días desde que se completa", Reminder{AfterDays: 3}, "2026-10-07 18:00", "2026-10-10 09:00"},
		{"agotada", Reminder{RRule: "FREQ=DAILY;COUNT=1"}, "2026-10-01 10:00", ""},
		{"sin repetición", Reminder{}, "2026-10-01 10:00", ""},
	}
	for _, c := range casos {
		c.r.TZID, c.r.DueAt = "Europe/Madrid", en("2026-10-01 09:00").UTC()
		if c.nombre == "cambio de hora" {
			c.r.DueAt = en("2026-10-24 09:00").UTC()
		}
		got, ok := Siguiente(c.r, en(c.ahora))
		if c.sale == "" {
			if ok {
				t.Errorf("%s: no debía haber siguiente y sale %v", c.nombre, got.In(madrid))
			}
			continue
		}
		if !ok || !got.Equal(en(c.sale)) {
			t.Errorf("%s: sale %v (%v), quiero %s", c.nombre, got.In(madrid), ok, c.sale)
		}
	}
}

// «Cada mes» el 31: el último día del mes que no lo tiene y otra vez el 31 en el que
// sí, porque se cuenta desde el primer vencimiento (DTStart) y no desde el último.
// Y un COUNT se agota contando desde el principio, no desde cero en cada vuelta.
func TestSiguienteCuentaDesdeElPrimerVencimiento(t *testing.T) {
	madrid, _ := time.LoadLocation("Europe/Madrid")
	dia := func(m time.Month, d int) time.Time { return time.Date(2027, m, d, 9, 0, 0, 0, madrid) }
	r := Reminder{RRule: "FREQ=MONTHLY", TZID: "Europe/Madrid", DTStart: dia(1, 31).UTC(), DueAt: dia(1, 31).UTC()}
	for _, quiero := range []time.Time{dia(2, 28), dia(3, 31), dia(4, 30), dia(5, 31)} {
		sig, ok := Siguiente(r, r.DueAt.Add(time.Hour))
		if !ok || !sig.Equal(quiero) {
			t.Fatalf("cada mes desde el 31: tras %v sale %v, quiero %v", r.DueAt.In(madrid), sig.In(madrid), quiero)
		}
		r.DueAt = sig
	}
	// El 30 cae el 28 en febrero y vuelve al 30 en marzo.
	r = Reminder{RRule: "FREQ=MONTHLY", TZID: "Europe/Madrid", DTStart: dia(1, 30).UTC(), DueAt: dia(2, 28).UTC()}
	if sig, _ := Siguiente(r, r.DueAt.Add(time.Hour)); !sig.Equal(dia(3, 30)) {
		t.Errorf("cada mes desde el 30, tras febrero: %v", sig.In(madrid))
	}
	r = Reminder{RRule: "FREQ=DAILY;COUNT=2", TZID: "Europe/Madrid", DTStart: dia(1, 1).UTC(), DueAt: dia(1, 2).UTC()}
	if sig, ok := Siguiente(r, r.DueAt.Add(time.Hour)); ok {
		t.Errorf("COUNT=2 ya sonó dos veces y sigue: %v", sig.In(madrid))
	}
}

func TestNormalizarRecordatorio(t *testing.T) {
	cuando := time.Date(2026, 10, 1, 7, 0, 30, 0, time.UTC)
	bien := []ReminderInput{
		{DueAt: cuando},
		{DueAt: cuando, RRule: "RRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", TZID: "Europe/Madrid"},
		{DueAt: cuando, AfterDays: 2, TZID: "Atlantic/Canary"},
	}
	for _, in := range bien {
		if err := in.Normalize(); err != nil {
			t.Errorf("%+v: %v", in, err)
		}
		if in.DueAt.Second() != 0 || in.TZID == "" || strings.HasPrefix(in.RRule, "R") {
			t.Errorf("no se normaliza: %+v", in)
		}
	}
	mal := []ReminderInput{
		{},
		{DueAt: cuando, TZID: "Marte/Olympus"},
		{DueAt: cuando, RRule: "FREQ=HOURLY"},
		{DueAt: cuando, RRule: "FREQ=DAILY\nDTSTART:20260101T000000Z"},
		{DueAt: cuando, RRule: "DTSTART:20260101T000000Z;FREQ=DAILY"},
		{DueAt: cuando, RRule: "FREQ=DAILY", AfterDays: 2},
		{DueAt: cuando, AfterDays: 400},
		{DueAt: cuando, RRule: "cada lunes"},
		// Más de una vez al día: la hora la pone el vencimiento.
		{DueAt: cuando, RRule: "FREQ=DAILY;BYHOUR=8,9,10,11,12,13,14,15,16,17,18,19,20"},
		{DueAt: cuando, RRule: "FREQ=DAILY;BYMINUTE=0,5,10,15,20,25,30,35,40,45,50,55"},
		{DueAt: cuando, RRule: "FREQ=WEEKLY;BYSECOND=0,30"},
		{DueAt: cuando, RRule: "FREQ=DAILY;BYHOUR=9"},
	}
	for _, in := range mal {
		if err := in.Normalize(); err == nil {
			t.Errorf("%+v debía ser inválido", in)
		}
	}
}
