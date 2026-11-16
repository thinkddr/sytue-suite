//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// These fixtures exercise SQL durability/RLS/transitions. SDK signature and
// exact-wire commitments have separate native/Wasm publication conformance tests.
func publicationTx(f *authorityFixture, contract bool, tenant, owner string, fn func(pgx.Tx) error) error {
	ctx := tenantCtx(tenant)
	return f.h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		flag := ""
		if contract {
			flag = "1"
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('app.matrix_authority_owner',$1,true),set_config('app.matrix_authority_product',$2,true),set_config('app.matrix_publication_contract',$3,true)`, owner, f.scope.Product, flag); err != nil {
			return err
		}
		return fn(tx)
	})
}

func insertPublicationFixture(t *testing.T, f *authorityFixture, kind, device, txn, event string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	err := publicationTx(f, true, f.scope.Tenant, f.scope.Subject, func(tx pgx.Tx) error {
		if kind == "legacy" {
			_, err := tx.Exec(t.Context(), `INSERT INTO matrix_storage_publications(tenant,subject,product,id,namespace,origin_kind,matrix_user_id,device_id,room_id,transaction_id,cipher_sha256,authority_generation,status,event_id) VALUES($1,$2,$3,$4,'storage:'||$3,'legacy',$5,'',$6,$7,$8,0,'sending',$9)`, f.scope.Tenant, f.scope.Subject, f.scope.Product, id, f.gate.keys.Origin.UserID.String(), f.target.RoomID, txn, strings.Repeat("a", 64), event)
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO matrix_storage_publications(tenant,subject,product,id,namespace,origin_kind,matrix_user_id,device_id,room_id,transaction_id,marker_id,cipher_sha256,request_sha256,authority_generation,request,producer_evidence,status) VALUES($1,$2,$3,$4,'storage:'||$3,'signed',$5,$6,$7,$8,$9,$10,$10,1,'{}','{}','reserved')`, f.scope.Tenant, f.scope.Subject, f.scope.Product, id, f.gate.keys.Origin.UserID.String(), device, f.target.RoomID, txn, strings.Repeat("A", 43), strings.Repeat("a", 64))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPublicationHoldPreventsDeviceRevocationUntilOriginAccepted(t *testing.T) {
	f, proof := deviceFenceFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	id := insertPublicationFixture(t, f, "legacy", "", "legacy-origin-1", "$accepted-later")
	if _, err := f.st.PlanMatrixStorageDeviceFence(ctx, proof); err != nil {
		t.Fatal(err)
	}
	calls := 0
	verify := func(context.Context, storage.Scope, ingress.Assignment, bool) error { calls++; return nil }
	if _, err := f.st.ConfirmMatrixStorageDeviceFence(ctx, proof, verify); !errors.Is(err, domain.ErrConflict) || calls != 0 {
		t.Fatal("pending origin reached device revocation", err, calls)
	}
	// An older writer omitting the publication RLS opt-in still cannot confirm.
	err := publicationTx(f, false, f.scope.Tenant, f.scope.Subject, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE matrix_storage_device_fences SET status='confirmed',confirmed_at=clock_timestamp() WHERE tenant=$1 AND subject=$2 AND product=$3 AND id=$4`, f.scope.Tenant, f.scope.Subject, f.scope.Product, proof.Request.Handoff.ID)
		return err
	})
	if err == nil {
		t.Fatal("old writer hid pending publication behind RLS")
	}
	if err = publicationTx(f, true, f.scope.Tenant, f.scope.Subject, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE matrix_storage_publications SET status='accepted',accepted_at=clock_timestamp() WHERE id=$1`, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	confirmed, err := f.st.ConfirmMatrixStorageDeviceFence(ctx, proof, verify)
	if err != nil || confirmed.Status != "confirmed" || calls != 1 {
		t.Fatal("accepted origin did not unblock durable fence", err, calls)
	}
	if err = publicationTx(f, true, f.scope.Tenant, f.scope.Subject, func(tx pgx.Tx) error {
		var state, event string
		if err := tx.QueryRow(ctx, `SELECT status,event_id FROM matrix_storage_publications WHERE id=$1`, id).Scan(&state, &event); err != nil {
			return err
		}
		if state != "accepted" || event != "$accepted-later" {
			return fmt.Errorf("origin evidence changed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPublicationPlanCancelsReservedAndRetainsImmutableEvidence(t *testing.T) {
	f, proof := deviceFenceFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	id := insertPublicationFixture(t, f, "signed", proof.Request.Handoff.Previous.Target.DeviceID, "reserved-before-plan", "")
	if _, err := f.st.PlanMatrixStorageDeviceFence(ctx, proof); err != nil {
		t.Fatal(err)
	}
	if err := publicationTx(f, true, f.scope.Tenant, f.scope.Subject, func(tx pgx.Tx) error {
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM matrix_storage_publications WHERE id=$1`, id).Scan(&status); err != nil {
			return err
		}
		if status != "cancelled" {
			return fmt.Errorf("planned fence did not cancel reservation")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`UPDATE matrix_storage_publications SET status='sending',event_id='$too-late' WHERE id=$1`,
		`UPDATE matrix_storage_publications SET cipher_sha256=repeat('b',64) WHERE id=$1`,
		`UPDATE matrix_storage_publications SET request='{"changed":true}' WHERE id=$1`,
		`UPDATE matrix_storage_publications SET namespace='resources:mail' WHERE id=$1`,
		`DELETE FROM matrix_storage_publications WHERE id=$1`,
	} {
		if err := publicationTx(f, true, f.scope.Tenant, f.scope.Subject, func(tx pgx.Tx) error { _, err := tx.Exec(ctx, query, id); return err }); err == nil {
			t.Fatal("retired or altered publication admitted", query)
		}
	}
	calls := 0
	if _, err := f.st.ConfirmMatrixStorageDeviceFence(ctx, proof, func(context.Context, storage.Scope, ingress.Assignment, bool) error { calls++; return nil }); err != nil || calls != 1 {
		t.Fatal("cancelled pre-origin reservation blocked completion", err)
	}
}

func TestPublicationRLSAndNullableFieldsFailClosed(t *testing.T) {
	f, _ := deviceFenceFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	insertPublicationFixture(t, f, "legacy", "", "rls-origin-1", "$rls-origin")
	for _, view := range []struct {
		contract      bool
		tenant, owner string
	}{{false, f.scope.Tenant, f.scope.Subject}, {true, "other", f.scope.Subject}, {true, f.scope.Tenant, "other"}} {
		if err := publicationTx(f, view.contract, view.tenant, view.owner, func(tx pgx.Tx) error {
			var count int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM matrix_storage_publications`).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return fmt.Errorf("publication metadata escaped RLS")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{
		`INSERT INTO matrix_storage_publications(tenant,subject,product,id,namespace,origin_kind,matrix_user_id,device_id,room_id,transaction_id,cipher_sha256,authority_generation,status) VALUES($1,$2,$3,$4,'storage:'||$3,'signed',$5,'DEVICE',$6,'missing-public-evidence',repeat('a',64),1,'reserved')`,
		`INSERT INTO matrix_storage_publications(tenant,subject,product,id,namespace,origin_kind,matrix_user_id,device_id,room_id,transaction_id,cipher_sha256,authority_generation,status) VALUES($1,$2,$3,$4,'storage:'||$3,'legacy',$5,'',$6,'missing-event',repeat('a',64),0,'sending')`,
	} {
		if err := publicationTx(f, true, f.scope.Tenant, f.scope.Subject, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, query, f.scope.Tenant, f.scope.Subject, f.scope.Product, uuid.New(), f.gate.keys.Origin.UserID.String(), f.target.RoomID)
			return err
		}); err == nil {
			t.Fatal("nullable constraint admitted incomplete evidence")
		}
	}
}
