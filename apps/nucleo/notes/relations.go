// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

type rowRelation struct {
	ID         string  `json:"id"`
	ParentID   *string `json:"parent_id"`
	NotebookID string  `json:"notebook_id"`
	GroupID    *string `json:"group_id"`
}

func (r *Repository) validateRelations(ctx context.Context, catalog records.Catalog) error {
	versions, err := catalog.Entries()
	if err != nil {
		return err
	}
	books := map[string]*string{}
	notes := map[string]rowRelation{}
	for _, version := range versions {
		if version.Kind != "notebook" && version.Kind != "note" {
			continue
		}
		raw, err := catalog.ReadJSON(ctx, version)
		if err != nil {
			return err
		}
		var relation rowRelation
		err = json.Unmarshal(raw, &relation)
		clear(raw)
		if err != nil || relation.ID != version.ID {
			return ErrSource
		}
		if version.Kind == "notebook" {
			books[relation.ID] = relation.ParentID
		} else {
			notes[relation.ID] = relation
		}
	}
	if err := validateNotebookParents(books); err != nil {
		return err
	}
	return validateNoteGroups(books, notes)
}

func validateNotebookParents(books map[string]*string) error {
	for id := range books {
		seen := map[string]bool{}
		for id != "" {
			if seen[id] {
				return ErrSource
			}
			seen[id] = true
			parent, ok := books[id]
			if !ok {
				return ErrSource
			}
			if parent == nil {
				break
			}
			id = *parent
		}
	}
	return nil
}

func validateNoteGroups(books map[string]*string, notes map[string]rowRelation) error {
	for id, note := range notes {
		if _, found := books[note.NotebookID]; !found {
			return ErrSource
		}
		seen := map[string]bool{}
		for id != "" {
			if seen[id] {
				return ErrSource
			}
			seen[id] = true
			current, found := notes[id]
			if !found || current.NotebookID != note.NotebookID {
				return ErrSource
			}
			if current.GroupID == nil {
				break
			}
			id = *current.GroupID
		}
	}
	return nil
}

// checkNoteRelations rejects a move or grouping change before publishing any
// content. In particular, moving a group cannot strand its children elsewhere.
func (r *Repository) checkNoteRelations(ctx context.Context, id, notebook string, group *string) error {
	versions, err := r.selected("note")
	if err != nil {
		return err
	}
	generacion, guardar := r.engine.VerifiedContentGeneration()
	rows := map[string]rowRelation{}
	for _, version := range versions {
		value, ok := rowRelation{}, false
		if guardar {
			value, ok = r.relaciones.leida(version, generacion)
		}
		if !ok {
			raw, err := r.readJSON(ctx, version)
			if err != nil {
				return err
			}
			err = json.Unmarshal(raw, &value)
			clear(raw)
			if err != nil {
				return ErrSource
			}
			if guardar {
				r.relaciones.guardar(version, generacion, value)
			}
		}
		rows[value.ID] = value
	}
	if guardar {
		r.relaciones.quedarse(versions)
	}
	rows[id] = rowRelation{ID: id, NotebookID: notebook, GroupID: group}
	books, err := r.notebooks(ctx)
	if err != nil {
		return err
	}
	parents := make(map[string]*string, len(books))
	for id, book := range books {
		parents[id] = book.ParentID
	}
	return validateNoteGroups(parents, rows)
}

// relacionesLeidas es lo que checkNoteRelations ya leyó de cada nota, por la referencia
// del índice de su revisión. Guardar una nota leía y descodificaba TODAS las demás para
// comprobar sus grupos (con 5.000 notas, la mitad de lo que costaba guardar, medido en
// la prueba de carga del almacén local). Solo se usa cuando el motor guarda lo ya
// comprobado (records.VerifiedContentGeneration: el almacén en claro de Sytue Office): ahí
// lo que lee una referencia no cambia mientras no cambie la generación, así que la
// relación descodificada es la misma; cuando cambia (el almacén se relee al compactar),
// se olvida todo. En la edición conectada todo es como siempre.
type relacionesLeidas struct {
	mu         sync.Mutex
	generacion uint64
	filas      map[storage.Reference]relacionLeida
}

// relacionLeida es la relación de una revisión y lo que se miró de su índice.
type relacionLeida struct {
	objeto    string
	metadatos []byte
	contenido storage.ContentPointer
	relacion  rowRelation
}

// leida devuelve la relación ya descodificada de version, si su índice es el mismo y la
// generación también.
func (m *relacionesLeidas) leida(version storage.ObjectVersion, generacion uint64) (rowRelation, bool) {
	if version.Commit.Content == nil {
		return rowRelation{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if generacion != m.generacion {
		m.generacion, m.filas = generacion, nil
		return rowRelation{}, false
	}
	f, ok := m.filas[version.Reference]
	if !ok || f.objeto != version.ObjectID || f.contenido != *version.Commit.Content || !bytes.Equal(f.metadatos, version.Commit.Metadata) {
		return rowRelation{}, false
	}
	return f.relacion, true
}

// guardar anota la relación descodificada de version, leída en la generación
// generacion.
func (m *relacionesLeidas) guardar(version storage.ObjectVersion, generacion uint64, relacion rowRelation) {
	if !version.Reference.Valid() || version.Commit.Content == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if generacion != m.generacion {
		m.generacion, m.filas = generacion, nil
	}
	if m.filas == nil {
		m.filas = map[storage.Reference]relacionLeida{}
	}
	m.filas[version.Reference] = relacionLeida{objeto: version.ObjectID, metadatos: slices.Clone(version.Commit.Metadata), contenido: *version.Commit.Content, relacion: relacion}
}

// quedarse suelta las revisiones que ya no son ninguna de versions (las versiones viejas
// de cada nota guardada), cuando son más que las que quedan.
func (m *relacionesLeidas) quedarse(versions []storage.ObjectVersion) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.filas) <= 2*len(versions) {
		return
	}
	vivas := make(map[storage.Reference]bool, len(versions))
	for _, v := range versions {
		vivas[v.Reference] = true
	}
	for ref := range m.filas {
		if !vivas[ref] {
			delete(m.filas, ref)
		}
	}
}
