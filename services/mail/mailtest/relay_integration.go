// SPDX-License-Identifier: Apache-2.0
//go:build integration

// Package mailtest mounts Mail's real private submission API and PostgreSQL
// receipts for disposable native SDK proofs. It is absent from production.
package mailtest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/storagetest"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/smtptest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/config"
	"git.sytue.eu/sytue/one/services/mail/internal/delivery"
	"git.sytue.eu/sytue/one/services/mail/internal/jmap"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

type Relay struct {
	mount    func(chi.Router)
	SMTP     *smtptest.Server
	mu       sync.Mutex
	received []byte
}
type fixedIngressScope struct{ scope storage.Scope }

func (s fixedIngressScope) PreparePrivateIngress(_ context.Context, scope storage.Scope) error {
	if scope != s.scope {
		return errors.New("fixture scope changed")
	}
	return nil
}

func (s fixedIngressScope) PrivateIngressReady(_ context.Context, scope storage.Scope) (bool, error) {
	return scope == s.scope, nil
}

// NewRelay retains only synthetic routing from the existing snapshot. The
// SMTP body, global generation fencing, source freeze and receipt SQL are real.
func NewRelay(t *testing.T, snapshot storagetest.MailSnapshot) *Relay {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	scope := snapshot.State.Scope
	ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: scope.Tenant})
	var primary string
	var aliases []string
	for _, row := range snapshot.Rows {
		if row.Descriptor.Kind != "account" && row.Descriptor.Kind != "alias" {
			continue
		}
		var address struct {
			Address string `json:"address"`
		}
		if json.Unmarshal(row.Body, &address) != nil || address.Address == "" {
			t.Fatal("snapshot sender identity is incomplete")
		}
		if row.Descriptor.Kind == "account" {
			if primary != "" {
				t.Fatal("snapshot has multiple owners")
			}
			primary = address.Address
		} else {
			aliases = append(aliases, address.Address)
		}
	}
	if primary == "" {
		t.Fatal("snapshot has no sender identity")
	}
	st := store.New(h.Pool)
	account, err := st.EnsureAccount(ctx, scope.Subject, primary, "", delivery.DefaultQuotaBytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		for _, address := range aliases {
			if _, err := tx.Exec(ctx, "INSERT INTO aliases(tenant,account_id,address) VALUES($1,$2,$3)", scope.Tenant, account.ID, address); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	epoch, err := uuid.Parse(snapshot.State.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.BeginPrivateStorageMigration(ctx, scope.Subject, epoch, func(context.Context, string) (io.ReadCloser, error) {
		return nil, errors.New("relay fixture must not materialize any MIME")
	}); err != nil {
		t.Fatal(err)
	}
	smtp := smtptest.New(t, smtptest.Config{LoseQuitACK: true})
	logger := slog.New(slog.DiscardHandler)
	sender := delivery.NewPrivateFixtureSender(logger, smtp.Host, smtp.Port, smtp.Roots)
	outbound := delivery.NewOutbound(st, nil, nil, logger, snapshot.State.MaxBodyBytes, delivery.DefaultQuotaBytes).WithPrivateSender(sender)
	api := jmap.New(config.Config{MaxMessageBytes: snapshot.State.MaxBodyBytes}, st, nil, outbound, logger).WithPrivateIngress(fixedIngressScope{scope: scope})
	return &Relay{mount: api.MountPrivateSubmissions, SMTP: smtp}
}

func (r *Relay) Mount(router chi.Router) { r.mount(router) }

// Proof returns only the synthetic fixture's in-memory SMTP observation.
func (r *Relay) Proof(w http.ResponseWriter, request *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.received == nil {
		select {
		case r.received = <-r.SMTP.Received:
		default:
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Accepted int32
		Raw      []byte
	}{r.SMTP.Accepted(), r.received})
}
