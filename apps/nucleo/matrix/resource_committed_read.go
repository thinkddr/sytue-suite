// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

// ReadCommitted uses a read-only historical proof for the exact committed ref.
// Current reader B remains the SDK owner throughout; producer A appears only in
// its captured signed certificate and ciphertext, never as a live SDK identity.
func (r *ResourceRoom) ReadCommitted(ctx context.Context, verified resource.VerifiedCommittedRead) ([]byte, error) {
	if r == nil {
		return nil, ErrIdentity
	}
	if err := verified.RevalidateReader(ctx, r.access, r.fence); err != nil {
		return nil, err
	}
	return r.readResourceFrame(ctx, verified.Receipt().Reference, verified.Value(), verified.Session(), committedResourceFrame{proof: verified})
}

type resourceFrameOpener interface {
	open(context.Context, *resource.EpochHandle, pqc.Envelope, resource.VerifiedAccess, resource.Fence) ([]byte, error)
}
type pendingResourceFrame struct {
	proof resource.VerifiedPendingPublication
}

func (p pendingResourceFrame) open(ctx context.Context, h *resource.EpochHandle, envelope pqc.Envelope, reader resource.VerifiedAccess, fence resource.Fence) ([]byte, error) {
	return h.OpenFrame(ctx, p.proof, envelope, reader, fence)
}

type committedResourceFrame struct {
	proof resource.VerifiedCommittedRead
}

func (p committedResourceFrame) open(ctx context.Context, h *resource.EpochHandle, envelope pqc.Envelope, reader resource.VerifiedAccess, fence resource.Fence) ([]byte, error) {
	return h.OpenCommittedFrame(ctx, p.proof, envelope, reader, fence)
}

// Both entry points retain the original exact SDK wire/provenance checks before
// age opens the inner envelope, and the original current-read fence afterward.
func (r *ResourceRoom) readResourceFrame(ctx context.Context, reference storage.Reference, publication resource.CipherPublication, session sessionproof.Session, opener resourceFrameOpener) ([]byte, error) {
	return r.readVerifiedResourceFrame(ctx, reference, publication.Actor, publication.OuterCipherSHA256, session, opener)
}

// Normal committed records and accepted genesis descriptors share this ONE SDK
// wire/session provenance reader; only the verified inner-envelope opener varies.
func (r *ResourceRoom) readVerifiedResourceFrame(ctx context.Context, reference storage.Reference, actor resource.Actor, outer string, session sessionproof.Session, opener resourceFrameOpener) ([]byte, error) {
	if err := r.Revalidate(ctx); err != nil {
		return nil, err
	}
	if r.access.Action() != resource.Read || reference.RoomID != r.RoomID() || !reference.Valid() {
		return nil, ErrIdentity
	}
	c := r.owner
	bound := context.WithValue(r.operationContext(ctx), resourceReadKey{}, resourceReadProof{room: r, reference: reference, expected: session, outerSHA256: outer})
	c.stateMu.RLock()
	var value DecryptedEvent
	err := func() error {
		if err := c.requireEncrypted(bound, reference.RoomID); err != nil {
			return err
		}
		var err error
		value, err = c.readEncryptedEvent(bound, reference)
		return err
	}()
	c.stateMu.RUnlock()
	if err != nil {
		return nil, err
	}
	defer clear(value.Content)
	if value.Type != resource.FrameEventType || value.Sender != actor.UserID || value.OuterCipherSHA256 != outer {
		return nil, ErrIdentity
	}
	envelope, err := resource.DecodeFrame(value.Content)
	if err != nil {
		return nil, err
	}
	defer clear(envelope.Ciphertext)
	payload, err := opener.open(ctx, r.handle, envelope, r.access, r.fence)
	if err != nil {
		return nil, err
	}
	if err := r.Revalidate(ctx); err != nil {
		clear(payload)
		return nil, err
	}
	return payload, nil
}
