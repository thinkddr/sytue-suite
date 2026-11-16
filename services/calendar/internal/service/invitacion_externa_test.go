// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

// requestDeLuis es la invitación por correo de alguien de fuera a Ana.
var requestDeLuis = strings.Replace(invitacionDeFuera, "VERSION:2.0\r\n", "VERSION:2.0\r\nMETHOD:REQUEST\r\n", 1)

func deAnaConBuzon() context.Context {
	return authn.Into(context.Background(), authn.Claims{Subject: "ana-1", Email: "ana@personal.example", Mailbox: "ana@sytue.eu"})
}

// Lo que se rechaza ANTES de tocar el calendario: el servicio de estos tests no tiene
// store, así que si alguno llegara a guardar, el test se caería.
func TestImportarInvitacionRechazaAntesDeGuardar(t *testing.T) {
	s, _ := servicioConCorreo(t)
	casos := []struct {
		nombre string
		ctx    context.Context
		ics    string
		ps     domain.ParticipationStatus
		quiere error
	}{
		{"sin respuesta", deAnaConBuzon(), requestDeLuis, domain.PartNeedsAction, domain.ErrInvalidEvent},
		{"no es REQUEST", deAnaConBuzon(), invitacionDeFuera, domain.PartAccepted, domain.ErrInvalidEvent},
		{"no está invitada", authn.Into(context.Background(), authn.Claims{Subject: "eva-1", Mailbox: "eva@sytue.eu"}), requestDeLuis, domain.PartAccepted, ErrNoInvitado},
		{"token delegado", authn.Into(context.Background(), authn.Claims{Subject: "ana-1", Mailbox: "ana@sytue.eu", Delegado: true}), requestDeLuis, domain.PartAccepted, ErrNoInvitado},
		// El cuerpo no elige a quién se contesta: el login no es el buzón invitado.
		{"solo por el email de entrar", authn.Into(context.Background(), authn.Claims{Subject: "x-1", Email: "ana@sytue.eu"}), requestDeLuis, domain.PartAccepted, ErrNoInvitado},
	}
	for _, c := range casos {
		if _, err := s.ImportarInvitacion(c.ctx, c.ics, c.ps); !errors.Is(err, c.quiere) {
			t.Errorf("%s: error %v, quería %v", c.nombre, err, c.quiere)
		}
	}
}
