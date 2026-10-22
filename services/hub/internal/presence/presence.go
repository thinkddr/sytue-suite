// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package presence gestiona la presencia de usuarios de Sytue Hub: heartbeat,
// consulta agregada y offline explícito. La presencia es EFÍMERA: con Dragonfly
// configurado vive como claves con TTL ("presence:{tenant}:{sub}") que expiran
// solas — sin UPSERT por heartbeat, sin purga ni vacuum en PostgreSQL. Sin
// Dragonfly (DRAGONFLY_URL vacía o caída) degrada al camino PostgreSQL
// original (upsert + derivación de offline por TTL al leer). Ambos caminos
// devuelven la misma forma ([]domain.Presence) al handler.
package presence

import (
	"context"
	"encoding/json"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/cache"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

// store es la superficie mínima de persistencia que consume el servicio
// (*store.Store en producción; fakes en tests, mismo patrón que los handlers).
type store interface {
	Heartbeat(ctx context.Context, subject string, state domain.PresenceState, status string) error
	SetOffline(ctx context.Context, subject string) error
	GetPresence(ctx context.Context, subjects []string, ttl time.Duration) ([]domain.Presence, error)
}

// Service expone las operaciones de presencia.
type Service struct {
	store store
	cache *cache.Client // nil => sin Dragonfly, camino PostgreSQL
	ttl   time.Duration
}

// New construye el servicio de presencia. ttl es la ventana tras la que un
// heartbeat se considera obsoleto (usuario offline si no renueva). c puede ser
// nil: la presencia queda solo en PostgreSQL. OJO: no pasar un cliente
// deshabilitado (cache.New con url vacía) — sus SetTTL no-op tirarían los
// heartbeats en silencio; con Dragonfly ausente inyectar nil.
func New(st store, c *cache.Client, ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = 45 * time.Second
	}
	return &Service{store: st, cache: c, ttl: ttl}
}

// TTL devuelve la ventana de presencia (la usa el handler para sugerir el
// intervalo de heartbeat al cliente: ~TTL/2).
func (s *Service) TTL() time.Duration { return s.ttl }

// key devuelve la clave de presencia de un usuario en Dragonfly.
func key(tenant, subject string) string {
	return "presence:" + tenant + ":" + subject
}

// Heartbeat renueva la presencia del usuario con el estado declarado. Con
// Dragonfly escribe una clave con TTL (que expira sola => offline, la misma
// semántica que derivedState en PostgreSQL); si Dragonfly no responde degrada
// al UPSERT original.
func (s *Service) Heartbeat(ctx context.Context, subject string, state domain.PresenceState, status string) error {
	if state == "" {
		state = domain.StateOnline
	}
	if !domain.ValidState(state) {
		return domain.ErrInvalidInput
	}
	if len(status) > 140 {
		return domain.ErrInvalidInput
	}
	if s.cache != nil {
		b, err := json.Marshal(domain.Presence{
			Subject: subject, State: state, Status: status, LastSeen: time.Now().UTC(),
		})
		if err != nil {
			return err
		}
		t := tenancy.MustFrom(ctx)
		if err = s.cache.SetTTL(ctx, key(t.Slug, subject), string(b), s.ttl); err == nil {
			return nil
		}
		log.From(ctx).Warn("presencia: Dragonfly no responde, degradando a PostgreSQL", "error", err)
	}
	return s.store.Heartbeat(ctx, subject, state, status)
}

// Offline marca al usuario como offline (logout/cierre de pestaña). Con
// Dragonfly basta borrar la clave (ausente => offline al leer).
func (s *Service) Offline(ctx context.Context, subject string) error {
	if s.cache != nil {
		t := tenancy.MustFrom(ctx)
		err := s.cache.Del(ctx, key(t.Slug, subject))
		if err == nil {
			return nil
		}
		log.From(ctx).Warn("presencia: Dragonfly no responde, degradando a PostgreSQL", "error", err)
	}
	return s.store.SetOffline(ctx, subject)
}

// Query devuelve la presencia de los usuarios pedidos, ya derivada por TTL. Los
// subjects desconocidos se devuelven como offline para una respuesta completa.
func (s *Service) Query(ctx context.Context, subjects []string) ([]domain.Presence, error) {
	uniq := dedupe(subjects)
	if len(uniq) == 0 {
		return nil, nil
	}
	if s.cache != nil {
		out, err := s.queryCache(ctx, uniq)
		if err == nil {
			return out, nil
		}
		// Con Dragonfly caído los heartbeats también degradan al UPSERT, así
		// que PostgreSQL vuelve a estar fresco en <= un intervalo de heartbeat.
		log.From(ctx).Warn("presencia: Dragonfly no responde, degradando a PostgreSQL", "error", err)
	}
	found, err := s.store.GetPresence(ctx, uniq, s.ttl)
	if err != nil {
		return nil, err
	}
	bySub := make(map[string]domain.Presence, len(found))
	for _, p := range found {
		bySub[p.Subject] = p
	}
	out := make([]domain.Presence, 0, len(uniq))
	for _, sub := range uniq {
		if p, ok := bySub[sub]; ok {
			out = append(out, p)
		} else {
			out = append(out, domain.Presence{Subject: sub, State: domain.StateOffline})
		}
	}
	return out, nil
}

// queryCache resuelve la consulta desde Dragonfly: clave viva => estado
// declarado; clave expirada o ausente => offline.
func (s *Service) queryCache(ctx context.Context, uniq []string) ([]domain.Presence, error) {
	t := tenancy.MustFrom(ctx)
	out := make([]domain.Presence, 0, len(uniq))

	keys := make([]string, len(uniq))
	for i, sub := range uniq {
		keys[i] = key(t.Slug, sub)
	}

	vals, err := s.cache.MGet(ctx, keys...)
	if err != nil {
		return nil, err
	}

	for _, sub := range uniq {
		k := key(t.Slug, sub)
		p := domain.Presence{Subject: sub, State: domain.StateOffline}
		if val, ok := vals[k]; ok {
			if err := json.Unmarshal([]byte(val), &p); err != nil {
				// Valor corrupto: tratar como offline en vez de romper la consulta.
				p = domain.Presence{Subject: sub, State: domain.StateOffline}
			}
			p.Subject = sub
		}
		out = append(out, p)
	}
	return out, nil
}

// dedupe elimina duplicados y vacíos preservando el orden de aparición.
func dedupe(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
