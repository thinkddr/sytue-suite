// SPDX-License-Identifier: Apache-2.0
package integrationshttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
)

func TestExternalSlackNeverFallsBackToBotCredential(t *testing.T) {
	var response tokenResponse
	raw := `{"access_token":"bot-must-not-be-used","token_type":"bot","authed_user":{"id":"U123","token_type":"user","access_token":"personal-user-secret","refresh_token":"personal-refresh","expires_in":43200,"scope":"chat:write,channels:read,channels:history,groups:read,groups:history,im:read,im:history,mpim:read,mpim:history"}}`
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		t.Fatal(err)
	}
	personal, err := externalSlackUserToken(response)
	if err != nil || personal.AccessToken != "personal-user-secret" || personal.RefreshToken != "personal-refresh" || personal.ExpiresIn != 43200 {
		t.Fatalf("user grant not selected: %+v %v", personal, err)
	}
	response.AuthedUser = nil
	if _, err := externalSlackUserToken(response); err == nil {
		t.Fatal("bot credential accepted for personal chat")
	}
}

func TestExternalSlackReadsHistoryAndSendsUserThread(t *testing.T) {
	calls := 0
	rt := roomsTestRuntime(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "slack.com" {
			t.Fatalf("unexpected provider %s", r.URL)
		}
		switch r.URL.Path {
		case "/api/conversations.replies":
			if r.URL.Query().Get("channel") != "C123" || r.URL.Query().Get("ts") != "123.456" || r.URL.Query().Get("limit") != "15" {
				t.Fatalf("wrong paginated thread request: %s", r.URL)
			}
			return roomsTestResponse(http.StatusOK, `{"ok":true,"messages":[{"ts":"123.457","user":"U123","text":"private external content","thread_ts":"123.456"}],"response_metadata":{"next_cursor":"opaque"}}`), nil
		case "/api/chat.postMessage":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["channel"] != "C123" || body["thread_ts"] != "123.456" || body["text"] != "reply from Rooms" || body["unfurl_links"] != false {
				t.Fatalf("wrong thread body %+v", body)
			}
			return roomsTestResponse(http.StatusOK, `{"ok":true,"ts":"123.458","message":{"ts":"123.458","user":"U123","text":"reply from Rooms","thread_ts":"123.456"}}`), nil
		default:
			t.Fatalf("unexpected path %s", r.URL)
			return nil, errors.New("unexpected path")
		}
	})
	conn := connector.Instance{TemplateKey: tmplSlackChat, Config: map[string]string{"_external_user_id": "U123"}}
	page, err := externalListMessages(context.Background(), conn, rt, "C123", "", "123.456")
	if err != nil || len(page.Items) != 1 || page.Items[0].Text != "private external content" || !page.Items[0].Owned || page.NextCursor != "opaque" {
		t.Fatalf("history=%+v err=%v", page, err)
	}
	sent, err := externalSend(context.Background(), conn, rt, externalWrite{RemoteID: "C123", Text: "reply from Rooms", ThreadID: "123.456"})
	if err != nil || !sent.Owned || sent.ThreadID != "123.456" {
		t.Fatalf("sent=%+v err=%v", sent, err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestExternalInputsCannotBecomeProviderURLs(t *testing.T) {
	calls := 0
	rt := roomsTestRuntime(func(*http.Request) (*http.Response, error) {
		calls++
		return roomsTestResponse(http.StatusOK, `{}`), nil
	})
	for _, test := range []struct{ key, remote, thread string }{
		{tmplSlackChat, "https://attacker.example", ""},
		{tmplGoogleConversations, "spaces/AAA", "spaces/OTHER/threads/secret"},
		{tmplTeamsChat, "chat/../../me/drive", ""},
		{tmplTeamsChat, "00000000-0000-0000-0000-000000000001/19:channel", "../../me/drive"},
	} {
		conn := connector.Instance{TemplateKey: test.key}
		if _, err := externalListMessages(context.Background(), conn, rt, test.remote, "", test.thread); err == nil {
			t.Fatalf("accepted cross-resource input %+v", test)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid input reached provider %d times", calls)
	}
	if _, err := externalGraphNext("https://attacker.example/v1.0/chats/C/messages?$skiptoken=opaque", "/v1.0/chats/C/messages"); err == nil {
		t.Fatal("foreign continuation origin accepted")
	}
	if _, err := externalGraphNext("https://graph.microsoft.com/v1.0/me/drive?$skiptoken=opaque", "/v1.0/chats/C/messages"); err == nil {
		t.Fatal("foreign continuation path accepted")
	}
}

func TestExternalTeamsTextAndDeleteUseRealUserEndpoint(t *testing.T) {
	msg := teamsExternalMessage{}
	msg.Body.Type = "html"
	msg.Body.Content = `<p>Hello <b>world</b></p><script>secret()</script><img src="https://tracker.example">`
	clean := externalTeamsMessageDTO(msg, "chat/19:dm", "self")
	if clean.Text != "Hello world" {
		t.Fatalf("unsafe HTML projection %q", clean.Text)
	}
	rt := roomsTestRuntime(func(r *http.Request) (*http.Response, error) {
		expected := "/v1.0/users/12345678-1234-1234-1234-123456789abc/chats/19:dm/messages/123456/softDelete"
		if r.Method != http.MethodPost || r.URL.Path != expected || r.ContentLength != 0 {
			t.Fatalf("wrong Graph delete endpoint %s %s", r.Method, r.URL)
		}
		return roomsTestResponse(http.StatusNoContent, ""), nil
	})
	conn := connector.Instance{TemplateKey: tmplTeamsChat, Config: map[string]string{"_external_user_id": "12345678-1234-1234-1234-123456789abc"}}
	if err := externalApplyAction(context.Background(), conn, rt, externalAction{Action: "delete", RemoteID: "chat/19:dm", MessageID: "123456"}); err != nil {
		t.Fatal(err)
	}
	if externalCapabilities("microsoft-teams", "team/channel")["edit"] {
		t.Fatal("channel edit exposed without its scope")
	}
}

// Google Chat solo tiene chat.messages.create: ni leer ni tocar mensajes, y sin
// llamar a Google para listarlos.
func TestGoogleChatSoloEnvia(t *testing.T) {
	c := externalCapabilities("google-chat", "spaces/AAA")
	if c["read"] || c["edit"] || c["delete"] || !c["send"] {
		t.Fatalf("capacidades de Google Chat: %v", c)
	}
	rt := roomsTestRuntime(func(r *http.Request) (*http.Response, error) {
		t.Fatalf("llamada inesperada a %s", r.URL)
		return nil, errors.New("llamada inesperada")
	})
	page, err := externalListMessages(context.Background(), connector.Instance{TemplateKey: tmplGoogleConversations}, rt, "spaces/AAA", "", "")
	if err != nil || len(page.Items) != 0 || page.Capabilities["read"] {
		t.Fatalf("página %+v, %v", page, err)
	}
}

func TestExternalProviderErrorsDoNotEchoBodyOrToken(t *testing.T) {
	rt := roomsTestRuntime(func(*http.Request) (*http.Response, error) {
		return roomsTestResponse(http.StatusForbidden, `{"error":"personal-user-secret private text"}`), nil
	})
	_, err := externalSlackChannels(context.Background(), rt, "")
	if err == nil || strings.Contains(err.Error(), "personal-user-secret") || strings.Contains(err.Error(), "private text") {
		t.Fatalf("unsafe provider error %v", err)
	}
}
