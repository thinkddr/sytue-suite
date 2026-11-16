// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	resourcework "git.sytue.eu/sytue/one/apps/nucleo/resource/work"
)

var ErrResourcePublicationAcknowledged = errors.New("matrix: resource publication is already acknowledged")

// AcceptedReceipt reports retained completion without minting another admission
// or outbound SDK session. A caller must use the original committed reference.
func (p *PreparedResourcePublication) AcceptedReceipt(ctx context.Context) (resource.Receipt, bool, error) {
	if err := p.Revalidate(ctx); err != nil {
		return resource.Receipt{}, false, err
	}
	if p.receipt == nil {
		return resource.Receipt{}, false, nil
	}
	return *p.receipt, true, nil
}

// AcknowledgeCommitted uses the same Go durable completion as native and Wasm.
// Only this actual SDK reader can open the committed event; producer A stays
// historical and a different account never mounts its private journal.
func (r *ResourceRoom) AcknowledgeCommitted(ctx context.Context, proof resource.VerifiedCommittedRead) (bool, error) {
	if r == nil || r.owner == nil || r.session == nil {
		return false, ErrIdentity
	}
	cfg := resourcework.Acknowledger{Journal: r.journal, Current: r.current, Access: r.access, Fence: r.fence, Lease: r.session.lease, Reader: r, ValidateLocal: func(ctx context.Context) error {
		c := r.owner
		c.stateMu.RLock()
		same := !c.closed.Load() && !r.retired.Load() && c.resourceScope() == r.ActorScope() && c.session.UserID == r.session.user && c.session.DeviceID == r.session.device
		c.stateMu.RUnlock()
		if !same {
			return ErrIdentity
		}
		return ctx.Err()
	}}
	return cfg.Acknowledge(ctx, proof)
}
