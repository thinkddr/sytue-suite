// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestExternalChatUsesPersonalConsentAndProviderOrigin(t *testing.T) {
	for _, tc := range []struct {
		provider, target string
		valid            bool
	}{
		{"slack", "https://slack.com/oauth/v2/authorize?state=nonce", true},
		{"microsoft-teams", "https://login.microsoftonline.com/common/oauth2/v2.0/authorize?state=nonce", true},
		{"google-chat", "https://accounts.google.com/o/oauth2/v2/auth?state=nonce", true},
		{"slack", "https://slack.com.evil.example/?state=nonce", false},
		{"slack", "https://user:secret@slack.com/?state=nonce", false},
		{"slack", "https://slack.com:8443/?state=nonce", false},
		{"slack", "http://slack.com/?state=nonce", false},
		{"slack", "https://slack.com/?state=a&state=b", false},
		{"slack", "https://slack.com/?state=nonce#secret", false},
		{"slack", "https://accounts.google.com/?state=nonce", false},
		{"slack", "https://slack.com/", false},
	} {
		t.Run(tc.provider+tc.target, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/v1/external-conversations/connect" || r.Header.Get("Authorization") == "" {
					t.Errorf("wrong personal route: %s %s", r.Method, r.URL.Path)
				}
				var in map[string]string
				if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
					t.Error(err)
				}
				if len(in) != 2 || in["provider"] != tc.provider || in["return_to"] != "native-rooms" {
					t.Errorf("consent body: %+v", in)
				}
				_ = json.NewEncoder(w).Encode(map[string]string{"instance_id": "own-account", "authorize_url": tc.target})
			}))
			defer srv.Close()
			gateway := NewIntegraciones(srv.URL, &sesionFalsa{tokens: []string{"bearer"}}, nil)
			got, err := gateway.ExternalChatStart(context.Background(), tc.provider, "")
			if tc.valid {
				if err != nil || got.State != "nonce" || got.Section != "native-rooms" {
					t.Fatalf("%+v %v", got, err)
				}
			} else if !errors.Is(err, ErrOAuthState) {
				t.Fatalf("unsafe consent accepted: %+v %v", got, err)
			}
		})
	}
}

func TestMeetEnviaLaCuentaElegidaSinRomperLaLlamadaAnterior(t *testing.T) {
	for _, instancia := range []string{"", "cuenta-elegida"} {
		t.Run(instancia, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/v1/external-conversations/meeting" {
					t.Errorf("petición inesperada %s %s", r.Method, r.URL.Path)
				}
				var body map[string]string
				if json.NewDecoder(r.Body).Decode(&body) != nil || body["provider"] != "google-meet" || body["instance_id"] != instancia {
					t.Errorf("la reunión usa otra identidad: %v", body)
				}
				if instancia == "" && len(body) != 1 {
					t.Error("el cliente antiguo ya no omite la cuenta")
				}
				_, _ = w.Write([]byte(`{"url":"https://meet.google.com/abc-defg-hij"}`))
			}))
			defer server.Close()
			client := NewIntegraciones(server.URL, nil, server.Client())
			var err error
			if instancia == "" {
				_, err = client.ExternalMeetingCreate(t.Context())
			} else {
				_, err = client.ExternalMeetingCreate(t.Context(), instancia)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestExternalChatThreadIdentifiersRemainDataAndWritesAreNotReplayed(t *testing.T) {
	var sends atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/external-conversations/own/messages" {
			t.Errorf("unexpected route %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("missing session")
		}
		if r.Method == http.MethodGet {
			if r.URL.Query().Get("remote_id") != "team/channel?other=1" || r.URL.Query().Get("thread_id") != "thread/+?&" || r.URL.Query().Get("cursor") != "cursor/+=" {
				t.Error("query escaped as routing")
			}
			_, _ = w.Write([]byte(`{"items":[{"id":"one","remote_id":"team/channel?other=1","owned":true,"text":"external","created_at":"2026-10-16T10:00:00Z"}],"capabilities":{"read":true,"send":true}}`))
			return
		}
		sends.Add(1)
		var in ExternalChatWrite
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.RemoteID != "team/channel?other=1" || in.ThreadID != "thread/+?&" || in.Text != "reply" {
			t.Errorf("wrong write %+v", in)
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	gateway := NewIntegraciones(srv.URL, &sesionFalsa{tokens: []string{"bearer"}}, nil)
	got, err := gateway.ExternalChatMessages(context.Background(), "own", "team/channel?other=1", "cursor/+=", "thread/+?&")
	if err != nil || len(got.Items) != 1 || !got.Items[0].Owned || !got.Capabilities.Send {
		t.Fatalf("read %+v %v", got, err)
	}
	_, err = gateway.ExternalChatSend(context.Background(), "own", ExternalChatWrite{RemoteID: "team/channel?other=1", ThreadID: "thread/+?&", Text: "reply"})
	if err == nil || sends.Load() != 1 {
		t.Fatalf("ambiguous write replayed: attempts %d, %v", sends.Load(), err)
	}
	if err = gateway.ExternalChatAction(context.Background(), "own", ExternalChatAction{Action: "forward-native"}); err == nil {
		t.Fatal("unsupported external action accepted")
	}
	if sends.Load() != 1 {
		t.Fatal("invalid action reached provider")
	}
}

func TestProviderCooldownReadsWrappedErrorAndIsBounded(t *testing.T) {
	for _, tc := range []struct {
		body string
		want time.Duration
	}{
		{`{"error":{"code":"rate_limited","params":{"retry_after_seconds":75}}}`, 75 * time.Second},
		{`{"params":{"retry_after_seconds":9000000}}`, 24 * time.Hour},
		{`{"params":{"retry_after_seconds":-1}}`, 0},
		{`{"params":{"retry_after_seconds":"75"}}`, 0},
		{`not-json`, 0},
	} {
		if got := (&Error{Body: tc.body}).RetryAfter(); got != tc.want {
			t.Errorf("%s: got %s want %s", strings.TrimSpace(tc.body), got, tc.want)
		}
	}
}
