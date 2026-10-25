// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

func TestAccount(t *testing.T) {
	var parche, etiqueta string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/me":
			_, _ = io.WriteString(w, `{"id":"u1","email":"ada@sytue.eu","username":"ada","email_verified":true,"name":"Ada Lovelace","title":"Analista","locale":"es","mfa_enabled":true,"memberships":[]}`)
		case "PATCH /api/v1/me":
			cuerpo, _ := io.ReadAll(r.Body)
			parche = string(cuerpo)
			_, _ = io.WriteString(w, `{"id":"u1","name":"Ada King","title":"","locale":"es"}`)
		case "GET /api/v1/me/sessions":
			_, _ = io.WriteString(w, `{"sessions":[{"id":"s1","ip":"","user_agent":"Mozilla/5.0","device":"Chrome · macOS","amr":["pwd"],"auth_time":"2026-09-20T10:00:00Z","expires_at":"2026-09-21T10:00:00Z","current":false}]}`)
		case "DELETE /api/v1/me/sessions/s1", "DELETE /api/v1/me/sessions":
			w.WriteHeader(http.StatusNoContent)
		case "GET /api/v1/me/devices":
			_, _ = io.WriteString(w, `{"devices":[{"id":"d1","name":"Chrome · macOS","trusted":true,"last_seen":"2026-09-20T10:00:00Z","current":false}]}`)
		case "GET /api/v1/webauthn/credentials":
			_, _ = io.WriteString(w, `{"credentials":[{"id":"k1","credential_id":"AAAA","name":"Llave USB","created_at":"2026-09-01T10:00:00Z","last_used":null}]}`)
		case "GET /api/v1/me/app-passwords":
			_, _ = io.WriteString(w, `{"app_passwords":[{"id":"p1","label":"Correo del portátil","created_at":"2026-09-01T10:00:00Z","last_used_at":null}]}`)
		case "POST /api/v1/me/app-passwords":
			cuerpo, _ := io.ReadAll(r.Body)
			etiqueta = string(cuerpo)
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"p2","label":"Archivos · Mac","created_at":"2026-10-19T10:00:00Z","password":"abcd-efgh","org_id":"org-ada","tenant":"ada"}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	a, ctx := NewAccount(srv.URL, nil, nil), context.Background()

	yo, err := a.Me(ctx)
	if err != nil || yo.Name != "Ada Lovelace" || yo.Username != "ada" || !yo.MFAEnabled || !yo.EmailVerified || yo.Locale != "es" {
		t.Fatalf("ficha: %+v, %v", yo, err)
	}
	// Solo viaja lo que se ha tocado, y un campo vaciado a propósito viaja vacío.
	nombre, puesto := "Ada King", ""
	guardada, err := a.UpdateMe(ctx, ProfilePatch{Name: &nombre, Title: &puesto})
	if err != nil || guardada.Name != "Ada King" || parche != `{"name":"Ada King","title":""}` {
		t.Fatalf("guardar: %+v, %v; ha viajado %s", guardada, err, parche)
	}
	sesiones, err := a.Sessions(ctx)
	if err != nil || len(sesiones) != 1 || sesiones[0].Device != "Chrome · macOS" || sesiones[0].AuthTime.IsZero() || sesiones[0].Current {
		t.Fatalf("sesiones: %+v, %v", sesiones, err)
	}
	if err := a.RevokeSession(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	if err := a.RevokeOtherSessions(ctx); err != nil {
		t.Fatal(err)
	}
	if d, err := a.Devices(ctx); err != nil || len(d) != 1 || !d[0].Trusted || d[0].LastSeen.IsZero() {
		t.Fatalf("dispositivos: %+v, %v", d, err)
	}
	if k, err := a.Passkeys(ctx); err != nil || len(k) != 1 || k[0].Name != "Llave USB" {
		t.Fatalf("claves de acceso: %+v, %v", k, err)
	}
	if p, err := a.AppPasswords(ctx); err != nil || len(p) != 1 || p[0].Label != "Correo del portátil" || p[0].LastUsedAt != nil {
		t.Fatalf("contraseñas de aplicación: %+v, %v", p, err)
	}
	if c, err := a.CreateAppPassword(ctx, "Archivos · Mac"); err != nil || c.ID != "p2" || c.Password != "abcd-efgh" || c.OrgID != "org-ada" || c.Tenant != "ada" || etiqueta != `{"label":"Archivos · Mac"}` {
		t.Fatalf("crear contraseña: %+v, %v; ha viajado %s", c, err, etiqueta)
	}
}

// TestMisOrganizaciones: la lista es la del BFF de la web —solo las membresías activas
// con slug, la ACTIVA por `active_org`, el slug si no hay nombre— y cambiar es un PUT
// con `org_id`. Atado al servicio: los campos de `handleMe` y la ruta del cambio.
//
// Visto en ROJO a mano (en el frontal): quitando el filtro de `status`, la suspendida
// sale en la lista y el test lo dice.
func TestMisOrganizaciones(t *testing.T) {
	var cambio string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/me":
			_, _ = io.WriteString(w, `{"id":"u1","active_org":"acme","memberships":[`+
				`{"org_id":"o1","role":"owner","slug":"acme","name":"Acme","status":"active","kind":"org"},`+
				`{"org_id":"o2","role":"member","slug":"globex","name":"","status":"active","kind":"org"},`+
				`{"org_id":"o3","role":"member","slug":"cortada","name":"Cortada","status":"suspended","kind":"org"}]}`)
		case "PUT /api/v1/me/active-organization":
			cuerpo, _ := io.ReadAll(r.Body)
			cambio = string(cuerpo)
			_, _ = io.WriteString(w, `{"org_id":"o2","slug":"globex","reauthorize":true}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	a, ctx := NewAccount(srv.URL, nil, nil), context.Background()
	orgs, err := a.Organizaciones(ctx)
	quiero := []Organizacion{{ID: "o1", Slug: "acme", Nombre: "Acme", Activa: true}, {ID: "o2", Slug: "globex", Nombre: "globex"}}
	if err != nil || !slices.Equal(orgs, quiero) {
		t.Fatalf("organizaciones: %+v, %v; quiero %+v", orgs, err, quiero)
	}
	if err := a.CambiarOrganizacion(ctx, "o2"); err != nil || cambio != `{"org_id":"o2"}` {
		t.Fatalf("cambiar: %v; ha viajado %s", err, cambio)
	}

	servidor := webref.Read(t, "services/id/internal/http/server.go")
	perfil := webref.Read(t, "services/id/internal/http/admin.go")
	cambiar := webref.Read(t, "services/id/internal/http/me_orgs.go")
	for fuente, literales := range map[string][]string{
		servidor: {`r.Put("/me/active-organization", s.handleSetActiveOrg)`},
		perfil:   {`"org_id": m.OrgID`, `"slug": m.Slug, "name": m.Name, "status": m.Status`, `out["memberships"] = orgs`, `out["active_org"] = c.Tenant`},
		cambiar:  {"OrgID string `json:\"org_id\"`"},
	} {
		for _, literal := range literales {
			if !strings.Contains(fuente, literal) {
				t.Errorf("services/id ya no dice %s: revisar Organizaciones en account.go", literal)
			}
		}
	}
}

func TestLaCuentaHablaElIdiomaDeSytueID(t *testing.T) {
	rutas := webref.Read(t, "services/id/internal/http/server.go")
	for _, ruta := range []string{
		`r.Route("/api/v1"`, `r.Get("/me", s.handleMe)`, `r.Patch("/me", s.handleMeUpdate)`,
		`r.Get("/me/sessions", s.handleMeSessions)`, `r.Delete("/me/sessions/{id}", s.handleMeSessionRevoke)`,
		`r.Delete("/me/sessions", s.handleMeSessionRevokeOthers)`, `r.Get("/me/devices", s.handleMeDevices)`,
		`r.Get("/me/app-passwords", s.handleMeAppPasswords)`,
		`r.With(loginRL).Post("/me/app-passwords", s.handleMeAppPasswordCreate)`,
		`r.Get("/me/avatar", s.handleMeAvatarGet)`,
		`r.Get("/subscription", s.handleSubscriptionStatus)`, `r.Get("/", s.handleOnboardingGet)`,
		`r.Route("/webauthn"`, `r.Get("/credentials", s.handleWebAuthnList)`,
	} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("services/id ya no monta %s: revisar internal/api/account.go", ruta)
		}
	}
	for fichero, literales := range map[string][]string{
		"services/id/internal/http/me_profile.go": {
			// lo que se lee (meProfileWire)…
			`"id":`, `"email":`, `"username":`, `"email_verified":`, `"name":`, `"given_name":`, `"family_name":`,
			`"title":`, `"department":`, `"location":`, `"locale":`, `"mfa_enabled":`,
			// …y lo que se escribe (updateProfileRequest), con punteros: nil no viaja.
			"*string `json:\"name\"`", "*string `json:\"given_name\"`", "*string `json:\"family_name\"`",
			"*string `json:\"title\"`", "*string `json:\"department\"`", "*string `json:\"location\"`", "*string `json:\"locale\"`",
		},
		"services/id/internal/http/account_sessions.go": {`"sessions": out`, `"id":`, `"user_agent":`, `"device":`, `"auth_time":`, `"expires_at":`, `"current":`},
		"services/id/internal/http/me_devices.go":       {`"devices": out`, `"id":`, `"name":`, `"trusted":`, `"last_seen":`, `"current":`},
		"services/id/internal/http/webauthn.go":         {`"credentials": out`, `"id":`, `"name":`, `"created_at":`},
		"services/id/internal/http/me_app_passwords.go": {`"app_passwords": list`, `"id":         fila.ID`, `"password": claro`},
		"services/id/internal/http/onboarding.go":       {`"completed":         completado`},
		"services/id/internal/store/apppasswords.go":    {"`json:\"id\"`", "`json:\"label\"`", "`json:\"created_at\"`", "`json:\"last_used_at\"`"},
		"services/id/internal/http/subscriptions.go": {
			`"status": sub.Status`, `"plan": sub.EffectivePlan(now)`, `"contracted_plan": sub.Plan`,
			`"storage_bytes": sub.StorageBytes(now)`, `"mailbox_bytes": sub.MailboxBytes(now)`,
			`"paid_until": nullableTime(sub.PaidUntil)`, `"personal": org.Personal()`,
			`"education":  org.Education`, `"upgrade_available":`,
		},
		// El espacio gastado lo sirve Drive, no Sytue ID: la tarjeta del plan llama a
		// los dos, igual que PlanCard.tsx.
		"services/drive/internal/http/server.go": {`api.Get("/quota", h.GetQuota)`},
	} {
		fuente := webref.Read(t, fichero)
		for _, literal := range literales {
			if !strings.Contains(fuente, literal) {
				t.Errorf("%s ya no dice %s: revisar internal/api/account.go", fichero, literal)
			}
		}
	}
}

func TestSuscripcionYAvatar(t *testing.T) {
	var vistas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vistas = append(vistas, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/onboarding/subscription":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"active","plan":"pro","contracted_plan":"pro","storage_bytes":214748364800,
				"mailbox_bytes":53687091200,"paid_until":"2026-12-31T00:00:00Z","entitled":true,"personal":false,
				"education":false,"upgrade_available":true}`))
		case "/api/v1/onboarding":
			_, _ = w.Write([]byte(`{"mode":"organization","step":"org","completed":false}`))
		case "/api/v1/me/avatar":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("\x89PNG\r\n\x1a\n"))
		case "/api/v1/quota":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"limit_bytes":100,"used_bytes":40}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	a := NewAccount(srv.URL, nil, srv.Client(), srv.URL)
	sub, err := a.Subscription(t.Context())
	if err != nil || sub.Plan != "pro" || sub.StorageBytes != 200<<30 || sub.PaidUntil == nil || !sub.UpgradeAvailable {
		t.Fatalf("suscripción: %+v, %v", sub, err)
	}
	datos, tipo, err := a.Avatar(t.Context())
	if err != nil || tipo != "image/png" || len(datos) != 8 {
		t.Fatalf("avatar: %d bytes, %q, %v", len(datos), tipo, err)
	}
	q, hay, err := a.Quota(t.Context())
	if err != nil || !hay || q.UsedBytes != 40 {
		t.Fatalf("cuota: %+v, %v, %v", q, hay, err)
	}
	if hecho, err := a.AltaCompletada(t.Context()); err != nil || hecho {
		t.Fatalf("alta: %v, %v", hecho, err)
	}
	if quiero := []string{"GET /api/v1/onboarding/subscription", "GET /api/v1/me/avatar", "GET /api/v1/quota", "GET /api/v1/onboarding"}; !slices.Equal(vistas, quiero) {
		t.Fatalf("rutas %q, quiero %q", vistas, quiero)
	}

	// Sin la raíz de Drive no hay cuota, y eso NO es un error: es que este cliente no
	// sabe a quién preguntarle.
	sinDrive := NewAccount(srv.URL, nil, srv.Client())
	if q, hay, err := sinDrive.Quota(t.Context()); hay || err != nil || q.LimitBytes != 0 {
		t.Fatalf("sin Drive: %+v, %v, %v", q, hay, err)
	}
}

// TestElAvatarQueNoHayNoEsUnFallo: quedarse en las iniciales es lo normal, no algo que
// contar. Si esto se rompe, «Mi cuenta» enseñaría un error a quien no ha subido foto.
func TestElAvatarQueNoHayNoEsUnFallo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	datos, tipo, err := NewAccount(srv.URL, nil, srv.Client()).Avatar(t.Context())
	if err != nil || datos != nil || tipo != "" {
		t.Fatalf("sin avatar: %d bytes, %q, %v", len(datos), tipo, err)
	}
}
