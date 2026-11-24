// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// La autoridad de almacenamiento de un ámbito que vive en la bóveda (ADR 0087).
//
// Un ámbito tiene UNA autoridad, y la regla para saber cuál es no deja huecos:
//
//   - Si hay autoridad de Matrix (una fila en matrix_storage_authorities, activa o
//     revocada), esa es la autoridad. Quien ya abre su correo por Matrix sigue
//     igual, y una clave de bóveda nunca la sustituye por la puerta de atrás.
//   - Si no la hay y Sytue ID tiene una clave de bóveda activa para ese ámbito, la
//     autoridad es la de la bóveda: su generación y su raíz (la mitad Ed25519).
//   - Si no hay ninguna de las dos, es un alta, como antes.
//
// Y para que la segunda no cambie por debajo, un ámbito de bóveda ya no admite
// dar de alta una raíz de Matrix.
//
// Solo para los productos que reciben por la bóveda: todas las bóvedas registran
// su clave en Sytue ID (ADR 0087, entrega 4a), pero solo el correo verifica con
// ella su entrada. Calendario sigue con Matrix aunque tenga clave.

// productosConEntradaDeBoveda son los productos cuya entrada puede anclarse en la bóveda.
var productosConEntradaDeBoveda = map[string]bool{"mail": true}

// autoridadConBoveda decide la autoridad de un ámbito a partir de la de Matrix
// (`matrix`, `errMatrix` tal como los devolvió matrixAuthorityTx) y del registro
// de claves de bóveda. No mira nada más.
func autoridadConBoveda(ctx context.Context, scope storage.Scope, matrix matrixproof.Authority, errMatrix error, claves ClavesBoveda) (matrixproof.Authority, error) {
	if !errors.Is(errMatrix, matrixproof.ErrRootEnrollment) || claves == nil || !productosConEntradaDeBoveda[scope.Product] {
		return matrix, errMatrix
	}
	reg, err := claves.Activa(ctx, scope)
	switch {
	case errors.Is(err, ErrSinClaveDeBoveda):
		return matrix, errMatrix
	case err != nil:
		// Sin saber si hay bóveda no se puede contestar «alta»: un cliente de Matrix
		// daría de alta su raíz encima de una bóveda.
		return matrixproof.Authority{}, domain.ErrMatrixUnavailable
	}
	return autoridadSobre(scope, reg, 0)
}

// raizMatrixPermitidaEn dice si en este ámbito se puede dar de alta una raíz de
// Matrix: no, si el ámbito ya es de la bóveda.
func raizMatrixPermitidaEn(ctx context.Context, scope storage.Scope, claves ClavesBoveda) error {
	if claves == nil || !productosConEntradaDeBoveda[scope.Product] {
		return nil
	}
	_, err := claves.Activa(ctx, scope)
	switch {
	case errors.Is(err, ErrSinClaveDeBoveda):
		return nil
	case err != nil:
		return domain.ErrMatrixUnavailable
	}
	return domain.ErrConflict
}

// raizMatrixPermitida: ni en un ámbito de la bóveda ni con Matrix retirada en ese
// producto (retirada_matrix.go). Guarda el alta y la confirmación de cualquier
// cambio de raíz (ConfirmMatrixRoot).
func (s *Store) raizMatrixPermitida(ctx context.Context, subject, product string) error {
	scope, err := cutoverScope(ctx, subject, product)
	if err != nil {
		return err
	}
	if err := s.raizMatrixAdmitida(scope.Product); err != nil {
		return err
	}
	return raizMatrixPermitidaEn(ctx, scope, s.clavesBoveda)
}
