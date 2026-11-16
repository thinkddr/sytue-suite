// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"bytes"
	"context"
	"encoding/json"

	"maunium.net/go/mautrix/crypto/ssss"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type storageAuthSecret struct {
	Version    int               `json:"v"`
	Scope      storage.Scope     `json:"scope"`
	Homeserver string            `json:"homeserver"`
	UserID     string            `json:"user_id"`
	KeyID      string            `json:"key_id"`
	Key        []byte            `json:"key"`
	Metadata   *ssss.KeyMetadata `json:"metadata"`
}

// This is an app-private OS keychain entry. It is never a shared suite session,
// widget summary or FileProvider credential and requires no new entitlement.
func (c *Client) storageAuthSecretKey() string { return "rooms-matrix-storage-auth-" + c.account }

func (c *Client) decodeStorageAuthKey(raw []byte) (*ssss.Key, error) {
	var secret storageAuthSecret
	defer func() { clear(secret.Key) }()
	if len(raw) > 4096 || json.Unmarshal(raw, &secret) != nil {
		return nil, ErrSecureStore
	}
	if secret.Version != 1 || secret.Scope != c.storageScope() || !secret.Scope.Valid() || secret.Homeserver != c.cfg.Bootstrap.HomeserverURL || secret.UserID != c.cfg.Bootstrap.MatrixUserID || !ownerKeyID(secret.KeyID) || len(secret.Key) != 32 || secret.Metadata == nil || secret.Metadata.Algorithm != ssss.AlgorithmAESHMACSHA2 || !ownerProofBytes(secret.Metadata.IV, 16) || !ownerProofBytes(secret.Metadata.MAC, 32) || !secret.Metadata.VerifyKey(secret.Key) {
		return nil, ErrSecureStore
	}
	return &ssss.Key{ID: secret.KeyID, Key: bytes.Clone(secret.Key), Metadata: secret.Metadata}, nil
}

func (c *Client) storageAuthKey(ctx context.Context, keyID string) (*ssss.Key, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.closed.Load() {
		return nil, ErrClosed
	}
	if c.cfg.Bootstrap.StorageProduct == "" {
		return nil, ErrIdentity
	}
	raw, err := c.cfg.Secrets.Read(c.storageAuthSecretKey())
	if err != nil {
		return nil, ErrSecureStore
	}
	defer clear(raw)
	if len(raw) == 0 {
		return nil, ErrRecoveryRequired
	}
	key, err := c.decodeStorageAuthKey(raw)
	if err != nil {
		return nil, err
	}
	if keyID != "" && key.ID != keyID {
		clear(key.Key)
		return nil, ErrRecoveryRequired
	}
	if err := ctx.Err(); err != nil {
		clear(key.Key)
		return nil, err
	}
	if c.closed.Load() {
		clear(key.Key)
		return nil, ErrClosed
	}
	return key, nil
}

// Caller holds stateMu for recovery. Never replace another storage recovery key
// implicitly: rotating it needs a verified migration of every historical proof.
func (c *Client) saveStorageAuthKey(ctx context.Context, key *ssss.Key) error {
	if c.cfg.Bootstrap.StorageProduct == "" {
		return nil
	}
	if !validStorageAuthKey(key) {
		return ErrSecureStore
	}
	previous, err := c.cfg.Secrets.Read(c.storageAuthSecretKey())
	if err != nil {
		return ErrSecureStore
	}
	defer clear(previous)
	if len(previous) > 0 {
		existing, err := c.decodeStorageAuthKey(previous)
		if err != nil {
			return err
		}
		defer clear(existing.Key)
		if existing.ID != key.ID || !bytes.Equal(existing.Key, key.Key) {
			return ErrRecoveryExists
		}
		return nil
	}
	raw, err := c.encodeStorageAuthKey(key)
	if err != nil {
		return err
	}
	defer clear(raw)
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.closed.Load() {
		return ErrClosed
	}
	if err = c.cfg.Secrets.Write(c.storageAuthSecretKey(), raw); err != nil {
		return ErrSecureStore
	}
	checked, err := c.storageAuthKey(ctx, key.ID)
	if err != nil {
		return err
	}
	defer clear(checked.Key)
	if !bytes.Equal(checked.Key, key.Key) {
		return ErrSecureStore
	}
	return nil
}

func (c *Client) readPrivateStorageRecord(ctx context.Context, value DecryptedEvent, scope storage.Scope, own string) (storage.Record, error) {
	value, opened, err := c.openStoragePQValue(ctx, value, scope, own)
	if err != nil {
		return storage.Record{}, err
	}
	if opened {
		defer clear(value.Content)
	}
	rec, err := decodePrivateRecord(value, scope, own)
	if err != nil {
		return storage.Record{}, err
	}
	var content struct {
		Proof json.RawMessage `json:"org.sytue.storage_auth"` //nolint:tagliatelle // Matrix namespaced extension.
	}
	if json.Unmarshal(value.Content, &content) != nil {
		return storage.Record{}, storage.ErrRecord
	}
	if len(content.Proof) == 0 {
		// A legacy live, verified device can still migrate its original records.
		// Possession of the anonymous backup decryption key is not owner attestation;
		// recovered history still publishes when the exact Megolm session is
		// attested inside the owner's SSSS vault (verifyRecoveredStorageEvent).
		// A present proof is the authority on its own, mirroring client.ts.
		if !value.Recovered {
			if value.Trust < id.TrustStateCrossSignedVerified {
				return storage.Record{}, ErrUnverified
			}
		} else if err := c.verifyRecoveredStorageEvent(ctx, scope, value); err != nil {
			return storage.Record{}, err
		}
	} else {
		var proof storageOwnerProof
		if json.Unmarshal(content.Proof, &proof) != nil || !proof.valid() {
			return storage.Record{}, ErrUnverified
		}
		key, err := c.storageAuthKey(ctx, proof.KeyID)
		if err != nil {
			return storage.Record{}, err
		}
		defer clear(key.Key)
		if verifyStorageOwnerRecord(value.Reference.RoomID, rec, proof, key) != nil {
			return storage.Record{}, ErrUnverified
		}
	}
	if err := ctx.Err(); err != nil {
		return storage.Record{}, err
	}
	if c.closed.Load() {
		return storage.Record{}, ErrClosed
	}
	return rec, nil
}

func validStorageAuthKey(key *ssss.Key) bool {
	return key != nil && ownerKeyID(key.ID) && len(key.Key) == 32 && key.Metadata != nil && key.Metadata.Algorithm == ssss.AlgorithmAESHMACSHA2 && ownerProofBytes(key.Metadata.IV, 16) && ownerProofBytes(key.Metadata.MAC, 32) && key.Metadata.VerifyKey(key.Key)
}

func (c *Client) encodeStorageAuthKey(key *ssss.Key) ([]byte, error) {
	if !validStorageAuthKey(key) {
		return nil, ErrSecureStore
	}
	return json.Marshal(storageAuthSecret{Version: 1, Scope: c.storageScope(), Homeserver: c.cfg.Bootstrap.HomeserverURL, UserID: c.cfg.Bootstrap.MatrixUserID, KeyID: key.ID, Key: key.Key, Metadata: key.Metadata})
}
