// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/pages/db"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

// WEB-01. El canónico de un sitio solo puede ser un dominio ACTIVO de ese mismo sitio, y
// deja de serlo en cuanto el dominio deja de estar activo. Antes era texto libre del
// navegador y bastaba escribir el host de otro para quedarse con él.
func TestCanonicoSoloUnDominioActivoDelSitio(t *testing.T) {
	dosRoles(t, func(t *testing.T, s *Store, super *pg.Pool) {
		ctx := context.Background()
		sitio := func(tenant, slug string) uuid.UUID {
			t.Helper()
			sc, err := s.CrearSitio(ctx, tenant, domain.CrearSitioInput{Name: slug, Slug: slug}, "sub-"+tenant)
			if err != nil {
				t.Fatalf("crear %s: %v", slug, err)
			}
			return sc.ID
		}
		dominio := func(tenant string, sitio uuid.UUID, host string, estado domain.EstadoDominio) *domain.Domain {
			t.Helper()
			d := &domain.Domain{Tenant: tenant, SiteID: sitio, Host: host, Token: "0123456789abcdef0123456789abcdef", CreatedBy: "sub-" + tenant}
			if err := s.CrearDominio(ctx, d); err != nil {
				t.Fatalf("crear dominio %s: %v", host, err)
			}
			if estado != domain.DomainStatusPending {
				d.Status = estado
				if err := s.ActualizarDominio(ctx, d); err != nil {
					t.Fatalf("dominio %s a %s: %v", host, estado, err)
				}
			}
			return d
		}
		canonico := func(sitio uuid.UUID) string {
			t.Helper()
			var c *string
			if err := super.QueryRow(ctx, `SELECT canonical_host FROM sites WHERE id = $1`, sitio).Scan(&c); err != nil {
				t.Fatal(err)
			}
			if c == nil {
				return ""
			}
			return *c
		}

		victima := sitio("victima", "victima")
		atacante := sitio("atacante", "atacante")
		dominio("victima", victima, "www.victima.example", domain.DomainStatusActive)

		// Nada que no sea un dominio activo PROPIO.
		for _, host := range []string{"victima.sites.example.test", "www.victima.example", "nadie.example"} {
			if err := s.FijarCanonico(ctx, "atacante", atacante, host); !errors.Is(err, domain.ErrNoExiste) {
				t.Fatalf("FijarCanonico(%s) = %v; want ErrNoExiste", host, err)
			}
		}
		propio := dominio("atacante", atacante, "www.atacante.example", domain.DomainStatusPending)
		if err := s.FijarCanonico(ctx, "atacante", atacante, propio.Host); !errors.Is(err, domain.ErrNoExiste) {
			t.Fatalf("un dominio pendiente se aceptó como canónico: %v", err)
		}
		propio.Status = domain.DomainStatusVerified
		if err := s.ActualizarDominio(ctx, propio); err != nil {
			t.Fatal(err)
		}
		if err := s.FijarCanonico(ctx, "atacante", atacante, propio.Host); !errors.Is(err, domain.ErrNoExiste) {
			t.Fatalf("un dominio verificado pero no activo se aceptó como canónico: %v", err)
		}
		if got := canonico(atacante); got != "" {
			t.Fatalf("canónico = %q tras los rechazos", got)
		}

		// Activo y propio: sí. Y solo desde su organización.
		propio.Status = domain.DomainStatusActive
		if err := s.ActualizarDominio(ctx, propio); err != nil {
			t.Fatal(err)
		}
		if err := s.FijarCanonico(ctx, "victima", atacante, propio.Host); !errors.Is(err, domain.ErrNoExiste) {
			t.Fatalf("otra organización fijó el canónico de un sitio ajeno: %v", err)
		}
		if err := s.FijarCanonico(ctx, "atacante", atacante, propio.Host); err != nil {
			t.Fatalf("canónico legítimo rechazado: %v", err)
		}
		if got := canonico(atacante); got != propio.Host {
			t.Fatalf("canónico = %q; want %q", got, propio.Host)
		}

		// Deja de ser activo: deja de ser canónico, en la misma transacción.
		propio.Status = domain.DomainStatusFailed
		if err := s.ActualizarDominio(ctx, propio); err != nil {
			t.Fatal(err)
		}
		if got := canonico(atacante); got != "" {
			t.Fatalf("un dominio fallido sigue de canónico: %q", got)
		}

		// Se borra el dominio: igual.
		propio.Status = domain.DomainStatusActive
		if err := s.ActualizarDominio(ctx, propio); err != nil {
			t.Fatal(err)
		}
		if err := s.FijarCanonico(ctx, "atacante", atacante, propio.Host); err != nil {
			t.Fatal(err)
		}
		if err := s.BorrarDominio(ctx, "atacante", atacante, propio.ID); err != nil {
			t.Fatal(err)
		}
		if got := canonico(atacante); got != "" {
			t.Fatalf("un dominio borrado sigue de canónico: %q", got)
		}

		// Se borra el sitio: sus dominios pasan a fallidos y el canónico se suelta.
		otro := dominio("atacante", atacante, "tienda.atacante.example", domain.DomainStatusActive)
		if err := s.FijarCanonico(ctx, "atacante", atacante, otro.Host); err != nil {
			t.Fatal(err)
		}
		if err := s.BorrarSitio(ctx, "atacante", atacante); err != nil {
			t.Fatal(err)
		}
		if got := canonico(atacante); got != "" {
			t.Fatalf("un sitio borrado conserva el canónico: %q", got)
		}
	})
}

// Una fila que aún traiga un canónico de texto libre (de antes de WEB-01) no resuelve
// nada en la API pública, y la migración 0005 la limpia sin tocar los legítimos.
func TestCanonicoHeredadoNoResuelveYLaMigracionLoLimpia(t *testing.T) {
	dosRoles(t, func(t *testing.T, s *Store, super *pg.Pool) {
		ctx := context.Background()
		v, err := s.CrearSitio(ctx, "victima", domain.CrearSitioInput{Name: "Víctima", Slug: "victima"}, "sub-v")
		if err != nil {
			t.Fatal(err)
		}
		a, err := s.CrearSitio(ctx, "atacante", domain.CrearSitioInput{Name: "Atacante", Slug: "atacante"}, "sub-a")
		if err != nil {
			t.Fatal(err)
		}
		d := &domain.Domain{Tenant: "victima", SiteID: v.ID, Host: "www.victima.example", Token: "0123456789abcdef0123456789abcdef", CreatedBy: "sub-v"}
		if err := s.CrearDominio(ctx, d); err != nil {
			t.Fatal(err)
		}
		d.Status = domain.DomainStatusActive
		if err := s.ActualizarDominio(ctx, d); err != nil {
			t.Fatal(err)
		}
		if err := s.FijarCanonico(ctx, "victima", v.ID, d.Host); err != nil {
			t.Fatal(err)
		}
		// Lo que dejaba el PUT antiguo: el host por slug de la víctima, escrito a mano.
		if _, err := super.Exec(ctx, `UPDATE sites SET canonical_host = 'victima.sites.example.test' WHERE id = $1`, a.ID); err != nil {
			t.Fatal(err)
		}

		// El host por slug de la víctima es de la víctima, nunca del canónico de otro.
		if tenant, id, err := s.ResolverHostPublico(ctx, "victima.sites.example.test", "sites.example.test"); err != nil || tenant != "victima" || id != v.ID {
			t.Fatalf("el host de la víctima resuelve a %s/%s (err=%v)", tenant, id, err)
		}
		if tenant, id, err := s.ResolverHostPublico(ctx, "victima.sites.example.test", ""); !errors.Is(err, domain.ErrNoExiste) {
			t.Fatalf("el host de la víctima resuelve a %s/%s por el canónico de otro (err=%v)", tenant, id, err)
		}
		if tenant, id, err := s.ResolverHostPublico(ctx, d.Host, "sites.example.test"); err != nil || tenant != "victima" || id != v.ID {
			t.Fatalf("el dominio activo de la víctima resuelve a %s/%s (err=%v)", tenant, id, err)
		}

		sql, err := fs.ReadFile(db.Migrations, db.MigrationsDir+"/0005_canonico_verificado.up.sql")
		if err != nil {
			t.Fatal(err)
		}
		if err := super.InTx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, string(sql))
			return err
		}); err != nil {
			t.Fatalf("migración 0005: %v", err)
		}
		var deA, deV *string
		if err := super.QueryRow(ctx, `SELECT canonical_host FROM sites WHERE id = $1`, a.ID).Scan(&deA); err != nil {
			t.Fatal(err)
		}
		if err := super.QueryRow(ctx, `SELECT canonical_host FROM sites WHERE id = $1`, v.ID).Scan(&deV); err != nil {
			t.Fatal(err)
		}
		if deA != nil {
			t.Fatalf("la migración dejó el canónico sin verificar: %q", *deA)
		}
		if deV == nil || *deV != d.Host {
			t.Fatalf("la migración tocó un canónico legítimo: %v", deV)
		}
	})
}
