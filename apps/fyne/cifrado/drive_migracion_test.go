// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"context"
	"errors"
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
)

// Con trabajo de Matrix sin terminar no se abre ni se toca la bóveda.
func TestMigrarDriveABovedaWaitsForMatrixWork(t *testing.T) {
	abiertas := 0
	err := MigrarDriveABoveda(t.Context(), MigracionDrive{
		Matrix: &privateDrive.Repository{}, Copiar: CopiarDesdeMatrix(nil), DriveURL: "https://drive.example.test",
		AbrirBoveda: func(context.Context, *api.PQVaultExpect) (*pqvault.Vault, error) {
			abiertas++
			return nil, pqvault.ErrSignin
		},
		PendientesMatrix: func(context.Context) (int, error) { return 1, nil },
	})
	if !errors.Is(err, ErrMatrixPendiente) || abiertas != 0 {
		t.Fatal("se copió con trabajo de Matrix pendiente", err, abiertas)
	}
	if err := MigrarDriveABoveda(t.Context(), MigracionDrive{}); !errors.Is(err, ErrPrivateResources) {
		t.Fatal("sin piezas no hay copia", err)
	}
}

// El paso no copia sin la casilla, y enseña lo que pasó: hecho, pendiente,
// entrar en Sytue ID, otro dispositivo copiando o fallo (Matrix sigue intacto y
// se puede reintentar).
func TestPasoBovedaNeedsConfirmationAndReportsOutcome(t *testing.T) {
	test.NewTempApp(t)
	t.Cleanup(func() { test.NewTempApp(t) })
	casos := []struct {
		err    error
		quiero EstadoPaso
	}{
		{nil, PasoHecho},
		{ErrMatrixPendiente, PasoPendiente},
		{pqvault.ErrSignin, PasoEntrar},
		{privateDrive.ErrVaultCopyInProgress, PasoOcupado},
		{pqvault.ErrCopyInProgress, PasoOcupado},
		{pqvault.ErrChanged, PasoFallo},
		{errors.New("red"), PasoFallo},
	}
	for _, caso := range casos {
		llamadas, hecho := 0, false
		paso := NuevoPasoBoveda(t.Context(), func(k string) string { return k })
		cola := make(chan func(), 1)
		paso.Dispatch = func(f func()) { cola <- f }
		paso.Migrar = func(context.Context) error {
			llamadas++
			return caso.err
		}
		paso.Hecho = func() { hecho = true }
		paso.Confirmar()
		if llamadas != 0 || paso.Estado() != PasoOferta {
			t.Fatal("se copió sin marcar la casilla")
		}
		paso.entiendo.SetChecked(true)
		paso.Confirmar()
		if paso.Estado() != PasoCopiando {
			t.Fatal("la copia no se ve en marcha")
		}
		(<-cola)() // terminar, en la rutina de la interfaz
		if paso.Estado() != caso.quiero || hecho != (caso.err == nil) {
			t.Fatalf("con %v enseña %d, quería %d (hecho=%v)", caso.err, paso.Estado(), caso.quiero, hecho)
		}
	}
}
