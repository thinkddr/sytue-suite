// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build js

package altavoz

import "git.sytue.eu/sytue/one/apps/fyne/llamada"

// Sistema es el hueco de js/wasm: miniaudio es cgo y ese destino no tiene tarjeta (ver
// llamada_js.go).
type Sistema struct{ Micro, Altavoz string }

var _ llamada.Dispositivo = Sistema{}

// Abrir dice que aquí no hay llamada nativa.
func (Sistema) Abrir(_, _ func([]int16)) (func(), error) { return nil, llamada.ErrSinLlamadaNativa }

// Dispositivo es un micro o un altavoz (ninguno en js/wasm).
type Dispositivo struct{ ID, Nombre string }

// Dispositivos no hay.
func Dispositivos() (micros, altavoces []Dispositivo) { return nil, nil }
