// SPDX-License-Identifier: Apache-2.0
package records

import (
	"context"
	"encoding/json"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// MutationContent captures metadata without effects before the first WAL write.
// Prepare can then stage encrypted content using only that durable input and
// target. Validate runs before Prepared is stored and on every prepared retry.
// Acknowledge runs only after the exact selected index has been read back.
// Callbacks receive owned JSON buffers; Prepare returns bytes owned by this call.
type MutationContent struct {
	Capture     func(context.Context, []byte, storage.PublicationTarget) ([]byte, error)
	Prepare     func(context.Context, []byte, []byte, storage.PublicationTarget) ([]byte, error)
	Validate    func(context.Context, []byte, []byte, storage.PublicationTarget) error
	Acknowledge func(context.Context, []byte, []byte, storage.PublicationTarget, storage.ObjectVersion) error
}

type mutationCall struct {
	operation, input, key string
	prepare               func(context.Context) (string, string, string, []byte, error)
	content               MutationContent
	acknowledge           func(context.Context, []byte) error
}

func (r *Repository) MutateWithContent(parent context.Context, operation string, input any, prepare func(context.Context) (string, string, string, []byte, error), content MutationContent) ([]byte, error) {
	if content.Capture == nil || content.Prepare == nil || content.Validate == nil || content.Acknowledge == nil {
		return nil, ErrSource
	}
	return r.mutate(parent, operation, input, prepare, content, nil)
}

// MutateAcknowledged keeps the exact mutation and publication in the encrypted
// journal until the caller has durably acknowledged its local source. The
// callback receives an owned copy of the accepted JSON and must be idempotent;
// it must not call back into this repository. A callback failure retains both
// journals. Retrying the same input verifies the accepted revision before any
// preparation or publication, including after recovery on another device.
func (r *Repository) MutateAcknowledged(parent context.Context, operation string, input any, prepare func(context.Context) (string, string, string, []byte, error), acknowledge func(context.Context, []byte) error) ([]byte, error) {
	if acknowledge == nil {
		return nil, ErrSource
	}
	return r.mutate(parent, operation, input, prepare, MutationContent{}, acknowledge)
}

// ResumeAcknowledged resumes only an existing acknowledged mutation. A missing
// WAL returns found=false without preparing or publishing anything; that alone
// does not prove that a request succeeded. A caller with its own durable local
// acknowledgment can use this to close cleanup after a crash, without applying
// an already accepted write again. The callback follows MutateAcknowledged's
// ownership and idempotency rules.
func (r *Repository) ResumeAcknowledged(parent context.Context, operation string, input any, acknowledge func(context.Context, []byte) error) ([]byte, bool, error) {
	if acknowledge == nil {
		return nil, false, ErrSource
	}
	return r.runMutation(parent, operation, input, nil, MutationContent{}, acknowledge, true)
}

func (r *Repository) mutate(parent context.Context, operation string, input any, prepare func(context.Context) (string, string, string, []byte, error), content MutationContent, acknowledge func(context.Context, []byte) error) ([]byte, error) {
	result, _, err := r.runMutation(parent, operation, input, prepare, content, acknowledge, false)
	return result, err
}

func (r *Repository) runMutation(parent context.Context, operation string, input any, prepare func(context.Context) (string, string, string, []byte, error), content MutationContent, acknowledge func(context.Context, []byte) error, resumeOnly bool) ([]byte, bool, error) {
	ctx, done, err := r.Start(parent)
	if err != nil {
		return nil, false, err
	}
	defer done()
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil, false, err
	}
	defer clear(encoded)
	fingerprint := hashBytes(encoded)
	call := mutationCall{operation: operation, input: fingerprint, key: "mutation_" + hashBytes([]byte(operation+"\x00"+fingerprint)), prepare: prepare, content: content, acknowledge: acknowledge}
	var result []byte
	var found bool
	err = r.cfg.Journal.Exclusive(ctx, r.owner(), r.namespace(), "catalog", func() error {
		if resumeOnly {
			if err := r.requireReady(); err != nil {
				return err
			}
			var work mutation
			var err error
			found, err = r.cfg.Journal.Read(r.owner(), r.namespace(), call.key, &work)
			clear(work.JSON)
			clear(work.ContentInput)
			if err != nil || !found {
				return err
			}
		}
		var err error
		result, err = r.resumeMutation(ctx, call)
		return err
	})
	return result, found, err
}

func (r *Repository) resumeMutation(ctx context.Context, call mutationCall) ([]byte, error) {
	if err := r.requireReady(); err != nil {
		return nil, err
	}
	if err := r.refresh(ctx); err != nil {
		return nil, err
	}
	work, err := r.loadMutation(ctx, call)
	if err != nil {
		return nil, err
	}
	defer func() { clear(work.JSON); clear(work.ContentInput) }()
	if err := r.validateMutation(ctx, work, call.operation, call.input); err != nil {
		return nil, err
	}
	if work.Content != (call.content.Prepare != nil) || work.Acknowledge != (call.acknowledge != nil) {
		return nil, ErrSource
	}
	if err := r.prepareMutationContent(ctx, call, &work); err != nil {
		return nil, err
	}
	if err := r.publishMutation(ctx, call, work); err != nil {
		return nil, err
	}
	result := slices.Clone(work.JSON)
	for _, key := range []string{call.key + "_publication", call.key} {
		if err := r.cfg.Journal.Delete(r.owner(), r.namespace(), key); err != nil {
			clear(result)
			return nil, err
		}
	}
	return result, nil
}

func (r *Repository) loadMutation(ctx context.Context, call mutationCall) (mutation, error) {
	var work mutation
	found, err := r.cfg.Journal.Read(r.owner(), r.namespace(), call.key, &work)
	if err != nil || found {
		return work, err
	}
	if call.prepare == nil {
		return mutation{}, ErrSource
	}
	// Heads are replaced wholesale on refresh, so this map is the exact
	// catalog prepare reads from even if another refresh swaps it meanwhile.
	r.mu.Lock()
	observedHeads, observedTarget := r.heads, r.binding.Target
	r.mu.Unlock()
	kind, id, state, raw, err := call.prepare(ctx)
	if err != nil {
		return work, err
	}
	work = mutation{Version: 1, Scope: r.cfg.Scope, Operation: call.operation, Input: call.input, Entity: kind, ID: id, State: state, JSON: raw, Content: call.content.Prepare != nil}
	heads := observedHeads[r.object(kind, id)]
	parents := make([]storage.Reference, len(heads))
	for i, head := range heads {
		parents[i] = head.Reference
	}
	work.Observed = &mutationObservation{Target: observedTarget, Parents: parents}
	if call.acknowledge != nil {
		work.Acknowledge = true
		target := observedTarget
		work.Target = &target
		if err := r.contentTarget(work); err != nil {
			clear(raw)
			return mutation{}, err
		}
	}
	if work.Content {
		if err := r.captureMutationContent(ctx, call.content, &work); err != nil {
			clear(raw)
			clear(work.ContentInput)
			return mutation{}, err
		}
		// Capture binds content to the target it checked; the observation is
		// the same intent and must not name a different device.
		work.Observed.Target = *work.Target
		if err := r.validateMutation(ctx, work, call.operation, call.input); err != nil {
			clear(raw)
			clear(work.ContentInput)
			return mutation{}, err
		}
	}
	if err := r.cfg.Journal.Write(r.owner(), r.namespace(), call.key, work); err != nil {
		clear(raw)
		clear(work.ContentInput)
		return mutation{}, err
	}
	return work, nil
}

func (r *Repository) captureMutationContent(ctx context.Context, content MutationContent, work *mutation) error {
	r.mu.Lock()
	target := r.binding.Target
	r.mu.Unlock()
	if !target.Valid() {
		return ErrSource
	}
	if err := r.cfg.Crypto.CheckStoragePublicationTarget(target); err != nil {
		return err
	}
	input := slices.Clone(work.JSON)
	captured, err := content.Capture(ctx, input, target)
	work.ContentInput = slices.Clone(captured)
	clear(input)
	clear(captured)
	if err != nil {
		return err
	}
	work.Target = &target
	return r.contentTarget(*work)
}

func (r *Repository) prepareMutationContent(ctx context.Context, call mutationCall, work *mutation) error {
	if !work.Content {
		return nil
	}
	if work.Prepared {
		return r.validateMutationContent(ctx, *work, call.content)
	}
	if err := r.contentTarget(*work); err != nil {
		return err
	}
	raw, captured := slices.Clone(work.JSON), slices.Clone(work.ContentInput)
	prepared, err := call.content.Prepare(ctx, raw, captured, *work.Target)
	next := slices.Clone(prepared)
	clear(raw)
	clear(captured)
	clear(prepared)
	if err != nil {
		clear(next)
		return err
	}
	candidate := *work
	candidate.JSON = next
	candidate.Prepared = true
	if err := r.validateMutation(ctx, candidate, call.operation, call.input); err != nil {
		clear(next)
		return err
	}
	if err := r.validateMutationContent(ctx, candidate, call.content); err != nil {
		clear(next)
		return err
	}
	if err := r.cfg.Journal.Write(r.owner(), r.namespace(), call.key, candidate); err != nil {
		clear(next)
		return err
	}
	clear(work.JSON)
	*work = candidate
	return nil
}

func (r *Repository) publishMutation(ctx context.Context, call mutationCall, work mutation) error {
	reused, err := r.reuseAcceptedMutation(ctx, call, work)
	if err != nil {
		return err
	}
	if !reused {
		if work.Acknowledge {
			if err := r.contentTarget(work); err != nil {
				return err
			}
		}
		if err := r.retainedPlanObservable(call, work); err != nil {
			return err
		}
		if err := r.publishJSON(ctx, call.key+"_publication", work.Entity, work.ID, work.State, work.JSON, nil, work.Observed); err != nil {
			return err
		}
	}
	if err := r.refresh(ctx); err != nil {
		return err
	}
	return r.acknowledgeMutation(ctx, call, work)
}

// retainedPlanObservable refuses a plain WAL written before observations were
// pinned when no publication plan exists yet: nothing records which head its
// prepare read, so planning now could only adopt the current head and attach
// stale intent to a later writer. The WAL is retained for resolution.
func (r *Repository) retainedPlanObservable(call mutationCall, work mutation) error {
	if work.Observed != nil || work.Content || work.Acknowledge {
		return nil
	}
	var plan publication
	found, err := r.cfg.Journal.Read(r.owner(), r.namespace(), call.key+"_publication", &plan)
	if err != nil {
		return err
	}
	if !found {
		return ErrConflict
	}
	return nil
}

func (r *Repository) acknowledgeMutation(ctx context.Context, call mutationCall, work mutation) error {
	if call.content.Acknowledge == nil && call.acknowledge == nil {
		return nil
	}
	accepted, err := r.acceptedMutation(ctx, call.key+"_publication", work)
	if err != nil {
		return err
	}
	if work.Accepted == nil {
		work.Accepted = &accepted.Reference
		if err := r.cfg.Journal.Write(r.owner(), r.namespace(), call.key, work); err != nil {
			return err
		}
	}
	raw, captured := slices.Clone(work.JSON), slices.Clone(work.ContentInput)
	defer clear(raw)
	defer clear(captured)
	if err := ctx.Err(); err != nil {
		return err
	}
	if call.acknowledge != nil {
		return call.acknowledge(ctx, raw)
	}
	return call.content.Acknowledge(ctx, raw, captured, *work.Target, accepted)
}

func (r *Repository) reuseAcceptedMutation(ctx context.Context, call mutationCall, work mutation) (bool, error) {
	if !work.Content && !work.Acknowledge {
		return false, nil
	}
	if work.Accepted != nil {
		_, err := r.acceptedMutation(ctx, call.key+"_publication", work)
		return err == nil, err
	}
	var previous publication
	found, err := r.cfg.Journal.Read(r.owner(), r.namespace(), call.key+"_publication", &previous)
	if err != nil {
		return false, err
	}
	if !found || previous.Index == nil {
		return false, nil
	}
	_, err = r.acceptedMutation(ctx, call.key+"_publication", work)
	return err == nil, err
}

func (r *Repository) acceptedMutation(ctx context.Context, key string, work mutation) (storage.ObjectVersion, error) {
	if work.Accepted != nil {
		return r.acceptedMutationReference(ctx, work, *work.Accepted)
	}
	var publication publication
	found, err := r.cfg.Journal.Read(r.owner(), r.namespace(), key, &publication)
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	if !found || publication.Index == nil {
		return storage.ObjectVersion{}, ErrSource
	}
	if (work.Content || work.Acknowledge) && (work.Target == nil || publication.Target != *work.Target) {
		return storage.ObjectVersion{}, ErrSource
	}
	if err := publication.validate(r.cfg.Scope, work.Entity, work.ID, work.State, work.JSON, nil); err != nil {
		return storage.ObjectVersion{}, err
	}
	version, err := r.acceptedMutationVersion(work, *publication.Index)
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	if version.Reference != *publication.Index || version.RevisionID != publication.Revision+"_index" {
		return storage.ObjectVersion{}, ErrConflict
	}
	if err := r.checkPublishedContent(ctx, version, work.JSON); err != nil {
		return storage.ObjectVersion{}, err
	}
	return version, nil
}

func (r *Repository) contentTarget(work mutation) error {
	if work.Target == nil {
		return ErrSource
	}
	r.mu.Lock()
	target := r.binding.Target
	r.mu.Unlock()
	if target != *work.Target {
		return ErrSource
	}
	return r.cfg.Crypto.CheckStoragePublicationTarget(*work.Target)
}

func (r *Repository) validateMutationContent(ctx context.Context, work mutation, content MutationContent) error {
	if err := r.contentTarget(work); err != nil {
		return err
	}
	raw, captured := slices.Clone(work.JSON), slices.Clone(work.ContentInput)
	err := content.Validate(ctx, raw, captured, *work.Target)
	clear(raw)
	clear(captured)
	return err
}

func (r *Repository) acceptedMutationReference(ctx context.Context, work mutation, reference storage.Reference) (storage.ObjectVersion, error) {
	version, err := r.acceptedMutationVersion(work, reference)
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	if work.Target == nil || reference.RoomID != work.Target.RoomID || version.Reference != reference {
		return storage.ObjectVersion{}, ErrConflict
	}
	if err := r.checkPublishedContent(ctx, version, work.JSON); err != nil {
		return storage.ObjectVersion{}, err
	}
	return version, nil
}

// Local source cleanup acknowledges a particular accepted write, not the
// current head. A subsequent remote edit must not strand that local source.
// The exact reference must belong to the fully authenticated retained catalog;
// this does not select a branch or authorize another publication.
func (r *Repository) acceptedMutationVersion(work mutation, reference storage.Reference) (storage.ObjectVersion, error) {
	if !work.Acknowledge {
		return r.ReadyVersion(work.Entity, work.ID)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return storage.ObjectVersion{}, ErrClosed
	}
	if !r.ready {
		return storage.ObjectVersion{}, ErrNotReady
	}
	if !r.binding.Same(r.catalog.Binding) {
		return storage.ObjectVersion{}, ErrSource
	}
	version, found, err := storage.AcceptedAtomicObject(r.catalog, r.cfg.Scope, r.object(work.Entity, work.ID), "", &reference)
	if err != nil || !found {
		return storage.ObjectVersion{}, ErrSource
	}
	return version, nil
}
