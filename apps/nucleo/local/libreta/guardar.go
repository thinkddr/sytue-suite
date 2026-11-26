// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package libreta

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/card"
)

// # Guardar lo que cambia
//
// Una escritura compone su cambio sobre la parte que leyó (antes) y da otra instantánea
// (despues), validada como Contacts valida la libreta: sus libretas, contactos y
// etiquetas se proyectan sin error. Guardar es escribir la diferencia, fila a fila (las
// que ya no están se borran; las nuevas o con otro JSON se ponen), y rehacer lo que se
// ve de cada libreta, contacto y etiqueta tocados con lo que Contacts proyecta de ellos
// en despues. Lo que no se leyó no cambia.

// marcaBorrado es el formato de `contacto.borrado`: ANCHO FIJO y en UTC, para que ordenar
// el texto sea ordenar el tiempo.
const marcaBorrado = "2006-01-02T15:04:05.000000000Z"

// vistas son las tres proyecciones de una instantánea: validarlas es la condición para
// guardarla, como en Contacts (`validateViews`).
type vistas struct {
	libros    []contacts.BookView
	contactos []contacts.ContactView
	grupos    []contacts.GroupView
}

func proyectar(s *contacts.Snapshot) (vistas, error) {
	var v vistas
	var err error
	if v.libros, err = s.Books(); err != nil {
		return v, err
	}
	if v.contactos, err = s.Contacts(); err != nil {
		return v, err
	}
	v.grupos, err = s.Groups()
	return v, err
}

// tocados son las libretas, contactos y etiquetas con alguna fila cambiada.
type tocados struct {
	libretas, contactos, grupos map[string]bool
}

// guardar escribe en tx lo que cambia de antes a despues y lo que se ve de lo tocado.
func (x *Libreta) guardar(ctx context.Context, tx *sql.Tx, antes, despues *contacts.Snapshot) error {
	v, err := proyectar(despues)
	if err != nil {
		return err
	}
	viejas, nuevas := filasPorClave(antes), filasPorClave(despues)
	duenos := map[string]string{} // tarjeta → clave del contacto o etiqueta
	for _, filas := range []map[string]contacts.Row{viejas, nuevas} {
		for _, r := range filas {
			if r.Kind == "contact" || r.Kind == "group" {
				if c, err := columnasDe(r); err == nil && c.objeto != "" {
					duenos[c.objeto] = clave(r.Kind, r.ID)
				}
			}
		}
	}
	t := tocados{libretas: map[string]bool{}, contactos: map[string]bool{}, grupos: map[string]bool{}}
	for k, r := range viejas {
		if _, sigue := nuevas[k]; sigue {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM fila WHERE clase = ? AND id = ?`, r.Kind, r.ID); err != nil {
			return err
		}
		t.marcar(r, duenos)
	}
	for k, r := range nuevas {
		if vieja, estaba := viejas[k]; estaba && bytes.Equal(vieja.JSON, r.JSON) {
			continue
		}
		if err := ponerFila(ctx, tx, r); err != nil {
			return err
		}
		t.marcar(r, duenos)
		if vieja, estaba := viejas[k]; estaba {
			t.marcar(vieja, duenos)
		}
	}
	return x.ponerVistas(ctx, tx, v, t)
}

func filasPorClave(s *contacts.Snapshot) map[string]contacts.Row {
	out := map[string]contacts.Row{}
	s.EachRow(func(r contacts.Row) bool {
		out[clave(r.Kind, r.ID)] = r
		return true
	})
	return out
}

// marcar apunta lo que toca la fila r: su libreta, su contacto o su etiqueta.
func (t tocados) marcar(r contacts.Row, duenos map[string]string) {
	c, _ := columnasDe(r)
	switch r.Kind {
	case "book":
		t.libretas[r.ID] = true
	case "contact":
		t.contactos[r.ID] = true
	case "email", "phone":
		t.contactos[c.contacto] = true
	case "group":
		t.grupos[r.ID] = true
	case "member":
		t.grupos[c.grupo] = true
	case "card":
		if dueno, ok := duenos[r.ID]; ok {
			kind, id, _ := strings.Cut(dueno, "\x00")
			if kind == "contact" {
				t.contactos[id] = true
			} else {
				t.grupos[id] = true
			}
		}
	}
}

// columnas son las de una fila en la tabla fila.
type columnas struct {
	libreta, objeto, contacto, grupo, uid string
}

func columnasDe(r contacts.Row) (columnas, error) {
	var v struct {
		Book       string  `json:"address_book_id"`
		Object     string  `json:"object_id"`
		Contact    *string `json:"contact_id"`
		Group      string  `json:"group_id"`
		UID        string  `json:"uid"`
		ContactUID string  `json:"contact_uid"`
	}
	if err := json.Unmarshal(r.JSON, &v); err != nil {
		return columnas{}, contacts.ErrSnapshot
	}
	c := columnas{libreta: v.Book, objeto: v.Object, grupo: v.Group, uid: v.UID}
	if v.Contact != nil {
		c.contacto = *v.Contact
	}
	if r.Kind == "member" {
		c.uid = v.ContactUID
	}
	return c, nil
}

// ejecutor es una transacción o una sentencia preparada de la migración.
type ejecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

const sqlFila = `INSERT INTO fila (clase, id, libreta, objeto, contacto, grupo, uid, json)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT (clase, id) DO UPDATE SET libreta = excluded.libreta, objeto = excluded.objeto,
	contacto = excluded.contacto, grupo = excluded.grupo, uid = excluded.uid, json = excluded.json`

// ponerFila guarda r en la tabla fila (nueva, o en lugar de la que tenga su clase e id).
func ponerFila(ctx context.Context, tx ejecutor, r contacts.Row) error {
	c, err := columnasDe(r)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, sqlFila, r.Kind, r.ID, c.libreta, c.objeto, c.contacto, c.grupo, c.uid, string(r.JSON))
	return err
}

// ponerVistas rehace en tx lo que se ve de lo tocado, con las vistas de despues, y sube
// la versión de cada libreta con algo cambiado (la huella de sus páginas).
func (x *Libreta) ponerVistas(ctx context.Context, tx *sql.Tx, v vistas, t tocados) error {
	versiones := map[string]bool{}
	if err := ponerLibretas(ctx, tx, v.libros, t.libretas, versiones); err != nil {
		return err
	}
	if err := ponerContactos(ctx, tx, v.contactos, t.contactos, versiones); err != nil {
		return err
	}
	if err := ponerGrupos(ctx, tx, v.grupos, t.grupos); err != nil {
		return err
	}
	for id := range versiones {
		if _, err := tx.ExecContext(ctx, `UPDATE libreta SET version = version + 1 WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

// ponerLibretas rehace las libretas tocadas; lo de una que se borra deja de verse.
func ponerLibretas(ctx context.Context, tx *sql.Tx, libros []contacts.BookView, tocadas, versiones map[string]bool) error {
	for _, b := range libros {
		if !tocadas[b.ID] {
			continue
		}
		var antes sql.NullInt64
		if err := tx.QueryRowContext(ctx, `SELECT borrada FROM libreta WHERE id = ?`, b.ID).Scan(&antes); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := ponerLibreta(ctx, tx, b); err != nil {
			return err
		}
		if borrada := b.DeletedAt != nil; !antes.Valid || (antes.Int64 == 1) != borrada {
			// Lo de una libreta borrada no se ve (ni en la papelera).
			for _, tabla := range []string{"contacto", "grupo"} {
				//nolint:gosec // G202: tabla es una de las dos de arriba
				if _, err := tx.ExecContext(ctx, `UPDATE `+tabla+` SET oculto = ? WHERE libreta = ?`, borrada, b.ID); err != nil {
					return err
				}
			}
		}
		versiones[b.ID] = true
	}
	return nil
}

// ponerContactos rehace los contactos tocados (y quita los que ya no están).
func ponerContactos(ctx context.Context, tx *sql.Tx, vistos []contacts.ContactView, tocados, versiones map[string]bool) error {
	porID := make(map[string]*contacts.ContactView, len(vistos))
	for i := range vistos {
		porID[vistos[i].ID] = &vistos[i]
	}
	for id := range tocados {
		var antes string
		if err := tx.QueryRowContext(ctx, `SELECT libreta FROM contacto WHERE id = ?`, id).Scan(&antes); err == nil {
			versiones[antes] = true
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM correo WHERE contacto = ?`, id); err != nil {
			return err
		}
		c, sigue := porID[id]
		if !sigue {
			if _, err := tx.ExecContext(ctx, `DELETE FROM contacto WHERE id = ?`, id); err != nil {
				return err
			}
			continue
		}
		if err := ponerContacto(ctx, tx, c); err != nil {
			return err
		}
		versiones[c.AddressBookID] = true
	}
	return nil
}

// ponerGrupos rehace las etiquetas tocadas (y quita las que ya no están).
func ponerGrupos(ctx context.Context, tx *sql.Tx, vistos []contacts.GroupView, tocados map[string]bool) error {
	porID := make(map[string]*contacts.GroupView, len(vistos))
	for i := range vistos {
		porID[vistos[i].ID] = &vistos[i]
	}
	for id := range tocados {
		g, sigue := porID[id]
		if !sigue {
			if _, err := tx.ExecContext(ctx, `DELETE FROM grupo WHERE id = ?`, id); err != nil {
				return err
			}
			continue
		}
		if err := ponerGrupo(ctx, tx, g); err != nil {
			return err
		}
	}
	return nil
}

func ponerLibreta(ctx context.Context, tx ejecutor, b contacts.BookView) error {
	vista, err := json.Marshal(b.AddressBook)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO libreta (id, nombre, orden, borrada, vista) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET nombre = excluded.nombre, orden = excluded.orden, borrada = excluded.borrada, vista = excluded.vista`,
		b.ID, b.Name, ordenDe(contacts.ClaveDeLibreta(b.AddressBook)), b.DeletedAt != nil, string(vista))
	return err
}

func ponerContacto(ctx context.Context, tx ejecutor, c *contacts.ContactView) error {
	vista, err := json.Marshal(c.Contact)
	if err != nil {
		return err
	}
	var borrado any
	if c.DeletedAt != nil {
		borrado = c.DeletedAt.UTC().Format(marcaBorrado)
	}
	cumple := c.Birthday != nil || c.BirthdayNoYear != ""
	_, err = tx.ExecContext(ctx, `INSERT INTO contacto (id, libreta, uid, uid_limpio, objeto, orden, buscable, favorito, borrado, oculto, cumple, vista)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET libreta = excluded.libreta, uid = excluded.uid, uid_limpio = excluded.uid_limpio,
		objeto = excluded.objeto, orden = excluded.orden, buscable = excluded.buscable, favorito = excluded.favorito,
		borrado = excluded.borrado, oculto = excluded.oculto, cumple = excluded.cumple, vista = excluded.vista`,
		c.ID, c.AddressBookID, c.UID, card.CleanUID(c.UID), c.ObjectID, ordenDe(contacts.ClaveDeOrden(&c.Contact)),
		contacts.TextoBuscable(&c.Contact), c.Favorite, borrado, c.BookDeleted, cumple, string(vista))
	if err != nil {
		return err
	}
	for _, e := range c.Emails {
		if _, err := tx.ExecContext(ctx, `INSERT INTO correo (contacto, libreta, direccion) VALUES (?, ?, ?)`,
			c.ID, c.AddressBookID, direccionDe(e)); err != nil {
			return err
		}
	}
	return nil
}

// direccionDe es un correo como lo compara importar un CSV: en minúsculas y sin espacios
// alrededor.
func direccionDe(e api.ContactEmail) string { return strings.ToLower(strings.TrimSpace(e.Email)) }

func ponerGrupo(ctx context.Context, tx ejecutor, g *contacts.GroupView) error {
	vista, err := json.Marshal(g.ContactGroup)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO grupo (id, libreta, uid_limpio, objeto, orden, borrado, oculto, vista)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET libreta = excluded.libreta, uid_limpio = excluded.uid_limpio, objeto = excluded.objeto,
		orden = excluded.orden, borrado = excluded.borrado, oculto = excluded.oculto, vista = excluded.vista`,
		g.ID, g.AddressBookID, card.CleanUID(g.UID), g.ObjectID, ordenDe(contacts.ClaveDeGrupo(g.ContactGroup)),
		g.DeletedAt != nil, g.BookDeleted, string(vista))
	return err
}

// ordenDe es una clave de orden de Contacts como BLOB: comparar los bytes (lo que hace
// SQLite) es comparar la clave campo a campo, como `strings.Compare` en cada uno. Cada
// campo lleva sus ceros como 0x00 0xFF y termina en 0x00 0x01.
func ordenDe[K ~[2]string | ~[3]string | ~[4]string](k K) []byte {
	var b bytes.Buffer
	for i := range len(k) {
		for j := 0; j < len(k[i]); j++ {
			if k[i][j] == 0 {
				b.Write([]byte{0, 0xFF})
				continue
			}
			b.WriteByte(k[i][j])
		}
		b.Write([]byte{0, 1})
	}
	return b.Bytes()
}

// borradoDe es la hora de `contacto.borrado`.
func borradoDe(s string) (time.Time, error) { return time.Parse(marcaBorrado, s) }
