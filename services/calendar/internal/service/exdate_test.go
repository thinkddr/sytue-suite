// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/calendar/internal/caldav"
)

// El recurso iCalendar que el servicio escribe para un evento de la API REST lleva las
// EXDATE y RDATE de la serie, las mismas que van a sus columnas: sin ellas, la expansión
// del servicio ocultaba la ocurrencia quitada y los clientes CalDAV que sincronizan el
// recurso (iCloud, Thunderbird, las apps Fyne) la volvían a enseñar.
func TestElRecursoDeRESTLlevaLasFechasDeLaSerie(t *testing.T) {
	madrid, _ := time.LoadLocation("Europe/Madrid")
	quitada := time.Date(2026, 12, 9, 10, 0, 0, 0, madrid)
	anadida := time.Date(2026, 12, 19, 12, 0, 0, 0, madrid)
	si, err := (&Service{}).toStoreInput(EventInput{
		UID: "serie@rest", Summary: "Seguimiento",
		DTStart: time.Date(2026, 12, 2, 10, 0, 0, 0, madrid), DTEnd: time.Date(2026, 12, 2, 11, 0, 0, 0, madrid),
		TZID: "Europe/Madrid", RRule: "FREQ=WEEKLY;COUNT=6",
		ExDate: []time.Time{quitada.UTC()}, RDate: []time.Time{anadida.UTC()},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"EXDATE;TZID=Europe/Madrid:20261209T100000\r\n", "RDATE;TZID=Europe/Madrid:20261219T120000\r\n"} {
		if !strings.Contains(si.ICalData, q) {
			t.Errorf("falta %q:\n%s", q, si.ICalData)
		}
	}
	po, err := caldav.DecodeCalendarObject(si.ICalData, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(po.Master.ExDate) != 1 || !po.Master.ExDate[0].Equal(quitada) || len(po.Master.RDate) != 1 || !po.Master.RDate[0].Equal(anadida) {
		t.Fatalf("el recurso no dice lo que las columnas: EXDATE %v, RDATE %v", po.Master.ExDate, po.Master.RDate)
	}
}

// El recurso que se escribe al editar por REST lleva la SEQUENCE de la columna: sin ella,
// el cliente CalDAV que la compara veía la versión 0 y podía no tomar el cambio por nuevo.
func TestElRecursoDeRESTLlevaLaSEQUENCE(t *testing.T) {
	inicio := time.Date(2026, 12, 2, 9, 0, 0, 0, time.UTC)
	si, err := (&Service{}).toStoreInput(EventInput{UID: "secuencia@rest", Summary: "Revisión", DTStart: inicio, DTEnd: inicio.Add(time.Hour)}, 4)
	if err != nil {
		t.Fatal(err)
	}
	po, err := caldav.DecodeCalendarObject(si.ICalData, "")
	if err != nil {
		t.Fatal(err)
	}
	if si.SequenceNo != 4 || po.Master.SequenceNo != 4 {
		t.Fatalf("SEQUENCE %d en la columna y %d en el recurso, quiero 4:\n%s", si.SequenceNo, po.Master.SequenceNo, si.ICalData)
	}
	si, err = (&Service{}).toStoreInput(EventInput{UID: "nuevo@rest", Summary: "Revisión", DTStart: inicio, DTEnd: inicio.Add(time.Hour)}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(si.ICalData, "SEQUENCE") {
		t.Errorf("un evento nuevo, en la versión 0, no la escribe:\n%s", si.ICalData)
	}
}
