// SPDX-License-Identifier: Apache-2.0
package integrationshttp

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
)

// The first page lists the user's teams/channels. A separate fixed cursor then
// lists personal/group chats, including their server-issued continuation token.
func externalTeamsChannels(ctx context.Context, rt connector.Runtime, cursor string) (externalChannelPage, error) {
	page := externalChannelPage{Items: []externalChannel{}}
	if cursor == "" {
		channels, err := listTeamsRooms(ctx, rt)
		if err != nil {
			return page, err
		}
		for _, ch := range channels {
			page.Items = append(page.Items, externalChannel{ID: ch.ID, Name: ch.Name, Kind: "channel", Capabilities: externalCapabilities("microsoft-teams", ch.ID)})
		}
		page.NextCursor = "chats:"
		return page, nil
	}
	token, ok := strings.CutPrefix(cursor, "chats:")
	if !ok {
		return page, httpx.ErrBadRequest
	}
	var out struct {
		Value []struct {
			ID      string `json:"id"`
			Topic   string `json:"topic"`
			Type    string `json:"chatType"` //nolint:tagliatelle // Graph contract
			Members []struct {
				DisplayName string `json:"displayName"` //nolint:tagliatelle // Graph contract
			} `json:"members"`
		} `json:"value"`
		Next string `json:"@odata.nextLink"` //nolint:tagliatelle // Graph contract
	}
	q := url.Values{"$top": {"50"}, "$expand": {"members"}}
	if token != "" {
		q.Set("$skiptoken", token)
	}
	if err := roomsProviderJSON(ctx, rt, http.MethodGet, roomsGraphAPI+"/me/chats?"+q.Encode(), nil, &out); err != nil {
		return page, err
	}
	for _, chat := range out.Value {
		if !externalTeamsChatID(chat.ID) {
			continue
		}
		name := chat.Topic
		if name == "" {
			names := []string{}
			for _, member := range chat.Members {
				if member.DisplayName != "" {
					names = append(names, member.DisplayName)
				}
			}
			name = strings.Join(names, ", ")
		}
		if name == "" {
			name = chat.ID
		}
		remote := "chat/" + chat.ID
		page.Items = append(page.Items, externalChannel{ID: remote, Name: name, Kind: "direct", Capabilities: externalCapabilities("microsoft-teams", remote)})
	}
	next, err := externalGraphNext(out.Next, "/v1.0/me/chats")
	if err != nil {
		return page, err
	}
	if next != "" {
		page.NextCursor = "chats:" + next
	}
	return page, nil
}

func externalTeamsChatID(id string) bool {
	return id != "" && len(id) <= 512 && !strings.ContainsAny(id, "/?#\r\n\x00")
}

func externalTeamsPath(remote, thread string) (string, error) {
	if chat, ok := strings.CutPrefix(remote, "chat/"); ok {
		if !externalTeamsChatID(chat) || thread != "" {
			return "", httpx.ErrBadRequest
		}
		return "/chats/" + url.PathEscape(chat) + "/messages", nil
	}
	if !validRoomsTeamsID(remote) {
		return "", httpx.ErrBadRequest
	}
	team, channel, _ := strings.Cut(remote, "/")
	path := "/teams/" + url.PathEscape(team) + "/channels/" + url.PathEscape(channel) + "/messages"
	if thread != "" {
		if !externalTeamsMessageID(thread) {
			return "", httpx.ErrBadRequest
		}
		path += "/" + url.PathEscape(thread) + "/replies"
	}
	return path, nil
}

func externalTeamsMessageID(id string) bool {
	return id != "" && len(id) <= 128 && !strings.ContainsAny(id, "/?#\r\n\x00")
}

type teamsExternalMessage struct {
	ID      string     `json:"id"`
	Created time.Time  `json:"createdDateTime"`    //nolint:tagliatelle // Graph contract
	Edited  *time.Time `json:"lastEditedDateTime"` //nolint:tagliatelle // Graph contract
	Deleted *time.Time `json:"deletedDateTime"`    //nolint:tagliatelle // Graph contract
	ReplyTo string     `json:"replyToId"`          //nolint:tagliatelle // Graph contract
	From    struct {
		User struct {
			ID   string `json:"id"`
			Name string `json:"displayName"` //nolint:tagliatelle // Graph contract
		} `json:"user"`
	} `json:"from"`
	Body struct {
		Type    string `json:"contentType"` //nolint:tagliatelle // Graph contract
		Content string `json:"content"`
	} `json:"body"`
}

func externalTeamsMessageDTO(msg teamsExternalMessage, remote, self string) externalMessage {
	text := msg.Body.Content
	if msg.Body.Type == "html" {
		text = htmltext.Text(text)
	}
	return externalMessage{ID: msg.ID, RemoteID: remote, AuthorID: msg.From.User.ID, AuthorName: msg.From.User.Name, Text: text, CreatedAt: msg.Created, Edited: msg.Edited != nil, ThreadID: msg.ReplyTo, Owned: self != "" && msg.From.User.ID == self}
}

func externalTeamsMessages(ctx context.Context, conn connector.Instance, rt connector.Runtime, remote, cursor, thread string) (externalMessagePage, error) {
	path, err := externalTeamsPath(remote, thread)
	if err != nil {
		return externalMessagePage{}, err
	}
	var out struct {
		Value []teamsExternalMessage `json:"value"`
		Next  string                 `json:"@odata.nextLink"` //nolint:tagliatelle // Graph contract
	}
	if err := roomsProviderJSON(ctx, rt, http.MethodGet, externalGraphTarget(path, cursor), nil, &out); err != nil {
		return externalMessagePage{}, err
	}
	page := externalMessagePage{Items: []externalMessage{}}
	for _, msg := range out.Value {
		if msg.Deleted == nil {
			page.Items = append(page.Items, externalTeamsMessageDTO(msg, remote, conn.Config["_external_user_id"]))
		}
	}
	page.NextCursor, err = externalGraphNext(out.Next, "/v1.0"+path)
	return page, err
}

func externalTeamsSend(ctx context.Context, conn connector.Instance, rt connector.Runtime, in externalWrite) (externalMessage, error) {
	path, err := externalTeamsPath(in.RemoteID, in.ThreadID)
	if err != nil {
		return externalMessage{}, err
	}
	var out teamsExternalMessage
	if err := roomsProviderJSON(ctx, rt, http.MethodPost, roomsGraphAPI+path, map[string]any{"body": map[string]string{"contentType": "text", "content": in.Text}}, &out); err != nil {
		return externalMessage{}, err
	}
	return externalTeamsMessageDTO(out, in.RemoteID, conn.Config["_external_user_id"]), nil
}

func externalTeamsAction(ctx context.Context, conn connector.Instance, rt connector.Runtime, in externalAction) error {
	if !strings.HasPrefix(in.RemoteID, "chat/") || !externalTeamsMessageID(in.MessageID) {
		return httpx.ErrBadRequest
	}
	path, err := externalTeamsPath(in.RemoteID, "")
	if err != nil {
		return err
	}
	target := roomsGraphAPI + path + "/" + url.PathEscape(in.MessageID)
	if in.Action == "delete" {
		self := conn.Config["_external_user_id"]
		if _, err := uuid.Parse(self); err != nil {
			return httpx.ErrForbidden
		}
		return roomsProviderJSON(ctx, rt, http.MethodPost, roomsGraphAPI+"/users/"+url.PathEscape(self)+path+"/"+url.PathEscape(in.MessageID)+"/softDelete", nil, nil)
	}
	return roomsProviderJSON(ctx, rt, http.MethodPatch, target, map[string]any{"body": map[string]string{"contentType": "text", "content": in.Text}}, nil)
}
