// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

func mailSourceReader(raw []byte) func(context.Context, string) (io.ReadCloser, error) {
	return func(context.Context, string) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(raw)), nil }
}

func TestPrivateMailSourceCompleteAndFenced(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("mail-private")
	acc := seedAccount(ctx, t, st, "source@sytue.eu")
	other := seedAccount(ctx, t, st, "other@sytue.eu")
	inbox, err := st.MailboxByRole(ctx, acc.ID, domain.RoleInbox)
	if err != nil {
		t.Fatal(err)
	}
	raw := bytes.Repeat([]byte("x"), 100)
	live := seedMessage(ctx, t, st, acc, inbox.ID, "alive", "alice@example.test", nil)
	deleted := seedMessage(ctx, t, st, acc, inbox.ID, "deleted", "alice@example.test", nil)
	if err := st.DeleteMessage(ctx, acc.ID, deleted.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateLabel(ctx, acc.ID, "Work", "blue"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSignature(ctx, acc.ID, "private signature"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetVacation(ctx, acc.ID, domain.Vacation{Enabled: true, Subject: "Away", Body: "private vacation"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ClaimAutoreply(ctx, acc.ID, "sender@example.test", time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateFilter(ctx, acc.ID, domain.Filter{Name: "Rule", MatchSubject: "private", MarkRead: true}); err != nil {
		t.Fatal(err)
	}
	o := domain.OutboundMessage{AccountID: acc.ID, From: acc.Address, Recipients: []string{"bob@example.test"}, BlobKey: "outgoing/original.eml", Subject: "scheduled"}
	if _, err := st.EnqueueHeld(ctx, &o, time.Now().Add(-time.Hour), "scheduled"); err != nil {
		t.Fatal(err)
	}
	epoch := uuid.New()
	frozen, err := st.BeginPrivateStorageMigration(ctx, acc.Subject, epoch, mailSourceReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if frozen.Mode != "frozen" || frozen.Blobs != 3 || frozen.Validate(storage.Scope{Tenant: "mail-private", Subject: acc.Subject, Product: "mail"}) != nil {
		t.Fatalf("state=%+v", frozen)
	}
	repeat, err := st.BeginPrivateStorageMigration(ctx, acc.Subject, epoch, func(context.Context, string) (io.ReadCloser, error) { return nil, errors.New("retry reopened source") })
	if err != nil || !reflect.DeepEqual(repeat, frozen) {
		t.Fatalf("repeat=%+v %v", repeat, err)
	}
	after := storage.LegacySourceCursor{}
	kinds := map[string]int{}
	var rowCount int64
	for {
		page, err := st.PrivateStorageSourcePage(ctx, acc.Subject, epoch, after, 2)
		if err != nil {
			t.Fatal(err)
		}
		if page.Validate(frozen.Scope, epoch.String(), after) != nil {
			t.Fatalf("invalid page %+v", page)
		}
		for _, row := range page.Rows {
			body, err := st.PrivateStorageSourceBody(ctx, acc.Subject, epoch, row.Kind, row.ID)
			if err != nil {
				t.Fatal(err)
			}
			if body.Descriptor != row {
				t.Fatal("descriptor changed")
			}
			value := body.Payload
			if body.StorageKey != "" {
				value = raw
			}
			sum := sha256.Sum256(value)
			if int64(len(value)) != row.Size || hex.EncodeToString(sum[:]) != row.SHA256 {
				t.Fatalf("wrong bytes %s/%s", row.Kind, row.ID)
			}
			if body.StorageKey == "" && (!json.Valid(value) || bytes.Contains(value, []byte("blob_key")) || bytes.Contains(value, []byte("storage_key"))) {
				t.Fatal("invalid or leaking source JSON")
			}
			if row.Kind == "message" && row.ID == deleted.ID.String() && !bytes.Contains(value, []byte(`"deleted_at":`)) {
				t.Fatal("lost deleted marker")
			}
			kinds[row.Kind]++
			rowCount++
		}
		if page.Next == nil {
			break
		}
		after = *page.Next
	}
	if rowCount != frozen.Rows+frozen.Blobs || kinds["message"] != 2 || kinds["account"] != 1 || kinds["mailbox"] != 6 || kinds["label"] != 1 || kinds["filter"] != 1 || kinds["vacation"] != 1 || kinds["vacation_log"] != 1 || kinds["outbound"] != 1 {
		t.Fatalf("incomplete snapshot rows=%d state=%+v kinds=%v", rowCount, frozen, kinds)
	}
	for name, write := range map[string]func() error{
		"flags":     func() error { return st.SetFlags(ctx, acc.ID, live.ID, domain.FlagSeen, true) },
		"signature": func() error { return st.SetSignature(ctx, acc.ID, "changed") },
		"folder":    func() error { _, err := st.CreateMailbox(ctx, acc.ID, nil, "blocked"); return err },
		"vacation":  func() error { return st.SetVacation(ctx, acc.ID, domain.Vacation{}) },
		"enqueue": func() error {
			return st.Enqueue(ctx, &domain.OutboundMessage{AccountID: acc.ID, From: acc.Address, Recipients: []string{"a@example.test"}, BlobKey: "new.eml"})
		},
		"insert": func() error {
			return st.InsertMessage(ctx, &domain.Message{AccountID: acc.ID, MailboxID: inbox.ID, BlobKey: "new.eml"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := store.PrivateStorageError(write()); !errors.Is(err, store.ErrPrivateStorageFrozen) {
				t.Fatalf("write escaped fence: %v", err)
			}
		})
	}
	if _, err := st.PrivateStorageSourcePage(ctx, other.Subject, epoch, storage.LegacySourceCursor{}, 2); !errors.Is(err, store.ErrPrivateStorageEpoch) {
		t.Fatalf("other owner read source: %v", err)
	}
	if _, err := st.PrivateStorageSourcePage(tenantCtx("different"), acc.Subject, epoch, storage.LegacySourceCursor{}, 2); !errors.Is(err, store.ErrPrivateStorageEpoch) {
		t.Fatalf("cross-tenant read source: %v", err)
	}
	if _, err := st.BeginPrivateStorageMigration(ctx, acc.Subject, uuid.New(), mailSourceReader(raw)); !errors.Is(err, store.ErrPrivateStorageEpoch) {
		t.Fatalf("replaced epoch: %v", err)
	}
	if _, err := st.ClaimDue(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if n, err := st.PromoteDueHeld(context.Background(), time.Now(), 10, func(context.Context, string, string) ([]byte, error) {
		return nil, errors.New("frozen scheduled raw opened")
	}); err != nil || n != 0 {
		t.Fatalf("released frozen scheduled: %d %v", n, err)
	}
	if n, keys, err := st.PurgeDeleted(ctx, time.Now().Add(time.Hour)); err != nil || n != 0 || len(keys) != 0 {
		t.Fatalf("purged retained original: %d %v %v", n, keys, err)
	}
	if _, err := st.EDiscoverySearch(ctx, acc.ID, store.EDiscoveryFilter{}); !errors.Is(err, store.ErrPrivateStorageFrozen) {
		t.Fatal("admin legacy search reopened frozen personal content", err)
	}
	for _, query := range []string{`UPDATE mail_private_storage_states SET source_rows=source_rows+1 WHERE tenant='mail-private'`, `UPDATE mail_private_storage_states SET mode='legacy',epoch=NULL,started_at=NULL WHERE tenant='mail-private'`} {
		err := h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error { _, err := tx.Exec(ctx, query); return err })
		if !errors.Is(store.PrivateStorageError(err), store.ErrPrivateStorageFrozen) {
			t.Fatal("changed frozen source identity", err)
		}
	}
	// The snapshot itself has no UPDATE/DELETE policy, including maintenance.
	if err := h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM mail_private_storage_source WHERE tenant=$1`, "mail-private")
		if tag.RowsAffected() != 0 {
			return fmt.Errorf("maintenance deleted retained source: %d", tag.RowsAffected())
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateMailSourceAbortAndInflight(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("mail-abort")
	acc := seedAccount(ctx, t, st, "abort@sytue.eu")
	inbox, _ := st.MailboxByRole(ctx, acc.ID, domain.RoleInbox)
	seedMessage(ctx, t, st, acc, inbox.ID, "original", "a@example.test", nil)
	epoch := uuid.New()
	if _, err := st.BeginPrivateStorageMigration(ctx, acc.Subject, epoch, mailSourceReader([]byte("short"))); err == nil {
		t.Fatal("accepted changed MIME size")
	}
	state, err := st.PrivateStorageState(ctx, acc.Subject)
	if err != nil || state.Mode != "legacy" || state.Rows != 0 {
		t.Fatalf("partial freeze survived rollback: %+v %v", state, err)
	}
	item := domain.OutboundMessage{AccountID: acc.ID, From: acc.Address, Recipients: []string{"a@example.test"}, BlobKey: "outbound.eml"}
	if err := st.Enqueue(ctx, &item); err != nil {
		t.Fatal(err)
	}
	claimed, err := st.ClaimDue(context.Background(), 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	if _, err := st.BeginPrivateStorageMigration(ctx, acc.Subject, epoch, mailSourceReader(bytes.Repeat([]byte("x"), 100))); !errors.Is(err, store.ErrPrivateStorageBusy) {
		t.Fatalf("froze in-flight SMTP: %v", err)
	}
	if err := st.MarkSent(context.Background(), claimed[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.BeginPrivateStorageMigration(ctx, acc.Subject, epoch, mailSourceReader(bytes.Repeat([]byte("x"), 100))); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateMailOldSnapshotCannotWrite(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("mail-repeatable")
	acc := seedAccount(ctx, t, st, "repeatable@sytue.eu")
	inbox, _ := st.MailboxByRole(ctx, acc.ID, domain.RoleInbox)
	m := seedMessage(ctx, t, st, acc, inbox.ID, "before", "a@example.test", nil)
	tx, err := h.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant',$1,true)`, "mail-repeatable"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM messages WHERE tenant=$1`, "mail-repeatable").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if _, err := st.BeginPrivateStorageMigration(ctx, acc.Subject, uuid.New(), mailSourceReader(bytes.Repeat([]byte("x"), 100))); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `UPDATE messages SET subject='escaped' WHERE tenant=$1 AND id=$2`, "mail-repeatable", m.ID)
	if err == nil {
		t.Fatal("old repeatable-read writer escaped freeze")
	}
	if !strings.Contains(err.Error(), "serialize") && !errors.Is(store.PrivateStorageError(err), store.ErrPrivateStorageFrozen) {
		t.Fatalf("unexpected block: %v", err)
	}
}

func TestPrivateMailSourceHashesBodiesAcrossItsBoundedPage(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("mail-pages")
	acc := seedAccount(ctx, t, st, "pages@sytue.eu")
	inbox, err := st.MailboxByRole(ctx, acc.ID, domain.RoleInbox)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 51; i++ {
		seedMessage(ctx, t, st, acc, inbox.ID, "retained source", "a@example.test", nil)
	}
	opens := 0
	state, err := st.BeginPrivateStorageMigration(ctx, acc.Subject, uuid.New(), func(context.Context, string) (io.ReadCloser, error) {
		opens++
		return io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("x"), 100))), nil
	})
	if err != nil || opens != 51 || state.Blobs != 51 {
		t.Fatal("body routing page boundary lost originals", opens, state, err)
	}
}
