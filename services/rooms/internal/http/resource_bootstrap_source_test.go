// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
)

type sourceControlService struct {
	*service.Service
	calls          int
	err            error
	value          resource.BootstrapSourceSnapshot
	query          resource.SignedBootstrapSourceQuery
	actor, product string
}

func (s *sourceControlService) MatrixResourceBootstrapSource(_ context.Context, actor, product string, q resource.SignedBootstrapSourceQuery) (resource.BootstrapSourceSnapshot, error) {
	s.calls++
	s.actor = actor
	s.product = product
	s.query = q
	return s.value, s.err
}

func TestResourceSourceControlHTTPPreservesSignedBodyAndErrors(t *testing.T) {
	svc := &sourceControlService{}
	handlers := &Handlers{svc: svc}
	router := chi.NewRouter()
	router.Post("/matrix/resources/{product}/bootstrap/source", handlers.MatrixResourceBootstrapSource)
	query := resource.SignedBootstrapSourceQuery{BootstrapSourceQuery: resource.BootstrapSourceQuery{Version: 1, Domain: "org.sytue.resource.bootstrap.source.v1"}}
	raw, _ := json.Marshal(query)
	request := func(body []byte) int {
		req := httptest.NewRequest(http.MethodPost, "/matrix/resources/docs/bootstrap/source", bytes.NewReader(body))
		req = req.WithContext(authn.Into(req.Context(), authn.Claims{Subject: "original-actor", Tenant: "acme", Audience: []string{"sytue-docus"}}))
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		return out.Code
	}
	if got := request(raw); got != http.StatusOK || svc.calls != 1 || svc.actor != "original-actor" || svc.product != "docs" || svc.query.BootstrapSourceQuery != query.BootstrapSourceQuery {
		t.Fatal("source HTTP changed original actor/product/signed intent", got)
	}
	before := svc.calls
	bad := append(append([]byte{}, raw[:len(raw)-1]...), []byte(",\"endpoint\":\"https://attacker.invalid\"}")...)
	if got := request(bad); got != http.StatusBadRequest || svc.calls != before {
		t.Fatal("request chose a backend or unknown body field", got)
	}
	for _, test := range []struct {
		err    error
		status int
	}{{resource.ErrInvalid, 400}, {resource.ErrRevoked, 403}, {domain.ErrMatrixUnavailable, 503}} {
		svc.err = test.err
		if got := request(raw); got != test.status {
			t.Fatal("source HTTP lost domain authority status", got, test.status)
		}
	}
}
