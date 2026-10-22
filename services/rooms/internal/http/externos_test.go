// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
)

// Entrar en una conversación de OTRA organización cambia el tenant a la anfitriona
// y QUITA los roles del token: un owner de la suya no es nadie en la anfitriona.
// Si este test se pone en verde con los roles puestos, el vídeo le daría a
// cualquier admin de fuera la moderación y las grabaciones de la sala.
func TestEnAnfitrion_CambiaDeOrganizacionSinRoles(t *testing.T) {
	f := &fakeService{anfitrion: "otra"}
	var gotTenant string
	var gotClaims authn.Claims
	mw := enAnfitrionPor(f, "channelID", func(id uuid.UUID) service.RefAnfitrion {
		return service.RefAnfitrion{Canal: id}
	})
	h := mw(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		tn, _ := tenancy.From(r.Context())
		gotTenant = tn.Slug
		gotClaims, _ = authn.FromContext(r.Context())
	}))

	c := claims()
	c.Roles = []string{"owner", "admin"}
	r := req(http.MethodGet, "/", "", c, channelID(uuid.NewString()))
	r = r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: "acme"}))
	h.ServeHTTP(httptest.NewRecorder(), r)

	if gotTenant != "otra" || gotClaims.Tenant != "otra" {
		t.Fatalf("tenant = %q / claims %q, quiero la anfitriona «otra»", gotTenant, gotClaims.Tenant)
	}
	if len(gotClaims.Roles) != 0 || gotClaims.HasAnyRole("owner", "admin") {
		t.Fatalf("los roles de su organización no pueden viajar a la anfitriona: %v", gotClaims.Roles)
	}
	if gotClaims.Subject != "user-1" {
		t.Fatalf("la identidad sí se conserva: %q", gotClaims.Subject)
	}

	// Sin conversación de fuera, la petición no se toca.
	f.anfitrion = ""
	r = req(http.MethodGet, "/", "", c, channelID(uuid.NewString()))
	r = r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: "acme"}))
	h.ServeHTTP(httptest.NewRecorder(), r)
	if gotTenant != "acme" || !gotClaims.HasAnyRole("owner") {
		t.Fatalf("en su propia organización nada cambia: tenant=%q roles=%v", gotTenant, gotClaims.Roles)
	}
}

func TestResolvePerson(t *testing.T) {
	f := &fakeService{persona: service.Persona{UserID: "u-9", Name: "Bea", Username: "bea"}}
	rec := httptest.NewRecorder()
	newHandlers(f).ResolvePerson(rec, req(http.MethodPost, "/", `{"query":"@bea"}`, claims(), nil))
	if rec.Code != http.StatusOK || decode(t, rec)["user_id"] != "u-9" {
		t.Fatalf("code=%d body=%v", rec.Code, decode(t, rec))
	}

	for err, quiere := range map[error]int{
		domain.ErrNotFound:       http.StatusNotFound,
		domain.ErrTooManyLookups: http.StatusTooManyRequests,
	} {
		rec = httptest.NewRecorder()
		newHandlers(&fakeService{personaErr: err}).ResolvePerson(rec, req(http.MethodPost, "/", `{"query":"@bea"}`, claims(), nil))
		if rec.Code != quiere {
			t.Errorf("%v: code=%d, quiero %d", err, rec.Code, quiere)
		}
	}
	rec = httptest.NewRecorder()
	newHandlers(f).ResolvePerson(rec, req(http.MethodPost, "/", `{"query":"  "}`, claims(), nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("consulta vacía: code=%d", rec.Code)
	}
}
