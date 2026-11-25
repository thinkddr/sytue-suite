// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package integrationshttp

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	integrationsdb "git.sytue.eu/sytue/one/services/integrations/db"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

// TestTraeTusDatosUnaVezPorCuenta recorre la puerta entera contra Postgres: la misma
// cuenta de Google conectada dos veces —y además para Drive— es UN origen; lanzar con
// cada una de sus conexiones deja UN trabajo; y crear una tercera conexión del mismo
// dominio se rechaza con `instance_exists`.
func TestTraeTusDatosUnaVezPorCuenta(t *testing.T) {
	pg := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	ctx := context.Background()
	for _, k := range []string{"google-workspace", "google-drive"} {
		if _, err := pg.Pool.Exec(ctx,
			`INSERT INTO app_templates (key, manifest, tier, enabled) VALUES ($1, '{}'::jsonb, 'standard', true)`, k); err != nil {
			t.Fatalf("plantilla %s: %v", k, err)
		}
	}
	k := make([]byte, keycrypto.KeyLen)
	_, _ = rand.Read(k)
	cifrador, err := keycrypto.New(k)
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(pg.Pool, cifrador)
	acme := tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"})
	conectar := func(plantilla, nombre, cfg string) string {
		inst, err := st.CreateInstance(acme, plantilla, nombre, json.RawMessage(cfg), nil)
		if err != nil {
			t.Fatalf("instancia %s: %v", nombre, err)
		}
		if err := st.PutSecrets(acme, inst.ID, map[string]string{store.SecretKeyBearer: "tok"}); err != nil {
			t.Fatal(err)
		}
		if err := st.SetStatus(acme, inst.ID, domain.StatusActive); err != nil {
			t.Fatal(err)
		}
		return inst.ID
	}
	vieja := conectar("google-workspace", "Google", `{"domain":"acme.com"}`)
	nueva := conectar("google-workspace", "Google otra vez", `{"_import_account":"google:acme.com","_import_account_label":"acme.com"}`)
	conectar("google-drive", "Drive de Google", `{}`)

	h := &Handlers{store: st, fondo: ctx}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := authn.Claims{Subject: "sub-ana", Email: "ana@acme.com", Tenant: "acme", Roles: []string{"admin"}}
			next.ServeHTTP(w, r.WithContext(tenancy.Into(authn.Into(r.Context(), c), tenancy.Tenant{Slug: "acme"})))
		})
	})
	r.Get("/migrations/self/origins", h.ListImportOrigins)
	r.Post("/migrations/self", h.CreateSelfMigration)
	r.Get("/migrations/self", h.ListSelfMigrations)
	r.Post("/instances", h.CreateInstance)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	pedir := func(metodo, ruta, cuerpo string, out any) int {
		t.Helper()
		req, _ := http.NewRequest(metodo, srv.URL+ruta, strings.NewReader(cuerpo))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		crudo, _ := io.ReadAll(resp.Body)
		if out != nil {
			if err := json.Unmarshal(crudo, out); err != nil {
				t.Fatalf("%s %s: %s", metodo, ruta, crudo)
			}
		}
		return resp.StatusCode
	}

	var origenes struct{ Origins []importOrigin }
	if c := pedir(http.MethodGet, "/migrations/self/origins", "", &origenes); c != http.StatusOK ||
		len(origenes.Origins) != 1 || len(origenes.Origins[0].InstanceIDs) != 2 {
		t.Fatalf("orígenes = %d %+v; quiero UNO con las dos conexiones de acme.com", c, origenes)
	}

	var primero, segundo struct{ Migrations []selfJob }
	if c := pedir(http.MethodPost, "/migrations/self", `{"instance_id":"`+vieja+`","source":"google","domains":["mail"]}`, &primero); c != http.StatusAccepted {
		t.Fatalf("lanzar: %d", c)
	}
	if c := pedir(http.MethodPost, "/migrations/self", `{"instance_id":"`+nueva+`","source":"google","domains":["mail"]}`, &segundo); c != http.StatusAccepted {
		t.Fatalf("relanzar: %d", c)
	}
	if len(primero.Migrations) != 1 || len(segundo.Migrations) != 1 || primero.Migrations[0].ID != segundo.Migrations[0].ID {
		t.Fatalf("lanzar dos veces la misma cuenta creó dos trabajos: %+v / %+v", primero, segundo)
	}
	if got := primero.Migrations[0]; got.AccountKey != "google:acme.com" {
		t.Fatalf("el trabajo no dice de qué cuenta es: %+v", got)
	}
	var mios struct{ Migrations []selfJob }
	if c := pedir(http.MethodGet, "/migrations/self", "", &mios); c != http.StatusOK || len(mios.Migrations) != 1 {
		t.Fatalf("mis importaciones = %d %+v", c, mios)
	}

	var rechazo struct {
		Error struct {
			Code   string
			Params map[string]any
		}
	}
	if c := pedir(http.MethodPost, "/instances", `{"template_key":"google-workspace","name":"Otra","config":{"domain":"ACME.com"}}`, &rechazo); c != http.StatusConflict ||
		rechazo.Error.Code != "instance_exists" {
		t.Fatalf("una tercera conexión de acme.com: %d %+v; quiero 409 instance_exists", c, rechazo)
	}
}
