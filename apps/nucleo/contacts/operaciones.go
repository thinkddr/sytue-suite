// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contacts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/card"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// # Las operaciones, sobre una instantánea
//
// Cada escritura del repositorio prepara su agregado sobre una instantánea: la de la
// libreta entera (Repository) o la de la parte de la libreta que esa escritura mira y
// toca, sacada con índices de una base (la libreta SQLite de Sytue Office,
// `local/libreta`, el plan B de PLAN-EDICION-LOCAL §5.6). Son las MISMAS funciones para
// las dos: lo que se valida, los errores y las filas que salen no dependen de dónde se
// guarde la libreta. Lo de aquí es lo que el repositorio hacía dentro de cada escritura,
// sacado tal cual (sin cambiar lo que hace) para que la base no lo copie.
//
// Las claves de orden y lo que mira la búsqueda también están aquí: la base guarda la de
// cada contacto en una columna con índice y tiene que salir el mismo orden que el de
// Contacts (ClaveDeOrden, ClaveDeLibreta, ClaveDeGrupo, TextoBuscable).

// ErrFilaCSVRepetida es la fila de un CSV que ya está en la libreta (su UID o uno de sus
// correos): no se importa y cuenta como omitida.
var ErrFilaCSVRepetida = errors.New("contacts: CSV row already exists")

// ErrorDeConsulta es el error de una consulta o escritura, con el estado y el código del
// servicio («private_contact_not_found»…): el mismo que dan las de Contacts.
func ErrorDeConsulta(status int, code string) error { return queryError(status, code) }

// LimitePagina es el tope de una página: value si va de 1 a 500; si no, defaultValue.
func LimitePagina(value, defaultValue int) int { return pageLimit(value, defaultValue) }

// LimpiarUIDs son los UID sin «urn:uuid:», sin espacios, sin vacíos y sin repetir, en su
// orden: lo que miran favoritos, papelera y etiquetas.
func LimpiarUIDs(uids []string) []string { return cleanMembers(uids) }

// PrepararAlta es el agregado de un contacto nuevo en book con un UID nuevo (Create).
func (s *Snapshot) PrepararAlta(book string, in api.ContactInput, now time.Time) (Aggregate, error) {
	person, err := contactInput(in)
	if err != nil {
		return Aggregate{}, err
	}
	person.UID = card.NewUID()
	return s.NewPerson(book, person, "", now)
}

// PrepararTarjeta es el agregado de un contacto nuevo en book con una vCard (ImportCard).
func (s *Snapshot) PrepararTarjeta(book, data string, now time.Time) (Aggregate, error) {
	person, canonical, err := card.ImportCard(data)
	if err != nil {
		return Aggregate{}, err
	}
	return s.NewPerson(book, person, canonical, now)
}

// PrepararTarjetaCruda es el agregado de PonerTarjetaCruda: id "" crea el contacto con el
// UID que trae la vCard; si no, reemplaza el de ese id, que tiene que tener el mismo UID
// en la misma libreta y, si esperado no es "", seguir con esa vCard (ErrCambioLocal).
func (s *Snapshot) PrepararTarjetaCruda(libreta, id, esperado, datos string, now time.Time) (Aggregate, error) {
	persona, base, err := card.ImportCardConUID(datos)
	if err != nil {
		return Aggregate{}, err
	}
	if id == "" {
		return s.NewPerson(libreta, persona, base, now)
	}
	actual, err := s.FindContact(id)
	if err != nil {
		return Aggregate{}, err
	}
	if esperado != "" && actual.VCard != esperado {
		return Aggregate{}, ErrCambioLocal
	}
	if actual.AddressBookID != libreta || card.CleanUID(actual.UID) != card.CleanUID(persona.UID) {
		return Aggregate{}, queryError(http.StatusConflict, "contact_uid_changed")
	}
	persona.ID, persona.ObjectID = uuid.MustParse(id), uuid.MustParse(actual.ObjectID)
	persona.AddressBookID = uuid.MustParse(libreta)
	persona.Tenant = s.Scope().Tenant
	persona.UID = actual.UID
	persona.ETag = uuid.New()
	persona.UpdatedAt = now
	data, err := card.EncodeContact(persona, base)
	if err != nil {
		return Aggregate{}, err
	}
	a, err := editAggregate(s, "contact_state", id)
	if err != nil {
		return Aggregate{}, err
	}
	return s.writePerson(a, persona, data, now)
}

// PrepararGrupoCrudo es el agregado de PonerGrupoCrudo: id "" crea la etiqueta con el UID
// que trae la vCard de grupo (si ya hay otra con ese UID en la libreta, falla); si no,
// reemplaza la de ese id, con el mismo UID en la misma libreta y, si esperado no es "",
// con esa vCard todavía (ErrCambioLocal).
func (s *Snapshot) PrepararGrupoCrudo(libreta, id, esperado, datos string, now time.Time) (Aggregate, error) {
	g, err := card.DecodeGroup(datos)
	if err != nil {
		return Aggregate{}, err
	}
	g.UID = card.CleanUID(g.UID)
	if id == "" {
		grupos, err := s.Groups()
		if err != nil {
			return Aggregate{}, err
		}
		for _, otro := range grupos {
			if otro.AddressBookID == libreta && otro.DeletedAt == nil && card.CleanUID(otro.UID) == g.UID {
				return Aggregate{}, queryError(http.StatusConflict, "group_uid_exists")
			}
		}
		return s.newGroupWith(libreta, g, datos, now)
	}
	actual, err := s.findGroup(id)
	if err != nil {
		return Aggregate{}, err
	}
	if esperado != "" && actual.VCard != esperado {
		return Aggregate{}, ErrCambioLocal
	}
	if actual.AddressBookID != libreta || card.CleanUID(actual.UID) != g.UID {
		return Aggregate{}, queryError(http.StatusConflict, "group_uid_changed")
	}
	return s.updateGroupWith(actual, g.Name, g.Members, datos, now)
}

// PrepararBorrarGrupo es el agregado que borra la etiqueta id (DeleteGroup).
func (s *Snapshot) PrepararBorrarGrupo(id string, now time.Time) (Aggregate, error) {
	if _, err := s.findGroup(id); err != nil {
		return Aggregate{}, err
	}
	change, err := editAggregate(s, "group_state", id)
	if err != nil {
		return Aggregate{}, err
	}
	if err := aggregateTimestamp(&change, now); err != nil {
		return Aggregate{}, err
	}
	return change, nil
}

// PrepararMiembros es el agregado que añade (o quita) esos UID de la etiqueta id
// (BatchGroupMembers).
func (s *Snapshot) PrepararMiembros(id string, uids []string, add bool, now time.Time) (Aggregate, error) {
	current, err := s.findGroup(id)
	if err != nil {
		return Aggregate{}, err
	}
	members := slices.Clone(current.Members)
	for _, uid := range cleanMembers(uids) {
		at := slices.Index(members, uid)
		if add && at < 0 {
			members = append(members, uid)
		} else if !add && at >= 0 {
			members = append(members[:at], members[at+1:]...)
		}
	}
	return s.UpdateGroup(id, current.Name, members, now)
}

// FindGroup es la etiqueta id, si se ve (ni borrada ni en una libreta borrada).
func (s *Snapshot) FindGroup(id string) (GroupView, error) { return s.findGroup(id) }

// LibretaDe es la libreta que deja el agregado de NewBook o RenameBook.
func LibretaDe(change Aggregate) (api.AddressBook, error) { return aggregateBook(change) }

// LibretaExterna son la ruta y el rótulo de la libreta de aquí que respalda una de una
// cuenta llamada nombre (CrearLibretaExterna): sin nombre, «Contactos».
func LibretaExterna(nombre string) (ruta, rotulo string) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		nombre = "Contactos"
	}
	return "externa-" + uuid.NewString()[:8], nombre
}

// FilasCSV son las personas de un CSV de Google Contacts o de Outlook que se importan en
// book, cada una con el UID que le toca (el mismo si se importa otra vez el mismo
// fichero: así no se repiten), y cuántas filas se omitieron al leerlo.
func FilasCSV(scope storage.Scope, book string, raw []byte) ([]card.Contact, int, error) {
	if len(raw) > 5<<20 {
		return nil, 0, queryError(http.StatusRequestEntityTooLarge, "csv_too_large")
	}
	parsed, skipped, err := card.ParseContactsCSV(bytes.NewReader(raw))
	if err != nil {
		return nil, 0, err
	}
	sum := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(sum[:])
	header, err := storage.EncodeScopeHeader(scope)
	if err != nil {
		return nil, 0, err
	}
	for i := range parsed {
		parsed[i].UID = uuid.NewSHA1(uuid.NameSpaceURL, []byte("sytue-contacts-csv-v1\x00"+header+"\x00"+book+"\x00"+fingerprint+"\x00"+strconv.Itoa(i))).String()
	}
	return parsed, skipped, nil
}

// PrepararFilaCSV es el agregado de una persona de FilasCSV en book, o
// ErrFilaCSVRepetida si ya está en la libreta (su UID o uno de sus correos).
func (s *Snapshot) PrepararFilaCSV(book string, person card.Contact, now time.Time) (Aggregate, error) {
	duplicate, err := s.csvDuplicate(book, person)
	if err != nil {
		return Aggregate{}, err
	}
	if duplicate {
		return Aggregate{}, ErrFilaCSVRepetida
	}
	return s.NewPerson(book, person, "", now)
}

// ClaveDeOrden es por lo que se ordenan los contactos: apellidos, nombre, nombre completo
// e identificador, sin mayúsculas, campo a campo.
func ClaveDeOrden(c *api.Contact) [4]string {
	return [4]string{strings.ToLower(c.FamilyName), strings.ToLower(c.GivenName), strings.ToLower(c.FormattedName), strings.ToLower(c.ID)}
}

// ClaveDeLibreta es por lo que se ordenan las libretas: la predeterminada primero («0»;
// las demás, «1»), luego el rótulo sin mayúsculas y el identificador.
func ClaveDeLibreta(b api.AddressBook) [3]string {
	primera := "1"
	if b.IsDefault {
		primera = "0"
	}
	return [3]string{primera, strings.ToLower(b.DisplayName), b.ID}
}

// ClaveDeGrupo es por lo que se ordenan las etiquetas: el nombre sin mayúsculas y el
// identificador.
func ClaveDeGrupo(g api.ContactGroup) [2]string {
	return [2]string{strings.ToLower(g.Name), g.ID}
}

// TextoBuscable es lo que mira la búsqueda de un contacto (nombre, apellidos,
// organización, apodo, nota, correos y teléfonos), sin acentos ni mayúsculas.
func TextoBuscable(c *api.Contact) string { return textoBuscable(c) }

// CorreoPrincipal es el correo que la papelera enseña de un contacto: el principal o, si
// no tiene, el primero.
func CorreoPrincipal(emails []api.ContactEmail) string {
	primary := ""
	for _, email := range emails {
		if primary == "" || email.IsPrimary {
			primary = email.Email
		}
		if email.IsPrimary {
			break
		}
	}
	return primary
}

// FotoDeVCard es la foto que lleva dentro una vCard, o «private_contact_photo_not_found».
func FotoDeVCard(vcard string) ([]byte, error) {
	value, err := card.PhotoDataURI(vcard)
	if err != nil {
		return nil, err
	}
	if value == "" {
		return nil, queryError(http.StatusNotFound, "private_contact_photo_not_found")
	}
	raw, _, err := card.PhotoBytes(value)
	return raw, err
}

// CSVDeContactos es «Exportar CSV» de esos contactos, en su orden: el nombre del fichero
// y el CSV de Google Contacts.
func CSVDeContactos(cs []api.Contact) (string, []byte, error) {
	out := make([]card.Contact, 0, len(cs))
	for i := range cs {
		contact, err := contactoCSV(&cs[i])
		if err != nil {
			return "", nil, err
		}
		out = append(out, contact)
	}
	var output bytes.Buffer
	if err := card.WriteContactsCSV(&output, out); err != nil {
		return "", nil, err
	}
	return "contacts.csv", output.Bytes(), nil
}

// contactoCSV es un contacto de la libreta como lo escribe el CSV.
func contactoCSV(c *api.Contact) (card.Contact, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return card.Contact{}, err
	}
	var contact card.Contact
	err = json.Unmarshal(raw, &contact)
	clear(raw)
	return contact, err
}
