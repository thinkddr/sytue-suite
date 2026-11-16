// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/apps/nucleo/office/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/db"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

type officeObjects map[string][]byte

func (o officeObjects) Get(_ context.Context, _ tenancy.Tenant, key string) (io.ReadCloser, error) {
	raw, ok := o[key]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(raw)), nil
}

func officeBlocked(err error) bool {
	var failure *pgconn.PgError
	return errors.As(err, &failure) && failure.Code == "PST01"
}

func TestOfficePrivateSourceRetainsThreeProductsACLHistoryAndOriginalBytes(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	exec := func(query string, args ...any) {
		t.Helper()
		if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error { _, err := tx.Exec(ctx, query, args...); return err }); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO doc_org_brand(tenant,version,name,bg,ink,accent,created_by)VALUES('acme',1,'first','#fff','#000','#00f','admin'),('acme',2,'second','#fff','#000','#f00','admin')`)
	if err := h.PoolSinRLS(t).InTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `ALTER TABLE documents ADD COLUMN retained_future numeric`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, product := range []string{"docs", "sheets", "slides"} {
		t.Run(product, func(t *testing.T) {
			kind, _ := model.ProductKind(product)
			scope := storage.Scope{Tenant: "acme", Subject: "alice-" + product, Product: product}
			doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: kind, Title: "historical / 漢字", OwnerSub: scope.Subject})
			if err != nil {
				t.Fatal(err)
			}
			foreign, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: kind, Title: "another owner", OwnerSub: "bob"})
			if err != nil {
				t.Fatal(err)
			}
			exec(`UPDATE documents SET retained_future=900719925474099312345,seq=1,deleted_at=now()-interval '4 days' WHERE id=$1`, doc.ID)
			exec(`INSERT INTO doc_operations(document_id,seq,tenant,actor,author_sub,lamport,op)VALUES($1,1,'acme','a',$2,1,'{"kind":"set","dot":{"a":"a","c":1},"key":"doc:html","value":"<p>old</p>","lamport":1,"future":900719925474099312345}')`, doc.ID, scope.Subject)
			exec(`INSERT INTO doc_permissions(document_id,tenant,principal_kind,principal_id,role,granted_by)VALUES($1,'acme','user','bob','editor',$2),($1,'acme','group','org/acme:engineering','commenter',$2)`, doc.ID, scope.Subject)
			exec(`INSERT INTO doc_comments(document_id,tenant,author_sub,anchor,body)VALUES($1,'acme','bob','{"dot":{"a":"a","c":1}}','retained collaborator comment')`, doc.ID)
			exec(`INSERT INTO doc_vistas_filtro(document_id,tenant,sub,estado)VALUES($1,'acme','bob','{"private":"bob-only-filter"}'),($1,'acme',$2,'{"private":"alice-owned-filter"}'),($3,'acme',$2,'{"private":"alice-shared-filter"}')`, doc.ID, scope.Subject, foreign.ID)
			snapshot := uuid.New().String()
			snapshotBody := []byte(`{"seq":1,"lamport":1,"elements":[],"registers":{},"future":900719925474099312345}
`)
			snapshotKey := "docs/" + doc.ID + "/snapshots/1.json"
			exportKey := "docs/" + doc.ID + "/exports/original.docx"
			exportBody := []byte{0, 1, 2, 3, 255, 17, 42}
			exec(`INSERT INTO doc_snapshots(id,document_id,tenant,seq,blob_key,size_bytes)VALUES($1,$2,'acme',1,$3,$4)`, snapshot, doc.ID, snapshotKey, len(snapshotBody))
			exec(`INSERT INTO doc_versions(document_id,tenant,snapshot_id,label,author_sub,seq,auto)VALUES($1,'acme',$2,'retained version',$3,1,false)`, doc.ID, snapshot, scope.Subject)
			exec(`INSERT INTO doc_blobs(document_id,tenant,format,blob_key,seq,size_bytes,content_type,created_by)VALUES($1,'acme','ooxml',$2,1,$3,'application/octet-stream',$4)`, doc.ID, exportKey, len(exportBody), scope.Subject)
			script := uuid.New().String()
			code := "const retained=9007199254740993"
			digest := sha256.Sum256([]byte(code))
			exec(`INSERT INTO doc_scripts(tenant,id,name,kind,author_sub,document_id)VALUES('acme',$1,'retained script',$2,$3,$4)`, script, string(kind), scope.Subject, doc.ID)
			exec(`INSERT INTO doc_script_versions(tenant,script_id,version,code,code_hash,manifest,created_by)VALUES('acme',$1,1,$2,$3,'{}',$4)`, script, code, hex.EncodeToString(digest[:]), scope.Subject)
			objects := officeObjects{snapshotKey: snapshotBody, exportKey: exportBody}
			source, err := store.NewPrivateSource(st, objects)
			if err != nil {
				t.Fatal(err)
			}
			state, err := source.Begin(ctx, scope, uuid.New())
			if err != nil {
				t.Fatal(err)
			}
			if state.Mode != "frozen" || !state.Shared || state.Blobs != 2 {
				t.Fatal("incomplete or incorrectly personal source", state.Mode, state.Rows, state.Blobs, state.Shared)
			}
			counts := map[string]int{}
			var cursor storage.LegacySourceCursor
			seenPrecise, seenTrash := false, false
			for {
				page, err := source.Page(ctx, scope, uuid.MustParse(state.Epoch), cursor, 2)
				if err != nil || page.Validate(scope, state.Epoch, cursor) != nil {
					t.Fatal("invalid source page", err)
				}
				for _, row := range page.Rows {
					actual, reader, err := source.Open(ctx, scope, uuid.MustParse(state.Epoch), row.Kind, row.ID)
					if err != nil || actual != row {
						t.Fatal("source descriptor changed", err)
					}
					raw, err := io.ReadAll(reader)
					closeErr := reader.Close()
					if err != nil || closeErr != nil {
						t.Fatal(err, closeErr)
					}
					digest := sha256.Sum256(raw)
					if int64(len(raw)) != row.Size || hex.EncodeToString(digest[:]) != row.SHA256 {
						t.Fatal("source bytes changed")
					}
					if bytes.Contains(raw, []byte("bob-only-filter")) {
						t.Fatal("owner received collaborator's private view")
					}
					if row.Kind == "document" {
						seenPrecise = bytes.Contains(raw, []byte("900719925474099312345"))
						seenTrash = bytes.Contains(raw, []byte(`"deleted_at":`))
					}
					counts[row.Kind]++
					clear(raw)
				}
				if page.Next == nil {
					break
				}
				cursor = *page.Next
			}
			for _, kind := range []string{"document", "operation", "snapshot", "version", "comment", "blob", "script", "script_version"} {
				if counts[kind] != 1 {
					t.Fatal("historical relation omitted", kind, counts[kind])
				}
			}
			if counts["permission"] != 2 || counts["filter_view"] != 2 || counts["brand"] != 2 || counts[model.BinarySourceKind] != 2 || !seenPrecise || !seenTrash {
				t.Fatal("ACL, views, versions, trash or precise metadata omitted", counts)
			}
			again, err := source.Begin(ctx, scope, uuid.MustParse(state.Epoch))
			if err != nil || !reflect.DeepEqual(again, state) {
				t.Fatal("same epoch did not resume", err)
			}
			if _, err := source.Begin(ctx, scope, uuid.New()); !errors.Is(err, pg.ErrPrivateSnapshotEpoch) {
				t.Fatal("foreign epoch replaced source", err)
			}
			other := scope
			other.Subject = "bob"
			if _, err := source.Page(ctx, other, uuid.MustParse(state.Epoch), storage.LegacySourceCursor{}, 2); err == nil {
				t.Fatal("other owner opened source")
			}
			if err := st.RestaurarDocumento(ctx, doc.ID); !officeBlocked(err) {
				t.Fatal("SQL writer bypassed freeze", err)
			}
			expired, err := st.PapeleraCaducada(ctx, time.Now(), nil, 500)
			if err != nil {
				t.Fatal(err)
			}
			for _, candidate := range expired {
				if candidate.Doc.ID == doc.ID {
					t.Fatal("GC scheduled frozen original")
				}
			}
			invoked := false
			err = st.WithLegacyOfficeObjectWrite(ctx, snapshotKey, func() error { invoked = true; return nil })
			if !officeBlocked(err) || invoked {
				t.Fatal("object effect bypassed SQL owner fence", err)
			}
			for _, query := range []string{`UPDATE documents SET title='replacement' WHERE owner_sub=$1`, `DELETE FROM documents WHERE owner_sub=$1`, `UPDATE doc_comments SET body='replacement' WHERE document_id=(SELECT id FROM documents WHERE owner_sub=$1 LIMIT 1)`} {
				if err := h.PoolSinRLS(t).InMaintenanceTx(ctx, func(tx pgx.Tx) error { _, err := tx.Exec(ctx, query, scope.Subject); return err }); !officeBlocked(err) {
					t.Fatal("maintenance escaped physical owner fence", err)
				}
			}
			rollback, err := fs.ReadFile(db.Migrations, "migrations/0035_private_storage.down.sql")
			if err != nil {
				t.Fatal(err)
			}
			err = h.PoolSinRLS(t).InTx(ctx, func(tx pgx.Tx) error { _, err := tx.Exec(ctx, string(rollback)); return err })
			if !officeBlocked(err) {
				t.Fatal("rollback discarded retained authority", err)
			}
		})
	}
}

func TestOfficePrivateSourceRejectsDamagedObjectWithoutFreezing(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "docs"}
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "original", OwnerSub: scope.Subject})
	if err != nil {
		t.Fatal(err)
	}
	key := "docs/" + doc.ID + "/exports/original.docx"
	_, err = st.RecordBlob(ctx, domain.Blob{DocumentID: doc.ID, Format: domain.FormatOOXML, BlobKey: key, SizeBytes: 4, ContentType: "application/octet-stream", CreatedBy: scope.Subject})
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range [][]byte{nil, []byte("wrong length")} {
		objects := officeObjects{}
		if body != nil {
			objects[key] = body
		}
		source, err := store.NewPrivateSource(st, objects)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := source.Begin(ctx, scope, uuid.New()); err == nil {
			t.Fatal("damaged historical object was frozen")
		}
		state, err := source.State(ctx, scope)
		if err != nil || state.Mode != "legacy" || state.Rows != 0 {
			t.Fatal("failed capture published partial source", err)
		}
		if err := st.RenameDocument(ctx, doc.ID, "still writable"); err != nil {
			t.Fatal("failed capture disabled legacy writes", err)
		}
	}
	if err := st.WithLegacyOfficeObjectWrite(ctx, strings.Replace(key, doc.ID, "../..", 1), func() error { t.Fatal("invalid route executed"); return nil }); err == nil {
		t.Fatal("invalid object route admitted")
	}
}

func TestOfficeLegacyEffectCreatesCommittedFenceBeforeNestedSQL(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	ctx, cancel := context.WithTimeout(tenantCtx("acme"), 3*time.Second)
	defer cancel()
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "original", OwnerSub: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate documents created before this migration, whose state is absent.
	if err := h.PoolSinRLS(t).InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM docs_private_storage_states WHERE tenant='acme' AND owner_sub='alice'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	called := 0
	if err := st.WithLegacyDocumentEffect(ctx, doc.ID, func() error { called++; return st.RenameDocument(ctx, doc.ID, "nested SQL completed") }); err != nil || called != 1 {
		t.Fatal("outer fence blocked its nested SQL", err, called)
	}
	// A callback returning ErrNoRows is not mistaken for a missing fence and must
	// never execute twice after an external effect has already happened.
	called = 0
	err = st.WithLegacyDocumentEffect(ctx, doc.ID, func() error { called++; return pgx.ErrNoRows })
	if !errors.Is(err, pgx.ErrNoRows) || called != 1 {
		t.Fatal("effect replayed on its own SQL error", err, called)
	}
}

func TestOfficePrivateSourceRejectsUnknownContentRelationWithoutFreezing(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "docs"}
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "retained", OwnerSub: scope.Subject})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.PoolSinRLS(t).InTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `CREATE TABLE future_office_content(tenant text,owner_sub text,body text)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	source, err := store.NewPrivateSource(st, officeObjects{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Begin(ctx, scope, uuid.New()); !errors.Is(err, model.ErrSource) {
		t.Fatal("future content was silently omitted", err)
	}
	state, err := source.State(ctx, scope)
	if err != nil || state.Mode != "legacy" || state.Rows != 0 {
		t.Fatal("incomplete inventory froze legacy source", err)
	}
	if err := st.RenameDocument(ctx, doc.ID, "still usable"); err != nil {
		t.Fatal(err)
	}
}

func TestOfficeLegacyEffectPinsRoutingAcrossConcurrentOwnerTransfer(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "retained", OwnerSub: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.WithLegacyDocumentEffect(ctx, doc.ID, func() error {
		transfer, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		err := h.PoolSinRLS(t).InMaintenanceTx(transfer, func(tx pgx.Tx) error {
			_, err := tx.Exec(transfer, `UPDATE documents SET owner_sub='bob' WHERE id=$1`, doc.ID)
			return err
		})
		if err == nil || !errors.Is(transfer.Err(), context.DeadlineExceeded) {
			t.Fatal("owner transfer overtook external effect", err)
		}
		return st.RenameDocument(ctx, doc.ID, "nested ordinary SQL remains usable")
	}); err != nil {
		t.Fatal(err)
	}
	current, err := st.GetDocument(ctx, doc.ID)
	if err != nil || current.OwnerSub != "alice" {
		t.Fatal("routing changed while effect was fenced", err)
	}
}

func TestOfficePrivateSourceReportsOwnViewsOnSharedDocuments(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "sheets"}
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindSpreadsheet, Title: "shared", OwnerSub: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO doc_vistas_filtro(document_id,tenant,sub,estado)VALUES($1,'acme','alice','{}')`, doc.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	source, err := store.NewPrivateSource(st, officeObjects{})
	if err != nil {
		t.Fatal(err)
	}
	state, err := source.State(ctx, scope)
	if err != nil || !state.Shared {
		t.Fatal("foreign-document view looked owner-only", err)
	}
}
