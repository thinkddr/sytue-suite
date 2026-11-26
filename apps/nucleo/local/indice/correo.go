// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package indice

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3" // el motor del índice de Mail

	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
)

// # El correo de Mail, en solo lectura
//
// Mail guarda los sobres de todas sus cuentas en un índice SQLite
// (`<datos>/Correo/indice.sqlite`, `local/correo`) que puede estar abierto y
// escribiéndose a la vez. Aquí se abre con `mode=ro`: SQLite no deja escribir por esa
// conexión, no crea el fichero si falta y, en modo WAL, lee lo confirmado sin bloquear
// a Mail. No se carga entero en memoria: se le pregunta al buscar y para «Hoy».

// PlazoCorreo es lo más que se espera al índice de Mail: si está ocupado, se sigue sin él.
const PlazoCorreo = 3 * time.Second

// rutaCorreo es el índice de Mail de la carpeta de datos, o vacía si no hay correo.
func rutaCorreo(datos string) string {
	ruta := filepath.Join(datos, "Correo", "indice.sqlite")
	if info, err := os.Stat(ruta); err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return ruta
}

// abrirCorreo abre el índice en solo lectura.
func abrirCorreo(ruta string) (*sql.DB, error) {
	if ruta == "" {
		return nil, fs.ErrNotExist
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(ruta), RawQuery: "mode=ro&_busy_timeout=2000"}
	db, err := sql.Open("sqlite3", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// noBuscados son los roles de carpeta cuyo correo no se enseña: lo tirado y el spam.
const noBuscados = "COALESCE(f.rol, '') NOT IN ('trash', 'junk', 'spam')"

const columnasCorreo = `m.id, m.hilo, m.asunto, m.de, m.de_nombre, m.vista, MAX(m.fecha, m.recibido)`

func leerCorreos(ctx context.Context, db *sql.DB, donde string, args []any, n int) ([]Elemento, error) {
	//nolint:gosec // G202: donde son condiciones fijas de este fichero, con `?`; los valores van en args
	filas, err := db.QueryContext(ctx, `SELECT `+columnasCorreo+` FROM mensajes m LEFT JOIN carpetas f
		ON f.cuenta = m.cuenta AND f.nombre = m.carpeta WHERE `+donde+` ORDER BY MAX(m.fecha, m.recibido) DESC, m.id DESC LIMIT ?`,
		append(args, n)...)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var out []Elemento
	for filas.Next() {
		var id, fecha int64
		var hilo, asunto, de, deNombre, vista string
		if err := filas.Scan(&id, &hilo, &asunto, &de, &deNombre, &vista, &fecha); err != nil {
			return nil, err
		}
		e := Elemento{
			Producto: "mail", Clase: Correo, ID: strconv.FormatInt(id, 10), Titulo: asunto,
			Detalle: cmp.Or(strings.TrimSpace(deNombre), de), Texto: vista,
		}
		if hilo != "" {
			// Mail abre conversaciones: el enlace lleva a la del correo.
			e.Clase, e.ID = Hilo, hilo
		}
		if fecha > 0 {
			e.Fecha = time.Unix(fecha, 0).UTC()
		}
		out = append(out, e)
	}
	return out, filas.Err()
}

// buscarCorreo son los n correos más recientes en los que aparecen todos los términos
// (en el asunto, los remitentes y destinatarios o el cuerpo ya descargado: el
// `buscable` de Mail, ya sin acentos ni mayúsculas).
func buscarCorreo(ruta string, terminos []string, n int) ([]Elemento, error) {
	db, err := abrirCorreo(ruta)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	ctx, cancelar := context.WithTimeout(context.Background(), PlazoCorreo)
	defer cancelar()
	conds := []string{noBuscados}
	var args []any
	for _, t := range terminos {
		conds = append(conds, "m.buscable LIKE ? ESCAPE '\\'")
		args = append(args, "%"+escaparLike(buscar.Plegar(t))+"%")
	}
	return leerCorreos(ctx, db, strings.Join(conds, " AND "), args, n)
}

func escaparLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// sinLeer es cuántos correos hay sin leer en Recibidos (de todas las cuentas, sin lo
// pospuesto, como el contador de Mail) y los n más recientes.
func sinLeer(ruta string, n int) (int, []Elemento, error) {
	db, err := abrirCorreo(ruta)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil, nil
		}
		return 0, nil, err
	}
	defer db.Close()
	ctx, cancelar := context.WithTimeout(context.Background(), PlazoCorreo)
	defer cancelar()
	const donde = `f.rol = 'inbox' AND m.pospuesto = 0 AND m.marcas NOT LIKE '%\Seen%'`
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mensajes m LEFT JOIN carpetas f
		ON f.cuenta = m.cuenta AND f.nombre = m.carpeta WHERE `+donde).Scan(&total); err != nil {
		return 0, nil, err
	}
	es, err := leerCorreos(ctx, db, donde, nil, n)
	return total, es, err
}
