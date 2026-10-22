// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests HTTP de las acciones en LOTE (multiselección) y del delta incremental
// (changes) con el fake del store: propagación de ids/destino/flag, traducción de
// keyword, validación de entrada, mapeo de errores y el sobre JSON de respuesta.
package jmap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// --- batchMove -----------------------------------------------------------

func TestBatchMoveOK(t *testing.T) {
	dest := uuid.New()
	id1, id2 := uuid.New(), uuid.New()
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	body := `{"ids":["` + id1.String() + `","` + id2.String() + `"],"mailbox_id":"` + dest.String() + `"}`
	a.batchMove(rec, req(http.MethodPost, "/api/v1/messages/batch/move", body, true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d, quiero 200 (%s)", rec.Code, rec.Body.String())
	}
	if st.lastBatchDest != dest {
		t.Fatalf("destino = %v, quiero %v", st.lastBatchDest, dest)
	}
	if len(st.lastBatchIDs) != 2 || st.lastBatchIDs[0] != id1 || st.lastBatchIDs[1] != id2 {
		t.Fatalf("ids propagados = %v", st.lastBatchIDs)
	}
	m := decodeBody(t, rec)
	if m["moved"] != float64(2) {
		t.Fatalf("moved = %v, quiero 2", m["moved"])
	}
}

func TestBatchMoveBadDest(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.batchMove(rec, req(http.MethodPost, "/x", `{"ids":["`+uuid.NewString()+`"],"mailbox_id":"nope"}`, true, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("mailbox_id inválido => %d, quiero 400", rec.Code)
	}
}

// Un id mal formado en la lista es un error de cliente (400); no debe llegar al
// store. (Un id bien formado pero ajeno lo omite el store, no se rechaza aquí.)
func TestBatchMoveBadID(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	body := `{"ids":["not-a-uuid"],"mailbox_id":"` + uuid.NewString() + `"}`
	a.batchMove(rec, req(http.MethodPost, "/x", body, true, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("id inválido => %d, quiero 400", rec.Code)
	}
	if st.lastBatchIDs != nil {
		t.Fatalf("no debe llegar al store con ids inválidos: %v", st.lastBatchIDs)
	}
}

func TestBatchMoveNoAuth(t *testing.T) {
	a := newAPI(&fakeStore{}, nil, nil)
	rec := httptest.NewRecorder()
	a.batchMove(rec, noAuthReq(http.MethodPost, "/x", `{"ids":[],"mailbox_id":"`+uuid.NewString()+`"}`))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin auth => %d, quiero 401", rec.Code)
	}
}

// Un destino ajeno (el store devuelve ErrNotFound) se mapea a 404, sin mover nada.
func TestBatchMoveDestNotOwned(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, batchErr: domain.ErrNotFound}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	body := `{"ids":["` + uuid.NewString() + `"],"mailbox_id":"` + uuid.NewString() + `"}`
	a.batchMove(rec, req(http.MethodPost, "/x", body, true, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("destino ajeno => %d, quiero 404", rec.Code)
	}
}

// --- batchFlags ----------------------------------------------------------

func TestBatchFlagsTranslatesKeyword(t *testing.T) {
	id := uuid.New()
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	// El cliente envía el keyword JMAP $seen; el store recibe el flag IMAP \Seen.
	a.batchFlags(rec, req(http.MethodPost, "/x", `{"ids":["`+id.String()+`"],"flag":"$seen","add":true}`, true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d, quiero 200 (%s)", rec.Code, rec.Body.String())
	}
	if st.lastBatchFlag != domain.FlagSeen || !st.lastBatchAdd {
		t.Fatalf("flag/add = %q/%v, quiero \\Seen/true", st.lastBatchFlag, st.lastBatchAdd)
	}
	m := decodeBody(t, rec)
	if m["updated"] != float64(1) {
		t.Fatalf("updated = %v, quiero 1", m["updated"])
	}
}

func TestBatchFlagsMissingFlag(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.batchFlags(rec, req(http.MethodPost, "/x", `{"ids":["`+uuid.NewString()+`"],"add":true}`, true, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("flag ausente => %d, quiero 400", rec.Code)
	}
}

func TestBatchFlagsBadID(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.batchFlags(rec, req(http.MethodPost, "/x", `{"ids":["bad"],"flag":"$seen","add":false}`, true, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("id inválido => %d, quiero 400", rec.Code)
	}
}

// --- changes -------------------------------------------------------------

func TestChangesOK(t *testing.T) {
	ts := time.Date(2026, 7, 20, 10, 0, 0, 500000000, time.UTC)
	tsID := uuid.New()
	destroyed := uuid.New()
	st := &fakeStore{
		acc:          domain.Account{ID: uuid.New()},
		changedRet:   []domain.Message{{ID: uuid.New(), Subject: "nuevo"}},
		destroyedRet: []uuid.UUID{destroyed},
		changesRetTS: ts,
		changesRetID: tsID,
	}
	a := newAPI(st, nil, nil)
	// since = cursor de un estado previo; el store lo recibe parseado.
	since := formatCursor(time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC), uuid.New())
	rec := httptest.NewRecorder()
	a.changes(rec, req(http.MethodGet, "/api/v1/changes?since="+since, "", true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d, quiero 200 (%s)", rec.Code, rec.Body.String())
	}
	if st.lastSince.IsZero() {
		t.Fatalf("since no propagado al store")
	}
	var got struct {
		Changed   []domain.Message `json:"changed"`
		Destroyed []string         `json:"destroyed"`
		State     string           `json:"state"`
		HasMore   bool             `json:"has_more"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Changed) != 1 || len(got.Destroyed) != 1 || got.Destroyed[0] != destroyed.String() {
		t.Fatalf("changed/destroyed = %+v / %+v", got.Changed, got.Destroyed)
	}
	// El state devuelto es el nuevo cursor (updated_at,id) y hace round-trip.
	if got.State != formatCursor(ts, tsID) {
		t.Fatalf("state = %q, quiero %q", got.State, formatCursor(ts, tsID))
	}
}

// Sin ?since (arranque): el store recibe un cursor cero (bootstrap).
func TestChangesBootstrapNoSince(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.changes(rec, req(http.MethodGet, "/api/v1/changes", "", true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d, quiero 200", rec.Code)
	}
	if !st.lastSince.IsZero() || st.lastSinceID != uuid.Nil {
		t.Fatalf("bootstrap debe pasar cursor cero, got %v/%v", st.lastSince, st.lastSinceID)
	}
}

func TestChangesBadSince(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.changes(rec, req(http.MethodGet, "/api/v1/changes?since=garbage", "", true, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("since inválido => %d, quiero 400", rec.Code)
	}
}

func TestChangesNoAuth(t *testing.T) {
	a := newAPI(&fakeStore{}, nil, nil)
	rec := httptest.NewRecorder()
	a.changes(rec, noAuthReq(http.MethodGet, "/api/v1/changes", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin auth => %d, quiero 401", rec.Code)
	}
}
