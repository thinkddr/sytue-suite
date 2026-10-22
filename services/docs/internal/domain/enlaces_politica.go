// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"time"
)

// Techo de caducidad de los enlaces de compartición (docs/security.md#matriz-acl).
//
// Cada organización tiene un máximo de días de vida para sus enlaces. Sin decisión de
// quien la administra son TechoEnlacesPorDefecto; 0 es «sin techo», el único modo de
// crear enlaces sin caducidad.
const (
	TechoEnlacesPorDefecto = 365
	TechoEnlacesMaximo     = 3650
	// GraciaEnlacesDias: al bajar el techo, un enlace que ya existía no caduca antes de
	// estos días (o del propio techo, si es menor): avisa, no rompe.
	GraciaEnlacesDias = 30
)

var (
	// ErrCaducidadPasada: se pidió una caducidad que ya pasó.
	ErrCaducidadPasada = errors.New("docs: la caducidad del enlace ya ha pasado")
	// ErrCaducidadExcedida: se pidió una caducidad más allá del techo de la organización.
	ErrCaducidadExcedida = errors.New("docs: la caducidad del enlace supera el máximo de la organización")
	// ErrTechoInvalido: el techo no está entre 0 y TechoEnlacesMaximo.
	ErrTechoInvalido = errors.New("docs: máximo de días de los enlaces fuera de rango")
)

// PoliticaEnlaces es el techo vigente de una organización.
type PoliticaEnlaces struct {
	// MaxDias es el máximo de días de vida de un enlace; 0 = sin techo.
	MaxDias int `json:"max_days"`
	// PorDefecto: nadie la ha fijado todavía (vale TechoEnlacesPorDefecto).
	PorDefecto bool `json:"is_default"`
}

// CaducidadDeEnlace aplica el techo a la caducidad pedida para un enlace NUEVO.
// Sin fecha, caduca en el techo (o nunca, solo si la organización no tiene techo). Una
// fecha pasada o más allá del techo se rechaza: no se recorta en silencio, porque quien
// la pide creería tener un enlace que dura lo que dijo.
func CaducidadDeEnlace(ahora time.Time, pedida *time.Time, maxDias int) (*time.Time, error) {
	var techo time.Time
	if maxDias > 0 {
		techo = ahora.Add(time.Duration(maxDias) * 24 * time.Hour)
	}
	if pedida == nil {
		if maxDias <= 0 {
			return nil, nil //nolint:nilnil // sin techo y sin fecha: enlace sin caducidad, permitido por la organización.
		}
		return &techo, nil
	}
	if !pedida.After(ahora) {
		return nil, ErrCaducidadPasada
	}
	// Un minuto de holgura: la interfaz calcula «hoy + techo» con su reloj.
	if maxDias > 0 && pedida.After(techo.Add(time.Minute)) {
		return nil, ErrCaducidadExcedida
	}
	return pedida, nil
}
