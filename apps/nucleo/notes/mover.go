// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

var _ api.NotesMoveData = (*Repository)(nil)

// nodo es lo que hace falta de una nota para moverla con lo que cuelga de ella.
type nodo struct {
	ID         string     `json:"id"`
	NotebookID string     `json:"notebook_id"`
	GroupID    *string    `json:"group_id"`
	Kind       string     `json:"kind"`
	Cipher     string     `json:"content_cipher"`
	Position   int        `json:"position"`
	Owner      string     `json:"owner_sub"`
	Checked    bool       `json:"checked"`
	Cancelled  *time.Time `json:"cancelled_at"`
	Created    time.Time  `json:"created_at"`
	Deleted    *time.Time `json:"deleted_at"`
	PurgedAt   *time.Time `json:"purged_at"`
}

// nodos son todas las notas, vivas o en la papelera: las de la papelera también apuntan
// a su grupo, y dejarlas atrás rompería la relación.
func (r *Repository) nodos(ctx context.Context) (map[string]nodo, error) {
	out := map[string]nodo{}
	err := r.eachRow(ctx, "note", func(raw []byte, _ string) error {
		var n nodo
		if json.Unmarshal(raw, &n) != nil || n.ID == "" {
			return ErrSource
		}
		out[n.ID] = n
		return nil
	})
	return out, err
}

// rama es la nota raíz y sus descendientes, del padre a los hijos (anchura primero).
func rama(todas map[string]nodo, raiz string) []nodo {
	hijos := map[string][]nodo{}
	for _, n := range todas {
		if n.GroupID != nil {
			hijos[*n.GroupID] = append(hijos[*n.GroupID], n)
		}
	}
	for k := range hijos {
		slices.SortFunc(hijos[k], func(a, b nodo) int { return strings.Compare(a.ID, b.ID) })
	}
	out := []nodo{todas[raiz]}
	visto := map[string]bool{raiz: true}
	for i := 0; i < len(out) && i < 4096; i++ {
		for _, h := range hijos[out[i].ID] {
			if !visto[h.ID] {
				visto[h.ID] = true
				out = append(out, h)
			}
		}
	}
	return out
}

// moverRama lleva la nota `raiz`, con todo lo que cuelga de ella, al cuaderno `destino`,
// al final y en la primera columna. `extra` son los cambios de la raíz (el traslado del
// diario apunta de dónde salió).
//
// En tres pasadas para que el catálogo sea válido después de CADA escritura (las notas
// de un grupo comparten cuaderno): primero se sueltan los descendientes de su padre sin
// cambiarlos de cuaderno, luego se mueve cada nota, y al final se vuelven a agrupar, del
// padre a los hijos. Un corte a mitad deja notas sueltas, nunca un grupo partido.
func (r *Repository) moverRama(ctx context.Context, raiz, destino string, posicion int, extra map[string]any) error {
	todas, err := r.nodos(ctx)
	if err != nil {
		return err
	}
	if _, ok := todas[raiz]; !ok {
		return ErrNotFound
	}
	nodos := rama(todas, raiz)
	padres := map[string]*string{}
	for _, n := range nodos[1:] {
		padres[n.ID] = n.GroupID
		if err := r.reescribir(ctx, "move-ungroup", "note", n.ID, map[string]any{"group_id": nil}, false); err != nil {
			return err
		}
	}
	ahora := time.Now().UTC()
	for i, n := range nodos {
		cambios := map[string]any{"notebook_id": destino, "group_id": nil, "updated_at": ahora}
		if i == 0 {
			cambios["col"], cambios["position"] = 0, posicion
			for k, v := range extra {
				cambios[k] = v
			}
		}
		if err := r.reescribir(ctx, "move-note", "note", n.ID, cambios, false); err != nil {
			return err
		}
	}
	for _, n := range nodos[1:] {
		if err := r.reescribir(ctx, "move-regroup", "note", n.ID, map[string]any{"group_id": padres[n.ID]}, false); err != nil {
			return err
		}
	}
	return nil
}

// finalDe es la posición tras la última nota viva del cuaderno.
func finalDe(todas map[string]nodo, cuaderno string) int {
	fin := 0
	for _, n := range todas {
		if n.NotebookID == cuaderno && n.Deleted == nil {
			fin = max(fin, n.Position)
		}
	}
	return fin
}

// MoveNoteToNotebook cambia la nota de cuaderno con las que tenga agrupadas; cae al final
// del destino. Ni a un cuaderno cifrado ni desde uno: el contenido no se recifra aquí.
func (r *Repository) MoveNoteToNotebook(parent context.Context, id, cuaderno string) error {
	ctx, done, err := r.start(parent)
	if err != nil {
		return err
	}
	defer done()
	note, origen, err := r.notaViva(ctx, id)
	if err != nil {
		return err
	}
	destino, err := r.GetNotebook(ctx, cuaderno)
	if err != nil {
		return err
	}
	if origen.Encrypted || destino.Encrypted || note.ContentCipher != "" {
		return ErrConflict
	}
	if note.NotebookID == cuaderno {
		return nil
	}
	todas, err := r.nodos(ctx)
	if err != nil {
		return err
	}
	return r.moverRama(ctx, id, cuaderno, finalDe(todas, cuaderno)+1, nil)
}

// MoveNote coloca una nota dentro de SU cuaderno: arrastrarla, cambiarla de columna,
// meterla dentro de otra o sacarla de su grupo.
func (r *Repository) MoveNote(parent context.Context, id string, p api.NotePosition) error {
	ctx, done, err := r.start(parent)
	if err != nil {
		return err
	}
	defer done()
	note, _, err := r.notaViva(ctx, id)
	if err != nil {
		return err
	}
	if p.GroupID != nil && *p.GroupID == id {
		return ErrSource
	}
	if err := r.checkNoteRelations(ctx, id, note.NotebookID, p.GroupID); err != nil {
		return err
	}
	return r.reescribir(ctx, "position-note", "note", id, map[string]any{
		"x": p.X, "y": p.Y, "col": p.Col, "position": p.Position, "group_id": p.GroupID,
		"updated_at": time.Now().UTC(),
	}, false)
}

// ArchiveNotebook archiva el cuaderno con su rama (true) o la devuelve al árbol (false).
// Un diario no se archiva: es el día, no un tema.
func (r *Repository) ArchiveNotebook(parent context.Context, id string, archivar bool) error {
	ctx, done, err := r.start(parent)
	if err != nil {
		return err
	}
	defer done()
	nb, err := r.GetNotebook(ctx, id)
	if err != nil {
		return err
	}
	if nb.DailyDate != "" {
		return ErrSource
	}
	books, err := r.notebooks(ctx)
	if err != nil {
		return err
	}
	var valor any
	if archivar {
		valor = time.Now().UTC()
	}
	// La raíz y todo lo que cuelga de ella, del padre a los hijos.
	cola := []string{id}
	for i := 0; i < len(cola) && i < 4096; i++ {
		for _, b := range books {
			if b.ParentID != nil && *b.ParentID == cola[i] && !slices.Contains(cola, b.ID) {
				cola = append(cola, b.ID)
			}
		}
	}
	for _, b := range cola {
		if err := r.reescribir(ctx, "archive-notebook", "notebook", b, map[string]any{"archived_at": valor}, false); err != nil {
			return err
		}
	}
	return nil
}

// PurgeNote borra para siempre una nota de la papelera: su versión nueva se queda sin
// título, contenido, datos ni enlaces, con `purged_at`, y la papelera deja de listarla.
// La fila no desaparece (otras notas de la papelera pueden apuntarla como grupo).
//
// ponytail: el historial del almacén en claro conserva las versiones anteriores hasta
// que se compacte (§5.6); el borrado es el de lo que se ve, no el del disco.
func (r *Repository) PurgeNote(parent context.Context, id string) error {
	ctx, done, err := r.start(parent)
	if err != nil {
		return err
	}
	defer done()
	version, err := r.readyVersion("note", id)
	if err != nil {
		return err
	}
	if version.Commit.State != "trash" {
		return ErrNotFound
	}
	ahora := time.Now().UTC()
	return r.reescribir(ctx, "purge-note", "note", id, map[string]any{
		"title": "", "content": "", "content_cipher": "", "data": nil, "links": nil, "states": nil,
		"purged_at": ahora, "updated_at": ahora,
	}, false)
}
