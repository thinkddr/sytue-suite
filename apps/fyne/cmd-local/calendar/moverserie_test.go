// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/calendario"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateCalendar "git.sytue.eu/sytue/one/apps/nucleo/calendar"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// serieConExcepciones es la serie de la cuenta con dos excepciones: la de la cuarta
// semana, que creó la respuesta de Luis (en la hora de su ocurrencia), y la de la sexta,
// que Ana pasó a las 08:00 a propósito.
func serieConExcepciones(inicio time.Time) string {
	aux := &mesaSerie{inicio: inicio}
	return conExcepcion(conExcepcion(serieDeLaCuenta(inicio),
		aux.excepcionSemana(3, "DECLINED", "NEEDS-ACTION", "Seguimiento", 0)),
		aux.excepcionSemana(5, "NEEDS-ACTION", "ACCEPTED", "Seguimiento (con la dirección)", -2*time.Hour))
}

// Mover la serie una hora más tarde con el formulario —desde la primera ocurrencia o
// desde otra— mueve con ella sus EXDATE y sus excepciones: la ocurrencia que Ana quitó
// («solo este» → eliminar) y la que vino quitada de la cuenta siguen sin salir; la
// excepción que tenía la hora de su ocurrencia se va con ella a la hora nueva (con la
// respuesta de Luis), y la que Ana puso a las 08:00 a propósito se queda a las 08:00,
// sin que su ocurrencia salga además. Antes las EXDATE se quedaban en la hora vieja (la
// quitada volvía a salir) y las excepciones se perdían. Con el repositorio de Sytue
// Office las líneas siguen escritas como venían (en la hora de Madrid); sin sus opciones
// (el de la edición conectada), lo que se ve es lo mismo.
func TestMoverLaSerieMueveSusExdateYSusExcepciones(t *testing.T) {
	repos := []struct {
		nombre string
		nuevo  func(*almacen.Recursos) (*privateCalendar.Repository, error)
		forma  bool
	}{
		{"Sytue Office", nuevoRepositorio, true},
		{"sin las opciones", func(r *almacen.Recursos) (*privateCalendar.Repository, error) { return privateCalendar.New(r.Config) }, false},
	}
	for _, repo := range repos {
		for _, desde := range []int{0, 2} {
			t.Run(fmt.Sprintf("%s, desde la semana %d", repo.nombre, desde), func(t *testing.T) {
				moverLaSerieUnaHora(t, nuevaMesa(t, repo.nuevo, serieConExcepciones), desde, repo.forma)
			})
		}
	}
}

func moverLaSerieUnaHora(t *testing.T, m *mesaSerie, desde int, forma bool) {
	ctx := t.Context()
	// Ana quita la segunda: «solo este» → eliminar, como la vista rápida.
	oc, _ := m.abrirSemana(1)
	if err := calendario.BorrarSerie(ctx, m.facade, oc, calendario.AlcanceEste); err != nil {
		t.Fatal(err)
	}
	if is := m.semana(1); len(is) != 0 {
		t.Fatalf("la segunda, quitada, sale: %+v", is)
	}
	// Y mueve la serie entera una hora más tarde.
	oc, e := m.abrirSemana(desde)
	e.Inicio, e.Fin = e.Inicio.Add(time.Hour), e.Fin.Add(time.Hour)
	if err := calendario.EditarSerie(ctx, m.facade, oc, calendario.AlcanceTodos, e.EntradaDe(m.p), e.Regla); err != nil {
		t.Fatal(err)
	}
	guardado := m.objetos()[uidSerie]
	for n := range 8 {
		w := m.inicio.AddDate(0, 0, 7*n)
		once := time.Date(w.Year(), w.Month(), w.Day(), 11, 0, 0, 0, madrid)
		is := m.semana(n)
		switch n {
		case 1, 4:
			if len(is) != 0 {
				t.Errorf("la semana %d, quitada, vuelve a salir: %v", n, instantes(is))
			}
		case 3:
			if len(is) != 1 || is[0].Event.RecurrenceID == nil || !is[0].Start.Equal(once) || !declino(is[0], "luis@example.org") {
				t.Errorf("la cuarta, la excepción de la respuesta de Luis, una vez y a las 11:00: %v", instantes(is))
			}
		case 5:
			ocho := time.Date(w.Year(), w.Month(), w.Day(), 8, 0, 0, 0, madrid)
			if len(is) != 1 || is[0].Event.RecurrenceID == nil || !is[0].Start.Equal(ocho) || is[0].Event.Summary != "Seguimiento (con la dirección)" {
				t.Errorf("la sexta, la que Ana puso a las 08:00, una vez y a su hora: %v", instantes(is))
			}
		default:
			if len(is) != 1 || is[0].Event.RecurrenceID != nil || !is[0].Start.Equal(once) {
				t.Errorf("la semana %d, una vez y a las 11:00: %v", n, instantes(is))
			}
		}
	}
	if forma {
		// Escritas como venían: en la hora de Madrid, ya a las 11:00.
		for _, l := range []string{
			"EXDATE;TZID=Europe/Madrid:" + hora(m.inicio.AddDate(0, 0, 7).Add(time.Hour)),
			"EXDATE;TZID=Europe/Madrid:" + hora(m.inicio.AddDate(0, 0, 28).Add(time.Hour)),
			"RECURRENCE-ID;TZID=Europe/Madrid:" + hora(m.inicio.AddDate(0, 0, 21).Add(time.Hour)),
			"DTSTART;TZID=Europe/Madrid:" + hora(m.inicio.AddDate(0, 0, 21).Add(time.Hour)),
			"DTEND;TZID=Europe/Madrid:" + hora(m.inicio.AddDate(0, 0, 21).Add(2*time.Hour)),
			"RECURRENCE-ID;TZID=Europe/Madrid:" + hora(m.inicio.AddDate(0, 0, 35).Add(time.Hour)),
			"DTSTART;TZID=Europe/Madrid:" + hora(m.inicio.AddDate(0, 0, 35).Add(-2*time.Hour)),
		} {
			if !lineasSin(guardado)[l] {
				t.Errorf("falta %q", l)
			}
		}
	}
	if t.Failed() {
		t.Logf("guardado:\n%s", guardado)
	}
}

// instantes son las ocurrencias para un mensaje: su inicio en Madrid y si son excepción.
func instantes(is []api.Instance) []string {
	out := make([]string, 0, len(is))
	for _, i := range is {
		s := i.Start.In(madrid).Format("02/01 15:04")
		if i.Event.RecurrenceID != nil {
			s += " (excepción)"
		}
		out = append(out, s)
	}
	return out
}

// declino: en la ocurrencia, quien tiene esa dirección declinó.
func declino(i api.Instance, correo string) bool {
	for _, a := range i.Event.Attendees {
		if strings.EqualFold(a.Email, correo) {
			return a.PartStat == "DECLINED"
		}
	}
	return false
}
