// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// WopiEditor es el editor externo que una ORGANIZACIÓN ha conectado para abrir sus
// documentos. Estaba en una variable de entorno del servicio: conectar el Collabora de
// una empresa exigía editar el despliegue, y todas las organizaciones de una instalación
// compartían editor lo quisieran o no.
type WopiEditor struct {
	Tenant       string
	DiscoveryURL string
	Label        string
	Enabled      bool
	// CheckedAt/CheckOK/CheckError son el resultado de la última comprobación: la consola
	// dice si el editor responde en vez de dejar a quien lo conecta adivinando.
	CheckedAt  *time.Time
	CheckOK    bool
	CheckError string
	// Extensions son los tipos que el editor declaró la última vez que respondió.
	Extensions []string
	UpdatedBy  string
	UpdatedAt  time.Time
}

const wopiEditorCols = `tenant, discovery_url, label, enabled, checked_at, check_ok, check_error, extensions, updated_by, updated_at`

func scanWopiEditor(row pgx.Row) (WopiEditor, error) {
	var e WopiEditor
	err := row.Scan(&e.Tenant, &e.DiscoveryURL, &e.Label, &e.Enabled,
		&e.CheckedAt, &e.CheckOK, &e.CheckError, &e.Extensions, &e.UpdatedBy, &e.UpdatedAt)
	return e, err
}

// GetWopiEditor devuelve el editor de la organización del contexto.
func (s *Store) GetWopiEditor(ctx context.Context) (WopiEditor, error) {
	t := tenancy.MustFrom(ctx)
	var e WopiEditor
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		e, err = scanWopiEditor(tx.QueryRow(ctx,
			`SELECT `+wopiEditorCols+` FROM wopi_editors WHERE tenant=$1`, t.Slug))
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return WopiEditor{}, domain.ErrNotFound
	}
	return e, err
}

// WopiEditorFor resuelve el editor de UNA organización concreta, sin sesión de usuario.
//
// Lo necesita el host WOPI: el editor llama de vuelta con su propio token y en esa
// petición no hay JWT del que sacar la organización — viene DENTRO del token. Va por la
// transacción de mantenimiento, igual que el endpoint público de webhooks de Rooms, y
// nunca con un rol que se salte RLS.
func (s *Store) WopiEditorFor(ctx context.Context, tenant string) (WopiEditor, error) {
	var e WopiEditor
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		var err error
		e, err = scanWopiEditor(tx.QueryRow(ctx,
			`SELECT `+wopiEditorCols+` FROM wopi_editors WHERE tenant=$1 AND enabled`, tenant))
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return WopiEditor{}, domain.ErrNotFound
	}
	return e, err
}

// SaveWopiEditor guarda (o reemplaza) el editor de la organización.
//
// El resultado de la comprobación se escribe en la MISMA fila y no en una tabla aparte:
// solo interesa el último, y guardar un histórico de intentos fallidos de configuración
// no le sirve a nadie.
func (s *Store) SaveWopiEditor(ctx context.Context, e WopiEditor, actor string) (WopiEditor, error) {
	t := tenancy.MustFrom(ctx)
	// Un nil de Go llega a PostgreSQL como NULL y la columna no lo admite. Se normaliza
	// aquí y no en cada llamante: olvidarlo daría un error de restricción que no dice
	// nada de lo que se hizo mal.
	if e.Extensions == nil {
		e.Extensions = []string{}
	}
	var out WopiEditor
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = scanWopiEditor(tx.QueryRow(ctx, `
			INSERT INTO wopi_editors
			  (tenant, discovery_url, label, enabled, checked_at, check_ok, check_error, extensions, updated_by, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
			ON CONFLICT (tenant) DO UPDATE
			   SET discovery_url = EXCLUDED.discovery_url,
			       label         = EXCLUDED.label,
			       enabled       = EXCLUDED.enabled,
			       checked_at    = EXCLUDED.checked_at,
			       check_ok      = EXCLUDED.check_ok,
			       check_error   = EXCLUDED.check_error,
			       extensions    = EXCLUDED.extensions,
			       updated_by    = EXCLUDED.updated_by,
			       updated_at    = now()
			RETURNING `+wopiEditorCols,
			t.Slug, e.DiscoveryURL, e.Label, e.Enabled,
			e.CheckedAt, e.CheckOK, e.CheckError, e.Extensions, actor))
		return err
	})
	if err != nil {
		return WopiEditor{}, fmt.Errorf("store: guardar editor wopi: %w", err)
	}
	return out, nil
}

// DeleteWopiEditor desconecta el editor de la organización.
func (s *Store) DeleteWopiEditor(ctx context.Context) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM wopi_editors WHERE tenant=$1`, t.Slug)
		if err != nil {
			return fmt.Errorf("store: desconectar editor wopi: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}
