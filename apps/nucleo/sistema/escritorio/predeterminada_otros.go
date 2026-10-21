// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !windows && (!darwin || ios)

package escritorio

// En Linux lo hace el instalador (`instalar.sh`); en el móvil elige el sistema.
func predeterminada(string, []string) error { return ErrNoDisponible }
