//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func TestPublicationLegacyOriginDrainsBeforeFreezeWithoutIsolatedAuthority(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	scope := storage.Scope{Tenant: "acme", Subject: uuid.NewString(), Product: "mail"}
	ctx := tenantCtx(scope.Tenant)
	room := "!canonical-source:matrix.test"
	if _, err := st.EnsureMatrixStorageRooms(ctx, scope.Subject, scope.Product, func(context.Context, string) (string, error) { return room, nil }); err != nil {
		t.Fatal(err)
	}
	route, found, err := st.MatrixPublicationRoomRoute(ctx, room)
	if err != nil || !found || route.Scope != scope || route.Identity != storage.LegacyIdentity {
		t.Fatal("canonical trusted route lost", err)
	}
	event := publication.OriginEvent{Version: 1, Reference: storage.Reference{RoomID: room, EventID: "$canonical"}, Sender: "@sytue_" + scope.Subject + ":matrix.test", Type: "m.room.encrypted", Content: json.RawMessage(`{"algorithm":"m.megolm.v1.aes-sha2","session_id":"synthetic-legacy","ciphertext":"opaque"}`)}
	for range 2 {
		if err := st.AdmitLegacyMatrixPublicationOrigin(ctx, scope, event.Sender, event); err != nil {
			t.Fatal("canonical event requires isolated root", err)
		}
	}
	for _, fault := range []string{"tenant", "subject", "product", "room", "sender", "plaintext", "marker"} {
		t.Run(fault, func(t *testing.T) {
			actual := scope
			changed := event
			switch fault {
			case "tenant":
				actual.Tenant = "other"
			case "subject":
				actual.Subject = uuid.NewString()
			case "product":
				actual.Product = "calendar"
			case "room":
				changed.Reference.RoomID = "!other:matrix.test"
			case "sender":
				changed.Sender = "@other:matrix.test"
			case "plaintext":
				changed.Type = "m.room.message"
				changed.Content = nil
			case "marker":
				var err error
				changed.Content, err = publication.AttachMarker(changed.Content, publication.Marker{Version: 1, ID: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"})
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := st.AdmitLegacyMatrixPublicationOrigin(ctx, actual, event.Sender, changed); err == nil {
				t.Fatal("foreign legacy origin accepted", fault)
			}
		})
	}
	epoch := uuid.NewString()
	if state, err := st.BeginMatrixStorageCutover(ctx, scope.Subject, scope.Product, epoch); err != nil || state.Phase != "freezing" {
		t.Fatal("durable begin", err)
	}
	calls := 0
	freeze := func(context.Context, string) error { calls++; return nil }
	if _, err := st.FreezeMatrixStorageCutover(ctx, scope.Subject, scope.Product, epoch, freeze); !errors.Is(err, domain.ErrConflict) || calls != 0 {
		t.Fatal("pending canonical origin reached remote freeze", err, calls)
	}
	// An old SQL writer omits the new publication contract entirely. RLS must
	// not make the guard see an empty set and authorize a false frozen phase.
	err = h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant',$1,true),set_config('app.matrix_publication_contract','',true),set_config('app.matrix_authority_owner','',true),set_config('app.matrix_authority_product','',true)`, scope.Tenant); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE matrix_storage_cutovers SET phase='frozen' WHERE tenant=$1 AND subject=$2 AND product=$3 AND phase='freezing'`, scope.Tenant, scope.Subject, scope.Product)
		if err == nil && tag.RowsAffected() != 1 {
			t.Fatal("fixture never exercised old cutover writer")
		}
		return err
	})
	if err == nil {
		t.Fatal("old SQL writer hid pending canonical origin")
	}
	changed := event
	changed.Reference.EventID = "$late"
	if err := st.AdmitLegacyMatrixPublicationOrigin(ctx, scope, event.Sender, changed); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("migration admitted late canonical write", err)
	}
	rows, _, err := st.PendingMatrixPublicationOrigins(t.Context(), nil, 16)
	if err != nil || len(rows) != 1 || rows[0].MatrixUserID != event.Sender || rows[0].Reference != event.Reference {
		t.Fatal("lost canonical sender needed for positive reconciliation", err)
	}
	restarted := store.New(h.Pool, nil)
	if err := restarted.AcceptMatrixPublicationOrigin(ctx, scope, event.Sender, event); err != nil {
		t.Fatal("positive canonical acknowledgement requires active isolated device", err)
	}
	state, err := restarted.FreezeMatrixStorageCutover(ctx, scope.Subject, scope.Product, epoch, freeze)
	if err != nil || state.Phase != "frozen" || calls != 1 {
		t.Fatal("accepted origin failed to drain after restart", err, calls)
	}
	// This exact origin is an idempotent replay, never fresh publication authority.
	if err := restarted.AdmitLegacyMatrixPublicationOrigin(ctx, scope, event.Sender, event); err != nil {
		t.Fatal("exact retained admission retry lost", err)
	}
	changed.Content = json.RawMessage(`{"algorithm":"m.megolm.v1.aes-sha2","session_id":"synthetic-legacy","ciphertext":"changed"}`)
	if err := restarted.AdmitLegacyMatrixPublicationOrigin(ctx, scope, event.Sender, changed); err == nil {
		t.Fatal("frozen source accepted changed wire")
	}
	if _, err := st.EnsureMatrixStorageRoomsForIdentity(ctx, scope.Subject, scope.Product, storage.IsolatedIdentity, func(context.Context, string) (string, error) { return room, nil }); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.MatrixPublicationRoomRoute(ctx, room); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("ambiguous canonical/isolated room was routed", err)
	}
}
