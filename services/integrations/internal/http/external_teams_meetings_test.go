// SPDX-License-Identifier: Apache-2.0
package integrationshttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

const teamsMeetingActor = "12345678-1234-1234-1234-123456789abc"

func externalTestResponse(value any) *http.Response {
	body, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return roomsTestResponse(http.StatusOK, string(body))
}

func TestTeamsConsentMeetingDoesNotReadChatsAndPreservesExistingGrants(t *testing.T) {
	meetingOnly, err := teamsConsentScopes(teamsMeetService, nil, false)
	if err != nil || !slices.Equal(meetingOnly, []string{"offline_access", "User.Read", teamsMeetScope}) {
		t.Fatalf("a meeting-only consent requested unrelated data: %v %v", meetingOnly, err)
	}
	for _, scope := range teamsChatScopes {
		if slices.Contains(meetingOnly, scope) {
			t.Fatal("a new meeting connection reads chats")
		}
	}
	cfg := map[string]string{}
	if err := applyTeamsGrant(cfg, nil, meetingOnly, nil); err != nil || teamsServiceEnabled(cfg, teamsChatService) || !teamsServiceEnabled(cfg, teamsMeetService) {
		t.Fatal("meeting-only grant was lost or enabled chats", err)
	}
	expanded, err := teamsConsentScopes(teamsChatService, cfg, true)
	if err != nil || !slices.Contains(expanded, teamsMeetScope) {
		t.Fatal("chat consent revoked the existing meeting grant", err)
	}
	if err := applyTeamsGrant(cfg, json.RawMessage(`"User.Read Chat.ReadWrite"`), expanded, nil); err == nil {
		t.Fatal("a partial token grant was treated as a complete consent")
	}
	patch, err := narrowedTeamsServices(cfg, json.RawMessage(`"User.Read"`))
	if err != nil || teamsServiceEnabled(patch, teamsMeetService) || teamsServiceEnabled(patch, teamsChatService) {
		t.Fatal("a narrowed refresh retained revoked services", err)
	}
	if _, err := teamsConsentScopes("calendar", nil, false); err == nil {
		t.Fatal("unknown purpose reached OAuth")
	}
}

func protectedMeetingFixture() teamsMeeting {
	protected := true
	meeting := teamsMeeting{ID: "opaque-meeting", URL: "https://teams.microsoft.com/l/meetup-join/opaque", E2EE: &protected}
	meeting.Participants.Organizer.Identity.User.ID = teamsMeetingActor
	return meeting
}

func TestTeamsMeetingOnlyReturnsFreshlyVerifiedProtectedLink(t *testing.T) {
	for _, edit := range []bool{false, true} {
		t.Run(map[bool]string{false: "create", true: "edit"}[edit], func(t *testing.T) {
			request := externalMeetingRequest{Account: teamsMeetingActor}
			if edit {
				request.ID = "opaque-meeting"
			}
			writes, reads := 0, 0
			body := protectedMeetingFixture()
			rt := roomsTestRuntime(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "graph.microsoft.com" {
					t.Fatal("request left the pinned Graph API")
				}
				if r.URL.Path == "/v1.0/me" {
					return externalTestResponse(map[string]string{"id": teamsMeetingActor}), nil
				}
				if r.Method == http.MethodGet {
					reads++
					if r.URL.Path != "/v1.0/me/onlineMeetings/opaque-meeting" {
						t.Fatal("verification read a different meeting")
					}
				} else {
					writes++
					var input map[string]any
					if json.NewDecoder(r.Body).Decode(&input) != nil || input["isEndToEndEncryptionEnabled"] != true {
						t.Fatal("write did not force E2EE")
					}
					if edit && (r.Method != http.MethodPatch || len(input) != 1) {
						t.Fatal("editing protection changed unrelated meeting data")
					}
					if !edit && (r.Method != http.MethodPost || input["subject"] != "Sytue Rooms") {
						t.Fatal("create leaked a Rooms title or used the wrong method")
					}
				}
				return externalTestResponse(body), nil
			})
			result, err := protectedTeamsMeeting(t.Context(), rt, request, time.Now())
			wantReads := 1
			if edit {
				wantReads++
			}
			if err != nil || !result.E2EE || !result.OfficialClient || result.URL != body.URL || writes != 1 || reads != wantReads {
				t.Fatalf("missing authoritative E2EE verification: %+v %v writes=%d reads=%d", result, err, writes, reads)
			}
		})
	}
}

func TestTeamsMeetingNeverDegradesOnFalseMissingIdentityURLOrAmbiguity(t *testing.T) {
	for _, scenario := range []string{"false", "missing", "owner", "url", "id", "permission", "ambiguous", "truncated"} {
		t.Run(scenario, func(t *testing.T) {
			writes := 0
			rt := roomsTestRuntime(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/v1.0/me" {
					return externalTestResponse(map[string]string{"id": teamsMeetingActor}), nil
				}
				if r.Method != http.MethodGet {
					writes++
					switch scenario {
					case "permission":
						return &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader(`{"error":"private-provider-data"}`))}, nil
					case "ambiguous":
						return nil, errors.New("synthetic network loss after acceptance")
					case "truncated":
						return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(`{"id":`))}, nil
					}
					return externalTestResponse(protectedMeetingFixture()), nil
				}
				meeting := protectedMeetingFixture()
				switch scenario {
				case "false":
					no := false
					meeting.E2EE = &no
				case "missing":
					meeting.E2EE = nil
				case "owner":
					meeting.Participants.Organizer.Identity.User.ID = "different-user"
				case "url":
					meeting.URL = "https://teams.microsoft.com.attacker.example/meet/id"
				case "id":
					meeting.ID = "another-meeting"
				}
				return externalTestResponse(meeting), nil
			})
			result, err := protectedTeamsMeeting(context.Background(), rt, externalMeetingRequest{Account: teamsMeetingActor}, time.Now())
			if err == nil || result.URL != "" || result.E2EE || writes != 1 {
				t.Fatalf("unprotected result or replay: %+v %v %d", result, err, writes)
			}
			var apiError *httpx.Error
			if !errors.As(err, &apiError) || strings.Contains(apiError.Message, "private-provider-data") {
				t.Fatal("provider data escaped or error lost its stable code")
			}
			if (scenario == "ambiguous" || scenario == "truncated") && apiError.Code != "external_meeting_unknown" {
				t.Fatal("an uncertain write permits replay")
			}
		})
	}
}

func TestTeamsMeetingRejectsDowngradeAndAccountSwitchBeforeWrite(t *testing.T) {
	for _, scenario := range []string{"false", "account"} {
		no := false
		request := externalMeetingRequest{Account: teamsMeetingActor}
		if scenario == "false" {
			request.E2EE = &no
		}
		writes := 0
		rt := roomsTestRuntime(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet {
				writes++
			}
			return externalTestResponse(map[string]string{"id": "different-user"}), nil
		})
		if result, err := protectedTeamsMeeting(t.Context(), rt, request, time.Now()); err == nil || result.URL != "" || writes != 0 {
			t.Fatal("downgrade/account switch reached a provider write", scenario)
		}
	}
}

func TestTeamsGrantRecognizesOnlyEquivalentMicrosoftGraphScopeFormats(t *testing.T) {
	requested := []string{"offline_access", "User.Read", teamsMeetScope}
	for _, granted := range [][]string{{"user.read", "onlinemeetings.readwrite"}, {"https://graph.microsoft.com/User.Read", "https://graph.microsoft.com/OnlineMeetings.ReadWrite"}} {
		if !slices.Equal(teamsGranted(requested, granted), []string{teamsMeetService}) {
			t.Fatalf("equivalent Graph scope lost: %v", granted)
		}
	}
	if len(teamsGranted(requested, []string{"https://attacker.example/OnlineMeetings.ReadWrite"})) != 0 {
		t.Fatal("another resource impersonated a Graph permission")
	}
}
