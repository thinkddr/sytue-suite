// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/cast/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

func TestPrivateCastInventoryPreservesTrashJobsAndExternalAccessWithoutFreezing(t *testing.T) {
	st, h := montarConHarness(t)
	ctx := ctxCorto(t)
	owned := listo(ctx, t, st, "t1", "alice", domain.VisibilityPeople, domain.StatusReady)
	foreign := listo(ctx, t, st, "other", "bob", domain.VisibilityPeople, domain.StatusReady)
	if err := st.AnadirPersona(ctx, "t1", owned.ID, domain.Persona{UserID: "carol"}, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := st.AnadirPersona(ctx, "other", foreign.ID, domain.Persona{UserID: "alice"}, "bob"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Encolar(ctx, "t1", owned.ID, domain.JobPoster, []byte(`{"at_ms":1234,"future_integer":900719925474099312345}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Borrar(ctx, "t1", owned.ID); err != nil {
		t.Fatal(err)
	}
	scope := storage.Scope{Tenant: "t1", Subject: "alice", Product: "cast"}
	ctx = tenancy.Into(ctx, tenancy.Tenant{Slug: "t1"})
	var snapshot *model.Snapshot
	read := func() error {
		return h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			var err error
			snapshot, err = store.ReadPrivateStorageInventory(ctx, tx, scope)
			return err
		})
	}
	if err := read(); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Rows()) != 4 {
		t.Fatal("owned/trashed graph or external access omitted", len(snapshot.Rows()))
	}
	if !errors.Is(snapshot.PersonalCompatible(), model.ErrShared) {
		t.Fatal("zero plaintext recipient access migrated implicitly")
	}
	found := map[string]bool{}
	for _, row := range snapshot.Rows() {
		found[row.Kind] = true
		if row.Kind == "video" && (!bytes.Contains(row.JSON, []byte(`"deleted_at":`)) || !bytes.Contains(row.JSON, []byte(`"hls_qualities":`))) {
			t.Fatal("video projection lost retained columns")
		}
		if row.Kind == "job" && !bytes.Contains(row.JSON, []byte(`900719925474099312345`)) {
			t.Fatal("job payload numbers changed")
		}
		if row.Kind == "external_access" && !bytes.Contains(row.JSON, []byte(`"tenant": "other"`)) {
			t.Fatal("incoming share tenant rewritten")
		}
	}
	if !found["external_access"] || !found["person"] || !found["job"] {
		t.Fatal("relationship omitted")
	}
	scope.Subject = "unrelated"
	if err := read(); err != nil || len(snapshot.Rows()) != 0 {
		t.Fatal("another person's graph leaked", err)
	}
	scope.Tenant = "other"
	if err := read(); !errors.Is(err, pg.ErrPrivateStorageScope) {
		t.Fatal("tenant binding bypassed", err)
	}
}
