// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Cambiar el esquema de una tabla sin rehacerla: las opciones de un campo de selección
// y el orden de los campos. Puro, como lib/esquema.ts de la web.

// listaDeOpciones son las opciones escritas una por línea: sin espacios de más, sin
// líneas vacías y sin repetidas (la primera manda). Igual al crear que al editar.
func listaDeOpciones(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" && !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}

// opcionesQueSeRetiran son las que había y ya no: los registros que las usan las
// conservan, retiradas.
func opcionesQueSeRetiran(antes, ahora []string) []string {
	var out []string
	for _, o := range antes {
		if !slices.Contains(ahora, o) {
			out = append(out, o)
		}
	}
	return out
}

// moverCampoEn es el orden de `ids` con `id` puesto donde estaba `destino` (los de en
// medio se corren un puesto). Si alguno no está, el orden no cambia.
func moverCampoEn(ids []string, id, destino string) []string {
	de, a := slices.Index(ids, id), slices.Index(ids, destino)
	if de < 0 || a < 0 || de == a {
		return slices.Clone(ids)
	}
	out := slices.Delete(slices.Clone(ids), de, de+1)
	return slices.Insert(out, a, id)
}

// ordenarCampos deja `campos` (en su sitio) en el orden de `ids`; los que no están en
// él van al final.
func ordenarCampos(campos []api.BasesField, ids []string) {
	pos := func(c api.BasesField) int {
		if i := slices.Index(ids, c.ID); i >= 0 {
			return i
		}
		return len(ids)
	}
	slices.SortStableFunc(campos, func(a, b api.BasesField) int { return pos(a) - pos(b) })
}
