// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build js

package sesion

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// DiarioBoveda no existe en wasm (diario_boveda_nativo.go es `!js`), como
// AlmacenPrivadoAplicacion: wasm solo sirve para tipar los `cmd/` (`make
// fyne-cmd-check`), y sin esto cifrado/drive_boveda.go no compilaba allí.
func (*Suite) DiarioBoveda(context.Context, string, auth.TestigoTrabajo) (*offline.Store, error) {
	return nil, offline.ErrEncryptedStore
}
