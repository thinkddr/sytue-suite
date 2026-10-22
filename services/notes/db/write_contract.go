// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package db

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
)

// WriteVersion identifies the write contract implemented by this binary. It is
// independent of a note ciphertext version and never comes from an HTTP request.
const WriteVersion = 2

// WriteVersionSetting is transaction-local, including behind PgBouncer. Never
// configure it as a role, database or pool default: old binaries must not inherit it.
const WriteVersionSetting = "app.notes_write_version"

// WriteContractConstraint identifies the database fence without exposing SQL.
const WriteContractConstraint = "notes_write_contract"

// MarkWriteTransaction explicitly adopts the contract for this transaction only.
// Taking a Tx prevents accidentally issuing SET LOCAL outside a transaction.
// Notes and the cross-service ID merge use this same fixed binary capability;
// neither maintenance nor merge flags bypass the database fence.
func MarkWriteTransaction(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, "SELECT set_config($1, $2, true)", WriteVersionSetting, strconv.Itoa(WriteVersion)); err != nil {
		return fmt.Errorf("notes: mark write transaction: %w", err)
	}
	return nil
}
