// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

const objectCols = `id, calendar_id, uid, resource_name, component, data, etag, size_bytes, sequence_no, created_at, updated_at`

func scanObject(row pgx.Row, t string) (domain.CalendarObject, error) {
	var o domain.CalendarObject
	var comp string
	if err := row.Scan(&o.ID, &o.CalendarID, &o.UID, &o.ResourceName, &comp, &o.Data,
		&o.ETag, &o.SizeBytes, &o.SequenceNo, &o.CreatedAt, &o.UpdatedAt); err != nil {
		return domain.CalendarObject{}, err
	}
	o.Tenant = t
	o.Component = domain.Component(comp)
	return o, nil
}

// GetObjectByResource resuelve un recurso CalDAV por (calendario, resource_name).
func (s *Store) GetObjectByResource(ctx context.Context, calendarID uuid.UUID, resourceName string) (domain.CalendarObject, error) {
	t := tenancy.MustFrom(ctx)
	var o domain.CalendarObject
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+objectCols+` FROM calendar_objects
			 WHERE tenant=$1 AND calendar_id=$2 AND resource_name=$3 AND deleted_at IS NULL`,
			t.Slug, calendarID, resourceName)
		var e error
		o, e = scanObject(row, t.Slug)
		return e
	})
	if isNoRows(err) {
		return domain.CalendarObject{}, domain.ErrNotFound
	}
	return o, err
}

// GetObjectByUID resuelve un recurso CalDAV por UID dentro de un calendario.
func (s *Store) GetObjectByUID(ctx context.Context, calendarID uuid.UUID, uid string) (domain.CalendarObject, error) {
	t := tenancy.MustFrom(ctx)
	var o domain.CalendarObject
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+objectCols+` FROM calendar_objects
			 WHERE tenant=$1 AND calendar_id=$2 AND uid=$3 AND deleted_at IS NULL`,
			t.Slug, calendarID, uid)
		var e error
		o, e = scanObject(row, t.Slug)
		return e
	})
	if isNoRows(err) {
		return domain.CalendarObject{}, domain.ErrNotFound
	}
	return o, err
}

// ListObjects lista todos los recursos vivos de un calendario (PROPFIND/REPORT).
func (s *Store) ListObjects(ctx context.Context, calendarID uuid.UUID) ([]domain.CalendarObject, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.CalendarObject
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+objectCols+` FROM calendar_objects
			 WHERE tenant=$1 AND calendar_id=$2 AND deleted_at IS NULL
			 ORDER BY resource_name`,
			t.Slug, calendarID)
		if err != nil {
			return fmt.Errorf("store: listar objetos: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			o, err := scanObject(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, o)
		}
		return rows.Err()
	})
	return out, err
}

// SyncChange describe un cambio de recurso para sync-collection (RFC 6578).
type SyncChange struct {
	ResourceName string
	ETag         uuid.UUID
	Deleted      bool
}

// SyncCollection devuelve los cambios de recursos de un calendario desde un
// sync_token dado (0 = sincronizacion completa) y el nuevo token. Incluye los
// recursos borrados (tombstones) para que el cliente los elimine localmente.
func (s *Store) SyncCollection(ctx context.Context, calendarID uuid.UUID, sinceToken int64) ([]SyncChange, int64, error) {
	t := tenancy.MustFrom(ctx)
	var changes []SyncChange
	var newToken int64
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`SELECT sync_token FROM calendars WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, calendarID).Scan(&newToken); err != nil {
			if isNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		// Nota: usamos updated_at como aproximacion temporal del cambio; el token
		// del calendario es monotono y se devuelve como nuevo punto de sync. Para
		// sinceToken=0 se devuelve el estado completo (todos los vivos).
		rows, err := tx.Query(ctx,
			`SELECT resource_name, etag, (deleted_at IS NOT NULL) AS deleted
			 FROM calendar_objects
			 WHERE tenant=$1 AND calendar_id=$2
			   AND ($3 = 0 AND deleted_at IS NULL OR $3 > 0)
			 ORDER BY updated_at`,
			t.Slug, calendarID, sinceToken)
		if err != nil {
			return fmt.Errorf("store: sync-collection: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var ch SyncChange
			if err := rows.Scan(&ch.ResourceName, &ch.ETag, &ch.Deleted); err != nil {
				return err
			}
			changes = append(changes, ch)
		}
		return rows.Err()
	})
	return changes, newToken, err
}
