// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/integrations/db/seeds"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
)

// La salida admite los dos destinos que tienen escritor; sin esto el asistente
// ofrecía Microsoft 365 y el alta del trabajo lo rechazaba con un 400.
func TestValidOutbound(t *testing.T) {
	ficheros := []domain.MigrationDomain{domain.DomainDrive, domain.DomainContacts}
	for _, src := range []domain.MigrationSource{domain.SourceGoogle, domain.SourceM365} {
		if err := validOutbound(src, ficheros); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
	if validOutbound(domain.SourceGitHub, ficheros) == nil {
		t.Error("GitHub no es un destino de salida")
	}
}

// TestMigracionesDeOrganizacionExigenRolAdmin comprueba en los propios handlers que
// lanzar, planificar, listar, cancelar o reanudar migraciones de organización (incluida
// la salida delegada) exige rol owner o admin.
func TestMigracionesDeOrganizacionExigenRolAdmin(t *testing.T) {
	h := &Handlers{}
	miembro := authn.Into(context.Background(), authn.Claims{
		Subject: "u-miembro",
		Email:   "ana@acme.eu",
		Tenant:  "acme",
		Roles:   []string{"member"},
	})
	casos := []struct {
		nombre  string
		metodo  string
		ruta    string
		cuerpo  string
		handler http.HandlerFunc
	}{
		{
			nombre:  "crear migración de organización (salida delegada)",
			metodo:  http.MethodPost,
			ruta:    "/api/v1/migrations",
			cuerpo:  `{"instance_id":"11111111-1111-1111-1111-111111111111","source":"google","direction":"out","domains":["drive"],"targets":[{"source_user":"ana@acme.com","target_user":"ana@acme.eu"}]}`,
			handler: h.CreateMigration,
		},
		{
			nombre:  "asistente automático de organización",
			metodo:  http.MethodPost,
			ruta:    "/api/v1/migrations/auto",
			cuerpo:  `{"instance_id":"11111111-1111-1111-1111-111111111111"}`,
			handler: h.AutoMigrate,
		},
		{
			nombre:  "plan de migración de organización",
			metodo:  http.MethodGet,
			ruta:    "/api/v1/instances/11111111-1111-1111-1111-111111111111/migration-plan",
			handler: h.MigrationPlan,
		},
		{
			nombre:  "salida hacia conexión de organización",
			metodo:  http.MethodPost,
			ruta:    "/api/v1/migrations/self/out",
			cuerpo:  `{"instance_id":"11111111-1111-1111-1111-111111111111","domains":["drive"]}`,
			handler: h.CreateSelfOutbound,
		},
		{
			nombre:  "listar migraciones de organización",
			metodo:  http.MethodGet,
			ruta:    "/api/v1/migrations",
			handler: h.ListMigrations,
		},
		{
			nombre:  "cancelar migración de organización",
			metodo:  http.MethodPost,
			ruta:    "/api/v1/migrations/11111111-1111-1111-1111-111111111111/cancel",
			handler: h.CancelMigration,
		},
		{
			nombre:  "reanudar migración de organización",
			metodo:  http.MethodPost,
			ruta:    "/api/v1/migrations/11111111-1111-1111-1111-111111111111/resume",
			handler: h.ResumeMigration,
		},
	}
	for _, tc := range casos {
		t.Run(tc.nombre, func(t *testing.T) {
			req := httptest.NewRequest(tc.metodo, tc.ruta, strings.NewReader(tc.cuerpo)).WithContext(miembro)
			rec := httptest.NewRecorder()
			tc.handler(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s: código = %d, quiero 403 Forbidden para miembro sin rol de admin", tc.nombre, rec.Code)
			}
		})
	}
}

// TestManifiestosDeProveedoresDeclaranSoloHostsOficiales comprueba que las semillas de
// Google Workspace y Microsoft 365 declaran sus hosts oficiales y rechazan terceros.
func TestManifiestosDeProveedoresDeclaranSoloHostsOficiales(t *testing.T) {
	lista, err := seeds.Load()
	if err != nil {
		t.Fatal(err)
	}
	porClave := map[string]*manifest.Manifest{}
	for _, s := range lista {
		m, err := manifest.Parse(s.Manifest)
		if err != nil {
			t.Fatalf("%s: %v", s.Key, err)
		}
		porClave[s.Key] = m
	}

	gws := porClave["google-workspace"].HostsOficiales(map[string]string{"domain": "acme.com"})
	for _, h := range []string{"www.googleapis.com", "people.googleapis.com", "oauth2.googleapis.com", "storage.googleapis.com"} {
		if !manifest.HostPermitido(h, gws) {
			t.Errorf("google-workspace debería permitir su host oficial %q (lista: %v)", h, gws)
		}
	}
	if manifest.HostPermitido("evil.example", gws) {
		t.Error("google-workspace permitió un host ajeno al manifiesto")
	}

	m365 := porClave["microsoft365"].HostsOficiales(nil)
	for _, h := range []string{"graph.microsoft.com", "login.microsoftonline.com", "acme-my.sharepoint.com", "api.1drv.com"} {
		if !manifest.HostPermitido(h, m365) {
			t.Errorf("microsoft365 debería permitir su host oficial %q (lista: %v)", h, m365)
		}
	}
	if manifest.HostPermitido("evil.example", m365) {
		t.Error("microsoft365 permitió un host ajeno al manifiesto")
	}
}
