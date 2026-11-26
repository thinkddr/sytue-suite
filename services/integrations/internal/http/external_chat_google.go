// SPDX-License-Identifier: Apache-2.0
package integrationshttp

import (
	"context"
	"net/http"
	"net/url"

	cuentaschat "git.sytue.eu/sytue/one/apps/nucleo/cuentas/chat"
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
	if err := roomsGoogleChat(rt).Pedir(ctx, http.MethodGet, "spaces?"+q.Encode(), nil, &out); err != nil {
		return externalChannelPage{}, roomsProviderError(err)
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

// El mensaje de Google Chat y la preparación del envío viven en el núcleo
// (`apps/nucleo/cuentas/chat`, RO-01), que también usa Sytue Office.
type googleExternalMessage = cuentaschat.MensajeGoogle

func externalGoogleMessageDTO(msg googleExternalMessage, remote, self string) externalMessage {
	return externalMessage{ID: msg.Name, RemoteID: remote, AuthorID: msg.Sender.Name, AuthorName: msg.Sender.DisplayName, Text: msg.Text, CreatedAt: msg.CreateTime, ThreadID: msg.Thread.Name, Edited: msg.UpdateTime != nil, Owned: self != "" && msg.Sender.Name == self}
}

func externalGoogleSend(ctx context.Context, conn connector.Instance, rt connector.Runtime, in externalWrite) (externalMessage, error) {
	if _, _, err := cuentaschat.EnvioGoogle(in.RemoteID, in.ThreadID, in.Text); err != nil {
		return externalMessage{}, httpx.ErrBadRequest
	}
	out, err := roomsGoogleChat(rt).Enviar(ctx, in.RemoteID, in.ThreadID, in.Text)
	if err != nil {
		return externalMessage{}, roomsProviderError(err)
	}
	return externalGoogleMessageDTO(out, in.RemoteID, conn.Config["_external_user_id"]), nil
}
