// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactshttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
)

// pngDataURI es un PNG 1x1 válido (los magic bytes hacen que el sniff lo reconozca).
const pngDataURI = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

// --- Lote --------------------------------------------------------------------

func TestBatchTrashOK(t *testing.T) {
	f := &fakeStore{batchN: 2}
	rec := httptest.NewRecorder()
	newHandlers(f).BatchTrash(rec, req(http.MethodPost, "/", `{"uids":["a","b"]}`, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if decode(t, rec)["trashed"].(float64) != 2 {
		t.Errorf("trashed != 2: %v", rec.Body.String())
	}
	if len(f.gotBatchUIDs) != 2 {
		t.Errorf("uids no propagados: %v", f.gotBatchUIDs)
	}
}

func TestBatchTrashEmptyRejected(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeStore{}).BatchTrash(rec, req(http.MethodPost, "/", `{"uids":[]}`, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestBatchFavoriteOK(t *testing.T) {
	f := &fakeStore{batchN: 3}
	rec := httptest.NewRecorder()
	newHandlers(f).BatchFavorite(rec, req(http.MethodPost, "/", `{"uids":["a","b","c"],"favorite":true}`, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if !f.gotBatchOn {
		t.Error("favorite=true no propagado")
	}
}

func TestBatchGroupMembersOK(t *testing.T) {
	f := &fakeStore{group: domain.Group{Name: "Familia", ETag: uuid.New()}}
	rec := httptest.NewRecorder()
	newHandlers(f).BatchGroupMembers(rec, req(http.MethodPost, "/", `{"uids":["a"],"add":true}`, id()))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if !f.gotBatchAdd {
		t.Error("add=true no propagado")
	}
	if rec.Header().Get("ETag") == "" {
		t.Error("falta ETag del grupo")
	}
}

func TestBatchGroupMembersBadID(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeStore{}).BatchGroupMembers(rec, req(http.MethodPost, "/", `{"uids":["a"]}`, map[string]string{"id": "nope"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

// --- Foto --------------------------------------------------------------------

func TestSetPhotoOK(t *testing.T) {
	f := &fakeStore{}
	rec := httptest.NewRecorder()
	newHandlers(f).SetPhoto(rec, req(http.MethodPut, "/", `{"uid":"u1","data":"`+pngDataURI+`"}`, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.HasPrefix(f.gotPhotoData, "data:image/png;base64,") {
		t.Errorf("data URI no normalizado: %q", f.gotPhotoData)
	}
}

func TestSetPhotoRejectsSVG(t *testing.T) {
	rec := httptest.NewRecorder()
	// SVG (vectorial con posible script) NO está en la lista blanca raster.
	svg := "data:image/svg+xml;base64,PHN2Zz48L3N2Zz4="
	newHandlers(&fakeStore{}).SetPhoto(rec, req(http.MethodPut, "/", `{"uid":"u1","data":"`+svg+`"}`, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400 (SVG rechazado)", rec.Code)
	}
}

func TestSetPhotoRejectsTooLarge(t *testing.T) {
	rec := httptest.NewRecorder()
	// ~300 KiB decodificados (> 256 KiB): "AAAA" -> 3 bytes cero cada 4 chars.
	big := "data:image/png;base64," + strings.Repeat("AAAA", 100000)
	newHandlers(&fakeStore{}).SetPhoto(rec, req(http.MethodPut, "/", `{"uid":"u1","data":"`+big+`"}`, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400 (tamaño rechazado)", rec.Code)
	}
}

func TestSetPhotoMissingUID(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeStore{}).SetPhoto(rec, req(http.MethodPut, "/", `{"data":"`+pngDataURI+`"}`, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestGetPhotoServesBytes(t *testing.T) {
	f := &fakeStore{photoDataURI: pngDataURI, photoEtag: uuid.New()}
	rec := httptest.NewRecorder()
	newHandlers(f).GetPhoto(rec, req(http.MethodGet, "/?uid=u1", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("content-type = %q, want image/png", ct)
	}
	if rec.Header().Get("ETag") == "" {
		t.Error("falta ETag")
	}
	if rec.Body.Len() == 0 {
		t.Error("cuerpo de imagen vacío")
	}
}

func TestGetPhotoNotFound(t *testing.T) {
	f := &fakeStore{photoErr: domain.ErrNotFound}
	rec := httptest.NewRecorder()
	newHandlers(f).GetPhoto(rec, req(http.MethodGet, "/?uid=u1", "", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestGetPhotoConditional304(t *testing.T) {
	etag := uuid.New()
	f := &fakeStore{photoDataURI: pngDataURI, photoEtag: etag}
	r := req(http.MethodGet, "/?uid=u1", "", nil)
	r.Header.Set("If-None-Match", `"`+etag.String()+`"`)
	rec := httptest.NewRecorder()
	newHandlers(f).GetPhoto(rec, r)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("code = %d, want 304", rec.Code)
	}
}

func TestRemovePhotoOK(t *testing.T) {
	f := &fakeStore{}
	rec := httptest.NewRecorder()
	newHandlers(f).RemovePhoto(rec, req(http.MethodDelete, "/?uid=u1", "", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
	if f.gotPhotoUID != "u1" {
		t.Errorf("uid no propagado: %q", f.gotPhotoUID)
	}
}

func TestRemovePhotoMissingUID(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeStore{}).RemovePhoto(rec, req(http.MethodDelete, "/", "", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}
