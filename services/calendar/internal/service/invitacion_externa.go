// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/calendar/internal/caldav"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

// Invitaciones de organizadores de FUERA de la suite (D-MAIL-04). Quien organiza en
// Google, Outlook o Apple manda un correo iMIP (RFC 6047) con su REQUEST; llega al
// buzón de Sytue Mail como cualquier otro, y hasta ahora se quedaba en un adjunto
// `invite.ics` que había que bajar e importar a mano, sin manera de contestar. Ahora el
// lector ofrece Sí/Quizá/No: el evento entra en el calendario por defecto con esa
// respuesta y el REPLY sale por correo a quien organiza, que es lo que su cliente
// espera para apuntarla (mandarRespuestaPorCorreo, el mismo de la respuesta desde el
// evento).

// InvitacionImportada es el resultado de contestar una invitación de fuera.
type InvitacionImportada struct {
	CalendarID uuid.UUID                  `json:"calendar_id"`
	UID        string                     `json:"uid"`
	Attendee   string                     `json:"attendee"`
	PartStat   domain.ParticipationStatus `json:"partstat"`
	// ReplySent dice si el REPLY ha salido hacia quien organiza. No sale si es de
	// dentro (su respuesta va por la bandeja), sin relé o sin cupo.
	ReplySent bool `json:"reply_sent"`
}

// ErrNoInvitado: ninguna de las direcciones de quien llama figura como asistente del
// REQUEST. Un correo reenviado no convierte a nadie en invitado ni le deja contestar
// por otro.
var ErrNoInvitado = errors.New("calendar: no figuras como invitado de este evento")

// ImportarInvitacion guarda en el calendario por defecto de quien llama el evento del
// REQUEST `ics` con su respuesta `partstat` (ACCEPTED, TENTATIVE o DECLINED) y manda el
// REPLY a quien organiza.
//
// Solo con identidad propia (personales): un token delegado lee el buzón de otro, pero
// contestar en su nombre es firmar por él. El asistente se busca entre los buzones del
// token, no se toma del cuerpo. Si el evento ya estaba (se contesta otra vez, o llegó
// una actualización), se reescribe con el ETag leído; una invitación más vieja que la
// guardada (SEQUENCE menor) no pisa el evento: solo cambia la respuesta.
func (s *Service) ImportarInvitacion(ctx context.Context, ics string, partstat domain.ParticipationStatus) (InvitacionImportada, error) {
	partstat = domain.ParticipationStatus(strings.ToUpper(strings.TrimSpace(string(partstat))))
	if _, ok := partstatAInvitacion(string(partstat)); !ok {
		return InvitacionImportada{}, fmt.Errorf("%w: respuesta no válida %q", domain.ErrInvalidEvent, partstat)
	}
	inv, err := caldav.LeerInvitacion(ics)
	if err != nil {
		return InvitacionImportada{}, fmt.Errorf("%w: %w", domain.ErrInvalidEvent, err)
	}
	sub, direcciones := personales(ctx)
	if sub == "" {
		return InvitacionImportada{}, ErrNoInvitado
	}
	asistente, objeto := "", ""
	for _, d := range direcciones {
		nuevo, figura, err := caldav.ConPartStat(inv.Objeto, d, partstat)
		if err != nil {
			return InvitacionImportada{}, fmt.Errorf("%w: %w", domain.ErrInvalidEvent, err)
		}
		if figura {
			asistente, objeto = d, nuevo
			break
		}
	}
	if asistente == "" || strings.EqualFold(asistente, inv.Organizador) {
		return InvitacionImportada{}, ErrNoInvitado
	}

	cal, err := s.EnsureDefaultCalendar(ctx, sub)
	if err != nil {
		return InvitacionImportada{}, err
	}
	actual, err := s.store.GetObjectByUID(ctx, cal.ID, inv.UID)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		_, err = s.PutObject(ctx, cal.ID, objeto, uuid.Nil, true)
	case err != nil:
	default:
		if actual.SequenceNo > inv.Secuencia {
			// Lo guardado es más nuevo que este correo: se conserva y solo cambia la
			// respuesta (ConPartStat devuelve igual si ya era esa).
			objeto, _, err = caldav.ConPartStat(actual.Data, asistente, partstat)
			if err != nil {
				return InvitacionImportada{}, fmt.Errorf("%w: %w", domain.ErrInvalidEvent, err)
			}
		}
		if objeto != actual.Data {
			_, err = s.PutObject(ctx, cal.ID, objeto, actual.ETag, false)
		}
	}
	if err != nil {
		return InvitacionImportada{}, err
	}
	enviada := s.mandarRespuestaPorCorreo(ctx, objeto, asistente, partstat)
	return InvitacionImportada{CalendarID: cal.ID, UID: inv.UID, Attendee: asistente, PartStat: partstat, ReplySent: enviada}, nil
}
