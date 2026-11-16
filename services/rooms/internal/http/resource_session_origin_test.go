// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/resourcepg"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

type sessionOriginFixture struct {
	calls     int
	malformed bool
}

func (f *sessionOriginFixture) ResourceSessionOriginForDocs(_ context.Context, q resourcepg.SessionOriginRequest) (resourcepg.SessionOriginResponse, error) {
	f.calls++
	if f.malformed {
		return resourcepg.SessionOriginResponse{Version: 1}, nil
	}
	return resourcepg.SessionOriginResponse{}, domain.ErrMatrixUnavailable
}

func TestResourceSessionOriginInternalDocsBoundary(t *testing.T) {
	secret := strings.Repeat("a", 64)
	body := "{\"nonce\":\"" + strings.Repeat("b", 64) + "\",\"v\":1,\"scope\":{\"tenant\":\"acme\",\"subject\":\"owner\",\"product\":\"docs\"},\"device_id\":\"SDK_DEVICE\"}"
	for _, tc := range []struct {
		name, secret, auth, body, query, xff string
		status, calls                        int
		malformed                            bool
	}{
		{name: "actual service unavailable", secret: secret, auth: "Bearer " + secret, body: body, status: 503, calls: 1},
		{name: "no configured endpoint", auth: "Bearer " + secret, body: body, status: 404},
		{name: "invalid configured secret", secret: "short", auth: "Bearer short", body: body, status: 404},
		{name: "public ingress", secret: secret, auth: "Bearer " + secret, body: body, xff: "203.0.113.1", status: 404},
		{name: "no credential", secret: secret, body: body, status: 401},
		{name: "wrong credential", secret: secret, auth: "Bearer " + strings.Repeat("b", 64), body: body, status: 401},
		{name: "foreign service", secret: secret, auth: "Bearer " + secret, body: strings.Replace(body, "docs", "mail", 1), status: 400},
		{name: "unknown backend field", secret: secret, auth: "Bearer " + secret, body: strings.Replace(body, "\"v\":1", "\"v\":1,\"url\":\"https://attacker.invalid\"", 1), status: 400},
		{name: "boolean version", secret: secret, auth: "Bearer " + secret, body: strings.Replace(body, "\"v\":1", "\"v\":true", 1), status: 400},
		{name: "extra object", secret: secret, auth: "Bearer " + secret, body: body + "{}", status: 400},
		{name: "oversized", secret: secret, auth: "Bearer " + secret, body: body + strings.Repeat(" ", 4096) + "{}", status: 400},
		{name: "query cannot choose source", secret: secret, auth: "Bearer " + secret, body: body, query: "?service=mail", status: 400},
		{name: "malformed service evidence", secret: secret, auth: "Bearer " + secret, body: body, status: 503, calls: 1, malformed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &sessionOriginFixture{malformed: tc.malformed}
			r := chi.NewRouter()
			mountResourceSessionOrigin(r, tc.secret, f)
			q := httptest.NewRequest(http.MethodPost, "/internal/resources/sessions/docs/current"+tc.query, strings.NewReader(tc.body))
			q.Header.Set("Authorization", tc.auth)
			q.Header.Set("X-Forwarded-For", tc.xff)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, q)
			if w.Code != tc.status || f.calls != tc.calls {
				t.Fatal("internal session boundary changed", w.Code, f.calls)
			}
		})
	}
}
