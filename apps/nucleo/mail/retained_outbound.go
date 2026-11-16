// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// RetainedOutbound retains the exact historical delivery state. Only a held,
// never-attempted item may be prepared for delivery after fresh SMTP consent.
// Sending/uncertain/failed/sent items remain exportable; none is auto-retried.
type RetainedOutbound struct {
	Source     storage.StreamSourceRow `json:"source"`
	Operation  string                  `json:"operation"`
	From       string                  `json:"from"`
	Recipients []string                `json:"recipients"`
	State      string                  `json:"state"`
	Hold       string                  `json:"hold"`
	ReleaseAt  time.Time               `json:"release_at"`
	Eligible   bool                    `json:"eligible"`
}
type retainedOutboundMetadata struct {
	ID         string    `json:"id"`
	From       string    `json:"mail_from"`
	Recipients []string  `json:"recipients"`
	State      string    `json:"state"`
	Hold       string    `json:"hold"`
	Attempts   *int      `json:"attempts"`
	ReleaseAt  time.Time `json:"next_retry_at"`
}

func (r *Repository) retainedOutboundOperation(id string) string {
	// UUIDv8 from SHA-256: a stable operation across devices, not key material.
	input, _ := json.Marshal([]string{"mail-retained-outbound-v1", r.cfg.Scope.Tenant, r.cfg.Scope.Subject, r.cfg.MigrationState.Epoch, id})
	sum := sha256.Sum256(input)
	clear(input)
	var operation uuid.UUID
	copy(operation[:], sum[:16])
	operation[6] = operation[6]&0x0f | 0x80
	operation[8] = operation[8]&0x3f | 0x80
	return operation.String()
}

func (r *Repository) retainedOutbound(ctx context.Context, row storage.StreamSourceRow) (RetainedOutbound, error) {
	value := RetainedOutbound{Source: row, Operation: r.retainedOutboundOperation(row.ID)}
	if row.Kind != "outbound" || row.Validate() != nil || row.Size > 1<<20 {
		return value, ErrRecord
	}
	reader, err := r.OpenRetainedSource(ctx, row)
	if err != nil {
		return value, err
	}
	raw, err := io.ReadAll(reader)
	closeErr := reader.Close()
	defer clear(raw)
	if err != nil {
		return value, err
	}
	if closeErr != nil {
		return value, closeErr
	}
	var metadata retainedOutboundMetadata
	if json.Unmarshal(raw, &metadata) != nil || metadata.ID != row.ID {
		return value, ErrRecord
	}
	value.From, value.Recipients, value.State, value.Hold, value.ReleaseAt = metadata.From, metadata.Recipients, metadata.State, metadata.Hold, metadata.ReleaseAt
	// Reuse the pure immutable submission validator for envelope syntax. This does
	// not persist, submit, or grant consent to any operation.
	envelope := model.Submission{Operation: value.Operation, Epoch: r.cfg.MigrationState.Epoch, From: value.From, Recipients: value.Recipients, Consent: model.SMTPConsent}
	value.Eligible = value.State == "queued" && (value.Hold == "scheduled" || value.Hold == "undo") && metadata.Attempts != nil && *metadata.Attempts == 0 && !value.ReleaseAt.IsZero() && envelope.Validate() == nil
	return value, nil
}

func (r *Repository) RetainedOutbounds(ctx context.Context) ([]RetainedOutbound, error) {
	output := []RetainedOutbound{}
	cursor := storage.LegacySourceCursor{}
	for {
		page, err := r.RetainedSourcePage(ctx, cursor, storage.MaxLegacySourceRows)
		if err != nil {
			return nil, err
		}
		for _, row := range page.Rows {
			if row.Kind == "outbound" {
				value, err := r.retainedOutbound(ctx, row)
				if err != nil {
					return nil, err
				}
				output = append(output, value)
			}
		}
		if page.Next == nil {
			return output, nil
		}
		cursor = *page.Next
	}
}

func (r *Repository) retainedOutboundPair(ctx context.Context, id string) (RetainedOutbound, storage.StreamSourceRow, error) {
	var outbound, mime storage.StreamSourceRow
	cursor := storage.LegacySourceCursor{}
	for {
		page, err := r.RetainedSourcePage(ctx, cursor, storage.MaxLegacySourceRows)
		if err != nil {
			return RetainedOutbound{}, mime, err
		}
		for _, row := range page.Rows {
			if row.ID == id {
				switch row.Kind {
				case "outbound":
					outbound = row
				case "mime_outbound":
					mime = row
				}
			}
		}
		if page.Next == nil {
			break
		}
		cursor = *page.Next
	}
	if outbound.Kind == "" || mime.Kind == "" {
		return RetainedOutbound{}, mime, ErrNotFound
	}
	value, err := r.retainedOutbound(ctx, outbound)
	return value, mime, err
}

// PrepareRetainedOutbound is an explicit new consent action. It preserves the
// original RFC822 bytes and release instant; its stable operation prevents a
// second logical delivery after reload or adoption by a different device.
func (r *Repository) PrepareRetainedOutbound(ctx context.Context, id, consent string) (api.MailOutgoingAttempt, error) {
	var attempt api.MailOutgoingAttempt
	if consent != model.SMTPConsent {
		return attempt, model.ErrSubmission
	}
	value, mime, err := r.retainedOutboundPair(ctx, id)
	if err != nil {
		return attempt, err
	}
	if !value.Eligible {
		return attempt, model.ErrSubmissionStarted
	}
	if mime.Size > r.messageLimit() {
		return attempt, api.ErrMailTooLarge
	}
	reader, err := r.OpenRetainedSource(ctx, mime)
	if err != nil {
		return attempt, err
	}
	raw, err := io.ReadAll(reader)
	closeErr := reader.Close()
	defer clear(raw)
	if err != nil {
		return attempt, err
	}
	if closeErr != nil {
		return attempt, closeErr
	}
	return r.PrepareOutbound(ctx, api.MailOutgoing{Operation: value.Operation, From: value.From, Recipients: value.Recipients, MIME: raw, Consent: consent, ReleaseAt: value.ReleaseAt})
}
