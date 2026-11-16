// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package merge

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	drivedb "git.sytue.eu/sytue/one/services/drive/db"
)

func TestMoveServiceDriveLegacyDoesNotRecreateSourceOwnerWhileMovingFolders(t *testing.T) {
	h := pgtest.New(t, drivedb.Migrations, drivedb.MigrationsDir)
	ctx := context.Background()
	admin := h.PoolSinRLS(t)
	if _, err := admin.Exec(ctx, `INSERT INTO folders(tenant,name,is_root,owner_sub) VALUES($1,'Synthetic personal root',true,'alice')`, origen); err != nil {
		t.Fatal(err)
	}
	res, err := MoveService(ctx, h.Pool, origen, destino)
	if err != nil || res.Rows["folders"] != 1 || res.Rows["drive_private_storage_states"] != 1 {
		t.Fatal("legacy Drive merge failed", res, err)
	}
	leftover, err := LeftoversIn(ctx, h.Pool, "drive", origen)
	if err != nil || len(leftover) != 0 {
		t.Fatal("legacy trigger recreated moved source owner", leftover, err)
	}
	if cuenta(ctx, t, admin, "folders", destino) != 1 || cuenta(ctx, t, admin, "drive_private_storage_states", destino) != 1 {
		t.Fatal("merge lost destination data")
	}
}

func TestMoveServiceDriveFrozenScopeAbortsAtomically(t *testing.T) {
	h := pgtest.New(t, drivedb.Migrations, drivedb.MigrationsDir)
	ctx := context.Background()
	admin := h.PoolSinRLS(t)
	if _, err := admin.Exec(ctx, `INSERT INTO folders(tenant,name,is_root,owner_sub) VALUES($1,'Synthetic personal root',true,'alice')`, origen); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `UPDATE drive_private_storage_states SET mode='frozen',epoch=gen_random_uuid(),started_at=now() WHERE tenant=$1`, origen); err != nil {
		t.Fatal(err)
	}
	_, err := MoveService(ctx, h.Pool, origen, destino)
	if !errors.Is(err, pg.ErrScopeBoundMerge) {
		t.Fatal("preflight did not reject the frozen authenticated scope", err)
	}
	// The independent database guard remains effective even if a caller omits
	// the Go preflight. This is a real SQL failure, not the domain sentinel above.
	direct := h.Pool.InMergeTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE drive_private_storage_states SET tenant=$1 WHERE tenant=$2`, destino, origen)
		return err
	})
	var failure *pgconn.PgError
	if !errors.As(direct, &failure) || failure.Code != "PST01" {
		t.Fatal("database allowed a frozen Scope rewrite", direct)
	}
	if cuenta(ctx, t, admin, "folders", origen) != 1 || cuenta(ctx, t, admin, "drive_private_storage_states", origen) != 1 || cuenta(ctx, t, admin, "folders", destino) != 0 {
		t.Fatal("failed merge did not roll back atomically")
	}
}
