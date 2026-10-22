// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package merge

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	notesdb "git.sytue.eu/sytue/one/services/notes/db"
)

const notesMergeCipher = "encv1:AAECAwQFBgcICQ=="

type notesMergeFixture struct {
	admin, app *pg.Pool
	notebook   string
	note       string
}

func notesMergeEnvironment(t *testing.T) notesMergeFixture {
	t.Helper()
	ctx := t.Context()
	h := pgtest.New(t, notesdb.Migrations, notesdb.MigrationsDir)
	admin := h.PoolSinRLS(t)
	app, err := pg.Connect(ctx, pg.Config{DSN: h.AppDSN, MaxConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	f := notesMergeFixture{admin: admin, app: app}
	err = admin.InTx(ctx, func(tx pgx.Tx) error {
		if err := notesdb.MarkWriteTransaction(ctx, tx); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO notebooks
		    (tenant,owner_sub,title,encrypted,indexable,crypto)
		    VALUES ($1,'source-owner','Sealed',true,false,'{"salt":"fixture","iterations":600000}')
		    RETURNING id::text`, origen).Scan(&f.notebook); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO notes
		    (tenant,notebook_id,owner_sub,content_cipher,data)
		    VALUES ($1,$2,'source-owner',$3,'{"client_id":"opaque-capture"}')
		    RETURNING id::text`, origen, f.notebook, notesMergeCipher).Scan(&f.note); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO notebooks (tenant,owner_sub,title)
		    VALUES ($1,'target-owner','Existing target')`, destino)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestMoveServiceAdoptsNotesContractBeforePrepare(t *testing.T) {
	f := notesMergeEnvironment(t)
	ctx := t.Context()
	_, err := f.admin.Exec(ctx, `CREATE FUNCTION platform_merge_prepare(text,text) RETURNS text
	    LANGUAGE plpgsql AS $$ BEGIN
	      IF current_setting('app.notes_write_version',true) IS DISTINCT FROM '2' THEN
	        RAISE EXCEPTION 'merge prepared without notes contract';
	      END IF;
	      RETURN 'notes contract adopted';
	    END $$`)
	if err != nil {
		t.Fatal(err)
	}
	r, err := MoveService(ctx, f.app, origen, destino)
	if err != nil {
		t.Fatalf("merge encrypted Notes: %v", err)
	}
	if r.Rows["notebooks"] != 1 || r.Rows["notes"] != 1 || r.Prepared != "notes contract adopted" {
		t.Fatalf("unexpected merge result: %+v", r)
	}
	var preserved bool
	err = f.admin.QueryRow(ctx, `SELECT n.tenant=$2 AND nb.tenant=$2
	    AND n.owner_sub='source-owner' AND nb.owner_sub='source-owner'
	    AND n.content_cipher=$3 AND n.title='' AND n.content=''
	    AND n.data='{"client_id":"opaque-capture"}'::jsonb
	    AND nb.crypto='{"salt":"fixture","iterations":600000}'::jsonb
	    AND n.min_write_version=2 AND nb.min_write_version=2
	    FROM notes n JOIN notebooks nb ON nb.id=n.notebook_id WHERE n.id=$1`, f.note, destino, notesMergeCipher).Scan(&preserved)
	if err != nil || !preserved {
		t.Fatalf("merge changed opaque content, ownership or write floor: preserved=%v err=%v", preserved, err)
	}
	for _, tenant := range []string{origen, destino, "unrelated-org"} {
		err := f.app.InTx(ctx, func(tx pgx.Tx) error {
			if err := pg.SetTenant(ctx, tx, tenant); err != nil {
				return err
			}
			var count int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM notes`).Scan(&count); err != nil {
				return err
			}
			want := 0
			if tenant == destino {
				want = 1
			}
			if count != want {
				t.Errorf("tenant %s sees %d notes, want %d", tenant, count, want)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// A single physical connection must not lend the contract to an old writer.
	err = f.app.InMergeTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE notes SET tenant=tenant WHERE id=$1`, f.note)
		return err
	})
	var blocked *pgconn.PgError
	if !errors.As(err, &blocked) || blocked.ConstraintName != notesdb.WriteContractConstraint {
		t.Fatalf("old merge writer inherited capability: %v", err)
	}
}

func TestMoveServiceCannotAdoptFutureNotesContract(t *testing.T) {
	f := notesMergeEnvironment(t)
	ctx := t.Context()
	err := f.admin.InTx(ctx, func(tx pgx.Tx) error {
		// Only a future binary could write this floor; the current helper stays 2.
		if _, err := tx.Exec(ctx, `SELECT set_config('app.notes_write_version','3',true)`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE notes SET min_write_version=3 WHERE id=$1`, f.note)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = MoveService(ctx, f.app, origen, destino)
	var blocked *pgconn.PgError
	if !errors.As(err, &blocked) || blocked.ConstraintName != notesdb.WriteContractConstraint {
		t.Fatalf("merge forged a future capability: %v", err)
	}
	if got := cuenta(ctx, t, f.admin, "notebooks", origen); got != 1 {
		t.Fatalf("failed merge did not roll back notebook: %d", got)
	}
	if got := cuenta(ctx, t, f.admin, "notes", origen); got != 1 {
		t.Fatalf("failed merge did not roll back note: %d", got)
	}
}
