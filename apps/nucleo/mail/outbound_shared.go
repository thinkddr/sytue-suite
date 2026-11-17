// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type outboundProposal struct {
	ExpectedGeneration int64     `json:"expected_generation"`
	ExpectedControl    string    `json:"expected_control"`
	Generation         int64     `json:"generation"`
	Nonce              []byte    `json:"nonce"`
	ReleaseAt          time.Time `json:"release_at"`
	Cancel             bool      `json:"cancel"`
	Control            string    `json:"control"`
}

type outboundSharedJob struct {
	Version      int                     `json:"v"`
	Attempt      api.MailOutgoingAttempt `json:"attempt"`
	Input        model.Submission        `json:"input"`
	Fingerprint  string                  `json:"fingerprint"`
	Message      api.MailMessage         `json:"message"`
	Envelope     []byte                  `json:"envelope"`
	MIME         storage.Reference       `json:"mime"`
	Size         int64                   `json:"size"`
	SHA256       string                  `json:"sha256"`
	SentMailbox  string                  `json:"sent_mailbox"`
	DraftMailbox string                  `json:"draft_mailbox"`
	ReleaseAt    time.Time               `json:"release_at"`
	Nonce        []byte                  `json:"nonce"`
	Proposal     *outboundProposal       `json:"proposal,omitempty"`
}

// The nonce hides low-entropy schedule values from the relay's opaque CAS
// commitment. It is retained only in the existing encrypted SDK and journal.
func outboundControl(work *outboundIntent, generation int64, nonce []byte, release time.Time, cancel bool) (string, error) {
	if generation <= 0 || len(nonce) != 32 {
		return "", ErrRecord
	}
	encoded, err := json.Marshal(struct {
		Kind        string                  `json:"kind"`
		Attempt     api.MailOutgoingAttempt `json:"attempt"`
		Fingerprint string                  `json:"fingerprint"`
		Generation  int64                   `json:"generation"`
		Nonce       []byte                  `json:"nonce"`
		ReleaseAt   time.Time               `json:"release_at"`
		Cancel      bool                    `json:"cancel"`
	}{"org.sytue.mail.outbound.control.v1", work.Attempt, work.Fingerprint, generation, nonce, release.UTC(), cancel})
	if err != nil {
		return "", err
	}
	defer clear(encoded)
	return digest(encoded), nil
}

func (r *Repository) sharedJob(work *outboundIntent) outboundSharedJob {
	return outboundSharedJob{Version: 1, Attempt: work.Attempt, Input: work.Input, Fingerprint: work.Fingerprint, Message: work.Message, Envelope: work.Envelope, MIME: work.MIME, Size: work.Size, SHA256: work.SHA256, SentMailbox: work.SentMailbox, DraftMailbox: work.DraftMailbox, ReleaseAt: work.ReleaseAt, Nonce: work.Nonce, Proposal: work.Proposal}
}

func (r *Repository) validateSharedJob(job outboundSharedJob) error {
	work := outboundIntent{Version: 1, Attempt: job.Attempt, Input: job.Input, Fingerprint: job.Fingerprint, Message: job.Message, Envelope: job.Envelope, MIME: job.MIME, Size: job.Size, SHA256: job.SHA256, SentMailbox: job.SentMailbox, DraftMailbox: job.DraftMailbox, ReleaseAt: job.ReleaseAt, Nonce: job.Nonce, Proposal: job.Proposal}
	if job.Version != 1 || !job.MIME.Valid() || job.Input.Generation <= 0 || r.validateOutbound(work) != nil {
		return ErrRecord
	}
	commitment, err := outboundControl(&work, job.Input.Generation, job.Nonce, job.ReleaseAt, false)
	if err != nil || commitment != job.Input.Control {
		return ErrRecord
	}
	if proposal := job.Proposal; proposal != nil {
		if proposal.ExpectedGeneration != job.Input.Generation || proposal.ExpectedControl != job.Input.Control || proposal.Generation != job.Input.Generation+1 {
			return ErrRecord
		}
		want, err := outboundControl(&work, proposal.Generation, proposal.Nonce, proposal.ReleaseAt, proposal.Cancel)
		if err != nil || want != proposal.Control {
			return ErrRecord
		}
	}
	return nil
}

func (r *Repository) readSharedJob(ctx context.Context, s storage.AtomicCatalogSnapshot, operation string) (outboundSharedJob, error) {
	var job outboundSharedJob
	raw, err := r.document(ctx, s, "outbound", operation)
	if err != nil {
		return job, err
	}
	defer clear(raw)
	if json.Unmarshal(raw, &job) != nil || job.Attempt.Operation != operation {
		return job, ErrRecord
	}
	return job, r.validateSharedJob(job)
}

func (r *Repository) writeSharedJob(ctx context.Context, s storage.AtomicCatalogSnapshot, work *outboundIntent) error {
	if !work.MIME.Valid() {
		return ErrRecord
	}
	raw, err := json.Marshal(r.sharedJob(work))
	if err != nil {
		return err
	}
	defer clear(raw)
	return r.writeDocument(ctx, s, "outbound", work.Attempt.Operation, raw, "live")
}

func validateImmutableSubmissionReceipt(work *outboundIntent, receipt model.SubmissionReceipt) error {
	input := work.Input
	input.Generation, input.Control = 0, ""
	return receipt.Validate(input, work.Fingerprint)
}

func (r *Repository) saveOutbound(work *outboundIntent) error {
	return r.cfg.Journal.Write(r.outboundOwner(), outboundNamespace, work.Attempt.Operation, work)
}

func (r *Repository) ensureSharedOutbound(ctx context.Context, _ storage.AtomicCatalogSnapshot, work *outboundIntent) error {
	if err := r.cfg.AuthorizeSubmission(ctx); err != nil {
		return err
	}
	current, err := storage.LoadAtomicCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
	if err != nil {
		return err
	}
	if err := r.reconcileSharedOutboundWrites(ctx, current, work.Attempt.Operation); err != nil {
		return err
	}
	job, readErr := r.readSharedJob(ctx, current, work.Attempt.Operation)
	if readErr == nil {
		return r.reconcileExistingSharedOutbound(ctx, current, job, work)
	}
	if !errors.Is(readErr, ErrNotFound) {
		return readErr
	}
	if work.Shared || len(heads(current, objectID("outbound", work.Attempt.Operation))) != 0 {
		receipt, err := r.cfg.Submission.Receipt(ctx, work.Attempt.Operation)
		if err != nil {
			return err
		}
		if validateImmutableSubmissionReceipt(work, receipt) != nil || !receipt.Canceled && !receipt.Accepted() {
			return ErrConflict
		}
		work.Receipt = &receipt
		work.Canceled = receipt.Canceled
		return r.saveOutbound(work)
	}
	return r.registerSharedOutbound(ctx, current, work)
}

func (r *Repository) reconcileExistingSharedOutbound(ctx context.Context, current storage.AtomicCatalogSnapshot, job outboundSharedJob, work *outboundIntent) error {
	localProposal := work.Proposal
	if localProposal != nil && job.Input.Generation == localProposal.ExpectedGeneration && job.Input.Control == localProposal.ExpectedControl {
		if job.Proposal != nil && job.Proposal.Control != localProposal.Control {
			return ErrConflict
		}
		if job.Proposal == nil {
			job.Proposal = localProposal
		}
	}
	if job.Fingerprint != work.Fingerprint || job.SHA256 != work.SHA256 || job.Size != work.Size || job.Message.ID != work.Message.ID {
		return ErrConflict
	}
	work.Input = job.Input
	work.ReleaseAt = job.ReleaseAt
	work.Nonce = bytes.Clone(job.Nonce)
	work.MIME = job.MIME
	work.Proposal = job.Proposal
	work.Shared = true
	if localProposal != nil {
		if err := r.writeSharedJob(ctx, current, work); err != nil {
			return err
		}
	}
	return r.synchronizeSharedOutbound(ctx, current, work)
}

func (r *Repository) registerSharedOutbound(ctx context.Context, current storage.AtomicCatalogSnapshot, work *outboundIntent) error {
	if work.Input.Generation == 0 {
		nonce := make([]byte, 32)
		if _, err := rand.Read(nonce); err != nil {
			return err
		}
		commitment, err := outboundControl(work, 1, nonce, work.ReleaseAt, false)
		if err != nil {
			clear(nonce)
			return err
		}
		work.Nonce, work.Input.Generation, work.Input.Control = nonce, 1, commitment
		if err := r.saveOutbound(work); err != nil {
			return err
		}
	}
	registered, err := r.cfg.Submission.Register(ctx, model.SubmissionRegistration{Input: work.Input, Fingerprint: work.Fingerprint})
	if err != nil {
		return err
	}
	if registered.Validate(work.Input, work.Fingerprint) != nil {
		return model.ErrSubmissionControl
	}
	if !work.MIME.Valid() {
		raw, err := r.document(ctx, current, "message", work.Message.ID)
		if err != nil {
			return err
		}
		_, message, err := decodeMessage(raw, work.Message.ID)
		clear(raw)
		clear(message.Envelope)
		if err != nil || message.Operation != "outbound_queue_"+work.Attempt.Operation {
			return ErrRecord
		}
		work.MIME = message.MIME
	}
	if err := r.writeSharedJob(ctx, current, work); err != nil {
		return err
	}
	work.Shared = true
	work.Receipt = &registered
	if err := r.saveOutbound(work); err != nil {
		return err
	}
	// Reload after publishing: another device may already have proposed a change.
	current, err = storage.LoadAtomicCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
	if err != nil {
		return err
	}
	return r.synchronizeSharedOutbound(ctx, current, work)
}

func (r *Repository) synchronizeSharedOutbound(ctx context.Context, s storage.AtomicCatalogSnapshot, work *outboundIntent) error {
	if err := r.cfg.AuthorizeSubmission(ctx); err != nil {
		return err
	}
	receipt, err := r.cfg.Submission.Receipt(ctx, work.Attempt.Operation)
	if err != nil {
		return err
	}
	if validateImmutableSubmissionReceipt(work, receipt) != nil {
		return ErrRecord
	}
	if receipt.Canceled {
		work.Receipt = &receipt
		work.Canceled = true
		return r.saveOutbound(work)
	}
	if proposal := work.Proposal; proposal != nil {
		change := model.SubmissionControlChange{Epoch: work.Attempt.Epoch, Operation: work.Attempt.Operation, Fingerprint: work.Fingerprint, Generation: proposal.ExpectedGeneration, Control: proposal.ExpectedControl, NextControl: proposal.Control, Cancel: proposal.Cancel}
		receipt, err = r.cfg.Submission.Control(ctx, change)
		if err != nil {
			return err
		}
		if validateImmutableSubmissionReceipt(work, receipt) != nil || receipt.Generation != proposal.Generation || receipt.Control != proposal.Control {
			return model.ErrSubmissionControl
		}
		if proposal.Cancel {
			work.Receipt = &receipt
			work.Canceled = true
			return r.saveOutbound(work)
		}
		work.Input.Generation, work.Input.Control = proposal.Generation, proposal.Control
		work.Nonce, work.ReleaseAt = bytes.Clone(proposal.Nonce), proposal.ReleaseAt
		work.Proposal = nil
		if err := r.saveOutbound(work); err != nil {
			return err
		}
		if err := r.writeSharedJob(ctx, s, work); err != nil {
			return err
		}
	}
	if receipt.Validate(work.Input, work.Fingerprint) != nil {
		return model.ErrSubmissionControl
	}
	work.Receipt = &receipt
	return r.saveOutbound(work)
}

// stageSharedOutbound uses the existing authenticated MIME reader. No new
// cipher, plaintext file cache or legacy message endpoint is introduced.
func (r *Repository) stageSharedOutbound(ctx context.Context, work *outboundIntent) error {
	if work.Source != nil {
		return nil
	}
	snapshot, err := storage.LoadAtomicCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
	if err != nil {
		return err
	}
	version, err := r.readMIMEVersion(snapshot, work.MIME, work.Message.ID)
	if err != nil {
		return err
	}
	pointer := version.Commit.Content
	if pointer == nil || pointer.Kind != "file" {
		return ErrRecord
	}
	_, checksum, err := r.cfg.Files.Commitment(ctx, pointer.Reference, version.ObjectID, pointer.RevisionID)
	if err != nil || checksum != work.SHA256 {
		return ErrRecord
	}
	body, err := r.openMIMEReader(ctx, ctx, version, work.Size)
	if err != nil {
		return err
	}
	defer body.Close()
	staged, err := r.cfg.Journal.StageFile(ctx, r.outboundOwner(), outboundStage, body, work.Size)
	if err != nil {
		return err
	}
	if staged.SHA256 != work.SHA256 || staged.SizeBytes != work.Size {
		return ErrRecord
	}
	work.Source = &staged
	return r.saveOutbound(work)
}

func (r *Repository) adoptSharedOutbound(ctx context.Context, s storage.AtomicCatalogSnapshot, operation string) (outboundIntent, error) {
	job, err := r.readSharedJob(ctx, s, operation)
	if err != nil {
		return outboundIntent{}, err
	}
	work := outboundIntent{Version: 1, Attempt: job.Attempt, Input: job.Input, Fingerprint: job.Fingerprint, Message: job.Message, Envelope: bytes.Clone(job.Envelope), MIME: job.MIME, Size: job.Size, SHA256: job.SHA256, SentMailbox: job.SentMailbox, DraftMailbox: job.DraftMailbox, ReleaseAt: job.ReleaseAt, Nonce: bytes.Clone(job.Nonce), Proposal: job.Proposal, Published: true, Shared: true}
	if err := r.validateOutbound(work); err != nil {
		return work, err
	}
	if err := r.synchronizeSharedOutbound(ctx, s, &work); err != nil {
		return work, err
	}
	err = r.saveOutbound(&work)
	return work, err
}

func (r *Repository) retireSharedOutbound(parent context.Context, attempt api.MailOutgoingAttempt) error {
	return r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		versions := heads(s, objectID("outbound", attempt.Operation))
		if len(versions) == 0 || len(versions) == 1 && versions[0].Commit.State == "deleted" {
			return nil
		}
		return r.writeDocument(ctx, s, "outbound", attempt.Operation, []byte("null"), "deleted")
	})
}

func (r *Repository) proposeOutboundControl(ctx context.Context, s storage.AtomicCatalogSnapshot, work *outboundIntent, release time.Time, cancel bool) error {
	if err := r.prepareSharedOutbound(ctx, s, work); err != nil {
		return err
	}
	if work.Receipt == nil {
		return ErrRecord
	}
	if work.Receipt.Canceled {
		if cancel {
			return nil
		}
		return api.ErrDemasiadoTarde
	}
	for _, state := range work.Receipt.States {
		if state != "ready" && state != "failed" {
			return api.ErrDemasiadoTarde
		}
	}
	if work.Proposal == nil {
		nonce := make([]byte, 32)
		if _, err := rand.Read(nonce); err != nil {
			return err
		}
		generation := work.Input.Generation + 1
		commitment, err := outboundControl(work, generation, nonce, release, cancel)
		if err != nil {
			clear(nonce)
			return err
		}
		work.Proposal = &outboundProposal{ExpectedGeneration: work.Input.Generation, ExpectedControl: work.Input.Control, Generation: generation, Nonce: nonce, ReleaseAt: release, Cancel: cancel, Control: commitment}
		if err := r.saveOutbound(work); err != nil {
			return err
		}
	} else if work.Proposal.Cancel != cancel || !work.Proposal.ReleaseAt.Equal(release) {
		return ErrConflict
	}
	current, err := storage.LoadAtomicCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
	if err != nil {
		return err
	}
	// The proposal is E2E and durable before the global CAS. A fresh authorized
	// device can complete it even if its origin disappears after this step.
	if err := r.writeSharedJob(ctx, current, work); err != nil {
		return err
	}
	current, err = storage.LoadAtomicCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
	if err != nil {
		return err
	}
	return r.synchronizeSharedOutbound(ctx, current, work)
}

func (r *Repository) DiscoverOutbound(parent context.Context) error {
	return r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		seen, failures, err := r.discoverCatalogOutbound(ctx, s)
		if err != nil {
			return err
		}
		localFailures, err := r.discoverLocalOutbound(ctx, s, seen)
		if err != nil {
			return err
		}
		return errors.Join(append(failures, localFailures...)...)
	})
}

func (r *Repository) discoverCatalogOutbound(ctx context.Context, s storage.AtomicCatalogSnapshot) (map[string]bool, []error, error) {
	seen := map[string]bool{}
	var failures []error
	for _, version := range s.Heads {
		header, err := decodeHeader(version)
		if err != nil {
			return nil, nil, err
		}
		if header.Type != "outbound" || seen[header.ID] || version.Commit.State == "deleted" {
			continue
		}
		seen[header.ID] = true
		err = r.cfg.Journal.Exclusive(ctx, r.outboundOwner(), outboundNamespace, header.ID, func() error {
			work, err := r.loadOutbound(header.ID)
			defer clear(work.Envelope)
			if err == nil {
				return r.ensureSharedOutbound(ctx, s, &work)
			}
			if !errors.Is(err, ErrNotFound) {
				return err
			}
			adopted, err := r.adoptSharedOutbound(ctx, s, header.ID)
			clear(adopted.Envelope)
			return err
		})
		if err != nil {
			failures = append(failures, err)
		}
	}
	return seen, failures, nil
}

func (r *Repository) discoverLocalOutbound(ctx context.Context, s storage.AtomicCatalogSnapshot, seen map[string]bool) ([]error, error) {
	// Completed jobs are tombstoned in the SDK. Retained local owners still
	// reconcile their relay receipt so schedules disappear promptly everywhere.
	local, err := r.cfg.Journal.List(r.outboundOwner(), outboundNamespace)
	if err != nil {
		return nil, err
	}
	var failures []error
	for _, row := range local {
		if seen[row.ID] {
			continue
		}
		err := r.cfg.Journal.Exclusive(ctx, r.outboundOwner(), outboundNamespace, row.ID, func() error {
			work, err := r.loadOutbound(row.ID)
			if err != nil {
				return err
			}
			defer clear(work.Envelope)
			if !work.Shared {
				return nil
			}
			return r.ensureSharedOutbound(ctx, s, &work)
		})
		if err != nil {
			failures = append(failures, err)
		}
	}
	return failures, nil
}

// Job metadata can advance on another device after its publication ACK was
// lost. Retire only authenticated accepted writes for this retained job; never
// auto-acknowledge an unrelated editor's content or publish an old intent.
func (r *Repository) reconcileSharedOutboundWrites(ctx context.Context, s storage.AtomicCatalogSnapshot, operation string) error {
	owner := r.outboundOwner()
	rows, err := r.cfg.Journal.List(owner, writesNamespace)
	if err != nil {
		return err
	}
	for _, row := range rows {
		var candidate writeIntent
		if json.Unmarshal(row.Value, &candidate) != nil {
			return ErrRecord
		}
		clear(candidate.Payload)
		if candidate.Header.Type != "outbound" || candidate.Header.ID != operation {
			continue
		}
		if err := r.cfg.Journal.Exclusive(ctx, owner, writesNamespace, row.ID, func() error {
			var work writeIntent
			found, err := r.cfg.Journal.Read(owner, writesNamespace, row.ID, &work)
			if err != nil || !found {
				return err
			}
			defer clear(work.Payload)
			if work.Header.Type != "outbound" || work.Header.ID != operation || work.validate(r.cfg.Scope, work.Header, work.Payload, work.ForceFile) != nil {
				return ErrRecord
			}
			object := objectID("outbound", operation)
			version, accepted, err := storage.AcceptedAtomicObject(s, r.cfg.Scope, object, work.Revision+"_index", work.Index)
			if err != nil || !accepted {
				return err
			}
			state := version.Commit.State
			raw, _ := json.Marshal(work.Header)
			key := digest(append(append(bytes.Clone(raw), []byte("\x00"+state+"\x00")...), work.Payload...))
			if key != row.ID {
				return ErrRecord
			}
			_, err = r.completeDocumentWrite(ctx, s, owner, row.ID, object, &work, state)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}
