// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios || android || js || !(darwin || windows || linux)

package ficheros

// ordenElegir: fuera del escritorio no hay un programa del sistema que abra un selector.
// En móvil lo da el propio sistema a la app, y eso no pasa por aquí.
func ordenElegir(string, []string) (string, []string, error) { return "", nil, ErrNoDisponible }
