// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/services/integrations/internal/config"
)

func TestConsentAvailabilityRequiresRealCredentialsAndSealedState(t *testing.T) {
	cipher, err := keycrypto.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	const raw = `{"name":"Google Meet","description":"Meeting","color":"#123456","icon":"<svg/>","data":"odata","identity":{"kind":"none"},"launch":{"kind":"none"},"oauth":{"authorize_url":"https://accounts.google.com/o/oauth2/v2/auth","token_url":"https://oauth2.googleapis.com/token","scopes":["meetings.space.created"]}}`
	for _, tc := range []struct {
		name                 string
		db, env              map[string]config.OAuthClient
		own                  map[string]string
		secret, cipher, want bool
	}{
		{name: "catalogue alone", cipher: true},
		{name: "DB platform", db: map[string]config.OAuthClient{"google-workspace": {ClientID: "db-client", ClientSecret: "db-secret"}}, cipher: true, want: true},
		{name: "environment", env: map[string]config.OAuthClient{"google-workspace": {ClientID: "env-client", ClientSecret: "env-secret"}}, cipher: true, want: true},
		{name: "incomplete platform", env: map[string]config.OAuthClient{"google-workspace": {ClientID: "client"}}, cipher: true},
		{name: "own registered app", own: map[string]string{configKeyClientID: "own-client"}, secret: true, cipher: true, want: true},
		{name: "own app without secret", own: map[string]string{configKeyClientID: "own-client"}, cipher: true},
		{name: "no state encryption", own: map[string]string{configKeyClientID: "own-client"}, secret: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := map[string]struct{ ClientID, ClientSecret string }{}
			for key, value := range tc.db {
				db[key] = struct{ ClientID, ClientSecret string }{value.ClientID, value.ClientSecret}
			}
			h := &Handlers{oauthClients: newOAuthClientResolver(&loaderFalso{datos: db}, tc.env)}
			if tc.cipher {
				h.oauthCipher = cipher
			}
			if got := h.oauthAvailable(t.Context(), "google-meet", []byte(raw), tc.own, tc.secret); got != tc.want {
				t.Fatalf("ready=%v want=%v", got, tc.want)
			}
		})
	}
}
