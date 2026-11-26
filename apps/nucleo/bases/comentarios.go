// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/bases/dominio"
)

// Comentarios de un registro (B-04): notas de quien usa la base, una fila `comment` por
// comentario, con las reglas del servicio (texto limpio, hilos de un nivel, tope por
// registro y lápida para una raíz borrada que tiene respuestas). Con una sola persona,
// quien mira puede borrar todos.

func (r *Repository) comentariosDe(ctx context.Context, registro string) ([]api.BasesComment, error) {
	todos, err := leerTodos[api.BasesComment](ctx, r, "comment")
	if err != nil {
		return nil, err
	}
	var out []api.BasesComment
	for _, c := range todos {
		if c.RecordID == registro {
			out = append(out, c)
		}
	}
	slices.SortFunc(out, func(a, b api.BasesComment) int {
		if n := a.CreatedAt.Compare(b.CreatedAt); n != 0 {
			return n
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}

func (r *Repository) ListComments(ctx context.Context, registro string) ([]api.BasesComment, error) {
	if _, err := leerUno[api.BasesRecord](ctx, r, "record", registro); err != nil {
		return nil, err
	}
	cs, err := r.comentariosDe(ctx, registro)
	if err != nil {
		return nil, err
	}
	out := []api.BasesComment{}
	for _, c := range cs {
		c.CanDelete = !c.Deleted
		out = append(out, c)
	}
	return out, nil
}

func (r *Repository) CreateComment(ctx context.Context, registro, cuerpo, padre string) (api.BasesComment, error) {
	texto, err := dominio.LimpiarComentario(cuerpo)
	if err != nil {
		return api.BasesComment{}, errorDe(err)
	}
	if _, err := leerUno[api.BasesRecord](ctx, r, "record", registro); err != nil {
		return api.BasesComment{}, err
	}
	entrada := struct {
		Registro string `json:"record"`
		Padre    string `json:"parent"`
		Texto    string `json:"body"`
	}{registro, padre, texto}
	c, err := crear(ctx, r, "comment-create", "comment", entrada, func(ctx context.Context) (string, api.BasesComment, error) {
		cs, err := r.comentariosDe(ctx, registro)
		if err != nil {
			return "", api.BasesComment{}, err
		}
		if len(cs) >= dominio.MaxComentariosPorRegistro {
			return "", api.BasesComment{}, errorDe(dominio.ErrLimite)
		}
		if padre != "" {
			// Solo se responde a una raíz: los hilos tienen un nivel.
			i := slices.IndexFunc(cs, func(c api.BasesComment) bool { return c.ID == padre })
			if i < 0 || cs[i].ParentID != "" {
				return "", api.BasesComment{}, errorDe(dominio.ErrInvalido)
			}
		}
		c := api.BasesComment{ID: uuid.NewString(), RecordID: registro, ParentID: padre, AuthorSub: r.scope.Subject, Body: texto, CreatedAt: time.Now().UTC()}
		return c.ID, c, nil
	})
	if err != nil {
		return api.BasesComment{}, err
	}
	c.CanDelete = true
	return c, nil
}

// DeleteComment borra un comentario. Una raíz con respuestas se queda como lápida (sin
// texto ni autor) para que el hilo no pierda su sitio.
func (r *Repository) DeleteComment(ctx context.Context, comentario string) error {
	c, err := leerUno[api.BasesComment](ctx, r, "comment", comentario)
	if err != nil {
		return err
	}
	cs, err := r.comentariosDe(ctx, c.RecordID)
	if err != nil {
		return err
	}
	respuestas := slices.ContainsFunc(cs, func(x api.BasesComment) bool { return x.ParentID == comentario })
	if !respuestas {
		if err := r.trashRow(ctx, "comment-delete", "comment", comentario); err != nil {
			return err
		}
		// Una lápida que se queda sin respuestas ya no hace falta.
		if c.ParentID != "" {
			hermanas := slices.ContainsFunc(cs, func(x api.BasesComment) bool { return x.ParentID == c.ParentID && x.ID != comentario })
			if i := slices.IndexFunc(cs, func(x api.BasesComment) bool { return x.ID == c.ParentID }); i >= 0 && cs[i].Deleted && !hermanas {
				return r.trashRow(ctx, "comment-delete", "comment", c.ParentID)
			}
		}
		return nil
	}
	_, err = cambiar(ctx, r, "comment-tombstone", "comment", comentario, comentario, func(x *api.BasesComment) error {
		x.Deleted, x.Body, x.AuthorSub, x.AuthorName = true, "", "", ""
		return nil
	})
	return err
}

var _ api.BasesCommentData = (*Repository)(nil)
