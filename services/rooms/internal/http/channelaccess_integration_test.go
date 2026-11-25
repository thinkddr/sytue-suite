// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package chathttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/personas"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// directorioDeDos es un Sytue ID de mentira con Ana (acme) y Bea (otra).
type directorioDeDos map[string]personas.Persona

func (d directorioDeDos) Resolver(context.Context, string) (personas.Persona, error) {
	return personas.Persona{}, domain.ErrNotFound
}

func (d directorioDeDos) PorID(_ context.Context, id string) (personas.Persona, error) {
	if p, ok := d[id]; ok {
		return p, nil
	}
	return personas.Persona{}, domain.ErrNotFound
}

// La verja de los adjuntos que consulta Drive (ADR 0040): a quien está en la
// conversación de su organización le responde 204, como siempre; a la persona de fuera,
// 200 con la organización ANFITRIONA, que es donde viven los adjuntos; a quien no está,
// 404 venga de donde venga.
func TestChannelAccessDevuelveLaAnfitriona(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	s := service.New(config.Config{MaxMessageBytes: 4096, MessagePageSize: 50}, st, nil, nil)
	s.SetDirectorio(directorioDeDos{
		"ana": {UserID: "ana", Name: "Ana Acme", Username: "ana"},
		"bea": {UserID: "bea", Name: "Bea Otra", Username: "bea"},
	})
	acme := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	otra := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "otra"})
	equiposAcme, err := s.EnsureDefaultTeams(acme, "ana", "Ana")
	if err != nil {
		t.Fatal(err)
	}
	equiposOtra, err := s.EnsureDefaultTeams(otra, "bea", "Bea")
	if err != nil {
		t.Fatal(err)
	}
	dm, err := s.OpenGroupDirect(acme, equiposAcme[0].ID, "ana", []string{"bea"})
	if err != nil {
		t.Fatal(err)
	}
	// Abrir ella misma el DM desde la suya es aceptarlo.
	if _, err := s.OpenGroupDirect(otra, equiposOtra[0].ID, "bea", []string{"ana"}); err != nil {
		t.Fatal(err)
	}

	const secreto = "s3cr3to-interno"
	r := NewRouter(Deps{Cfg: config.Config{InternalSecret: secreto}, Service: s})
	preguntar := func(tenant, subject string) *httptest.ResponseRecorder {
		q := url.Values{"tenant": {tenant}, "channel": {dm.ID.String()}, "subject": {subject}}
		req := httptest.NewRequest(http.MethodGet, "/internal/channel-access?"+q.Encode(), http.NoBody)
		req.Header.Set("Authorization", "Bearer "+secreto)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	if rec := preguntar("acme", "ana"); rec.Code != http.StatusNoContent {
		t.Fatalf("Ana, en su organización => %d, quiero 204", rec.Code)
	}
	rec := preguntar("otra", "bea")
	var cuerpo struct {
		Tenant string `json:"tenant"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &cuerpo) != nil || cuerpo.Tenant != "acme" {
		t.Fatalf("Bea, desde la suya => %d %s, quiero 200 con acme", rec.Code, rec.Body.String())
	}
	for _, c := range []struct{ tenant, subject string }{{"otra", "carla"}, {"acme", "carla"}, {"otra", "ana"}} {
		if rec := preguntar(c.tenant, c.subject); rec.Code != http.StatusNotFound {
			t.Fatalf("%s desde %s => %d, quiero 404", c.subject, c.tenant, rec.Code)
		}
	}
}
