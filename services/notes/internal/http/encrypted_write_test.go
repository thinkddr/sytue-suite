// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package noteshttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

func TestEncryptedWritesHaveDistinctRoutesAndRejectOldVersionsBeforeStorage(t *testing.T) {
	router := NewRouter(Deps{Store: &store.Store{}}).(chi.Router)
	const id = "0b9f8c1e-2f9a-4c55-9a0e-3f2d9b6c1a10"
	for _, test := range []struct {
		method, path string
		handler      http.HandlerFunc
	}{
		{http.MethodPost, "/api/v1/notebooks/" + id + "/encrypted-notes", (&Handlers{}).CreateEncryptedNote},
		{http.MethodPut, "/api/v1/notes/" + id + "/encrypted", (&Handlers{}).UpdateEncryptedNote},
	} {
		if !router.Match(chi.NewRouteContext(), test.method, test.path) {
			t.Fatalf("missing versioned route: %s %s", test.method, test.path)
		}
		for _, version := range []string{"0", "1", "3"} {
			req := httptest.NewRequest(test.method, test.path, strings.NewReader(`{"cipher_version":`+version+`,"data":{}}`))
			route := chi.NewRouteContext()
			route.URLParams.Add("id", id)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
			rec := httptest.NewRecorder()
			test.handler(rec, req)
			if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "encrypted_client_upgrade_required") {
				t.Fatalf("version %s reached storage or was accepted by %s: %d %s", version, test.path, rec.Code, rec.Body.String())
			}
		}
	}
}
