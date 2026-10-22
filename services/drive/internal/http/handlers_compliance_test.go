// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// fakeCompliance implementa complianceStore para los tests de handlers (sin PG).
type fakeCompliance struct {
	hold        domain.LegalHold
	holds       []domain.LegalHold
	hits        []domain.EDiscoveryHit
	releaseErr  error
	auditErr    error
	auditLog    []domain.AuditEntry
	auditLogErr error
	lastSubject string           // último subject buscado
	audits      []auditCall      // trazas escritas
	createArg   domain.LegalHold // último hold creado
}

type auditCall struct {
	actor, action, target string
}

func (f *fakeCompliance) CreateHold(_ context.Context, h domain.LegalHold) (domain.LegalHold, error) {
	f.createArg = h
	out := f.hold
	out.Subject = h.Subject
	out.CreatedBy = h.CreatedBy
	return out, nil
}

func (f *fakeCompliance) ListHolds(_ context.Context, _ bool) ([]domain.LegalHold, error) {
	return f.holds, nil
}

func (f *fakeCompliance) ReleaseHold(_ context.Context, _ uuid.UUID, _ string) error {
	return f.releaseErr
}

func (f *fakeCompliance) EDiscoverySearch(_ context.Context, subject string, _ store.EDiscoveryFilter) ([]domain.EDiscoveryHit, error) {
	f.lastSubject = subject
	return f.hits, nil
}

func (f *fakeCompliance) WriteAudit(_ context.Context, actor, action, target string, _ any) error {
	if f.auditErr != nil {
		return f.auditErr
	}
	f.audits = append(f.audits, auditCall{actor, action, target})
	return nil
}

func (f *fakeCompliance) ListAudit(_ context.Context, _ int) ([]domain.AuditEntry, error) {
	return f.auditLog, f.auditLogErr
}

// fakeBlobs implementa blobGetter: devuelve un contenido fijo por clave.
type fakeBlobs struct {
	data map[string]string
	err  error
}

func (b fakeBlobs) Get(_ context.Context, _ tenancy.Tenant, key string) (io.ReadCloser, error) {
	if b.err != nil {
		return nil, b.err
	}
	return io.NopCloser(strings.NewReader(b.data[key])), nil
}

// adminReq construye una petición con claims de admin (rol) y tenancy fijados.
func adminReq(method, target, body string, params map[string]string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, target, http.NoBody)
	}
	ctx := authn.Into(r.Context(), authn.Claims{Subject: "admin-1", Tenant: "acme", Roles: []string{"admin"}})
	ctx = tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"})
	r = r.WithContext(ctx)
	if len(params) > 0 {
		r = withParams(r, params)
	}
	return r
}

// TestComplianceRequiresAdminRole: sin rol de compliance, 403 (defensa en
// profundidad además del middleware del montaje).
func TestComplianceRequiresAdminRole(t *testing.T) {
	a := &complianceHandlers{store: &fakeCompliance{}}
	// Petición autenticada pero SIN rol admin/owner.
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"subject":"u"}`))
	r = r.WithContext(authn.Into(r.Context(), authn.Claims{Subject: "u", Tenant: "acme", Roles: []string{"member"}}))
	rec := httptest.NewRecorder()
	a.createHold(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("createHold sin rol => %d, quiero 403", rec.Code)
	}
}

// TestCreateHoldValidation: sin subject y sin org_wide => 400.
func TestCreateHoldValidation(t *testing.T) {
	a := &complianceHandlers{store: &fakeCompliance{}}
	rec := httptest.NewRecorder()
	a.createHold(rec, adminReq(http.MethodPost, "/x", `{"reason":"litigio"}`, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("createHold sin objetivo => %d, quiero 400", rec.Code)
	}
}

// TestCreateHoldOK: crea un hold por subject (201) y propaga el actor.
func TestCreateHoldOK(t *testing.T) {
	fake := &fakeCompliance{hold: domain.LegalHold{ID: uuid.New()}}
	a := &complianceHandlers{store: fake}
	rec := httptest.NewRecorder()
	a.createHold(rec, adminReq(http.MethodPost, "/x", `{"subject":"alice","case":"C1"}`, nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("createHold => %d, quiero 201 (%s)", rec.Code, rec.Body.String())
	}
	if fake.createArg.Subject != "alice" || fake.createArg.CreatedBy != "admin-1" || fake.createArg.Case != "C1" {
		t.Fatalf("hold creado inesperado: %+v", fake.createArg)
	}
}

// TestCreateHoldOrgWide: org_wide crea un hold con subject vacío (toda la org).
func TestCreateHoldOrgWide(t *testing.T) {
	fake := &fakeCompliance{hold: domain.LegalHold{ID: uuid.New()}}
	a := &complianceHandlers{store: fake}
	rec := httptest.NewRecorder()
	a.createHold(rec, adminReq(http.MethodPost, "/x", `{"org_wide":true,"subject":"ignorado"}`, nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("createHold org_wide => %d, quiero 201", rec.Code)
	}
	if fake.createArg.Subject != "" {
		t.Fatalf("org_wide debe crear hold sin subject, got %q", fake.createArg.Subject)
	}
}

// TestReleaseHoldNotFound: un hold inexistente/ya liberado => 404.
func TestReleaseHoldNotFound(t *testing.T) {
	a := &complianceHandlers{store: &fakeCompliance{releaseErr: domain.ErrNotFound}}
	rec := httptest.NewRecorder()
	a.releaseHold(rec, adminReq(http.MethodPost, "/x", "", map[string]string{"holdID": uuid.New().String()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("releaseHold inexistente => %d, quiero 404", rec.Code)
	}
}

// TestEDiscoverySearchMetaAndAudit: la búsqueda devuelve metadatos con la marca
// Preserved y AUDITA (fail-closed OK).
func TestEDiscoverySearchMetaAndAudit(t *testing.T) {
	fake := &fakeCompliance{hits: []domain.EDiscoveryHit{
		{File: domain.File{ID: uuid.New(), Name: "vivo.pdf", ContentType: "application/pdf"}, Preserved: false},
		{File: domain.File{ID: uuid.New(), Name: "borrado.pdf"}, Preserved: true},
	}}
	a := &complianceHandlers{store: fake}
	rec := httptest.NewRecorder()
	a.ediscoverySearch(rec, adminReq(http.MethodPost, "/x", `{"subject":"alice","name":"pdf","case":"C2"}`, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("search => %d, quiero 200 (%s)", rec.Code, rec.Body.String())
	}
	if fake.lastSubject != "alice" {
		t.Fatalf("subject buscado = %q, quiero alice", fake.lastSubject)
	}
	if !strings.Contains(rec.Body.String(), `"preserved":true`) {
		t.Fatalf("la respuesta debe marcar Preserved: %s", rec.Body.String())
	}
	// Auditó la búsqueda (fail-closed satisfecho).
	if len(fake.audits) != 1 || fake.audits[0].action != "ediscovery.search" || fake.audits[0].target != "alice" {
		t.Fatalf("auditoría de búsqueda inesperada: %+v", fake.audits)
	}
}

// TestEDiscoverySearchFailClosed: si la auditoría falla, NO se sirve el resultado
// (500) — el acceso privilegiado siempre queda trazado.
func TestEDiscoverySearchFailClosed(t *testing.T) {
	fake := &fakeCompliance{
		hits:     []domain.EDiscoveryHit{{File: domain.File{ID: uuid.New(), Name: "x"}}},
		auditErr: errors.New("db caída"),
	}
	a := &complianceHandlers{store: fake}
	rec := httptest.NewRecorder()
	a.ediscoverySearch(rec, adminReq(http.MethodPost, "/x", `{"subject":"alice"}`, nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("search con auditoría fallida => %d, quiero 500", rec.Code)
	}
}

// TestEDiscoveryExportZip: el export empaqueta el contenido de cada fichero (desde
// «S3») en un ZIP y audita.
func TestEDiscoveryExportZip(t *testing.T) {
	id1, id2 := uuid.New(), uuid.New()
	fake := &fakeCompliance{hits: []domain.EDiscoveryHit{
		{File: domain.File{ID: id1, Name: "informe.pdf", StorageKey: "k1"}},
		{File: domain.File{ID: id2, Name: "borrado.pdf", StorageKey: "k2"}, Preserved: true},
	}}
	blobs := fakeBlobs{data: map[string]string{"k1": "PDF-UNO", "k2": "PDF-DOS"}}
	a := &complianceHandlers{store: fake, blobs: blobs}
	rec := httptest.NewRecorder()
	a.ediscoveryExport(rec, adminReq(http.MethodGet, "/x?subject=alice&case=C3", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("export => %d, quiero 200 (%s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("Content-Type = %q, quiero application/zip", ct)
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("el cuerpo no es un ZIP válido: %v", err)
	}
	got := map[string]string{}
	for _, zf := range zr.File {
		rc, _ := zf.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		got[zf.Name] = string(b)
	}
	if len(got) != 2 {
		t.Fatalf("entradas ZIP = %d, quiero 2 (%v)", len(got), got)
	}
	if got["informe.pdf"] != "PDF-UNO" {
		t.Fatalf("contenido de informe.pdf = %q", got["informe.pdf"])
	}
	// El preservado se prefija con deleted/ para el investigador.
	if got["deleted/borrado.pdf"] != "PDF-DOS" {
		t.Fatalf("el preservado debe ir en deleted/: %v", got)
	}
	if len(fake.audits) != 1 || fake.audits[0].action != "ediscovery.export" {
		t.Fatalf("auditoría de export inesperada: %+v", fake.audits)
	}
}

// TestEDiscoveryExportNoBlobs: sin objectstore configurado, 500 controlado (no
// stream a medias).
func TestEDiscoveryExportNoBlobs(t *testing.T) {
	a := &complianceHandlers{store: &fakeCompliance{}, blobs: nil}
	rec := httptest.NewRecorder()
	a.ediscoveryExport(rec, adminReq(http.MethodGet, "/x?subject=alice", "", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("export sin S3 => %d, quiero 500", rec.Code)
	}
}
