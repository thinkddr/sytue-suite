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

// B12: una cuenta conectada solo para Calendario no es un buzón en Mail.
func TestExternalMailAccountServesMailOnlyWithTheMailService(t *testing.T) {
	for _, tc := range []struct {
		services []string
		want     bool
	}{{nil, true}, {[]string{"mail", "calendar"}, true}, {[]string{"calendar", "notes"}, false}, {[]string{}, false}} {
		if got := (ExternalMailAccount{Services: tc.services}).ServesMail(); got != tc.want {
			t.Errorf("%v = %v; quiero %v", tc.services, got, tc.want)
		}
	}
}

func TestExternalMailAccountIsICloudByItsIncomingServer(t *testing.T) {
	cuenta := func(provider, host string) ExternalMailAccount {
		a := ExternalMailAccount{Provider: provider}
		a.Configuration = &struct {
			Address  string             `json:"address"`
			Incoming ExternalMailServer `json:"incoming"`
			Outgoing ExternalMailServer `json:"outgoing"`
		}{Incoming: ExternalMailServer{Host: host}}
		return a
	}
	for _, tc := range []struct {
		a    ExternalMailAccount
		want bool
	}{
		{cuenta("imap", "IMAP.mail.me.com"), true},
		{cuenta("imap", "imap.example.eu"), false},
		{cuenta("pop3", "imap.mail.me.com"), false},
		{ExternalMailAccount{Provider: "imap"}, false},
	} {
		if got := tc.a.IsICloud(); got != tc.want {
			t.Errorf("%s %+v: IsICloud = %v, quiero %v", tc.a.Provider, tc.a.Configuration, got, tc.want)
		}
	}
}

// La lista y el lateral nombran una cuenta de iCloud como tal, no como «IMAP».
func TestExternalMailAccountProviderNameNamesICloud(t *testing.T) {
	icloud := ExternalMailAccount{Provider: "imap"}
	icloud.Configuration = &struct {
		Address  string             `json:"address"`
		Incoming ExternalMailServer `json:"incoming"`
		Outgoing ExternalMailServer `json:"outgoing"`
	}{Incoming: ICloudMail.Incoming}
	for a, want := range map[*ExternalMailAccount]string{
		&icloud:             "iCloud Mail",
		{Provider: "imap"}:  "IMAP",
		{Provider: "yahoo"}: "Yahoo Mail",
	} {
		if got := a.ProviderName(); got != want {
			t.Errorf("%s: ProviderName = %q, quiero %q", a.Provider, got, want)
		}
	}
}

// Param lee el `instance_id` que el 409 mail_account_exists trae en `params`, con
// sobre o sin él; lo que no es texto no se inventa.
func TestErrorParamReadsTextParams(t *testing.T) {
	for _, body := range []string{
		`{"error":{"code":"mail_account_exists","params":{"instance_id":"la-de-antes"}}}`,
		`{"code":"mail_account_exists","params":{"instance_id":"la-de-antes"}}`,
	} {
		e := &Error{Status: 409, Body: body}
		if got := e.Param("instance_id"); got != "la-de-antes" {
			t.Errorf("%s → %q", body, got)
		}
	}
	e := &Error{Status: 409, Body: `{"error":{"params":{"instance_id":7}}}`}
	if got := e.Param("instance_id") + e.Param("missing"); got != "" {
		t.Errorf("param que no es texto → %q", got)
	}
}

// El resumen del lateral es un GET sin instancia que trae, por cuenta, su estado y
// sus carpetas con los no leídos. VISTO EN ROJO (2026-11-06): con la ruta mal
// escrita, el servidor de prueba no la reconoce.
func TestMailSummaryTraeCadaCuentaConSusCarpetas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/external-mail/summary" || r.URL.RawQuery != "" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"items":[{"instance_id":"own","provider":"imap","address":"ana@icloud.com","connected":true,"services":["mail"],"configuration":{"incoming":{"host":"imap.mail.me.com"}},"state":"ok","folders":[{"id":"INBOX","name":"INBOX","role":"inbox","unread":4}]},{"instance_id":"old","provider":"gmail","connected":true,"state":"reconnect","folders":[]}]}`))
	}))
	defer server.Close()
	client := NewIntegraciones(server.URL, &sesionFalsa{tokens: []string{"bearer"}}, nil)
	items, err := client.MailSummary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].State != ExternalMailOK || !items[0].IsICloud() || items[0].Folders[0].Unread != 4 || items[1].State != ExternalMailReconnect {
		t.Fatalf("resumen = %+v", items)
	}
}
