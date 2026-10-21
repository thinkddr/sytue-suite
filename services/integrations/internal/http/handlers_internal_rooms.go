// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
)

const (
	tmplSlackRooms          = "slack-rooms"
	tmplTeamsRooms          = "microsoft-teams-rooms"
	tmplGoogleChat          = "google-chat"
	tmplGoogleMeet          = "google-meet"
	roomsGraphAPI           = "https://graph.microsoft.com/v1.0"
	roomsChatAPI            = "https://chat.googleapis.com/v1/"
	roomsMeetAPI            = "https://meet.googleapis.com/v2/spaces"
	roomsSlackWebhookOrigin = "https://hooks.slack.com"
	secretKeyRoomsWebhook   = "rooms_webhook" //nolint:gosec // nombre de la clave del vault
	roomsChannelID          = "_rooms_channel_id"
	roomsChannelName        = "_rooms_channel_name"
	roomsTeamID             = "_rooms_team_id"
	roomsTeamName           = "_rooms_team_name"
	maxRoomsDestinations    = 500
)

var roomsSpaceName = regexp.MustCompile(`^spaces/[A-Za-z0-9_-]+$`)

func esPlantillaDeRooms(key string) bool {
	return key == tmplSlackRooms || key == tmplTeamsRooms || key == tmplGoogleChat || key == tmplGoogleMeet || externalChatTemplate(key)
}

type roomsDestination struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// InternalRoomsDestinations enumera únicamente los destinos de la cuenta que
// dio consentimiento. La URL del webhook y el token nunca salen de la pasarela.
func (h *Handlers) InternalRoomsDestinations(w http.ResponseWriter, r *http.Request) {
	t, err := tenantDe(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := tenancy.Into(r.Context(), t)
	conn, _, err := h.oauthConnection(ctx, r.URL.Query().Get("instance"), esPlantillaDeRooms)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, err := listRoomsDestinations(ctx, conn, h.egressRuntime(conn, log.From(ctx)))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

type roomsInternalRequest struct {
	Tenant    string `json:"tenant"`
	Instance  string `json:"instance"`
	RemoteID  string `json:"remote_id"`
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id,omitempty"`
}

func roomsRequestTenant(req roomsInternalRequest) (tenancy.Tenant, error) {
	t, err := tenancy.New(req.Tenant)
	if err != nil || req.Instance == "" {
		return tenancy.Tenant{}, httpx.ErrBadRequest.WithMessage("tenant and instance are required")
	}
	return t, nil
}

// InternalRoomsMessage emits only a generic notice and authenticated Rooms link.
// The contract accepts identifiers, never conversation content or encryption keys.
func (h *Handlers) InternalRoomsMessage(w http.ResponseWriter, r *http.Request) {
	var req roomsInternalRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, err := roomsRequestTenant(req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := tenancy.Into(r.Context(), t)
	notice := hubnotify.AvisoMensajeRooms(h.roomsPublicURL, t.Slug, req.ChannelID, req.MessageID)
	if notice == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("A valid Rooms notification destination is required."))
		return
	}
	conn, _, err := h.oauthConnection(ctx, req.Instance, esPlantillaDeRooms)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := sendRoomsMessage(ctx, conn, h.egressRuntime(conn, log.From(ctx)), req.RemoteID, notice); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// InternalRoomsMeeting crea un espacio real de Google Meet. Lo que recibe Rooms
// es su enlace, para que el cliente lo abra o lo comparta tras cifrarlo.
func (h *Handlers) InternalRoomsMeeting(w http.ResponseWriter, r *http.Request) {
	var req roomsInternalRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, err := roomsRequestTenant(req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := tenancy.Into(r.Context(), t)
	conn, _, err := h.oauthConnection(ctx, req.Instance, func(key string) bool { return key == tmplGoogleMeet })
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	meetingURL, err := createRoomsMeeting(ctx, h.egressRuntime(conn, log.From(ctx)))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]string{"url": meetingURL})
}

// roomsProviderJSON usa siempre el runtime con SSRF, política, auditoría y auth
// del vault. Un error del proveedor nunca devuelve su cuerpo ni una URL secreta.
func roomsProviderJSON(ctx context.Context, rt connector.Runtime, method, target string, body, out any) error {
	var reader io.Reader = http.NoBody
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return httpx.ErrInternal
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return httpx.ErrInternal
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := rt.HTTP.Do(req)
	if err != nil {
		return httpx.NewError(http.StatusBadGateway, "rooms_provider_unreachable", "The connected app could not be reached.")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
			return httpx.NewError(http.StatusUnprocessableEntity, "rooms_provider_permission", "The connected account cannot access this destination. Check its permissions or reconnect it.")
		}
		if res.StatusCode == http.StatusTooManyRequests {
			seconds, parseErr := strconv.Atoi(res.Header.Get("Retry-After"))
			if parseErr != nil || seconds < 1 {
				seconds = 60
			}
			if seconds > 86400 {
				seconds = 86400
			}
			return httpx.NewError(http.StatusTooManyRequests, "external_rate_limited", "The connected app is busy. Try again shortly.").WithParams(map[string]any{"retry_after_seconds": seconds})
		}
		return httpx.NewError(http.StatusBadGateway, "rooms_provider_rejected", "The connected app did not accept the request.")
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out); err != nil {
		return httpx.NewError(http.StatusBadGateway, "rooms_provider_response", "The connected app returned an unreadable response.")
	}
	return nil
}

func listRoomsDestinations(ctx context.Context, conn connector.Instance, rt connector.Runtime) ([]roomsDestination, error) {
	switch conn.TemplateKey {
	case tmplSlackRooms:
		if !validSlackRoomsWebhook(conn.Secrets[secretKeyRoomsWebhook]) || conn.Config[roomsChannelID] == "" {
			return nil, httpx.ErrConflict.WithMessage("Connect Slack again and choose a channel.")
		}
		name := conn.Config[roomsChannelName]
		if name == "" {
			name = conn.Config[roomsChannelID]
		}
		if team := conn.Config[roomsTeamName]; team != "" {
			name = team + " / " + name
		}
		return []roomsDestination{{ID: conn.Config[roomsChannelID], Name: name}}, nil
	case tmplGoogleChat:
		return listGoogleRooms(ctx, rt)
	case tmplTeamsRooms:
		return listTeamsRooms(ctx, rt)
	case tmplGoogleMeet:
		return []roomsDestination{}, nil
	default:
		return nil, httpx.ErrNotFound
	}
}

func listGoogleRooms(ctx context.Context, rt connector.Runtime) ([]roomsDestination, error) {
	items := make([]roomsDestination, 0, 100)
	pageToken := ""
	for page := 0; page < 20 && len(items) < maxRoomsDestinations; page++ {
		var out struct {
			Spaces []struct {
				Name        string `json:"name"`
				DisplayName string `json:"displayName"` //nolint:tagliatelle // contrato de Google Chat
			} `json:"spaces"` //nolint:tagliatelle // contrato de Google Chat
			NextPageToken string `json:"nextPageToken"` //nolint:tagliatelle // contrato de Google Chat
		}
		q := url.Values{"pageSize": {"100"}}
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		if err := roomsProviderJSON(ctx, rt, http.MethodGet, roomsChatAPI+"spaces?"+q.Encode(), nil, &out); err != nil {
			return nil, err
		}
		for _, space := range out.Spaces {
			if !roomsSpaceName.MatchString(space.Name) {
				continue
			}
			name := space.DisplayName
			if name == "" {
				name = space.Name
			}
			items = append(items, roomsDestination{ID: space.Name, Name: name})
			if len(items) == maxRoomsDestinations {
				break
			}
		}
		if out.NextPageToken == "" || out.NextPageToken == pageToken {
			break
		}
		pageToken = out.NextPageToken
	}
	return items, nil
}

type graphRoom struct {
	ID   string `json:"id"`
	Name string `json:"displayName"` //nolint:tagliatelle // contrato de Graph
}

func listGraphRooms(ctx context.Context, rt connector.Runtime, target string) ([]graphRoom, error) {
	items := make([]graphRoom, 0, 100)
	for page := 0; target != "" && page < 20 && len(items) < maxRoomsDestinations; page++ {
		if !validRoomsGraphURL(target) {
			return nil, httpx.ErrBadRequest.WithMessage("Invalid Teams page.")
		}
		var out struct {
			Value []graphRoom `json:"value"`
			Next  string      `json:"@odata.nextLink"` //nolint:tagliatelle // contrato de Graph
		}
		if err := roomsProviderJSON(ctx, rt, http.MethodGet, target, nil, &out); err != nil {
			return nil, err
		}
		for _, item := range out.Value {
			items = append(items, item)
			if len(items) == maxRoomsDestinations {
				break
			}
		}
		if out.Next == target {
			break
		}
		target = out.Next
	}
	return items, nil
}

func validRoomsGraphURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() == "graph.microsoft.com" && u.User == nil &&
		(u.Port() == "" || u.Port() == "443") && strings.HasPrefix(u.Path, "/v1.0/")
}

func listTeamsRooms(ctx context.Context, rt connector.Runtime) ([]roomsDestination, error) {
	teams, err := listGraphRooms(ctx, rt, roomsGraphAPI+"/me/joinedTeams?$select=id,displayName")
	if err != nil {
		return nil, err
	}
	items := make([]roomsDestination, 0, 100)
	for _, team := range teams {
		if _, err := uuid.Parse(team.ID); err != nil {
			continue
		}
		channels, err := listGraphRooms(ctx, rt, roomsGraphAPI+"/teams/"+url.PathEscape(team.ID)+"/channels?$select=id,displayName")
		if err != nil {
			return nil, err
		}
		for _, channel := range channels {
			if !validRoomsTeamsID(team.ID + "/" + channel.ID) {
				continue
			}
			items = append(items, roomsDestination{ID: team.ID + "/" + channel.ID, Name: team.Name + " / " + channel.Name})
			if len(items) == maxRoomsDestinations {
				return items, nil
			}
		}
	}
	return items, nil
}

func validRoomsTeamsID(remote string) bool {
	team, channel, ok := strings.Cut(remote, "/")
	_, err := uuid.Parse(team)
	return ok && err == nil && channel != "" && len(channel) <= 512 && !strings.ContainsAny(channel, "/?#\r\n\x00")
}

func validSlackRoomsWebhook(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme+"://"+u.Hostname() == roomsSlackWebhookOrigin && u.User == nil &&
		(u.Port() == "" || u.Port() == "443") && strings.HasPrefix(u.Path, "/services/") && len(u.Path) > len("/services/") && u.RawQuery == "" && u.Fragment == ""
}

func sendRoomsMessage(ctx context.Context, conn connector.Instance, rt connector.Runtime, remote, text string) error {
	if strings.TrimSpace(text) == "" || utf8.RuneCountInString(text) > 40000 {
		return httpx.ErrBadRequest.WithMessage("A message between 1 and 40000 characters is required.")
	}
	switch conn.TemplateKey {
	case tmplSlackRooms:
		if remote == "" || remote != conn.Config[roomsChannelID] || !validSlackRoomsWebhook(conn.Secrets[secretKeyRoomsWebhook]) {
			return httpx.ErrBadRequest.WithMessage("Choose the Slack channel authorized by this connection.")
		}
		// El webhook ya autoriza por su URL: nunca recibe el Bearer de la app.
		return roomsProviderJSON(policy.SinCredencial(ctx), rt, http.MethodPost, conn.Secrets[secretKeyRoomsWebhook], map[string]any{"text": text, "mrkdwn": false, "unfurl_links": false, "unfurl_media": false}, nil)
	case tmplGoogleChat:
		if !roomsSpaceName.MatchString(remote) {
			return httpx.ErrBadRequest.WithMessage("Invalid Google Chat space.")
		}
		return roomsProviderJSON(ctx, rt, http.MethodPost, roomsChatAPI+remote+"/messages", map[string]string{"text": text}, nil)
	case tmplTeamsRooms:
		if !validRoomsTeamsID(remote) {
			return httpx.ErrBadRequest.WithMessage("Invalid Teams channel.")
		}
		team, channel, _ := strings.Cut(remote, "/")
		target := roomsGraphAPI + "/teams/" + url.PathEscape(team) + "/channels/" + url.PathEscape(channel) + "/messages"
		return roomsProviderJSON(ctx, rt, http.MethodPost, target, map[string]any{"body": map[string]string{"contentType": "text", "content": text}}, nil)
	default:
		return httpx.ErrBadRequest.WithMessage("This app does not receive channel messages.")
	}
}

func createRoomsMeeting(ctx context.Context, rt connector.Runtime) (string, error) {
	var out struct {
		URI string `json:"meetingUri"` //nolint:tagliatelle // contrato de Google Meet
	}
	if err := roomsProviderJSON(ctx, rt, http.MethodPost, roomsMeetAPI, map[string]any{}, &out); err != nil {
		return "", err
	}
	u, err := url.Parse(out.URI)
	if err != nil || u.Scheme != "https" || u.Host != "meet.google.com" || u.User != nil || len(u.Path) < 2 {
		return "", httpx.NewError(http.StatusBadGateway, "rooms_meeting_response", "Google Meet did not return a valid meeting link.")
	}
	return u.String(), nil
}

// La URL del webhook autoriza a publicar: solo vive en el vault cifrado. Los
// nombres e ids del canal/equipo sí son metadatos públicos de la conexión.
type roomsWebhookGrant struct {
	URL       string `json:"url"`
	ChannelID string `json:"channel_id"`
	Channel   string `json:"channel"`
}

type roomsTeamGrant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func slackRoomsGrant(tok tokenResponse) (map[string]string, string, error) {
	webhook := tok.IncomingWebhook
	if webhook == nil || !validSlackRoomsWebhook(webhook.URL) || strings.TrimSpace(webhook.ChannelID) == "" {
		return nil, "", errors.New("slack did not authorize a channel; connect again and choose one")
	}
	metadata := map[string]string{roomsChannelID: webhook.ChannelID, roomsChannelName: webhook.Channel}
	if tok.Team != nil {
		metadata[roomsTeamID], metadata[roomsTeamName] = tok.Team.ID, tok.Team.Name
	}
	return metadata, webhook.URL, nil
}

func applyRoomsConsentGrant(template string, tok tokenResponse, cfg, secrets map[string]string) error {
	if template != tmplSlackRooms {
		return nil
	}
	metadata, webhook, err := slackRoomsGrant(tok)
	if err != nil {
		return err
	}
	secrets[secretKeyRoomsWebhook] = webhook
	for key, value := range metadata {
		cfg[key] = value
	}
	return nil
}
