// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const (
	outboundNamespace = storage.MailOutboundNamespace
	outboundStage     = storage.MailOutboundStagingNamespace
	// Completed ACKs are encrypted local receipts, not pending operations.
	outboundReceipts = storage.MailOutboundReceiptNamespace
)

var _ api.PrivateMailOutbound = (*Repository)(nil)

type outboundIntent struct {
	Version      int                      `json:"v"`
	Attempt      api.MailOutgoingAttempt  `json:"attempt"`
	Input        model.Submission         `json:"input"`
	DraftID      string                   `json:"draft_id,omitempty"`
	Message      api.MailMessage          `json:"message"`
	Envelope     []byte                   `json:"envelope"`
	SentMailbox  string                   `json:"sent_mailbox"`
	DraftMailbox string                   `json:"draft_mailbox"`
	Size         int64                    `json:"size"`
	SHA256       string                   `json:"sha256"`
	Fingerprint  string                   `json:"fingerprint"`
	Source       *offline.StagedFile      `json:"source,omitempty"`
	ReleaseAt    time.Time                `json:"release_at"`
	Published    bool                     `json:"published,omitempty"`
	Attempted    bool                     `json:"attempted,omitempty"`
	Canceled     bool                     `json:"canceled,omitempty"`
	Receipt      *model.SubmissionReceipt `json:"receipt,omitempty"`
	MIME         storage.Reference        `json:"mime,omitempty"`
	Nonce        []byte                   `json:"nonce,omitempty"`
	Shared       bool                     `json:"shared,omitempty"`
	Proposal     *outboundProposal        `json:"proposal,omitempty"`
}

func (r *Repository) validOutgoing(attempt api.MailOutgoingAttempt) bool {
	return attempt.Scope == r.cfg.Scope && attempt.Epoch == r.cfg.MigrationState.Epoch && canonicalDraftUUID(attempt.Operation)
}
func (r *Repository) outboundOwner() string { return r.draftRevisionOwner() }
func (r *Repository) validateOutbound(work outboundIntent) error {
	if work.Version != 1 || !r.validOutgoing(work.Attempt) || work.Input.Validate() != nil || work.Input.Operation != work.Attempt.Operation || work.Input.Epoch != work.Attempt.Epoch || work.Size <= 0 || work.Size > r.messageLimit() || !storage.Identifier(work.Message.ID) || !storage.Identifier(work.SentMailbox) || !storage.Identifier(work.DraftMailbox) {
		return ErrRecord
	}
	if work.Source != nil && (work.Source.SHA256 != work.SHA256 || work.Source.SizeBytes != work.Size) {
		return ErrRecord
	}
	descriptor := storage.StreamSourceRow{Kind: "mime_message", ID: work.Message.ID, Size: work.Size, SHA256: work.SHA256}
	fingerprint, fpErr := work.Input.FingerprintSHA256(work.SHA256)
	if descriptor.Validate() != nil || fpErr != nil || fingerprint != work.Fingerprint {
		return ErrRecord
	}
	if work.Receipt != nil && validateImmutableSubmissionReceipt(&work, *work.Receipt) != nil {
		return ErrRecord
	}
	return nil
}

// PrepareOutbound captures one immutable message before any external delivery.
// Consent is scoped to this operation and is retained inside the encrypted WAL.
func (r *Repository) PrepareOutbound(parent context.Context, input api.MailOutgoing) (api.MailOutgoingAttempt, error) {
	attempt := api.MailOutgoingAttempt{Scope: r.cfg.Scope, Epoch: r.cfg.MigrationState.Epoch, Operation: input.Operation}
	if !r.validOutgoing(attempt) || len(input.MIME) == 0 || int64(len(input.MIME)) > r.messageLimit() || r.cfg.Submission == nil || r.cfg.AuthorizeSubmission == nil {
		return attempt, ErrRecord
	}
	raw := bytes.Clone(input.MIME)
	defer clear(raw)
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		return r.cfg.Journal.Exclusive(ctx, r.outboundOwner(), outboundNamespace, input.Operation, func() error {
			return r.prepareOutboundLocked(ctx, s, attempt, input, raw)
		})
	})
	return attempt, err
}

func (r *Repository) prepareOutboundLocked(ctx context.Context, s storage.AtomicCatalogSnapshot, attempt api.MailOutgoingAttempt, input api.MailOutgoing, raw []byte) error {
	var work outboundIntent
	found, err := r.cfg.Journal.Read(r.outboundOwner(), outboundNamespace, input.Operation, &work)
	if err != nil {
		return err
	}
	defer clear(work.Envelope)
	if found {
		candidate := model.Submission{Operation: input.Operation, Epoch: attempt.Epoch, From: input.From, Recipients: input.Recipients, Consent: input.Consent}
		if candidate.From == "" {
			candidate.From = work.Input.From
		}
		fingerprint, err := candidate.Fingerprint(raw)
		if err != nil || fingerprint != work.Fingerprint || input.DraftID != work.DraftID || !input.ReleaseAt.Equal(work.ReleaseAt) {
			return ErrConflict
		}
	} else {
		s, err = r.ensureOutboundMailboxes(ctx, s)
		if err != nil {
			return err
		}
		work, err = r.prepareOutgoing(ctx, s, attempt, input, raw)
		if err != nil {
			return err
		}
		if err := r.cfg.Journal.Write(r.outboundOwner(), outboundNamespace, input.Operation, work); err != nil {
			return err
		}
	}
	if err := r.validateOutbound(work); err != nil {
		return err
	}
	if work.Source != nil {
		return nil
	}
	staged, err := r.cfg.Journal.StageFile(ctx, r.outboundOwner(), outboundStage, bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return err
	}
	if staged.SHA256 != work.SHA256 {
		return ErrRecord
	}
	work.Source = &staged
	return r.cfg.Journal.Write(r.outboundOwner(), outboundNamespace, input.Operation, work)
}

func (r *Repository) prepareOutgoing(ctx context.Context, s storage.AtomicCatalogSnapshot, attempt api.MailOutgoingAttempt, input api.MailOutgoing, raw []byte) (outboundIntent, error) {
	var work outboundIntent
	draft, err := r.prepareDraftFromRaw(ctx, s, "outgoing_"+input.Operation, input.From, raw)
	if err != nil {
		return work, err
	}
	defer clear(draft.envelope)
	scheduled, err := r.roleMailbox(ctx, s, "scheduled")
	if err != nil {
		return work, err
	}
	sent, err := r.roleMailbox(ctx, s, "sent")
	if err != nil {
		return work, err
	}
	drafts, err := r.roleMailbox(ctx, s, "drafts")
	if err != nil {
		return work, err
	}
	message := draft.message
	message.MailboxID = scheduled
	message.Keywords = []string{api.MailFlagSeen}
	envelope, err := draftEnvelope(message)
	if err != nil {
		return work, err
	}
	submission := model.Submission{Operation: input.Operation, Epoch: attempt.Epoch, From: message.From, Recipients: input.Recipients, Consent: input.Consent}
	fingerprint, err := submission.Fingerprint(raw)
	if err != nil {
		clear(envelope)
		return work, err
	}
	return outboundIntent{Version: 1, Attempt: attempt, Input: submission, DraftID: input.DraftID, Message: message, Envelope: envelope, SentMailbox: sent, DraftMailbox: drafts, Size: int64(len(raw)), SHA256: digest(raw), Fingerprint: fingerprint, ReleaseAt: input.ReleaseAt}, nil
}

func (r *Repository) loadOutbound(operation string) (outboundIntent, error) {
	var work outboundIntent
	found, err := r.cfg.Journal.Read(r.outboundOwner(), outboundNamespace, operation, &work)
	if err != nil {
		return work, err
	}
	if !found {
		return work, ErrNotFound
	}
	return work, r.validateOutbound(work)
}

func (r *Repository) CommitOutbound(parent context.Context, attempt api.MailOutgoingAttempt) (api.MailOutgoingStatus, error) {
	status := api.MailOutgoingStatus{Attempt: attempt}
	if !r.validOutgoing(attempt) {
		return status, ErrRecord
	}
	var completed *outboundIntent
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		return r.cfg.Journal.Exclusive(ctx, r.outboundOwner(), outboundNamespace, attempt.Operation, func() error {
			var err error
			status, completed, err = r.commitOutboundLocked(ctx, s, attempt)
			return err
		})
	})
	if err != nil || completed == nil {
		return status, err
	}
	return status, r.finalizeCommittedOutbound(parent, attempt, completed)
}

func (r *Repository) commitOutboundLocked(ctx context.Context, s storage.AtomicCatalogSnapshot, attempt api.MailOutgoingAttempt) (api.MailOutgoingStatus, *outboundIntent, error) {
	status := api.MailOutgoingStatus{Attempt: attempt}
	work, err := r.loadOutbound(attempt.Operation)
	if errors.Is(err, ErrNotFound) {
		var receipt api.MailOutgoingStatus
		found, readErr := r.cfg.Journal.Read(r.outboundOwner(), outboundReceipts, attempt.Operation, &receipt)
		if readErr != nil {
			return status, nil, readErr
		}
		switch {
		case !found:
			work, err = r.adoptSharedOutbound(ctx, s, attempt.Operation)
			if err != nil {
				return status, nil, err
			}
		case receipt.Attempt != attempt || !receipt.Receipt.Accepted() && !receipt.Receipt.Canceled:
			return status, nil, ErrNotFound
		default:
			return receipt, nil, nil
		}
	}
	if err != nil {
		return status, nil, err
	}
	defer clear(work.Envelope)
	if work.Canceled && (work.Receipt == nil || !work.Receipt.Canceled) {
		return status, nil, api.ErrDemasiadoTarde
	}
	status.ReleaseAt = work.ReleaseAt
	if work.Receipt != nil && (work.Receipt.Accepted() || work.Receipt.Canceled) {
		status.Receipt = *work.Receipt
		return status, &work, nil
	}
	if err := r.publishAndSubmitOutgoing(ctx, s, &work); err != nil {
		return status, nil, err
	}
	status.ReleaseAt = work.ReleaseAt
	status.Queued = work.ReleaseAt.After(time.Now())
	if work.Receipt != nil {
		status.Receipt = *work.Receipt
		if work.Receipt.Accepted() || work.Receipt.Canceled {
			return status, &work, nil
		}
	}
	return status, nil, nil
}

func (r *Repository) finalizeCommittedOutbound(parent context.Context, attempt api.MailOutgoingAttempt, completed *outboundIntent) error {
	// Publish Sent only after the durable external receipt accepts every recipient.
	// Drafts remain intact: a scheduled operation must not delete a newer editor.
	destination := completed.SentMailbox
	if completed.Receipt.Canceled {
		destination = completed.DraftMailbox
	}
	if err := r.changeMessage(parent, []string{completed.Message.ID}, "mailbox", destination, func(message api.MailMessage) (api.MailMessage, error) {
		message.MailboxID = destination
		if completed.Receipt.Canceled && !message.Has("\\Draft") {
			message.Keywords = append(message.Keywords, "\\Draft")
		}
		return message, nil
	}); err != nil {
		return err
	}
	if err := r.retireSharedOutbound(parent, attempt); err != nil {
		return err
	}
	return r.cleanupOutbound(parent, attempt)
}

func (r *Repository) publishQueueOutbound(ctx context.Context, s storage.AtomicCatalogSnapshot, work *outboundIntent) error {
	if work.Published {
		return nil
	}
	if work.Source == nil {
		return ErrDraftRevisionSource
	}
	source, err := r.cfg.Journal.OpenStagedFile(ctx, r.outboundOwner(), outboundStage, *work.Source)
	if err != nil {
		return err
	}
	defer source.Close()
	checksum, err := sourceDigest(ctx, source, work.Size, r.messageLimit())
	if err != nil {
		return err
	}
	if checksum != work.SHA256 {
		return ErrRecord
	}
	if err := r.storeMessage(ctx, s, "outbound_queue_"+work.Attempt.Operation, ingestMessage{ID: work.Message.ID, MailboxID: work.Message.MailboxID, Size: work.Size}, work.Envelope, source, work.Size); err != nil {
		return err
	}
	work.Published = true
	return r.saveOutbound(work)
}

func (r *Repository) prepareSharedOutbound(ctx context.Context, s storage.AtomicCatalogSnapshot, work *outboundIntent) error {
	if err := r.publishQueueOutbound(ctx, s, work); err != nil {
		return err
	}
	return r.ensureSharedOutbound(ctx, s, work)
}

func (r *Repository) publishAndSubmitOutgoing(ctx context.Context, s storage.AtomicCatalogSnapshot, work *outboundIntent) error {
	if err := r.prepareSharedOutbound(ctx, s, work); err != nil {
		return err
	}
	if work.Receipt != nil && !containsReady(work.Receipt.States) {
		return nil
	}
	if work.ReleaseAt.After(time.Now()) {
		return nil
	}
	if err := r.stageSharedOutbound(ctx, work); err != nil {
		return err
	}
	source, err := r.cfg.Journal.OpenStagedFile(ctx, r.outboundOwner(), outboundStage, *work.Source)
	if err != nil {
		return err
	}
	defer source.Close()
	checksum, err := sourceDigest(ctx, source, work.Size, r.messageLimit())
	if err != nil {
		return err
	}
	if checksum != work.SHA256 {
		return ErrRecord
	}
	raw, err := io.ReadAll(io.NewSectionReader(source, 0, work.Size))
	if err != nil {
		clear(raw)
		return err
	}
	defer clear(raw)
	fingerprint, err := work.Input.Fingerprint(raw)
	if err != nil || fingerprint != work.Fingerprint {
		return ErrRecord
	}
	work.Attempted = true
	if err := r.saveOutbound(work); err != nil {
		return err
	}
	if err := r.cfg.AuthorizeSubmission(ctx); err != nil {
		return err
	}
	receipt, err := r.cfg.Submission.Submit(ctx, work.Input, bytes.NewReader(raw), work.Size)
	if err != nil {
		return err
	}
	if receipt.Validate(work.Input, work.Fingerprint) != nil {
		return model.ErrSubmissionControl
	}
	if err := r.cfg.AuthorizeSubmission(ctx); err != nil {
		return err
	}
	work.Receipt = &receipt
	return r.saveOutbound(work)
}

func containsReady(states []string) bool {
	for _, state := range states {
		if state == "ready" {
			return true
		}
	}
	return false
}

func (r *Repository) cleanupOutbound(parent context.Context, attempt api.MailOutgoingAttempt) error {
	return r.run(parent, true, func(ctx context.Context, _ storage.AtomicCatalogSnapshot) error {
		return r.cfg.Journal.Exclusive(ctx, r.outboundOwner(), outboundNamespace, attempt.Operation, func() error {
			work, err := r.loadOutbound(attempt.Operation)
			if err != nil {
				return err
			}
			defer clear(work.Envelope)
			if work.Receipt == nil || !work.Receipt.Accepted() && !work.Receipt.Canceled {
				return ErrRecord
			}
			receipt := api.MailOutgoingStatus{Attempt: attempt, Receipt: *work.Receipt, ReleaseAt: work.ReleaseAt}
			if err := r.cfg.Journal.Write(r.outboundOwner(), outboundReceipts, attempt.Operation, receipt); err != nil {
				return err
			}
			if work.Source != nil {
				if err := r.cfg.Journal.DeleteStagedFile(r.outboundOwner(), outboundStage, *work.Source); err != nil {
					return err
				}
			}
			return r.cfg.Journal.Delete(r.outboundOwner(), outboundNamespace, attempt.Operation)
		})
	})
}

func (r *Repository) PendingOutbound(parent context.Context) ([]api.MailOutgoingAttempt, error) {
	var out []api.MailOutgoingAttempt
	err := r.run(parent, true, func(ctx context.Context, _ storage.AtomicCatalogSnapshot) error {
		rows, err := r.cfg.Journal.List(r.outboundOwner(), outboundNamespace)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if err := ctx.Err(); err != nil {
				return err
			}
			var work outboundIntent
			if json.Unmarshal(row.Value, &work) != nil || work.Attempt.Operation != row.ID || r.validateOutbound(work) != nil {
				return ErrRecord
			}
			if !work.Canceled {
				out = append(out, work.Attempt)
			}
			clear(work.Envelope)
		}
		return nil
	})
	return out, err
}

func (r *Repository) CancelPrivateOutbound(parent context.Context, attempt api.MailOutgoingAttempt) error {
	if !r.validOutgoing(attempt) {
		return ErrRecord
	}
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		return r.cfg.Journal.Exclusive(ctx, r.outboundOwner(), outboundNamespace, attempt.Operation, func() error {
			work, err := r.loadOutbound(attempt.Operation)
			if errors.Is(err, ErrNotFound) {
				var completed api.MailOutgoingStatus
				found, readErr := r.cfg.Journal.Read(r.outboundOwner(), outboundReceipts, attempt.Operation, &completed)
				if readErr != nil {
					return readErr
				}
				if found && completed.Attempt == attempt && completed.Receipt.Canceled {
					return nil
				}
				work, err = r.adoptSharedOutbound(ctx, s, attempt.Operation)
			}
			if err != nil {
				return err
			}
			defer clear(work.Envelope)
			return r.proposeOutboundControl(ctx, s, &work, work.ReleaseAt, true)
		})
	})
	if err != nil {
		return err
	}
	status, err := r.CommitOutbound(parent, attempt)
	if err != nil {
		return err
	}
	if !status.Receipt.Canceled {
		return api.ErrDemasiadoTarde
	}
	return nil
}

// Scheduled is a client-owned E2E mailbox, not a legacy SQL account setting.
// Existing accounts may predate it (or Sent/Drafts); connecting a mailbox must
// not require manual folder setup before the first private send.
func (r *Repository) ensureOutboundMailboxes(ctx context.Context, s storage.AtomicCatalogSnapshot) (storage.AtomicCatalogSnapshot, error) {
	documents, err := r.list(ctx, s, "mailbox")
	if err != nil {
		return s, err
	}
	defer clearDocuments(documents)
	roles := map[string]bool{}
	for _, raw := range documents {
		var box api.Mailbox
		if json.Unmarshal(raw, &box) != nil {
			return s, ErrRecord
		}
		if box.Role != "" {
			if roles[box.Role] {
				return s, ErrConflict
			}
			roles[box.Role] = true
		}
	}
	changed := false
	for _, role := range []struct{ id, name string }{{"drafts", "Drafts"}, {"sent", "Sent"}, {"scheduled", "Scheduled"}} {
		if roles[role.id] {
			continue
		}
		id := "builtin_" + role.id
		if _, exists := documents[id]; exists {
			return s, ErrConflict
		}
		raw, err := json.Marshal(api.Mailbox{ID: id, Name: role.name, Role: role.id})
		if err != nil {
			return s, err
		}
		if err := r.writeDocument(ctx, s, "mailbox", id, raw, "live"); err != nil {
			return s, err
		}
		changed = true
	}
	if changed {
		return storage.LoadAtomicCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
	}
	return s, nil
}
