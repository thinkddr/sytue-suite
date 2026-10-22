// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Cambio de organización activa contra un PostgreSQL real (pgtest, build-tag
// `integration`): quien pertenece a dos organizaciones puede pasar a la segunda,
// sus sesiones vivas la adoptan, los refresh tokens que llevaban la anterior
// congelada caen, y las tres verjas del login siguen en pie (membresía, estado
// de la organización y forma del identificador).
package http

import (
	"net/http"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// mkLiveSession abre una sesión SSO en la organización dada, con su refresh
// token vivo colgando: es el estado del que hay que partir para poder ver que
// el cambio de organización arrastra a los dos.
func (l *liveServer) mkLiveSession(t *testing.T, userID, orgID string) (string, string) {
	t.Helper()
	ahora := time.Now()
	sessID, err := l.st.InsertSession(l.ctx, nil, domain.Session{
		TokenHash: domain.HashToken(userID + orgID + "sess"),
		UserID:    userID, OrgID: orgID, AuthTime: ahora,
		AMR: []string{"pwd"}, ExpiresAt: ahora.Add(8 * time.Hour),
	})
	if err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
	rtID, err := l.st.InsertRefreshToken(l.ctx, nil, domain.RefreshToken{
		TokenHash: domain.HashToken(userID + orgID + "rt"),
		ClientID:  "sytue-console", UserID: userID, OrgID: orgID, SessionID: sessID,
		Scope: "openid offline_access", AuthTime: ahora,
		ExpiresAt: ahora.Add(30 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("InsertRefreshToken: %v", err)
	}
	return sessID, rtID
}

func (l *liveServer) orgDeLaSesion(t *testing.T, sessID string) string {
	t.Helper()
	var org string
	if err := l.st.DB().QueryRow(l.ctx,
		`SELECT coalesce(org_id::text,'') FROM sessions WHERE id=$1`, sessID).Scan(&org); err != nil {
		t.Fatalf("leer sessions.org_id: %v", err)
	}
	return org
}

func (l *liveServer) refreshVivo(t *testing.T, rtID string) bool {
	t.Helper()
	var revocado *time.Time
	if err := l.st.DB().QueryRow(l.ctx,
		`SELECT revoked_at FROM refresh_tokens WHERE id=$1`, rtID).Scan(&revocado); err != nil {
		t.Fatalf("leer refresh_tokens.revoked_at: %v", err)
	}
	return revocado == nil
}

// El caso que motivó todo esto: miembro en la organización vieja, owner en la
// nueva. Antes solo podía administrar la vieja —donde no es nadie— y la consola
// le respondía 403 para siempre.
func TestCambiarDeOrganizacionActiva(t *testing.T) {
	l := newLiveServer(t)

	vieja, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "vieja", Name: "Vieja"})
	if err != nil {
		t.Fatalf("CreateOrg(vieja): %v", err)
	}
	nueva, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "nueva", Name: "Nueva"})
	if err != nil {
		t.Fatalf("CreateOrg(nueva): %v", err)
	}
	u := l.mkUser(t, "dos-orgs@example.com", "una-clave-larga-123")
	for _, m := range []domain.Membership{
		{OrgID: vieja.ID, UserID: u.ID, Role: domain.RoleMember},
		{OrgID: nueva.ID, UserID: u.ID, Role: domain.RoleOwner},
	} {
		if err := l.st.UpsertMembership(l.ctx, nil, m); err != nil {
			t.Fatalf("UpsertMembership(%s): %v", m.OrgID, err)
		}
	}
	sessID, rtID := l.mkLiveSession(t, u.ID, vieja.ID)
	tok := l.token(t, u.ID, vieja.Slug, []string{"member"})

	// 1) El listado de /me trae ya con qué pintar el selector: nombre, slug y
	//    cuál está activa. Sin esto no hay lista donde elegir.
	rr, me := l.do(t, http.MethodGet, "/api/v1/me", tok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /me: código=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if me["active_org"] != vieja.Slug {
		t.Fatalf("active_org=%v; quería %q", me["active_org"], vieja.Slug)
	}
	mems, _ := me["memberships"].([]any)
	if len(mems) != 2 {
		t.Fatalf("membresías=%d; quería 2", len(mems))
	}
	primera, _ := mems[0].(map[string]any)
	if primera["name"] == nil || primera["slug"] == nil {
		t.Fatalf("membresía sin nombre ni slug: %v", primera)
	}

	// 2) Una organización de la que NO es miembro: 403, y el mismo 403 que si no
	//    existiera (no es un detector de organizaciones ajenas).
	ajena, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "ajena", Name: "Ajena"})
	if err != nil {
		t.Fatalf("CreateOrg(ajena): %v", err)
	}
	if rr, _ := l.do(t, http.MethodPut, "/api/v1/me/active-organization", tok,
		map[string]any{"org_id": ajena.ID}); rr.Code != http.StatusForbidden {
		t.Fatalf("org ajena: código=%d; quería 403", rr.Code)
	}
	if got := l.orgDeLaSesion(t, sessID); got != vieja.ID {
		t.Fatalf("la sesión cambió con un 403: %s", got)
	}

	// 3) Un identificador que no es UUID: 400, no un 500 de sintaxis SQL.
	if rr, _ := l.do(t, http.MethodPut, "/api/v1/me/active-organization", tok,
		map[string]any{"org_id": "nueva"}); rr.Code != http.StatusBadRequest {
		t.Fatalf("org_id no-UUID: código=%d; quería 400", rr.Code)
	}

	// 4) El cambio de verdad.
	rr, out := l.do(t, http.MethodPut, "/api/v1/me/active-organization", tok,
		map[string]any{"org_id": nueva.ID})
	if rr.Code != http.StatusOK {
		t.Fatalf("cambio: código=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if out["slug"] != nueva.Slug {
		t.Fatalf("respuesta=%v; quería slug %q", out, nueva.Slug)
	}
	if got := l.orgDeLaSesion(t, sessID); got != nueva.ID {
		t.Fatalf("sessions.org_id=%s; quería %s", got, nueva.ID)
	}
	// Sin esto el cambio sería cosmético: el refresh lleva la organización
	// congelada y seguiría emitiendo tokens de la vieja durante días.
	if l.refreshVivo(t, rtID) {
		t.Fatal("el refresh token de la organización anterior sigue vivo")
	}
}

// Una organización SUSPENDIDA no se puede activar: sería la puerta de atrás a un
// tenant cortado por impago o por incidente, que es justo lo que ese estado
// existe para impedir. El login ya lo comprueba en createSession.
func TestNoSePuedeActivarUnaOrganizacionSuspendida(t *testing.T) {
	l := newLiveServer(t)

	activa, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "activa", Name: "Activa"})
	if err != nil {
		t.Fatalf("CreateOrg(activa): %v", err)
	}
	cortada, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "cortada", Name: "Cortada"})
	if err != nil {
		t.Fatalf("CreateOrg(cortada): %v", err)
	}
	u := l.mkUser(t, "suspendida@example.com", "una-clave-larga-123")
	for _, m := range []domain.Membership{
		{OrgID: activa.ID, UserID: u.ID, Role: domain.RoleMember},
		{OrgID: cortada.ID, UserID: u.ID, Role: domain.RoleOwner},
	} {
		if err := l.st.UpsertMembership(l.ctx, nil, m); err != nil {
			t.Fatalf("UpsertMembership: %v", err)
		}
	}
	if _, err := l.st.SetOrgStatus(l.ctx, nil, cortada.ID, domain.StatusSuspended); err != nil {
		t.Fatalf("SetOrgStatus: %v", err)
	}
	sessID, _ := l.mkLiveSession(t, u.ID, activa.ID)
	tok := l.token(t, u.ID, activa.Slug, []string{"member"})

	if rr, _ := l.do(t, http.MethodPut, "/api/v1/me/active-organization", tok,
		map[string]any{"org_id": cortada.ID}); rr.Code != http.StatusForbidden {
		t.Fatalf("org suspendida: código=%d; quería 403", rr.Code)
	}
	if got := l.orgDeLaSesion(t, sessID); got != activa.ID {
		t.Fatalf("la sesión cambió a una organización suspendida: %s", got)
	}
}
