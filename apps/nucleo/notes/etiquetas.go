// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/notes/model"
)

var _ api.NotesTagsData = (*Repository)(nil)

// Las etiquetas con estados en el almacenamiento privado tienen las tres filas que
// exporta el servicio: `tag` (el eje), `tag_state` (cada valor) y `tag_mark` (el valor
// marcado en una nota, una por nota y etiqueta: los estados de una etiqueta son
// excluyentes). Quitar es mandar la fila a la papelera.

// marca es una fila `tag_mark`.
type marca struct {
	NoteID  string `json:"note_id"`
	TagID   string `json:"tag_id"`
	StateID string `json:"state_id"`
}

// SaveTag crea una etiqueta (`id` vacío) o la sustituye con TODOS sus estados: el que no
// venga se borra, con sus marcas.
func (r *Repository) SaveTag(parent context.Context, id string, in api.TagInput) (api.Tag, error) {
	var norm model.TagInput
	if err := recodificar(in, &norm); err != nil {
		return api.Tag{}, ErrSource
	}
	if err := norm.Normalize(); err != nil {
		return api.Tag{}, err
	}
	ctx, done, err := r.start(parent)
	if err != nil {
		return api.Tag{}, err
	}
	defer done()
	tags, err := r.ListTags(ctx)
	if err != nil {
		return api.Tag{}, err
	}
	ahora := time.Now().UTC()
	var actual api.Tag
	if id == "" {
		id = uuid.NewString()
		actual = api.Tag{ID: id, Position: len(tags)}
		// Primero el eje: un estado sin su etiqueta es un catálogo roto.
		if err := r.reescribir(ctx, "create-tag", "tag", id, map[string]any{
			"id": id, "owner_sub": r.cfg.Scope.Subject, "name": norm.Name, "position": actual.Position,
			"created_at": ahora, "updated_at": ahora,
		}, true); err != nil {
			return api.Tag{}, err
		}
	} else {
		i := slices.IndexFunc(tags, func(t api.Tag) bool { return t.ID == id })
		if i < 0 {
			return api.Tag{}, ErrNotFound
		}
		actual = tags[i]
		if err := r.reescribir(ctx, "update-tag", "tag", id, map[string]any{"name": norm.Name, "updated_at": ahora}, false); err != nil {
			return api.Tag{}, err
		}
	}
	if err := r.guardarEstados(ctx, id, in.States, norm.States, actual, ahora); err != nil {
		return api.Tag{}, err
	}
	tags, err = r.ListTags(ctx)
	if err != nil {
		return api.Tag{}, err
	}
	for _, t := range tags {
		if t.ID == id {
			return t, nil
		}
	}
	return api.Tag{}, ErrNotFound
}

// guardarEstados escribe, en su orden, los estados de la etiqueta id (los de entrada, ya
// normalizados en norm) y borra, con sus marcas, los de actual que ya no vienen.
func (r *Repository) guardarEstados(ctx context.Context, id string, entrada []api.TagStateInput, norm []model.TagStateInput, actual api.Tag, ahora time.Time) error {
	quedan := map[string]bool{}
	for i, s := range entrada {
		sid := ""
		if s.ID != nil && *s.ID != "" {
			sid = *s.ID
			if !slices.ContainsFunc(actual.States, func(e api.TagState) bool { return e.ID == sid }) {
				return ErrNotFound
			}
		} else {
			sid = uuid.NewString()
		}
		quedan[sid] = true
		ns := norm[i]
		if err := r.reescribir(ctx, "save-tag-state", "tag_state", sid, map[string]any{
			"id": sid, "tag_id": id, "name": ns.Name, "emoji": ns.Emoji, "color": ns.Color,
			"shortcut": ns.Shortcut, "position": i, "updated_at": ahora,
		}, true); err != nil {
			return err
		}
	}
	for _, e := range actual.States {
		if !quedan[e.ID] {
			if err := r.quitarEstado(ctx, e.ID, ahora); err != nil {
				return err
			}
		}
	}
	return nil
}

// quitarEstado borra un estado y, antes, las marcas que lo llevan.
func (r *Repository) quitarEstado(ctx context.Context, estado string, ahora time.Time) error {
	if err := r.quitarMarcas(ctx, func(m marca) bool { return m.StateID == estado }, ahora); err != nil {
		return err
	}
	return r.reescribir(ctx, "delete-tag-state", "tag_state", estado, borrado(ahora), false)
}

func (r *Repository) quitarMarcas(ctx context.Context, cual func(marca) bool, ahora time.Time) error {
	var ids []string
	err := r.eachRow(ctx, "tag_mark", func(raw []byte, state string) error {
		var m marca
		if json.Unmarshal(raw, &m) != nil {
			return ErrSource
		}
		if state == "live" && cual(m) {
			ids = append(ids, idCompuesto(m.NoteID, m.TagID))
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := r.reescribir(ctx, "delete-tag-mark", "tag_mark", id, borrado(ahora), false); err != nil {
			return err
		}
	}
	return nil
}

// DeleteTag borra una etiqueta, sus estados y sus marcas en todas las notas, en ese
// orden inverso: lo último que se borra es el eje.
func (r *Repository) DeleteTag(parent context.Context, id string) error {
	ctx, done, err := r.start(parent)
	if err != nil {
		return err
	}
	defer done()
	tags, err := r.ListTags(ctx)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(tags, func(t api.Tag) bool { return t.ID == id })
	if i < 0 {
		return ErrNotFound
	}
	ahora := time.Now().UTC()
	if err := r.quitarMarcas(ctx, func(m marca) bool { return m.TagID == id }, ahora); err != nil {
		return err
	}
	for _, s := range tags[i].States {
		if err := r.reescribir(ctx, "delete-tag-state", "tag_state", s.ID, borrado(ahora), false); err != nil {
			return err
		}
	}
	return r.reescribir(ctx, "delete-tag", "tag", id, borrado(ahora), false)
}

// SetNoteState marca un estado de etiqueta en una nota; con `estado` vacío lo quita.
func (r *Repository) SetNoteState(parent context.Context, nota, etiqueta, estado string) error {
	ctx, done, err := r.start(parent)
	if err != nil {
		return err
	}
	defer done()
	if _, _, err := r.notaViva(ctx, nota); err != nil {
		return err
	}
	tags, err := r.ListTags(ctx)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(tags, func(t api.Tag) bool { return t.ID == etiqueta })
	if i < 0 {
		return ErrNotFound
	}
	id := idCompuesto(nota, etiqueta)
	ahora := time.Now().UTC()
	if estado == "" {
		var m marca
		ok, err := r.filaViva(ctx, "tag_mark", id, &m)
		if err != nil || !ok {
			return err
		}
		return r.reescribir(ctx, "delete-tag-mark", "tag_mark", id, borrado(ahora), false)
	}
	if !slices.ContainsFunc(tags[i].States, func(s api.TagState) bool { return s.ID == estado }) {
		return ErrNotFound
	}
	return r.reescribir(ctx, "set-tag-mark", "tag_mark", id, map[string]any{
		"note_id": nota, "tag_id": etiqueta, "state_id": estado, "deleted_at": nil, "created_at": ahora,
	}, true)
}

// estadosDeNotas pone en cada nota sus estados de etiqueta, con el nombre y el color
// dentro, como los manda el servicio. Las marcas son la verdad: lo que la nota traiga
// en su propio JSON se sustituye.
func (r *Repository) estadosDeNotas(ctx context.Context, notes map[string]api.Note) error {
	for id, note := range notes {
		note.States = nil
		notes[id] = note
	}
	tags := map[string]api.Tag{}
	if err := r.eachLiveJSON(ctx, "tag", func(raw []byte) error {
		var t api.Tag
		if json.Unmarshal(raw, &t) != nil || t.ID == "" {
			return ErrSource
		}
		tags[t.ID] = t
		return nil
	}); err != nil {
		return err
	}
	if len(tags) == 0 {
		return nil
	}
	estados := map[string]api.TagState{}
	if err := r.eachLiveJSON(ctx, "tag_state", func(raw []byte) error {
		var s api.TagState
		if json.Unmarshal(raw, &s) != nil || s.ID == "" {
			return ErrSource
		}
		estados[s.ID] = s
		return nil
	}); err != nil {
		return err
	}
	return r.eachLiveJSON(ctx, "tag_mark", func(raw []byte) error {
		var m marca
		if json.Unmarshal(raw, &m) != nil {
			return ErrSource
		}
		note, ok := notes[m.NoteID]
		t, okT := tags[m.TagID]
		s, okS := estados[m.StateID]
		if !ok || !okT || !okS || s.TagID != t.ID {
			return nil
		}
		note.States = append(note.States, api.NoteState{TagID: t.ID, TagName: t.Name, StateID: s.ID, Name: s.Name, Emoji: s.Emoji, Color: s.Color})
		slices.SortFunc(note.States, func(a, b api.NoteState) int { return cmp.Compare(a.TagID, b.TagID) })
		notes[m.NoteID] = note
		return nil
	})
}
