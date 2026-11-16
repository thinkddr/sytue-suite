// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

// ReadBootstrapAccepted keeps B as the actual maintained SDK reader throughout.
// It fetches/decrypts only the original accepted reference/session/ciphertext and
// opens its hybrid descriptor through the same current Source fence. It never
// restores A, publishes, or acknowledges/deletes any pending bootstrap intent.
func (r *ResourceRoom) ReadBootstrapAccepted(ctx context.Context, proof resource.VerifiedBootstrapRead) ([]byte, error) {
	if r == nil {
		return nil, ErrIdentity
	}
	if err := proof.RevalidateReader(ctx, r.access, r.fence); err != nil {
		return nil, err
	}
	p := proof.Publication()
	return r.readVerifiedResourceFrame(ctx, proof.Reference(), p.Actor, p.OuterCipherSHA256, proof.Session(), acceptedBootstrapFrame{proof: proof})
}

type acceptedBootstrapFrame struct {
	proof resource.VerifiedBootstrapRead
}

func (p acceptedBootstrapFrame) open(ctx context.Context, h *resource.EpochHandle, envelope pqc.Envelope, reader resource.VerifiedAccess, fence resource.Fence) ([]byte, error) {
	return h.OpenBootstrapFrame(ctx, p.proof, envelope, reader, fence)
}
