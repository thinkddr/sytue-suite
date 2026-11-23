// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// ReleasePrivateStorage es «soltar la fuente» (ADR 0086, platform/pkg/soltarfuente):
// devuelve a legacy la fuente frozen/private de esta persona, en la misma
// transacción de dueño que su congelación. El servicio ya comprobó la
// confirmación y la reautenticación.
func (s *Store) ReleasePrivateStorage(ctx context.Context, scope storage.Scope, release pg.PrivateStorageRelease) error {
	return s.privateBasesTx(ctx, scope.Subject, func(own storage.Scope, tx pgx.Tx) error {
		if own != scope {
			return pg.ErrPrivateStorageScope
		}
		_, err := pg.ReleasePrivateStorageState(ctx, tx, privateBasesStateTable, scope.Tenant, scope.Subject, release)
		return err
	})
}
