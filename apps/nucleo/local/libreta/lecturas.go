// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package libreta

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/card"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Las consultas leen lo que se ve (las tablas libreta, contacto y grupo) por índice y
// dan lo mismo que las del repositorio de Contacts sobre la libreta entera: los mismos
// filtros (ni la papelera ni las libretas borradas), el mismo orden (las claves de orden
// de Contacts, guardadas en cada fila) y los mismos topes y errores.

// Las consultas, cada una por su índice (TestLasConsultasVanPorIndice).
const (
	sqlTodos      = `SELECT vista FROM contacto WHERE ` + vivos + ` ORDER BY orden LIMIT ? OFFSET ?`
	sqlContactos  = `SELECT vista FROM contacto WHERE ` + vivos + ` ORDER BY orden`
	sqlCumpleanos = `SELECT vista FROM contacto WHERE cumple = 1 AND ` + vivos + ` ORDER BY orden`
	// La búsqueda recorre en orden el índice de los que se ven, que lleva el texto que
	// mira (sin leer la tabla de los que no casan), y para en cuanto tiene los que pide.
	sqlBuscar     = `SELECT vista FROM contacto INDEXED BY contacto_vivos WHERE ` + vivos + ` AND instr(buscable, ?) > 0 ORDER BY orden LIMIT ?`
	sqlConTarjeta = `SELECT c.vista, f.json FROM contacto c JOIN fila f ON f.clase = 'card' AND f.id = c.objeto
		WHERE c.id = ? AND c.borrado IS NULL AND c.oculto = 0`
	sqlFotoPorUID = `SELECT f.json FROM contacto c JOIN fila f ON f.clase = 'card' AND f.id = c.objeto
		WHERE c.uid_limpio = ? AND c.borrado IS NULL AND c.oculto = 0 ORDER BY c.orden LIMIT 2`
	sqlFavoritos = `SELECT uid FROM contacto WHERE favorito = 1 AND ` + vivos + ` ORDER BY orden`
	sqlTarjetas  = `SELECT c.id, c.uid, f.json FROM contacto c JOIN fila f ON f.clase = 'card' AND f.id = c.objeto
		WHERE c.libreta = ? AND c.borrado IS NULL AND c.oculto = 0 ORDER BY c.orden`
	sqlPapelera = `SELECT vista, borrado FROM contacto WHERE borrado IS NOT NULL AND oculto = 0
		ORDER BY borrado DESC, id DESC LIMIT ?`
	sqlAncla       = `SELECT orden FROM contacto WHERE id = ? AND libreta = ? AND ` + vivos
	sqlPagina      = `SELECT vista FROM contacto WHERE libreta = ? AND ` + vivos + ` ORDER BY orden LIMIT ?`
	sqlPaginaDesde = `SELECT vista FROM contacto WHERE libreta = ? AND ` + vivos + ` AND orden > ? ORDER BY orden LIMIT ?`
)

func noEncontrado() error { return errorDe(http.StatusNotFound, "private_contact_not_found") }

func libretaNoEncontrada() error {
	return errorDe(http.StatusNotFound, "private_address_book_not_found")
}

// contactosDe lee la columna vista (un api.Contact) de cada fila de rs, y lo cierra.
func contactosDe(rs *sql.Rows) ([]api.Contact, error) {
	defer func() { _ = rs.Close() }()
	out := []api.Contact{}
	for rs.Next() {
		var vista string
		if err := rs.Scan(&vista); err != nil {
			return nil, err
		}
		var c api.Contact
		if err := json.Unmarshal([]byte(vista), &c); err != nil {
			return nil, contacts.ErrSnapshot
		}
		out = append(out, c)
	}
	return out, rs.Err()
}

// datosDe es la vCard de una fila «card».
func datosDe(fila string) (string, error) {
	var v struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal([]byte(fila), &v); err != nil {
		return "", contacts.ErrSnapshot
	}
	return v.Data, nil
}

// AddressBooks son las libretas que se ven, la predeterminada primero.
func (x *Libreta) AddressBooks(ctx context.Context) ([]api.AddressBook, error) {
	rs, err := x.leer.QueryContext(ctx, `SELECT vista FROM libreta WHERE borrada = 0 ORDER BY orden`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rs.Close() }()
	out := []api.AddressBook{}
	for rs.Next() {
		var vista string
		if err := rs.Scan(&vista); err != nil {
			return nil, err
		}
		var b api.AddressBook
		if err := json.Unmarshal([]byte(vista), &b); err != nil {
			return nil, contacts.ErrSnapshot
		}
		out = append(out, b)
	}
	return out, rs.Err()
}

// ExisteLibreta dice si la libreta sigue (no se borró).
func (x *Libreta) ExisteLibreta(ctx context.Context, id string) (bool, error) {
	var uno int
	err := x.leer.QueryRowContext(ctx, `SELECT 1 FROM libreta WHERE id = ? AND borrada = 0`, id).Scan(&uno)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// All es una página de los contactos que se ven, en orden: hasta limit desde offset y el
// desplazamiento de la siguiente, o 0 si no hay más.
func (x *Libreta) All(ctx context.Context, limit, offset int) ([]api.Contact, int, error) {
	limit = contacts.LimitePagina(limit, 100)
	offset = max(0, offset)
	rs, err := x.leer.QueryContext(ctx, sqlTodos, limit+1, offset)
	if err != nil {
		return nil, 0, err
	}
	cs, err := contactosDe(rs)
	if err != nil {
		return nil, 0, err
	}
	if len(cs) > limit {
		return cs[:limit], offset + limit, nil
	}
	return cs, 0, nil
}

// Contactos son todos los contactos que se ven, en orden (los de All, de una vez): para
// quien lee la libreta entera (los cumpleaños de Calendar, el Hub, exportar).
func (x *Libreta) Contactos(ctx context.Context) ([]api.Contact, error) {
	rs, err := x.leer.QueryContext(ctx, sqlContactos)
	if err != nil {
		return nil, err
	}
	return contactosDe(rs)
}

// ConCumpleanos son los contactos que se ven con cumpleaños (con año o sin él), en orden.
func (x *Libreta) ConCumpleanos(ctx context.Context) ([]api.Contact, error) {
	rs, err := x.leer.QueryContext(ctx, sqlCumpleanos)
	if err != nil {
		return nil, err
	}
	return contactosDe(rs)
}

// Search son los contactos que se ven cuyo nombre, apellidos, organización, apodo, nota,
// correos o teléfonos llevan text, sin acentos ni mayúsculas, en orden y hasta limit.
func (x *Libreta) Search(ctx context.Context, text string, limit int) ([]api.Contact, error) {
	query := buscar.Plegar(strings.TrimSpace(text))
	if query == "" {
		return nil, errorDe(http.StatusBadRequest, "contact_query_required")
	}
	rs, err := x.leer.QueryContext(ctx, sqlBuscar, query, contacts.LimitePagina(limit, 50))
	if err != nil {
		return nil, err
	}
	return contactosDe(rs)
}

// Contact es el contacto id, si se ve.
func (x *Libreta) Contact(ctx context.Context, id string) (api.Contact, error) {
	c, _, err := x.contactoConTarjeta(ctx, id)
	return c, err
}

// contactoConTarjeta es el contacto id, si se ve, con su vCard.
func (x *Libreta) contactoConTarjeta(ctx context.Context, id string) (api.Contact, string, error) {
	var vista, fila string
	err := x.leer.QueryRowContext(ctx, sqlConTarjeta, id).Scan(&vista, &fila)
	if errors.Is(err, sql.ErrNoRows) {
		return api.Contact{}, "", noEncontrado()
	}
	if err != nil {
		return api.Contact{}, "", err
	}
	var c api.Contact
	if err := json.Unmarshal([]byte(vista), &c); err != nil {
		return api.Contact{}, "", contacts.ErrSnapshot
	}
	vcard, err := datosDe(fila)
	return c, vcard, err
}

// VCard es la vCard guardada del contacto id.
func (x *Libreta) VCard(ctx context.Context, id string) ([]byte, error) {
	_, vcard, err := x.contactoConTarjeta(ctx, id)
	if err != nil {
		return nil, err
	}
	return []byte(vcard), nil
}

// Photo es la foto del contacto de ese UID; si hay más de uno que se ve con él, ninguno.
func (x *Libreta) Photo(ctx context.Context, uid string) ([]byte, error) {
	uid = card.CleanUID(uid)
	if uid == "" {
		return nil, noEncontrado()
	}
	rs, err := x.leer.QueryContext(ctx, sqlFotoPorUID, uid)
	if err != nil {
		return nil, err
	}
	filas, err := columna(rs)
	switch {
	case err != nil:
		return nil, err
	case len(filas) > 1:
		return nil, errorDe(http.StatusConflict, "private_contact_uid_ambiguous")
	case len(filas) == 0:
		return nil, noEncontrado()
	}
	vcard, err := datosDe(filas[0])
	if err != nil {
		return nil, err
	}
	return contacts.FotoDeVCard(vcard)
}

// PhotoByID es la foto del contacto id si su tarjeta sigue con la etiqueta etag.
func (x *Libreta) PhotoByID(ctx context.Context, id, etag string) ([]byte, error) {
	if strings.TrimSpace(etag) == "" {
		return nil, errorDe(http.StatusBadRequest, "contact_etag_required")
	}
	c, vcard, err := x.contactoConTarjeta(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.ETag != etag {
		return nil, errorDe(http.StatusPreconditionFailed, "contact_etag_changed")
	}
	return contacts.FotoDeVCard(vcard)
}

// Favorites son los UID de los contactos destacados que se ven, sin repetir, en orden.
func (x *Libreta) Favorites(ctx context.Context) ([]string, error) {
	rs, err := x.leer.QueryContext(ctx, sqlFavoritos)
	if err != nil {
		return nil, err
	}
	uids, err := columna(rs)
	if err != nil {
		return nil, err
	}
	out := []string{}
	visto := map[string]bool{}
	for _, uid := range uids {
		if !visto[uid] {
			visto[uid] = true
			out = append(out, uid)
		}
	}
	return out, nil
}

// grupoLeido es una etiqueta que se ve con su vCard.
type grupoLeido struct {
	api.ContactGroup
	VCard string
}

// gruposDonde son las etiquetas que se ven y cumplen donde, en orden, con su vCard.
func (x *Libreta) gruposVisibles(ctx context.Context, donde string, args ...any) ([]grupoLeido, error) {
	//nolint:gosec // G202: donde son condiciones fijas de este paquete, con `?`; los valores van en args
	rs, err := x.leer.QueryContext(ctx, `SELECT g.vista, f.json FROM grupo g JOIN fila f ON f.clase = 'card' AND f.id = g.objeto
		WHERE g.borrado = 0 AND g.oculto = 0 AND `+donde+` ORDER BY g.orden`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rs.Close() }()
	var out []grupoLeido
	for rs.Next() {
		var vista, fila string
		if err := rs.Scan(&vista, &fila); err != nil {
			return nil, err
		}
		var g grupoLeido
		if err := json.Unmarshal([]byte(vista), &g.ContactGroup); err != nil {
			return nil, contacts.ErrSnapshot
		}
		if g.VCard, err = datosDe(fila); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rs.Err()
}

// grupoVisible es la etiqueta id, si se ve.
func (x *Libreta) grupoVisible(ctx context.Context, id string) (grupoLeido, error) {
	gs, err := x.gruposVisibles(ctx, `g.id = ?`, id)
	if err != nil {
		return grupoLeido{}, err
	}
	if len(gs) == 0 {
		return grupoLeido{}, errorDe(http.StatusNotFound, "private_group_not_found")
	}
	return gs[0], nil
}

// Groups son las etiquetas de una libreta que se ve.
func (x *Libreta) Groups(ctx context.Context, book string) ([]api.ContactGroup, error) {
	if ok, err := x.ExisteLibreta(ctx, book); err != nil {
		return nil, err
	} else if !ok {
		return nil, libretaNoEncontrada()
	}
	gs, err := x.gruposVisibles(ctx, `g.libreta = ?`, book)
	if err != nil {
		return nil, err
	}
	out := make([]api.ContactGroup, 0, len(gs))
	for _, g := range gs {
		out = append(out, g.ContactGroup)
	}
	return out, nil
}

// GruposCrudos son las etiquetas vivas de una libreta como vCard de grupo.
func (x *Libreta) GruposCrudos(ctx context.Context, libreta string) ([]contacts.TarjetaCruda, error) {
	gs, err := x.gruposVisibles(ctx, `g.libreta = ?`, libreta)
	if err != nil {
		return nil, err
	}
	var out []contacts.TarjetaCruda
	for _, g := range gs {
		out = append(out, contacts.TarjetaCruda{ID: g.ID, UID: g.UID, Datos: g.VCard})
	}
	return out, nil
}

// TarjetasCrudas son los contactos que se ven de una libreta, en orden, como vCard.
func (x *Libreta) TarjetasCrudas(ctx context.Context, libreta string) ([]contacts.TarjetaCruda, error) {
	rs, err := x.leer.QueryContext(ctx, sqlTarjetas, libreta)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rs.Close() }()
	var out []contacts.TarjetaCruda
	for rs.Next() {
		var t contacts.TarjetaCruda
		var fila string
		if err := rs.Scan(&t.ID, &t.UID, &fila); err != nil {
			return nil, err
		}
		if t.Datos, err = datosDe(fila); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rs.Err()
}

// Trash son los contactos de la papelera (de libretas que se ven), el último tirado
// primero, hasta limit.
func (x *Libreta) Trash(ctx context.Context, limit int) ([]api.TrashItem, error) {
	rs, err := x.leer.QueryContext(ctx, sqlPapelera, contacts.LimitePagina(limit, 200))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rs.Close() }()
	out := []api.TrashItem{}
	for rs.Next() {
		var vista, borrado string
		if err := rs.Scan(&vista, &borrado); err != nil {
			return nil, err
		}
		var c api.Contact
		if err := json.Unmarshal([]byte(vista), &c); err != nil {
			return nil, contacts.ErrSnapshot
		}
		cuando, err := borradoDe(borrado)
		if err != nil {
			return nil, contacts.ErrSnapshot
		}
		out = append(out, api.TrashItem{ID: c.ID, UID: c.UID, FormattedName: c.FormattedName, Organization: c.Organization, PrimaryEmail: contacts.CorreoPrincipal(c.Emails), DeletedAt: cuando})
	}
	return out, rs.Err()
}

// ExportCSV es la libreta entera (lo que se ve) en el CSV de Google Contacts.
func (x *Libreta) ExportCSV(ctx context.Context) (string, []byte, error) {
	cs, err := x.Contactos(ctx)
	if err != nil {
		return "", nil, err
	}
	return contacts.CSVDeContactos(cs)
}

// BatchVCard junta en un `.vcf` las vCard de esos contactos, por id y en el orden pedido;
// los que no se ven o no existen no vienen.
func (x *Libreta) BatchVCard(ctx context.Context, ids []string) ([]byte, error) {
	if len(ids) > api.MaxLoteContactos {
		return nil, errorDe(http.StatusBadRequest, "contact_batch_too_large")
	}
	porID := map[string]string{}
	err := enTrozos(ids, func(trozo []string) error {
		//nolint:gosec // G202: solo las marcas `?` del IN; los valores van en args
		rs, err := x.leer.QueryContext(ctx, `SELECT c.id, f.json FROM contacto c JOIN fila f ON f.clase = 'card' AND f.id = c.objeto
			WHERE c.id IN (`+marcas(len(trozo))+`) AND c.borrado IS NULL AND c.oculto = 0`, argumentos(trozo)...)
		if err != nil {
			return err
		}
		defer func() { _ = rs.Close() }()
		for rs.Next() {
			var id, fila string
			if err := rs.Scan(&id, &fila); err != nil {
				return err
			}
			if porID[id], err = datosDe(fila); err != nil {
				return err
			}
		}
		return rs.Err()
	})
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	visto := map[string]bool{}
	for _, id := range ids {
		tarjeta, ok := porID[id]
		if !ok || visto[id] || tarjeta == "" {
			continue
		}
		visto[id] = true
		b.WriteString(tarjeta)
		if !strings.HasSuffix(tarjeta, "\n") {
			b.WriteString("\r\n")
		}
	}
	return []byte(b.String()), nil
}

// InBookPage es una página (100) de los contactos que se ven de una libreta, en orden. El
// cursor de la siguiente lleva el último de esta y la huella de la libreta: si la libreta
// cambia entre dos páginas, la siguiente es un conflicto (409) en vez de una lista a
// medias. La huella es la de su versión (sube con cada cambio de lo suyo, guardar.go), así
// que pedir una página no lee la libreta entera.
func (x *Libreta) InBookPage(ctx context.Context, book, cursor string) (api.ContactPage, error) {
	tx, err := x.leer.BeginTx(ctx, nil)
	if err != nil {
		return api.ContactPage{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var version int64
	err = tx.QueryRowContext(ctx, `SELECT version FROM libreta WHERE id = ? AND borrada = 0`, book).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return api.ContactPage{}, libretaNoEncontrada()
	}
	if err != nil {
		return api.ContactPage{}, err
	}
	huella, err := huellaDePaginas(x.scope, book, version)
	if err != nil {
		return api.ContactPage{}, err
	}
	var desde []byte
	if cursor != "" {
		id, digest, found := strings.Cut(cursor, ".")
		if !found || len(digest) != 64 {
			return api.ContactPage{}, errorDe(http.StatusBadRequest, "invalid_contact_cursor")
		}
		if digest != huella {
			return api.ContactPage{}, errorDe(http.StatusConflict, "private_contacts_changed")
		}
		err := tx.QueryRowContext(ctx, sqlAncla, id, book).Scan(&desde)
		if errors.Is(err, sql.ErrNoRows) {
			return api.ContactPage{}, errorDe(http.StatusConflict, "private_contacts_changed")
		}
		if err != nil {
			return api.ContactPage{}, err
		}
	}
	const pagina = 100
	q, args := sqlPagina, []any{book, pagina + 1}
	if desde != nil {
		q, args = sqlPaginaDesde, []any{book, desde, pagina + 1}
	}
	rs, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return api.ContactPage{}, err
	}
	cs, err := contactosDe(rs)
	if err != nil {
		return api.ContactPage{}, err
	}
	out := api.ContactPage{Contacts: cs}
	if len(cs) > pagina {
		out.Contacts = cs[:pagina]
		out.NextCursor = cs[pagina-1].ID + "." + huella
	}
	return out, nil
}

// huellaDePaginas es la huella pública de las páginas de una libreta en una versión.
func huellaDePaginas(scope storage.Scope, book string, version int64) (string, error) {
	cabecera, err := storage.EncodeScopeHeader(scope)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte("sytue-libreta-paginas-v1\x00" + cabecera + "\x00" + book + "\x00" + strconv.FormatInt(version, 10)))
	return hex.EncodeToString(sum[:]), nil
}
