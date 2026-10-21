// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
)

// Las dos mitades se leen juntas, y la hora cae en su sitio también el día que cambia
// la hora: el 25 de octubre de 2026 las 9:00 de Madrid NO son medianoche más nueve.
//
// VISTO EN ROJO: con `dia.Add(time.Duration(h)*time.Hour + …)` —la cuenta de antes, la
// de Mail— el caso del cambio de hora sale a las 8:00.
func TestLaFechaYLaHoraSeLeenJuntas(t *testing.T) {
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Skip("sin tzdata:", err)
	}
	casos := []struct {
		fecha, hora string
		quiero      time.Time
		vale        bool
	}{
		{"24/09/2026", "09:30", time.Date(2026, 9, 24, 9, 30, 0, 0, madrid), true},
		{"25/10/2026", "9", time.Date(2026, 10, 25, 9, 0, 0, 0, madrid), true},
		{"24/09/2026", "", time.Time{}, false},
		{"", "09:30", time.Time{}, false},
		{"31/02/2026", "09:30", time.Time{}, false},
	}
	for _, c := range casos {
		got, ok := LeerFechaHora(c.fecha, c.hora, madrid)
		if ok != c.vale || (ok && (!got.Equal(c.quiero) || got.Hour() != c.quiero.Hour())) {
			t.Errorf("LeerFechaHora(%q, %q) = %v, %v; quiero %v, %v", c.fecha, c.hora, got, ok, c.quiero, c.vale)
		}
	}
}

// Intro confirma lo tecleado, y solo si vale: a medio teclear no se reprograma nada.
func TestElCampoDeFechaYHoraConfirmaConIntro(t *testing.T) {
	c := NuevoCampoFechaHora(CampoSM, time.Date(2026, 9, 24, 9, 0, 0, 0, time.Local))
	if c.Fecha.Texto() != "24/09/2026" || c.Hora.Texto() != "09:00" {
		t.Fatalf("escribe %q y %q", c.Fecha.Texto(), c.Hora.Texto())
	}
	var confirmado []time.Time
	c.AlConfirmar = func(t time.Time) { confirmado = append(confirmado, t) }
	c.Hora.PonerTexto("09:3")
	c.Hora.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if len(confirmado) != 0 {
		t.Errorf("confirma una hora a medias: %v", confirmado)
	}
	c.Hora.PonerTexto("09:30")
	c.Fecha.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnter})
	if len(confirmado) != 1 || confirmado[0].Minute() != 30 {
		t.Errorf("no confirma la hora entera: %v", confirmado)
	}
	if c.Fecha.MinSize().Height != AltoCampo(CampoSM) {
		t.Errorf("el campo mide %v de alto y la talla %v", c.Fecha.MinSize().Height, AltoCampo(CampoSM))
	}
}
