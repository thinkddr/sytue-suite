// SPDX-License-Identifier: Apache-2.0
//go:build linux && !android

package sesion

import (
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestPrivateWorkLeasePreservesRefreshAndRejectsAnotherAccount(t *testing.T) {
	suite, work, tokens := privateSuiteFixture(t)
	witness, err := suite.TestigoTrabajo(t.Context(), work.scope)
	if err != nil {
		t.Fatal(err)
	}
	acquire := suite.ArrendamientoTrabajoPrivado(witness, work.scope)
	release, err := acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	tokens.Refresh = "synthetic-refreshed"
	refreshed := make(chan error, 1)
	go func() {
		unlock, err := ajustes.CerrojoSesion(t.Context())
		if err != nil {
			refreshed <- err
			return
		}
		defer unlock()
		refreshed <- suite.almacen.Guardar(tokens)
	}()
	select {
	case err := <-refreshed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("OIDC refresh waited for a private work lease")
	}
	if err := witness.Validar(); err != nil {
		t.Fatal("refresh rotated the work witness", err)
	}
	tokens.Subject = "bea"
	if err := suite.Sesion.AdoptarCon(t.Context(), tokens); err != nil {
		t.Fatal(err)
	}
	if next, err := acquire(t.Context()); next != nil || !errors.Is(err, auth.ErrTrabajoSesion) {
		t.Fatal("another account inherited the old lease", err)
	}
	if suite.Sesion.Subject() != "bea" {
		t.Fatal("stale work changed the new account")
	}
}

func TestPrivateWorkMetadataCountsOneOwnerAndDiscardsStagingFromMetadata(t *testing.T) {
	_, work, _ := privateSuiteFixture(t)
	owner := "acme\x00ana\x00mail"
	foreign := "acme\x00bea\x00mail"
	for _, who := range []string{owner, foreign} {
		for _, namespace := range []string{storage.PrivateJournalNamespaces("mail")[0], storage.PrivateStagingNamespaces("mail")[0], storage.PendingUploadsNamespace} {
			if err := work.journal.Write(who, namespace, "pending", "synthetic-private-data"); err != nil {
				t.Fatal(err)
			}
		}
	}
	scope := storage.Scope{Tenant: "acme", Subject: "ana", Product: "mail"}
	metadata := metadatosPrivadosUID()
	if count, err := metadata.Contar(scope); err != nil || count != 2 {
		t.Fatal("staging blocks inflated pending operations", count, err)
	}
	if err := metadata.Desechar(t.Context(), scope); err != nil {
		t.Fatal(err)
	}
	namespaces := append(storage.PrivateJournalNamespaces("mail"), storage.PrivateStagingNamespaces("mail")...)
	namespaces = append(namespaces, storage.PendingUploadsNamespace)
	if count, err := work.journal.CountOwner(owner, namespaces...); err != nil || count != 0 {
		t.Fatal("discard acknowledged retained staging", count, err)
	}
	if count, err := work.journal.CountOwner(foreign, namespaces...); err != nil || count != 3 {
		t.Fatal("discard crossed to another account", count, err)
	}
}

func TestPrivateWorkLeaseRejectsInvalidOrForeignScopeBeforeIO(t *testing.T) {
	suite, work, _ := privateSuiteFixture(t)
	witness, err := suite.TestigoTrabajo(t.Context(), work.scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []storage.Scope{{Tenant: "", Subject: "ana", Product: "notes"}, {Tenant: "acme", Subject: "bea", Product: "notes"}} {
		if release, err := suite.ArrendamientoTrabajoPrivado(witness, scope)(t.Context()); release != nil || !errors.Is(err, auth.ErrTrabajoSesion) {
			t.Fatal("foreign scope inherited private IO", err)
		}
	}
}
