// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package coedicion

import "slices"

// AnclarOffset es `anclarOffset` de `state/cursors.ts`: dónde cae `offset` (un offset de
// texto de `antes`, en unidades UTF-16) en `despues`. Es la red de seguridad del cursor
// que no trae puntos de la secuencia, o cuando la secuencia no coincide con el folio: sin
// ella el caret se queda en su número y el texto se le va de debajo.
//
// Lo que no cambió por delante (prefijo común) ni por detrás (sufijo común) delimita la
// zona editada. Antes de ella el offset no se mueve; después, se desplaza lo que creció o
// encogió el texto; dentro de una zona borrada o reescrita se queda al principio.
//
// ponytail: como en la web, dos ediciones simultáneas lejanas se ven como una sola zona y
// el caret que quede entre ellas va al principio de la zona.
func AnclarOffset(offset int, antes, despues []uint16) int {
	if slices.Equal(antes, despues) {
		return offset
	}
	tope := min(len(antes), len(despues))
	pre := 0
	for pre < tope && antes[pre] == despues[pre] {
		pre++
	}
	suf := 0
	for suf < tope-pre && antes[len(antes)-1-suf] == despues[len(despues)-1-suf] {
		suf++
	}
	// Un caret al final sigue al final cuando se añade detrás: con texto repetido
	// («aaaa» → «aaaaa») el prefijo común se come el texto de antes entero.
	if suf == 0 && offset >= len(antes) {
		return len(despues)
	}
	switch {
	case offset <= pre:
		return offset
	case offset >= len(antes)-suf:
		return offset + len(despues) - len(antes)
	}
	return pre
}
