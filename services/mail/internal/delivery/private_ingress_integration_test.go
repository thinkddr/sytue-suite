// SPDX-License-Identifier: Apache-2.0
//go:build integration

package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/agecipher"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

type smtpCipherObjects struct {
	body               []byte
	key                string
	failPut            bool
	plaintextAttempted bool
}

func (o *smtpCipherObjects) Put(_ context.Context, _ tenancy.Tenant, key string, body io.Reader, size int64, options objectstore.PutOptions) error {
	if o.failPut {
		return errors.New("synthetic object store unavailable")
	}
	if options.ContentType != "application/octet-stream" || options.Comprimir {
		o.plaintextAttempted = true
		return ingress.ErrContract
	}
	raw, err := io.ReadAll(io.LimitReader(body, size+1))
	if err != nil {
		return err
	}
	if int64(len(raw)) != size || !bytes.HasPrefix(raw, []byte("age-encryption.org/v1")) {
		return ingress.ErrContract
	}
	o.body = raw
	o.key = key
	return nil
}

func (o *smtpCipherObjects) Get(_ context.Context, _ tenancy.Tenant, key string) (io.ReadCloser, error) {
	if key != o.key {
		return nil, ingress.ErrContract
	}
	return io.NopCloser(bytes.NewReader(o.body)), nil
}

func TestPrivateSMTPPersistsOnlyAgeCiphertextAndOpaqueTask(t *testing.T) {
	for _, fault := range []string{"valid", "object-store-down"} {
		t.Run(fault, func(t *testing.T) {
			h := pgtest.New(t, db.Migrations, db.MigrationsDir)
			ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: "acme"})
			st := store.New(h.Pool)
			account, err := st.EnsureAccount(ctx, "private-smtp-owner", "private@example.test", "Private", 1<<30)
			if err != nil {
				t.Fatal(err)
			}
			epoch := uuid.New()
			if _, err := st.BeginPrivateStorageMigration(ctx, account.Subject, epoch, func(context.Context, string) (io.ReadCloser, error) {
				return nil, errors.New("empty mailbox has no original MIME")
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
			// This fixture isolates SMTP storage; signature authority is exercised by the
			// separate genuine-SDK nonce and Rooms origin conformance tests.
			if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
				return pg.EnrollPrivateIngress(ctx, tx, cfg, plan, storage.PublicationTarget{RoomID: "!private:matrix.test", DeviceID: "OWNER"}, "fixture public signing key")
			}); err != nil {
				t.Fatal(err)
			}
			objects := &smtpCipherObjects{failPut: fault == "object-store-down"}
			receiver := NewInbound(st, nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), InboundConfig{MaxBytes: 50 << 20})
			receiver.privateObjects = objects
			receiver.blobs = objects
			raw := []byte("From: outside@example.test\r\nTo: private@example.test\r\nSubject: PRIVATE-SMTP-ENVELOPE-MARKER\r\nMessage-ID: <private-original@example.test>\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nPRIVATE-SMTP-BODY-MARKER original MIME 漢字\r\n")
			err = receiver.Deliver(ctx, InboundMessage{RemoteIP: net.ParseIP("192.0.2.1"), MailFrom: "outside@example.test", Recipients: []string{account.Address}, Body: bytes.NewReader(raw)})
			if objects.plaintextAttempted {
				t.Fatal("SMTP attempted legacy plaintext object persistence")
			}
			if fault == "valid" && err != nil {
				t.Fatal(err)
			}
			if fault != "valid" && err == nil {
				t.Fatal("unsealed private input was accepted")
			}
			if bytes.Contains(objects.body, []byte("PRIVATE-SMTP")) {
				t.Fatal("SMTP plaintext reached object storage")
			}
			var task ingress.Task
			var blobID string
			if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
				if err := pg.BindPrivateIngressScope(ctx, tx, cfg); err != nil {
					return err
				}
				var messages, threads, legacyBlobs, announced int
				if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM messages),(SELECT count(*) FROM threads),(SELECT count(*) FROM blob_refs),(SELECT count(*) FROM events_outbox WHERE type='private_ingress.sealed')`).Scan(&messages, &threads, &legacyBlobs, &announced); err != nil {
					return err
				}
				if messages != 0 || threads != 0 || legacyBlobs != 0 {
					return errors.New("private SMTP used legacy plaintext persistence")
				}
				var id string
				if err := tx.QueryRow(ctx, `SELECT id FROM private_ingress_tasks`).Scan(&id); err != nil {
					return err
				}
				delivery, err := pg.ReadPrivateIngressDelivery(ctx, tx, cfg, id)
				if fault != "valid" {
					if !errors.Is(err, pg.ErrPrivateIngressPending) || announced != 0 {
						return errors.New("failed upload announced a private task")
					}
					return nil
				}
				if err != nil {
					return err
				}
				if announced != 1 {
					return errors.New("durable input did not publish one opaque notification")
				}
				task, blobID = delivery.Task, delivery.BlobID
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if fault == "valid" {
				key, err := objectstore.PrivateIngressBlobKey(scope, blobID)
				if err != nil || key != objects.key {
					t.Fatal("ciphertext locator changed", err)
				}
				metadata, body, err := agecipher.Open(ctx, bytes.NewReader(objects.body), task, identity)
				if err != nil {
					t.Fatal(err)
				}
				defer clear(metadata)
				var envelope struct {
					ID        string `json:"id"`
					MailboxID string `json:"mailbox_id"`
					Size      int64  `json:"size"`
				}
				if err := json.Unmarshal(metadata, &envelope); err != nil || envelope.ID == "" || envelope.MailboxID == "" || envelope.Size != int64(len(raw)) {
					t.Fatal("encrypted routing metadata lost original", err)
				}
				plaintext, err := io.ReadAll(body)
				defer clear(plaintext)
				if err != nil || !bytes.Equal(plaintext, raw) {
					t.Fatal("authorized device did not recover exact MIME", err)
				}
				inbox, err := st.MailboxByRole(ctx, account.ID, domain.RoleInbox)
				if err != nil || envelope.MailboxID != inbox.ID.String() {
					t.Fatal("private input targeted wrong mailbox", err)
				}
			}
			if strings.Contains(objects.key, account.Address) {
				t.Fatal("private envelope appeared in object key")
			}
		})
	}
}
