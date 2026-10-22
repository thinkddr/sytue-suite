// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DocActividad es una línea de la auditoría de un documento (domain.EntradaAuditoria):
// un grupo de operaciones de una persona, del mismo tipo y clave, en el mismo minuto.
// Nunca trae el contenido de lo escrito.
type DocActividad struct {
	SeqDesde  int64     `json:"seq_desde"`
	SeqHasta  int64     `json:"seq_hasta"`
	AuthorSub string    `json:"author_sub"`
	Kind      string    `json:"kind"`
	Key       string    `json:"key"`
	Count     int       `json:"count"`
	At        time.Time `json:"at"`
	// Paths es dónde tocaron los parches del grupo (hoja/celda, diapositiva/caja).
	Paths []string `json:"paths"`
}

// Actividad es GET …/audit: quién cambió qué y cuándo, SOLO para el propietario (403 al
// resto). `limite` son las operaciones más recientes que resume (0 = las 200 de la web).
func (d *Docs) Actividad(ctx context.Context, id string, limite int) ([]DocActividad, error) {
	if limite <= 0 {
		limite = 200
	}
	out, err := JSON[struct {
		Entries []DocActividad `json:"entries"`
	}](ctx, d.c, Request{
		Path:  "/documents/" + url.PathEscape(id) + "/audit",
		Query: url.Values{"limit": {strconv.Itoa(limite)}},
	})
	return out.Entries, err
}

// maxRutasActividad: cuántas rutas se nombran en la línea; el resto, «…».
const maxRutasActividad = 3

// RutaCorta es una ruta de parche en pocas palabras: la celda (`B2`), la caja
// (`boxes·html`) o la propiedad de la hoja o la diapositiva (`freeze`), sin el id de la
// hoja, que es un UUID. Es `rutaCorta` de editor/state/actividad.ts.
func RutaCorta(ruta string) string {
	partes := strings.Split(ruta, "/")
	if partes[0] != "sheets" && partes[0] != "slides" {
		return ruta
	}
	var resto []string
	if len(partes) > 2 {
		resto = partes[2:]
	}
	switch {
	case len(resto) > 1 && resto[0] == "cells":
		return resto[1]
	case len(resto) > 0 && resto[0] == "boxes":
		cola := []string{"boxes"}
		if len(resto) > 2 {
			cola = append(cola, resto[2:]...)
		}
		return strings.Join(cola, "·")
	case len(resto) > 0:
		return strings.Join(resto, "/")
	}
	return partes[0]
}

// QueToco es qué tocó el grupo: las celdas o cajas de sus parches, o la clave (o el tipo).
func (a DocActividad) QueToco() string {
	var cortas []string
	vistas := map[string]bool{}
	for _, p := range a.Paths {
		if c := RutaCorta(p); !vistas[c] {
			vistas[c] = true
			cortas = append(cortas, c)
		}
	}
	if len(cortas) == 0 {
		if a.Key != "" {
			return a.Key
		}
		return a.Kind
	}
	out := strings.Join(cortas[:min(len(cortas), maxRutasActividad)], ", ")
	if len(cortas) > maxRutasActividad {
		out += ", …"
	}
	return out
}

// Descripcion es la línea de la web: «quien — qué ×n» (la fecha la pone quien la pinta).
func (a DocActividad) Descripcion() string {
	quien := a.AuthorSub
	if len(quien) > 8 {
		quien = quien[:8]
	}
	return quien + " — " + a.QueToco() + " ×" + strconv.Itoa(a.Count)
}
