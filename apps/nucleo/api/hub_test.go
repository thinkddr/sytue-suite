// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

func TestHub(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/notifications":
			if r.URL.Query().Get("limit") != "30" {
				t.Errorf("limit: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"notifications":[{"id":"n1","source":"mail","title":"Correo nuevo","body":"De Grace","url":"https://mail.sytue.eu/m/1","read":false,"created_at":"2026-09-20T10:00:00Z"}]}`)
		case "GET /api/v1/notifications/unread_count":
			_, _ = io.WriteString(w, `{"unread":3,"mail_unread":2}`)
		case "POST /api/v1/notifications/n1/read":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	h, ctx := NewHub(srv.URL, nil, nil), context.Background()
	avisos, err := h.Notifications(ctx, 30)
	if err != nil || len(avisos) != 1 || avisos[0].Source != "mail" || avisos[0].Read || avisos[0].CreatedAt.IsZero() {
		t.Fatalf("avisos: %+v, %v", avisos, err)
	}
	if c, err := h.Contadores(ctx); err != nil || c.Unread != 3 || c.MailUnread == nil || *c.MailUnread != 2 {
		t.Fatalf("sin leer: %+v, %v", c, err)
	}
	if err := h.MarkRead(ctx, "n1"); err != nil {
		t.Fatal(err)
	}
}

func TestLosCamposDelHubSonLosDelServicio(t *testing.T) {
	dominio := webref.Read(t, "services/hub/internal/domain/domain.go")
	for _, campo := range []string{"`json:\"source\"`", "`json:\"title\"`", "`json:\"body,omitempty\"`", "`json:\"url,omitempty\"`", "`json:\"read\"`", "`json:\"created_at\"`"} {
		if !strings.Contains(dominio, campo) {
			t.Errorf("services/hub ya no declara %s: revisar internal/api/hub.go", campo)
		}
	}
	manejadores := webref.Read(t, "services/hub/internal/http/handlers.go")
	literales := []string{
		`"notifications": items`, `"unread": count`, `Query().Get("limit")`,
		`"prefs": prefs`, "Kind   string `json:\"kind\"`", `"apps": apps`, `p.Get("sources")`,
	}
	for _, literal := range literales {
		if !strings.Contains(manejadores, literal) {
			t.Errorf("services/hub/internal/http/handlers.go ya no dice %s", literal)
		}
	}
	rutas := webref.Read(t, "services/hub/internal/http/router.go")
	montadas := []string{
		`"/notifications"`, `"/notifications/unread_count"`, `"/notifications/{id}/read"`, `"/export"`,
		`"/notifications/read_all"`, `"/notifications/prefs"`, `"/apps"`, `"/recent"`,
		`"/instaladores"`, `"/instaladores/{fichero}"`,
	}
	for _, ruta := range montadas {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("services/hub ya no monta %s", ruta)
		}
	}
	// Los tres formatos y el nombre del parámetro son los del servicio: si el Hub
	// dejara de aceptar uno, aquí saldría un botón que descarga un error.
	exportacion := webref.Read(t, "services/hub/internal/http/export.go")
	for _, literal := range []string{`Query().Get("format")`, `"application/zip"`, `"Content-Disposition"`} {
		if !strings.Contains(exportacion, literal) {
			t.Errorf("services/hub/internal/http/export.go ya no dice %s", literal)
		}
	}
	// El instalador: los campos y el parámetro son los del servicio.
	inst := webref.Read(t, "services/hub/internal/instaladores/instaladores.go") +
		webref.Read(t, "services/hub/internal/http/instaladores.go")
	literalesInstalador := []string{
		"`json:\"version\"`", "`json:\"sha256\"`", "`json:\"url,omitempty\"`", "`json:\"fichero\"`",
		`Query().Get("plataforma")`, `"instaladores": lista`, `WithCode("` + codigoSinInstaladores + `")`,
	}
	for _, literal := range literalesInstalador {
		if !strings.Contains(inst, literal) {
			t.Errorf("services/hub (instaladores) ya no dice %s", literal)
		}
	}
	formatos := webref.Read(t, "platform/pkg/portabilidad/portabilidad.go")
	for _, f := range HubFormatos {
		if !strings.Contains(formatos, `= "`+string(f)+`"`) {
			t.Errorf("portabilidad ya no admite el formato %q", f)
		}
	}
}

// TestHubExport: la copia de «Tus datos» se pide entera y se devuelve SIN LEER, con el
// nombre que propone el servidor (RGPD art. 20).
func TestHubExport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method+" "+r.URL.Path != "GET /api/v1/export" {
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if f := r.URL.Query().Get("format"); f != "csv" {
			t.Errorf("format: %q", f)
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="sytue-one-acme-2026-09-21.zip"`)
		_, _ = io.WriteString(w, "PK\x03\x04el ZIP")
	}))
	defer srv.Close()
	h := NewHub(srv.URL, nil, nil)
	cuerpo, nombre, err := h.Export(context.Background(), HubCSV)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cuerpo.Close() }()
	if nombre != "sytue-one-acme-2026-09-21.zip" {
		t.Errorf("nombre: %q", nombre)
	}
	datos, err := io.ReadAll(cuerpo)
	if err != nil || string(datos) != "PK\x03\x04el ZIP" {
		t.Errorf("cuerpo: %q, %v", datos, err)
	}
}

func TestNombreAdjunto(t *testing.T) {
	for _, c := range []struct{ cabecera, quiero string }{
		{`attachment; filename="sytue-one-2026-09-21.zip"`, "sytue-one-2026-09-21.zip"},
		{`attachment; filename=copia.zip`, "copia.zip"},
		{"", "sytue-one.zip"},                      // sin cabecera: el nombre de respaldo
		{"attachment", "sytue-one.zip"},            // sin filename
		{"attachment; filename=", "sytue-one.zip"}, // vacío no es un nombre
		{"que no es una cabecera;;", "sytue-one.zip"},
	} {
		if got := nombreAdjunto(c.cabecera, "sytue-one.zip"); got != c.quiero {
			t.Errorf("%q -> %q, quiero %q", c.cabecera, got, c.quiero)
		}
	}
}

// TestHubBandeja: lo que la bandeja completa pide y el panel de la barra no —marcar todo
// y las reglas de silencio—, más la lista de lo desplegado y el filtro de la actividad.
func TestHubBandeja(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/notifications/read_all":
			_, _ = io.WriteString(w, `{"updated":4}`)
		case "GET /api/v1/notifications/prefs":
			_, _ = io.WriteString(w, `{"prefs":[{"source":"mail","muted":true},{"source":"drive","kind":"share","muted":false}]}`)
		case "PUT /api/v1/notifications/prefs":
			cuerpo, _ := io.ReadAll(r.Body)
			// El `kind` viaja siempre, vacío si no lo hay, como lo manda la web.
			if string(cuerpo) != `{"kind":"","muted":true,"source":"rooms"}` {
				t.Errorf("cuerpo del PUT: %s", cuerpo)
			}
			_, _ = io.WriteString(w, `{"source":"rooms","muted":true}`)
		case "GET /api/v1/apps":
			_, _ = io.WriteString(w, `{"apps":["mail","docs"]}`)
		case "GET /api/v1/recent":
			if q := r.URL.Query(); q.Get("sources") != "drive,mail" || q.Get("limit") != "20" {
				t.Errorf("recent: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"hits":[]}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	h, ctx := NewHub(srv.URL, nil, nil), context.Background()
	if err := h.MarkAllRead(ctx); err != nil {
		t.Fatal(err)
	}
	prefs, err := h.NotificationPrefs(ctx)
	if err != nil || len(prefs) != 2 || prefs[0].Source != "mail" || !prefs[0].Muted || prefs[1].Kind != "share" {
		t.Fatalf("prefs: %+v, %v", prefs, err)
	}
	if p, err := h.SetNotificationPref(ctx, HubNotificationPref{Source: "rooms", Muted: true}); err != nil || p.Source != "rooms" || !p.Muted {
		t.Fatalf("pref: %+v, %v", p, err)
	}
	if apps, err := h.Apps(ctx); err != nil || len(apps) != 2 || apps[1] != "docs" {
		t.Fatalf("apps: %v, %v", apps, err)
	}
	if _, err := h.RecentDe(ctx, []string{"drive", "mail"}, 20); err != nil {
		t.Fatal(err)
	}
}

// TestResolverUnIntentPideElEnlaceAlHub: el cuerpo es el `IntentRequest` del servicio y
// lo que vuelve, su `IntentResult`. Los nombres de campo se comprueban contra
// `services/hub/internal/domain`, para que un renombrado allí no deje el botón muerto.
func TestResolverUnIntentPideElEnlaceAlHub(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method+" "+r.URL.Path != "POST /api/v1/intents" {
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
		cuerpo, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(cuerpo), `"type":"create-event-from-mail"`) || !strings.Contains(string(cuerpo), `"subject":"Reunión"`) {
			t.Errorf("cuerpo: %s", cuerpo)
		}
		_, _ = io.WriteString(w, `{"type":"create-event-from-mail","url":"https://calendar.sytue.eu/?action=create&title=Reuni%C3%B3n","target":"calendar","label":"Crear evento"}`)
	}))
	defer srv.Close()
	r, err := NewHub(srv.URL, nil, nil).ResolveIntent(context.Background(), "create-event-from-mail",
		map[string]string{"subject": "Reunión"})
	if err != nil || r.Target != "calendar" || !strings.HasPrefix(r.URL, "https://calendar.sytue.eu/") {
		t.Fatalf("intent: %+v, %v", r, err)
	}
	dominio := webref.Read(t, "services/hub/internal/domain/domain.go")
	for _, campo := range []string{"`json:\"type\"`", "`json:\"params\"`", "`json:\"url\"`", "`json:\"target\"`", "`json:\"label\"`"} {
		if !strings.Contains(dominio, campo) {
			t.Errorf("services/hub ya no declara %s en el intent", campo)
		}
	}
}

// TestElInstaladorSeBajaPorSuURLOPorElHub: con URL firmada se baja de ahí y SIN token
// (el almacén rechaza dos autenticaciones a la vez); sin ella, por el Hub, con token.
// VISTO EN ROJO mandando el token también a la URL firmada.
func TestElInstaladorSeBajaPorSuURLOPorElHub(t *testing.T) {
	almacen := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			w.WriteHeader(http.StatusBadRequest) // lo que hace S3 con dos autenticaciones
			return
		}
		_, _ = io.WriteString(w, "desde el almacén")
	}))
	defer almacen.Close()
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/instaladores" && r.URL.Query().Get("plataforma") == "macos":
			_, _ = io.WriteString(w, `{"instaladores":[{"app":"hub","fichero":"sytue-hub-macos-universal.dmg","version":"3.6.0","sha256":"ab","bytes":16,"url":"`+almacen.URL+`/x"}]}`)
		case r.URL.Path == "/api/v1/instaladores/sytue-hub-macos-universal.dmg" && r.Header.Get("Authorization") == "Bearer tok":
			_, _ = io.WriteString(w, "desde el Hub")
		default:
			t.Errorf("petición inesperada: %s %v", r.URL, r.Header)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer hub.Close()
	h, ctx := NewHub(hub.URL, &sesionFalsa{tokens: []string{"tok"}}, nil), context.Background()
	lista, err := h.Instaladores(ctx, "macos")
	if err != nil || len(lista) != 1 || lista[0].Version != "3.6.0" || lista[0].SHA256 != "ab" {
		t.Fatalf("lista: %+v, %v", lista, err)
	}
	var b strings.Builder
	if err := h.BajarInstalador(ctx, lista[0], &b); err != nil || b.String() != "desde el almacén" {
		t.Errorf("por la URL firmada: %q, %v", b.String(), err)
	}
	b.Reset()
	lista[0].URL = ""
	if err := h.BajarInstalador(ctx, lista[0], &b); err != nil || b.String() != "desde el Hub" {
		t.Errorf("por el Hub: %q, %v", b.String(), err)
	}
}

// TestLaPaginaDeDescargasCuentaCadaInstaladorUnaVez: `/descargas` pide el catálogo de
// todas las plataformas y enseña el instalador de un bundle UNA vez, aunque el catálogo lo
// repita por cada app que lleva; sin bucket es «sin configurar» y no un fallo, pero un
// bucket que no contesta sí lo es; y al descargar se vuelve a pedir la plataforma, que la
// URL firmada de la lista caduca.
// VISTO EN ROJO sin quitar los repetidos (salían tres), bajando con la URL vieja y
// tomando cualquier 503 por «sin configurar».
func TestLaPaginaDeDescargasCuentaCadaInstaladorUnaVez(t *testing.T) {
	codigo503 := ""
	almacen := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nueva" {
			w.WriteHeader(http.StatusForbidden) // la firma de la lista ya ha caducado
			return
		}
		_, _ = io.WriteString(w, "DMG")
	}))
	defer almacen.Close()
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case codigo503 != "":
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":{"code":"`+codigo503+`","message":"x"},"request_id":"r"}`)
		case r.URL.Path == "/api/v1/instaladores" && !r.URL.Query().Has("plataforma"):
			_, _ = io.WriteString(w, `{"instaladores":[
				{"app":"hub","bundle":"one","plataforma":"macos","fichero":"sytue-one-macos-universal.dmg","version":"3.7.0","bytes":3,"url":"`+almacen.URL+`/vieja"},
				{"app":"mail","bundle":"one","plataforma":"macos","fichero":"sytue-one-macos-universal.dmg","version":"3.7.0","bytes":3,"url":"`+almacen.URL+`/vieja"},
				{"app":"hub","plataforma":"android","fichero":"sytue-hub-android.apk","version":"3.7.0","bytes":5}]}`)
		case r.URL.Path == "/api/v1/instaladores" && r.URL.Query().Get("plataforma") == "macos":
			_, _ = io.WriteString(w, `{"instaladores":[{"app":"hub","plataforma":"macos","fichero":"sytue-one-macos-universal.dmg","bytes":3,"url":"`+almacen.URL+`/nueva"}]}`)
		default:
			t.Errorf("petición inesperada: %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer hub.Close()
	h, ctx := NewHub(hub.URL, &sesionFalsa{tokens: []string{"tok"}}, nil), context.Background()
	d, err := h.Descargas(ctx)
	if err != nil || len(d.Instaladores) != 2 || d.Version != "v3.7.0" || d.Motivo != "" {
		t.Fatalf("descargas: %+v, %v", d, err)
	}
	cuerpo, nombre, err := h.Descargar(ctx, d.Instaladores[0])
	if err != nil {
		t.Fatal(err)
	}
	datos, _ := io.ReadAll(cuerpo)
	_ = cuerpo.Close()
	if string(datos) != "DMG" || nombre != "sytue-one-macos-universal.dmg" {
		t.Errorf("descargar: %q %q", datos, nombre)
	}
	codigo503 = "installers_unconfigured"
	if d, err := h.Descargas(ctx); err != nil || d.Motivo != DescargasSinConfigurar || d.Instaladores == nil {
		t.Errorf("sin bucket: %+v, %v", d, err)
	}
	codigo503 = "unavailable"
	if d, err := h.Descargas(ctx); err == nil {
		t.Errorf("un bucket que no contesta es un fallo, no «sin configurar»: %+v", d)
	}
}
