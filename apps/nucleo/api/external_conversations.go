// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// External conversations stay in the consenting actor's provider account. Native
// encrypted Rooms messages and provider credentials never enter these DTOs.
type ExternalChatCapabilities struct {
	Read        bool `json:"read"`
	Send        bool `json:"send"`
	Threads     bool `json:"threads"`
	Edit        bool `json:"edit"`
	Delete      bool `json:"delete"`
	Attachments bool `json:"attachments"`
	E2EE        bool `json:"e2ee"`
}
type ExternalChatAccount struct {
	InstanceID    string `json:"instance_id"`
	Provider      string `json:"provider"`
	Name          string `json:"name"`
	Connected     bool   `json:"connected"`
	WorkspaceName string `json:"workspace_name"`
	AccountID     string `json:"account_id"`
}
type ExternalChatProvider struct {
	Provider      string                   `json:"provider"`
	Available     bool                     `json:"available"`
	DisabledByOrg bool                     `json:"disabled_by_org"`
	Capabilities  ExternalChatCapabilities `json:"capabilities"`
}
type ExternalChatAccounts struct {
	Items     []ExternalChatAccount  `json:"items"`
	Providers []ExternalChatProvider `json:"providers"`
	Meetings  []ExternalMeeting      `json:"meetings"`
}

// ExternalMeeting is the exact personal Google Meet or Teams meeting account.
type ExternalMeeting struct {
	Provider      string `json:"provider"`
	Name          string `json:"name,omitempty"`
	Available     bool   `json:"available"`
	Connected     bool   `json:"connected"`
	InstanceID    string `json:"instance_id"`
	AccountID     string `json:"account_id"`
	E2EERequired  bool   `json:"e2ee_required"`
	DisabledByOrg bool   `json:"disabled_by_org"`
}

type ExternalMeetingResult struct {
	Provider               string `json:"provider"`
	ID                     string `json:"meeting_id"`
	URL                    string `json:"url"`
	E2EE                   bool   `json:"e2ee"`
	OfficialClientRequired bool   `json:"official_client_required"`
}
type ExternalChatChannel struct {
	ID           string                   `json:"id"`
	Name         string                   `json:"name"`
	Kind         string                   `json:"kind"`
	Capabilities ExternalChatCapabilities `json:"capabilities"`
}
type ExternalChatChannels struct {
	Items      []ExternalChatChannel `json:"items"`
	NextCursor string                `json:"next_cursor"`
}
type ExternalChatMessage struct {
	ID         string    `json:"id"`
	RemoteID   string    `json:"remote_id"`
	AuthorID   string    `json:"author_id"`
	AuthorName string    `json:"author_name"`
	Text       string    `json:"text"`
	CreatedAt  time.Time `json:"created_at"`
	ThreadID   string    `json:"thread_id"`
	ReplyCount int       `json:"reply_count"`
	Edited     bool      `json:"edited"`
	Owned      bool      `json:"owned"`
}
type ExternalChatMessages struct {
	Items        []ExternalChatMessage    `json:"items"`
	NextCursor   string                   `json:"next_cursor"`
	Capabilities ExternalChatCapabilities `json:"capabilities"`
}
type ExternalChatWrite struct {
	RemoteID string `json:"remote_id"`
	Text     string `json:"text"`
	ThreadID string `json:"thread_id,omitempty"`
}
type ExternalChatAction struct {
	Action    string `json:"action"`
	RemoteID  string `json:"remote_id"`
	MessageID string `json:"message_id"`
	ThreadID  string `json:"thread_id,omitempty"`
	Text      string `json:"text,omitempty"`
}

func (i *Integraciones) ExternalChatAccounts(ctx context.Context) (ExternalChatAccounts, error) {
	return JSON[ExternalChatAccounts](ctx, i.c, Request{Path: "/external-conversations/accounts"})
}

// ExternalChatStart uses the personal gateway flow; generic instance creation is
// reserved for organisation administration and must not be used here.
func (i *Integraciones) ExternalChatStart(ctx context.Context, provider, instance string) (OAuthAuthorization, error) {
	return i.externalConsent(ctx, provider, instance, "")
}

func (i *Integraciones) ExternalMeetingStart(ctx context.Context, provider, instance string) (OAuthAuthorization, error) {
	if provider == "google-meet" {
		return i.ExternalChatStart(ctx, provider, instance)
	}
	if provider != "microsoft-teams" {
		return OAuthAuthorization{}, ErrOAuthState
	}
	return i.externalConsent(ctx, provider, instance, "meeting")
}

func (i *Integraciones) externalConsent(ctx context.Context, provider, instance, purpose string) (OAuthAuthorization, error) {
	var host string
	switch provider {
	case "slack":
		host = "slack.com"
	case "microsoft-teams":
		host = "login.microsoftonline.com"
	case "google-chat", "google-meet":
		host = "accounts.google.com"
	default:
		return OAuthAuthorization{}, ErrOAuthState
	}
	result, err := PostJSON[struct {
		InstanceID   string `json:"instance_id"`
		AuthorizeURL string `json:"authorize_url"`
	}](ctx, i.c, "/external-conversations/connect", struct {
		Provider string `json:"provider"`
		Instance string `json:"instance_id,omitempty"`
		ReturnTo string `json:"return_to"`
		Purpose  string `json:"purpose,omitempty"`
	}{provider, instance, "native-rooms", purpose})
	if err != nil {
		return OAuthAuthorization{}, err
	}
	return autorizacionPersonal(result.AuthorizeURL, result.InstanceID, host, "native-rooms")
}

// ProtectedTeamsMeeting validates the confirmation independently of the gateway.
// An uncertain error must be inspected in Teams; calling this again creates anew.
func (i *Integraciones) ProtectedTeamsMeeting(ctx context.Context, instance, account, meeting string) (ExternalMeetingResult, error) {
	if instance == "" || account == "" {
		return ExternalMeetingResult{}, ErrOAuthState
	}
	method := http.MethodPost
	if meeting != "" {
		method = http.MethodPatch
	}
	body, err := json.Marshal(struct {
		Provider string `json:"provider"`
		Instance string `json:"instance_id"`
		Account  string `json:"expected_account_id"`
		Meeting  string `json:"meeting_id,omitempty"`
		E2EE     bool   `json:"e2ee"`
	}{"microsoft-teams", instance, account, meeting, true})
	if err != nil {
		return ExternalMeetingResult{}, err
	}
	result, err := JSON[ExternalMeetingResult](ctx, i.c, Request{Method: method, Path: "/external-conversations/meeting", Body: body, Header: http.Header{"Content-Type": {"application/json"}}})
	if err != nil {
		return ExternalMeetingResult{}, err
	}
	if result.Provider != "microsoft-teams" || !result.E2EE || !result.OfficialClientRequired || result.ID == "" || meeting != "" && result.ID != meeting || !validTeamsMeetingLink(result.URL) {
		return ExternalMeetingResult{}, errors.New("external meeting: protected Teams confirmation invalid")
	}
	return result, nil
}

func validTeamsMeetingLink(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "teams.microsoft.com" && u.User == nil && u.Fragment == "" &&
		(strings.HasPrefix(u.Path, "/l/meetup-join/") && len(u.Path) > len("/l/meetup-join/") || strings.HasPrefix(u.Path, "/meet/") && len(u.Path) > len("/meet/"))
}

// autorizacionPersonal valida la URL de consentimiento que devuelve la pasarela para
// una cuenta personal: https, el host exacto del proveedor y un único `state`.
func autorizacionPersonal(authorizeURL, instanceID, host, section string) (OAuthAuthorization, error) {
	target, err := url.Parse(authorizeURL)
	if err != nil || target.Scheme != "https" || target.Host != host || target.User != nil || target.Fragment != "" || target.Opaque != "" || instanceID == "" {
		return OAuthAuthorization{}, ErrOAuthState
	}
	state := target.Query().Get("state")
	if state == "" || len(state) > 4096 || len(target.Query()["state"]) != 1 {
		return OAuthAuthorization{}, ErrOAuthState
	}
	return OAuthAuthorization{InstanceID: instanceID, AuthorizeURL: target.String(), State: state, Section: section}, nil
}

// ExternalMeetingCreate pide un enlace nuevo de Google Meet con la cuenta personal.
func (i *Integraciones) ExternalMeetingCreate(ctx context.Context, instance ...string) (string, error) {
	if len(instance) > 1 {
		return "", ErrOAuthState
	}
	account := ""
	if len(instance) == 1 {
		account = instance[0]
	}
	result, err := PostJSON[struct {
		URL string `json:"url"`
	}](ctx, i.c, "/external-conversations/meeting", struct {
		Provider string `json:"provider"`
		Instance string `json:"instance_id,omitempty"`
	}{"google-meet", account})
	if err != nil {
		return "", err
	}
	u, err := url.Parse(result.URL)
	if err != nil || u.Scheme != "https" || u.Host != "meet.google.com" || u.User != nil {
		return "", fmt.Errorf("integraciones: enlace de Meet no válido")
	}
	return u.String(), nil
}

func (i *Integraciones) ExternalChatChannels(ctx context.Context, instance, cursor string) (ExternalChatChannels, error) {
	return JSON[ExternalChatChannels](ctx, i.c, Request{Path: ruta("/external-conversations/", instance, "/channels"), Query: url.Values{"cursor": {cursor}}})
}

func (i *Integraciones) ExternalChatMessages(ctx context.Context, instance, remote, cursor, thread string) (ExternalChatMessages, error) {
	query := url.Values{"remote_id": {remote}, "cursor": {cursor}}
	if thread != "" {
		query.Set("thread_id", thread)
	}
	return JSON[ExternalChatMessages](ctx, i.c, Request{Path: ruta("/external-conversations/", instance, "/messages"), Query: query})
}

func (i *Integraciones) ExternalChatSend(ctx context.Context, instance string, in ExternalChatWrite) (ExternalChatMessage, error) {
	return PostJSON[ExternalChatMessage](ctx, i.c, ruta("/external-conversations/", instance, "/messages"), in)
}

func (i *Integraciones) ExternalChatAction(ctx context.Context, instance string, in ExternalChatAction) error {
	if in.Action != "edit" && in.Action != "delete" {
		return errors.New("external chat: unsupported action")
	}
	_, err := PostJSON[map[string]any](ctx, i.c, ruta("/external-conversations/", instance, "/actions"), in)
	return err
}

func (i *Integraciones) ExternalChatDisconnect(ctx context.Context, instance string) error {
	res, err := i.c.Do(ctx, Request{Method: http.MethodDelete, Path: ruta("/external-conversations/", instance, "")})
	if err != nil {
		return err
	}
	return res.Body.Close()
}
