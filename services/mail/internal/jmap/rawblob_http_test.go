// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// Un mensaje que no cabe en el JSON de 1 MiB se sube antes por /jmap/upload y se envía
// por referencia (`raw_blob_id`): el servicio lo lee de SU cuenta, lo encola tal cual
// y borra el blob de subida. Un blob de otra cuenta no se envía.
func TestSendMessageConRawSubido(t *testing.T) {
	acc := domain.Account{ID: uuid.New(), Address: "ada@sytue.eu"}
	raw := "From: ada@sytue.eu\r\nTo: bob@x.com\r\nSubject: grande\r\n\r\n" + strings.Repeat("A", 500)
	blobs := &fakeBlobs{getBody: raw}
	out := &fakeOutbound{}
	a := newAPI(&fakeStore{acc: acc}, blobs, out)
	clave := "blobs/" + acc.ID.String() + "/" + uuid.NewString()
	rec := httptest.NewRecorder()
	a.sendMessage(rec, req(http.MethodPost, "/api/v1/send", `{"recipients":["bob@x.com"],"raw_blob_id":`+strconv.Quote(clave)+`}`, true, nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if string(out.lastRaw) != raw {
		t.Fatalf("encolado %d bytes, quiero el blob entero (%d)", len(out.lastRaw), len(raw))
	}
	if blobs.deletedKey != clave {
		t.Errorf("el blob de subida no se borra: %q", blobs.deletedKey)
	}

	ajeno := "blobs/" + uuid.NewString() + "/" + uuid.NewString()
	out.lastRaw = nil
	rec = httptest.NewRecorder()
	a.sendMessage(rec, req(http.MethodPost, "/api/v1/send", `{"recipients":["bob@x.com"],"raw_blob_id":`+strconv.Quote(ajeno)+`}`, true, nil))
	if rec.Code != http.StatusBadRequest || out.lastRaw != nil {
		t.Errorf("un blob de otra cuenta: código %d, encolado %v", rec.Code, out.lastRaw != nil)
	}
	rec = httptest.NewRecorder()
	a.sendMessage(rec, req(http.MethodPost, "/api/v1/send", `{"recipients":["bob@x.com"],"raw_blob_id":"blobs/../messages/x"}`, true, nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("una clave con rutas relativas: código %d", rec.Code)
	}
}

// Un borrador grande entra igual: por referencia.
func TestCreateDraftConRawSubido(t *testing.T) {
	st := &fakeStore{
		acc:    domain.Account{ID: uuid.New(), Address: "ada@sytue.eu"},
		byRole: domain.Mailbox{ID: uuid.New(), Role: domain.RoleDrafts},
	}
	blobs := &fakeBlobs{getBody: "From: ada@sytue.eu\r\nSubject: a medias\r\n\r\ncuerpo"}
	a := newAPI(st, blobs, nil)
	clave := "blobs/" + st.acc.ID.String() + "/" + uuid.NewString()
	rec := httptest.NewRecorder()
	a.createDraft(rec, req(http.MethodPost, "/api/v1/drafts", `{"recipients":[],"raw_blob_id":`+strconv.Quote(clave)+`}`, true, nil))
	if rec.Code != http.StatusCreated || st.lastInsert == nil || st.lastInsert.Subject != "a medias" {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if blobs.deletedKey != clave {
		t.Errorf("el blob de subida no se borra: %q", blobs.deletedKey)
	}
}
