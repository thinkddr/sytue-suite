// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate_test

// Tests OFFLINE de la migración de contenido: orígenes simulados con httptest
// (Microsoft Graph y Google Drive/People), un Sytue de destino también simulado y
// un store en memoria. Sin red ni SaaS reales.
//
// El egress al origen va por el Runtime REAL de la pasarela (policy.BuildRuntime →
// ssrfhttp), así que los tests relajan la barrera SSRF para loopback con
// SetIPPolicyForTest y la restauran. Sin t.Parallel(): la política SSRF es una var
// de proceso.

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/migrate"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
)

func allowLoopback(t *testing.T) {
	t.Helper()
	t.Cleanup(ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false }))
}

func runtime() connector.Runtime {
	// Graph y GWS mintean su propio token: el Runtime no inyecta bearer.
	return policy.BuildRuntime(policy.RuntimeParams{OrgID: "org-1", InstanceID: "inst-1", Log: slog.Default()})
}

// --- Store en memoria (sin base de datos) ------------------------------------

// memStore imita migration_jobs + migration_map con la MISMA semántica que el
// store real: el mapa es por (org, job) y las escrituras respetan el estado.
type memStore struct {
	org      string
	jobs     map[string]*domain.MigrationJob
	items    map[string]map[string]domain.MigrationItem // org/job → sourceID → item
	starts   int
	finishes int
}

func newMemStore(org string) *memStore {
	return &memStore{org: org, jobs: map[string]*domain.MigrationJob{}, items: map[string]map[string]domain.MigrationItem{}}
}

// key aísla por organización igual que hace RLS en el store real: un job de otra
// org es sencillamente invisible.
func (m *memStore) key(jobID string) string { return m.org + "/" + jobID }

func (m *memStore) put(job domain.MigrationJob) *domain.MigrationJob {
	j := job
	j.OrgID = m.org
	m.jobs[m.key(job.ID)] = &j
	return &j
}

func (m *memStore) get(jobID string) (*domain.MigrationJob, error) {
	j, ok := m.jobs[m.key(jobID)]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return j, nil
}

func (m *memStore) LoadMigrationMap(_ context.Context, jobID string) ([]domain.MigrationItem, error) {
	if _, err := m.get(jobID); err != nil {
		return nil, err
	}
	out := make([]domain.MigrationItem, 0, len(m.items[m.key(jobID)]))
	for _, it := range m.items[m.key(jobID)] {
		out = append(out, it)
	}
	return out, nil
}

func (m *memStore) RecordMigrationItems(_ context.Context, jobID string, items []domain.MigrationItem) error {
	if _, err := m.get(jobID); err != nil {
		return err
	}
	k := m.key(jobID)
	if m.items[k] == nil {
		m.items[k] = map[string]domain.MigrationItem{}
	}
	for _, it := range items {
		m.items[k][it.SourceID] = it
	}
	return nil
}

func (m *memStore) SaveMigrationProgress(_ context.Context, jobID, cursor string, c domain.MigrationCounts) (domain.MigrationStatus, error) {
	j, err := m.get(jobID)
	if err != nil {
		return "", err
	}
	if j.Status == domain.MigrationRunning {
		j.Cursor, j.Counts = cursor, c
	}
	return j.Status, nil
}

func (m *memStore) StartMigrationJob(_ context.Context, jobID string) error {
	j, err := m.get(jobID)
	if err != nil {
		return err
	}
	m.starts++
	j.Status = domain.MigrationRunning
	return nil
}

func (m *memStore) FinishMigrationJob(_ context.Context, jobID string, st domain.MigrationStatus, lastErr string) error {
	j, err := m.get(jobID)
	if err != nil {
		return err
	}
	m.finishes++
	j.Status, j.LastError = st, lastErr
	return nil
}

// --- Destino Sytue simulado ---------------------------------------------------

// fakeSytue es un Sytue Drive/Contacts mínimo en memoria que CUENTA las
// escrituras: es la sonda de la idempotencia (segunda pasada ⇒ 0 escrituras).
type fakeSytue struct {
	folders  map[string]string // id → "parent/nombre"
	files    map[string]string // id → "carpeta/nombre"
	contacts map[string]string // id → nombre
	bodies   map[string]string // id de fichero → contenido subido
	writes   int
	n        int
	failFrom int // > 0: la escritura número failFrom (y siguientes) falla
}

func newFakeSytue() *fakeSytue {
	return &fakeSytue{
		folders:  map[string]string{"root": "/"},
		files:    map[string]string{},
		contacts: map[string]string{},
		bodies:   map[string]string{},
	}
}

func (f *fakeSytue) Root(context.Context) (string, error) { return "root", nil }

func (f *fakeSytue) next(prefix string) (string, error) {
	f.writes++
	if f.failFrom > 0 && f.writes >= f.failFrom {
		return "", errors.New("sytue simulado: escritura rechazada")
	}
	f.n++
	return fmt.Sprintf("%s-%d", prefix, f.n), nil
}

func (f *fakeSytue) CreateFolder(_ context.Context, parentID, name string) (string, error) {
	id, err := f.next("fld")
	if err != nil {
		return "", err
	}
	f.folders[id] = parentID + "/" + name
	return id, nil
}

func (f *fakeSytue) UploadFile(_ context.Context, folderID, name, _ string, body io.Reader) (string, error) {
	id, err := f.next("fil")
	if err != nil {
		return "", err
	}
	b, rerr := io.ReadAll(body)
	if rerr != nil {
		return "", rerr
	}
	f.files[id] = folderID + "/" + name
	f.bodies[id] = string(b)
	return id, nil
}

func (f *fakeSytue) AddressBook(context.Context) (string, error) { return "book-1", nil }

func (f *fakeSytue) CreateContact(_ context.Context, _ string, c migrate.Contact) (string, error) {
	id, err := f.next("con")
	if err != nil {
		return "", err
	}
	f.contacts[id] = c.FormattedName
	return id, nil
}

// --- Orígenes simulados -------------------------------------------------------

// graphServer levanta un Microsoft Graph mínimo: token endpoint de Entra, la
// jerarquía de OneDrive de un usuario en dos niveles (con paginación por
// @odata.nextLink en la raíz), el contenido de los ficheros y los contactos.
func graphServer(t *testing.T) (*httptest.Server, *bool) {
	t.Helper()
	var srv *httptest.Server
	// down corta el listado de la subcarpeta: simula que el origen se cae a media
	// jerarquía (para probar la reanudación por cursor).
	down := false
	mux := http.NewServeMux()

	mux.HandleFunc("/tenant-1/oauth2/v2.0/token", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"access_token": "tok-m365", "expires_in": 3600})
	})
	mux.HandleFunc("/v1.0/users/alice@old.example/drive/root/children", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			writeJSON(w, map[string]any{"value": []any{
				item("f-notas", "notas.txt", 5, srv.URL+"/dl/f-notas"),
			}})
			return
		}
		writeJSON(w, map[string]any{
			"value": []any{
				folder("d-proyectos", "Proyectos"),
				item("f-leeme", "leeme.txt", 4, srv.URL+"/dl/f-leeme"),
			},
			"@odata.nextLink": srv.URL + "/v1.0/users/alice@old.example/drive/root/children?page=2",
		})
	})
	mux.HandleFunc("/v1.0/users/alice@old.example/drive/items/d-proyectos/children", func(w http.ResponseWriter, _ *http.Request) {
		if down {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]any{"value": []any{
			folder("d-2026", "2026"),
		}})
	})
	mux.HandleFunc("/v1.0/users/alice@old.example/drive/items/d-2026/children", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"value": []any{
			item("f-plan", "plan.pdf", 8, srv.URL+"/dl/f-plan"),
		}})
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("bytes:" + strings.TrimPrefix(r.URL.Path, "/dl/")))
	})
	mux.HandleFunc("/v1.0/users/alice@old.example/contacts", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"value": []any{
			map[string]any{
				"id": "c-1", "displayName": "Ana Ruiz", "givenName": "Ana", "surname": "Ruiz",
				"emailAddresses": []any{map[string]any{"address": "ana@old.example"}},
				"businessPhones": []any{"+34600000000"},
			},
			map[string]any{"id": "c-2", "displayName": "", "givenName": "", "surname": ""}, // sin nombre ⇒ omitido
		}})
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &down
}

func folder(id, name string) map[string]any {
	return map[string]any{"id": id, "name": name, "folder": map[string]any{"childCount": 1}}
}

func item(id, name string, size int, dl string) map[string]any {
	return map[string]any{
		"id": id, "name": name, "size": size,
		"file":                         map[string]any{"mimeType": "text/plain"},
		"lastModifiedDateTime":         "2026-01-02T03:04:05Z",
		"@microsoft.graph.downloadUrl": dl,
	}
}

// googleServer levanta un Google Drive v3 + People API mínimos sobre el mismo
// host: token endpoint (jwt-bearer), listados por carpeta, descarga alt=media y
// connections. Incluye un Google Doc NATIVO para comprobar que se omite con motivo.
func googleServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"access_token": "tok-gws", "expires_in": 3600})
	})
	mux.HandleFunc("/drive/v3/files", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Query().Get("q"), "'root' in parents"):
			writeJSON(w, map[string]any{"files": []any{
				gfile("g-dir", "Equipo", "application/vnd.google-apps.folder", "0"),
				gfile("g-doc", "Acta", "application/vnd.google-apps.document", "0"),
				gfile("g-bin", "logo.png", "image/png", "9"),
			}})
		case strings.Contains(r.URL.Query().Get("q"), "'g-dir' in parents"):
			writeJSON(w, map[string]any{"files": []any{
				gfile("g-hoja", "datos.csv", "text/csv", "3"),
			}})
		default:
			writeJSON(w, map[string]any{"files": []any{}})
		}
	})
	mux.HandleFunc("/drive/v3/files/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("bytes:" + strings.TrimPrefix(r.URL.Path, "/drive/v3/files/")))
	})
	mux.HandleFunc("/v1/people/me/connections", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"connections": []any{
			map[string]any{
				"resourceName":   "people/c1",
				"names":          []any{map[string]any{"displayName": "Luis Gil", "givenName": "Luis", "familyName": "Gil"}},
				"emailAddresses": []any{map[string]any{"value": "luis@old.example"}},
			},
		}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func gfile(id, name, mime, size string) map[string]any {
	return map[string]any{"id": id, "name": name, "mimeType": mime, "size": size, "modifiedTime": "2026-01-02T03:04:05Z"}
}

// serviceAccountJSON fabrica el JSON de una cuenta de servicio de Google con una
// clave RSA nueva y el token endpoint apuntando al httptest.
func serviceAccountJSON(t *testing.T, tokenURI string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generar clave: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal PKCS8: %v", err)
	}
	saJSON, err := json.Marshal(map[string]any{
		"type":         "service_account",
		"client_email": "svc@proj.iam.gserviceaccount.com",
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"token_uri":    tokenURI,
	})
	if err != nil {
		t.Fatalf("marshal SA: %v", err)
	}
	return string(saJSON)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// --- Instancias de conector ---------------------------------------------------

func m365Instance(base string) connector.Instance {
	return connector.Instance{
		ID: "inst-1", OrgID: "org-1", TemplateKey: "microsoft365",
		Config:  map[string]string{"tenant": "tenant-1", "client_id": "app-1", "graph_base": base, "authority_base": base},
		Secrets: map[string]string{"client_secret": "shhh"},
	}
}

func googleInstance(t *testing.T, base string) connector.Instance {
	t.Helper()
	return connector.Instance{
		ID: "inst-1", OrgID: "org-1", TemplateKey: "google-workspace",
		Config: map[string]string{
			"admin_email":        "admin@old.example",
			"google_drive_base":  base,
			"google_people_base": base,
		},
		Secrets: map[string]string{"service_account_key": serviceAccountJSON(t, base+"/token")},
	}
}

func job(id string, src domain.MigrationSource, dom domain.MigrationDomain) domain.MigrationJob {
	return domain.MigrationJob{
		ID: id, OrgID: "org-1", InstanceID: "inst-1",
		TargetUser: "grace@acme.test", SourceUser: sourceUserFor(src),
		Source: src, Domain: dom, Status: domain.MigrationQueued,
	}
}

func sourceUserFor(src domain.MigrationSource) string {
	if src == domain.SourceGoogle {
		return "alice@old.example"
	}
	return "alice@old.example"
}

// newRun prepara store + destino + origen para un trabajo.
func newRun(t *testing.T, inst connector.Instance, j domain.MigrationJob) (*memStore, *fakeSytue, migrate.Deps) {
	t.Helper()
	st := newMemStore("org-1")
	st.put(j)
	dst := newFakeSytue()
	src, err := migrate.NewSource(context.Background(), runtime(), inst, j)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	return st, dst, migrate.Deps{Src: src, Dst: dst, Store: st}
}

// --- Tests --------------------------------------------------------------------

// TestDriveJerarquiaM365 comprueba que se recrea la jerarquía anidada completa
// (raíz → Proyectos → 2026), siguiendo la paginación del origen, y que el
// contenido de cada fichero llega íntegro.
func TestDriveJerarquiaM365(t *testing.T) {
	allowLoopback(t)
	srv, _ := graphServer(t)
	j := job("job-1", domain.SourceM365, domain.DomainDrive)
	st, dst, deps := newRun(t, m365Instance(srv.URL), j)

	if err := migrate.Run(context.Background(), deps, j); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, err := st.get("job-1")
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Status != domain.MigrationDone {
		t.Fatalf("estado final = %q (%s)", got.Status, got.LastError)
	}
	// 2 carpetas + 3 ficheros migrados; nada omitido.
	if got.Counts.Migrated != 5 || got.Counts.Skipped != 0 || got.Counts.Errors != 0 {
		t.Fatalf("contadores = %+v; se esperaba 5 migrados y 0 omitidos/errores", got.Counts)
	}
	if len(dst.folders) != 3 { // raíz + Proyectos + 2026
		t.Fatalf("carpetas en destino = %v", dst.folders)
	}
	if len(dst.files) != 3 {
		t.Fatalf("ficheros en destino = %v", dst.files)
	}
	// La RUTA se preserva: plan.pdf cuelga de la carpeta 2026, que cuelga de Proyectos.
	if !hasPath(dst, "Proyectos", "2026", "plan.pdf") {
		t.Fatalf("la jerarquía anidada no se recreó: carpetas=%v ficheros=%v", dst.folders, dst.files)
	}
	// Y el CONTENIDO llega íntegro (no un fichero vacío).
	for id, path := range dst.files {
		if strings.HasSuffix(path, "plan.pdf") && dst.bodies[id] != "bytes:f-plan" {
			t.Fatalf("contenido de plan.pdf = %q", dst.bodies[id])
		}
	}
}

// hasPath verifica que existe /child dentro de /mid dentro de /parent.
func hasPath(f *fakeSytue, parent, mid, file string) bool {
	parentID := ""
	for id, p := range f.folders {
		if strings.HasSuffix(p, "/"+parent) {
			parentID = id
		}
	}
	midID := ""
	for id, p := range f.folders {
		if p == parentID+"/"+mid {
			midID = id
		}
	}
	if midID == "" {
		return false
	}
	for _, p := range f.files {
		if p == midID+"/"+file {
			return true
		}
	}
	return false
}

// TestIdempotencia: una SEGUNDA pasada del mismo trabajo no escribe nada nuevo.
func TestIdempotencia(t *testing.T) {
	allowLoopback(t)
	srv, _ := graphServer(t)
	j := job("job-1", domain.SourceM365, domain.DomainDrive)
	st, dst, deps := newRun(t, m365Instance(srv.URL), j)

	if err := migrate.Run(context.Background(), deps, j); err != nil {
		t.Fatalf("primera pasada: %v", err)
	}
	first := dst.writes
	if first == 0 {
		t.Fatal("la primera pasada no escribió nada")
	}

	// Segunda pasada con el trabajo tal y como quedó persistido (cursor incluido).
	again, err := st.get("job-1")
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	again.Cursor = "" // reejecutar desde cero: el MAPA es quien debe frenar la duplicación.
	if err := migrate.Run(context.Background(), deps, *again); err != nil {
		t.Fatalf("segunda pasada: %v", err)
	}
	if dst.writes != first {
		t.Fatalf("la segunda pasada escribió %d veces más (idempotencia rota)", dst.writes-first)
	}
	if len(dst.files) != 3 || len(dst.folders) != 3 {
		t.Fatalf("se duplicaron entidades: carpetas=%d ficheros=%d", len(dst.folders), len(dst.files))
	}
}

// TestReanudacion: el origen se cae al bajar a la subcarpeta. El trabajo queda a
// medias con un CURSOR; al reanudar continúa por él —sin repetir lo ya copiado— y
// termina la jerarquía.
func TestReanudacion(t *testing.T) {
	allowLoopback(t)
	srv, down := graphServer(t)
	j := job("job-1", domain.SourceM365, domain.DomainDrive)
	st, dst, deps := newRun(t, m365Instance(srv.URL), j)

	*down = true // el listado de "Proyectos" responde 502.
	if err := migrate.Run(context.Background(), deps, j); err == nil {
		t.Fatal("se esperaba que la primera pasada fallara al caerse el origen")
	}
	partial, err := st.get("job-1")
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if partial.Cursor == "" {
		t.Fatal("el trabajo interrumpido no dejó cursor de reanudación")
	}
	if partial.Status != domain.MigrationError {
		t.Fatalf("un trabajo a medias no puede quedar en %q", partial.Status)
	}
	// La raíz sí se copió: 1 carpeta creada + los 2 ficheros de la raíz.
	writesBefore, copiedBefore := dst.writes, len(dst.folders)+len(dst.files)
	if copiedBefore != 1+2+1 { // raíz preexistente + Proyectos + 2 ficheros
		t.Fatalf("la primera pasada copió %d entidades: %v %v", copiedBefore, dst.folders, dst.files)
	}
	// El cursor apunta a lo que FALTA (la subcarpeta), no a la raíz.
	if !strings.Contains(partial.Cursor, "d-proyectos") {
		t.Fatalf("el cursor no apunta a la carpeta pendiente: %s", partial.Cursor)
	}

	// Reanudar con el origen sano.
	*down = false
	partial.Status = domain.MigrationQueued
	if err := migrate.Run(context.Background(), deps, *partial); err != nil {
		t.Fatalf("reanudación: %v", err)
	}
	final, err := st.get("job-1")
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if final.Status != domain.MigrationDone {
		t.Fatalf("tras reanudar, estado = %q (%s)", final.Status, final.LastError)
	}
	if len(dst.folders) != 3 || len(dst.files) != 3 {
		t.Fatalf("la reanudación no completó: carpetas=%v ficheros=%v", dst.folders, dst.files)
	}
	// Continuó, no repitió: solo las 2 entidades que faltaban (2026 + plan.pdf).
	if got := dst.writes - writesBefore; got != 2 {
		t.Fatalf("la reanudación hizo %d escrituras; se esperaban 2 (lo pendiente)", got)
	}
}

// TestGoogleSinDocsOmiteNativosConMotivo: sin Sytue Docs configurado, un
// documento nativo NO se intenta en silencio ni cuenta como migrado — queda
// registrado como omitido, con el porqué. Es la degradación honesta.
func TestGoogleSinDocsOmiteNativosConMotivo(t *testing.T) {
	allowLoopback(t)
	srv := googleServer(t)
	j := job("job-g", domain.SourceGoogle, domain.DomainDrive)
	st, dst, deps := newRun(t, googleInstance(t, srv.URL), j) // deps.Docs == nil

	if err := migrate.Run(context.Background(), deps, j); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := st.get("job-g")
	if got.Counts.Skipped != 1 {
		t.Fatalf("omitidos = %d, se esperaba 1 (el Google Doc)", got.Counts.Skipped)
	}
	// 1 carpeta + 2 binarios copiados; el nativo NO cuenta como éxito.
	if got.Counts.Migrated != 3 {
		t.Fatalf("migrados = %d, se esperaba 3", got.Counts.Migrated)
	}
	if len(dst.files) != 2 {
		t.Fatalf("ficheros en destino = %v; el nativo no debía subirse", dst.files)
	}
	skipped := st.items[st.key("job-g")]["g-doc"]
	if skipped.SkipReason == "" {
		t.Fatal("el documento nativo se omitió SIN motivo registrado")
	}
	if !strings.Contains(skipped.SkipReason, "Sytue Docs") {
		t.Fatalf("el motivo no explica la causa: %q", skipped.SkipReason)
	}
	if skipped.SytueID != "" {
		t.Fatalf("una entidad omitida no puede tener id en Sytue: %+v", skipped)
	}
}

// fakeDocs es Sytue Docs: convierte, crea el documento y recibe la siembra del
// contenido. Guarda lo suficiente para comprobar que el documento llega ENTERO y
// del tipo correcto, que es lo que distingue «migrado» de «adjunto suelto».
type fakeDocs struct {
	srv       *httptest.Server
	created   map[string]string // id → tipo
	seeded    map[string]string // id → contenido sembrado
	seededKey map[string]string // id → registro donde se sembró
	uploaded  map[string]string // nombre de fichero recibido → contenido
}

func newFakeDocs(t *testing.T) *fakeDocs {
	t.Helper()
	f := &fakeDocs{
		created: map[string]string{}, seeded: map[string]string{},
		seededKey: map[string]string{}, uploaded: map[string]string{},
	}
	mux := http.NewServeMux()

	// El convertidor real deduce el tipo por la extensión: aquí igual.
	mux.HandleFunc("/api/v1/import", func(w http.ResponseWriter, r *http.Request) {
		file, header, err := r.FormFile("file")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer file.Close()
		raw, _ := io.ReadAll(file)
		f.uploaded[header.Filename] = string(raw)

		kind := "document"
		switch {
		case strings.HasSuffix(header.Filename, ".xlsx"):
			kind = "spreadsheet"
		case strings.HasSuffix(header.Filename, ".pptx"):
			kind = "presentation"
		}
		writeJSON(w, map[string]any{
			"kind": kind, "title": strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename)),
			"content": "<p>" + string(raw) + "</p>",
		})
	})
	mux.HandleFunc("/api/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Kind, Title string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		id := "doc-" + strconv.Itoa(len(f.created)+1)
		f.created[id] = body.Kind
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]any{"document": map[string]any{"id": id, "kind": body.Kind, "title": body.Title}})
	})
	mux.HandleFunc("/api/v1/documents/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/documents/"), "/ops")
		var body struct {
			Ops []struct {
				Kind, Key string
				Value     string
			} `json:"ops"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		for _, op := range body.Ops {
			f.seeded[id] = op.Value
			f.seededKey[id] = op.Key
		}
		writeJSON(w, map[string]any{"seq": 1})
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// TestGoogleDocsLleganEditablesASytueDocs: un Google Doc se EXPORTA y aterriza en
// Sytue Docs como documento editable —con su contenido sembrado en el registro
// que le toca—, mientras los binarios siguen yendo a Drive. Es la diferencia
// entre migrar un documento y dejar un .docx que nadie abre.
func TestGoogleDocsLleganEditablesASytueDocs(t *testing.T) {
	allowLoopback(t)
	srv := googleServer(t)
	docs := newFakeDocs(t)

	j := job("job-docs", domain.SourceGoogle, domain.DomainDrive)
	st, dst, deps := newRun(t, googleInstance(t, srv.URL), j)
	deps.Docs = migrate.NewSytueDocs(docs.srv.URL, migrate.StaticToken("tok-docs"), nil)

	if err := migrate.Run(context.Background(), deps, j); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := st.get("job-docs")
	if got.Status != domain.MigrationDone {
		t.Fatalf("estado final = %q (%s)", got.Status, got.LastError)
	}
	// Ya no se omite nada: 1 carpeta + 2 binarios + 1 documento nativo.
	if got.Counts.Skipped != 0 || got.Counts.Migrated != 4 {
		t.Fatalf("contadores = %+v; se esperaban 4 migrados y 0 omitidos", got.Counts)
	}
	if len(docs.created) != 1 {
		t.Fatalf("documentos creados en Sytue Docs = %v", docs.created)
	}
	var id string
	for k := range docs.created {
		id = k
	}
	if docs.created[id] != "document" {
		t.Errorf("tipo del documento = %q, quiero «document»", docs.created[id])
	}
	if docs.seededKey[id] != "doc:html" {
		t.Errorf("el contenido se sembró en %q, no en el registro del editor", docs.seededKey[id])
	}
	if docs.seeded[id] == "" {
		t.Error("el documento se creó VACÍO: se perdió el contenido por el camino")
	}
	// Se pidió la EXPORTACIÓN a .docx, no los bytes crudos (que no existen).
	if _, ok := docs.uploaded["Acta.docx"]; !ok {
		t.Errorf("Sytue Docs no recibió el documento exportado: %v", docs.uploaded)
	}
	// Y el nativo NO se subió a Drive como adjunto.
	for _, path := range dst.files {
		if strings.Contains(path, "Acta") {
			t.Errorf("el documento nativo acabó como fichero en Drive: %s", path)
		}
	}
	// El registro del informe lo marca como documento, no como fichero.
	item := st.items[st.key("job-docs")]["g-doc"]
	if item.Kind != domain.KindDoc || item.SytueID == "" {
		t.Errorf("el informe no registra el documento creado: %+v", item)
	}
}

// TestContactsM365 importa la agenda y omite —con motivo— el contacto sin nombre.
func TestContactsM365(t *testing.T) {
	allowLoopback(t)
	srv, _ := graphServer(t)
	j := job("job-c", domain.SourceM365, domain.DomainContacts)
	st, dst, deps := newRun(t, m365Instance(srv.URL), j)

	if err := migrate.Run(context.Background(), deps, j); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := st.get("job-c")
	if got.Counts.Migrated != 1 || got.Counts.Skipped != 1 {
		t.Fatalf("contadores = %+v; se esperaba 1 migrado y 1 omitido", got.Counts)
	}
	if len(dst.contacts) != 1 {
		t.Fatalf("contactos en destino = %v", dst.contacts)
	}
	if st.items[st.key("job-c")]["c-2"].SkipReason == "" {
		t.Fatal("el contacto sin nombre se omitió sin motivo")
	}
	// Segunda pasada: idempotente también en contactos.
	writes := dst.writes
	again, _ := st.get("job-c")
	again.Cursor = ""
	if err := migrate.Run(context.Background(), deps, *again); err != nil {
		t.Fatalf("segunda pasada: %v", err)
	}
	if dst.writes != writes {
		t.Fatalf("la segunda pasada de contactos duplicó (%d escrituras nuevas)", dst.writes-writes)
	}
}

// TestContactsGoogle comprueba el otro origen de agenda (People API).
func TestContactsGoogle(t *testing.T) {
	allowLoopback(t)
	srv := googleServer(t)
	j := job("job-gc", domain.SourceGoogle, domain.DomainContacts)
	st, dst, deps := newRun(t, googleInstance(t, srv.URL), j)

	if err := migrate.Run(context.Background(), deps, j); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := st.get("job-gc")
	if got.Counts.Migrated != 1 {
		t.Fatalf("migrados = %d, se esperaba 1", got.Counts.Migrated)
	}
	if len(dst.contacts) != 1 {
		t.Fatalf("contactos en destino = %v", dst.contacts)
	}
}

// TestAislamientoPorOrg: el runner solo ve los trabajos de SU organización. Es el
// espejo en memoria de lo que garantiza RLS (verificado contra PostgreSQL real en
// migration_integration_test.go).
func TestAislamientoPorOrg(t *testing.T) {
	allowLoopback(t)
	srv, _ := graphServer(t)
	j := job("job-1", domain.SourceM365, domain.DomainDrive)

	stA := newMemStore("org-a")
	stA.put(j)
	stB := newMemStore("org-b") // misma id de trabajo, otra org: invisible.

	src, err := migrate.NewSource(context.Background(), runtime(), m365Instance(srv.URL), j)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	dst := newFakeSytue()
	err = migrate.Run(context.Background(), migrate.Deps{Src: src, Dst: dst, Store: stB}, j)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("otra org pudo ejecutar el trabajo (err=%v)", err)
	}
	if dst.writes != 0 {
		t.Fatalf("otra org escribió %d veces en el destino", dst.writes)
	}
}

// TestOrigenPrivadoBloqueado: sin relajar la barrera SSRF, el egress al origen se
// corta antes de conectar (el destino es loopback).
func TestOrigenPrivadoBloqueado(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"access_token": "tok", "expires_in": 3600})
	}))
	t.Cleanup(srv.Close)

	j := job("job-1", domain.SourceM365, domain.DomainDrive)
	_, err := migrate.NewSource(context.Background(), runtime(), m365Instance(srv.URL), j)
	if !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Fatalf("se esperaba ssrfhttp.ErrBlocked al salir a un host privado, se obtuvo: %v", err)
	}
}

// TestDelegationSetupURL: el atajo a la delegación lleva EXACTAMENTE los scopes que
// la migración pide después. Es la única comprobación que atrapa el fallo caro de
// esta pantalla: se autoriza una lista corta, todo parece bien, y el trabajo del
// correo muere con `unauthorized_client` horas más tarde, con el administrador ya
// fuera de la consola de Google.
func TestDelegationSetupURL(t *testing.T) {
	u := migrate.DelegationSetupURL("112233445566", false)
	if !strings.HasPrefix(u, "https://admin.google.com/ac/owl/domainwidedelegation?") {
		t.Fatalf("la URL no lleva a la pantalla de delegación: %s", u)
	}
	q, err := url.Parse(u)
	if err != nil {
		t.Fatalf("la URL no es válida: %v", err)
	}
	if got := q.Query().Get("clientIdToAdd"); got != "112233445566" {
		t.Errorf("clientIdToAdd = %q, quiero el client_id numérico de la cuenta de servicio", got)
	}
	scopes := strings.Split(q.Query().Get("clientScopes"), ",")
	// Uno por dominio migrable (migrate.go elige el scope según el dominio) más los
	// dos del Directory, que el driver pide SIEMPRE juntos en el mismo JWT.
	quiero := []string{
		"https://www.googleapis.com/auth/drive.readonly",
		"https://www.googleapis.com/auth/contacts.readonly",
		"https://www.googleapis.com/auth/gmail.readonly",
		"https://www.googleapis.com/auth/calendar.readonly",
		"https://www.googleapis.com/auth/admin.directory.user.readonly",
		"https://www.googleapis.com/auth/admin.directory.group.readonly",
	}
	for _, s := range quiero {
		if !slices.Contains(scopes, s) {
			t.Errorf("falta el scope %q en el enlace de delegación: %v", s, scopes)
		}
	}
	// Sin proyecto de Google Cloud no se pide permiso sobre la plataforma entera:
	// pedir de más es lo que hace que un administrador cierre la pantalla.
	if slices.Contains(scopes, "https://www.googleapis.com/auth/cloud-platform.read-only") {
		t.Error("sin proyecto de GCP el enlace no debe pedir el scope de Google Cloud")
	}
	conNube := strings.Split(
		mustQuery(t, migrate.DelegationSetupURL("112233445566", true)).Get("clientScopes"), ",",
	)
	if !slices.Contains(conNube, "https://www.googleapis.com/auth/cloud-platform.read-only") {
		t.Errorf("con proyecto de GCP falta el scope de Google Cloud: %v", conNube)
	}
	// Sin cuenta de servicio subida, el enlace sigue valiendo: lleva los permisos
	// aunque el cliente lo teclee a mano quien esté delante.
	sinCliente := migrate.DelegationSetupURL("", false)
	if strings.Contains(sinCliente, "clientIdToAdd") {
		t.Errorf("sin client_id no debe aparecer clientIdToAdd: %s", sinCliente)
	}
	if !strings.Contains(sinCliente, "clientScopes=") {
		t.Errorf("sin client_id el enlace debe seguir llevando los permisos: %s", sinCliente)
	}
}

// mustQuery parsea una URL del test y devuelve sus parámetros.
func mustQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("la URL no es válida: %v", err)
	}
	return u.Query()
}
