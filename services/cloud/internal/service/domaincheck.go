// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// DomainCheckInterval es el tope del «Comprobar ahora»: una comprobación cada
// diez segundos por dominio. Cada una son varias consultas DNS a resolvedores
// ajenos; sin tope, un botón pulsado en bucle (o un script contra la API) las
// convertiría en ruido sin acelerar nada, porque el DNS propaga a su ritmo.
const DomainCheckInterval = 10 * time.Second

// DomainVerifier comprueba el DNS de un dominio propio y devuelve el motivo. Lo
// satisfacen controller.DNSDomainVerifier (producción) y controller.StubPlatform
// (dev); se declara aquí porque controller ya importa service.
type DomainVerifier interface {
	VerifyDomain(ctx context.Context, hostname, token, ingressTarget string) (domain.DomainCheck, error)
}

// domainCheckState guarda el verificador y la última comprobación de cada
// dominio. El tope es por réplica de la API: con varias, alguien podría colar
// una comprobación más cada diez segundos, que es inocuo; lo que importa es que
// un bucle no las dispare sin freno.
type domainCheckState struct {
	verifier DomainVerifier
	mu       sync.Mutex
	last     map[uuid.UUID]time.Time
	now      func() time.Time
}

// SetDomainVerifier cablea el verificador DNS del «Comprobar ahora» (cloud-api).
func (s *Service) SetDomainVerifier(v DomainVerifier) { s.domainCheck.verifier = v }

// reserve apunta una comprobación del dominio si ya pasó el intervalo; si no,
// devuelve cuánto falta.
func (st *domainCheckState) reserve(id uuid.UUID) time.Duration {
	st.mu.Lock()
	defer st.mu.Unlock()
	now := time.Now()
	if st.now != nil {
		now = st.now()
	}
	if st.last == nil {
		st.last = map[uuid.UUID]time.Time{}
	}
	if prev, ok := st.last[id]; ok {
		if wait := DomainCheckInterval - now.Sub(prev); wait > 0 {
			return wait
		}
	}
	// Barrido de lo caducado: el mapa no crece con cada dominio comprobado alguna
	// vez, solo con los de los últimos diez segundos.
	for k, t := range st.last {
		if now.Sub(t) >= DomainCheckInterval {
			delete(st.last, k)
		}
	}
	st.last[id] = now
	return 0
}

// VerifyDomainNow comprueba AHORA el DNS de un dominio propio y devuelve el motivo.
//
// Un dominio pendiente esperaba al bucle del controller sin forma de pedirlo ni
// de saber qué fallaba: «pendiente» igual podía ser que faltaba el TXT, que tenía
// el token de un intento anterior o que todo estaba bien y solo tocaba esperar.
// Si el TXT es correcto el dominio pasa a `verified` aquí mismo; el controller
// lo encuentra ya verificado en su siguiente vuelta (cada pocos segundos) y pide
// el certificado. El evento cloud.domain.verified es para los consumidores del
// bus (webhooks, auditoría externa): el controller no lo escucha, va por tick.
//
// La comprobación es la misma que la del bucle (mismo código, mismo resolvedor),
// así que el panel nunca dice «ok» de un dominio que el controller rechazaría.
func (s *Service) VerifyDomainNow(ctx context.Context, id uuid.UUID) (domain.Domain, domain.DomainCheck, error) {
	t := tenancy.MustFrom(ctx)
	d, err := s.store.GetDomain(ctx, id) // bajo RLS: un id de otro tenant da ErrNotFound
	if err != nil {
		return domain.Domain{}, domain.DomainCheck{}, err
	}
	if d.Status == domain.DomainFailed {
		// Caducado o perdido frente a otro proyecto: se vuelve a añadir.
		return domain.Domain{}, domain.DomainCheck{}, fmt.Errorf("%w: el dominio caducó; vuelve a añadirlo", domain.ErrInvalidState)
	}
	v := s.domainCheck.verifier
	if v == nil {
		return domain.Domain{}, domain.DomainCheck{}, fmt.Errorf("%w: comprobación DNS no disponible", domain.ErrInvalidState)
	}
	if wait := s.domainCheck.reserve(d.ID); wait > 0 {
		return domain.Domain{}, domain.DomainCheck{}, domain.ErrTooSoon
	}
	check, err := v.VerifyDomain(ctx, d.Hostname, d.VerifyToken, s.cfg.IngressTarget)
	if err != nil {
		return domain.Domain{}, domain.DomainCheck{}, fmt.Errorf("service: comprobar dominio: %w", err)
	}
	if check.Verified && d.Status == domain.DomainPending {
		if err := s.store.SetDomainStatus(ctx, t.Slug, d.ID, domain.DomainVerified); err != nil {
			if errors.Is(err, domain.ErrDomainTaken) {
				// Otro proyecto lo verificó antes: esta reclamación se cierra, como
				// hace el bucle del controller en el mismo caso.
				_ = s.store.SetDomainStatus(ctx, t.Slug, d.ID, domain.DomainFailed)
			}
			return domain.Domain{}, domain.DomainCheck{}, err
		}
		now := time.Now().UTC()
		d.Status, d.VerifiedAt = domain.DomainVerified, &now
		_ = s.emit(ctx, EventDomainVerified, t.Slug, map[string]any{"domain_id": d.ID, "hostname": d.Hostname})
	}
	s.audit(ctx, "domain.verify", d.Hostname, map[string]any{"id": d.ID, "reason": string(check.Reason)})
	return d, check, nil
}
