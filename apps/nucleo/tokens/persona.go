// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tokens

import (
	"strconv"
	"unicode/utf16"
)

// Personas es cuántos colores de persona hay: `--sytue-avatar-1` a `--sytue-avatar-6`
// (`AVATAR_COUNT` de `state/cursors.ts`).
const Personas = 6

// Persona es el token del color de una persona: el `peerColor` de `state/cursors.ts` y
// el `bgFor` de `Avatar.tsx`, que son el MISMO hash —`h * 31 + charCodeAt(i)` truncado a
// 32 bits sin signo— sobre la clave que cada uno recibe (el `sub` para los cursores y las
// marcas del control de cambios, el nombre para el avatar). Devuelve «avatar-1» a
// «avatar-6».
//
// Recorre unidades UTF-16, que es lo que recorre `charCodeAt`: un carácter fuera del plano
// básico cuenta como DOS, y sin eso la misma persona saldría de otro color que en la web.
func Persona(clave string) string {
	var h uint32
	for _, u := range utf16.Encode([]rune(clave)) {
		h = h*31 + uint32(u)
	}
	return "avatar-" + strconv.Itoa(int(h%Personas)+1)
}
