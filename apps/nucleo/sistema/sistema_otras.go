// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android && !(darwin && cgo)

package sistema

// Fuera de Android y de los sistemas de Apple no hay sistema al que preguntar por este
// camino:
//
//   - Linux y Windows: el idioma viene del entorno POSIX, que ya mira `Idioma`. El tema
//     lo pinta Fyne con lo que sabe del escritorio (`tema.De`).
//   - Web: `cmd/web` se lo pregunta al navegador —`navigator.language`, `matchMedia`—,
//     que sabe más que nosotros y lo tiene delante.
//
// macOS e iOS están en `sistema_darwin.go`.
func delSistema() string { return "" }

func temaOscuro() (oscuro, sabido bool) { return false, false }
