// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build !js

package sesion

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestAppleAppJournalRetainsCacheAndAccountsAndRedirectsExistingHTTPClients(t *testing.T) {
	suite, work, _ := privateSuiteFixture(t)
	suite.app = consentApp{App: suite.app, metadata: fyne.AppMetadata{Custom: map[string]string{"esquema": "sytue-mail"}}}
	_, client := suite.Conexion("mail")
	transport := client.Transport.(*offline.Transport)
	source, err := directorioAlmacen()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(source, 0o700) })
	for _, subject := range []string{"ana", "bea"} {
		if err = work.journal.Write("acme\x00"+subject+"\x00mail", storage.PendingUploadsNamespace, "pending", subject); err != nil {
			t.Fatal(err)
		}
	}
	if err = work.journal.Write("http-owner", "http:mail", "response", "unchanged cache"); err != nil {
		t.Fatal(err)
	}
	witness, err := suite.TestigoTrabajo(t.Context(), storage.Scope{Tenant: "acme", Subject: "ana", Product: "mail"})
	if err != nil {
		t.Fatal(err)
	}
	migrated, err := suite.almacenPrivadoAplicacion(t.Context(), "mail", witness, true)
	if err != nil {
		t.Fatal(err)
	}
	target, err := directorioAlmacenAplicacion("mail")
	if err != nil {
		t.Fatal(err)
	}
	if suite.cacheAlmacen.Load().dir != target {
		t.Fatal("native app adopted another app's journal")
	}
	resolved, err := transport.ResolveStore(t.Context())
	if err != nil || resolved != migrated {
		t.Fatal("HTTP client stayed on a retired source", err)
	}
	var cached string
	if found, err := resolved.Read("http-owner", "http:mail", "response", &cached); err != nil || !found || cached != "unchanged cache" {
		t.Fatal("cache was lost during native relocation", err)
	}
	if count, err := suite.PendientesSalida(); err != nil || count != 1 {
		t.Fatal("relocated source was counted twice or hidden", count, err)
	}
	if err = migrated.Delete("acme\x00ana\x00mail", storage.PendingUploadsNamespace, "pending"); err != nil {
		t.Fatal(err)
	}
	if count, err := suite.PendientesSalida(); err != nil || count != 0 {
		t.Fatal("retained old source blocked acknowledged work forever", count, err)
	}
	reopened, err := suite.almacenPrivadoAplicacion(t.Context(), "mail", witness, true)
	if err != nil {
		t.Fatal(err)
	}
	if count, err := reopened.CountOwner("acme\x00ana\x00mail", storage.PendingUploadsNamespace); err != nil || count != 0 {
		t.Fatal("restart revived acknowledged work", count, err)
	}
	var other string
	if found, err := reopened.Read("acme\x00bea\x00mail", storage.PendingUploadsNamespace, "pending", &other); err != nil || !found || other != "bea" {
		t.Fatal("relocation or acknowledgement erased another account", err)
	}
	if retired, err := offline.JournalRelocationRetired(t.Context(), source, target); err != nil || !retired {
		t.Fatal("source archive was not durably retained", retired, err)
	}
}

func TestAppleAppJournalRejectsStaleWitnessBeforeCreatingDestination(t *testing.T) {
	suite, _, tokens := privateSuiteFixture(t)
	suite.app = consentApp{App: suite.app, metadata: fyne.AppMetadata{Custom: map[string]string{"esquema": "sytue-mail"}}}
	witness, err := suite.TestigoTrabajo(t.Context(), storage.Scope{Tenant: "acme", Subject: "ana", Product: "mail"})
	if err != nil {
		t.Fatal(err)
	}
	tokens.Subject = "bea"
	if err = suite.Sesion.AdoptarCon(t.Context(), tokens); err != nil {
		t.Fatal(err)
	}
	tokens.Subject = "ana"
	if err = suite.Sesion.AdoptarCon(t.Context(), tokens); err != nil {
		t.Fatal(err)
	}
	if _, err = suite.almacenPrivadoAplicacion(t.Context(), "mail", witness, true); !errors.Is(err, auth.ErrTrabajoSesion) {
		t.Fatal("journal adopted old A to B to A login", err)
	}
	base, err := ajustes.Carpeta()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(base, "private-work", "mail")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("stale session created a new app journal", err)
	}
}

func TestAppleJournalRoutingDoesNotCaptureAnotherAppDataScope(t *testing.T) {
	suite, _, _ := privateSuiteFixture(t)
	suite.app = consentApp{App: suite.app, metadata: fyne.AppMetadata{Custom: map[string]string{"esquema": "sytue-pdf"}}}
	witness, err := suite.TestigoTrabajo(context.Background(), storage.Scope{Tenant: "acme", Subject: "ana", Product: "mail"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = suite.almacenPrivadoAplicacion(t.Context(), "mail", witness, true); !errors.Is(err, ajustes.ErrTrabajoCompartidoNoDisponible) {
		t.Fatal("PDF app opened Mail authority", err)
	}
}

func TestRelocatedLegacyNotesDiscardKeepsOtherAccountAndSourceArchive(t *testing.T) {
	suite, work, _ := privateSuiteFixture(t)
	suite.app = consentApp{App: suite.app, metadata: fyne.AppMetadata{Custom: map[string]string{"esquema": "sytue-notes"}}}
	source, err := directorioAlmacen()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(source, 0o700) })
	for _, subject := range []string{"ana", "bea"} {
		if err = work.journal.Write(subject+"/acme", notas.EspacioBorradores, "pending", subject); err != nil {
			t.Fatal(err)
		}
	}
	witness, err := suite.TestigoTrabajo(t.Context(), storage.Scope{Tenant: "acme", Subject: "ana", Product: "notes"})
	if err != nil {
		t.Fatal(err)
	}
	journal, err := suite.almacenPrivadoAplicacion(t.Context(), "notes", witness, true)
	if err != nil {
		t.Fatal(err)
	}
	// The fixture registered an old view before migration. Retire that view: a
	// native presenter must be constructed with Resources.Journal after cutover.
	work.journal = journal
	if count, err := suite.PendientesSalida(); err != nil || count != 1 {
		t.Fatal("relocated legacy notes lost their current owner boundary", count, err)
	}
	if err = suite.SalirDescartando(t.Context()); err != nil {
		t.Fatal(err)
	}
	if count, err := journal.CountOwner("ana/acme", notas.EspacioBorradores); err != nil || count != 0 {
		t.Fatal("current legacy discard did not complete", count, err)
	}
	var other string
	if found, err := journal.Read("bea/acme", notas.EspacioBorradores, "pending", &other); err != nil || !found || other != "bea" {
		t.Fatal("logout erased another person's retained legacy draft", err)
	}
	target, err := directorioAlmacenAplicacion("notes")
	if err != nil {
		t.Fatal(err)
	}
	if retired, err := offline.JournalRelocationRetired(t.Context(), source, target); err != nil || !retired {
		t.Fatal("discard destroyed retained archive", retired, err)
	}
}

func TestClosedRelocationUncertaintyBlocksGlobalLogoutWithoutOpeningItsSource(t *testing.T) {
	suite, _, _ := privateSuiteFixture(t)
	source := t.TempDir()
	t.Cleanup(func() { _ = os.Chmod(source, 0o700) })
	staged, err := offline.OpenContext(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	if err = staged.Write("acme\x00ana\x00mail", storage.PendingUploadsNamespace, "pending", "retained"); err != nil {
		t.Fatal(err)
	}
	target, err := directorioAlmacenAplicacion("mail")
	if err != nil {
		t.Fatal(err)
	}
	witness, err := suite.TestigoTrabajo(t.Context(), storage.Scope{Tenant: "acme", Subject: "ana", Product: "mail"})
	if err != nil {
		t.Fatal(err)
	}
	journal, err := offline.RelocateJournalContext(t.Context(), source, target, witness.Exclusivo)
	if err != nil {
		t.Fatal(err)
	}
	if err = journal.Delete("acme\x00ana\x00mail", storage.PendingUploadsNamespace, "pending"); err != nil {
		t.Fatal(err)
	}
	if count, err := suite.PendientesSalida(); err != nil || count != 0 {
		t.Fatal("closed retired source permanently blocked its acknowledged work", count, err)
	}
	// Corrupt only the retained opaque archive. The Hub must detect uncertainty
	// from shared metadata rather than decrypting another app's data or key.
	retained := filepath.Join(filepath.Dir(target), "retained")
	directories, err := os.ReadDir(retained)
	if err != nil || len(directories) != 1 {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(retained, directories[0].Name(), "unknown.tmp"), []byte{0x01}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = suite.PendientesSalida(); !errors.Is(err, offline.ErrPendingJournal) {
		t.Fatal("closed source uncertainty became a clean global logout", err)
	}
	if err = suite.SalirDescartando(t.Context()); !errors.Is(err, offline.ErrPendingJournal) {
		t.Fatal("discard bypassed an uncertain closed source", err)
	}
}
