// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"

	"git.sytue.eu/sytue/one/platform/pkg/livekit"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/store"
)

func (s *Service) handleCallFinished(ctx context.Context, ev livekit.WebhookEvent) error {
	if ev.Room == nil {
		return nil
	}
	endedAt := eventTime(ev.CreatedAt)
	// Foto ANTES de marcar: da el origen (canal de chat) y el started_at para la
	// tarjeta, y su status sirve de dedupe —si la sala ya estaba 'ended' (reentrega
	// del webhook), no volvemos a avisar a Chat—. Solo se consulta si el aviso está
	// habilitado (MarkRoomEnded ya es idempotente por sí mismo).
	var pre domain.Room
	// La sala se lee SIEMPRE, no solo cuando hay notificador de chat: su
	// metadata es lo único que dice de qué canal salió la llamada, y sin eso
	// el aviso de «terminada» no llega y el banner se queda colgado.
	var havePre bool
	if r, err := s.store.GetRoomByName(ctx, ev.Room.Name); err == nil {
		pre, havePre = r, true
	}
	if havePre && s.encryption != nil {
		finished, err := s.encryption.FinishCallTransport(ctx, pre.ID, ev.Room.Name)
		if err != nil {
			return err
		}
		if !finished {
			return nil
		}
	}
	if err := s.store.MarkRoomEnded(ctx, ev.Room.Name, endedAt); err != nil {
		return err
	}
	carga := map[string]any{"name": ev.Room.Name}
	if havePre {
		carga = cargaDeCierre(pre)
		carga["name"] = ev.Room.Name
	}
	s.publish(ctx, "meet.room.finished", carga)
	if havePre && pre.Status != domain.RoomEnded {
		s.notifyCallEnded(ctx, pre, endedAt)
	}
	return nil
}

func (s *Service) handleCallJoined(ctx context.Context, ev livekit.WebhookEvent) error {
	if ev.Room == nil || ev.Participant == nil {
		return nil
	}
	if accepted, err := s.encryptedPresence(ctx, ev, true); err != nil {
		return err
	} else if !accepted {
		return nil
	}
	if err := s.store.RecordJoin(ctx, store.RecordJoinInput{
		RoomName:   ev.Room.Name,
		Identity:   ev.Participant.Identity,
		Name:       ev.Participant.Name,
		Role:       roleFromMetadata(ev.Participant.Metadata),
		LiveKitSID: ev.Participant.SID,
		JoinedAt:   eventTime(ev.CreatedAt),
	}); err != nil {
		return err
	}
	s.publish(ctx, "meet.participant.joined", map[string]any{
		"name": ev.Room.Name, "identity": ev.Participant.Identity,
	})
	return nil
}

func (s *Service) handleCallLeft(ctx context.Context, ev livekit.WebhookEvent) error {
	if ev.Room == nil || ev.Participant == nil {
		return nil
	}
	accepted, presenceErr := s.encryptedPresence(ctx, ev, false)
	if presenceErr != nil {
		return presenceErr
	}
	if err := s.store.RecordLeave(ctx, ev.Room.Name, ev.Participant.SID, eventTime(ev.CreatedAt)); err != nil {
		return err
	}
	if !accepted {
		return nil
	}
	s.publish(ctx, "meet.participant.left", map[string]any{
		"name": ev.Room.Name, "identity": ev.Participant.Identity,
	})
	return nil
}
