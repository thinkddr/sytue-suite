// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
)

func TestRoomsConsentUsesRealDestinationsAndNeverSendsProviderTokens(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			if r.URL.Query().Get("instance") != "account/one" {
				t.Error("instance query changed")
			}
			_, _ = w.Write([]byte(`{"items":[{"id":"remote","name":"General"}]}`))
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		switch r.URL.Path {
		case "/api/v1/channels/local/bridges":
			if !reflect.DeepEqual(body, map[string]string{"platform": "slack", "instance_id": "account/one", "remote_id": "remote"}) {
				t.Errorf("bridge payload: %v", body)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"bridge":{"id":"bridge"}}`))
		case "/api/v1/whatsapp/consent":
			if !reflect.DeepEqual(body, map[string]string{"team_id": "team", "instance_id": "business", "phone_number_id": "123", "pin": "123456"}) {
				t.Errorf("WhatsApp payload: %v", body)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"account":{"id":"number"}}`))
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := NewRooms(server.URL, nil, server.Client())
	destinations, err := client.IntegrationDestinations(context.Background(), "account/one")
	if err != nil || len(destinations) != 1 || destinations[0].ID != "remote" {
		t.Fatalf("destinations: %v %v", destinations, err)
	}
	if _, err := client.ConnectOAuthBridge(context.Background(), "local", "slack", "account/one", "remote"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ConnectWhatsAppConsent(context.Background(), RoomsWhatsAppConsent{TeamID: "team", InstanceID: "business", PhoneNumberID: "123", PIN: "123456"}); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 3 {
		t.Fatalf("unexpected request count %v", requests)
	}
}

func TestRoomsWhatsAppReturnCarriesNoSuccess(t *testing.T) {
	for _, raw := range []string{"sytue-rooms://connect/whatsapp?instance=one&code=secret", "sytue-drive://connect/whatsapp?instance=one", "sytue-rooms://user@connect/whatsapp?instance=one", "sytue-rooms://connect/whatsapp?instance=one&instance=two", "sytue-rooms://connect/callback?instance=one"} {
		u, _ := url.Parse(raw)
		if _, ok := LeerRetornoWhatsApp(u); ok {
			t.Fatalf("invalid return accepted %s", raw)
		}
	}
	u, _ := url.Parse("sytue-rooms://connect/whatsapp?instance=one")
	if id, ok := LeerRetornoWhatsApp(u); !ok || id != "one" {
		t.Fatal("fixed native return rejected")
	}
}
