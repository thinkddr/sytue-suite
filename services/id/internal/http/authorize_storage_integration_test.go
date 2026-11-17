// SPDX-License-Identifier: Apache-2.0
//go:build integration

package http

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

const storageRedirectFixture = "https://matrix.sytue.eu/_synapse/client/oidc/callback"

func storageAuthorizationFixture(t *testing.T) (*liveServer, domain.Organization, domain.Organization, domain.User, *http.Cookie) {
	t.Helper()
	l := newLiveServer(t)
	orgs := make([]domain.Organization, 2)
	for i, slug := range []string{"acme", "beta"} {
		org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: slug, Name: slug})
		if err != nil {
			t.Fatal(err)
		}
		orgs[i] = org
	}
	cookie := l.memberSession(t, "private-owner@storage.test", orgs[1].ID)
	u, err := l.st.GetUserByEmail(l.ctx, "private-owner@storage.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{OrgID: orgs[0].ID, UserID: u.ID, Role: domain.RoleMember}); err != nil {
		t.Fatal(err)
	}
	l.setClientSecret(t, "sytue-matrix", "artificial-storage-client-secret")
	return l, orgs[0], orgs[1], u, cookie
}

func storageAuthorizeQuery(subject string) (url.Values, string) {
	verifier := "dBjftJeZ4PV8RcPHoP_RbYoRM8z5JJD4mBUleDoPJBQ"
	sum := sha256.Sum256([]byte(verifier))
	return url.Values{
		"client_id": {"sytue-matrix"}, "redirect_uri": {storageRedirectFixture},
		"response_type": {"code"}, "scope": {"openid profile"}, "state": {"artificial-state"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"},
		"sytue_tenant": {"acme"}, "sytue_subject": {subject},
	}, verifier
}

func (l *liveServer) storageAuthorize(t *testing.T, query url.Values, cookie *http.Cookie) (*httptest.ResponseRecorder, *url.URL) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/oauth2/authorize?"+query.Encode(), http.NoBody)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	l.mux.ServeHTTP(rr, req)
	location, err := url.Parse(rr.Header().Get("Location"))
	if err != nil || rr.Code != http.StatusFound {
		t.Fatalf("authorize: status=%d err=%v", rr.Code, err)
	}
	return rr, location
}

func (l *liveServer) storageRedeem(t *testing.T, code, verifier string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {storageRedirectFixture}, "code_verifier": {verifier}}
	req := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("sytue-matrix", "artificial-storage-client-secret")
	rr := httptest.NewRecorder()
	l.mux.ServeHTTP(rr, req)
	return rr
}

func TestStorageAuthorizationUsesGrantTenantWithoutSwitchingSharedSession(t *testing.T) {
	l, acme, beta, u, cookie := storageAuthorizationFixture(t)
	query, verifier := storageAuthorizeQuery(u.ID)
	_, location := l.storageAuthorize(t, query, cookie)
	code := location.Query().Get("code")
	if code == "" || location.Query().Get("state") != "artificial-state" {
		t.Fatal("authorization did not preserve the private grant")
	}
	rr := l.storageRedeem(t, code, verifier)
	if rr.Code != http.StatusOK {
		t.Fatalf("redeem: status=%d", rr.Code)
	}
	var tokens tokenResponse
	if err := json.NewDecoder(rr.Body).Decode(&tokens); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/oauth2/userinfo", http.NoBody)
	request.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	profile := httptest.NewRecorder()
	l.mux.ServeHTTP(profile, request)
	var claims map[string]any
	if err := json.NewDecoder(profile.Body).Decode(&claims); err != nil {
		t.Fatal(err)
	}
	if profile.Code != http.StatusOK || claims["sub"] != u.ID || claims["tenant"] != acme.Slug {
		t.Fatal("userinfo did not bind the grant's subject and tenant", profile.Code)
	}
	stored, err := l.st.GetSessionByToken(l.ctx, domain.HashToken(cookie.Value))
	if err != nil || stored.OrgID != beta.ID {
		t.Fatal("private authorization changed the global session", err)
	}
	if replay := l.storageRedeem(t, code, verifier); replay.Code == http.StatusOK {
		t.Fatal("authorization code replay accepted")
	}
	_, wrongPKCE := l.storageAuthorize(t, query, cookie)
	if l.storageRedeem(t, wrongPKCE.Query().Get("code"), verifier+"wrong").Code == http.StatusOK {
		t.Fatal("incorrect PKCE verifier accepted")
	}
}

func TestStorageAuthorizationRejectsForeignAmbiguousAndRevokedContext(t *testing.T) {
	l, acme, _, u, cookie := storageAuthorizationFixture(t)
	for _, test := range []struct {
		name   string
		change func(url.Values)
		want   string
	}{
		{"foreign-tenant", func(q url.Values) { q.Set("sytue_tenant", "foreign") }, "access_denied"},
		{"duplicate-tenant", func(q url.Values) { q.Add("sytue_tenant", "beta") }, "invalid_request"},
		{"duplicate-subject", func(q url.Values) { q.Add("sytue_subject", u.ID) }, "invalid_request"},
		{"missing-subject", func(q url.Values) { q.Del("sytue_subject") }, "invalid_request"},
		{"empty-subject", func(q url.Values) { q.Set("sytue_subject", "") }, "invalid_request"},
		{"invalid-subject", func(q url.Values) { q.Set("sytue_subject", "person") }, "invalid_request"},
		{"other-client", func(q url.Values) {
			q.Set("client_id", "sytue-contacts")
			q.Set("redirect_uri", "https://contacts.sytue.eu/contacts/auth/callback")
		}, "invalid_request"},
	} {
		t.Run(test.name, func(t *testing.T) {
			q, _ := storageAuthorizeQuery(u.ID)
			test.change(q)
			_, location := l.storageAuthorize(t, q, cookie)
			if location.Query().Get("code") != "" || location.Query().Get("error") != test.want {
				t.Fatal("unsafe storage context accepted", location.Query().Get("error"))
			}
		})
	}
	q, verifier := storageAuthorizeQuery(u.ID)
	_, pending := l.storageAuthorize(t, q, cookie)
	if pending.Query().Get("code") == "" {
		t.Fatal("missing positive control")
	}
	if _, err := l.st.DB().Exec(l.ctx, `DELETE FROM memberships WHERE org_id=$1 AND user_id=$2`, acme.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	if l.storageRedeem(t, pending.Query().Get("code"), verifier).Code == http.StatusOK {
		t.Fatal("removed member redeemed an earlier authorization")
	}
	_, denied := l.storageAuthorize(t, q, cookie)
	if denied.Query().Get("error") != "access_denied" {
		t.Fatal("removed member obtained another authorization")
	}
	other := l.memberSession(t, "other-person@storage.test", acme.ID)
	_, login := l.storageAuthorize(t, q, other)
	if login.Path != "/login/" || login.Query().Get("return_to") != "/oauth2/authorize?"+q.Encode() {
		t.Fatal("wrong browser person was substituted for the grant owner")
	}
}

func TestStorageAuthorizationChecksDestinationConditionalAccess(t *testing.T) {
	l, acme, _, u, cookie := storageAuthorizationFixture(t)
	q, _ := storageAuthorizeQuery(u.ID)
	policy, err := l.st.CreateAccessPolicy(l.ctx, nil, domain.AccessPolicy{OrgID: acme.ID, Name: "Storage destination", Enabled: true, Action: domain.AccessBlock, StartMinute: -1, EndMinute: -1})
	if err != nil {
		t.Fatal(err)
	}
	_, blocked := l.storageAuthorize(t, q, cookie)
	if blocked.Query().Get("error") != "access_denied" {
		t.Fatal("destination conditional access block bypassed")
	}
	if _, err := l.st.DB().Exec(l.ctx, `UPDATE access_policies SET action='require_mfa' WHERE id=$1`, policy.ID); err != nil {
		t.Fatal(err)
	}
	_, enroll := l.storageAuthorize(t, q, cookie)
	if enroll.Path != "/account" || enroll.Query().Get("mfa") != "required" {
		t.Fatal("destination MFA policy bypassed")
	}
	if _, err := l.st.DB().Exec(l.ctx, `UPDATE users SET mfa_enabled=true WHERE id=$1`, u.ID); err != nil {
		t.Fatal(err)
	}
	rr, step := l.storageAuthorize(t, q, cookie)
	if step.Path != "/login/" || step.Query().Get("step_up") != "1" || cookieNamed(rr.Result().Cookies(), sessionCookieName) != nil {
		t.Fatal("MFA challenge did not preserve the original session")
	}
}
