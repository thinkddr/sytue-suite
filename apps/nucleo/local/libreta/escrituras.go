// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package libreta

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/card"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/model"
)

// Cada escritura dice qué parte de la libreta lee (cargar, además de las libretas) y con
// qué función de Contacts prepara su cambio (parte.go). Lo que cada una necesita es lo
// que mira esa función: el contacto o la etiqueta que cambia, y los que podrían chocar
// con él o con ella por su UID o sus correos.

// vivos es la condición de un contacto que se ve, en la tabla contacto.
const vivos = `borrado IS NULL AND oculto = 0`

// agregado lee la parte de la libreta de cargar, prepara el cambio con preparar, lo
// compone y lo guarda, en una transacción. Es lo que el repositorio de Contacts hace en
// cada escritura de un agregado (`mutateAggregate`).
func (x *Libreta) agregado(ctx context.Context, cargar func(*parte) error, preparar func(*contacts.Snapshot, time.Time) (contacts.Aggregate, error)) (contacts.Aggregate, error) {
	var change contacts.Aggregate
	err := x.escribir(ctx, func(tx *sql.Tx) error {
		var err error
		change, err = x.agregadoEn(ctx, tx, cargar, preparar)
		return err
	})
	return change, err
}

func (x *Libreta) agregadoEn(ctx context.Context, tx *sql.Tx, cargar func(*parte) error, preparar func(*contacts.Snapshot, time.Time) (contacts.Aggregate, error)) (contacts.Aggregate, error) {
	p := nuevaParte(ctx, tx)
	if err := p.libretas(); err != nil {
		return contacts.Aggregate{}, err
	}
	if cargar != nil {
		if err := cargar(p); err != nil {
			return contacts.Aggregate{}, err
		}
	}
	antes, err := p.instantanea(x)
	if err != nil {
		return contacts.Aggregate{}, err
	}
	change, err := preparar(antes, x.ahora().UTC())
	if err != nil {
		return contacts.Aggregate{}, err
	}
	if err := change.Validate(x.scope); err != nil {
		return contacts.Aggregate{}, err
	}
	despues, err := antes.WithAggregates([]contacts.Aggregate{change})
	if err != nil {
		return contacts.Aggregate{}, err
	}
	if despues, err = x.conFavoritos(ctx, tx, despues, change); err != nil {
		return contacts.Aggregate{}, err
	}
	return change, x.guardar(ctx, tx, antes, despues)
}

// conFavoritos es s con el índice de favoritos aplicado a los contactos de change: en
// Contacts, la libreta se compone siempre con él.
func (x *Libreta) conFavoritos(ctx context.Context, tx *sql.Tx, s *contacts.Snapshot, change contacts.Aggregate) (*contacts.Snapshot, error) {
	var uids []string
	for _, r := range change.Rows {
		if r.Kind == "contact" {
			if c, err := columnasDe(r); err == nil {
				uids = append(uids, c.uid)
			}
		}
	}
	valores, err := favoritosDe(ctx, tx, uids)
	if err != nil || len(valores) == 0 {
		return s, err
	}
	return s.WithIndexes(&contacts.FavoriteIndex{Version: 1, Scope: x.scope, ID: model.PrivateIndexID, Values: valores}, nil)
}

// favoritosDe son los valores del índice de favoritos para esos UID.
func favoritosDe(ctx context.Context, q consulta, uids []string) (map[string]bool, error) {
	valores := map[string]bool{}
	err := enTrozos(uids, func(trozo []string) error {
		//nolint:gosec // G202: solo las marcas `?` del IN; los valores van en args
		rs, err := q.QueryContext(ctx, `SELECT uid, valor FROM favorito WHERE uid IN (`+marcas(len(trozo))+`)`, argumentos(trozo)...)
		if err != nil {
			return err
		}
		defer func() { _ = rs.Close() }()
		for rs.Next() {
			var uid string
			var valor bool
			if err := rs.Scan(&uid, &valor); err != nil {
				return err
			}
			valores[uid] = valor
		}
		return rs.Err()
	})
	return valores, err
}

// indices es como agregado para los índices de favoritos y de papelera: preparar da los
// valores que cambian (los demás ya están en las filas) y se aplican sobre la parte.
func (x *Libreta) indices(ctx context.Context, cargar func(*parte) error, preparar func(*contacts.Snapshot, time.Time) (*contacts.FavoriteIndex, *contacts.TrashIndex, error)) error {
	return x.escribir(ctx, func(tx *sql.Tx) error {
		p := nuevaParte(ctx, tx)
		if err := p.libretas(); err != nil {
			return err
		}
		if err := cargar(p); err != nil {
			return err
		}
		antes, err := p.instantanea(x)
		if err != nil {
			return err
		}
		favoritos, papelera, err := preparar(antes, x.ahora().UTC())
		if err != nil {
			return err
		}
		despues, err := antes.WithIndexes(favoritos, papelera)
		if err != nil {
			return err
		}
		if favoritos != nil {
			for uid, valor := range favoritos.Values {
				if _, err := tx.ExecContext(ctx, `INSERT INTO favorito (uid, valor) VALUES (?, ?)
					ON CONFLICT (uid) DO UPDATE SET valor = excluded.valor`, uid, valor); err != nil {
					return err
				}
			}
		}
		return x.guardar(ctx, tx, antes, despues)
	})
}

func (x *Libreta) indiceVacio() (contacts.FavoriteIndex, contacts.TrashIndex) {
	return contacts.FavoriteIndex{Version: 1, Scope: x.scope, ID: model.PrivateIndexID, Values: map[string]bool{}},
		contacts.TrashIndex{Version: 1, Scope: x.scope, ID: model.PrivateIndexID, Values: map[string]contacts.Removal{}}
}

// ----- Contactos -----

// Create da de alta un contacto con un UID nuevo.
func (x *Libreta) Create(ctx context.Context, book string, in api.ContactInput) (api.Contact, error) {
	change, err := x.agregado(ctx, nil, func(s *contacts.Snapshot, now time.Time) (contacts.Aggregate, error) {
		return s.PrepararAlta(book, in, now)
	})
	if err != nil {
		return api.Contact{}, err
	}
	return x.Contact(ctx, change.ID)
}

// cargarConUID lee los contactos que se ven en book con ese UID: con los que chocaría uno
// nuevo (NewPerson).
func cargarConUID(book, uid string) func(*parte) error {
	return func(p *parte) error {
		return p.contactosDonde(`libreta = ? AND uid = ? AND `+vivos, book, uid)
	}
}

// ImportCard da de alta un contacto con una vCard.
func (x *Libreta) ImportCard(ctx context.Context, book, data string) (api.Contact, error) {
	var cargar func(*parte) error
	if person, _, err := card.ImportCard(data); err == nil {
		cargar = cargarConUID(book, person.UID)
	}
	change, err := x.agregado(ctx, cargar, func(s *contacts.Snapshot, now time.Time) (contacts.Aggregate, error) {
		return s.PrepararTarjeta(book, data, now)
	})
	if err != nil {
		return api.Contact{}, err
	}
	return x.Contact(ctx, change.ID)
}

// Update sustituye el contacto id por in, si sigue con la etiqueta etag ("" no la mira).
func (x *Libreta) Update(ctx context.Context, id string, in api.ContactInput, etag string) (api.Contact, error) {
	_, err := x.agregado(ctx, func(p *parte) error { return p.contacto(id) }, func(s *contacts.Snapshot, now time.Time) (contacts.Aggregate, error) {
		return s.UpdatePerson(id, in, etag, now)
	})
	if err != nil {
		return api.Contact{}, err
	}
	return x.Contact(ctx, id)
}

// SetPhoto pone la foto (un data URI) en la tarjeta del contacto de ese UID.
func (x *Libreta) SetPhoto(ctx context.Context, uid, data string) error {
	canonical, err := card.ValidatePhotoDataURI(data)
	if err != nil {
		return err
	}
	return x.cambiarFoto(ctx, uid, canonical, false)
}

// RemovePhoto quita la foto de la tarjeta del contacto de ese UID.
func (x *Libreta) RemovePhoto(ctx context.Context, uid string) error {
	return x.cambiarFoto(ctx, uid, "", true)
}

func (x *Libreta) cambiarFoto(ctx context.Context, uid, data string, quitar bool) error {
	_, err := x.agregado(ctx, func(p *parte) error {
		return p.contactosDonde(`uid_limpio = ? AND `+vivos, card.CleanUID(uid))
	}, func(s *contacts.Snapshot, now time.Time) (contacts.Aggregate, error) {
		return s.ChangePhoto(uid, data, quitar, now)
	})
	return err
}

// ----- Favoritos y papelera -----

// SetFavorite destaca (o no) los contactos de ese UID.
func (x *Libreta) SetFavorite(ctx context.Context, uid string, on bool) error {
	return x.favoritos(ctx, []string{uid}, on, true)
}

// BatchFavorite destaca (o no) los contactos de esos UID.
func (x *Libreta) BatchFavorite(ctx context.Context, uids []string, on bool) error {
	return x.favoritos(ctx, uids, on, false)
}

func (x *Libreta) favoritos(ctx context.Context, uids []string, on, strict bool) error {
	uids = contacts.LimpiarUIDs(uids)
	if len(uids) == 0 {
		if strict {
			return errorDe(http.StatusNotFound, "private_contact_not_found")
		}
		return nil
	}
	// El índice se aplica a todos los contactos con esos UID, también a los de la papelera.
	return x.indices(ctx, func(p *parte) error { return p.contactosCon("uid", uids, "") },
		func(s *contacts.Snapshot, _ time.Time) (*contacts.FavoriteIndex, *contacts.TrashIndex, error) {
			vacio, _ := x.indiceVacio()
			state, err := s.ChangeFavorites(vacio, uids, on, strict)
			return &state, nil, err
		})
}

// Delete manda el contacto a la papelera.
func (x *Libreta) Delete(ctx context.Context, id string) error {
	return x.papelera(ctx, "delete", []string{id}, false)
}

// BatchTrash manda a la papelera los contactos de esos UID.
func (x *Libreta) BatchTrash(ctx context.Context, uids []string) error {
	return x.papelera(ctx, "batch-trash", contacts.LimpiarUIDs(uids), true)
}

// Restore saca el contacto de la papelera.
func (x *Libreta) Restore(ctx context.Context, id string) error {
	return x.papelera(ctx, "restore", []string{id}, false)
}

// Purge elimina para siempre un contacto de la papelera: sus filas se borran de la base y
// lo borrado se va del disco (vaciado.go).
func (x *Libreta) Purge(ctx context.Context, id string) error {
	err := x.papelera(ctx, "purge", []string{id}, false)
	if err == nil {
		x.vaciarPronto(ctx)
	}
	return err
}

func (x *Libreta) papelera(ctx context.Context, operacion string, ids []string, porUID bool) error {
	if len(ids) == 0 {
		return nil
	}
	return x.indices(ctx, func(p *parte) error {
		col := "id"
		if porUID {
			col = "uid"
		}
		if err := p.contactosCon(col, ids, ""); err != nil {
			return err
		}
		if operacion != "purge" {
			return nil
		}
		// Eliminar un contacto deja sin resolver a los miembros que lo nombran.
		var leidos []string
		for id := range p.contactos {
			leidos = append(leidos, id)
		}
		return p.gruposQueNombran(leidos)
	}, func(s *contacts.Snapshot, now time.Time) (*contacts.FavoriteIndex, *contacts.TrashIndex, error) {
		_, vacio := x.indiceVacio()
		state, err := s.ChangeTrash(vacio, operacion, ids, porUID, now)
		return nil, &state, err
	})
}

// BatchRestore es el «Deshacer» de BatchTrash: saca de la papelera lo último que se tiró
// de cada UID (en cada libreta) y dice cuántos volvieron.
func (x *Libreta) BatchRestore(ctx context.Context, uids []string) (int, error) {
	uids = contacts.LimpiarUIDs(uids)
	if len(uids) == 0 {
		return 0, nil
	}
	vueltos := 0
	err := x.indices(ctx, func(p *parte) error { return p.contactosCon("uid", uids, "") },
		func(s *contacts.Snapshot, _ time.Time) (*contacts.FavoriteIndex, *contacts.TrashIndex, error) {
			_, vacio := x.indiceVacio()
			state, n, err := s.RestoreLatest(vacio, uids)
			vueltos = n
			return nil, &state, err
		})
	if err != nil {
		return 0, err
	}
	return vueltos, nil
}

// ----- Libretas -----

// CreateAddressBook crea una libreta con ese rótulo (y su ruta CardDAV sacada de él).
func (x *Libreta) CreateAddressBook(ctx context.Context, display string) (api.AddressBook, error) {
	return x.CreateAddressBookAt(ctx, api.RutaDeLibreta(display), display, "")
}

// CreateAddressBookAt crea una libreta con esa ruta, rótulo y color.
func (x *Libreta) CreateAddressBookAt(ctx context.Context, name, display, color string) (api.AddressBook, error) {
	change, err := x.agregado(ctx, nil, func(s *contacts.Snapshot, now time.Time) (contacts.Aggregate, error) {
		return s.NewBook(name, display, color, now)
	})
	if err != nil {
		return api.AddressBook{}, err
	}
	return contacts.LibretaDe(change)
}

// RenameAddressBook cambia el rótulo de una libreta.
func (x *Libreta) RenameAddressBook(ctx context.Context, id, name string) (api.AddressBook, error) {
	change, err := x.agregado(ctx, nil, func(s *contacts.Snapshot, now time.Time) (contacts.Aggregate, error) {
		return s.RenameBook(id, name, now)
	})
	if err != nil {
		return api.AddressBook{}, err
	}
	return contacts.LibretaDe(change)
}

// DeleteAddressBook borra una libreta (no la predeterminada): ella y lo suyo dejan de verse.
func (x *Libreta) DeleteAddressBook(ctx context.Context, id string) error {
	_, err := x.agregado(ctx, nil, func(s *contacts.Snapshot, now time.Time) (contacts.Aggregate, error) {
		return s.DeleteBook(id, now)
	})
	return err
}

// CrearLibretaExterna da de alta la libreta de aquí que respalda una de una cuenta.
func (x *Libreta) CrearLibretaExterna(ctx context.Context, nombre string) (api.AddressBook, error) {
	ruta, rotulo := contacts.LibretaExterna(nombre)
	return x.CreateAddressBookAt(ctx, ruta, rotulo, "")
}

// ----- Etiquetas -----

// CreateGroup crea una etiqueta vacía en una libreta.
func (x *Libreta) CreateGroup(ctx context.Context, book, name string) (api.ContactGroup, error) {
	change, err := x.agregado(ctx, nil, func(s *contacts.Snapshot, now time.Time) (contacts.Aggregate, error) {
		return s.NewGroup(book, name, now)
	})
	if err != nil {
		return api.ContactGroup{}, err
	}
	g, err := x.grupoVisible(ctx, change.ID)
	return g.ContactGroup, err
}

// cargarGrupo lee la etiqueta id entera y los contactos que se ven en su libreta con
// alguno de esos UID (los miembros que se resuelven al escribirla).
func cargarGrupo(id string, uids func(p *parte) ([]string, error)) func(*parte) error {
	return func(p *parte) error {
		if err := p.grupo(id); err != nil {
			return err
		}
		var libreta string
		err := p.q.QueryRowContext(p.ctx, `SELECT libreta FROM fila WHERE clase = 'group' AND id = ?`, id).Scan(&libreta)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		miembros, err := uids(p)
		if err != nil {
			return err
		}
		return p.contactosCon("uid", miembros, `libreta = ? AND `+vivos, libreta)
	}
}

// RenameGroup cambia el nombre de una etiqueta y sus miembros (por UID).
func (x *Libreta) RenameGroup(ctx context.Context, id, name string, members []string) (api.ContactGroup, error) {
	_, err := x.agregado(ctx, cargarGrupo(id, func(*parte) ([]string, error) { return contacts.LimpiarUIDs(members), nil }),
		func(s *contacts.Snapshot, now time.Time) (contacts.Aggregate, error) {
			return s.UpdateGroup(id, name, members, now)
		})
	if err != nil {
		return api.ContactGroup{}, err
	}
	g, err := x.grupoVisible(ctx, id)
	return g.ContactGroup, err
}

// DeleteGroup borra una etiqueta; sus contactos no se tocan.
func (x *Libreta) DeleteGroup(ctx context.Context, id string) error {
	_, err := x.agregado(ctx, func(p *parte) error { return p.grupo(id) }, func(s *contacts.Snapshot, now time.Time) (contacts.Aggregate, error) {
		return s.PrepararBorrarGrupo(id, now)
	})
	return err
}

// BatchGroupMembers añade (o quita) esos UID de una etiqueta.
func (x *Libreta) BatchGroupMembers(ctx context.Context, id string, uids []string, add bool) error {
	_, err := x.agregado(ctx, cargarGrupo(id, func(p *parte) ([]string, error) {
		//nolint:gosec // G202: consulta fija
		rs, err := p.q.QueryContext(p.ctx, `SELECT uid FROM fila WHERE grupo = ? AND clase = 'member'`, id)
		if err != nil {
			return nil, err
		}
		actuales, err := columna(rs)
		return append(actuales, contacts.LimpiarUIDs(uids)...), err
	}), func(s *contacts.Snapshot, now time.Time) (contacts.Aggregate, error) {
		return s.PrepararMiembros(id, uids, add, now)
	})
	return err
}

// ----- Lo de las cuentas (local/agenda) y «Traer» (local/traer) -----

// PonerTarjetaCruda guarda una vCard en la libreta (contacts.Repository.PonerTarjetaCruda).
func (x *Libreta) PonerTarjetaCruda(ctx context.Context, libreta, id, esperado, datos string) (contacts.TarjetaCruda, error) {
	cargar := func(p *parte) error { return p.contacto(id) }
	if id == "" {
		cargar = nil
		if persona, _, err := card.ImportCardConUID(datos); err == nil {
			cargar = func(p *parte) error {
				return p.contactosCon("uid", []string{persona.UID, card.CleanUID(persona.UID)}, `libreta = ? AND `+vivos, libreta)
			}
		}
	}
	change, err := x.agregado(ctx, cargar, func(s *contacts.Snapshot, now time.Time) (contacts.Aggregate, error) {
		return s.PrepararTarjetaCruda(libreta, id, esperado, datos, now)
	})
	if err != nil {
		return contacts.TarjetaCruda{}, err
	}
	c, vcard, err := x.contactoConTarjeta(ctx, change.ID)
	if err != nil {
		return contacts.TarjetaCruda{}, err
	}
	return contacts.TarjetaCruda{ID: c.ID, UID: c.UID, Datos: vcard}, nil
}

// BorrarTarjetaCruda manda el contacto a la papelera si sigue con la vCard esperado (""
// no lo mira). Si ya no estaba, no es un error.
func (x *Libreta) BorrarTarjetaCruda(ctx context.Context, id, esperado string) error {
	_, vcard, err := x.contactoConTarjeta(ctx, id)
	if err != nil {
		if noEsta(err) {
			return nil
		}
		return err
	}
	if esperado != "" && vcard != esperado {
		return contacts.ErrCambioLocal
	}
	return x.Delete(ctx, id)
}

// PonerGrupoCrudo guarda una vCard de grupo como etiqueta de la libreta
// (contacts.Repository.PonerGrupoCrudo).
func (x *Libreta) PonerGrupoCrudo(ctx context.Context, libreta, id, esperado, datos string) (contacts.TarjetaCruda, error) {
	var cargar func(*parte) error
	if g, err := card.DecodeGroup(datos); err == nil {
		cargar = func(p *parte) error {
			if id == "" {
				if err := p.gruposDonde(`libreta = ? AND uid_limpio = ?`, libreta, card.CleanUID(g.UID)); err != nil {
					return err
				}
			} else if err := p.grupo(id); err != nil {
				return err
			}
			return p.contactosCon("uid", contacts.LimpiarUIDs(g.Members), `libreta = ? AND `+vivos, libreta)
		}
	} else if id != "" {
		cargar = func(p *parte) error { return p.grupo(id) }
	}
	change, err := x.agregado(ctx, cargar, func(s *contacts.Snapshot, now time.Time) (contacts.Aggregate, error) {
		return s.PrepararGrupoCrudo(libreta, id, esperado, datos, now)
	})
	if err != nil {
		return contacts.TarjetaCruda{}, err
	}
	g, err := x.grupoVisible(ctx, change.ID)
	if err != nil {
		return contacts.TarjetaCruda{}, err
	}
	return contacts.TarjetaCruda{ID: g.ID, UID: g.UID, Datos: g.VCard}, nil
}

// BorrarGrupoCrudo borra la etiqueta si sigue con la vCard esperado ("" no lo mira). Si
// ya no estaba, no es un error.
func (x *Libreta) BorrarGrupoCrudo(ctx context.Context, id, esperado string) error {
	g, err := x.grupoVisible(ctx, id)
	if err != nil {
		if noEsta(err) {
			return nil
		}
		return err
	}
	if esperado != "" && g.VCard != esperado {
		return contacts.ErrCambioLocal
	}
	return x.DeleteGroup(ctx, id)
}

// noEsta dice si err es un «no está» (404) de Contacts.
func noEsta(err error) bool {
	var ae *api.Error
	return errors.As(err, &ae) && ae.Status == http.StatusNotFound
}

// ----- Importar un CSV -----

// ImportCSV importa un CSV de Google Contacts o de Outlook en una libreta, en una sola
// transacción: cada fila es una escritura de Contacts (y no repite a nadie que ya esté
// por su UID o sus correos), pero se confirman todas juntas. Si una fila falla, o se
// cancela ctx, lo importado hasta entonces se queda, como en Contacts.
func (x *Libreta) ImportCSV(ctx context.Context, book string, raw []byte) (int, int, error) {
	parsed, skipped, err := contacts.FilasCSV(x.scope, book, raw)
	if err != nil {
		return 0, 0, err
	}
	if ok, err := x.ExisteLibreta(ctx, book); err != nil {
		return 0, 0, err
	} else if !ok {
		return 0, 0, errorDe(http.StatusNotFound, "private_address_book_not_found")
	}
	imported := 0
	var fallo error
	// La transacción no se corta con ctx: se confirma lo hecho.
	escribir := context.WithoutCancel(ctx)
	err = x.escribir(escribir, func(tx *sql.Tx) error {
		var err error
		imported, skipped, fallo, err = x.importarFilas(ctx, escribir, tx, book, parsed, skipped)
		return err
	})
	if err != nil {
		return 0, skipped, err
	}
	return imported, skipped, fallo
}

// importarFilas importa en tx las personas del CSV hasta acabar, hasta que Contacts
// rechace una o hasta que se cancele ctx (fallo dice cuál); err es un fallo de la base,
// y entonces no se confirma nada. escribir es el contexto de las escrituras.
func (x *Libreta) importarFilas(ctx, escribir context.Context, tx *sql.Tx, book string, parsed []card.Contact, skipped int) (imported, saltadas int, fallo, err error) {
	saltadas = skipped
	for _, person := range parsed {
		if cortado := ctx.Err(); cortado != nil {
			return imported, saltadas, cortado, nil
		}
		hecho, err := x.filaCSV(escribir, tx, book, person)
		switch {
		case err != nil:
			return imported, saltadas, nil, err
		case errors.Is(hecho, contacts.ErrFilaCSVRepetida):
			saltadas++
		case hecho != nil:
			return imported, saltadas, hecho, nil
		default:
			imported++
		}
	}
	return imported, saltadas, nil, nil
}

// filaCSV importa una persona del CSV en tx, en su propio punto de guardado: si no se
// puede (repetida, o la rechaza Contacts) no deja nada y lo dice en hecho; err es un
// fallo de la base.
func (x *Libreta) filaCSV(ctx context.Context, tx *sql.Tx, book string, person card.Contact) (hecho, err error) {
	if _, err := tx.ExecContext(ctx, `SAVEPOINT fila_csv`); err != nil {
		return nil, err
	}
	_, hecho = x.agregadoEn(ctx, tx, cargarFilaCSV(book, person), func(s *contacts.Snapshot, now time.Time) (contacts.Aggregate, error) {
		return s.PrepararFilaCSV(book, person, now)
	})
	if hecho != nil {
		if _, err := tx.ExecContext(ctx, `ROLLBACK TO fila_csv`); err != nil {
			return nil, err
		}
	}
	_, err = tx.ExecContext(ctx, `RELEASE fila_csv`)
	return hecho, err
}

// cargarFilaCSV lee los contactos que se ven en book con el UID de person o alguno de sus
// correos: con los que chocaría (contacts.Snapshot.PrepararFilaCSV).
func cargarFilaCSV(book string, person card.Contact) func(*parte) error {
	return func(p *parte) error {
		if err := cargarConUID(book, person.UID)(p); err != nil {
			return err
		}
		direcciones := make([]string, 0, len(person.Emails))
		for _, e := range person.Emails {
			direcciones = append(direcciones, direccionDe(api.ContactEmail{Email: e.Email}))
		}
		return enTrozos(direcciones, func(trozo []string) error {
			//nolint:gosec // G202: solo las marcas `?` del IN; los valores van en args
			rs, err := p.q.QueryContext(p.ctx, `SELECT DISTINCT c.id FROM correo e JOIN contacto c ON c.id = e.contacto
				WHERE e.libreta = ? AND e.direccion IN (`+marcas(len(trozo))+`) AND c.borrado IS NULL AND c.oculto = 0`,
				append([]any{book}, argumentos(trozo)...)...)
			if err != nil {
				return err
			}
			ids, err := columna(rs)
			if err != nil {
				return err
			}
			for _, id := range ids {
				if err := p.contacto(id); err != nil {
					return err
				}
			}
			return nil
		})
	}
}
