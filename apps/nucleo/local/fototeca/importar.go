// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fototeca

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/miniatura"
	"git.sytue.eu/sytue/one/apps/nucleo/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/exif"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	photorender "git.sytue.eu/sytue/one/apps/nucleo/photos/render"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// # Entrar en la fototeca
//
// Las tres puertas de la conectada, con sus mismas reglas (las mide photos.FilaDeCabecera,
// photos.MedirExterno y photos.FilaDeVideo; las derivadas, photos.DerivarVariantes):
//
//   - ImportarFoto COPIA una foto: su original va a objetos/ tal cual;
//   - ImportarExterno ENLAZA un fichero del equipo sin copiarlo (FO-09): se apunta su
//     ruta, su tamaño y su huella, y sus derivadas sí se guardan aquí;
//   - ImportarVideo COPIA un vídeo, con las derivadas de su póster.
//
// Cada una es UNA transacción con sus ficheros: o entra la foto entera, con su original y
// sus derivadas, o no entra nada (un corte deja, como mucho, temporales que se barren).
// Por eso aquí no hay importaciones a medias que reanudar. Importar dos veces el mismo
// trabajo (el mismo job) devuelve la foto que ya entró; con otros datos, ErrConflict.

// ImportarFoto copia a la fototeca la foto que lee de input (size bytes) y la devuelve.
// job es el UUID de la foto nueva.
func (f *Fototeca) ImportarFoto(ctx context.Context, job, name, contentType string, input io.Reader, size int64) (api.Photo, error) {
	if !uuidCanonico(job) || input == nil || !storage.ValidDriveName(name) || size <= 0 || size > photorender.MaxOriginal {
		return api.Photo{}, model.ErrInvalidInput
	}
	name = strings.TrimSpace(name)
	original, err := f.subir(ctx, input, size)
	if err != nil {
		return api.Photo{}, err
	}
	defer original.tirar()
	if ya, hecho, err := f.yaImportada(ctx, job, name, size, original.sha); hecho || err != nil {
		return ya, err
	}
	datos, err := f.leerTemporal(original, photorender.MaxOriginal)
	if err != nil {
		return api.Photo{}, err
	}
	cabecera := datos[:min(len(datos), exif.BytesDeCabecera)]
	fila, err := photos.FilaDeCabecera(f.scope, f.extra, job, name, contentType, size, cabecera)
	if err != nil {
		return api.Photo{}, err
	}
	derivadas, err := f.derivar(ctx, func(context.Context) ([]byte, error) { return datos, nil }, fila, model.RecetaVacia(), fila.MediaType == "image/png")
	if err != nil {
		return api.Photo{}, err
	}
	defer derivadas.tirar()
	return f.guardarNueva(ctx, foto{fila: fila, huella: original.sha, retenido: true}, original, derivadas)
}

// ImportarExterno enlaza en la fototeca el fichero o.Ruta sin copiarlo: mide la foto (o
// toma lo que se dice del vídeo), guarda sus derivadas y apunta la ruta y la huella del
// original. job es el UUID de la foto nueva.
func (f *Fototeca) ImportarExterno(ctx context.Context, job string, o photos.OrigenExterno) (api.Photo, error) {
	if !uuidCanonico(job) || !filepath.IsAbs(o.Ruta) {
		return api.Photo{}, model.ErrInvalidInput
	}
	ruta := filepath.Clean(o.Ruta)
	nombre := strings.TrimSpace(o.Nombre)
	if nombre == "" {
		nombre = filepath.Base(ruta)
	}
	info, suma, cabecera, err := photos.HuellaDeFichero(ctx, ruta)
	if err != nil {
		return api.Photo{}, err
	}
	if !storage.ValidDriveName(nombre) || info.Size() > storage.MaxFileBytes {
		return api.Photo{}, model.ErrInvalidInput
	}
	if ya, hecho, err := f.yaImportada(ctx, job, nombre, info.Size(), suma); hecho || err != nil {
		return ya, err
	}
	video := photos.EsVideo(o.Tipo)
	fila, err := photos.MedirExterno(f.scope, f.extra, job, nombre, o, video, info.Size(), cabecera)
	if err != nil {
		return api.Photo{}, err
	}
	if fila.MediaType == "" || len(fila.MediaType) > 1024 || strings.ContainsAny(fila.MediaType, "\r\n\x00") {
		return api.Photo{}, model.ErrInvalidInput
	}
	cargar := func(ctx context.Context) ([]byte, error) {
		return photos.LeerExterno(ctx, ruta, info.Size(), suma, photorender.MaxOriginal)
	}
	if video {
		poster := o.Poster
		if _, _, err := miniatura.Cabecera(poster); len(poster) == 0 || err != nil {
			poster = photos.PosterNeutro(fila.Width, fila.Height)
		}
		cargar = func(context.Context) ([]byte, error) { return poster, nil }
	}
	derivadas, err := f.derivar(ctx, cargar, fila, model.RecetaVacia(), fila.MediaType == "image/png")
	if err != nil {
		return api.Photo{}, err
	}
	defer derivadas.tirar()
	modificado := info.ModTime().UTC()
	return f.guardarNueva(ctx, foto{fila: fila, huella: suma, ruta: ruta, modificado: &modificado}, nil, derivadas)
}

// ImportarVideo copia un vídeo a la fototeca: sus bytes tal cual como original (sin el
// tope de una foto) y las derivadas de su póster. job es el UUID del vídeo nuevo.
func (f *Fototeca) ImportarVideo(ctx context.Context, job string, o photos.OrigenVideo) (api.Photo, error) {
	nombre := strings.TrimSpace(o.Nombre)
	tipo := strings.ToLower(strings.TrimSpace(strings.Split(o.Tipo, ";")[0]))
	if !uuidCanonico(job) || o.Datos == nil || !storage.ValidDriveName(nombre) || !photos.EsVideo(tipo) ||
		o.Tamano <= 0 || o.Tamano > storage.MaxFileBytes || len(tipo) > 1024 || strings.ContainsAny(tipo, "\r\n\x00") {
		return api.Photo{}, model.ErrInvalidInput
	}
	fila := photos.FilaDeVideo(f.scope, job, nombre, tipo, o.Tamano, o.Poster, o.Duracion, o.Ancho, o.Alto, o.TomadoEn)
	poster := o.Poster
	if _, _, err := miniatura.Cabecera(poster); len(poster) == 0 || int64(len(poster)) > photorender.MaxOriginal || err != nil {
		poster = photos.PosterNeutro(fila.Width, fila.Height)
	}
	original, err := f.subir(ctx, o.Datos, o.Tamano)
	if err != nil {
		return api.Photo{}, err
	}
	defer original.tirar()
	if ya, hecho, err := f.yaImportada(ctx, job, nombre, o.Tamano, original.sha); hecho || err != nil {
		return ya, err
	}
	derivadas, err := f.derivar(ctx, func(context.Context) ([]byte, error) { return poster, nil }, fila, model.RecetaVacia(), false)
	if err != nil {
		return api.Photo{}, err
	}
	defer derivadas.tirar()
	return f.guardarNueva(ctx, foto{fila: fila, huella: original.sha, retenido: true}, original, derivadas)
}

// ReanudarImportaciones no tiene nada que reanudar (api.PhotosDirectIngestData): aquí una
// importación entra entera o no entra.
func (f *Fototeca) ReanudarImportaciones(ctx context.Context) ([]api.Photo, error) {
	return []api.Photo{}, ctx.Err()
}

// yaImportada mira si el trabajo job ya entró: con los mismos datos, devuelve su foto
// (hecho); con otros, ErrConflict.
func (f *Fototeca) yaImportada(ctx context.Context, job, nombre string, tamano int64, sha string) (api.Photo, bool, error) {
	x, err := f.unaFoto(ctx, f.db, job)
	if errors.Is(err, ErrNotFound) {
		return api.Photo{}, false, nil
	}
	if err != nil {
		return api.Photo{}, true, err
	}
	if x.huella != sha || x.fila.Name != nombre || x.fila.Bytes != tamano || x.fila.PurgedAt != nil {
		return api.Photo{}, true, ErrConflict
	}
	if x.fila.DeletedAt != nil {
		return api.Photo{}, true, ErrNotFound
	}
	return photos.ConEstado(x.vista()), true, nil
}

// leerTemporal lee el temporal de s, de hasta tope bytes.
func (f *Fototeca) leerTemporal(s *subida, tope int64) ([]byte, error) {
	if s.bytes > tope {
		return nil, model.ErrInvalidInput
	}
	return os.ReadFile(s.tmp)
}

// derivadasNuevas son las derivadas de una receta, escritas a sus temporales.
type derivadasNuevas struct {
	receta  string
	formato string
	subidas map[model.Variante]*subida
}

func (d *derivadasNuevas) tirar() {
	for _, s := range d.subidas {
		s.tirar()
	}
}

// derivar genera las derivadas de la foto fila con la receta, del original que da
// cargar, y las deja en sus temporales.
func (f *Fototeca) derivar(ctx context.Context, cargar func(context.Context) ([]byte, error), fila model.AssetRow, receta model.Receta, png bool) (*derivadasNuevas, error) {
	huella, err := receta.Huella()
	if err != nil {
		return nil, err
	}
	d := &derivadasNuevas{receta: huella, subidas: map[model.Variante]*subida{}}
	err = photos.DerivarVariantes(ctx, cargar, model.OrientacionDe(fila.Orientation), receta, png, func(v model.Variante, formato string, datos []byte) error {
		s, err := f.subirBytes(ctx, datos)
		if err != nil {
			return err
		}
		d.formato, d.subidas[v] = formato, s
		return nil
	})
	if err == nil && len(d.subidas) != len(model.Variantes()) {
		err = ErrSource
	}
	if err != nil {
		d.tirar()
		return nil, err
	}
	return d, nil
}

// colocarDerivadas pone en su sitio, dentro de tx, las derivadas de la foto id, y apunta
// para borrar las que sustituyen.
func (f *Fototeca) colocarDerivadas(ctx context.Context, tx *sql.Tx, id string, d *derivadasNuevas) error {
	for _, v := range model.Variantes() {
		s := d.subidas[v]
		var antes string
		err := tx.QueryRowContext(ctx, `SELECT sha FROM derivadas WHERE foto = ? AND receta = ? AND variante = ?`, id, d.receta, string(v)).Scan(&antes)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := f.colocar(ctx, tx, s); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO derivadas (foto, receta, variante, formato, sha, bytes) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (foto, receta, variante) DO UPDATE SET formato = excluded.formato, sha = excluded.sha, bytes = excluded.bytes`,
			id, d.receta, string(v), d.formato, s.sha, s.bytes); err != nil {
			return err
		}
		if antes != "" && antes != s.sha {
			if err := apuntarParaBorrar(ctx, tx, antes); err != nil {
				return err
			}
		}
	}
	return nil
}

// guardarNueva da de alta la foto x con su original (si es copiado) y sus derivadas, en
// una transacción. Si entretanto entró otra con su id, es ErrConflict (o ella misma).
func (f *Fototeca) guardarNueva(ctx context.Context, x foto, original *subida, d *derivadasNuevas) (api.Photo, error) {
	err := f.tx(ctx, func(tx *sql.Tx) error {
		var hay int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM fotos WHERE id = ?`, x.fila.ID).Scan(&hay); err != nil {
			return err
		}
		if hay > 0 {
			return errYaEsta
		}
		if original != nil {
			if err := f.colocar(ctx, tx, original); err != nil {
				return err
			}
		}
		if err := insertarFoto(ctx, tx, x); err != nil {
			return err
		}
		return f.colocarDerivadas(ctx, tx, x.fila.ID, d)
	})
	if errors.Is(err, errYaEsta) {
		ya, _, err := f.yaImportada(ctx, x.fila.ID, x.fila.Name, x.fila.Bytes, x.huella)
		return ya, err
	}
	if err != nil {
		return api.Photo{}, err
	}
	return f.Photo(ctx, x.fila.ID)
}

var errYaEsta = errors.New("fototeca: ya está")

// insertarFoto escribe en tx la fila de x.
func insertarFoto(ctx context.Context, tx *sql.Tx, x foto) error {
	a := x.fila
	fecha, anyo, mesDia := fechasDe(a)
	var lat, lon any
	if a.Latitude != nil && a.Longitude != nil {
		lat, lon = *a.Latitude, *a.Longitude
	}
	huella := x.huella
	if huella == "" {
		huella = strings.ToLower(a.OriginalSHA256)
	}
	creada, editada := a.CreatedAt, a.UpdatedAt
	if creada.IsZero() {
		creada = a.UploadedAt
	}
	if editada.IsZero() {
		editada = creada
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO fotos (id, nombre, tipo, bytes, ancho, alto, orientacion, tomada, subida, fecha, anyo, mes_dia,
		camara, objetivo, favorita, descripcion, etiquetas, texto, lat, lon, duracion_ms, huella, retenido, ruta, modificado,
		borrada, purgada, creada, editada) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, a.MediaType, a.Bytes, a.Width, a.Height, a.Orientation, marcaNula(a.TakenAt), enMarca(a.UploadedAt), fecha, anyo, mesDia,
		a.Camera, a.Lens, a.Favorite, a.Description, etiquetasJSON(a.Tags), photos.TextoDeBusqueda(a.Name, a.Description, a.Tags),
		lat, lon, a.DurationMs, huella, x.retenido, x.ruta, marcaNula(x.modificado),
		marcaNula(a.DeletedAt), marcaNula(a.PurgedAt), enMarca(creada), enMarca(editada))
	return err
}

// uuidCanonico dice si s es un UUID escrito como lo escribe uuid.String.
func uuidCanonico(s string) bool {
	id, err := uuid.Parse(s)
	return err == nil && id != uuid.Nil && id.String() == s
}
