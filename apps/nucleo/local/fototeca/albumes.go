// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fototeca

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
)

// # Álbumes
//
// Un álbum es su fila y sus miembros en orden (`album_fotos`, por posición). Borrar un
// álbum lo marca (su origen sigue reconociéndose al traer), nunca toca sus fotos; una foto
// en la papelera sigue en sus álbumes y vuelve con ella al restaurarla, y una borrada
// para siempre sale de ellos. La portada es la elegida si sigue en el álbum o, si no, la
// primera.

// vistaDeAlbum es la consulta de un álbum vivo como lo ve la pantalla: su número de fotos
// (las que no están en la papelera) y su portada.
const vistaDeAlbum = `SELECT a.id, a.titulo,
	(SELECT COUNT(*) FROM album_fotos m JOIN fotos f ON f.id = m.foto WHERE m.album = a.id AND f.borrada IS NULL),
	COALESCE((SELECT m.foto FROM album_fotos m JOIN fotos f ON f.id = m.foto
		WHERE m.album = a.id AND m.foto = a.portada AND f.borrada IS NULL), ''),
	COALESCE((SELECT m.foto FROM album_fotos m JOIN fotos f ON f.id = m.foto
		WHERE m.album = a.id AND f.borrada IS NULL ORDER BY m.posicion, m.foto LIMIT 1), '')
	FROM albumes a WHERE a.borrado IS NULL`

func leerAlbumes(filas *sql.Rows) ([]api.Album, error) {
	defer func() { _ = filas.Close() }()
	out := []api.Album{}
	for filas.Next() {
		var a api.Album
		var elegida, primera string
		if err := filas.Scan(&a.ID, &a.Title, &a.Count, &elegida, &primera); err != nil {
			return nil, err
		}
		a.CoverAssetID = primera
		if elegida != "" {
			a.CoverAssetID = elegida
		}
		out = append(out, a)
	}
	return out, filas.Err()
}

// Albums son los álbumes, del más nuevo al más viejo.
func (f *Fototeca) Albums(ctx context.Context) ([]api.Album, error) { return albumesDe(ctx, f.db) }

// consultor es lo que tienen la base y una transacción para leer.
type consultor interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func albumesDe(ctx context.Context, q consultor) ([]api.Album, error) {
	filas, err := q.QueryContext(ctx, vistaDeAlbum+` ORDER BY a.creado DESC, a.id DESC`)
	if err != nil {
		return nil, err
	}
	return leerAlbumes(filas)
}

// Album es el álbum id; ErrNotFound si no está o está borrado.
func (f *Fototeca) Album(ctx context.Context, id string) (api.Album, error) {
	filas, err := f.db.QueryContext(ctx, vistaDeAlbum+` AND a.id = ?`, id)
	if err != nil {
		return api.Album{}, err
	}
	as, err := leerAlbumes(filas)
	if err != nil {
		return api.Album{}, err
	}
	if len(as) == 0 {
		return api.Album{}, ErrNotFound
	}
	return as[0], nil
}

// AlbumPhotos son las fotos del álbum en su orden, sin las de la papelera.
func (f *Fototeca) AlbumPhotos(ctx context.Context, id string) ([]api.Photo, error) {
	if err := albumVivo(ctx, f.db, id); err != nil {
		return nil, err
	}
	fs, err := f.consultarFotos(ctx, f.db, `SELECT `+columnasFoto+` FROM album_fotos m JOIN fotos f ON f.id = m.foto
		WHERE m.album = ? AND f.borrada IS NULL ORDER BY m.posicion, m.foto`, id)
	if err != nil {
		return nil, err
	}
	return vistas(fs), nil
}

// albumVivo es nil si el álbum id está y no está borrado; ErrNotFound si no.
func albumVivo(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string,
) error {
	var borrado sql.NullString
	err := q.QueryRowContext(ctx, `SELECT borrado FROM albumes WHERE id = ?`, id).Scan(&borrado)
	if errors.Is(err, sql.ErrNoRows) || err == nil && borrado.Valid {
		return ErrNotFound
	}
	return err
}

// CreateAlbum crea un álbum vacío.
func (f *Fototeca) CreateAlbum(ctx context.Context, title string) (api.Album, error) {
	return f.crearAlbum(ctx, title, "")
}

// CrearAlbumDe crea el álbum title con su origen (de dónde vino: «sytue:<id>»), que no
// cambia al renombrarlo (photos.Repository.CrearAlbumDe).
func (f *Fototeca) CrearAlbumDe(ctx context.Context, title, origen string) (api.Album, error) {
	origen = strings.TrimSpace(origen)
	if origen == "" || len(origen) > 200 {
		return api.Album{}, model.ErrInvalidInput
	}
	return f.crearAlbum(ctx, title, origen)
}

func (f *Fototeca) crearAlbum(ctx context.Context, title, origen string) (api.Album, error) {
	title, err := model.TituloDeAlbum(title)
	if err != nil {
		return api.Album{}, err
	}
	a := api.Album{ID: uuid.NewString(), Title: title}
	ahora := enMarca(f.ahora())
	err = f.tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO albumes (id, titulo, origen, creado, editado) VALUES (?, ?, ?, ?, ?)`, a.ID, title, origen, ahora, ahora)
		return err
	})
	if err != nil {
		return api.Album{}, err
	}
	return a, nil
}

// cambiarAlbum cambia en una transacción el álbum id, que tiene que estar vivo.
func (f *Fototeca) cambiarAlbum(ctx context.Context, id string, cambiar func(*sql.Tx) error) error {
	return f.tx(ctx, func(tx *sql.Tx) error {
		if err := albumVivo(ctx, tx, id); err != nil {
			return err
		}
		if err := cambiar(tx); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE albumes SET editado = ? WHERE id = ?`, enMarca(f.ahora()), id)
		return err
	})
}

// RenameAlbum le cambia el título.
func (f *Fototeca) RenameAlbum(ctx context.Context, id, title string) error {
	title, err := model.TituloDeAlbum(title)
	if err != nil {
		return err
	}
	return f.cambiarAlbum(ctx, id, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE albumes SET titulo = ? WHERE id = ?`, title, id)
		return err
	})
}

// DeleteAlbum borra el álbum (sus fotos no).
func (f *Fototeca) DeleteAlbum(ctx context.Context, id string) error {
	return f.cambiarAlbum(ctx, id, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE albumes SET borrado = ? WHERE id = ?`, enMarca(f.ahora()), id)
		return err
	})
}

// AddToAlbum añade al final del álbum las fotos ids (de la galería, fuera de la papelera)
// que no estaban, y dice cuántas ha añadido.
func (f *Fototeca) AddToAlbum(ctx context.Context, album string, ids []string) (int, error) {
	anadidas := 0
	err := f.cambiarAlbum(ctx, album, func(tx *sql.Tx) error {
		for _, id := range ids {
			x, err := f.unaFoto(ctx, tx, id)
			if err == nil && x.fila.DeletedAt != nil {
				err = ErrNotFound
			}
			if err != nil {
				return err
			}
		}
		var posicion int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(posicion) + 1, 0) FROM album_fotos WHERE album = ?`, album).Scan(&posicion); err != nil {
			return err
		}
		ahora := enMarca(f.ahora())
		vistos := map[string]bool{}
		for _, id := range ids {
			if vistos[id] {
				continue
			}
			vistos[id] = true
			if _, err := model.MembershipID(album, id); err != nil {
				return model.ErrInvalidInput
			}
			res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO album_fotos (album, foto, posicion, anadida) VALUES (?, ?, ?, ?)`, album, id, posicion, ahora)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				posicion++
				anadidas++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return anadidas, nil
}

// RemoveFromAlbum quita la foto del álbum (si estaba).
func (f *Fototeca) RemoveFromAlbum(ctx context.Context, album, asset string) error {
	if _, err := model.MembershipID(album, asset); err != nil {
		return model.ErrInvalidInput
	}
	return f.cambiarAlbum(ctx, album, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM album_fotos WHERE album = ? AND foto = ?`, album, asset)
		return err
	})
}

// PonerPortada elige la foto de portada del álbum, que tiene que estar en él (y fuera de
// la papelera). El título no cambia.
func (f *Fototeca) PonerPortada(ctx context.Context, id, _, foto string) error {
	if err := albumVivo(ctx, f.db, id); err != nil {
		return err
	}
	return f.cambiarAlbum(ctx, id, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM album_fotos m JOIN fotos f ON f.id = m.foto
			WHERE m.album = ? AND m.foto = ? AND f.borrada IS NULL`, id, foto).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return model.ErrInvalidInput
		}
		_, err := tx.ExecContext(ctx, `UPDATE albumes SET portada = ? WHERE id = ?`, foto, id)
		return err
	})
}

// AlbumesPorOrigen son los álbumes que vinieron de fuera, por su origen, también los
// borrados; si dos tienen el mismo, cuenta el vivo.
func (f *Fototeca) AlbumesPorOrigen(ctx context.Context) (map[string]photos.AlbumDeOrigen, error) {
	filas, err := f.db.QueryContext(ctx, `SELECT id, origen, borrado IS NOT NULL FROM albumes WHERE origen <> '' ORDER BY creado, id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = filas.Close() }()
	out := map[string]photos.AlbumDeOrigen{}
	for filas.Next() {
		var id, origen string
		var borrado bool
		if err := filas.Scan(&id, &origen, &borrado); err != nil {
			return nil, err
		}
		if ya, ok := out[origen]; ok && !ya.Borrado {
			continue
		}
		out[origen] = photos.AlbumDeOrigen{ID: id, Borrado: borrado}
	}
	return out, filas.Err()
}

// Lo que la fachada de las pantallas (api.Photos) busca en la fototeca.
var (
	_ api.PhotosData                = (*Fototeca)(nil)
	_ api.PhotosMediaData           = (*Fototeca)(nil)
	_ api.PhotosExportData          = (*Fototeca)(nil)
	_ api.PhotosRecipeData          = (*Fototeca)(nil)
	_ api.PhotosAlbumData           = (*Fototeca)(nil)
	_ api.PhotosAlbumMembershipData = (*Fototeca)(nil)
	_ api.PhotosCalendarData        = (*Fototeca)(nil)
	_ api.PhotosDiscoveryData       = (*Fototeca)(nil)
	_ api.PhotosPurgeData           = (*Fototeca)(nil)
	_ api.PhotosAlbumCoverData      = (*Fototeca)(nil)
	_ api.PhotosDirectIngestData    = (*Fototeca)(nil)
	_ api.PrivateDataRefresher      = (*Fototeca)(nil)
)
