// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fototeca

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// # Leer la fototeca de OTRA app, sin abrirla
//
// El Hub busca en las fotos y los álbumes (`local/indice`). No puede abrir la fototeca
// con Abrir: eso crea la base y migra, es decir, ESCRIBE en la carpeta de Photos, que
// puede estar abierta a la vez. LeerSinAbrir la abre en solo lectura (`mode=ro`): no
// crea nada si falta y, en WAL, lee lo confirmado sin bloquear a Photos.

// PlazoLectura es lo más que se espera a la base si está ocupada.
const PlazoLectura = 3 * time.Second

// Lectura es lo que hay en la fototeca, para buscar.
type Lectura struct {
	// Migrada: la fototeca de SQLite es la buena (ya no hay que leer el almacén de antes).
	Migrada bool
	// Fotos son las de la galería (fuera de la papelera).
	Fotos []FotoLeida
	// Albumes son los vivos, del más nuevo al más viejo.
	Albumes []AlbumLeido
}

// FotoLeida es una foto de la galería, con lo que se busca de ella.
type FotoLeida struct {
	ID, Nombre, Descripcion, Camara string
	Etiquetas                       []string
	// Tomada es la fecha de captura; nil si no se sabe. Creada, cuándo entró.
	Tomada *time.Time
	Creada time.Time
}

// AlbumLeido es un álbum, con su título y cuándo cambió.
type AlbumLeido struct {
	ID, Titulo string
	Editado    time.Time
}

// LeerSinAbrir lee la fototeca de dir sin escribir nada. Sin base, una Lectura vacía (no
// migrada) y sin error.
func LeerSinAbrir(dir string) (Lectura, error) {
	var l Lectura
	ruta := filepath.Join(dir, Fichero)
	if info, err := os.Stat(ruta); err != nil || !info.Mode().IsRegular() {
		return l, nil //nolint:nilerr // sin base no hay nada que leer
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(ruta), RawQuery: "mode=ro&_busy_timeout=2000"}
	db, err := sql.Open("sqlite3", u.String())
	if err != nil {
		return l, err
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	ctx, fin := context.WithTimeout(context.Background(), PlazoLectura)
	defer fin()
	hecha, err := meta(ctx, db, claveMigrada)
	if err != nil {
		return l, err
	}
	l.Migrada = hecha != ""
	if !l.Migrada {
		return l, nil
	}
	if l.Fotos, err = fotosLeidas(ctx, db); err != nil {
		return l, err
	}
	l.Albumes, err = albumesLeidos(ctx, db)
	return l, err
}

func fotosLeidas(ctx context.Context, db *sql.DB) ([]FotoLeida, error) {
	filas, err := db.QueryContext(ctx, `SELECT id, nombre, descripcion, camara, etiquetas, tomada, creada FROM fotos WHERE borrada IS NULL ORDER BY creada, id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = filas.Close() }()
	var out []FotoLeida
	for filas.Next() {
		var x FotoLeida
		var etiquetas, creada string
		var tomada sql.NullString
		if err := filas.Scan(&x.ID, &x.Nombre, &x.Descripcion, &x.Camara, &etiquetas, &tomada, &creada); err != nil {
			return nil, err
		}
		x.Etiquetas = leerEtiquetas(etiquetas)
		if x.Tomada, err = deMarcaNula(tomada); err != nil {
			return nil, err
		}
		if x.Creada, err = deMarca(creada); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, filas.Err()
}

func albumesLeidos(ctx context.Context, db *sql.DB) ([]AlbumLeido, error) {
	filas, err := db.QueryContext(ctx, `SELECT id, titulo, editado FROM albumes WHERE borrado IS NULL ORDER BY creado DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = filas.Close() }()
	var out []AlbumLeido
	for filas.Next() {
		var a AlbumLeido
		var editado string
		if err := filas.Scan(&a.ID, &a.Titulo, &editado); err != nil {
			return nil, err
		}
		if a.Editado, err = deMarca(editado); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, filas.Err()
}
