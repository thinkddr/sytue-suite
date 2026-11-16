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

func TestGetReadingPrefsDevuelveLasDeLaCuenta(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, readingPrefs: domain.ReadingPrefs{
		AutoAdvance: "newer", SwipeRight: "read", SwipeLeft: "none",
	}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.getReadingPrefs(rec, req(http.MethodGet, "/api/v1/reading-prefs", "", true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	assertJSONContentType(t, rec)
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["auto_advance"] != "newer" || got["swipe_right"] != "read" || got["swipe_left"] != "none" {
		t.Fatalf("cuerpo = %v", got)
	}
}

func TestSetReadingPrefsGuardaLoValido(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.setReadingPrefs(rec, req(http.MethodPut, "/api/v1/reading-prefs",
		`{"auto_advance":"list","swipe_right":"trash","swipe_left":"archive"}`, true, nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204 (%s)", rec.Code, rec.Body.String())
	}
	want := domain.ReadingPrefs{AutoAdvance: "list", SwipeRight: "trash", SwipeLeft: "archive"}
	if st.lastReading == nil || *st.lastReading != want {
		t.Fatalf("llegó al store %+v, quiero %+v", st.lastReading, want)
	}
}

// Fuera de las listas se RECHAZA entero: ni se redondea ni se guarda a medias.
func TestSetReadingPrefsRechazaLoQueNoEsUnaOpcion(t *testing.T) {
	for _, cuerpo := range []string{
		`{}`,
		`{"auto_advance":"siguiente","swipe_right":"archive","swipe_left":"trash"}`,
		`{"auto_advance":"older","swipe_right":"delete","swipe_left":"trash"}`,
		`{"auto_advance":"older","swipe_right":"archive","swipe_left":""}`,
		`{"auto_advance":"OLDER","swipe_right":"archive","swipe_left":"trash"}`,
	} {
		st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
		a := newAPI(st, nil, nil)
		rec := httptest.NewRecorder()
		a.setReadingPrefs(rec, req(http.MethodPut, "/api/v1/reading-prefs", cuerpo, true, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: código = %d, quiero 400", cuerpo, rec.Code)
		}
		if st.lastReading != nil {
			t.Fatalf("%s: un valor rechazado llegó al store", cuerpo)
		}
	}
}

func TestReadingPrefsCuentaAjenaEs404(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, readingErr: domain.ErrNotFound}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.getReadingPrefs(rec, req(http.MethodGet, "/api/v1/reading-prefs", "", true, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get: código = %d, quiero 404", rec.Code)
	}
}

func TestLecturaSinAuth(t *testing.T) {
	a := newAPI(&fakeStore{}, nil, nil)
	for nombre, h := range map[string]func(http.ResponseWriter, *http.Request){
		"get prefs": a.getReadingPrefs,
		"put prefs": a.setReadingPrefs,
		"list":      a.listTrustedSenders,
		"add":       a.addTrustedSender,
		"remove":    a.removeTrustedSender,
	} {
		rec := httptest.NewRecorder()
		h(rec, noAuthReq(http.MethodGet, "/api/v1/x", `{}`))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s: código = %d, quiero 401", nombre, rec.Code)
		}
	}
}

func TestListTrustedSendersVacioEsLista(t *testing.T) {
	a := newAPI(&fakeStore{acc: domain.Account{ID: uuid.New()}}, nil, nil)
	rec := httptest.NewRecorder()
	a.listTrustedSenders(rec, req(http.MethodGet, "/api/v1/trusted-senders", "", true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	// `null` obligaría a cada cliente a distinguirlo de la lista vacía.
	if got := rec.Body.String(); got != "{\"senders\":[]}\n" && got != `{"senders":[]}` {
		t.Fatalf("cuerpo = %q, quiero una lista vacía", got)
	}
}

func TestAddTrustedSenderDevuelveLaNormalizada(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.addTrustedSender(rec, req(http.MethodPost, "/api/v1/trusted-senders", `{"address":"  Ana@Boletin.EU "}`, true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	var got struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Address != "ana@boletin.eu" {
		t.Fatalf("address = %q", got.Address)
	}
}

func TestAddTrustedSenderRechazaLoQueNoEsDireccion(t *testing.T) {
	for _, cuerpo := range []string{`{}`, `{"address":"ana"}`, `{"address":"a@b@c"}`, `{"address":"ana @x.eu"}`, `{"address":"<a@x.eu>"}`} {
		a := newAPI(&fakeStore{acc: domain.Account{ID: uuid.New()}}, nil, nil)
		rec := httptest.NewRecorder()
		a.addTrustedSender(rec, req(http.MethodPost, "/api/v1/trusted-senders", cuerpo, true, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: código = %d, quiero 400", cuerpo, rec.Code)
		}
	}
}

func TestAddTrustedSenderTopeEs409(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, trustedErr: domain.ErrTrustedSenderLimite}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.addTrustedSender(rec, req(http.MethodPost, "/api/v1/trusted-senders", `{"address":"a@x.eu"}`, true, nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("código = %d, quiero 409", rec.Code)
	}
}

func TestRemoveTrustedSenderLeeLaQuery(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.removeTrustedSender(rec, req(http.MethodDelete, "/api/v1/trusted-senders?address=ana%2Bnews%40x.eu", "", true, nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if st.lastTrustedDel != "ana+news@x.eu" {
		t.Fatalf("llegó al store %q", st.lastTrustedDel)
	}
}
