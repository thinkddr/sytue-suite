// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	native "git.sytue.eu/sytue/one/apps/nucleo/contacts/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/contacts/db"
	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
	"git.sytue.eu/sytue/one/services/contacts/internal/store"
)

func TestPrivateSnapshotIncludesTrashGroupsMetadataAndEnforcesEveryTable(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	book, err := st.EnsureDefaultAddressBook(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	input := sampleContact(book.ID, "person", "Ada")
	input.Contact.Note = strings.Repeat("artificial private Contacts source marker;", 1024)
	input.CardData, err = domain.EncodeContact(input.Contact, input.CardData)
	if err != nil {
		t.Fatal(err)
	}
	person, err := st.CreateContact(ctx, "alice", input)
	if err != nil {
		t.Fatal(err)
	}
	avatar := image.NewRGBA(image.Rect(0, 0, 1, 1))
	avatar.SetRGBA(0, 0, color.RGBA{R: 40, G: 80, B: 120, A: 255})
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, avatar); err != nil {
		t.Fatal(err)
	}
	if err := st.SetContactPhoto(ctx, "alice", person.UID, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(imageBytes.Bytes())); err != nil {
		t.Fatal(err)
	}
	if err := st.SetFavorite(ctx, "alice", person.UID, true); err != nil {
		t.Fatal(err)
	}
	group, err := st.CreateGroup(ctx, "alice", store.UpsertGroup{AddressBookID: book.ID, Group: domain.Group{UID: "group", Name: "Friends", Members: []string{person.UID, "unresolved"}}, CardData: "BEGIN:VCARD\r\nVERSION:4.0\r\nKIND:group\r\nUID:group\r\nFN:Friends\r\nEND:VCARD\r\n"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteContact(ctx, "alice", person.ID); err != nil {
		t.Fatal(err)
	}
	foreign, err := st.EnsureDefaultAddressBook(ctx, "bob")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateContact(ctx, "bob", sampleContact(foreign.ID, "foreign", "Bob")); err != nil {
		t.Fatal(err)
	}
	epoch := uuid.New()
	state, err := st.BeginPrivateStorageMigration(ctx, "alice", epoch)
	if err != nil {
		t.Fatal(err)
	}
	if state.Version != 1 || state.Scope.Subject != "alice" || state.Mode != "frozen" || state.Rows != 9 || state.Blobs != 0 {
		t.Fatalf("incomplete state: %+v", state)
	}
	replay, err := st.BeginPrivateStorageMigration(ctx, "alice", epoch)
	if err != nil || replay.Epoch != state.Epoch || replay.Rows != state.Rows {
		t.Fatal("retry changed snapshot", err)
	}
	if _, err := st.BeginPrivateStorageMigration(ctx, "alice", uuid.New()); !errors.Is(err, store.ErrPrivateStorageEpoch) {
		t.Fatal("changed frozen epoch", err)
	}
	rows := []native.Row{}
	after := storage.LegacySourceCursor{}
	for {
		page, err := st.PrivateStorageSourcePage(ctx, "alice", epoch, after)
		if err != nil {
			t.Fatal(err)
		}
		if err := page.Validate(state.Scope, epoch.String(), after); err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Rows {
			info, body, err := st.OpenPrivateStorageSource(ctx, "alice", epoch, item.Kind, item.ID)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(body)
			if err != nil {
				t.Fatal(err)
			}
			if err := body.Close(); err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(raw)
			if info != item || int64(len(raw)) != item.Size || hex.EncodeToString(hash[:]) != item.SHA256 {
				t.Fatal("stream not bound to inventory")
			}
			rows = append(rows, native.Row{Kind: item.Kind, ID: item.ID, JSON: raw})
		}
		if page.Next == nil {
			break
		}
		after = *page.Next
	}
	if path := os.Getenv("SYTUE_CONTACTS_PRIVATE_VECTOR"); path != "" {
		vector := struct {
			State store.PrivateStorageState `json:"state"`
			Rows  []native.Row              `json:"rows"`
		}{State: state, Rows: rows}
		raw, err := json.Marshal(vector)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := native.NewSnapshot(state.Scope, rows)
	if err != nil {
		t.Fatal(err)
	}
	var contactSeen, memberCount int
	for _, row := range snapshot.Rows() {
		if row.Kind == "contact" {
			var value struct {
				ID        string     `json:"id"`
				DeletedAt *time.Time `json:"deleted_at"`
				Favorite  bool       `json:"is_favorite"`
			}
			if json.Unmarshal(row.JSON, &value) != nil || value.ID != person.ID.String() || value.DeletedAt == nil || !value.Favorite {
				t.Fatal("lost trash/favorite or leaked foreign row")
			}
			contactSeen++
		}
		if row.Kind == "member" {
			memberCount++
		}
	}
	if contactSeen != 1 || memberCount != 2 || group.ID == uuid.Nil {
		t.Fatal("incomplete contact/group source")
	}

	for _, table := range []string{"address_books", "card_objects", "contacts", "contact_emails", "contact_phones", "contact_groups", "contact_group_members"} {
		err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE `+pgx.Identifier{table}.Sanitize()+` SET tenant=tenant WHERE tenant='acme'`)
			return err
		})
		var failure *pgconn.PgError
		if !errors.As(err, &failure) || failure.Code != "PST01" {
			t.Fatalf("%s allowed a frozen write: %v", table, err)
		}
	}
	if _, err := st.CreateContact(ctx, "bob", sampleContact(foreign.ID, "still-legacy", "Allowed")); err != nil {
		t.Fatal("froze unrelated owner", err)
	}
	if _, _, err := st.OpenPrivateStorageSource(ctx, "bob", epoch, "book", book.ID.String()); !errors.Is(err, store.ErrPrivateStorageEpoch) {
		t.Fatal("read foreign owner's snapshot", err)
	}
	if _, _, err := st.OpenPrivateStorageSource(tenantCtx("other"), "alice", epoch, "book", book.ID.String()); !errors.Is(err, store.ErrPrivateStorageEpoch) {
		t.Fatal("read foreign tenant snapshot", err)
	}
	err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.owner_sub','alice',true)"); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE contacts_private_storage_snapshot SET payload=convert_to('{}','UTF8')`)
		if err == nil && tag.RowsAffected() != 0 {
			t.Fatal("retained source was mutable")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPrivateSnapshotRejectsAWriterUsingAnOldRepeatableReadSnapshot(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx, cancel := context.WithTimeout(tenantCtx("acme"), 30*time.Second)
	defer cancel()
	book, err := st.EnsureDefaultAddressBook(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	person, err := st.CreateContact(ctx, "alice", sampleContact(book.ID, "person", "Before"))
	if err != nil {
		t.Fatal(err)
	}
	established, proceed := make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- h.Pool.InTxOpts(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead}, func(tx pgx.Tx) error {
			if err := pg.SetTenant(ctx, tx, "acme"); err != nil {
				return err
			}
			var count int
			if err := tx.QueryRow(ctx, "SELECT count(*) FROM contacts").Scan(&count); err != nil {
				return err
			}
			close(established)
			select {
			case <-proceed:
			case <-ctx.Done():
				return ctx.Err()
			}
			_, err := tx.Exec(ctx, "UPDATE contacts SET formatted_name='After' WHERE id=$1", person.ID)
			return err
		})
	}()
	select {
	case <-established:
	case err := <-result:
		t.Fatal("writer setup failed", err)
	case <-time.After(10 * time.Second):
		t.Fatal("writer setup stalled")
	}
	if _, err := st.BeginPrivateStorageMigration(ctx, "alice", uuid.New()); err != nil {
		close(proceed)
		t.Fatal(err)
	}
	close(proceed)
	select {
	case err := <-result:
		var failure *pgconn.PgError
		if !errors.As(err, &failure) || (failure.Code != "40001" && failure.Code != "PST01") {
			t.Fatal("old writer passed freeze", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("writer did not finish")
	}
}

func TestPrivateSnapshotIncompleteBuildCannotCommit(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := tenantCtx("acme")
	err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.owner_sub','alice',true)"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO contacts_private_storage_states(tenant,owner_sub,mode,epoch,started_at) VALUES('acme','alice','building',$1,now())`, uuid.New())
		return err
	})
	var failure *pgconn.PgError
	if !errors.As(err, &failure) || failure.Code != "PST02" {
		t.Fatal("partial capture committed", err)
	}
}

func TestPrivateAvatarSyncSkipsFrozenOwnerAndStillUpdatesLegacyRecipients(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	alice, err := st.EnsureDefaultAddressBook(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	first, err := st.CreateContact(ctx, "alice", sampleContact(alice.ID, "alice-person", "Ada"))
	if err != nil {
		t.Fatal(err)
	}
	bob, err := st.EnsureDefaultAddressBook(ctx, "bob")
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.CreateContact(ctx, "bob", sampleContact(bob.ID, "bob-person", "Ada"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.BeginPrivateStorageMigration(ctx, "alice", uuid.New()); err != nil {
		t.Fatal(err)
	}
	updated, err := st.SyncAvatarByEmail(ctx, []string{"ada@sytue.eu"}, "")
	if err != nil || updated != 1 {
		t.Fatal("frozen owner blocked unrelated legacy avatar", updated, err)
	}
	still, err := st.GetContact(ctx, "alice", first.ID)
	if err != nil || still.ETag != first.ETag {
		t.Fatal("rewrote frozen avatar", err)
	}
	changed, err := st.GetContact(ctx, "bob", second.ID)
	if err != nil || changed.ETag == second.ETag {
		t.Fatal("did not update eligible legacy avatar", err)
	}
}

func TestPrivateSnapshotLifecycleStaysPinnedDuringPlatformMerge(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	if _, err := st.EnsureDefaultAddressBook(ctx, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.BeginPrivateStorageMigration(ctx, "alice", uuid.New()); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		"UPDATE contacts_private_storage_states SET owner_sub='other'",
		"UPDATE contacts_private_storage_states SET tenant='other'",
		"UPDATE contacts_private_storage_states SET epoch=gen_random_uuid()",
		"UPDATE contacts_private_storage_states SET source_rows=source_rows+1",
		"UPDATE contacts_private_storage_states SET mode='legacy',epoch=NULL,started_at=NULL",
		"DELETE FROM contacts_private_storage_states",
		"UPDATE contacts_private_storage_snapshot SET id='renamed'",
		"DELETE FROM contacts_private_storage_snapshot",
	} {
		err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, "SELECT set_config('app.merge','on',true)"); err != nil {
				return err
			}
			var count int
			if err := tx.QueryRow(ctx, "SELECT count(*) FROM contacts_private_storage_snapshot").Scan(&count); err != nil {
				return err
			}
			if count != 1 {
				t.Fatal("merge preflight cannot see retained source", count)
			}
			_, err := tx.Exec(ctx, sql)
			return err
		})
		var failure *pgconn.PgError
		if !errors.As(err, &failure) || failure.Code != "PST01" {
			t.Fatal("merge changed retained source identity", sql, err)
		}
	}
}
