// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"

	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	matrixclient "git.sytue.eu/sytue/one/services/rooms/internal/matrix"
)

func requireSamePublicationOrigin(ctx context.Context, backend *matrixclient.Client, previous matrixclient.PublicationCoverage) error {
	current, err := backend.RequirePublicationCoverage(ctx)
	if err != nil {
		return err
	}
	if !previous.SameProcess(current) {
		return domain.ErrMatrixUnavailable
	}
	return nil
}
