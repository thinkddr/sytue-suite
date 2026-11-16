// SPDX-License-Identifier: Apache-2.0
package http_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	docshttp "git.sytue.eu/sytue/one/services/docs/internal/http"
)

func TestResourceSourceIsNotMountedWithoutVerifiedProvider(t *testing.T) {
	for _, secret := range []string{"", strings.Repeat("a", 64)} {
		handler := docshttp.New(docshttp.Deps{ResourceSourceSecret: secret})
		router := chi.NewRouter()
		handler.Mount(router)
		req := httptest.NewRequest(http.MethodPost, httpx.ResourceBootstrapSourcePath, http.NoBody)
		req.Header.Set("Authorization", "Bearer "+secret)
		reply := httptest.NewRecorder()
		router.ServeHTTP(reply, req)
		if reply.Code != http.StatusNotFound {
			t.Fatal("unconfigured Source was mounted or marked ready", reply.Code)
		}
	}
}

func TestResourceBootstrapPeerRemainsUnmounted(t *testing.T) {
	handler := docshttp.New(docshttp.Deps{})
	router := chi.NewRouter()
	handler.Mount(router)
	request := httptest.NewRequest(http.MethodPost, docshttp.ResourceBootstrapPeerPath, http.NoBody)
	reply := httptest.NewRecorder()
	router.ServeHTTP(reply, request)
	if reply.Code != http.StatusNotFound {
		t.Fatal("peer metadata activated before verified composition", reply.Code)
	}
}
