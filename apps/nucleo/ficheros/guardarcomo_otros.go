// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios || android || js || !(darwin || windows || linux)

package ficheros

// ordenGuardarComo: fuera del escritorio no hay programa que lanzar; en el móvil lo da el
// toolkit ([GuardarEnElMovil]).
func ordenGuardarComo(string, string, []string) (string, []string, error) {
	return "", nil, ErrNoDisponible
}
