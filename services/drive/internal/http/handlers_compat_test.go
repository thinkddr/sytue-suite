// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests de la capa de compatibilidad de ficheros (Google Drive v3 y Microsoft
// Graph) con el fake del servicio: sin PostgreSQL ni S3. Cubren listado con
// paginación en los dos formatos, `q`, descarga, subida, los 501 honestos y el
// AISLAMIENTO (un usuario no ve por la vía compat lo que no vería por la nativa).
package drivehttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/drive/internal/config"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// --- fixtures y helpers ---------------------------------------------------

var compatTime = time.Date(2026, 7, 27, 8, 30, 0, 0, time.UTC)

func compatFixtures() (root, sub domain.Folder, file domain.File) {
	rootID := uuid.New()
	root = domain.Folder{ID: rootID, Name: "Mi unidad", IsRoot: true, CreatedAt: compatTime, UpdatedAt: compatTime}
	sub = domain.Folder{ID: uuid.New(), ParentID: &rootID, Name: "Docs", CreatedAt: compatTime, UpdatedAt: compatTime}
	file = domain.File{
		ID: uuid.New(), FolderID: rootID, Name: "nota.txt", SizeBytes: 5,
		ContentType: "text/plain", Checksum: "cafe", CreatedAt: compatTime, UpdatedAt: compatTime,
	}
	return root, sub, file
}

// compatRouter monta los dos dialectos con un middleware de auth de mentira que
// inyecta los claims del tenant dado (en producción es d.Verifier.RequireAuth).
func compatRouter(svc driveService, tenant string) http.Handler {
	r := chi.NewRouter()
	auth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			claims := authn.Claims{Subject: "user@" + tenant, Tenant: tenant, Name: "Ada Lovelace", Email: "ada@" + tenant + ".test"}
			next.ServeHTTP(w, req.WithContext(authn.Into(req.Context(), claims)))
		})
	}
	mountCompat(r, &compatHandlers{svc: svc, cfg: config.Config{PublicURL: "https://drive.sytue.eu", MaxUploadBytes: 1024}}, auth, auth)
	return r
}

// do lanza una petición contra el router de compat y devuelve la respuesta.
func do(t *testing.T, h http.Handler, method, target, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, target, http.NoBody)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// decode decodifica el cuerpo JSON en un mapa genérico.
func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("JSON inválido: %v (%s)", err, rec.Body.String())
	}
	return out
}

// names extrae los nombres de un listado de Google (files) o de Graph (value).
func names(t *testing.T, rec *httptest.ResponseRecorder, key string) []string {
	t.Helper()
	body := decode(t, rec)
	list, _ := body[key].([]any)
	out := make([]string, 0, len(list))
	for _, it := range list {
		m, _ := it.(map[string]any)
		out = append(out, m["name"].(string))
	}
	return out
}

// --- Google Drive v3 ------------------------------------------------------

func TestCompatGoogleAbout(t *testing.T) {
	h := compatRouter(&fakeSvc{quota: domain.Quota{Tenant: "acme", LimitBytes: 1000, UsedBytes: 400}}, "acme")
	rec := do(t, h, http.MethodGet, "/compat/google/drive/v3/about?fields=user,storageQuota", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	if body["kind"] != "drive#about" {
		t.Fatalf("kind = %v", body["kind"])
	}
	user := body["user"].(map[string]any)
	if user["emailAddress"] != "ada@acme.test" || user["me"] != true {
		t.Fatalf("user = %+v", user)
	}
	quota := body["storageQuota"].(map[string]any)
	if quota["limit"] != "1000" || quota["usage"] != "400" {
		t.Fatalf("storageQuota = %+v", quota)
	}
}

// TestCompatGoogleAboutSinCuota: si el servicio no sabe la cuota, el campo se
// OMITE (no se inventa) y el about sigue respondiendo 200.
func TestCompatGoogleAboutSinCuota(t *testing.T) {
	h := compatRouter(&fakeSvc{quotaErr: domain.ErrNotFound}, "acme")
	rec := do(t, h, http.MethodGet, "/compat/google/drive/v3/about", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	if _, ok := decode(t, rec)["storageQuota"]; ok {
		t.Fatalf("storageQuota no debería estar: %s", rec.Body.String())
	}
}

func TestCompatGoogleListByParent(t *testing.T) {
	root, sub, file := compatFixtures()
	svc := &fakeSvc{
		root:      root,
		childFold: []domain.Folder{sub},
		page:      service.FilePage{Files: []domain.File{file}, NextCursor: "cur-2"},
	}
	h := compatRouter(svc, "acme")

	rec := do(t, h, http.MethodGet, "/compat/google/drive/v3/files?q="+urlq("'root' in parents and trashed = false")+"&pageSize=100", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	if body["kind"] != "drive#fileList" || body["nextPageToken"] != "cur-2" {
		t.Fatalf("sobre = %+v", body)
	}
	files := body["files"].([]any)
	if len(files) != 2 {
		t.Fatalf("quiero carpeta + fichero, tengo %d", len(files))
	}
	folder := files[0].(map[string]any)
	if folder["mimeType"] != "application/vnd.google-apps.folder" || folder["name"] != "Docs" {
		t.Fatalf("carpeta = %+v", folder)
	}
	f := files[1].(map[string]any)
	if f["mimeType"] != "text/plain" || f["size"] != "5" || f["modifiedTime"] != "2026-07-27T08:30:00Z" {
		t.Fatalf("fichero = %+v", f)
	}
	if parents := f["parents"].([]any); parents[0] != root.ID.String() {
		t.Fatalf("parents = %v", parents)
	}
}

// TestCompatGoogleListSegundaPagina: con pageToken las subcarpetas ya no viajan
// (igual que en el listado nativo) y el token se pasa al servicio tal cual.
func TestCompatGoogleListSegundaPagina(t *testing.T) {
	root, sub, file := compatFixtures()
	svc := &fakeSvc{root: root, childFold: []domain.Folder{sub}, page: service.FilePage{Files: []domain.File{file}}}
	h := compatRouter(svc, "acme")
	rec := do(t, h, http.MethodGet, "/compat/google/drive/v3/files?q="+urlq("'root' in parents")+"&pageToken=cur-2", "", "")
	if got := names(t, rec, "files"); len(got) != 1 || got[0] != "nota.txt" {
		t.Fatalf("segunda página = %v", got)
	}
	if _, ok := decode(t, rec)["nextPageToken"]; ok {
		t.Fatalf("no debería haber nextPageToken: %s", rec.Body.String())
	}
}

func TestCompatGoogleListFiltros(t *testing.T) {
	root, sub, file := compatFixtures()
	svc := &fakeSvc{root: root, childFold: []domain.Folder{sub}, page: service.FilePage{Files: []domain.File{file}}}
	h := compatRouter(svc, "acme")

	for _, tc := range []struct {
		name, q string
		want    []string
	}{
		{name: "nombre exacto", q: "'root' in parents and name = 'nota.txt'", want: []string{"nota.txt"}},
		{name: "nombre sin coincidencia", q: "'root' in parents and name = 'otro.txt'", want: []string{}},
		{name: "solo carpetas", q: "'root' in parents and mimeType = 'application/vnd.google-apps.folder'", want: []string{"Docs"}},
		{name: "todo menos carpetas", q: "'root' in parents and mimeType != 'application/vnd.google-apps.folder'", want: []string{"nota.txt"}},
		{name: "tipo concreto", q: "'root' in parents and mimeType = 'text/plain'", want: []string{"nota.txt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, h, http.MethodGet, "/compat/google/drive/v3/files?q="+urlq(tc.q), "", "")
			if rec.Code != http.StatusOK {
				t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
			}
			got := names(t, rec, "files")
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("q=%q ⇒ %v, quiero %v", tc.q, got, tc.want)
			}
		})
	}
}

// TestCompatGoogleListRecientes: sin `in parents` el listado es transversal (la
// vista «Recientes» del servicio, con su propio cursor keyset).
func TestCompatGoogleListRecientes(t *testing.T) {
	svc := &fakeSvc{recent: service.RecentPage{
		Entries: []service.Entry{
			{ID: uuid.New(), Name: "nota.txt", SizeBytes: 5, ContentType: "text/plain", UpdatedAt: compatTime},
			{ID: uuid.New(), Name: "Docs", IsDir: true, UpdatedAt: compatTime},
		},
		NextCursor: "rec-2",
	}}
	h := compatRouter(svc, "acme")
	rec := do(t, h, http.MethodGet, "/compat/google/drive/v3/files", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if got := names(t, rec, "files"); len(got) != 2 {
		t.Fatalf("recientes = %v", got)
	}
	if decode(t, rec)["nextPageToken"] != "rec-2" {
		t.Fatalf("falta el token de continuación: %s", rec.Body.String())
	}
}

func TestCompatGoogleListRechazos(t *testing.T) {
	h := compatRouter(&fakeSvc{}, "acme")
	for _, tc := range []struct{ name, q string }{
		{name: "filtro no soportado", q: "name contains 'informe'"},
		{name: "papelera", q: "trashed = true"},
		{name: "carpetas sin padre", q: "mimeType = 'application/vnd.google-apps.folder'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, h, http.MethodGet, "/compat/google/drive/v3/files?q="+urlq(tc.q), "", "")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("código = %d, quiero 400 (%s)", rec.Code, rec.Body.String())
			}
			if reason := googleReason(t, rec); reason != "badRequest" {
				t.Fatalf("reason = %q", reason)
			}
		})
	}
}

func TestCompatGoogleGetYDescarga(t *testing.T) {
	_, _, file := compatFixtures()
	h := compatRouter(&fakeSvc{file: file, openFile: file, openBody: "hola!"}, "acme")

	rec := do(t, h, http.MethodGet, "/compat/google/drive/v3/files/"+file.ID.String(), "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	if body := decode(t, rec); body["id"] != file.ID.String() || body["mimeType"] != "text/plain" {
		t.Fatalf("metadatos = %+v", body)
	}

	rec = do(t, h, http.MethodGet, "/compat/google/drive/v3/files/"+file.ID.String()+"?alt=media", "", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "hola!" {
		t.Fatalf("descarga = %d %q", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("falta nosniff")
	}
}

// TestCompatGoogleGetCarpeta: si el id no es un fichero se prueba como carpeta
// (espacios de id distintos en Drive).
func TestCompatGoogleGetCarpeta(t *testing.T) {
	_, sub, _ := compatFixtures()
	h := compatRouter(&fakeSvc{fileErr: domain.ErrNotFound, folder: sub}, "acme")
	rec := do(t, h, http.MethodGet, "/compat/google/drive/v3/files/"+sub.ID.String(), "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if body := decode(t, rec); body["mimeType"] != "application/vnd.google-apps.folder" {
		t.Fatalf("carpeta = %+v", body)
	}
}

func TestCompatGoogleDelete(t *testing.T) {
	_, _, file := compatFixtures()
	h := compatRouter(&fakeSvc{file: file}, "acme")
	rec := do(t, h, http.MethodDelete, "/compat/google/drive/v3/files/"+file.ID.String(), "", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestCompatGoogleUploadMedia(t *testing.T) {
	root, _, file := compatFixtures()
	svc := &fakeSvc{root: root, uploaded: file}
	h := compatRouter(svc, "acme")

	rec := do(t, h, http.MethodPost, "/compat/google/upload/drive/v3/files?uploadType=media&name=nota.txt", "text/plain", "hola!")
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if svc.lastUploadName != "nota.txt" {
		t.Fatalf("nombre subido = %q", svc.lastUploadName)
	}
	if body := decode(t, rec); body["id"] != file.ID.String() {
		t.Fatalf("respuesta = %+v", body)
	}

	// Sin nombre no se puede crear el fichero: 400 explícito.
	rec = do(t, h, http.MethodPost, "/compat/google/upload/drive/v3/files?uploadType=media", "text/plain", "hola!")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sin nombre ⇒ %d, quiero 400", rec.Code)
	}
}

func TestCompatGoogleUploadMultipart(t *testing.T) {
	root, _, file := compatFixtures()
	svc := &fakeSvc{root: root, uploaded: file}
	h := compatRouter(svc, "acme")

	body := "--B\r\nContent-Type: application/json; charset=UTF-8\r\n\r\n" +
		`{"name":"informe.pdf","mimeType":"application/pdf","parents":["` + root.ID.String() + `"]}` + "\r\n" +
		"--B\r\nContent-Type: application/pdf\r\n\r\n%PDF-1.4\r\n--B--\r\n"

	rec := do(t, h, http.MethodPost, "/compat/google/upload/drive/v3/files?uploadType=multipart", "multipart/related; boundary=B", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if svc.lastUploadName != "informe.pdf" {
		t.Fatalf("nombre subido = %q", svc.lastUploadName)
	}

	// Cuerpo multipart sin metadatos ⇒ 400 claro.
	rec = do(t, h, http.MethodPost, "/compat/google/upload/drive/v3/files?uploadType=multipart", "application/json", `{"name":"x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("multipart inválido ⇒ %d, quiero 400", rec.Code)
	}
}

// files.create sin contenido crea carpetas (o un fichero vacío), files.update
// renombra y mueve con addParents, y files.copy copia junto al original o a parents.
func TestCompatGoogleEscrituraDeMetadatos(t *testing.T) {
	root, sub, file := compatFixtures()
	f := &fakeSvc{
		root: root, created: sub, file: file, uploaded: file,
		renFile:  domain.File{ID: file.ID, FolderID: root.ID, Name: "renombrado.txt"},
		movFile:  domain.File{ID: file.ID, FolderID: sub.ID, Name: "nota.txt"},
		copyFile: domain.File{ID: uuid.New(), FolderID: root.ID, Name: "nota.txt"},
	}
	h := compatRouter(f, "acme")

	rec := do(t, h, http.MethodPost, "/compat/google/drive/v3/files", "application/json",
		`{"name":"Docs","mimeType":"application/vnd.google-apps.folder","parents":["root"]}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"mimeType":"application/vnd.google-apps.folder"`) {
		t.Fatalf("crear carpeta: %d (%s)", rec.Code, rec.Body.String())
	}
	rec = do(t, h, http.MethodPost, "/compat/google/drive/v3/files", "application/json", `{"name":"vacio.txt","mimeType":"text/plain"}`)
	if rec.Code != http.StatusOK || f.lastUploadName != "vacio.txt" {
		t.Fatalf("crear fichero vacío: %d (%s)", rec.Code, rec.Body.String())
	}

	rec = do(t, h, http.MethodPatch, "/compat/google/drive/v3/files/"+file.ID.String(), "application/json", `{"name":"renombrado.txt"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "renombrado.txt") {
		t.Fatalf("renombrar: %d (%s)", rec.Code, rec.Body.String())
	}
	rec = do(t, h, http.MethodPatch, "/compat/google/drive/v3/files/"+file.ID.String()+"?addParents="+sub.ID.String()+"&removeParents="+root.ID.String(), "", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), sub.ID.String()) {
		t.Fatalf("mover: %d (%s)", rec.Code, rec.Body.String())
	}

	rec = do(t, h, http.MethodPost, "/compat/google/drive/v3/files/"+file.ID.String()+"/copy", "application/json", `{}`)
	if rec.Code != http.StatusOK || f.lastCopyID != file.ID || f.lastCopyDs != root.ID || f.lastCopyPr != domain.PermWrite {
		t.Fatalf("copiar: %d, src=%s dest=%s", rec.Code, f.lastCopyID, f.lastCopyDs)
	}

	for nombre, c := range map[string]struct{ method, target, body string }{
		"crear sin nombre":        {http.MethodPost, "/compat/google/drive/v3/files", `{"mimeType":"application/vnd.google-apps.folder"}`},
		"campo que no se escribe": {http.MethodPost, "/compat/google/drive/v3/files", `{"name":"x","starred":true}`},
		"dos padres":              {http.MethodPatch, "/compat/google/drive/v3/files/" + file.ID.String() + "?addParents=a,b", ``},
		"removeParents que no es": {http.MethodPatch, "/compat/google/drive/v3/files/" + file.ID.String() + "?addParents=" + sub.ID.String() + "&removeParents=" + uuid.NewString(), ``},
	} {
		ct := ""
		if c.body != "" {
			ct = "application/json"
		}
		if rec := do(t, h, c.method, c.target, ct, c.body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d (%s)", nombre, rec.Code, rec.Body.String())
		}
	}
}

func TestCompatGoogleNoImplementado(t *testing.T) {
	h := compatRouter(&fakeSvc{}, "acme")
	for _, target := range []string{
		"/compat/google/drive/v3/files/x/permissions",
		"/compat/google/drive/v3/changes",
		"/compat/google/upload/drive/v3/files?uploadType=resumable",
	} {
		rec := do(t, h, http.MethodPost, target, "", "")
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("%s ⇒ %d, quiero 501 (%s)", target, rec.Code, rec.Body.String())
		}
		if reason := googleReason(t, rec); reason != "notImplemented" {
			t.Fatalf("%s ⇒ reason %q", target, reason)
		}
	}
}

// TestCompatRutasEnElRouterReal comprueba que el montaje de producción registra
// ambos dialectos sin colisionar con las rutas nativas (chi entra en pánico al
// registrar patrones incompatibles: mejor descubrirlo aquí que en el arranque).
func TestCompatRutasEnElRouterReal(t *testing.T) {
	r, ok := NewRouter(Deps{Cfg: config.Config{PublicURL: "https://drive.sytue.eu"}}).(chi.Routes)
	if !ok {
		t.Fatal("NewRouter no devuelve un router de chi")
	}
	registradas := map[string]bool{}
	if err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		registradas[method+" "+route] = true
		return nil
	}); err != nil {
		t.Fatalf("walk: %v", err)
	}
	for _, want := range []string{
		"GET /compat/google/drive/v3/about",
		"GET /compat/google/drive/v3/files",
		"GET /compat/google/drive/v3/files/{id}",
		"DELETE /compat/google/drive/v3/files/{id}",
		"POST /compat/google/upload/drive/v3/files",
		"POST /compat/google/drive/v3/files",
		"PATCH /compat/google/drive/v3/files/{id}",
		"POST /compat/google/drive/v3/files/{id}/copy",
		"GET /compat/graph/v1.0/me/drive",
		"GET /compat/graph/v1.0/me/drive/root/children",
		"GET /compat/graph/v1.0/me/drive/items/{id}",
		"GET /compat/graph/v1.0/me/drive/items/{id}/children",
		"GET /compat/graph/v1.0/me/drive/items/{id}/content",
		"PUT /compat/graph/v1.0/me/drive/items/{id}/content",
		"GET /api/v1/files/{id}", // la API nativa sigue intacta
	} {
		if !registradas[want] {
			t.Fatalf("falta la ruta %q", want)
		}
	}
}

// --- Microsoft Graph ------------------------------------------------------

func TestCompatGraphDrive(t *testing.T) {
	root, _, _ := compatFixtures()
	h := compatRouter(&fakeSvc{root: root, quota: domain.Quota{LimitBytes: 1000, UsedBytes: 400}}, "acme")
	rec := do(t, h, http.MethodGet, "/compat/graph/v1.0/me/drive", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	if body["id"] != root.ID.String() || body["driveType"] != "business" {
		t.Fatalf("drive = %+v", body)
	}
	quota := body["quota"].(map[string]any)
	if quota["total"].(float64) != 1000 || quota["remaining"].(float64) != 600 {
		t.Fatalf("quota = %+v", quota)
	}
	if _, ok := body["root"].(map[string]any)["folder"]; !ok {
		t.Fatalf("la raíz debe llevar faceta folder: %s", rec.Body.String())
	}
}

func TestCompatGraphChildren(t *testing.T) {
	root, sub, file := compatFixtures()
	svc := &fakeSvc{
		root:      root,
		childFold: []domain.Folder{sub},
		page:      service.FilePage{Files: []domain.File{file}, NextCursor: "cur-2"},
	}
	h := compatRouter(svc, "acme")
	rec := do(t, h, http.MethodGet, "/compat/graph/v1.0/me/drive/root/children?$top=50", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	items := body["value"].([]any)
	if len(items) != 2 {
		t.Fatalf("hijos = %d", len(items))
	}
	if _, ok := items[0].(map[string]any)["folder"]; !ok {
		t.Fatalf("el primer hijo debe ser carpeta: %s", rec.Body.String())
	}
	f := items[1].(map[string]any)
	if f["size"].(float64) != 5 || f["lastModifiedDateTime"] != "2026-07-27T08:30:00Z" {
		t.Fatalf("fichero = %+v", f)
	}
	if f["file"].(map[string]any)["mimeType"] != "text/plain" {
		t.Fatalf("faceta file = %+v", f["file"])
	}
	next, _ := body["@odata.nextLink"].(string)
	if !strings.HasPrefix(next, "https://drive.sytue.eu/compat/graph/v1.0/me/drive/root/children?") ||
		!strings.Contains(next, "skiptoken=cur-2") || !strings.Contains(next, "top=50") {
		t.Fatalf("@odata.nextLink = %q", next)
	}
}

func TestCompatGraphItemYContenido(t *testing.T) {
	_, _, file := compatFixtures()
	h := compatRouter(&fakeSvc{file: file, openFile: file, openBody: "hola!"}, "acme")

	rec := do(t, h, http.MethodGet, "/compat/graph/v1.0/me/drive/items/"+file.ID.String(), "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	if body := decode(t, rec); body["id"] != file.ID.String() || body["file"] == nil {
		t.Fatalf("driveItem = %+v", body)
	}

	rec = do(t, h, http.MethodGet, "/compat/graph/v1.0/me/drive/items/"+file.ID.String()+"/content", "", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "hola!" {
		t.Fatalf("contenido = %d %q", rec.Code, rec.Body.String())
	}
}

func TestCompatGraphPutContent(t *testing.T) {
	_, _, file := compatFixtures()
	svc := &fakeSvc{file: file, uploaded: file}
	h := compatRouter(svc, "acme")
	rec := do(t, h, http.MethodPut, "/compat/graph/v1.0/me/drive/items/"+file.ID.String()+"/content", "text/plain", "adios")
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if svc.lastUploadName != "nota.txt" {
		t.Fatalf("sustituye el MISMO fichero: %q", svc.lastUploadName)
	}
	if body := decode(t, rec); body["id"] != file.ID.String() {
		t.Fatalf("driveItem = %+v", body)
	}
}

func TestCompatGraphNoImplementado(t *testing.T) {
	h := compatRouter(&fakeSvc{}, "acme")
	for _, target := range []string{
		"/compat/graph/v1.0/me/drive/root/delta",
		"/compat/graph/v1.0/me/drive/root:/carpeta/fichero.txt:/content",
		"/compat/graph/v1.0/sites/root/drives",
	} {
		rec := do(t, h, http.MethodGet, target, "", "")
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("%s ⇒ %d, quiero 501 (%s)", target, rec.Code, rec.Body.String())
		}
		if code := decode(t, rec)["error"].(map[string]any)["code"]; code != "notImplemented" {
			t.Fatalf("%s ⇒ code %v", target, code)
		}
	}
}

// --- Aislamiento ----------------------------------------------------------

// isolatedSvc reproduce el aislamiento REAL del servicio (RLS por tenant): solo
// el tenant dueño ve el contenido; para cualquier otro simplemente no existe.
type isolatedSvc struct {
	*fakeSvc
	owner     string
	ownerRoot domain.Folder
	otherRoot domain.Folder
}

func (s *isolatedSvc) owns(ctx context.Context) bool {
	c, _ := authn.FromContext(ctx)
	return c.Tenant == s.owner
}

func (s *isolatedSvc) CopyChannelFileToMyDrive(ctx context.Context, _, _ uuid.UUID, _ string) (domain.File, error) {
	if !s.owns(ctx) {
		return domain.File{}, domain.ErrNotFound
	}
	return domain.File{ID: uuid.New()}, nil
}

func (s *isolatedSvc) Root(ctx context.Context) (domain.Folder, error) {
	if s.owns(ctx) {
		return s.ownerRoot, nil
	}
	return s.otherRoot, nil
}

func (s *isolatedSvc) GetFile(ctx context.Context, id uuid.UUID) (domain.File, error) {
	if !s.owns(ctx) {
		return domain.File{}, domain.ErrNotFound
	}
	return s.fakeSvc.GetFile(ctx, id)
}

func (s *isolatedSvc) GetFolder(ctx context.Context, id uuid.UUID) (domain.Folder, error) {
	if !s.owns(ctx) {
		return domain.Folder{}, domain.ErrNotFound
	}
	return s.fakeSvc.GetFolder(ctx, id)
}

func (s *isolatedSvc) ListSubfolders(ctx context.Context, id uuid.UUID) ([]domain.Folder, error) {
	if !s.owns(ctx) {
		return nil, nil
	}
	return s.fakeSvc.ListSubfolders(ctx, id)
}

func (s *isolatedSvc) ListFilesPage(ctx context.Context, id uuid.UUID, cursor string, limit int) (service.FilePage, error) {
	if !s.owns(ctx) {
		return service.FilePage{}, nil
	}
	return s.fakeSvc.ListFilesPage(ctx, id, cursor, limit)
}

func (s *isolatedSvc) Recent(ctx context.Context, cursor string, limit int) (service.RecentPage, error) {
	if !s.owns(ctx) {
		return service.RecentPage{}, nil
	}
	return s.fakeSvc.Recent(ctx, cursor, limit)
}

func (*isolatedSvc) ExternalStorageEnabled(context.Context) bool { return false }
func (*isolatedSvc) ForgetExternalStorage(string)                {}
func (*isolatedSvc) ExternalInstances(context.Context) (service.ExternalAccounts, error) {
	return service.ExternalAccounts{}, nil
}

func (*isolatedSvc) ProbeExternal(context.Context, store.ExternalStorage, string) string { return "" }

func (*isolatedSvc) ConnectNAS(context.Context, service.NASRequest, string) (service.NASStatus, error) {
	return service.NASStatus{}, nil
}

func (*isolatedSvc) NASStatus(context.Context) (service.NASStatus, error) {
	return service.NASStatus{}, nil
}
func (*isolatedSvc) TeardownNAS(context.Context, store.ExternalStorage) error { return nil }

var _ driveService = (*isolatedSvc)(nil)

// TestCompatAislamiento: el usuario de otro tenant NO ve por la vía compat lo que
// no vería por la nativa — ni listando (Google y Graph) ni pidiendo el id directo,
// ni descargando el contenido. La verja es la del servicio: los handlers de compat
// no consultan el store en crudo.
func TestCompatAislamiento(t *testing.T) {
	rootA, subA, fileA := compatFixtures()
	base := &fakeSvc{
		root:      rootA,
		childFold: []domain.Folder{subA},
		page:      service.FilePage{Files: []domain.File{fileA}},
		file:      fileA,
		folder:    subA,
		openFile:  fileA,
		openBody:  "secreto de A",
		recent: service.RecentPage{Entries: []service.Entry{
			{ID: fileA.ID, Name: fileA.Name, SizeBytes: fileA.SizeBytes, ContentType: fileA.ContentType, UpdatedAt: compatTime},
		}},
	}
	svc := &isolatedSvc{
		fakeSvc:   base,
		owner:     "acme",
		ownerRoot: rootA,
		otherRoot: domain.Folder{ID: uuid.New(), Name: "Mi unidad", IsRoot: true},
	}

	// El dueño sí lo ve (control: la ruta funciona).
	owner := compatRouter(svc, "acme")
	if got := names(t, do(t, owner, http.MethodGet, "/compat/google/drive/v3/files?q="+urlq("'root' in parents"), "", ""), "files"); len(got) != 2 {
		t.Fatalf("el dueño debe ver su contenido: %v", got)
	}

	intruder := compatRouter(svc, "evilcorp")
	listados := []struct {
		name, target, key string
	}{
		{name: "google listado por padre", target: "/compat/google/drive/v3/files?q=" + urlq("'root' in parents"), key: "files"},
		{name: "google listado transversal", target: "/compat/google/drive/v3/files", key: "files"},
		{name: "graph hijos de la raíz", target: "/compat/graph/v1.0/me/drive/root/children", key: "value"},
	}
	for _, tc := range listados {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, intruder, http.MethodGet, tc.target, "", "")
			if rec.Code != http.StatusOK {
				t.Fatalf("código = %d", rec.Code)
			}
			if got := names(t, rec, tc.key); len(got) != 0 {
				t.Fatalf("el intruso no debe ver nada: %v", got)
			}
			if strings.Contains(rec.Body.String(), fileA.Name) || strings.Contains(rec.Body.String(), fileA.ID.String()) {
				t.Fatalf("fuga en el listado: %s", rec.Body.String())
			}
		})
	}

	directos := []struct{ name, target string }{
		{name: "google por id", target: "/compat/google/drive/v3/files/" + fileA.ID.String()},
		{name: "google descarga", target: "/compat/google/drive/v3/files/" + fileA.ID.String() + "?alt=media"},
		{name: "google carpeta por id", target: "/compat/google/drive/v3/files/" + subA.ID.String()},
		{name: "graph por id", target: "/compat/graph/v1.0/me/drive/items/" + fileA.ID.String()},
		{name: "graph contenido", target: "/compat/graph/v1.0/me/drive/items/" + fileA.ID.String() + "/content"},
	}
	for _, tc := range directos {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, intruder, http.MethodGet, tc.target, "", "")
			if rec.Code != http.StatusNotFound {
				t.Fatalf("código = %d, quiero 404 (%s)", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), fileA.Name) || strings.Contains(rec.Body.String(), "secreto de A") {
				t.Fatalf("fuga en el error: %s", rec.Body.String())
			}
		})
	}

	// Y el borrado tampoco alcanza lo ajeno.
	if rec := do(t, intruder, http.MethodDelete, "/compat/google/drive/v3/files/"+fileA.ID.String(), "", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("borrado ajeno = %d, quiero 404", rec.Code)
	}
}

// --- utilidades -----------------------------------------------------------

// urlq codifica un valor para la query string.
func urlq(v string) string {
	return strings.NewReplacer(" ", "%20", "'", "%27", "=", "%3D", "!", "%21", "/", "%2F").Replace(v)
}

// googleReason extrae error.errors[0].reason del sobre de error de Google.
func googleReason(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	body := decode(t, rec)
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("no es sobre de error de Google: %s", rec.Body.String())
	}
	items, _ := errObj["errors"].([]any)
	if len(items) == 0 {
		t.Fatalf("sobre sin errores: %s", rec.Body.String())
	}
	return items[0].(map[string]any)["reason"].(string)
}
