// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fototeca

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/photos"
)

// # La línea de tiempo y lo que se calcula sobre ella
//
// La línea es el índice `fotos_linea` (fecha de la galería y id, de mayor a menor, solo
// lo que no está en la papelera): una página es leerlo desde el cursor, sin recorrer la
// galería. El cursor y el orden son los de la conectada (photos.CursorDeLinea,
// photos.CompararEnLinea): la fecha guardada es la misma de la vista, en UTC y con ancho
// fijo, así que ordenar el texto es ordenar el tiempo. Los años, los recuerdos, las
// repetidas y «Por aquí» preguntan por su índice lo que les hace falta y pasan esas
// fotos, en el orden de la línea, por las reglas compartidas (photos.Anyos…).

// Timeline es una página de la línea de tiempo: la más reciente primero. cursor vacío es
// la primera; con favorites, solo las favoritas; con text (o los términos also con que
// la IA amplía la búsqueda), las que lo llevan en el nombre, la descripción o las
// etiquetas, sin mayúsculas ni acentos.
func (f *Fototeca) Timeline(ctx context.Context, cursor string, limit int, favorites bool, text string, also ...string) ([]api.Photo, string, error) {
	return f.linea(ctx, cursor, false, limit, favorites, text, also)
}

// TimelineDesde es la primera página de la línea empezando en una fecha (`2019`,
// `2019-07` o `2019-07-14`); las siguientes, con Timeline y su cursor.
func (f *Fototeca) TimelineDesde(ctx context.Context, desde string, limit int, favorites bool) ([]api.Photo, string, error) {
	cursor, err := photos.CursorDesde(f.scope, desde)
	if err != nil {
		return nil, "", err
	}
	return f.linea(ctx, cursor, true, limit, favorites, "", nil)
}

func (f *Fototeca) linea(ctx context.Context, cursor string, desdeFecha bool, limit int, favorites bool, text string, also []string) ([]api.Photo, string, error) {
	var (
		donde = []string{"f.borrada IS NULL"}
		args  []any
	)
	if cursor != "" {
		fecha, id, err := photos.LeerCursorDeLinea(f.scope, cursor, desdeFecha)
		if err != nil {
			return nil, "", err
		}
		donde = append(donde, "(f.fecha, f.id) < (?, ?)")
		args = append(args, enMarca(fecha), id)
	}
	if limit <= 0 || limit > 200 {
		limit = 60
	}
	if favorites {
		donde = append(donde, "f.favorita = 1")
	}
	if text != "" {
		casa, todas := consultaDeTexto(append([]string{text}, also...))
		if !todas {
			donde = append(donde, casa)
			for _, q := range append([]string{text}, also...) {
				if q != "" {
					args = append(args, buscar.Plegar(q))
				}
			}
		}
	}
	args = append(args, limit+1)
	//nolint:gosec // G202: donde son condiciones fijas de este fichero, con `?`; los valores van en args
	fs, err := f.consultarFotos(ctx, f.db, `SELECT `+columnasFoto+` FROM fotos f WHERE `+strings.Join(donde, " AND ")+
		` ORDER BY f.fecha DESC, f.id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(fs) > limit {
		last := fs[limit-1].vista()
		next = photos.CursorDeLinea(f.scope, last.TakenAt, last.ID)
		fs = fs[:limit]
	}
	return vistas(fs), next, nil
}

// consultaDeTexto es la condición de las fotos que llevan alguna de las consultas no
// vacías: `instr` sobre el texto ya plegado, con un `?` por consulta. todas es true si
// alguna, plegada, queda vacía (lo lleva cualquier foto).
func consultaDeTexto(consultas []string) (casa string, todas bool) {
	var o []string
	for _, q := range consultas {
		if q == "" {
			continue
		}
		if buscar.Plegar(q) == "" {
			return "", true
		}
		o = append(o, "instr(f.texto, ?) > 0")
	}
	if len(o) == 0 {
		return "0", false
	}
	return "(" + strings.Join(o, " OR ") + ")", false
}

// TimelineYears son los años con fotos, del más reciente al más antiguo, con cuántas
// tiene cada uno (solo las favoritas, con favorites).
func (f *Fototeca) TimelineYears(ctx context.Context, favorites bool) ([]api.TimelineYear, error) {
	q := `SELECT anyo, COUNT(*) FROM fotos WHERE borrada IS NULL GROUP BY anyo ORDER BY anyo DESC`
	if favorites {
		q = `SELECT anyo, COUNT(*) FROM fotos WHERE borrada IS NULL AND favorita = 1 GROUP BY anyo ORDER BY anyo DESC`
	}
	filas, err := f.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer func() { _ = filas.Close() }()
	out := []api.TimelineYear{}
	for filas.Next() {
		var y api.TimelineYear
		if err := filas.Scan(&y.Year, &y.Count); err != nil {
			return nil, err
		}
		out = append(out, y)
	}
	return out, filas.Err()
}

// Memories son las fotos hechas un día como hoy en años anteriores (photos.Recuerdos).
func (f *Fototeca) Memories(ctx context.Context) ([]api.Memory, error) {
	hoy := f.ahora()
	fs, err := f.consultarFotos(ctx, f.db, `SELECT `+columnasFoto+` FROM fotos f
		WHERE f.borrada IS NULL AND f.mes_dia > 0 AND f.mes_dia = ? AND f.anyo < ? ORDER BY f.fecha DESC, f.id DESC`,
		int(hoy.Month())*100+hoy.Day(), hoy.Year())
	if err != nil {
		return nil, err
	}
	return photos.Recuerdos(soloVistas(fs), hoy), nil
}

func soloVistas(fs []foto) []api.Photo {
	out := make([]api.Photo, len(fs))
	for i, x := range fs {
		out[i] = x.vista()
	}
	return out
}

// Duplicates son los grupos de fotos con el mismo original (photos.Repetidas).
func (f *Fototeca) Duplicates(ctx context.Context, limite int) ([][]api.Photo, error) {
	if limite <= 0 || limite > 200 {
		limite = 50
	}
	fs, err := f.consultarFotos(ctx, f.db, `SELECT `+columnasFoto+` FROM fotos f
		WHERE f.borrada IS NULL AND f.huella IN (
			SELECT huella FROM fotos WHERE borrada IS NULL AND huella <> '' GROUP BY huella HAVING COUNT(*) > 1)
		ORDER BY f.fecha DESC, f.id DESC`)
	if err != nil {
		return nil, err
	}
	con := make([]photos.FotoConHuella, len(fs))
	for i, x := range fs {
		con[i] = x.conHuella()
	}
	return photos.Repetidas(con, limite), nil
}

// gradosDeCerca es lo que se aparta en latitud una foto a menos de RadioDeCerca: un
// kilómetro de meridiano son 1/111,2 grados en una Tierra de 6.371 km; con margen.
const gradosDeCerca = photos.RadioDeCerca/111.0 + 0.001

// Nearby son las fotos hechas a menos de photos.RadioDeCerca de la dada (photos.Cerca).
// Solo se miran las de su franja de latitud: lo que está más al norte o al sur ya está
// más lejos.
func (f *Fototeca) Nearby(ctx context.Context, id string, limite int) ([]api.Photo, error) {
	if limite <= 0 || limite > photos.TopeDeCerca {
		limite = 48
	}
	centro, err := f.unaFoto(ctx, f.db, id)
	if err != nil || centro.fila.DeletedAt != nil {
		return nil, ErrNotFound
	}
	if centro.fila.Latitude == nil || centro.fila.Longitude == nil {
		return []api.Photo{}, nil
	}
	lat := *centro.fila.Latitude
	fs, err := f.consultarFotos(ctx, f.db, `SELECT `+columnasFoto+` FROM fotos f
		WHERE f.borrada IS NULL AND f.lat IS NOT NULL AND f.lon IS NOT NULL AND f.lat BETWEEN ? AND ?
		ORDER BY f.fecha DESC, f.id DESC`, lat-gradosDeCerca, lat+gradosDeCerca)
	if err != nil {
		return nil, err
	}
	return photos.Cerca(soloVistas(fs), id, limite)
}

// ExportarFototeca saca los originales de la galería (fuera de la papelera) a
// dir/AAAA/MM/, tal y como entraron (photos.ExportarOriginales).
func (f *Fototeca) ExportarFototeca(ctx context.Context, dir string, avance func(hechas, total int)) (photos.ResumenExportacion, error) {
	if dir == "" {
		return photos.ResumenExportacion{}, os.ErrInvalid
	}
	fs, err := f.consultarFotos(ctx, f.db, `SELECT `+columnasFoto+` FROM fotos f WHERE f.borrada IS NULL ORDER BY f.fecha DESC, f.id DESC`)
	if err != nil {
		return photos.ResumenExportacion{}, err
	}
	con := make([]photos.FotoConHuella, len(fs))
	for i, x := range fs {
		con[i] = x.conHuella()
	}
	return photos.ExportarOriginales(ctx, dir, con, f.Original, avance)
}

// logBarrido apunta lo que barrió vaciar la papelera.
func logBarrido(n int, liberados int64) {
	slog.Info("fototeca: ficheros sin citar barridos", "ficheros", n, "bytes", liberados, "cuando", time.Now().UTC().Format(time.RFC3339))
}
