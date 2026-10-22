// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package directo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/livekit"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
)

// Webhook recibe un aviso de LiveKit (`POST /webhooks/livekit`): verifica la
// firma con el secreto de la API y aplica el evento. El SFU es el mismo que el de
// Rooms y avisa de TODAS sus rooms: lo que no es de Cast se ignora en silencio.
//
// Devuelve ErrWebhookInvalido si la firma no cuadra (401) y cualquier otro error
// para que LiveKit reintente la entrega (500).
func (s *Servicio) Webhook(ctx context.Context, authorization string, cuerpo []byte) error {
	ev, err := s.firma.ParseWebhook(authorization, cuerpo)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrWebhookInvalido, err)
	}
	return s.aplicar(ctx, ev)
}

// aplicar es la máquina de estados que dictan los avisos. Cada rama es
// idempotente con el sondeo: la transición que ya se hizo no se repite.
func (s *Servicio) aplicar(ctx context.Context, ev livekit.WebhookEvent) error {
	switch ev.Event {
	case livekit.EventIngressStarted:
		return s.ingresoEmpezo(ctx, ev.IngressInfo)
	case livekit.EventIngressEnded:
		return s.ingresoAcabo(ctx, ev.IngressInfo)
	case livekit.EventTrackPublished:
		return s.pistaPublicada(ctx, ev)
	case livekit.EventParticipantJoined, livekit.EventParticipantLeft:
		return s.participante(ctx, ev)
	case livekit.EventRoomFinished:
		return s.roomAcabo(ctx, ev.Room)
	case livekit.EventEgressEnded:
		return s.egressAcabo(ctx, ev.EgressInfo)
	}
	return nil
}

// ingresoEmpezo: OBS ha conectado. El directo pasa a `live` (o se crea si no
// había ninguno programado en esa room).
func (s *Servicio) ingresoEmpezo(ctx context.Context, in *livekit.WebhookIngress) error {
	if in == nil || in.IngressID == "" {
		return nil
	}
	c, err := s.propio.clavePorIngress(ctx, in.IngressID)
	if err != nil {
		if errors.Is(err, domain.ErrNoExiste) {
			return nil // un ingress que no es de Cast
		}
		return err
	}
	v, err := s.directoDeIngress(ctx, c, in.RoomName)
	if err != nil {
		if errors.Is(err, domain.ErrEntradaInvalida) {
			s.logger.Warn("cast: ingress en una room ajena", "ingress", in.IngressID, "room", in.RoomName)
			return nil
		}
		return err
	}
	return s.arrancar(ctx, v)
}

// ingresoAcabo: OBS ha desconectado. El directo termina.
func (s *Servicio) ingresoAcabo(ctx context.Context, in *livekit.WebhookIngress) error {
	if in == nil || in.IngressID == "" {
		return nil
	}
	c, err := s.propio.clavePorIngress(ctx, in.IngressID)
	if err != nil {
		if errors.Is(err, domain.ErrNoExiste) {
			return nil
		}
		return err
	}
	v, err := s.videoActivo(ctx, c.Tenant, in.RoomName)
	if err != nil {
		if errors.Is(err, domain.ErrNoExiste) {
			return nil
		}
		return err
	}
	return s.terminar(ctx, v)
}

// pistaPublicada: la primera pista del emisor (por WebRTC el ingress no avisa) pasa
// el directo a `live`. Las pistas de los demás no significan nada aquí.
func (s *Servicio) pistaPublicada(ctx context.Context, ev livekit.WebhookEvent) error {
	if ev.Room == nil || ev.Participant == nil || !strings.HasPrefix(ev.Participant.Identity, prefijoEmisor) {
		return nil
	}
	tenant, ok := roomDe(ev.Room.Name)
	if !ok {
		return nil
	}
	v, err := s.videoActivo(ctx, tenant, ev.Room.Name)
	if err != nil {
		if errors.Is(err, domain.ErrNoExiste) {
			return nil
		}
		return err
	}
	if ev.Participant.Identity != identidadEmisor(v.OwnerSub) {
		return nil
	}
	return s.arrancar(ctx, v)
}

// participante: quien entra o sale de la room cuenta como espectador, salvo el
// emisor, el ingress y el egress.
func (s *Servicio) participante(ctx context.Context, ev livekit.WebhookEvent) error {
	if ev.Room == nil || ev.Participant == nil || !ev.Participant.Espectador() ||
		strings.HasPrefix(ev.Participant.Identity, prefijoEmisor) {
		return nil
	}
	tenant, ok := roomDe(ev.Room.Name)
	if !ok {
		return nil
	}
	id, err := s.propio.videoActivoPorRoom(ctx, tenant, ev.Room.Name)
	if err != nil {
		if errors.Is(err, domain.ErrNoExiste) {
			return nil
		}
		return err
	}
	_, err = s.propio.espectador(ctx, tenant, id, ev.Participant.Identity, ev.Event == livekit.EventParticipantJoined)
	if errors.Is(err, domain.ErrNoExiste) {
		return nil
	}
	return err
}

// roomAcabo: LiveKit cerró la room (se vació o alguien la borró). El directo termina.
func (s *Servicio) roomAcabo(ctx context.Context, room *livekit.WebhookRoom) error {
	if room == nil {
		return nil
	}
	tenant, ok := roomDe(room.Name)
	if !ok {
		return nil
	}
	v, err := s.videoActivo(ctx, tenant, room.Name)
	if err != nil {
		if errors.Is(err, domain.ErrNoExiste) {
			return nil
		}
		return err
	}
	// Un directo programado cuya room se cierra sin haber emitido no ha
	// terminado: se queda esperando a que el emisor entre.
	if v.Status != domain.StatusLive {
		return nil
	}
	return s.terminar(ctx, v)
}

// egressAcabo: la grabación está en el bucket.
func (s *Servicio) egressAcabo(ctx context.Context, eg *livekit.WebhookEgress) error {
	if eg == nil || eg.EgressID == "" {
		return nil
	}
	tenant, ok := roomDe(eg.RoomName)
	if !ok {
		return nil
	}
	return s.grabacionTerminada(ctx, tenant, *eg)
}
