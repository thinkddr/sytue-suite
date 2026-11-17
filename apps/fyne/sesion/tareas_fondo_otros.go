// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android

package sesion

// En iOS la tarea la lanza la tarea de refresco del sistema (fondo.go) y en el escritorio
// no hay app cerrada que despertar: no hay nada que programar.
func programarTareaEnFondo(string, bool, bool) {}
