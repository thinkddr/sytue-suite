// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactshttp

import (
	"errors"
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

func TestBatchRestoreOK(t *testing.T) {
	f := &fakeStore{batchN: 2}
	rec := httptest.NewRecorder()
	newHandlers(f).BatchRestore(rec, req(http.MethodPost, "/", `{"uids":["a","b","c"]}`, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if decode(t, rec)["restored"].(float64) != 2 {
		t.Errorf("restored != 2: %v", rec.Body.String())
	}
	if len(f.gotBatchUIDs) != 3 {
		t.Errorf("uids no propagados: %v", f.gotBatchUIDs)
	}
}

func TestBatchRestoreEmptyRejected(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeStore{}).BatchRestore(rec, req(http.MethodPost, "/", `{"uids":[]}`, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

// TestBatchVCardUnaRespuestaConTodas: la exportación de una selección es UNA respuesta
// `text/vcard` con las tarjetas guardadas, en el orden pedido y por id de contacto.
func TestBatchVCardUnaRespuestaConTodas(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	f := &fakeStore{batchCards: []string{"BEGIN:VCARD\r\nFN:Ana\r\nEND:VCARD\r\n", "BEGIN:VCARD\r\nFN:Luis\r\nEND:VCARD"}}
	rec := httptest.NewRecorder()
	newHandlers(f).BatchVCard(rec, req(http.MethodPost, "/", `{"ids":["`+a.String()+`"," `+b.String()+`"]}`, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/vcard") {
		t.Errorf("Content-Type = %q, want text/vcard", ct)
	}
	quiero := "BEGIN:VCARD\r\nFN:Ana\r\nEND:VCARD\r\nBEGIN:VCARD\r\nFN:Luis\r\nEND:VCARD\r\n"
	if rec.Body.String() != quiero {
		t.Errorf("cuerpo = %q, want %q", rec.Body.String(), quiero)
	}
	if len(f.gotBatchIDs) != 2 || f.gotBatchIDs[0] != a || f.gotBatchIDs[1] != b {
		t.Errorf("ids no propagados en orden: %v", f.gotBatchIDs)
	}
}

// TestBatchVCardEscribeSegunLee: cada tarjeta sale a la respuesta antes de leer la
// siguiente. Juntarlas todas para una sola escritura tiraba el pod por OOM al exportar
// mil contactos con foto.
func TestBatchVCardEscribeSegunLee(t *testing.T) {
	rec := httptest.NewRecorder()
	cards := []string{"BEGIN:VCARD\r\nFN:Ana\r\nEND:VCARD", "BEGIN:VCARD\r\nFN:Luis\r\nEND:VCARD", "BEGIN:VCARD\r\nFN:Eva\r\nEND:VCARD"}
	f := &fakeStore{batchCards: cards}
	f.antesDeCard = func(i int) {
		var ya strings.Builder
		for _, c := range cards[:i] {
			ya.WriteString(c + "\r\n")
		}
		if rec.Body.String() != ya.String() {
			t.Errorf("antes de leer la tarjeta %d el cuerpo es %q, want %q", i, rec.Body.String(), ya.String())
		}
	}
	newHandlers(f).BatchVCard(rec, req(http.MethodPost, "/", `{"ids":["`+uuid.NewString()+`"]}`, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
}

// TestBatchVCardFalloAMitadCortaLaDescarga: si la lectura falla con media exportación
// ya enviada, se aborta la conexión (no se cierra bien un .vcf al que le faltan
// tarjetas); si falla antes de la primera, es un error normal.
func TestBatchVCardFalloAMitadCortaLaDescarga(t *testing.T) {
	cuerpo := `{"ids":["` + uuid.NewString() + `"]}`
	rec := httptest.NewRecorder()
	newHandlers(&fakeStore{batchErr: domain.ErrConflict}).BatchVCard(rec, req(http.MethodPost, "/", cuerpo, nil))
	if rec.Code == http.StatusOK {
		t.Errorf("fallo antes de la primera tarjeta: code = 200, want un error")
	}

	defer func() {
		if r, _ := recover().(error); !errors.Is(r, http.ErrAbortHandler) {
			t.Errorf("fallo a mitad: recover() = %v, want http.ErrAbortHandler", r)
		}
	}()
	f := &fakeStore{batchCards: []string{"BEGIN:VCARD\r\nFN:Ana\r\nEND:VCARD"}, batchErr: domain.ErrConflict}
	newHandlers(f).BatchVCard(httptest.NewRecorder(), req(http.MethodPost, "/", cuerpo, nil))
}

// TestBatchVCardNingunaVivaEsUnVcfVacio: si ninguna sigue viva, 200 sin cuerpo (como el
// resto de los lotes); el aviso lo da quien exporta.
func TestBatchVCardNingunaVivaEsUnVcfVacio(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeStore{}).BatchVCard(rec, req(http.MethodPost, "/", `{"ids":["`+uuid.NewString()+`"]}`, nil))
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Errorf("code = %d, cuerpo %q; want 200 vacío", rec.Code, rec.Body.String())
	}
}

func TestBatchVCardRechazaVacioOIdMalo(t *testing.T) {
	for _, cuerpo := range []string{`{"ids":[]}`, `{"ids":["no-es-un-id"]}`} {
		rec := httptest.NewRecorder()
		newHandlers(&fakeStore{}).BatchVCard(rec, req(http.MethodPost, "/", cuerpo, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: code = %d, want 400", cuerpo, rec.Code)
		}
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
