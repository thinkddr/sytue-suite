//go:build integration

// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

func TestStoragePublicationOriginRealSynapseReadsAfterProducerLogout(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_ORIGIN_FIXTURE") != "1" {
		t.Skip("disposable remote Synapse and module required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	base := "http://127.0.0.1:18978"
	server := "storage.test:18978"
	scope := storage.Scope{Tenant: "acme", Subject: uuid.NewString(), Product: "mail"}
	producer, err := mautrix.NewClient(base, "", "")
	if err != nil {
		t.Fatal(err)
	}
	user := "sytue_storage_mail_acme_" + scope.Subject
	matrixprooftest.Register(ctx, t, producer, user, uuid.NewString(), id.DeviceID("PRODUCER"))
	controller, err := mautrix.NewClient(base, "", "")
	if err != nil {
		t.Fatal(err)
	}
	matrixprooftest.Register(ctx, t, controller, "rooms_controller", uuid.NewString(), id.DeviceID("CONTROL"))
	room := matrixprooftest.EncryptedStorageRoom(ctx, t, controller, producer.UserID)
	if _, err := producer.JoinRoomByID(ctx, room); err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"algorithm":"m.megolm.v1.aes-sha2","session_id":"synthetic-source-boundary","ciphertext":"positive-readback"}`)
	sent, err := producer.SendMessageEvent(ctx, room, event.EventEncrypted, raw, mautrix.ReqSendEvent{TransactionID: "positive-readback"})
	if err != nil {
		t.Fatal(err)
	}
	source := New(Config{InternalURL: base, HomeserverURL: "https://" + server, ServerName: server, SSOProviderID: "artificial", AdminToken: controller.AccessToken})
	ref := storage.Reference{RoomID: room.String(), EventID: sent.EventID.String()}
	scoped := tenancy.Into(ctx, tenancy.Tenant{Slug: scope.Tenant})
	for phase := range 2 {
		origin, err := source.StoragePublicationOrigin(scoped, scope, ref)
		if err != nil || origin.Reference != ref || origin.Sender != producer.UserID.String() {
			t.Fatal("persisted opaque readback failed", phase, err)
		}
		if phase == 0 {
			if _, err := producer.Logout(ctx); err != nil {
				t.Fatal(err)
			}
			// This proves readback does not impersonate the withdrawn producer.
			if _, err := producer.MakeRequest(ctx, http.MethodGet, producer.BuildClientURL("v3", "account", "whoami"), nil, nil); err == nil {
				t.Fatal("producer logout was not exercised")
			}
		}
	}
}

func TestRetainedPublicationOriginRealSynapseCanonicalReadAfterLogout(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_ORIGIN_FIXTURE") != "1" {
		t.Skip("disposable remote Synapse module required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	base := "http://127.0.0.1:18978"
	server := "storage.test:18978"
	scope := storage.Scope{Tenant: "acme", Subject: uuid.NewString(), Product: "mail"}
	producer, err := mautrix.NewClient(base, "", "")
	if err != nil {
		t.Fatal(err)
	}
	matrixprooftest.Register(ctx, t, producer, "sytue_"+scope.Subject, uuid.NewString(), id.DeviceID("CANONICAL"))
	reader, err := mautrix.NewClient(base, "", "")
	if err != nil {
		t.Fatal(err)
	}
	matrixprooftest.Register(ctx, t, reader, "canonical_reader_"+uuid.NewString(), uuid.NewString(), id.DeviceID("READER"))
	room := matrixprooftest.EncryptedStorageRoom(ctx, t, reader, producer.UserID)
	if _, err := producer.JoinRoomByID(ctx, room); err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"algorithm":"m.megolm.v1.aes-sha2","session_id":"synthetic-source-boundary","ciphertext":"canonical-readback"}`)
	sent, err := producer.SendMessageEvent(ctx, room, event.EventEncrypted, raw, mautrix.ReqSendEvent{TransactionID: "canonical-readback"})
	if err != nil {
		t.Fatal(err)
	}
	source := New(Config{InternalURL: base, HomeserverURL: "https://" + server, ServerName: server, SSOProviderID: "artificial", AdminToken: reader.AccessToken})
	ref := storage.Reference{RoomID: room.String(), EventID: sent.EventID.String()}
	scoped := tenancy.Into(ctx, tenancy.Tenant{Slug: scope.Tenant})
	if _, err := source.StoragePublicationOrigin(scoped, scope, ref); err == nil {
		t.Fatal("isolated readback accepted canonical sender")
	}
	if _, err := producer.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := producer.MakeRequest(ctx, http.MethodGet, producer.BuildClientURL("v3", "account", "whoami"), nil, nil); err == nil {
		t.Fatal("canonical producer logout not exercised")
	}
	origin, err := source.RetainedPublicationOrigin(scoped, scope, ref, producer.UserID.String())
	if err != nil || origin.Reference != ref || origin.Sender != producer.UserID.String() {
		t.Fatal("canonical positive readback required isolated or retired producer", err)
	}
}
