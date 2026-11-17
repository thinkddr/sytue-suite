// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"errors"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

func TestClosedConsumerJournalsBlockLogoutAndDiscardOnlyCurrentOwner(t *testing.T) {
	suite, _, _ := privateSuiteFixture(t)
	base, err := ajustes.Carpeta()
	if err != nil {
		t.Fatal(err)
	}
	for _, consumer := range []string{"pdf", "docs"} {
		// Independent per-app keys. Logout reads only sealed record paths, never
		// borrows another consumer's key or opens/decrypts its journal.
		journal, err := offline.OpenWithSecrets(filepath.Join(base, "private-work", consumer, "offline"), new(authtest.Secrets))
		if err != nil {
			t.Fatal(err)
		}
		for _, subject := range []string{"ana", "bea"} {
			if err = journal.Write("acme\x00"+subject+"\x00drive", "drive-private-migration-v1", "retained", subject); err != nil {
				t.Fatal(err)
			}
		}
	}
	if count, err := suite.PendientesSalida(); err != nil || count != 2 {
		t.Fatal("closed consumers disappeared from global pending count", count, err)
	}
	if err = suite.SalirConError(); !errors.Is(err, ErrTrabajoPendiente) {
		t.Fatal("logout passed closed native consumer work", err)
	}
	if err = suite.SalirDescartando(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, consumer := range []string{"pdf", "docs"} {
		dir := filepath.Join(base, "private-work", consumer, "offline")
		if count, err := offline.CountDirectoryOwner(dir, "acme\x00ana\x00drive", "drive-private-migration-v1"); err != nil || count != 0 {
			t.Fatal("current consumer work was not explicitly discarded", count, err)
		}
		if count, err := offline.CountDirectoryOwner(dir, "acme\x00bea\x00drive", "drive-private-migration-v1"); err != nil || count != 1 {
			t.Fatal("consumer discard erased another person", count, err)
		}
	}
}

func TestNativeConsumerComesFromTheRegisteredReturnScheme(t *testing.T) {
	for _, tc := range []struct {
		product, scheme, want string
		valid                 bool
	}{
		{"drive", "sytue-drive", "", true},
		{"drive", "sytue-pdf", "pdf", true},
		{"drive", "sytue-docs", "docs", true},
		{"drive", "sytue-unknown", "", false},
		{"drive", "sytue-pdf ", "", false},
		{"mail", "sytue-pdf", "", false},
	} {
		suite := Suite{app: consentApp{metadata: fyne.AppMetadata{Custom: map[string]string{"esquema": tc.scheme}}}}
		got, err := suite.ConsumidorAlmacenamiento(tc.product)
		if (err == nil) != tc.valid || got != tc.want {
			t.Fatal("recipient did not match installed scheme", tc, got, err)
		}
	}
	if api.Esquema("docs") != "sytue-docs" {
		t.Fatal("update test from authoritative native app scheme")
	}
}

func TestConsumerJournalPersistsIndependentlyAcrossAppRestart(t *testing.T) {
	aparte(t)
	pdf, err := AlmacenConsumidor(t.Context(), "pdf")
	if err != nil {
		t.Fatal(err)
	}
	docs, err := AlmacenConsumidor(t.Context(), "docs")
	if err != nil {
		t.Fatal(err)
	}
	owner := "acme\x00ana\x00drive"
	if err = pdf.Write(owner, "drive-private-migration-v1", "same-record", "pdf"); err != nil {
		t.Fatal(err)
	}
	if err = docs.Write(owner, "drive-private-migration-v1", "same-record", "docs"); err != nil {
		t.Fatal(err)
	}
	reopened, err := AlmacenConsumidor(t.Context(), "pdf")
	if err != nil {
		t.Fatal(err)
	}
	var value string
	if found, err := reopened.Read(owner, "drive-private-migration-v1", "same-record", &value); err != nil || !found || value != "pdf" {
		t.Fatal("consumer journal was lost or shared with another app", value, err)
	}
	for _, invalid := range []string{"", "drive", "../pdf", "PDF"} {
		if store, err := AlmacenConsumidor(t.Context(), invalid); store != nil || err == nil {
			t.Fatal("ambiguous journal recipient accepted", invalid)
		}
	}
}
