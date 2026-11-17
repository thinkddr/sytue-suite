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

func TestGetUndoSendDevuelveLaDeLaCuenta(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, undoSend: 20}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.getUndoSend(rec, req(http.MethodGet, "/api/v1/undo-send", "", true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	assertJSONContentType(t, rec)
	var got struct {
		Seconds int `json:"seconds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Seconds != 20 {
		t.Fatalf("seconds = %d, quiero 20", got.Seconds)
	}
}

func TestSetUndoSendAceptaLasCuatroOpciones(t *testing.T) {
	for _, secs := range []int{5, 10, 20, 30} {
		st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
		a := newAPI(st, nil, nil)
		rec := httptest.NewRecorder()
		body, _ := json.Marshal(map[string]int{"seconds": secs})
		a.setUndoSend(rec, req(http.MethodPut, "/api/v1/undo-send", string(body), true, nil))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%d s: código = %d, quiero 204 (%s)", secs, rec.Code, rec.Body.String())
		}
		if st.lastUndoSend != secs {
			t.Fatalf("%d s: llegó al store %d", secs, st.lastUndoSend)
		}
	}
}

// Fuera de la lista se RECHAZA, sin redondear: guardar otra cosa y contestar 204
// dejaría al cliente enseñando una ventana que no es la guardada.
func TestSetUndoSendRechazaLoQueNoEsUnaOpcion(t *testing.T) {
	for _, cuerpo := range []string{`{"seconds":0}`, `{"seconds":7}`, `{"seconds":31}`, `{"seconds":-5}`, `{}`} {
		st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
		a := newAPI(st, nil, nil)
		rec := httptest.NewRecorder()
		a.setUndoSend(rec, req(http.MethodPut, "/api/v1/undo-send", cuerpo, true, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: código = %d, quiero 400", cuerpo, rec.Code)
		}
		if st.lastUndoSend != 0 {
			t.Fatalf("%s: un valor rechazado no puede llegar al store (%d)", cuerpo, st.lastUndoSend)
		}
	}
}

func TestUndoSendCuentaAjenaEs404(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, getUndoErr: domain.ErrNotFound, setUndoErr: domain.ErrNotFound}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.getUndoSend(rec, req(http.MethodGet, "/api/v1/undo-send", "", true, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get: código = %d, quiero 404", rec.Code)
	}
	rec = httptest.NewRecorder()
	a.setUndoSend(rec, req(http.MethodPut, "/api/v1/undo-send", `{"seconds":10}`, true, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("put: código = %d, quiero 404", rec.Code)
	}
}

func TestUndoSendSinAuth(t *testing.T) {
	a := newAPI(&fakeStore{}, nil, nil)
	rec := httptest.NewRecorder()
	a.getUndoSend(rec, noAuthReq(http.MethodGet, "/api/v1/undo-send", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("get: código = %d, quiero 401", rec.Code)
	}
	rec = httptest.NewRecorder()
	a.setUndoSend(rec, noAuthReq(http.MethodPut, "/api/v1/undo-send", `{"seconds":10}`))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("put: código = %d, quiero 401", rec.Code)
	}
}
