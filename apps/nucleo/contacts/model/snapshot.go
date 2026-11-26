// SPDX-License-Identifier: Apache-2.0
// Package model validates the complete personal Contacts graph without network,
// credential storage, or operating-system dependencies.
package model

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const MaxRowBytes = 8 << 20

func SourceKind(kind string) bool {
	switch kind {
	case "book", "card", "contact", "email", "phone", "group", "member":
		return true
	default:
		return false
	}
}

var ErrSnapshot = errors.New("contacts: invalid or incomplete private snapshot")

// Row preserves the complete source JSON, rather than rebuilding it from the
// subset of fields shown by an app. Photos, unknown vCard fields, trash, sync
// metadata, row IDs and future JSON properties consequently survive migration.
type Row struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	JSON []byte `json:"json"`
}

// Snapshot is validated as a whole before a private import can be acknowledged.
// All buffers returned by Rows are independent of the retained originals.
//
// Una Snapshot no cambia nunca una vez compuesta, ni los buffers JSON de sus filas: las
// que derivan de ella comparten las filas que no cambian, y con ellas sus enlaces ya
// descodificados, así que componer una descodifica solo las filas nuevas.
type Snapshot struct {
	scope storage.Scope
	rows  map[string]Row
	// links tiene, de cada fila, los campos de relación que decodeRow leyó de ese mismo
	// buffer JSON: lo que miran validateRelation y aggregateOwnedRows.
	links map[string]link
}

type relation struct {
	kind, id, book, object, contact, group, uid string
	owner                                       string
	raw                                         map[string]json.RawMessage
}

// link es una relation descodificada sin sus campos JSON.
type link struct {
	kind, id, book, object, contact, group, uid string
	owner                                       string
}

func (r relation) link() link {
	return link{kind: r.kind, id: r.id, book: r.book, object: r.object, contact: r.contact, group: r.group, uid: r.uid, owner: r.owner}
}

func rowKey(kind, id string) string { return kind + "\x00" + id }

func validUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

func stringField(raw map[string]json.RawMessage, key string, required bool) (string, error) {
	data, found := raw[key]
	if !found || string(data) == "null" {
		if required {
			return "", ErrSnapshot
		}
		return "", nil
	}
	var value string
	if json.Unmarshal(data, &value) != nil || (required && value == "") {
		return "", ErrSnapshot
	}
	return value, nil
}

func uuidField(raw map[string]json.RawMessage, key string, required bool) (string, error) {
	value, err := stringField(raw, key, required)
	if err != nil || value != "" && !validUUID(value) {
		return "", ErrSnapshot
	}
	return value, nil
}

func decodeRow(scope storage.Scope, row Row) (relation, error) {
	out := relation{kind: row.Kind, id: row.ID}
	if !scope.Valid() || scope.Product != "contacts" || !validUUID(row.ID) || len(row.JSON) == 0 || len(row.JSON) > MaxRowBytes || json.Unmarshal(row.JSON, &out.raw) != nil || out.raw == nil {
		return out, ErrSnapshot
	}
	id, err := uuidField(out.raw, "id", true)
	if err != nil || id != row.ID {
		return out, ErrSnapshot
	}
	tenant, err := stringField(out.raw, "tenant", true)
	if err != nil || tenant != scope.Tenant {
		return out, ErrSnapshot
	}
	return decodeRelation(scope, row, out)
}

func decodeRelation(scope storage.Scope, row Row, out relation) (relation, error) {
	var err error
	switch row.Kind {
	case "book":
		out.owner, err = stringField(out.raw, "owner_sub", true)
		if err != nil || out.owner != scope.Subject {
			return out, ErrSnapshot
		}
		if _, err = stringField(out.raw, "name", true); err != nil {
			return out, err
		}
	case "card", "contact", "group":
		out.book, err = uuidField(out.raw, "address_book_id", true)
		if err != nil {
			return out, err
		}
		out.uid, err = stringField(out.raw, "uid", true)
		if err != nil {
			return out, err
		}
		if row.Kind != "card" {
			out.object, err = uuidField(out.raw, "object_id", true)
			if err != nil {
				return out, err
			}
		} else if err := validateCardRow(out.raw); err != nil {
			return out, err
		}
	case "email", "phone":
		out.contact, err = uuidField(out.raw, "contact_id", true)
		if err != nil {
			return out, err
		}
	case "member":
		out.group, err = uuidField(out.raw, "group_id", true)
		if err != nil {
			return out, err
		}
		out.contact, err = uuidField(out.raw, "contact_id", false)
		if err != nil {
			return out, err
		}
		out.uid, err = stringField(out.raw, "contact_uid", true)
		if err != nil {
			return out, err
		}
	default:
		return out, ErrSnapshot
	}
	return out, nil
}

func validateCardRow(raw map[string]json.RawMessage) error {
	if _, err := uuidField(raw, "etag", true); err != nil {
		return err
	}
	if _, err := stringField(raw, "resource_name", true); err != nil {
		return err
	}
	data, err := stringField(raw, "data", true)
	var size int64
	if err != nil || json.Unmarshal(raw["size_bytes"], &size) != nil || size != int64(len(data)) {
		return ErrSnapshot
	}
	return nil
}

// ValidateRow checks the source's scope and foreign-key shape without discarding
// unknown fields or parsing/re-encoding large integer values. NewSnapshot also
// checks every relationship against the complete retained inventory.
func ValidateRow(scope storage.Scope, row Row) error {
	_, err := decodeRow(scope, row)
	return err
}

func NewSnapshot(scope storage.Scope, rows []Row) (*Snapshot, error) {
	if !scope.Valid() || scope.Product != "contacts" {
		return nil, ErrSnapshot
	}
	snapshot := &Snapshot{scope: scope, rows: make(map[string]Row, len(rows)), links: make(map[string]link, len(rows))}
	for _, row := range rows {
		relation, err := decodeRow(scope, row)
		key := rowKey(row.Kind, row.ID)
		if err != nil {
			return nil, err
		}
		if _, duplicate := snapshot.rows[key]; duplicate {
			return nil, ErrSnapshot
		}
		row.JSON = slices.Clone(row.JSON)
		snapshot.rows[key], snapshot.links[key] = row, relation.link()
	}
	if err := validateLinks(snapshot.links); err != nil {
		return nil, err
	}
	return snapshot, nil
}

// compose es NewSnapshot sobre un mapa de filas, descodificando solo las que no tienen
// enlace. La instantánea nueva se queda con rows y links: cada enlace tiene que salir
// del mismo buffer JSON de su fila, y links no tiene claves que no estén en rows. Como
// NewSnapshot, valida todas las relaciones.
func compose(scope storage.Scope, rows map[string]Row, links map[string]link) (*Snapshot, error) {
	if !scope.Valid() || scope.Product != "contacts" {
		return nil, ErrSnapshot
	}
	for key, row := range rows {
		if _, found := links[key]; found {
			continue
		}
		relation, err := decodeRow(scope, row)
		if err != nil {
			return nil, err
		}
		links[key] = relation.link()
	}
	if len(links) != len(rows) {
		return nil, ErrSnapshot
	}
	if err := validateLinks(links); err != nil {
		return nil, err
	}
	return &Snapshot{scope: scope, rows: rows, links: links}, nil
}

func validateLinks(all map[string]link) error {
	for _, row := range all {
		if err := validateRelation(row, all); err != nil {
			return err
		}
	}
	return nil
}

// sameBuffer dice si a y b son el mismo buffer JSON, así que el enlace descodificado
// de uno vale para el otro.
func sameBuffer(a, b []byte) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

// keptLinks son los enlaces de s que siguen valiendo para rows: los de las filas que
// siguen siendo el mismo buffer.
func (s *Snapshot) keptLinks(rows map[string]Row) map[string]link {
	kept := make(map[string]link, len(rows))
	for key, row := range rows {
		if value, found := s.links[key]; found && sameBuffer(s.rows[key].JSON, row.JSON) {
			kept[key] = value
		}
	}
	return kept
}

func validateRelation(row link, all map[string]link) error {
	switch row.kind {
	case "book":
		return nil
	case "card":
		if _, found := all[rowKey("book", row.book)]; !found {
			return ErrSnapshot
		}
	case "contact", "group":
		object, found := all[rowKey("card", row.object)]
		if !found || object.book != row.book || object.uid != row.uid {
			return ErrSnapshot
		}
	case "email", "phone":
		if _, found := all[rowKey("contact", row.contact)]; !found {
			return ErrSnapshot
		}
	case "member":
		group, found := all[rowKey("group", row.group)]
		if !found {
			return ErrSnapshot
		}
		// Unresolved MEMBER UIDs are legal CardDAV data. A resolved reference must
		// name the same local book and UID, never another person's contact.
		if row.contact != "" {
			contact, found := all[rowKey("contact", row.contact)]
			if !found || contact.book != group.book || contact.uid != row.uid {
				return ErrSnapshot
			}
		}
	default:
		return ErrSnapshot
	}
	return nil
}

func (s *Snapshot) Scope() storage.Scope { return s.scope }

func (s *Snapshot) Rows() []Row {
	rows := make([]Row, 0, len(s.rows))
	for _, row := range s.rows {
		row.JSON = slices.Clone(row.JSON)
		rows = append(rows, row)
	}
	slices.SortFunc(rows, compareRows)
	return rows
}

// EachRow llama a fn con cada fila, en cualquier orden, hasta que fn devuelve false.
// Las filas llevan los buffers JSON de la propia instantánea, que no cambian nunca y
// comparten las que derivan de ella, así que el mismo buffer es la misma fila: fn solo
// puede leerlos. Rows da copias propias.
func (s *Snapshot) EachRow(fn func(Row) bool) {
	for _, row := range s.rows {
		if !fn(row) {
			return
		}
	}
}

func compareRows(a, b Row) int {
	if compared := strings.Compare(a.Kind, b.Kind); compared != 0 {
		return compared
	}
	return strings.Compare(a.ID, b.ID)
}
