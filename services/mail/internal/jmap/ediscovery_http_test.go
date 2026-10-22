// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests de los handlers de compliance (legal hold + eDiscovery) con un fake del
// store, sin PostgreSQL ni S3. Cubren: gating por rol (owner|admin) — 401/403;
// creación/liberación de hold; búsqueda que surfacea el correo PRESERVADO sin
// filtrar por deleted_at; auditoría fail-closed; aislamiento por org (objetivo de
// otra org => 404); y el formato mbox del export. El aislamiento REAL por RLS y
// la preservación frente a la purga se prueban en el test de integración del store.
package jmap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// --- fakes ----------------------------------------------------------------

type fakeCompliance struct {
	byAddr    domain.Account
	byAddrErr error
	bySub     domain.Account
	bySubErr  error

	holdRet     domain.LegalHold
	holdErr     error
	holds       []domain.LegalHold
	holdsErr    error
	relErr      error
	searchRet   []domain.EDiscoveryHit
	searchErr   error
	auditErr    error
	auditLog    []domain.AuditEntry
	auditLogErr error

	lastHold      domain.LegalHold
	lastReleaseID uuid.UUID
	lastRelActor  string
	lastAccountID uuid.UUID
	lastFilter    store.EDiscoveryFilter
	audits        []auditCall
}

type auditCall struct {
	actor, action, target string
	detail                map[string]any
}

func (f *fakeCompliance) AccountByAddress(context.Context, string) (domain.Account, error) {
	return f.byAddr, f.byAddrErr
}

func (f *fakeCompliance) AccountBySubject(context.Context, string) (domain.Account, error) {
	return f.bySub, f.bySubErr
}

func (f *fakeCompliance) CreateHold(_ context.Context, h domain.LegalHold) (domain.LegalHold, error) {
	f.lastHold = h
	if f.holdErr != nil {
		return domain.LegalHold{}, f.holdErr
	}
	out := f.holdRet
	out.Subject = h.Subject
	out.CreatedBy = h.CreatedBy
	return out, nil
}

func (f *fakeCompliance) ListHolds(context.Context, bool) ([]domain.LegalHold, error) {
	return f.holds, f.holdsErr
}

func (f *fakeCompliance) ReleaseHold(_ context.Context, id uuid.UUID, actor string) error {
	f.lastReleaseID, f.lastRelActor = id, actor
	return f.relErr
}

func (f *fakeCompliance) EDiscoverySearch(_ context.Context, accountID uuid.UUID, filter store.EDiscoveryFilter) ([]domain.EDiscoveryHit, error) {
	f.lastAccountID, f.lastFilter = accountID, filter
	return f.searchRet, f.searchErr
}

func (f *fakeCompliance) WriteAudit(_ context.Context, actor, action, target string, detail any) error {
	if f.auditErr != nil {
		return f.auditErr
	}
	m, _ := detail.(map[string]any)
	f.audits = append(f.audits, auditCall{actor: actor, action: action, target: target, detail: m})
	return nil
}

func (f *fakeCompliance) ListAudit(_ context.Context, _ int) ([]domain.AuditEntry, error) {
	return f.auditLog, f.auditLogErr
}

// complianceAPI construye una API con solo la dependencia de compliance (+ blobs).
func complianceAPI(fc *fakeCompliance, blobs blobStore) *API {
	a := newAPI(nil, blobs, nil)
	a.compliance = fc
	return a
}

// adminReq es como req pero con claims de rol admin (owner|admin) + tenant.
func adminReq(method, target, body string, params map[string]string) *http.Request {
	r := req(method, target, body, true, params)
	ctx := authn.Into(r.Context(), authn.Claims{
		Subject: "admin-1", Email: "boss@sytue.eu", Tenant: "acme", Roles: []string{"admin"},
	})
	ctx = tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"})
	return r.WithContext(ctx)
}

// --- gating por rol -------------------------------------------------------

func TestCreateHoldForbiddenNonAdmin(t *testing.T) {
	fc := &fakeCompliance{}
	a := complianceAPI(fc, nil)
	rec := httptest.NewRecorder()
	// req() inyecta claims SIN roles => requireAdmin debe rechazar.
	a.createHold(rec, req(http.MethodPost, "/api/v1/admin/legal-holds", `{"org_wide":true}`, true, nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("sin rol admin => %d, quiero 403", rec.Code)
	}
	if fc.lastHold.CreatedBy != "" {
		t.Fatal("no debe llegar al store sin rol")
	}
}

func TestCreateHoldNoAuth(t *testing.T) {
	a := complianceAPI(&fakeCompliance{}, nil)
	rec := httptest.NewRecorder()
	a.createHold(rec, noAuthReq(http.MethodPost, "/x", `{"org_wide":true}`))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin auth => %d, quiero 401", rec.Code)
	}
}

func TestEdiscoverySearchForbiddenNonAdmin(t *testing.T) {
	fc := &fakeCompliance{}
	a := complianceAPI(fc, nil)
	rec := httptest.NewRecorder()
	a.ediscoverySearch(rec, req(http.MethodPost, "/x", `{"email":"x@acme.eu"}`, true, nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("sin rol admin => %d, quiero 403", rec.Code)
	}
	if fc.lastAccountID != uuid.Nil {
		t.Fatal("no debe buscar sin rol")
	}
}

// --- legal hold -----------------------------------------------------------

func TestCreateHoldByEmailResolvesSubject(t *testing.T) {
	fc := &fakeCompliance{byAddr: domain.Account{ID: uuid.New(), Address: "sue@acme.eu", Subject: "sub-sue"}}
	a := complianceAPI(fc, nil)
	rec := httptest.NewRecorder()
	a.createHold(rec, adminReq(http.MethodPost, "/x", `{"email":"sue@acme.eu","case":"CASE-1","reason":"litigio"}`, nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if fc.lastHold.Subject != "sub-sue" || fc.lastHold.Case != "CASE-1" || fc.lastHold.CreatedBy != "admin-1" {
		t.Fatalf("hold no propagado: %+v", fc.lastHold)
	}
}

func TestCreateHoldOrgWide(t *testing.T) {
	fc := &fakeCompliance{}
	a := complianceAPI(fc, nil)
	rec := httptest.NewRecorder()
	a.createHold(rec, adminReq(http.MethodPost, "/x", `{"org_wide":true,"case":"ORG"}`, nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if fc.lastHold.Subject != "" {
		t.Fatalf("org-wide debe ir sin sujeto, got %q", fc.lastHold.Subject)
	}
	if fc.byAddr.Subject != "" { // no debe intentar resolver cuenta
		t.Fatal("org-wide no resuelve cuenta objetivo")
	}
}

func TestCreateHoldTargetOtherOrgNotFound(t *testing.T) {
	// Objetivo inexistente en la org del admin (RLS lo ocultaría) => 404, sin hold.
	fc := &fakeCompliance{byAddrErr: domain.ErrNotFound}
	a := complianceAPI(fc, nil)
	rec := httptest.NewRecorder()
	a.createHold(rec, adminReq(http.MethodPost, "/x", `{"email":"ghost@other.eu"}`, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("objetivo ajeno => %d, quiero 404", rec.Code)
	}
	if fc.lastHold.CreatedBy != "" {
		t.Fatal("no debe crear hold sobre objetivo inexistente")
	}
}

func TestReleaseHoldOK(t *testing.T) {
	fc := &fakeCompliance{}
	a := complianceAPI(fc, nil)
	id := uuid.New()
	rec := httptest.NewRecorder()
	a.releaseHold(rec, adminReq(http.MethodPost, "/x", "", map[string]string{"holdID": id.String()}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if fc.lastReleaseID != id || fc.lastRelActor != "admin-1" {
		t.Fatalf("release no propagado: %v %q", fc.lastReleaseID, fc.lastRelActor)
	}
}

func TestListHoldsOK(t *testing.T) {
	fc := &fakeCompliance{holds: []domain.LegalHold{{ID: uuid.New(), Case: "C1"}}}
	a := complianceAPI(fc, nil)
	rec := httptest.NewRecorder()
	a.listHolds(rec, adminReq(http.MethodGet, "/x", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	m := decodeBody(t, rec)
	if items, ok := m["holds"].([]any); !ok || len(items) != 1 {
		t.Fatalf("holds = %+v", m["holds"])
	}
}

// --- eDiscovery search ----------------------------------------------------

func TestEdiscoverySearchSurfacesPreservedAndAudits(t *testing.T) {
	target := domain.Account{ID: uuid.New(), Address: "sue@acme.eu", Subject: "sub-sue"}
	fc := &fakeCompliance{
		byAddr: target,
		searchRet: []domain.EDiscoveryHit{
			{Message: domain.Message{ID: uuid.New(), Subject: "vivo", FromAddr: "a@x"}, Preserved: false},
			{Message: domain.Message{ID: uuid.New(), Subject: "borrado-retenido", FromAddr: "b@x"}, Preserved: true},
		},
	}
	a := complianceAPI(fc, nil)
	rec := httptest.NewRecorder()
	body := `{"email":"sue@acme.eu","from":"2026-01-01","to":"2026-12-31","query":"factura","case":"CASE-1"}`
	a.ediscoverySearch(rec, adminReq(http.MethodPost, "/x", body, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	// El objetivo se buscó por account id resuelto y con el rango/consulta.
	if fc.lastAccountID != target.ID || fc.lastFilter.Query != "factura" || fc.lastFilter.From.IsZero() || fc.lastFilter.To.IsZero() {
		t.Fatalf("filtro no propagado: acc=%v filter=%+v", fc.lastAccountID, fc.lastFilter)
	}
	m := decodeBody(t, rec)
	msgs, ok := m["messages"].([]any)
	if !ok || len(msgs) != 2 {
		t.Fatalf("messages = %+v", m["messages"])
	}
	// El correo retenido aparece marcado preserved=true (visible para el caso).
	var sawPreserved bool
	for _, it := range msgs {
		row := it.(map[string]any)
		if row["subject"] == "borrado-retenido" && row["preserved"] == true {
			sawPreserved = true
		}
		if _, leaked := row["blob_key"]; leaked {
			t.Fatal("la lista NO debe filtrar blob_key/cuerpo")
		}
	}
	if !sawPreserved {
		t.Fatal("el correo preservado debe surfacearse con preserved=true")
	}
	// Auditoría escrita: quién, acción, sujeto y caso.
	if len(fc.audits) != 1 {
		t.Fatalf("auditoría = %d entradas, quiero 1", len(fc.audits))
	}
	au := fc.audits[0]
	if au.actor != "admin-1" || au.action != "ediscovery.search" || au.target != "sue@acme.eu" || au.detail["case"] != "CASE-1" {
		t.Fatalf("auditoría incompleta: %+v", au)
	}
}

func TestEdiscoverySearchFailClosedOnAuditError(t *testing.T) {
	// Si la auditoría falla, NO se sirve el resultado (fail-closed): 500.
	fc := &fakeCompliance{
		byAddr:    domain.Account{ID: uuid.New(), Address: "sue@acme.eu", Subject: "sub-sue"},
		searchRet: []domain.EDiscoveryHit{{Message: domain.Message{ID: uuid.New()}}},
		auditErr:  context.DeadlineExceeded,
	}
	a := complianceAPI(fc, nil)
	rec := httptest.NewRecorder()
	a.ediscoverySearch(rec, adminReq(http.MethodPost, "/x", `{"email":"sue@acme.eu"}`, nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("auditoría fallida => %d, quiero 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `"messages"`) {
		t.Fatal("no debe filtrar resultados sin traza")
	}
}

func TestEdiscoverySearchTargetOtherOrgNotFound(t *testing.T) {
	fc := &fakeCompliance{byAddrErr: domain.ErrNotFound}
	a := complianceAPI(fc, nil)
	rec := httptest.NewRecorder()
	a.ediscoverySearch(rec, adminReq(http.MethodPost, "/x", `{"email":"ghost@other.eu"}`, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("objetivo ajeno => %d, quiero 404", rec.Code)
	}
	if len(fc.audits) != 0 {
		t.Fatal("no debe auditar una búsqueda sobre objetivo inexistente")
	}
}

// --- eDiscovery export ----------------------------------------------------

func TestEdiscoveryExportMboxOK(t *testing.T) {
	eml := "Subject: hola\r\nFrom nobody\r\n\r\ncuerpo\r\n"
	fc := &fakeCompliance{
		byAddr:    domain.Account{ID: uuid.New(), Address: "sue@acme.eu", Subject: "sub-sue"},
		searchRet: []domain.EDiscoveryHit{{Message: domain.Message{ID: uuid.New(), FromAddr: "a@x.eu", BlobKey: "k1", ReceivedAt: time.Now()}}},
	}
	a := complianceAPI(fc, &fakeBlobs{getBody: eml})
	rec := httptest.NewRecorder()
	a.ediscoveryExport(rec, adminReq(http.MethodGet, "/x?email=sue@acme.eu&case=CASE-1", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/mbox" {
		t.Fatalf("Content-Type = %q", ct)
	}
	out := rec.Body.String()
	if !strings.HasPrefix(out, "From a@x.eu ") {
		t.Fatalf("falta el separador mbox From_: %q", out[:min(40, len(out))])
	}
	if !strings.Contains(out, "Subject: hola") || !strings.Contains(out, "cuerpo") {
		t.Fatalf("el .eml no se incluyó: %q", out)
	}
	// mboxrd: la línea de cuerpo "From nobody" se escapa con '>'.
	if !strings.Contains(out, ">From nobody") {
		t.Fatalf("mboxrd no escapó 'From ' del cuerpo: %q", out)
	}
	if len(fc.audits) != 1 || fc.audits[0].action != "ediscovery.export" {
		t.Fatalf("export debe auditar: %+v", fc.audits)
	}
}

func TestEdiscoveryExportFailClosedOnAuditError(t *testing.T) {
	fc := &fakeCompliance{
		byAddr:    domain.Account{ID: uuid.New(), Address: "sue@acme.eu"},
		searchRet: []domain.EDiscoveryHit{{Message: domain.Message{BlobKey: "k1"}}},
		auditErr:  context.DeadlineExceeded,
	}
	a := complianceAPI(fc, &fakeBlobs{getBody: "x"})
	rec := httptest.NewRecorder()
	a.ediscoveryExport(rec, adminReq(http.MethodGet, "/x?email=sue@acme.eu", "", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("auditoría fallida => %d, quiero 500 (sin stream)", rec.Code)
	}
}

// --- mbox writer (puro) ---------------------------------------------------

func TestWriteMboxEntryQuoting(t *testing.T) {
	var b strings.Builder
	raw := []byte("Header: v\nFrom the boss\n>From ya escapado\nnormal\n")
	if err := writeMboxEntry(&b, "sender@x.eu", time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC), raw); err != nil {
		t.Fatalf("writeMboxEntry: %v", err)
	}
	out := b.String()
	if !strings.HasPrefix(out, "From sender@x.eu ") {
		t.Fatalf("separador From_ ausente: %q", out)
	}
	if !strings.Contains(out, "\n>From the boss\n") {
		t.Fatalf("no escapó 'From ' del cuerpo: %q", out)
	}
	if !strings.Contains(out, "\n>>From ya escapado\n") {
		t.Fatalf("no escapó '>From ' (mboxrd): %q", out)
	}
	if !strings.Contains(out, "\nnormal\n") {
		t.Fatalf("línea normal alterada: %q", out)
	}
}
