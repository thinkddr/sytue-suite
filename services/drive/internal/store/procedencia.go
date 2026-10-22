// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// Procedencia es de dónde salió una versión de un fichero que Drive leyó de otro producto
// (mig 0029). `Detail` es lo que contó el origen; nunca lleva credenciales.
type Procedencia struct {
	Version   int             `json:"version"`
	Source    string          `json:"source"`
	Detail    json.RawMessage `json:"detail"`
	CreatedBy string          `json:"created_by"`
	CreatedAt time.Time       `json:"created_at"`
}

// GuardarProcedencia anota de dónde salió la versión `version` del fichero. Reexportar la
// misma versión (no debería ocurrir: cada exportación sube la versión) la reemplaza.
func (s *Store) GuardarProcedencia(ctx context.Context, fileID uuid.UUID, version int, source string, detail json.RawMessage, sub string) error {
	t := tenancy.MustFrom(ctx)
	return s.inTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO drive_file_provenance (tenant, file_id, version, source, detail, created_by)
			VALUES ($1, $2, $3, $4, $5::jsonb, $6)
			ON CONFLICT (file_id, version) DO UPDATE SET source = EXCLUDED.source, detail = EXCLUDED.detail,
				created_by = EXCLUDED.created_by, created_at = now()`,
			t.Slug, fileID, version, source, []byte(detail), sub)
		return err
	})
}

// ProcedenciaDe devuelve la procedencia de las versiones de un fichero, la más nueva
// primero. Vacío si el fichero lo subió una persona.
func (s *Store) ProcedenciaDe(ctx context.Context, fileID uuid.UUID) ([]Procedencia, error) {
	t := tenancy.MustFrom(ctx)
	out := []Procedencia{}
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT version, source, detail, created_by, created_at FROM drive_file_provenance
			 WHERE tenant = $1 AND file_id = $2 ORDER BY version DESC LIMIT 100`, t.Slug, fileID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p Procedencia
			var d []byte
			if err := rows.Scan(&p.Version, &p.Source, &d, &p.CreatedBy, &p.CreatedAt); err != nil {
				return err
			}
			p.Detail = d
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}
