// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !unix && !windows

package archivos

// espacioDisco no se sabe en este sistema: Quota falla y la pantalla no pinta el medidor.
func espacioDisco(string) (int64, int64, error) { return 0, 0, ErrNoDisponible }
