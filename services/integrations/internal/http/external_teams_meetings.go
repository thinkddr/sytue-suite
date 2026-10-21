// SPDX-License-Identifier: Apache-2.0
package integrationshttp

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
)

type externalMeetingRequest struct {
	Provider string `json:"provider,omitempty"`
	Instance string `json:"instance_id,omitempty"`
	Account  string `json:"expected_account_id,omitempty"`
	ID       string `json:"meeting_id,omitempty"`
	E2EE     *bool  `json:"e2ee,omitempty"`
}

type externalMeetingResult struct {
	Provider       string `json:"provider"`
	ID             string `json:"meeting_id,omitempty"`
	URL            string `json:"url"`
	E2EE           bool   `json:"e2ee"`
	OfficialClient bool   `json:"official_client_required"`
}

type teamsMeeting struct {
	ID           string `json:"id"`
	URL          string `json:"joinWebUrl"`                  //nolint:tagliatelle // Graph contract
	E2EE         *bool  `json:"isEndToEndEncryptionEnabled"` //nolint:tagliatelle // Graph contract
	Participants struct {
		Organizer struct {
			Identity struct {
				User struct {
					ID string `json:"id"`
				} `json:"user"`
			} `json:"identity"`
		} `json:"organizer"`
	} `json:"participants"`
}

func validTeamsMeetingID(id string) bool {
	return id != "" && len(id) <= 2048 && !strings.ContainsAny(id, "\r\n\x00?#")
}

func validTeamsMeetingURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "teams.microsoft.com" && u.User == nil && u.Fragment == "" &&
		(strings.HasPrefix(u.Path, "/l/meetup-join/") && len(u.Path) > len("/l/meetup-join/") ||
			strings.HasPrefix(u.Path, "/meet/") && len(u.Path) > len("/meet/"))
}

// Create and update request E2EE explicitly. No URL leaves the gateway unless a
// fresh GET confirms the same meeting, organizer and encryption property.
func protectedTeamsMeeting(ctx context.Context, rt connector.Runtime, req externalMeetingRequest, now time.Time) (externalMeetingResult, error) {
	if _, err := uuid.Parse(req.Account); err != nil || req.E2EE != nil && !*req.E2EE {
		return externalMeetingResult{}, httpx.ErrBadRequest
	}
	var who struct {
		ID string `json:"id"`
	}
	if err := roomsProviderJSON(ctx, rt, http.MethodGet, roomsGraphAPI+"/me?$select=id", nil, &who); err != nil {
		return externalMeetingResult{}, err
	}
	if who.ID != req.Account {
		return externalMeetingResult{}, httpx.ErrConflict.WithCode("external_reconnect_required")
	}
	created, err := writeProtectedTeamsMeeting(ctx, rt, req, now)
	if err != nil {
		return externalMeetingResult{}, err
	}
	var verified teamsMeeting
	if err := roomsProviderJSON(ctx, rt, http.MethodGet, roomsGraphAPI+"/me/onlineMeetings/"+url.PathEscape(created.ID), nil, &verified); err != nil {
		return externalMeetingResult{}, httpx.ErrConflict.WithCode("external_meeting_unknown")
	}
	if verified.ID != created.ID || verified.Participants.Organizer.Identity.User.ID != req.Account {
		return externalMeetingResult{}, httpx.ErrForbidden.WithCode("external_meeting_unprotected")
	}
	if verified.E2EE == nil || !*verified.E2EE || !validTeamsMeetingURL(verified.URL) {
		return externalMeetingResult{}, httpx.ErrConflict.WithCode("external_meeting_unprotected").WithMessage("Microsoft Teams did not confirm end-to-end encryption. Check the organizer's Teams Premium license and encryption policy.")
	}
	return externalMeetingResult{Provider: "microsoft-teams", ID: verified.ID, URL: verified.URL, E2EE: true, OfficialClient: true}, nil
}

func writeProtectedTeamsMeeting(ctx context.Context, rt connector.Runtime, req externalMeetingRequest, now time.Time) (teamsMeeting, error) {
	target, method := roomsGraphAPI+"/me/onlineMeetings", http.MethodPost
	body := map[string]any{"isEndToEndEncryptionEnabled": true}
	if req.ID == "" {
		body["subject"] = "Sytue Rooms"
		body["startDateTime"] = now.UTC().Format(time.RFC3339)
		body["endDateTime"] = now.Add(time.Hour).UTC().Format(time.RFC3339)
	} else {
		if !validTeamsMeetingID(req.ID) {
			return teamsMeeting{}, httpx.ErrBadRequest
		}
		target += "/" + url.PathEscape(req.ID)
		method = http.MethodPatch
		var original teamsMeeting
		if err := roomsProviderJSON(ctx, rt, http.MethodGet, target, nil, &original); err != nil {
			return teamsMeeting{}, err
		}
		if original.ID != req.ID || original.Participants.Organizer.Identity.User.ID != req.Account {
			return teamsMeeting{}, httpx.ErrForbidden
		}
	}
	var created teamsMeeting
	if err := roomsProviderJSON(ctx, rt, method, target, body, &created); err != nil {
		var apiError *httpx.Error
		if errors.As(err, &apiError) && (apiError.Code == "rooms_provider_permission" || apiError.Status == http.StatusTooManyRequests) {
			return teamsMeeting{}, err
		}
		return teamsMeeting{}, httpx.ErrConflict.WithCode("external_meeting_unknown").WithMessage("The meeting request could not be confirmed. Check Microsoft Teams before creating another meeting.")
	}
	if !validTeamsMeetingID(created.ID) || req.ID != "" && req.ID != created.ID {
		return teamsMeeting{}, httpx.ErrConflict.WithCode("external_meeting_unknown")
	}
	return created, nil
}
