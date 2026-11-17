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
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/storagetest"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// This synthetic export composes actual PostgreSQL/RLS snapshots with the
// maintained SDK's Synapse conformance test across the repository's modules.
// The capability belongs only to this fixture, not a production ingress bridge.
func TestPrivateMailSourceProducesRetainedSynapseVector(t *testing.T) {
	path := os.Getenv("SYTUE_MAIL_PRIVATE_VECTOR")
	if path == "" {
		t.Skip("requires the remote synthetic Mail/Synapse composition harness")
	}
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("mail-vector")
	acc, err := st.EnsureAccount(ctx, uuid.NewString(), "synthetic-owner@example.test", "Synthetic fixture", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	inbox, err := st.MailboxByRole(ctx, acc.ID, domain.RoleInbox)
	if err != nil {
		t.Fatal(err)
	}
	label, err := st.CreateLabel(ctx, acc.ID, "Synthetic private label", "blue")
	if err != nil {
		t.Fatal(err)
	}
	signature := strings.Repeat("SYNTHETIC_PRIVATE_SIGNATURE ", 3000)
	if err := st.SetSignature(ctx, acc.ID, signature); err != nil {
		t.Fatal(err)
	}
	if err := st.SetVacation(ctx, acc.ID, domain.Vacation{Enabled: true, Subject: "Synthetic away", Body: "SYNTHETIC_PRIVATE_AUTOREPLY"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateFilter(ctx, acc.ID, domain.Filter{Name: "Synthetic private rule", MatchSubject: "SYNTHETIC_PRIVATE", MarkRead: true}); err != nil {
		t.Fatal(err)
	}
	blobs := map[string][]byte{}
	insert := func(subject string, raw []byte) domain.Message {
		t.Helper()
		message := domain.Message{AccountID: acc.ID, MailboxID: inbox.ID, Subject: subject, FromAddr: "sender@example.test", To: []string{acc.Address}, MessageID: uuid.NewString() + "@example.test", BlobKey: "fixture/" + uuid.NewString(), SizeBytes: int64(len(raw)), Flags: []string{label.ID.String()}, ReceivedAt: time.Date(2026, 10, 30, 1, 2, 3, 123456000, time.UTC)}
		if err := st.InsertMessage(ctx, &message); err != nil {
			t.Fatal(err)
		}
		blobs[message.BlobKey] = raw
		return message
	}
	live := insert("SYNTHETIC_PRIVATE_LIVE", []byte("Subject: Synthetic historical MIME\r\n\r\n"+strings.Repeat("SYNTHETIC_PRIVATE_MIME ", 15000)))
	deleted := insert("SYNTHETIC_PRIVATE_DELETED", []byte("Subject: Synthetic deleted MIME\r\n\r\nSYNTHETIC_PRIVATE_DELETED_MIME"))
	if err := st.DeleteMessage(ctx, acc.ID, deleted.ID); err != nil {
		t.Fatal(err)
	}
	queued := domain.OutboundMessage{AccountID: acc.ID, From: acc.Address, Recipients: []string{"outside@example.test"}, Subject: "SYNTHETIC_PRIVATE_QUEUED", BlobKey: "fixture/queued"}
	blobs[queued.BlobKey] = []byte("Subject: Synthetic queued MIME\r\n\r\nSYNTHETIC_PRIVATE_QUEUE")
	if _, err := st.EnqueueHeld(ctx, &queued, time.Now().Add(time.Hour), "scheduled"); err != nil {
		t.Fatal(err)
	}
	epoch := uuid.New()
	state, err := st.BeginPrivateStorageMigration(ctx, acc.Subject, epoch, func(ctx context.Context, key string) (io.ReadCloser, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		raw, found := blobs[key]
		if !found {
			return nil, errors.New("synthetic source requested unknown body")
		}
		return io.NopCloser(bytes.NewReader(raw)), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	vector := storagetest.MailSnapshot{State: storage.AppMigrationState{Version: 1, LegacyMigrationState: state, MaxBodyBytes: 50 << 20, PrivateIngressReady: true}, LiveID: live.ID.String(), DeletedID: deleted.ID.String(), InboxID: inbox.ID.String(), LabelID: label.ID.String(), Signature: signature}
	after := storage.LegacySourceCursor{}
	for {
		page, err := st.PrivateStorageSourcePage(ctx, acc.Subject, epoch, after, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range page.Rows {
			body, err := st.PrivateStorageSourceBody(ctx, acc.Subject, epoch, row.Kind, row.ID)
			if err != nil {
				t.Fatal(err)
			}
			raw := body.Payload
			if body.StorageKey != "" {
				raw = blobs[body.StorageKey]
			}
			sum := sha256.Sum256(raw)
			if body.Descriptor != row || int64(len(raw)) != row.Size || hex.EncodeToString(sum[:]) != row.SHA256 {
				t.Fatal("PostgreSQL source changed during exact-byte export")
			}
			vector.Rows = append(vector.Rows, storagetest.MailSnapshotRow{Descriptor: row, Body: raw})
		}
		if page.Next == nil {
			break
		}
		after = *page.Next
	}
	if state.Mode != "frozen" || state.Blobs != 3 || int64(len(vector.Rows)) != state.Rows+state.Blobs {
		t.Fatal("PostgreSQL source omitted retained data", state)
	}
	if err := vector.Validate(); err != nil {
		t.Fatal("PostgreSQL vector lost its complete byte commitments", err)
	}
	if err := store.PrivateStorageError(st.SetSignature(ctx, acc.ID, "blocked")); !errors.Is(err, store.ErrPrivateStorageFrozen) {
		t.Fatal("source did not retain its real writer fence", err)
	}
	raw, err := json.Marshal(vector)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Log("synthetic actual PostgreSQL snapshot exported: live/deleted/queued MIME, labels, large signature, rules and vacation; no production ingress or retirement capability granted")
}
