// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

// WEB-08. Los slugs de las webs no se acaparan: los nombres reservados de la plataforma
// no se dan (ni al crear ni al renombrar), el de una web borrada no vuelve a quedar libre
// para otra organización, y una organización tiene como mucho 20 webs (contando las
// borradas que aún se pueden restaurar) y 5 nuevas al día.
func TestLosSlugsDeLasWebsNoSeAcaparan(t *testing.T) {
	dosRoles(t, func(t *testing.T, s *Store, super *pg.Pool) {
		ctx := context.Background()
		crear := func(org, slug string) (*domain.SitioCompleto, error) {
			return s.CrearSitio(ctx, org, domain.CrearSitioInput{Name: "Web " + slug, Slug: slug}, "sub-"+org)
		}

		for _, slug := range []string{"login", "soporte", "nas-acme"} {
			if _, err := crear("a", slug); !errors.Is(err, domain.ErrConflictoBorrador) {
				t.Fatalf("crear %q: %v; quiero que esté reservado", slug, err)
			}
			if libre, err := s.ComprobarSlugDisponible(ctx, slug); err != nil || libre {
				t.Fatalf("%q sale disponible (%v)", slug, err)
			}
		}
		propio, err := crear("a", "panaderia")
		if err != nil {
			t.Fatal(err)
		}
		admin := "admin"
		if _, err := s.ActualizarAjustesSitio(ctx, "a", propio.ID, domain.ActualizarSitioInput{Slug: &admin}, true); !errors.Is(err, domain.ErrConflictoBorrador) {
			t.Fatalf("renombrar a %q: %v; quiero que esté reservado", admin, err)
		}

		// Una web borrada hace más de 30 días: su slug sigue siendo suyo.
		if err := s.BorrarSitio(ctx, "a", propio.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := super.Exec(ctx, `UPDATE sites SET deleted_at = now() - interval '31 days' WHERE id = $1`, propio.ID); err != nil {
			t.Fatal(err)
		}
		if libre, err := s.ComprobarSlugDisponible(ctx, "panaderia"); err != nil || libre {
			t.Fatalf("el slug de una web borrada sale disponible (%v)", err)
		}
		if _, err := crear("b", "panaderia"); !errors.Is(err, domain.ErrConflictoBorrador) {
			t.Fatalf("otra organización toma el slug de una web borrada: %v", err)
		}

		// Topes: 5 nuevas al día y 20 a la vez, con las borradas restaurables dentro.
		envejecer := func() {
			t.Helper()
			if _, err := super.Exec(ctx, `UPDATE sites SET created_at = now() - interval '2 days' WHERE tenant = 'c'`); err != nil {
				t.Fatal(err)
			}
		}
		for i := 0; i < 20; i++ {
			if i > 0 && i%5 == 0 {
				if _, err := crear("c", fmt.Sprintf("c-hoy-%d", i)); !errors.Is(err, domain.ErrLimiteSitios) {
					t.Fatalf("la sexta web del día: %v; quiero ErrLimiteSitios", err)
				}
				envejecer()
			}
			if _, err := crear("c", fmt.Sprintf("c-%d", i)); err != nil {
				t.Fatalf("web %d: %v", i+1, err)
			}
		}
		envejecer()
		if _, err := crear("c", "c-21"); !errors.Is(err, domain.ErrLimiteSitios) {
			t.Fatalf("la web 21: %v; quiero ErrLimiteSitios", err)
		}
		var borrada domain.SitioCompleto
		if err := super.QueryRow(ctx, `SELECT id FROM sites WHERE tenant = 'c' AND slug = 'c-0'`).Scan(&borrada.ID); err != nil {
			t.Fatal(err)
		}
		if err := s.BorrarSitio(ctx, "c", borrada.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := crear("c", "c-21"); !errors.Is(err, domain.ErrLimiteSitios) {
			t.Fatalf("con una borrada restaurable: %v; sigue contando", err)
		}
		if _, err := super.Exec(ctx, `UPDATE sites SET deleted_at = now() - interval '31 days' WHERE id = $1`, borrada.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := crear("c", "c-21"); err != nil {
			t.Fatalf("pasados los 30 días de la borrada, la web 20: %v", err)
		}
	})
}
