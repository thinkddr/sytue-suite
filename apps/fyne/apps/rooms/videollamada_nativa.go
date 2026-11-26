// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js && !edicionlocal

package rooms

import "git.sytue.eu/sytue/one/apps/fyne/llamada"

// La llamada nativa cumple EnCurso. Solo donde existe: en js/wasm y en la edición local
// (`edicionlocal`), `llamada` es su API vacía y no tiene el tipo.
var _ EnCurso = (*llamada.Llamada)(nil)
