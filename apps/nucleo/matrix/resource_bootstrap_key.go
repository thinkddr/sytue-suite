// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

type bootstrapKeyWitness struct {
	Version           int
	Binding           resource.SessionBinding
	Resource          resource.Resource
	PlanID, Recipient string
}

func (c *Client) bootstrapKeyName(device, plan string) string {
	return "matrix-resource-bootstrap-epoch-" + matrixAccountName(c.account+"\x00"+device+"\x00"+plan)
}

func (c *Client) bootstrapKey(ctx context.Context, current resource.VerifiedSession, key resource.Resource, planID string, lease offline.WorkLease, journal *offline.Store, create bool) (*pqc.Identity, error) {
	parsed, err := uuid.Parse(planID)
	if c == nil || err != nil || parsed == uuid.Nil || parsed.String() != planID || !key.Valid() || key.Product != c.cfg.ResourceProduct || !journal.Encrypted() {
		return nil, ErrIdentity
	}
	// Provider/network work precedes the local lease. A queued account writer must
	// never make this callback recursively acquire the same read lock.
	if _, err := c.ResourcePublicationSession(ctx, current, lease); err != nil {
		return nil, err
	}
	release, err := resourceLease(ctx, lease)
	if err != nil {
		return nil, err
	}
	var identity *pqc.Identity
	err = func() error {
		defer release()
		var err error
		identity, err = c.bootstrapKeyLocked(ctx, current, key, planID, journal, create)
		return err
	}()
	if err != nil {
		return nil, err
	}
	if err := current.Revalidate(ctx); err != nil {
		return nil, err
	}
	return identity, nil
}

func (c *Client) bootstrapKeyLocked(ctx context.Context, current resource.VerifiedSession, key resource.Resource, planID string, journal *offline.Store, create bool) (*pqc.Identity, error) {
	c.resourceLeafMu.Lock()
	defer c.resourceLeafMu.Unlock()
	c.stateMu.RLock()
	valid := !c.closed.Load() && current.Scope() == c.resourceScope() && current.Binding().UserID == c.session.UserID.String() && current.Binding().DeviceID == c.session.DeviceID.String()
	c.stateMu.RUnlock()
	if !valid {
		return nil, ErrIdentity
	}
	owner := current.Scope().Tenant + "\x00" + current.Scope().Subject + "\x00" + current.Scope().Product
	namespace := "resource-bootstrap-key-witness:" + c.account
	id := current.Actor().DeviceID + "/" + planID
	var witness bootstrapKeyWitness
	found, err := journal.Read(owner, namespace, id, &witness)
	if err != nil {
		return nil, err
	}
	if found && (witness.Version != 1 || witness.Binding != current.Binding() || witness.Resource != key || witness.PlanID != planID || witness.Recipient == "") {
		return nil, ErrIdentity
	}
	identity, err := c.loadOrCreateBootstrapIdentity(current.Actor().DeviceID, planID, create, found)
	if err != nil {
		return nil, err
	}
	recipient := identity.Recipient().String()
	if found && witness.Recipient != recipient {
		return nil, ErrRecoveryRequired
	}
	if !found {
		if !create {
			return nil, ErrRecoveryRequired
		}
		if err := journal.Write(owner, namespace, id, bootstrapKeyWitness{Version: 1, Binding: current.Binding(), Resource: key, PlanID: planID, Recipient: recipient}); err != nil {
			return nil, err
		}
	}
	return identity, ctx.Err()
}

func (c *Client) loadOrCreateBootstrapIdentity(device, planID string, create, found bool) (*pqc.Identity, error) {
	raw, err := c.cfg.Secrets.Read(c.bootstrapKeyName(device, planID))
	if err != nil {
		return nil, ErrSecureStore
	}
	defer clear(raw)
	if len(raw) == 0 {
		if !create || found {
			return nil, ErrRecoveryRequired
		}
		identity, err := pqc.GenerateIdentity()
		if err != nil {
			return nil, err
		}
		encoded := []byte(identity.String())
		defer clear(encoded)
		if err := c.cfg.Secrets.Write(c.bootstrapKeyName(device, planID), encoded); err != nil {
			return nil, ErrSecureStore
		}
		return identity, nil
	}
	identity, err := pqc.ParseIdentity(string(raw))
	if err != nil || identity.Profile() != pqc.HybridProfile {
		return nil, ErrSecureStore
	}
	return identity, nil
}

// ResourceBootstrapRecipient returns only the public key for a new local plan.
// The candidate stays in this enrolled SDK device's OS keychain. Binding a room
// additionally requires exact readback in the same product's isolated SDK2 SSSS;
// canonical global SSSS and widget/shared summary containers are never used.
// An encrypted witness prevents replacing a previously signed plan's lost key.
func (c *Client) ResourceBootstrapRecipient(ctx context.Context, current resource.VerifiedSession, key resource.Resource, planID string, lease offline.WorkLease, journal *offline.Store) (string, error) {
	identity, err := c.bootstrapKey(ctx, current, key, planID, lease, journal, true)
	if err != nil {
		return "", err
	}
	return identity.Recipient().String(), nil
}
