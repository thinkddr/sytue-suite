// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// RoomsFreeGif es una animación CC0 comprobada por Rooms. Los clientes no
// reciben la URL de descarga de Commons: el servicio controla su licencia.
type RoomsFreeGif struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Source  string `json:"source"`
	License string `json:"license"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Size    int64  `json:"size"`
}

// RoomsFreeGifPage es una única página; el cursor permite recorrer el índice.
type RoomsFreeGifPage struct {
	Items []RoomsFreeGif `json:"items"`
	Next  string         `json:"next,omitempty"`
}

// FreeGifs busca en el catálogo libre, sin enviar datos de la conversación.
func (r *Rooms) FreeGifs(ctx context.Context, query, cursor string) (RoomsFreeGifPage, error) {
	return PostJSON[RoomsFreeGifPage](ctx, r.c, "/gifs/search", map[string]string{"query": query, "cursor": cursor})
}

// FreeGifContent abre solo el original elegido o una miniatura estática.
func (r *Rooms) FreeGifContent(ctx context.Context, id int64, preview bool) (io.ReadCloser, error) {
	q := url.Values{}
	if preview {
		q.Set("preview", "1")
	}
	res, err := r.c.Stream(ctx, http.MethodGet, "/gifs/"+strconv.FormatInt(id, 10), q, nil, nil, 0)
	if err != nil {
		return nil, err
	}
	return res.Body, nil
}
