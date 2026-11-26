// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// La actividad de Mi cuenta enseña lo de quien llama y SOLO lo de su cuenta: ni lo
// que hizo como administradora (una acción de `org.`) ni lo de otra persona, y con la
// clase pública en vez del nombre interno del log.
func TestActividadDeLaCuentaEsSoloLaPropia(t *testing.T) {
	l := newLiveServer(t)
	org, adminTok := l.orgAdmin(t, "acme", "ana@acme.example")
	ana, err := l.st.GetUserByEmail(l.ctx, "ana@acme.example")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	otra := l.mkUser(t, "otra@acme.example", "otra-pass-123")
	apunta := func(actor, accion string) {
		t.Helper()
		if err := l.st.Audit(l.ctx, nil, store.AuditEntry{ActorID: actor, OrgID: org.ID, Action: accion, IP: "203.0.113.7"}); err != nil {
			t.Fatalf("Audit %s: %v", accion, err)
		}
	}
	apunta(ana.ID, "auth.login.success")
	apunta(ana.ID, "user.password.changed")
	apunta(ana.ID, "org.member.role_changed") // administración: no es de su cuenta
	apunta(otra.ID, "auth.login.success")     // de otra persona

	rr, out := l.do(t, http.MethodGet, "/api/v1/me/activity", adminTok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /me/activity: código=%d cuerpo=%s", rr.Code, rr.Body)
	}
	eventos, _ := out["events"].([]any)
	if len(eventos) != 2 {
		t.Fatalf("eventos=%d (%s), quiero 2: el inicio de sesión y el cambio de contraseña de Ana", len(eventos), rr.Body)
	}
	// Del más reciente al más antiguo, y con la clase pública.
	primero, _ := eventos[0].(map[string]any)
	segundo, _ := eventos[1].(map[string]any)
	if primero["kind"] != "password_changed" || segundo["kind"] != "signin" {
		t.Errorf("clases=%v,%v; quiero password_changed,signin", primero["kind"], segundo["kind"])
	}
	if primero["ip"] != "203.0.113.7" {
		t.Errorf("ip=%v, quiero la del suceso", primero["ip"])
	}
}

// Renombrar una passkey: la propia cambia de nombre y queda en la actividad; la de
// otra persona es un 404 y no se toca; un nombre vacío es un 400.
func TestRenombrarPasskeyPropia(t *testing.T) {
	l := newLiveServer(t)
	_, tok := l.orgAdmin(t, "acme", "ana@acme.example")
	ana, err := l.st.GetUserByEmail(l.ctx, "ana@acme.example")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	otra := l.mkUser(t, "otra@acme.example", "otra-pass-123")
	alta := func(userID, nombre string) string {
		t.Helper()
		if err := l.st.InsertCredential(l.ctx, nil, domain.WebAuthnCredential{
			UserID: userID, CredID: []byte("cred-" + userID), PublicKey: []byte("pk"),
			AAGUID: make([]byte, 16), SignCount: 1, Name: nombre,
		}); err != nil {
			t.Fatalf("InsertCredential: %v", err)
		}
		creds, err := l.st.ListCredentialsForUser(l.ctx, userID)
		if err != nil || len(creds) != 1 {
			t.Fatalf("ListCredentialsForUser: %v (%d)", err, len(creds))
		}
		return creds[0].ID
	}
	propia := alta(ana.ID, "Passkey")
	ajena := alta(otra.ID, "La de otra")

	if rr, _ := l.do(t, http.MethodPatch, "/api/v1/webauthn/credentials/"+propia, tok, map[string]any{"name": "  "}); rr.Code != http.StatusBadRequest {
		t.Errorf("nombre vacío: código=%d, quiero 400", rr.Code)
	}
	if rr, _ := l.do(t, http.MethodPatch, "/api/v1/webauthn/credentials/"+ajena, tok, map[string]any{"name": "Mía"}); rr.Code != http.StatusNotFound {
		t.Errorf("passkey ajena: código=%d, quiero 404", rr.Code)
	}
	rr, out := l.do(t, http.MethodPatch, "/api/v1/webauthn/credentials/"+propia, tok, map[string]any{"name": " Llave USB "})
	if rr.Code != http.StatusOK || out["name"] != "Llave USB" {
		t.Fatalf("renombrar: código=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if creds, _ := l.st.ListCredentialsForUser(l.ctx, ana.ID); len(creds) != 1 || creds[0].Name != "Llave USB" {
		t.Errorf("la passkey no quedó renombrada: %+v", creds)
	}
	if creds, _ := l.st.ListCredentialsForUser(l.ctx, otra.ID); len(creds) != 1 || creds[0].Name != "La de otra" {
		t.Errorf("se tocó la passkey de otra persona: %+v", creds)
	}
	_, act := l.do(t, http.MethodGet, "/api/v1/me/activity", tok, nil)
	eventos, _ := act["events"].([]any)
	if len(eventos) == 0 || eventos[0].(map[string]any)["kind"] != "passkey_renamed" {
		t.Errorf("el cambio de nombre no aparece en la actividad: %v", act)
	}
}
