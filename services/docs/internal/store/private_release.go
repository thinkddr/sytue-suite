// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// Release es «soltar la fuente» (ADR 0086, platform/pkg/soltarfuente):
// devuelve a legacy la fuente frozen/private de esta persona, en la misma
// transacción de dueño que su congelación. El servicio ya comprobó la
// confirmación y la reautenticación.
func (p *PrivateSource) Release(ctx context.Context, scope storage.Scope, release pg.PrivateStorageRelease) error {
	return p.ownerTx(ctx, scope, func(tx pgx.Tx) error {
		_, err := pg.ReleasePrivateStorageState(ctx, tx, scope.Product+"_private_storage_states", scope.Tenant, scope.Subject, release)
		return err
	})
}
