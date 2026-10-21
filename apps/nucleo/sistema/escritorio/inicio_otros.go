// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !(linux || darwin || windows) || android || ios

package escritorio

// En el móvil el sistema decide qué arranca; no hay lista de inicio que tocar.
func alIniciarSesion(string, string, string, bool) error { return ErrNoDisponible }

func arrancaAlIniciarSesion(string) bool { return false }
