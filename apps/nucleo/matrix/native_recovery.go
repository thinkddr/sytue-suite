// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"

	"maunium.net/go/mautrix/crypto/ssss"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

// NativeRecoveryConfig permits acknowledged SSSS recovery between authorized
// apps on one OS user/session. Device credentials, SDK master/database keys and
// pending recovery setups stay private. Binding is the persisted login revision,
// preserved by refresh and invalidated by logout; Authorize checks its witness.
type NativeRecoveryConfig struct {
	Binding   string
	Secrets   Secrets
	Authorize func(context.Context) error
}

type nativeRecoveryEnvelope struct {
	Binding string          `json:"binding"`
	Secret  json.RawMessage `json:"secret"`
}

func (c *Client) nativeRecoveryKey() string {
	scope := "storage:drive\x00" + c.cfg.Bootstrap.HomeserverURL + "\x00" + c.cfg.Bootstrap.MatrixUserID + "\x00" + c.cfg.Subject + "\x00" + c.cfg.Tenant
	return auth.PrefijoRecuperacionAlmacen + matrixAccountName(scope) + "-" + c.cfg.NativeRecovery.Binding
}

func (c *Client) authorizeNativeRecovery(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cfg := c.cfg.NativeRecovery
	if c.closed.Load() {
		return ErrClosed
	}
	if cfg == nil || cfg.Secrets == nil || cfg.Authorize == nil || c.cfg.Bootstrap.StorageProduct != "drive" || len(cfg.Binding) != 32 {
		return ErrIdentity
	}
	binding, err := hex.DecodeString(cfg.Binding)
	defer clear(binding)
	if err != nil || hex.EncodeToString(binding) != cfg.Binding {
		return ErrIdentity
	}
	return cfg.Authorize(ctx)
}

func (c *Client) decodeNativeRecovery(raw []byte) (*ssss.Key, error) {
	var envelope nativeRecoveryEnvelope
	if len(raw) > 8192 || json.Unmarshal(raw, &envelope) != nil || envelope.Binding != c.cfg.NativeRecovery.Binding {
		return nil, ErrSecureStore
	}
	defer clear(envelope.Secret)
	return c.decodeStorageAuthKey(envelope.Secret)
}

// SyncNativeRecovery restores an independent device using the maintained SDK,
// or publishes only an already verified, acknowledged owner's SSSS material.
// Missing material preserves the manual recovery UI; malformed/foreign material
// fails closed. Every OS-store operation is bracketed by the login witness.
func (c *Client) SyncNativeRecovery(ctx context.Context) error {
	if c.cfg.NativeRecovery == nil {
		return nil
	}
	if err := c.authorizeNativeRecovery(ctx); err != nil {
		return err
	}
	status, err := c.VerificationStatus(ctx)
	if err != nil {
		return err
	}
	if status.RecoverySetupPending {
		return ErrRecoveryRequired
	}
	if !status.Verified {
		raw, err := c.cfg.NativeRecovery.Secrets.Read(c.nativeRecoveryKey())
		if err != nil {
			return ErrSecureStore
		}
		defer clear(raw)
		if err := c.authorizeNativeRecovery(ctx); err != nil {
			return err
		}
		if len(raw) == 0 {
			return ErrRecoveryRequired
		}
		key, err := c.decodeNativeRecovery(raw)
		if err != nil {
			return err
		}
		defer clear(key.Key)
		if err := c.Recover(ctx, key.RecoveryKey()); err != nil {
			return err
		}
	}
	return c.publishNativeRecovery(ctx)
}

func (c *Client) publishNativeRecovery(ctx context.Context) error {
	if err := c.authorizeNativeRecovery(ctx); err != nil {
		return err
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	status, err := c.verificationStatus(ctx)
	if err != nil {
		return err
	}
	if !status.Verified || status.RecoverySetupPending {
		return ErrRecoveryRequired
	}
	key, err := c.storageAuthKey(ctx, "")
	if err != nil {
		return err
	}
	defer clear(key.Key)
	raw, err := c.encodeStorageAuthKey(key)
	if err != nil {
		return err
	}
	defer clear(raw)
	envelope, err := json.Marshal(nativeRecoveryEnvelope{Binding: c.cfg.NativeRecovery.Binding, Secret: raw})
	if err != nil {
		return err
	}
	defer clear(envelope)
	name := c.nativeRecoveryKey()
	previous, err := c.cfg.NativeRecovery.Secrets.Read(name)
	if err != nil {
		return ErrSecureStore
	}
	defer clear(previous)
	if len(previous) != 0 {
		existing, err := c.decodeNativeRecovery(previous)
		if err != nil {
			return err
		}
		defer clear(existing.Key)
		if existing.ID != key.ID || !bytes.Equal(existing.Key, key.Key) {
			return ErrRecoveryExists
		}
	}
	if err := c.authorizeNativeRecovery(ctx); err != nil {
		return err
	}
	if err = c.cfg.NativeRecovery.Secrets.Write(name, envelope); err != nil {
		return ErrSecureStore
	}
	checked, err := c.cfg.NativeRecovery.Secrets.Read(name)
	if err != nil {
		return ErrSecureStore
	}
	defer clear(checked)
	if !bytes.Equal(checked, envelope) {
		return ErrSecureStore
	}
	return c.authorizeNativeRecovery(ctx)
}
