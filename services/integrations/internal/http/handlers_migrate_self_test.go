// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// TestSelfNoCaeEnLaRutaDeAdministracion es la comprobación que no se puede suponer:
// `/migrations/self` y `/migrations/{id}` conviven en el MISMO árbol de chi, pero en
// grupos con middleware distinto. Si el literal no ganara al parámetro, la petición de
// una persona normal entraría por el grupo de administración y recibiría un 403 sin que
// nada en el código lo delatara.
func TestSelfNoCaeEnLaRutaDeAdministracion(t *testing.T) {
	routes, ok := NewRouter(Deps{}).(chi.Routes)
	if !ok {
		t.Fatal("NewRouter debería devolver un chi.Routes")
	}
	for _, c := range []struct{ metodo, ruta string }{
		{http.MethodPost, "/api/v1/migrations/self"},
		{http.MethodGet, "/api/v1/migrations/self"},
	} {
		rctx := chi.NewRouteContext()
		if !routes.Match(rctx, c.metodo, c.ruta) {
			t.Fatalf("%s %s no casa con ninguna ruta", c.metodo, c.ruta)
		}
		if got := rctx.RoutePattern(); !strings.HasSuffix(got, "/migrations/self") {
			t.Fatalf("%s %s resolvió a %q: la ruta de administración se la quedó", c.metodo, c.ruta, got)
		}
	}
}

// TestSelfSinIdentidadNoMigra: sin sub o sin correo no hay a qué cuenta mirar. Migrar
// «algo» en ese estado tocaría datos de quien no se sabe.
func TestSelfSinIdentidadNoMigra(t *testing.T) {
	h := &Handlers{}
	for nombre, ctx := range map[string]context.Context{
		"sin claims":  context.Background(),
		"sin correo":  authn.Into(context.Background(), authn.Claims{Subject: "u-1"}),
		"sin subject": authn.Into(context.Background(), authn.Claims{Email: "ana@ejemplo.test"}),
	} {
		t.Run(nombre, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/migrations/self",
				strings.NewReader(`{"instance_id":"11111111-1111-1111-1111-111111111111","source":"google","domains":["mail"]}`)).
				WithContext(ctx)
			rec := httptest.NewRecorder()
			h.CreateSelfMigration(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("código = %d, quiero 401", rec.Code)
			}
		})
	}
}

// TestSelfSoloContenidoPersonal: los repositorios, los equipos y la nube pertenecen a la
// organización. Que una persona los migrara «para sí» copiaría contenido de todos.
func TestSelfSoloContenidoPersonal(t *testing.T) {
	for _, d := range []domain.MigrationDomain{
		domain.DomainRepos, domain.DomainGists, domain.DomainTeams,
		domain.DomainTeamFiles, domain.DomainCloud,
	} {
		if _, ok := selfMigrationDomains[d]; ok {
			t.Fatalf("%q es contenido de la organización y se dejó en la puerta self-service", d)
		}
	}
	for _, d := range []domain.MigrationDomain{
		domain.DomainMail, domain.DomainDrive, domain.DomainCalendar, domain.DomainContacts,
	} {
		if _, ok := selfMigrationDomains[d]; !ok {
			t.Fatalf("%q es contenido personal y debería poder traerse solo", d)
		}
	}
}

// TestSelfRechazaContenidoDeLaOrganizacion comprueba la verja en el handler, no solo en
// la tabla: es la que responde a una petición fabricada a mano.
func TestSelfRechazaContenidoDeLaOrganizacion(t *testing.T) {
	h := &Handlers{}
	ctx := authn.Into(context.Background(), authn.Claims{
		Subject: "u-1", Email: "ana@ejemplo.test",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/migrations/self",
		strings.NewReader(`{"instance_id":"11111111-1111-1111-1111-111111111111","source":"github","domains":["repos"]}`)).
		WithContext(ctx)
	rec := httptest.NewRecorder()
	h.CreateSelfMigration(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("código = %d, quiero 403 (contenido de la organización)", rec.Code)
	}
}
