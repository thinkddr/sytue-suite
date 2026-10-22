// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// El permiso de «Pedir acceso» de un buzón compartido: se avisa a quien lo gestiona;
// solo él ve y concede, y solo a quien lo pidió; conceder pasa a Ben de `read` a
// `send`, y la lista de buzones deja de decir que lo tiene pedido.
func TestPedirYConcederAccesoAUnBuzonCompartido(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	ada := seedAccount(t, ctx, st, "ada@sytue.eu")
	ben := seedAccount(t, ctx, st, "ben@sytue.eu")
	shared, err := st.EnsureSharedMailbox(ctx, "soporte@sytue.eu", "Soporte", 1<<30, ada.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.GrantAccess(ctx, shared.ID, "ben@sytue.eu", domain.PermRead); err != nil {
		t.Fatal(err)
	}
	nueva, avisar, err := st.PedirEdicion(ctx, shared.ID, peticiones.Peticion{Sujeto: ben.Subject, Nombre: "Ben"})
	if err != nil || !nueva || len(avisar) != 1 || avisar[0] != ada.Subject {
		t.Fatalf("pedir: nueva=%v avisar=%v err=%v (quiero avisar a Ada)", nueva, avisar, err)
	}
	if bs, _ := st.ListAccessibleMailboxes(ctx, ben.Subject); len(bs) != 1 || !bs[0].EditRequested {
		t.Errorf("la lista de buzones no dice que Ben lo tiene pedido: %+v", bs)
	}
	if _, err := st.PeticionesEdicion(ctx, shared.ID, ben.Subject); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Ben ve las peticiones: %v", err)
	}
	if err := st.ResolverPeticionEdicion(ctx, shared.ID, ben.Subject, ben.Subject, true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Ben se concede a sí mismo: %v", err)
	}
	if err := st.ResolverPeticionEdicion(ctx, shared.ID, ada.Subject, "nadie", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("concedió a quien no lo pidió: %v", err)
	}
	if err := st.ResolverPeticionEdicion(ctx, shared.ID, ada.Subject, ben.Subject, true); err != nil {
		t.Fatal(err)
	}
	if p, err := st.GranteePermission(ctx, ben.Subject, shared.ID); err != nil || p != domain.PermSend {
		t.Errorf("tras conceder, Ben tiene %q (%v) y tenía que ser send", p, err)
	}
}
