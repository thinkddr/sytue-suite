// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// ReleasePrivateStorage es «soltar la fuente» (ADR 0086, platform/pkg/soltarfuente):
// devuelve a legacy la fuente frozen/private del Drive de esta persona, con sus
// datos tal como se congelaron (la guarda de escritura no dejó tocarlos después).
// El servicio ya comprobó la confirmación y la reautenticación; el ámbito tiene que
// ser el de la sesión.
func (s *Store) ReleasePrivateStorage(ctx context.Context, scope storage.Scope, release pg.PrivateStorageRelease) error {
	owner, err := privateOwner(ctx)
	if err != nil {
		return err
	}
	tenant := tenancy.MustFrom(ctx).Slug
	if scope != (storage.Scope{Tenant: tenant, Subject: owner, Product: "drive"}) {
		return pg.ErrPrivateStorageScope
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		_, err := pg.ReleasePrivateStorageState(ctx, tx, "drive_private_storage_states", tenant, owner, release)
		return err
	})
}
