// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package llamada

import (
	"encoding/json"
	"slices"
)

// Las señales del canal de datos de la sala: la mano alzada y las reacciones. Son las
// de la web (`lib/salaDatos.ts`), byte a byte: `{"t":"mano","alzada":true}` y
// `{"t":"reaccion","emoji":"👏"}`, para que un cliente nativo y uno web se entiendan.

// Reacciones son las seis de la web. Lista cerrada: lo que llega es de otro cliente.
var Reacciones = []string{"👏", "👍", "🎉", "❤️", "😂", "🤔"}

// Senal es una mano o una reacción.
type Senal struct {
	Tipo   string `json:"t"` // "mano" o "reaccion"
	Alzada bool   `json:"alzada,omitempty"`
	Emoji  string `json:"emoji,omitempty"`
}

// Mano y Reaccion construyen las dos señales.
func Mano(alzada bool) Senal      { return Senal{Tipo: "mano", Alzada: alzada} }
func Reaccion(emoji string) Senal { return Senal{Tipo: "reaccion", Emoji: emoji} }

// Codificar es el JSON de la web. La mano lleva `alzada` también cuando es false.
func (s Senal) Codificar() []byte {
	if s.Tipo == "mano" {
		b, _ := json.Marshal(struct {
			T      string `json:"t"`
			Alzada bool   `json:"alzada"`
		}{"mano", s.Alzada})
		return b
	}
	b, _ := json.Marshal(struct {
		T     string `json:"t"`
		Emoji string `json:"emoji"`
	}{"reaccion", s.Emoji})
	return b
}

// DescodificarSenal lee lo que llega; lo que no es una señal conocida da false.
func DescodificarSenal(b []byte) (Senal, bool) {
	var v struct {
		T      string `json:"t"`
		Alzada *bool  `json:"alzada"`
		Emoji  string `json:"emoji"`
	}
	if json.Unmarshal(b, &v) != nil {
		return Senal{}, false
	}
	switch {
	case v.T == "mano" && v.Alzada != nil:
		return Mano(*v.Alzada), true
	case v.T == "reaccion" && slices.Contains(Reacciones, v.Emoji):
		return Reaccion(v.Emoji), true
	}
	return Senal{}, false
}
