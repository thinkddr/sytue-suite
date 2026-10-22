// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// ETIQUETAS CON ESTADO
//
// Una etiqueta es un EJE ("Prioridad") con estados excluyentes ("Alta", "Media",
// "Baja"). Marcar un estado sustituye al anterior DE LA MISMA etiqueta: eso lo
// garantiza la clave primaria (note_id, tag_id) de note_tag_states, no un
// borrado previo que se pueda olvidar.

// ListTags devuelve las etiquetas de `sub` con sus estados.
func (s *Store) ListTags(ctx context.Context, sub string) ([]domain.Tag, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Tag
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT t.id, t.name, t.position,
			       ts.id, ts.name, ts.emoji, ts.color, ts.shortcut, ts.position
			  FROM tags t
			  LEFT JOIN tag_states ts ON ts.tag_id = t.id
			 WHERE t.tenant=$1 AND t.owner_sub=$2
			 ORDER BY t.position, t.name, ts.position`, t.Slug, sub)
		if err != nil {
			return err
		}
		defer rows.Close()
		porID := map[uuid.UUID]int{}
		for rows.Next() {
			var tagID uuid.UUID
			var name string
			var pos int
			var stID *uuid.UUID
			var stName, stEmoji, stColor, stShortcut *string
			var stPos *int
			if err := rows.Scan(&tagID, &name, &pos, &stID, &stName, &stEmoji, &stColor, &stShortcut, &stPos); err != nil {
				return err
			}
			idx, ok := porID[tagID]
			if !ok {
				out = append(out, domain.Tag{ID: tagID, Tenant: t.Slug, OwnerSub: sub, Name: name, Position: pos})
				idx = len(out) - 1
				porID[tagID] = idx
			}
			if stID != nil {
				out[idx].States = append(out[idx].States, domain.TagState{
					ID: *stID, TagID: tagID, Name: deref(stName), Emoji: deref(stEmoji),
					Color: deref(stColor), Shortcut: deref(stShortcut), Position: derefInt(stPos),
				})
			}
		}
		return rows.Err()
	})
	return out, err
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}

// SaveTag crea o actualiza una etiqueta con TODOS sus estados de una vez: el
// editor de etiquetas es una sola pantalla y guardarla en varias peticiones deja
// estados huérfanos si una falla a mitad.
func (s *Store) SaveTag(ctx context.Context, sub string, id *uuid.UUID, in domain.TagInput) (domain.Tag, error) {
	t := tenancy.MustFrom(ctx)
	var tag domain.Tag
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tagID, err := guardarEtiqueta(ctx, tx, t.Slug, sub, id, in)
		if err != nil {
			return err
		}
		if err := sincronizarEstados(ctx, tx, t.Slug, tagID, in.States); err != nil {
			return err
		}
		tag = domain.Tag{ID: tagID, Tenant: t.Slug, OwnerSub: sub, Name: in.Name}
		return nil
	})
	if err != nil {
		return domain.Tag{}, err
	}
	// Se relee para devolver los estados con sus ids definitivos.
	tags, err := s.ListTags(ctx, sub)
	if err != nil {
		return tag, nil //nolint:nilerr // la etiqueta ya está guardada; releerla es cortesía.
	}
	for _, x := range tags {
		if x.ID == tag.ID {
			return x, nil
		}
	}
	return tag, nil
}

// DeleteTag borra una etiqueta y, en cascada, sus estados y sus marcas.
func (s *Store) DeleteTag(ctx context.Context, id uuid.UUID, sub string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM tags WHERE tenant=$1 AND owner_sub=$2 AND id=$3`, t.Slug, sub, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// SetNoteState marca un estado en una nota. stateID nil = quitar la marca de esa
// etiqueta. Un único punto de entrada para poner y quitar: dos endpoints para lo
// mismo acaban divergiendo en los permisos.
func (s *Store) SetNoteState(ctx context.Context, noteID, tagID uuid.UUID, stateID *uuid.UUID, sub string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// La nota tiene que ser escribible por quien marca.
		var ok bool
		err := tx.QueryRow(ctx, `
			SELECT true FROM notes n JOIN notebooks nb ON nb.id = n.notebook_id
			 WHERE n.tenant=$1 AND n.id=$2 AND n.deleted_at IS NULL AND nb.deleted_at IS NULL
			   AND (nb.owner_sub = $3 OR nb.visibility = 'organization' OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = $3))`, t.Slug, noteID, sub).Scan(&ok)
		if isNoRows(err) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if stateID == nil {
			_, err := tx.Exec(ctx, `DELETE FROM note_tag_states WHERE tenant=$1 AND note_id=$2 AND tag_id=$3`,
				t.Slug, noteID, tagID)
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO note_tag_states (tenant, note_id, tag_id, tag_state_id)
			SELECT $1,$2,$3,$4 FROM tag_states ts
			 WHERE ts.tenant=$1 AND ts.id=$4 AND ts.tag_id=$3
			ON CONFLICT (note_id, tag_id) DO UPDATE SET tag_state_id = EXCLUDED.tag_state_id`,
			t.Slug, noteID, tagID, *stateID)
		return err
	})
}

// guardarEtiqueta inserta la etiqueta o actualiza la que se pida, y devuelve su
// id. Sale de `SaveTag` porque eran dos ramas casi simétricas de ocho líneas que
// tapaban lo que viene después.
func guardarEtiqueta(
	ctx context.Context, tx pgx.Tx, tenant, sub string, id *uuid.UUID, in domain.TagInput,
) (uuid.UUID, error) {
	var tagID uuid.UUID
	if id == nil {
		err := tx.QueryRow(ctx, `
			INSERT INTO tags (tenant, owner_sub, name, position)
			VALUES ($1,$2,$3, COALESCE($4,(SELECT COALESCE(max(position),0)+1 FROM tags WHERE tenant=$1 AND owner_sub=$2)))
			RETURNING id`, tenant, sub, in.Name, in.Position).Scan(&tagID)
		if uniqueViolation(err) {
			return uuid.Nil, domain.ErrInvalidInput
		}
		return tagID, err
	}
	err := tx.QueryRow(ctx, `
		UPDATE tags SET name=$4, position=COALESCE($5, position)
		 WHERE tenant=$1 AND owner_sub=$2 AND id=$3 RETURNING id`,
		tenant, sub, *id, in.Name, in.Position).Scan(&tagID)
	if isNoRows(err) {
		return uuid.Nil, domain.ErrNotFound
	}
	if uniqueViolation(err) {
		return uuid.Nil, domain.ErrInvalidInput
	}
	return tagID, err
}

// sincronizarEstados deja los estados de la etiqueta como los manda el cliente.
//
// Los que ya no vienen desaparecen; los que traen id se ACTUALIZAN en vez de
// borrarse y reinsertarse, porque `tag_states` cae en cascada sobre las notas ya
// etiquetadas: recrearlos con id nuevo les quitaría el estado a todas.
func sincronizarEstados(
	ctx context.Context, tx pgx.Tx, tenant string, tagID uuid.UUID, states []domain.TagStateInput,
) error {
	conservar := make([]uuid.UUID, 0, len(states))
	for i, st := range states {
		var stID uuid.UUID
		if st.ID != nil {
			err := tx.QueryRow(ctx, `
				UPDATE tag_states SET name=$3, emoji=$4, color=$5, shortcut=$6, position=$7
				 WHERE tenant=$1 AND tag_id=$2 AND id=$8 RETURNING id`,
				tenant, tagID, st.Name, st.Emoji, st.Color, st.Shortcut, i, *st.ID).Scan(&stID)
			if err != nil && !isNoRows(err) {
				return err
			}
		}
		if stID == uuid.Nil {
			if err := tx.QueryRow(ctx, `
				INSERT INTO tag_states (tenant, tag_id, name, emoji, color, shortcut, position)
				VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
				tenant, tagID, st.Name, st.Emoji, st.Color, st.Shortcut, i).Scan(&stID); err != nil {
				return err
			}
		}
		conservar = append(conservar, stID)
	}
	_, err := tx.Exec(ctx,
		`DELETE FROM tag_states WHERE tenant=$1 AND tag_id=$2 AND NOT (id = ANY($3))`,
		tenant, tagID, conservar)
	return err
}
