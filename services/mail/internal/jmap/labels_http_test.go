// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

func TestListLabelsOK(t *testing.T) {
	st := &fakeStore{
		acc:    domain.Account{ID: uuid.New()},
		labels: []domain.Label{{ID: uuid.New(), Name: "Trabajo", Color: "brass", UnreadCount: 3}},
	}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.listLabels(rec, req(http.MethodGet, "/api/v1/labels", "", true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	assertJSONContentType(t, rec)
	var got struct {
		Labels []domain.Label `json:"labels"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Labels) != 1 || got.Labels[0].Name != "Trabajo" || got.Labels[0].UnreadCount != 3 {
		t.Fatalf("labels = %+v", got.Labels)
	}
}

func TestCreateLabelOK(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.createLabel(rec, req(http.MethodPost, "/api/v1/labels", `{"name":"Facturas","color":"slate"}`, true, nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d, quiero 201 (%s)", rec.Code, rec.Body.String())
	}
	if st.lastLabelName != "Facturas" || st.lastLabelColor != "slate" {
		t.Fatalf("no pasó name/color al store: %q %q", st.lastLabelName, st.lastLabelColor)
	}
}

func TestCreateLabelDuplicate(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, createLErr: domain.ErrLabelExists}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.createLabel(rec, req(http.MethodPost, "/api/v1/labels", `{"name":"Trabajo"}`, true, nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicada => %d, quiero 409", rec.Code)
	}
}

func TestCreateLabelInvalid(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, createLErr: domain.ErrInvalidAddress}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.createLabel(rec, req(http.MethodPost, "/api/v1/labels", `{"name":"","color":"neon"}`, true, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("inválida => %d, quiero 400", rec.Code)
	}
}

func TestUpdateLabelOK(t *testing.T) {
	id := uuid.New()
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.updateLabel(rec, req(http.MethodPut, "/api/v1/labels/"+id.String(),
		`{"name":"Nuevo","color":"teal"}`, true, map[string]string{"labelID": id.String()}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204 (%s)", rec.Code, rec.Body.String())
	}
	if st.lastLabelID != id || st.lastLabelName != "Nuevo" || st.lastLabelColor != "teal" {
		t.Fatalf("update no propagó args: %v %q %q", st.lastLabelID, st.lastLabelName, st.lastLabelColor)
	}
}

func TestUpdateLabelBadID(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.updateLabel(rec, req(http.MethodPut, "/api/v1/labels/nope", `{"name":"x"}`, true,
		map[string]string{"labelID": "nope"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("id inválido => %d, quiero 400", rec.Code)
	}
}

func TestDeleteLabelOK(t *testing.T) {
	id := uuid.New()
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.deleteLabel(rec, req(http.MethodDelete, "/api/v1/labels/"+id.String(), "", true,
		map[string]string{"labelID": id.String()}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204", rec.Code)
	}
	if st.lastLabelID != id {
		t.Fatalf("delete no propagó id: %v", st.lastLabelID)
	}
}

func TestApplyLabelBatch(t *testing.T) {
	id := uuid.New()
	m1, m2 := uuid.New(), uuid.New()
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	body := `{"message_ids":["` + m1.String() + `","` + m2.String() + `"],"add":true}`
	a.applyLabel(rec, req(http.MethodPost, "/api/v1/labels/"+id.String()+"/apply", body, true,
		map[string]string{"labelID": id.String()}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204 (%s)", rec.Code, rec.Body.String())
	}
	if st.lastLabelID != id || len(st.lastApplyIDs) != 2 || !st.lastApplyAdd {
		t.Fatalf("apply no propagó lote: id=%v ids=%v add=%v", st.lastLabelID, st.lastApplyIDs, st.lastApplyAdd)
	}
}

func TestApplyLabelBadMessageID(t *testing.T) {
	id := uuid.New()
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.applyLabel(rec, req(http.MethodPost, "/api/v1/labels/"+id.String()+"/apply",
		`{"message_ids":["not-a-uuid"],"add":true}`, true, map[string]string{"labelID": id.String()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("message_id inválido => %d, quiero 400", rec.Code)
	}
}

func TestApplyLabelNotOwned(t *testing.T) {
	id := uuid.New()
	m1 := uuid.New()
	// El store devuelve ErrNotFound cuando la etiqueta no es de la cuenta.
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, applyLErr: domain.ErrNotFound}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.applyLabel(rec, req(http.MethodPost, "/api/v1/labels/"+id.String()+"/apply",
		`{"message_ids":["`+m1.String()+`"],"add":true}`, true, map[string]string{"labelID": id.String()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("etiqueta ajena => %d, quiero 404", rec.Code)
	}
}

func TestListLabelMessagesKeyset(t *testing.T) {
	id := uuid.New()
	st := &fakeStore{
		acc:       domain.Account{ID: uuid.New()},
		labelMsgs: []domain.Message{{ID: uuid.New(), Subject: "hola"}},
	}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.listLabelMessages(rec, req(http.MethodGet, "/api/v1/labels/"+id.String()+"/messages?limit=1", "",
		true, map[string]string{"labelID": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if st.lastLabelID != id {
		t.Fatalf("no pasó labelID al store: %v", st.lastLabelID)
	}
	var got struct {
		Messages   []domain.Message `json:"messages"`
		NextCursor string           `json:"next_cursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// len(msgs)==limit(1) ⇒ hay next_cursor.
	if len(got.Messages) != 1 || got.NextCursor == "" {
		t.Fatalf("respuesta = %+v", got)
	}
}
