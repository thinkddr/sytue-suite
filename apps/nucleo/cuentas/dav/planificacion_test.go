// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package dav_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/dav"
	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/dav/davprueba"
)

// RFC 6638: el principal da las direcciones de la persona y, si el servidor planifica,
// su buzón de salida; por él se pregunta la ocupación de otras personas con un VFREEBUSY.
// A quien el servidor no conoce no se le inventa nada; sin buzón, ErrSinPlanificacion.
func TestOcupacionPorElBuzonDeSalida(t *testing.T) {
	s := davprueba.Nuevo()
	defer s.Close()
	s.Planifica = true
	c := cliente(t, s, "/cal/")
	ctx := t.Context()
	pl, err := c.Planificacion(ctx)
	if err != nil || !strings.HasSuffix(pl.Buzon, davprueba.Buzon) || !slices.Equal(pl.Direcciones, []string{"ana@example.org"}) {
		t.Fatalf("planificación: %+v %v", pl, err)
	}
	dia := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	s.Ocupado("Luis@Example.org", dia.Add(9*time.Hour), dia.Add(10*time.Hour))
	s.Ocupado("luis@example.org", dia.Add(30*time.Hour), dia.Add(31*time.Hour)) // fuera
	s.Ocupado("marta@example.org", dia.Add(12*time.Hour), dia.Add(13*time.Hour+30*time.Minute))
	m, err := c.Ocupacion(ctx, pl.Buzon, "mailto:ana@example.org", []string{"luis@example.org", "MARTA@example.org", "nadie@example.org", "luis@example.org"}, dia, dia.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 || len(m["luis@example.org"]) != 1 || !m["luis@example.org"][0].Desde.Equal(dia.Add(9*time.Hour)) ||
		len(m["marta@example.org"]) != 1 || !m["marta@example.org"][0].Hasta.Equal(dia.Add(13*time.Hour+30*time.Minute)) {
		t.Fatalf("ocupación: %+v", m)
	}
	if _, conocido := m["nadie@example.org"]; conocido {
		t.Fatal("se inventó la ocupación de quien el servidor no conoce")
	}
	if len(s.Preguntas) != 1 || !strings.Contains(s.Preguntas[0], "METHOD:REQUEST") || !strings.Contains(s.Preguntas[0], "ORGANIZER:mailto:ana@example.org") ||
		strings.Count(s.Preguntas[0], "ATTENDEE:") != 3 {
		t.Fatalf("pregunta: %q", s.Preguntas)
	}

	// Sin planificación no hay buzón: no se pregunta a nadie.
	s.SinBuzon = true
	pl, err = c.Planificacion(ctx)
	if err != nil || pl.Buzon != "" {
		t.Fatalf("sin buzón: %+v %v", pl, err)
	}
	if _, err := c.Ocupacion(ctx, pl.Buzon, "ana@example.org", []string{"luis@example.org"}, dia, dia.Add(time.Hour)); !errors.Is(err, dav.ErrSinPlanificacion) {
		t.Fatalf("sin buzón: %v", err)
	}
}

// Los FREEBUSY con duración, varios periodos y FBTYPE: lo libre no cuenta y lo
// provisional se marca.
func TestTramosVFREEBUSY(t *testing.T) {
	ts, err := dav.TramosVFREEBUSY("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:x\r\nBEGIN:VFREEBUSY\r\nUID:a\r\nDTSTAMP:20260101T000000Z\r\n" +
		"FREEBUSY:20261201T100000Z/PT1H,20261201T080000Z/20261201T083000Z\r\n" +
		"FREEBUSY;FBTYPE=FREE:20261201T120000Z/PT1H\r\nFREEBUSY;FBTYPE=BUSY-TENTATIVE:20261201T150000Z/PT30M\r\nEND:VFREEBUSY\r\nEND:VCALENDAR\r\n")
	if err != nil || len(ts) != 3 || ts[0].Desde.Hour() != 8 || ts[1].Hasta.Hour() != 11 || !ts[2].Provisional || ts[1].Provisional {
		t.Fatalf("%+v %v", ts, err)
	}
}

// MKCOL extendido: la libreta nueva sale en la lista con su nombre y se puede escribir.
func TestCrearLibreta(t *testing.T) {
	s := davprueba.Nuevo()
	defer s.Close()
	ctx := t.Context()
	c := cliente(t, s, "/card/")
	_, _, casa, err := c.Principal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	u, err := c.CrearLibreta(ctx, casa, "Clientes <2027>")
	if err != nil || !strings.HasPrefix(u, casa) || !strings.HasSuffix(u, "/") {
		t.Fatalf("crear: %q %v", u, err)
	}
	ls, err := c.Colecciones(ctx, casa, dav.Libreta)
	if err != nil || len(ls) != 1 || ls[0].URL != u || ls[0].Nombre != "Clientes <2027>" {
		t.Fatalf("lista: %+v %v", ls, err)
	}
	if _, err := c.Poner(ctx, dav.URLNueva(u, "a", dav.Libreta), "", tarjeta("a", "Ana"), dav.Libreta); err != nil {
		t.Fatalf("escribir en la nueva: %v", err)
	}
	if _, err := c.CrearLibreta(ctx, casa, " "); err == nil {
		t.Fatal("creó una libreta sin nombre")
	}
	if _, err := c.CrearLibreta(ctx, s.URL+"/card/u/", "x"); !errors.Is(err, dav.ErrAcceso) {
		t.Fatalf("fuera de la casa: %v", err)
	}
}
