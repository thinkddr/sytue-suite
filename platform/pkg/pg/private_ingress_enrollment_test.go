// SPDX-License-Identifier: Apache-2.0
//go:build integration

package pg_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

func TestPrivateIngressEnrollmentNeverResurrectsRevokedLatestGeneration(t *testing.T) {
	h, cfg, reader, _ := newIngressControlFixture(t)
	ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: cfg.Scope.Tenant})
	queue := ingressQueueFixture(h, cfg)
	state, err := queue.Enrollment(ctx, cfg.Scope, cfg.Epoch.String())
	if err != nil || state.Status != "active" || state.Plan == nil || *state.Plan != h.plan {
		t.Fatal("current enrollment changed", state, err)
	}
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	latest := h.plan
	latest.Generation = 2
	latest.Recipient = identity.Recipient().String()
	latest.KeyID = ingress.PublicFingerprint(latest.Recipient)
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		if err := pg.EnrollPrivateIngress(ctx, tx, h.cfg, latest, h.target, strings.Repeat("A", 43)); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE private_ingress_key_plans SET revoked_at=clock_timestamp() WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND key_generation=2`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	state, err = queue.Enrollment(ctx, cfg.Scope, cfg.Epoch.String())
	if err != nil || state.Status != "revoked" || state.Generation != 2 || state.Plan != nil {
		t.Fatal("latest revoked generation revived or hidden", state, err)
	}
	absent := cfg
	absent.Epoch = uuid.New()
	state, err = ingressQueueFixture(h, absent).Enrollment(ctx, absent.Scope, absent.Epoch.String())
	if !errors.Is(err, ingress.ErrEnrollmentEpoch) || state.Status != "" {
		t.Fatal("retained enrollment was reinterpreted as a new epoch", state, err)
	}
	// A different owner has an independent, genuinely absent enrollment.
	empty := h.cfg
	empty.Scope.Subject = "empty_owner"
	if err := h.run(ctx, func(ctx context.Context, tx pgx.Tx) error {
		state, err = pg.ReadPrivateIngressEnrollment(ctx, tx, empty)
		return err
	}); err != nil || state.Status != "absent" || state.Generation != 0 || state.Plan != nil {
		t.Fatal("explicit owner-scoped absence changed", state, err)
	}
	reader.authority.Status = "revoked"
	state, err = queue.Enrollment(ctx, cfg.Scope, cfg.Epoch.String())
	if !errors.Is(err, ingress.ErrRevoked) || state.Status != "" {
		t.Fatal("revoked authority became a usable enrollment state", state, err)
	}
}

func TestPrivateIngressConcurrentFirstEnrollmentKeepsOneEpoch(t *testing.T) {
	h := newIngressHarness(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	first, second := h.cfg, h.cfg
	first.Scope.Subject = "competing_owner"
	second.Scope = first.Scope
	first.Epoch, second.Epoch = uuid.New(), uuid.New()
	makePlan := func(cfg pg.PrivateIngressConfig) ingress.KeyPlan {
		identity, err := age.GenerateX25519Identity()
		if err != nil {
			t.Fatal(err)
		}
		plan := ingress.KeyPlan{Version: 1, Scope: cfg.Scope, Epoch: cfg.Epoch.String(), Generation: 1, Profile: ingress.Profile, Recipient: identity.Recipient().String()}
		plan.KeyID = ingress.PublicFingerprint(plan.Recipient)
		return plan
	}
	planA, planB := makePlan(first), makePlan(second)
	readyA, readyB := make(chan int32, 1), make(chan int32, 1)
	resultA, resultB := make(chan error, 1), make(chan error, 1)
	release := make(chan struct{})
	go func() {
		resultA <- h.run(ctx, func(ctx context.Context, tx pgx.Tx) error {
			if err := pg.EnrollPrivateIngress(ctx, tx, first, planA, h.target, strings.Repeat("A", 43)); err != nil {
				return err
			}
			var pid int32
			if err := tx.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
				return err
			}
			readyA <- pid
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	var pidA int32
	select {
	case pidA = <-readyA:
	case err := <-resultA:
		t.Fatal("first enrollment failed", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	go func() {
		resultB <- h.run(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var pid int32
			if err := tx.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
				return err
			}
			readyB <- pid
			return pg.EnrollPrivateIngress(ctx, tx, second, planB, h.target, strings.Repeat("A", 43))
		})
	}()
	var pidB int32
	select {
	case pidB = <-readyB:
	case err := <-resultB:
		t.Fatal("second enrollment failed before guard", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	waitForSnapshotBlock(ctx, t, h.Pool, pidB, pidA)
	close(release)
	if err := <-resultA; err != nil {
		t.Fatal(err)
	}
	if err := <-resultB; !errors.Is(err, ingress.ErrEnrollmentEpoch) {
		t.Fatal("competing epoch was accepted", err)
	}
	if err := h.run(ctx, func(ctx context.Context, tx pgx.Tx) error {
		state, err := pg.ReadPrivateIngressEnrollment(ctx, tx, first)
		if err != nil {
			return err
		}
		if state.Status != "active" || state.Plan == nil || *state.Plan != planA {
			return errors.New("first accepted plan changed")
		}
		_, err = pg.ReadPrivateIngressEnrollment(ctx, tx, second)
		if !errors.Is(err, ingress.ErrEnrollmentEpoch) {
			return errors.New("other epoch became absent after first enrollment")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
