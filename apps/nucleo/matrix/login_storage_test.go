// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestStorageNativeLoginRejectsChangedTenantBeforeOpeningBrowser(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"product": "drive", "identity_version": 2, "tenant": "other-org", "state": "artificial-state", "authorize_url": "https://rooms.sytue.eu/rooms/matrix/native/start?state=artificial-state"})
	}))
	defer server.Close()
	cfg := storageConfig(t, server, "drive")
	cfg.SytueApex = "sytue.eu"
	cfg.Bootstrap.StorageIdentityVersion = storage.IsolatedIdentity
	cfg.Bootstrap.MatrixUserID = storage.IsolatedMatrixUserPrefix(storage.Scope{Subject: cfg.Subject, Tenant: cfg.Tenant, Product: "drive"}) + "matrix.test"
	_, err := LoginNative(t.Context(), cfg, api.NewRooms(server.URL, nil, server.Client()), func(context.Context, api.MatrixNativeAuthorization) (api.MatrixNativeCallback, error) {
		t.Fatal("wrong-organization grant opened the system browser")
		return api.MatrixNativeCallback{}, nil
	})
	if !errors.Is(err, api.ErrMatrixSSO) {
		t.Fatal("organization changed between bootstrap and authorization", err)
	}
}

func TestStorageDesktopLoginPinsOrganizationAndPersonInSSO(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"flows":[{"type":"m.login.sso"},{"type":"m.login.token"}]}`))
	}))
	defer server.Close()
	cfg := storageConfig(t, server, "drive")
	cfg.Bootstrap.StorageIdentityVersion = storage.IsolatedIdentity
	cfg.Bootstrap.MatrixUserID = storage.IsolatedMatrixUserPrefix(storage.Scope{Subject: cfg.Subject, Tenant: cfg.Tenant, Product: "drive"}) + "matrix.test"
	cfg.Bootstrap.SSOProviderID = "oidc-sytue-storage-drive"
	stop := errors.New("artificial browser stop")
	_, err := Login(t.Context(), cfg, func(target string) error {
		u, err := url.Parse(target)
		if err != nil || u.Query().Get("sytue_tenant") != cfg.Tenant || u.Query().Get("sytue_subject") != cfg.Subject || u.Query().Get("redirectUrl") == "" {
			t.Fatal("desktop SSO did not bind its scope", err)
		}
		return stop
	})
	if !errors.Is(err, stop) {
		t.Fatal("SSO never reached the scope-bound system browser", err)
	}
}
