// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// Categorías del sidebar por usuario (Ola 11). Estado PER-USUARIO (subject): no exige
// pertenencia a ningún team. La asignación canal→categoría vive en channel_members
// (category_id/sort_order) y la lee ListChannels; ver mig. 0009.

const categoryCols = `id, name, sort_order, collapsed, created_at`

func scanCategory(row pgx.Row, tenant, subject string) (domain.Category, error) {
	var c domain.Category
	if err := row.Scan(&c.ID, &c.Name, &c.SortOrder, &c.Collapsed, &c.CreatedAt); err != nil {
		return domain.Category{}, err
	}
	c.Tenant = tenant
	c.Subject = subject
	return c, nil
}

// ListCategories devuelve las categorías del sidebar del usuario ordenadas por
// sort_order (created_at como desempate estable). Cubierto por channel_categories_order.
func (s *Store) ListCategories(ctx context.Context, subject string) ([]domain.Category, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Category
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+categoryCols+` FROM channel_categories
			 WHERE tenant=$1 AND subject=$2
			 ORDER BY sort_order, created_at`, t.Slug, subject)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			c, err := scanCategory(rows, t.Slug, subject)
			if err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// CreateCategory crea una categoría al final del orden del usuario (sort_order = max+1).
// El nombre ya viene validado por el servicio.
func (s *Store) CreateCategory(ctx context.Context, subject, name string) (domain.Category, error) {
	t := tenancy.MustFrom(ctx)
	var c domain.Category
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`INSERT INTO channel_categories (tenant, subject, name, sort_order)
			 VALUES ($1, $2, $3,
			         COALESCE((SELECT max(sort_order)+1 FROM channel_categories
			                   WHERE tenant=$1 AND subject=$2), 0))
			 RETURNING `+categoryCols,
			t.Slug, subject, strings.TrimSpace(name))
		var e error
		c, e = scanCategory(row, t.Slug, subject)
		return e
	})
	return c, err
}

// UpdateCategoryRecord son los cambios opcionales de una categoría (nil = sin cambio):
// renombrar, colapsar/expandir y reordenar.
type UpdateCategoryRecord struct {
	Name      *string
	Collapsed *bool
	SortOrder *int
}

// UpdateCategory renombra, colapsa/expande y/o reordena una categoría del usuario.
// ErrNotFound si la categoría no es del usuario.
func (s *Store) UpdateCategory(ctx context.Context, id uuid.UUID, subject string, upd UpdateCategoryRecord) (domain.Category, error) {
	t := tenancy.MustFrom(ctx)
	var name *string
	if upd.Name != nil {
		trimmed := strings.TrimSpace(*upd.Name)
		name = &trimmed
	}
	var c domain.Category
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`UPDATE channel_categories
			 SET name = COALESCE($3, name),
			     collapsed = COALESCE($4, collapsed),
			     sort_order = COALESCE($5, sort_order)
			 WHERE tenant=$1 AND subject=$2 AND id=$6
			 RETURNING `+categoryCols,
			t.Slug, subject, name, upd.Collapsed, upd.SortOrder, id)
		var e error
		c, e = scanCategory(row, t.Slug, subject)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Category{}, domain.ErrNotFound
	}
	return c, err
}

// DeleteCategory borra una categoría del usuario. Sus canales caen a la categoría por
// defecto: en la MISMA transacción se pone category_id=NULL (y sort_order=0) en las
// filas de pertenencia del usuario que la referenciaban — por eso no hay FK: un
// ON DELETE SET NULL sobre la PK compuesta anularía también tenant/subject. ErrNotFound
// si no existía.
func (s *Store) DeleteCategory(ctx context.Context, id uuid.UUID, subject string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`UPDATE channel_members SET category_id=NULL, sort_order=0
			 WHERE tenant=$1 AND subject=$2 AND category_id=$3`,
			t.Slug, subject, id); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx,
			`DELETE FROM channel_categories WHERE tenant=$1 AND subject=$2 AND id=$3`,
			t.Slug, subject, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// SetChannelCategory asigna el canal a una categoría del usuario (categoryID nil =
// categoría por defecto según kind) y opcionalmente fija su orden (sortOrder nil = sin
// cambio). Requiere acceso al canal; asegura la fila de pertenencia (como SetChannelPref:
// un público no-miembro que agrupa el canal lo muestra en su sidebar). Si categoryID no
// es nil, valida que la categoría es del propio usuario (ErrNotFound si no). Devuelve el
// canal con CategoryID/SortOrder (y favorite/muted) ya resueltos.
func (s *Store) SetChannelCategory(ctx context.Context, id uuid.UUID, subject, name string, categoryID *uuid.UUID, sortOrder *int) (domain.Channel, error) {
	t := tenancy.MustFrom(ctx)
	var ch domain.Channel
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		cur, err := getChannelTx(ctx, tx, t.Slug, id)
		if err != nil {
			return err
		}
		if err := requireChannelAccess(ctx, tx, t.Slug, cur, subject); err != nil {
			return err
		}
		if categoryID != nil {
			var exists bool
			if err := tx.QueryRow(ctx,
				`SELECT EXISTS(SELECT 1 FROM channel_categories
				              WHERE tenant=$1 AND subject=$2 AND id=$3)`,
				t.Slug, subject, *categoryID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return domain.ErrNotFound
			}
		}
		// Asegura la fila de membresía (público no-miembro que agrupa el canal).
		if _, err := tx.Exec(ctx,
			`INSERT INTO channel_members (tenant, channel_id, subject, name, role)
			 VALUES ($1, $2, $3, $4, 'member')
			 ON CONFLICT (tenant, channel_id, subject) DO NOTHING`,
			t.Slug, id, subject, name); err != nil {
			return err
		}
		var catID *uuid.UUID
		var sort int
		var fav bool
		var level string
		if err := tx.QueryRow(ctx,
			`UPDATE channel_members
			 SET category_id = $4, sort_order = COALESCE($5, sort_order)
			 WHERE tenant=$1 AND channel_id=$2 AND subject=$3
			 RETURNING category_id, sort_order, favorite, notify_level`,
			t.Slug, id, subject, categoryID, sortOrder).Scan(&catID, &sort, &fav, &level); err != nil {
			return err
		}
		cur.CategoryID = catID
		cur.SortOrder = sort
		cur.Favorite = fav
		// Muted derivado de notify_level (fuente de verdad), como en ListChannels.
		cur.NotifyLevel = level
		cur.Muted = level == domain.NotifyLevelNone
		ch = cur
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Channel{}, domain.ErrNotFound
	}
	return ch, err
}
