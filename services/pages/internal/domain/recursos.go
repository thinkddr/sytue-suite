// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrRecursoEnUso se devuelve cuando se intenta borrar un asset referenciado en el sitio.
	ErrRecursoEnUso = errors.New("pages: recurso en uso")
	// ErrRecursoPublicado: la versión publicada lo usa; borrarlo pide confirmación (WEB-04).
	ErrRecursoPublicado = errors.New("pages: recurso de la versión publicada")
	// ErrRecursoInvalido se devuelve si el formato, tamaño o nombre del asset es inválido.
	ErrRecursoInvalido = errors.New("pages: recurso inválido")
)

// Recurso modela un asset alojado para un sitio web.
type Recurso struct {
	ID        uuid.UUID `json:"id"`
	SiteID    uuid.UUID `json:"site_id"`
	File      string    `json:"file"`
	Ext       string    `json:"ext"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	Bytes     int64     `json:"bytes"`
	Variants  []int     `json:"variants"`
	Origin    string    `json:"origin"`
	Name      string    `json:"name"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// Ficheros son los nombres con que el recurso vive en el almacén, bajo
// `shared/sitios/<sitio>/recursos/`: el maestro y una copia por ancho. Los sirve
// `pages sitios` en `/_astro/<nombre>` sin mirar la tabla, así que borrar el recurso
// es borrar TODOS (WEB-04).
func (r Recurso) Ficheros() []string {
	out := make([]string, 0, 1+len(r.Variants))
	out = append(out, r.File+"."+r.Ext)
	for _, ancho := range r.Variants {
		out = append(out, fmt.Sprintf("%s-%d.%s", r.File, ancho, r.Ext))
	}
	return out
}
