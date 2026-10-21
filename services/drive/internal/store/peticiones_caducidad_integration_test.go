// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// Conceder edición exige una compartición VIVA propia. Una compartición caducada no
// cuenta —como en el resto de verjas de Drive (recipientPred, el enlace público)—: quien
// la hizo ya no puede abrirle la edición a nadie con ella.
func TestConcederEdicionNoVaLigadoAUnaComparticionCaducada(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	root, err := st.EnsureRoot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	carpeta, err := st.CreateFolder(ctx, root.ID, "Informes", "ana", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateShare(ctx, store.NewShareRecord{FolderID: &carpeta.ID, Kind: domain.ShareUser, Subject: "bob", Permission: domain.PermRead, CreatedBy: "ana"}); err != nil {
		t.Fatal(err)
	}
	ayer := time.Now().Add(-24 * time.Hour)
	if _, err := st.CreateShare(ctx, store.NewShareRecord{FolderID: &carpeta.ID, Kind: domain.ShareUser, Subject: "dave", Permission: domain.PermRead, CreatedBy: "carlos", ExpiresAt: &ayer}); err != nil {
		t.Fatal(err)
	}
	if nueva, err := st.PedirEdicion(ctx, carpeta.ID, peticiones.Peticion{Sujeto: "bob", Nombre: "Bob"}); err != nil || !nueva {
		t.Fatalf("pedir: %v %v", nueva, err)
	}
	if err := st.ResolverPeticionEdicion(ctx, carpeta.ID, "carlos", "bob", true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("carlos concedió con una compartición caducada: %v", err)
	}
	// Con una vigente, sí.
	manana := time.Now().Add(24 * time.Hour)
	if _, err := st.CreateShare(ctx, store.NewShareRecord{FolderID: &carpeta.ID, Kind: domain.ShareUser, Subject: "erin", Permission: domain.PermRead, CreatedBy: "carlos", ExpiresAt: &manana}); err != nil {
		t.Fatal(err)
	}
	if err := st.ResolverPeticionEdicion(ctx, carpeta.ID, "carlos", "bob", true); err != nil {
		t.Fatalf("con una compartición vigente: %v", err)
	}
}
