// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// verificadorFijo contesta lo mismo que controller.StubPlatform con DomainChecks
// (no se puede importar controller aquí: controller importa service) y cuenta las
// llamadas, que es lo que permite afirmar el tope.
type verificadorFijo struct {
	check  domain.DomainCheck
	target string
	calls  int
}

func (v *verificadorFijo) VerifyDomain(_ context.Context, _, _, target string) (domain.DomainCheck, error) {
	v.calls++
	v.target = target
	return v.check, nil
}

func dominioPendiente() domain.Domain {
	return domain.Domain{
		ID: uuid.New(), Hostname: "www.example.com", Status: domain.DomainPending,
		VerifyToken: "sytue-verify=abc",
	}
}

// Con el TXT bien, el dominio pasa a `verified` en la misma petición (el objetivo
// es no esperar a que el bucle lo verifique) y se publica cloud.domain.verified.
func TestVerifyDomainNowVerificaSinEsperarAlBucle(t *testing.T) {
	fs := &fakeStore{dom: dominioPendiente()}
	pub := &capturePub{}
	s := newTestService(fs, pub)
	v := &verificadorFijo{check: domain.DomainCheck{Verified: true, Reason: domain.DomainCheckOK}}
	s.SetDomainVerifier(v)

	d, check, err := s.VerifyDomainNow(tenantCtx(t), fs.dom.ID)
	if err != nil {
		t.Fatalf("VerifyDomainNow: %v", err)
	}
	if check.Reason != domain.DomainCheckOK || d.Status != domain.DomainVerified || d.VerifiedAt == nil {
		t.Fatalf("= %+v / %s; quiero ok y verified", check, d.Status)
	}
	if len(fs.domStatuses) != 1 || fs.domStatuses[0] != domain.DomainVerified {
		t.Errorf("estados guardados = %v; quiero [verified]", fs.domStatuses)
	}
	if len(pub.types) != 1 || pub.types[0] != EventDomainVerified {
		t.Errorf("eventos = %v; quiero [cloud.domain.verified]", pub.types)
	}
	if v.target != "ingress.sytue.eu" {
		t.Errorf("ingress pasado = %q; el aviso del CNAME necesita el destino real", v.target)
	}
}

// Sin TXT (o sin CNAME) se devuelve el motivo y el dominio NO cambia de estado...
// salvo cname_distinto, que prueba la propiedad: se verifica y se avisa.
func TestVerifyDomainNowDevuelveElMotivo(t *testing.T) {
	casos := []struct {
		nombre  string
		check   domain.DomainCheck
		estados []domain.DomainStatus
	}{
		{"sin TXT", domain.DomainCheck{Reason: domain.DomainCheckTXTMissing}, nil},
		{"TXT con otro token", domain.DomainCheck{Reason: domain.DomainCheckTXTMismatch}, nil},
		{"sin CNAME al ingress", domain.DomainCheck{
			Verified: true, Reason: domain.DomainCheckCNAMEMismatch,
			Expected: "ingress.sytue.eu", Found: "otro.example",
		}, []domain.DomainStatus{domain.DomainVerified}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			fs := &fakeStore{dom: dominioPendiente()}
			s := newTestService(fs, &capturePub{})
			s.SetDomainVerifier(&verificadorFijo{check: c.check})
			_, got, err := s.VerifyDomainNow(tenantCtx(t), fs.dom.ID)
			if err != nil {
				t.Fatalf("VerifyDomainNow: %v", err)
			}
			if got != c.check {
				t.Errorf("check = %+v; quiero %+v", got, c.check)
			}
			if len(fs.domStatuses) != len(c.estados) {
				t.Errorf("estados guardados = %v; quiero %v", fs.domStatuses, c.estados)
			}
		})
	}
}

// Una comprobación cada diez segundos por dominio: la segunda dentro del
// intervalo no llega al DNS; pasado el intervalo, sí. Y el tope es POR dominio.
func TestVerifyDomainNowTope(t *testing.T) {
	fs := &fakeStore{dom: dominioPendiente()}
	s := newTestService(fs, nil)
	v := &verificadorFijo{check: domain.DomainCheck{Reason: domain.DomainCheckTXTMissing}}
	s.SetDomainVerifier(v)
	reloj := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	s.domainCheck.now = func() time.Time { return reloj }
	ctx := tenantCtx(t)

	if _, _, err := s.VerifyDomainNow(ctx, fs.dom.ID); err != nil {
		t.Fatalf("primera: %v", err)
	}
	reloj = reloj.Add(9 * time.Second)
	if _, _, err := s.VerifyDomainNow(ctx, fs.dom.ID); !errors.Is(err, domain.ErrTooSoon) {
		t.Fatalf("segunda a los 9 s = %v; quiero ErrTooSoon", err)
	}
	if v.calls != 1 {
		t.Errorf("consultas DNS = %d; la bloqueada no puede llegar al DNS", v.calls)
	}

	otro := dominioPendiente()
	fs.dom = otro
	if _, _, err := s.VerifyDomainNow(ctx, otro.ID); err != nil {
		t.Errorf("otro dominio en el mismo segundo: %v; el tope es por dominio", err)
	}

	reloj = reloj.Add(time.Second)
	if _, _, err := s.VerifyDomainNow(ctx, otro.ID); !errors.Is(err, domain.ErrTooSoon) {
		t.Errorf("el segundo dominio también tiene su tope: %v", err)
	}
}

// Lo que no se puede comprobar se dice con su error, no con un «ok» vacío.
func TestVerifyDomainNowErrores(t *testing.T) {
	t.Run("dominio caducado", func(t *testing.T) {
		d := dominioPendiente()
		d.Status = domain.DomainFailed
		s := newTestService(&fakeStore{dom: d}, nil)
		s.SetDomainVerifier(&verificadorFijo{})
		if _, _, err := s.VerifyDomainNow(tenantCtx(t), d.ID); !errors.Is(err, domain.ErrInvalidState) {
			t.Errorf("caducado = %v; quiero ErrInvalidState", err)
		}
	})
	t.Run("de otro tenant", func(t *testing.T) {
		s := newTestService(&fakeStore{domErr: domain.ErrNotFound}, nil)
		s.SetDomainVerifier(&verificadorFijo{})
		if _, _, err := s.VerifyDomainNow(tenantCtx(t), uuid.New()); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("= %v; quiero ErrNotFound", err)
		}
	})
	t.Run("ya reclamado por otro proyecto", func(t *testing.T) {
		fs := &fakeStore{dom: dominioPendiente(), domStatusErr: domain.ErrDomainTaken}
		s := newTestService(fs, nil)
		s.SetDomainVerifier(&verificadorFijo{check: domain.DomainCheck{Verified: true, Reason: domain.DomainCheckOK}})
		if _, _, err := s.VerifyDomainNow(tenantCtx(t), fs.dom.ID); !errors.Is(err, domain.ErrDomainTaken) {
			t.Fatalf("= %v; quiero ErrDomainTaken", err)
		}
		if n := len(fs.domStatuses); n != 2 || fs.domStatuses[1] != domain.DomainFailed {
			t.Errorf("estados = %v; la reclamación perdedora se cierra como en el bucle", fs.domStatuses)
		}
	})
}
