// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
)

// This exercises ID's real membership removal, password login and OAuth code
// grant. Removal requests that Hub forget the address, but does not itself
// remove the mailbox assignment. The Hub is a request observer only: this test
// does not establish that a real Hub persisted a tombstone, that iat follows it,
// or that any notice is visible or inaccessible after reassignment.
func TestRemovedMembershipCanLeaveMailboxAsLoginEmail(t *testing.T) {
	l := newLiveServer(t)
	// Delay mailbox allocation until the two memberships exist so that the
	// collision is created with the ordinary mailbox allocator, not raw SQL.
	l.st.SetSuiteDomain("")
	retired, admin := l.seedOrgAdmin(t, "retired")
	other, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "other", Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	const email = "alias@retired.suite.test"
	const password = "member-pass-123"
	member := l.mkUser(t, email, password)
	for _, org := range []domain.Organization{retired, other} {
		if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{
			OrgID: org.ID, UserID: member.ID, Role: domain.RoleMember,
		}); err != nil {
			t.Fatal(err)
		}
	}
	l.st.SetSuiteDomain("suite.test")
	address, err := l.st.EnsureOrgMailbox(l.ctx, nil, retired.ID, member.ID, "alias")
	if err != nil || address != email {
		t.Fatalf("ordinary mailbox allocation: address=%q error=%v", address, err)
	}

	var forgotten []string
	var forgottenMu sync.Mutex
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/internal/notifications/forget" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		var body struct {
			Address string `json:"address"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		forgottenMu.Lock()
		forgotten = append(forgotten, body.Address)
		forgottenMu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer hub.Close()
	l.srv.cfg.HubBaseURL = hub.URL
	l.srv.cfg.HubInternalSecret = "test-hub-secret"
	adminToken := l.token(t, admin.ID, retired.Slug, []string{"admin"})
	rr, _ := l.do(t, http.MethodDelete, "/api/v1/admin/users/"+member.ID+"/membership", adminToken, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("remove membership: status=%d", rr.Code)
	}
	forgottenMu.Lock()
	observed := append([]string(nil), forgotten...)
	forgottenMu.Unlock()
	if len(observed) != 1 || observed[0] != email {
		t.Fatalf("ID requested mailbox forgetting for %v, want [%q]", observed, email)
	}
	if _, err := l.st.GetMembership(l.ctx, retired.ID, member.ID); err == nil {
		t.Fatal("retired membership remains")
	}
	if _, err := l.st.GetMembership(l.ctx, other.ID, member.ID); err != nil {
		t.Fatalf("remaining membership: %v", err)
	}
	mailboxes, err := l.st.ListOrgMailboxes(l.ctx, retired.ID, member.ID)
	if err != nil || len(mailboxes) != 1 || mailboxes[0].Address != email {
		t.Fatalf("membership removal does not retire mailbox: mailboxes=%v error=%v", mailboxes, err)
	}

	// Log in with the unchanged account email after removal and obtain a
	// freshly signed token for the only remaining membership.
	login := l.postForm(t, "/login/", url.Values{"email": {email}, "password": {password}})
	cookie := cookieNamed(login.Result().Cookies(), sessionCookieName)
	if login.Code != http.StatusFound || cookie == nil {
		t.Fatalf("new login: status=%d session=%t", login.Code, cookie != nil)
	}
	const clientID = "sytue-contacts"
	const clientSecret = "test-contacts-secret"
	const redirectURI = "https://contacts.sytue.eu/contacts/auth/callback"
	l.setClientSecret(t, clientID, clientSecret)
	accessToken := l.tokenDeApp(t, clientID, clientSecret, redirectURI, cookie)
	keys, err := l.srv.keys.PublicJWKS()
	if err != nil {
		t.Fatal(err)
	}
	token, err := oidc.Verify(accessToken, itIssuer, itAud, keys)
	if err != nil {
		t.Fatalf("verify access token: %v", err)
	}
	if token.Subject() != member.ID {
		t.Fatal("wrong subject in access token")
	}
	if got, _ := token.Get("tenant"); got != other.Slug {
		t.Fatalf("tenant = %v, want remaining organization", got)
	}
	if got, _ := token.Get("email"); got != email {
		t.Fatalf("login email = %v, want %q", got, email)
	}
}

// The alias retirement is performed through ID's handler. PurgeMailbox is called
// directly to model the successful reconciler step; this test does not run Mail,
// the reconciler or a real Hub, and says nothing about Hub's tombstone or access.
func TestReassignedAliasCanRemainInPreviousOwnersSignedEmail(t *testing.T) {
	l := newLiveServer(t)
	l.st.SetSuiteDomain("")
	org, admin := l.seedOrgAdmin(t, "retired")
	other, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "other", Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	const address = "alias@retired.suite.test"
	const password = "member-pass-123"
	oldOwner := l.mkUser(t, address, password)
	newOwner := l.mkUser(t, "new-owner@outside.test", password)
	for _, membership := range []domain.Membership{
		{OrgID: org.ID, UserID: oldOwner.ID, Role: domain.RoleMember},
		{OrgID: other.ID, UserID: oldOwner.ID, Role: domain.RoleMember},
		{OrgID: org.ID, UserID: newOwner.ID, Role: domain.RoleMember},
	} {
		if err := l.st.UpsertMembership(l.ctx, nil, membership); err != nil {
			t.Fatal(err)
		}
	}
	l.st.SetSuiteDomain("suite.test")
	for _, user := range []domain.User{oldOwner, newOwner} {
		if _, err := l.st.EnsureOrgMailbox(l.ctx, nil, org.ID, user.ID, user.Username); err != nil {
			t.Fatal(err)
		}
	}
	oldAlias, err := l.st.AddMailboxAlias(l.ctx, org.ID, oldOwner.ID, address)
	if err != nil || oldAlias.Address != address {
		t.Fatalf("allocate old alias: address=%q error=%v", oldAlias.Address, err)
	}

	adminToken := l.token(t, admin.ID, org.Slug, []string{"admin"})
	rr, _ := l.do(t, http.MethodDelete, "/api/v1/admin/users/"+oldOwner.ID+"/membership", adminToken, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("remove membership: status=%d", rr.Code)
	}
	if _, err := l.st.GetMembership(l.ctx, org.ID, oldOwner.ID); err == nil {
		t.Fatal("previous owner's membership remains in retired organization")
	}
	if _, err := l.st.GetMembership(l.ctx, other.ID, oldOwner.ID); err != nil {
		t.Fatalf("previous owner's remaining membership: %v", err)
	}
	if _, err := l.st.GetMembership(l.ctx, org.ID, newOwner.ID); err != nil {
		t.Fatalf("new owner's membership: %v", err)
	}
	rr, _ = l.do(t, http.MethodDelete, "/api/v1/admin/mailboxes/"+oldAlias.ID, adminToken, nil)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("retire alias: status=%d", rr.Code)
	}
	// The production reconciler must first remove the alias from Mail and make
	// Hub forget it. Neither side effect is represented by this direct purge.
	if err := l.st.PurgeMailbox(l.ctx, oldAlias.ID); err != nil {
		t.Fatalf("model reconciler purge: %v", err)
	}
	rr, _ = l.do(t, http.MethodPost, "/api/v1/admin/users/"+newOwner.ID+"/mailbox", adminToken, map[string]string{"address": address})
	if rr.Code != http.StatusOK {
		t.Fatalf("reassign alias: status=%d", rr.Code)
	}
	resolved, err := l.st.GetUserByAddress(l.ctx, address)
	if err != nil || resolved.ID != newOwner.ID {
		t.Fatalf("reassigned alias owner: id=%q error=%v", resolved.ID, err)
	}

	const clientID = "sytue-contacts"
	const clientSecret = "test-contacts-secret"
	const redirectURI = "https://contacts.sytue.eu/contacts/auth/callback"
	l.setClientSecret(t, clientID, clientSecret)
	keys, err := l.srv.keys.PublicJWKS()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, username, subject, tenant, email string
	}{
		{"previous", oldOwner.Username, oldOwner.ID, other.Slug, address},
		{"new", newOwner.Username, newOwner.ID, org.Slug, newOwner.Email},
	} {
		t.Run(tc.name, func(t *testing.T) {
			login := l.postForm(t, "/login/", url.Values{"email": {"@" + tc.username}, "password": {password}})
			cookie := cookieNamed(login.Result().Cookies(), sessionCookieName)
			if login.Code != http.StatusFound || cookie == nil {
				t.Fatalf("login by username: status=%d session=%t", login.Code, cookie != nil)
			}
			raw := l.tokenDeApp(t, clientID, clientSecret, redirectURI, cookie)
			token, err := oidc.Verify(raw, itIssuer, itAud, keys)
			if err != nil {
				t.Fatalf("verify access token: %v", err)
			}
			if token.Subject() != tc.subject {
				t.Fatal("wrong subject in access token")
			}
			if got, _ := token.Get("tenant"); got != tc.tenant {
				t.Fatalf("tenant = %v, want %q", got, tc.tenant)
			}
			if got, _ := token.Get("email"); got != tc.email {
				t.Fatalf("email = %v, want %q", got, tc.email)
			}
			mailbox, _ := token.Get("mailbox")
			if tc.name == "new" && mailbox != address {
				t.Fatalf("new owner's mailbox = %v, want reassigned address", mailbox)
			}
			if tc.name == "previous" && mailbox == address {
				t.Fatal("previous owner still has reassigned address as mailbox")
			}
		})
	}
}
