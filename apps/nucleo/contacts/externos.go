// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contacts

import (
	"context"
	"errors"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Lo que necesita el sincronizador de cuentas externas (`local/agenda`, K-05): cada
// libreta de una cuenta (CardDAV, Google, Microsoft) es una libreta de aquí, y sus
// contactos se leen y se escriben como la vCard ENTERA de cada uno, con su UID. Las
// escrituras pasan por la misma proyección que las de la pantalla (`writePerson`).

// TarjetaCruda es un contacto de una libreta como vCard.
type TarjetaCruda struct {
	ID    string // el del contacto en este equipo
	UID   string
	Datos string // la vCard
}

// CrearLibretaExterna da de alta la libreta de aquí que respalda una de una cuenta.
func (r *Repository) CrearLibretaExterna(ctx context.Context, nombre string) (api.AddressBook, error) {
	ruta, rotulo := LibretaExterna(nombre)
	return r.CreateAddressBookAt(ctx, ruta, rotulo, "")
}

// ExisteLibreta dice si la libreta sigue (no se borró).
func (r *Repository) ExisteLibreta(ctx context.Context, id string) (bool, error) {
	libros, err := r.AddressBooks(ctx)
	if err != nil {
		return false, err
	}
	for _, b := range libros {
		if b.ID == id {
			return true, nil
		}
	}
	return false, nil
}

// TarjetasCrudas son los contactos vivos (fuera de la papelera) de una libreta.
func (r *Repository) TarjetasCrudas(ctx context.Context, libreta string) ([]TarjetaCruda, error) {
	return readQuery(ctx, r, func(s *Snapshot) ([]TarjetaCruda, error) {
		gente, err := s.visiblePeople()
		if err != nil {
			return nil, err
		}
		var out []TarjetaCruda
		for _, p := range gente {
			if p.AddressBookID == libreta {
				out = append(out, TarjetaCruda{ID: p.ID, UID: p.UID, Datos: p.VCard})
			}
		}
		return out, nil
	})
}

// ErrCambioLocal: el contacto cambió en este equipo desde que se leyó. No se pisa.
var ErrCambioLocal = errors.New("contacts: el contacto cambió en este equipo")

// PonerTarjetaCruda guarda una vCard en la libreta: id "" crea el contacto (con el UID
// que trae; si ya hay uno con ese UID en la libreta, falla), y si no reemplaza el de ese
// id, que tiene que tener el mismo UID y, si esperado no es "", seguir con esa vCard (si
// no, ErrCambioLocal). Devuelve el contacto como quedó guardado.
func (r *Repository) PonerTarjetaCruda(ctx context.Context, libreta, id, esperado, datos string) (TarjetaCruda, error) {
	change, err := r.mutateAggregate(ctx, "put-external-card", struct{ Libreta, ID, Esperado, Datos string }{libreta, id, esperado, datos}, func(s *Snapshot) (Aggregate, error) {
		return s.PrepararTarjetaCruda(libreta, id, esperado, datos, time.Now().UTC())
	})
	if err != nil {
		return TarjetaCruda{}, err
	}
	c, err := readQuery(ctx, r, func(s *Snapshot) (ContactView, error) { return s.FindContact(change.ID) })
	if err != nil {
		return TarjetaCruda{}, err
	}
	return TarjetaCruda{ID: c.ID, UID: c.UID, Datos: c.VCard}, nil
}

// BorrarTarjetaCruda manda el contacto a la papelera, como el borrado de la pantalla,
// si sigue con la vCard esperado ("" no lo mira). Si ya no estaba, no es un error.
//
// La comprobación y el borrado no son una sola escritura (la papelera es un índice
// aparte): entre las dos solo cabe una edición de la persona en el mismo instante, que
// queda en la papelera y se puede recuperar.
func (r *Repository) BorrarTarjetaCruda(ctx context.Context, id, esperado string) error {
	c, err := readQuery(ctx, r, func(s *Snapshot) (ContactView, error) { return s.FindContact(id) })
	if err != nil {
		var ae *api.Error
		if errors.As(err, &ae) && ae.Status == http.StatusNotFound {
			return nil
		}
		return err
	}
	if esperado != "" && c.VCard != esperado {
		return ErrCambioLocal
	}
	return r.Delete(ctx, id)
}

// Los grupos (KIND:group, RFC 6350 §6.1.4) de una libreta sincronizada son sus
// etiquetas: se leen y se escriben como la vCard del grupo, con su UID, igual que los
// contactos. Los miembros son los UID de los contactos (MEMBER:urn:uuid:…).

// GruposCrudos son las etiquetas vivas de una libreta como vCard de grupo.
func (r *Repository) GruposCrudos(ctx context.Context, libreta string) ([]TarjetaCruda, error) {
	return readQuery(ctx, r, func(s *Snapshot) ([]TarjetaCruda, error) {
		grupos, err := s.Groups()
		if err != nil {
			return nil, err
		}
		var out []TarjetaCruda
		for _, g := range grupos {
			if g.AddressBookID == libreta && !g.BookDeleted && g.DeletedAt == nil {
				out = append(out, TarjetaCruda{ID: g.ID, UID: g.UID, Datos: g.VCard})
			}
		}
		return out, nil
	})
}

// PonerGrupoCrudo guarda una vCard de grupo como etiqueta de la libreta: id "" la crea
// con el UID que trae (si ya hay otra con ese UID en la libreta, falla), y si no
// reemplaza la de ese id, que tiene que tener el mismo UID y, si esperado no es "",
// seguir con esa vCard (si no, ErrCambioLocal). Lo que la vCard trae y Contacts no usa
// se conserva. Devuelve la etiqueta como quedó guardada.
func (r *Repository) PonerGrupoCrudo(ctx context.Context, libreta, id, esperado, datos string) (TarjetaCruda, error) {
	change, err := r.mutateAggregate(ctx, "put-external-group", struct{ Libreta, ID, Esperado, Datos string }{libreta, id, esperado, datos}, func(s *Snapshot) (Aggregate, error) {
		return s.PrepararGrupoCrudo(libreta, id, esperado, datos, time.Now().UTC())
	})
	if err != nil {
		return TarjetaCruda{}, err
	}
	g, err := readQuery(ctx, r, func(s *Snapshot) (GroupView, error) { return s.findGroup(change.ID) })
	if err != nil {
		return TarjetaCruda{}, err
	}
	return TarjetaCruda{ID: g.ID, UID: g.UID, Datos: g.VCard}, nil
}

// BorrarGrupoCrudo borra la etiqueta si sigue con la vCard esperado ("" no lo mira). Si
// ya no estaba, no es un error.
func (r *Repository) BorrarGrupoCrudo(ctx context.Context, id, esperado string) error {
	g, err := readQuery(ctx, r, func(s *Snapshot) (GroupView, error) { return s.findGroup(id) })
	if err != nil {
		var ae *api.Error
		if errors.As(err, &ae) && ae.Status == http.StatusNotFound {
			return nil
		}
		return err
	}
	if esperado != "" && g.VCard != esperado {
		return ErrCambioLocal
	}
	return r.DeleteGroup(ctx, id)
}

// LibretasExternas es lo que piden de Contacts quienes escriben en él libretas que
// vienen de fuera: el sincronizador de cuentas (`local/agenda`) y «Traer desde Sytue»
// del Hub (`local/traer`). Lo cumplen este repositorio y la libreta SQLite de Sytue
// Office (`local/libreta`), con los mismos errores (ErrCambioLocal).
type LibretasExternas interface {
	AddressBooks(ctx context.Context) ([]api.AddressBook, error)
	CreateAddressBookAt(ctx context.Context, name, display, color string) (api.AddressBook, error)
	RenameAddressBook(ctx context.Context, id, name string) (api.AddressBook, error)
	CrearLibretaExterna(ctx context.Context, nombre string) (api.AddressBook, error)
	ExisteLibreta(ctx context.Context, id string) (bool, error)
	TarjetasCrudas(ctx context.Context, libreta string) ([]TarjetaCruda, error)
	PonerTarjetaCruda(ctx context.Context, libreta, id, esperado, datos string) (TarjetaCruda, error)
	BorrarTarjetaCruda(ctx context.Context, id, esperado string) error
	GruposCrudos(ctx context.Context, libreta string) ([]TarjetaCruda, error)
	PonerGrupoCrudo(ctx context.Context, libreta, id, esperado, datos string) (TarjetaCruda, error)
	BorrarGrupoCrudo(ctx context.Context, id, esperado string) error
}

var _ LibretasExternas = (*Repository)(nil)
