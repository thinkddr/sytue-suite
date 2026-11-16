// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func TestMatrixNativeLifecyclePermanentlyRetiresOnlyDeniedScope(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, testCipher(t))
	gate := &matrixGate{}
	st.SetMatrixGate(gate)
	acme, beta := tenantCtx("acme"), tenantCtx("beta")
	const challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	create := func(ctx context.Context, subject string) string {
		t.Helper()
		state, err := st.CreateMatrixNativeConsentForIdentity(ctx, subject, challenge, "drive", storage.IsolatedIdentity)
		if err != nil {
			t.Fatal(err)
		}
		return state
	}
	a, b, other := create(acme, "alice"), create(beta, "alice"), create(acme, "bob")
	ticket, _, err := st.AcceptMatrixNativeConsentWithDestination(acme, "alice", a, "artificial-only-login-token")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ApplyMatrixTenantAccess(acme, "alice", "removed", 10, func(context.Context) error { t.Error("removal requested activation proof"); return nil }); err != nil {
		t.Fatal(err)
	}
	var acmeCount, betaCount, otherCount int
	count := func() {
		t.Helper()
		err := h.Pool.InMaintenanceTx(t.Context(), func(tx pgx.Tx) error {
			return tx.QueryRow(t.Context(), `SELECT count(*) FILTER(WHERE subject='alice' AND tenant='acme'),count(*) FILTER(WHERE subject='alice' AND tenant='beta'),count(*) FILTER(WHERE subject='bob') FROM matrix_native_consents`).Scan(&acmeCount, &betaCount, &otherCount)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	count()
	if acmeCount != 0 || betaCount != 1 || otherCount != 1 {
		t.Fatal("tenant retirement crossed scopes", acmeCount, betaCount, otherCount)
	}
	if len(gate.storageRevocations) != 1 || gate.storageRevocations[0] != [2]string{"alice", "acme"} {
		t.Fatal("scoped remote withdrawal was not applied", gate.storageRevocations)
	}
	if _, err := st.CreateMatrixNativeConsentForIdentity(acme, "alice", challenge, "drive", storage.IsolatedIdentity); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("denied actor created new consent", err)
	}
	if err := st.ApplyMatrixTenantAccess(acme, "alice", "active", 11, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := st.FinishMatrixNativeConsentForIdentity(acme, "alice", a, ticket, challenge, "drive", storage.IsolatedIdentity); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("reactivation recovered a retired ticket", err)
	}
	if _, err := st.PreviewMatrixNativeDestination(beta, "alice", b); err != nil {
		t.Fatal("other organization lost consent", err)
	}
	if _, err := st.PreviewMatrixNativeDestination(acme, "bob", other); err != nil {
		t.Fatal("other actor lost consent", err)
	}
	fresh := create(acme, "alice")
	if err := st.ApplyMatrixAccountAccess(acme, "alice", "suspended", 20, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	count()
	if acmeCount != 0 || betaCount != 0 || otherCount != 1 {
		t.Fatal("global retirement crossed a subject or missed another tenant", acmeCount, betaCount, otherCount)
	}
	if err := st.ApplyMatrixAccountAccess(acme, "alice", "active", 21, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		ctx   context.Context
		state string
	}{{acme, fresh}, {beta, b}} {
		if _, err := st.PreviewMatrixNativeDestination(entry.ctx, "alice", entry.state); !errors.Is(err, domain.ErrForbidden) {
			t.Fatal("global reactivation restored prior consent", err)
		}
	}
	_ = create(acme, "alice")
}

func TestMatrixNativeCreationSerializesWithIdentityWithdrawal(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, testCipher(t))
	st.SetMatrixGate(&matrixGate{})
	ctx, cancel := context.WithTimeout(tenantCtx("acme"), 10*time.Second)
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	revoked := make(chan error, 1)
	go func() {
		revoked <- st.ApplyMatrixAccountAccess(ctx, "alice", "suspended", 30, func(context.Context) error {
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	created := make(chan error, 1)
	go func() {
		_, err := st.CreateMatrixNativeConsentForIdentity(ctx, "alice", "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", "drive", storage.IsolatedIdentity)
		created <- err
	}()
	early := false
	var creationErr error
	select {
	case creationErr = <-created:
		early = true
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-revoked; err != nil {
		t.Fatal(err)
	}
	if !early {
		creationErr = <-created
	}
	if early || !errors.Is(creationErr, domain.ErrForbidden) {
		t.Fatal("consent escaped the identity withdrawal lock", early, creationErr)
	}
}

func TestMatrixTenantStorageRevocationRetryKeepsDenialAndDoesNotRelockActivation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	gate := &matrixGate{err: domain.ErrMatrixUnavailable}
	st.SetMatrixGate(gate)
	ctx := tenantCtx("acme")
	if err := st.ApplyMatrixTenantAccess(ctx, "alice", "removed", 40, func(context.Context) error { return nil }); !errors.Is(err, domain.ErrMatrixUnavailable) {
		t.Fatal("remote withdrawal failure acknowledged", err)
	}
	if err := st.MatrixActorAllowed(ctx, "alice"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("failed remote withdrawal lost durable denial", err)
	}
	gate.err = nil
	if err := st.ApplyMatrixTenantAccess(ctx, "alice", "active", 41, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	before := len(gate.storageRevocations)
	if err := st.ApplyMatrixTenantAccess(ctx, "alice", "removed", 40, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if len(gate.storageRevocations) != before {
		t.Fatal("stale retry relocked reactivated storage")
	}
	if err := st.MatrixActorAllowed(ctx, "alice"); err != nil {
		t.Fatal(err)
	}
}

func TestMatrixTenantStorageWithdrawalLosesToNewerActivationWhileWaitingForIdentity(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	gate := &matrixGate{}
	st.SetMatrixGate(gate)
	acme, cancel := context.WithTimeout(tenantCtx("acme"), 10*time.Second)
	defer cancel()
	beta := tenantCtx("beta")
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	releaseRoot := func() { once.Do(func() { close(release) }) }
	defer releaseRoot()
	holder := make(chan error, 1)
	go func() {
		holder <- st.WithMatrixIdentity(beta, "alice", func(context.Context) error {
			close(entered)
			select {
			case <-release:
				return nil
			case <-acme.Done():
				return acme.Err()
			}
		})
	}()
	select {
	case <-entered:
	case <-acme.Done():
		t.Fatal(acme.Err())
	}
	removed := make(chan error, 1)
	go func() {
		removed <- st.ApplyMatrixTenantAccess(acme, "alice", "removed", 50, func(context.Context) error { return nil })
	}()
	for {
		err := st.MatrixActorAllowed(acme, "alice")
		if errors.Is(err, domain.ErrForbidden) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-time.After(10 * time.Millisecond):
		case <-acme.Done():
			t.Fatal(acme.Err())
		}
	}
	// The durable denial committed; its remote phase still waits on the person
	// lock. A newer activation must win before that waiting withdrawal resumes.
	if err := st.ApplyMatrixTenantAccess(acme, "alice", "active", 51, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	releaseRoot()
	if err := <-holder; err != nil {
		t.Fatal(err)
	}
	if err := <-removed; err != nil {
		t.Fatal(err)
	}
	if len(gate.storageRevocations) != 0 {
		t.Fatal("waiting stale withdrawal relocked an active tenant", gate.storageRevocations)
	}
	if err := st.MatrixActorAllowed(acme, "alice"); err != nil {
		t.Fatal(err)
	}
}
