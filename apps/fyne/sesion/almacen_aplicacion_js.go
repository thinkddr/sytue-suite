// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build js

package sesion

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

func (*Suite) AlmacenPrivadoAplicacion(context.Context, string, auth.TestigoTrabajo) (*offline.Store, error) {
	return nil, offline.ErrEncryptedStore
}
