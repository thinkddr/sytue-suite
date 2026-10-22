// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package directo

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/livekit"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
)

// BucleDeSondeo contrasta el estado con LiveKit cada PollInterval hasta que el
// contexto se cancele. Con PollInterval 0 no hace nada: el ciclo depende solo de
// los webhooks.
func (s *Servicio) BucleDeSondeo(ctx context.Context) {
	if s.cfg.PollInterval <= 0 {
		return
	}
	t := time.NewTicker(s.cfg.PollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.Sondear(ctx); err != nil && !errors.Is(err, context.Canceled) {
				s.logger.Warn("cast: sondeo de LiveKit", "error", err)
			}
		}
	}
}

// Sondear es UNA ronda del respaldo sin webhooks: pregunta a LiveKit qué rooms
// de Cast viven y quién está en ellas, y deja la base como dice LiveKit.
//
//   - Room viva con directo `scheduled` y el emisor dentro con pistas → `live`.
//   - Room viva: la lista de espectadores es la de LiveKit (sin emisor ni egress).
//   - Directo `live` cuya room ya no existe → `ended`.
//   - Directo `ended` con grabación pendiente → consultar egress y encolar su VOD.
//
// Es idempotente con el webhook: comparte con él `arrancar` y `terminar`, que
// no repiten una transición hecha. Si LiveKit no contesta, la ronda no toca nada:
// un SFU caído no es motivo para dar por acabados todos los directos.
func (s *Servicio) Sondear(ctx context.Context) error {
	rooms, err := s.lk.ListRooms(ctx)
	if err != nil {
		return err
	}
	vivas := make(map[string]bool, len(rooms))
	for _, r := range rooms {
		tenant, ok := roomDe(r.Name)
		if !ok {
			continue
		}
		vivas[r.Name] = true
		if err := s.sondearRoom(ctx, tenant, r.Name); err != nil {
			s.logger.Warn("cast: sondear room", "room", r.Name, "error", err)
		}
	}
	activos, err := s.propio.directosEnCurso(ctx)
	if err != nil {
		return err
	}
	for _, a := range activos {
		if a.Status == domain.StatusEnded {
			if err := s.sondearGrabacion(ctx, a); err != nil {
				s.logger.Warn("cast: reconciliar la grabación terminada", "video", a.ID, "error", err)
			}
			continue
		}
		if a.Status != domain.StatusLive || vivas[a.Room] {
			continue
		}
		v, err := s.st.EnOrganizacion(ctx, a.Tenant, a.ID)
		if err != nil {
			if errors.Is(err, domain.ErrNoExiste) {
				continue
			}
			return err
		}
		if err := s.terminar(ctx, v); err != nil {
			s.logger.Warn("cast: terminar un directo sin room", "video", a.ID, "error", err)
		}
	}
	return nil
}

// sondearGrabacion recupera el fin aunque se perdiera egress_ended o la réplica se
// apagara entre guardar el HLS y encolarlo. Solo el transcoder, después de leer los
// objetos reales, puede publicar ready.
func (s *Servicio) sondearGrabacion(ctx context.Context, a activo) error {
	v, err := s.st.EnOrganizacion(ctx, a.Tenant, a.ID)
	if err != nil {
		return err
	}
	if v.Status != domain.StatusEnded {
		return nil
	}
	if v.HlsKey != "" {
		return s.encolarGrabacion(ctx, v)
	}
	grabaciones, err := s.lk.ListEgress(ctx, a.EgressID)
	if err != nil {
		var fallo *livekit.APIError
		if errors.As(err, &fallo) && fallo.StatusCode == http.StatusNotFound && fallo.Code == "not_found" {
			return s.propio.fallarGrabacion(ctx, a.Tenant, a.ID, "grabación no disponible: LiveKit ya no conserva el egress")
		}
		return err
	}
	if len(grabaciones) == 0 {
		return s.propio.fallarGrabacion(ctx, a.Tenant, a.ID, "grabación no disponible: LiveKit ya no conserva el egress")
	}
	for _, eg := range grabaciones {
		if eg.EgressID != a.EgressID {
			continue
		}
		switch eg.Status {
		case "EGRESS_COMPLETE", "EGRESS_FAILED", "EGRESS_ABORTED", "EGRESS_LIMIT_REACHED":
			fin := livekit.WebhookEgress{EgressID: eg.EgressID, RoomName: eg.RoomName, Status: eg.Status, Error: eg.Error}
			for _, seg := range eg.SegmentResults {
				fin.SegmentResults = append(fin.SegmentResults, livekit.WebhookEgressSegments(seg))
			}
			return s.grabacionTerminada(ctx, a.Tenant, fin)
		}
	}
	return nil
}

// sondearRoom reconcilia un directo con lo que hay en su room.
func (s *Servicio) sondearRoom(ctx context.Context, tenant, room string) error {
	v, err := s.videoActivo(ctx, tenant, room)
	if err != nil {
		if errors.Is(err, domain.ErrNoExiste) {
			return nil // una room de Cast sin directo: la de espera de un ingress, o ya terminado
		}
		return err
	}
	ps, err := s.lk.ListParticipants(ctx, room)
	if err != nil {
		return err
	}
	emisor := identidadEmisor(v.OwnerSub)
	emitiendo := false
	espectadores := make([]string, 0, len(ps))
	for _, p := range ps {
		switch {
		case p.Identity == emisor || p.Kind == livekit.ParticipanteIngress:
			emitiendo = emitiendo || len(p.Tracks) > 0
		case p.Kind == "" || p.Kind == livekit.ParticipanteEstandar:
			if !strings.HasPrefix(p.Identity, prefijoEmisor) {
				espectadores = append(espectadores, p.Identity)
			}
		}
	}
	if v.Status == domain.StatusScheduled && emitiendo {
		if err := s.arrancar(ctx, v); err != nil {
			return err
		}
	}
	_, err = s.propio.espectadores(ctx, tenant, v.ID, espectadores)
	if errors.Is(err, domain.ErrNoExiste) {
		return nil
	}
	return err
}
