// SPDX-License-Identifier: Apache-2.0
package mailbox

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

type cloudHTTP func(*http.Request) (*http.Response, error)

func (f cloudHTTP) Do(r *http.Request) (*http.Response, error) { return f(r) }
func cloudReply(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

func TestCloudSendUsesValidatedSenderAndKeepsBlindRecipients(t *testing.T) {
	for _, provider := range []string{"gmail", "microsoft"} {
		t.Run(provider, func(t *testing.T) {
			calls := 0
			client := NewCloudClient(Account{Provider: provider, Address: "owner@example.eu"}, cloudHTTP(func(r *http.Request) (*http.Response, error) {
				calls++
				data, _ := io.ReadAll(r.Body)
				var raw []byte
				var err error
				if provider == "gmail" {
					var wire struct {
						Raw string `json:"raw"`
					}
					if json.Unmarshal(data, &wire) != nil {
						t.Fatal("invalid API payload")
					}
					raw, err = base64.RawURLEncoding.DecodeString(wire.Raw)
				} else {
					raw, err = base64.StdEncoding.DecodeString(string(data))
					if r.Header.Get("Content-Type") != "text/plain" {
						t.Fatal("Graph MIME content type")
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				text := string(raw)
				if !strings.Contains(text, "From: <owner@example.eu>") || !strings.Contains(text, "Bcc: <blind@example.eu>") {
					t.Fatalf("missing sender or envelope Bcc header: %q", text)
				}
				if provider == "gmail" {
					return cloudReply(200, `{"id":"sent-1"}`), nil
				}
				return cloudReply(202, ""), nil
			}))
			_, err := client.Send(t.Context(), Outgoing{To: []Address{{Email: "to@example.eu"}}, Bcc: []Address{{Email: "blind@example.eu"}}, Subject: "fixture", Text: "fixture only"})
			if err != nil || calls != 1 {
				t.Fatalf("send: %v, calls %d", err, calls)
			}
		})
	}
}

func TestCloudSendNeverRetriesAnAmbiguousSubmission(t *testing.T) {
	for _, provider := range []string{"gmail", "microsoft"} {
		for _, status := range []int{0, 500, 200} {
			t.Run(provider+strconv.Itoa(status), func(t *testing.T) {
				calls := 0
				client := NewCloudClient(Account{Provider: provider, Address: "owner@example.eu"}, cloudHTTP(func(*http.Request) (*http.Response, error) {
					calls++
					if status == 0 {
						return nil, io.ErrUnexpectedEOF
					}
					if provider == "microsoft" && status == 200 {
						return cloudReply(503, ""), nil
					}
					return cloudReply(status, "malformed after acceptance"), nil
				}))
				_, err := client.Send(t.Context(), Outgoing{To: []Address{{Email: "to@example.eu"}}, Text: "fixture"})
				if !errors.Is(err, ErrSendUnknown) || calls != 1 {
					t.Fatalf("result %v; requests=%d", err, calls)
				}
			})
		}
	}
}

func TestGraphPagingAndInboxRole(t *testing.T) {
	calls := []string{}
	client := NewCloudClient(Account{Provider: "microsoft"}, cloudHTTP(func(r *http.Request) (*http.Response, error) {
		calls = append(calls, r.URL.RequestURI())
		switch r.URL.Path {
		case "/v1.0/$batch":
			return cloudReply(200, `{"responses":[{"id":"inbox","status":200,"body":{"id":"inbox-id"}}]}`), nil
		case "/v1.0/me/mailFolders":
			return cloudReply(200, `{"value":[{"id":"archive","displayName":"Archive"},{"id":"inbox-id","displayName":"Inbox"}]}`), nil
		case "/v1.0/me/mailFolders/inbox-id/messages":
			if r.URL.Query().Get("$skip") == "50" {
				if r.URL.Query().Get("$select") == "" || r.URL.Query().Get("$top") != "50" {
					t.Fatal("cursor lost fixed query")
				}
				return cloudReply(200, `{"value":[]}`), nil
			}
			return cloudReply(200, `{"value":[{"id":"message","subject":"fixture","from":{"emailAddress":{"address":"sender@example.eu","name":"Sender"}}}],"@odata.nextLink":"https://graph.microsoft.com/v1.0/me/mailFolders/inbox-id/messages?$skip=50&$top=50"}`), nil
		}
		t.Fatalf("unexpected request %s", r.URL)
		return nil, ErrUnavailable
	}))
	folders, err := client.Folders(t.Context())
	if err != nil || len(folders) != 2 || folders[1].Role != "inbox" {
		t.Fatalf("folders %#v: %v", folders, err)
	}
	page, err := client.Messages(t.Context(), "inbox-id", "")
	if err != nil || len(page.Items) != 1 || page.Items[0].From[0].Email != "sender@example.eu" || page.NextCursor == "" {
		t.Fatalf("page %#v: %v", page, err)
	}
	_, err = client.Messages(t.Context(), "inbox-id", page.NextCursor)
	if err != nil || len(calls) != 4 {
		t.Fatalf("pagination %v, requests %v", err, calls)
	}
}

// Graph escribe el nextLink a su manera, no como se le pidió: con la clave entre
// comillas (OData), con el `=` del id codificado y `$` como `%24`, o por
// `/users/<id>/`. Con cualquiera de ellas la carpeta tiene que paginar
// (2026-11-08: las carpetas de más de 50 mensajes daban 502 en la primera página).
func TestGraphPaginatesWithGraphShapedNextLinks(t *testing.T) {
	const folder = "AQMkADAwATNiZmYAZC1iN2NhLTNkNzUt_VaBNmJrgEAGIggNul5EEuWnx92WntMbAAAAgEMAAAA="
	for _, nextLink := range []string{
		"https://graph.microsoft.com/v1.0/me/mailFolders('AQMkADAwATNiZmYAZC1iN2NhLTNkNzUt_VaBNmJrgEAGIggNul5EEuWnx92WntMbAAAAgEMAAAA%3D')/messages?%24top=50&%24skip=50",
		"https://graph.microsoft.com/v1.0/users/00000000-0000-0000-0000-000000000001/mailFolders/" + folder + "/messages?$skip=50",
		"https://graph.microsoft.com/v1.0/me/mailFolders/" + folder + "/messages?$skiptoken=abc",
	} {
		pedidas := []string{}
		client := NewCloudClient(Account{Provider: "microsoft"}, cloudHTTP(func(r *http.Request) (*http.Response, error) {
			pedidas = append(pedidas, r.URL.Path)
			if r.URL.Query().Get("$skip") != "" || r.URL.Query().Get("$skiptoken") != "" {
				return cloudReply(200, `{"value":[]}`), nil
			}
			return cloudReply(200, `{"value":[{"id":"m1","subject":"s"}],"@odata.nextLink":"`+nextLink+`"}`), nil
		}))
		page, err := client.Messages(t.Context(), folder, "")
		if err != nil || len(page.Items) != 1 || page.NextCursor == "" {
			t.Fatalf("%s: primera página %#v: %v", nextLink, page, err)
		}
		if _, err := client.Messages(t.Context(), folder, page.NextCursor); err != nil {
			t.Fatalf("%s: segunda página: %v", nextLink, err)
		}
		// La segunda petición va por la ruta PROPIA, no por la del nextLink.
		if len(pedidas) != 2 || pedidas[1] != "/v1.0/me/mailFolders/"+folder+"/messages" {
			t.Fatalf("%s: peticiones %v", nextLink, pedidas)
		}
	}
}

func TestGraphRejectsForeignPaginationOrigin(t *testing.T) {
	client := NewCloudClient(Account{Provider: "microsoft"}, cloudHTTP(func(*http.Request) (*http.Response, error) {
		return cloudReply(200, `{"value":[],"@odata.nextLink":"https://evil.example/me?$skip=1"}`), nil
	}))
	if _, err := client.Messages(t.Context(), "inbox", ""); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("foreign nextLink: %v", err)
	}
}

func TestCloudSendRejectsHeaderInjectionBeforeNetwork(t *testing.T) {
	client := NewCloudClient(Account{Provider: "gmail", Address: "owner@example.eu"}, cloudHTTP(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid email reached network")
		return nil, ErrUnavailable
	}))
	_, err := client.Send(t.Context(), Outgoing{To: []Address{{Email: "to@example.eu"}}, Bcc: []Address{{Email: "blind@example.eu", Name: "bad\r\nX-Test: bad"}}, Text: "fixture"})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("header injection: %v", err)
	}
}

func TestGmailPageUsesOneBoundedMetadataBatch(t *testing.T) {
	calls := 0
	client := NewCloudClient(Account{Provider: "gmail"}, cloudHTTP(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return cloudReply(200, `{"messages":[{"id":"a"},{"id":"b"}],"nextPageToken":"next"}`), nil
		}
		if r.URL.Path != "/batch/gmail/v1" || r.Method != http.MethodPost {
			t.Fatalf("metadata was not batched %s", r.URL)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "GET /gmail/v1/users/me/messages/a?format=metadata") || !strings.Contains(string(body), "GET /gmail/v1/users/me/messages/b?format=metadata") {
			t.Fatal("wrong batch requests")
		}
		reply := cloudReply(200, "--fixture\r\nContent-Type: application/http\r\nContent-ID: <response-item-0>\r\n\r\nHTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"id\":\"a\",\"payload\":{\"headers\":[{\"name\":\"Subject\",\"value\":\"=?UTF-8?Q?M=C3=A1s?=\"}]}}\r\n--fixture\r\nContent-Type: application/http\r\nContent-ID: <response-item-1>\r\n\r\nHTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"id\":\"b\"}\r\n--fixture--\r\n")
		reply.Header = http.Header{"Content-Type": {"multipart/mixed; boundary=fixture"}}
		return reply, nil
	}))
	page, err := client.Messages(t.Context(), "INBOX", "")
	if err != nil || len(page.Items) != 2 || page.Items[0].Subject != "Más" || page.Items[1].ID != "b" || page.NextCursor != "next" || calls != 2 {
		t.Fatalf("page %#v, error %v, calls %d", page, err, calls)
	}
}

func TestOAuthIdentityDoesNotRequireAnyMailboxRequest(t *testing.T) {
	for _, provider := range []string{"gmail", "microsoft", "yahoo"} {
		t.Run(provider, func(t *testing.T) {
			calls := 0
			profile, err := OAuthProfile(t.Context(), provider, cloudHTTP(func(r *http.Request) (*http.Response, error) {
				calls++
				target := "https://openidconnect.googleapis.com/v1/userinfo"
				body := `{"email":"owner@example.eu","name":"Fixture"}`
				if provider == "yahoo" {
					target = "https://api.login.yahoo.com/openid/v1/userinfo"
				}
				if provider == "microsoft" {
					target = "https://graph.microsoft.com/v1.0/me?$select=mail,userPrincipalName,displayName"
					body = `{"mail":"owner@example.eu","displayName":"Fixture"}`
				}
				if r.Method != http.MethodGet || r.URL.String() != target {
					t.Errorf("identity-only opened mailbox: %s %s", r.Method, r.URL)
				}
				return cloudReply(200, body), nil
			}))
			if err != nil || calls != 1 || profile.Address != "owner@example.eu" {
				t.Fatalf("identity requires mailbox: %#v %v calls=%d", profile, err, calls)
			}
		})
	}
}

// Graph v1.0 no dice qué carpeta es cuál: sin pedir los nombres conocidos, Enviados,
// Borradores, Correo no deseado y Elementos eliminados llegaban sin rol y el cliente
// no podía traducirlas ni ordenarlas. Va en UN $batch, no en seis peticiones.
func TestGraphFoldersCarryWellKnownRolesInOneBatch(t *testing.T) {
	batches := 0
	client := NewCloudClient(Account{Provider: "microsoft"}, cloudHTTP(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/v1.0/$batch":
			batches++
			var body struct {
				Requests []struct {
					ID, Method, URL string
				} `json:"requests"`
			}
			if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Requests) != 6 {
				t.Fatalf("well-known folders not batched: %s %+v", r.Method, body)
			}
			return cloudReply(200, `{"responses":[
				{"id":"inbox","status":200,"body":{"id":"in"}},
				{"id":"sent","status":200,"body":{"id":"out"}},
				{"id":"drafts","status":200,"body":{"id":"dr"}},
				{"id":"junk","status":200,"body":{"id":"sp"}},
				{"id":"trash","status":200,"body":{"id":"del"}},
				{"id":"archive","status":404,"body":{"error":{"code":"ErrorFolderNotFound"}}},
				{"id":"forged","status":200,"body":{"id":"mine"}}]}`), nil
		case "/v1.0/me/mailFolders":
			return cloudReply(200, `{"value":[{"id":"in","displayName":"Inbox","unreadItemCount":3},{"id":"out","displayName":"Sent Items"},{"id":"dr","displayName":"Drafts"},{"id":"sp","displayName":"Junk Email"},{"id":"del","displayName":"Deleted Items"},{"id":"mine","displayName":"Projects"}]}`), nil
		}
		t.Fatalf("unexpected request %s", r.URL)
		return nil, ErrUnavailable
	}))
	folders, err := client.Folders(t.Context())
	if err != nil || batches != 1 {
		t.Fatalf("folders: %v, batches %d", err, batches)
	}
	got := map[string]string{}
	for _, f := range folders {
		got[f.ID] = f.Role
	}
	want := map[string]string{"in": "inbox", "out": "sent", "dr": "drafts", "sp": "junk", "del": "trash", "mine": ""}
	for id, role := range want {
		if got[id] != role {
			t.Errorf("folder %s role %q, want %q", id, got[id], role)
		}
	}
}

func TestGraphFoldersWithoutInboxRoleAreNotTrusted(t *testing.T) {
	client := NewCloudClient(Account{Provider: "microsoft"}, cloudHTTP(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v1.0/$batch" {
			return cloudReply(200, `{"responses":[{"id":"inbox","status":401,"body":{}}]}`), nil
		}
		t.Fatalf("listed folders after an unauthorized batch: %s", r.URL)
		return nil, ErrUnavailable
	}))
	if _, err := client.Folders(t.Context()); !errors.Is(err, ErrAuth) {
		t.Fatalf("unauthorized inbox: %v", err)
	}
}

// labels.list de Gmail no trae contadores: sin labels.get de INBOX, la Entrada de una
// cuenta de Gmail nunca decía cuántos tenía sin leer. Y si ese contador falla, las
// carpetas siguen saliendo.
func TestGmailInboxCarriesUnreadCount(t *testing.T) {
	for _, counterFails := range []bool{false, true} {
		client := NewCloudClient(Account{Provider: "gmail"}, cloudHTTP(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/gmail/v1/users/me/labels":
				return cloudReply(200, `{"labels":[{"id":"INBOX","name":"INBOX","type":"system"},{"id":"SENT","name":"SENT","type":"system"},{"id":"UNREAD","name":"UNREAD","type":"system"},{"id":"Label_1","name":"Viajes","type":"user"}]}`), nil
			case "/gmail/v1/users/me/labels/INBOX":
				if counterFails {
					return cloudReply(503, ""), nil
				}
				return cloudReply(200, `{"id":"INBOX","messagesTotal":40,"messagesUnread":7}`), nil
			}
			t.Fatalf("unexpected request %s", r.URL)
			return nil, ErrUnavailable
		}))
		folders, err := client.Folders(t.Context())
		if err != nil || len(folders) != 3 {
			t.Fatalf("folders %#v: %v", folders, err)
		}
		inbox := folders[0]
		if inbox.Role != "inbox" || (!counterFails && (inbox.Unread != 7 || inbox.Total != 40)) || (counterFails && inbox.Unread != 0) {
			t.Fatalf("inbox counters (counter fails=%v): %#v", counterFails, inbox)
		}
		if folders[1].Role != "sent" || folders[1].Unread != 0 {
			t.Fatalf("only the inbox pays an extra request: %#v", folders[1])
		}
	}
}
