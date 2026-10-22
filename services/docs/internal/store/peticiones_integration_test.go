// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// La tabla común de peticiones contra Postgres de verdad: pedir dos veces avisa una;
// otra organización no la ve; conceder da editor y quita la petición a la vez, y no
// concede a quien no la tenía.
func TestPedirYConcederEdicionEnLaBase(t *testing.T) {
	st := newStore(t)
	acme, globex := tenantCtx("acme"), tenantCtx("globex")
	doc, _, err := st.CreateDocument(acme, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "T", OwnerSub: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	p := peticiones.Peticion{Recurso: doc.ID, Sujeto: "ana", Nombre: "Ana"}
	if nueva, err := st.PedirEdicion(acme, p); err != nil || !nueva {
		t.Fatalf("primera petición: nueva=%v err=%v", nueva, err)
	}
	if nueva, _ := st.PedirEdicion(acme, p); nueva {
		t.Error("pedirlo dos veces cuenta como nueva: el dueño recibiría dos avisos")
	}
	if ps, _ := st.PeticionesEdicion(globex, doc.ID); len(ps) != 0 {
		t.Errorf("otra organización ve la petición: %v", ps)
	}
	if err := st.ResolverPeticionEdicion(acme, doc.ID, "nadie", "owner"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("concedió a quien no lo pidió: %v", err)
	}
	if err := st.ResolverPeticionEdicion(acme, doc.ID, "ana", "owner"); err != nil {
		t.Fatal(err)
	}
	perms, _ := st.ListPermissions(acme, doc.ID)
	if len(perms) != 1 || perms[0].PrincipalID != "ana" || perms[0].Role != domain.RoleEditor {
		t.Errorf("tras conceder, los permisos son %+v", perms)
	}
	if hay, _ := st.HayPeticionEdicion(acme, doc.ID, "ana"); hay {
		t.Error("la petición sigue ahí después de concederla")
	}
}
