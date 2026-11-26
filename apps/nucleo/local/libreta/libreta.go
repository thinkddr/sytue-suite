// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package libreta es la libreta de direcciones de Sytue Office sobre SQLite: el plan B de
// PLAN-EDICION-LOCAL §5.6 para Contacts. La prueba de carga dio, con 20.000 contactos en
// el almacén de registros, 6,8 s para abrir (frente a 2 s), 1 GB de pico de memoria, casi
// medio segundo por cada alta y medio minuto para importar un CSV de 2.000 filas: el motor
// lee y valida la libreta entera al abrir y la recorre en cada escritura. Aquí cada cosa
// va con índices y cada escritura toca solo lo suyo.
//
// # Qué hay en la base
//
// `Datos/contacts/libreta.sqlite`, en claro y en modo WAL (los lectores de otras apps
// leen mientras Contacts escribe), con un solo escritor:
//
//   - `fila`: la libreta tal cual la entiende Contacts (`contacts/model`): el JSON de cada
//     libreta, tarjeta (la vCard entera, con la foto y lo que Contacts no usa), contacto,
//     correo, teléfono, etiqueta y miembro, con lo que lo relaciona en columnas
//     (libreta, tarjeta, contacto, etiqueta, UID) para leer por índice lo que una
//     escritura necesita. Es la verdad: lo demás se puede rehacer de ella.
//   - `libreta`, `contacto` y `grupo`: lo que se ve de cada uno (su `api.AddressBook`,
//     `api.Contact` y `api.ContactGroup` en JSON) con sus claves de orden, el texto
//     plegado que mira la búsqueda, si está destacado, en la papelera o en una libreta
//     borrada, y los índices de cada consulta de la pantalla: la lista en orden, la
//     búsqueda (sin acentos), la página de una libreta, la papelera, los favoritos, los
//     cumpleaños y los UID (CardDAV, «Traer»).
//   - `correo`: los correos de cada contacto, en minúsculas, por libreta: lo que mira
//     importar un CSV para no repetir a nadie.
//   - `favorito`: el índice de favoritos de Contacts, por UID: un UID destacado lo sigue
//     estando en los contactos que lleguen después con él.
//   - `meta`: la versión del esquema, la revisión (sube con cada escritura) y la marca de
//     la migración.
//
// # Las mismas reglas que Contacts
//
// Cada escritura lee de la base, por índice, la parte de la libreta que mira y toca (las
// libretas, el contacto o la etiqueta con todas sus filas, los que comparten su UID…),
// hace con ella una instantánea de Contacts y prepara el cambio con las MISMAS funciones
// que el repositorio de Contacts (`contacts.Snapshot.PrepararAlta`, `UpdatePerson`,
// `ChangeTrash`…), lo compone y valida igual, y guarda lo que cambia de una a otra. Así
// lo que se valida, los errores y las filas que salen son los de Contacts sin copiar su
// lógica. Las consultas leen lo que se ve, por índice, sin descodificar ninguna vCard.
//
// La edición conectada no cambia: sigue con `contacts.Repository` sobre el motor.
package libreta

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3" // el motor de SQLite de la libreta

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Fichero es el nombre de la base dentro de la carpeta del producto «contacts».
const Fichero = "libreta.sqlite"

// ErrSoloLectura es lo que contesta cualquier escritura en una libreta abierta para leer.
var ErrSoloLectura = errors.New("libreta: abierta en solo lectura")

// ErrSinMigrar: en esa carpeta no hay libreta SQLite, o aún no tiene lo del almacén de
// antes (Contacts no se ha abierto desde que la hay). Quien lee desde otra app sigue con
// el almacén de registros.
var ErrSinMigrar = errors.New("libreta: todavía no hay libreta SQLite en esta carpeta")

// Libreta es la libreta de Contacts de este equipo en SQLite. Cumple `api.ContactsData`
// (y las interfaces opcionales que mira la fachada) y `contacts.LibretasExternas`.
type Libreta struct {
	ruta  string
	scope storage.Scope
	// db es el escritor: una conexión, BEGIN IMMEDIATE. Nil en solo lectura.
	db *sql.DB
	// leer son las conexiones de las consultas: en WAL no esperan al escritor.
	leer *sql.DB
	// mu pone en fila las escrituras de este proceso (también lo que leen antes).
	mu sync.Mutex
	// ahora es el reloj de las escrituras; las pruebas lo fijan.
	ahora func() time.Time

	vaciado vaciado
}

var (
	_ api.ContactsData          = (*Libreta)(nil)
	_ api.ContactsBatchData     = (*Libreta)(nil)
	_ api.ContactsPhotoReader   = (*Libreta)(nil)
	_ api.PrivateDataRefresher  = (*Libreta)(nil)
	_ contacts.LibretasExternas = (*Libreta)(nil)
)

// Alcance es el de los contactos de la persona sujeto en Sytue Office: el mismo con el
// que los guardaba el almacén de registros.
func Alcance(sujeto string) storage.Scope {
	return storage.Scope{Tenant: "local", Subject: sujeto, Product: "contacts"}
}

// Abrir abre (o crea) la libreta de la carpeta dir (la del producto «contacts») de la
// persona sujeto y, la primera vez, pasa a ella lo que hubiera en el almacén de registros
// de esa carpeta (migrar.go). Quien llama se asegura de que nadie más escribe en Contacts
// mientras tanto: Contacts es de una sola copia y el Hub trae con las apps cerradas.
func Abrir(ctx context.Context, dir, sujeto string) (*Libreta, error) {
	return AbrirCon(ctx, dir, sujeto, Opciones{})
}

// Opciones de AbrirCon.
type Opciones struct {
	// Avance, si no es nil, dice cómo va el paso del almacén de antes a la base
	// (migrar.go): la fase y, al copiar, cuánto de cuánto. Sin almacén de antes, o con
	// la libreta ya migrada, no se llama.
	Avance func(Avance)
}

// AbrirCon es Abrir con sus opciones: Contacts enseña así cómo va la migración, que con
// 20.000 contactos tarda unos segundos (HayQueMigrar dice antes si la habrá).
func AbrirCon(ctx context.Context, dir, sujeto string, o Opciones) (*Libreta, error) {
	x, err := abrirBase(ctx, dir, Alcance(sujeto))
	if err != nil {
		return nil, err
	}
	if err := x.migrar(ctx, dir, o.Avance); err != nil {
		_ = x.Cerrar()
		return nil, err
	}
	return x, nil
}

// AbrirLectura abre en SOLO LECTURA la libreta de la carpeta dir para otra app (Mail,
// Calendar, el Hub): no crea nada, no escribe nada y lee lo último que Contacts confirmó
// mientras sigue escribiendo. ErrSinMigrar si aún no hay libreta (o no tiene lo de antes).
func AbrirLectura(ctx context.Context, dir, sujeto string) (*Libreta, error) {
	ruta := filepath.Join(dir, Fichero)
	if fi, err := os.Stat(ruta); err != nil || !fi.Mode().IsRegular() {
		return nil, ErrSinMigrar
	}
	leer, err := sql.Open("sqlite3", dsn(ruta, "mode=ro&_busy_timeout=5000"))
	if err != nil {
		return nil, err
	}
	leer.SetMaxOpenConns(2)
	x := &Libreta{ruta: ruta, scope: Alcance(sujeto), leer: leer, ahora: time.Now}
	if ok, err := x.migrada(ctx, leer); err != nil || !ok {
		_ = leer.Close()
		if err == nil || errors.Is(err, errSinEsquema) {
			err = ErrSinMigrar
		}
		return nil, err
	}
	return x, nil
}

// dsn es la dirección de SQLite del fichero ruta con esas opciones.
func dsn(ruta, opciones string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(ruta), RawQuery: opciones}).String()
}

// abrirBase abre (o crea) la base de dir y su esquema, sin migrar nada.
func abrirBase(ctx context.Context, dir string, scope storage.Scope) (*Libreta, error) {
	if !scope.Valid() || scope.Product != "contacts" {
		return nil, contacts.ErrSnapshot
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	ruta := filepath.Join(dir, Fichero)
	// Solo para la persona: SQLite crea el diario (-wal) y su índice (-shm) con los
	// permisos de la base, así que la base tiene que existir ya así antes de abrirla.
	f, err := os.OpenFile(ruta, os.O_RDWR|os.O_CREATE, 0o600) //nolint:gosec // G304: la carpeta de datos de la edición
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	if err := os.Chmod(ruta, 0o600); err != nil {
		slog.Debug("libreta: no se pudieron ajustar los permisos de la base", "error", err)
	}
	// secure_delete: lo que se borra (un contacto eliminado para siempre) se sobrescribe
	// con ceros en la base; el diario WAL se vacía después (vaciado.go).
	db, err := sql.Open("sqlite3", dsn(ruta, "_journal_mode=WAL&_busy_timeout=10000&_txlock=immediate&_synchronous=FULL&_secure_delete=true"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, esquema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("libreta: preparar la base: %w", err)
	}
	leer, err := sql.Open("sqlite3", dsn(ruta, "_busy_timeout=10000&_query_only=true"))
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	leer.SetMaxOpenConns(4)
	return &Libreta{ruta: ruta, scope: scope, db: db, leer: leer, ahora: time.Now}, nil
}

// Cerrar suelta la base. SQLite vuelca y borra su diario al cerrarse la última conexión
// (si otra app la tiene abierta, lo hará ella al cerrar).
func (x *Libreta) Cerrar() error {
	if x == nil {
		return nil
	}
	var errs []error
	if x.leer != nil {
		errs = append(errs, x.leer.Close())
	}
	if x.db != nil {
		// El escritor, el último: es el que vuelca el diario al cerrarse.
		x.vaciado.parar()
		x.mu.Lock() // que no quede ninguna escritura ni vaciado a medias
		errs = append(errs, x.db.Close())
		x.mu.Unlock()
	}
	return errors.Join(errs...)
}

// Close es Cerrar sin error, para quien cierra repositorios (`l.AlCerrar`).
func (x *Libreta) Close() { _ = x.Cerrar() }

// Ruta es el fichero de la base.
func (x *Libreta) Ruta() string { return x.ruta }

// CopiarEnCaliente deja en destino una copia coherente de la base abierta (`VACUUM
// INTO`), sin cerrarla: la copia de seguridad (`copia.BaseAbierta`).
func (x *Libreta) CopiarEnCaliente(ctx context.Context, destino string) error {
	db := x.db
	if db == nil {
		db = x.leer
	}
	_, err := db.ExecContext(ctx, "VACUUM INTO ?", destino)
	return err
}

// PrivateStorageScope es el alcance de los contactos (`api.ContactsData`).
func (x *Libreta) PrivateStorageScope() storage.Scope { return x.scope }

// RefreshPrivateData no tiene nada que hacer: cada consulta lee la base tal como está.
func (x *Libreta) RefreshPrivateData(context.Context) error { return nil }

// Revision es la de la base: sube con cada escritura. -1 si no se puede leer.
func (x *Libreta) Revision(ctx context.Context) int64 {
	var r int64
	if err := x.leer.QueryRowContext(ctx, `SELECT CAST(valor AS INTEGER) FROM meta WHERE clave = 'revision'`).Scan(&r); err != nil {
		return -1
	}
	return r
}

// escribir hace f en una transacción del escritor (BEGIN IMMEDIATE: una sola a la vez,
// también entre procesos) y sube la revisión. Si f falla, no queda nada.
func (x *Libreta) escribir(ctx context.Context, f func(*sql.Tx) error) error {
	if x.db == nil {
		return ErrSoloLectura
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	tx, err := x.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := f(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE meta SET valor = CAST(valor AS INTEGER) + 1 WHERE clave = 'revision'`); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// errorDe es el error de consulta de Contacts con ese estado y código.
func errorDe(status int, code string) error { return contacts.ErrorDeConsulta(status, code) }
