// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contacts

import (
	"encoding/json"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # Las vistas de una instantánea, una vez
//
// Books, Contacts y Groups proyectan la instantánea entera: Contacts descodifica la vCard
// de cada contacto. Antes se proyectaba de nuevo en cada consulta y varias veces en
// cada alta (comprobar el UID, validar lo que se escribe, devolver el contacto). Como
// una instantánea no cambia, sus vistas tampoco: se calculan una vez, la primera que se
// piden, y se quedan con ella (y con la guardada del repositorio, foto.go).
//
// Una instantánea derivada de otra (la candidate de una escritura, WithAggregates)
// comparte con ella las filas que no cambian, con el mismo buffer (model.EachRow): sus
// vistas reutilizan lo ya descodificado de esas filas, y de cada contacto cuya fila,
// tarjeta, correos, teléfonos y libreta son los mismos, su proyección. Así validar lo
// que se escribe proyecta solo lo que cambia. El resultado es el mismo que proyectar
// de cero: si algo falla, se proyecta de cero (calcularLibros, calcularContactos,
// calcularGrupos) para dar el mismo error que siempre.
//
// Quien las pide desde fuera recibe su propia copia, con sus listas y fechas aparte,
// así que cambiarla no toca la guardada. Lo de dentro que solo lee usa las guardadas
// (libros, contactos, grupos, cadaVisible) sin copiarlas.

// vistas son las proyecciones guardadas de una instantánea.
type vistas struct {
	una   sync.Once
	hecha atomic.Pointer[proyeccion]
	// desde son las vistas de la instantánea de la que deriva esta, para reutilizar lo
	// suyo; se sueltan al proyectar.
	desde *vistas
}

// proyeccion son las tres vistas de una instantánea y, si salieron sin errores, lo
// descodificado de cada fila con el buffer del que salió.
type proyeccion struct {
	libros       []BookView
	librosErr    error
	contactos    []ContactView
	contactosErr error
	grupos       []GroupView
	gruposErr    error

	filas *filasVistas // nil si hubo algún error
}

// filasVistas es lo descodificado de cada fila, por su id, con su buffer.
type filasVistas struct {
	libros    map[string]filaDe[BookView]
	tarjetas  map[string]filaDe[sourceCard]
	correos   map[string]filaDe[sourceEmail]
	telefonos map[string]filaDe[sourcePhone]
	miembros  map[string]filaDe[sourceMember]
	grupos    map[string]filaDe[sourceGroup]
	// contactos son las proyecciones con los buffers de su fila y de su tarjeta.
	contactos map[string]contactoProyectado
}

type filaDe[T any] struct {
	json  []byte
	valor T
}

type contactoProyectado struct {
	fila, tarjeta []byte
	vista         ContactView
	clave         claveContacto
}

// proyeccion es la de s, calculada una vez.
func (s *Snapshot) proyeccion() *proyeccion {
	if s.vistas == nil {
		return s.proyectarDeCero()
	}
	v := s.vistas
	v.una.Do(func() {
		var base *filasVistas
		if v.desde != nil {
			if p := v.desde.hecha.Load(); p != nil {
				base = p.filas
			}
		}
		p := s.proyectar(base)
		v.desde = nil
		v.hecha.Store(p)
	})
	return v.hecha.Load()
}

// proyectarDeCero son las vistas de siempre, cada una por su lado.
func (s *Snapshot) proyectarDeCero() *proyeccion {
	p := &proyeccion{}
	p.libros, p.librosErr = s.calcularLibros()
	p.contactos, p.contactosErr = s.calcularContactos()
	p.grupos, p.gruposErr = s.calcularGrupos()
	return p
}

// libros son las libretas de la instantánea, calculadas una vez. SOLO para leer.
func (s *Snapshot) libros() ([]BookView, error) {
	p := s.proyeccion()
	return p.libros, p.librosErr
}

// contactos son los contactos de la instantánea, calculados una vez. SOLO para leer.
func (s *Snapshot) contactos() ([]ContactView, error) {
	p := s.proyeccion()
	return p.contactos, p.contactosErr
}

// grupos son las etiquetas de la instantánea, calculadas una vez. SOLO para leer.
func (s *Snapshot) grupos() ([]GroupView, error) {
	p := s.proyeccion()
	return p.grupos, p.gruposErr
}

// proyectar calcula las tres vistas reutilizando lo de base (si no es nil) para las
// filas con el mismo buffer. Si cualquier fila falla, las calcula de cero.
func (s *Snapshot) proyectar(base *filasVistas) *proyeccion {
	if base == nil {
		base = &filasVistas{}
	}
	f, filasContacto, ok := s.descodificarFilas(base)
	if !ok {
		return s.proyectarDeCero()
	}
	p := &proyeccion{filas: f}
	p.libros = make([]BookView, 0, len(f.libros))
	books := make(map[string]bool, len(f.libros))
	for _, fila := range f.libros {
		p.libros = append(p.libros, fila.valor)
		books[fila.valor.ID] = fila.valor.DeletedAt != nil
	}
	slices.SortFunc(p.libros, compararLibros)
	cards := make(map[string]sourceCard, len(f.tarjetas))
	for id, fila := range f.tarjetas {
		cards[id] = fila.valor
	}
	if p.contactos, ok = f.proyectarContactos(base, filasContacto, cards, books); !ok {
		return s.proyectarDeCero()
	}
	if p.grupos, ok = f.proyectarGrupos(cards, books); !ok {
		return s.proyectarDeCero()
	}
	return p
}

// descodificarFilas descodifica cada fila de s que no sea un contacto (lo de base si es
// el mismo buffer) y aparta las de los contactos. false si alguna no se descodifica.
func (s *Snapshot) descodificarFilas(base *filasVistas) (*filasVistas, []Row, bool) {
	f := &filasVistas{
		libros: map[string]filaDe[BookView]{}, tarjetas: map[string]filaDe[sourceCard]{},
		correos: map[string]filaDe[sourceEmail]{}, telefonos: map[string]filaDe[sourcePhone]{},
		miembros: map[string]filaDe[sourceMember]{}, grupos: map[string]filaDe[sourceGroup]{},
		contactos: map[string]contactoProyectado{},
	}
	var filasContacto []Row
	ok := true
	s.EachRow(func(row Row) bool {
		switch row.Kind {
		case "book":
			ok = descodificar(row, base.libros, f.libros)
		case "card":
			ok = descodificar(row, base.tarjetas, f.tarjetas)
		case "email":
			ok = descodificar(row, base.correos, f.correos)
		case "phone":
			ok = descodificar(row, base.telefonos, f.telefonos)
		case "member":
			ok = descodificar(row, base.miembros, f.miembros)
		case "group":
			ok = descodificar(row, base.grupos, f.grupos)
		case "contact":
			filasContacto = append(filasContacto, row)
		}
		return ok
	})
	return f, filasContacto, ok
}

// proyectarContactos es lo de calcularContactos con lo ya descodificado en f: de cada
// contacto cuya fila, tarjeta, correos, teléfonos y libreta son los de base, su
// proyección de base. false si alguno no se proyecta.
func (f *filasVistas) proyectarContactos(base *filasVistas, filasContacto []Row, cards map[string]sourceCard, books map[string]bool) ([]ContactView, bool) {
	emails, phones := agruparCanales(f.correos, f.telefonos)
	contacts := make([]ContactView, 0, len(filasContacto))
	claves := make([]claveContacto, 0, len(filasContacto))
	for _, row := range filasContacto {
		hecho, ok := base.contactos[row.ID]
		if !ok || !hecho.sigue(row.JSON, f.tarjetas[hecho.vista.ObjectID].json, emails, phones, books) {
			value, err := projectContact(row.JSON, cards, emails, phones, books)
			if err != nil {
				return nil, false
			}
			hecho = contactoProyectado{fila: row.JSON, tarjeta: f.tarjetas[value.ObjectID].json, vista: value, clave: claveDe(&value)}
		}
		f.contactos[row.ID] = hecho
		contacts = append(contacts, hecho.vista)
		claves = append(claves, hecho.clave)
	}
	return ordenarContactos(contacts, claves), true
}

// sigue dice si projectContact daría lo mismo que hecho con esta fila, esta tarjeta y
// estos canales y libretas.
func (hecho contactoProyectado) sigue(fila, tarjeta []byte, emails map[string][]api.ContactEmail, phones map[string][]api.ContactPhone, books map[string]bool) bool {
	v := &hecho.vista
	return mismoBuffer(hecho.fila, fila) && mismoBuffer(hecho.tarjeta, tarjeta) &&
		slices.Equal(v.Emails, emails[v.ID]) && slices.Equal(v.Phones, phones[v.ID]) &&
		v.BookDeleted == books[v.AddressBookID]
}

// proyectarGrupos es lo de calcularGrupos con lo ya descodificado en f. false si a un
// grupo le falta su tarjeta.
func (f *filasVistas) proyectarGrupos(cards map[string]sourceCard, books map[string]bool) ([]GroupView, bool) {
	members := map[string][]string{}
	for _, fila := range f.miembros {
		members[fila.valor.GroupID] = append(members[fila.valor.GroupID], fila.valor.UID)
	}
	grupos := make([]GroupView, 0, len(f.grupos))
	for _, fila := range f.grupos {
		group, found := projectGroup(fila.valor, cards, members, books)
		if !found {
			return nil, false
		}
		grupos = append(grupos, group)
	}
	slices.SortFunc(grupos, compararGrupos)
	return grupos, true
}

// descodificar pone en out lo de row: lo de base si es el mismo buffer, o lo que da
// descodificarla. false si no se descodifica.
func descodificar[T any](row Row, base, out map[string]filaDe[T]) bool {
	if hecho, ok := base[row.ID]; ok && mismoBuffer(hecho.json, row.JSON) {
		out[row.ID] = hecho
		return true
	}
	var value T
	if json.Unmarshal(row.JSON, &value) != nil {
		return false
	}
	out[row.ID] = filaDe[T]{json: row.JSON, valor: value}
	return true
}

// agruparCanales son los correos y teléfonos de cada contacto como los da channels: por
// posición y, a igualdad, por id.
func agruparCanales(correos map[string]filaDe[sourceEmail], telefonos map[string]filaDe[sourcePhone]) (map[string][]api.ContactEmail, map[string][]api.ContactPhone) {
	emails := make([]sourceEmail, 0, len(correos))
	for _, fila := range correos {
		emails = append(emails, fila.valor)
	}
	phones := make([]sourcePhone, 0, len(telefonos))
	for _, fila := range telefonos {
		phones = append(phones, fila.valor)
	}
	return agruparCorreos(emails), agruparTelefonos(phones)
}

// mismoBuffer dice si a y b son el mismo buffer (model.EachRow).
func mismoBuffer(a, b []byte) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

func (s *Snapshot) Books() ([]BookView, error) {
	books, err := s.libros()
	if err != nil {
		return nil, err
	}
	out := make([]BookView, len(books))
	for i, book := range books {
		book.DeletedAt = copiarHora(book.DeletedAt)
		out[i] = book
	}
	return out, nil
}

func (s *Snapshot) Contacts() ([]ContactView, error) {
	people, err := s.contactos()
	if err != nil {
		return nil, err
	}
	out := make([]ContactView, len(people))
	for i, person := range people {
		out[i] = copiarContacto(person)
	}
	return out, nil
}

func (s *Snapshot) Groups() ([]GroupView, error) {
	groups, err := s.grupos()
	if err != nil {
		return nil, err
	}
	out := make([]GroupView, len(groups))
	for i, group := range groups {
		group.Members = slices.Clone(group.Members)
		group.DeletedAt = copiarHora(group.DeletedAt)
		out[i] = group
	}
	return out, nil
}

// copiarContacto es person con sus listas y fechas aparte.
func copiarContacto(person ContactView) ContactView {
	person.Birthday = copiarHora(person.Birthday)
	person.Anniversary = copiarHora(person.Anniversary)
	person.DeletedAt = copiarHora(person.DeletedAt)
	person.Emails = slices.Clone(person.Emails)
	person.Phones = slices.Clone(person.Phones)
	person.Addresses = slices.Clone(person.Addresses)
	person.URLs = slices.Clone(person.URLs)
	person.Related = slices.Clone(person.Related)
	return person
}

func copiarHora(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	copia := *t
	return &copia
}
