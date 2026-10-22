// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// `DELETE /api/v1/me/sessions` —«cerrar las demás sesiones»— contra un PostgreSQL real.
//
// Este manejador no tenía NINGÚN test HTTP, y por eso llevaba roto desde que se
// escribió: le pasaba a `RevokeOtherSessions` el hash de la cookie (64 hex) o la cadena
// vacía, y la SQL compara ese parámetro con `sessions.id`, que es `uuid`. Postgres
// contestaba 22P02 y el endpoint devolvía 500 sin cerrar una sola sesión — desde ningún
// cliente, con cookie o sin ella. El test de integración del store no lo cazaba porque
// llamaba a la función con un id real, que es justo lo que el manejador NO le pasaba: el
// agujero estaba en el trozo que ninguno de los dos cubría.
//
// Por eso estas pruebas entran por el ROUTER y no por el store. Vistas en rojo contra el
// código anterior: los tres casos daban 500.
package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// revocarOtras llama al endpoint con el bearer de `sub` y, si `cookie` no está vacía, con
// la cookie de sesión del IdP (en el harness `CookieSecure` es false ⇒ sin `__Host-`).
func (l *liveServer) revocarOtras(t *testing.T, token, cookie string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodDelete, "/api/v1/me/sessions", http.NoBody)
	r.Header.Set("Authorization", "Bearer "+token)
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	}
	rr := httptest.NewRecorder()
	l.mux.ServeHTTP(rr, r)
	return rr
}

// viva dice si la sesión de ese token en claro sigue abierta.
func (l *liveServer) viva(t *testing.T, tokenEnClaro string) bool {
	t.Helper()
	ss, err := l.st.GetSessionByToken(l.ctx, domain.HashToken(tokenEnClaro))
	if err != nil {
		t.Fatalf("GetSessionByToken(%q): %v", tokenEnClaro, err)
	}
	return ss.Valid(time.Now())
}

func TestMeSessionRevokeOthers_ConCookieConservaLaActual(t *testing.T) {
	l := newLiveServer(t)
	org, _ := l.seedOrgAdmin(t, "acme-rev1")
	ada := l.addMember(t, org, "ada@acme-rev1.test", domain.RoleMember)
	eve := l.addMember(t, org, "eve@acme-rev1.test", domain.RoleMember)
	l.liveSession(t, org, ada.ID, "ada-actual")
	l.liveSession(t, org, ada.ID, "ada-tablet")
	l.liveSession(t, org, ada.ID, "ada-movil")
	l.liveSession(t, org, eve.ID, "eve-1")

	rr := l.revocarOtras(t, l.token(t, ada.ID, org.ID, nil), "ada-actual")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("DELETE /me/sessions = %d, quiero 204; cuerpo %s", rr.Code, rr.Body.String())
	}
	if !l.viva(t, "ada-actual") {
		t.Error("la sesión desde la que se pulsa NO debe cerrarse")
	}
	if l.viva(t, "ada-tablet") || l.viva(t, "ada-movil") {
		t.Error("las otras sesiones de Ada siguen abiertas")
	}
	if !l.viva(t, "eve-1") {
		t.Error("se ha cerrado la sesión de OTRA persona")
	}
}

func TestMeSessionRevokeOthers_SinCookieCierraTodas(t *testing.T) {
	// La decisión: un bearer no lleva `sid`, así que sin la cookie el servicio no puede
	// saber cuál es la sesión de quien llama. Cerrarlas TODAS es la opción segura —quien
	// pulsa quiere que no quede ninguna que no controle— y lo peor que pasa es que hay
	// que volver a entrar. Dejar viva «alguna» dejaría viva justo la no identificable.
	l := newLiveServer(t)
	org, _ := l.seedOrgAdmin(t, "acme-rev2")
	ada := l.addMember(t, org, "ada@acme-rev2.test", domain.RoleMember)
	eve := l.addMember(t, org, "eve@acme-rev2.test", domain.RoleMember)
	l.liveSession(t, org, ada.ID, "ada-portatil")
	l.liveSession(t, org, ada.ID, "ada-movil")
	l.liveSession(t, org, eve.ID, "eve-1")

	rr := l.revocarOtras(t, l.token(t, ada.ID, org.ID, nil), "")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("DELETE /me/sessions = %d, quiero 204; cuerpo %s", rr.Code, rr.Body.String())
	}
	if l.viva(t, "ada-portatil") || l.viva(t, "ada-movil") {
		t.Error("sin cookie tienen que cerrarse TODAS las sesiones de quien llama")
	}
	if !l.viva(t, "eve-1") {
		t.Error("se ha cerrado la sesión de OTRA persona")
	}
}

func TestMeSessionRevokeOthers_CookieAjenaNoSalvaNada(t *testing.T) {
	// Mandar la cookie de otra persona no puede conservar una sesión propia (no hay
	// ninguna que conservar) ni tocar las de esa otra persona.
	l := newLiveServer(t)
	org, _ := l.seedOrgAdmin(t, "acme-rev3")
	ada := l.addMember(t, org, "ada@acme-rev3.test", domain.RoleMember)
	eve := l.addMember(t, org, "eve@acme-rev3.test", domain.RoleMember)
	l.liveSession(t, org, ada.ID, "ada-1")
	l.liveSession(t, org, eve.ID, "eve-actual")

	rr := l.revocarOtras(t, l.token(t, ada.ID, org.ID, nil), "eve-actual")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("DELETE /me/sessions = %d, quiero 204; cuerpo %s", rr.Code, rr.Body.String())
	}
	if l.viva(t, "ada-1") {
		t.Error("la cookie de otra persona no conserva las sesiones propias")
	}
	if !l.viva(t, "eve-actual") {
		t.Error("se ha cerrado la sesión de OTRA persona")
	}
}
