// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"errors"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

var ErrRecoveryExists = errors.New("matrix: recover or verify the existing account instead of replacing its identity")

// BootstrapRecovery generates random secret storage/cross-signing material in the
// client. The returned recovery key is shown to its owner or, under ADR 0081, held by
// the Rooms service for the OIDC session; it never goes to Synapse, disk or logs.
// UIA is the maintained homeserver authentication callback, not a password prompt
// invented by this adapter. An existing identity is never reset implicitly.
func (c *Client) BootstrapRecovery(ctx context.Context, uia mautrix.UIACallback) (string, error) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.closed.Load() {
		return "", ErrClosed
	}
	machine := c.crypto.Machine()
	pending, err := c.prepareRecoverySetup(ctx)
	if err != nil {
		return "", err
	}
	defer pending.clear()
	if !pending.Complete {
		if err := c.publishRecoverySetup(ctx, pending, uia); err != nil {
			return "", err
		}
	} else if err := machine.ImportCrossSigningKeys(cloneRecoverySeeds(pending.Seeds)); err != nil {
		return "", err
	}
	recovery := pending.storageKey().RecoveryKey()
	if err := machine.SignOwnDevice(ctx, machine.OwnIdentity()); err != nil {
		return "", err
	}
	if err := machine.SignOwnMasterKey(ctx); err != nil {
		return "", err
	}
	if err := machine.FlushStore(ctx); err != nil {
		return "", err
	}
	if err := c.prepareBackup(ctx, recovery); err != nil {
		return "", err
	}
	if err := c.saveStorageAuthKey(ctx, pending.storageKey()); err != nil {
		return "", err
	}
	pending.Complete = true
	if err := c.saveRecoverySetup(ctx, pending); err != nil {
		return "", err
	}
	return recovery, nil
}

// Recover imports the account's encrypted cross-signing identity from secret
// storage and signs this device, entirely on the terminal using the supplied key.
func (c *Client) Recover(ctx context.Context, recoveryKey string) error {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.closed.Load() {
		return ErrClosed
	}
	machine := c.crypto.Machine()
	key, err := c.recoveryStorageKey(ctx, recoveryKey)
	if err != nil {
		return err
	}
	defer clear(key.Key)
	if err := machine.VerifyWithRecoveryKey(ctx, recoveryKey); err != nil {
		return err
	}
	if err := machine.FlushStore(ctx); err != nil {
		return err
	}
	if err := c.prepareBackup(ctx, recoveryKey); err != nil {
		return err
	}
	if err := c.restoreBackup(ctx); err != nil {
		return err
	}
	return c.saveStorageAuthKey(ctx, key)
}

// VerificationStatus contains no secrets. Both the account cross-signing identity
// and this particular device must be verified before native Rooms sends content.
type VerificationStatus struct {
	HasIdentity          bool
	Verified             bool
	DeviceID             string
	RecoverySetupPending bool
}

func (c *Client) VerificationStatus(ctx context.Context) (VerificationStatus, error) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.verificationStatus(ctx)
}

func (c *Client) verificationStatus(ctx context.Context) (VerificationStatus, error) {
	if c.closed.Load() {
		return VerificationStatus{}, ErrClosed
	}
	machine := c.crypto.Machine()
	hasIdentity, deviceVerified, err := machine.GetOwnVerificationStatus(ctx)
	if err != nil {
		return VerificationStatus{}, err
	}
	master, self, user, err := machine.GetOwnCrossSigningVerificationStatus(ctx)
	if err == nil {
		cached, cacheErr := machine.GetOwnCrossSigningPublicKeys(ctx)
		stored, storeErr := machine.CryptoStore.GetCrossSigningKeys(ctx, c.cli.UserID)
		if cacheErr != nil || storeErr != nil {
			return VerificationStatus{}, errors.Join(cacheErr, storeErr)
		}
		if current, ok := stored[id.XSUsageMaster]; ok && cached != nil && current.Key != cached.MasterKey {
			master = false
		}
	}
	pending, pendingErr := c.readRecoverySetup()
	defer pending.clear()
	if pendingErr != nil && !errors.Is(pendingErr, errNoRecoverySetup) {
		return VerificationStatus{}, pendingErr
	}
	return VerificationStatus{HasIdentity: hasIdentity, Verified: deviceVerified && master && self && user, DeviceID: c.cli.DeviceID.String(), RecoverySetupPending: pending != nil}, err
}

// ErrResetUnsupported: private storage keeps its vault (ADR 0081 sessions and
// secrets) inside secret storage, so a reset would orphan it. Same as the web.
var ErrResetUnsupported = errors.New("matrix: private storage cannot reset its encryption")

// maxStaleBackups bounds the cleanup loop if a homeserver keeps answering a
// version it never deletes.
const maxStaleBackups = 16

// ResetRecovery is «I forgot my recovery key», with the web's resetEncryption
// contract: new secret storage key, new cross-signing identity and a new key
// backup; the old backups (sealed with the lost key) are deleted. It is only
// ever an explicit request of the account owner, gated by homeserver UIA before
// anything is deleted. Messages whose keys only lived in the old backup become
// unreadable on new devices, and other devices and contacts must verify again.
// The new recovery key is returned even after a partial failure once the new
// identity is published, because without it the account could not be recovered.
func (c *Client) ResetRecovery(ctx context.Context, uia mautrix.UIACallback) (string, error) {
	return c.resetRecovery(ctx, uia, nil)
}

// resetRecovery hace el restablecimiento. Con escrow, la clave nueva se guarda
// antes de publicar la identidad nueva y no se devuelve (ADR 0082).
func (c *Client) resetRecovery(ctx context.Context, uia mautrix.UIACallback, escrow RecoveryEscrow) (string, error) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.closed.Load() {
		return "", ErrClosed
	}
	if c.cfg.Bootstrap.StorageProduct != "" {
		return "", ErrResetUnsupported
	}
	machine := c.crypto.Machine()
	// Unlike GenerateAndUploadCrossSigningKeys, the identity is published (UIA)
	// before the new seeds overwrite secret storage: a refused or cancelled
	// authentication leaves the old identity recoverable with the old key.
	storageKey, err := machine.SSSS.GenerateAndUploadKey(ctx, "")
	if err != nil {
		return "", err
	}
	defer clear(storageKey.Key)
	keys, err := machine.GenerateCrossSigningKeys()
	if err != nil {
		return "", err
	}
	recovery := storageKey.RecoveryKey()
	shown := recovery
	if escrow != nil {
		if err := escrow(ctx, storageKey.ID, recovery); err != nil {
			return "", err
		}
		// Con custodia la clave no sale de aquí: nadie tiene que verla.
		shown = ""
	}
	if err := machine.PublishCrossSigningKeys(ctx, keys, uia); err != nil {
		return "", err
	}
	if err := machine.UploadCrossSigningKeysToSSSS(ctx, storageKey, keys); err != nil {
		return shown, err
	}
	if err := machine.SSSS.SetDefaultKeyID(ctx, storageKey.ID); err != nil {
		return shown, err
	}
	// setupBackup below overwrites the local SSSS key and backup secret of the
	// forgotten identity; until then readBackup rejects the stale ones by version.
	if err := machine.SignOwnDevice(ctx, machine.OwnIdentity()); err != nil {
		return shown, err
	}
	if err := machine.SignOwnMasterKey(ctx); err != nil {
		return shown, err
	}
	if err := machine.FlushStore(ctx); err != nil {
		return shown, err
	}
	if err := c.deleteStaleBackups(ctx); err != nil {
		return shown, err
	}
	if err := c.setupBackup(ctx, recovery, true); err != nil {
		return shown, err
	}
	status, err := c.verificationStatus(ctx)
	if err == nil && !status.Verified {
		err = ErrUnverified
	}
	if err == nil {
		c.scheduleBackup()
	}
	return shown, err
}

func (c *Client) deleteStaleBackups(ctx context.Context) error {
	for range maxStaleBackups {
		latest, err := c.cli.GetKeyBackupLatestVersion(ctx)
		if errors.Is(err, mautrix.MNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if latest == nil || latest.Version == "" {
			return errors.New("matrix: missing backup version")
		}
		if err := c.cli.DeleteKeyBackupVersion(ctx, latest.Version); err != nil {
			return err
		}
	}
	return errors.New("matrix: stale backups were not deleted")
}
