// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !(linux || darwin || windows)

package escritorio

func insignia(string, int) error { return nil }

// En el móvil nadie llega a preguntar.
func hayBandeja() bool { return true }

func avisar(Aviso) error { return ErrNoDisponible }
