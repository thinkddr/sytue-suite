// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var (
	ErrDraftRevisionPending = errors.New("mail: an admitted draft revision still awaits acknowledgement")
	ErrDraftRevisionSource  = errors.New("mail: draft revision capture is incomplete")
)

var _ api.PrivateMailDraftRevisions = (*Repository)(nil)

type draftRevisionIntent struct {
	Version  int                       `json:"v"`
	Attempt  api.MailDraftAttempt      `json:"attempt"`
	Epoch    string                    `json:"epoch"`
	Target   storage.PublicationTarget `json:"target"`
	Parents  []storage.Reference       `json:"parents"`
	Message  api.MailMessage           `json:"message"`
	Envelope []byte                    `json:"envelope"`
	Size     int64                     `json:"size"`
	SHA256   string                    `json:"sha256"`
	Source   *offline.StagedFile       `json:"source,omitempty"`
	Result   *api.MailMessage          `json:"result,omitempty"`
}

func canonicalDraftUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == value
}

func (r *Repository) draftRevisionOwner() string {
	return r.cfg.Scope.Tenant + "\x00" + r.cfg.Scope.Subject + "\x00mail"
}

func draftRevisionOperation(attempt api.MailDraftAttempt) string {
	return "draft_revision_" + digest([]byte(attempt.Scope.Tenant+"\x00"+attempt.Scope.Subject+"\x00mail\x00"+attempt.Epoch+"\x00"+attempt.Editor+"\x00"+attempt.Revision))
}

func (r *Repository) validDraftAttempt(value api.MailDraftAttempt) bool {
	return value.Scope == r.cfg.Scope && value.Epoch == r.cfg.MigrationState.Epoch && canonicalDraftUUID(value.Editor) && canonicalDraftUUID(value.Revision) && storage.Identifier(value.ID)
}

func (r *Repository) validateDraftRevision(work draftRevisionIntent) error {
	if work.Version != 1 || !r.validDraftAttempt(work.Attempt) || work.Epoch != r.cfg.MigrationState.Epoch || work.Size <= 0 || work.Size > r.messageLimit() || len(work.Parents) > 1 {
		return ErrRecord
	}
	row := storage.StreamSourceRow{Kind: "mime_message", ID: work.Attempt.ID, Size: work.Size, SHA256: work.SHA256}
	if row.Validate() != nil || work.Message.ID != work.Attempt.ID || work.Message.Size != work.Size || validateDraftRevisionEnvelope(work) != nil {
		return ErrRecord
	}
	for _, parent := range work.Parents {
		if !parent.Valid() {
			return ErrRecord
		}
	}
	if work.Source != nil && (work.Source.SizeBytes != work.Size || work.Source.SHA256 != work.SHA256) {
		return ErrRecord
	}
	if work.Result != nil && work.Result.ID != work.Attempt.ID {
		return ErrRecord
	}
	return r.cfg.Crypto.CheckStoragePublicationTarget(work.Target)
}

func validateDraftRevisionEnvelope(work draftRevisionIntent) error {
	var envelope ingestMessage
	if len(work.Envelope) > storage.MaxRecordBytes/2 || json.Unmarshal(work.Envelope, &envelope) != nil || envelope.ID != work.Attempt.ID || envelope.MailboxID != work.Message.MailboxID || envelope.Size != work.Size {
		return ErrRecord
	}
	return nil
}

// PrepareDraftRevision admits exactly one immutable revision for an editor. The
// intent exists before capture, so cancellation cannot hide active draft work.
func (r *Repository) PrepareDraftRevision(parent context.Context, input api.MailDraftRevision) (api.MailDraftAttempt, error) {
	if !canonicalDraftUUID(input.Editor) || !canonicalDraftUUID(input.Revision) || len(input.MIME) == 0 || input.ID != "" && !storage.Identifier(input.ID) {
		return api.MailDraftAttempt{}, ErrRecord
	}
	if int64(len(input.MIME)) > r.messageLimit() {
		return api.MailDraftAttempt{}, api.ErrMailTooLarge
	}
	raw := bytes.Clone(input.MIME)
	defer clear(raw)
	if input.ID == "" {
		input.ID = "draft_editor_" + digest([]byte(input.Editor))
	}
	attempt := api.MailDraftAttempt{Scope: r.cfg.Scope, Epoch: r.cfg.MigrationState.Epoch, Editor: input.Editor, Revision: input.Revision, ID: input.ID}
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		owner := r.draftRevisionOwner()
		return r.cfg.Journal.Exclusive(ctx, owner, storage.MailDraftRevisionNamespace, input.Editor, func() error {
			work, err := r.loadDraftRevision(ctx, s, attempt, input.From, raw)
			if err != nil {
				return err
			}
			defer clear(work.Envelope)
			if err := r.validateDraftRevision(work); err != nil {
				return err
			}
			if work.Source != nil || work.Result != nil {
				return nil
			}
			source, err := r.cfg.Journal.StageFile(ctx, owner, storage.MailDraftRevisionStagingNamespace, bytes.NewReader(raw), int64(len(raw)))
			if err != nil {
				return err
			}
			if source.SHA256 != work.SHA256 || source.SizeBytes != work.Size {
				return ErrRecord
			}
			work.Source = &source
			// Preserve the encrypted source even if the acknowledgement of this local
			// write is uncertain. A retry must not destroy a possibly admitted intent.
			return r.cfg.Journal.Write(owner, storage.MailDraftRevisionNamespace, input.Editor, work)
		})
	})
	return attempt, err
}

func (r *Repository) loadDraftRevision(ctx context.Context, s storage.AtomicCatalogSnapshot, attempt api.MailDraftAttempt, from string, raw []byte) (draftRevisionIntent, error) {
	owner := r.draftRevisionOwner()
	var work draftRevisionIntent
	found, err := r.cfg.Journal.Read(owner, storage.MailDraftRevisionNamespace, attempt.Editor, &work)
	if err != nil {
		return work, err
	}
	if found {
		if work.Attempt != attempt {
			return work, ErrDraftRevisionPending
		}
		if work.SHA256 != digest(raw) || work.Size != int64(len(raw)) || from != "" && !strings.EqualFold(from, work.Message.From) {
			return work, ErrRecord
		}
		return work, nil
	}
	prepared, err := r.prepareDraftFromRaw(ctx, s, attempt.ID, from, raw)
	if err != nil {
		return work, err
	}
	defer clear(prepared.envelope)
	parents, err := versionReferences(s, objectID("message", attempt.ID))
	if err != nil {
		return work, err
	}
	work = draftRevisionIntent{Version: 1, Attempt: attempt, Epoch: r.cfg.MigrationState.Epoch, Target: s.Binding.Target, Parents: parents, Message: prepared.message, Envelope: bytes.Clone(prepared.envelope), Size: int64(len(raw)), SHA256: digest(raw)}
	return work, r.cfg.Journal.Write(owner, storage.MailDraftRevisionNamespace, attempt.Editor, work)
}

// CommitDraftRevision reads the original encrypted capture. It never accepts
// replacement MIME or recaptures newer parents after an uncertain attempt.
func (r *Repository) CommitDraftRevision(parent context.Context, attempt api.MailDraftAttempt) (api.MailMessage, error) {
	if !r.validDraftAttempt(attempt) {
		return api.MailMessage{}, ErrRecord
	}
	var result api.MailMessage
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		owner := r.draftRevisionOwner()
		return r.cfg.Journal.Exclusive(ctx, owner, storage.MailDraftRevisionNamespace, attempt.Editor, func() error {
			var err error
			result, err = r.commitDraftRevisionLocked(ctx, s, owner, attempt)
			return err
		})
	})
	return result, err
}

// commitDraftRevisionLocked retains both repository admission and the editor's
// journal exclusion until the immutable source and its intent finish cleanup.
func (r *Repository) commitDraftRevisionLocked(ctx context.Context, s storage.AtomicCatalogSnapshot, owner string, attempt api.MailDraftAttempt) (api.MailMessage, error) {
	var work draftRevisionIntent
	found, err := r.cfg.Journal.Read(owner, storage.MailDraftRevisionNamespace, attempt.Editor, &work)
	if err != nil {
		return api.MailMessage{}, err
	}
	if !found {
		result, committed, err := r.findDraftRevision(ctx, s, attempt, nil)
		if err != nil {
			return result, err
		}
		if !committed {
			return result, ErrNotFound
		}
		return result, nil
	}
	if work.Attempt != attempt {
		return api.MailMessage{}, ErrDraftRevisionPending
	}
	defer clear(work.Envelope)
	if err := r.validateDraftRevision(work); err != nil {
		return api.MailMessage{}, err
	}
	if work.Result == nil {
		if err := r.commitDraftRevision(ctx, s, owner, &work); err != nil {
			return api.MailMessage{}, err
		}
	}
	result := *work.Result
	if work.Source != nil {
		if err := r.cfg.Journal.DeleteStagedFile(owner, storage.MailDraftRevisionStagingNamespace, *work.Source); err != nil {
			return result, err
		}
	}
	return result, r.cfg.Journal.Delete(owner, storage.MailDraftRevisionNamespace, attempt.Editor)
}

func (r *Repository) commitDraftRevision(ctx context.Context, s storage.AtomicCatalogSnapshot, owner string, work *draftRevisionIntent) error {
	committed, found, err := r.findDraftRevision(ctx, s, work.Attempt, work)
	if err != nil {
		return err
	}
	if !found {
		current, err := versionReferences(s, objectID("message", work.Attempt.ID))
		if err != nil {
			return err
		}
		if !slices.Equal(current, work.Parents) {
			return ErrConflict
		}
		if work.Source == nil {
			return ErrDraftRevisionSource
		}
		if err := r.resumeCapturedDraft(ctx, s, owner, work); err != nil {
			return err
		}
		committed = work.Message
	} else {
		// A message index may be visible while its SDK/local final ACK is still
		// pending. Resume that exact operation before retiring the editor's capture.
		var pending ingestIntent
		exists, err := r.cfg.Journal.Read(owner, ingestNamespace, draftRevisionOperation(work.Attempt), &pending)
		clear(pending.Envelope)
		if err != nil {
			return err
		}
		if exists {
			if work.Source == nil {
				return ErrDraftRevisionSource
			}
			if err := r.resumeCapturedDraft(ctx, s, owner, work); err != nil {
				return err
			}
		}
	}
	work.Result = &committed
	return r.cfg.Journal.Write(owner, storage.MailDraftRevisionNamespace, work.Attempt.Editor, work)
}

func (r *Repository) resumeCapturedDraft(ctx context.Context, s storage.AtomicCatalogSnapshot, owner string, work *draftRevisionIntent) error {
	if work.Source == nil {
		return ErrDraftRevisionSource
	}
	source, err := r.cfg.Journal.OpenStagedFile(ctx, owner, storage.MailDraftRevisionStagingNamespace, *work.Source)
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
	return r.storeMessage(ctx, s, draftRevisionOperation(work.Attempt), ingestMessage{ID: work.Attempt.ID, MailboxID: work.Message.MailboxID, Size: work.Size}, work.Envelope, source, work.Size)
}

// findDraftRevision reconciles an ACK against authenticated history, including a
// superseded revision. Reconciliation never makes an old revision visible again.
func (r *Repository) findDraftRevision(ctx context.Context, s storage.AtomicCatalogSnapshot, attempt api.MailDraftAttempt, expected *draftRevisionIntent) (api.MailMessage, bool, error) {
	var result api.MailMessage
	found := false
	for _, version := range s.History {
		h, err := decodeHeader(version)
		if err != nil {
			return result, false, err
		}
		if h.Type != "message" || h.ID != attempt.ID || version.Commit.State != "live" {
			continue
		}
		raw, err := r.readDocument(ctx, version)
		if err != nil {
			return result, false, err
		}
		message, doc, err := decodeMessage(raw, attempt.ID)
		clear(raw)
		if err != nil {
			clear(doc.Envelope)
			return result, false, err
		}
		matching := doc.Operation == draftRevisionOperation(attempt)
		if matching && expected != nil {
			if !bytes.Equal(doc.Envelope, expected.Envelope) {
				clear(doc.Envelope)
				return result, false, ErrConflict
			}
			equal, err := r.sameDraftMIME(ctx, s, doc, expected.SHA256, attempt.ID)
			if err != nil || !equal {
				clear(doc.Envelope)
				if err != nil {
					return result, false, err
				}
				return result, false, ErrRecord
			}
		}
		clear(doc.Envelope)
		if !matching {
			continue
		}
		if found {
			return result, false, ErrConflict
		}
		result, found = message, true
	}
	return result, found, nil
}

// PendingDraftRevisions lists local admitted revisions without requiring the
// editor's old in-memory MIME or composing a replacement Message-ID.
func (r *Repository) PendingDraftRevisions(parent context.Context) ([]api.MailDraftAttempt, error) {
	var out []api.MailDraftAttempt
	err := r.run(parent, true, func(ctx context.Context, _ storage.AtomicCatalogSnapshot) error {
		values, err := r.cfg.Journal.List(r.draftRevisionOwner(), storage.MailDraftRevisionNamespace)
		if err != nil {
			return err
		}
		for _, record := range values {
			if err := ctx.Err(); err != nil {
				return err
			}
			var work draftRevisionIntent
			if json.Unmarshal(record.Value, &work) != nil || work.Attempt.Editor != record.ID {
				return ErrRecord
			}
			if err := r.validateDraftRevision(work); err != nil {
				return err
			}
			out = append(out, work.Attempt)
			clear(work.Envelope)
		}
		return nil
	})
	return out, err
}
