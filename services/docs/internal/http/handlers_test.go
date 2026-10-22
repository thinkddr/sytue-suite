// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/services/docs/internal/collab"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// errFakeUnused lo devuelven los métodos del fake que ningún test ejercita, para
// no violar la regla nilnil (no devolver (nil, nil) en pares valor/error).
var errFakeUnused = errors.New("fake: método no usado en este test")

// fakeStore implementa domain.Store para los tests de handlers HTTP sin PostgreSQL.
type borradoComentario struct {
	by        string
	moderador bool
}

type fakeStore struct {
	doc        *domain.Document
	perms      []domain.Permission
	getErr     error
	comments   []domain.Comment
	blobs      []domain.Blob
	shareRes   *domain.ShareResolution
	shareErr   error
	loaded     *domain.LoadedState
	recorded   *domain.Blob
	upsertPerm *domain.Permission
	creado     *domain.CreateDocumentParams // lo último que se pidió crear
	borrado    *borradoComentario           // lo último que se pidió borrar
	maxDias    *int                         // techo de caducidad de enlaces (nil = por defecto)
	claveHash  string                       // hash de contraseña del último enlace creado o cambiado
	// baseSeq es el base_seq que llegó al último AppendOps (nil = sin condición).
	baseSeq  *int64
	appended int
}

func (f *fakeStore) CreateDocument(_ context.Context, p domain.CreateDocumentParams) (*domain.Document, bool, error) {
	f.creado = &p
	return &domain.Document{ID: "new", Kind: p.Kind, Title: p.Title, OwnerSub: p.OwnerSub, SourceRef: p.SourceRef}, true, nil
}

func (f *fakeStore) GetDocument(_ context.Context, _ string) (*domain.Document, error) {
	return f.doc, f.getErr
}

func (f *fakeStore) ListDocuments(_ context.Context, _ domain.Subject, _ domain.ListFilter) ([]domain.Document, error) {
	if f.doc == nil {
		return nil, nil
	}
	return []domain.Document{*f.doc}, nil
}
func (f *fakeStore) RenameDocument(_ context.Context, _, _ string) error     { return nil }
func (f *fakeStore) SoftDeleteDocument(_ context.Context, _ string) error    { return nil }
func (f *fakeStore) SetStarred(_ context.Context, _, _ string, _ bool) error { return nil }
func (f *fakeStore) AppendOps(_ context.Context, _ string, _ domain.Subject, ops []domain.Op, baseSeq *int64) ([]domain.SequencedOp, int64, error) {
	f.baseSeq = baseSeq
	if baseSeq != nil && *baseSeq < 0 {
		return nil, 0, domain.ErrStaleSeq // el fake no tiene log: una base negativa simula «otro escribió»
	}
	f.appended += len(ops)
	out := make([]domain.SequencedOp, len(ops))
	for i, op := range ops {
		out[i] = domain.SequencedOp{Seq: int64(i + 1), Op: op}
	}
	return out, int64(len(ops)), nil
}

func (f *fakeStore) OpsSince(_ context.Context, _ string, _ int64, _ int) ([]domain.SequencedOp, error) {
	return nil, nil
}

func (f *fakeStore) LoadState(_ context.Context, _ string) (*domain.LoadedState, error) {
	if f.loaded != nil {
		return f.loaded, nil
	}
	return &domain.LoadedState{State: domain.NewState()}, nil
}

func (f *fakeStore) SaveSnapshot(_ context.Context, _ domain.SaveSnapshotParams) (string, error) {
	return "snap-1", nil
}

func (f *fakeStore) GetSnapshot(_ context.Context, _ string) (*domain.Snapshot, error) {
	return nil, errFakeUnused
}

func (f *fakeStore) ListPermissions(_ context.Context, _ string) ([]domain.Permission, error) {
	return f.perms, nil
}

func (f *fakeStore) UpsertPermission(_ context.Context, _ string, p domain.Permission) error {
	f.upsertPerm = &p
	return nil
}

func (f *fakeStore) RevokePermission(_ context.Context, _ string, _ domain.PrincipalKind, _ string) error {
	return nil
}

func (f *fakeStore) CreateShareLink(_ context.Context, docID, _, passwordHash, createdBy string, role domain.Role, expiresAt *time.Time) (*domain.ShareLink, error) {
	f.claveHash = passwordHash
	return &domain.ShareLink{ID: "l1", DocumentID: docID, Role: role, CreatedBy: createdBy, ExpiresAt: expiresAt, HasPassword: passwordHash != ""}, nil
}

func (f *fakeStore) ListShareLinks(_ context.Context, _ string) ([]domain.ShareLink, error) {
	return nil, nil
}
func (f *fakeStore) RevokeShareLink(_ context.Context, _, _ string) error { return nil }

// ClaveDeEnlace: el fake no tiene enlaces de verdad; resuelve como ResolveShareToken y
// dice la contraseña del último enlace creado o cambiado.
func (f *fakeStore) ClaveDeEnlace(_ context.Context, _ string) (string, string, error) {
	if f.shareErr != nil {
		return "", "", f.shareErr
	}
	return "l1", f.claveHash, nil
}

func (f *fakeStore) EstablecerClaveEnlace(_ context.Context, _, _, hash string) error {
	f.claveHash = hash
	return nil
}

func (f *fakeStore) PoliticaEnlaces(_ context.Context) (domain.PoliticaEnlaces, error) {
	if f.maxDias != nil {
		return domain.PoliticaEnlaces{MaxDias: *f.maxDias}, nil
	}
	return domain.PoliticaEnlaces{MaxDias: domain.TechoEnlacesPorDefecto, PorDefecto: true}, nil
}

func (f *fakeStore) ResolveShareToken(_ context.Context, _ string) (*domain.ShareResolution, error) {
	return f.shareRes, f.shareErr
}

func (f *fakeStore) CreateVersion(_ context.Context, docID, snapshotID, authorSub string, seq int64, label *string, auto bool) (*domain.Version, error) {
	return &domain.Version{ID: "v1", DocumentID: docID, SnapshotID: snapshotID, Seq: seq}, nil
}

func (f *fakeStore) ListVersions(_ context.Context, _ string, _ int) ([]domain.Version, error) {
	return nil, nil
}

func (f *fakeStore) GetVersion(_ context.Context, _, _ string) (*domain.Version, error) {
	return &domain.Version{ID: "v1", SnapshotID: "snap-1"}, nil
}

func (f *fakeStore) SnapshotState(_ context.Context, _ string, _ func(ctx context.Context, blobKey string) (*domain.State, error)) (*domain.State, int64, error) {
	return domain.NewState(), 0, nil
}

func (f *fakeStore) CreateComment(_ context.Context, p domain.CreateCommentParams) (*domain.Comment, error) {
	return &domain.Comment{ID: "c1", DocumentID: p.DocumentID, Body: p.Body, AuthorSub: p.AuthorSub}, nil
}

func (f *fakeStore) ListComments(_ context.Context, _ string, _ bool) ([]domain.Comment, error) {
	return f.comments, nil
}
func (f *fakeStore) ResolveComment(_ context.Context, _, _, _ string, _ bool) error { return nil }

func (f *fakeStore) DeleteComment(_ context.Context, _, _, by string, moderador bool) error {
	f.borrado = &borradoComentario{by: by, moderador: moderador}
	return nil
}

func (f *fakeStore) RecordBlob(_ context.Context, b domain.Blob) (*domain.Blob, error) {
	f.recorded = &b
	return &b, nil
}

func (f *fakeStore) GetBlob(_ context.Context, _, _ string) (*domain.Blob, error) {
	return nil, errFakeUnused
}

func (f *fakeStore) ListBlobs(_ context.Context, _ string) ([]domain.Blob, error) {
	return f.blobs, nil
}

var _ domain.Store = (*fakeStore)(nil)

// newHandler construye un Handler con un store fake y un hub real (en memoria).
func newHandler(fs *fakeStore) *Handler {
	svc := domain.NewService(fs)
	return &Handler{
		svc: svc, store: fs, hub: collab.NewHub(nil),
		presence: collab.NewPresenceRegistry(), inline: 256 * 1024,
		claveEnlaces: fs, limitesClave: nuevosLimitesClave(),
	}
}

// req construye una petición con claims de Sytue ID inyectados y, opcionalmente,
// un path param {id} y un cuerpo JSON.
func req(t *testing.T, method, target string, claims *authn.Claims, params map[string]string, body string) *http.Request {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, target, http.NoBody)
	}
	if claims != nil {
		r = r.WithContext(authn.Into(r.Context(), *claims))
	}
	if params != nil {
		rc := chi.NewRouteContext()
		for k, v := range params {
			rc.URLParams.Add(k, v)
		}
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
	}
	return r
}

func ownerClaims() *authn.Claims { return &authn.Claims{Subject: "owner", Tenant: "acme"} }

func ownedDoc() *domain.Document {
	return &domain.Document{ID: "d1", OwnerSub: "owner", Kind: domain.KindDocument, Title: "T"}
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("respuesta no es JSON: %v (%s)", err, rec.Body.String())
	}
	return m
}

func TestCreateDocument(t *testing.T) {
	h := newHandler(&fakeStore{})

	// Sin claims -> 401.
	rec := httptest.NewRecorder()
	h.createDocument(rec, req(t, "POST", "/v1/documents", nil, nil, `{"kind":"document","title":"X"}`))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin auth: code=%d", rec.Code)
	}

	// Cuerpo inválido -> 400.
	rec = httptest.NewRecorder()
	h.createDocument(rec, req(t, "POST", "/v1/documents", ownerClaims(), nil, `{bad`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("json inválido: code=%d", rec.Code)
	}

	// Kind inválido -> 400 (mapeado de ErrInvalidKind).
	rec = httptest.NewRecorder()
	h.createDocument(rec, req(t, "POST", "/v1/documents", ownerClaims(), nil, `{"kind":"bogus","title":"X"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("kind inválido: code=%d", rec.Code)
	}

	// Feliz -> 201, envelope con document + role owner.
	rec = httptest.NewRecorder()
	h.createDocument(rec, req(t, "POST", "/v1/documents", ownerClaims(), nil, `{"kind":"document","title":"Hola"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("crear: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type = %q", ct)
	}
	body := decodeBody(t, rec)
	if body["role"] != "owner" {
		t.Errorf("role = %v, want owner", body["role"])
	}
}

func TestGetDocument(t *testing.T) {
	// Documento del owner -> 200.
	h := newHandler(&fakeStore{doc: ownedDoc()})
	rec := httptest.NewRecorder()
	h.getDocument(rec, req(t, "GET", "/v1/documents/d1", ownerClaims(), map[string]string{"id": "d1"}, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("get: code=%d", rec.Code)
	}

	// Documento ajeno sin permisos -> 403.
	h = newHandler(&fakeStore{doc: &domain.Document{ID: "d1", OwnerSub: "other"}})
	rec = httptest.NewRecorder()
	h.getDocument(rec, req(t, "GET", "/v1/documents/d1", &authn.Claims{Subject: "stranger"}, map[string]string{"id": "d1"}, ""))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("forbidden: code=%d", rec.Code)
	}

	// Documento inexistente -> 404.
	h = newHandler(&fakeStore{getErr: domain.ErrNotFound})
	rec = httptest.NewRecorder()
	h.getDocument(rec, req(t, "GET", "/v1/documents/d1", ownerClaims(), map[string]string{"id": "d1"}, ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("not found: code=%d", rec.Code)
	}
}

func TestListDocuments(t *testing.T) {
	h := newHandler(&fakeStore{doc: ownedDoc()})
	rec := httptest.NewRecorder()
	cursor := "2026-01-02T03:04:05.000006Z~0195a4c8-0000-7000-8000-000000000000"
	h.listDocuments(rec, req(t, "GET", "/v1/documents?limit=5&kind=document&cursor="+cursor, ownerClaims(), nil, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("list: code=%d", rec.Code)
	}
	body := decodeBody(t, rec)
	if _, ok := body["documents"]; !ok {
		t.Error("falta clave documents")
	}

	// Cursor corrupto -> 400 (nunca un error de cast en la BD).
	for _, bad := range []string{"garbage", "2026-01-02T03:04:05Z~no-es-uuid", "no-fecha~0195a4c8-0000-7000-8000-000000000000"} {
		rec = httptest.NewRecorder()
		h.listDocuments(rec, req(t, "GET", "/v1/documents?cursor="+bad, ownerClaims(), nil, ""))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("cursor %q: code=%d, quería 400", bad, rec.Code)
		}
	}
}

func TestParseListCursorRoundTrip(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 6000, time.UTC) // microsegundos (precisión de timestamptz)
	id := "0195a4c8-0000-7000-8000-000000000000"
	gotAt, gotID, err := parseListCursor(at.Format(time.RFC3339Nano) + "~" + id)
	if err != nil || !gotAt.Equal(at) || gotID != id {
		t.Fatalf("parseListCursor => %v %q (%v)", gotAt, gotID, err)
	}
}

func TestRenameDeleteStar(t *testing.T) {
	h := newHandler(&fakeStore{doc: ownedDoc()})

	// Rename feliz -> 204.
	rec := httptest.NewRecorder()
	h.renameDocument(rec, req(t, "PATCH", "/v1/documents/d1", ownerClaims(), map[string]string{"id": "d1"}, `{"title":"Nuevo"}`))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("rename: code=%d", rec.Code)
	}

	// Rename título vacío -> 400.
	rec = httptest.NewRecorder()
	h.renameDocument(rec, req(t, "PATCH", "/v1/documents/d1", ownerClaims(), map[string]string{"id": "d1"}, `{"title":"  "}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("rename vacío: code=%d", rec.Code)
	}

	// Delete feliz -> 204.
	rec = httptest.NewRecorder()
	h.deleteDocument(rec, req(t, "DELETE", "/v1/documents/d1", ownerClaims(), map[string]string{"id": "d1"}, ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: code=%d", rec.Code)
	}

	// Star feliz -> 204.
	rec = httptest.NewRecorder()
	h.starDocument(rec, req(t, "PUT", "/v1/documents/d1/star", ownerClaims(), map[string]string{"id": "d1"}, `{"starred":true}`))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("star: code=%d", rec.Code)
	}
}

func TestPostAndGetOps(t *testing.T) {
	h := newHandler(&fakeStore{doc: ownedDoc()})

	// Lote vacío -> 400.
	rec := httptest.NewRecorder()
	h.postOps(rec, req(t, "POST", "/v1/documents/d1/ops", ownerClaims(), map[string]string{"id": "d1"}, `{"ops":[]}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("ops vacías: code=%d", rec.Code)
	}

	// Lote con una op válida -> 200.
	rec = httptest.NewRecorder()
	h.postOps(rec, req(t, "POST", "/v1/documents/d1/ops", ownerClaims(), map[string]string{"id": "d1"},
		`{"ops":[{"kind":"insert","dot":{"a":"x","c":1},"value":"\"H\"","lamport":1}]}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("postOps: code=%d body=%s", rec.Code, rec.Body.String())
	}

	// base_seq llega al store; una base obsoleta es 409 y el cliente fusiona.
	fs := &fakeStore{doc: ownedDoc()}
	hb := newHandler(fs)
	rec = httptest.NewRecorder()
	hb.postOps(rec, req(t, "POST", "/v1/documents/d1/ops", ownerClaims(), map[string]string{"id": "d1"},
		`{"ops":[{"kind":"set","dot":{"a":"x","c":1},"key":"doc:sheet","value":"\"{}\"","lamport":1}],"base_seq":7}`))
	if rec.Code != http.StatusOK || fs.baseSeq == nil || *fs.baseSeq != 7 {
		t.Fatalf("base_seq no llegó al store: code=%d base=%v", rec.Code, fs.baseSeq)
	}
	rec = httptest.NewRecorder()
	hb.postOps(rec, req(t, "POST", "/v1/documents/d1/ops", ownerClaims(), map[string]string{"id": "d1"},
		`{"ops":[{"kind":"set","dot":{"a":"x","c":2},"key":"doc:sheet","value":"\"{}\"","lamport":2}],"base_seq":-1}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("base obsoleta: code=%d, quiero 409", rec.Code)
	}

	// Un cliente anterior (Fyne, Pages) no manda base_seq: escritura LWW incondicional.
	rec = httptest.NewRecorder()
	hb.postOps(rec, req(t, "POST", "/v1/documents/d1/ops", ownerClaims(), map[string]string{"id": "d1"},
		`{"ops":[{"kind":"set","dot":{"a":"x","c":3},"key":"doc:sheet","value":"\"{}\"","lamport":3}]}`))
	if rec.Code != http.StatusOK || fs.baseSeq != nil {
		t.Fatalf("sin base_seq: code=%d base=%v, quiero 200 y sin condición", rec.Code, fs.baseSeq)
	}

	// getOps -> 200.
	rec = httptest.NewRecorder()
	h.getOps(rec, req(t, "GET", "/v1/documents/d1/ops?since=0", ownerClaims(), map[string]string{"id": "d1"}, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("getOps: code=%d", rec.Code)
	}
}

// Las llamadas REST directas no pueden saltarse el modo de solo lectura del editor.
func TestPostOpsRoleMatrix(t *testing.T) {
	for _, kind := range []domain.Kind{domain.KindDocument, domain.KindSpreadsheet, domain.KindPresentation} {
		for _, role := range []domain.Role{"", domain.RoleViewer, domain.RoleCommenter, domain.RoleEditor, domain.RoleOwner, "unknown"} {
			t.Run(string(kind)+"/"+string(role), func(t *testing.T) {
				fs := &fakeStore{doc: &domain.Document{ID: "d1", Kind: kind, OwnerSub: "someone-else"}}
				fs.perms = []domain.Permission{{PrincipalKind: domain.PrincipalUser, PrincipalID: "owner", Role: role}}
				rec := httptest.NewRecorder()
				newHandler(fs).postOps(rec, req(t, "POST", "/v1/documents/d1/ops", ownerClaims(), map[string]string{"id": "d1"},
					`{"ops":[{"kind":"insert","dot":{"a":"x","c":1},"value":"\"H\"","lamport":1}]}`))
				want, count := http.StatusForbidden, 0
				if role.CanWrite() {
					want, count = http.StatusOK, 1
				}
				if rec.Code != want || fs.appended != count {
					t.Fatalf("rol %q: HTTP=%d, escrituras=%d; quiero %d/%d", role, rec.Code, fs.appended, want, count)
				}
			})
		}
	}
}

func TestPresence(t *testing.T) {
	h := newHandler(&fakeStore{doc: ownedDoc()})

	// Sin conn -> 400.
	rec := httptest.NewRecorder()
	h.putPresence(rec, req(t, "PUT", "/v1/documents/d1/presence", ownerClaims(), map[string]string{"id": "d1"}, `{"conn":""}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("presence sin conn: code=%d", rec.Code)
	}

	// Heartbeat feliz -> 200, el propio solicitante ya aparece como miembro.
	rec = httptest.NewRecorder()
	h.putPresence(rec, req(t, "PUT", "/v1/documents/d1/presence", ownerClaims(), map[string]string{"id": "d1"}, `{"conn":"tabA"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("presence: code=%d body=%s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	members, _ := body["members"].([]any)
	if len(members) != 1 {
		t.Fatalf("primer heartbeat: %d miembros, want 1 (%s)", len(members), rec.Body.String())
	}

	// Segunda pestaña del mismo owner -> 2 miembros (dos conexiones, un usuario):
	// el lector pasivo aparece sin haber emitido ninguna op.
	rec = httptest.NewRecorder()
	h.putPresence(rec, req(t, "PUT", "/v1/documents/d1/presence", ownerClaims(), map[string]string{"id": "d1"}, `{"conn":"tabB"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("presence 2: code=%d", rec.Code)
	}
	if members, _ := decodeBody(t, rec)["members"].([]any); len(members) != 2 {
		t.Fatalf("segunda pestaña: %d miembros, want 2", len(members))
	}

	// Documento ajeno sin permiso -> 403 (no se registra presencia).
	h = newHandler(&fakeStore{doc: &domain.Document{ID: "d1", OwnerSub: "other"}})
	rec = httptest.NewRecorder()
	h.putPresence(rec, req(t, "PUT", "/v1/documents/d1/presence", &authn.Claims{Subject: "stranger"}, map[string]string{"id": "d1"}, `{"conn":"x"}`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("presence ajeno: code=%d", rec.Code)
	}
}

func TestGetState(t *testing.T) {
	h := newHandler(&fakeStore{doc: ownedDoc()})
	rec := httptest.NewRecorder()
	h.getState(rec, req(t, "GET", "/v1/documents/d1/state", ownerClaims(), map[string]string{"id": "d1"}, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("getState: code=%d", rec.Code)
	}
	body := decodeBody(t, rec)
	if _, ok := body["state"]; !ok {
		t.Error("falta clave state")
	}
}

func TestComments(t *testing.T) {
	h := newHandler(&fakeStore{doc: ownedDoc()})

	// list -> 200.
	rec := httptest.NewRecorder()
	h.listComments(rec, req(t, "GET", "/v1/documents/d1/comments", ownerClaims(), map[string]string{"id": "d1"}, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("listComments: code=%d", rec.Code)
	}

	// create cuerpo vacío -> 400.
	rec = httptest.NewRecorder()
	h.createComment(rec, req(t, "POST", "/v1/documents/d1/comments", ownerClaims(), map[string]string{"id": "d1"}, `{"body":"   "}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("comment vacío: code=%d", rec.Code)
	}

	// create feliz -> 201.
	rec = httptest.NewRecorder()
	h.createComment(rec, req(t, "POST", "/v1/documents/d1/comments", ownerClaims(), map[string]string{"id": "d1"}, `{"body":"hola"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("createComment: code=%d", rec.Code)
	}

	// resolve -> 204.
	rec = httptest.NewRecorder()
	h.resolveComment(rec, req(t, "PUT", "/v1/documents/d1/comments/c1/resolve", ownerClaims(),
		map[string]string{"id": "d1", "commentId": "c1"}, `{"resolved":true}`))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("resolveComment: code=%d", rec.Code)
	}
}

func TestPermissions(t *testing.T) {
	h := newHandler(&fakeStore{doc: ownedDoc()})

	// list (requiere owner) -> 200.
	rec := httptest.NewRecorder()
	h.listPermissions(rec, req(t, "GET", "/v1/documents/d1/permissions", ownerClaims(), map[string]string{"id": "d1"}, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("listPermissions: code=%d", rec.Code)
	}

	// upsert principal public -> 400.
	rec = httptest.NewRecorder()
	h.upsertPermission(rec, req(t, "PUT", "/v1/documents/d1/permissions", ownerClaims(), map[string]string{"id": "d1"},
		`{"principal_kind":"public","principal_id":"","role":"viewer"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("upsert public: code=%d", rec.Code)
	}

	// upsert rol inválido -> 400.
	rec = httptest.NewRecorder()
	h.upsertPermission(rec, req(t, "PUT", "/v1/documents/d1/permissions", ownerClaims(), map[string]string{"id": "d1"},
		`{"principal_kind":"user","principal_id":"u2","role":"bogus"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("upsert rol inválido: code=%d", rec.Code)
	}

	// upsert feliz -> 200.
	fs := &fakeStore{doc: ownedDoc()}
	h = newHandler(fs)
	rec = httptest.NewRecorder()
	h.upsertPermission(rec, req(t, "PUT", "/v1/documents/d1/permissions", ownerClaims(), map[string]string{"id": "d1"},
		`{"principal_kind":"user","principal_id":"u2","role":"editor"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("upsert: code=%d", rec.Code)
	}
	if fs.upsertPerm == nil || fs.upsertPerm.Role != domain.RoleEditor {
		t.Errorf("upsertPerm no propagado: %+v", fs.upsertPerm)
	}

	// revoke sin params -> 400.
	rec = httptest.NewRecorder()
	h.revokePermission(rec, req(t, "DELETE", "/v1/documents/d1/permissions", ownerClaims(), map[string]string{"id": "d1"}, ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("revoke sin params: code=%d", rec.Code)
	}

	// Un no-owner no puede listar permisos -> 403.
	h = newHandler(&fakeStore{doc: &domain.Document{ID: "d1", OwnerSub: "x"}})
	rec = httptest.NewRecorder()
	h.listPermissions(rec, req(t, "GET", "/v1/documents/d1/permissions", &authn.Claims{Subject: "y"}, map[string]string{"id": "d1"}, ""))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("listPermissions no-owner: code=%d", rec.Code)
	}
}

func TestShares(t *testing.T) {
	h := newHandler(&fakeStore{doc: ownedDoc()})

	// createShare rol owner (no permitido en enlaces) -> 400.
	rec := httptest.NewRecorder()
	h.createShare(rec, req(t, "POST", "/v1/documents/d1/shares", ownerClaims(), map[string]string{"id": "d1"}, `{"role":"owner"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("share owner: code=%d", rec.Code)
	}

	// createShare expiry inválido -> 400.
	rec = httptest.NewRecorder()
	h.createShare(rec, req(t, "POST", "/v1/documents/d1/shares", ownerClaims(), map[string]string{"id": "d1"}, `{"role":"viewer","expires_at":"ayer"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("share expiry inválido: code=%d", rec.Code)
	}

	// createShare feliz -> 201.
	rec = httptest.NewRecorder()
	h.createShare(rec, req(t, "POST", "/v1/documents/d1/shares", ownerClaims(), map[string]string{"id": "d1"}, `{"role":"viewer"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("createShare: code=%d", rec.Code)
	}
}

func TestResolveShareHandler(t *testing.T) {
	// Token vacío -> 400.
	h := newHandler(&fakeStore{})
	rec := httptest.NewRecorder()
	h.resolveShare(rec, req(t, "GET", "/v1/share/", nil, map[string]string{"token": ""}, ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("share token vacío: code=%d", rec.Code)
	}

	// Token caducado -> 410.
	h = newHandler(&fakeStore{shareErr: domain.ErrShareExpired})
	rec = httptest.NewRecorder()
	h.resolveShare(rec, req(t, "GET", "/v1/share/tok", nil, map[string]string{"token": "tok"}, ""))
	if rec.Code != http.StatusGone {
		t.Fatalf("share caducado: code=%d", rec.Code)
	}

	// Token válido -> 200 con documento y rol.
	h = newHandler(&fakeStore{
		shareRes: &domain.ShareResolution{DocumentID: "d1", Tenant: "acme", Role: domain.RoleViewer},
		doc:      ownedDoc(),
	})
	rec = httptest.NewRecorder()
	h.resolveShare(rec, req(t, "GET", "/v1/share/tok", nil, map[string]string{"token": "tok"}, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("share válido: code=%d body=%s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	if body["role"] != "viewer" {
		t.Errorf("role = %v", body["role"])
	}
}

func TestViewShareHandler(t *testing.T) {
	// Caducado o revocado -> 410, igual que al resolverlo: la web lo convierte en 404.
	h := newHandler(&fakeStore{shareErr: domain.ErrShareExpired})
	rec := httptest.NewRecorder()
	h.viewShare(rec, req(t, "GET", "/v1/share/tok/view", nil, map[string]string{"token": "tok"}, ""))
	if rec.Code != http.StatusGone {
		t.Fatalf("vista caducada: code=%d", rec.Code)
	}

	// Vigente -> el HTML del editor y el rol, sin JWT y fuera de los buscadores.
	st, err := domain.StateFromJSON(json.RawMessage(`{"registers":{"doc:html":{"value":"\"<p>Hola</p>\""}}}`))
	if err != nil {
		t.Fatal(err)
	}
	h = newHandler(&fakeStore{
		shareRes: &domain.ShareResolution{DocumentID: "d1", Tenant: "acme", Role: domain.RoleEditor},
		doc:      ownedDoc(),
		loaded:   &domain.LoadedState{State: st},
	})
	rec = httptest.NewRecorder()
	h.viewShare(rec, req(t, "GET", "/v1/share/tok/view", nil, map[string]string{"token": "tok"}, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("vista: code=%d body=%s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	if html, _ := body["html"].(string); !strings.Contains(html, "<p>Hola</p>") {
		t.Errorf("html = %q", html)
	}
	if body["role"] != "editor" || body["title"] != "T" {
		t.Errorf("role/title = %v/%v", body["role"], body["title"])
	}
	if got := rec.Header().Get("X-Robots-Tag"); !strings.Contains(got, "noindex") {
		t.Errorf("X-Robots-Tag = %q", got)
	}
}

func TestExports(t *testing.T) {
	h := newHandler(&fakeStore{doc: ownedDoc()})

	// list -> 200.
	rec := httptest.NewRecorder()
	h.listExports(rec, req(t, "GET", "/v1/documents/d1/exports", ownerClaims(), map[string]string{"id": "d1"}, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("listExports: code=%d", rec.Code)
	}

	// createExport formato inválido -> 400.
	rec = httptest.NewRecorder()
	h.createExport(rec, req(t, "POST", "/v1/documents/d1/exports", ownerClaims(), map[string]string{"id": "d1"}, `{"format":"bogus"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("export inválido: code=%d", rec.Code)
	}

	// createExport sin object storage -> 503.
	rec = httptest.NewRecorder()
	h.createExport(rec, req(t, "POST", "/v1/documents/d1/exports", ownerClaims(), map[string]string{"id": "d1"}, `{"format":"markdown"}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("export sin S3: code=%d", rec.Code)
	}
}

func TestVersions(t *testing.T) {
	h := newHandler(&fakeStore{doc: ownedDoc(), loaded: &domain.LoadedState{State: domain.NewState()}})

	// list -> 200.
	rec := httptest.NewRecorder()
	h.listVersions(rec, req(t, "GET", "/v1/documents/d1/versions", ownerClaims(), map[string]string{"id": "d1"}, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("listVersions: code=%d", rec.Code)
	}

	// restore -> 200 (documento idéntico => Changed=false, sigue 200).
	rec = httptest.NewRecorder()
	h.restoreVersion(rec, req(t, "POST", "/v1/documents/d1/versions/v1/restore", ownerClaims(),
		map[string]string{"id": "d1", "versionId": "v1"}, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("restoreVersion: code=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestWSOriginPatternsCerrada fija la frontera de confianza del handshake del
// WebSocket de colaboración. Bajo la zona sytue.eu viven *.apps.sytue.eu y
// *.preview.sytue.eu, donde Cloud ejecuta código de sus clientes: un comodín de
// zona ("*.sytue.eu") los daría por buenos, así que la allowlist debe ser la
// lista cerrada de primera parte. Se evalúa con path.Match, el matcher exacto
// que usa coder/websocket.
func TestWSOriginPatternsCerrada(t *testing.T) {
	rechazados := []string{
		"app-de-un-cliente.apps.sytue.eu",
		"pr-42.preview.sytue.eu",
		"cualquiera.apps.sytue.localhost",
		"sytue.eu.atacante.com",
	}
	prod := wsOriginPatterns(pcfg.EnvProd)
	for _, host := range rechazados {
		for _, pat := range prod {
			ok, err := path.Match(strings.ToLower(pat), strings.ToLower(host))
			if err != nil {
				t.Fatalf("patrón inválido %q: %v", pat, err)
			}
			if ok {
				t.Errorf("el patrón %q autoriza %q; la allowlist WS no debe confiar en dominios de código de cliente", pat, host)
			}
		}
	}
	// El editor corre en la app de cada producto (ADR 0076): los cuatro abren el canal.
	for _, host := range []string{"docus.sytue.eu", "calcus.sytue.eu", "diapos.sytue.eu", "certipdf.sytue.eu"} {
		if !slices.Contains(prod, host) {
			t.Errorf("wsOriginPatterns debe incluir el editor de %s: %v", host, prod)
		}
	}
	// En producción no entra NINGÚN host de desarrollo: la lista era una sola
	// constante con prod y dev concatenados, y eso viajaba tal cual a prod.
	for _, pat := range prod {
		if strings.HasSuffix(pat, ".localhost") {
			t.Errorf("la allowlist WS de prod incluye un host de desarrollo: %q", pat)
		}
	}
	// Y en dev sí, o no se puede trabajar contra sytue.localhost.
	if !slices.Contains(wsOriginPatterns(pcfg.EnvDev), "calcus.sytue.localhost") {
		t.Error("en dev la allowlist WS debe seguir admitiendo calcus.sytue.localhost")
	}
}
