// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

func TestStorageReceiptChecksPinnedAuthorityAndOwnedTargetBeforeOrigin(t *testing.T) {
	f := newAuthorityFixture(t)
	ctx := tenantCtx(f.scope.Tenant)
	calls := 0
	receipt := storage.Reference{RoomID: f.target.RoomID, EventID: "$encrypted_commit"}
	verify := func(_ context.Context, scope storage.Scope, ref storage.Reference) error {
		calls++
		if scope != f.scope || ref != receipt {
			return errors.New("receipt binding changed before configured origin")
		}
		return nil
	}
	if err := f.st.VerifyMatrixStorageReceipt(ctx, f.scope.Subject, f.scope.Product, f.target, receipt, verify); !errors.Is(err, matrixproof.ErrRootEnrollment) || calls != 0 {
		t.Fatal("unconsented root reached origin verifier", err, calls)
	}
	challenge := f.challenge(t)
	if _, err := f.st.ConfirmMatrixRoot(ctx, f.scope.Subject, f.scope.Product, f.sign(t, challenge.Action)); err != nil {
		t.Fatal(err)
	}
	if err := f.st.VerifyMatrixStorageReceipt(ctx, f.scope.Subject, f.scope.Product, f.target, receipt, verify); err != nil || calls != 1 {
		t.Fatal("authorized ciphertext receipt rejected", err, calls)
	}
	wrong := f.target
	wrong.RoomID = "!foreign:matrix.test"
	other := receipt
	other.RoomID = wrong.RoomID
	if err := f.st.VerifyMatrixStorageReceipt(ctx, f.scope.Subject, f.scope.Product, wrong, other, verify); err == nil || calls != 1 {
		t.Fatal("foreign target reached origin verifier", err, calls)
	}
	if err := f.st.ApplyMatrixAccountAccess(ctx, f.scope.Subject, "suspended", 3, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := f.st.VerifyMatrixStorageReceipt(ctx, f.scope.Subject, f.scope.Product, f.target, receipt, verify); err == nil || calls != 1 {
		t.Fatal("revoked authority accepted receipt", err, calls)
	}
}
