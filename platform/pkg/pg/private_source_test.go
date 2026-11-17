// SPDX-License-Identifier: Apache-2.0
//go:build integration

package pg_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

func newPrivateSourceHarness(t *testing.T) *pgtest.Harness {
	t.Helper()
	var ddl strings.Builder
	for _, product := range []string{"calendar", "contacts", "notes", "mail", "photos"} {
		owner := "app.owner_sub"
		table := product + "_private_storage_snapshot"
		extra := "size_bytes bigint NOT NULL,"
		if product == "contacts" {
			extra = ""
		}
		if product == "mail" {
			owner = "app.private_mail_owner"
			table = "mail_private_storage_source"
			extra += "storage_key text,"
		}
		if product == "photos" {
			extra += "storage_key text,"
		}
		fmt.Fprintf(&ddl, `CREATE TABLE %[1]s_private_storage_states(tenant text NOT NULL,owner_sub text NOT NULL,mode text NOT NULL,epoch uuid,started_at timestamptz,source_rows bigint NOT NULL DEFAULT 0,source_blobs bigint NOT NULL DEFAULT 0,PRIMARY KEY(tenant,owner_sub));
CREATE TABLE %[2]s(tenant text NOT NULL,owner_sub text NOT NULL,epoch uuid NOT NULL,kind text COLLATE "C" NOT NULL,id text COLLATE "C" NOT NULL,payload bytea,%[3]ssha256 text NOT NULL,PRIMARY KEY(tenant,owner_sub,epoch,kind,id));
ALTER TABLE %[1]s_private_storage_states ENABLE ROW LEVEL SECURITY;ALTER TABLE %[1]s_private_storage_states FORCE ROW LEVEL SECURITY;
CREATE POLICY source_owner ON %[1]s_private_storage_states USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('%[4]s',true)) WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('%[4]s',true));
ALTER TABLE %[2]s ENABLE ROW LEVEL SECURITY;ALTER TABLE %[2]s FORCE ROW LEVEL SECURITY;
CREATE POLICY source_owner ON %[2]s USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('%[4]s',true)) WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('%[4]s',true));
`, product, table, extra, owner)
	}
	return pgtest.New(t, fstest.MapFS{"migrations/0001_source.up.sql": &fstest.MapFile{Data: []byte(ddl.String())}}, "migrations")
}

func privateSourceConfig(product string) pg.PrivateSourceConfig {
	return pg.PrivateSourceConfig{Scope: storage.Scope{Tenant: "acme", Subject: "alice", Product: product}, Epoch: uuid.New(), Kind: func(k string) bool { return k == "calendar" || k == "object" || k == "mime_message" }}
}

func privateSourceTestTx(ctx context.Context, h *pgtest.Harness, cfg pg.PrivateSourceConfig, fn func(pgx.Tx) error) error {
	return h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		owner := "app.owner_sub"
		if cfg.Scope.Product == "mail" {
			owner = "app.private_mail_owner"
		}
		if _, err := tx.Exec(ctx, `SELECT set_config($1,$2,true)`, owner, cfg.Scope.Subject); err != nil {
			return err
		}
		return fn(tx)
	})
}

func seedPrivateSource(ctx context.Context, t *testing.T, h *pgtest.Harness, cfg pg.PrivateSourceConfig) {
	t.Helper()
	err := privateSourceTestTx(ctx, h, cfg, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO `+cfg.Scope.Product+`_private_storage_states(tenant,owner_sub,mode,epoch,started_at,source_rows)VALUES($1,$2,'frozen',$3,clock_timestamp(),3)`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Epoch)
		if err != nil {
			return err
		}
		table := cfg.Scope.Product + "_private_storage_snapshot"
		sizeCol, sizeValue := "size_bytes,", "octet_length($6::bytea),"
		if cfg.Scope.Product == "contacts" {
			sizeCol, sizeValue = "", ""
		}
		if cfg.Scope.Product == "mail" {
			table = "mail_private_storage_source"
		}
		for _, row := range []struct{ kind, id string }{{"calendar", "A"}, {"calendar", "a"}, {"object", "z"}} {
			data := []byte(`{"future_integer":9007199254740993,"original":"ICS\\n漢字"}`)
			sum := sha256.Sum256(data)
			_, err = tx.Exec(ctx, `INSERT INTO `+table+`(tenant,owner_sub,epoch,kind,id,payload,`+sizeCol+`sha256)VALUES($1,$2,$3,$4,$5,$6,`+sizeValue+`$7)`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Epoch, row.kind, row.id, data, hex.EncodeToString(sum[:]))
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type untouchedPrivateSourceTx struct{ pgx.Tx }

func privateSourceFailure(args ...any) error { return errors.New(fmt.Sprint(args...)) }

func TestPrivateSourceRejectsInvalidRoutingBeforeIO(t *testing.T) {
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	cfg := privateSourceConfig("calendar")
	for _, invalid := range []pg.PrivateSourceConfig{{}, {Scope: cfg.Scope, Epoch: cfg.Epoch}, {Scope: cfg.Scope, Kind: cfg.Kind}, {Scope: storage.Scope{Tenant: "acme", Subject: "alice", Product: "drive"}, Epoch: cfg.Epoch, Kind: cfg.Kind}} {
		if _, err := pg.ReadPrivateStorageSourcePage(ctx, untouchedPrivateSourceTx{}, invalid, storage.LegacySourceCursor{}, 1); err == nil {
			t.Fatal("invalid routing accepted")
		}
		if _, err := pg.ReadPrivateStorageSourceBody(ctx, untouchedPrivateSourceTx{}, invalid, "calendar", "A"); err == nil {
			t.Fatal("invalid body routing accepted")
		}
	}
	for _, limit := range []int{0, -1, storage.MaxLegacySourceRows + 1} {
		if _, err := pg.ReadPrivateStorageSourcePage(ctx, untouchedPrivateSourceTx{}, cfg, storage.LegacySourceCursor{}, limit); err == nil {
			t.Fatal("invalid page limit accepted")
		}
	}
	for _, cursor := range []storage.LegacySourceCursor{{Kind: "unknown", ID: "A"}, {Kind: "calendar", ID: "invalid\x00"}} {
		if _, err := pg.ReadPrivateStorageSourcePage(ctx, untouchedPrivateSourceTx{}, cfg, cursor, 1); err == nil {
			t.Fatal("invalid cursor accepted")
		}
	}
}

func TestPrivateSourceRealSchemasPageByteOrderAndExactJSON(t *testing.T) {
	h := newPrivateSourceHarness(t)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	for _, product := range []string{"calendar", "contacts", "notes", "mail"} {
		t.Run(product, func(t *testing.T) {
			cfg := privateSourceConfig(product)
			seedPrivateSource(ctx, t, h, cfg)
			err := privateSourceTestTx(ctx, h, cfg, func(tx pgx.Tx) error {
				page, err := pg.ReadPrivateStorageSourcePage(ctx, tx, cfg, storage.LegacySourceCursor{}, 2)
				if err != nil {
					return err
				}
				if len(page.Rows) != 2 || page.Rows[0].ID != "A" || page.Rows[1].ID != "a" || page.Next == nil || page.Next.ID != "a" {
					return privateSourceFailure("byte order or bounded page changed", page)
				}
				next, err := pg.ReadPrivateStorageSourcePage(ctx, tx, cfg, *page.Next, 2)
				if err != nil {
					return err
				}
				if len(next.Rows) != 1 || next.Rows[0].Kind != "object" || next.Next != nil {
					return privateSourceFailure("last page changed", next)
				}
				body, err := pg.ReadPrivateStorageSourceBody(ctx, tx, cfg, "calendar", "A")
				if err != nil {
					return err
				}
				reader, err := body.OpenJSON(ctx)
				if err != nil {
					return err
				}
				data, err := io.ReadAll(reader)
				if err != nil {
					return err
				}
				if err := reader.Close(); err != nil {
					return err
				}
				if !bytes.Contains(data, []byte("9007199254740993")) || !json.Valid(data) {
					return privateSourceFailure("exact original JSON lost")
				}
				if _, err := body.OpenJSON(ctx); err == nil {
					return privateSourceFailure("transferred private buffer reopened")
				}
				closed, err := io.ReadAll(reader)
				if err != nil || len(closed) != 0 {
					return privateSourceFailure("closed source remained readable")
				}
				body, err = pg.ReadPrivateStorageSourceBody(ctx, tx, cfg, "calendar", "a")
				if err != nil {
					return err
				}
				life, cancel := context.WithCancel(ctx)
				reader, err = body.OpenJSON(life)
				if err != nil {
					cancel()
					return err
				}
				cancel()
				_, err = io.Copy(io.Discard, reader)
				_ = reader.Close()
				if !errors.Is(err, context.Canceled) {
					return privateSourceFailure("io.Copy bypassed the source lifetime", err)
				}
				if _, err := pg.ReadPrivateStorageSourceBody(ctx, tx, cfg, "calendar", "missing"); !errors.Is(err, pgx.ErrNoRows) {
					return privateSourceFailure("missing row became empty success", err)
				}
				if _, err := pg.ReadPrivateStorageSourcePage(ctx, tx, cfg, storage.LegacySourceCursor{Kind: "calendar", ID: "phantom"}, 2); !errors.Is(err, pg.ErrPrivateSource) {
					return privateSourceFailure("phantom cursor accepted", err)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPrivateSourceRequiresExactTransactionOwnerEpochAndRLS(t *testing.T) {
	h := newPrivateSourceHarness(t)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	cfg := privateSourceConfig("calendar")
	seedPrivateSource(ctx, t, h, cfg)
	for _, sql := range []string{`SELECT set_config('app.owner_sub','bob',true)`, `SELECT set_config('app.tenant','other',true)`, `SELECT set_config('app.maintenance','on',true)`, `SELECT set_config('app.merge','on',true)`} {
		err := privateSourceTestTx(ctx, h, cfg, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, sql); err != nil {
				return err
			}
			if _, err := pg.ReadPrivateStorageSourcePage(ctx, tx, cfg, storage.LegacySourceCursor{}, 1); !errors.Is(err, pg.ErrPrivateSource) {
				return privateSourceFailure("transaction scope override accepted", err)
			}
			if _, err := pg.ReadPrivateStorageSourceBody(ctx, tx, cfg, "calendar", "A"); !errors.Is(err, pg.ErrPrivateSource) {
				return privateSourceFailure("transaction body override accepted", err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	err := privateSourceTestTx(ctx, h, cfg, func(tx pgx.Tx) error {
		foreign := cfg
		foreign.Epoch = uuid.New()
		if _, err := tx.Exec(ctx, `INSERT INTO calendar_private_storage_snapshot(tenant,owner_sub,epoch,kind,id,payload,size_bytes,sha256) SELECT tenant,owner_sub,$1,kind,id,payload,size_bytes,sha256 FROM calendar_private_storage_snapshot WHERE tenant='acme' AND owner_sub='alice' AND epoch=$2`, foreign.Epoch, cfg.Epoch); err != nil {
			return err
		}
		if _, err := pg.ReadPrivateStorageSourcePage(ctx, tx, foreign, storage.LegacySourceCursor{}, 1); !errors.Is(err, pg.ErrPrivateSource) {
			return privateSourceFailure("foreign epoch accepted", err)
		}
		if _, err := pg.ReadPrivateStorageSourceBody(ctx, tx, foreign, "calendar", "A"); !errors.Is(err, pg.ErrPrivateSource) {
			return privateSourceFailure("foreign body epoch accepted", err)
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('app.owner_sub','bob',true)`); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM calendar_private_storage_snapshot WHERE tenant='acme'`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return privateSourceFailure("raw owner RLS leaked retained rows")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPrivateSourceBoundsJSONAndKeepsMIMERoutingInternal(t *testing.T) {
	h := newPrivateSourceHarness(t)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	calendar, mail := privateSourceConfig("calendar"), privateSourceConfig("mail")
	seedPrivateSource(ctx, t, h, calendar)
	seedPrivateSource(ctx, t, h, mail)
	err := privateSourceTestTx(ctx, h, calendar, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE calendar_private_storage_snapshot SET payload=convert_to('{"x":"'||repeat('x',8388608)||'"}','UTF8'),size_bytes=8388616 WHERE tenant='acme' AND owner_sub='alice' AND kind='calendar' AND id='A'`); err != nil {
			return err
		}
		if _, err := pg.ReadPrivateStorageSourceBody(ctx, tx, calendar, "calendar", "A"); !errors.Is(err, pg.ErrPrivateSource) {
			return privateSourceFailure("oversized JSON accepted", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE calendar_private_storage_snapshot SET sha256=repeat('0',64) WHERE tenant='acme' AND owner_sub='alice' AND kind='calendar' AND id='a'`); err != nil {
			return err
		}
		if _, err := pg.ReadPrivateStorageSourceBody(ctx, tx, calendar, "calendar", "a"); !errors.Is(err, pg.ErrPrivateSource) {
			return privateSourceFailure("corrupt hash accepted", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.PoolSinRLS(t).Exec(ctx, `ALTER TABLE calendar_private_storage_snapshot RENAME TO calendar_source_fixture;
CREATE FUNCTION calendar_source_payload_canary(bytea)RETURNS bytea LANGUAGE plpgsql STABLE AS $$BEGIN RAISE EXCEPTION 'oversized_payload_was_materialized';END;$$;
CREATE VIEW calendar_private_storage_snapshot WITH(security_invoker=true) AS SELECT tenant,owner_sub,epoch,kind,id,CASE WHEN id='A' THEN calendar_source_payload_canary(payload) ELSE payload END AS payload,size_bytes,sha256 FROM calendar_source_fixture;`)
	if err != nil {
		t.Fatal(err)
	}
	err = privateSourceTestTx(ctx, h, calendar, func(tx pgx.Tx) error {
		if _, err := pg.ReadPrivateStorageSourceBody(ctx, tx, calendar, "calendar", "A"); !errors.Is(err, pg.ErrPrivateSource) {
			return privateSourceFailure("oversized payload was read before validating its size", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = privateSourceTestTx(ctx, h, mail, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO mail_private_storage_source(tenant,owner_sub,epoch,kind,id,payload,storage_key,size_bytes,sha256)VALUES('acme','alice',$1,'mime_message','huge',NULL,'retained/internal.eml',107374182400,repeat('a',64))`, mail.Epoch)
		if err != nil {
			return err
		}
		body, err := pg.ReadPrivateStorageSourceBody(ctx, tx, mail, "mime_message", "huge")
		if err != nil {
			return err
		}
		if body.StorageKey != "retained/internal.eml" || body.Descriptor.Size != 107374182400 {
			return privateSourceFailure("MIME routing was materialized or changed")
		}
		if _, err := body.OpenJSON(ctx); err == nil {
			return privateSourceFailure("MIME opened as JSON")
		}
		wire, err := json.Marshal(body)
		if err != nil {
			return err
		}
		if bytes.Contains(wire, []byte("internal.eml")) {
			return privateSourceFailure("internal blob key left service")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPrivateSourceRejectsUnpublishedCorruptOrMissingInventory(t *testing.T) {
	h := newPrivateSourceHarness(t)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	cfg := privateSourceConfig("calendar")
	seedPrivateSource(ctx, t, h, cfg)
	for _, change := range []string{
		`mode='legacy',epoch=NULL,started_at=NULL,source_rows=0`,
		`mode='building'`,
		`source_rows=-1`,
		`source_rows=0`,
	} {
		err := privateSourceTestTx(ctx, h, cfg, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `UPDATE calendar_private_storage_states SET mode='frozen',epoch=$1,started_at=clock_timestamp(),source_rows=3 WHERE tenant='acme' AND owner_sub='alice'`, cfg.Epoch); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE calendar_private_storage_states SET `+change+` WHERE tenant='acme' AND owner_sub='alice'`); err != nil {
				return err
			}
			if _, err := pg.ReadPrivateStorageSourcePage(ctx, tx, cfg, storage.LegacySourceCursor{}, 1); !errors.Is(err, pg.ErrPrivateSource) {
				return privateSourceFailure("unpublished or corrupt state accepted", change, err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	err := privateSourceTestTx(ctx, h, cfg, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE calendar_private_storage_states SET mode='private',source_rows=3 WHERE tenant='acme' AND owner_sub='alice'`); err != nil {
			return err
		}
		if _, err := pg.ReadPrivateStorageSourcePage(ctx, tx, cfg, storage.LegacySourceCursor{}, 1); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM calendar_private_storage_snapshot WHERE tenant='acme' AND owner_sub='alice'`); err != nil {
			return err
		}
		if _, err := pg.ReadPrivateStorageSourcePage(ctx, tx, cfg, storage.LegacySourceCursor{}, 1); !errors.Is(err, pg.ErrPrivateSource) {
			return privateSourceFailure("missing declared inventory became empty success", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPrivateSourceReaderAllowsConcurrentReadAndClose(t *testing.T) {
	h := newPrivateSourceHarness(t)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	cfg := privateSourceConfig("calendar")
	seedPrivateSource(ctx, t, h, cfg)
	err := privateSourceTestTx(ctx, h, cfg, func(tx pgx.Tx) error {
		body, err := pg.ReadPrivateStorageSourceBody(ctx, tx, cfg, "calendar", "A")
		if err != nil {
			return err
		}
		reader, err := body.OpenJSON(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = reader.Close() }()
		start := make(chan struct{})
		fail := make(chan error, 1)
		var workers sync.WaitGroup
		workers.Add(2)
		go func() {
			defer workers.Done()
			<-start
			var buffer [1]byte
			for range 2000 {
				if _, err := reader.Read(buffer[:]); err != nil && !errors.Is(err, io.EOF) {
					select {
					case fail <- err:
					default:
					}
					return
				}
				runtime.Gosched()
			}
		}()
		go func() {
			defer workers.Done()
			<-start
			for range 2000 {
				if err := reader.Close(); err != nil {
					select {
					case fail <- err:
					default:
					}
					return
				}
				runtime.Gosched()
			}
		}()
		close(start)
		workers.Wait()
		select {
		case err := <-fail:
			return err
		default:
		}
		data, err := io.ReadAll(reader)
		if err != nil || len(data) != 0 {
			return privateSourceFailure("closed concurrent reader remained available", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
