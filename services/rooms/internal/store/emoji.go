// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// customEmojiCols son las columnas de un emoji personalizado en orden de escaneo.
const customEmojiCols = `name, file_id, content_type, created_by, created_at`

func scanCustomEmoji(row pgx.Row) (domain.CustomEmoji, error) {
	var e domain.CustomEmoji
	err := row.Scan(&e.Name, &e.FileID, &e.ContentType, &e.CreatedBy, &e.CreatedAt)
	return e, err
}

// CreateCustomEmoji inserta un emoji personalizado del tenant. El nombre es único
// por tenant (PK): un duplicado devuelve ErrConflict. Aislado por tenant vía RLS
// (mig. 0013). El servicio ya validó el nombre y resolvió content_type en Drive.
func (s *Store) CreateCustomEmoji(ctx context.Context, name string, fileID uuid.UUID, contentType, createdBy string) (domain.CustomEmoji, error) {
	t := tenancy.MustFrom(ctx)
	var e domain.CustomEmoji
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`INSERT INTO custom_emoji (tenant, name, file_id, content_type, created_by)
			 VALUES ($1, $2, $3, $4, $5) RETURNING `+customEmojiCols,
			t.Slug, name, fileID, contentType, createdBy)
		var e2 error
		e, e2 = scanCustomEmoji(row)
		if uniqueViolation(e2) {
			return domain.ErrConflict
		}
		return e2
	})
	return e, err
}

// ListCustomEmoji lista los emoji personalizados del tenant, por nombre. Una sola
// consulta cubierta por la PK (tenant, name); el catálogo es acotado por tenant.
func (s *Store) ListCustomEmoji(ctx context.Context) ([]domain.CustomEmoji, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.CustomEmoji
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+customEmojiCols+` FROM custom_emoji WHERE tenant=$1 ORDER BY name`, t.Slug)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			e, err := scanCustomEmoji(rows)
			if err != nil {
				return err
			}
			out = append(out, e)
		}
		return rows.Err()
	})
	return out, err
}

// DeleteCustomEmoji borra un emoji personalizado. Solo su autor o un administrador
// del workspace (admin de algún team del tenant) pueden. ErrNotFound si no existe.
func (s *Store) DeleteCustomEmoji(ctx context.Context, name, actor string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var createdBy string
		err := tx.QueryRow(ctx,
			`SELECT created_by FROM custom_emoji WHERE tenant=$1 AND name=$2`,
			t.Slug, name).Scan(&createdBy)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if actor != createdBy {
			if !esAdminDeLaOrganizacion(ctx, actor) {
				return domain.ErrForbidden
			}
		}
		_, err = tx.Exec(ctx, `DELETE FROM custom_emoji WHERE tenant=$1 AND name=$2`, t.Slug, name)
		return err
	})
}

// esAdminDeLaOrganizacion dice si `subject` es quien llama Y tiene en su token el rol
// `owner` o `admin` de la organización de la sesión. Es el «administrador del espacio»
// de Rooms: quien gestiona lo que es de TODA la organización (bots, comandos, WhatsApp,
// webhooks salientes sin canal, moderar emoji).
//
// EL ROL LO DA SYTUE ID, NO UN EQUIPO. Antes valía ser admin de ALGÚN equipo, y un
// equipo lo crea cualquiera quedando como su admin: con eso, cualquier miembro se
// hacía administrador del espacio y, con un webhook saliente sin canal, recibía los
// mensajes de todos los canales, privados y directos incluidos.
//
// Sale del contexto y no de la base porque es un dato de la sesión: una persona que
// viene de OTRA organización llega aquí sin roles (chathttp.EnAnfitrion se los quita),
// un bot nunca los tiene, y un proceso sin sesión tampoco.
func esAdminDeLaOrganizacion(ctx context.Context, subject string) bool {
	c, ok := authn.FromContext(ctx)
	if !ok || subject == "" || c.Subject != subject {
		return false
	}
	if t, err := tenancy.From(ctx); err != nil || c.Tenant != t.Slug {
		return false
	}
	return c.HasAnyRole("owner", "admin")
}
