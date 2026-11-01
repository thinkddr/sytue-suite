// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strconv"

	"maunium.net/go/mautrix"
	mxcrypto "maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/ssss"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const (
	ssssVaultEventPrefix   = "org.sytue.vault."
	ssssVaultEnvelopeKind  = "org.sytue.ssss.v1"
	storageSessionsKind    = "org.sytue.storage.sessions.v1"
	maxAttestedSessionRows = 512
)

type storedSSSSSecret struct {
	KeyID string `json:"key_id"`
	Key   []byte `json:"key"`
}

type ssssVaultEnvelope struct {
	Kind     string        `json:"kind"`
	ID       string        `json:"id"`
	Scope    storage.Scope `json:"scope"`
	Revision int64         `json:"revision"`
	Payload  []byte        `json:"payload"`
	SHA256   string        `json:"sha256"`
}

type attestedStorageSession struct {
	RoomID    string `json:"room_id"`
	SessionID string `json:"session_id"`
	SenderKey string `json:"sender_key"`
}

type attestedStorageSessionList struct {
	Kind     string                   `json:"kind"`
	Sessions []attestedStorageSession `json:"sessions"`
}

func (c *Client) ssssSecretKey() string { return "rooms-matrix-ssss-" + c.account }

func (c *Client) vaultScope() storage.Scope {
	product := c.cfg.Bootstrap.StorageProduct
	if product == "" {
		product = "rooms"
	}
	return storage.Scope{Tenant: c.cfg.Tenant, Subject: c.cfg.Subject, Product: product}
}

func (c *Client) saveSSSSKey(storageKey *ssss.Key) error {
	if storageKey == nil || storageKey.ID == "" || len(storageKey.Key) != 32 {
		return ErrSecureStore
	}
	raw, err := json.Marshal(storedSSSSSecret{KeyID: storageKey.ID, Key: storageKey.Key})
	if err != nil {
		return err
	}
	defer clear(raw)
	if err := c.cfg.Secrets.Write(c.ssssSecretKey(), raw); err != nil {
		return ErrSecureStore
	}
	return nil
}

func (c *Client) readSSSSKey(ctx context.Context) (*ssss.Key, error) {
	if c.closed.Load() {
		return nil, ErrClosed
	}
	raw, err := c.cfg.Secrets.Read(c.ssssSecretKey())
	if err != nil {
		return nil, ErrSecureStore
	}
	defer clear(raw)
	if len(raw) == 0 {
		return nil, ErrRecoveryRequired
	}
	var saved storedSSSSSecret
	if json.Unmarshal(raw, &saved) != nil || saved.KeyID == "" || len(saved.Key) != 32 {
		clear(saved.Key)
		return nil, ErrSecureStore
	}
	defer clear(saved.Key)
	defaultID, metadata, err := c.crypto.Machine().SSSS.GetDefaultKeyData(ctx)
	if err != nil {
		return nil, err
	}
	if defaultID != saved.KeyID || metadata == nil || !metadata.VerifyKey(saved.Key) {
		return nil, ErrRecoveryRequired
	}
	return &ssss.Key{
		ID:       defaultID,
		Key:      bytes.Clone(saved.Key),
		Metadata: metadata,
	}, nil
}

func ssssEnvelopeDigest(secretID string, scope storage.Scope, revision int64, payload []byte) string {
	h := sha256.New()
	_, _ = h.Write([]byte(ssssVaultEnvelopeKind))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(secretID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(scope.Tenant))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(scope.Subject))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(scope.Product))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(strconv.FormatInt(revision, 10)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

func encodeSSSSVaultEnvelope(secretID string, scope storage.Scope, revision int64, payload []byte) ([]byte, error) {
	if !storage.Identifier(secretID) || !scope.Valid() || revision < 1 || len(payload) > storage.MaxRecordBytes {
		return nil, storage.ErrRecord
	}
	body := payload
	if body == nil {
		body = []byte{}
	}
	env := ssssVaultEnvelope{
		Kind:     ssssVaultEnvelopeKind,
		ID:       secretID,
		Scope:    scope,
		Revision: revision,
		Payload:  body,
		SHA256:   ssssEnvelopeDigest(secretID, scope, revision, body),
	}
	return json.Marshal(env)
}

func decodeSSSSVaultEnvelope(raw []byte, expectedID string, expectedScope storage.Scope, minRevision int64) ([]byte, int64, error) {
	if !storage.Identifier(expectedID) || !expectedScope.Valid() || minRevision < 1 || len(raw) == 0 || len(raw) > storage.MaxRecordBytes*2 {
		return nil, 0, storage.ErrRecord
	}
	var env ssssVaultEnvelope
	if json.Unmarshal(raw, &env) != nil {
		return nil, 0, ErrIdentity
	}
	if env.Kind != ssssVaultEnvelopeKind || env.ID != expectedID || env.Scope != expectedScope || env.Revision < 1 || env.Payload == nil || len(env.Payload) > storage.MaxRecordBytes {
		return nil, 0, ErrIdentity
	}
	if env.Revision < minRevision {
		return nil, 0, ErrIdentity
	}
	if env.SHA256 != ssssEnvelopeDigest(expectedID, expectedScope, env.Revision, env.Payload) {
		return nil, 0, ErrIdentity
	}
	return bytes.Clone(env.Payload), env.Revision, nil
}

// PutSecret stores a context-bound personal secret inside standard Matrix SSSS
// (`org.sytue.vault.<id>`), interoperable byte-for-byte with web's SuiteMatrix.
func (c *Client) PutSecret(ctx context.Context, secretID string, payload []byte, revision int64) error {
	if c.closed.Load() {
		return ErrClosed
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	status, err := c.verificationStatus(ctx)
	if err != nil {
		return err
	}
	if !status.Verified {
		return ErrUnverified
	}
	return c.putSecretUnlocked(ctx, c.vaultScope(), secretID, payload, revision)
}

func (c *Client) putSecretUnlocked(ctx context.Context, scope storage.Scope, secretID string, payload []byte, revision int64) error {
	encoded, err := encodeSSSSVaultEnvelope(secretID, scope, revision, payload)
	if err != nil {
		return err
	}
	defer clear(encoded)
	storageKey, err := c.readSSSSKey(ctx)
	if err != nil {
		return err
	}
	defer clear(storageKey.Key)
	evtType := event.Type{Type: ssssVaultEventPrefix + secretID, Class: event.AccountDataEventType}
	return c.crypto.Machine().SSSS.SetEncryptedAccountData(ctx, evtType, encoded, storageKey)
}

// ReadSecret decrypts and validates a context-bound personal secret from SSSS.
// Missing secrets return (nil, 0, false, nil); tampered IVs, moved names, wrong
// scopes or revisions older than minRevision fail visibly with ErrIdentity.
func (c *Client) ReadSecret(ctx context.Context, secretID string, minRevision int64) ([]byte, int64, bool, error) {
	if c.closed.Load() {
		return nil, 0, false, ErrClosed
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.readSecretUnlocked(ctx, c.vaultScope(), secretID, minRevision)
}

func (c *Client) readSecretUnlocked(ctx context.Context, scope storage.Scope, secretID string, minRevision int64) ([]byte, int64, bool, error) {
	if !storage.Identifier(secretID) || !scope.Valid() || minRevision < 1 {
		return nil, 0, false, storage.ErrRecord
	}
	storageKey, err := c.readSSSSKey(ctx)
	if err != nil {
		return nil, 0, false, err
	}
	defer clear(storageKey.Key)
	evtType := event.Type{Type: ssssVaultEventPrefix + secretID, Class: event.AccountDataEventType}
	decrypted, err := c.crypto.Machine().SSSS.GetDecryptedAccountData(ctx, evtType, storageKey)
	if errors.Is(err, mautrix.MNotFound) || errors.Is(err, ssss.ErrNotEncryptedForKey) && len(decrypted) == 0 {
		if errors.Is(err, mautrix.MNotFound) {
			return nil, 0, false, nil
		}
		return nil, 0, false, ErrIdentity
	}
	if err != nil {
		return nil, 0, false, err
	}
	defer clear(decrypted)
	payload, revision, err := decodeSSSSVaultEnvelope(decrypted, secretID, scope, minRevision)
	if err != nil {
		return nil, 0, false, err
	}
	return payload, revision, true, nil
}

func storageSessionsSecretID(scope storage.Scope) string {
	sum := sha256.Sum256([]byte(storageSessionsKind + "\x00" + scope.Tenant + "\x00" + scope.Subject + "\x00" + scope.Product))
	return "sessions_" + hex.EncodeToString(sum[:16])
}

func validAttestedSession(entry attestedStorageSession) bool {
	return entry.RoomID != "" && len(entry.RoomID) <= 255 && entry.RoomID[0] == '!' &&
		entry.SessionID != "" && len(entry.SessionID) <= 255 &&
		entry.SenderKey != "" && len(entry.SenderKey) <= 255
}

func normalizeAttestedSessions(entries []attestedStorageSession) ([]attestedStorageSession, error) {
	seen := make(map[attestedStorageSession]struct{}, len(entries))
	out := make([]attestedStorageSession, 0, len(entries))
	for _, entry := range entries {
		if !validAttestedSession(entry) {
			return nil, ErrIdentity
		}
		if _, exists := seen[entry]; exists {
			continue
		}
		seen[entry] = struct{}{}
		out = append(out, entry)
	}
	slices.SortFunc(out, func(a, b attestedStorageSession) int {
		if d := cmp.Compare(a.RoomID, b.RoomID); d != 0 {
			return d
		}
		if d := cmp.Compare(a.SessionID, b.SessionID); d != 0 {
			return d
		}
		return cmp.Compare(a.SenderKey, b.SenderKey)
	})
	if len(out) > maxAttestedSessionRows {
		out = out[len(out)-maxAttestedSessionRows:]
	}
	return out, nil
}

// attestStorageSessions records direct verified private-storage Megolm sessions
// inside SSSS so a recovered device (Go or web) can distinguish authentic owner
// sessions from unauthenticated Curve25519 backup injections.
func (c *Client) attestStorageSessions(ctx context.Context, sessions []*mxcrypto.InboundGroupSession) error {
	if c.cfg.Bootstrap.StorageProduct == "" {
		return nil
	}
	scope := c.storageScope()
	if !scope.Valid() {
		return ErrIdentity
	}
	var direct []attestedStorageSession
	for _, session := range sessions {
		if session == nil || session.KeySource != id.KeySourceDirect || len(session.ForwardingChains) != 0 || !c.allowedRoom(session.RoomID.String()) {
			continue
		}
		entry := attestedStorageSession{
			RoomID:    session.RoomID.String(),
			SessionID: session.ID().String(),
			SenderKey: session.SenderKey.String(),
		}
		if validAttestedSession(entry) {
			direct = append(direct, entry)
		}
	}
	if len(direct) == 0 {
		return nil
	}
	secretID := storageSessionsSecretID(scope)
	existingBytes, currentRev, found, err := c.readSecretUnlocked(ctx, scope, secretID, 1)
	if err != nil {
		return err
	}
	defer clear(existingBytes)
	var combined []attestedStorageSession
	if found {
		var parsed attestedStorageSessionList
		if json.Unmarshal(existingBytes, &parsed) != nil || parsed.Kind != storageSessionsKind {
			return ErrIdentity
		}
		combined = append(combined, parsed.Sessions...)
	}
	combined = append(combined, direct...)
	normalized, err := normalizeAttestedSessions(combined)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(attestedStorageSessionList{
		Kind:     storageSessionsKind,
		Sessions: normalized,
	})
	if err != nil {
		return err
	}
	defer clear(raw)
	nextRev := currentRev + 1
	if nextRev < 1 {
		nextRev = 1
	}
	if err := c.putSecretUnlocked(ctx, scope, secretID, raw, nextRev); err != nil {
		return err
	}
	c.cacheAttestedSessions(normalized)
	return nil
}

func (c *Client) cacheAttestedSessions(entries []attestedStorageSession) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.attested == nil {
		c.attested = make(map[attestedStorageSession]struct{}, len(entries))
	}
	for _, entry := range entries {
		c.attested[entry] = struct{}{}
	}
}

func (c *Client) clearAttestedSessions() {
	c.mu.Lock()
	clear(c.attested)
	c.attested = nil
	c.mu.Unlock()
}

func (c *Client) verifyRecoveredStorageEvent(ctx context.Context, scope storage.Scope, value DecryptedEvent) error {
	if !value.Recovered {
		return nil
	}
	candidate := attestedStorageSession{
		RoomID:    value.Reference.RoomID,
		SessionID: value.SessionID,
		SenderKey: value.SenderKey,
	}
	if !validAttestedSession(candidate) {
		return ErrUnverified
	}
	c.mu.Lock()
	_, cached := c.attested[candidate]
	c.mu.Unlock()
	if cached {
		return nil
	}
	c.stateMu.RLock()
	raw, _, found, err := c.readSecretUnlocked(ctx, scope, storageSessionsSecretID(scope), 1)
	c.stateMu.RUnlock()
	if err != nil {
		if errors.Is(err, ErrRecoveryRequired) || errors.Is(err, ErrIdentity) {
			return ErrUnverified
		}
		return err
	}
	defer clear(raw)
	if !found {
		return ErrUnverified
	}
	var parsed attestedStorageSessionList
	if json.Unmarshal(raw, &parsed) != nil || parsed.Kind != storageSessionsKind {
		return ErrUnverified
	}
	normalized, normErr := normalizeAttestedSessions(parsed.Sessions)
	if normErr != nil {
		return ErrUnverified
	}
	c.cacheAttestedSessions(normalized)
	c.mu.Lock()
	_, ok := c.attested[candidate]
	c.mu.Unlock()
	if !ok {
		return ErrUnverified
	}
	return nil
}
