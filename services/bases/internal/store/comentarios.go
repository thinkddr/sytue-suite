// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// AvisoComentario es a quién hay que avisar de un comentario nuevo y con qué nombre:
// lo calcula la MISMA transacción que lo guarda, así que solo salen personas que en ese
// instante pueden leer el registro.
type AvisoComentario struct {
	BaseID, BaseName, TableID string
	// Destinatarios son los `sub`: la propietaria de la base y, en una respuesta, quienes
	// escriben en ese hilo. Nunca quien comenta.
	Destinatarios []string
}

// maxAvisados acota a cuánta gente avisa un comentario (y cuántos roles se resuelven).
const maxAvisados = 20

// registroDe resuelve la tabla y la base REALES de un registro: el rol se calcula
// siempre sobre ellas, nunca sobre lo que diga el cliente.
func registroDe(ctx context.Context, tx pgx.Tx, a domain.Actor, registro string) (tabla, base string, err error) {
	err = errNoFilas(tx.QueryRow(ctx, `SELECT table_id::text, base_id::text FROM base_records WHERE id = $1 AND tenant = $2`, registro, a.Tenant).Scan(&tabla, &base))
	return tabla, base, err
}

// RolEnRegistro es el rol efectivo de `a` sobre la tabla de un registro. ErrNoExiste
// si no hay registro o no tiene ningún acceso. Lo usa la capa HTTP para no avisar de
// una mención a quien no puede leer el registro.
func (s *Store) RolEnRegistro(ctx context.Context, a domain.Actor, registro string) (domain.Rol, error) {
	var rol domain.Rol
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		tabla, base, err := registroDe(ctx, tx, a, registro)
		if err != nil {
			return err
		}
		rol, err = rolEn(ctx, tx, a, base, tabla)
		return err
	})
	return rol, err
}

// ListarComentarios devuelve el hilo de un registro, del más antiguo al más reciente
// (lector). Una lápida —comentario borrado que aún tiene respuestas— sale sin texto ni autor.
func (s *Store) ListarComentarios(ctx context.Context, a domain.Actor, registro string) ([]domain.Comentario, error) {
	out := []domain.Comentario{}
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		tabla, base, err := registroDe(ctx, tx, a, registro)
		if err != nil {
			return err
		}
		rol, err := rolEn(ctx, tx, a, base, tabla)
		if err != nil {
			return err
		}
		if !domain.Permite(rol, domain.AccLeer) {
			return domain.ErrSinPermiso
		}
		rows, err := tx.Query(ctx, `
			SELECT id::text, coalesce(parent_id::text, ''), author_sub, body, created_at, deleted_at IS NOT NULL
			  FROM base_comments WHERE tenant = $1 AND record_id = $2
			 ORDER BY created_at, id LIMIT $3`, a.Tenant, registro, domain.MaxComentariosPorRegistro)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			c := domain.Comentario{RecordID: registro}
			var lapida bool
			if err := rows.Scan(&c.ID, &c.ParentID, &c.AuthorSub, &c.Body, &c.CreatedAt, &lapida); err != nil {
				return err
			}
			if lapida {
				c.AuthorSub, c.Deleted = "", true
			} else {
				c.CanDelete = domain.PuedeBorrarComentario(rol, c.AuthorSub == a.Sub)
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// CrearComentario añade un comentario a un registro (comentarista). `padre` vacío abre
// hilo; con valor ha de ser un comentario RAÍZ y vivo de ESE registro (los hilos tienen
// un nivel): uno de otro registro, de otra base o de otra organización no existe.
func (s *Store) CrearComentario(ctx context.Context, a domain.Actor, registro, padre, cuerpo string) (domain.Comentario, AvisoComentario, error) {
	cuerpo, err := domain.LimpiarComentario(cuerpo)
	if err != nil {
		return domain.Comentario{}, AvisoComentario{}, err
	}
	c := domain.Comentario{RecordID: registro, ParentID: padre, AuthorSub: a.Sub, Body: cuerpo, CanDelete: true}
	var av AvisoComentario
	err = s.en(ctx, a, func(tx pgx.Tx) error {
		tabla, base, err := registroDe(ctx, tx, a, registro)
		if err != nil {
			return err
		}
		if err := autorizar(ctx, tx, a, base, tabla, domain.AccComentar); err != nil {
			return err
		}
		// El registro bloqueado serializa el recuento: dos comentarios a la vez no pasan del tope.
		if _, err := tx.Exec(ctx, `SELECT 1 FROM base_records WHERE id = $1 AND tenant = $2 FOR NO KEY UPDATE`, registro, a.Tenant); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM base_comments WHERE tenant = $1 AND record_id = $2`, a.Tenant, registro).Scan(&n); err != nil {
			return err
		}
		if n >= domain.MaxComentariosPorRegistro {
			return domain.ErrLimite
		}
		var owner string
		if err := tx.QueryRow(ctx, `SELECT owner_sub, name FROM bases WHERE id = $1 AND tenant = $2`, base, a.Tenant).Scan(&owner, &av.BaseName); err != nil {
			return errNoFilas(err)
		}
		candidatos, err := autoresDelHilo(ctx, tx, a, registro, padre)
		if err != nil {
			return err
		}
		candidatos = append(candidatos, owner)
		if err := tx.QueryRow(ctx, `
			INSERT INTO base_comments (tenant, base_id, table_id, record_id, parent_id, author_sub, body)
			VALUES ($1, $2, $3, $4, $5::uuid, $6, $7) RETURNING id::text, created_at`,
			a.Tenant, base, tabla, registro, ptr(padre), a.Sub, cuerpo).Scan(&c.ID, &c.CreatedAt); err != nil {
			return err
		}
		av.BaseID, av.TableID = base, tabla
		av.Destinatarios = quienesLeen(ctx, tx, a, base, tabla, candidatos)
		// La traza dice QUÉ pasó, no guarda el texto: un comentario borrado no sobrevive en ella.
		return auditar(ctx, tx, a, base, &tabla, &registro, nil, "comment.create", nil, jsonDe(map[string]string{"comment": c.ID, "parent": padre}))
	})
	return c, av, err
}

// autoresDelHilo comprueba que `padre` es una raíz viva de ESE registro y devuelve
// quienes escriben en su hilo. Sin padre, nadie.
func autoresDelHilo(ctx context.Context, tx pgx.Tx, a domain.Actor, registro, padre string) ([]string, error) {
	if padre == "" {
		return nil, nil
	}
	var ok bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM base_comments
		WHERE id = $1 AND tenant = $2 AND record_id = $3 AND parent_id IS NULL AND deleted_at IS NULL)`, padre, a.Tenant, registro).Scan(&ok); err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: el hilo no existe", domain.ErrInvalido)
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT author_sub FROM base_comments
		WHERE tenant = $1 AND record_id = $2 AND (id = $3 OR parent_id = $3) AND deleted_at IS NULL LIMIT $4`, a.Tenant, registro, padre, maxAvisados)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// quienesLeen deja de `candidatos` a quien AHORA puede leer la tabla, sin repetir y
// sin quien comenta: alguien del hilo a quien se le retiró el acceso no recibe en la
// campana lo que ya no puede abrir.
func quienesLeen(ctx context.Context, tx pgx.Tx, a domain.Actor, base, tabla string, candidatos []string) []string {
	var out []string
	for _, sub := range candidatos {
		if sub == "" || sub == a.Sub || slices.Contains(out, sub) {
			continue
		}
		if _, err := rolEn(ctx, tx, domain.Actor{Sub: sub, Tenant: a.Tenant}, base, tabla); err == nil {
			out = append(out, sub)
		}
	}
	return out
}

// BorrarComentario borra un comentario: quien lo escribió (si aún puede comentar) o
// quien modera (editor o propietario). Sin acceso al registro, no existe. Si tiene
// respuestas vivas queda la lápida, sin texto; si no, la fila desaparece, y con ella la
// lápida de su raíz cuando era la última respuesta.
func (s *Store) BorrarComentario(ctx context.Context, a domain.Actor, comentario string) error {
	return s.en(ctx, a, func(tx pgx.Tx) error {
		var registro, tabla, base, autor, padre string
		if err := errNoFilas(tx.QueryRow(ctx, `
			SELECT record_id::text, table_id::text, base_id::text, author_sub, coalesce(parent_id::text, '')
			  FROM base_comments WHERE id = $1 AND tenant = $2 AND deleted_at IS NULL FOR UPDATE`, comentario, a.Tenant).
			Scan(&registro, &tabla, &base, &autor, &padre)); err != nil {
			return err
		}
		rol, err := rolEn(ctx, tx, a, base, tabla)
		if err != nil {
			return err
		}
		if !domain.PuedeBorrarComentario(rol, autor == a.Sub) {
			return domain.ErrSinPermiso
		}
		tag, err := tx.Exec(ctx, `UPDATE base_comments SET body = '', deleted_at = now()
			WHERE id = $1 AND tenant = $2 AND EXISTS (SELECT 1 FROM base_comments r WHERE r.tenant = $2 AND r.parent_id = $1)`, comentario, a.Tenant)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			if _, err := tx.Exec(ctx, `DELETE FROM base_comments WHERE id = $1 AND tenant = $2`, comentario, a.Tenant); err != nil {
				return err
			}
			if padre != "" {
				if _, err := tx.Exec(ctx, `DELETE FROM base_comments p WHERE p.id = $1 AND p.tenant = $2 AND p.deleted_at IS NOT NULL
					AND NOT EXISTS (SELECT 1 FROM base_comments r WHERE r.tenant = $2 AND r.parent_id = $1)`, padre, a.Tenant); err != nil {
					return err
				}
			}
		}
		return auditar(ctx, tx, a, base, &tabla, &registro, nil, "comment.delete", jsonDe(map[string]string{"comment": comentario, "author": autor}), nil)
	})
}
