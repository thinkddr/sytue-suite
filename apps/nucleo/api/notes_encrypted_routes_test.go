// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNotasCifradasServidorAntiguoNoEscribeNiRetrocede(t *testing.T) {
	inputs := map[string]NoteInput{
		"sealed":       {CipherVersion: 2, ExpectedCrypto: json.RawMessage(`{"v":1,"salt":"public","check":"check"}`), ContentCipher: "encv1:sealed"},
		"metadata":     {CipherVersion: 2, ExpectedCrypto: json.RawMessage(`{"v":1}`), Pinned: new(true)},
		"crypto only":  {ExpectedCrypto: json.RawMessage(`{"v":1}`)},
		"cipher only":  {ContentCipher: "encv1:old"},
		"old version":  {CipherVersion: 1},
		"next version": {CipherVersion: 3},
	}
	for name, input := range inputs {
		for _, method := range []string{http.MethodPost, http.MethodPut} {
			t.Run(name+"/"+method, func(t *testing.T) {
				requests, writes := 0, 0
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests++
					// This old backend would accept and persist any legacy write,
					// without enforcing cipher_version or expected_crypto.
					if r.URL.Path == "/api/v1/notebooks/book/notes" || r.URL.Path == "/api/v1/notes/note" {
						writes++
						_, _ = w.Write([]byte(`{"id":"unsafe-write"}`))
						return
					}
					want := "/api/v1/notebooks/book/encrypted-notes"
					if method == http.MethodPut {
						want = "/api/v1/notes/note/encrypted"
					}
					if r.Method != method || r.URL.Path != want {
						t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					}
					http.NotFound(w, r)
				}))
				defer srv.Close()
				svc := NewNotes(srv.URL, nil, nil)
				var err error
				if method == http.MethodPost {
					_, err = svc.CreateNote(context.Background(), "book", input)
				} else {
					_, err = svc.UpdateNote(context.Background(), "note", "book", input)
				}
				var response *Error
				if !errors.As(err, &response) || response.Status != http.StatusNotFound || requests != 1 || writes != 0 {
					t.Fatalf("unsafe old backend: requests=%d writes=%d err=%v", requests, writes, err)
				}
			})
		}
	}
}

func TestNotasCifradasUsanRutaVersionadaConCASCompleto(t *testing.T) {
	crypto := json.RawMessage(`{"v":1,"salt":"public","iterations":600000,"check":"check","extra":"keep"}`)
	in := NoteInput{CipherVersion: 2, ExpectedCrypto: crypto, ContentCipher: "encv1:sealed", Data: json.RawMessage(`{"client_id":"opaque"}`)}
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				want := "/api/v1/notebooks/book/encrypted-notes"
				if method == http.MethodPut {
					want = "/api/v1/notes/note/encrypted"
				}
				var got struct {
					NoteInput
					NotebookID string `json:"notebook_id"`
				}
				if r.Method != method || r.URL.Path != want || json.NewDecoder(r.Body).Decode(&got) != nil {
					t.Errorf("unexpected encrypted request: %s %s", r.Method, r.URL.Path)
				}
				if got.CipherVersion != 2 || !reflectEqual(crypto, got.ExpectedCrypto) || got.ContentCipher != in.ContentCipher || string(got.Data) != string(in.Data) {
					t.Errorf("encrypted preconditions or payload changed: %+v", got)
				}
				if method == http.MethodPut && got.NotebookID != "book" {
					t.Error("update lost notebook identity")
				}
				_, _ = w.Write([]byte(`{"id":"saved","notebook_id":"book","content_cipher":"encv1:sealed"}`))
			}))
			defer srv.Close()
			svc := NewNotes(srv.URL, nil, nil)
			var got Note
			var err error
			if method == http.MethodPost {
				got, err = svc.CreateNote(context.Background(), "book", in)
			} else {
				got, err = svc.UpdateNote(context.Background(), "note", "book", in)
			}
			if err != nil || calls != 1 || got.ID != "saved" {
				t.Fatalf("encrypted write: calls=%d note=%+v err=%v", calls, got, err)
			}
		})
	}
}

func TestNotasSinCifrarConservanRutaExistente(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				want := "/api/v1/notebooks/book/notes"
				if method == http.MethodPut {
					want = "/api/v1/notes/note"
				}
				if r.Method != method || r.URL.Path != want {
					t.Errorf("plaintext route changed: %s %s", r.Method, r.URL.Path)
				}
				_, _ = w.Write([]byte(`{"id":"plain"}`))
			}))
			defer srv.Close()
			svc := NewNotes(srv.URL, nil, nil)
			input := NoteInput{Title: "ordinary note"}
			var err error
			if method == http.MethodPost {
				_, err = svc.CreateNote(context.Background(), "book", input)
			} else {
				_, err = svc.UpdateNote(context.Background(), "note", "book", input)
			}
			if err != nil || calls != 1 {
				t.Fatalf("plaintext write: calls=%d err=%v", calls, err)
			}
		})
	}
}
