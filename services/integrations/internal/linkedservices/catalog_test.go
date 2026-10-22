// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package linkedservices

import (
	"errors"
	"reflect"
	"slices"
	"testing"
)

func TestConsentNeverRequestsAnUnselectedOrUnsupportedService(t *testing.T) {
	base := []string{"User.Read", "Mail.ReadWrite", "Mail.Send", "offline_access"}
	plan, err := ConsentPlan("microsoft", base, []string{Contacts, Mail, Contacts})
	if err != nil || !reflect.DeepEqual(plan.Services, []string{Mail, Contacts}) || !slices.Contains(plan.Scopes, "Contacts.ReadWrite") || slices.Contains(plan.Scopes, "Calendars.ReadWrite") || slices.Contains(plan.Scopes, "Notes.ReadWrite") {
		t.Fatalf("scope escalation or missing service: %#v %v", plan, err)
	}
	for _, tc := range []struct {
		provider string
		services []string
	}{{"microsoft", []string{Contacts}}, {"gmail", []string{Mail, Notes}}, {"yahoo", []string{Mail, Notes}}, {"imap", []string{Mail, Calendar}}, {"pop3", []string{Mail, Contacts}}, {"microsoft", []string{Mail, "https://evil.invalid/admin"}}, {"unknown", []string{Mail}}} {
		if _, err := ConsentPlan(tc.provider, base, tc.services); !errors.Is(err, ErrSelection) {
			t.Fatalf("unsupported scope accepted: %#v %v", tc, err)
		}
	}
}

func TestGranularConsentDoesNotEnableAnOmittedPermission(t *testing.T) {
	base := []string{"https://www.googleapis.com/auth/gmail.modify"}
	plan, err := ConsentPlan("gmail", base, []string{Mail, Calendar, Contacts})
	if err != nil {
		t.Fatal(err)
	}
	granted := []string{base[0], "https://www.googleapis.com/auth/contacts"}
	if got := Granted("gmail", plan.Scopes, granted); !reflect.DeepEqual(got, []string{Mail, Contacts}) {
		t.Fatalf("calendar enabled without consent: %v", got)
	}
	if got := Granted("gmail", base, plan.Scopes); !reflect.DeepEqual(got, []string{Mail}) {
		t.Fatalf("old provider grant enabled a deselected service: %v", got)
	}
	if got := Granted("gmail", plan.Scopes, []string{}); len(got) != 0 {
		t.Fatalf("empty grant enabled services: %v", got)
	}
}

func TestMicrosoftRefreshIdentityScopesAreNotResourcePermissions(t *testing.T) {
	base := []string{"User.Read", "Mail.ReadWrite", "Mail.Send", "offline_access"}
	plan, err := ConsentPlan("microsoft", base, []string{Mail, Calendar, Notes})
	if err != nil {
		t.Fatal(err)
	}
	granted := []string{"User.Read", "Mail.ReadWrite", "Mail.Send", "Calendars.ReadWrite", "Notes.ReadWrite"}
	if got := Granted("microsoft", plan.Scopes, granted); !reflect.DeepEqual(got, []string{Mail, Calendar, Notes}) {
		t.Fatalf("offline_access is not a token resource permission: %v", got)
	}
}

func TestYahooServicesUseApprovedOAuthDAVPermissions(t *testing.T) {
	plan, err := ConsentPlan("yahoo", []string{"openid", "email", "mail-r", "mail-w"}, []string{Mail, Calendar, Contacts})
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"ycal-r", "ycal-w", "sdct-r", "sdct-w"} {
		if !slices.Contains(plan.Scopes, scope) {
			t.Fatalf("missing Yahoo permission %s", scope)
		}
	}
	for _, capability := range Offered("yahoo") {
		if capability.Service == Notes && capability.Available {
			t.Fatal("Yahoo notes has no public delegated API")
		}
	}
}

func TestOriginatingAppDoesNotAcquireMailboxPermissions(t *testing.T) {
	for _, provider := range []string{"gmail", "microsoft", "yahoo"} {
		for _, service := range []string{Calendar, Contacts, Notes} {
			if !supported(provider, service) {
				continue
			}
			t.Run(provider+"/"+service, func(t *testing.T) {
				base := append([]string{"openid", "email", "User.Read", "offline_access"}, serviceScopes(provider, Mail)...)
				plan, err := ConsentPlan(provider, base, []string{service}, service)
				if err != nil || !reflect.DeepEqual(plan.Services, []string{service}) {
					t.Fatalf("origin-only rejected: %#v %v", plan, err)
				}
				for _, scope := range serviceScopes(provider, Mail) {
					if slices.Contains(plan.Scopes, scope) {
						t.Errorf("unselected mailbox scope %s", scope)
					}
				}
				if got := Granted(provider, plan.Scopes, nil); !reflect.DeepEqual(got, []string{service}) {
					t.Fatalf("unselected mailbox enabled: %v", got)
				}
				if _, err := ConsentPlan(provider, base, []string{Mail}, service); !errors.Is(err, ErrSelection) {
					t.Fatal("originating service was omitted")
				}
			})
		}
	}
}
