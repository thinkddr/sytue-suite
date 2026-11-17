// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

func TestPrivateStorageMergeSeesSourceButCannotRewriteFrozenScope(t *testing.T) {
	h, st, ctx, _, _ := privateDriveFixture(t)
	epoch := uuid.New()
	state, err := st.BeginPrivateStorageMigration(ctx, epoch)
	if err != nil {
		t.Fatal(err)
	}
	merge := func(ctx context.Context, query string) error {
		return h.Pool.InMergeTx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, query)
			return err
		})
	}
	if err := h.Pool.InMergeTx(tenantCtx("other"), func(tx pgx.Tx) error {
		for _, table := range []string{"drive_private_storage_states", "drive_private_storage_snapshot", "drive_private_storage_source_blobs"} {
			var count int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE tenant='acme'`).Scan(&count); err != nil {
				return err
			}
			if count == 0 {
				return fmt.Errorf("merge preflight did not see %s", table)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`UPDATE drive_private_storage_states SET tenant='other' WHERE tenant='acme'`,
		`UPDATE drive_private_storage_states SET owner_sub='bob' WHERE tenant='acme'`,
		`UPDATE drive_private_storage_states SET mode='legacy',epoch=NULL,started_at=NULL WHERE tenant='acme'`,
		`DELETE FROM drive_private_storage_states WHERE tenant='acme'`,
		`UPDATE drive_private_storage_snapshot SET tenant='other' WHERE tenant='acme'`,
		`UPDATE drive_private_storage_snapshot SET payload='{}'::bytea WHERE tenant='acme'`,
		`DELETE FROM drive_private_storage_snapshot WHERE tenant='acme'`,
		`UPDATE drive_private_storage_source_blobs SET storage_key='changed' WHERE tenant='acme'`,
		`DELETE FROM drive_private_storage_source_blobs WHERE tenant='acme'`,
	} {
		if err := merge(tenantCtx("other"), query); !errors.Is(store.PrivateStorageError(err), domain.ErrPrivateStorageFrozen) {
			t.Fatalf("merge changed retained source: %s: %v", query, err)
		}
	}
	again, err := st.PrivateStorageState(ctx)
	if err != nil || again.Epoch != state.Epoch || again.Rows != state.Rows || len(privateRows(ctx, t, st, epoch, 50)) != int(state.Rows) {
		t.Fatal("failed merge damaged source", again, err)
	}
}
