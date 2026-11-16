// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Los recuentos que hacen accionable la portada, contra PostgreSQL real.
//
// La forma de que esto engañe no es fallar: es CONTAR DE MÁS. Un «12 sin
// segundo factor» que incluye a gente de otra organización, o a cuentas
// suspendidas que ya no entran, es un aviso que se aprende a ignorar — y el día
// que el número importe, ya nadie lo mira.
package http

import (
	"net/http"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

func TestAdminInsights(t *testing.T) {
	l := newLiveServer(t)
	org, admin := l.seedOrgAdmin(t, "acme")
	adminTok := l.token(t, admin.ID, org.Slug, []string{"admin"})

	// El admin del seed ya cuenta como uno sin MFA. Dos más sin MFA, uno con,
	// uno suspendido y uno sin verificar el correo.
	l.addMember(t, org, "sinmfa1@acme.test", domain.RoleMember)
	l.addMember(t, org, "sinmfa2@acme.test", domain.RoleMember)
	conMFA := l.addMember(t, org, "conmfa@acme.test", domain.RoleMember)
	if _, err := l.st.DB().Exec(l.ctx, `UPDATE users SET mfa_enabled = true WHERE id = $1`, conMFA.ID); err != nil {
		t.Fatalf("activar MFA: %v", err)
	}
	suspendido := l.addMember(t, org, "fuera@acme.test", domain.RoleMember)
	if _, err := l.st.SetUserStatus(l.ctx, nil, suspendido.ID, domain.StatusSuspended); err != nil {
		t.Fatalf("SetUserStatus: %v", err)
	}
	sinVerificar := l.addMember(t, org, "sinverificar@acme.test", domain.RoleMember)
	if _, err := l.st.DB().Exec(l.ctx, `UPDATE users SET email_verified = false WHERE id = $1`, sinVerificar.ID); err != nil {
		t.Fatalf("desverificar: %v", err)
	}

	// Una invitación viva y otra caducada.
	ahora := time.Now()
	if err := l.st.InsertInvitation(l.ctx, nil, "hash-viva", org.ID, "espera@acme.test",
		string(domain.RoleMember), admin.ID, "", "", ahora.Add(48*time.Hour)); err != nil {
		t.Fatalf("InsertInvitation viva: %v", err)
	}
	if err := l.st.InsertInvitation(l.ctx, nil, "hash-caducada", org.ID, "olvidado@acme.test",
		string(domain.RoleMember), admin.ID, "", "", ahora.Add(-time.Hour)); err != nil {
		t.Fatalf("InsertInvitation caducada: %v", err)
	}

	// Y una organización ajena con de todo, que NO debe sumar en ningún recuento.
	otra, _ := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "otra", Name: "Otra"})
	l.addMember(t, otra, "ajeno1@otra.test", domain.RoleMember)
	l.addMember(t, otra, "ajeno2@otra.test", domain.RoleMember)
	if err := l.st.InsertInvitation(l.ctx, nil, "hash-ajena", otra.ID, "ajena@otra.test",
		string(domain.RoleMember), admin.ID, "", "", ahora.Add(-time.Hour)); err != nil {
		t.Fatalf("InsertInvitation ajena: %v", err)
	}

	rr, body := l.do(t, http.MethodGet, "/api/v1/admin/insights", adminTok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("insights: código=%d body=%v", rr.Code, body)
	}
	quiero := map[string]int{
		// admin + sinmfa1 + sinmfa2 + sinverificar = 4 activos sin MFA. El de
		// MFA no cuenta, y el SUSPENDIDO tampoco: ya no entra, así que no es un
		// riesgo pendiente sino una cuenta cerrada.
		"without_mfa":         4,
		"suspended":           1,
		"unverified_email":    1,
		"invitations_expired": 1,
		"invitations_pending": 1,
	}
	for clave, esperado := range quiero {
		got, _ := body[clave].(float64)
		if int(got) != esperado {
			t.Errorf("%s = %v, quiero %d (body=%v)", clave, body[clave], esperado, body)
		}
	}
}

// TestAdminUsersFiltroCuadraConLosAvisos: cada aviso de la portada enlaza a
// Usuarios con su filtro puesto, y la lista tiene que enseñar EXACTAMENTE a la
// gente que el aviso cuenta. Si el aviso dice 4 y el enlace abre 5 —porque uno
// mira solo las cuentas activas y el otro no—, el número deja de creerse.
func TestAdminUsersFiltroCuadraConLosAvisos(t *testing.T) {
	l := newLiveServer(t)
	org, admin := l.seedOrgAdmin(t, "acme")
	adminTok := l.token(t, admin.ID, org.Slug, []string{"admin"})

	sinMFA := l.addMember(t, org, "sinmfa@acme.test", domain.RoleMember)
	conMFA := l.addMember(t, org, "conmfa@acme.test", domain.RoleMember)
	suspendido := l.addMember(t, org, "fuera@acme.test", domain.RoleMember)
	sinVerificar := l.addMember(t, org, "sinverificar@acme.test", domain.RoleMember)
	for _, q := range []struct {
		sql string
		id  string
	}{
		{`UPDATE users SET mfa_enabled = true WHERE id = $1`, conMFA.ID},
		// El suspendido no tiene MFA: aun así NO es de «sin verificación en dos
		// pasos», porque ya no entra.
		{`UPDATE users SET status = 'suspended' WHERE id = $1`, suspendido.ID},
		{`UPDATE users SET email_verified = false, mfa_enabled = true WHERE id = $1`, sinVerificar.ID},
	} {
		if _, err := l.st.DB().Exec(l.ctx, q.sql, q.id); err != nil {
			t.Fatalf("preparar %s: %v", q.id, err)
		}
	}
	otra, _ := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "otra", Name: "Otra"})
	l.addMember(t, otra, "ajeno@otra.test", domain.RoleMember)

	rr, avisos := l.do(t, http.MethodGet, "/api/v1/admin/insights", adminTok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("insights: código=%d body=%v", rr.Code, avisos)
	}
	quiero := map[string][]string{
		"without_mfa":      {admin.ID, sinMFA.ID},
		"suspended":        {suspendido.ID},
		"unverified_email": {sinVerificar.ID},
	}
	for filtro, ids := range quiero {
		rr, body := l.do(t, http.MethodGet, "/api/v1/admin/users?filter="+filtro, adminTok, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("filter=%s: código=%d body=%v", filtro, rr.Code, body)
		}
		items, _ := body["items"].([]any)
		got := map[string]bool{}
		for _, it := range items {
			m, _ := it.(map[string]any)
			id, _ := m["id"].(string)
			got[id] = true
		}
		if len(got) != len(ids) {
			t.Errorf("filter=%s devolvió %d personas, quiero %d (%v)", filtro, len(got), len(ids), items)
		}
		for _, id := range ids {
			if !got[id] {
				t.Errorf("filter=%s no trae a %s", filtro, id)
			}
		}
		if n, _ := avisos[filtro].(float64); int(n) != len(items) {
			t.Errorf("el aviso %s cuenta %v y su lista enseña %d", filtro, avisos[filtro], len(items))
		}
	}

	// Un filtro que no existe es un 400, no la lista entera: quien pide «los
	// suspendidos» con una errata y recibe a todo el mundo cree que lo están todos.
	rr, body := l.do(t, http.MethodGet, "/api/v1/admin/users?filter=suspendidos", adminTok, nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("filter desconocido: código=%d body=%v, quiero 400", rr.Code, body)
	}
}
