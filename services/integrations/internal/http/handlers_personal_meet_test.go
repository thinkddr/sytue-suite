// SPDX-License-Identifier: Apache-2.0
package integrationshttp

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

func TestPersonalMeetNeverChoosesAnArbitraryOrPendingAccount(t *testing.T) {
	active := func(id string) domain.AppInstance {
		return domain.AppInstance{ID: id, TemplateKey: tmplGoogleMeet, Status: domain.StatusActive, HasSecrets: true}
	}
	draft := domain.AppInstance{ID: "draft", TemplateKey: tmplGoogleMeet, Status: domain.StatusDraft}
	other := active("other-provider")
	other.TemplateKey = tmplGoogleChat
	for _, tc := range []struct {
		name, requested, want, code string
		accounts                    []domain.AppInstance
	}{
		{name: "one connected after a newer draft", accounts: []domain.AppInstance{draft, active("a")}, want: "a"},
		{name: "multiple need an explicit choice", accounts: []domain.AppInstance{active("a"), active("b")}, code: "meeting_account_required"},
		{name: "explicit second identity", accounts: []domain.AppInstance{draft, active("a"), active("b")}, requested: "b", want: "b"},
		{name: "pending choice cannot fall back", accounts: []domain.AppInstance{draft, active("a")}, requested: "draft", code: "meeting_not_connected"},
		{name: "unknown or foreign identity", accounts: []domain.AppInstance{active("a")}, requested: "foreign", code: "forbidden"},
		{name: "another provider", accounts: []domain.AppInstance{other, active("a")}, requested: "other-provider", code: "forbidden"},
		{name: "no consent", accounts: []domain.AppInstance{draft}, code: "meeting_not_connected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id, err := personalMeetAccount(tc.accounts, tc.requested)
			if tc.code != "" {
				if err == nil || httpx.AsError(err).Code != tc.code || id != "" {
					t.Fatalf("selected %q, error %v", id, err)
				}
			} else if err != nil || id != tc.want {
				t.Fatalf("selected %q, error %v", id, err)
			}
		})
	}
}

func TestConsentDraftConfigNormalizesNull(t *testing.T) {
	for _, raw := range []string{"", "null", "{}"} {
		cfg, err := validConfig([]byte(raw))
		if err != nil || string(cfg) != "{}" {
			t.Fatalf("draft %q: %s %v", raw, cfg, err)
		}
	}
}
