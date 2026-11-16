// SPDX-License-Identifier: Apache-2.0
package resourcepg

import (
	"context"
	"errors"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

// AdmitPublication is fixed by the composition root and must verify/persist the
// client's exact SDK-signed origin request. Resource metadata is never mapped
// to a personal storage namespace, and the server never signs for the producer.
// No resource dispatcher can run until the real product SessionProvider and
// resources origin registry supply this callback.
type AdmitPublication func(context.Context, resource.JournalEntry, resource.VerifiedPublication) (publication.Marker, error)

type CipherTransport interface {
	Send(context.Context, resource.Candidate, resource.VerifiedPublication, publication.Marker) (storage.Reference, error)
}

// DispatchError reports a retained ambiguous operation, never a logical commit.
// A known reference is useful for ACK-only recovery after a failed SQL commit;
// the durable origin record is still authoritative if this response is lost.
type DispatchError struct {
	reference storage.Reference
	cause     error
}

func (e *DispatchError) Error() string                        { return "resource: publication remains pending reconciliation" }
func (e *DispatchError) Unwrap() error                        { return e.cause }
func (e *DispatchError) AcceptedReference() storage.Reference { return e.reference }

// Dispatch permits HTTP only for the prepared->sending transition won inside
// the resource row transaction. A Sending/Ambiguous row encountered after crash
// or by another dispatcher is NEVER blindly resent. It remains fenced until an
// exact accepted receipt or independently certified not-sent result resolves it.
func (r *Relay) Dispatch(ctx context.Context, candidate resource.Candidate, admit AdmitPublication, transport CipherTransport) (resource.Receipt, error) {
	if r == nil || admit == nil || transport == nil {
		return resource.Receipt{}, resource.ErrInvalid
	}
	staged, err := r.Stage(ctx, candidate)
	if err != nil {
		return resource.Receipt{}, err
	}
	if staged.State == resource.Committed && staged.Receipt != nil {
		return *staged.Receipt, nil
	}
	if staged.State == resource.WaitingValidation {
		return resource.Receipt{}, resource.ErrValidatorUnavailable
	}
	entry, verified, fresh, err := r.acquireSending(ctx, candidate.Proof.Resource, candidate.Proof.OpID)
	if err != nil {
		return resource.Receipt{}, err
	}
	if entry.State == resource.Committed && entry.Receipt != nil {
		return *entry.Receipt, nil
	}
	if !fresh {
		return resource.Receipt{}, resource.ErrPending
	}
	fail := func(cause error, ref storage.Reference) (resource.Receipt, error) {
		// A caller cancellation must not erase the ambiguous state transition. The
		// tenant/subject context values are retained; no privilege is manufactured.
		reconcile, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		markerErr := r.MarkAmbiguous(reconcile, entry)
		return resource.Receipt{}, &DispatchError{reference: ref, cause: errors.Join(cause, markerErr)}
	}
	marker, err := admit(ctx, entry, verified)
	if err != nil {
		return fail(err, storage.Reference{})
	}
	if !marker.Valid() {
		return fail(resource.ErrInvalid, storage.Reference{})
	}
	if err := ctx.Err(); err != nil {
		return fail(err, storage.Reference{})
	}
	reference, err := transport.Send(ctx, entry.Candidate, verified, marker)
	if err != nil {
		return fail(err, storage.Reference{})
	}
	if !reference.Valid() || reference.RoomID != entry.Candidate.Proof.RoomID {
		return fail(resource.ErrInvalid, storage.Reference{})
	}
	receipt, err := r.Commit(ctx, entry, reference)
	if err != nil {
		return fail(err, reference)
	}
	return receipt, nil
}
