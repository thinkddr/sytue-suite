// SPDX-License-Identifier: Apache-2.0
//go:build integration

package pg_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

func ingressQueueFixture(h *ingressHarness, cfg pg.AuthorizedPrivateIngressConfig) pg.PrivateIngressQueue {
	return pg.PrivateIngressQueue{Transaction: func(ctx context.Context, scope storage.Scope, epoch, _ string, operation func(pgx.Tx, pg.AuthorizedPrivateIngressConfig) error) error {
		if scope != cfg.Scope || epoch != cfg.Epoch.String() {
			return ingress.ErrContract
		}
		return h.run(ctx, func(_ context.Context, tx pgx.Tx) error { return operation(tx, cfg) })
	}, Body: func(_ context.Context, scope storage.Scope, id string) (io.ReadCloser, error) {
		if scope != cfg.Scope || id != "opaque_blob_01" {
			return nil, ingress.ErrContract
		}
		return io.NopCloser(bytes.NewReader(h.cipher)), nil
	}}
}

func TestPrivateIngressQueueRevalidatesFreshNonceWithoutRenewingLease(t *testing.T) {
	h, cfg, reader, enrollment := newIngressControlFixture(t)
	queue := ingressQueueFixture(h, cfg)
	enrolled := ingressControlChallenge(t, h, cfg, enrollment, reader.keys)
	if _, err := ingressApplyControl(t, h, cfg, enrollment, enrolled); err != nil {
		t.Fatal(err)
	}
	h.seal(t)
	request := ingress.ControlRequest{Version: 1, Scope: cfg.Scope, Epoch: cfg.Epoch.String(), Kind: "task.claim", Target: enrollment.Target, Claim: &ingress.ClaimRequest{TaskID: h.task.ID, Token: "queue_claim", LeaseSeconds: 60}}
	proof := ingressControlChallenge(t, h, cfg, request, reader.keys)
	result, err := queue.Apply(tenancy.Into(t.Context(), tenancy.Tenant{Slug: cfg.Scope.Tenant}), cfg.Scope, request, proof)
	if err != nil || result.Claim == nil {
		t.Fatal(err)
	}
	claim := *result.Claim
	check := ingressControlChallenge(t, h, cfg, request, reader.keys)
	if _, err := queue.Revalidate(tenancy.Into(t.Context(), tenancy.Tenant{Slug: cfg.Scope.Tenant}), cfg.Scope, request, check, claim); err != nil {
		t.Fatal("fresh signed original claim did not revalidate", err)
	}
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		if err := pg.BindPrivateIngressScope(ctx, tx, h.cfg); err != nil {
			return err
		}
		var lease time.Time
		var consumed *time.Time
		if err := tx.QueryRow(ctx, `SELECT lease_until FROM private_ingress_tasks WHERE id=$1`, h.task.ID).Scan(&lease); err != nil {
			return err
		}
		if !lease.Equal(claim.ExpiresAt.Truncate(time.Microsecond)) {
			return errors.New("read-only publication check renewed the lease")
		}
		if err := tx.QueryRow(ctx, `SELECT consumed_at FROM private_ingress_actions WHERE nonce=$1`, check.Nonce).Scan(&consumed); err != nil {
			return err
		}
		if consumed == nil {
			return errors.New("fresh verification nonce did not commit")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reader.authority.Generation++
	if _, err := queue.Revalidate(tenancy.Into(t.Context(), tenancy.Tenant{Slug: cfg.Scope.Tenant}), cfg.Scope, request, check, claim); err == nil {
		t.Fatal("consumed nonce bypassed current authority")
	}
}

func TestPrivateIngressPendingExcludesUnsealedAndAcknowledgedTasks(t *testing.T) {
	h, cfg, reader, enrollment := newIngressControlFixture(t)
	queue := ingressQueueFixture(h, cfg)
	page, err := queue.Pending(tenancy.Into(t.Context(), tenancy.Tenant{Slug: cfg.Scope.Tenant}), cfg.Scope, cfg.Epoch.String(), "", 10)
	if err != nil || len(page.Tasks) != 0 {
		t.Fatal("unsealed input was listed", err)
	}
	enrolled := ingressControlChallenge(t, h, cfg, enrollment, reader.keys)
	if _, err := ingressApplyControl(t, h, cfg, enrollment, enrolled); err != nil {
		t.Fatal(err)
	}
	h.seal(t)
	page, err = queue.Pending(tenancy.Into(t.Context(), tenancy.Tenant{Slug: cfg.Scope.Tenant}), cfg.Scope, cfg.Epoch.String(), "", 10)
	if err != nil || len(page.Tasks) != 1 || page.Tasks[0] != h.task {
		t.Fatal("pending ciphertext not discoverable", err)
	}
	request := ingress.ControlRequest{Version: 1, Scope: cfg.Scope, Epoch: cfg.Epoch.String(), Kind: "task.claim", Target: enrollment.Target, Claim: &ingress.ClaimRequest{TaskID: h.task.ID, Token: "queue_claim", LeaseSeconds: 60}}
	proof := ingressControlChallenge(t, h, cfg, request, reader.keys)
	result, err := queue.Apply(tenancy.Into(t.Context(), tenancy.Tenant{Slug: cfg.Scope.Tenant}), cfg.Scope, request, proof)
	if err != nil || result.Claim == nil {
		t.Fatal(err)
	}
	claim := *result.Claim
	row, body, err := queue.Open(tenancy.Into(t.Context(), tenancy.Tenant{Slug: cfg.Scope.Tenant}), cfg.Scope, request, proof, claim)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || row != h.task.Cipher || !bytes.Equal(raw, h.cipher) {
		t.Fatal("opaque body did not match immutable delivery", err)
	}
	// A completed historical task can outlive its lease. A fresh signature may
	// reconcile its existing receipt, but Claim still refuses any new publisher.
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		if err := pg.BindPrivateIngressScope(ctx, tx, h.cfg); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE private_ingress_tasks SET acknowledged_at=now(),receipt_room=room_id,receipt_event='$committed',lease_until=now()-interval '1 minute' WHERE id=$1`, h.task.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	page, err = queue.Pending(tenancy.Into(t.Context(), tenancy.Tenant{Slug: cfg.Scope.Tenant}), cfg.Scope, cfg.Epoch.String(), "", 10)
	if err != nil || len(page.Tasks) != 0 {
		t.Fatal("acknowledged input remained pending", err)
	}
	fresh := ingressControlChallenge(t, h, cfg, request, reader.keys)
	if _, err := queue.Revalidate(tenancy.Into(t.Context(), tenancy.Tenant{Slug: cfg.Scope.Tenant}), cfg.Scope, request, fresh, claim); err != nil {
		t.Fatal("expired acknowledged original could not reconcile", err)
	}
	if _, err := queue.Apply(tenancy.Into(t.Context(), tenancy.Tenant{Slug: cfg.Scope.Tenant}), cfg.Scope, request, fresh); !errors.Is(err, ingress.ErrReceipt) {
		t.Fatal("completed task granted another publication claim", err)
	}
}
