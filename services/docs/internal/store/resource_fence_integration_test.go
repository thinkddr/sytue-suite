// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/services/docs/db"
)

func e2eFenceBlocked(err error) bool {
	var e *pgconn.PgError
	return errors.As(err, &e) && e.Code == "PST02"
}

// These are database-fence tests, not fake signature acceptance. Runtime routes
// additionally require the shared root-pinned resource VerifiedAccess/Publication.
func TestOfficeEncryptedResourceDurableHoldAndAtomicReceipt(t *testing.T) {
	h := harnessComoEnProduccion(t)
	ctx := tenantCtx("acme")
	hash := strings.Repeat("a", 64)
	nextHash := strings.Repeat("b", 64)
	policy := strings.Repeat("c", 64)
	const initial = int64(9007199254740993)
	run := func(q string, args ...any) error {
		return h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error { _, e := tx.Exec(ctx, q, args...); return e })
	}
	must := func(q string, args ...any) {
		t.Helper()
		if e := run(q, args...); e != nil {
			t.Fatal(e)
		}
	}
	rejected := func(q string, args ...any) {
		t.Helper()
		if e := run(q, args...); !e2eFenceBlocked(e) {
			t.Fatalf("expected durable fence rejection: %v", e)
		}
	}
	if e := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `INSERT INTO resource_e2e_resources(tenant,product,kind,resource_id,epoch,generation,policy_sha256,membership_sha256,private_rules,rules_version,baseline_seq,baseline_head_cipher_sha256,seq,head_cipher_sha256)VALUES('acme','docs','document','doc-1','epoch-1',1,$1,$1,true,$1,$2,$3,$2,$3)`, policy, initial, hash); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `INSERT INTO resource_e2e_epochs(tenant,product,kind,resource_id,epoch,generation,policy_sha256,signed_policy)VALUES('acme','docs','document','doc-1','epoch-1',1,$1,$2)`, policy, []byte(`{"public":"fixture metadata only"}`))
		return e
	}); e != nil {
		t.Fatal(e)
	}
	candidate := func(id string) {
		t.Helper()
		attempt := uuid.NewString()
		must(`INSERT INTO resource_e2e_candidates(tenant,product,kind,resource_id,op_id,epoch,generation,publication_sha256,candidate_sha256,signed_publication,encrypted_event,session_certificate,base_seq,base_head_cipher_sha256,op_count,outer_cipher_sha256,state,attempt_id)VALUES('acme','docs','document','doc-1',$1,'epoch-1',1,$2,$2,$3,$4,$8,$5,$6,2,$7,'prepared',$9)`, id, policy, []byte(`{"room_id":"!room:example.test","transaction_id":"`+attempt+`"}`), []byte(`{ "algorithm":"m.megolm.v1.aes-sha2", "ciphertext":"opaque-sdk-ciphertext", "session_id":"sdk-session" }`), initial, hash, nextHash, []byte(`{"fixture":"session certificate"}`), attempt)
	}
	candidate("op-1")
	candidate("op-2")
	// Crash after stage: exact bytes/attempt survive; seq stays at its full int64.
	var attempt string
	var payload []byte
	var seq int64
	if e := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if e := tx.QueryRow(ctx, `SELECT attempt_id::text,encrypted_event FROM resource_e2e_candidates WHERE op_id='op-1'`).Scan(&attempt, &payload); e != nil {
			return e
		}
		return tx.QueryRow(ctx, `SELECT seq FROM resource_e2e_resources`).Scan(&seq)
	}); e != nil {
		t.Fatal(e)
	}
	if attempt == "" || seq != initial || !strings.Contains(string(payload), "{ ") {
		t.Fatal("stage changed sequence or canonicalized durable ciphertext")
	}
	// No certificate: acquire must roll back both candidate state and hold.
	if e := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `UPDATE resource_e2e_candidates SET state='sending',reservation=convert_to('{"db_guard_fixture":true}','UTF8'),reservation_sha256=encode(digest(convert_to('{"db_guard_fixture":true}','UTF8'),'sha256'),'hex') WHERE op_id='op-1'`); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `UPDATE resource_e2e_resources SET hold_op_id='op-1'`)
		return e
	}); !e2eFenceBlocked(e) {
		t.Fatal("private rules bypassed", e)
	}
	must(`UPDATE resource_e2e_candidates SET validation_certificate=$1 WHERE op_id='op-1'`, []byte(`{"fixture":"verified by runtime before SQL"}`))
	if e := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `UPDATE resource_e2e_candidates SET state='sending',reservation=convert_to('{"db_guard_fixture":true}','UTF8'),reservation_sha256=encode(digest(convert_to('{"db_guard_fixture":true}','UTF8'),'sha256'),'hex') WHERE op_id='op-1'`); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `UPDATE resource_e2e_resources SET hold_op_id='op-1'`)
		return e
	}); e != nil {
		t.Fatal(e)
	}
	// A second producer, a TTL release, replacement bytes and fake commit fail.
	rejected(`UPDATE resource_e2e_resources SET hold_op_id='op-2'`)
	rejected(`UPDATE resource_e2e_resources SET hold_op_id=NULL`)
	rejected(`UPDATE resource_e2e_candidates SET encrypted_event=$1 WHERE op_id='op-1'`, []byte(`{"algorithm":"m.megolm.v1.aes-sha2","ciphertext":"replacement","session_id":"same"}`))
	rejected(`UPDATE resource_e2e_candidates SET state='committed' WHERE op_id='op-1'`)
	rejected(`UPDATE resource_e2e_candidates SET state='cancelled' WHERE op_id='op-1'`)
	must(`UPDATE resource_e2e_resources SET state='draining'`)
	must(`UPDATE resource_e2e_candidates SET state='ambiguous',updated_at=now()-interval '365 days' WHERE op_id='op-1'`)
	rejected(`UPDATE resource_e2e_resources SET state='revoked'`)
	rejected(`UPDATE resource_e2e_resources SET epoch='epoch-2',generation=2`)
	rejected(`UPDATE resource_e2e_resources SET hold_op_id=NULL`)
	receipt := `INSERT INTO resource_e2e_receipts(tenant,product,kind,resource_id,op_id,publication_sha256,epoch,base_seq,seq,base_head_cipher_sha256,head_cipher_sha256,room_id,event_id,receipt)VALUES('acme','docs','document','doc-1','op-1',$1,'epoch-1',$2,$3,$4,$5,'!room:example.test','$accepted',$6)`
	rejected(receipt, policy, initial, initial+3, hash, nextHash, []byte(`{"fixture":"receipt"}`))
	if e := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, receipt, policy, initial, initial+2, hash, nextHash, []byte(`{"fixture":"receipt"}`)); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `UPDATE resource_e2e_candidates SET state='committed' WHERE op_id='op-1'`); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `UPDATE resource_e2e_resources SET seq=$1,head_cipher_sha256=$2,hold_op_id=NULL`, initial+2, nextHash)
		return e
	}); e != nil {
		t.Fatal(e)
	}
	rejected(`UPDATE resource_e2e_receipts SET event_id='$replace'`)
	rejected(`DELETE FROM resource_e2e_receipts`)
	rejected(`UPDATE resource_e2e_resources SET seq=$1`, initial)
	// Drain can now finish rotation, preserving the precise committed position.
	if e := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `INSERT INTO resource_e2e_epochs(tenant,product,kind,resource_id,epoch,generation,policy_sha256,signed_policy)VALUES('acme','docs','document','doc-1','epoch-2',2,$1,$2)`, nextHash, []byte(`{"fixture":"next signed epoch"}`)); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `UPDATE resource_e2e_resources SET epoch='epoch-2',generation=2,policy_sha256=$1,state='active'`, nextHash)
		return e
	}); e != nil {
		t.Fatal(e)
	}
	var visible int
	if e := h.Pool.InTenantTx(tenantCtx("other"), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM resource_e2e_receipts`).Scan(&visible)
	}); e != nil || visible != 0 {
		t.Fatal("cross-tenant receipt visible", e, visible)
	}
	down, e := fs.ReadFile(db.Migrations, "migrations/0036_encrypted_resources.down.sql")
	if e != nil {
		t.Fatal(e)
	}
	if e = h.PoolSinRLS(t).InTx(ctx, func(tx pgx.Tx) error { _, e := tx.Exec(ctx, string(down)); return e }); !e2eFenceBlocked(e) {
		t.Fatal("rollback discarded durable encrypted history", e)
	}
}

func TestOfficeEncryptedResourceRejectsPlaintextAndMissingCipherFields(t *testing.T) {
	h := harnessComoEnProduccion(t)
	ctx := tenantCtx("acme")
	for _, raw := range []string{`{}`, `{"body":"secret"}`, `{"algorithm":"m.megolm.v1.aes-sha2","session_id":"s"}`, `{"algorithm":"m.megolm.v1.aes-sha2","ciphertext":"x","session_id":"s","msgtype":"m.text"}`} {
		var valid bool
		if e := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT sytue_resource_e2e_ciphertext_only($1)`, []byte(raw)).Scan(&valid)
		}); e != nil {
			t.Fatal(e)
		}
		if valid {
			t.Fatal("plaintext/missing-field content accepted", raw)
		}
	}
}
