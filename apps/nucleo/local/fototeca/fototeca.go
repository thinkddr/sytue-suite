// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package fototeca es la fototeca de Sytue Photos de Sytue Office sobre SQLite: el plan
// B de §5.6 del plan de la edición local (`apps/fyne/PLAN-EDICION-LOCAL.md`).
//
// # Por qué
//
// La fototeca vivía en el motor de registros de la edición conectada sobre el almacén
// en claro (`local/almacen`), que reconstruye y valida en memoria el catálogo entero al
// abrir: con 50.000 fotos y 200 álbumes la prueba de carga dio 26,7 s para abrir y 2,7 GB
// de pico (umbral: 4 s y 400 MB). Compactar no bastaba, así que Photos —solo Photos—
// pasa a SQLite con índices, como Drive, Cast y Mail (D-09).
//
// # Qué hay dónde
//
//	<dir>/fototeca.sqlite     las filas y los índices (WAL)
//	<dir>/objetos/ab/<sha>    los ficheros, en claro y por su huella SHA-256: originales
//	                          copiados, derivadas y pósters, como en el almacén de antes
//
// Las REGLAS son las del repositorio de la conectada (`nucleo/photos`), que las comparte
// como funciones puras (`photos/compartido.go`): cómo se mide una foto o un vídeo, cómo
// se ve, el orden y el cursor de la línea de tiempo, qué casa con una búsqueda, los años,
// los recuerdos, las repetidas, «Por aquí», las derivadas y cómo se exportan los
// originales. Lo que cambia es dónde se guarda: aquí cada lectura es una consulta por un
// índice, no un recorrido del catálogo. El mismo juego de pruebas de comportamiento
// (`photos/contrato`) pasa contra las dos.
//
// # Escribir
//
// Una escritura es UNA transacción (BEGIN IMMEDIATE): o está entera o no está. Un
// fichero nuevo se escribe antes a un temporal de `objetos/` y se pone en su sitio
// DENTRO de la transacción, con el cerrojo de escritura de SQLite; y un fichero que ya
// no cita nadie se borra también con ese cerrojo (objetos.go). Así dos procesos (Photos y
// el «Traer» del Hub) no se pisan: SQLite serializa las escrituras y cada uno ve lo del
// otro en cuanto se confirma.
//
// # Migrar
//
// Quien ya tenía fotos en el almacén de registros las pasa a SQLite la primera vez que
// abre (migrar.go): en una transacción, comprobando los recuentos, con el diario viejo
// conservado y renombrado, nunca borrado.
package fototeca

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3" // el motor de SQLite de la fototeca

	"git.sytue.eu/sytue/one/apps/nucleo/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Fichero es el nombre de la base dentro de la carpeta de Photos.
const Fichero = "fototeca.sqlite"

// Producto es la clave del producto: la carpeta `<Datos>/Datos/photos` y el alcance.
const Producto = "photos"

// marca es el formato de las fechas guardadas: ANCHO FIJO y en UTC, para que ordenar el
// texto sea ordenar el tiempo (la línea de tiempo se ordena por él en el índice).
const marca = "2006-01-02T15:04:05.000000000Z"

// Errores de la fototeca: los del repositorio de la conectada, para que la pantalla los
// trate igual.
var (
	ErrNotFound = photos.ErrNotFound
	ErrConflict = photos.ErrConflict
	ErrSource   = photos.ErrSource
	// ErrCerrada: la fototeca ya está cerrada.
	ErrCerrada = errors.New("fototeca: la fototeca está cerrada")
)

const esquema = `
CREATE TABLE IF NOT EXISTS meta (clave TEXT PRIMARY KEY, valor TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS fotos (
  id          TEXT PRIMARY KEY,
  nombre      TEXT NOT NULL,
  tipo        TEXT NOT NULL,
  bytes       INTEGER NOT NULL,
  ancho       INTEGER NOT NULL,
  alto        INTEGER NOT NULL,
  orientacion INTEGER NOT NULL,
  tomada      TEXT,                       -- la de captura (EXIF o la que se dijo); NULL, no se sabe
  subida      TEXT NOT NULL,              -- cuándo entró
  fecha       TEXT NOT NULL,              -- la de la galería: tomada o, sin ella, subida
  anyo        INTEGER NOT NULL,           -- el año de fecha, en UTC
  mes_dia     INTEGER NOT NULL,           -- mes*100+día de tomada en UTC; 0 sin fecha de captura
  camara      TEXT NOT NULL DEFAULT '',
  objetivo    TEXT NOT NULL DEFAULT '',
  favorita    INTEGER NOT NULL DEFAULT 0,
  descripcion TEXT NOT NULL DEFAULT '',
  etiquetas   TEXT NOT NULL DEFAULT '[]', -- JSON
  texto       TEXT NOT NULL DEFAULT '',   -- lo que se busca, plegado
  lat         REAL,
  lon         REAL,
  duracion_ms INTEGER NOT NULL DEFAULT 0,
  huella      TEXT NOT NULL DEFAULT '',   -- SHA-256 del original, también de una purgada
  retenido    INTEGER NOT NULL DEFAULT 0, -- el original es un fichero de objetos/
  ruta        TEXT NOT NULL DEFAULT '',   -- el original enlazado (FO-09): su fichero
  modificado  TEXT,                       -- y su fecha al enlazarlo
  borrada     TEXT,                       -- en la papelera desde entonces
  purgada     TEXT,                       -- borrada para siempre
  creada      TEXT NOT NULL,
  editada     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS fotos_linea ON fotos (fecha DESC, id DESC) WHERE borrada IS NULL;
CREATE INDEX IF NOT EXISTS fotos_favoritas ON fotos (fecha DESC, id DESC) WHERE borrada IS NULL AND favorita = 1;
CREATE INDEX IF NOT EXISTS fotos_papelera ON fotos (borrada DESC, id DESC) WHERE borrada IS NOT NULL AND purgada IS NULL;
CREATE INDEX IF NOT EXISTS fotos_anyos ON fotos (anyo, favorita) WHERE borrada IS NULL;
CREATE INDEX IF NOT EXISTS fotos_recuerdos ON fotos (mes_dia, fecha DESC, id DESC) WHERE borrada IS NULL AND mes_dia > 0;
CREATE INDEX IF NOT EXISTS fotos_huella ON fotos (huella);
CREATE INDEX IF NOT EXISTS fotos_lugar ON fotos (lat) WHERE borrada IS NULL AND lat IS NOT NULL AND lon IS NOT NULL;
CREATE INDEX IF NOT EXISTS fotos_enlazadas ON fotos (bytes) WHERE ruta <> '' AND purgada IS NULL;
CREATE TABLE IF NOT EXISTS derivadas (
  foto     TEXT NOT NULL,
  receta   TEXT NOT NULL,                 -- la huella de la receta de la que sale
  variante TEXT NOT NULL,
  formato  TEXT NOT NULL,                 -- jpg | png
  sha      TEXT NOT NULL,                 -- el fichero en objetos/
  bytes    INTEGER NOT NULL,
  PRIMARY KEY (foto, receta, variante)
);
CREATE INDEX IF NOT EXISTS derivadas_sha ON derivadas (sha);
CREATE TABLE IF NOT EXISTS ediciones (
  foto    TEXT PRIMARY KEY,
  receta  TEXT NOT NULL,                  -- JSON (photos/model.Receta)
  huella  TEXT NOT NULL,
  editada TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS ediciones_a_medias (
  foto   TEXT PRIMARY KEY,
  receta TEXT NOT NULL,
  base   TEXT NOT NULL,
  creada TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS albumes (
  id      TEXT PRIMARY KEY,
  titulo  TEXT NOT NULL,
  portada TEXT,
  origen  TEXT NOT NULL DEFAULT '',       -- de dónde vino («sytue:<id>»); vacío, de aquí
  creado  TEXT NOT NULL,
  editado TEXT NOT NULL,
  borrado TEXT
);
CREATE INDEX IF NOT EXISTS albumes_orden ON albumes (creado DESC, id DESC) WHERE borrado IS NULL;
CREATE INDEX IF NOT EXISTS albumes_origen ON albumes (origen) WHERE origen <> '';
CREATE TABLE IF NOT EXISTS album_fotos (
  album    TEXT NOT NULL,
  foto     TEXT NOT NULL,
  posicion INTEGER NOT NULL,
  anadida  TEXT NOT NULL,
  PRIMARY KEY (album, foto)
);
CREATE INDEX IF NOT EXISTS album_fotos_orden ON album_fotos (album, posicion, foto);
CREATE INDEX IF NOT EXISTS album_fotos_foto ON album_fotos (foto);
CREATE TABLE IF NOT EXISTS por_borrar (sha TEXT PRIMARY KEY);
INSERT OR IGNORE INTO meta (clave, valor) VALUES ('version', '1');
`

// Fototeca es la fototeca de una persona en una carpeta.
type Fototeca struct {
	db    *sql.DB
	dir   string
	scope storage.Scope
	// extra son los formatos de foto que se admiten además de JPEG, PNG y GIF
	// (AdmitirFormatos).
	extra map[string]bool
	// ahora es el reloj; las pruebas lo fijan.
	ahora func() time.Time

	mu      sync.Mutex
	cerrada bool
}

// Opciones de Abrir.
type Opciones struct {
	// Avance, si no es nil, dice cómo va la migración del almacén de antes (migrar.go):
	// la fase y, al copiar, cuántas filas de cuántas.
	Avance func(Avance)
	// Formatos son los de foto que se admiten además de JPEG, PNG y GIF, como los
	// nombra `image.RegisterFormat` («webp», «avif», «bmp»); también al migrar.
	Formatos []string
}

// Abrir abre (o crea) la fototeca de la carpeta dir de la persona sujeto (el id de la
// instalación, como el almacén de antes). Si en dir hay fotos del almacén de registros
// sin migrar, las migra antes (migrar.go); si alguien tiene ese almacén abierto, falla
// con ErrEnUso y no toca nada.
func Abrir(ctx context.Context, dir, sujeto string, o Opciones) (*Fototeca, error) {
	scope := storage.Scope{Tenant: "local", Subject: sujeto, Product: Producto}
	if !scope.Valid() {
		return nil, fmt.Errorf("fototeca: alcance inválido")
	}
	if err := os.MkdirAll(filepath.Join(dir, "objetos"), 0o700); err != nil {
		return nil, err
	}
	u := url.URL{
		Scheme: "file", Path: filepath.ToSlash(filepath.Join(dir, Fichero)),
		RawQuery: "_journal_mode=WAL&_busy_timeout=10000&_txlock=immediate&_synchronous=FULL&_foreign_keys=0",
	}
	db, err := sql.Open("sqlite3", u.String())
	if err != nil {
		return nil, err
	}
	// Unas pocas conexiones: las lecturas no esperan a una escritura larga (WAL), y las
	// escrituras se serializan con el cerrojo de SQLite.
	db.SetMaxOpenConns(4)
	if _, err := db.ExecContext(ctx, esquema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("fototeca: no se pudo preparar la base: %w", err)
	}
	f := &Fototeca{db: db, dir: dir, scope: scope, ahora: time.Now}
	f.AdmitirFormatos(o.Formatos...)
	if err := f.alDia(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := f.migrarSiHaceFalta(ctx, o.Avance); err != nil {
		_ = db.Close()
		return nil, err
	}
	return f, nil
}

// AdmitirFormatos admite en la ingesta, además de JPEG, PNG y GIF, esos formatos de foto
// (photos.Repository.AdmitirFormatos). Se llama antes de usar la fototeca.
func (f *Fototeca) AdmitirFormatos(formatos ...string) {
	if f.extra == nil {
		f.extra = map[string]bool{}
	}
	for _, x := range formatos {
		f.extra[x] = true
	}
}

// Close cierra la fototeca. Antes borra los ficheros que quedaron por borrar.
func (f *Fototeca) Close() {
	f.mu.Lock()
	if f.cerrada {
		f.mu.Unlock()
		return
	}
	f.cerrada = true
	f.mu.Unlock()
	ctx, fin := context.WithTimeout(context.Background(), 30*time.Second)
	f.borrarPendientes(ctx)
	fin()
	_ = f.db.Close()
}

// Ruta es el fichero de la base.
func (f *Fototeca) Ruta() string { return filepath.Join(f.dir, Fichero) }

// Carpeta es la carpeta de la fototeca.
func (f *Fototeca) Carpeta() string { return f.dir }

// CopiarEnCaliente deja en destino una copia coherente de la base abierta (`VACUUM
// INTO`), sin cerrarla: la copia de seguridad (`copia.BaseAbierta`). Los ficheros de
// `objetos/` ya son ficheros.
func (f *Fototeca) CopiarEnCaliente(ctx context.Context, destino string) error {
	_, err := f.db.ExecContext(ctx, "VACUUM INTO ?", destino)
	return err
}

// PrivateStorageScope es el alcance de la fototeca (api.PhotosData): el de la fachada.
func (f *Fototeca) PrivateStorageScope() storage.Scope { return f.scope }

// RefreshPrivateData no tiene nada que hacer (api.PrivateDataRefresher): cada lectura
// va a la base, que ya tiene lo de todos los procesos.
func (f *Fototeca) RefreshPrivateData(ctx context.Context) error { return ctx.Err() }

// tx ejecuta escribir en una transacción de escritura.
func (f *Fototeca) tx(ctx context.Context, escribir func(*sql.Tx) error) error {
	t, err := f.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := escribir(t); err != nil {
		_ = t.Rollback()
		return err
	}
	return t.Commit()
}

// meta lee una clave de meta; vacía si no está.
func meta(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, clave string,
) (string, error) {
	var v string
	err := q.QueryRowContext(ctx, `SELECT valor FROM meta WHERE clave = ?`, clave).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// ponerMeta guarda una clave de meta.
func ponerMeta(ctx context.Context, tx *sql.Tx, clave, valor string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO meta (clave, valor) VALUES (?, ?) ON CONFLICT (clave) DO UPDATE SET valor = excluded.valor`, clave, valor)
	return err
}

// alDia vuelve a plegar el texto de búsqueda si el plegado ha cambiado desde que se
// guardó: la búsqueda compara lo guardado con la consulta plegada AHORA.
func (f *Fototeca) alDia(ctx context.Context) error {
	sello := selloDePlegado()
	guardado, err := meta(ctx, f.db, "plegado")
	if err != nil || guardado == sello {
		return err
	}
	return f.tx(ctx, func(tx *sql.Tx) error {
		fs, err := f.consultarFotos(ctx, tx, `SELECT `+columnasFoto+` FROM fotos f`)
		if err != nil {
			return err
		}
		for _, x := range fs {
			texto := photos.TextoDeBusqueda(x.fila.Name, x.fila.Description, x.fila.Tags)
			if _, err := tx.ExecContext(ctx, `UPDATE fotos SET texto = ? WHERE id = ?`, texto, x.fila.ID); err != nil {
				return err
			}
		}
		return ponerMeta(ctx, tx, "plegado", sello)
	})
}

// columna son los valores de la primera columna (texto) de una consulta.
func columna(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, consulta string, args ...any,
) ([]string, error) {
	filas, err := q.QueryContext(ctx, consulta, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = filas.Close() }()
	var out []string
	for filas.Next() {
		var v string
		if err := filas.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, filas.Err()
}

// selloDePlegado identifica el plegado de hoy: el de un texto con lo que pliega.
func selloDePlegado() string {
	return photos.TextoDeBusqueda("ÁÀÄÂÃÅáàäâãåÉÈËÊéèëêÍÌÏÎíìïîÓÒÖÔÕóòöôõÚÙÜÛúùüûÑñÇçŒœÆæßẞĞğŞşİıŁłØø", "ΑΒΓΔάέήίόύώ", []string{"ЖЗИЙ", "ﬁﬂ", "Ⅻ"})
}

// enMarca es t en el formato guardado.
func enMarca(t time.Time) string { return t.UTC().Format(marca) }

// deMarca lee una fecha guardada.
func deMarca(s string) (time.Time, error) {
	t, err := time.Parse(marca, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: fecha %q", ErrSource, s)
	}
	return t, nil
}

// deMarcaNula lee una fecha guardada que puede faltar.
func deMarcaNula(s sql.NullString) (*time.Time, error) {
	if !s.Valid {
		return nil, nil //nolint:nilnil // sin fecha no es un error
	}
	t, err := deMarca(s.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// marcaNula es t guardada, o NULL.
func marcaNula(t *time.Time) any {
	if t == nil {
		return nil
	}
	return enMarca(*t)
}

// recetaVacia es la huella de la receta vacía: la de las derivadas de una foto sin editar.
func recetaVacia() string {
	h, _ := model.RecetaVacia().Huella()
	return h
}
