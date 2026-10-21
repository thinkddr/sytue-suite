// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
)

// # Álbum compartido con una persona (0009)
//
// Lo da y lo retira SU DUEÑA; quien lo recibe lo ve con su sesión, solo para ver. Lo
// que no es de quien lo pide no existe (ErrNotFound), igual que en el resto del store.

// PersonasDelAlbum son las personas con quien se ha compartido un álbum. Solo su dueña.
func (s *Store) PersonasDelAlbum(ctx context.Context, album uuid.UUID, duenya string) ([]domain.PersonaDelAlbum, error) {
	out := []domain.PersonaDelAlbum{}
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		if err := s.albumDe(ctx, tx, tenant, album, duenya); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT sujeto, puede_editar, creado FROM album_personas
			WHERE tenant = $1 AND album_id = $2 ORDER BY creado, sujeto`, tenant, album)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p domain.PersonaDelAlbum
			if err := rows.Scan(&p.Sujeto, &p.Editar, &p.Creado); err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

// CompartirConPersona da el álbum a `sujeto`, para ver o para editar (o le cambia el
// nivel). `nueva` dice si no lo tenía ya (a quien avisar); `titulo` es el del álbum.
// Compartir con una misma es entrada inválida.
func (s *Store) CompartirConPersona(ctx context.Context, album uuid.UUID, duenya, sujeto string, editar bool) (nueva bool, titulo string, err error) {
	if sujeto == "" || sujeto == duenya {
		return false, "", fmt.Errorf("%w: no se comparte un álbum con su dueña", domain.ErrInvalidInput)
	}
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT title FROM albums WHERE tenant = $1 AND id = $2 AND owner_sub = $3`,
			tenant, album, duenya).Scan(&titulo); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		// `xmax = 0` distingue la fila recién insertada de la que el ON CONFLICT actualizó.
		return tx.QueryRow(ctx, `INSERT INTO album_personas (album_id, tenant, sujeto, concedido_por, puede_editar)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (album_id, sujeto) DO UPDATE SET puede_editar = EXCLUDED.puede_editar
			RETURNING xmax = 0`, album, tenant, sujeto, duenya, editar).Scan(&nueva)
	})
	return nueva, titulo, err
}

// DejarDeCompartirConPersona le quita el álbum a `sujeto`. ErrNotFound si no lo tenía.
func (s *Store) DejarDeCompartirConPersona(ctx context.Context, album uuid.UUID, duenya, sujeto string) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		if err := s.albumDe(ctx, tx, tenant, album, duenya); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM album_personas WHERE tenant = $1 AND album_id = $2 AND sujeto = $3`,
			tenant, album, sujeto)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// AlbumesCompartidos es «Compartido conmigo»: los álbumes que otras personas han
// compartido con `sujeto`, el último compartido primero. `Duenyo` es quien lo compartió;
// el token del enlace no sale: es una credencial de su dueña.
func (s *Store) AlbumesCompartidos(ctx context.Context, sujeto string) ([]domain.Album, error) {
	var out []domain.Album
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT `+colsAlbum+`, p.puede_editar FROM album_personas p JOIN albums al ON al.id = p.album_id
			WHERE p.tenant = $1 AND p.sujeto = $2
			ORDER BY p.creado DESC, al.id`, tenant, sujeto)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var editable bool
			al, err := scanAlbum(rows, &editable)
			if err != nil {
				return err
			}
			al.Compartido, al.Editable = false, editable
			out = append(out, al)
		}
		return rows.Err()
	})
	return out, err
}

// AlbumCompartido devuelve un álbum que alguien ha compartido con `sujeto`; si no se
// lo han compartido, no existe. Sin el token del enlace, como en AlbumesCompartidos.
//
// Es la verja de todo lo que se sirve por «Compartido conmigo»: las fotos se leen
// DESPUÉS con FotosDelAlbumPublico/FotoDelAlbumPublico, que solo dan las del álbum ya
// resuelto.
func (s *Store) AlbumCompartido(ctx context.Context, album uuid.UUID, sujeto string) (domain.Album, error) {
	var al domain.Album
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		var editable bool
		al, err = scanAlbum(tx.QueryRow(ctx, `
			SELECT `+colsAlbum+`, p.puede_editar FROM album_personas p JOIN albums al ON al.id = p.album_id
			WHERE p.tenant = $1 AND p.album_id = $2 AND p.sujeto = $3`, tenant, album, sujeto), &editable)
		al.Editable = editable
		return err
	})
	al.Compartido = false
	return al, err
}

// DuenyaParaEditar es la dueña del álbum si `sub` puede EDITARLO: porque es ella o
// porque se lo compartió con edición. ErrNotFound si no: quien solo lo ve, o nadie.
// `editora` dice que es lo segundo.
func (s *Store) DuenyaParaEditar(ctx context.Context, album uuid.UUID, sub string) (duenya string, editora bool, err error) {
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		duenya, err = duenyaEditable(ctx, tx, tenant, album, sub)
		return err
	})
	return duenya, err == nil && duenya != sub, err
}
