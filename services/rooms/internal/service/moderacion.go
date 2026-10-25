// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// Denuncias y bloqueos (ADR 0073). Las verjas están en el almacén, dentro de la
// transacción que escribe (store/moderacion.go); aquí va lo que pasa DESPUÉS: el
// aviso a quien modera, la traza en el log y los eventos a los clientes.
//
// La traza dice quién, qué y cuándo. NO lleva ni el texto del mensaje ni la nota
// de quien denuncia: el resto del servicio tampoco escribe contenido en el log.

// Report registra una denuncia de un mensaje (rec.MessageID) o de una persona en
// un canal. Si es nueva, avisa por el Hub a los administradores del equipo que
// aloja la conversación; repetida, devuelve la que había y no avisa a nadie.
func (s *Service) Report(ctx context.Context, rec store.NewReport) (uuid.UUID, error) {
	r, err := s.pstore.CreateReport(ctx, rec)
	if err != nil {
		return uuid.Nil, err
	}
	if !r.Nueva {
		return r.ID, nil
	}
	t, _ := tenancy.From(ctx)
	log.From(ctx).Info("rooms: denuncia registrada",
		"report", r.ID, "tenant", t.Slug, "reporter", rec.Reporter, "reason", logsafe.Text(rec.Reason),
		"channel", rec.ChannelID, "message", rec.MessageID, "moderadores", len(r.Moderadores))
	if s.hubNotifier != nil {
		for _, sub := range r.Moderadores {
			// Ni quién denuncia ni a quién: eso se ve en la cola, tras su verja.
			s.hubNotifier.SendAsync(ctx, hubnotify.Notification{
				Tenant: t.Slug, Subject: sub, Source: "rooms", Type: "rooms.report", Level: "warning",
				Title: "Nueva denuncia en Sytue Rooms",
				URL:   hubnotify.EnlaceDenuncias(s.cfg.PublicURL, t.Slug),
			})
		}
	}
	return r.ID, nil
}

// ListReports devuelve la cola de denuncias que `actor` puede moderar.
func (s *Service) ListReports(ctx context.Context, actor string, orgAdmin, abiertas bool) ([]domain.Report, error) {
	return s.pstore.ListReports(ctx, actor, orgAdmin, abiertas)
}

// ResolveReport cierra una denuncia y difunde lo que la acción cambió en el canal.
func (s *Service) ResolveReport(ctx context.Context, id uuid.UUID, actor string, orgAdmin bool, action string) (domain.Report, error) {
	r, err := s.pstore.ResolveReport(ctx, id, actor, orgAdmin, action)
	if err != nil {
		return domain.Report{}, err
	}
	return s.trasResolver(ctx, r, actor, action), nil
}

// ListReportsOperador es la cola del operador de la plataforma (ADR 0073, «Cola
// del operador»): todas las organizaciones. Quien llama ya pasó por Sytue ID.
func (s *Service) ListReportsOperador(ctx context.Context, actor string, abiertas bool) ([]domain.Report, error) {
	return s.pstore.ListReportsOperador(ctx, actor, abiertas)
}

// OrgDeDenuncia dice de qué organización es una denuncia.
func (s *Service) OrgDeDenuncia(ctx context.Context, id uuid.UUID) (string, error) {
	return s.pstore.OrgDeDenuncia(ctx, id)
}

// ResolveReportOperador resuelve desde la cola del operador una denuncia de
// `org`, como lo haría quien la administra y a nombre del operador.
func (s *Service) ResolveReportOperador(ctx context.Context, org string, id uuid.UUID, actor, action string) (domain.Report, error) {
	ctx = tenancy.Into(ctx, tenancy.Tenant{Slug: org})
	r, err := s.pstore.ResolveReportOperador(ctx, id, actor, action)
	if err != nil {
		return domain.Report{}, err
	}
	return s.trasResolver(ctx, r, actor, action), nil
}

// trasResolver deja la línea de log y difunde lo que la acción cambió en el canal.
func (s *Service) trasResolver(ctx context.Context, r domain.Report, actor, action string) domain.Report {
	t, _ := tenancy.From(ctx)
	log.From(ctx).Info("rooms: denuncia resuelta",
		"report", r.ID, "tenant", t.Slug, "actor", actor, "action", logsafe.Text(action), "channel", r.ChannelID, "from", r.ResolvedFrom)
	switch action {
	case domain.ReportMessageDeleted:
		s.publish(ctx, EventMessageDeleted, r.ChannelID, map[string]any{"id": r.MessageID, "channel_id": r.ChannelID})
	case domain.ReportMemberRemoved:
		s.publish(ctx, EventChannelMemberRemoved, r.ChannelID,
			memberEvent("removed", r.ChannelID, actor, &domain.Member{Subject: r.TargetSub}))
	}
	return r
}

// BlockMember bloquea a alguien con quien `actor` comparte el canal.
func (s *Service) BlockMember(ctx context.Context, channelID uuid.UUID, actor, subject string) (domain.Block, error) {
	return s.pstore.BlockMember(ctx, channelID, actor, subject)
}

// ListBlocks devuelve a quién tiene bloqueado `actor`.
func (s *Service) ListBlocks(ctx context.Context, actor string) ([]domain.Block, error) {
	return s.pstore.ListBlocks(ctx, actor)
}

// Unblock quita un bloqueo de `actor`.
func (s *Service) Unblock(ctx context.Context, actor, subject string) error {
	return s.pstore.Unblock(ctx, actor, subject)
}
