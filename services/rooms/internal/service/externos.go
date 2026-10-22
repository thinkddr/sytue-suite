// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/personas"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// Personas de OTRA organización en Rooms (ADR 0040): se les abre un DM, se les
// añade a un canal o se les llama escribiendo su correo o su @usuario de Sytue One.
//
// LA CONVERSACIÓN VIVE EN LA ORGANIZACIÓN DE QUIEN LA ABRE. A la persona de fuera
// la encuentra Sytue ID —el dueño del dato— y aquí solo se guarda su `sub` con el
// nombre que Sytue ID devolvió: el nombre de alguien de fuera no puede venir del
// navegador de quien le añade, porque es como le verán todos los demás.

// Persona es alguien de Sytue One, con lo mínimo para escribirle y pintarle.
type Persona = personas.Persona

// Directorio encuentra en Sytue ID a alguien de CUALQUIER organización
// (platform/pkg/personas). Lo implementa DirectorioID; se inyecta para poder
// probar sin Sytue ID.
type Directorio = personas.Directorio

// SetDirectorio conecta el directorio de Sytue ID. Sin él no se puede añadir a
// nadie de fuera: fail-closed.
func (s *Service) SetDirectorio(d Directorio) { s.directorio = d }

// ResolvePerson encuentra a alguien de cualquier organización.
func (s *Service) ResolvePerson(ctx context.Context, consulta string) (Persona, error) {
	if s.directorio == nil {
		return Persona{}, domain.ErrNotFound
	}
	return s.directorio.Resolver(ctx, consulta)
}

// personasDeFuera devuelve, de los subs dados, los que no están en el team
// resueltos en Sytue ID (sub → nombre). Vacío si todos son del team, que es el
// caso normal y no llama a nadie.
func (s *Service) personasDeFuera(ctx context.Context, teamID uuid.UUID, subs []string) (map[string]string, error) {
	fuera, err := s.pstore.SubjectsFueraDelEquipo(ctx, teamID, subs)
	if err != nil || len(fuera) == 0 {
		return nil, err
	}
	if s.directorio == nil {
		return nil, domain.ErrNotMember
	}
	out := make(map[string]string, len(fuera))
	for _, sub := range fuera {
		p, err := s.directorio.PorID(ctx, sub)
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrNotMember
		}
		if err != nil {
			return nil, err
		}
		out[sub] = nombreDePersona(p)
	}
	return out, nil
}

// nombreDePersona: el nombre visible, o su @usuario si no tiene, o el sub.
func nombreDePersona(p Persona) string { return p.Visible() }

// ListExternalChannels lista las conversaciones de OTRAS organizaciones donde el
// usuario está como persona de fuera, con sus no leídos.
func (s *Service) ListExternalChannels(ctx context.Context, sub string) ([]domain.Channel, []domain.ReadState, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return nil, nil, err
	}
	if s.pstore == nil {
		return nil, nil, nil
	}
	return s.pstore.ListExternalChannels(ctx, sub, t.Slug)
}

// RefAnfitrion es el recurso de una ruta por el que se busca la organización
// anfitriona: un canal, un mensaje o una sala (uno solo).
type RefAnfitrion struct {
	Canal, Mensaje, Sala uuid.UUID
}

// Anfitrion devuelve la organización donde vive el recurso cuando quien pregunta
// está en esa conversación como persona de fuera; "" en el caso normal.
func (s *Service) Anfitrion(ctx context.Context, subject string, ref RefAnfitrion) (string, error) {
	t, err := tenancy.From(ctx)
	if err != nil || s.pstore == nil || subject == "" {
		return "", err
	}
	switch {
	case ref.Canal != uuid.Nil:
		return s.pstore.AnfitrionDeCanal(ctx, subject, t.Slug, ref.Canal)
	case ref.Mensaje != uuid.Nil:
		return s.pstore.AnfitrionDeMensaje(ctx, subject, t.Slug, ref.Mensaje)
	case ref.Sala != uuid.Nil:
		anfitrion, err := s.pstore.AnfitrionDeSala(ctx, subject, t.Slug, ref.Sala)
		if err != nil || anfitrion != "" {
			return anfitrion, err
		}
		// La reunión de un evento de otra organización: se va allí sin roles, y si entra
		// lo decide la verja de la sala (Calendar o la clave del enlace).
		return s.pstore.AnfitrionDeReunionDeEvento(ctx, t.Slug, ref.Sala)
	}
	return "", nil
}

// DirectorioID es el Directorio real: pregunta a Sytue ID con el token de quien
// llama (platform/pkg/personas), así que Sytue ID aplica su propio límite por
// persona. Rooms lleva el bearer en su propio contexto (WithBearer, el mismo que
// usa para Drive), así que aquí se le pasa al paquete compartido, y los errores
// del paquete se traducen a los del dominio de Rooms.
type DirectorioID struct {
	c *personas.DirectorioID
}

// NewDirectorioID construye el cliente. baseURL vacía = sin directorio (nil).
func NewDirectorioID(baseURL string) *DirectorioID {
	c := personas.NewDirectorioID(baseURL)
	if c == nil {
		return nil
	}
	return &DirectorioID{c: c}
}

// Resolver implementa Directorio.
func (d *DirectorioID) Resolver(ctx context.Context, consulta string) (Persona, error) {
	p, err := d.c.Resolver(personas.ConBearer(ctx, bearerFrom(ctx)), consulta)
	return p, dePersonas(err)
}

// PorID implementa Directorio.
func (d *DirectorioID) PorID(ctx context.Context, userID string) (Persona, error) {
	p, err := d.c.PorID(personas.ConBearer(ctx, bearerFrom(ctx)), userID)
	return p, dePersonas(err)
}

// dePersonas traduce los errores del paquete compartido a los del dominio de Rooms.
func dePersonas(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, personas.ErrSinSesion):
		return domain.ErrForbidden
	case errors.Is(err, personas.ErrNoExiste):
		return domain.ErrNotFound
	case errors.Is(err, personas.ErrEntradaInvalida):
		return domain.ErrInvalidInput
	case errors.Is(err, personas.ErrDemasiadas):
		return domain.ErrTooManyLookups
	}
	return err
}

// avisarInvitacion deja en la campana de quien invitan desde OTRA organización el aviso
// de su invitación (COM-15, decidido por el dueño el 2026-10-17): la entrada es de la
// anfitriona —el Hub la marca con ella al verla desde la organización de la persona—,
// va a su `sub`, que es suyo en todas sus organizaciones (ADR 0053), y el enlace lleva a
// SUS invitaciones, sin la organización anfitriona: aceptar se hace desde la suya, y el
// aviso no abre nada más de la que invita. Best-effort, como el resto de avisos.
func (s *Service) avisarInvitacion(ctx context.Context, channelID uuid.UUID, subject string) {
	if s.hubNotifier == nil {
		return
	}
	inv, err := s.pstore.InvitacionDelCanal(ctx, channelID, subject)
	if err != nil {
		log.From(ctx).Warn("rooms: aviso de invitación", "channel", channelID, "error", err)
		return
	}
	if inv.NombreDeQuienInvita == "" && s.directorio != nil {
		// Su fila del canal puede no llevar nombre (quien crea un canal entra sin él):
		// el que da Sytue ID, como el de la persona de fuera.
		if p, err := s.directorio.PorID(ctx, inv.InvitadoPor); err == nil {
			inv.NombreDeQuienInvita = nombreDePersona(p)
		}
	}
	n := hubnotify.Notification{
		Tenant: inv.Organizacion, Subject: subject, Source: "rooms", Level: "info",
		URL:    hubnotify.EnlaceInvitacionesRooms(s.cfg.PublicURL, ""),
		Params: map[string]string{"actor": inv.NombreDeQuienInvita},
	}
	if inv.Clase == domain.ChannelDirect {
		n.Type = "rooms.invitation.direct"
		n.Title = hubnotify.RecortarTitulo(inv.NombreDeQuienInvita + " quiere hablar contigo en Rooms")
	} else {
		n.Type = "rooms.invitation"
		n.Params["channel"] = inv.Nombre
		n.Title = hubnotify.RecortarTitulo(inv.NombreDeQuienInvita + " te invita a «" + inv.Nombre + "» en Rooms")
	}
	s.hubNotifier.SendAsync(ctx, n)
}
