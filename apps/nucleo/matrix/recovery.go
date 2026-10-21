// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"errors"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/ssss"
	"maunium.net/go/mautrix/id"
)

var ErrRecoveryExists = errors.New("matrix: recover or verify the existing account instead of replacing its identity")

// BootstrapRecovery generates random secret storage/cross-signing material in the
// client. The returned recovery key must be shown to its owner, never sent to Rooms.
// UIA is the maintained homeserver authentication callback, not a password prompt
// invented by this adapter. An existing identity is never reset implicitly.
func (c *Client) BootstrapRecovery(ctx context.Context, uia mautrix.UIACallback) (string, error) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.closed.Load() {
		return "", ErrClosed
	}
	machine := c.crypto.Machine()
	// Recheck the server instead of trusting a cached "not configured" result
	// from before another device may have established this account's identity.
	current, err := c.cli.QueryKeys(ctx, &mautrix.ReqQueryKeys{DeviceKeys: mautrix.DeviceKeysRequest{c.cli.UserID: {}}})
	if err != nil {
		return "", err
	}
	if _, exists := current.MasterKeys[c.cli.UserID]; exists {
		return "", ErrRecoveryExists
	}
	existing, err := machine.GetOwnCrossSigningPublicKeys(ctx)
	if err != nil {
		return "", err
	}
	if existing != nil {
		return "", ErrRecoveryExists
	}
	if _, err = machine.SSSS.GetDefaultKeyID(ctx); err == nil {
		return "", ErrRecoveryExists
	} else if !errors.Is(err, ssss.ErrNoDefaultKeyAccountDataEvent) {
		return "", err
	}
	recovery, _, err := machine.GenerateAndUploadCrossSigningKeys(ctx, uia, "")
	if err != nil {
		return recovery, err
	}
	if err := machine.SignOwnDevice(ctx, machine.OwnIdentity()); err != nil {
		return recovery, err
	}
	if err := machine.SignOwnMasterKey(ctx); err != nil {
		return recovery, err
	}
	if err := machine.FlushStore(ctx); err != nil {
		return recovery, err
	}
	return recovery, c.prepareBackup(ctx, recovery)
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
	if _, err := c.recoveryStorageKey(ctx, recoveryKey); err != nil {
		return err
	}
	if err := machine.VerifyWithRecoveryKey(ctx, recoveryKey); err != nil {
		return err
	}
	if err := machine.FlushStore(ctx); err != nil {
		return err
	}
	if err := c.prepareBackup(ctx, recoveryKey); err != nil {
		return err
	}
	return c.restoreBackup(ctx)
}

// VerificationStatus contains no secrets. Both the account cross-signing identity
// and this particular device must be verified before native Rooms sends content.
type VerificationStatus struct {
	HasIdentity bool
	Verified    bool
	DeviceID    string
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
	return VerificationStatus{HasIdentity: hasIdentity, Verified: deviceVerified && master && self && user, DeviceID: c.cli.DeviceID.String()}, err
}
