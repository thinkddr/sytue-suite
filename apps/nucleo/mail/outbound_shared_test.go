// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

func freshOutboundRepository(t *testing.T, f *importFixture, transport *outboundTransportFixture) (*Repository, *cryptoFixture) {
	t.Helper()
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	clone := *f.crypto
	clone.binding.Target.DeviceID = "MAIL-FRESH"
	cfg := f.cfg
	cfg.Journal = journal
	cfg.Crypto = &clone
	cfg.Files = nil
	cfg.Submission = transport
	cfg.AuthorizeSubmission = func(context.Context) error { return nil }
	repository, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(repository.Close)
	if err := repository.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return repository, &clone
}

func TestSharedPrivateOutboundEmptyJournalAdoptsExactMIMEAndNewerEncryptedSchedule(t *testing.T) {
	f := setupDrafts(t, "sent", "scheduled")
	raw := revisionMIME(t, "Shared outgoing original")
	transport := &outboundTransportFixture{raw: raw}
	f.repo.cfg.Submission = transport
	f.repo.cfg.AuthorizeSubmission = func(context.Context) error { return nil }
	input := api.MailOutgoing{Operation: uuid.NewString(), From: "writing@example.test", Recipients: []string{"recipient@external.test"}, MIME: raw, Consent: model.SMTPConsent, ReleaseAt: time.Now().Add(10 * time.Minute)}
	attempt, err := f.repo.PrepareOutbound(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if status, err := f.repo.CommitOutbound(t.Context(), attempt); err != nil || !status.Queued {
		t.Fatal("producer did not publish its encrypted job", err)
	}
	fresh, _ := freshOutboundRepository(t, f, transport)
	if pending, err := fresh.PendingOutbound(t.Context()); err != nil || len(pending) != 0 {
		t.Fatal("fresh fixture did not start with an empty journal", err)
	}
	if err := fresh.DiscoverOutbound(t.Context()); err != nil {
		t.Fatal(err)
	}
	adopted, err := fresh.loadOutbound(attempt.Operation)
	if err != nil || adopted.Source != nil || !adopted.MIME.Valid() {
		t.Fatal("fresh device downloaded MIME before the job was due", err)
	}
	later := time.Now().Add(time.Hour)
	if err := fresh.Reschedule(t.Context(), attempt.Operation, later); err != nil {
		t.Fatal(err)
	}
	// A keeps a stale local schedule. It must read the newer authenticated E2E
	// job instead of merely advancing a generation from the relay's receipt.
	stale, err := f.repo.loadOutbound(attempt.Operation)
	if err != nil {
		t.Fatal(err)
	}
	stale.ReleaseAt = time.Now().Add(-time.Hour)
	if err := f.repo.saveOutbound(&stale); err != nil {
		t.Fatal(err)
	}
	if status, err := f.repo.CommitOutbound(t.Context(), attempt); err != nil || !status.Queued || !status.ReleaseAt.Equal(later) || transport.sent != 0 {
		t.Fatal("stale local schedule escaped its E2E generation", status.ReleaseAt, transport.sent, err)
	}
	due := time.Now().Add(20 * time.Millisecond)
	if err := fresh.Reschedule(t.Context(), attempt.Operation, due); err != nil {
		t.Fatal(err)
	}
	if delay := time.Until(due); delay > 0 {
		time.Sleep(delay + time.Millisecond)
	}
	result, err := fresh.CommitOutbound(t.Context(), attempt)
	if err != nil || !result.Receipt.Accepted() || transport.sent != 1 {
		t.Fatal("fresh device did not deliver the original exactly once", transport.sent, err)
	}
	sent, _, err := fresh.Messages(t.Context(), "sent", "", 20)
	if err != nil || len(sent) != 1 {
		t.Fatal("accepted shared message did not reach encrypted Sent", len(sent), err)
	}
	body, err := fresh.Raw(t.Context(), sent[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(body)
	closeErr := body.Close()
	defer clear(actual)
	if err != nil || closeErr != nil || !bytes.Equal(actual, raw) {
		t.Fatal("fresh device recomposed MIME or Message-ID", err, closeErr)
	}
	if status, err := f.repo.CommitOutbound(t.Context(), attempt); err != nil || !status.Receipt.Accepted() || transport.sent != 1 {
		t.Fatal("producer resent a job completed by another device", transport.sent, err)
	}
}

func TestSharedPrivateOutboundFreshDeviceCancelsWithoutDownloadingOriginal(t *testing.T) {
	f := setupDrafts(t, "sent", "scheduled")
	raw := revisionMIME(t, "Shared canceled original")
	transport := &outboundTransportFixture{raw: raw}
	f.repo.cfg.Submission = transport
	f.repo.cfg.AuthorizeSubmission = func(context.Context) error { return nil }
	input := api.MailOutgoing{Operation: uuid.NewString(), From: "writing@example.test", Recipients: []string{"recipient@external.test"}, MIME: raw, Consent: model.SMTPConsent, ReleaseAt: time.Now().Add(time.Hour)}
	attempt, err := f.repo.PrepareOutbound(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.CommitOutbound(t.Context(), attempt); err != nil {
		t.Fatal(err)
	}
	fresh, _ := freshOutboundRepository(t, f, transport)
	if err := fresh.DiscoverOutbound(t.Context()); err != nil {
		t.Fatal(err)
	}
	before, err := fresh.loadOutbound(attempt.Operation)
	if err != nil || before.Source != nil {
		t.Fatal("cancel fixture fetched the private MIME", err)
	}
	if err := fresh.CancelPrivateOutbound(t.Context(), attempt); err != nil {
		t.Fatal(err)
	}
	if transport.sent != 0 || !transport.receipt.Canceled {
		t.Fatal("shared cancellation reached SMTP")
	}
	if scheduled, err := f.repo.Scheduled(t.Context()); err != nil || len(scheduled) != 0 {
		t.Fatal("producer kept a schedule canceled by another device", len(scheduled), err)
	}
	messages, _, err := fresh.Messages(t.Context(), "drafts", "", 20)
	if err != nil || len(messages) != 1 || !messages[0].Has("\\Draft") {
		t.Fatal("canceled shared original is not an editable draft", err)
	}
	if err := fresh.CancelPrivateOutbound(t.Context(), attempt); err != nil {
		t.Fatal("lost local cancel callback did not reconcile", err)
	}
	status, err := f.repo.CommitOutbound(t.Context(), attempt)
	if err != nil || !status.Receipt.Canceled || transport.sent != 0 {
		t.Fatal("stale producer ignored durable cancellation", transport.sent, err)
	}
}

type lostSharedControlACK struct {
	*outboundTransportFixture
	lose bool
}

func (f *lostSharedControlACK) Control(ctx context.Context, change model.SubmissionControlChange) (model.SubmissionReceipt, error) {
	receipt, err := f.outboundTransportFixture.Control(ctx, change)
	if err == nil && f.lose {
		f.lose = false
		return model.SubmissionReceipt{}, errors.New("synthetic accepted generation ACK lost")
	}
	return receipt, err
}

func TestSharedPrivateOutboundFreshDeviceCompletesEncryptedProposalAfterLostControlACK(t *testing.T) {
	f := setupDrafts(t, "sent", "scheduled")
	raw := revisionMIME(t, "Shared proposal original")
	base := &outboundTransportFixture{raw: raw}
	transport := &lostSharedControlACK{outboundTransportFixture: base}
	f.repo.cfg.Submission = transport
	f.repo.cfg.AuthorizeSubmission = func(context.Context) error { return nil }
	input := api.MailOutgoing{Operation: uuid.NewString(), From: "writing@example.test", Recipients: []string{"recipient@external.test"}, MIME: raw, Consent: model.SMTPConsent, ReleaseAt: time.Now().Add(time.Hour)}
	attempt, err := f.repo.PrepareOutbound(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.CommitOutbound(t.Context(), attempt); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * time.Hour)
	transport.lose = true
	if err := f.repo.Reschedule(t.Context(), attempt.Operation, later); err == nil {
		t.Fatal("accepted generation ACK was not lost")
	}
	if base.receipt.Generation != 2 || base.sent != 0 {
		t.Fatal("opaque generation did not advance without SMTP")
	}
	fresh, _ := freshOutboundRepository(t, f, base)
	if err := fresh.DiscoverOutbound(t.Context()); err != nil {
		t.Fatal("fresh device could not replay encrypted proposal", err)
	}
	adopted, err := fresh.loadOutbound(attempt.Operation)
	if err != nil || adopted.Input.Generation != 2 || adopted.Proposal != nil || !adopted.ReleaseAt.Equal(later) || adopted.Source != nil {
		t.Fatal("fresh device invented schedule or downloaded MIME", err)
	}
	if status, err := fresh.CommitOutbound(t.Context(), attempt); err != nil || !status.Queued || base.sent != 0 {
		t.Fatal("recovered future schedule escaped to SMTP", err)
	}
}

func TestPrivateOutboundProvisionsMissingPrivateSystemMailboxesWithoutConfiguration(t *testing.T) {
	f := setupDrafts(t)
	raw := revisionMIME(t, "Existing account first private send")
	transport := &outboundTransportFixture{raw: raw}
	f.repo.cfg.Submission = transport
	f.repo.cfg.AuthorizeSubmission = func(context.Context) error { return nil }
	input := api.MailOutgoing{Operation: uuid.NewString(), From: "writing@example.test", Recipients: []string{"recipient@external.test"}, MIME: raw, Consent: model.SMTPConsent, ReleaseAt: time.Now().Add(time.Hour)}
	attempt, err := f.repo.PrepareOutbound(t.Context(), input)
	if err != nil {
		t.Fatal("historical account required manual private folder setup", err)
	}
	if status, err := f.repo.CommitOutbound(t.Context(), attempt); err != nil || !status.Queued {
		t.Fatal("first private send did not queue", err)
	}
	mailboxes, err := f.repo.Mailboxes(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string]string{}
	for _, box := range mailboxes {
		if box.Role != "" {
			if roles[box.Role] != "" {
				t.Fatal("private folder provisioning duplicated a role")
			}
			roles[box.Role] = box.ID
		}
	}
	if roles["drafts"] != "drafts" || roles["sent"] == "" || roles["scheduled"] == "" {
		t.Fatal("private folder provisioning replaced or omitted a system role", roles)
	}
	input.Operation = uuid.NewString()
	if _, err := f.repo.PrepareOutbound(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	after, err := f.repo.Mailboxes(t.Context())
	if err != nil || len(after) != len(mailboxes) {
		t.Fatal("later private send duplicated system folders", err)
	}
}
