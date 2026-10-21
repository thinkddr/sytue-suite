// SPDX-License-Identifier: Apache-2.0
package integrationshttp

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
)

type slackUserGrant struct {
	ID           string `json:"id"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int64  `json:"expires_in"`
}

func externalSlackUserToken(tok tokenResponse) (tokenResponse, error) {
	user := tok.AuthedUser
	if user == nil || user.TokenType != "user" || user.AccessToken == "" || user.ID == "" {
		return tokenResponse{}, errors.New("slack did not grant a personal user token")
	}
	scopes := map[string]bool{}
	for _, scope := range strings.Split(user.Scope, ",") {
		scopes[scope] = true
	}
	for _, required := range []string{"chat:write", "channels:read", "channels:history", "groups:read", "groups:history", "im:read", "im:history", "mpim:read", "mpim:history"} {
		if !scopes[required] {
			return tokenResponse{}, errors.New("slack user grant is missing required conversation scopes")
		}
	}
	tok.AccessToken, tok.RefreshToken, tok.TokenType, tok.ExpiresIn = user.AccessToken, user.RefreshToken, user.TokenType, user.ExpiresIn
	return tok, nil
}

type slackStatus struct {
	OK               bool   `json:"ok"`
	Error            string `json:"error"`
	ResponseMetadata struct {
		Cursor string `json:"next_cursor"`
	} `json:"response_metadata"`
}

func (s slackStatus) check() error {
	if s.OK {
		return nil
	}
	switch s.Error {
	case "missing_scope", "not_allowed_token_type", "channel_not_found", "not_in_channel", "cant_update_message", "cant_delete_message":
		return httpx.ErrForbidden.WithCode("external_provider_permission").WithMessage("This account cannot perform that action in this conversation.")
	case "invalid_auth", "token_revoked", "account_inactive", "token_expired":
		return httpx.ErrConflict.WithCode("external_reconnect_required").WithMessage("Connect this account again.")
	case "ratelimited":
		return httpx.NewError(http.StatusTooManyRequests, "external_rate_limited", "Slack is busy. Try again shortly.")
	default:
		return httpx.ErrUnavailable.WithCode("external_provider_rejected").WithMessage("Slack could not complete the request.")
	}
}

func externalSlackChannels(ctx context.Context, rt connector.Runtime, cursor string) (externalChannelPage, error) {
	var out struct {
		slackStatus
		Channels []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			User    string `json:"user"`
			Private bool   `json:"is_private"`
			IM      bool   `json:"is_im"`
			MPIM    bool   `json:"is_mpim"`
		} `json:"channels"`
	}
	q := url.Values{"types": {"public_channel,private_channel,mpim,im"}, "exclude_archived": {"true"}, "limit": {"100"}}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	if err := roomsProviderJSON(ctx, rt, http.MethodGet, "https://slack.com/api/conversations.list?"+q.Encode(), nil, &out); err != nil {
		return externalChannelPage{}, err
	}
	if err := out.check(); err != nil {
		return externalChannelPage{}, err
	}
	page := externalChannelPage{Items: []externalChannel{}, NextCursor: out.ResponseMetadata.Cursor}
	for _, channel := range out.Channels {
		if !externalSlackID.MatchString(channel.ID) {
			continue
		}
		kind := "public"
		if channel.Private {
			kind = "private"
		}
		if channel.IM || channel.MPIM {
			kind = "direct"
		}
		name := channel.Name
		if name == "" {
			name = channel.User
		}
		if name == "" {
			name = channel.ID
		}
		page.Items = append(page.Items, externalChannel{ID: channel.ID, Name: name, Kind: kind, Capabilities: externalCapabilities("slack", channel.ID)})
	}
	return page, nil
}

type slackMessage struct {
	TS      string `json:"ts"`
	User    string `json:"user"`
	Bot     string `json:"bot_id"`
	Text    string `json:"text"`
	Thread  string `json:"thread_ts"`
	Replies int    `json:"reply_count"`
	Edited  *struct {
		TS string `json:"ts"`
	} `json:"edited"`
}

func externalSlackMessage(msg slackMessage, remote, self string) externalMessage {
	secs, fraction, _ := strings.Cut(msg.TS, ".")
	unix, _ := strconv.ParseInt(secs, 10, 64)
	fraction = (fraction + "000000000")[:9]
	nanos, _ := strconv.ParseInt(fraction, 10, 64)
	author := msg.User
	if author == "" {
		author = msg.Bot
	}
	return externalMessage{ID: msg.TS, RemoteID: remote, AuthorID: author, Text: msg.Text, CreatedAt: time.Unix(unix, nanos).UTC(), ThreadID: msg.Thread, ReplyCount: msg.Replies, Edited: msg.Edited != nil, Owned: self != "" && msg.User == self}
}

func externalSlackMessages(ctx context.Context, conn connector.Instance, rt connector.Runtime, remote, cursor, thread string) (externalMessagePage, error) {
	if !externalSlackID.MatchString(remote) || thread != "" && !externalSlackTS.MatchString(thread) {
		return externalMessagePage{}, httpx.ErrBadRequest
	}
	method := "conversations.history"
	q := url.Values{"channel": {remote}, "limit": {"15"}}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	if thread != "" {
		method = "conversations.replies"
		q.Set("ts", thread)
	}
	var out struct {
		slackStatus
		Messages []slackMessage `json:"messages"`
	}
	if err := roomsProviderJSON(ctx, rt, http.MethodGet, "https://slack.com/api/"+method+"?"+q.Encode(), nil, &out); err != nil {
		return externalMessagePage{}, err
	}
	if err := out.check(); err != nil {
		return externalMessagePage{}, err
	}
	page := externalMessagePage{Items: []externalMessage{}, NextCursor: out.ResponseMetadata.Cursor}
	for _, message := range out.Messages {
		if externalSlackTS.MatchString(message.TS) {
			page.Items = append(page.Items, externalSlackMessage(message, remote, conn.Config["_external_user_id"]))
		}
	}
	return page, nil
}

func externalSlackSend(ctx context.Context, conn connector.Instance, rt connector.Runtime, in externalWrite) (externalMessage, error) {
	if !externalSlackID.MatchString(in.RemoteID) || in.ThreadID != "" && !externalSlackTS.MatchString(in.ThreadID) {
		return externalMessage{}, httpx.ErrBadRequest
	}
	body := map[string]any{"channel": in.RemoteID, "text": in.Text, "unfurl_links": false, "unfurl_media": false}
	if in.ThreadID != "" {
		body["thread_ts"] = in.ThreadID
	}
	var out struct {
		slackStatus
		TS      string       `json:"ts"`
		Message slackMessage `json:"message"`
	}
	if err := roomsProviderJSON(ctx, rt, http.MethodPost, "https://slack.com/api/chat.postMessage", body, &out); err != nil {
		return externalMessage{}, err
	}
	if err := out.check(); err != nil {
		return externalMessage{}, err
	}
	out.Message.TS = out.TS
	if out.Message.Text == "" {
		out.Message.Text = in.Text
	}
	return externalSlackMessage(out.Message, in.RemoteID, conn.Config["_external_user_id"]), nil
}

func externalSlackAction(ctx context.Context, rt connector.Runtime, in externalAction) error {
	if !externalSlackID.MatchString(in.RemoteID) || !externalSlackTS.MatchString(in.MessageID) {
		return httpx.ErrBadRequest
	}
	method := "chat.delete"
	body := map[string]string{"channel": in.RemoteID, "ts": in.MessageID}
	if in.Action == "edit" {
		method = "chat.update"
		body["text"] = in.Text
	}
	var out slackStatus
	if err := roomsProviderJSON(ctx, rt, http.MethodPost, "https://slack.com/api/"+method, body, &out); err != nil {
		return err
	}
	return out.check()
}
