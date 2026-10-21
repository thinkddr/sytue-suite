// SPDX-License-Identifier: Apache-2.0
//go:build integration

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func TestWhatsAppConsentPublishesOnlyAfterCallbackAndRecovers(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	cipher, err := keycrypto.New([]byte("clave-de-pruebas-de-32-bytes...."))
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(h.Pool, cipher)
	ctx := authn.Into(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"}), authn.Claims{Subject: "alice", Tenant: "acme", Roles: []string{"owner"}})
	instance := uuid.NewString()
	const phone = "1234567890123456"
	var svc *Service
	var calls, activations atomic.Int64
	var fail atomic.Bool
	var callback atomic.Value
	fail.Store(true)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-Sytue-Internal") != "internal-test" {
			t.Error("wrong internal secret")
		}
		switch r.URL.Path {
		case "/internal/v1/rooms/whatsapp/account":
			if r.URL.Query().Get("tenant") != "acme" || r.URL.Query().Get("instance") != instance || r.URL.Query().Get("phone_number_id") != phone {
				t.Error("wrong consent scope")
			}
			_ = json.NewEncoder(w).Encode(whatsappConsentAccount{PhoneNumberID: phone, DisplayPhone: "+34910000000", AccessToken: "provider-token", AppSecret: "provider-secret", RegistrationRequired: true})
		case "/internal/v1/rooms/whatsapp/activate":
			activations.Add(1)
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			token := body["verify_token"]
			callback.Store(token)
			if body["tenant"] != "acme" || body["instance"] != instance || body["phone_number_id"] != phone || body["pin"] != "135790" || token == "" || body["callback_url"] != "https://rooms.sytue.eu/webhooks/whatsapp/"+token {
				t.Error("activation payload mismatch")
			}
			if !svc.VerifyWhatsAppSubscription(context.Background(), token, "subscribe", token) {
				t.Error("Meta cannot verify the pending callback")
			}
			accounts, err := svc.ListWhatsApp(ctx, "alice")
			if err != nil || len(accounts) != 0 {
				t.Error("unfinished account was published")
			}
			if err := svc.ReceiveWhatsApp(context.Background(), token, []byte(`{}`), ""); !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("unfinished callback accepted messages: %v", err)
			}
			if fail.Load() {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gateway.Close()
	svc = New(config.Config{PublicURL: "https://rooms.sytue.eu", MaxMessageBytes: 4096, IntegrationsURL: gateway.URL, IntegrationsInternalSecret: "internal-test"}, st, nil, nil)
	teams, err := svc.EnsureDefaultTeams(ctx, "alice", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	teamID := teams[0].ID
	member := authn.Into(ctx, authn.Claims{Subject: "bob", Tenant: "acme", Roles: []string{"member"}})
	if _, err := svc.ConnectWhatsAppConsent(member, "bob", teamID, instance, phone, "135790"); !errors.Is(err, domain.ErrForbidden) || calls.Load() != 0 {
		t.Fatalf("member reached provider: %v", err)
	}
	if _, err := svc.ConnectWhatsAppConsent(ctx, "alice", teamID, instance, phone, ""); !errors.Is(err, domain.ErrInvalidInput) || activations.Load() != 0 {
		t.Fatalf("registration without PIN: %v", err)
	}
	if _, err := svc.ConnectWhatsAppConsent(ctx, "alice", teamID, instance, phone, "135790"); !errors.Is(err, ErrRoomsIntegrationUnavailable) {
		t.Fatalf("provider rejection: %v", err)
	}
	failedToken := callback.Load().(string)
	if svc.VerifyWhatsAppSubscription(context.Background(), failedToken, "subscribe", failedToken) {
		t.Fatal("failed setup retained its callback")
	}
	fail.Store(false)
	account, err := svc.ConnectWhatsAppConsent(ctx, "alice", teamID, instance, phone, "135790")
	if err != nil || !account.Ready {
		t.Fatalf("retry: %v", err)
	}
	accounts, err := svc.ListWhatsApp(ctx, "alice")
	if err != nil || len(accounts) != 1 || accounts[0].ID != account.ID {
		t.Fatalf("finished account not published: %v", err)
	}
	raw, err := json.Marshal(account)
	if err != nil || strings.Contains(string(raw), "provider-token") || strings.Contains(string(raw), "provider-secret") {
		t.Fatal("connection response exposed provider credentials")
	}
	before := activations.Load()
	if _, err := svc.ConnectWhatsAppConsent(ctx, "alice", teamID, instance, phone, "135790"); !errors.Is(err, domain.ErrConflict) || activations.Load() != before {
		t.Fatalf("active number was overwritten: %v", err)
	}
}
