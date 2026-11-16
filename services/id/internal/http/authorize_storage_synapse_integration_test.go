// SPDX-License-Identifier: Apache-2.0
//go:build integration

package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
)

// This fixture uses the unmodified digest-pinned Synapse image and the real ID
// authorization, PKCE, token, JWT/JWKS and userinfo endpoints. HTTP and synthetic
// credentials are confined to the disposable loopback fixture.
func TestStorageAuthorizationAgainstRealSynapse(t *testing.T) {
	if os.Getenv("SYTUE_STORAGE_OIDC_HTTP") != "1" {
		t.Skip("requires disposable Synapse OIDC fixture")
	}
	l, acme, beta, person, cookie := storageAuthorizationFixture(t)
	const identity = "http://127.0.0.1:18587"
	const matrix = "http://127.0.0.1:18585"
	listener, err := net.Listen("tcp", "127.0.0.1:18587")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(nil)
	_ = server.Listener.Close()
	server.Listener = listener
	l.srv.cfg.Issuer = identity
	l.srv.issuer = oidc.NewIssuer(l.srv.keys, identity, itAud, 15*time.Minute, time.Hour)
	server.Config.Handler = l.srv.Router()
	server.Start()
	t.Cleanup(server.Close)
	if _, err := l.st.DB().Exec(l.ctx, `UPDATE oauth_clients SET redirect_uris=ARRAY[$1] WHERE id='sytue-matrix'`, matrix+"/_synapse/client/oidc/callback"); err != nil {
		t.Fatal(err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	idURL, _ := url.Parse(identity)
	jar.SetCookies(idURL, []*http.Cookie{cookie})
	client := &http.Client{Jar: jar, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	requestJSON := func(method, path, bearer string, input any) (int, map[string]any) {
		t.Helper()
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(t.Context(), method, matrix+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
			t.Fatal("invalid Matrix JSON response", resp.StatusCode, err)
		}
		return resp.StatusCode, out
	}
	status, admin := requestJSON("POST", "/_matrix/client/v3/login", "", map[string]any{"type": "m.login.password", "identifier": map[string]any{"type": "m.id.user", "user": "rooms_controller"}, "password": "artificial-oidc-controller-password"})
	adminToken, _ := admin["access_token"].(string)
	if status != http.StatusOK || adminToken == "" {
		t.Fatal("fixture controller unavailable", status)
	}
	redirect := func(target string) *url.URL {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusSeeOther {
			t.Fatal("OIDC did not redirect", req.URL.Path, resp.StatusCode)
		}
		location, err := url.Parse(resp.Header.Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		return req.URL.ResolveReference(location)
	}
	for _, scope := range []struct{ tenant, product string }{{acme.Slug, "drive"}, {acme.Slug, "mail"}, {beta.Slug, "drive"}} {
		t.Run(scope.tenant+"-"+scope.product, func(t *testing.T) {
			provider := "oidc-sytue-storage-" + scope.product
			userID := "@sytue_storage_" + scope.product + "_" + scope.tenant + "_" + person.ID + ":storage.test"
			status, _ := requestJSON("PUT", "/_synapse/admin/v2/users/"+url.PathEscape(userID), adminToken, map[string]any{"displayname": "Artificial Storage Person", "external_ids": []map[string]string{{"auth_provider": provider, "external_id": scope.tenant + "|" + person.ID}}})
			if status != http.StatusCreated {
				t.Fatal("scoped fixture provisioning failed", status)
			}
			start := matrix + "/_matrix/client/v3/login/sso/redirect/" + provider + "?" + url.Values{"redirectUrl": {"http://127.0.0.1:18588/callback"}, "sytue_tenant": {scope.tenant}, "sytue_subject": {person.ID}}.Encode()
			authorize := redirect(start)
			q := authorize.Query()
			if authorize.Scheme+"://"+authorize.Host != identity || q.Get("sytue_tenant") != scope.tenant || q.Get("sytue_subject") != person.ID || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
				t.Fatal("Synapse failed to bind PKCE and scope")
			}
			callback := redirect(authorize.String())
			if callback.Query().Get("code") == "" || callback.Query().Get("state") != q.Get("state") {
				t.Fatal("ID authorization did not preserve Synapse state")
			}
			completed := redirect(callback.String())
			token := completed.Query().Get("loginToken")
			if token == "" {
				t.Fatal("Synapse did not issue a one-use login token")
			}
			status, session := requestJSON("POST", "/_matrix/client/v3/login", "", map[string]any{"type": "m.login.token", "token": token})
			access, _ := session["access_token"].(string)
			if status != http.StatusOK || session["user_id"] != userID || access == "" {
				t.Fatal("OIDC selected the browser's other identity or organization", status)
			}
			status, who := requestJSON("GET", "/_matrix/client/v3/account/whoami", access, nil)
			if status != http.StatusOK || who["user_id"] != userID {
				t.Fatal("Matrix Whoami identity mismatch", status)
			}
			status, _ = requestJSON("POST", "/_matrix/client/v3/login", "", map[string]any{"type": "m.login.token", "token": token})
			if status == http.StatusOK {
				t.Fatal("Matrix login token replay accepted")
			}
			if stored, err := l.st.GetSessionByToken(l.ctx, domain.HashToken(cookie.Value)); err != nil || stored.OrgID != beta.ID {
				t.Fatal("SSO changed another app's organization", err)
			}
		})
	}
}
