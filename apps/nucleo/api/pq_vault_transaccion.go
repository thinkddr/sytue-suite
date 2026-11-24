// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"net/url"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// FindPQVaultRecord busca en la bóveda `vaultID` el registro guardado con
// `transaction` (`GET records?vault_id=…&transaction_id=…`, una página de cero o
// un registro). `found` es falso si no hay ninguno; más de uno, o uno de otra
// bóveda, es un error: el servidor no puede elegir cuál vale. Es la consulta con
// la que la web lee los secretos de entrada del correo (ADR 0087).
func (d *Drive) FindPQVaultRecord(ctx context.Context, scope storage.Scope, vaultID, transaction string) (record PQVaultRecord, found bool, err error) {
	if !storage.Identifier(transaction) {
		return PQVaultRecord{}, false, storage.ErrRecord
	}
	path, headers, err := pqVaultRequest(scope, "/records")
	if err != nil {
		return PQVaultRecord{}, false, err
	}
	query := url.Values{"vault_id": {vaultID}, "transaction_id": {transaction}}
	page, err := JSON[PQVaultRecordPage](ctx, d.c, Request{Path: path, Header: headers, Query: query})
	if err != nil {
		return PQVaultRecord{}, false, err
	}
	switch len(page.Entries) {
	case 0:
		if page.NextCursor != "" {
			return PQVaultRecord{}, false, storage.ErrRecord
		}
		return PQVaultRecord{}, false, nil
	case 1:
		if page.Entries[0].VaultID != vaultID || page.NextCursor != "" {
			return PQVaultRecord{}, false, storage.ErrRecord
		}
		return page.Entries[0], true, nil
	default:
		return PQVaultRecord{}, false, storage.ErrRecord
	}
}
