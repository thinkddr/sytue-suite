// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"errors"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// ProvisionPrivateStorage is an explicit first-use operation. The account,
// system folders, complete metadata source and frozen routing become visible
// atomically. SMTP cannot observe a new account in legacy clear-storage mode.
// An existing account keeps its full original source; it is never replaced by
// a manufactured empty inventory.
func (s *Store) ProvisionPrivateStorage(ctx context.Context, owner, address, name string, quota int64, epoch uuid.UUID, open func(context.Context, string) (io.ReadCloser, error)) (storage.LegacyMigrationState, error) {
	if epoch == uuid.Nil || address == "" || quota <= 0 || open == nil {
		return storage.LegacyMigrationState{}, ErrPrivateStorageEpoch
	}
	var result storage.LegacyMigrationState
	err := s.privateTx(ctx, owner, func(tx pgx.Tx) error {
		_, _, err := insertAccountIfAbsent(ctx, tx, tenantOf(ctx), owner, address, name, quota)
		if err != nil {
			return err
		}
		result, err = s.beginPrivateStorageTx(ctx, tx, owner, epoch, open)
		return err
	})
	return result, err
}

// Shared by ordinary provisioning and explicit private first-use. A concurrent
// winner is left unchanged; callers decide whether existing profile updates or
// a full source capture are appropriate.
func insertAccountIfAbsent(ctx context.Context, tx pgx.Tx, tenant, owner, address, name string, quota int64) (accountID uuid.UUID, created bool, err error) {
	err = tx.QueryRow(ctx, "INSERT INTO accounts(tenant,subject,address,display_name,quota_bytes) VALUES($1,$2,$3,$4,$5) ON CONFLICT(tenant,subject) DO NOTHING RETURNING id", tenant, owner, address, name, quota).Scan(&accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	if err := ensureDefaultMailboxes(ctx, tx, tenant, accountID); err != nil {
		return uuid.Nil, false, err
	}
	return accountID, true, nil
}
