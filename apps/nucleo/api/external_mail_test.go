// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMailPersonalConsentProviderOrigins(t *testing.T) {
	for _, tc := range []struct {
		provider, target string
		valid            bool
	}{{"gmail", "https://accounts.google.com/authorize?state=sealed", true}, {"microsoft", "https://login.microsoftonline.com/common/oauth2/v2.0/authorize?state=sealed", true}, {"yahoo", "https://api.login.yahoo.com/oauth2/request_auth?state=sealed", true}, {"gmail", "https://accounts.google.com.evil.invalid/?state=sealed", false}, {"gmail", "https://accounts.google.com/?state=a&state=b", false}, {"gmail", "https://accounts.google.com/", false}} {
		t.Run(tc.provider+tc.target, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]string
				_ = json.NewDecoder(r.Body).Decode(&body)
				if r.Method != http.MethodPost || r.URL.Path != "/api/v1/external-mail/connect" || body["provider"] != tc.provider || body["return_to"] != "native-mail" || body["instance_id"] != "account" {
					t.Errorf("wrong consent route/body %s %#v", r.URL, body)
				}
				_ = json.NewEncoder(w).Encode(map[string]string{"instance_id": "account", "authorize_url": tc.target})
			}))
			defer server.Close()
			client := NewIntegraciones(server.URL, &sesionFalsa{tokens: []string{"bearer"}}, nil)
			got, err := client.MailConnect(context.Background(), tc.provider, "account")
			if tc.valid {
				if err != nil || got.Section != "native-mail" || got.State != "sealed" {
					t.Fatalf("consent %v %v", got, err)
				}
			} else if !errors.Is(err, ErrOAuthState) {
				t.Fatalf("unsafe consent accepted %v", err)
			}
		})
	}
}

func TestMailActionsAcceptNoContentAndSendingIsNotReplayed(t *testing.T) {
	sends := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["expected_address"] != "fixture@example.eu" {
			t.Error("mutating operation lost its original account identity")
		}
		if r.URL.Path == "/api/v1/external-mail/own/send" {
			sends++
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":{"code":"mail_send_unknown"}}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := NewIntegraciones(server.URL, &sesionFalsa{tokens: []string{"bearer"}}, nil)
	if err := client.MailAction(context.Background(), "own", "INBOX", "message", "read", "", "fixture@example.eu"); err != nil {
		t.Fatal(err)
	}
	_, err := client.MailSend(context.Background(), "own", ExternalMailOutgoing{ExpectedAddress: "fixture@example.eu", To: []ExternalMailAddress{{Email: "to@example.eu"}}, Text: "fixture"})
	if err == nil || sends != 1 {
		t.Fatalf("send replayed: %d, %v", sends, err)
	}
}
