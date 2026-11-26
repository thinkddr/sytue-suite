// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contacts_test

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Exportar vCard en lote y deshacer el borrado en lote, sin servicio y por la fachada
// privada (K-02).
func TestLoteDeVCardYDeshacerBorrado(t *testing.T) {
	repo, cerrar := abrirLocal(t, t.TempDir())
	defer cerrar()
	ctx := t.Context()
	lb, err := repo.CreateAddressBook(ctx, "Personal")
	if err != nil {
		t.Fatal(err)
	}
	ada, err := repo.ImportCard(ctx, lb.ID, tarjetaDePrueba)
	if err != nil {
		t.Fatal(err)
	}
	grace, err := repo.Create(ctx, lb.ID, api.ContactInput{FormattedName: "Grace Hopper", GivenName: "Grace", FamilyName: "Hopper"})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := api.NewContacts("", nil, nil).WithPrivateData(repo)
	if err != nil {
		t.Fatal(err)
	}

	vcf, err := svc.BatchVCard(ctx, []string{ada.ID, grace.ID, "no-existe"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(vcf), "BEGIN:VCARD") != 2 || !strings.Contains(string(vcf), "Ada Lovelace") || !strings.Contains(string(vcf), "Grace Hopper") {
		t.Fatalf("el .vcf no trae las dos tarjetas:\n%s", vcf)
	}

	if _, err := svc.BatchTrash(ctx, []string{ada.UID, grace.UID}); err != nil {
		t.Fatal(err)
	}
	if todos, _, _ := repo.All(ctx, 10, 0); len(todos) != 0 {
		t.Fatal("borrados", todos)
	}
	n, err := svc.BatchRestore(ctx, []string{ada.UID, grace.UID, "otro"})
	if err != nil || n != 2 {
		t.Fatal("deshacer", n, err)
	}
	if todos, _, _ := repo.All(ctx, 10, 0); len(todos) != 2 {
		t.Fatal("vuelven los dos", todos)
	}
	if papelera, _ := repo.Trash(ctx, 10); len(papelera) != 0 {
		t.Fatal("la papelera queda vacía", papelera)
	}
}
