// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

func TestGetSignatureOK(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, signature: "Ada Lovelace\nCTO · Sytue"}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.getSignature(rec, req(http.MethodGet, "/api/v1/signature", "", true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	assertJSONContentType(t, rec)
	var got struct {
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Los saltos de línea son la firma: si se perdieran, llegaría en una sola línea.
	if got.Signature != "Ada Lovelace\nCTO · Sytue" {
		t.Fatalf("signature = %q", got.Signature)
	}
}

// Sin firma no es un error: es una cuenta que aún no la ha puesto.
func TestGetSignatureVacia(t *testing.T) {
	a := newAPI(&fakeStore{acc: domain.Account{ID: uuid.New()}}, nil, nil)
	rec := httptest.NewRecorder()
	a.getSignature(rec, req(http.MethodGet, "/api/v1/signature", "", true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d, quiero 200", rec.Code)
	}
}

func TestSetSignatureOK(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.setSignature(rec, req(http.MethodPut, "/api/v1/signature",
		`{"signature":"Ada Lovelace\nCTO · Sytue"}`, true, nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204 (%s)", rec.Code, rec.Body.String())
	}
	if st.lastSignature != "Ada Lovelace\nCTO · Sytue" {
		t.Fatalf("no llegó al store tal cual: %q", st.lastSignature)
	}
}

// Borrar la firma (cadena vacía) es una operación legítima, no un cuerpo inválido.
func TestSetSignatureVaciaBorra(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, signature: "algo"}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.setSignature(rec, req(http.MethodPut, "/api/v1/signature", `{"signature":""}`, true, nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204", rec.Code)
	}
	if st.lastSignature != "" {
		t.Fatalf("no se guardó el borrado: %q", st.lastSignature)
	}
}

// Tope de tamaño: se RECHAZA, no se trunca. Una firma cortada por la mitad se
// descubre cuando ya la ha recibido alguien.
func TestSetSignatureDemasiadoLarga(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.setSignature(rec, req(http.MethodPut, "/api/v1/signature",
		`{"signature":"`+strings.Repeat("x", maxSignatureBytes+1)+`"}`, true, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("código = %d, quiero 400", rec.Code)
	}
	if st.lastSignature != "" {
		t.Fatalf("una firma rechazada no puede llegar al store: %q", st.lastSignature)
	}
}

func TestSetSignatureEnElLimiteSeAcepta(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.setSignature(rec, req(http.MethodPut, "/api/v1/signature",
		`{"signature":"`+strings.Repeat("x", maxSignatureBytes)+`"}`, true, nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204 (%s)", rec.Code, rec.Body.String())
	}
}

func TestSignatureSinAuth(t *testing.T) {
	a := newAPI(&fakeStore{}, nil, nil)
	for _, tc := range []struct {
		name string
		call func(*httptest.ResponseRecorder)
	}{
		{"get", func(rec *httptest.ResponseRecorder) {
			a.getSignature(rec, noAuthReq(http.MethodGet, "/api/v1/signature", ""))
		}},
		{"put", func(rec *httptest.ResponseRecorder) {
			a.setSignature(rec, noAuthReq(http.MethodPut, "/api/v1/signature", `{"signature":"x"}`))
		}},
	} {
		rec := httptest.NewRecorder()
		tc.call(rec)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s: código = %d, quiero 401", tc.name, rec.Code)
		}
	}
}
