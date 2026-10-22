// SPDX-License-Identifier: Apache-2.0
//go:build integration

package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func TestWhatsAppInboundPersistsOnlyGenericNotices(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	cipher, err := keycrypto.New([]byte("clave-de-pruebas-de-32-bytes...."))
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(h.Pool, cipher)
	svc := New(config.Config{PublicURL: "https://rooms.sytue.eu", MaxMessageBytes: 4096, MessagePageSize: 50}, st, nil, nil)
	ctx := authn.Into(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"}), authn.Claims{Subject: "alice", Tenant: "acme", Roles: []string{"owner"}})
	teams, err := svc.EnsureDefaultTeams(ctx, "alice", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	const token, secret, phone, recipient = "callback-token", "app-secret", "1234567890", "34600111222"
	account, err := st.CreateWhatsAppAccount(ctx, domain.WhatsAppAccount{
		TeamID: teams[0].ID, PhoneNumberID: phone, AccessToken: "provider-token", AppSecret: secret, CreatedBy: "alice",
	}, domain.HashToken(token))
	if err != nil {
		t.Fatal(err)
	}
	chat, err := st.EnsureWhatsAppChat(ctx, account, recipient, "WhatsApp", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"entry":[{"changes":[{"field":"messages","value":{"metadata":{"phone_number_id":"1234567890"},"contacts":[{"wa_id":"34600111222","profile":{"name":"PRIVATE_NAME"}}],"messages":[{"from":"34600111222","id":"wamid.privacy-text","type":"text","text":{"body":"PRIVATE_BODY key=PRIVATE_KEY"}},{"from":"34600111222","id":"wamid.privacy-file","type":"document","document":{"id":"PRIVATE_MEDIA","filename":"PRIVATE_FILENAME","caption":"PRIVATE_CAPTION"}}]}}]}]}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	for range 2 {
		if err := svc.ReceiveWhatsApp(context.Background(), token, body, signature); err != nil {
			t.Fatal(err)
		}
	}
	messages, err := st.ListMessages(ctx, chat.ChannelID, "alice", store.MessagePage{Limit: 20})
	if err != nil || len(messages) != 2 {
		t.Fatalf("deduplicated notices = %+v, %v", messages, err)
	}
	for _, msg := range messages {
		if msg.Body != domain.WhatsAppInboundNotice || msg.AuthorName != "WhatsApp" {
			t.Fatalf("provider content imported: %+v", msg)
		}
	}
	if persisted := fmt.Sprintf("%+v", messages); strings.Contains(persisted, "PRIVATE_") {
		t.Fatalf("private provider content was persisted: %s", persisted)
	}
}
