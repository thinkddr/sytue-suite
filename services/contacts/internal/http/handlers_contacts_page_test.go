// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package contactshttp

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
)

func TestContactsPageCursorAndLegacyCompatibility(t *testing.T) {
	book := uuid.New()
	contacts := make([]domain.Contact, 501)
	for i := range contacts {
		contacts[i].ID = uuid.New()
	}
	h := newHandlers(&fakeStore{contacts: contacts})
	rec := httptest.NewRecorder()
	r := req(http.MethodGet, "/?limit=100", "", map[string]string{"id": book.String()})
	r = r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: "acme"}))
	h.ListContactsPage(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var body struct {
		Contacts []domain.Contact `json:"contacts"`
		Next     string           `json:"next_cursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Contacts) != 100 || body.Next == "" {
		t.Fatalf("page=%+v", body)
	}
	pos, err := readContactsCursor(body.Next, "acme", book)
	if err != nil || pos.ID != contacts[99].ID {
		t.Fatal(pos, err)
	}
	for _, raw := range []string{"?", base64.RawURLEncoding.EncodeToString([]byte(`{"v":1}`)), body.Next + "!"} {
		if _, err := readContactsCursor(raw, "acme", book); err == nil {
			t.Fatal("accepted", raw)
		}
	}
	for _, scope := range []struct {
		tenant string
		book   uuid.UUID
	}{{"otra", book}, {"acme", uuid.New()}} {
		if _, err := readContactsCursor(body.Next, scope.tenant, scope.book); err == nil {
			t.Fatal("wrong scope accepted")
		}
	}
	// The legacy handler still delegates to the original store contract.
	rec = httptest.NewRecorder()
	h.ListContacts(rec, req(http.MethodGet, "/?limit=5", "", map[string]string{"id": book.String()}))
	if rec.Code != http.StatusOK {
		t.Fatal("legacy changed", rec.Code)
	}
}

func TestContactsPageRejectsBadLimitCursorAndAuthorization(t *testing.T) {
	for _, query := range []string{"limit=0", "limit=501", "limit=abc", "cursor=bad"} {
		r := req(http.MethodGet, "/?"+query, "", id())
		r = r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: "acme"}))
		rec := httptest.NewRecorder()
		newHandlers(&fakeStore{}).ListContactsPage(rec, r)
		if rec.Code != http.StatusBadRequest {
			t.Fatal(query, rec.Code)
		}
	}
	r := req(http.MethodGet, "/", "", id())
	r = r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: "acme"}))
	rec := httptest.NewRecorder()
	newHandlers(&fakeStore{contactErr: domain.ErrNotFound}).ListContactsPage(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatal(rec.Code)
	}
}
