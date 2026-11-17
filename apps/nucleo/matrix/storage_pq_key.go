// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
	"encoding/json"

	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

type (
	storagePQSecret = pqc.StorageKeySecret
	storagePQPin    = pqc.StorageKeyPin
)

// One private OS keychain entry; never a shared session or widget credential.
func (c *Client) storagePQSecretKey() string { return "rooms-matrix-storage-pq-" + c.account }

func storagePQSecretID(scope storage.Scope, keyID string) string {
	return pqc.StorageKeySecretID(scope, keyID)
}
func storagePQPinID(scope storage.Scope) string { return pqc.StorageKeyPinID(scope) }
func validStoragePQPin(pin storagePQPin, recoveryID string) bool {
	return pqc.ValidStorageKeyPin(pin, recoveryID)
}

func (c *Client) storagePQBinding(recoveryID string) pqc.StorageKeyBinding {
	return pqc.StorageKeyBinding{
		Scope: c.storageScope(), IdentityVersion: c.cfg.Bootstrap.StorageIdentityVersion.Normalized(),
		Homeserver: id.UserID(c.cfg.Bootstrap.MatrixUserID).Homeserver(), UserID: c.cfg.Bootstrap.MatrixUserID, RecoveryKeyID: recoveryID,
	}
}

func (c *Client) decodeStoragePQSecret(raw []byte, recoveryID string) (storagePQSecret, *pqc.Identity, error) {
	secret, identity, err := c.storageKeyDecoder.Decode(raw, c.storagePQBinding(recoveryID))
	if err != nil {
		return storagePQSecret{}, nil, ErrIdentity
	}
	return secret, identity, nil
}

func (c *Client) newStoragePQSecret(recoveryID string) (storagePQSecret, error) {
	return pqc.NewStorageKeySecret(c.storagePQBinding(recoveryID))
}

// An immutable recipient-addressed SSSS secret is saved BEFORE selecting it as
// active. Concurrent first devices can retain both keys without orphaning data;
// no later rotation overwrites a historical private identity.
func (c *Client) publishStoragePQSecret(ctx context.Context, secret storagePQSecret) error {
	name := storagePQSecretID(secret.Scope, secret.KeyID)
	raw, err := json.Marshal(secret)
	if err != nil {
		return err
	}
	defer clear(raw)
	existing, revision, found, err := c.readSecretUnlocked(ctx, secret.Scope, name, 1)
	defer clear(existing)
	if err != nil {
		return err
	}
	if found {
		if revision != 1 || !bytes.Equal(existing, raw) {
			return ErrRecoveryExists
		}
		return nil
	}
	if err := c.putSecretUnlocked(ctx, secret.Scope, name, raw, 1); err != nil {
		return err
	}
	checked, revision, found, err := c.readSecretUnlocked(ctx, secret.Scope, name, 1)
	defer clear(checked)
	if err != nil {
		return err
	}
	if !found || revision != 1 || !bytes.Equal(checked, raw) {
		return ErrRecoveryRequired
	}
	return nil
}

func (c *Client) readStoragePQSecret(ctx context.Context, pin storagePQPin, recoveryID string) (storagePQSecret, *pqc.Identity, error) {
	if !validStoragePQPin(pin, recoveryID) {
		return storagePQSecret{}, nil, ErrIdentity
	}
	raw, revision, found, err := c.readSecretUnlocked(ctx, c.storageScope(), storagePQSecretID(c.storageScope(), pin.KeyID), 1)
	defer clear(raw)
	if err != nil {
		return storagePQSecret{}, nil, err
	}
	if !found || revision != 1 {
		return storagePQSecret{}, nil, ErrRecoveryRequired
	}
	secret, identity, err := c.decodeStoragePQSecret(raw, recoveryID)
	if err != nil || secret.Pin() != pin {
		return storagePQSecret{}, nil, ErrIdentity
	}
	return secret, identity, nil
}

func (c *Client) resolveStoragePQPin(ctx context.Context, recoveryID string, create bool) (storagePQPin, error) {
	name := storagePQPinID(c.storageScope())
	raw, revision, found, err := c.readSecretUnlocked(ctx, c.storageScope(), name, 1)
	defer clear(raw)
	if err != nil {
		return storagePQPin{}, err
	}
	if found {
		var pin storagePQPin
		if revision != 1 || json.Unmarshal(raw, &pin) != nil || !validStoragePQPin(pin, recoveryID) {
			return storagePQPin{}, ErrIdentity
		}
		return pin, nil
	}
	if !create {
		return storagePQPin{}, ErrRecoveryRequired
	}
	secret, err := c.newStoragePQSecret(recoveryID)
	if err != nil {
		return storagePQPin{}, err
	}
	if err := c.publishStoragePQSecret(ctx, secret); err != nil {
		return storagePQPin{}, err
	}
	pinRaw, err := json.Marshal(secret.Pin())
	if err != nil {
		return storagePQPin{}, err
	}
	defer clear(pinRaw)
	if err := c.putSecretUnlocked(ctx, c.storageScope(), name, pinRaw, 1); err != nil {
		return storagePQPin{}, err
	}
	// Read the actual active pin after upload. A concurrent first device may have
	// selected another independently retained key before any application write.
	checked, revision, found, err := c.readSecretUnlocked(ctx, c.storageScope(), name, 1)
	defer clear(checked)
	if err != nil {
		return storagePQPin{}, err
	}
	var pin storagePQPin
	if !found || revision != 1 || json.Unmarshal(checked, &pin) != nil || !validStoragePQPin(pin, recoveryID) {
		return storagePQPin{}, ErrRecoveryRequired
	}
	return pin, nil
}

// storagePQIdentity serializes setup with the existing backup gate, then uses
// the same state-lock ordering as backup/recovery. Historical reads select their
// immutable key explicitly; they never replace the active pin or create a key.
func (c *Client) storagePQIdentity(ctx context.Context, requested string, refresh bool) (*pqc.Identity, error) {
	if requested != "" && !validFileHash(requested) {
		return nil, ErrIdentity
	}
	release, err := c.lockBackup(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.closed.Load() {
		return nil, ErrClosed
	}
	if !c.storageScope().Valid() || c.crypto == nil {
		return nil, ErrIdentity
	}
	owner, err := c.storageAuthKey(ctx, "")
	if err != nil {
		return nil, err
	}
	defer clear(owner.Key)
	cachedRaw, err := c.cfg.Secrets.Read(c.storagePQSecretKey())
	if err != nil {
		return nil, ErrSecureStore
	}
	defer clear(cachedRaw)
	cached, identity, done, err := c.checkCachedStoragePQSecret(ctx, cachedRaw, requested, owner.ID, refresh)
	if err != nil || done {
		return identity, err
	}
	pin, err := c.resolveStoragePQPin(ctx, owner.ID, requested == "" && len(cachedRaw) == 0)
	if err != nil {
		return nil, err
	}
	if len(cachedRaw) > 0 && pin != cached.Pin() {
		return nil, ErrRecoveryExists
	}
	if len(cachedRaw) == 0 {
		cached, identity, err = c.loadAndCacheStoragePQSecret(ctx, pin, owner.ID)
		if err != nil {
			return nil, err
		}
	}
	if requested == "" || requested == cached.KeyID {
		return identity, nil
	}
	return c.readHistoricalStoragePQIdentity(ctx, requested, owner.ID)
}

func (c *Client) checkCachedStoragePQSecret(ctx context.Context, cachedRaw []byte, requested, recoveryID string, refresh bool) (storagePQSecret, *pqc.Identity, bool, error) {
	if len(cachedRaw) == 0 {
		return storagePQSecret{}, nil, false, nil
	}
	cached, identity, err := c.decodeStoragePQSecret(cachedRaw, recoveryID)
	if err != nil {
		return storagePQSecret{}, nil, true, err
	}
	if !refresh && (requested == "" || requested == cached.KeyID) {
		return cached, identity, true, nil
	}
	if !refresh && requested != "" {
		historical, err := c.readHistoricalStoragePQIdentity(ctx, requested, recoveryID)
		return cached, historical, true, err
	}
	return cached, identity, false, nil
}

func (c *Client) loadAndCacheStoragePQSecret(ctx context.Context, pin storagePQPin, recoveryID string) (storagePQSecret, *pqc.Identity, error) {
	secret, active, err := c.readStoragePQSecret(ctx, pin, recoveryID)
	if err != nil {
		return storagePQSecret{}, nil, err
	}
	raw, err := json.Marshal(secret)
	if err != nil {
		return storagePQSecret{}, nil, err
	}
	defer clear(raw)
	if err := ctx.Err(); err != nil {
		return storagePQSecret{}, nil, err
	}
	if c.closed.Load() {
		return storagePQSecret{}, nil, ErrClosed
	}
	if err := c.cfg.Secrets.Write(c.storagePQSecretKey(), raw); err != nil {
		return storagePQSecret{}, nil, ErrSecureStore
	}
	return secret, active, nil
}

func (c *Client) readHistoricalStoragePQIdentity(ctx context.Context, requested, recoveryID string) (*pqc.Identity, error) {
	raw, revision, found, err := c.readSecretUnlocked(ctx, c.storageScope(), storagePQSecretID(c.storageScope(), requested), 1)
	defer clear(raw)
	if err != nil {
		return nil, err
	}
	if !found || revision != 1 {
		return nil, ErrRecoveryRequired
	}
	secret, historical, err := c.decodeStoragePQSecret(raw, recoveryID)
	if err != nil || secret.KeyID != requested {
		return nil, ErrIdentity
	}
	return historical, nil
}
