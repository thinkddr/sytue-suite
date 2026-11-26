// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fototeca

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
)

// columnasFoto son las de una fila de fotos, en el orden de leerFoto.
const columnasFoto = `f.id, f.nombre, f.tipo, f.bytes, f.ancho, f.alto, f.orientacion, f.tomada, f.subida,
	f.camara, f.objetivo, f.favorita, f.descripcion, f.etiquetas, f.lat, f.lon, f.duracion_ms,
	f.huella, f.retenido, f.ruta, f.modificado, f.borrada, f.purgada, f.creada, f.editada`

// foto es una fila de fotos: la del modelo (sin lo que cita sus ficheros) y dónde está su
// original.
type foto struct {
	fila model.AssetRow
	// huella es la del original; retenido, si es un fichero de objetos/.
	huella   string
	retenido bool
	// ruta y modificado son los de un original enlazado.
	ruta       string
	modificado *time.Time
}

// vista es la foto como la ve la galería (sin el estado de su original externo).
func (x foto) vista() api.Photo {
	v := photos.Vista(x.fila)
	v.Externo = x.ruta
	return v
}

// conHuella es la foto con su huella, para los cálculos compartidos.
func (x foto) conHuella() photos.FotoConHuella {
	return photos.FotoConHuella{Vista: x.vista(), Subida: x.fila.UploadedAt, Huella: x.huella}
}

type escaner interface{ Scan(...any) error }

// leerFoto lee una fila de columnasFoto.
func (f *Fototeca) leerFoto(s escaner) (foto, error) {
	var (
		x                                    foto
		tomada, modificado, borrada, purgada sql.NullString
		subida, creada, editada, etiquetas   string
		lat, lon                             sql.NullFloat64
		favorita, retenido                   int
	)
	a := &x.fila
	if err := s.Scan(&a.ID, &a.Name, &a.MediaType, &a.Bytes, &a.Width, &a.Height, &a.Orientation, &tomada, &subida,
		&a.Camera, &a.Lens, &favorita, &a.Description, &etiquetas, &lat, &lon, &a.DurationMs,
		&x.huella, &retenido, &x.ruta, &modificado, &borrada, &purgada, &creada, &editada); err != nil {
		return x, err
	}
	var err error
	if a.TakenAt, err = deMarcaNula(tomada); err != nil {
		return x, err
	}
	if a.UploadedAt, err = deMarca(subida); err != nil {
		return x, err
	}
	if a.CreatedAt, err = deMarca(creada); err != nil {
		return x, err
	}
	if a.UpdatedAt, err = deMarca(editada); err != nil {
		return x, err
	}
	if a.DeletedAt, err = deMarcaNula(borrada); err != nil {
		return x, err
	}
	if a.PurgedAt, err = deMarcaNula(purgada); err != nil {
		return x, err
	}
	if x.modificado, err = deMarcaNula(modificado); err != nil {
		return x, err
	}
	if lat.Valid && lon.Valid {
		la, lo := lat.Float64, lon.Float64
		a.Latitude, a.Longitude = &la, &lo
	}
	a.Tenant, a.Owner, a.DriveID = f.scope.Tenant, f.scope.Subject, "private:"+a.ID
	a.Favorite, x.retenido = favorita != 0, retenido != 0
	a.Tags = leerEtiquetas(etiquetas)
	if x.fila.PurgedAt != nil && x.huella != "" {
		a.OriginalSHA256 = x.huella
	}
	return x, nil
}

func leerEtiquetas(raw string) []string {
	var tags []string
	if json.Unmarshal([]byte(raw), &tags) != nil || tags == nil {
		return []string{}
	}
	return tags
}

func etiquetasJSON(tags []string) string {
	if tags == nil {
		tags = []string{}
	}
	raw, _ := json.Marshal(tags)
	return string(raw)
}

// consultarFotos son las fotos de una consulta sobre `fotos f` que devuelve columnasFoto.
func (f *Fototeca) consultarFotos(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, consulta string, args ...any,
) ([]foto, error) {
	filas, err := q.QueryContext(ctx, consulta, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = filas.Close() }()
	var out []foto
	for filas.Next() {
		x, err := f.leerFoto(filas)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, filas.Err()
}

// unaFoto es la fila id, también si está en la papelera o purgada; ErrNotFound si no está.
func (f *Fototeca) unaFoto(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, id string,
) (foto, error) {
	fs, err := f.consultarFotos(ctx, q, `SELECT `+columnasFoto+` FROM fotos f WHERE f.id = ?`, id)
	if err != nil {
		return foto{}, err
	}
	if len(fs) == 0 {
		return foto{}, ErrNotFound
	}
	return fs[0], nil
}

// vistas son las vistas de fs, con el estado de su original externo.
func vistas(fs []foto) []api.Photo {
	out := make([]api.Photo, len(fs))
	for i, x := range fs {
		out[i] = photos.ConEstado(x.vista())
	}
	return out
}

// Photo es la foto id de la galería; ErrNotFound si no está o está en la papelera.
func (f *Fototeca) Photo(ctx context.Context, id string) (api.Photo, error) {
	x, err := f.unaFoto(ctx, f.db, id)
	if err != nil {
		return api.Photo{}, err
	}
	if x.fila.DeletedAt != nil {
		return api.Photo{}, ErrNotFound
	}
	return photos.ConEstado(x.vista()), nil
}

// Papelera son las fotos de la papelera, de la última que se tiró a la primera.
func (f *Fototeca) Papelera(ctx context.Context) ([]api.Photo, error) {
	fs, err := f.consultarFotos(ctx, f.db, `SELECT `+columnasFoto+` FROM fotos f
		WHERE f.borrada IS NOT NULL AND f.purgada IS NULL ORDER BY f.borrada DESC, f.id DESC`)
	if err != nil {
		return nil, err
	}
	return vistas(fs), nil
}

// cambiarFoto cambia en una transacción la foto id con cambiar (que hace sus UPDATE):
// tiene que estar en la papelera si papelera, y fuera si no, y no purgada.
func (f *Fototeca) cambiarFoto(ctx context.Context, id string, papelera bool, cambiar func(*sql.Tx, foto) error) error {
	return f.tx(ctx, func(tx *sql.Tx) error {
		x, err := f.unaFoto(ctx, tx, id)
		if err != nil {
			return err
		}
		if (x.fila.DeletedAt != nil) != papelera || x.fila.PurgedAt != nil {
			return ErrNotFound
		}
		if err := cambiar(tx, x); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE fotos SET editada = ? WHERE id = ?`, enMarca(f.ahora()), id)
		return err
	})
}

// SetFavorite marca o desmarca la foto como favorita.
func (f *Fototeca) SetFavorite(ctx context.Context, id string, favorite bool) error {
	return f.cambiarFoto(ctx, id, false, func(tx *sql.Tx, _ foto) error {
		_, err := tx.ExecContext(ctx, `UPDATE fotos SET favorita = ? WHERE id = ?`, favorite, id)
		return err
	})
}

// SetDescription guarda la descripción y las etiquetas, que también se buscan.
func (f *Fototeca) SetDescription(ctx context.Context, id, description string, tags []string) error {
	if err := photos.DescripcionValida(description, tags); err != nil {
		return err
	}
	return f.cambiarFoto(ctx, id, false, func(tx *sql.Tx, x foto) error {
		_, err := tx.ExecContext(ctx, `UPDATE fotos SET descripcion = ?, etiquetas = ?, texto = ? WHERE id = ?`,
			description, etiquetasJSON(tags), photos.TextoDeBusqueda(x.fila.Name, description, tags), id)
		return err
	})
}

// Trash lleva la foto a la papelera.
func (f *Fototeca) Trash(ctx context.Context, id string) error {
	return f.cambiarFoto(ctx, id, false, func(tx *sql.Tx, _ foto) error {
		_, err := tx.ExecContext(ctx, `UPDATE fotos SET borrada = ? WHERE id = ?`, enMarca(f.ahora()), id)
		return err
	})
}

// Restaurar devuelve la foto de la papelera a la galería (y a sus álbumes).
func (f *Fototeca) Restaurar(ctx context.Context, id string) error {
	return f.cambiarFoto(ctx, id, true, func(tx *sql.Tx, _ foto) error {
		_, err := tx.ExecContext(ctx, `UPDATE fotos SET borrada = NULL WHERE id = ?`, id)
		return err
	})
}

// PonerFechas pone a la foto id su fecha de captura, si no tiene ninguna, y la de subida
// (photos.Repository.PonerFechas). Las nil no se tocan.
func (f *Fototeca) PonerFechas(ctx context.Context, id string, tomada, subida *time.Time) error {
	if tomada != nil && tomada.IsZero() {
		tomada = nil
	}
	if subida != nil && subida.IsZero() {
		subida = nil
	}
	if tomada == nil && subida == nil {
		return nil
	}
	return f.cambiarFoto(ctx, id, false, func(tx *sql.Tx, x foto) error {
		a := x.fila
		if tomada != nil && a.TakenAt == nil {
			t := tomada.UTC()
			a.TakenAt = &t
		}
		if subida != nil {
			a.UploadedAt = subida.UTC()
		}
		fecha, anyo, mesDia := fechasDe(a)
		_, err := tx.ExecContext(ctx, `UPDATE fotos SET tomada = ?, subida = ?, fecha = ?, anyo = ?, mes_dia = ? WHERE id = ?`,
			marcaNula(a.TakenAt), enMarca(a.UploadedAt), fecha, anyo, mesDia, id)
		return err
	})
}

// fechasDe son las columnas de fecha de la galería de a: la fecha que ordena la línea,
// su año y, si es la de captura, su mes y día (los recuerdos). Las mismas que la vista.
func fechasDe(a model.AssetRow) (fecha string, anyo, mesDia int) {
	v := photos.Vista(a)
	t := v.TakenAt.UTC()
	if v.TakenFromEXIF {
		mesDia = int(t.Month())*100 + t.Day()
	}
	return enMarca(t), t.Year(), mesDia
}

// FotoPorHuella es la foto de la galería (fuera de la papelera) con ese original: el
// SHA-256 en hexadecimal de sus bytes. ok es false si no hay ninguna.
func (f *Fototeca) FotoPorHuella(ctx context.Context, sha256Hex string) (api.Photo, bool, error) {
	sha256Hex = strings.ToLower(strings.TrimSpace(sha256Hex))
	if len(sha256Hex) != 64 {
		return api.Photo{}, false, nil
	}
	fs, err := f.consultarFotos(ctx, f.db, `SELECT `+columnasFoto+` FROM fotos f WHERE f.huella = ? AND f.borrada IS NULL ORDER BY f.creada, f.id LIMIT 1`, sha256Hex)
	if err != nil || len(fs) == 0 {
		return api.Photo{}, false, err
	}
	return photos.ConEstado(fs[0].vista()), true, nil
}

// HuellasConocidas son las de los originales de la fototeca, también los de la papelera
// y los borrados para siempre: lo que «Importar» ya no vuelve a traer.
func (f *Fototeca) HuellasConocidas(ctx context.Context) (map[string]bool, error) {
	hs, err := columna(ctx, f.db, `SELECT DISTINCT huella FROM fotos WHERE huella <> ''`)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(hs))
	for _, h := range hs {
		out[h] = true
	}
	return out, nil
}

// HuellaOriginal es la huella del original de una foto o un vídeo, venga de donde venga;
// vacía si no la tiene.
func (f *Fototeca) HuellaOriginal(ctx context.Context, id string) (string, error) {
	x, err := f.unaFoto(ctx, f.db, id)
	if err != nil {
		return "", err
	}
	return x.huella, nil
}

// Original copia a out los bytes del original, tal cual entró: del fichero de la
// fototeca o, si es enlazado, de su fichero, comprobando que sigue siendo el mismo
// (photos.ErrOriginalExterno si no).
func (f *Fototeca) Original(ctx context.Context, id string, out io.Writer) error {
	x, err := f.unaFoto(ctx, f.db, id)
	if err != nil {
		return err
	}
	return f.copiarOriginal(ctx, x, out)
}

func (f *Fototeca) copiarOriginal(ctx context.Context, x foto, out io.Writer) error {
	switch {
	case x.ruta != "" && x.fila.PurgedAt == nil:
		return photos.CopiarExterno(ctx, out, x.ruta, x.fila.Bytes, x.huella)
	case x.retenido && x.fila.PurgedAt == nil:
		return f.copiarObjeto(ctx, x.huella, out)
	}
	return ErrNotFound
}

// leerOriginal lee entero el original de x, de hasta tope bytes.
func (f *Fototeca) leerOriginal(ctx context.Context, x foto, tope int64) ([]byte, error) {
	if x.fila.Bytes > tope {
		return nil, model.ErrInvalidInput
	}
	if x.ruta != "" && x.fila.PurgedAt == nil {
		return photos.LeerExterno(ctx, x.ruta, x.fila.Bytes, x.huella, tope)
	}
	if !x.retenido || x.fila.PurgedAt != nil {
		return nil, ErrNotFound
	}
	datos, err := f.leerObjeto(ctx, x.huella, tope)
	if errors.Is(err, errDemasiado) {
		err = model.ErrInvalidInput
	}
	return datos, err
}

// RutaExterna es dónde está el original enlazado de una foto y su estado; ok es false si
// no es enlazado.
func (f *Fototeca) RutaExterna(ctx context.Context, id string) (ruta, estado string, ok bool, err error) {
	x, err := f.unaFoto(ctx, f.db, id)
	if err != nil || x.ruta == "" {
		return "", "", false, err
	}
	return x.ruta, photos.ConEstado(x.vista()).EstadoExterno, true, nil
}

// Recolocar busca entre candidatas (los ficheros que hay ahora en las carpetas vigiladas)
// los originales enlazados que ya no están en su sitio, por su tamaño y su huella, y
// apunta la ruta nueva (photos.Repository.Recolocar). Devuelve cuántos ha recolocado.
func (f *Fototeca) Recolocar(ctx context.Context, candidatas []string) (int, error) {
	fs, err := f.consultarFotos(ctx, f.db, `SELECT `+columnasFoto+` FROM fotos f WHERE f.ruta <> '' AND f.purgada IS NULL`)
	if err != nil {
		return 0, err
	}
	porTamano := map[int64][]foto{}
	enUso := map[string]bool{}
	for _, x := range fs {
		if photos.ConEstado(x.vista()).EstadoExterno == "" {
			enUso[filepath.Clean(x.ruta)] = true
			continue
		}
		porTamano[x.fila.Bytes] = append(porTamano[x.fila.Bytes], x)
	}
	if len(porTamano) == 0 {
		return 0, ctx.Err()
	}
	hechas := 0
	for _, c := range candidatas {
		if err := ctx.Err(); err != nil {
			return hechas, err
		}
		c = filepath.Clean(c)
		info, err := os.Stat(c)
		if err != nil || !info.Mode().IsRegular() || enUso[c] || len(porTamano[info.Size()]) == 0 {
			continue
		}
		_, suma, _, err := photos.HuellaDeFichero(ctx, c)
		if err != nil {
			continue
		}
		lista := porTamano[info.Size()]
		for i, x := range lista {
			if x.huella != suma {
				continue
			}
			cuando := info.ModTime()
			err := f.tx(ctx, func(tx *sql.Tx) error {
				_, err := tx.ExecContext(ctx, `UPDATE fotos SET ruta = ?, modificado = ?, editada = ? WHERE id = ? AND purgada IS NULL`,
					c, enMarca(cuando), enMarca(f.ahora()), x.fila.ID)
				return err
			})
			if err != nil {
				return hechas, err
			}
			hechas++
			enUso[c] = true
			porTamano[info.Size()] = append(lista[:i:i], lista[i+1:]...)
			break
		}
	}
	return hechas, nil
}

// Purgar borra para siempre una foto de la papelera, o la papelera entera con id vacío,
// y devuelve cuántas se han ido. Se va de la galería, de sus álbumes y de las repetidas,
// y sus ficheros (el original copiado y las derivadas) del disco en el acto si nadie más
// los cita; el de una enlazada no se toca. Queda su huella, para que lo borrado no se
// vuelva a importar. Vaciar la papelera barre además lo que un corte dejó sin citar.
func (f *Fototeca) Purgar(ctx context.Context, id string) (int, error) {
	hechas := 0
	err := f.tx(ctx, func(tx *sql.Tx) error {
		var fs []foto
		var err error
		if id == "" {
			fs, err = f.consultarFotos(ctx, tx, `SELECT `+columnasFoto+` FROM fotos f WHERE f.borrada IS NOT NULL AND f.purgada IS NULL`)
		} else {
			var x foto
			x, err = f.unaFoto(ctx, tx, id)
			if err == nil && (x.fila.DeletedAt == nil || x.fila.PurgedAt != nil) {
				err = ErrNotFound
			}
			fs = []foto{x}
		}
		if err != nil {
			return err
		}
		ahora := enMarca(f.ahora())
		for _, x := range fs {
			if err := f.soltarFicheros(ctx, tx, x); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE fotos SET purgada = ?, editada = ? WHERE id = ?`, ahora, ahora, x.fila.ID); err != nil {
				return err
			}
			hechas++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	f.borrarPendientes(ctx)
	if id == "" {
		if n, b, err := f.barrer(ctx); err == nil && n > 0 {
			logBarrido(n, b)
		}
	}
	return hechas, nil
}

// soltarFicheros quita en tx lo que cita los ficheros de x (sus derivadas, su receta y
// sus álbumes) y apunta para borrar los que dejan de citarse, el original copiado entre
// ellos.
func (f *Fototeca) soltarFicheros(ctx context.Context, tx *sql.Tx, x foto) error {
	shas, err := columna(ctx, tx, `SELECT sha FROM derivadas WHERE foto = ?`, x.fila.ID)
	if err != nil {
		return err
	}
	if x.retenido {
		shas = append(shas, x.huella)
	}
	for _, q := range []string{
		`DELETE FROM derivadas WHERE foto = ?`, `DELETE FROM ediciones WHERE foto = ?`,
		`DELETE FROM ediciones_a_medias WHERE foto = ?`, `DELETE FROM album_fotos WHERE foto = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, x.fila.ID); err != nil {
			return err
		}
	}
	return apuntarParaBorrar(ctx, tx, shas...)
}
