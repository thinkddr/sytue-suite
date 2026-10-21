// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import "context"

// El catálogo de imágenes de la ORGANIZACIÓN (/api/emoji de la web): los emoji propios
// y los GIF, que son lo mismo —una imagen con nombre—. La imagen se sube antes a la raíz
// de Drive; aquí solo se liga el atajo al fichero.

// RoomsCustomEmoji es una imagen del catálogo, que se escribe `:nombre:`.
type RoomsCustomEmoji struct {
	Name        string `json:"name"`
	FileID      string `json:"file_id"`
	ContentType string `json:"content_type,omitempty"`
}

// CustomEmoji es el catálogo entero.
func (r *Rooms) CustomEmoji(ctx context.Context) ([]RoomsCustomEmoji, error) {
	out, err := JSON[struct {
		Emoji []RoomsCustomEmoji `json:"emoji"`
	}](ctx, r.c, Request{Path: "/emoji"})
	return out.Emoji, err
}

// CreateCustomEmoji liga el atajo `nombre` a la imagen `fichero` de Drive; el servicio
// comprueba que sea una imagen a la que se tiene acceso.
func (r *Rooms) CreateCustomEmoji(ctx context.Context, nombre, fichero string) (RoomsCustomEmoji, error) {
	return PostJSON[RoomsCustomEmoji](ctx, r.c, "/emoji", map[string]string{"name": nombre, "file_id": fichero})
}
