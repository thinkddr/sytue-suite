// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInBookRecorreTodasLasPaginas(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/v1/address-books/libreta/contacts/page" {
			t.Errorf("ruta %s", r.URL.Path)
		}
		start := 0
		if _, err := fmt.Sscan(r.URL.Query().Get("cursor"), &start); err != nil && calls > 1 {
			t.Error(err)
		}
		page := ContactPage{}
		for i := start; i < min(start+100, 601); i++ {
			page.Contacts = append(page.Contacts, Contact{ID: fmt.Sprint(i), AddressBookID: "libreta"})
		}
		if start+100 < 601 {
			page.NextCursor = fmt.Sprint(start + 100)
		}
		_ = json.NewEncoder(w).Encode(page)
	}))
	defer srv.Close()
	got, err := NewContacts(srv.URL, nil, nil).InBook(context.Background(), "libreta")
	if err != nil || len(got) != 601 || calls != 7 {
		t.Fatalf("contactos=%d llamadas=%d error=%v", len(got), calls, err)
	}
}

func TestInBookNoDevuelveInventariosIncompletos(t *testing.T) {
	for _, kind := range []string{"cursor-repetido", "pagina-vacia", "error", "error-404", "cancelar", "duplicado"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 1 {
					_ = json.NewEncoder(w).Encode(ContactPage{Contacts: []Contact{{ID: "a"}}, NextCursor: "siguiente"})
					return
				}
				switch kind {
				case "cursor-repetido":
					_ = json.NewEncoder(w).Encode(ContactPage{Contacts: []Contact{{ID: "b"}}, NextCursor: "siguiente"})
				case "pagina-vacia":
					_ = json.NewEncoder(w).Encode(ContactPage{NextCursor: "otro"})
				case "error":
					w.WriteHeader(http.StatusInternalServerError)
				case "error-404":
					w.WriteHeader(http.StatusNotFound)
				case "cancelar":
					cancel()
					<-r.Context().Done()
				case "duplicado":
					_ = json.NewEncoder(w).Encode(ContactPage{Contacts: []Contact{{ID: "a"}}})
				}
			}))
			defer srv.Close()
			got, err := NewContacts(srv.URL, nil, nil).InBook(ctx, "libreta")
			if err == nil || got != nil || calls != 2 {
				t.Fatalf("parcial=%v llamadas=%d error=%v", got, calls, err)
			}
			if kind == "cancelar" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}

func TestInBookLegacySoloSiEsCompletoY404Inicial(t *testing.T) {
	for _, count := range []int{2, 500} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/page") {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				page := ContactPage{}
				for i := range count {
					page.Contacts = append(page.Contacts, Contact{ID: fmt.Sprint(i)})
				}
				_ = json.NewEncoder(w).Encode(page)
			}))
			defer srv.Close()
			got, err := NewContacts(srv.URL, nil, nil).InBook(context.Background(), "book")
			if count < 500 && (err != nil || len(got) != count) || count == 500 && (err == nil || got != nil) {
				t.Fatalf("n=%d got=%d err=%v", count, len(got), err)
			}
		})
	}
}
