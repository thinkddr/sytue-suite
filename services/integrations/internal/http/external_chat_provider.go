// SPDX-License-Identifier: Apache-2.0
package integrationshttp

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
)

type externalChannel struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Kind         string          `json:"kind"`
	Capabilities map[string]bool `json:"capabilities"`
}
type externalChannelPage struct {
	Items      []externalChannel `json:"items"`
	NextCursor string            `json:"next_cursor,omitempty"`
}
type externalMessage struct {
	ID         string    `json:"id"`
	RemoteID   string    `json:"remote_id"`
	AuthorID   string    `json:"author_id"`
	AuthorName string    `json:"author_name,omitempty"`
	Text       string    `json:"text"`
	CreatedAt  time.Time `json:"created_at"`
	ThreadID   string    `json:"thread_id,omitempty"`
	ReplyCount int       `json:"reply_count,omitempty"`
	Edited     bool      `json:"edited,omitempty"`
	Owned      bool      `json:"owned"`
}
type externalMessagePage struct {
	Items        []externalMessage `json:"items"`
	NextCursor   string            `json:"next_cursor,omitempty"`
	Capabilities map[string]bool   `json:"capabilities"`
}
type externalWrite struct {
	RemoteID string `json:"remote_id"`
	Text     string `json:"text"`
	ThreadID string `json:"thread_id,omitempty"`
}
type externalAction struct {
	Action    string `json:"action"`
	RemoteID  string `json:"remote_id"`
	MessageID string `json:"message_id"`
	ThreadID  string `json:"thread_id,omitempty"`
	Text      string `json:"text,omitempty"`
}

var (
	externalSlackID      = regexp.MustCompile(`^[CDG][A-Z0-9]{2,31}$`)
	externalSlackTS      = regexp.MustCompile(`^\d{1,20}\.\d{1,10}$`)
	externalGoogleThread = regexp.MustCompile(`^spaces/[A-Za-z0-9_-]+/threads/[A-Za-z0-9_.-]+$`)
)

func externalValidCursor(cursor string) bool {
	return len(cursor) <= 8192 && !strings.ContainsAny(cursor, "\r\n\x00")
}

func externalValidText(text string) bool {
	return strings.TrimSpace(text) != "" && utf8.ValidString(text) && utf8.RuneCountInString(text) <= 40000
}

func externalListChannels(ctx context.Context, conn connector.Instance, rt connector.Runtime, cursor string) (externalChannelPage, error) {
	if !externalValidCursor(cursor) {
		return externalChannelPage{}, httpx.ErrBadRequest
	}
	switch conn.TemplateKey {
	case tmplSlackChat:
		return externalSlackChannels(ctx, rt, cursor)
	case tmplTeamsChat:
		return externalTeamsChannels(ctx, rt, cursor)
	case tmplGoogleConversations:
		return externalGoogleChannels(ctx, rt, cursor)
	default:
		return externalChannelPage{}, httpx.ErrNotFound
	}
}

func externalListMessages(ctx context.Context, conn connector.Instance, rt connector.Runtime, remote, cursor, thread string) (externalMessagePage, error) {
	if !externalValidCursor(cursor) {
		return externalMessagePage{}, httpx.ErrBadRequest
	}
	var page externalMessagePage
	var err error
	switch conn.TemplateKey {
	case tmplSlackChat:
		page, err = externalSlackMessages(ctx, conn, rt, remote, cursor, thread)
	case tmplTeamsChat:
		page, err = externalTeamsMessages(ctx, conn, rt, remote, cursor, thread)
	case tmplGoogleConversations:
		// Sin permiso de lectura (ver externalCapabilities) la página va vacía, pero la
		// entrada se valida igual: un hilo de otro espacio sigue siendo un 400.
		if !roomsSpaceName.MatchString(remote) || thread != "" && (!externalGoogleThread.MatchString(thread) || !strings.HasPrefix(thread, remote+"/threads/")) {
			err = httpx.ErrBadRequest
		}
		page.Items = []externalMessage{}
	default:
		err = httpx.ErrNotFound
	}
	page.Capabilities = externalCapabilities(externalChatProvider(conn.TemplateKey), remote)
	return page, err
}

func externalSend(ctx context.Context, conn connector.Instance, rt connector.Runtime, in externalWrite) (externalMessage, error) {
	if !externalValidText(in.Text) {
		return externalMessage{}, httpx.ErrBadRequest.WithMessage("Write a message between 1 and 40000 characters.")
	}
	switch conn.TemplateKey {
	case tmplSlackChat:
		return externalSlackSend(ctx, conn, rt, in)
	case tmplTeamsChat:
		return externalTeamsSend(ctx, conn, rt, in)
	case tmplGoogleConversations:
		return externalGoogleSend(ctx, conn, rt, in)
	default:
		return externalMessage{}, httpx.ErrNotFound
	}
}

func externalApplyAction(ctx context.Context, conn connector.Instance, rt connector.Runtime, in externalAction) error {
	if in.Action != "edit" && in.Action != "delete" || in.Action == "edit" && !externalValidText(in.Text) {
		return httpx.ErrBadRequest
	}
	if !externalCapabilities(externalChatProvider(conn.TemplateKey), in.RemoteID)[in.Action] {
		return httpx.ErrBadRequest.WithCode("external_action_unavailable").WithMessage("This action is not available for this conversation.")
	}
	switch conn.TemplateKey {
	case tmplSlackChat:
		return externalSlackAction(ctx, rt, in)
	case tmplTeamsChat:
		return externalTeamsAction(ctx, conn, rt, in)
	default:
		return httpx.ErrNotFound
	}
}

// Graph cursors contain only its opaque skip token. Never accept a client URL or
// copy an upstream continuation URL to the next request without checking its path.
func externalGraphNext(raw, path string) (string, error) {
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || !validRoomsGraphURL(raw) || u.Path != externalDecodedPath(path) || u.Fragment != "" {
		return "", httpx.ErrUnavailable.WithCode("external_provider_response")
	}
	cursor := u.Query().Get("$skiptoken")
	if !externalValidCursor(cursor) || cursor == "" {
		return "", httpx.ErrUnavailable.WithCode("external_provider_response")
	}
	return cursor, nil
}

func externalGraphTarget(path, cursor string) string {
	q := url.Values{"$top": {"20"}}
	if cursor != "" {
		q.Set("$skiptoken", cursor)
	}
	return roomsGraphAPI + path + "?" + q.Encode()
}

func externalDecodedPath(path string) string {
	decoded, err := url.PathUnescape(path)
	if err != nil {
		return ""
	}
	return decoded
}
