// SPDX-License-Identifier: Apache-2.0
package integrationshttp

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestLinkedConsentEnablesOnlyRequestedAndGrantedServices(t *testing.T) {
	requested := []string{"Mail.ReadWrite", "Mail.Send", "Calendars.ReadWrite", "Contacts.ReadWrite"}
	cfg := map[string]string{}
	if err := applyMailServicesGrant("microsoft-mail", cfg, json.RawMessage(`"Mail.ReadWrite Mail.Send Contacts.ReadWrite Notes.ReadWrite"`), requested, nil); err != nil {
		t.Fatal(err)
	}
	if got := connectedMailServices(cfg); !reflect.DeepEqual(got, []string{"mail", "contacts"}) {
		t.Fatalf("omitted calendar or unrequested notes enabled: %v", got)
	}
	for _, raw := range []json.RawMessage{json.RawMessage(`""`), json.RawMessage(`[]`), json.RawMessage(`null`), json.RawMessage(`42`)} {
		if err := applyMailServicesGrant("microsoft-mail", map[string]string{}, raw, requested, nil); err == nil {
			t.Fatalf("invalid/denied Mail grant connected: %s", raw)
		}
	}
}

func TestLinkedRefreshCanNarrowButCannotRestoreDeselectedServices(t *testing.T) {
	cfg := map[string]string{mailServicesKey: `["mail","contacts"]`, mailScopesKey: `["Mail.ReadWrite","Mail.Send","Calendars.ReadWrite","Contacts.ReadWrite"]`}
	patch, err := narrowedMailServices("microsoft", cfg, json.RawMessage(`"Mail.ReadWrite Mail.Send Calendars.ReadWrite"`))
	if err != nil {
		t.Fatal(err)
	}
	if patch[mailServicesKey] != `["mail"]` {
		t.Fatalf("refresh enabled a deselected service: %v", patch)
	}
}

func TestLinkedNativeReturnsUseOnlyTheOriginatingProductCallback(t *testing.T) {
	h := Handlers{}
	for _, product := range []string{"calendar", "contacts", "notes"} {
		section := "native-" + product
		if !mailConsentReturn(section) || !consoleSections[section] || h.consentDestination(section) != "sytue-"+product+"://connect/callback" {
			t.Fatalf("unregistered linked return: %s", section)
		}
		if err := validateConsentTemplate(section, "gmail-mail"); err != nil {
			t.Fatal(err)
		}
		if err := validateConsentTemplate(section, tmplSlackChat); err == nil {
			t.Fatal("linked callback accepted an unrelated template")
		}
	}
	for _, section := range []string{"native-storage", "native-apps", "https://evil.invalid", "native-notes://foreign"} {
		if mailConsentReturn(section) {
			t.Fatalf("unsafe Mail return: %s", section)
		}
	}
}

func TestLinkedWebReturnsUseOnlyConfiguredAppOrigins(t *testing.T) {
	h := Handlers{calendarPublicURL: "https://calendar.sytue.eu/", contactsPublicURL: "https://contacts.sytue.eu", notesPublicURL: "https://notes.sytue.eu"}
	for _, product := range []string{"calendar", "contacts", "notes"} {
		section := "external-" + product
		if !mailConsentReturn(section) || !consoleSections[section] || h.consentDestination(section) != "https://"+product+".sytue.eu/"+product+"/api/linked-services-finish" {
			t.Fatalf("linked web callback lost app: %s", section)
		}
		if err := validateConsentTemplate(section, "microsoft-mail"); err != nil {
			t.Fatal(err)
		}
		if err := validateConsentTemplate(section, tmplSlackChat); err == nil {
			t.Fatal("Mail app callback accepted Slack")
		}
	}
}

func TestLinkedSourceGrantRequiresOnlyItsOriginatingService(t *testing.T) {
	for _, service := range []string{"calendar", "contacts", "notes"} {
		scope := map[string]string{"calendar": "Calendars.ReadWrite", "contacts": "Contacts.ReadWrite", "notes": "Notes.ReadWrite"}[service]
		cfg := map[string]string{}
		if err := applyMailServicesGrant("microsoft-mail", cfg, json.RawMessage(`"`+scope+`"`), []string{scope}, nil, mailConsentService("native-"+service)); err != nil {
			t.Fatalf("origin-only denied: %s %v", service, err)
		}
		if got := connectedMailServices(cfg); !reflect.DeepEqual(got, []string{service}) {
			t.Fatalf("mail granted without consent: %v", got)
		}
		for key, value := range connectedMailCapabilities("microsoft", cfg) {
			if value {
				t.Errorf("ungranted mail capability %s", key)
			}
		}
	}
}
