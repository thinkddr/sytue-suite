// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// «Soltar la fuente» en Drive (ADR 0086): quien vació la bóveda de su Drive vuelve a
// los datos del servidor tal como se congelaron, y una congelación nueva los exporta
// otra vez para la bóveda nueva. Solo con la época vista y en el ámbito de la sesión.
func TestPrivateStorageReleaseReturnsDriveToTheFrozenServerData(t *testing.T) {
	_, st, ctx, _, file := privateDriveFixture(t)
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "drive"}
	release := func(epoch uuid.UUID) pg.PrivateStorageRelease {
		return pg.PrivateStorageRelease{Epoch: epoch, AuthTime: time.Now(), AMR: []string{"pwd"}}
	}
	first := uuid.New()
	frozen, err := st.BeginPrivateStorageMigration(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	rows := privateRows(ctx, t, st, first, storage.MaxLegacySourceRows)

	// Otra época, otro ámbito u otra persona: nada cambia.
	if err := st.ReleasePrivateStorage(ctx, scope, release(uuid.New())); !errors.Is(err, pg.ErrPrivateStorageReleaseStale) {
		t.Fatalf("stale epoch released: %v", err)
	}
	for _, other := range []storage.Scope{
		{Tenant: "acme", Subject: "bob", Product: "drive"},
		{Tenant: "other", Subject: "alice", Product: "drive"},
		{Tenant: "acme", Subject: "alice", Product: "calendar"},
	} {
		if err := st.ReleasePrivateStorage(ctx, other, release(first)); !errors.Is(err, pg.ErrPrivateStorageScope) {
			t.Fatalf("foreign scope %v released: %v", other, err)
		}
	}
	if state, err := st.PrivateStorageState(ctx); err != nil || state.Mode != "frozen" || state.Epoch != first.String() {
		t.Fatalf("refused release changed the state: %+v %v", state, err)
	}

	if err := st.ReleasePrivateStorage(ctx, scope, release(first)); err != nil {
		t.Fatal(err)
	}
	state, err := st.PrivateStorageState(ctx)
	if err != nil || state.Mode != "legacy" || state.Epoch != "" {
		t.Fatalf("released source is not legacy: %+v %v", state, err)
	}
	// Repetirlo con la época de antes ya no hace nada.
	if err := st.ReleasePrivateStorage(ctx, scope, release(first)); !errors.Is(err, pg.ErrPrivateStorageReleaseStale) {
		t.Fatalf("release repeated: %v", err)
	}
	// Legacy otra vez: el servidor vuelve a aceptar escrituras de esta persona.
	if _, err := st.RenameFile(ctx, file.ID, file.Name); err != nil {
		t.Fatal("legacy writes still fenced after release", err)
	}

	// La bóveda nueva congela otra vez y recibe los mismos datos.
	second := uuid.New()
	again, err := st.BeginPrivateStorageMigration(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	if again.Mode != "frozen" || again.Rows != frozen.Rows || again.Blobs != frozen.Blobs {
		t.Fatalf("refrozen source differs: before %+v after %+v", frozen, again)
	}
	if got := privateRows(ctx, t, st, second, storage.MaxLegacySourceRows); len(got) != len(rows) {
		t.Fatalf("refrozen rows: got %d want %d", len(got), len(rows))
	}
	if _, err := st.PrivateStorageSourcePage(ctx, first, store.PrivateSourceCursor{}, 10); !errors.Is(err, domain.ErrPrivateStorageEpoch) {
		t.Fatalf("released epoch still exported: %v", err)
	}
}
