// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build cgo

package matrix

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestSSSSVaultEnvelopeMatchesWebVectorByteForByte(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "person", Product: "drive"}
	if got := storageSessionsSecretID(scope); got != "sessions_413ada11ea36bb9ea24f2589b1363702" {
		t.Fatalf("storageSessionsSecretID mismatch: %s", got)
	}
	payload := []byte(`{"k":"artificial-vault-secret","n":9007199254740993}`)
	if got := ssssEnvelopeDigest("vault_pin", scope, 3, payload); got != "feab8c4026c3cd8b8061b4a5a5b2904ae57b8069674f16e38135bc2dca41b448" {
		t.Fatalf("ssssEnvelopeDigest mismatch: %s", got)
	}
	encoded, err := encodeSSSSVaultEnvelope("vault_pin", scope, 3, payload)
	if err != nil {
		t.Fatal(err)
	}
	wantUnpadded := "eyJraW5kIjoib3JnLnN5dHVlLnNzc3MudjEiLCJpZCI6InZhdWx0X3BpbiIsInNjb3BlIjp7InRlbmFudCI6ImFjbWUiLCJzdWJqZWN0IjoicGVyc29uIiwicHJvZHVjdCI6ImRyaXZlIn0sInJldmlzaW9uIjozLCJwYXlsb2FkIjoiZXlKcklqb2lZWEowYVdacFkybGhiQzEyWVhWc2RDMXpaV055WlhRaUxDSnVJam81TURBM01UazVNalUwTnpRd09Ua3pmUT09Iiwic2hhMjU2IjoiZmVhYjhjNDAyNmMzY2Q4YjgwNjFiNGE1YTViMjkwNGFlNTdiODA2OTY3NGYxNmUzODEzNWJjMmRjYTQxYjQ0OCJ9"
	if got := base64.RawStdEncoding.EncodeToString(encoded); got != wantUnpadded {
		t.Fatalf("unpadded SSSS envelope mismatch:\n got: %s\nwant: %s", got, wantUnpadded)
	}
	decoded, rev, err := decodeSSSSVaultEnvelope(encoded, "vault_pin", scope, 3)
	if err != nil || rev != 3 || !bytes.Equal(decoded, payload) {
		t.Fatalf("decodeSSSSVaultEnvelope mismatch: rev=%d err=%v", rev, err)
	}
	if _, _, err := decodeSSSSVaultEnvelope(encoded, "vault_pin", scope, 4); !errors.Is(err, ErrIdentity) {
		t.Fatalf("expected rollback rejection, got %v", err)
	}
	if _, _, err := decodeSSSSVaultEnvelope(encoded, "other_id", scope, 1); !errors.Is(err, ErrIdentity) {
		t.Fatalf("expected ID substitution rejection, got %v", err)
	}
	otherScope := scope
	otherScope.Product = "mail"
	if _, _, err := decodeSSSSVaultEnvelope(encoded, "vault_pin", otherScope, 1); !errors.Is(err, ErrIdentity) {
		t.Fatalf("expected scope substitution rejection, got %v", err)
	}
}

func TestSSSSVaultAndAttestedStorageRecoveryRejectUnauthenticatedBackupInjection(t *testing.T) {
	var mu sync.Mutex
	deviceKeysByDevice := map[string]json.RawMessage{}
	accountData := map[string]json.RawMessage{}
	readEvents := map[string]json.RawMessage{}
	var backupVersion json.RawMessage
	var keyBackup json.RawMessage
	var crossSigning map[string]json.RawMessage
	currentWhoamiDevice := "DEV_WRITER"
	var eventSeq int

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/account/whoami"):
			_, _ = io.WriteString(w, `{"user_id":"@sytue_person:matrix.test","device_id":"`+currentWhoamiDevice+`"}`)
		case strings.Contains(r.URL.Path, "/account_data/"):
			key := strings.SplitN(r.URL.Path, "/account_data/", 2)[1]
			switch {
			case r.Method == http.MethodPut:
				accountData[key], _ = io.ReadAll(r.Body)
				_, _ = io.WriteString(w, `{}`)
			case accountData[key] != nil:
				_, _ = w.Write(accountData[key])
			default:
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errcode":"M_NOT_FOUND"}`)
			}
		case strings.HasSuffix(r.URL.Path, "/keys/device_signing/upload"):
			_ = json.NewDecoder(r.Body).Decode(&crossSigning)
			_, _ = io.WriteString(w, `{}`)
		case strings.HasSuffix(r.URL.Path, "/keys/signatures/upload"):
			_, _ = io.WriteString(w, `{"failures":{}}`)
		case strings.HasSuffix(r.URL.Path, "/room_keys/version"):
			switch {
			case r.Method == http.MethodPost:
				backupVersion, _ = io.ReadAll(r.Body)
				_, _ = io.WriteString(w, `{"version":"1"}`)
			case len(backupVersion) > 0:
				var version map[string]any
				_ = json.Unmarshal(backupVersion, &version)
				version["version"] = "1"
				version["count"] = 1
				_ = json.NewEncoder(w).Encode(version)
			default:
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errcode":"M_NOT_FOUND"}`)
			}
		case strings.HasSuffix(r.URL.Path, "/room_keys/keys"):
			switch {
			case r.Method == http.MethodPut:
				keyBackup, _ = io.ReadAll(r.Body)
				_, _ = io.WriteString(w, `{"count":1,"etag":"one"}`)
			case len(keyBackup) > 0:
				_, _ = w.Write(keyBackup)
			default:
				_, _ = io.WriteString(w, `{"rooms":{}}`)
			}
		case strings.HasSuffix(r.URL.Path, "/keys/query"):
			devs := map[string]any{}
			for id, raw := range deviceKeysByDevice {
				devs[id] = raw
			}
			response := map[string]any{"device_keys": map[string]any{"@sytue_person:matrix.test": devs}}
			for name, keys := range crossSigning {
				response[name+"s"] = map[string]any{"@sytue_person:matrix.test": keys}
			}
			_ = json.NewEncoder(w).Encode(response)
		case strings.HasSuffix(r.URL.Path, "/keys/upload"):
			var body struct {
				DeviceKeys json.RawMessage `json:"device_keys"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body.DeviceKeys) > 0 {
				var parsed struct {
					DeviceID string `json:"device_id"`
				}
				_ = json.Unmarshal(body.DeviceKeys, &parsed)
				if parsed.DeviceID != "" {
					deviceKeysByDevice[parsed.DeviceID] = body.DeviceKeys
				}
			}
			_, _ = io.WriteString(w, `{"one_time_key_counts":{"signed_curve25519":50}}`)
		case strings.Contains(r.URL.Path, "/state/m.room.encryption"):
			_, _ = io.WriteString(w, `{"algorithm":"m.megolm.v1.aes-sha2"}`)
		case strings.Contains(r.URL.Path, "/state/m.room.history_visibility"):
			_, _ = io.WriteString(w, `{"history_visibility":"joined"}`)
		case strings.HasSuffix(r.URL.Path, "/joined_members"):
			_, _ = io.WriteString(w, `{"joined":{"@sytue_person:matrix.test":{}}}`)
		case strings.HasSuffix(r.URL.Path, "/members"):
			_, _ = io.WriteString(w, `{"chunk":[{"type":"m.room.member","state_key":"@sytue_person:matrix.test","content":{"membership":"join"}}]}`)
		case strings.Contains(r.URL.Path, "/join"):
			_, _ = io.WriteString(w, `{"room_id":"!drive:matrix.test"}`)
		case strings.Contains(r.URL.Path, "/sendToDevice/"):
			_, _ = io.WriteString(w, `{}`)
		case strings.Contains(r.URL.Path, "/send/m.room.encrypted/"):
			eventSeq++
			evtID := "$evt_1"
			if eventSeq > 1 {
				evtID = "$evt_2"
			}
			rawContent, _ := io.ReadAll(r.Body)
			storedEvt, _ := json.Marshal(map[string]any{
				"event_id":         evtID,
				"room_id":          "!drive:matrix.test",
				"sender":           "@sytue_person:matrix.test",
				"type":             "m.room.encrypted",
				"origin_server_ts": 1792152000000,
				"content":          json.RawMessage(rawContent),
			})
			readEvents[evtID] = storedEvt
			_, _ = io.WriteString(w, `{"event_id":"`+evtID+`"}`)
		case strings.Contains(r.URL.Path, "/event/"):
			evtID := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			if data, ok := readEvents[evtID]; ok {
				_, _ = w.Write(data)
			} else {
				http.Error(w, `{"errcode":"M_NOT_FOUND"}`, http.StatusNotFound)
			}
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	defer server.Close()

	ctx := t.Context()
	writerCfg := storageConfig(t, server, "drive")
	writerCfg.Bootstrap.MatrixUserID = "@sytue_person:matrix.test"
	writerCfg.Subject = "person"
	writer, err := newClient(writerCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Close() }()
	writer.session = credentials{
		Homeserver:  server.URL,
		UserID:      "@sytue_person:matrix.test",
		DeviceID:    "DEV_WRITER",
		AccessToken: "artificial-writer-token",
		Subject:     writerCfg.Subject,
		Tenant:      writerCfg.Tenant,
	}
	if err := writer.loadMaster(true); err != nil {
		t.Fatal(err)
	}
	if err := writer.initialize(ctx, true); err != nil {
		t.Fatal(err)
	}
	recoveryKey, err := writer.BootstrapRecovery(ctx, func(*mautrix.RespUserInteractive) any {
		return map[string]any{"type": "m.login.dummy"}
	})
	if err != nil {
		t.Fatal(err)
	}
	writer.ready.Store(true)

	// Missing secret returns found == false without error.
	if got, rev, found, err := writer.ReadSecret(ctx, "personal_key", 1); err != nil || found || rev != 0 || got != nil {
		t.Fatalf("unexpected missing secret result: found=%v rev=%d err=%v", found, rev, err)
	}
	secretValue := []byte(`{"vault":"secret-bytes","precise":9007199254740993}`)
	if err := writer.PutSecret(ctx, "personal_key", secretValue, 2); err != nil {
		t.Fatal(err)
	}
	gotSecret, gotRev, found, err := writer.ReadSecret(ctx, "personal_key", 2)
	if err != nil || !found || gotRev != 2 || !bytes.Equal(gotSecret, secretValue) {
		t.Fatalf("ReadSecret roundtrip failed: found=%v rev=%d err=%v", found, gotRev, err)
	}
	if _, _, _, err := writer.ReadSecret(ctx, "personal_key", 3); !errors.Is(err, ErrIdentity) {
		t.Fatalf("expected rollback rejection, got %v", err)
	}

	// Moving encrypted account data to another event name fails HKDF/ID binding.
	mu.Lock()
	accountData["org.sytue.vault.moved_key"] = bytes.Clone(accountData["org.sytue.vault.personal_key"])
	mu.Unlock()
	if _, _, _, err := writer.ReadSecret(ctx, "moved_key", 1); err == nil {
		t.Fatal("moved SSSS secret did not fail verification")
	}

	// Publish private storage record + Backup() (attests direct session into SSSS).
	scope := writer.storageScope()
	rec := storage.Record{
		Version:    1,
		Scope:      scope,
		ObjectID:   "doc_1",
		RevisionID: "rev_1",
		Role:       "content",
		Payload:    json.RawMessage(`{"title":"confidential","amount":9007199254740993}`),
	}
	ref, err := writer.PutStorageRecord(ctx, "tx_1", rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Backup(ctx); err != nil {
		t.Fatal(err)
	}

	// Second fresh device recovers via Recovery Key and reads the SSSS-attested private record.
	mu.Lock()
	currentWhoamiDevice = "DEV_READER"
	mu.Unlock()
	readerCfg := storageConfig(t, server, "drive")
	readerCfg.Bootstrap.MatrixUserID = "@sytue_person:matrix.test"
	readerCfg.Subject = "person"
	readerCfg.StoreDir = t.TempDir()
	readerCfg.Secrets = &memorySecrets{values: map[string][]byte{}}
	reader, err := newClient(readerCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	reader.session = credentials{
		Homeserver:  server.URL,
		UserID:      "@sytue_person:matrix.test",
		DeviceID:    "DEV_READER",
		AccessToken: "artificial-reader-token",
		Subject:     readerCfg.Subject,
		Tenant:      readerCfg.Tenant,
	}
	if err := reader.loadMaster(true); err != nil {
		t.Fatal(err)
	}
	if err := reader.initialize(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := reader.Recover(ctx, recoveryKey); err != nil {
		t.Fatal(err)
	}
	reader.ready.Store(true)

	recoveredSecret, recoveredRev, found, err := reader.ReadSecret(ctx, "personal_key", 2)
	if err != nil || !found || recoveredRev != 2 || !bytes.Equal(recoveredSecret, secretValue) {
		t.Fatalf("second device ReadSecret failed: found=%v rev=%d err=%v", found, recoveredRev, err)
	}

	recoveredRec, err := reader.ReadStorageRecord(ctx, ref, "doc_1", "content")
	if err != nil || !bytes.Equal(recoveredRec.Payload, rec.Payload) {
		t.Fatalf("recovered SSSS-attested storage record failed: %v", err)
	}

	// Verify that an unattested backup-restored event (Curve25519 backup injection) is rejected!
	unattested := DecryptedEvent{
		Reference: storage.Reference{RoomID: "!drive:matrix.test", EventID: "$injected"},
		Sender:    "@sytue_person:matrix.test",
		Type:      event.EventMessage.Type,
		SessionID: "unattested_injected_backup_session",
		SenderKey: "unattested",
		Recovered: true,
	}
	if err := reader.verifyRecoveredStorageEvent(ctx, scope, unattested); !errors.Is(err, ErrUnverified) {
		t.Fatalf("unattested Curve25519 backup session was accepted: %v", err)
	}
}
