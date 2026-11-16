// SPDX-License-Identifier: Apache-2.0
// Package work retains resource publication intents using the existing sealed
// offline store. It grants no send, signing or source authority.
package work

import (
	"bytes"
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type CommittedReader = resource.CommittedReader

// Acknowledger is built by the original enrolled device broker. ValidateLocal
// checks the original SDK/intent while its lease is held and must perform no
// Provider/network work or acquire that same lease recursively.
type Acknowledger struct {
	Journal       *offline.Store
	Current       resource.VerifiedSession
	Access        resource.VerifiedAccess
	Fence         resource.Fence
	Lease         offline.WorkLease
	Reader        CommittedReader
	ValidateLocal func(context.Context) error
}

func (a Acknowledger) revalidate(ctx context.Context) error {
	binding, ok := a.Access.CurrentSessionBinding()
	if !ok || binding != a.Current.Binding() || a.Access.Action() != resource.Read || a.Access.Scope() != a.Current.Scope() {
		return resource.ErrRevoked
	}
	return a.Current.Revalidate(ctx)
}

func reconcileCompletedWork(work resource.PublicationWork, pending bool, history resource.AcknowledgedPublicationWork, proof resource.VerifiedCommittedRead) (resource.PublicationWork, error) {
	if !history.Valid() || history.Receipt != proof.Receipt() {
		return resource.PublicationWork{}, resource.ErrConflict
	}
	if !pending {
		return history.Work, nil
	}
	if !history.Work.Matches(work.Binding, work.Publication, work.Payload) || work.Candidate == nil {
		return resource.PublicationWork{}, resource.ErrConflict
	}
	oldHash, e1 := resource.CandidateDigest(*history.Work.Candidate)
	newHash, e2 := resource.CandidateDigest(*work.Candidate)
	if e1 != nil || e2 != nil || oldHash != newHash {
		return resource.PublicationWork{}, resource.ErrConflict
	}
	return work, nil
}

func (a Acknowledger) verifyCommittedPayload(ctx context.Context, work resource.PublicationWork, proof resource.VerifiedCommittedRead) error {
	if err := resource.CheckCommittedPublicationWork(ctx, work, proof, a.Access, a.Fence); err != nil {
		return err
	}
	actual, err := a.Reader.ReadCommitted(ctx, proof)
	if err != nil {
		return err
	}
	exact := bytes.Equal(actual, work.Payload)
	clear(actual)
	if !exact {
		return resource.ErrInvalid
	}
	return resource.CheckCommittedPublicationWork(ctx, work, proof, a.Access, a.Fence)
}

func (a Acknowledger) persistAcknowledgedWork(ctx context.Context, owner, id string, work resource.PublicationWork, pending bool, proof resource.VerifiedCommittedRead) error {
	// Provider/fence IO is complete. Only local persistence runs under the ORIGINAL
	// retained work lease, so an account writer cannot race journal retirement.
	release, err := a.Lease(ctx)
	if err == nil && release == nil {
		err = offline.ErrWorkLease
	}
	if err != nil {
		return err
	}
	defer release()
	if err := a.ValidateLocal(ctx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	receipt := resource.AcknowledgedPublicationWork{Version: 1, Work: work, Receipt: proof.Receipt()}
	if !receipt.Valid() {
		return resource.ErrInvalid
	}
	if err := a.Journal.Write(owner, storage.ResourcePublicationHistoryNamespace, id, receipt); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if pending {
		return a.Journal.Delete(owner, storage.ResourcePublicationNamespace, id)
	}
	return nil
}

func (a Acknowledger) acknowledgeLocked(ctx context.Context, owner, id string, proof resource.VerifiedCommittedRead) (bool, error) {
	var work resource.PublicationWork
	pending, err := a.Journal.Read(owner, storage.ResourcePublicationNamespace, id, &work)
	if err != nil {
		return false, err
	}
	defer clear(work.Payload)
	var history resource.AcknowledgedPublicationWork
	completed, err := a.Journal.Read(owner, storage.ResourcePublicationHistoryNamespace, id, &history)
	if err != nil {
		return false, err
	}
	defer clear(history.Work.Payload)
	if !pending && !completed {
		return false, nil
	}
	if completed {
		work, err = reconcileCompletedWork(work, pending, history, proof)
		if err != nil {
			return false, err
		}
	}
	if err := a.verifyCommittedPayload(ctx, work, proof); err != nil {
		return false, err
	}
	if err := a.persistAcknowledgedWork(ctx, owner, id, work, pending, proof); err != nil {
		return false, err
	}
	return true, nil
}

// Acknowledge verifies the original intent through the CURRENT enrolled reader,
// persists exact encrypted history, and only then retires pending work. No old
// SDK is created. A 404, timeout or unauthenticated receipt never permits cleanup.
func (a Acknowledger) Acknowledge(ctx context.Context, proof resource.VerifiedCommittedRead) (bool, error) {
	if a.Journal == nil || !a.Journal.Encrypted() || a.Reader == nil || a.Lease == nil || a.ValidateLocal == nil {
		return false, resource.ErrInvalid
	}
	if err := proof.RevalidateReader(ctx, a.Access, a.Fence); err != nil {
		return false, err
	}
	if err := a.revalidate(ctx); err != nil {
		return false, err
	}
	owner, err := a.Current.Scope().JournalOwner()
	if err != nil {
		return false, err
	}
	id, err := resource.PublicationWorkKey(proof.Value().Resource, proof.Value().OpID)
	if err != nil {
		return false, err
	}
	var acknowledged bool
	err = a.Journal.Exclusive(ctx, owner, storage.ResourcePublicationNamespace, id, func() error {
		var err error
		acknowledged, err = a.acknowledgeLocked(ctx, owner, id, proof)
		return err
	})
	if err != nil {
		return false, err
	}
	if err := proof.RevalidateReader(ctx, a.Access, a.Fence); err != nil {
		return false, err
	}
	return acknowledged, nil
}
