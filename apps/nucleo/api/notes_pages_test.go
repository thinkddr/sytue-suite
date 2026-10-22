// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNotasPaginadasUsanCienSoloEnServidorConCapacidad(t *testing.T) {
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI())
		if strings.HasSuffix(r.URL.Path, "/page") {
			if r.URL.Query().Get("limit") != "100" {
				t.Error("modern page has wrong limit")
			}
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("limit") != "1" || r.URL.Query().Get("raw_data") != "1" {
			t.Error("legacy fallback can materialize large batches")
		}
		if r.URL.Query().Get("cursor") == "" {
			_, _ = w.Write([]byte(`{"notes":[{"id":"a","data_json":"{\"future\":9007199254740993}"}],"next_cursor":"a"}`))
		} else {
			_, _ = w.Write([]byte(`{"notes":[{"id":"b"}]}`))
		}
	}))
	defer srv.Close()
	rows, err := NewNotes(srv.URL, nil, nil).ListNotesForEncryption(context.Background(), "book")
	if err != nil || len(rows) != 2 || len(requests) != 3 || !strings.Contains(string(rows[0].Data), "9007199254740993") {
		t.Fatalf("bounded legacy fallback: %d %v requests=%v", len(rows), err, requests)
	}
}

func TestNotasPaginadasNoRetrocedenEnFallosDistintosA404(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusRequestEntityTooLarge, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(status)
			}))
			defer srv.Close()
			_, err := NewNotes(srv.URL, nil, nil).ListNotes(context.Background(), "book")
			if err == nil || calls != 1 {
				t.Fatalf("read error bypassed modern bound: %d %v", calls, err)
			}
		})
	}
}

func TestNotasReinicianLecturaSiDesapareceCapacidadDurantePaginacion(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch {
		case calls == 1:
			_, _ = w.Write([]byte(`{"notes":[{"id":"a"}],"next_cursor":"a"}`))
		case strings.HasSuffix(r.URL.Path, "/page"):
			http.NotFound(w, r)
		default:
			// Pre-pagination backends ignore cursor and return the full list.
			if r.URL.Query().Get("cursor") != "" || r.URL.Query().Get("limit") != "1" {
				t.Error("fallback retained a modern cursor")
			}
			_, _ = w.Write([]byte(`{"notes":[{"id":"a"},{"id":"b"}]}`))
		}
	}))
	defer srv.Close()
	rows, err := NewNotes(srv.URL, nil, nil).ListNotes(context.Background(), "book")
	if err != nil || len(rows) != 2 || calls != 3 {
		t.Fatalf("mixed page generations duplicated or lost notes: %d %d %v", len(rows), calls, err)
	}
}
