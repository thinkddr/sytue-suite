// SPDX-License-Identifier: Apache-2.0
package integrationshttp

import (
	"context"
	"strings"

	"github.com/google/uuid"

	cuentaschat "git.sytue.eu/sytue/one/apps/nucleo/cuentas/chat"
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
	chats, next, err := roomsGraph(rt).PaginaChats(ctx, token)
	if err != nil {
		return page, roomsProviderError(err)
	}
	for _, chat := range chats {
		if !externalTeamsChatID(chat.ID) {
			continue
		}
		name := chat.Tema
		if name == "" {
			names := []string{}
			for _, member := range chat.Miembros {
				if member.Nombre != "" {
					names = append(names, member.Nombre)
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
	next, err = externalGraphNext(next, "/v1.0/me/chats")
	if err != nil {
		return page, err
	}
	if next != "" {
		page.NextCursor = "chats:" + next
	}
	return page, nil
}

// Las comprobaciones de los identificadores, la ruta de los mensajes y el mensaje de
// Graph viven en el núcleo (`apps/nucleo/cuentas/chat`, RO-01), que también usa Sytue
// Office: aquí solo se traduce su error de entrada al de la pasarela.
func externalTeamsChatID(id string) bool { return cuentaschat.IDChatTeamsValido(id) }

func externalTeamsPath(remote, thread string) (string, error) {
	path, err := cuentaschat.RutaMensajesTeams(remote, thread)
	if err != nil {
		return "", httpx.ErrBadRequest
	}
	return path, nil
}

func externalTeamsMessageID(id string) bool { return cuentaschat.IDMensajeTeamsValido(id) }

type teamsExternalMessage = cuentaschat.MensajeGraph

func externalTeamsMessageDTO(msg teamsExternalMessage, remote, self string) externalMessage {
	return externalMessage{ID: msg.ID, RemoteID: remote, AuthorID: msg.From.User.ID, AuthorName: msg.From.User.Name, Text: msg.Texto(), CreatedAt: msg.Created, Edited: msg.Edited != nil, ThreadID: msg.ReplyTo, Owned: self != "" && msg.From.User.ID == self}
}

func externalTeamsMessages(ctx context.Context, conn connector.Instance, rt connector.Runtime, remote, cursor, thread string) (externalMessagePage, error) {
	path, err := externalTeamsPath(remote, thread)
	if err != nil {
		return externalMessagePage{}, err
	}
	msgs, next, err := roomsGraph(rt).PaginaMensajes(ctx, remote, thread, cursor, externalGraphPage)
	if err != nil {
		return externalMessagePage{}, roomsProviderError(err)
	}
	page := externalMessagePage{Items: []externalMessage{}}
	for _, msg := range msgs {
		if msg.Deleted == nil {
			page.Items = append(page.Items, externalTeamsMessageDTO(msg, remote, conn.Config["_external_user_id"]))
		}
	}
	page.NextCursor, err = externalGraphNext(next, "/v1.0"+path)
	return page, err
}

func externalTeamsSend(ctx context.Context, conn connector.Instance, rt connector.Runtime, in externalWrite) (externalMessage, error) {
	if _, err := externalTeamsPath(in.RemoteID, in.ThreadID); err != nil {
		return externalMessage{}, err
	}
	out, err := roomsGraph(rt).Enviar(ctx, in.RemoteID, in.ThreadID, in.Text)
	if err != nil {
		return externalMessage{}, roomsProviderError(err)
	}
	return externalTeamsMessageDTO(out, in.RemoteID, conn.Config["_external_user_id"]), nil
}

func externalTeamsAction(ctx context.Context, conn connector.Instance, rt connector.Runtime, in externalAction) error {
	if !strings.HasPrefix(in.RemoteID, "chat/") || !externalTeamsMessageID(in.MessageID) {
		return httpx.ErrBadRequest
	}
	if _, err := externalTeamsPath(in.RemoteID, ""); err != nil {
		return err
	}
	if in.Action == "delete" {
		self := conn.Config["_external_user_id"]
		if _, err := uuid.Parse(self); err != nil {
			return httpx.ErrForbidden
		}
		return roomsProviderError(roomsGraph(rt).Borrar(ctx, self, in.RemoteID, in.MessageID))
	}
	return roomsProviderError(roomsGraph(rt).Editar(ctx, in.RemoteID, in.MessageID, in.Text))
}
