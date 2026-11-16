//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/office/model"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

func TestOfficeResourceBootstrapSourceUsesActualFrozenDocumentOwnerAndVersion(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	for _, product := range []string{"docs", "sheets", "slides"} {
		t.Run(product, func(t *testing.T) {
			ctx := tenantCtx("acme")
			scope := storage.Scope{Tenant: "acme", Subject: "owner-" + product, Product: product}
			kind, _ := model.ProductKind(product)
			doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: kind, Title: "original", OwnerSub: scope.Subject})
			if err != nil {
				t.Fatal(err)
			}
			if err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `UPDATE documents SET seq=9007199254740993 WHERE tenant=$1 AND id=$2`, scope.Tenant, doc.ID)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			key := resource.Resource{Tenant: scope.Tenant, Product: product, Kind: string(kind), ID: doc.ID}
			source, err := store.NewResourceBootstrapSource(st, scope)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = source.BootstrapSource(ctx, key); !errors.Is(err, resource.ErrRevoked) {
				t.Fatal("unfrozen source became bootstrap authority", err)
			}
			original, err := store.NewPrivateSource(st, officeObjects{})
			if err != nil {
				t.Fatal(err)
			}
			frozen, err := original.Begin(ctx, scope, uuid.New())
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := source.BootstrapSource(ctx, key)
			if err != nil || snapshot.SourceEpoch != frozen.Epoch || snapshot.SourceSeq != "9007199254740993" || snapshot.OwnerScope != scope || !snapshot.Valid() {
				t.Fatal("original source lost authority/binding", err)
			}
			if err = source.CheckBootstrapSource(ctx, snapshot); err != nil {
				t.Fatal(err)
			}
			exerciseOriginalBootstrapHTTP(t, ctx, st, snapshot)
			changed := snapshot
			changed.SourceEpoch = uuid.NewString()
			if err = source.CheckBootstrapSource(ctx, changed); err == nil {
				t.Fatal("another source epoch accepted")
			}
			foreign := scope
			foreign.Subject = "other"
			other, err := store.NewResourceBootstrapSource(st, foreign)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = other.BootstrapSource(ctx, key); err == nil {
				t.Fatal("other owner bootstrapped retained document")
			}
			if _, err = source.BootstrapSource(tenantCtx("other"), key); err == nil {
				t.Fatal("cross tenant source read")
			}
			wrong := key
			wrong.Product = "mail"
			if _, err = source.BootstrapSource(ctx, wrong); err == nil {
				t.Fatal("product scope changed")
			}
			// The original source remains frozen and immutable until the client proves
			// the full encrypted copy; bootstrap metadata never retires it.
			if err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `UPDATE documents SET title='replacement' WHERE tenant=$1 AND id=$2`, scope.Tenant, doc.ID)
				return err
			}); !officeBlocked(err) {
				t.Fatal("bootstrap thawed original source", err)
			}
		})
	}
}
