// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Operaciones SEMÁNTICAS de hoja y de presentación.
//
// Hasta aquí Calcus y Diapos guardaban el modelo ENTERO en un registro LWW
// (doc:sheet, doc:slides) y la co-edición dependía de que el cliente fusionara
// antes de mandar (base_seq + 409). Una operación `patch` manda solo lo que
// cambió —una celda, una caja, una diapositiva— y es el SERVIDOR quien la aplica
// sobre el contenido vigente, dentro de la transacción que ya serializa el
// documento:
//
//   - Orden por seq: dos parches a la vez se aplican uno tras otro, en el orden
//     en que el bloqueo del documento los deja pasar; no hay 409 que reintentar.
//   - Idempotencia: cada parche lleva un `op_id` del cliente; reenviarlo (reintento
//     tras un corte, replay al reconectar) no lo aplica dos veces.
//   - Compatibilidad: el parche se materializa como un `set` ordinario del registro
//     de contenido (Op.Cambios y Op.OpID quedan además en el log como auditoría), así
//     que un cliente que escribe y lee el registro completo ve exactamente lo de
//     siempre, y la protección de hojas valida el resultado como si lo hubiera
//     mandado él.

// Cambio es una modificación puntual del modelo: poner o quitar el valor de una ruta.
//
// La ruta alterna claves de objeto (string) y elementos de lista por identidad
// (`{"id": "..."}`), p. ej. una celda: ["sheets", {"id":"h1"}, "cells", "A1"]; una
// caja: ["slides", {"id":"s1"}, "boxes", {"id":"b1"}, "html"].
//
//   - Valor en una clave: la asigna (y crea los objetos intermedios que falten).
//   - Valor en un elemento de lista: lo reemplaza, o lo AÑADE AL FINAL si no está.
//   - Del: quita la clave o el elemento.
//
// Si un elemento de lista por el camino ya no existe (otra persona borró la hoja o
// la diapositiva), el cambio se descarta: no resucita lo borrado.
type Cambio struct {
	Path  []json.RawMessage `json:"path"`
	Value json.RawMessage   `json:"value,omitempty"`
	Del   bool              `json:"del,omitempty"`
}

const (
	maxCambios  = 2000
	maxRuta     = 10
	maxOpIDLen  = 64
	maxIDLen    = 128
	maxClaveLen = 256
)

// segmento es un tramo ya validado de la ruta: clave de objeto o id de elemento.
type segmento struct {
	clave string
	id    string
	esID  bool
}

func (c Cambio) segmentos() ([]segmento, error) {
	if len(c.Path) == 0 || len(c.Path) > maxRuta {
		return nil, fmt.Errorf("%w: ruta de cambio vacía o demasiado larga", ErrInvalidOp)
	}
	if !c.Del && len(c.Value) == 0 {
		return nil, fmt.Errorf("%w: cambio sin valor ni del", ErrInvalidOp)
	}
	out := make([]segmento, len(c.Path))
	for i, raw := range c.Path {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			if s == "" || len(s) > maxClaveLen {
				return nil, fmt.Errorf("%w: clave de ruta inválida", ErrInvalidOp)
			}
			out[i] = segmento{clave: s}
			continue
		}
		var o struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &o) != nil || o.ID == "" || len(o.ID) > maxIDLen {
			return nil, fmt.Errorf("%w: tramo de ruta inválido", ErrInvalidOp)
		}
		out[i] = segmento{id: o.ID, esID: true}
	}
	if out[0].esID {
		return nil, fmt.Errorf("%w: la ruta empieza por una clave", ErrInvalidOp)
	}
	return out, nil
}

// validarParche comprueba un `patch` (Op.Validate lo llama).
func (o Op) validarParche() error {
	if o.Key != ClaveHoja && o.Key != ClaveDiapos {
		return fmt.Errorf("%w: patch solo sobre %s o %s", ErrInvalidOp, ClaveHoja, ClaveDiapos)
	}
	if o.OpID == "" || len(o.OpID) > maxOpIDLen {
		return fmt.Errorf("%w: patch sin op_id válido", ErrInvalidOp)
	}
	if len(o.Cambios) == 0 || len(o.Cambios) > maxCambios {
		return fmt.Errorf("%w: patch con %d cambios", ErrInvalidOp, len(o.Cambios))
	}
	for _, c := range o.Cambios {
		if _, err := c.segmentos(); err != nil {
			return err
		}
	}
	return nil
}

// Claves de los registros de contenido de hojas y presentaciones.
const (
	ClaveHoja   = claveHoja
	ClaveDiapos = "doc:slides"
)

// AplicarCambios aplica los cambios, en orden, al modelo JSON `contenido` (el valor
// del registro: un string con el JSON) y devuelve el modelo resultante. Los números
// se conservan tal cual (sin pasar por float64).
func AplicarCambios(contenido string, cambios []Cambio) (string, error) {
	dec := json.NewDecoder(bytes.NewReader([]byte(contenido)))
	dec.UseNumber()
	var raiz map[string]any
	if err := dec.Decode(&raiz); err != nil || raiz == nil {
		return "", fmt.Errorf("%w: el contenido no es un modelo JSON", ErrInvalidOp)
	}
	for _, c := range cambios {
		segs, err := c.segmentos()
		if err != nil {
			return "", err
		}
		var valor any
		if !c.Del {
			d := json.NewDecoder(bytes.NewReader(c.Value))
			d.UseNumber()
			if err := d.Decode(&valor); err != nil {
				return "", fmt.Errorf("%w: valor de cambio no es JSON", ErrInvalidOp)
			}
		}
		aplicarCambio(raiz, segs, valor, c.Del)
	}
	out, err := json.Marshal(raiz)
	if err != nil {
		return "", fmt.Errorf("docs: serializar modelo: %w", err)
	}
	return string(out), nil
}

// aplicarCambio desciende por la ruta; devuelve sin hacer nada si un elemento de
// lista del camino no existe.
func aplicarCambio(nodo map[string]any, segs []segmento, valor any, del bool) {
	clave := segs[0].clave
	if len(segs) == 1 {
		if del {
			delete(nodo, clave)
		} else {
			nodo[clave] = valor
		}
		return
	}
	if segs[1].esID {
		lista, _ := nodo[clave].([]any)
		i := buscarID(lista, segs[1].id)
		if len(segs) == 2 { // el propio elemento de la lista
			nodo[clave] = cambiarElemento(lista, i, segs[1].id, valor, del)
			return
		}
		if i < 0 {
			return // la hoja/diapositiva/caja ya no está: no se resucita
		}
		if hijo, ok := lista[i].(map[string]any); ok {
			aplicarCambio(hijo, segs[2:], valor, del)
		}
		return
	}
	hijo, ok := nodo[clave].(map[string]any)
	if !ok {
		if del {
			return
		}
		hijo = map[string]any{}
		nodo[clave] = hijo
	}
	aplicarCambio(hijo, segs[1:], valor, del)
}

func buscarID(lista []any, id string) int {
	for i, e := range lista {
		if m, ok := e.(map[string]any); ok && m["id"] == id {
			return i
		}
	}
	return -1
}

// cambiarElemento reemplaza, añade al final o quita el elemento `id` de la lista.
func cambiarElemento(lista []any, i int, id string, valor any, del bool) []any {
	switch {
	case del && i >= 0:
		return append(lista[:i:i], lista[i+1:]...)
	case del:
		return lista
	}
	if m, ok := valor.(map[string]any); ok {
		m["id"] = id // la identidad la manda la ruta, no el cuerpo
	}
	if i >= 0 {
		out := append([]any(nil), lista...)
		out[i] = valor
		return out
	}
	return append(lista, valor)
}
