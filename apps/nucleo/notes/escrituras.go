// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// Piezas de escritura de las rutas que antes solo tenía el servicio (diario, tareas,
// etiquetas, recordatorios, mover y archivar): PLAN-EDICION-LOCAL.md §6.5 N-03. Cada
// cambio es UNA fila publicada por el motor de registros, como el resto del repositorio;
// lo que en el servicio era una transacción (mover una nota con lo que cuelga de ella)
// aquí es una serie de escrituras ordenada para que cada estado intermedio siga siendo
// un catálogo válido.

// estadoDe es el estado que el motor exige a una fila: `trash` si tiene `deleted_at`.
func estadoDe(fields map[string]json.RawMessage) string {
	if v, ok := fields["deleted_at"]; ok && string(v) != "null" {
		return "trash"
	}
	return "live"
}

// reescribir publica una versión nueva de la fila `kind/id` con `cambios` sobre lo que
// ya tiene (los campos que no conoce se conservan tal cual). `nueva` crea la fila si no
// existe con esos campos; si no, una fila ausente es ErrNotFound.
func (r *Repository) reescribir(ctx context.Context, op, kind, id string, cambios map[string]any, nueva bool) error {
	raw, err := r.mutate(ctx, op, struct {
		Kind, ID string
		Cambios  map[string]any
		Nueva    bool
	}{kind, id, cambios, nueva}, func(ctx context.Context) (string, string, string, []byte, error) {
		fields := map[string]json.RawMessage{}
		old, err := r.ReadRaw(ctx, kind, id)
		switch {
		case err == nil:
			defer clear(old)
			if fields, err = rawObject(old); err != nil {
				return "", "", "", nil, err
			}
		case errors.Is(err, records.ErrNotFound) && nueva:
		default:
			return "", "", "", nil, err
		}
		for k, v := range cambios {
			if err := put(fields, k, v); err != nil {
				return "", "", "", nil, err
			}
		}
		encoded, err := json.Marshal(fields)
		return kind, id, estadoDe(fields), encoded, err
	})
	clear(raw)
	return err
}

// filaViva lee una fila viva y la decodifica en v. false: no existe o está en la papelera.
func (r *Repository) filaViva(ctx context.Context, kind, id string, v any) (bool, error) {
	version, err := r.readyVersion(kind, id)
	if errors.Is(err, records.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if version.Commit.State != "live" {
		return false, nil
	}
	raw, err := r.readJSON(ctx, version)
	if err != nil {
		return false, err
	}
	defer clear(raw)
	if json.Unmarshal(raw, v) != nil {
		return false, ErrSource
	}
	return true, nil
}

// notaViva es una nota viva de un cuaderno visible.
func (r *Repository) notaViva(ctx context.Context, id string) (api.Note, api.Notebook, error) {
	var note api.Note
	ok, err := r.filaViva(ctx, "note", id, &note)
	if err != nil {
		return api.Note{}, api.Notebook{}, err
	}
	if !ok || note.ID != id {
		return api.Note{}, api.Notebook{}, ErrNotFound
	}
	nb, err := r.GetNotebook(ctx, note.NotebookID)
	if err != nil {
		return api.Note{}, api.Notebook{}, err
	}
	return note, nb, nil
}

// borrado son los campos que mandan una fila auxiliar a la papelera.
func borrado(ahora time.Time) map[string]any {
	return map[string]any{"deleted_at": ahora, "updated_at": ahora}
}

// idCompuesto es el identificador de las filas que el servicio indexa por un par
// (marca de etiqueta: nota y etiqueta; recordatorio: nota y persona), el mismo que usa
// al exportarlas: sha256 en hexadecimal de «a:b».
func idCompuesto(a, b string) string {
	sum := sha256.Sum256([]byte(a + ":" + b))
	return hex.EncodeToString(sum[:])
}

// eachRow recorre las filas de un tipo en cualquier estado, con el suyo.
func (r *Repository) eachRow(ctx context.Context, kind string, visit func(raw []byte, state string) error) error {
	versions, err := r.selected(kind)
	if err != nil {
		return err
	}
	for _, v := range versions {
		if err := ctx.Err(); err != nil {
			return err
		}
		raw, err := r.readJSON(ctx, v)
		if err != nil {
			return err
		}
		err = visit(raw, v.Commit.State)
		clear(raw)
		if err != nil {
			return err
		}
	}
	return nil
}
