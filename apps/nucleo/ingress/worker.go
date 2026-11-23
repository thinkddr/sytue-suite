// SPDX-License-Identifier: Apache-2.0
// Package ingress executes durable opaque inputs on an authorized device. The
// maintained SDK owns key custody and publication; offline owns the encrypted
// WAL, immutable staging, cross-process exclusion and session work lease.
package ingress

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	contract "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type Source interface {
	Authority(context.Context, storage.Scope) (matrixproof.Authority, error)
	Revalidate(context.Context, contract.Claim, matrixproof.Authority) error
	Claim(context.Context, storage.Scope, string, string, storage.PublicationTarget, string, matrixproof.Authority) (contract.Claim, error)
	Open(context.Context, contract.Claim, matrixproof.Authority) (storage.StreamSourceRow, io.ReadCloser, error)
	Acknowledge(context.Context, contract.Claim, contract.Receipt, matrixproof.Authority) (contract.Receipt, error)
}

type Device interface {
	PrepareStorage(context.Context, storage.Scope) (storage.Binding, error)
	CheckStoragePublicationTarget(storage.PublicationTarget) error
	RequireRecoverableIngress(context.Context, storage.Scope, storage.PublicationTarget) (context.Context, error)
	RequireAuthorizedStoragePublication(context.Context, storage.Scope, storage.PublicationTarget, storage.PublicationPolicy) (context.Context, error)
	OpenIngressBody(context.Context, contract.Task, io.ReaderAt) ([]byte, *io.SectionReader, error)
}

// Reconcile reads authenticated complete domain history. It returns a single
// matching task commit, no match, or a conflict; it never chooses a branch.
// Execute must retain its own publication WAL before any SDK Send and use this
// original claim's room/device. Verify is mandatory readback after execution.
type Executor interface {
	Reconcile(context.Context, contract.Claim, []byte, io.ReaderAt, int64) (*storage.Reference, error)
	Execute(context.Context, contract.Claim, []byte, io.ReaderAt, int64) (storage.Reference, error)
}

type Config struct {
	Context  context.Context
	Scope    storage.Scope
	Epoch    string
	Journal  *offline.Store
	Lease    offline.WorkLease
	Source   Source
	Device   Device
	Executor Executor
	// KeyID, si lo da el anfitrión (la bóveda, ADR 0087), es la clave de entrada
	// que este dispositivo tiene. Las entregas cifradas para otra (la de Matrix
	// tras un traspaso, la de antes de vaciar) no se reclaman: se quedan en la
	// cola, intactas, para quien pueda abrirlas, en vez de bloquearse aquí.
	KeyID string
}

type Worker struct {
	cfg        Config
	gate       *offline.WorkGate
	pollMu     sync.Mutex
	pollAfter  string
	retryAfter string
}

func New(cfg Config) (*Worker, error) {
	if cfg.Context == nil || !cfg.Scope.Valid() || !storage.MigrationEpoch(cfg.Epoch) || !cfg.Journal.Encrypted() || cfg.Lease == nil || cfg.Source == nil || cfg.Device == nil || cfg.Executor == nil {
		return nil, contract.ErrContract
	}
	return &Worker{cfg: cfg, gate: offline.NewWorkGateWithLease(cfg.Context, cfg.Lease)}, nil
}

func (w *Worker) owner() string {
	return w.cfg.Scope.Tenant + "\x00" + w.cfg.Scope.Subject + "\x00" + w.cfg.Scope.Product
}
func (w *Worker) PrivateStorageScope() storage.Scope { return w.cfg.Scope }
func (w *Worker) Drain(ctx context.Context) error    { return w.gate.Drain(ctx) }
func (w *Worker) Resume() error                      { return w.gate.Resume() }
func (w *Worker) Close()                             { w.gate.Close() }
func (w *Worker) Pending(ctx context.Context) (offline.PendingWork, error) {
	out := offline.PendingWork{Scope: w.cfg.Scope}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	ids, err := w.cfg.Journal.RecordIDs(w.owner(), storage.PrivateIngressNamespace)
	out.Count = len(ids)
	return out, err
}

type intent struct {
	Authority    matrixproof.Authority     `json:"authority"`
	Version      int                       `json:"v"`
	Scope        storage.Scope             `json:"scope"`
	Epoch        string                    `json:"epoch"`
	ID           string                    `json:"id"`
	Target       storage.PublicationTarget `json:"target"`
	Token        string                    `json:"token"`
	Claim        *contract.Claim           `json:"claim,omitempty"`
	Cipher       *offline.StagedFile       `json:"cipher,omitempty"`
	Commit       *storage.Reference        `json:"commit,omitempty"`
	Acknowledged bool                      `json:"acknowledged,omitempty"`
	Handoff      *handoffIntent            `json:"handoff,omitempty"`
}

func (i intent) validate(cfg Config, id string) error {
	if i.Authority.RequireActive(cfg.Scope, i.Authority.Generation) != nil || i.Version != 1 || i.Scope != cfg.Scope || i.Epoch != cfg.Epoch || i.ID != id || !storage.Identifier(i.ID) || !i.Target.Valid() || !storage.Identifier(i.Token) {
		return contract.ErrContract
	}
	return i.validateDelivery(cfg)
}

func (i intent) validateDelivery(cfg Config) error {
	if i.Handoff != nil {
		if err := i.Handoff.validate(cfg, i); err != nil {
			return err
		}
	}
	if i.Claim != nil && (i.Claim.Validate(cfg.Scope, cfg.Epoch) != nil || i.Claim.Task.ID != i.ID || i.Claim.Target != i.Target || i.Claim.Token != i.Token) {
		return contract.ErrClaim
	}
	if i.Cipher != nil && (i.Claim == nil || i.Cipher.SizeBytes != i.Claim.Task.Cipher.Size || i.Cipher.SHA256 != i.Claim.Task.Cipher.SHA256) {
		return contract.ErrContract
	}
	if i.Commit != nil && (i.Claim == nil || i.Cipher == nil || !i.Commit.Valid() || i.Commit.RoomID != i.Target.RoomID) || i.Acknowledged && i.Commit == nil {
		return contract.ErrReceipt
	}
	return nil
}

func (w *Worker) save(i *intent) error {
	return w.cfg.Journal.Write(w.owner(), storage.PrivateIngressNamespace, i.ID, i)
}

// Process is safe to schedule repeatedly after notifications or polling. Its
// initial token/target reach durable encrypted WAL before the first claim HTTP.
// Failure retains the WAL and ciphertext; there is no plaintext disk fallback.
func (w *Worker) Process(parent context.Context, id string) error {
	if !storage.Identifier(id) {
		return contract.ErrContract
	}
	ctx, done, err := w.gate.Start(parent)
	if err != nil {
		return err
	}
	defer done()
	return w.processAdmitted(ctx, id)
}

// processAdmitted retains the caller's real work permission for this complete
// job. Poll and Process share the same WAL/claim/SDK path without another lease.
func (w *Worker) processAdmitted(ctx context.Context, id string) error {
	if !storage.Identifier(id) {
		return contract.ErrContract
	}
	return w.cfg.Journal.Exclusive(ctx, w.owner(), storage.PrivateIngressNamespace, id, func() error {
		var work intent
		found, err := w.cfg.Journal.Read(w.owner(), storage.PrivateIngressNamespace, id, &work)
		if err != nil {
			return err
		}
		if !found {
			work, err = w.newIntent(ctx, id)
			if err != nil {
				return err
			}
			if err := w.save(&work); err != nil {
				return err
			}
		}
		if err := work.validate(w.cfg, id); err != nil {
			return err
		}
		if err := w.cfg.Device.CheckStoragePublicationTarget(work.Target); err != nil {
			return err
		}
		ctx, err = w.cfg.Device.RequireRecoverableIngress(ctx, w.cfg.Scope, work.Target)
		if err != nil {
			return err
		}
		if work.Handoff != nil {
			return w.resumeHandoff(ctx, &work)
		}
		return w.resume(ctx, &work)
	})
}

func (w *Worker) claim(ctx context.Context, work *intent) error {
	claim, err := w.cfg.Source.Claim(ctx, w.cfg.Scope, w.cfg.Epoch, work.ID, work.Target, work.Token, work.Authority)
	if err != nil {
		return err
	}
	if claim.Validate(w.cfg.Scope, w.cfg.Epoch) != nil || claim.Task.ID != work.ID || claim.Target != work.Target || claim.Token != work.Token || !claim.ExpiresAt.After(time.Now()) {
		return contract.ErrClaim
	}
	if work.Claim != nil && (claim.Task != work.Claim.Task || claim.Generation != work.Claim.Generation) {
		return contract.ErrClaim
	}
	work.Claim = &claim
	return w.save(work)
}

func (w *Worker) capture(ctx context.Context, work *intent) error {
	if work.Cipher != nil {
		return nil
	}
	descriptor, input, err := w.cfg.Source.Open(ctx, *work.Claim, work.Authority)
	if input != nil {
		defer func() { _ = input.Close() }()
	}
	if err != nil {
		return err
	}
	if input == nil || descriptor != work.Claim.Task.Cipher {
		return contract.ErrContract
	}
	stop := context.AfterFunc(ctx, func() { _ = input.Close() })
	defer stop()
	file, err := w.cfg.Journal.StageFile(ctx, w.owner(), storage.PrivateIngressStagingNamespace, input, descriptor.Size)
	if err != nil {
		return err
	}
	if file.SizeBytes != descriptor.Size || file.SHA256 != descriptor.SHA256 {
		_ = w.cfg.Journal.DeleteStagedFile(w.owner(), storage.PrivateIngressStagingNamespace, file)
		return contract.ErrContract
	}
	work.Cipher = &file
	if err := w.save(work); err != nil {
		_ = w.cfg.Journal.DeleteStagedFile(w.owner(), storage.PrivateIngressStagingNamespace, file)
		return err
	}
	return nil
}

func (w *Worker) resume(ctx context.Context, work *intent) error {
	if work.Acknowledged {
		return w.cleanup(work)
	}
	if work.Commit == nil {
		if err := w.claim(ctx, work); err != nil {
			return err
		}
	}
	if work.Claim == nil {
		return contract.ErrClaim
	}
	if work.Commit != nil && !work.Claim.ExpiresAt.After(time.Now()) {
		// An ACK may already have committed while its HTTP response was lost.
		// Such an immutable completed claim can be reconciled after expiry but
		// must never request a new publication permission. Only an explicitly
		// still-unacknowledged expired claim needs the original-endpoint renewal.
		err := w.cfg.Source.Revalidate(ctx, *work.Claim, work.Authority)
		if errors.Is(err, contract.ErrClaim) {
			err = w.claim(ctx, work)
		}
		if err != nil {
			return err
		}
	}
	originalClaim, originalAuthority := *work.Claim, work.Authority
	policy := storage.PublicationPolicy(func(checkCtx context.Context) error {
		return w.cfg.Source.Revalidate(checkCtx, originalClaim, originalAuthority)
	})
	ctx, err := w.cfg.Device.RequireAuthorizedStoragePublication(ctx, w.cfg.Scope, work.Target, policy)
	if err != nil {
		return err
	}
	if err := policy(ctx); err != nil {
		return err
	}
	if err := w.capture(ctx, work); err != nil {
		return err
	}
	cipher, err := w.cfg.Journal.OpenStagedFile(ctx, w.owner(), storage.PrivateIngressStagingNamespace, *work.Cipher)
	if err != nil {
		return err
	}
	defer func() { _ = cipher.Close() }()
	metadata, body, err := w.cfg.Device.OpenIngressBody(ctx, work.Claim.Task, cipher)
	if err != nil {
		return err
	}
	defer clear(metadata)
	if err := w.publish(ctx, work, metadata, body); err != nil {
		return err
	}
	return w.acknowledge(ctx, work)
}

func (w *Worker) publish(ctx context.Context, work *intent, metadata []byte, body *io.SectionReader) error {
	accepted, err := w.cfg.Executor.Reconcile(ctx, *work.Claim, metadata, body, body.Size())
	if err != nil {
		return err
	}
	if work.Commit != nil {
		if accepted == nil || *accepted != *work.Commit {
			return contract.ErrReceipt
		}
	} else {
		if accepted == nil {
			ref, err := w.cfg.Executor.Execute(ctx, *work.Claim, metadata, body, body.Size())
			if err != nil {
				return err
			}
			accepted = &ref
			checked, err := w.cfg.Executor.Reconcile(ctx, *work.Claim, metadata, body, body.Size())
			if err != nil {
				return err
			}
			if checked == nil || *checked != ref {
				return contract.ErrReceipt
			}
		}
		if !accepted.Valid() || accepted.RoomID != work.Target.RoomID {
			return contract.ErrReceipt
		}
		work.Commit = accepted
		if err := w.save(work); err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) acknowledge(ctx context.Context, work *intent) error {
	if err := w.cfg.Source.Revalidate(ctx, *work.Claim, work.Authority); err != nil {
		return err
	}
	receipt := contract.Receipt{Version: 1, Scope: w.cfg.Scope, Epoch: w.cfg.Epoch, TaskID: work.ID, CipherSHA256: work.Claim.Task.Cipher.SHA256, ClaimGeneration: work.Claim.Generation, Commit: *work.Commit}
	if err := receipt.Validate(*work.Claim); err != nil {
		return err
	}
	acknowledged, err := w.cfg.Source.Acknowledge(ctx, *work.Claim, receipt, work.Authority)
	if errors.Is(err, contract.ErrClaim) {
		if err := w.claim(ctx, work); err != nil {
			return err
		}
		acknowledged, err = w.cfg.Source.Acknowledge(ctx, *work.Claim, receipt, work.Authority)
	}
	if err != nil {
		return err
	}
	if acknowledged != receipt {
		return contract.ErrReceipt
	}
	work.Acknowledged = true
	if err := w.save(work); err != nil {
		return err
	}
	return w.cleanup(work)
}

func (w *Worker) cleanup(work *intent) error {
	if err := w.cfg.Journal.DeleteStagedFile(w.owner(), storage.PrivateIngressStagingNamespace, *work.Cipher); err != nil {
		return err
	}
	return w.cfg.Journal.Delete(w.owner(), storage.PrivateIngressNamespace, work.ID)
}

var _ offline.PrivateWork = (*Worker)(nil)

func (w *Worker) newIntent(ctx context.Context, id string) (intent, error) {
	binding, err := w.cfg.Device.PrepareStorage(ctx, w.cfg.Scope)
	if err != nil {
		return intent{}, err
	}
	if !binding.Valid() || binding.Scope != w.cfg.Scope {
		return intent{}, contract.ErrContract
	}
	authority, err := w.cfg.Source.Authority(ctx, w.cfg.Scope)
	if err != nil {
		return intent{}, err
	}
	if authority.RequireActive(w.cfg.Scope, authority.Generation) != nil {
		return intent{}, contract.ErrRevoked
	}
	work := intent{Authority: authority, Version: 1, Scope: w.cfg.Scope, Epoch: w.cfg.Epoch, ID: id, Target: binding.Target, Token: uuid.NewString()}
	if err := w.prepareHandoff(ctx, &work); err != nil {
		return intent{}, err
	}
	return work, nil
}
