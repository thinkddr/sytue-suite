// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !((darwin && !ios) || (linux && !android) || windows)

package voz

// leer no existe aquí: ver el comentario del paquete.
func leer(string, string, func()) (func(), error) { return nil, ErrNoDisponible }
