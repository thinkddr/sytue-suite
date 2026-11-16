// SPDX-License-Identifier: Apache-2.0
//go:build integration

package delivery

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

type retryBeforeDATA struct {
	next  PrivateSender
	calls atomic.Int32
}

func (s *retryBeforeDATA) SendPrivate(ctx context.Context, from string, to []string, raw []byte) (bool, error) {
	if s.calls.Add(1) == 1 {
		return false, errors.New("synthetic connection refused before DATA")
	}
	return s.next.SendPrivate(ctx, from, to, raw)
}

func TestPrivateSubmissionActualPostgresTLSLostACKNoPlaintextPersistence(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: "private-outbound"})
	st := store.New(h.Pool)
	account, err := st.EnsureAccount(ctx, "private-owner", "owner@private.test", "", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	epoch := uuid.New()
	if _, err := st.BeginPrivateStorageMigration(ctx, account.Subject, epoch, func(context.Context, string) (io.ReadCloser, error) {
		return nil, errors.New("empty source tried to open plaintext storage")
	}); err != nil {
		t.Fatal(err)
	}
	raw := []byte("From: owner@private.test\r\nTo: recipient@external.test\r\nMessage-ID: <immutable-private-send@private.test>\r\nSubject: PRIVATE-SMTP-SUBJECT\r\n\r\nPRIVATE-SMTP-MIME-AND-ATTACHMENT\r\n")
	input := model.Submission{Operation: uuid.NewString(), Epoch: epoch.String(), From: account.Address, Recipients: []string{"recipient@external.test"}, Consent: model.SMTPConsent, Generation: 1, Control: strings.Repeat("a", 64)}
	fixture := newPrivateSMTPFixture(t, false, true)
	makeOutbound := func(sender PrivateSender) *Outbound {
		return NewOutbound(store.New(h.Pool), nil, nil, slog.New(slog.DiscardHandler), 1<<20, 0).WithPrivateSender(sender)
	}
	outbound := makeOutbound(fixture.sender)
	register := func(sender *Outbound, input model.Submission) {
		fingerprint, err := input.Fingerprint(raw)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sender.RegisterPrivateSubmission(ctx, account.Subject, model.SubmissionRegistration{Input: input, Fingerprint: fingerprint}); err != nil {
			t.Fatal(err)
		}
	}
	register(outbound, input)
	receipt, err := outbound.SubmitPrivate(ctx, account.Subject, input, raw)
	if err != nil || !receipt.Accepted() || fixture.acceptedCount() != 1 {
		t.Fatal("private delivery did not survive lost QUIT ACK", receipt.States, err)
	}
	if !bytes.Contains(raw, []byte("PRIVATE-SMTP-MIME")) {
		t.Fatal("delivery cleared the caller's original MIME")
	}
	// Simulate the lost HTTP ACK and a process restart. Repeating the exact
	// original request reads the durable receipt and emits no second DATA.
	for range 3 {
		repeated, err := makeOutbound(fixture.sender).SubmitPrivate(ctx, account.Subject, input, raw)
		if err != nil || !repeated.Accepted() || fixture.acceptedCount() != 1 {
			t.Fatal("HTTP retry duplicated accepted SMTP", fixture.acceptedCount(), err)
		}
	}
	changed := bytes.Replace(raw, []byte("PRIVATE-SMTP-MIME"), []byte("DIFFERENT-PRIVATE-MIME"), 1)
	if _, err := outbound.SubmitPrivate(ctx, account.Subject, input, changed); !errors.Is(err, model.ErrSubmissionConflict) {
		t.Fatal("same operation admitted replacement MIME", err)
	}
	concurrent := input
	concurrent.Operation = uuid.NewString()
	register(outbound, concurrent)
	var group sync.WaitGroup
	failures := make(chan error, 12)
	for range 12 {
		group.Go(func() {
			_, err := outbound.SubmitPrivate(ctx, account.Subject, concurrent, raw)
			if err != nil {
				failures <- err
			}
		})
	}
	group.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}
	if fixture.acceptedCount() != 2 {
		t.Fatal("concurrent requests duplicated the SMTP transaction", fixture.acceptedCount())
	}
	ambiguous := newPrivateSMTPFixture(t, true, false)
	uncertain := input
	uncertain.Operation = uuid.NewString()
	register(makeOutbound(ambiguous.sender), uncertain)
	receipt, err = makeOutbound(ambiguous.sender).SubmitPrivate(ctx, account.Subject, uncertain, raw)
	if err != nil || len(receipt.States) != 1 || receipt.States[0] != "uncertain" {
		t.Fatal("lost DATA ACK was not retained as uncertain", receipt.States, err)
	}
	for range 3 {
		repeated, err := makeOutbound(ambiguous.sender).SubmitPrivate(ctx, account.Subject, uncertain, raw)
		if err != nil || repeated.States[0] != "uncertain" || ambiguous.acceptedCount() != 1 {
			t.Fatal("uncertain SMTP was sent again after restart", ambiguous.acceptedCount(), err)
		}
	}
	retrySender := &retryBeforeDATA{next: fixture.sender}
	retry := input
	retry.Operation = uuid.NewString()
	register(makeOutbound(retrySender), retry)
	receipt, err = makeOutbound(retrySender).SubmitPrivate(ctx, account.Subject, retry, raw)
	if err != nil || receipt.States[0] != "ready" || fixture.acceptedCount() != 2 {
		t.Fatal("pre-DATA failure became ambiguous or delivered", err)
	}
	receipt, err = makeOutbound(retrySender).SubmitPrivate(ctx, account.Subject, retry, raw)
	if err != nil || !receipt.Accepted() || fixture.acceptedCount() != 3 {
		t.Fatal("safe retry did not deliver the original exactly once", err)
	}
	invalid := input
	invalid.Operation = uuid.NewString()
	invalid.Consent = ""
	if _, err := outbound.SubmitPrivate(ctx, account.Subject, invalid, raw); err == nil {
		t.Fatal("missing operation consent reached SMTP")
	}
	stale := input
	stale.Operation = uuid.NewString()
	stale.Epoch = uuid.NewString()
	if _, err := outbound.SubmitPrivate(ctx, account.Subject, stale, raw); !errors.Is(err, store.ErrPrivateStorageEpoch) {
		t.Fatal("old storage epoch reached SMTP", err)
	}
	var legacyMessages, legacyQueue, operations int
	var persisted string
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.private_mail_owner',$1,true)", account.Subject); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, "SELECT (SELECT count(*) FROM messages),(SELECT count(*) FROM outbound),(SELECT count(*) FROM mail_private_submissions)").Scan(&legacyMessages, &legacyQueue, &operations); err != nil {
			return err
		}
		return tx.QueryRow(ctx, "SELECT coalesce(string_agg(to_jsonb(s)::text,''),'')||(SELECT coalesce(string_agg(to_jsonb(r)::text,''),'') FROM mail_private_submission_recipients r) FROM mail_private_submissions s").Scan(&persisted)
	}); err != nil {
		t.Fatal(err)
	}
	if legacyMessages != 0 || legacyQueue != 0 || operations != 4 {
		t.Fatal("private sending wrote the legacy plaintext mailbox or queue", legacyMessages, legacyQueue, operations)
	}
	for _, marker := range []string{"PRIVATE-SMTP-SUBJECT", "PRIVATE-SMTP-MIME-AND-ATTACHMENT", "owner@private.test", "recipient@external.test", "immutable-private-send"} {
		if strings.Contains(persisted, marker) {
			t.Fatal("receipt persisted message contents or addresses")
		}
	}
	// A newer opaque E2E control generation fences a stale device. The relay
	// learns no schedule; only the commitment/version can change here.
	fenced := input
	fenced.Operation = uuid.NewString()
	register(outbound, fenced)
	fp, err := fenced.Fingerprint(raw)
	if err != nil {
		t.Fatal(err)
	}
	change := model.SubmissionControlChange{Epoch: fenced.Epoch, Operation: fenced.Operation, Fingerprint: fp, Generation: 1, Control: fenced.Control, NextControl: strings.Repeat("b", 64)}
	advanced, err := outbound.ChangePrivateSubmissionControl(ctx, account.Subject, change)
	if err != nil || advanced.Generation != 2 {
		t.Fatal("control generation did not advance", err)
	}
	if claimed, err := st.ClaimPrivateSubmission(ctx, account.Subject, fenced, 0); err != nil || claimed {
		t.Fatal("stale generation claimed SMTP after a concurrent encrypted schedule change", err)
	}
	if _, err := outbound.SubmitPrivate(ctx, account.Subject, fenced, raw); !errors.Is(err, model.ErrSubmissionControl) {
		t.Fatal("stale device bypassed a changed E2E schedule", err)
	}
	if fixture.acceptedCount() != 3 {
		t.Fatal("stale generation reached SMTP")
	}
	fresh := fenced
	fresh.Generation = 2
	fresh.Control = change.NextControl
	if receipt, err := outbound.SubmitPrivate(ctx, account.Subject, fresh, raw); err != nil || !receipt.Accepted() {
		t.Fatal("current E2E generation could not deliver", err)
	}
	if _, err := outbound.ChangePrivateSubmissionControl(ctx, account.Subject, model.SubmissionControlChange{Epoch: fenced.Epoch, Operation: fenced.Operation, Fingerprint: fp, Generation: 2, Control: fresh.Control, NextControl: strings.Repeat("c", 64), Cancel: true}); !errors.Is(err, model.ErrSubmissionStarted) {
		t.Fatal("accepted SMTP was falsely canceled", err)
	}
	for round := range 10 {
		racing := input
		racing.Operation = uuid.NewString()
		register(outbound, racing)
		fingerprint, err := racing.Fingerprint(raw)
		if err != nil {
			t.Fatal(err)
		}
		cancelRequest := model.SubmissionControlChange{Epoch: racing.Epoch, Operation: racing.Operation, Fingerprint: fingerprint, Generation: 1, Control: racing.Control, NextControl: strings.Repeat("d", 64), Cancel: true}
		before := fixture.acceptedCount()
		start := make(chan struct{})
		type sendResult struct {
			receipt model.SubmissionReceipt
			err     error
		}
		sent := make(chan sendResult, 1)
		canceled := make(chan sendResult, 1)
		go func() {
			<-start
			receipt, err := outbound.SubmitPrivate(ctx, account.Subject, racing, raw)
			sent <- sendResult{receipt, err}
		}()
		go func() {
			<-start
			receipt, err := outbound.ChangePrivateSubmissionControl(ctx, account.Subject, cancelRequest)
			canceled <- sendResult{receipt, err}
		}()
		close(start)
		delivery, cancelResult := <-sent, <-canceled
		delta := fixture.acceptedCount() - before
		if cancelResult.err == nil {
			if !cancelResult.receipt.Canceled || delta != 0 {
				t.Fatal("confirmed cancellation allowed SMTP DATA", round, delta)
			}
			replay, err := outbound.ChangePrivateSubmissionControl(ctx, account.Subject, cancelRequest)
			if err != nil || !replay.Canceled || replay.Generation != 2 {
				t.Fatal("lost cancellation ACK created another control generation", err)
			}
			if delivery.err != nil && !errors.Is(delivery.err, model.ErrSubmissionControl) {
				t.Fatal("unexpected canceled submission failure", delivery.err)
			}
		} else {
			if !errors.Is(cancelResult.err, model.ErrSubmissionStarted) || delivery.err != nil || !delivery.receipt.Accepted() || delta != 1 {
				t.Fatal("cancel/release race lost its durable winner", round, delta, cancelResult.err, delivery.err)
			}
		}
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var exposed int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name IN ('mail_private_submissions','mail_private_submission_recipients') AND column_name IN ('release_at','send_at','not_before','recipients','subject','raw','mime')").Scan(&exposed); err != nil {
			return err
		}
		if exposed != 0 {
			return errors.New("relay stores private outbox scheduling or contents")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Use a temporary table owner without BYPASSRLS. The ordinary app role has
	// no DDL privileges, so a permission error would not prove this guard.
	down, err := fs.ReadFile(db.Migrations, db.MigrationsDir+"/0033_private_outbound_control.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgx.Connect(t.Context(), h.DSN)
	if err != nil {
		t.Fatal("disposable migration connection failed")
	}
	defer admin.Close(t.Context())
	tx, err := admin.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	role := pgx.Identifier{"mail_control_guard_" + strings.ReplaceAll(uuid.NewString(), "-", "")}.Sanitize()
	for _, statement := range []string{
		"CREATE ROLE " + role + " NOLOGIN NOSUPERUSER NOBYPASSRLS",
		"GRANT USAGE,CREATE ON SCHEMA public TO " + role,
		"ALTER TABLE mail_private_submissions OWNER TO " + role,
		"ALTER TABLE mail_private_submission_recipients OWNER TO " + role,
		"SET LOCAL ROLE " + role,
	} {
		if _, err := tx.Exec(t.Context(), statement); err != nil {
			t.Fatal("cannot prepare the no-bypass migration owner")
		}
	}
	if _, err := tx.Exec(t.Context(), "SELECT set_config('app.tenant','unrelated-outbound',true)"); err != nil {
		t.Fatal(err)
	}
	var visible int
	if err := tx.QueryRow(t.Context(), "SELECT count(*) FROM mail_private_submissions").Scan(&visible); err != nil || visible != 0 {
		t.Fatal("temporary owner did not exercise RLS")
	}
	_, err = tx.Exec(t.Context(), string(down))
	var guard *pgconn.PgError
	if !errors.As(err, &guard) || guard.Code != "23514" || guard.ConstraintName != "mail_private_submission_downgrade_guard" {
		t.Fatal("downgrade did not validate live control generations behind RLS")
	}
}
