// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build !js

package sesion

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// DiarioBoveda abre el diario de la bóveda de `product` de esta app instalada,
// con el mismo testigo de trabajo que el de Matrix: si la sesión ya no es la del
// testigo, no abre nada. Es un almacén cifrado aparte del de Matrix
// (diario_boveda.go); nunca comparte fichero ni espacio de nombres con él.
func (s *Suite) DiarioBoveda(ctx context.Context, product string, witness auth.TestigoTrabajo) (*offline.Store, error) {
	if s == nil || !storage.Product(product) {
		return nil, auth.ErrTrabajoSesion
	}
	app, err := productoAplicacionNativa(s.app)
	if err != nil {
		return nil, err
	}
	if app != product && storage.NativeReturnApp(product, app) == "" {
		return nil, auth.ErrTrabajoSesion
	}
	if err := s.validarAlmacenAplicacion(ctx, witness); err != nil {
		return nil, err
	}
	dir, err := directorioDiarioBoveda(app)
	if err != nil {
		return nil, err
	}
	journal, err := offline.OpenContext(ctx, dir)
	if err != nil {
		return nil, err
	}
	if !journal.Encrypted() {
		return nil, offline.ErrEncryptedStore
	}
	// Abrir no es permiso: la sesión puede haber cambiado mientras.
	if err := s.validarAlmacenAplicacion(ctx, witness); err != nil {
		return nil, err
	}
	return journal, nil
}
