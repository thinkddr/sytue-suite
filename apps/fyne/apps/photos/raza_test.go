// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build race

package photos

// conRaza dice si los tests corren con `-race`. Ahí pintar una pantalla entera cuesta
// unas diez veces más, y los barridos de capturas —que buscan pánicos y tokens
// inventados, no carreras— se recortan para que el paquete quepa en los diez minutos
// por binario de `go test`.
const conRaza = true
