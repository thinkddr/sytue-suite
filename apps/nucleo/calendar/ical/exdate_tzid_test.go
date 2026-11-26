// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ical

import (
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"

	domain "git.sytue.eu/sytue/one/apps/nucleo/calendar/event"
)

// EncodeEvent escribe las EXDATE y RDATE de la serie como su DTSTART: en la hora de su
// zona con su TZID; en UTC si la serie es en UTC; como fechas si es de días completos. Y
// se leen de vuelta como los mismos instantes.
func TestEncodeEventEscribeLasFechasDeLaSerieComoElInicio(t *testing.T) {
	madrid, _ := time.LoadLocation("Europe/Madrid")
	casos := []struct {
		nombre string
		ev     domain.Event
		quiero []string
	}{
		{"en Madrid", domain.Event{
			DTStart: time.Date(2026, 12, 2, 10, 0, 0, 0, madrid), DTEnd: time.Date(2026, 12, 2, 11, 0, 0, 0, madrid), TZID: "Europe/Madrid",
			ExDate: []time.Time{time.Date(2026, 12, 9, 10, 0, 0, 0, madrid), time.Date(2026, 12, 16, 10, 0, 0, 0, madrid)},
			RDate:  []time.Time{time.Date(2026, 12, 19, 12, 0, 0, 0, madrid)},
		}, []string{"EXDATE;TZID=Europe/Madrid:20261209T100000,20261216T100000\r\n", "RDATE;TZID=Europe/Madrid:20261219T120000\r\n"}},
		{"en UTC", domain.Event{
			DTStart: time.Date(2026, 12, 2, 9, 0, 0, 0, time.UTC), DTEnd: time.Date(2026, 12, 2, 10, 0, 0, 0, time.UTC), TZID: "UTC",
			ExDate: []time.Time{time.Date(2026, 12, 9, 9, 0, 0, 0, time.UTC)},
		}, []string{"EXDATE:20261209T090000Z\r\n"}},
		{"de días completos", domain.Event{
			DTStart: time.Date(2026, 12, 2, 0, 0, 0, 0, time.UTC), DTEnd: time.Date(2026, 12, 3, 0, 0, 0, 0, time.UTC), AllDay: true, TZID: "Europe/Madrid",
			ExDate: []time.Time{time.Date(2026, 12, 9, 0, 0, 0, 0, time.UTC)},
		}, []string{"EXDATE;VALUE=DATE:20261209\r\n"}},
	}
	for _, c := range casos {
		c.ev.UID, c.ev.RRule = "serie@prueba", "FREQ=WEEKLY;COUNT=4"
		data, err := EncodeEvent(c.ev, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, q := range c.quiero {
			if !strings.Contains(data, q) {
				t.Errorf("%s: falta %q:\n%s", c.nombre, q, data)
			}
		}
		po, err := DecodeCalendarObject(data, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(po.Master.ExDate) != len(c.ev.ExDate) || len(po.Master.RDate) != len(c.ev.RDate) {
			t.Fatalf("%s: se leen %v y %v", c.nombre, po.Master.ExDate, po.Master.RDate)
		}
		for i := range c.ev.ExDate {
			if !po.Master.ExDate[i].Equal(c.ev.ExDate[i]) {
				t.Errorf("%s: EXDATE %v, quiero %v", c.nombre, po.Master.ExDate[i], c.ev.ExDate[i])
			}
		}
		for i := range c.ev.RDate {
			if !po.Master.RDate[i].Equal(c.ev.RDate[i]) {
				t.Errorf("%s: RDATE %v, quiero %v", c.nombre, po.Master.RDate[i], c.ev.RDate[i])
			}
		}
	}
}

// Una EXDATE en hora local con TZID (como las escribe iCloud) es esa hora en esa zona:
// tiene que casar con la ocurrencia de las 10:00 de Madrid, no con las 10:00 UTC.
func TestEXDATEConTZIDEsHoraDeEsaZona(t *testing.T) {
	props := ical.Props{}
	p := ical.NewProp(ical.PropExceptionDates)
	p.Params.Set(ical.ParamTimezoneID, "Europe/Madrid")
	p.Value = "20261201T100000,20261208T100000"
	props.Add(p)
	z := ical.NewProp(ical.PropExceptionDates)
	z.Value = "20261215T090000Z"
	props.Add(z)
	d := ical.NewProp(ical.PropExceptionDates)
	d.Params.Set(ical.ParamValue, "DATE")
	d.Value = "20261225"
	props.Add(d)

	got := PropertyTimes(props, ical.PropExceptionDates)
	madrid, _ := time.LoadLocation("Europe/Madrid")
	quiero := []time.Time{
		time.Date(2026, 12, 1, 10, 0, 0, 0, madrid).UTC(),
		time.Date(2026, 12, 8, 10, 0, 0, 0, madrid).UTC(),
		time.Date(2026, 12, 15, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC),
	}
	if len(got) != len(quiero) {
		t.Fatalf("%v", got)
	}
	for i := range quiero {
		if !got[i].Equal(quiero[i]) {
			t.Errorf("%d: %v, quiero %v", i, got[i], quiero[i])
		}
	}
}
