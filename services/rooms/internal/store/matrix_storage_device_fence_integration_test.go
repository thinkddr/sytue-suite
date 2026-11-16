// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func deviceFenceFixture(t *testing.T) (*authorityFixture, ingress.ControlProof) {
	t.Helper()
	f := newAuthorityFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	challenge := f.challenge(t)
	if _, err := f.st.ConfirmMatrixRoot(ctx, f.scope.Subject, f.scope.Product, f.sign(t, challenge.Action)); err != nil {
		t.Fatal(err)
	}
	previous := ingress.Assignment{Target: storage.PublicationTarget{RoomID: f.target.RoomID, DeviceID: "ORIGINAL_A"}, Generation: 7, SigningKey: f.gate.keys.Self.PublicKey().String(), ExpiresAt: time.Now().UTC().Add(-time.Minute)}
	request := ingress.ControlRequest{Version: 1, Scope: f.scope, Epoch: f.epoch, Kind: "task.handoff", Target: f.target, Handoff: &ingress.HandoffRequest{ID: uuid.NewString(), Phase: "begin", TaskID: uuid.NewString(), CipherSHA256: strings.Repeat("a", 64), Previous: previous, Token: uuid.NewString()}}
	return f, signDeviceFence(t, f, request)
}

func signDeviceFence(t *testing.T, f *authorityFixture, request ingress.ControlRequest) ingress.ControlProof {
	t.Helper()
	action := f.challengeAction(t)
	action.Kind = "task.handoff"
	action.Target = request.Target
	action.Epoch = request.Epoch
	action.Nonce = uuid.NewString()
	digest, err := matrixproof.RequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	action.RequestSHA256 = digest
	return ingress.ControlProof{Request: request, Proof: f.sign(t, action)}
}

func (f *authorityFixture) challengeAction(t *testing.T) matrixproof.Action {
	t.Helper()
	authority, err := f.st.MatrixStorageAuthority(tenantCtx(f.scope.Tenant), f.scope.Subject, f.scope.Product)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	return matrixproof.Action{Version: 1, Generation: authority.Generation, Scope: f.scope, RootKey: authority.RootKey, IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(4 * time.Minute).UnixMilli()}
}

func TestStorageDeviceFenceCommitsPlanBeforeIOAndRetainsFailedConfirmation(t *testing.T) {
	f, proof := deviceFenceFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	planned, err := f.st.PlanMatrixStorageDeviceFence(ctx, proof)
	if err != nil || planned.Status != "planned" || planned.ValidateProof(proof) != nil {
		t.Fatal("durable plan", err)
	}
	calls := 0
	unavailable := errors.New("synthetic Matrix revoke acknowledgement lost")
	verify := func(ctx context.Context, scope storage.Scope, previous ingress.Assignment, revoke bool) error {
		calls++
		if scope != f.scope || !previous.Same(proof.Request.Handoff.Previous) || !revoke {
			t.Fatal("changed original fence")
		}
		// An independent connection must see the plan before privileged Matrix IO.
		conn, err := pgx.Connect(ctx, f.h.DSN)
		if err != nil {
			return err
		}
		defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()
		var status, raw, evidence string
		if err := conn.QueryRow(ctx, `SELECT status,fence::text,evidence::text FROM matrix_storage_device_fences WHERE tenant=$1 AND id=$2`, scope.Tenant, proof.Request.Handoff.ID).Scan(&status, &raw, &evidence); err != nil {
			return err
		}
		if status != "planned" || strings.Contains(raw+evidence, proof.Request.Handoff.Token) {
			t.Fatal("plan was uncommitted or retained a claim token")
		}
		var retained struct {
			Proof         matrixproof.SignedAction
			Authorization matrixproof.Authorization
		}
		if err := json.Unmarshal([]byte(evidence), &retained); err != nil {
			return err
		}
		if retained.Proof.Action != proof.Proof.Action || retained.Authorization.Validate(proof.Proof.Action) != nil || retained.Authorization.SigningKey != f.gate.keys.Device.PublicKey().String() {
			t.Fatal("verified public consent was not retained")
		}
		return unavailable
	}
	if _, err := f.st.ConfirmMatrixStorageDeviceFence(ctx, proof, verify); !errors.Is(err, unavailable) || calls != 1 {
		t.Fatal("failed IO confirmation", err, calls)
	}
	state, err := f.st.ReadMatrixStorageDeviceFence(ctx, proof, func(context.Context, storage.Scope, storage.PublicationTarget) error {
		t.Fatal("planned read performed IO")
		return nil
	})
	if err != nil || state.Status != "planned" || state.ConfirmedAt != nil {
		t.Fatal("failure did not retain planned evidence", state, err)
	}
}

func TestStorageDeviceFenceFreshResolveReadChecksOriginAndNeverRevokesAgain(t *testing.T) {
	f, proof := deviceFenceFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	if _, err := f.st.PlanMatrixStorageDeviceFence(ctx, proof); err != nil {
		t.Fatal(err)
	}
	revocations := 0
	confirm := func(_ context.Context, _ storage.Scope, _ ingress.Assignment, revoke bool) error {
		if revoke {
			revocations++
		}
		return nil
	}
	for range 2 {
		confirmed, err := f.st.ConfirmMatrixStorageDeviceFence(ctx, proof, confirm)
		if err != nil || confirmed.Status != "confirmed" || confirmed.ConfirmedAt == nil {
			t.Fatal("confirmation/retry", confirmed, err)
		}
	}
	if revocations != 1 {
		t.Fatal("confirmed retry revoked again", revocations)
	}
	request := proof.Request
	handoff := *request.Handoff
	handoff.Phase = "resolve"
	handoff.Resolution = "absent"
	handoff.LeaseSeconds = 30
	request.Handoff = &handoff
	resolve := signDeviceFence(t, f, request)
	checks := 0
	out, err := f.st.ReadMatrixStorageDeviceFence(ctx, resolve, func(_ context.Context, _ storage.Scope, target storage.PublicationTarget) error {
		checks++
		if target != proof.Request.Handoff.Previous.Target {
			t.Fatal("checked another device")
		}
		return nil
	})
	if err != nil || checks != 1 || out.ValidateProof(resolve) != nil {
		t.Fatal("fresh resolve did not read canonical begin", out, err)
	}
	if _, err := f.st.PlanMatrixStorageDeviceFence(ctx, resolve); err == nil {
		t.Fatal("resolve initiated a destructive fence")
	}
	if _, err := f.st.ReadMatrixStorageDeviceFence(ctx, resolve, func(context.Context, storage.Scope, storage.PublicationTarget) error { return domain.ErrConflict }); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("timestamp replaced independent absence read", err)
	}
}

func TestStorageDeviceFenceRejectsChangedIntentAndRevokedSignerBeforeIO(t *testing.T) {
	f, proof := deviceFenceFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	if _, err := f.st.PlanMatrixStorageDeviceFence(ctx, proof); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"token", "task", "cipher", "original-key", "original-generation"} {
		request := proof.Request
		handoff := *request.Handoff
		request.Handoff = &handoff
		switch field {
		case "token":
			handoff.Token = uuid.NewString()
		case "task":
			handoff.TaskID = uuid.NewString()
		case "cipher":
			handoff.CipherSHA256 = strings.Repeat("b", 64)
		case "original-key":
			handoff.Previous.SigningKey = f.gate.keys.Master.PublicKey().String()
		case "original-generation":
			handoff.Previous.Generation++
		}
		altered := signDeviceFence(t, f, request)
		if _, err := f.st.PlanMatrixStorageDeviceFence(ctx, altered); err == nil {
			t.Fatal("changed handoff replaced durable plan", field)
		}
	}
	f.gate.keys.Origin.DeviceActive = false
	calls := 0
	if _, err := f.st.ConfirmMatrixStorageDeviceFence(ctx, proof, func(context.Context, storage.Scope, ingress.Assignment, bool) error { calls++; return nil }); err == nil || calls != 0 {
		t.Fatal("revoked signer still revoked another device", err, calls)
	}
}

func TestStorageDeviceFenceImmutableEvidenceRLSAndRollback(t *testing.T) {
	f, proof := deviceFenceFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	if _, err := f.st.PlanMatrixStorageDeviceFence(ctx, proof); err != nil {
		t.Fatal(err)
	}
	if err := f.h.Pool.InTenantTx(tenantCtx("foreign"), func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.matrix_authority_owner',$1,true),set_config('app.matrix_authority_product',$2,true)`, f.scope.Subject, f.scope.Product); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM matrix_storage_device_fences`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return errors.New("foreign tenant read device fence")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, f.scope.Tenant); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('app.matrix_authority_owner','foreign',true),set_config('app.matrix_authority_product',$1,true)`, f.scope.Product); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM matrix_storage_device_fences`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return errors.New("foreign subject read device fence")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, f.h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()
	for _, statement := range []string{
		`UPDATE matrix_storage_device_fences SET evidence='{}'::jsonb`,
		`UPDATE matrix_storage_device_fences SET fence='{}'::jsonb`,
		`UPDATE matrix_storage_device_fences SET request_sha256=repeat('b',64)`,
		`UPDATE matrix_storage_device_fences SET status='confirmed',confirmed_at=clock_timestamp()`,
		`DELETE FROM matrix_storage_device_fences`,
	} {
		// No authority-owner GUCs: even a privileged fixture cannot mutate evidence.
		if _, err := conn.Exec(ctx, statement); err == nil {
			t.Fatal("unscoped evidence mutation succeeded", statement)
		}
	}
	rollback, err := fs.ReadFile(db.Migrations, "migrations/0071_storage_device_fences.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, string(rollback)); err == nil {
		t.Fatal("rollback destroyed retained fencing evidence")
	}
	if _, err := conn.Exec(ctx, "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM matrix_storage_device_fences WHERE id=$1`, proof.Request.Handoff.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("failed rollback lost plan", count, err)
	}
	// Under the correct scope, evidence and deletion remain immutable too.
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	for name, value := range map[string]string{"app.tenant": f.scope.Tenant, "app.matrix_authority_owner": f.scope.Subject, "app.matrix_authority_product": f.scope.Product} {
		if _, err := tx.Exec(ctx, `SELECT set_config($1,$2,true)`, name, value); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE matrix_storage_device_fences SET evidence='{}'::jsonb,status='confirmed',confirmed_at=clock_timestamp() WHERE id=$1`, proof.Request.Handoff.ID); err == nil {
		t.Fatal("owner changed retained consent")
	}
}
