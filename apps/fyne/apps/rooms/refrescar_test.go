// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// RefrescarCanales trae una conversación nueva al carril sin esqueletos, sin volver a
// escuchar y sin cambiar el canal abierto; con la lista en error, es «Reintentar».
func TestRefrescarCanalesTraeLoNuevoSinRecargar(t *testing.T) {
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	l := sa.P.Lateral
	if l.EstadoCanales != Listo || len(l.Canales) != 2 || l.CanalID != "c1" {
		t.Fatal(l.EstadoCanales, len(l.Canales), l.CanalID)
	}
	escuchas := s.veces("Listen(c1,d1)")
	s.canales = append(s.canales, api.RoomsChannel{ID: "d2", Name: "dm-nuevo", Kind: "direct", Peers: []string{"Carla Gil"}})
	s.canales[0].Topic = "Guardias de diciembre"
	s.pend = append(s.pend, api.RoomsUnread{ChannelID: "d2", Unread: 1})
	sa.RefrescarCanales()
	if l.EstadoCanales != Listo || len(l.Canales) != 3 || l.CanalID != "c1" {
		t.Fatal(l.EstadoCanales, len(l.Canales), l.CanalID)
	}
	var nuevo *Canal
	for i := range l.Canales {
		if l.Canales[i].ID == "d2" {
			nuevo = &l.Canales[i]
		}
	}
	if nuevo == nil || nuevo.Nombre != "Carla Gil" || nuevo.SinLeer != 1 || nuevo.Clase != Directo {
		t.Fatalf("el directo nuevo: %+v", nuevo)
	}
	if tema := sa.P.Canal.Cabecera.Canal.Tema; tema != "Guardias de diciembre" {
		t.Fatalf("la cabecera del abierto se quedó con %q", tema)
	}
	if s.veces("Listen(c1,d1)") != escuchas || s.pidio("Listen(c1,d1,d2)") {
		t.Fatalf("se volvió a escuchar: %v", s.llamadas)
	}
	// Con la lista en error, refrescar es reintentar (y vuelve a escuchar).
	l.EstadoCanales = Error
	sa.RefrescarCanales()
	if l.EstadoCanales != Listo || !s.pidio("Listen(c1,d1,d2)") {
		t.Fatal(l.EstadoCanales, s.llamadas)
	}
}
