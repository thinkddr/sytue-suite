// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pqvault

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// HasRecords dice, sin clave de desbloqueo y sin crear nada, si la bóveda de
// `scope` ya guarda algún registro. Es lo que un cliente nativo pregunta para
// elegir almacén: una bóveda con registros es que la web ya pasó ahí los datos de
// esa persona (ADR 0086, «Drive nativo sobre la bóveda»).
//
// Al contrario que Open, nunca crea ni sella una bóveda: solo lee el estado de
// las claves y la primera página de registros, que van cifrados. Ante cualquier
// error devuelve el error, nunca «vacía»: quien elige con esto no debe tomar una
// duda por una respuesta.
func HasRecords(ctx context.Context, server Server, scope storage.Scope) (bool, error) {
	if server == nil || !scope.Valid() {
		return false, ErrIdentity
	}
	state, err := server.PQVaultKeys(ctx, scope)
	if err != nil {
		return false, err
	}
	if state.VaultID == "" && len(state.Keys) == 0 {
		return false, nil
	}
	if !validVaultID(state.VaultID) {
		return false, ErrIdentity
	}
	page, err := server.PQVaultRecords(ctx, scope, state.VaultID, "", 1)
	if err != nil {
		return false, err
	}
	return len(page.Entries) > 0 || page.NextCursor != "", nil
}
