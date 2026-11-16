// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"time"

	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type (
	storagePublicationContext struct{}
	storagePublication        struct {
		client   *Client
		scope    storage.Scope
		target   storage.PublicationTarget
		identity storage.IdentityVersion
		policy   StoragePublicationPolicy
	}
)

// RequireRecoverableStorage grants a local publication context only after the
// verified storage device has its owner key and authenticated SDK backup. The
// context is bound to this exact client, scope and original room/device target.
func (c *Client) RequireRecoverableStorage(ctx context.Context, scope storage.Scope, target storage.PublicationTarget) (context.Context, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	binding, err := c.PrepareStorage(ctx, scope)
	if err != nil {
		return nil, err
	}
	if !binding.Valid() || target.DeviceID != binding.Target.DeviceID {
		return nil, ErrIdentity
	}
	bound, err := c.bindStoragePublication(ctx, scope, target)
	if err != nil {
		return nil, err
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if err := c.validateStoragePublication(bound, target.RoomID); err != nil {
		return nil, err
	}
	if _, _, err := c.readBackup(bound); err != nil {
		return nil, err
	}
	return bound, nil
}

// StoragePublicationPolicy checks the original authorization and generation.
// It may use the SDK signer or HTTP, so it is never called under stateMu or
// backupMu. It must not publish storage records or replace the captured intent.
type StoragePublicationPolicy = storage.PublicationPolicy

// RequireAuthorizedStoragePublication adds a mandatory live policy to the
// recoverable context. Repeated preparation preserves it; replacing it is denied.
func (c *Client) RequireAuthorizedStoragePublication(ctx context.Context, scope storage.Scope, target storage.PublicationTarget, policy StoragePublicationPolicy) (context.Context, error) {
	if policy == nil {
		return nil, ErrIdentity
	}
	bound, err := c.RequireRecoverableStorage(ctx, scope, target)
	if err != nil {
		return nil, err
	}
	proof, ok := bound.Value(storagePublicationContext{}).(storagePublication)
	if !ok || proof.policy != nil {
		return nil, ErrIdentity
	}
	proof.policy = policy
	return context.WithValue(bound, storagePublicationContext{}, proof), nil
}

func storagePublicationPolicy(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	proof, ok := ctx.Value(storagePublicationContext{}).(storagePublication)
	if ok && proof.policy != nil {
		return proof.policy(ctx)
	}
	return nil
}

func (c *Client) validateStoragePublication(ctx context.Context, room string) error {
	proof, ok := ctx.Value(storagePublicationContext{}).(storagePublication)
	if !ok || proof.client != c || proof.scope != c.storageScope() || proof.target.RoomID != room || proof.identity != c.cfg.Bootstrap.StorageIdentityVersion.Normalized() || !proof.identity.Valid() {
		return ErrIdentity
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.checkStoragePublicationTarget(proof.target)
}

// The caller holds stateMu after backing up without any state lock. Checking
// this exact session prevents an empty backup batch from becoming a readiness
// assertion for an unrelated or not-yet-persisted outbound encryption session.
func (c *Client) verifyPublicationSessionBackup(ctx context.Context, room string, session id.SessionID) error {
	if err := c.validateEncryptedPublication(ctx, room); err != nil {
		return err
	}
	// Sync invalidates the SDK's outbound session when membership changes. An
	// already encrypted payload must not use a key shared with a removed member.
	outbound, err := c.crypto.Machine().CryptoStore.GetOutboundGroupSession(ctx, id.RoomID(room))
	if err != nil {
		return err
	}
	if outbound == nil || outbound.ID() != session {
		return ErrIdentity
	}
	_, version, err := c.readBackup(ctx)
	if err != nil {
		return err
	}
	stored, err := c.crypto.Machine().CryptoStore.GetGroupSession(ctx, id.RoomID(room), session)
	if err != nil {
		return err
	}
	if stored == nil || stored.KeyBackupVersion != version {
		return ErrRecoveryRequired
	}
	return nil
}

// lockBackup preserves the existing mutex and lock ordering while allowing a
// storage cancellation to stop waiting behind another device backup operation.
func (c *Client) lockBackup(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.backupMu.TryLock() {
		if err := ctx.Err(); err != nil {
			c.backupMu.Unlock()
			return nil, err
		}
		return c.backupMu.Unlock, nil
	}
	timer := time.NewTimer(10 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
		if c.closed.Load() {
			return nil, ErrClosed
		}
		if c.backupMu.TryLock() {
			if err := ctx.Err(); err != nil {
				c.backupMu.Unlock()
				return nil, err
			}
			return c.backupMu.Unlock, nil
		}
		timer.Reset(10 * time.Millisecond)
	}
}

// bindStoragePublication pins the record's already captured owner before key IO.
// It never rebinds an outer publication context to a later account or target.
// Every storage record, including file pages and index commits, shares the
// backup-before-Send path with ordinary encrypted Rooms messages.
func (c *Client) bindStoragePublication(ctx context.Context, scope storage.Scope, target storage.PublicationTarget) (context.Context, error) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !scope.Valid() || scope != c.storageScope() || !c.cfg.Bootstrap.StorageIdentityVersion.Normalized().Valid() {
		return nil, ErrIdentity
	}
	if err := c.checkStoragePublicationTarget(target); err != nil {
		return nil, err
	}
	if proof, found := ctx.Value(storagePublicationContext{}).(storagePublication); found {
		if proof.target != target {
			return nil, ErrIdentity
		}
		if err := c.validateStoragePublication(ctx, target.RoomID); err != nil {
			return nil, err
		}
		return ctx, nil
	}
	return context.WithValue(ctx, storagePublicationContext{}, storagePublication{client: c, scope: scope, target: target, identity: c.cfg.Bootstrap.StorageIdentityVersion.Normalized()}), nil
}

// backupEncryptedPublication takes the established backupMu -> stateMu order.
// The prior encryption's client must still own this exact intent before any
// backup IO; a queued writer can progress without redirecting that IO.
func (c *Client) backupEncryptedPublication(ctx context.Context, room string, sameClient func() bool) error {
	release, err := c.lockBackup(ctx)
	if err != nil {
		return err
	}
	defer release()
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if !sameClient() {
		return ErrIdentity
	}
	if err := c.validateEncryptedPublication(ctx, room); err != nil {
		return err
	}
	return c.backupSessions(ctx)
}

// The caller holds stateMu. Domain policy remains bound to storage intents;
// ordinary Rooms publications revalidate the current encrypted room and members.
func (c *Client) validateEncryptedPublication(ctx context.Context, room string) error {
	if _, bound := ctx.Value(storagePublicationContext{}).(storagePublication); bound {
		return c.validateStoragePublication(ctx, room)
	}
	if err := c.requireEncrypted(ctx, room); err != nil {
		return err
	}
	return c.checkRoomIdentities(ctx, id.RoomID(room))
}
