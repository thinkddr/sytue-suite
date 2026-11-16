// SPDX-License-Identifier: Apache-2.0
//go:build integration

package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/matrix"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func TestMatrixNativeBrokerKeepsGrantTenantAndRechecksMembership(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	cipher, err := keycrypto.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(h.Pool, cipher)
	const actor = "c2236155-0c17-4d4f-ad5b-6b657d44286a"
	var member atomic.Bool
	member.Store(true)
	id := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			UserID string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.UserID != actor || r.URL.Path != "/internal/people/orgs" || r.Header.Get("Authorization") != "Bearer artificial-native-identity-proof" {
			http.Error(w, "denied", http.StatusForbidden)
			return
		}
		orgs := []string{"beta"}
		if member.Load() {
			orgs = append(orgs, "acme")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"orgs": orgs, "personal": false})
	}))
	defer id.Close()
	mc := matrix.New(matrix.Config{InternalURL: "http://matrix.storage.test", HomeserverURL: "https://matrix.storage.test", ServerName: "matrix.storage.test", SSOProviderID: "oidc-sytue", AdminToken: "artificial-controller"})
	s := &Service{pstore: st, matrix: mc, cfg: config.Config{PublicURL: "https://rooms.storage.test", MatrixHomeserverURL: "https://matrix.storage.test", MatrixServerName: "matrix.storage.test", MatrixSSOProviderID: "oidc-sytue", IDURL: id.URL, MatrixIdentitySecret: "artificial-native-identity-proof"}}
	acme := tenancy.Into(t.Context(), tenancy.Tenant{Slug: "acme"})
	beta := tenancy.Into(t.Context(), tenancy.Tenant{Slug: "beta"})
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge, err := matrixPKCEChallenge(verifier)
	if err != nil {
		t.Fatal(err)
	}
	for _, consumer := range []string{"", "pdf"} {
		state, err := st.CreateMatrixNativeConsentForConsumer(acme, actor, challenge, "drive", consumer, storage.IsolatedIdentity)
		if err != nil {
			t.Fatal(err)
		}
		preview, err := s.PreviewMatrixNative(beta, actor, state)
		if err != nil || preview.Tenant != "acme" || preview.Product != "drive" || preview.NativeConsumer != consumer {
			t.Fatal("browser tenant replaced immutable grant context", err)
		}
		target, err := url.Parse(preview.AuthorizeURL)
		if err != nil || target.Query().Get("sytue_tenant") != "acme" || target.Query().Get("sytue_subject") != actor {
			t.Fatal("grant scope did not reach OIDC", err)
		}
		member.Store(false)
		const loginToken = "artificial-native-one-use-login-token"
		if _, err := s.AcceptMatrixNative(beta, actor, state, loginToken); !errors.Is(err, domain.ErrForbidden) {
			t.Fatal("removed member accepted browser token", err)
		}
		member.Store(true)
		accepted, err := s.AcceptMatrixNative(beta, actor, state, loginToken)
		if err != nil {
			t.Fatal(err)
		}
		if accepted.NativeConsumer != consumer || !strings.HasPrefix(accepted.RedirectURL, "sytue-"+storage.NativeReturnApp("drive", consumer)+"://matrix/callback?") {
			t.Fatal("consumer callback changed the data account or recipient")
		}
		member.Store(false)
		if _, err := s.FinishMatrixNativeForConsumer(beta, actor, state, accepted.Ticket, verifier, "drive", consumer, storage.IsolatedIdentity); !errors.Is(err, domain.ErrForbidden) {
			t.Fatal("removed member finished private login", err)
		}
		member.Store(true)
		token, err := s.FinishMatrixNativeForConsumer(beta, actor, state, accepted.Ticket, verifier, "drive", consumer, storage.IsolatedIdentity)
		if err != nil || token != loginToken {
			t.Fatal("native callback could not return to its original organization", err)
		}
		if _, err := s.FinishMatrixNativeForConsumer(beta, actor, state, accepted.Ticket, verifier, "drive", consumer, storage.IsolatedIdentity); !errors.Is(err, domain.ErrForbidden) {
			t.Fatal("native token replay accepted", err)
		}
	}
}
