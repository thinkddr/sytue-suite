// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate_test

// Tests OFFLINE del origen GitHub y del destino Sytue Git: un GitHub simulado
// (repositorios, gists y perfil) y una forja simulada que habla la API de Gitea.
// Sin red ni servicios reales.
//
// Lo que se prueba aquí es lo que hace creíble al asistente automático: que con
// UNA autorización se descubre lo que hay, que los repositorios llegan a la forja
// a nombre de la persona correcta, que una segunda pasada no vuelve a escribir
// nada y que lo que no se puede traer sale en el informe con su motivo.

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

// bearerRuntime es el Runtime de una conexión hecha por CONSENTIMIENTO: la pasarela
// inyecta el access token en cada salida, igual que en producción.
func bearerRuntime() connector.Runtime {
	return policy.BuildRuntime(policy.RuntimeParams{
		OrgID: "org-1", InstanceID: "inst-1", Bearer: "gh-token", Log: slog.Default(),
	})
}

func githubInstance(base string) connector.Instance {
	return connector.Instance{
		ID: "inst-1", OrgID: "org-1", TemplateKey: "github",
		Config:    map[string]string{migrate.ConfigGitHubAPIBase: base},
		Secrets:   map[string]string{"bearer": "gh-token"},
		HasBearer: true,
	}
}

// --- GitHub simulado ---------------------------------------------------------

// githubServer sirve el perfil, dos páginas de repositorios y un gist. Exige el
// Bearer inyectado: si la pasarela no lo pusiera, todo devolvería 401 y el test
// fallaría por la razón correcta.
func githubServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer gh-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
		return true
	}
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}

	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		writeJSON(w, map[string]any{
			"login": "alice", "name": "Alice", "email": "alice@old.example",
			"public_repos": 1, "total_private_repos": 1, "public_gists": 1, "private_gists": 0,
		})
	})
	mux.HandleFunc("/user/repos", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		if r.URL.Query().Get("page") != "1" {
			writeJSON(w, []any{}) // una sola página.
			return
		}
		writeJSON(w, []map[string]any{
			{
				"id": 1, "name": "web", "full_name": "alice/web",
				"owner":     map[string]any{"login": "alice", "type": "User"},
				"clone_url": "https://github.com/alice/web.git", "private": true,
				"description": "sitio",
			},
			{
				"id": 2, "name": "infra", "full_name": "acme/infra",
				"owner":     map[string]any{"login": "acme", "type": "Organization"},
				"clone_url": "https://github.com/acme/infra.git", "private": true,
			},
		})
	})
	mux.HandleFunc("/gists", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		if r.URL.Query().Get("page") != "1" {
			writeJSON(w, []any{})
			return
		}
		writeJSON(w, []map[string]any{{
			"id": "abc1234567", "description": "notas de despliegue", "public": false,
			"files": map[string]any{"deploy.sh": map[string]any{"filename": "deploy.sh"}},
		}})
	})
	mux.HandleFunc("/gists/abc1234567", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		writeJSON(w, map[string]any{"files": map[string]any{
			"deploy.sh": map[string]any{
				"filename": "deploy.sh", "type": "application/x-sh",
				"size": 12, "truncated": false, "content": "echo hola\n",
			},
		}})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// --- Forja simulada (API de Gitea) -------------------------------------------

// fakeForge es Sytue Git: recuerda qué se importó y con qué cabecera Sudo, que es
// lo que garantiza que el repositorio acaba siendo de esa persona.
type fakeForge struct {
	srv       *httptest.Server
	imported  map[string]string // "owner/nombre" → Sudo con el que se creó
	orgs      map[string]bool
	occupied  map[string]bool // rutas que la forja dice que ya existen
	tokenSeen string          // auth_token que recibió para clonar
}

func newFakeForge(t *testing.T) *fakeForge {
	t.Helper()
	f := &fakeForge{imported: map[string]string{}, orgs: map[string]bool{}, occupied: map[string]bool{}}
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/users/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		out := map[string]any{"data": []any{}}
		if q == "grace@acme.test" {
			out = map[string]any{"data": []map[string]any{{"login": "grace", "email": "grace@acme.test"}}}
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("/api/v1/orgs", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username string `json:"username"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.orgs[body.Username] = true
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/api/v1/orgs/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/api/v1/orgs/")
		if !f.orgs[name] {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("/api/v1/repos/migrate", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RepoOwner string `json:"repo_owner"`
			RepoName  string `json:"repo_name"`
			AuthToken string `json:"auth_token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.tokenSeen = body.AuthToken
		f.imported[body.RepoOwner+"/"+body.RepoName] = r.Header.Get("Sudo")
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/api/v1/repos/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/repos/")
		if f.occupied[path] || f.imported[path] != "" {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// --- Tests -------------------------------------------------------------------

// TestReposLleganALaForja: los repositorios se importan a nombre de la persona
// destino, los de una organización crean su organización equivalente, y la forja
// recibe la credencial con la que clonar lo privado.
func TestReposLleganALaForja(t *testing.T) {
	allowLoopback(t)
	gh := githubServer(t)
	forge := newFakeForge(t)

	j := job("job-repos", domain.SourceGitHub, domain.DomainRepos)
	j.SourceUser = "" // el asistente automático migra lo de la cuenta que autorizó.
	st := newMemStore("org-1")
	st.put(j)
	src, err := migrate.NewSource(context.Background(), bearerRuntime(), githubInstance(gh.URL), j)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	deps := migrate.Deps{
		Src:   src,
		Git:   migrate.NewSytueGit(forge.srv.URL, "forge-admin-token", nil),
		Store: st,
	}

	if err := migrate.Run(context.Background(), deps, j); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := st.get("job-repos")
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Status != domain.MigrationDone {
		t.Fatalf("estado final = %q (%s)", got.Status, got.LastError)
	}
	if got.Counts.Migrated != 2 || got.Counts.Skipped != 0 || got.Counts.Errors != 0 {
		t.Fatalf("contadores = %+v; se esperaban 2 repositorios migrados", got.Counts)
	}
	if forge.imported["grace/web"] != "grace" {
		t.Errorf("el repositorio personal no se creó a nombre de grace: %v", forge.imported)
	}
	if forge.imported["acme/infra"] != "grace" {
		t.Errorf("el repositorio de la organización no llegó a acme/: %v", forge.imported)
	}
	if !forge.orgs["acme"] {
		t.Error("no se creó la organización acme en la forja")
	}
	if forge.tokenSeen != "gh-token" {
		t.Errorf("la forja no recibió la credencial para clonar lo privado: %q", forge.tokenSeen)
	}

	// Segunda pasada: idempotencia estricta, ni una importación más.
	before := len(forge.imported)
	j2, _ := st.get("job-repos")
	if err := migrate.Run(context.Background(), deps, *j2); err != nil {
		t.Fatalf("Run (2.ª pasada): %v", err)
	}
	if len(forge.imported) != before {
		t.Errorf("la segunda pasada volvió a importar: %v", forge.imported)
	}
}

// TestRepoOcupadoSeOmiteConMotivo: si la ruta ya está ocupada en la forja, no se
// machaca nada — se registra como omitido con su porqué, que es lo que acaba
// leyendo quien mira el informe.
func TestRepoOcupadoSeOmiteConMotivo(t *testing.T) {
	allowLoopback(t)
	gh := githubServer(t)
	forge := newFakeForge(t)
	forge.occupied["grace/web"] = true

	j := job("job-ocupado", domain.SourceGitHub, domain.DomainRepos)
	j.SourceUser = ""
	st := newMemStore("org-1")
	st.put(j)
	src, err := migrate.NewSource(context.Background(), bearerRuntime(), githubInstance(gh.URL), j)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	deps := migrate.Deps{Src: src, Git: migrate.NewSytueGit(forge.srv.URL, "t", nil), Store: st}

	if err := migrate.Run(context.Background(), deps, j); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := st.get("job-ocupado")
	if got.Counts.Migrated != 1 || got.Counts.Skipped != 1 || got.Counts.Errors != 0 {
		t.Fatalf("contadores = %+v; se esperaba 1 migrado y 1 omitido", got.Counts)
	}
	items, _ := st.LoadMigrationMap(context.Background(), "job-ocupado")
	var reason string
	for _, it := range items {
		if it.Path == "alice/web" {
			reason = it.SkipReason
		}
	}
	if !strings.Contains(reason, "grace/web") {
		t.Errorf("el motivo de la omisión no dice qué pasó: %q", reason)
	}
}

// TestGistsAterrizanEnDrive: cada gist es una carpeta con sus ficheros, y el
// contenido llega íntegro.
func TestGistsAterrizanEnDrive(t *testing.T) {
	allowLoopback(t)
	gh := githubServer(t)

	j := job("job-gists", domain.SourceGitHub, domain.DomainGists)
	j.SourceUser = ""
	st := newMemStore("org-1")
	st.put(j)
	src, err := migrate.NewSource(context.Background(), bearerRuntime(), githubInstance(gh.URL), j)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	dst := newFakeSytue()
	if err := migrate.Run(context.Background(), deps(src, dst, st), j); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := st.get("job-gists")
	if got.Status != domain.MigrationDone {
		t.Fatalf("estado final = %q (%s)", got.Status, got.LastError)
	}
	if got.Counts.Migrated != 2 { // la carpeta del gist + su fichero
		t.Fatalf("contadores = %+v; se esperaban 2 entidades", got.Counts)
	}
	var body string
	for _, b := range dst.bodies {
		body = b
	}
	if body != "echo hola\n" {
		t.Errorf("el contenido del gist no llegó íntegro: %q", body)
	}
}

// TestDescubreLoQueHayEnGitHub: con la conexión ya autorizada, el asistente sabe
// quién se conectó y cuánto hay, sin que nadie teclee nada.
func TestDescubreLoQueHayEnGitHub(t *testing.T) {
	allowLoopback(t)
	gh := githubServer(t)

	d, err := migrate.Discover(context.Background(), bearerRuntime(), githubInstance(gh.URL), domain.SourceGitHub)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if d.ConnectedAs != "alice" || d.Scope != migrate.ScopeUser {
		t.Fatalf("descubrimiento = %+v", d)
	}
	want := map[domain.MigrationDomain]int{domain.DomainRepos: 2, domain.DomainGists: 1}
	for _, it := range d.Items {
		if want[it.Domain] != it.Count {
			t.Errorf("%s: contador = %d, quiero %d", it.Domain, it.Count, want[it.Domain])
		}
	}
}

// TestGitHubSinAutorizarNoArranca: sin consentimiento no se intenta nada, y el
// mensaje dice qué falta en vez de fallar en mitad de la copia.
func TestGitHubSinAutorizarNoArranca(t *testing.T) {
	inst := githubInstance("https://api.github.com")
	inst.HasBearer = false
	inst.Secrets = nil

	_, err := migrate.NewSource(context.Background(), bearerRuntime(), inst,
		job("job-x", domain.SourceGitHub, domain.DomainRepos))
	if err == nil || !strings.Contains(err.Error(), "Conectar con GitHub") {
		t.Fatalf("error = %v; se esperaba que pidiera terminar la conexión", err)
	}
}

// TestGitHubNoTieneContactos: el dominio que un origen no sabe traer se rechaza al
// construirlo, no en mitad del trabajo.
func TestGitHubNoTieneContactos(t *testing.T) {
	_, err := migrate.NewSource(context.Background(), bearerRuntime(), githubInstance("https://api.github.com"),
		job("job-y", domain.SourceGitHub, domain.DomainContacts))
	if err == nil || !strings.Contains(err.Error(), "contacts") {
		t.Fatalf("error = %v; se esperaba el rechazo del dominio", err)
	}
}

// deps arma las dependencias de una ejecución con destino Drive/Contacts.
func deps(src migrate.Source, dst migrate.Destination, st migrate.Store) migrate.Deps {
	return migrate.Deps{Src: src, Dst: dst, Store: st}
}

// --- Google con un solo login -------------------------------------------------

// TestGoogleConSoloUnLogin: conectada por consentimiento y SIN cuenta de servicio,
// la migración trae el Drive de quien autorizó. Es el camino que hace que el
// asistente pida un login y nada más.
func TestGoogleConSoloUnLogin(t *testing.T) {
	allowLoopback(t)
	srv := googleServer(t)

	inst := connector.Instance{
		ID: "inst-1", OrgID: "org-1", TemplateKey: "google-workspace",
		Config: map[string]string{
			"google_drive_base":  srv.URL,
			"google_people_base": srv.URL,
		},
		Secrets:   map[string]string{"bearer": "goog-token"},
		HasBearer: true, // sin service_account_key: no hay delegación, y no hace falta.
	}
	j := job("job-google-consent", domain.SourceGoogle, domain.DomainDrive)
	j.SourceUser = "" // la cuenta que autorizó, no una impersonada.

	st := newMemStore("org-1")
	st.put(j)
	// El Runtime inyecta el token del consentimiento, como en producción.
	rt := policy.BuildRuntime(policy.RuntimeParams{
		OrgID: "org-1", InstanceID: "inst-1", Bearer: "goog-token", Log: slog.Default(),
	})
	src, err := migrate.NewSource(context.Background(), rt, inst, j)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	dst := newFakeSytue()
	if err := migrate.Run(context.Background(), deps(src, dst, st), j); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := st.get("job-google-consent")
	if got.Status != domain.MigrationDone {
		t.Fatalf("estado final = %q (%s)", got.Status, got.LastError)
	}
	if got.Counts.Migrated == 0 {
		t.Fatalf("no se migró nada con el consentimiento: %+v", got.Counts)
	}
}
