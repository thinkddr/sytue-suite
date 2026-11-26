// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// servicioConReuniones es el doble con proveedor de reuniones por canal (Sytue Office).
type servicioConReuniones struct {
	*servicioFalso
	prov    map[string]string
	errReun error
}

func (s *servicioConReuniones) ProveedorReunion(canal string) string { return s.prov[canal] }

func (s *servicioConReuniones) CrearReunion(_ context.Context, canal, titulo string) (api.RoomsMessage, error) {
	s.apuntar("CrearReunion", canal, titulo)
	if s.errReun != nil {
		return api.RoomsMessage{}, s.errReun
	}
	m := api.RoomsMessage{ID: "reunion", ChannelID: canal, AuthorSub: "yo", Body: "https://meet.ejemplo.eu/abc", CreatedAt: time.Now()}
	s.mensajes[canal] = append(s.mensajes[canal], m)
	return m, nil
}

// En Sytue Office, «Llamar» es «Reunión de <proveedor>» donde hay proveedor y no sale
// donde no; pulsarlo crea la reunión con el nombre del canal, la publica y recarga el
// final; si falla, lo dice. Con el servicio de la suite la cabecera no cambia.
func TestReunionDelProveedorEnLaCabecera(t *testing.T) {
	s := &servicioConReuniones{servicioFalso: servicioConDos(), prov: map[string]string{"c1": "Google Meet"}}
	sa, _ := salaDePrueba(t, s.servicioFalso)
	sa.S = s
	sa.Arrancar(context.Background())
	cab := sa.P.Canal.Cabecera
	if cab.SinLlamar || cab.RotuloLlamar != sa.P.T.T("link.joinMeeting", i18n.Vars{"provider": "Google Meet"}) {
		t.Fatalf("en c1: sin llamar %v, rótulo %q", cab.SinLlamar, cab.RotuloLlamar)
	}
	antes := s.veces("Messages(c1)")
	sa.LlamarEnCanal()
	if !s.pidio("CrearReunion(c1,equipo)") || s.veces("Messages(c1)") != antes+1 {
		t.Fatalf("crear la reunión: %v", s.llamadas)
	}
	s.errReun = errors.New("sin red")
	sa.LlamarEnCanal()
	avisos := sa.P.Avisos.Avisos()
	if len(avisos) == 0 || avisos[len(avisos)-1].Titulo != sa.P.T.T("meetings.createFailed", nil) || avisos[len(avisos)-1].Tono != ui.AvisoPeligro {
		t.Fatalf("el fallo no se dice: %+v", avisos)
	}
	for _, c := range sa.P.Lateral.Canales {
		if c.ID == "d1" {
			sa.Elegir(c)
		}
	}
	if !cab.SinLlamar || cab.RotuloLlamar != "" {
		t.Fatalf("en d1, sin proveedor: sin llamar %v, rótulo %q", cab.SinLlamar, cab.RotuloLlamar)
	}

	// La suite: el servicio no crea reuniones y la cabecera es la de siempre.
	s2 := servicioConDos()
	sa2, _ := salaDePrueba(t, s2)
	sa2.Arrancar(context.Background())
	if c := sa2.P.Canal.Cabecera; c.SinLlamar || c.RotuloLlamar != "" || c.rotuloLlamar() != sa2.P.T.T("call.call", nil) {
		t.Fatalf("la cabecera de la suite cambió: %v %q", c.SinLlamar, c.RotuloLlamar)
	}
}
