// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"errors"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var anaDrive = storage.Scope{Tenant: "acme", Subject: "ana", Product: "drive"}

const duenoAnaDrive = "acme\x00ana\x00drive"

// El diario de la bóveda es otro directorio que el de Matrix: lo que se escribe
// en uno no aparece en el otro, aunque use el mismo espacio de nombres.
func TestVaultJournalIsSeparateFromMatrixJournal(t *testing.T) {
	suite, _, _ := privateSuiteFixture(t)
	suite.app = consentApp{App: suite.app, metadata: fyne.AppMetadata{Custom: map[string]string{"esquema": "sytue-drive"}}}
	witness, err := suite.TestigoTrabajo(t.Context(), anaDrive)
	if err != nil {
		t.Fatal(err)
	}
	vault, err := suite.DiarioBoveda(t.Context(), "drive", witness)
	if err != nil {
		t.Fatal(err)
	}
	if err := vault.Write(duenoAnaDrive, storage.PendingUploadsNamespace, "job_boveda", "boveda"); err != nil {
		t.Fatal(err)
	}
	matrix, err := Almacen()
	if err != nil {
		t.Fatal(err)
	}
	if found, err := matrix.Read(duenoAnaDrive, storage.PendingUploadsNamespace, "job_boveda", new(string)); err != nil || found {
		t.Fatal("la intención de la bóveda apareció en el diario de Matrix", found, err)
	}
	if err := matrix.Write(duenoAnaDrive, storage.PendingUploadsNamespace, "job_matrix", "matrix"); err != nil {
		t.Fatal(err)
	}
	ana := suite.Sesion.SnapshotIdentity()
	if n, err := suite.PendientesPrivadosDe(t.Context(), ana, "drive", true); err != nil || n != 1 {
		t.Fatal("la bóveda cuenta solo lo suyo", n, err)
	}
	if n, err := suite.PendientesPrivadosDe(t.Context(), ana, "drive", false); err != nil || n != 1 {
		t.Fatal("Matrix cuenta solo lo suyo", n, err)
	}
	if found, err := vault.Read(duenoAnaDrive, storage.PendingUploadsNamespace, "job_matrix", new(string)); err != nil || found {
		t.Fatal("la intención de Matrix apareció en el diario de la bóveda", found, err)
	}

	// Drive no guarda el trabajo de otro producto que no consume.
	if _, err := suite.DiarioBoveda(t.Context(), "contacts", witness); err == nil {
		t.Fatal("drive abrió el diario de la bóveda de contacts")
	}
}

// Un testigo de otra sesión no abre el diario.
func TestVaultJournalRequiresCurrentWitness(t *testing.T) {
	suite, _, _ := privateSuiteFixture(t)
	suite.app = consentApp{App: suite.app, metadata: fyne.AppMetadata{Custom: map[string]string{"esquema": "sytue-drive"}}}
	if _, err := suite.DiarioBoveda(t.Context(), "drive", auth.TestigoTrabajo{}); err == nil {
		t.Fatal("sin testigo válido no hay diario")
	}
}

// Salir: el trabajo pendiente de la bóveda bloquea la salida como el de Matrix, y
// el descarte explícito lo borra solo para la cuenta que sale.
func TestVaultJournalBlocksLogoutAndIsWipedByDiscard(t *testing.T) {
	suite, _, _ := privateSuiteFixture(t)
	base, err := ajustes.Carpeta()
	if err != nil {
		t.Fatal(err)
	}
	dirs := map[string]string{}
	for _, app := range []string{"drive", "photos"} {
		dirs[app] = filepath.Join(base, "private-work", app, "pq-vault")
		journal, err := offline.OpenWithSecrets(dirs[app], new(authtest.Secrets))
		if err != nil {
			t.Fatal(err)
		}
		for _, subject := range []string{"ana", "bea"} {
			if err := journal.Write("acme\x00"+subject+"\x00drive", storage.PendingUploadsNamespace, "job", subject); err != nil {
				t.Fatal(err)
			}
		}
		// Lo que no es trabajo duradero se va con cualquier salida.
		if err := journal.Write("acme\x00bea\x00drive", "volatil-v1", "x", "x"); err != nil {
			t.Fatal(err)
		}
	}
	if count, err := suite.PendientesSalida(); err != nil || count != 2 {
		t.Fatal("la salida no vio el trabajo de la bóveda", count, err)
	}
	if err := suite.SalirConError(); !errors.Is(err, ErrTrabajoPendiente) {
		t.Fatal("se salió con subidas de la bóveda pendientes", err)
	}
	if err := suite.SalirDescartando(t.Context()); err != nil {
		t.Fatal(err)
	}
	for app, dir := range dirs {
		if count, err := offline.CountDirectoryOwner(dir, duenoAnaDrive, storage.PendingUploadsNamespace); err != nil || count != 0 {
			t.Fatal("el descarte no borró el diario de la bóveda", app, count, err)
		}
		if count, err := offline.CountDirectoryOwner(dir, "acme\x00bea\x00drive", storage.PendingUploadsNamespace); err != nil || count != 1 {
			t.Fatal("el descarte borró el trabajo de otra cuenta", app, count, err)
		}
		if count, err := offline.CountDirectory(dir, "volatil-v1"); err != nil || count != 0 {
			t.Fatal("la salida no limpió lo efímero del diario de la bóveda", app, count, err)
		}
	}
}
