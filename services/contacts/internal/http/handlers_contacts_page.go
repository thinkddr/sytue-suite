// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactshttp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/contacts/internal/store"
)

type contactsCursor struct {
	Version  int                 `json:"v"`
	Tenant   string              `json:"tenant"`
	Book     uuid.UUID           `json:"book"`
	Position store.ContactCursor `json:"position"`
}

func readContactsCursor(raw, tenant string, book uuid.UUID) (*store.ContactCursor, error) {
	if raw == "" {
		return nil, nil //nolint:nilnil // An absent cursor means the first page.
	}
	if len(raw) > 16384 {
		return nil, errors.New("cursor too long")
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}
	var cursor contactsCursor
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil {
		return nil, err
	}
	if decoder.Decode(new(any)) != io.EOF || cursor.Version != 1 || cursor.Tenant != tenant || cursor.Book != book || cursor.Position.ID == uuid.Nil {
		return nil, errors.New("invalid contact cursor scope")
	}
	return &cursor.Position, nil
}

// ListContactsPage explicitly opts clients into bounded keyset pages. The old route
// remains compatible; clients that need a complete inventory use this route.
func (h *Handlers) ListContactsPage(w http.ResponseWriter, r *http.Request) {
	book, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 500 {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid page size."))
			return
		}
	}
	tenant := tenancy.MustFrom(r.Context()).Slug
	cursor, err := readContactsCursor(r.URL.Query().Get("cursor"), tenant, book)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid contact cursor."))
		return
	}
	page, err := h.store.ListContactsPage(r.Context(), subFromCtx(r), book, cursor, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	next := ""
	if page.Next != nil {
		raw, err := json.Marshal(contactsCursor{Version: 1, Tenant: tenant, Book: book, Position: *page.Next})
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		next = base64.RawURLEncoding.EncodeToString(raw)
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"contacts": page.Contacts, "next_cursor": next})
}
