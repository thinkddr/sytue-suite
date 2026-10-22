// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// VistasFiltro devuelve las vistas de filtro de ESTA persona en el documento (0018).
// La clave de la fila es (documento, sub): no hay consulta que lea las de otra persona.
// ok=false si aún no ha guardado ninguna.
func (s *Store) VistasFiltro(ctx context.Context, docID, sub string) (estado json.RawMessage, ok bool, err error) {
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx,
			`SELECT estado FROM doc_vistas_filtro WHERE document_id = $1 AND sub = $2 LIMIT 1`,
			docID, sub).Scan(&estado)
		if isNoRows(e) {
			return nil
		}
		ok = e == nil
		return e
	})
	return estado, ok, err
}

// GuardarVistasFiltro sustituye las vistas de esta persona (último gana).
func (s *Store) GuardarVistasFiltro(ctx context.Context, docID, sub string, estado json.RawMessage) error {
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	return s.documentTx(ctx, docID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO doc_vistas_filtro (document_id, tenant, sub, estado)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (document_id, sub)
			DO UPDATE SET estado = EXCLUDED.estado, actualizada = now()`,
			docID, t.Slug, sub, []byte(estado))
		return err
	})
}
