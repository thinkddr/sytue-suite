// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build race

package pantalla

// conRaza dice si los tests corren con `-race`, como en Cast y Photos: pintar cuesta ahí
// unas diez veces más, y el barrido del muestrario —que busca secciones que no pintan o
// no caben, no carreras— se recorta a una combinación por modo (ver combinacionesDelBarrido).
const conRaza = true
