// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios || android || js || !(darwin || windows || linux)

package ficheros

func elegirAplicacion(string) (string, error) { return "", ErrNoDisponible }

func abrirCon(string, string) error { return ErrNoDisponible }
