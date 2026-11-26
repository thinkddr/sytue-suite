// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/calendario"
)

// suelto es un VCALENDAR con un evento de `uid` que empieza con `dtstart` (la línea
// entera, con su zona o su VALUE=DATE) y acaba con `dtend`.
func suelto(uid, dtstart, dtend string) string {
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Apple Inc.//iCloud//ES\r\nBEGIN:VEVENT\r\nUID:" + uid + "\r\nDTSTAMP:20261101T080000Z\r\nSEQUENCE:1\r\n" +
		dtstart + "\r\n" + dtend + "\r\nSUMMARY:Llamada\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
}

// Abrir un evento y guardarlo sin tocar nada no cambia su hora real, se pinte en la zona
// que se pinte (la del sistema u otra, igual o distinta de la del evento): ni la serie de
// Madrid (abierta en su primera y en su tercera), ni un evento de Nueva York, ni uno en
// UTC sin zona, ni uno de día completo, que no tiene zona y no cambia de día. Antes, con
// la pantalla en UTC, la serie de las 10:00 de Madrid se guardaba a las 09:00 de Madrid.
// Se escriben tal cual venían.
func TestAbrirYGuardarEnOtraZonaNoMueveLaHora(t *testing.T) {
	nuevaYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	kolkata, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Skip(err)
	}
	for _, z := range []struct {
		nombre string
		loc    *time.Location
	}{{"UTC", time.UTC}, {"Nueva York", nuevaYork}, {"Kolkata", kolkata}, {"Madrid", madrid}, {"la del sistema", nil}} {
		t.Run(z.nombre, func(t *testing.T) {
			m := nuevaMesa(t, nuevoRepositorio, serieDeLaCuenta)
			m.p = &calendario.Pantalla{Zona: z.loc, SinPersonas: true}
			ctx := t.Context()
			dia := m.inicio.AddDate(0, 0, 1)
			fecha := dia.Format("20060102")
			sueltos := map[string]string{
				"ny@icloud":  suelto("ny@icloud", "DTSTART;TZID=America/New_York:"+fecha+"T100000", "DTEND;TZID=America/New_York:"+fecha+"T113000"),
				"utc@icloud": suelto("utc@icloud", "DTSTART:"+fecha+"T233000Z", "DTEND:"+dia.AddDate(0, 0, 1).Format("20060102")+"T003000Z"),
				"dia@icloud": suelto("dia@icloud", "DTSTART;VALUE=DATE:"+fecha, "DTEND;VALUE=DATE:"+dia.AddDate(0, 0, 1).Format("20060102")),
			}
			for _, d := range sueltos {
				if _, err := m.repo.PonerObjetoCrudo(ctx, m.cal, "", "", d); err != nil {
					t.Fatal(err)
				}
			}
			original := m.objetos()

			for _, n := range []int{0, 2} {
				oc, e := m.abrirSemana(n)
				if err := calendario.EditarSerie(ctx, m.facade, oc, calendario.AlcanceTodos, e.EntradaDe(m.p), e.Regla); err != nil {
					t.Fatal(err)
				}
				mismasLineas(t, original[uidSerie], m.objetos()[uidSerie])
			}

			is, err := m.facade.Instances(ctx, m.cal, dia.Add(-36*time.Hour), dia.Add(48*time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			vistos := 0
			for _, i := range is {
				if _, ok := sueltos[i.Event.UID]; !ok {
					continue
				}
				vistos++
				e := m.p.EdicionDe(calendario.Ocurrencia{Instance: i, CalendarioID: m.cal})
				if _, err := m.facade.UpdateEvent(ctx, i.Event.ID, e.EntradaDe(m.p)); err != nil {
					t.Fatal(err)
				}
				mismasLineas(t, original[i.Event.UID], m.objetos()[i.Event.UID])
			}
			if vistos != len(sueltos) {
				t.Fatalf("se ven %d de los %d sueltos", vistos, len(sueltos))
			}
		})
	}
}
