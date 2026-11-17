// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type historicalStoragePublicationContext struct{ context.Context }

func (c historicalStoragePublicationContext) Value(key any) any {
	switch key.(type) {
	case storageAdmissionKey, storageRecordAdmissionKey:
		return nil
	}
	return c.Context.Value(key)
}

// HistoricalStoragePublicationContext projects ONE already authorized SDK1
// operation out of an enclosing SDK2 cutover. It removes only SDK2's private
// admission/record intent. Caller values, cancellation, work-lease witness and
// the exact SDK1 publication policy/target remain on the ORIGINAL context.
// It cannot authorize historical writes or rebind any SDK2 publication proof.
func (c *Client) HistoricalStoragePublicationContext(ctx context.Context) (context.Context, error) {
	if c == nil || ctx == nil {
		return nil, ErrIdentity
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	proof, ok := ctx.Value(storagePublicationContext{}).(storagePublication)
	if !ok || proof.client != c || proof.identity != storage.LegacyIdentity || proof.policy == nil {
		return nil, ErrIdentity
	}
	// The domain's real historical authorization may use SDK/HTTP/work leases.
	// Never invoke it while holding an SDK lock.
	if err := storagePublicationPolicy(ctx); err != nil {
		return nil, err
	}
	if err := c.validateHistoricalSourceState(ctx, proof); err != nil {
		return nil, err
	}
	inherited := ctx.Value(storageAdmissionKey{})
	if inherited == nil {
		return ctx, nil
	}
	if err := c.validateInheritedAdmission(ctx, inherited, proof.scope); err != nil {
		return nil, err
	}
	projected := historicalStoragePublicationContext{Context: ctx}
	if err := storagePublicationPolicy(projected); err != nil {
		return nil, err
	}
	return projected, nil
}

func (c *Client) validateHistoricalSourceState(ctx context.Context, proof storagePublication) error {
	c.stateMu.RLock()
	valid := c.cfg.ResourceProduct == "" && c.cfg.Bootstrap.StorageIdentityVersion.Normalized() == storage.LegacyIdentity && c.storageScope().Valid() && proof.scope == c.storageScope()
	var targetErr error
	if valid {
		targetErr = c.validateStoragePublication(ctx, proof.target.RoomID)
	}
	c.stateMu.RUnlock()
	if !valid {
		return ErrIdentity
	}
	return targetErr
}

func (c *Client) validateInheritedAdmission(ctx context.Context, inherited any, scope storage.Scope) error {
	fixed, ok := inherited.(*storageAdmission)
	if !ok || fixed == nil || fixed.client == nil || fixed.client == c || fixed.scope != scope {
		return ErrIdentity
	}
	fixed.client.stateMu.RLock()
	targetValid := fixed.client.cfg.ResourceProduct == "" && fixed.client.cfg.Bootstrap.StorageIdentityVersion.Normalized() == storage.IsolatedIdentity && fixed.client.storageScope() == scope
	fixed.client.stateMu.RUnlock()
	if !targetValid {
		return ErrIdentity
	}
	if intent := ctx.Value(storageRecordAdmissionKey{}); intent != nil {
		record, ok := intent.(*storageRecordAdmission)
		if !ok || record == nil || record.fixed != fixed {
			return ErrIdentity
		}
	}
	return nil
}
