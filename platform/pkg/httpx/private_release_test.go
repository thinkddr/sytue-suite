// SPDX-License-Identifier: Apache-2.0
package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

func TestSoltarLaFuenteExigeConfirmacionYReautenticacion(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "ada", Product: "notes"}
	epoch := uuid.New()
	authTime := time.Now().Add(-time.Minute)
	stale := errors.New("stale")
	for _, tc := range []struct {
		name, body string
		delegated  bool
		recent     bool
		release    error
		status     int
		code       string
		released   bool
	}{
		{name: "ok", body: `{"epoch":"` + epoch.String() + `","discard_private_writes":true}`, recent: true, status: 200, released: true},
		{name: "sin confirmar", body: `{"epoch":"` + epoch.String() + `"}`, recent: true, status: 400, code: "private_storage_release_unconfirmed"},
		{name: "confirmación falsa", body: `{"epoch":"` + epoch.String() + `","discard_private_writes":false}`, recent: true, status: 400, code: "private_storage_release_unconfirmed"},
		{name: "sin época", body: `{"discard_private_writes":true}`, recent: true, status: 400},
		{name: "época nula", body: `{"epoch":"00000000-0000-0000-0000-000000000000","discard_private_writes":true}`, recent: true, status: 400},
		{name: "sin reautenticar", body: `{"epoch":"` + epoch.String() + `","discard_private_writes":true}`, status: 401, code: "insufficient_user_authentication"},
		{name: "delegado", body: `{"epoch":"` + epoch.String() + `","discard_private_writes":true}`, delegated: true, recent: true, status: 403},
		{name: "época vieja", body: `{"epoch":"` + epoch.String() + `","discard_private_writes":true}`, recent: true, release: stale, status: 409, code: "private_storage_release_stale"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var released bool
			var notified bool
			handler, err := NewPrivateStorageRelease(PrivateStorageReleaseConfig{
				Product: "notes",
				Identity: func(context.Context) (PrivateSourceIdentity, bool) {
					return PrivateSourceIdentity{Tenant: "acme", Subject: "ada", Delegado: tc.delegated}, true
				},
				Reauthenticated: func(context.Context) (Reauthentication, bool) {
					return Reauthentication{AuthTime: authTime, AMR: []string{"pwd"}}, tc.recent
				},
				Release: func(_ context.Context, got storage.Scope, gotEpoch uuid.UUID, proof Reauthentication) error {
					if got != scope || gotEpoch != epoch || !proof.AuthTime.Equal(authTime) {
						t.Fatal("release got", got, gotEpoch, proof)
					}
					released = true
					return tc.release
				},
				MapError: func(err error) *Error {
					if errors.Is(err, stale) {
						return ErrConflict.WithCode("private_storage_release_stale")
					}
					return ErrInternal
				},
				Notify: func(_ context.Context, got storage.Scope) { notified = got == scope },
			})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/private-storage/migration/release", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			encoded, _ := storage.EncodeScopeHeader(scope)
			request.Header.Set(storage.ScopeHeader, encoded)
			request = request.WithContext(tenancy.Into(t.Context(), tenancy.Tenant{Slug: "acme"}))
			response := httptest.NewRecorder()
			handler(response, request)
			if response.Code != tc.status || (tc.code != "" && !strings.Contains(response.Body.String(), tc.code)) {
				t.Fatal(response.Code, response.Body.String())
			}
			if released != (tc.released || tc.release != nil) {
				t.Fatal("release called:", released)
			}
			if notified != (tc.status == 200) {
				t.Fatal("notified:", notified)
			}
			if tc.status == 401 && !strings.Contains(response.Header().Get("WWW-Authenticate"), "max_age=300") {
				t.Fatal("RFC 9470 header", response.Header())
			}
			if tc.status == 200 && !strings.Contains(response.Body.String(), `"released_epoch":"`+epoch.String()+`"`) {
				t.Fatal(response.Body.String())
			}
		})
	}
}
