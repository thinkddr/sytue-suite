// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !windows

package mediafoundation

import (
	"io"
	"time"
)

// Disponible: Media Foundation solo está en Windows.
func Disponible() bool { return false }

// Lector fuera de Windows no se puede abrir.
type Lector struct{}

// Abrir fuera de Windows es ErrNoDisponible.
func Abrir(string) (*Lector, error) { return nil, ErrNoDisponible }

// Info no dice nada.
func (*Lector) Info() Info { return Info{} }

// Siguiente se acaba enseguida.
func (*Lector) Siguiente() (Muestra, error) { return Muestra{}, io.EOF }

// Buscar no puede.
func (*Lector) Buscar(time.Duration) (time.Duration, error) { return 0, ErrNoDisponible }

// Close no tiene nada que cerrar.
func (*Lector) Close() error { return nil }
