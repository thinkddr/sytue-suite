// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"maunium.net/go/mautrix"
	mxcrypto "maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/ssss"
	"maunium.net/go/mautrix/id"
)

// Only the app-private OS keychain holds this unfinished setup. No part of it
// goes to suite preferences, widget containers, OIDC sessions or Rooms metadata.
// The maintained SDK generates, encrypts and signs all cryptographic material.
var errNoRecoverySetup = errors.New("matrix: no pending recovery setup")

type recoverySetup struct {
	Version  int                        `json:"v"`
	Account  string                     `json:"account"`
	KeyID    string                     `json:"key_id"`
	Key      []byte                     `json:"key"`
	Metadata *ssss.KeyMetadata          `json:"metadata"`
	Seeds    mxcrypto.CrossSigningSeeds `json:"seeds"`
	Complete bool                       `json:"complete"`
}

func (p *recoverySetup) storageKey() *ssss.Key {
	return &ssss.Key{ID: p.KeyID, Key: p.Key, Metadata: p.Metadata}
}

// goolm retains its seed slices. The SDK must own copies independent from the
// temporary keychain envelope that this adapter clears after every operation.
func cloneRecoverySeeds(seeds mxcrypto.CrossSigningSeeds) mxcrypto.CrossSigningSeeds {
	return mxcrypto.CrossSigningSeeds{MasterKey: bytes.Clone(seeds.MasterKey), SelfSigningKey: bytes.Clone(seeds.SelfSigningKey), UserSigningKey: bytes.Clone(seeds.UserSigningKey)}
}

func (p *recoverySetup) clear() {
	if p == nil {
		return
	}
	clear(p.Key)
	clear(p.Seeds.MasterKey)
	clear(p.Seeds.SelfSigningKey)
	clear(p.Seeds.UserSigningKey)
}

func (c *Client) recoverySetupKey() string { return "rooms-matrix-recovery-setup-" + c.account }

func (c *Client) readRecoverySetup() (*recoverySetup, error) {
	raw, err := c.cfg.Secrets.Read(c.recoverySetupKey())
	if err != nil {
		return nil, ErrSecureStore
	}
	defer clear(raw)
	if len(raw) == 0 {
		return nil, errNoRecoverySetup
	}
	var pending recoverySetup
	if len(raw) > 8192 || json.Unmarshal(raw, &pending) != nil {
		pending.clear()
		return nil, ErrSecureStore
	}
	if pending.Version != 1 || pending.Account != c.account || !validStorageAuthKey(pending.storageKey()) || len(pending.Seeds.MasterKey) != 32 || len(pending.Seeds.SelfSigningKey) != 32 || len(pending.Seeds.UserSigningKey) != 32 {
		pending.clear()
		return nil, ErrSecureStore
	}
	return &pending, nil
}

func (c *Client) saveRecoverySetup(ctx context.Context, pending *recoverySetup) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.closed.Load() {
		return ErrClosed
	}
	raw, err := json.Marshal(pending)
	if err != nil {
		return err
	}
	defer clear(raw)
	if err = c.cfg.Secrets.Write(c.recoverySetupKey(), raw); err != nil {
		return ErrSecureStore
	}
	checked, err := c.readRecoverySetup()
	if err != nil {
		return ErrSecureStore
	}
	defer checked.clear()
	if checked == nil || checked.Complete != pending.Complete || checked.KeyID != pending.KeyID || !bytes.Equal(checked.Key, pending.Key) || !bytes.Equal(checked.Seeds.MasterKey, pending.Seeds.MasterKey) || !bytes.Equal(checked.Seeds.SelfSigningKey, pending.Seeds.SelfSigningKey) || !bytes.Equal(checked.Seeds.UserSigningKey, pending.Seeds.UserSigningKey) {
		return ErrSecureStore
	}
	return ctx.Err()
}

// Query the current server identity before every attempt. A pending setup may
// resume only its own exact key trio; it never adopts or resets another identity.
func (c *Client) checkRecoverySetup(ctx context.Context, pending *recoverySetup) error {
	return c.checkRecoverySetupFor(ctx, pending, false)
}

// checkRecoverySetupFor con abandoned admite una clave por defecto que AbandonedRecovery
// acaba de dar por abandonada: la sustituye la nueva (o la pendiente de un reintento).
func (c *Client) checkRecoverySetupFor(ctx context.Context, pending *recoverySetup, abandoned bool) error {
	current, err := c.cli.QueryKeys(ctx, &mautrix.ReqQueryKeys{DeviceKeys: mautrix.DeviceKeysRequest{c.cli.UserID: {}}})
	if err != nil {
		return err
	}
	if err := checkRecoverySetupPublicKeys(current, pending, c.cli.UserID); err != nil {
		return err
	}
	defaultID, err := c.crypto.Machine().SSSS.GetDefaultKeyID(ctx)
	if err == nil && abandoned && (pending == nil || defaultID != pending.KeyID) {
		// La pendiente, si la hay, es la que sustituye a la abandonada (un reintento).
		return nil
	}
	if err == nil {
		if pending == nil || defaultID != pending.KeyID {
			return ErrRecoveryExists
		}
		exists, metadataErr := c.recoverySetupMetadata(ctx, pending)
		if metadataErr != nil {
			return metadataErr
		}
		if !exists {
			return ErrRecoveryRequired
		}
		return nil
	}
	if !errors.Is(err, ssss.ErrNoDefaultKeyAccountDataEvent) {
		return err
	}
	if pending != nil && pending.Complete {
		return ErrRecoveryRequired
	}
	return nil
}

func checkRecoverySetupPublicKeys(current *mautrix.RespQueryKeys, pending *recoverySetup, userID id.UserID) error {
	if len(current.Failures) != 0 {
		return ErrRecoveryRequired
	}
	master, hasMaster := current.MasterKeys[userID]
	self, hasSelf := current.SelfSigningKeys[userID]
	user, hasUser := current.UserSigningKeys[userID]
	if pending == nil {
		if hasMaster || hasSelf || hasUser {
			return ErrRecoveryExists
		}
	} else if hasMaster || hasSelf || hasUser {
		seeds := cloneRecoverySeeds(pending.Seeds)
		keys, decodeErr := seeds.Decode()
		if decodeErr != nil {
			return ErrSecureStore
		}
		if !recoveryPublicKey(master, id.XSUsageMaster, userID, keys.MasterKey.PublicKey()) || !recoveryPublicKey(self, id.XSUsageSelfSigning, userID, keys.SelfSigningKey.PublicKey()) || !recoveryPublicKey(user, id.XSUsageUserSigning, userID, keys.UserSigningKey.PublicKey()) {
			return ErrRecoveryExists
		}
	}
	if pending != nil && pending.Complete && (!hasMaster || !hasSelf || !hasUser) {
		return ErrRecoveryExists
	}
	return nil
}

func (c *Client) recoverySetupMetadata(ctx context.Context, pending *recoverySetup) (bool, error) {
	metadata, err := c.crypto.Machine().SSSS.GetKeyData(ctx, pending.KeyID)
	if errors.Is(err, mautrix.MNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if metadata.Algorithm != pending.Metadata.Algorithm || metadata.IV != pending.Metadata.IV || metadata.MAC != pending.Metadata.MAC || !metadata.VerifyKey(pending.Key) {
		return false, ErrRecoveryExists
	}
	return true, nil
}

func recoveryPublicKey(value mautrix.CrossSigningKeys, usage id.CrossSigningUsage, user id.UserID, key id.Ed25519) bool {
	return value.UserID == user && len(value.Usage) == 1 && value.Usage[0] == usage && len(value.Keys) == 1 && value.Keys[id.NewKeyID(id.KeyAlgorithmEd25519, key.String())] == key
}

func (c *Client) prepareRecoverySetup(ctx context.Context) (*recoverySetup, error) {
	return c.prepareRecoverySetupFor(ctx, false)
}

func (c *Client) prepareRecoverySetupFor(ctx context.Context, abandoned bool) (*recoverySetup, error) {
	pending, err := c.readRecoverySetup()
	if err != nil && !errors.Is(err, errNoRecoverySetup) {
		return nil, err
	}
	if err := c.checkRecoverySetupFor(ctx, pending, abandoned); err != nil {
		pending.clear()
		return nil, err
	}
	if pending != nil {
		return pending, nil
	}
	existing, err := c.crypto.Machine().GetOwnCrossSigningPublicKeys(ctx)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrRecoveryExists
	}
	key, err := ssss.NewKey("")
	if err != nil {
		return nil, err
	}
	defer clear(key.Key)
	keys, err := c.crypto.Machine().GenerateCrossSigningKeys()
	if err != nil {
		clear(key.Key)
		return nil, err
	}
	seeds := keys.Seeds()
	pending = &recoverySetup{Version: 1, Account: c.account, KeyID: key.ID, Key: bytes.Clone(key.Key), Metadata: key.Metadata, Seeds: cloneRecoverySeeds(seeds)}
	if err = c.saveRecoverySetup(ctx, pending); err != nil {
		pending.clear()
		return nil, err
	}
	return pending, nil
}

func (c *Client) publishRecoverySetup(ctx context.Context, pending *recoverySetup, uia mautrix.UIACallback) error {
	machine := c.crypto.Machine()
	exists, err := c.recoverySetupMetadata(ctx, pending)
	if err != nil {
		return err
	}
	if !exists {
		if err := machine.SSSS.SetKeyData(ctx, pending.KeyID, pending.Metadata); err != nil {
			return err
		}
	}
	seeds := cloneRecoverySeeds(pending.Seeds)
	keys, err := seeds.Decode()
	if err != nil {
		return ErrSecureStore
	}
	if err := machine.UploadCrossSigningKeysToSSSS(ctx, pending.storageKey(), &keys); err != nil {
		return err
	}
	if err := machine.PublishCrossSigningKeys(ctx, &keys, uia); err != nil {
		return err
	}
	return machine.SSSS.SetDefaultKeyID(ctx, pending.KeyID)
}

// AcknowledgeRecoverySetup retires the local journal only after the complete
// identity is still ours and the device is verified. A failed deletion keeps the
// owner-facing acknowledgment pending; an incomplete setup cannot be discarded.
func (c *Client) AcknowledgeRecoverySetup(ctx context.Context) error {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.closed.Load() {
		return ErrClosed
	}
	pending, err := c.readRecoverySetup()
	if errors.Is(err, errNoRecoverySetup) {
		return nil
	}
	if err != nil {
		return err
	}
	defer pending.clear()
	if !pending.Complete {
		return ErrRecoveryRequired
	}
	if err := c.checkRecoverySetup(ctx, pending); err != nil {
		return err
	}
	status, err := c.verificationStatus(ctx)
	if err != nil {
		return err
	}
	if !status.Verified {
		return ErrUnverified
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err = c.cfg.Secrets.Delete(c.recoverySetupKey()); err != nil {
		return ErrSecureStore
	}
	remaining, err := c.readRecoverySetup()
	defer remaining.clear()
	if errors.Is(err, errNoRecoverySetup) {
		return nil
	}
	if err != nil {
		return err
	}
	return ErrSecureStore
}
