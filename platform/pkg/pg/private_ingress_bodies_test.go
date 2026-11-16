// SPDX-License-Identifier: Apache-2.0
//go:build integration

package pg_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/agecipher"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

func TestPrivateIngressInlineBodyStoresOnlyRecoverableCiphertext(t *testing.T) {
	h := newIngressHarness(t)
	plain := []byte("BEGIN:VCALENDAR\nSUMMARY:calendar-private-marker\nEND:VCALENDAR")
	metadata := []byte(`{"calendar":"private-calendar-marker"}`)
	var task ingress.Task
	var blob string
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		task, blob, err = pg.SealPrivateIngressInline(ctx, tx, h.cfg, h.plan, h.task.ID, h.task.Kind, metadata, bytes.NewReader(plain), int64(len(plain)), 1<<20)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		body, err := pg.ReadPrivateIngressInlineBody(ctx, tx, h.cfg, blob)
		if err != nil {
			return err
		}
		defer body.Close()
		raw, err = io.ReadAll(body)
		if err != nil {
			return err
		}
		inventory, err := pg.ConsultaInventarioDeFusion(ctx, tx)
		if err != nil {
			return err
		}
		found := false
		for _, name := range inventory.ScopeBound {
			if name == "private_ingress_bodies" {
				found = true
			}
		}
		if !found {
			return errors.New("ciphertext body lacks Scope-bound merge contract")
		}
		var rows string
		if err := tx.QueryRow(ctx, `SELECT json_agg(row_to_json(t))::text FROM private_ingress_tasks t`).Scan(&rows); err != nil {
			return err
		}
		for _, marker := range []string{"calendar-private-marker", "private-calendar-marker", "AGE-SECRET-KEY"} {
			if strings.Contains(rows, marker) || bytes.Contains(raw, []byte(marker)) {
				return errors.New("plaintext persisted by Calendar ingress")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	decoded, body, err := agecipher.Open(t.Context(), bytes.NewReader(raw), task, h.identity)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := io.ReadAll(body)
	if err != nil || !bytes.Equal(recovered, plain) || !bytes.Equal(decoded, metadata) {
		t.Fatal("age recovery lost exact original Calendar input", err)
	}
	// Even the owning scope cannot replace accepted ciphertext after publication.
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		if err := pg.BindPrivateIngressScope(ctx, tx, h.cfg); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE private_ingress_bodies SET ciphertext=ciphertext WHERE id=$1`, blob)
		return err
	}); err == nil {
		t.Fatal("accepted ciphertext was mutable")
	}
	other := h.cfg
	other.Scope.Subject = "mallory"
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := pg.ReadPrivateIngressInlineBody(ctx, tx, other, blob)
		return err
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("cross-owner ciphertext was readable", err)
	}
}

type ingressReadCounter struct{ reads int }

func (r *ingressReadCounter) Read([]byte) (int, error) { r.reads++; return 0, io.EOF }

func TestPrivateIngressInlineRejectsOversizeBeforeReadingAndRollbackAnnouncesNothing(t *testing.T) {
	h := newIngressHarness(t)
	reader := &ingressReadCounter{}
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		_, _, err := pg.SealPrivateIngressInline(ctx, tx, h.cfg, h.plan, h.task.ID, h.task.Kind, []byte(`{}`), reader, 1025, 1024)
		return err
	}); !errors.Is(err, ingress.ErrContract) || reader.reads != 0 {
		t.Fatal("oversize input was read or accepted", reader.reads, err)
	}
	rollback := errors.New("domain transaction aborted")
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		if _, _, err := pg.SealPrivateIngressInline(ctx, tx, h.cfg, h.plan, h.task.ID, h.task.Kind, []byte(`{}`), strings.NewReader("ics"), 3, 1024); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		if err := pg.BindPrivateIngressScope(ctx, tx, h.cfg); err != nil {
			return err
		}
		var bodies, announcements int
		if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM private_ingress_bodies),(SELECT count(*) FROM events_outbox)`).Scan(&bodies, &announcements); err != nil {
			return err
		}
		if bodies != 0 || announcements != 0 {
			return errors.New("rollback exposed a partial ciphertext body or announcement")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
