// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !windows

package ocrwindows

import "context"

// Fuera de Windows no hay OCR de Windows: Disponible da false y lo demás, ErrNoDisponible.

func consultar() (consulta, error) { return consulta{}, ErrNoDisponible }

func reconocer(context.Context, []byte, int, int, string) ([]palabraCruda, error) {
	return nil, ErrNoDisponible
}
