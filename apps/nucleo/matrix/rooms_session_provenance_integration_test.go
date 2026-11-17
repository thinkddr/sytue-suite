//go:build integration && cgo

// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	mxcrypto "maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/olm"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestRoomsRejectAnonymousBackupAndRecoverAuthorizedHistory(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_ROOMS_PROVENANCE") != "1" {
		t.Skip("isolated remote Synapse required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 7*time.Minute)
	defer cancel()
	target, err := url.Parse(os.Getenv("SYTUE_MATRIX_ROOMS_PROVENANCE_URL"))
	if err != nil || target.Host == "" {
		t.Fatal("isolated Synapse URL required")
	}
	reverse := httputil.NewSingleHostReverseProxy(target)
	var missing atomic.Bool
	var missingID atomic.Value
	missingID.Store("")
	proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := missingID.Load().(string)
		if id != "" && strings.HasSuffix(r.URL.Path, ssssVaultEventPrefix+id) {
			if r.Method == http.MethodGet && missing.Load() {
				w.Header().Set("content-type", "application/json")
				w.WriteHeader(404)
				_, _ = w.Write([]byte(`{"errcode":"M_NOT_FOUND","error":"artificial old-device provenance migration"}`))
				return
			}
			if r.Method == http.MethodPut {
				missing.Store(false)
			}
		}
		reverse.ServeHTTP(w, r)
	}))
	defer proxy.Close()
	actor, channel := uuid.NewString(), uuid.NewString()
	const tenant = "rooms-provenance-fixture"
	original, room, recovery, login := searchSDKDevice(ctx, t, proxy.URL, proxy.Client(), actor, tenant, channel)
	defer func() { _ = original.Close() }()
	legitimate, err := original.SendContent(ctx, room, MessageContent{Version: 1, Body: "authorized oldest room message"}, Relation{})
	if err != nil {
		t.Fatal(err)
	}
	// More than the old global 512-entry list, using real SDK Megolm keys.
	own := original.crypto.Machine().OwnIdentity()
	for range 513 {
		outbound, err := olm.NewOutboundGroupSession()
		if err != nil {
			t.Fatal(err)
		}
		session, err := mxcrypto.NewInboundGroupSession(own.IdentityKey, own.SigningKey, id.RoomID(room), outbound.Key(), 0, 100, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		session.SourceUser = original.session.UserID
		if err := original.crypto.Machine().CryptoStore.PutGroupSession(ctx, session); err != nil {
			t.Fatal(err)
		}
	}
	if err := original.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	stored, err := original.cli.GetEvent(ctx, id.RoomID(room), id.EventID(legitimate.EventID))
	if err != nil {
		t.Fatal(err)
	}
	if err := stored.Content.ParseRaw(stored.Type); err != nil {
		t.Fatal(err)
	}
	sdkSession, err := original.crypto.Machine().CryptoStore.GetGroupSession(ctx, id.RoomID(room), stored.Content.AsEncrypted().SessionID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := sessionproof.Session{RoomID: room, SessionID: sdkSession.ID().String(), SenderKey: sdkSession.SenderKey.String(), SenderUser: original.session.UserID.String(), SigningKey: sdkSession.SigningKey.String()}
	encoded, err := sessionproof.Encode(original.vaultScope(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(encoded.Payload)
	missingID.Store(encoded.ID)
	missing.Store(true)
	forgedCipher, _ := anonymousStorageBackup(ctx, t, original.cli, id.RoomID(room), storage.Record{Version: 1, Scope: original.vaultScope(), ObjectID: "forged-message", RevisionID: "forged-revision", Role: "content", Payload: []byte(`{"body":"anonymous"}`)})
	sent, err := original.cli.SendMessageEvent(ctx, id.RoomID(room), event.EventEncrypted, forgedCipher)
	if err != nil {
		t.Fatal(err)
	}
	freshSession, err := login("FRESH_PROVENANCE")
	if err != nil {
		t.Fatal(err)
	}
	cfg := original.cfg
	cfg.StoreDir = t.TempDir()
	cfg.Secrets = &memorySecrets{values: map[string][]byte{}}
	fresh := storageSynapseSession(ctx, t, cfg, credentials{Homeserver: cfg.Bootstrap.HomeserverURL, UserID: freshSession.UserID, DeviceID: freshSession.DeviceID, AccessToken: freshSession.AccessToken, Subject: actor, Tenant: tenant})
	defer func() { _ = fresh.Close() }()
	go func() { _ = fresh.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := fresh.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if err := fresh.Recover(ctx, recovery); err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.Read(ctx, legitimate); !errors.Is(err, ErrSessionProvenanceMissing) {
		t.Fatal("history without authenticated provenance did not request migration", err)
	}
	if err := fresh.BackfillRoomSessionProvenance(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.Read(ctx, legitimate); !errors.Is(err, ErrSessionProvenanceMissing) {
		t.Fatal("backup-only device promoted missing provenance", err)
	}
	if err := original.BackfillRoomSessionProvenance(ctx); err != nil {
		t.Fatal("authorized old-device backfill failed", err)
	}
	if _, err := fresh.Read(ctx, legitimate); err != nil {
		t.Fatal("oldest authorized history unreadable after >512 SDK sessions and old-device migration", err)
	}
	forged := EventRef{RoomID: room, EventID: sent.EventID.String()}
	if _, err := fresh.Read(ctx, forged); !errors.Is(err, ErrUnverified) {
		t.Fatal("anonymous public-key backup became an authenticated Rooms event", err)
	}
	if err := fresh.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.Read(ctx, forged); !errors.Is(err, ErrUnverified) {
		t.Fatal("rebackup promoted anonymous backup into an attestation", err)
	}
	t.Log("real SDK: oldest authorized room history recovered after 514 SDK sessions and old-device migration; anonymous public-key writer and rebackup promotion rejected")
}
