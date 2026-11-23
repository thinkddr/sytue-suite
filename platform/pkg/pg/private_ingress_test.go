// SPDX-License-Identifier: Apache-2.0
//go:build integration

package pg_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/agecipher"
	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

type ingressHarness struct {
	*pgtest.Harness
	cfg      pg.PrivateIngressConfig
	plan     ingress.KeyPlan
	target   storage.PublicationTarget
	task     ingress.Task
	cipher   []byte
	identity *age.X25519Identity
}

func newIngressHarness(t *testing.T) *ingressHarness {
	t.Helper()
	return newIngressHarnessWithEpoch(t, true)
}

func newIngressHarnessWithEpoch(t *testing.T, canonical bool) *ingressHarness {
	t.Helper()
	migrations := fstest.MapFS{"migrations/0001_ingress.up.sql": {Data: []byte("-- sytue:scope-bound-merge-v1\n-- sytue:private-ingress-v1\n-- sytue:private-ingress-bodies-v1\n" + events.OutboxTableDDL)}, "migrations/0002_admission.up.sql": {Data: []byte("-- sytue:scope-admission-v1\nSELECT sytue_scope_admission_register('private_ingress_bodies'::regclass);")}, "migrations/0003_handoff.up.sql": {Data: []byte("-- sytue:private-ingress-handoff-v1")}, "migrations/0004_rejections.up.sql": {Data: []byte("-- sytue:private-ingress-rejections-v1")}}
	if canonical {
		migrations["migrations/0005_epoch.up.sql"] = &fstest.MapFile{Data: []byte("-- sytue:private-ingress-epoch-v1")}
		migrations["migrations/0006_pq.up.sql"] = &fstest.MapFile{Data: []byte("-- sytue:private-ingress-pq-v1")}
		migrations["migrations/0007_rotation.up.sql"] = &fstest.MapFile{Data: []byte("-- sytue:private-ingress-rotation-v1")}
	}
	fsys := pg.WithScopeAdmissionDDL(pg.WithScopeBoundMergeDDL(pg.WithPrivateIngressDDL(migrations)))
	h := &ingressHarness{Harness: pgtest.New(t, fsys, "migrations"), cfg: pg.PrivateIngressConfig{Scope: storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}, Epoch: uuid.New()}, target: storage.PublicationTarget{RoomID: "!ingress:test", DeviceID: "DEVICE_A"}}
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	h.identity = identity
	h.plan = ingress.KeyPlan{Version: 1, Scope: h.cfg.Scope, Epoch: h.cfg.Epoch.String(), Generation: 1, Profile: ingress.Profile, Recipient: identity.Recipient().String()}
	h.plan.KeyID = ingress.PublicFingerprint(h.plan.Recipient)
	h.cfg.Sealed = func(ctx context.Context, tx pgx.Tx, id string) error {
		ev, err := events.NewEvent("private_ingress", "private_ingress.sealed", h.cfg.Scope.Tenant, map[string]string{"task_id": id})
		if err != nil {
			return err
		}
		return events.Publish(ctx, tx, ev)
	}
	h.cfg.VerifyReceipt = func(context.Context, ingress.Claim, ingress.Receipt) error { return nil } // SQL test port; real Matrix authority belongs to joint fixture.
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		return pg.EnrollPrivateIngress(ctx, tx, h.cfg, h.plan, h.target, strings.Repeat("A", 43))
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		return pg.PlanPrivateIngress(ctx, tx, h.cfg, h.plan, "task_01", "mail.receive")
	}); err != nil {
		t.Fatal(err)
	}
	var cipher bytes.Buffer
	h.task, err = agecipher.Seal(t.Context(), &cipher, h.plan, "task_01", "mail.receive", []byte(`{"subject":"domain-fixture-marker","future":9007199254740993}`), strings.NewReader("original-private-MIME-marker"), int64(len("original-private-MIME-marker")))
	if err != nil {
		t.Fatal(err)
	}
	h.cipher = bytes.Clone(cipher.Bytes())
	t.Cleanup(func() { clear(h.cipher) })
	return h
}

func (h *ingressHarness) run(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	ctx = tenancy.Into(ctx, tenancy.Tenant{Slug: h.cfg.Scope.Tenant})
	return h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error { return fn(ctx, tx) })
}

func (h *ingressHarness) seal(t *testing.T) {
	t.Helper()
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		return pg.CommitPrivateIngressBody(ctx, tx, h.cfg, h.task, "opaque_blob_01")
	}); err != nil {
		t.Fatal(err)
	}
}

func (h *ingressHarness) claim(t *testing.T) ingress.Claim {
	t.Helper()
	var claim ingress.Claim
	err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		claim, err = pg.ClaimPrivateIngress(ctx, tx, h.cfg, h.task.ID, h.target, "private_claim_01", time.Minute)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return claim
}

func ingressReceipt(claim ingress.Claim) ingress.Receipt {
	return ingress.Receipt{Version: 1, Scope: claim.Task.Scope, Epoch: claim.Task.Epoch, TaskID: claim.Task.ID, CipherSHA256: claim.Task.Cipher.SHA256, ClaimGeneration: claim.Generation, Commit: storage.Reference{RoomID: claim.Target.RoomID, EventID: "$encrypted_commit"}}
}

func TestPrivateIngressDurableCiphertextClaimRestartAndIdempotentACK(t *testing.T) {
	h := newIngressHarness(t)
	h.seal(t)
	h.seal(t)
	claim := h.claim(t)
	if resumed := h.claim(t); resumed.Generation != claim.Generation || resumed.Token != claim.Token {
		t.Fatal("restart changed pinned claim")
	}
	receipt := ingressReceipt(claim)
	for range 2 {
		if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
			return pg.AcknowledgePrivateIngress(ctx, tx, h.cfg, claim, receipt)
		}); err != nil {
			t.Fatal(err)
		}
	}
	err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		delivery, err := pg.ReadPrivateIngressDelivery(ctx, tx, h.cfg, h.task.ID)
		if err != nil {
			return err
		}
		if delivery.Task != h.task || delivery.Receipt == nil || *delivery.Receipt != receipt {
			return errors.New("restart lost durable encrypted receipt")
		}
		var count int
		var payload string
		if err := tx.QueryRow(ctx, `SELECT count(*),min(payload::text) FROM events_outbox`).Scan(&count, &payload); err != nil {
			return err
		}
		if count != 1 || payload != `{"task_id": "task_01"}` {
			return fmt.Errorf("notification not atomic/opaque: count=%d", count)
		}
		var rows string
		if err := tx.QueryRow(ctx, `SELECT json_agg(row_to_json(t))::text FROM private_ingress_tasks t`).Scan(&rows); err != nil {
			return err
		}
		for _, marker := range []string{
			"original-private-MIME-marker",
			"domain-fixture-marker",
			"private_claim_01",
			"AGE-SECRET-KEY",
		} {
			if strings.Contains(rows, marker) || strings.Contains(payload, marker) || bytes.Contains(h.cipher, []byte(marker)) {
				return errors.New("plaintext reached durable ingress metadata")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	changed := receipt
	changed.Commit.EventID = "$different"
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		return pg.AcknowledgePrivateIngress(ctx, tx, h.cfg, claim, changed)
	}); !errors.Is(err, ingress.ErrReceipt) {
		t.Fatal("changed ACK accepted", err)
	}
}

func TestPrivateIngressScopeEpochRLSAndStaleIsolationFailClosed(t *testing.T) {
	h := newIngressHarness(t)
	h.seal(t)
	for _, fault := range []string{"owner", "tenant", "product", "epoch"} {
		t.Run(fault, func(t *testing.T) {
			cfg := h.cfg
			switch fault {
			case "owner":
				cfg.Scope.Subject = "bob"
			case "tenant":
				cfg.Scope.Tenant = "foreign"
			case "product":
				cfg.Scope.Product = "calendar"
			case "epoch":
				cfg.Epoch = uuid.New()
			}
			err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
				_, err := pg.ReadPrivateIngressDelivery(ctx, tx, cfg, h.task.ID)
				return err
			})
			if err == nil {
				t.Fatal("foreign binding exposed task")
			}
		})
	}
	err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		if err := pg.BindPrivateIngressScope(ctx, tx, h.cfg); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('app.private_ingress_owner','bob',true)`); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM private_ingress_tasks`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return errors.New("RLS exposed another owner")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: h.cfg.Scope.Tenant})
	err = h.Pool.InTxOpts(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead}, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, h.cfg.Scope.Tenant); err != nil {
			return err
		}
		return pg.BindPrivateIngressScope(ctx, tx, h.cfg)
	})
	if !errors.Is(err, ingress.ErrContract) {
		t.Fatal("stale repeatable-read accepted", err)
	}
	for _, setting := range []string{"app.maintenance", "app.merge"} {
		err = h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT set_config($1,'true',true)`, setting); err != nil {
				return err
			}
			return pg.BindPrivateIngressScope(ctx, tx, h.cfg)
		})
		if !errors.Is(err, ingress.ErrContract) {
			t.Fatal("privileged mode accepted", setting, err)
		}
	}
}

func TestPrivateIngressExpiredLeaseCannotSelectAnotherDeviceOrRoom(t *testing.T) {
	h := newIngressHarness(t)
	h.seal(t)
	h.claim(t)
	other := h.target
	other.DeviceID = "DEVICE_B"
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		return pg.EnrollPrivateIngress(ctx, tx, h.cfg, h.plan, other, "other_public_signing_key")
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		if err := pg.BindPrivateIngressScope(ctx, tx, h.cfg); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE private_ingress_tasks SET lease_until=now()-interval '1 hour'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, target := range []storage.PublicationTarget{other, {RoomID: "!foreign:test", DeviceID: h.target.DeviceID}} {
		err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
			_, err := pg.ClaimPrivateIngress(ctx, tx, h.cfg, h.task.ID, target, "other_claim", time.Minute)
			return err
		})
		if !errors.Is(err, ingress.ErrClaim) {
			t.Fatal("lease expiry changed publisher/room", err)
		}
	}
}

func TestPrivateIngressRevocationSerializesACKAndRetainsOriginal(t *testing.T) {
	h := newIngressHarness(t)
	h.seal(t)
	claim := h.claim(t)
	receipt := ingressReceipt(claim)
	entered := make(chan struct{})
	release := make(chan struct{})
	h.cfg.VerifyReceipt = func(context.Context, ingress.Claim, ingress.Receipt) error { close(entered); <-release; return nil }
	done := make(chan error, 1)
	go func() {
		done <- h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
			return pg.AcknowledgePrivateIngress(ctx, tx, h.cfg, claim, receipt)
		})
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("authority not reached")
	}
	err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		return pg.RevokePrivateIngressDevice(ctx, tx, h.cfg, h.target.DeviceID)
	})
	close(release)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ingress.ErrRevoked) {
			t.Fatal("revoked in-flight ACK accepted", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ACK/revocation deadlocked")
	}
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		return pg.EnrollPrivateIngress(ctx, tx, h.cfg, h.plan, h.target, strings.Repeat("A", 43))
	}); !errors.Is(err, ingress.ErrRevoked) {
		t.Fatal("revoked device revived", err)
	}
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		d, err := pg.ReadPrivateIngressDelivery(ctx, tx, h.cfg, h.task.ID)
		if err != nil {
			return err
		}
		if d.Task != h.task || d.Receipt != nil {
			return errors.New("revocation lost original or fabricated ACK")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateIngressImmutablePlansBodiesReceiptsAndOutboxRollback(t *testing.T) {
	h := newIngressHarness(t)
	h.cfg.Sealed = func(context.Context, pgx.Tx, string) error { return errors.New("synthetic notification failure") }
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		return pg.CommitPrivateIngressBody(ctx, tx, h.cfg, h.task, "opaque_blob_01")
	}); err == nil {
		t.Fatal("failed notification committed body")
	}
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := pg.ReadPrivateIngressDelivery(ctx, tx, h.cfg, h.task.ID)
		return err
	}); !errors.Is(err, pg.ErrPrivateIngressPending) {
		t.Fatal("rolled-back body became eligible", err)
	}
	h.cfg.Sealed = func(context.Context, pgx.Tx, string) error { return nil }
	h.seal(t)
	claim := h.claim(t)
	h.cfg.VerifyReceipt = nil
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		return pg.AcknowledgePrivateIngress(ctx, tx, h.cfg, claim, ingressReceipt(claim))
	}); !errors.Is(err, ingress.ErrReceipt) {
		t.Fatal("missing receipt authority accepted", err)
	}
	for _, query := range []string{`DELETE FROM private_ingress_tasks`, `UPDATE private_ingress_tasks SET cipher_size=cipher_size+1`, `UPDATE private_ingress_key_plans SET public_recipient='changed'`, `UPDATE private_ingress_devices SET signing_key='changed'`, `UPDATE private_ingress_tasks SET claim_token_sha256=sha256('changed'::bytea)`} {
		if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
			if err := pg.BindPrivateIngressScope(ctx, tx, h.cfg); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, query)
			return err
		}); err == nil {
			t.Fatal("immutable durable binding changed", query)
		}
	}
}
