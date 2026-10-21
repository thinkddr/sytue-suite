// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build race

package cast

// conRaza dice si los tests corren con `-race`, como en Photos: pintar una pantalla
// entera cuesta ahí unas diez veces más, y el barrido de estados —que busca pánicos,
// tokens y estados que se pintan igual, no carreras— se recorta a la lista y el fallo.
const conRaza = true
