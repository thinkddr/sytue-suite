// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ical

import (
	"strings"
	"testing"
	"time"
)

// serieMovible es una serie semanal de los miércoles a las 10:00 de Madrid desde octubre
// (horario de verano), con cuatro excepciones: la del 14 de octubre, en UTC y en la hora
// de su ocurrencia (08:00Z son las 10:00 de verano); la del 4 de noviembre, en la hora de
// Madrid y en la de su ocurrencia; la del 11, que alguien acortó a media hora; y la del
// 18, en UTC, que alguien pasó a las 16:00 a propósito.
const serieMovible = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Apple Inc.//iCloud//ES\r\n" +
	"BEGIN:VEVENT\r\nUID:serie@movible\r\nDTSTAMP:20261001T080000Z\r\nSEQUENCE:3\r\n" +
	"DTSTART;TZID=Europe/Madrid:20261007T100000\r\nDTEND;TZID=Europe/Madrid:20261007T110000\r\n" +
	"RRULE:FREQ=WEEKLY\r\nSUMMARY:Seguimiento\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:serie@movible\r\nDTSTAMP:20261001T080000Z\r\nSEQUENCE:1\r\n" +
	"RECURRENCE-ID:20261014T080000Z\r\nDTSTART:20261014T080000Z\r\nDTEND:20261014T090000Z\r\nSUMMARY:Seguimiento\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:serie@movible\r\nDTSTAMP:20261001T080000Z\r\nSEQUENCE:1\r\n" +
	"RECURRENCE-ID;TZID=Europe/Madrid:20261104T100000\r\nDTSTART;TZID=Europe/Madrid:20261104T100000\r\n" +
	"DTEND;TZID=Europe/Madrid:20261104T110000\r\nSUMMARY:Seguimiento\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:serie@movible\r\nDTSTAMP:20261001T080000Z\r\nSEQUENCE:1\r\n" +
	"RECURRENCE-ID;TZID=Europe/Madrid:20261111T100000\r\nDTSTART;TZID=Europe/Madrid:20261111T100000\r\n" +
	"DTEND;TZID=Europe/Madrid:20261111T103000\r\nSUMMARY:Seguimiento (corto)\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:serie@movible\r\nDTSTAMP:20261001T080000Z\r\nSEQUENCE:4\r\n" +
	"RECURRENCE-ID:20261118T090000Z\r\nDTSTART:20261118T150000Z\r\nDTEND:20261118T160000Z\r\nSUMMARY:Seguimiento (por la tarde)\r\nEND:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

// maestroMovido es el maestro que manda el formulario con la serie en `inicio` – `fin`
// (horas de Madrid), sin excepciones.
func maestroMovido(inicio, fin string) string {
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Sytue//Sytue Calendar//ES\r\n" +
		"BEGIN:VEVENT\r\nUID:serie@movible\r\nDTSTAMP:20261120T080000Z\r\nSEQUENCE:4\r\n" +
		"DTSTART;TZID=Europe/Madrid:" + inicio + "\r\nDTEND;TZID=Europe/Madrid:" + fin + "\r\n" +
		"RRULE:FREQ=WEEKLY\r\nSUMMARY:Seguimiento\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
}

// Mover la serie (la misma regla desde otro inicio) mueve con ella sus excepciones: el
// RECURRENCE-ID de cada una, los mismos días y la misma hora de pared que el inicio,
// también al otro lado del cambio de hora, escrito como venía (en UTC o en la hora de
// Madrid). La que estaba en la hora de su ocurrencia se va a la nueva (con la duración
// nueva de la serie si tenía la de antes, y con la suya si no); la que estaba a otra hora
// a propósito se queda a su hora. Su SEQUENCE sube a la del maestro si la tenía menor.
func TestMoverLaSerieMueveSusExcepciones(t *testing.T) {
	// Una hora más tarde y media hora más larga.
	out, err := CarryOver(maestroMovido("20261007T110000", "20261007T123000"), serieMovible, false)
	if err != nil {
		t.Fatal(err)
	}
	ev := vevents(out)
	if len(ev) != 5 {
		t.Fatalf("el maestro y sus cuatro excepciones:\n%s", out)
	}
	for rid, quiero := range map[string][]string{
		// 09:00Z son las 11:00 de verano; con la duración nueva de la serie.
		"20261014T090000Z": {"DTSTART:20261014T090000Z\r\n", "DTEND:20261014T103000Z\r\n", "SEQUENCE:4\r\n"},
		"20261104T110000":  {"RECURRENCE-ID;TZID=Europe/Madrid:20261104T110000\r\n", "DTSTART;TZID=Europe/Madrid:20261104T110000\r\n", "DTEND;TZID=Europe/Madrid:20261104T123000\r\n"},
		// Acortada a propósito: a la hora nueva, con su media hora.
		"20261111T110000": {"DTSTART;TZID=Europe/Madrid:20261111T110000\r\n", "DTEND;TZID=Europe/Madrid:20261111T113000\r\n"},
		// 10:00Z son las 11:00 de invierno; a las 16:00 a propósito, se queda.
		"20261118T100000Z": {"DTSTART:20261118T150000Z\r\n", "DTEND:20261118T160000Z\r\n", "SEQUENCE:4\r\n", "SUMMARY:Seguimiento (por la tarde)"},
	} {
		e, ok := ev[rid]
		if !ok {
			t.Errorf("falta la excepción %s:\n%s", rid, out)
			continue
		}
		for _, q := range quiero {
			if !strings.Contains(e, q) {
				t.Errorf("la excepción %s sin %q:\n%s", rid, q, e)
			}
		}
	}

	// Un día más tarde (los jueves) y a la misma hora.
	out, err = CarryOver(maestroMovido("20261008T100000", "20261008T110000"), serieMovible, false)
	if err != nil {
		t.Fatal(err)
	}
	ev = vevents(out)
	if e := ev["20261105T100000"]; !strings.Contains(e, "DTSTART;TZID=Europe/Madrid:20261105T100000\r\n") {
		t.Errorf("la del 4 de noviembre, al jueves 5:\n%s", out)
	}
	if e := ev["20261119T090000Z"]; !strings.Contains(e, "DTSTART:20261118T150000Z\r\n") {
		t.Errorf("la que estaba por la tarde a propósito, el día que estaba:\n%s", out)
	}

	// Desde el mismo inicio es la misma serie: las excepciones, tal cual.
	out, err = CarryOver(strings.Replace(maestroMovido("20261007T100000", "20261007T110000"), "SEQUENCE:4", "SEQUENCE:5", 1), serieMovible, false)
	if err != nil || strings.Count(out, "RECURRENCE-ID") != 4 || !strings.Contains(vevents(out)["20261014T080000Z"], "DTSTART:20261014T080000Z\r\n") ||
		strings.Count(out, "SEQUENCE:1\r\n") != 3 {
		t.Fatalf("la misma serie: %v\n%s", err, out)
	}
}

// Una serie de días completos movida un día mueve el día de sus excepciones.
func TestMoverUnaSerieDeDiasCompletos(t *testing.T) {
	anterior := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Otro//ES\r\n" +
		"BEGIN:VEVENT\r\nUID:dias@prueba\r\nDTSTAMP:20261001T080000Z\r\nDTSTART;VALUE=DATE:20261007\r\nDTEND;VALUE=DATE:20261008\r\n" +
		"RRULE:FREQ=WEEKLY\r\nSUMMARY:Guardia\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:dias@prueba\r\nDTSTAMP:20261001T080000Z\r\nRECURRENCE-ID;VALUE=DATE:20261021\r\n" +
		"DTSTART;VALUE=DATE:20261021\r\nDTEND;VALUE=DATE:20261022\r\nSUMMARY:Guardia (Luis)\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	nuevo := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Sytue//Sytue Calendar//ES\r\n" +
		"BEGIN:VEVENT\r\nUID:dias@prueba\r\nDTSTAMP:20261120T080000Z\r\nDTSTART;VALUE=DATE:20261008\r\nDTEND;VALUE=DATE:20261009\r\n" +
		"RRULE:FREQ=WEEKLY\r\nSUMMARY:Guardia\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	out, err := CarryOver(nuevo, anterior, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"RECURRENCE-ID;VALUE=DATE:20261022\r\n", "DTSTART;VALUE=DATE:20261022\r\n", "DTEND;VALUE=DATE:20261023\r\n", "SUMMARY:Guardia (Luis)"} {
		if !strings.Contains(out, q) {
			t.Errorf("falta %q:\n%s", q, out)
		}
	}
}

// Con la serie movida, la pantalla manda sus EXDATE movidas con ella (en UTC, como las
// escribe EncodeEvent): cada una sigue escrita como estaba, la de la hora de Madrid en
// la hora de Madrid. Si llegan sin mover (quien no las mueve), se quedan como estaban.
func TestLasExdateMovidasConservanSuForma(t *testing.T) {
	anterior := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Apple Inc.//iCloud//ES\r\n" +
		"BEGIN:VEVENT\r\nUID:serie@movible\r\nDTSTAMP:20261001T080000Z\r\nSEQUENCE:3\r\n" +
		"DTSTART;TZID=Europe/Madrid:20261007T100000\r\nDTEND;TZID=Europe/Madrid:20261007T110000\r\n" +
		"RRULE:FREQ=WEEKLY\r\nEXDATE;TZID=Europe/Madrid:20261014T100000,20261111T100000\r\nEXDATE:20261118T090000Z\r\n" +
		"SUMMARY:Seguimiento\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	movida := strings.Replace(maestroMovido("20261007T110000", "20261007T120000"), "RRULE:FREQ=WEEKLY\r\n",
		"RRULE:FREQ=WEEKLY\r\nEXDATE:20261014T090000Z,20261111T100000Z,20261118T100000Z\r\n", 1)
	out, err := ConservarForma(movida, anterior)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"EXDATE;TZID=Europe/Madrid:20261014T110000,20261111T110000\r\n", "EXDATE:20261118T100000Z\r\n"} {
		if !strings.Contains(out, q) {
			t.Errorf("falta %q:\n%s", q, out)
		}
	}
	if strings.Count(out, "EXDATE") != 2 {
		t.Errorf("las mismas tres EXDATE en sus dos líneas:\n%s", out)
	}
	sinMover := strings.Replace(maestroMovido("20261007T110000", "20261007T120000"), "RRULE:FREQ=WEEKLY\r\n",
		"RRULE:FREQ=WEEKLY\r\nEXDATE:20261014T080000Z,20261111T090000Z,20261118T090000Z\r\n", 1)
	if out, err = ConservarForma(sinMover, anterior); err != nil || !strings.Contains(out, "EXDATE;TZID=Europe/Madrid:20261014T100000,20261111T100000\r\n") {
		t.Fatalf("sin mover, como estaban: %v\n%s", err, out)
	}
}

// MoverConLaSerie mueve las EXDATE como el inicio: la misma hora de pared al otro lado de
// un cambio de hora (la serie de las 10:00 de octubre, pasada al día siguiente a las
// 11:00, deja la del 4 de noviembre el 5 a las 11:00 de invierno) y los mismos días en una
// de días completos.
func TestMoverConLaSerie(t *testing.T) {
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Fatal(err)
	}
	quitada := time.Date(2026, 11, 4, 10, 0, 0, 0, madrid)
	got := MoverConLaSerie([]time.Time{quitada.UTC()}, time.Date(2026, 10, 7, 10, 0, 0, 0, madrid), time.Date(2026, 10, 8, 11, 0, 0, 0, madrid))
	if quiero := time.Date(2026, 11, 5, 11, 0, 0, 0, madrid); len(got) != 1 || !got[0].Equal(quiero) || got[0].Location() != time.UTC {
		t.Errorf("con la hora de pared: %v, quiero %v en UTC", got, quiero)
	}
	dia := time.Date(2026, 12, 9, 0, 0, 0, 0, time.UTC)
	got = MoverConLaSerie([]time.Time{dia}, time.Date(2026, 12, 2, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 4, 0, 0, 0, 0, time.UTC))
	if quiero := dia.AddDate(0, 0, 2); len(got) != 1 || !got[0].Equal(quiero) {
		t.Errorf("de días completos: %v, quiero %v", got, quiero)
	}
	if got := MoverConLaSerie(nil, dia, dia.Add(time.Hour)); got != nil {
		t.Errorf("sin fechas: %v", got)
	}
}
