// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package linkedservices describes the services a personal provider account can
// connect. It never treats a one-off migration as a live connection.
package linkedservices

import (
	"errors"
	"slices"
)

const (
	Mail     = "mail"
	Calendar = "calendar"
	Contacts = "contacts"
	Notes    = "notes"
)

var ErrSelection = errors.New("linkedservices: unsupported service selection")

var serviceOrder = []string{Mail, Calendar, Contacts, Notes}

// Capability is independent of OAuth client readiness. The account wizard also
// checks that the platform has a complete, approved provider registration.
type Capability struct {
	Service   string `json:"service"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// Offered describes only supported public delegated APIs. Google Keep requires
// Workspace administrator approval; personal Gmail cannot authorise it through
// the ordinary user-consent path. Yahoo exposes no public notes API.
func Offered(provider string) []Capability {
	result := make([]Capability, 0, len(serviceOrder))
	for _, service := range serviceOrder {
		available := supported(provider, service)
		reason := ""
		if !available {
			reason = "not_supported"
			if provider == "gmail" && service == Notes {
				reason = "workspace_admin_required"
			}
		}
		result = append(result, Capability{Service: service, Available: available, Reason: reason})
	}
	return result
}

func supported(provider, service string) bool {
	switch provider {
	case "gmail", "yahoo":
		return service == Mail || service == Calendar || service == Contacts
	case "microsoft":
		return slices.Contains(serviceOrder, service)
	case "imap", "pop3":
		return service == Mail
	default:
		return false
	}
}

type Plan struct {
	Services []string
	Scopes   []string
}

// ConsentPlan requires the originating service. Its default preserves the legacy
// Mail-only API; another app requests identity and its selected resources only.
func ConsentPlan(provider string, baseScopes, requested []string, origin ...string) (Plan, error) {
	required := Mail
	if len(origin) > 0 {
		required = origin[0]
	}
	if requested == nil {
		requested = []string{required}
	}
	if !supported(provider, required) || !slices.Contains(requested, required) {
		return Plan{}, ErrSelection
	}
	for _, service := range requested {
		if !supported(provider, service) {
			return Plan{}, ErrSelection
		}
	}
	plan := Plan{Scopes: slices.Clone(baseScopes)}
	if required != Mail {
		switch provider {
		case "gmail", "yahoo":
			plan.Scopes = []string{"openid", "email"}
		case "microsoft":
			plan.Scopes = []string{"User.Read", "offline_access"}
		}
	}
	for _, service := range serviceOrder {
		if !slices.Contains(requested, service) {
			continue
		}
		plan.Services = append(plan.Services, service)
		for _, scope := range serviceScopes(provider, service) {
			if !slices.Contains(plan.Scopes, scope) {
				plan.Scopes = append(plan.Scopes, scope)
			}
		}
	}
	return plan, nil
}

// Granted intersects the sealed request with the scopes returned by the token
// endpoint. A missing scope member means the original request, per RFC 6749;
// an explicitly empty grant never enables a service.
func Granted(provider string, requestedScopes, grantedScopes []string) []string {
	if grantedScopes == nil {
		grantedScopes = requestedScopes
	}
	result := []string{}
	for _, service := range serviceOrder {
		if !supported(provider, service) {
			continue
		}
		required := serviceScopes(provider, service)
		if len(required) > 0 && containsAll(requestedScopes, required) && containsAll(grantedScopes, required) {
			result = append(result, service)
		}
	}
	return result
}

func containsAll(scopes, required []string) bool {
	for _, scope := range required {
		if !slices.Contains(scopes, scope) {
			return false
		}
	}
	return true
}

func serviceScopes(provider, service string) []string {
	switch provider + "/" + service {
	case "gmail/mail":
		return []string{"https://www.googleapis.com/auth/gmail.modify"}
	case "microsoft/mail":
		return []string{"Mail.ReadWrite", "Mail.Send"}
	case "yahoo/mail":
		return []string{"mail-r", "mail-w"}
	case "gmail/calendar":
		return []string{"https://www.googleapis.com/auth/calendar.calendarlist.readonly", "https://www.googleapis.com/auth/calendar.events"}
	case "gmail/contacts":
		return []string{"https://www.googleapis.com/auth/contacts"}
	case "microsoft/calendar":
		return []string{"Calendars.ReadWrite"}
	case "microsoft/contacts":
		return []string{"Contacts.ReadWrite"}
	case "microsoft/notes":
		return []string{"Notes.ReadWrite"}
	case "yahoo/calendar":
		return []string{"ycal-r", "ycal-w"}
	case "yahoo/contacts":
		return []string{"sdct-r", "sdct-w"}
	default:
		return nil
	}
}
