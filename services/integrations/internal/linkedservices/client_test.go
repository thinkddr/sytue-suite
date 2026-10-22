// SPDX-License-Identifier: Apache-2.0
package linkedservices

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func providerFixture(t *testing.T, provider string, fn fixtureTransport) *Client {
	t.Helper()
	client, err := NewClient(provider, &http.Client{Transport: fn})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func providerReply(r *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}
}

func TestProviderOriginsCursorsAndRedirectsCannotExfiltrateTokens(t *testing.T) {
	calls := 0
	client := providerFixture(t, "microsoft", func(r *http.Request) (*http.Response, error) {
		calls++
		return providerReply(r, 200, `{"value":[],"@odata.nextLink":"https://evil.invalid/steal"}`), nil
	})
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if _, err := client.Events(context.Background(), "calendar", from, from.Add(24*time.Hour), ""); !errors.Is(err, ErrInvalid) || calls != 1 {
		t.Fatalf("unsafe pagination: %d %v", calls, err)
	}
	for _, target := range []string{"http://graph.microsoft.com/v1.0/me/calendars", "https://graph.microsoft.com.evil.invalid/v1.0/me/calendars", "https://user@graph.microsoft.com/v1.0/me/calendars", "https://graph.microsoft.com:444/v1.0/me/calendars"} {
		if _, _, err := client.request(context.Background(), http.MethodGet, target, "", nil, nil); !errors.Is(err, ErrInvalid) {
			t.Errorf("unsafe origin accepted: %s %v", target, err)
		}
	}
	if calls != 1 {
		t.Fatal("rejected URL reached transport")
	}
	req, _ := http.NewRequest(http.MethodGet, "https://evil.invalid/redirect", http.NoBody)
	if err := client.http.CheckRedirect(req, nil); err == nil {
		t.Fatal("cross-origin redirect accepted")
	}
}

func TestCollectionPaginationStopsWhenNextLinkDisappears(t *testing.T) {
	calls := 0
	client := providerFixture(t, "microsoft", func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return providerReply(r, 200, `{"value":[{"id":"one","name":"One","canEdit":true}],"@odata.nextLink":"https://graph.microsoft.com/v1.0/me/calendars?$skiptoken=next"}`), nil
		}
		return providerReply(r, 200, `{"value":[{"id":"two","name":"Two","canEdit":false}]}`), nil
	})
	page, err := client.Collections(context.Background(), Calendar)
	if err != nil || calls != 2 || len(page.Items) != 2 || !page.Items[1].ReadOnly {
		t.Fatalf("collection pagination reused previous next link: %#v %d %v", page, calls, err)
	}
}

func TestGraphDefaultContactsDoNotDisappearWhenThereAreNoChildFolders(t *testing.T) {
	paths := []string{}
	client := providerFixture(t, "microsoft", func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/v1.0/me/contactFolders":
			return providerReply(r, 200, `{"value":[]}`), nil
		case "/v1.0/me/contacts":
			return providerReply(r, 200, `{"value":[{"id":"contact","@odata.etag":"version","displayName":"Fixture"}]}`), nil
		default:
			return providerReply(r, 200, `{"id":"contact","@odata.etag":"version","displayName":"Fixture"}`), nil
		}
	})
	collections, err := client.Collections(context.Background(), Contacts)
	if err != nil || len(collections.Items) != 1 || collections.Items[0].ID != "contacts" {
		t.Fatalf("default contacts hidden: %#v %v", collections, err)
	}
	items, err := client.Contacts(context.Background(), "contacts", "")
	if err != nil || len(items.Items) != 1 {
		t.Fatal(err)
	}
	name := "Updated"
	if _, err := client.SaveContact(context.Background(), "contacts", "contact", "version", ContactPatch{Name: &name}); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.Contains(path, "/contactFolders/contacts") {
			t.Fatalf("default alias treated as a child folder: %s", path)
		}
	}
	if len(paths) != 4 || paths[3] != "PATCH /v1.0/me/contacts/contact" {
		t.Fatalf("wrong default contact route: %v", paths)
	}
}

func TestGoogleContactPatchUsesSourceVersionAndPreservesUnknownFields(t *testing.T) {
	updates := 0
	const contact = `{"resourceName":"people/c1","etag":"person-v1","names":[{"displayName":"Before","givenName":"Before","honorificPrefix":"Dr","middleName":"Keep"}],"emailAddresses":[{"value":"fixture@example.eu","type":"work","customType":"Keep label"}],"metadata":{"sources":[{"type":"CONTACT","id":"c1","etag":"source-v1"}]},"addresses":[{"formattedValue":"Keep address"}],"photos":[{"url":"https://untrusted.invalid/private"}]}`
	client := providerFixture(t, "gmail", func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet {
			return providerReply(r, 200, contact), nil
		}
		updates++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		name := first(body, "names")
		metadata, _ := body["metadata"].(map[string]any)
		if r.Header.Get("If-Match") != "person-v1" || field(first(metadata, "sources"), "etag") != "source-v1" || field(name, "honorificPrefix") != "Dr" || field(name, "middleName") != "Keep" || body["addresses"] != nil || body["photos"] != nil || r.URL.Query().Get("updatePersonFields") != "names" {
			t.Errorf("unsafe full contact rewrite: %s %#v", r.URL, body)
		}
		return providerReply(r, 200, contact), nil
	})
	name := "Updated"
	if _, err := client.SaveContact(context.Background(), "contacts", "people/c1", "person-v1", ContactPatch{GivenName: &name}); err != nil || updates != 1 {
		t.Fatalf("update failed %d %v", updates, err)
	}
	if _, err := client.SaveContact(context.Background(), "contacts", "people/c1", "stale-v0", ContactPatch{GivenName: &name}); !errors.Is(err, ErrConflict) || updates != 1 {
		t.Fatal("stale contact overwrote remote change")
	}
}

func TestGoogleFullNameReplacementDoesNotAppendPreviousNameComponents(t *testing.T) {
	const original = `{"resourceName":"people/c1","etag":"v1","names":[{"displayName":"Dr John Henry Smith","givenName":"John","middleName":"Henry","familyName":"Smith","honorificPrefix":"Dr","honorificSuffix":"Jr"}],"metadata":{"sources":[{"type":"CONTACT","etag":"source-v1"}]}}`
	client := providerFixture(t, "gmail", func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet {
			return providerReply(r, 200, original), nil
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		name := first(body, "names")
		if field(name, "givenName") != "Alice Smith" || field(name, "familyName") != "" || name["honorificPrefix"] != nil || name["middleName"] != nil || name["honorificSuffix"] != nil {
			t.Errorf("previous full name components duplicated: %#v", name)
		}
		return providerReply(r, 200, original), nil
	})
	name := "Alice Smith"
	if _, err := client.SaveContact(context.Background(), "contacts", "people/c1", "v1", ContactPatch{Name: &name}); err != nil {
		t.Fatal(err)
	}
}

func TestGraphEventPatchIsPartialAndConcurrentChangesAreNotReplayed(t *testing.T) {
	patches := 0
	client := providerFixture(t, "microsoft", func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet {
			return providerReply(r, 200, `{"id":"event","@odata.etag":"v1","subject":"Before","type":"singleInstance","body":{"contentType":"html","content":"<p>Keep</p><img src=\"https://evil.invalid/tracker\">"},"start":{"dateTime":"2026-10-20T09:00:00","timeZone":"UTC"},"end":{"dateTime":"2026-10-20T10:00:00","timeZone":"UTC"}}`), nil
		}
		patches++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body) != 1 || body["subject"] != "Updated" || r.Header.Get("If-Match") != "v1" {
			t.Errorf("event edit flattened untouched fields: %#v", body)
		}
		return providerReply(r, 412, `{"error":"concurrent"}`), nil
	})
	title := "Updated"
	if _, err := client.SaveEvent(context.Background(), "calendar", "event", "v1", EventPatch{Summary: &title}); !errors.Is(err, ErrConflict) || patches != 1 {
		t.Fatalf("concurrent change lost/replayed %d %v", patches, err)
	}
}

func TestAllDayDatesPreserveSelectedLocalDay(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Madrid")
	from := time.Date(2026, 10, 20, 0, 0, 0, 0, loc)
	to := from.AddDate(0, 0, 1)
	day := true
	zone := "Europe/Madrid"
	title := "Fixture"
	for _, provider := range []string{"gmail", "microsoft"} {
		body, err := cloudEventPatch(provider, EventPatch{Summary: &title, Start: &from, End: &to, AllDay: &day, TimeZone: &zone})
		if err != nil {
			t.Fatal(err)
		}
		start := body["start"].(map[string]string)
		key := "date"
		expected := "2026-10-20"
		if provider == "microsoft" {
			key = "dateTime"
			expected += "T00:00:00"
		}
		if start[key] != expected {
			t.Errorf("%s shifted all-day date: %#v", provider, body)
		}
	}
}

func TestOneNoteAppendEscapesTextAndKeepsRichBody(t *testing.T) {
	calls := 0
	client := providerFixture(t, "microsoft", func(r *http.Request) (*http.Response, error) {
		calls++
		var commands []map[string]string
		_ = json.NewDecoder(r.Body).Decode(&commands)
		if r.Method != http.MethodPatch || !strings.HasSuffix(r.URL.Path, "/content") || len(commands) != 1 || commands[0]["action"] != "append" || commands[0]["target"] != "body" || strings.Contains(commands[0]["content"], "<img") || !strings.Contains(commands[0]["content"], "&lt;img") {
			t.Errorf("destructive or unsafe note edit: %#v", commands)
		}
		return providerReply(r, 204, ""), nil
	})
	if err := client.AppendNote(context.Background(), "section", "note", "<img src=tracker>\ntext"); err != nil || calls != 1 {
		t.Fatal(err)
	}
}

func TestAmbiguousWriteNeverRetriesAndPermissionFailureIsActionable(t *testing.T) {
	for _, status := range []int{503, 403, 400} {
		calls := 0
		client := providerFixture(t, "gmail", func(r *http.Request) (*http.Response, error) {
			calls++
			return providerReply(r, status, `{"error":{"status":"FAILED_PRECONDITION"}}`), nil
		})
		_, _, err := client.request(context.Background(), http.MethodPatch, googlePeople+"/people/c1:updateContact", "application/json", []byte(`{}`), nil)
		expected := ErrWriteUnknown
		if status == 403 {
			expected = ErrAuth
		}
		if status == 400 {
			expected = ErrConflict
		}
		if !errors.Is(err, expected) || calls != 1 {
			t.Fatalf("status %d: %d calls %v", status, calls, err)
		}
	}
}
