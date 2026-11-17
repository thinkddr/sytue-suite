// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"errors"
	"path/filepath"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestAllClosedNativeAppJournalsBlockLogoutWithoutReadingForeignAppKeys(t *testing.T) {
	suite, _, _ := privateSuiteFixture(t)
	base, err := ajustes.Carpeta()
	if err != nil {
		t.Fatal(err)
	}
	for _, app := range storage.Products() {
		dir := filepath.Join(base, "private-work", app, "offline")
		// Each installed app owns its key. Global coordination reads only sealed
		// paths and deliberately receives no ability to decrypt these stores.
		journal, err := offline.OpenWithSecrets(dir, new(authtest.Secrets))
		if err != nil {
			t.Fatal(err)
		}
		for _, subject := range []string{"ana", "bea"} {
			owner := "acme\x00" + subject + "\x00" + app
			if err = journal.Write(owner, storage.PendingUploadsNamespace, "pending", subject+app); err != nil {
				t.Fatal(err)
			}
		}
	}
	if count, err := suite.PendientesSalida(); err != nil || count != len(storage.Products()) {
		t.Fatal("closed app work was invisible to suite logout", count, err)
	}
	if err = suite.SalirConError(); !errors.Is(err, ErrTrabajoPendiente) {
		t.Fatal("logout passed another closed app's pending work", err)
	}
	if err = suite.SalirDescartando(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, app := range storage.Products() {
		dir := filepath.Join(base, "private-work", app, "offline")
		if count, err := offline.CountDirectoryOwner(dir, "acme\x00ana\x00"+app, storage.PendingUploadsNamespace); err != nil || count != 0 {
			t.Fatal("explicit current-owner discard did not reach a closed app", app, count, err)
		}
		if count, err := offline.CountDirectoryOwner(dir, "acme\x00bea\x00"+app, storage.PendingUploadsNamespace); err != nil || count != 1 {
			t.Fatal("explicit discard erased another closed-app account", app, count, err)
		}
	}
}

func TestClosedAppDiscardRejectsOldLoginAfterAccountReturns(t *testing.T) {
	suite, _, tokens := privateSuiteFixture(t)
	witness, err := suite.TestigoTrabajo(t.Context(), storage.Scope{Tenant: "acme", Subject: "ana", Product: "mail"})
	if err != nil {
		t.Fatal(err)
	}
	dir, err := directorioAlmacenAplicacion("mail")
	if err != nil {
		t.Fatal(err)
	}
	journal, err := offline.OpenWithSecrets(dir, new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	for _, subject := range []string{"ana", "bea"} {
		if err = journal.Write("acme\x00"+subject+"\x00mail", storage.PendingUploadsNamespace, "retained", subject); err != nil {
			t.Fatal(err)
		}
	}
	tokens.Subject = "bea"
	tokens.Refresh = "synthetic-b-login"
	if err = suite.Sesion.AdoptarCon(t.Context(), tokens); err != nil {
		t.Fatal(err)
	}
	tokens.Subject = "ana"
	tokens.Refresh = "synthetic-a-new-login"
	if err = suite.Sesion.AdoptarCon(t.Context(), tokens); err != nil {
		t.Fatal(err)
	}
	if err = suite.salirProtegidoConTestigo(t.Context(), true, witness); !errors.Is(err, ajustes.ErrAccesoSesion) {
		t.Fatal("stale discard accepted an A to B to A login", err)
	}
	for _, subject := range []string{"ana", "bea"} {
		if count, err := offline.CountDirectoryOwner(dir, "acme\x00"+subject+"\x00mail", storage.PendingUploadsNamespace); err != nil || count != 1 {
			t.Fatal("old discard changed closed-app work after a new login", subject, count, err)
		}
	}
}
