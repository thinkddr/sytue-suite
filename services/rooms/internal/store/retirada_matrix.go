// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// La retirada de la autoridad de Matrix del correo (ADR 0087, entrega 5).
//
// Con el conmutador (SYTUE_MAIL_SIN_MATRIX=activado), Rooms deja de honrar la
// autoridad de Matrix de `mail`. Un ámbito de Matrix sin traspaso queda como
// queda uno tras el traspaso, pero sin la bóveda:
//
//   - La lectura de la autoridad devuelve domain.ErrMatrixStorageRetired (el 409
//     `storage_matrix_retired`): «Matrix está retirada, pasa a la bóveda». Si
//     Sytue ID no contesta, no hay respuesta (ErrMatrixUnavailable): sin saber si
//     ya hubo un traspaso, ni Matrix ni «pasa» (podría pedir vaciar otra vez).
//   - El traspaso sigue igual: una clave `traspaso` posterior al último cambio de
//     la autoridad de Matrix da la de la bóveda, y queda escrito.
//   - No se da de alta ni se rota ni se reanuda una raíz de Matrix
//     (ErrMatrixStorageRetired), y las pruebas de Matrix (acciones de entrada,
//     vallas) y sus recibos se rechazan como tras un traspaso (ErrUnverified).
//
// Sin autoridad (cuenta nueva) o con la de la bóveda, nada cambia; y ningún otro
// producto cambia: calendario registra clave de bóveda pero recibe por Matrix.
// Apagado (el defecto), todo es como antes.

// SetCorreoSinMatrix enciende o apaga la retirada de la autoridad de Matrix del correo.
func (s *Store) SetCorreoSinMatrix(activado bool) { s.correoSinMatrix = activado }

// matrixRetirada dice si en este producto Rooms ya no honra la autoridad de Matrix.
func (s *Store) matrixRetirada(product string) bool {
	return s.correoSinMatrix && productosConEntradaDeBoveda[product]
}

// pruebaMatrixAdmitida rechaza las pruebas de Matrix de un producto retirado igual
// que las de un ámbito traspasado.
func (s *Store) pruebaMatrixAdmitida(product string) error {
	if s.matrixRetirada(product) {
		return matrixproof.ErrUnverified
	}
	return nil
}

// raizMatrixAdmitida rechaza dar de alta, rotar o reanudar una raíz de Matrix en
// un producto retirado.
func (s *Store) raizMatrixAdmitida(product string) error {
	if s.matrixRetirada(product) {
		return domain.ErrMatrixStorageRetired
	}
	return nil
}

// matrixSinTraspaso es la respuesta para un ámbito con autoridad de Matrix y sin
// traspaso: la de Matrix, o que está retirada.
func (s *Store) matrixSinTraspaso(product string, matrix matrixproof.Authority) (matrixproof.Authority, error) {
	if s.matrixRetirada(product) {
		return matrixproof.Authority{}, domain.ErrMatrixStorageRetired
	}
	return matrix, nil
}
