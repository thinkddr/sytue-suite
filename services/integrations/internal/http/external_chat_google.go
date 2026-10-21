// SPDX-License-Identifier: Apache-2.0
package integrationshttp

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
)

func externalGoogleChannels(ctx context.Context, rt connector.Runtime, cursor string) (externalChannelPage, error) {
	var out struct {
		Spaces []struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"` //nolint:tagliatelle // Google Chat contract
			SpaceType   string `json:"spaceType"`   //nolint:tagliatelle // Google Chat contract
		} `json:"spaces"`
		Next string `json:"nextPageToken"` //nolint:tagliatelle // Google Chat contract
	}
	q := url.Values{"pageSize": {"100"}}
	if cursor != "" {
		q.Set("pageToken", cursor)
	}
	if err := roomsProviderJSON(ctx, rt, http.MethodGet, roomsChatAPI+"spaces?"+q.Encode(), nil, &out); err != nil {
		return externalChannelPage{}, err
	}
	page := externalChannelPage{Items: []externalChannel{}, NextCursor: out.Next}
	for _, space := range out.Spaces {
		if !roomsSpaceName.MatchString(space.Name) {
			continue
		}
		name := space.DisplayName
		if name == "" {
			name = space.Name
		}
		kind := "space"
		if space.SpaceType == "DIRECT_MESSAGE" || space.SpaceType == "GROUP_CHAT" {
			kind = "direct"
		}
		page.Items = append(page.Items, externalChannel{ID: space.Name, Name: name, Kind: kind, Capabilities: externalCapabilities("google-chat", space.Name)})
	}
	return page, nil
}

type googleExternalMessage struct {
	Name       string     `json:"name"`
	Text       string     `json:"text"`
	CreateTime time.Time  `json:"createTime"`     //nolint:tagliatelle // Google Chat contract
	UpdateTime *time.Time `json:"lastUpdateTime"` //nolint:tagliatelle // Google Chat contract
	Sender     struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"` //nolint:tagliatelle // Google Chat contract
	} `json:"sender"`
	Thread struct {
		Name string `json:"name"`
	} `json:"thread"`
}

func externalGoogleMessageDTO(msg googleExternalMessage, remote, self string) externalMessage {
	return externalMessage{ID: msg.Name, RemoteID: remote, AuthorID: msg.Sender.Name, AuthorName: msg.Sender.DisplayName, Text: msg.Text, CreatedAt: msg.CreateTime, ThreadID: msg.Thread.Name, Edited: msg.UpdateTime != nil, Owned: self != "" && msg.Sender.Name == self}
}

func externalGoogleSend(ctx context.Context, conn connector.Instance, rt connector.Runtime, in externalWrite) (externalMessage, error) {
	if !roomsSpaceName.MatchString(in.RemoteID) || in.ThreadID != "" && (!externalGoogleThread.MatchString(in.ThreadID) || !strings.HasPrefix(in.ThreadID, in.RemoteID+"/threads/")) {
		return externalMessage{}, httpx.ErrBadRequest
	}
	body := map[string]any{"text": in.Text}
	q := url.Values{}
	if in.ThreadID != "" {
		body["thread"] = map[string]string{"name": in.ThreadID}
		q.Set("messageReplyOption", "REPLY_MESSAGE_OR_FAIL")
	}
	target := roomsChatAPI + in.RemoteID + "/messages"
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	var out googleExternalMessage
	if err := roomsProviderJSON(ctx, rt, http.MethodPost, target, body, &out); err != nil {
		return externalMessage{}, err
	}
	return externalGoogleMessageDTO(out, in.RemoteID, conn.Config["_external_user_id"]), nil
}
