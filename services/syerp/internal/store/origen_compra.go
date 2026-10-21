// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
)

func conservarOrigenCompra(previo, data map[string]any) error {
	original := cadenaDe(previo["externo_id"])
	if valor, hay := data["externo_id"]; hay && cadenaDe(valor) != original {
		return fmt.Errorf("la identidad de origen no se puede cambiar: %w", domain.ErrDatos)
	}
	if original != "" {
		data["externo_id"] = original
	}
	return nil
}

// El lector XML y el importador comparten la identidad proveedor/número. El
// cerrojo evita carreras sin imponer un índice a compras históricas que puedan
// estar duplicadas: no modifica ni elimina documentos existentes.
func bloquearOrigenCompra(ctx context.Context, tx pgx.Tx, modelo, externo string) error {
	if modelo != "factura_compra" || externo == "" {
		return nil
	}
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(jsonb_build_array(current_setting('app.tenant', true), $1::text, $2::text)::text, 0))`, modelo, externo)
	return err
}

func comprobarOrigenCompra(ctx context.Context, tx pgx.Tx, modelo string, data map[string]any) error {
	externo := cadenaDe(data["externo_id"])
	if modelo != "factura_compra" || externo == "" {
		return nil
	}
	if err := bloquearOrigenCompra(ctx, tx, modelo, externo); err != nil {
		return err
	}
	var existe bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM records WHERE tenant = current_setting('app.tenant', true) AND model=$1 AND data->>'externo_id'=$2 AND deleted_at IS NULL)`, modelo, externo).Scan(&existe); err != nil {
		return err
	}
	if existe {
		return ErrRefDuplicada
	}
	return nil
}
