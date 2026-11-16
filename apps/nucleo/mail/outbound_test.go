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
	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
)

type outboundTransportFixture struct {
	raw           []byte
	sent, queries int
	loseACK       bool
	state         string
	receipt       model.SubmissionReceipt
}

func (f *outboundTransportFixture) Submit(_ context.Context, input model.Submission, source io.Reader, size int64) (model.SubmissionReceipt, error) {
	raw, err := io.ReadAll(source)
	if err != nil {
		return model.SubmissionReceipt{}, err
	}
	defer clear(raw)
	if int64(len(raw)) != size || !bytes.Equal(raw, f.raw) {
		return model.SubmissionReceipt{}, errors.New("submission recaptured a different MIME")
	}
	if f.receipt.Canceled || input.Generation != f.receipt.Generation || input.Control != f.receipt.Control {
		return model.SubmissionReceipt{}, model.ErrSubmissionControl
	}
	f.sent++
	fingerprint, err := input.Fingerprint(raw)
	if err != nil {
		return model.SubmissionReceipt{}, err
	}
	state := f.state
	if state == "" {
		state = "accepted"
	}
	states := make([]string, len(input.Recipients))
	for i := range states {
		states[i] = state
	}
	f.receipt = model.SubmissionReceipt{Operation: input.Operation, Fingerprint: fingerprint, States: states, Generation: input.Generation, Control: input.Control}
	if f.loseACK {
		f.loseACK = false
		return model.SubmissionReceipt{}, errors.New("synthetic lost HTTP ACK after durable SMTP acceptance")
	}
	return f.receipt, nil
}

func (f *outboundTransportFixture) Receipt(context.Context, string) (model.SubmissionReceipt, error) {
	f.queries++
	return f.receipt, nil
}

func TestPrivateOutboundLostHTTPACKRetainsOriginalThenRecoversSentWithoutSecondSMTP(t *testing.T) {
	f := setupDrafts(t, "sent", "scheduled")
	raw := revisionMIME(t, "Private outgoing original")
	transport := &outboundTransportFixture{raw: raw, loseACK: true}
	f.repo.cfg.Submission = transport
	f.cfg.Submission = transport
	f.repo.cfg.AuthorizeSubmission = func(context.Context) error { return nil }
	f.cfg.AuthorizeSubmission = f.repo.cfg.AuthorizeSubmission
	input := api.MailOutgoing{Operation: uuid.NewString(), From: "writing@example.test", Recipients: []string{"recipient@external.test"}, MIME: raw, Consent: model.SMTPConsent}
	attempt, err := f.repo.PrepareOutbound(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.CommitOutbound(t.Context(), attempt); err == nil || transport.sent != 1 {
		t.Fatal("lost external ACK was not exercised", transport.sent, err)
	}
	pending, err := f.repo.PendingOutbound(t.Context())
	if err != nil || len(pending) != 1 {
		t.Fatal("uncertain transport ACK discarded the encrypted original", len(pending), err)
	}
	changed := input
	changed.MIME = revisionMIME(t, "Later edit")
	if _, err := f.repo.PrepareOutbound(t.Context(), changed); !errors.Is(err, ErrConflict) {
		t.Fatal("same operation replaced its admitted original", err)
	}
	reopened := f.reopen(t)
	if err := reopened.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	result, err := reopened.CommitOutbound(t.Context(), attempt)
	if err != nil || !result.Receipt.Accepted() || transport.sent != 1 || transport.queries < 2 {
		t.Fatal("recovery retransmitted accepted SMTP", transport.sent, transport.queries, err)
	}
	messages, _, err := reopened.Messages(t.Context(), "sent", "", 20)
	if err != nil || len(messages) != 1 {
		t.Fatal("accepted message did not appear in encrypted Sent", len(messages), err)
	}
	body, err := reopened.Raw(t.Context(), messages[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(body)
	closeErr := body.Close()
	defer clear(actual)
	if err != nil || closeErr != nil || !bytes.Equal(actual, raw) {
		t.Fatal("Sent changed original MIME or Message-ID", err, closeErr)
	}
	if pending, err := reopened.PendingOutbound(t.Context()); err != nil || len(pending) != 0 {
		t.Fatal("completed operation remained pending", len(pending), err)
	}
	again, err := reopened.CommitOutbound(t.Context(), attempt)
	if err != nil || !again.Receipt.Accepted() || transport.sent != 1 {
		t.Fatal("lost local callback could not recover its completed ACK", err)
	}
}

func TestPrivateOutboundUncertainSMTPIsNeverReplacedOrAutomaticallyResent(t *testing.T) {
	f := setupDrafts(t, "sent", "scheduled")
	raw := revisionMIME(t, "Uncertain outgoing original")
	transport := &outboundTransportFixture{raw: raw, state: "uncertain"}
	f.repo.cfg.Submission = transport
	f.repo.cfg.AuthorizeSubmission = func(context.Context) error { return nil }
	input := api.MailOutgoing{Operation: uuid.NewString(), From: "writing@example.test", Recipients: []string{"recipient@external.test"}, MIME: raw, Consent: model.SMTPConsent}
	attempt, err := f.repo.PrepareOutbound(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		result, err := f.repo.CommitOutbound(t.Context(), attempt)
		if err != nil || result.Receipt.Accepted() {
			t.Fatal("uncertain delivery became accepted", err)
		}
	}
	if transport.sent != 1 {
		t.Fatal("uncertain SMTP was resent", transport.sent)
	}
	if err := f.repo.CancelPrivateOutbound(t.Context(), attempt); !errors.Is(err, api.ErrDemasiadoTarde) {
		t.Fatal("uncertain external attempt was erased", err)
	}
	pending, err := f.repo.PendingOutbound(t.Context())
	if err != nil || len(pending) != 1 {
		t.Fatal("uncertain source was not retained", len(pending), err)
	}
}

func TestPrivateOutboundScheduleAndCancelKeepExactEncryptedMessageWithoutSMTP(t *testing.T) {
	f := setupDrafts(t, "sent", "scheduled")
	raw := revisionMIME(t, "Scheduled outgoing original")
	transport := &outboundTransportFixture{raw: raw}
	f.repo.cfg.Submission = transport
	f.repo.cfg.AuthorizeSubmission = func(context.Context) error { return nil }
	input := api.MailOutgoing{Operation: uuid.NewString(), From: "writing@example.test", Recipients: []string{"recipient@external.test"}, MIME: raw, Consent: model.SMTPConsent, ReleaseAt: time.Now().Add(time.Hour)}
	attempt, err := f.repo.PrepareOutbound(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.repo.CommitOutbound(t.Context(), attempt)
	if err != nil || !result.Queued || transport.sent != 0 {
		t.Fatal("scheduled original left before its release time", err)
	}
	if err := f.repo.CancelPrivateOutbound(t.Context(), attempt); err != nil {
		t.Fatal(err)
	}
	messages, _, err := f.repo.Messages(t.Context(), "drafts", "", 20)
	if err != nil || len(messages) != 1 {
		t.Fatal("cancel erased the original queued message", len(messages), err)
	}
	body, err := f.repo.Raw(t.Context(), messages[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(body)
	closeErr := body.Close()
	defer clear(actual)
	if err != nil || closeErr != nil || !bytes.Equal(actual, raw) || transport.sent != 0 {
		t.Fatal("cancel changed MIME or sent it externally", err, closeErr)
	}
}

func TestPrivateOutboundDefiniteRejectionCanRetireWithoutLosingOriginal(t *testing.T) {
	f := setupDrafts(t, "sent", "scheduled")
	raw := revisionMIME(t, "Rejected outgoing original")
	transport := &outboundTransportFixture{raw: raw, state: "failed"}
	f.repo.cfg.Submission = transport
	f.repo.cfg.AuthorizeSubmission = func(context.Context) error { return nil }
	input := api.MailOutgoing{Operation: uuid.NewString(), From: "writing@example.test", Recipients: []string{"recipient@external.test"}, MIME: raw, Consent: model.SMTPConsent}
	attempt, err := f.repo.PrepareOutbound(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.repo.CommitOutbound(t.Context(), attempt)
	if err != nil || !result.Receipt.Failed() {
		t.Fatal("definite rejection was not kept separate from uncertainty", err)
	}
	if err := f.repo.CancelPrivateOutbound(t.Context(), attempt); err != nil {
		t.Fatal("definite rejection could not be retired safely", err)
	}
	messages, _, err := f.repo.Messages(t.Context(), "drafts", "", 20)
	if err != nil || len(messages) != 1 || transport.sent != 1 {
		t.Fatal("definite rejection erased or resent its original", len(messages), err)
	}
}

func TestPrivateOutboundRevalidatesAuthorityBeforeDisclosingAnyMIME(t *testing.T) {
	f := setupDrafts(t, "sent", "scheduled")
	raw := revisionMIME(t, "Revoked outgoing original")
	transport := &outboundTransportFixture{raw: raw}
	f.repo.cfg.Submission = transport
	revoked := errors.New("synthetic retired account authority")
	f.repo.cfg.AuthorizeSubmission = func(context.Context) error { return revoked }
	input := api.MailOutgoing{Operation: uuid.NewString(), From: "writing@example.test", Recipients: []string{"recipient@external.test"}, MIME: raw, Consent: model.SMTPConsent}
	attempt, err := f.repo.PrepareOutbound(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.CommitOutbound(t.Context(), attempt); !errors.Is(err, revoked) || transport.sent != 0 || transport.queries != 0 {
		t.Fatal("revoked account disclosed MIME to SMTP", err)
	}
	pending, err := f.repo.PendingOutbound(t.Context())
	if err != nil || len(pending) != 1 {
		t.Fatal("revocation erased the encrypted original", len(pending), err)
	}
}

func (f *outboundTransportFixture) Register(_ context.Context, registration model.SubmissionRegistration) (model.SubmissionReceipt, error) {
	input := registration.Input
	if f.receipt.Operation == input.Operation {
		return f.receipt, nil
	}
	states := make([]string, len(input.Recipients))
	for i := range states {
		states[i] = "ready"
	}
	f.receipt = model.SubmissionReceipt{Operation: input.Operation, Fingerprint: registration.Fingerprint, States: states, Generation: input.Generation, Control: input.Control}
	return f.receipt, nil
}

func (f *outboundTransportFixture) Control(_ context.Context, change model.SubmissionControlChange) (model.SubmissionReceipt, error) {
	if f.receipt.Generation == change.Generation+1 && f.receipt.Control == change.NextControl && f.receipt.Canceled == change.Cancel {
		return f.receipt, nil
	}
	if f.receipt.Fingerprint != change.Fingerprint || f.receipt.Generation != change.Generation || f.receipt.Control != change.Control {
		return model.SubmissionReceipt{}, model.ErrSubmissionControl
	}
	for _, state := range f.receipt.States {
		if state != "ready" && state != "failed" {
			return model.SubmissionReceipt{}, model.ErrSubmissionStarted
		}
	}
	f.receipt.Generation++
	f.receipt.Control = change.NextControl
	f.receipt.Canceled = change.Cancel
	if change.Cancel {
		for i := range f.receipt.States {
			f.receipt.States[i] = "canceled"
		}
	}
	return f.receipt, nil
}
