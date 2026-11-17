// SPDX-License-Identifier: Apache-2.0
package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

func TestStorageAuthorizationHintsRequireProvisionedMatrixClient(t *testing.T) {
	client := domain.Client{ID: "sytue-matrix", Type: domain.ClientConfidential, RequirePKCE: true, TokenEndpointAuthMethod: "client_secret_basic"}
	query := "sytue_tenant=acme&sytue_subject=c2236155-0c17-4d4f-ad5b-6b657d44286a"
	req := parseAuthz(httptest.NewRequest(http.MethodGet, "/oauth2/authorize?"+query, http.NoBody))
	if !validStorageAuthorizationHint(req, client) {
		t.Fatal("provisioned client rejected")
	}
	for _, change := range []func(*domain.Client){
		func(c *domain.Client) { c.ID = "sytue-contacts" },
		func(c *domain.Client) { c.OrgID = "foreign" },
		func(c *domain.Client) { c.Type = domain.ClientPublic },
		func(c *domain.Client) { c.RequirePKCE = false },
		func(c *domain.Client) { c.TokenEndpointAuthMethod = "none" },
	} {
		other := client
		change(&other)
		if validStorageAuthorizationHint(req, other) {
			t.Fatal("untrusted client selected storage context")
		}
	}
	for _, invalid := range []string{query + "&sytue_tenant=beta", "sytue_tenant=", "sytue_subject=", "sytue_tenant=acme", "sytue_subject=c2236155-0c17-4d4f-ad5b-6b657d44286a", "sytue_tenant=acme&sytue_subject=C2236155-0C17-4D4F-AD5B-6B657D44286A"} {
		if validStorageAuthorizationHint(parseAuthz(httptest.NewRequest(http.MethodGet, "/oauth2/authorize?"+invalid, http.NoBody)), client) {
			t.Fatal("ambiguous storage hint accepted")
		}
	}
	if !validStorageAuthorizationHint(authzRequest{}, domain.Client{}) {
		t.Fatal("ordinary authorizations changed")
	}
}
