// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !(darwin && !ios) && !windows && !linux

package camara

// En iOS compartir la pantalla del teléfono pide una extensión de ReplayKit (Broadcast
// Upload), que es otro binario dentro del IPA y `fyne package` no sabe construir; la
// captura sin extensión solo ve la propia app. Ver docs/roadmap.md, «Llamadas nativas».
// PantallaDisponible indica que este destino implementa la captura; los permisos
// y el acceso al equipo se comprueban al abrirla.
const PantallaDisponible = false

func abrirPantalla() (Fuente, error) { return nil, ErrSinPantalla }
