// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func setupRetainedOutbound(t *testing.T, state, hold string, attempts *int) (*importFixture, []byte, time.Time) {
	t.Helper()
	f := setupImport(t)
	raw := revisionMIME(t, "Retained original with exact Message-ID")
	release := time.Now().UTC().Add(-time.Hour)
	put := func(kind, id string, raw []byte) {
		replaced := false
		row := storage.StreamSourceRow{Kind: kind, ID: id, Size: int64(len(raw)), SHA256: digest(raw)}
		for i := range f.source.rows {
			if f.source.rows[i].Kind == kind && f.source.rows[i].ID == id {
				f.source.rows[i] = row
				replaced = true
			}
		}
		if !replaced {
			f.source.rows = append(f.source.rows, row)
			if privateBody(row) {
				f.source.state.Blobs++
			} else {
				f.source.state.Rows++
			}
		}
		f.source.bodies[storage.LegacySourceCursor{Kind: kind, ID: id}] = raw
	}
	put("account", "account", []byte(`{"id":"account","address":"writing@example.test","signature":""}`))
	for _, role := range []string{"drafts", "sent", "scheduled"} {
		value, _ := json.Marshal(map[string]string{"id": role, "name": role, "role": role})
		put("mailbox", role, value)
	}
	metadata := map[string]any{"id": "held", "mail_from": "writing@example.test", "recipients": []string{"recipient@external.test"}, "state": state, "hold": hold, "next_retry_at": release}
	if attempts != nil {
		metadata["attempts"] = *attempts
	}
	value, _ := json.Marshal(metadata)
	put("outbound", "held", value)
	put("mime_outbound", "held", raw)
	slices.SortFunc(f.source.rows, func(a, b storage.StreamSourceRow) int {
		if n := strings.Compare(a.Kind, b.Kind); n != 0 {
			return n
		}
		return strings.Compare(a.ID, b.ID)
	})
	f.cfg.MigrationState = f.source.state
	f.repo.cfg.MigrationState = f.source.state
	if err := f.repo.ImportRetainedSource(t.Context(), f.source); err != nil {
		t.Fatal(err)
	}
	return f, raw, release
}

func TestPrivateMailRetainedOutboundNeedsFreshConsentAndKeepsStableExactMIME(t *testing.T) {
	zero := 0
	f, raw, release := setupRetainedOutbound(t, "queued", "scheduled", &zero)
	transport := &outboundTransportFixture{raw: raw, loseACK: true}
	f.cfg.Submission = transport
	f.repo.cfg.Submission = transport
	f.cfg.AuthorizeSubmission = func(context.Context) error { return nil }
	f.repo.cfg.AuthorizeSubmission = f.cfg.AuthorizeSubmission
	inventory, err := f.repo.RetainedOutbounds(t.Context())
	if err != nil || len(inventory) != 1 || !inventory[0].Eligible || !inventory[0].ReleaseAt.Equal(release) {
		t.Fatal("held original missing", err)
	}
	if _, err = f.repo.PrepareRetainedOutbound(t.Context(), "held", ""); !errors.Is(err, model.ErrSubmission) {
		t.Fatal("consent was inferred", err)
	}
	if pending, _ := f.repo.PendingOutbound(t.Context()); len(pending) != 0 || transport.sent != 0 {
		t.Fatal("listing/preconsent sent or staged external content")
	}
	attempt, err := f.repo.PrepareRetainedOutbound(t.Context(), "held", model.SMTPConsent)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.repo.PrepareRetainedOutbound(t.Context(), "held", model.SMTPConsent)
	if err != nil || again != attempt || attempt.Operation != inventory[0].Operation {
		t.Fatal("retry chose another operation", err)
	}
	if _, err = f.repo.CommitOutbound(t.Context(), attempt); err == nil || transport.sent != 1 {
		t.Fatal("lost ACK not exercised", err)
	}
	reopened := f.reopen(t)
	if err = reopened.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	status, err := reopened.CommitOutbound(t.Context(), attempt)
	if err != nil || !status.Receipt.Accepted() || transport.sent != 1 {
		t.Fatal("restoration resent accepted original", err)
	}
	sent, _, err := reopened.Messages(t.Context(), "sent", "", 20)
	if err != nil || len(sent) != 1 {
		t.Fatal("restored Sent missing", err)
	}
	body, err := reopened.Raw(t.Context(), sent[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(body)
	closeErr := body.Close()
	if err == nil {
		err = closeErr
	}
	defer clear(actual)
	if err != nil || !bytes.Equal(actual, raw) {
		t.Fatal("restoration changed original MIME", err)
	}
}

func TestPrivateMailRetainedOutboundNeverRequeuesAmbiguousOrAttemptedSources(t *testing.T) {
	zero, one := 0, 1
	for _, input := range []struct {
		name, state, hold string
		attempts          *int
	}{
		{"sending", "sending", "scheduled", &zero},
		{"already-sent", "sent", "scheduled", &zero},
		{"attempted", "queued", "scheduled", &one},
		{"immediate", "queued", "", &zero},
		{"missing-witness", "queued", "scheduled", nil},
	} {
		t.Run(input.name, func(t *testing.T) {
			f, _, _ := setupRetainedOutbound(t, input.state, input.hold, input.attempts)
			values, err := f.repo.RetainedOutbounds(t.Context())
			if err != nil || len(values) != 1 || values[0].Eligible {
				t.Fatal("unsafe source eligible", err)
			}
			if _, err = f.repo.PrepareRetainedOutbound(t.Context(), "held", model.SMTPConsent); !errors.Is(err, model.ErrSubmissionStarted) {
				t.Fatal("ambiguous source requeued", err)
			}
			if pending, _ := f.repo.PendingOutbound(t.Context()); len(pending) != 0 {
				t.Fatal("unsafe source staged")
			}
		})
	}
}
