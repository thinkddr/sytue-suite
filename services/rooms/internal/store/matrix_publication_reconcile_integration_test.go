//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func TestPublicationReconcilePagesRetainedRoutingWithoutCrossOwnerWrites(t *testing.T) {
	first := enrolledPublicationFixture(t)
	ctx := tenantCtx(first.scope.Tenant)
	event, _ := reservedOriginFixture(t, first, "reconcile-first")
	if err := first.st.AdmitMatrixPublicationOrigin(ctx, first.scope, event.Sender, event, true); err != nil {
		t.Fatal(err)
	}
	second := &authorityFixture{h: first.h, st: first.st, gate: first.gate, scope: storage.Scope{Tenant: "zeta", Subject: "bob", Product: "calendar"}, target: storage.PublicationTarget{RoomID: "!second:matrix.test", DeviceID: "NEXT_OWNER"}, epoch: uuid.NewString()}
	second.gate.keys = matrixprooftest.New(t, second.scope, "matrix.test", id.DeviceID(second.target.DeviceID))
	_, err := second.st.EnsureMatrixStorageRoomsForIdentity(tenantCtx(second.scope.Tenant), second.scope.Subject, second.scope.Product, storage.IsolatedIdentity, func(_ context.Context, _ string) (string, error) { return second.target.RoomID, nil })
	if err != nil {
		t.Fatal(err)
	}
	challenge := second.challenge(t)
	if _, err := second.st.ConfirmMatrixRoot(tenantCtx(second.scope.Tenant), second.scope.Subject, second.scope.Product, second.sign(t, challenge.Action)); err != nil {
		t.Fatal(err)
	}
	other, _ := reservedOriginFixture(t, second, "reconcile-second")
	if err := second.st.AdmitMatrixPublicationOrigin(tenantCtx(second.scope.Tenant), second.scope, other.Sender, other, true); err != nil {
		t.Fatal(err)
	}
	reservedOriginFixture(t, second, "not-origin-admitted")
	var after *store.MatrixPublicationCursor
	var seen []storage.Reference
	for round := range 3 {
		values, next, err := first.st.PendingMatrixPublicationOrigins(t.Context(), after, 1)
		if err != nil {
			t.Fatal(err)
		}
		if round < 2 {
			if len(values) != 1 || next == nil {
				t.Fatal("pending page lost durable hold", round)
			}
			seen = append(seen, values[0].Reference)
		} else if len(values) != 0 || next != nil {
			t.Fatal("pagination did not finish")
		}
		after = next
	}
	if seen[0] != event.Reference || seen[1] != other.Reference {
		t.Fatal("pending origin routing changed")
	}
	for _, maintenance := range []bool{false, true} {
		run := first.h.Pool.InTenantTx
		if maintenance {
			run = first.h.Pool.InMaintenanceTx
		}
		if err := run(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT set_config('app.matrix_publication_contract','1',true),set_config('app.matrix_publication_reconcile',$1,true)`, map[bool]string{true: "", false: "1"}[maintenance]); err != nil {
				return err
			}
			var count int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM matrix_storage_publications`).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				t.Fatal("partial maintenance capability exposed routing", count)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := first.h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.matrix_publication_contract','1',true),set_config('app.matrix_publication_reconcile','1',true)`); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE matrix_storage_publications SET status='accepted',accepted_at=clock_timestamp() WHERE status='sending'`)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 0 {
			t.Fatal("maintenance reader gained cross-owner writes")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	values, _, err := first.st.PendingMatrixPublicationOrigins(t.Context(), nil, 16)
	if err != nil || len(values) != 2 {
		t.Fatal("maintenance changed pending state", len(values), err)
	}
}
