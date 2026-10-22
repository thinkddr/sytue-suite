// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate_test

// Tests OFFLINE de Microsoft Teams: un Graph simulado (perfil, equipos, canales,
// miembros, mensajes y ficheros de canal) y un Sytue Rooms simulado. Sin red.
//
// Lo que se prueba es lo que se le promete a quien migra: que con UNA autorización
// se descubre lo que hay, que los canales y su gente llegan a Rooms, que quien no
// tiene cuenta en Sytue se queda fuera CON SU MOTIVO en vez de convertirse en un
// miembro fantasma, y que el historial aterriza en Drive con autor y fecha REALES.

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/migrate"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
)

// --- Microsoft Graph simulado (consentimiento delegado) ----------------------

func graphDelegatedServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer ms-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
		return true
	}

	mux.HandleFunc("/v1.0/me", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		writeJSON(w, map[string]any{
			"userPrincipalName": "ana@contoso.com", "displayName": "Ana Ruiz", "mail": "ana@contoso.com",
		})
	})
	// Sin permiso para leer la organización: es el caso NORMAL con el consentimiento
	// de una persona, y el descubrimiento debe apañárselas igual.
	mux.HandleFunc("/v1.0/organization", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	mux.HandleFunc("/v1.0/me/joinedTeams", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		writeJSON(w, map[string]any{"value": []map[string]any{
			{"id": "team-1", "displayName": "Producto", "description": "Equipo de producto"},
		}})
	})
	mux.HandleFunc("/v1.0/teams/team-1/channels", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		writeJSON(w, map[string]any{"value": []map[string]any{
			{"id": "chan-1", "displayName": "General", "description": "Todo el equipo", "membershipType": "standard"},
			{"id": "chan-2", "displayName": "Con el cliente", "membershipType": "shared"},
		}})
	})
	mux.HandleFunc("/v1.0/teams/team-1/channels/chan-1/members", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		writeJSON(w, map[string]any{"value": []map[string]any{
			{"displayName": "Ana Ruiz", "email": "ana@contoso.com"},
			{"displayName": "Externo", "email": "quien@otra.example"},
			{"displayName": "Sin correo"},
		}})
	})
	mux.HandleFunc("/v1.0/teams/team-1/channels/chan-1/messages", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		writeJSON(w, map[string]any{"value": []map[string]any{{
			"id": "msg-1", "createdDateTime": "2024-03-01T09:12:00Z", "messageType": "message",
			"from": map[string]any{"user": map[string]any{"displayName": "Ana Ruiz"}},
			"body": map[string]any{"contentType": "html", "content": "<p>Hola <b>equipo</b></p>"},
		}}})
	})
	mux.HandleFunc("/v1.0/teams/team-1/channels/chan-1/messages/msg-1/replies", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		writeJSON(w, map[string]any{"value": []map[string]any{{
			"id": "rep-1", "createdDateTime": "2024-03-01T09:20:00Z", "messageType": "message",
			"from": map[string]any{"user": map[string]any{"displayName": "Luis"}},
			"body": map[string]any{"contentType": "html", "content": "Voy con ello"},
		}}})
	})
	mux.HandleFunc("/v1.0/teams/team-1/channels/chan-1/filesFolder", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		writeJSON(w, map[string]any{
			"id": "root-1", "parentReference": map[string]any{"driveId": "drive-1"},
		})
	})
	mux.HandleFunc("/v1.0/drives/drive-1/items/root-1/children", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		writeJSON(w, map[string]any{"value": []map[string]any{{
			"id": "file-1", "name": "acta.txt", "size": 4,
			"file":                         map[string]any{"mimeType": "text/plain"},
			"lastModifiedDateTime":         "2024-03-01T10:00:00Z",
			"@microsoft.graph.downloadUrl": "http://" + r.Host + "/blob/file-1",
		}}})
	})
	mux.HandleFunc("/blob/file-1", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("acta"))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// m365Delegated es la instancia de una conexión hecha por CONSENTIMIENTO: sin
// credenciales de aplicación, solo el token que inyecta la pasarela.
func m365Delegated(base string) connector.Instance {
	return connector.Instance{
		ID: "inst-1", OrgID: "org-1", TemplateKey: "microsoft365",
		Config:    map[string]string{"graph_base": base},
		Secrets:   map[string]string{"bearer": "ms-token"},
		HasBearer: true,
	}
}

func msRuntime() connector.Runtime {
	return policy.BuildRuntime(policy.RuntimeParams{
		OrgID: "org-1", InstanceID: "inst-1", Bearer: "ms-token", Log: slog.Default(),
	})
}

// --- Sytue Rooms simulado ----------------------------------------------------

type fakeRooms struct {
	srv      *httptest.Server
	teams    map[string]string   // nombre → id
	channels map[string]string   // "teamID/nombre" → id
	members  map[string][]string // channelID → sujetos añadidos
}

func newFakeRooms(t *testing.T) *fakeRooms {
	t.Helper()
	f := &fakeRooms{teams: map[string]string{}, channels: map[string]string{}, members: map[string][]string{}}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}

	mux.HandleFunc("/api/v1/teams", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			list := make([]map[string]any, 0, len(f.teams))
			for name, id := range f.teams {
				list = append(list, map[string]any{"id": id, "name": name})
			}
			writeJSON(w, http.StatusOK, map[string]any{"teams": list})
			return
		}
		var body struct{ Name string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		id := "rooms-team-" + body.Name
		f.teams[body.Name] = id
		writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": body.Name})
	})
	mux.HandleFunc("/api/v1/teams/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/teams/")
		teamID, tail, _ := strings.Cut(rest, "/")
		switch {
		case tail == "channels" && r.Method == http.MethodGet:
			list := make([]map[string]any, 0)
			for key, id := range f.channels {
				if owner, name, _ := strings.Cut(key, "/"); owner == teamID {
					list = append(list, map[string]any{"id": id, "name": name})
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{"channels": list})
		case tail == "channels":
			var body struct{ Name string }
			_ = json.NewDecoder(r.Body).Decode(&body)
			id := "rooms-chan-" + body.Name
			f.channels[teamID+"/"+body.Name] = id
			writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": body.Name})
		case tail == "members":
			writeJSON(w, http.StatusCreated, map[string]any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	mux.HandleFunc("/api/v1/channels/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/channels/")
		channelID, tail, _ := strings.Cut(rest, "/")
		if tail != "members" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var body struct{ Subject string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.members[channelID] = append(f.members[channelID], body.Subject)
		writeJSON(w, http.StatusCreated, map[string]any{})
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// fakePeople es el directorio de Sytue: solo conoce a Ana.
type fakePeople struct{}

func (fakePeople) Subject(_ context.Context, email string) (string, string, bool, error) {
	if strings.EqualFold(email, "ana@contoso.com") {
		return "sub-ana", "Ana Ruiz", true, nil
	}
	return "", "", false, nil
}

// --- Tests -------------------------------------------------------------------

// TestTeamsLleganARooms: los canales se recrean, la gente con cuenta entra y la que
// no la tiene se queda fuera CON SU MOTIVO. Un canal compartido con otra
// organización no se toca.
func TestTeamsLleganARooms(t *testing.T) {
	allowLoopback(t)
	graph := graphDelegatedServer(t)
	rooms := newFakeRooms(t)

	j := job("job-teams", domain.SourceM365, domain.DomainTeams)
	j.SourceUser = "" // consentimiento: la cuenta que autorizó.
	st := newMemStore("org-1")
	st.put(j)
	src, err := migrate.NewSource(context.Background(), msRuntime(), m365Delegated(graph.URL), j)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	deps := migrate.Deps{
		Src:    src,
		Chat:   migrate.NewSytueRooms(rooms.srv.URL, migrate.StaticToken("tok"), nil),
		People: fakePeople{},
		Store:  st,
	}
	if err := migrate.Run(context.Background(), deps, j); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, _ := st.get("job-teams")
	if got.Status != domain.MigrationDone {
		t.Fatalf("estado final = %q (%s)", got.Status, got.LastError)
	}
	if rooms.teams["Producto"] == "" {
		t.Fatalf("no se creó el equipo: %v", rooms.teams)
	}
	if rooms.channels["rooms-team-Producto/General"] == "" {
		t.Fatalf("no se creó el canal: %v", rooms.channels)
	}
	if subs := rooms.members["rooms-chan-General"]; len(subs) != 1 || subs[0] != "sub-ana" {
		t.Fatalf("miembros del canal = %v; solo Ana tiene cuenta en Sytue", subs)
	}

	items, _ := st.LoadMigrationMap(context.Background(), "job-teams")
	var externo, compartido string
	for _, it := range items {
		if strings.Contains(it.Path, "quien@otra.example") {
			externo = it.SkipReason
		}
		if strings.Contains(it.Path, "Con el cliente") {
			compartido = it.SkipReason
		}
	}
	if externo == "" {
		t.Error("la persona sin cuenta en Sytue debía quedar registrada con su motivo")
	}
	if !strings.Contains(compartido, "compartido") {
		t.Errorf("el canal compartido con otra organización debía omitirse con motivo: %q", compartido)
	}

	// Segunda pasada: idempotente, ni un canal ni un miembro más.
	before := len(rooms.channels) + len(rooms.members["rooms-chan-General"])
	j2, _ := st.get("job-teams")
	if err := migrate.Run(context.Background(), deps, *j2); err != nil {
		t.Fatalf("Run (2.ª pasada): %v", err)
	}
	if after := len(rooms.channels) + len(rooms.members["rooms-chan-General"]); after != before {
		t.Errorf("la segunda pasada volvió a escribir en Rooms (%d → %d)", before, after)
	}
}

// TestHistorialDeTeamsConservaAutorYFecha: el contenido del canal aterriza en Drive
// —una carpeta por equipo y canal— y la transcripción lleva el autor y la fecha
// ORIGINALES, que es lo que hace honesto no republicar los mensajes en Rooms.
func TestHistorialDeTeamsConservaAutorYFecha(t *testing.T) {
	allowLoopback(t)
	graph := graphDelegatedServer(t)

	j := job("job-team-files", domain.SourceM365, domain.DomainTeamFiles)
	j.SourceUser = ""
	st := newMemStore("org-1")
	st.put(j)
	src, err := migrate.NewSource(context.Background(), msRuntime(), m365Delegated(graph.URL), j)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	dst := newFakeSytue()
	if err := migrate.Run(context.Background(), deps(src, dst, st), j); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := st.get("job-team-files")
	if got.Status != domain.MigrationDone {
		t.Fatalf("estado final = %q (%s)", got.Status, got.LastError)
	}

	// Estructura: Producto/ y Producto/General/.
	var tieneEquipo, tieneCanal bool
	for _, path := range dst.folders {
		if strings.HasSuffix(path, "/Producto") {
			tieneEquipo = true
		}
		if strings.HasSuffix(path, "/General") {
			tieneCanal = true
		}
	}
	if !tieneEquipo || !tieneCanal {
		t.Fatalf("no se recreó la estructura equipo/canal: %v", dst.folders)
	}

	var historial string
	for id, path := range dst.files {
		if strings.HasSuffix(path, "historial-de-conversacion.md") {
			historial = dst.bodies[id]
		}
	}
	if historial == "" {
		t.Fatal("no se subió la transcripción del canal")
	}
	for _, quiero := range []string{"Ana Ruiz", "2024-03-01 09:12 UTC", "Hola equipo", "> **Luis**"} {
		if !strings.Contains(historial, quiero) {
			t.Errorf("la transcripción no conserva %q:\n%s", quiero, historial)
		}
	}
}

// TestDescubreM365ConUnLogin: con la conexión ya autorizada, el asistente sabe
// quién entró, cuántos equipos hay y en qué inquilino está — sin pedir el inquilino
// ni permisos de administrador.
func TestDescubreM365ConUnLogin(t *testing.T) {
	allowLoopback(t)
	graph := graphDelegatedServer(t)

	d, err := migrate.Discover(context.Background(), msRuntime(), m365Delegated(graph.URL), domain.SourceM365)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if d.ConnectedAs != "ana@contoso.com" || d.Scope != migrate.ScopeUser {
		t.Fatalf("descubrimiento = %+v", d)
	}
	if d.TenantID != "contoso.com" {
		t.Errorf("inquilino = %q; sin permiso para leer la organización se deduce del dominio", d.TenantID)
	}
	if !d.CanExpandToOrg {
		t.Error("con un login de usuario, Microsoft SÍ permite ampliar a toda la organización")
	}
	for _, it := range d.Items {
		if it.Domain == domain.DomainTeams && it.Count != 1 {
			t.Errorf("equipos descubiertos = %d, quiero 1", it.Count)
		}
	}
}

// TestAdminConsentURL: el enlace del paso extra lleva al inquilino correcto y pide
// los permisos ya consentibles de una vez.
func TestAdminConsentURL(t *testing.T) {
	t.Parallel()
	u := migrate.AdminConsentURL("", "contoso.com", "app-123")
	for _, quiero := range []string{
		"https://login.microsoftonline.com/contoso.com/v2.0/adminconsent",
		"client_id=app-123",
		"scope=https%3A%2F%2Fgraph.microsoft.com%2F.default",
	} {
		if !strings.Contains(u, quiero) {
			t.Errorf("la URL de consentimiento no contiene %q: %s", quiero, u)
		}
	}
	// Sin inquilino conocido, Microsoft lo resuelve al entrar el administrador.
	if !strings.Contains(migrate.AdminConsentURL("", "", "app-123"), "/common/v2.0/adminconsent") {
		t.Error("sin inquilino, el enlace debe ir a /common")
	}
}
