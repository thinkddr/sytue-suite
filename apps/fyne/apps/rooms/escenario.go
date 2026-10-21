// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

// Quién ocupa el escenario grande de la llamada: `lib/escenario.ts` de la web. El orden
// de las reglas ES la función:
//
//  1. Lo FIJADO a mano gana siempre: que se lo pise el sistema es exactamente lo que hace
//     que nadie use la función.
//  2. Una pantalla compartida va en grande aunque el modo sea mosaico: se comparte para
//     que se vea.
//  3. En modo orador, el ÚLTIMO que habló —no el que habla ahora—: la señal del SFU se
//     vacía en cada pausa y el escenario se desmontaría entre frase y frase.
//  4. Si nada de eso aplica, mosaico.
//
// Y quien ya no está no sale en grande: colgar dejaba un hueco negro con su nombre.

// Modos del escenario.
const (
	ModoMosaico = "mosaico"
	ModoOrador  = "orador"
)

// Escena es lo que decide el protagonista. Las identidades son las de las teselas: la de
// la persona, o `pantalla:<persona>` para su pantalla compartida.
type Escena struct {
	Modo           string
	Fijado         string
	UltimoHablante string
	Compartiendo   string
	Presentes      []string
}

// Protagonista es la tesela que va en grande, o "" para mosaico.
func Protagonista(e Escena) string {
	presente := func(id string) string {
		for _, p := range e.Presentes {
			if id != "" && p == id {
				return id
			}
		}
		return ""
	}
	if id := presente(e.Fijado); id != "" {
		return id
	}
	if id := presente(e.Compartiendo); id != "" {
		return id
	}
	if e.Modo == ModoOrador {
		return presente(e.UltimoHablante)
	}
	return ""
}

// OrdenarPorProtagonista deja al protagonista el primero sin tocar el orden del resto.
func OrdenarPorProtagonista(ts []*TeselaLlamada, id string) []*TeselaLlamada {
	if id == "" {
		return ts
	}
	for i, t := range ts {
		if t.ID() != id {
			continue
		}
		if i == 0 {
			return ts
		}
		out := make([]*TeselaLlamada, 0, len(ts))
		out = append(out, t)
		out = append(out, ts[:i]...)
		return append(out, ts[i+1:]...)
	}
	return ts
}

// idPantalla es la identidad de la tesela de la pantalla de alguien.
func idPantalla(identidad string) string { return "pantalla:" + identidad }
