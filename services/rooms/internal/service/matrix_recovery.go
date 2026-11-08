// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"regexp"

	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// recoveryKeyShape es la forma de una clave de recuperación de Matrix: base58 en
// grupos de cuatro separados por espacios (~59 caracteres). Se deja holgura, pero
// nada que no sea esa forma entra en la custodia.
var recoveryKeyShape = regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{4}( [1-9A-HJ-NP-Za-km-z]{1,4}){8,24}$`)

// MatrixRecoveryKey devuelve la clave de recuperación que el servicio custodia
// para quien pregunta (ADR 0081). La identidad sale del token OIDC: nadie puede
// pedir la de otra persona.
func (s *Service) MatrixRecoveryKey(ctx context.Context, actor string) (string, error) {
	if err := s.matrixNativeActor(ctx, actor); err != nil {
		return "", err
	}
	return s.pstore.MatrixRecoveryKey(ctx, actor)
}

// StoreMatrixRecoveryKey custodia la clave que el navegador acaba de generar al
// configurar o restablecer el cifrado. La sustituye si ya había una.
func (s *Service) StoreMatrixRecoveryKey(ctx context.Context, actor, key string) error {
	if !recoveryKeyShape.MatchString(key) {
		return domain.ErrInvalidInput
	}
	if err := s.matrixNativeActor(ctx, actor); err != nil {
		return err
	}
	return s.pstore.PutMatrixRecoveryKey(ctx, actor, key)
}
