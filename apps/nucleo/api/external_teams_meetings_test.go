// SPDX-License-Identifier: Apache-2.0
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestProtectedTeamsRequiresVerifiedResultAndExactAccount(t *testing.T) {
	for _, tc := range []struct {
		name   string
		modify func(map[string]any)
		valid  bool
	}{
		{"verified", func(map[string]any) {}, true},
		{"false", func(r map[string]any) { r["e2ee"] = false }, false},
		{"missing", func(r map[string]any) { delete(r, "e2ee") }, false},
		{"client", func(r map[string]any) { r["official_client_required"] = false }, false},
		{"host", func(r map[string]any) { r["url"] = "https://teams.microsoft.com.evil.test/meet/opaque" }, false},
		{"identity", func(r map[string]any) { r["meeting_id"] = "other" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var input map[string]any
				if r.Method != http.MethodPatch || json.NewDecoder(r.Body).Decode(&input) != nil || input["instance_id"] != "selected" || input["expected_account_id"] != "account" || input["meeting_id"] != "opaque" || input["e2ee"] != true {
					t.Error("wrong meeting identity or downgrade")
				}
				output := map[string]any{"provider": "microsoft-teams", "meeting_id": "opaque", "url": "https://teams.microsoft.com/l/meetup-join/opaque", "e2ee": true, "official_client_required": true}
				tc.modify(output)
				_ = json.NewEncoder(w).Encode(output)
			}))
			defer srv.Close()
			result, err := NewIntegraciones(srv.URL, nil, srv.Client()).ProtectedTeamsMeeting(t.Context(), "selected", "account", "opaque")
			if tc.valid && (err != nil || !result.E2EE) || !tc.valid && (err == nil || result.URL != "") {
				t.Fatalf("invalid protected result accepted: %+v %v", result, err)
			}
		})
	}
}

type teamsMeetingOpener struct{ url *url.URL }

func (o *teamsMeetingOpener) OpenURL(u *url.URL) error { o.url = u; return nil }
func TestTeamsMeetingOnlyOpensOfficialClient(t *testing.T) {
	opener := &teamsMeetingOpener{}
	if err := AbrirReunionTeams(opener, "https://teams.microsoft.com/l/meetup-join/opaque?context=encoded"); err != nil || opener.url == nil || opener.url.Scheme != "msteams" || opener.url.Host != "teams.microsoft.com" || opener.url.RawQuery != "context=encoded" {
		t.Fatalf("wrong official app URL: %v %v", opener.url, err)
	}
	for _, u := range []string{"http://teams.microsoft.com/meet/opaque", "https://teams.microsoft.com:443/meet/opaque", "https://user:secret@teams.microsoft.com/meet/opaque", "https://teams.microsoft.com/l/meetup-join/", "https://teams.microsoft.com/meet/opaque#token"} {
		if AbrirReunionTeams(opener, u) == nil {
			t.Fatal("unsafe Teams link accepted")
		}
	}
}

func TestTeamsMeetingStartsNativeConsentWithOnlyMeetingPurpose(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input map[string]any
		if r.URL.Path != "/api/v1/external-conversations/connect" || json.NewDecoder(r.Body).Decode(&input) != nil || input["provider"] != "microsoft-teams" || input["purpose"] != "meeting" || input["instance_id"] != "existing-chat-vault" || input["return_to"] != "native-rooms" {
			t.Errorf("meeting consent borrowed another purpose: %+v", input)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"instance_id": "existing-chat-vault", "authorize_url": "https://login.microsoftonline.com/common/oauth2/v2.0/authorize?state=fixture"})
	}))
	defer srv.Close()
	auth, err := NewIntegraciones(srv.URL, nil, srv.Client()).ExternalMeetingStart(t.Context(), "microsoft-teams", "existing-chat-vault")
	if err != nil || auth.InstanceID != "existing-chat-vault" || auth.Section != "native-rooms" {
		t.Fatalf("meeting consent return changed: %+v %v", auth, err)
	}
}
