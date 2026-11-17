// SPDX-License-Identifier: Apache-2.0
//go:build integration

package pg_test

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

func TestPrivateIngressEpochUpgradeRetainsHistoryAndRestoresRLS(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		name := "one_epoch"
		if ambiguous {
			name = "ambiguous_history"
		}
		t.Run(name, func(t *testing.T) {
			h := newIngressHarnessWithEpoch(t, false)
			if ambiguous {
				identity, err := age.GenerateX25519Identity()
				if err != nil {
					t.Fatal(err)
				}
				other := h.cfg
				other.Epoch = uuid.New()
				plan := h.plan
				plan.Epoch = other.Epoch.String()
				plan.Recipient = identity.Recipient().String()
				plan.KeyID = ingress.PublicFingerprint(plan.Recipient)
				if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
					return pg.EnrollPrivateIngress(ctx, tx, other, plan, h.target, strings.Repeat("A", 43))
				}); err != nil {
					t.Fatal(err)
				}
			}
			source := pg.WithPrivateIngressDDL(fstest.MapFS{"migrations/epoch.up.sql": {Data: []byte("-- sytue:private-ingress-epoch-v1")}})
			sql, err := fs.ReadFile(source, "migrations/epoch.up.sql")
			if err != nil {
				t.Fatal(err)
			}
			admin := h.PoolSinRLS(t)
			// Exercise a table-owning migrator with FORCE RLS and no BYPASSRLS. A
			// superuser-only upgrade test would miss an empty, policy-filtered backfill.
			if _, err := admin.Exec(t.Context(), `GRANT CREATE ON SCHEMA public TO sytue_app; ALTER TABLE private_ingress_key_plans OWNER TO sytue_app`); err != nil {
				t.Fatal(err)
			}
			err = admin.InTx(t.Context(), func(tx pgx.Tx) error {
				if _, err := tx.Exec(t.Context(), `SET LOCAL ROLE sytue_app`); err != nil {
					return err
				}
				_, err := tx.Exec(t.Context(), string(sql))
				return err
			})
			if ambiguous {
				var failure *pgconn.PgError
				if !errors.As(err, &failure) || failure.Code != "PIG01" {
					t.Fatal("ambiguous epochs were silently reconciled", err)
				}
			} else if err != nil {
				t.Fatal("valid retained plan could not be upgraded", err)
			}
			var protected, missing bool
			var retained int
			if err := admin.QueryRow(t.Context(), `SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE oid='public.private_ingress_key_plans'::regclass`).Scan(&protected); err != nil || !protected {
				t.Fatal("upgrade weakened RLS", protected, err)
			}
			if err := admin.QueryRow(t.Context(), `SELECT to_regclass('public.private_ingress_scopes') IS NULL`).Scan(&missing); err != nil || missing != ambiguous {
				t.Fatal("failed upgrade left a partial binding", missing, err)
			}
			if err := admin.QueryRow(t.Context(), `SELECT count(*) FROM private_ingress_key_plans`).Scan(&retained); err != nil {
				t.Fatal(err)
			}
			expected := 1
			if ambiguous {
				expected = 2
			}
			if retained != expected {
				t.Fatal("upgrade discarded retained history", retained)
			}
			if !ambiguous {
				if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
					state, err := pg.ReadPrivateIngressEnrollment(ctx, tx, h.cfg)
					if err != nil {
						return err
					}
					if state.Plan == nil || *state.Plan != h.plan {
						return errors.New("upgrade changed accepted key plan")
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
