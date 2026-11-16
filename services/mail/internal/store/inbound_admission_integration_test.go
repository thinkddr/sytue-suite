// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

func oneConnectionMailStore(ctx context.Context, t *testing.T, h *pgtest.Harness) (*store.Store, int32) {
	t.Helper()
	pool, err := pg.Connect(ctx, pg.Config{DSN: h.AppDSN, MaxConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var pid int32
	if err := pool.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	return store.New(pool), pid
}

func TestPrivateInboundAdmissionSerializesFreezeAndUsesOneConnection(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx, cancel := context.WithTimeout(tenantCtx("acme"), 15*time.Second)
	defer cancel()
	admitted, pid := oneConnectionMailStore(ctx, t, h)
	freezer, freezePID := oneConnectionMailStore(ctx, t, h)
	account := seedAccount(ctx, t, admitted, "admitted@example.test")
	recipient := store.ResolvedAddress{Tenant: "acme", Subject: account.Subject, AccountID: account.ID, Address: account.Address}
	entered, release := make(chan struct{}), make(chan struct{})
	writeDone := make(chan error, 1)
	go func() {
		_, err := admitted.AdmitInbound(ctx, recipient, domain.RoleInbox, func(admission *store.InboundLegacyAdmission) error {
			mailbox, err := admission.Mailbox(ctx, domain.RoleInbox)
			if err != nil {
				return err
			}
			if _, err := admission.Filters(ctx); err != nil {
				return err
			}
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
			return admission.Insert(ctx, &domain.Message{AccountID: account.ID, MailboxID: mailbox.ID, BlobKey: "admitted-legacy.eml", SizeBytes: 32, Subject: "admitted original", MessageID: "admitted@example.test"})
		})
		writeDone <- err
	}()
	select {
	case <-entered:
	case err := <-writeDone:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	freezeDone := make(chan error, 1)
	epoch := uuid.New()
	go func() {
		_, err := freezer.BeginPrivateStorageMigration(ctx, account.Subject, epoch, func(context.Context, string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(strings.Repeat("x", 32))), nil
		})
		freezeDone <- err
	}()
	pgtest.WaitBlocked(ctx, t, h.Pool, freezePID, pid)
	close(release)
	if err := <-writeDone; err != nil {
		t.Fatal("single-connection admission reacquired its pool", err)
	}
	if err := <-freezeDone; err != nil {
		t.Fatal(err)
	}
	called := false
	if _, err := admitted.AdmitInbound(ctx, recipient, domain.RoleInbox, func(*store.InboundLegacyAdmission) error { called = true; return nil }); !errors.Is(err, pg.ErrPrivateIngressMissing) || called {
		t.Fatal("frozen recipient reached plaintext fallback", err, called)
	}
}

func TestPrivateInboundPlanCommitsBeforeBodyAndContainsNoEnvelope(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx, cancel := context.WithTimeout(tenantCtx("acme"), 15*time.Second)
	defer cancel()
	st := store.New(h.Pool)
	account := seedAccount(ctx, t, st, "opaque@example.test")
	epoch := uuid.New()
	if _, err := st.BeginPrivateStorageMigration(ctx, account.Subject, epoch, func(context.Context, string) (io.ReadCloser, error) {
		return nil, errors.New("empty mailbox has no body")
	}); err != nil {
		t.Fatal(err)
	}
	scope := storage.Scope{Tenant: "acme", Subject: account.Subject, Product: "mail"}
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	plan := ingress.KeyPlan{Version: 1, Scope: scope, Epoch: epoch.String(), Generation: 1, Profile: ingress.Profile, Recipient: identity.Recipient().String()}
	plan.KeyID = ingress.PublicFingerprint(plan.Recipient)
	cfg := pg.PrivateIngressConfig{Scope: scope, Epoch: epoch}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return pg.EnrollPrivateIngress(ctx, tx, cfg, plan, storage.PublicationTarget{RoomID: "!private:matrix.test", DeviceID: "OWNER"}, "fixture public signing key")
	}); err != nil {
		t.Fatal(err)
	}
	called := false
	route, err := st.AdmitInbound(ctx, store.ResolvedAddress{Tenant: "acme", Subject: account.Subject, AccountID: account.ID}, domain.RoleInbox, func(*store.InboundLegacyAdmission) error { called = true; return nil })
	if err != nil || called || route == nil || route.Key != plan || route.TaskID == "" || route.MailboxID == uuid.Nil {
		t.Fatal("private route did not retain original plan", err, called)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := pg.BindPrivateIngressScope(ctx, tx, cfg); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM private_ingress_tasks WHERE id=$1 AND blob_id IS NULL AND cipher_sha256 IS NULL`, route.TaskID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return errors.New("private task did not commit before body encryption")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
