// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !darwin && !linux && !windows && !android

package camara

// Sin implementación (js/wasm y lo que no sostenemos): no hay cámara.
func abrir(Pedido) (Fuente, error) { return nil, ErrSinCamara }
func dispositivos() []Dispositivo  { return nil }
