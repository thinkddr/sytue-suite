// SPDX-License-Identifier: Apache-2.0
//go:build integration

package pg_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

func TestPrivateIngressHybridEnrollmentPreservesClassicalHistory(t *testing.T) {
	h := newIngressHarness(t)
	identity, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	plan := h.plan
	plan.Generation++
	plan.Profile = identity.Profile()
	plan.Recipient = identity.Recipient().String()
	plan.KeyID = ingress.PublicFingerprint(plan.Recipient)
	if plan.Profile != ingress.HybridProfile || len(plan.Recipient) <= 256 {
		t.Fatal("fixture did not exercise the actual hybrid public key")
	}
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		if err := pg.EnrollPrivateIngress(ctx, tx, h.cfg, plan, h.target, strings.Repeat("A", 43)); err != nil {
			return err
		}
		state, err := pg.ReadPrivateIngressEnrollment(ctx, tx, h.cfg)
		if err != nil {
			return err
		}
		if state.Plan == nil || *state.Plan != plan {
			return errors.New("hybrid recipient or profile changed in PostgreSQL")
		}
		var recipient, profile string
		if err := tx.QueryRow(ctx, "SELECT public_recipient,profile FROM private_ingress_key_plans WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND key_generation=$5", h.cfg.Scope.Tenant, h.cfg.Scope.Subject, h.cfg.Scope.Product, h.cfg.Epoch, h.plan.Generation).Scan(&recipient, &profile); err != nil {
			return err
		}
		if recipient != h.plan.Recipient || profile != h.plan.Profile {
			return errors.New("classical enrollment history was rewritten")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	downgraded := plan
	downgraded.Generation++
	downgraded.Profile = ingress.Profile
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		return pg.EnrollPrivateIngress(ctx, tx, h.cfg, downgraded, h.target, strings.Repeat("A", 43))
	}); !errors.Is(err, ingress.ErrContract) {
		t.Fatal("hybrid key accepted under a classical profile", err)
	}
}
