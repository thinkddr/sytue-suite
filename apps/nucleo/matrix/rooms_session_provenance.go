// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	mxcrypto "maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
)

// ErrSessionProvenanceMissing requires an already authorized device to attest
// its direct SDK keys. An anonymous encrypted backup cannot authorize itself.
var ErrSessionProvenanceMissing = errors.New("matrix: recover session provenance from an authorized device")

type (
	roomWireKey struct{}
	roomWire    struct {
		content *event.EncryptedEventContent
		eventID id.EventID
		roomID  id.RoomID
	}
	stateReadKey struct{}
)

func lockedReadContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, stateReadKey{}, true)
}

func encryptedWireContext(ctx context.Context, evt *event.Event) context.Context {
	return context.WithValue(lockedReadContext(ctx), roomWireKey{}, roomWire{content: evt.Content.AsEncrypted(), eventID: evt.ID, roomID: evt.RoomID})
}

func provenanceCacheKey(session sessionproof.Session) attestedStorageSession {
	return attestedStorageSession{RoomID: session.RoomID, SessionID: session.SessionID, SenderKey: session.SenderKey, SenderUser: session.SenderUser, SigningKey: session.SigningKey}
}

func (c *Client) roomSessionCached(session sessionproof.Session) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.attested[provenanceCacheKey(session)]
	return ok
}

func (c *Client) cacheRoomSession(session sessionproof.Session) {
	c.cacheAttestedSessions([]attestedStorageSession{provenanceCacheKey(session)})
}

// Only maintained SDK direct sessions can create provenance. Backup/imported or
// forwarded keys are never upgraded merely because their public claims match.
func (c *Client) roomSessionCandidate(ctx context.Context, session *mxcrypto.InboundGroupSession, sender id.UserID) (sessionproof.Session, bool, error) {
	if session == nil || session.KeySource != id.KeySourceDirect || len(session.ForwardingChains) != 0 || !c.allowedRoomContext(ctx, session.RoomID.String()) {
		return sessionproof.Session{}, false, nil
	}
	if sender == "" {
		sender = session.SourceUser
	}
	if sender == "" {
		sender = c.session.UserID
	}
	if session.SourceUser != "" && session.SourceUser != sender {
		return sessionproof.Session{}, false, ErrIdentity
	}
	candidate := sessionproof.Session{RoomID: session.RoomID.String(), SessionID: session.ID().String(), SenderKey: session.SenderKey.String(), SenderUser: sender.String(), SigningKey: session.SigningKey.String()}
	if !candidate.Valid() {
		return sessionproof.Session{}, false, ErrIdentity
	}
	return candidate, true, nil
}

func (c *Client) authorizedRoomSession(ctx context.Context, candidate sessionproof.Session) (bool, error) {
	status, err := c.verificationStatus(ctx)
	if err != nil {
		return false, err
	}
	if !status.Verified {
		return false, ErrUnverified
	}
	machine := c.crypto.Machine()
	own := machine.OwnIdentity()
	if candidate.SenderUser == c.session.UserID.String() && candidate.SenderKey == own.IdentityKey.String() && candidate.SigningKey == own.SigningKey.String() {
		return true, nil
	}
	device, err := machine.GetOrFetchDeviceByKey(ctx, id.UserID(candidate.SenderUser), id.Curve25519(candidate.SenderKey))
	if err != nil {
		return false, err
	}
	if device == nil || device.IdentityKey.String() != candidate.SenderKey || device.SigningKey.String() != candidate.SigningKey {
		return false, nil
	}
	trust, err := machine.ResolveTrustContext(ctx, device)
	return trust >= id.TrustStateCrossSignedVerified, err
}

func (c *Client) directRoomSession(ctx context.Context, session *mxcrypto.InboundGroupSession, sender id.UserID) (sessionproof.Session, bool, error) {
	candidate, valid, err := c.roomSessionCandidate(ctx, session, sender)
	if err != nil || !valid {
		return candidate, valid, err
	}
	authorized, err := c.authorizedRoomSession(ctx, candidate)
	return candidate, authorized, err
}

func (c *Client) retainRoomSessionProof(ctx context.Context, session sessionproof.Session) error {
	if c.roomSessionCached(session) {
		return nil
	}
	encoded, err := sessionproof.Encode(c.vaultScope(), session)
	if err != nil {
		return err
	}
	defer clear(encoded.Payload)
	existing, _, found, err := c.readSecretUnlocked(ctx, c.vaultScope(), encoded.ID, 1)
	if err != nil {
		return err
	}
	defer clear(existing)
	if !found || !sessionproof.Matches(c.vaultScope(), session, existing) {
		if err := c.putSecretUnlocked(ctx, c.vaultScope(), encoded.ID, encoded.Payload, 1); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.closed.Load() {
		return ErrClosed
	}
	c.cacheRoomSession(session)
	return nil
}

func (c *Client) attestRoomSessions(ctx context.Context, sessions []*mxcrypto.InboundGroupSession) error {
	for _, session := range sessions {
		if err := ctx.Err(); err != nil {
			return err
		}
		candidate, valid, err := c.directRoomSession(ctx, session, "")
		if err != nil {
			return err
		}
		if valid {
			if err := c.retainRoomSessionProof(ctx, candidate); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Client) verifyRoomSessionProof(ctx context.Context, session sessionproof.Session) error {
	if c.roomSessionCached(session) {
		return nil
	}
	encoded, err := sessionproof.Encode(c.vaultScope(), session)
	if err != nil {
		return ErrUnverified
	}
	defer clear(encoded.Payload)
	payload, _, found, err := c.readSecretUnlocked(ctx, c.vaultScope(), encoded.ID, 1)
	if err != nil {
		return err
	}
	defer clear(payload)
	if !found {
		return fmt.Errorf("%w: %w", ErrUnverified, ErrSessionProvenanceMissing)
	}
	if !sessionproof.Matches(c.vaultScope(), session, payload) {
		return ErrUnverified
	}
	if err := c.readReady(ctx); err != nil {
		return err
	}
	c.cacheRoomSession(session)
	return nil
}

// Called before any recovered Rooms body is serialized, delivered or cached.
// Read/History pass the very ciphertext the SDK decrypted. Sync re-fetches and
// re-decrypts it, then requires identical authenticated content to avoid server
// equivocation between the sync event and the receipt endpoint.
func (c *Client) verifyRoomsEventProvenance(ctx context.Context, clearEvent *event.Event) error {
	if c.cfg.ResourceProduct != "" {
		return c.verifyResourceEventProvenance(ctx, clearEvent)
	}
	if c.cfg.Bootstrap.StorageProduct != "" || clearEvent.Mautrix.TrustState != id.TrustStateBackup {
		return nil
	}
	wire, err := c.resolveRoomWire(ctx, clearEvent)
	if err != nil {
		return err
	}
	if wire.content == nil {
		return ErrUnverified
	}
	session, err := c.crypto.Machine().CryptoStore.GetGroupSession(ctx, clearEvent.RoomID, wire.content.SessionID)
	if err != nil {
		return err
	}
	if session == nil || session.RoomID != clearEvent.RoomID || (wire.content.SenderKey != "" && wire.content.SenderKey != session.SenderKey) {
		return ErrUnverified
	}
	candidate := sessionproof.Session{RoomID: clearEvent.RoomID.String(), SessionID: session.ID().String(), SenderKey: session.SenderKey.String(), SenderUser: clearEvent.Sender.String(), SigningKey: session.SigningKey.String()}
	return c.verifyRoomSessionProof(ctx, candidate)
}

func (c *Client) resolveRoomWire(ctx context.Context, clearEvent *event.Event) (roomWire, error) {
	wire, hint := ctx.Value(roomWireKey{}).(roomWire)
	if hint && wire.content != nil && wire.eventID == clearEvent.ID && wire.roomID == clearEvent.RoomID {
		return wire, nil
	}
	encrypted, err := c.cli.GetEvent(ctx, clearEvent.RoomID, clearEvent.ID)
	if err != nil {
		return roomWire{}, err
	}
	if encrypted.Type != event.EventEncrypted || encrypted.ID != clearEvent.ID || encrypted.Sender != clearEvent.Sender || (encrypted.RoomID != "" && encrypted.RoomID != clearEvent.RoomID) {
		return roomWire{}, ErrIdentity
	}
	encrypted.RoomID = clearEvent.RoomID
	if err := encrypted.Content.ParseRaw(encrypted.Type); err != nil {
		return roomWire{}, err
	}
	wire = roomWire{content: encrypted.Content.AsEncrypted(), eventID: encrypted.ID, roomID: encrypted.RoomID}
	checked, err := c.crypto.Decrypt(ctx, encrypted)
	if err != nil {
		return roomWire{}, err
	}
	if err := sameDecryptedRoomEvent(checked, clearEvent); err != nil {
		return roomWire{}, err
	}
	return wire, nil
}

func sameDecryptedRoomEvent(checked, clearEvent *event.Event) error {
	original, err := eventContentBytes(clearEvent)
	if err != nil {
		return err
	}
	defer clear(original)
	current, err := eventContentBytes(checked)
	if err != nil {
		return err
	}
	defer clear(current)
	if checked.Type != clearEvent.Type || checked.Sender != clearEvent.Sender || checked.RoomID != clearEvent.RoomID || checked.ID != clearEvent.ID || checked.Timestamp != clearEvent.Timestamp || !bytes.Equal(original, current) {
		return ErrIdentity
	}
	return nil
}

// BackfillRoomSessionProvenance resumes per-session writes from this device's
// direct, verified SDK keys. Successful entries are independent immutable SSSS
// secrets, so retries never replace an index or discard older room history.
func (c *Client) BackfillRoomSessionProvenance(ctx context.Context) error {
	if c.cfg.Bootstrap.StorageProduct != "" {
		return ErrUnsupported
	}
	release, err := c.lockBackup(ctx)
	if err != nil {
		return err
	}
	defer release()
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.backfillRoomSessionProvenance(ctx)
}

func (c *Client) backfillRoomSessionProvenance(ctx context.Context) error {
	if c.cfg.Bootstrap.StorageProduct != "" {
		return nil
	}
	c.mu.Lock()
	for entry := range c.attested {
		if entry.SenderUser != "" {
			delete(c.attested, entry)
		}
	}
	c.mu.Unlock()
	// Close the SDK's SQLite iterator before trust lookups or SSSS HTTP writes.
	// Only PUBLIC session metadata is spooled; no ratchet/exported key, token or
	// message body is written. Memory stays bounded for arbitrarily old history.
	stale, err := filepath.Glob(filepath.Join(c.directory, ".room-session-provenance-*.jsonl"))
	if err != nil {
		return err
	}
	for _, file := range stale {
		if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	spool, err := os.CreateTemp(c.directory, ".room-session-provenance-*.jsonl")
	if err != nil {
		return err
	}
	defer func() { _ = spool.Close(); _ = os.Remove(spool.Name()) }()
	encoder := json.NewEncoder(spool)
	err = c.crypto.Machine().CryptoStore.GetAllGroupSessions(ctx).Iter(func(session *mxcrypto.InboundGroupSession) (bool, error) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		candidate, valid, err := c.roomSessionCandidate(ctx, session, "")
		if err != nil {
			return false, err
		}
		if valid {
			err = encoder.Encode(candidate)
		}
		return err == nil, err
	})
	if err != nil {
		return err
	}
	if _, err := spool.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return c.replaySpooledRoomSessions(ctx, spool)
}

func (c *Client) replaySpooledRoomSessions(ctx context.Context, spool *os.File) error {
	decoder := json.NewDecoder(spool)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var candidate sessionproof.Session
		err := decoder.Decode(&candidate)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		authorized, err := c.authorizedRoomSession(ctx, candidate)
		if err != nil {
			return err
		}
		if authorized {
			if err := c.retainRoomSessionProof(ctx, candidate); err != nil {
				return err
			}
		}
	}
}

// A device-local, non-secret migration stamp avoids scanning all SDK sessions
// on every new message. Each remote SSSS entry remains the resumable checkpoint.
func (c *Client) backfillRoomSessionProvenanceOnce(ctx context.Context) error {
	if c.cfg.Bootstrap.StorageProduct != "" {
		return nil
	}
	keyData, err := c.cfg.Secrets.Read(c.ssssSecretKey())
	if err != nil {
		return ErrSecureStore
	}
	defer clear(keyData)
	var key storedSSSSSecret
	if len(keyData) > 0 && json.Unmarshal(keyData, &key) != nil {
		return ErrSecureStore
	}
	defer clear(key.Key)
	var stamp []byte
	if key.KeyID != "" {
		stamp, err = json.Marshal(struct {
			KeyID    string `json:"key_id"`
			DeviceID string `json:"device_id"`
		}{key.KeyID, c.session.DeviceID.String()})
		if err != nil {
			return err
		}
	}
	name := "matrix-room-session-proof-v2-" + c.account
	saved, err := c.cfg.Secrets.Read(name)
	if err != nil {
		return ErrSecureStore
	}
	if len(stamp) > 0 && bytes.Equal(saved, stamp) {
		return nil
	}
	if err := c.backfillRoomSessionProvenance(ctx); err != nil {
		return err
	}
	if len(stamp) > 0 {
		if err := c.cfg.Secrets.Write(name, stamp); err != nil {
			return ErrSecureStore
		}
	}
	return nil
}
