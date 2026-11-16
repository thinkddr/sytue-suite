//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

func TestPrivateMailNewAccountProvisioningIsAtomicMetadataOnlyAndIdempotent(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("mail-new-private")
	owner := "new-private-owner"
	epoch := uuid.New()
	var opens atomic.Int32
	open := func(context.Context, string) (io.ReadCloser, error) {
		opens.Add(1)
		return nil, errors.New("new metadata account must never open a MIME object")
	}
	for range 2 {
		state, err := st.PrivateStorageState(ctx, owner)
		if err != nil || state.Mode != "legacy" || state.Rows != 0 || state.Blobs != 0 {
			t.Fatal("state lookup manufactured a source", err)
		}
		if _, err := st.AccountBySubject(ctx, owner); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal("GET/state lookup provisioned a missing account", err)
		}
	}
	if _, err := st.BeginPrivateStorageMigration(ctx, owner, epoch, open); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("ordinary import turned absent account into an empty source", err)
	}
	failures := make(chan error, 5)
	var group sync.WaitGroup
	for range 5 {
		group.Go(func() {
			state, err := st.ProvisionPrivateStorage(ctx, owner, "new@sytue.eu", "", 1<<30, epoch, open)
			if err == nil && (state.Mode != "frozen" || state.Blobs != 0 || state.Rows != 7) {
				err = errors.New("new private account has incomplete metadata inventory")
			}
			failures <- err
		})
	}
	group.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal("concurrent explicit provision failed", err)
		}
	}
	if opens.Load() != 0 {
		t.Fatal("new account provisioning accessed MIME")
	}
	account, err := st.AccountBySubject(ctx, owner)
	if err != nil || account.Address != "new@sytue.eu" {
		t.Fatal("assigned mailbox identity changed", err)
	}
	repeated, err := st.ProvisionPrivateStorage(ctx, owner, "other@external.test", "changed", 1<<20, epoch, open)
	if err != nil || repeated.Rows != 7 {
		t.Fatal("accepted provisioning ACK could not reconcile", err)
	}
	account, err = st.AccountBySubject(ctx, owner)
	if err != nil || account.Address != "new@sytue.eu" || account.DisplayName != "" {
		t.Fatal("repeated provisioning rewrote original account metadata", err)
	}
	page, err := st.PrivateStorageSourcePage(ctx, owner, epoch, storage.LegacySourceCursor{}, storage.MaxLegacySourceRows)
	if err != nil || len(page.Rows) != 7 || page.Next != nil {
		t.Fatal("new account lost its complete normal source", err)
	}
	for _, row := range page.Rows {
		if row.Kind != "account" && row.Kind != "mailbox" {
			t.Fatal("fresh metadata source contains an unexpected legacy content row")
		}
	}
	if _, err := st.CreateMailbox(ctx, account.ID, nil, "server clear mailbox"); err == nil {
		t.Fatal("new account became visible without its frozen routing fence")
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var accounts, mailboxes, messages, queued int
		err := tx.QueryRow(ctx, "SELECT (SELECT count(*) FROM accounts WHERE tenant=$1),(SELECT count(*) FROM mailboxes WHERE tenant=$1),(SELECT count(*) FROM messages WHERE tenant=$1),(SELECT count(*) FROM outbound WHERE tenant=$1)", "mail-new-private").Scan(&accounts, &mailboxes, &messages, &queued)
		if err == nil && (accounts != 1 || mailboxes != 6 || messages != 0 || queued != 0) {
			return errors.New("new account left duplicate routing or clear queued content")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateMailProvisioningExistingAccountRetainsEveryOriginalAndRollsBackFailedCapture(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("mail-existing-private")
	account := seedAccount(ctx, t, st, "existing@sytue.eu")
	inbox, err := st.MailboxByRole(ctx, account.ID, domain.RoleInbox)
	if err != nil {
		t.Fatal(err)
	}
	original := seedMessage(ctx, t, st, account, inbox.ID, "exact retained message", "sender@external.test", nil)
	epoch := uuid.New()
	if _, err := st.ProvisionPrivateStorage(ctx, account.Subject, account.Address, "", 1<<30, epoch, func(context.Context, string) (io.ReadCloser, error) {
		return nil, errors.New("retained original temporarily inaccessible")
	}); err == nil {
		t.Fatal("missing retained MIME was silently treated as a new empty account")
	}
	state, err := st.PrivateStorageState(ctx, account.Subject)
	if err != nil || state.Mode != "legacy" || state.Rows != 0 || state.Blobs != 0 {
		t.Fatal("failed source capture committed a partial inventory", err)
	}
	raw := make([]byte, original.SizeBytes)
	state, err = st.ProvisionPrivateStorage(ctx, account.Subject, "replacement@external.test", "", 1<<30, epoch, mailSourceReader(raw))
	if err != nil || state.Mode != "frozen" || state.Blobs != 1 || state.Rows <= 7 {
		t.Fatal("existing account source was replaced by metadata-only provisioning", err)
	}
	page, err := st.PrivateStorageSourcePage(ctx, account.Subject, epoch, storage.LegacySourceCursor{}, storage.MaxLegacySourceRows)
	if err != nil {
		t.Fatal(err)
	}
	message, mime := false, false
	for _, row := range page.Rows {
		if row.ID == original.ID.String() {
			message = message || row.Kind == "message"
			mime = mime || row.Kind == "mime_message"
		}
	}
	if !message || !mime {
		t.Fatal("explicit first-use lost an existing message or its exact original")
	}
}
