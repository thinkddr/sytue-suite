// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package coedicion

import (
	"encoding/json"
	"unicode/utf8"
)

// La POLÍTICA de `coedicion.ts`: cuándo se puede usar la secuencia. El contenido vive
// en UN registro LWW (`doc:html`) que es lo que lee todo el servicio (exportar, buscar),
// así que no se abandona:
//
//  1. CADA guardado escribe el registro, como siempre; la secuencia viaja ADEMÁS y solo
//     manda en el cliente.
//  2. Un `set` de contenido es AUTORITATIVO —«tira tu secuencia y quédate con mi
//     texto»— cuando va con un registro `<clave>:lww` en el mismo lote. Se manda cuando
//     quien guarda NO llevaba la secuencia al día: un documento de antes, la siembra, o
//     un cambio que no cabe en un lote.
//
// Solo para documentos de texto: fusionar dos JSON carácter a carácter da un JSON que no
// se lee, peor que perder una edición. Hojas y presentaciones siguen en LWW.

// ClaveLWW es la del registro que marca un `set` de contenido como autoritativo.
func ClaveLWW(clave string) string { return clave + ":lww" }

// MaxCuerpo es el tope de POST …/ops: el servicio corta en 1 MiB y el mismo cuerpo lleva
// el documento entero en el registro.
const MaxCuerpo = 900_000

// CoEdicion es la secuencia de un documento con su estado: si representa el texto que se
// ve, y lo ajeno que llegó mientras se escribía.
type CoEdicion struct {
	actor  string
	seq    *Secuencia
	sincro bool
	buffer []Op
}

// Nueva la arma desde los elementos del estado (`GET …/state`). Un documento de antes no
// tiene secuencia: hasta sembrarla, el canal por carácter no se usa.
func Nueva(actor string, elementos []Elemento, textoInicial string) *CoEdicion {
	s := DesdeElementos(actor, elementos)
	return &CoEdicion{actor: actor, seq: s, sincro: s.Texto() == textoInicial}
}

// Sincronizada dice si el canal por carácter está operativo; si no, manda el LWW.
func (c *CoEdicion) Sincronizada() bool { return c.sincro }

// Contador es el del actor local, que comparte con el dot de los `set` del registro.
func (c *CoEdicion) Contador() uint64 { return c.seq.Contador() }

// FijarContador lo adelanta.
func (c *CoEdicion) FijarContador(n uint64) { c.seq.FijarContador(n) }

// OpsLocales son las ops para llegar a `texto`, o ninguna cuando toca LWW: un documento
// sin sembrar con alguien más presente (sembrarlo a dos duplicaría el contenido) o una
// secuencia anulada por un lote autoritativo.
func (c *CoEdicion) OpsLocales(texto string, lamport uint64, aSolas bool) []Op {
	if !c.sincro && !aSolas {
		return nil
	}
	ops := c.seq.OpsDesdeTexto(texto, lamport)
	c.sincro = true
	return ops
}

// Recibir guarda las ops ajenas sin aplicarlas: mientras se escribe, el editor y la
// secuencia tienen que seguir diciendo lo mismo o el próximo diff borraría lo llegado.
func (c *CoEdicion) Recibir(ops []Op) {
	if !c.sincro {
		return // en LWW el contenido llega por el registro
	}
	for _, op := range ops {
		if op.Dot.A != c.actor { // lo propio ya está aplicado
			c.buffer = append(c.buffer, op)
		}
	}
}

// Fusionar aplica lo recibido y da el texto fusionado, o false si no hay nada. Solo sin
// edición local en vuelo.
func (c *CoEdicion) Fusionar() (string, bool) {
	if !c.sincro || len(c.buffer) == 0 {
		return "", false
	}
	for _, op := range c.buffer {
		if op.Kind == "insert" {
			c.seq.FijarContador(op.Dot.C) // lo insertado después va primero entre hermanos
		}
		c.seq.AplicarRemota(op)
	}
	c.buffer = nil
	return c.seq.Texto(), true
}

// Anular: un par ha declarado su `set` autoritativo y la secuencia describe un estado que
// ya no existe. Manda el registro; al volver a abrir, se evalúa de cero.
func (c *CoEdicion) Anular() {
	c.sincro, c.buffer = false, nil
}

// ElementosDeEstado es `elementosDeEstado`: de lo que da el servicio, solo los caracteres
// con identidad válida. Un elemento de presentación no es un carácter y no revienta nada.
func ElementosDeEstado(crudos []json.RawMessage) []Elemento {
	out := make([]Elemento, 0, len(crudos))
	for _, c := range crudos {
		var e Elemento
		if json.Unmarshal(c, &e) != nil || e.Value == "" {
			continue
		}
		out = append(out, e)
	}
	return out
}

// OpDeSecuencia es `opDeSecuencia`: una op ajena de la secuencia, validada, o false si no
// es un `insert` con UN carácter (una runa, como los que salen de InsertarLocal) ni un
// `delete` con su objetivo. Un insert de varios caracteres de otro coautor rompía el
// orden del RGA y la réplica dejaba de ver el mismo texto (FYE-05).
func OpDeSecuencia(crudo json.RawMessage) (Op, bool) {
	var op Op
	if json.Unmarshal(crudo, &op) != nil {
		return Op{}, false
	}
	switch {
	case op.Kind == "insert" && utf8.RuneCountInString(op.Value) == 1:
		return op, true
	case op.Kind == "delete" && op.Target != nil:
		return op, true
	}
	return Op{}, false
}
