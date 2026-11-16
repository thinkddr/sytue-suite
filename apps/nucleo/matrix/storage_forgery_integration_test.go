// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration && cgo

package matrix

import (
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"testing"

	"maunium.net/go/mautrix"
	mxcrypto "maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/backup"
	"maunium.net/go/mautrix/crypto/olm"
	"maunium.net/go/mautrix/crypto/ssss"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// This fixture has only REST access and PUBLIC recovery metadata. It never
// receives the owner's recovery key, local crypto store, SSSS key or backup
// private key. A malicious homeserver has at least this write access.
func anonymousStorageBackup(ctx context.Context, t *testing.T, cli *mautrix.Client, room id.RoomID, original storage.Record) (*event.EncryptedEventContent, storage.Record) {
	t.Helper()
	return anonymousStorageBackupWithProof(ctx, t, cli, room, original, true)
}

func anonymousStorageBackupWithProof(ctx context.Context, t *testing.T, cli *mautrix.Client, room id.RoomID, original storage.Record, includeProof bool) (*event.EncryptedEventContent, storage.Record) {
	t.Helper()
	info, err := cli.GetKeyBackupLatestVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	publicBytes, err := base64.RawStdEncoding.DecodeString(string(info.AuthData.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	public, err := ecdh.X25519().NewPublicKey(publicBytes)
	if err != nil {
		t.Fatal(err)
	}
	ownerKeyID, err := ssss.NewSSSSMachine(cli).GetDefaultKeyID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	attacker, err := ssss.NewKey("")
	if err != nil {
		t.Fatal(err)
	}
	defer clear(attacker.Key)
	attacker.ID = ownerKeyID
	record := original
	record.RevisionID = "anonymous-forgery"
	record.Payload = []byte(`{"text":"anonymous forged content","integer":900719925474099312347}`)
	proof, err := sealStorageOwnerRecord(room.String(), record, attacker)
	if err != nil {
		t.Fatal(err)
	}
	account := mxcrypto.NewOlmAccount()
	outbound, err := olm.NewOutboundGroupSession()
	if err != nil {
		t.Fatal(err)
	}
	inbound, err := olm.NewInboundGroupSession([]byte(outbound.Key()))
	if err != nil {
		t.Fatal(err)
	}
	exported, err := inbound.Export(inbound.FirstKnownIndex())
	if err != nil {
		t.Fatal(err)
	}
	defer clear(exported)
	content := map[string]any{"msgtype": "m.text", "body": "", "org.sytue.storage": record}
	if includeProof {
		content["org.sytue.storage_auth"] = proof
	}
	cleartext, err := json.Marshal(map[string]any{"room_id": room, "type": event.EventMessage.Type, "content": content})
	if err != nil {
		t.Fatal(err)
	}
	defer clear(cleartext)
	ciphertext, err := outbound.Encrypt(cleartext)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := backup.EncryptSessionDataWithPubkey(public, backup.MegolmSessionData{Algorithm: id.AlgorithmMegolmV1, SenderKey: account.IdentityKey(), SenderClaimedKeys: backup.SenderClaimedKeys{Ed25519: account.SigningKey()}, SessionKey: string(exported), ForwardingKeyChain: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cli.PutKeysInBackup(ctx, info.Version, &mautrix.ReqKeyBackup{Rooms: map[id.RoomID]mautrix.ReqRoomKeyBackup{room: {Sessions: map[id.SessionID]mautrix.ReqKeyBackupData{outbound.ID(): {FirstMessageIndex: 0, ForwardedCount: 0, IsVerified: true, SessionData: payload}}}}})
	if err != nil {
		t.Fatal(err)
	}
	return &event.EncryptedEventContent{Algorithm: id.AlgorithmMegolmV1, SenderKey: account.IdentityKey(), SessionID: outbound.ID(), MegolmCiphertext: ciphertext}, record
}
