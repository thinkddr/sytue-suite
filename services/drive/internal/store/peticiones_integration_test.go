// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// El permiso de «Pedir acceso de edición» de algo compartido: solo quien lo compartió
// (o el dueño) ve y concede; solo a quien lo pidió; conceder sube la compartición de
// Bob de lectura a escritura, y la petición deja de constar.
func TestPedirYConcederEdicionDeLoCompartido(t *testing.T) {
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
	if nueva, err := st.PedirEdicion(ctx, carpeta.ID, peticiones.Peticion{Sujeto: "bob", Nombre: "Bob"}); err != nil || !nueva {
		t.Fatalf("pedir: %v %v", nueva, err)
	}
	if p, _ := st.PedidasPor(ctx, "bob", []uuid.UUID{carpeta.ID}); !p[carpeta.ID] {
		t.Error("«compartido conmigo» no sabe que Bob lo tiene pedido")
	}
	if _, err := st.PeticionesEdicion(ctx, carpeta.ID, "bob"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("quien pide ve las peticiones: %v", err)
	}
	if err := st.ResolverPeticionEdicion(ctx, carpeta.ID, "bob", "bob", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Bob se concede a sí mismo: %v", err)
	}
	if err := st.ResolverPeticionEdicion(ctx, carpeta.ID, "ana", "nadie", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("concedió a quien no lo pidió: %v", err)
	}
	if err := st.ResolverPeticionEdicion(ctx, carpeta.ID, "ana", "bob", true); err != nil {
		t.Fatal(err)
	}
	sh, err := st.ShareForResource(ctx, "bob", nil, carpeta.ID)
	if err != nil || sh.Permission != domain.PermWrite {
		t.Errorf("tras conceder, Bob tiene %q (%v) y tenía que ser write", sh.Permission, err)
	}
	if p, _ := st.PedidasPor(ctx, "bob", []uuid.UUID{carpeta.ID}); p[carpeta.ID] {
		t.Error("tras conceder sigue pedida")
	}
}
