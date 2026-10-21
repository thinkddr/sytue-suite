// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestLinkedServiceWritesBindAccountVersionAndOnlyChangedFields(t *testing.T) {
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts++
		var body struct {
			Address string         `json:"expected_address"`
			ID      string         `json:"id"`
			ETag    string         `json:"etag"`
			Patch   map[string]any `json:"patch"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if r.URL.Path != "/api/v1/linked-services/own/calendar/save" || body.Address != "original@example.eu" || body.ETag != "\"revision-a\"" || body.ID != "event" || !reflect.DeepEqual(body.Patch, map[string]any{"summary": "Updated"}) {
			t.Errorf("lost account/version or flattened record: %s %#v", r.URL, body)
		}
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"code":"external_write_unknown"}}`))
	}))
	defer server.Close()
	client := NewIntegraciones(server.URL, &sesionFalsa{tokens: []string{"bearer"}}, nil)
	title := "Updated"
	_, err := client.SaveLinkedEvent(context.Background(), ExternalMailAccount{InstanceID: "own", Address: "original@example.eu"}, "calendar", "event", "\"revision-a\"", RemoteEventPatch{Summary: &title})
	if err == nil || posts != 1 {
		t.Fatalf("ambiguous change was retried: %d %v", posts, err)
	}
}

func TestLinkedReadsCarryIdentityAndNotesAppendAcceptsNoContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if r.URL.Query().Get("expected_address") != "original@example.eu" || r.URL.Query().Get("id") != "people/opaque?&" {
				t.Errorf("unsafe or missing read identity: %s", r.URL)
			}
			_, _ = w.Write([]byte(`{"id":"people/opaque?&","name":"Fixture"}`))
			return
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["id"] != "note" || body["text"] != "Append" || body["expected_address"] != "original@example.eu" || body["title"] != "" {
			t.Errorf("append reconstructed page: %#v", body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := NewIntegraciones(server.URL, &sesionFalsa{tokens: []string{"bearer"}}, nil)
	account := ExternalMailAccount{InstanceID: "own", Address: "original@example.eu"}
	if _, err := client.LinkedContact(context.Background(), account, "contacts", "people/opaque?&"); err != nil {
		t.Fatal(err)
	}
	if err := client.AppendLinkedNote(context.Background(), account, "section", "note", "Append"); err != nil {
		t.Fatal(err)
	}
}

func TestMailConsentSendsExplicitServiceSelection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Services []string `json:"services"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !reflect.DeepEqual(body.Services, []string{"mail", "calendar"}) {
			t.Errorf("wrong services: %v", body.Services)
		}
		_, _ = w.Write([]byte(`{"instance_id":"own","authorize_url":"https://accounts.google.com/authorize?state=sealed"}`))
	}))
	defer server.Close()
	client := NewIntegraciones(server.URL, &sesionFalsa{tokens: []string{"bearer"}}, nil)
	if _, err := client.MailConnect(context.Background(), "gmail", "", "mail", "calendar"); err != nil {
		t.Fatal(err)
	}
}

func TestLinkedConsentReturnsToOriginatingNativeProduct(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Section string `json:"return_to"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if NativeConsentProduct(body.Section) == "" {
			t.Errorf("unknown callback section: %s", body.Section)
		}
		_, _ = w.Write([]byte(`{"instance_id":"own","authorize_url":"https://login.microsoftonline.com/common/oauth2/v2.0/authorize?state=sealed"}`))
	}))
	defer server.Close()
	client := NewIntegraciones(server.URL, &sesionFalsa{tokens: []string{"bearer"}}, nil)
	for _, product := range []string{"calendar", "contacts", "notes"} {
		section := "native-" + product
		authorization, err := client.MailConnectDesde(context.Background(), "microsoft", "own", section, "mail", product)
		if err != nil || authorization.Section != section || NativeConsentProduct(authorization.Section) != product {
			t.Fatalf("origin lost: %s %#v %v", product, authorization, err)
		}
	}
	if _, err := client.MailConnectDesde(context.Background(), "microsoft", "own", "native-storage", "mail"); err == nil || calls != 3 {
		t.Fatal("unrelated callback product reached Mail consent")
	}
}
