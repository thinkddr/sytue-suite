//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

type rootAuthorityGate struct {
	matrixGate
	keys  *matrixprooftest.Fixture
	calls int
}

func (g *rootAuthorityGate) IngressDeviceOrigin(_ context.Context, scope storage.Scope, target storage.PublicationTarget) (matrixproof.Origin, error) {
	g.calls++
	if scope != g.keys.Origin.Scope || target.DeviceID != g.keys.Origin.DeviceID.String() {
		return matrixproof.Origin{}, domain.ErrForbidden
	}
	return g.keys.Origin, nil
}

type authorityFixture struct {
	h      *pgtest.Harness
	st     *store.Store
	gate   *rootAuthorityGate
	scope  storage.Scope
	target storage.PublicationTarget
	epoch  string
}

func newAuthorityFixture(t *testing.T) *authorityFixture {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	keys := matrixprooftest.New(t, scope, "matrix.test", id.DeviceID("OWNER_DEVICE"))
	gate := &rootAuthorityGate{keys: keys}
	st.SetMatrixGate(gate)
	rooms, err := st.EnsureMatrixStorageRoomsForIdentity(tenantCtx(scope.Tenant), scope.Subject, scope.Product, storage.IsolatedIdentity, func(_ context.Context, room string) (string, error) {
		if room == "" {
			return "!private:matrix.test", nil
		}
		return room, nil
	})
	if err != nil || len(rooms) != 1 {
		t.Fatal(err)
	}
	return &authorityFixture{h: h, st: st, gate: gate, scope: scope, target: storage.PublicationTarget{RoomID: rooms[0], DeviceID: keys.Origin.DeviceID.String()}, epoch: uuid.NewString()}
}

func (f *authorityFixture) challenge(t *testing.T) matrixproof.RootChallenge {
	t.Helper()
	challenge, err := f.st.MatrixRootChallenge(tenantCtx(f.scope.Tenant), f.scope.Subject, f.scope.Product, f.epoch, f.target)
	if err != nil {
		t.Fatal(err)
	}
	return challenge
}

func (f *authorityFixture) sign(t *testing.T, action matrixproof.Action) matrixproof.SignedAction {
	t.Helper()
	proof := matrixproof.SignedAction{Action: action}
	proof.Signatures = matrixprooftest.Sign(t, f.gate.keys.Device, proof, f.gate.keys.Origin.UserID, f.gate.keys.Origin.DeviceID.String())
	return proof
}

func TestMatrixAuthorityFirstPinRequiresRetainedConsentAndCannotBeReplaced(t *testing.T) {
	f := newAuthorityFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	if _, err := f.st.MatrixStorageAuthority(ctx, f.scope.Subject, f.scope.Product); !errors.Is(err, matrixproof.ErrRootEnrollment) {
		t.Fatal("missing root was manufactured", err)
	}
	challenge := f.challenge(t)
	proof := f.sign(t, challenge.Action)
	for _, fault := range []string{"root", "request", "room", "device", "epoch", "unsigned", "foreign-owner", "foreign-tenant", "inactive"} {
		t.Run(fault, func(t *testing.T) {
			altered := challenge.Action
			actor, tenant := f.scope.Subject, f.scope.Tenant
			switch fault {
			case "root":
				altered.RootKey = f.gate.keys.Self.PublicKey().String()
			case "request":
				altered.RequestSHA256, _ = matrixproof.RequestDigest(map[string]string{"scope": "other"})
			case "room":
				altered.Target.RoomID = "!other:matrix.test"
			case "device":
				altered.Target.DeviceID = "OTHER_DEVICE"
			case "epoch":
				altered.Epoch = uuid.NewString()
			case "foreign-owner":
				actor = "bob"
			case "foreign-tenant":
				tenant = "other"
			case "inactive":
				f.gate.keys.Origin.DeviceActive = false
			}
			signed := f.sign(t, altered)
			if fault == "unsigned" {
				signed.Signatures = nil
			}
			if _, err := f.st.ConfirmMatrixRoot(tenantCtx(tenant), actor, f.scope.Product, signed); err == nil {
				t.Fatal("changed consent pinned a root", fault)
			}
			f.gate.keys.Origin.DeviceActive = true
			if _, err := f.st.MatrixStorageAuthority(ctx, f.scope.Subject, f.scope.Product); !errors.Is(err, matrixproof.ErrRootEnrollment) {
				t.Fatal("failed consent changed root", err)
			}
		})
	}
	var first matrixproof.Authority
	for range 2 {
		result, err := f.st.ConfirmMatrixRoot(ctx, f.scope.Subject, f.scope.Product, proof)
		if err != nil || result.Generation != 1 || result.RootKey != f.gate.keys.Master.PublicKey().String() || result.Status != "active" {
			t.Fatal("consent/retry changed root", err)
		}
		if first.Version != 0 && first != result {
			t.Fatal("retry replaced authority")
		}
		first = result
	}
	if _, err := f.st.MatrixRootChallenge(ctx, f.scope.Subject, f.scope.Product, f.epoch, f.target); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("JWT-only challenge could replace existing pin", err)
	}
	if err := f.h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, f.scope.Tenant); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('app.matrix_authority_owner',$1,true),set_config('app.matrix_authority_product',$2,true)`, f.scope.Subject, f.scope.Product); err != nil {
			return err
		}
		var histories, consumed int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM matrix_storage_authority_history WHERE tenant=$1 AND subject=$2 AND product=$3`, f.scope.Tenant, f.scope.Subject, f.scope.Product).Scan(&histories); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM matrix_storage_root_consents WHERE tenant=$1 AND subject=$2 AND product=$3 AND consumed_at IS NOT NULL`, f.scope.Tenant, f.scope.Subject, f.scope.Product).Scan(&consumed); err != nil {
			return err
		}
		if histories != 1 || consumed != 1 {
			return fmt.Errorf("consent/evidence duplicated: %d/%d", histories, consumed)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMatrixAuthoritySQLRejectsRollbackMissingEvidenceAndTenantEscape(t *testing.T) {
	f := newAuthorityFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	challenge := f.challenge(t)
	if _, err := f.st.ConfirmMatrixRoot(ctx, f.scope.Subject, f.scope.Product, f.sign(t, challenge.Action)); err != nil {
		t.Fatal(err)
	}
	if err := f.h.Pool.InTenantTx(tenantCtx("other"), func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.matrix_authority_owner',$1,true),set_config('app.matrix_authority_product',$2,true)`, f.scope.Subject, f.scope.Product); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM matrix_storage_authorities`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return errors.New("root escaped tenant RLS")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`UPDATE matrix_storage_authorities SET generation=generation+1,status='revoked',updated_at=now() WHERE tenant=$1 AND subject=$2 AND product=$3`,
		`DELETE FROM matrix_storage_authority_history WHERE tenant=$1 AND subject=$2 AND product=$3`,
	} {
		err := f.h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
			if err := pg.SetTenant(ctx, tx, f.scope.Tenant); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `SELECT set_config('app.matrix_authority_owner',$1,true),set_config('app.matrix_authority_product',$2,true),set_config('app.matrix_authority_change','revoke',true)`, f.scope.Subject, f.scope.Product); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, query, f.scope.Tenant, f.scope.Subject, f.scope.Product)
			return err
		})
		if err == nil {
			t.Fatal("SQL discarded authority evidence")
		}
	}
	rollback, err := fs.ReadFile(db.Migrations, db.MigrationsDir+"/0067_matrix_storage_authorities.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, f.scope.Tenant); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, string(rollback))
		return err
	}); err == nil {
		t.Fatal("ordinary down migration removed root history")
	}
	actual, err := f.st.MatrixStorageAuthority(ctx, f.scope.Subject, f.scope.Product)
	if err != nil || actual.Generation != 1 || actual.Status != "active" {
		t.Fatal("failed SQL left partial root mutation", err)
	}
}

func TestMatrixAuthorityRevocationGenerationCommitsBeforeRemoteWithdrawal(t *testing.T) {
	for _, kind := range []string{"identity", "tenant"} {
		t.Run(kind, func(t *testing.T) {
			f := newAuthorityFixture(t)
			ctx := tenantCtx(f.scope.Tenant)
			challenge := f.challenge(t)
			if _, err := f.st.ConfirmMatrixRoot(ctx, f.scope.Subject, f.scope.Product, f.sign(t, challenge.Action)); err != nil {
				t.Fatal(err)
			}
			apply := func() error {
				if kind == "identity" {
					return f.st.ApplyMatrixAccountAccess(ctx, f.scope.Subject, "suspended", 3, func(context.Context) error { return nil })
				}
				return f.st.ApplyMatrixTenantAccess(ctx, f.scope.Subject, "removed", 3, func(context.Context) error { return nil })
			}
			remoteFailure := errors.New("artificial remote withdrawal failure")
			f.gate.err = remoteFailure
			if err := apply(); !errors.Is(err, remoteFailure) {
				t.Fatal("remote withdrawal failure not exercised", err)
			}
			if _, err := f.st.MatrixStorageAuthority(ctx, f.scope.Subject, f.scope.Product); !errors.Is(err, domain.ErrForbidden) {
				t.Fatal("failed remote withdrawal reopened authority", err)
			}
			f.gate.err = nil
			if err := apply(); err != nil {
				t.Fatal("idempotent lifecycle retry failed", err)
			}
			if _, err := f.st.MatrixStorageAuthority(ctx, f.scope.Subject, f.scope.Product); !errors.Is(err, domain.ErrForbidden) {
				t.Fatal("denied actor retained authority", err)
			}
			if err := f.h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
				if err := pg.SetTenant(ctx, tx, f.scope.Tenant); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `SELECT set_config('app.matrix_authority_owner',$1,true),set_config('app.matrix_authority_product',$2,true)`, f.scope.Subject, f.scope.Product); err != nil {
					return err
				}
				var generation int64
				var status string
				if err := tx.QueryRow(ctx, `SELECT generation,status FROM matrix_storage_authorities WHERE tenant=$1 AND subject=$2 AND product=$3`, f.scope.Tenant, f.scope.Subject, f.scope.Product).Scan(&generation, &status); err != nil {
					return err
				}
				if generation != 2 || status != "revoked" {
					return fmt.Errorf("stale authority after denial: %d/%s", generation, status)
				}
				var histories int
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM matrix_storage_authority_history WHERE tenant=$1 AND subject=$2 AND product=$3`, f.scope.Tenant, f.scope.Subject, f.scope.Product).Scan(&histories); err != nil {
					return err
				}
				if histories != 2 {
					return errors.New("revocation history duplicated or missing")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMatrixAuthorityActionRequiresSignedCurrentGeneration(t *testing.T) {
	f := newAuthorityFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	challenge := f.challenge(t)
	if _, err := f.st.ConfirmMatrixRoot(ctx, f.scope.Subject, f.scope.Product, f.sign(t, challenge.Action)); err != nil {
		t.Fatal(err)
	}
	action := challenge.Action
	action.Kind = "task.claim"
	action.Nonce = uuid.NewString()
	action.RequestSHA256, _ = matrixproof.RequestDigest(map[string]string{"task": "synthetic"})
	proof := f.sign(t, action)
	result, err := f.st.VerifyMatrixStorageAction(ctx, f.scope.Subject, f.scope.Product, action, proof)
	if err != nil || result.Validate(action) != nil {
		t.Fatal("current signed generation denied", err)
	}
	stale := action
	stale.Generation++
	if _, err := f.st.VerifyMatrixStorageAction(ctx, f.scope.Subject, f.scope.Product, stale, f.sign(t, stale)); err == nil {
		t.Fatal("request supplied a new authority generation")
	}
	f.gate.keys.Origin.DeviceActive = false
	if _, err := f.st.VerifyMatrixStorageAction(ctx, f.scope.Subject, f.scope.Product, action, proof); err == nil {
		t.Fatal("old proof survived remote device revocation")
	}
	f.gate.keys.Origin.DeviceActive = true
	if err := f.st.ApplyMatrixTenantAccess(ctx, f.scope.Subject, "removed", 4, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.st.VerifyMatrixStorageAction(ctx, f.scope.Subject, f.scope.Product, action, proof); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("old proof survived tenant revocation", err)
	}
}

func TestMatrixAuthorityResumeKeepsPinAndRejectsOldGeneration(t *testing.T) {
	f := newAuthorityFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	first := f.challenge(t)
	if _, err := f.st.ConfirmMatrixRoot(ctx, f.scope.Subject, f.scope.Product, f.sign(t, first.Action)); err != nil {
		t.Fatal(err)
	}
	if err := f.st.ApplyMatrixTenantAccess(ctx, f.scope.Subject, "removed", 3, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := f.st.ApplyMatrixTenantAccess(ctx, f.scope.Subject, "active", 4, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	challenge, err := f.st.MatrixRootChangeChallenge(ctx, f.scope.Subject, f.scope.Product, "root.resume", f.epoch, f.target)
	if err != nil {
		t.Fatal(err)
	}
	if challenge.Action.Generation != 2 || challenge.Request.PreviousGeneration != 2 || challenge.Request.Generation != 3 {
		t.Fatal("resume lost authority generation")
	}
	for range 2 {
		result, err := f.st.ConfirmMatrixRoot(ctx, f.scope.Subject, f.scope.Product, f.sign(t, challenge.Action))
		if err != nil || result.Generation != 3 || result.RootKey != first.Request.RootKey || result.Status != "active" {
			t.Fatal("resume/retry replaced pin", err)
		}
	}
	stale := challenge.Action
	stale.Kind = "task.claim"
	stale.Nonce = uuid.NewString()
	if _, err := f.st.VerifyMatrixStorageAction(ctx, f.scope.Subject, f.scope.Product, stale, f.sign(t, stale)); err == nil {
		t.Fatal("old signed generation survived resume")
	}
}

func TestMatrixAuthorityRotationRequiresSignatureFromRetainedOldRoot(t *testing.T) {
	f := newAuthorityFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	first := f.challenge(t)
	old := f.gate.keys
	if _, err := f.st.ConfirmMatrixRoot(ctx, f.scope.Subject, f.scope.Product, f.sign(t, first.Action)); err != nil {
		t.Fatal(err)
	}
	next := matrixprooftest.New(t, f.scope, "matrix.test", id.DeviceID(f.target.DeviceID))
	f.gate.keys = next
	challenge, err := f.st.MatrixRootChangeChallenge(ctx, f.scope.Subject, f.scope.Product, "root.rotate", f.epoch, f.target)
	if err != nil {
		t.Fatal(err)
	}
	proof := f.sign(t, challenge.Action)
	if _, err := f.st.ConfirmMatrixRoot(ctx, f.scope.Subject, f.scope.Product, proof); err == nil {
		t.Fatal("directory/JWT replaced retained root without old-root signature")
	}
	master := next.Origin.Keys.MasterKeys[next.Origin.UserID]
	master.Signatures = matrixprooftest.Sign(t, old.Master, master, next.Origin.UserID, old.Master.PublicKey().String())
	next.Origin.Keys.MasterKeys[next.Origin.UserID] = master
	for range 2 {
		result, err := f.st.ConfirmMatrixRoot(ctx, f.scope.Subject, f.scope.Product, proof)
		if err != nil || result.Generation != 2 || result.RootKey != next.Master.PublicKey().String() || result.Status != "active" {
			t.Fatal("old-root-authorized rotation/retry failed", err)
		}
	}
	before := f.gate.calls
	stale := first.Action
	stale.Kind = "task.claim"
	stale.Nonce = uuid.NewString()
	signed := matrixproof.SignedAction{Action: stale}
	signed.Signatures = matrixprooftest.Sign(t, old.Device, signed, old.Origin.UserID, old.Origin.DeviceID.String())
	if _, err := f.st.VerifyMatrixStorageAction(ctx, f.scope.Subject, f.scope.Product, stale, signed); err == nil {
		t.Fatal("rotated old root still authorized work")
	}
	if f.gate.calls != before {
		t.Fatal("stale generation reached remote directory")
	}
}
