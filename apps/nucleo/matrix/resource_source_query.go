// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"

	"maunium.net/go/mautrix/crypto/signatures"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
)

// signCurrentResourceMetadata is the one SDK signer for typed public resource
// intents. Provider/network checks stay outside the original operation lease
// and SDK state lock. No exported raw-signing API or storage identity fallback.
func (c *Client) signCurrentResourceMetadata(ctx context.Context, current resource.VerifiedSession, value any, lease offline.WorkLease) (signatures.Signatures, error) {
	if c == nil || lease == nil || c.cfg.ResourceProduct == "" || current.Scope() != c.resourceScope() || current.Binding().Namespace != "resources:"+c.cfg.ResourceProduct {
		return nil, ErrIdentity
	}
	if err := current.Revalidate(ctx); err != nil {
		return nil, err
	}
	release, err := resourceLease(ctx, lease)
	if err != nil {
		return nil, err
	}
	var signed signatures.Signatures
	err = func() error {
		defer release()
		c.stateMu.RLock()
		defer c.stateMu.RUnlock()
		actor, err := c.resourceSigningActor(ctx)
		if err != nil {
			return err
		}
		if actor != current.Actor() || current.Scope() != c.resourceScope() || current.Binding().DeviceID != c.session.DeviceID.String() {
			return ErrIdentity
		}
		signed, err = c.signResourceJSON(ctx, value)
		return err
	}()
	if err != nil {
		return nil, err
	}
	if err := current.Revalidate(ctx); err != nil {
		return nil, err
	}
	return signed, nil
}

// SignResourceBootstrapSourceQuery signs the current enrolled product and
// original resource/room. The shared core verifies the retained SDK transcript.
func (c *Client) SignResourceBootstrapSourceQuery(ctx context.Context, current resource.VerifiedSession, key resource.Resource, room string, lease offline.WorkLease) (resource.SignedBootstrapSourceQuery, error) {
	if c == nil || lease == nil || c.cfg.ResourceProduct == "" {
		return resource.SignedBootstrapSourceQuery{}, ErrIdentity
	}
	query, err := resource.PrepareBootstrapSourceQuery(ctx, current, key, room)
	if err != nil {
		return resource.SignedBootstrapSourceQuery{}, err
	}
	signed := resource.SignedBootstrapSourceQuery{BootstrapSourceQuery: query}
	signed.Signatures, err = c.signCurrentResourceMetadata(ctx, current, query, lease)
	if err != nil {
		return resource.SignedBootstrapSourceQuery{}, err
	}
	if _, err := resource.VerifyBootstrapSourceQuery(ctx, signed, current); err != nil {
		return resource.SignedBootstrapSourceQuery{}, err
	}
	return signed, nil
}
