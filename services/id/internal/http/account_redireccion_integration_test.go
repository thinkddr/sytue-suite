//go:build integration

package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// TestAccountRedirigeALaApp — `/account` manda a la app «Mi cuenta» cuando está
// desplegada —a su dirección global, nunca a la de una organización—, y sirve su
// propio HTML cuando no.
//
// LAS DOS RAMAS, y la segunda es la que importa: una instalación que no despliegue
// la app y reciba el 302 deja a su gente sin pantalla de cuenta, y sin forma de
// cambiar la contraseña ni de apagar el segundo factor. Por eso la redirección
// depende de un valor que se puede dejar vacío, y por eso se comprueba vacío.
func TestAccountRedirigeALaApp(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	u := caOrgUser(t, l, org, "cuenta@id.test")

	// Sesión SSO directa: lo que se prueba es la redirección, no el login.
	token := "tok-redireccion-cuenta"
	if _, err := l.st.InsertSession(l.ctx, nil, domain.Session{
		TokenHash: domain.HashToken(token),
		UserID:    u.ID,
		OrgID:     org.ID,
		AuthTime:  time.Now().UTC(),
		AMR:       []string{"pwd"},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}

	pedir := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/account", nil)
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
		rr := httptest.NewRecorder()
		l.mux.ServeHTTP(rr, r)
		return rr
	}

	// Sin la app desplegada: la página de siempre, servida por este servicio.
	l.srv.cfg.AccountAppPath = ""
	if rr := pedir(); rr.Code != http.StatusOK {
		t.Fatalf("sin app: status=%d, quiero 200 (la página propia)", rr.Code)
	}

	// Con la app: 302 a la dirección GLOBAL, y NUNCA al host de la organización
	// de la sesión —aunque la tenga—. La cuenta es de la persona (decisión del
	// dueño, 2026-09-24): si alguien vuelve a componer `https://<slug>.` aquí,
	// esto se pone en rojo.
	l.srv.cfg.AccountAppPath = "/account"
	rr := pedir()
	if rr.Code != http.StatusFound {
		t.Fatalf("con app: status=%d, quiero 302", rr.Code)
	}
	if destino := rr.Header().Get("Location"); !strings.HasPrefix(destino, "https://account.") ||
		!strings.HasSuffix(destino, "/account") || strings.Contains(destino, org.Slug+".") {
		t.Fatalf("destino=%q: se esperaba https://account.<dominio>/account, nunca el host de %q", destino, org.Slug)
	}

	// La query viaja hasta la app: `?mfa=required` es el aviso del step-up.
	rMfa := httptest.NewRequest(http.MethodGet, "/account?mfa=required", nil)
	rMfa.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rrMfa := httptest.NewRecorder()
	l.mux.ServeHTTP(rrMfa, rMfa)
	if got := rrMfa.Header().Get("Location"); !strings.HasSuffix(got, "/account?mfa=required") {
		t.Fatalf("con ?mfa=required: destino=%q, se perdía el aviso", got)
	}

	// Con cuenta personal, igual: account.<dominio>/account, nunca su slug
	// (las cuentas personales no tienen subdominio propio, ADR 0038).
	orgPers, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{
		Slug: domain.PersonalSlug("ana@personal.test"), Name: "ana@personal.test", Kind: domain.OrgKindPersonal,
	})
	if err != nil {
		t.Fatalf("CreateOrg personal: %v", err)
	}
	uPers := caOrgUser(t, l, orgPers, "ana@personal.test")
	tokPers := "tok-redireccion-personal"
	if _, err := l.st.InsertSession(l.ctx, nil, domain.Session{
		TokenHash: domain.HashToken(tokPers),
		UserID:    uPers.ID,
		OrgID:     orgPers.ID,
		AuthTime:  time.Now().UTC(),
		AMR:       []string{"pwd"},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("InsertSession personal: %v", err)
	}
	rPers := httptest.NewRequest(http.MethodGet, "/account", nil)
	rPers.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tokPers})
	rrPers := httptest.NewRecorder()
	l.mux.ServeHTTP(rrPers, rPers)
	if rrPers.Code != http.StatusFound {
		t.Fatalf("cuenta personal con app: status=%d, quiero 302", rrPers.Code)
	}
	destPers := rrPers.Header().Get("Location")
	if !strings.HasPrefix(destPers, "https://account.") || strings.Contains(destPers, orgPers.Slug) {
		t.Fatalf("destPers=%q: una cuenta personal debe ir a account.<dominio>/account, no a su slug", destPers)
	}

	// Sin sesión no se redirige a la app: se va al login, que es quien sabe qué
	// hacer. Mandar a la app a alguien sin sesión la devolvería aquí, y el rebote
	// entre dos hosts es lo que acaba en un bucle.
	rrAnon := httptest.NewRecorder()
	l.mux.ServeHTTP(rrAnon, httptest.NewRequest(http.MethodGet, "/account", nil))
	if rrAnon.Code != http.StatusFound || !strings.Contains(rrAnon.Header().Get("Location"), "/login/") {
		t.Fatalf("sin sesión: status=%d destino=%q, quiero el login",
			rrAnon.Code, rrAnon.Header().Get("Location"))
	}
}
