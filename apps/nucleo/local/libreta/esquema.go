// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package libreta

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// esquema es la base entera (libreta.go explica cada tabla). Lo que se ve de cada
// contacto («visible») es lo que no está en la papelera (borrado IS NULL) ni en una
// libreta borrada (oculto = 0): los índices parciales de abajo son los de esas consultas.
const esquema = `
CREATE TABLE IF NOT EXISTS meta (clave TEXT PRIMARY KEY, valor TEXT NOT NULL);
INSERT OR IGNORE INTO meta (clave, valor) VALUES ('esquema', '1'), ('revision', '0');

-- La libreta tal cual la entiende Contacts: una fila por libreta, tarjeta, contacto,
-- correo, teléfono, etiqueta y miembro, con su JSON entero y lo que la relaciona.
CREATE TABLE IF NOT EXISTS fila (
  clase    TEXT NOT NULL,              -- book, card, contact, email, phone, group, member
  id       TEXT NOT NULL,
  libreta  TEXT NOT NULL DEFAULT '',   -- address_book_id
  objeto   TEXT NOT NULL DEFAULT '',   -- object_id: la tarjeta de un contacto o etiqueta
  contacto TEXT NOT NULL DEFAULT '',   -- contact_id de un correo, teléfono o miembro
  grupo    TEXT NOT NULL DEFAULT '',   -- group_id de un miembro
  uid      TEXT NOT NULL DEFAULT '',   -- uid, o contact_uid de un miembro
  json     TEXT NOT NULL,
  UNIQUE (clase, id)
);
-- Sin WHERE: un índice parcial con «contacto <> ''» no le vale a «contacto = ?».
CREATE INDEX IF NOT EXISTS fila_contacto ON fila(contacto, clase);
CREATE INDEX IF NOT EXISTS fila_grupo ON fila(grupo, clase);

-- Lo que se ve de cada libreta, contacto y etiqueta, con sus índices.
CREATE TABLE IF NOT EXISTS libreta (
  id      TEXT PRIMARY KEY,
  nombre  TEXT NOT NULL,               -- name: la ruta CardDAV («sytue-traido»…)
  orden   BLOB NOT NULL,               -- contacts.ClaveDeLibreta
  borrada INTEGER NOT NULL DEFAULT 0,
  version INTEGER NOT NULL DEFAULT 0,  -- sube con cada cambio de lo suyo: la huella de sus páginas
  vista   TEXT NOT NULL                -- api.AddressBook
);
CREATE TABLE IF NOT EXISTS contacto (
  id         TEXT PRIMARY KEY,
  libreta    TEXT NOT NULL,
  uid        TEXT NOT NULL,
  uid_limpio TEXT NOT NULL,            -- sin «urn:uuid:» (card.CleanUID)
  objeto     TEXT NOT NULL,            -- su tarjeta (fila «card»)
  orden      BLOB NOT NULL,            -- contacts.ClaveDeOrden
  buscable   TEXT NOT NULL,            -- contacts.TextoBuscable: sin acentos ni mayúsculas
  favorito   INTEGER NOT NULL DEFAULT 0,
  borrado    TEXT,                     -- en la papelera desde entonces (UTC, ancho fijo); NULL, no
  oculto     INTEGER NOT NULL DEFAULT 0, -- su libreta está borrada
  cumple     INTEGER NOT NULL DEFAULT 0, -- tiene cumpleaños (con año o sin él)
  vista      TEXT NOT NULL             -- api.Contact
);
CREATE INDEX IF NOT EXISTS contacto_vivos ON contacto(orden, buscable) WHERE borrado IS NULL AND oculto = 0;
CREATE INDEX IF NOT EXISTS contacto_libreta ON contacto(libreta, orden) WHERE borrado IS NULL AND oculto = 0;
CREATE INDEX IF NOT EXISTS contacto_papelera ON contacto(borrado, id) WHERE borrado IS NOT NULL AND oculto = 0;
CREATE INDEX IF NOT EXISTS contacto_favoritos ON contacto(orden) WHERE favorito = 1 AND borrado IS NULL AND oculto = 0;
CREATE INDEX IF NOT EXISTS contacto_cumple ON contacto(orden) WHERE cumple = 1 AND borrado IS NULL AND oculto = 0;
CREATE INDEX IF NOT EXISTS contacto_uid ON contacto(uid, libreta);
CREATE INDEX IF NOT EXISTS contacto_uid_limpio ON contacto(uid_limpio);
CREATE INDEX IF NOT EXISTS contacto_de ON contacto(libreta);
CREATE TABLE IF NOT EXISTS correo (
  contacto  TEXT NOT NULL,
  libreta   TEXT NOT NULL,
  direccion TEXT NOT NULL              -- en minúsculas y sin espacios alrededor
);
CREATE INDEX IF NOT EXISTS correo_direccion ON correo(libreta, direccion);
CREATE INDEX IF NOT EXISTS correo_contacto ON correo(contacto);
CREATE TABLE IF NOT EXISTS grupo (
  id         TEXT PRIMARY KEY,
  libreta    TEXT NOT NULL,
  uid_limpio TEXT NOT NULL,
  objeto     TEXT NOT NULL,
  orden      BLOB NOT NULL,            -- contacts.ClaveDeGrupo
  borrado    INTEGER NOT NULL DEFAULT 0,
  oculto     INTEGER NOT NULL DEFAULT 0,
  vista      TEXT NOT NULL             -- api.ContactGroup, con sus miembros
);
CREATE INDEX IF NOT EXISTS grupo_libreta ON grupo(libreta, orden);
CREATE INDEX IF NOT EXISTS grupo_uid ON grupo(libreta, uid_limpio);

-- El índice de favoritos de Contacts, por UID.
CREATE TABLE IF NOT EXISTS favorito (uid TEXT PRIMARY KEY, valor INTEGER NOT NULL);
`

// errSinEsquema: el fichero existe pero no tiene la tabla meta (otra cosa, o a medio crear).
var errSinEsquema = errors.New("libreta: la base no tiene esquema")

// claveMigrada es la marca de la migración en meta: su valor dice de dónde y cuánto.
const claveMigrada = "migrada"

// consulta es lo que tienen en común una base y una transacción para leer.
type consulta interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// migrada dice si la base ya tiene lo del almacén de antes (o se comprobó que no había).
func (x *Libreta) migrada(ctx context.Context, q consulta) (bool, error) {
	var valor string
	err := q.QueryRowContext(ctx, `SELECT valor FROM meta WHERE clave = ?`, claveMigrada).Scan(&valor)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil && strings.Contains(err.Error(), "no such table"):
		return false, errSinEsquema
	case err != nil:
		return false, err
	}
	return true, nil
}
