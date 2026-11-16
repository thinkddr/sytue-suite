// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package office

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"slices"
	"strings"
	"sync"
)

// El registro común de comandos de EDICIÓN de la cinta y su taxonomía de pestañas.
//
// Las fuentes son `comandos.json` y `pestanas.json`; `scripts/sync-office-catalog.sh`
// publica la copia de build de la web (`apps/frontend/packages/office/src/editor/`) y el CI
// exige que sean iguales. La cinta web lee las mismas filas: rótulo, atajo, tipos,
// pestaña, grupo, permiso de edición y telemetría. Dos vocabularios serían dos
// comportamientos para el mismo documento.

//go:embed comandos.json
var datosComandos []byte

//go:embed pestanas.json
var datosPestanas []byte

// Tipos de documento, con los nombres del `DocKind` de la web.
const (
	Documento    = "document"
	Hoja         = "spreadsheet"
	Presentacion = "presentation"
)

// Atajo es un atajo de teclado de un comando, escrito siempre con «Ctrl» («Ctrl+Shift+X»).
type Atajo struct {
	Keys string `json:"keys"`
	// Value es el valor con el que se despacha el comando (Ctrl+X es `copy` con `cut`).
	Value string `json:"value"`
	// Canvas: lo atiende el propio lienzo editable; se enseña, no se captura.
	Canvas bool `json:"canvas"`
}

// Comando es una fila del registro.
type Comando struct {
	// ID es la clave de la fila; no viaja en el JSON.
	ID string `json:"-"`
	// LabelKey es la clave del catálogo con el rótulo del control.
	LabelKey string `json:"labelKey"`
	// Kinds son los tipos de documento en los que el comando existe.
	Kinds []string `json:"kinds"`
	// Tab es la pestaña de la cinta en la que vive (la taxonomía de [Pestanas]).
	Tab string `json:"tab"`
	// Group es la clave del rótulo del grupo de la cinta.
	Group string `json:"group"`
	// Edits: cambia el documento, así que se apaga en solo lectura.
	Edits bool `json:"edits"`
	// Script: un script de usuario puede despacharlo (ADR 0069). Ausente = no: lo que no se
	// marca no se abre por olvido.
	Script bool `json:"script"`
	// Selection es la selección que pide: «table» o «image»; vacío, ninguna.
	Selection string  `json:"selection"`
	Shortcuts []Atajo `json:"shortcuts"`
	// Telemetry: se puede contar su uso (el id y el tipo; NUNCA el valor, que es contenido).
	Telemetry bool `json:"telemetry"`
}

// Existe dice si el comando existe en ese tipo de documento.
func (c Comando) Existe(kind string) bool { return slices.Contains(c.Kinds, kind) }

// Bloqueado dice si el comando está apagado ahora: edita y el documento es de solo lectura.
// (Las selecciones —una tabla, una imagen— las conoce quien pinta, no este paquete.)
func (c Comando) Bloqueado(soloLectura bool) bool { return c.Edits && soloLectura }

var comandos = sync.OnceValue(func() map[string]Comando {
	var filas map[string]Comando
	if err := json.Unmarshal(datosComandos, &filas); err != nil || len(filas) == 0 {
		panic("invalid Office command registry")
	}
	for id, c := range filas {
		c.ID = id
		filas[id] = c
	}
	return filas
})

// Comandos devuelve el registro entero, por id. Es una copia: nadie puede alterar el de los demás.
func Comandos() map[string]Comando {
	out := make(map[string]Comando, len(comandos()))
	for id, c := range comandos() {
		c.Kinds = slices.Clone(c.Kinds)
		c.Shortcuts = slices.Clone(c.Shortcuts)
		out[id] = c
	}
	return out
}

// orden son los ids en el orden del JSON, que es el de la web (`Object.keys(COMANDOS)`):
// lo que se enseña en lista (la hoja de atajos) tiene que salir igual en las dos, y un
// mapa de Go no tiene orden.
var orden = sync.OnceValue(func() []string {
	dec := json.NewDecoder(bytes.NewReader(datosComandos))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		panic("invalid Office command registry")
	}
	var ids []string
	for dec.More() {
		t, err := dec.Token()
		id, ok := t.(string)
		if err != nil || !ok {
			panic("invalid Office command registry")
		}
		var fila json.RawMessage
		if err := dec.Decode(&fila); err != nil {
			panic("invalid Office command registry")
		}
		ids = append(ids, id)
	}
	return ids
})

// IDsEnOrden devuelve los ids del registro en el orden en que están escritos (el de la web).
func IDsEnOrden() []string { return slices.Clone(orden()) }

// BuscarComando devuelve la fila de un comando, o false si el registro no la tiene.
func BuscarComando(id string) (Comando, bool) {
	c, ok := comandos()[id]
	if !ok {
		return Comando{}, false
	}
	c.Kinds = slices.Clone(c.Kinds)
	c.Shortcuts = slices.Clone(c.Shortcuts)
	return c, true
}

// Pestana es una pestaña de la cinta.
type Pestana struct {
	ID string `json:"id"`
	// LabelKey es la clave del catálogo con el rótulo de la pestaña.
	LabelKey string `json:"labelKey"`
	// Kinds son los tipos de documento en los que se ve.
	Kinds []string `json:"kinds"`
}

// Visible dice si la pestaña se ve con ese tipo de documento.
func (p Pestana) Visible(kind string) bool { return slices.Contains(p.Kinds, kind) }

var pestanas = sync.OnceValue(func() []Pestana {
	var doc struct {
		Version  int       `json:"version"`
		Pestanas []Pestana `json:"pestanas"`
	}
	if err := json.Unmarshal(datosPestanas, &doc); err != nil || doc.Version != 1 || len(doc.Pestanas) == 0 {
		panic("invalid Office tab taxonomy")
	}
	return doc.Pestanas
})

// Pestanas devuelve la taxonomía de pestañas, en el orden en que se pintan.
// Es una copia independiente para cada pantalla.
func Pestanas() []Pestana {
	out := slices.Clone(pestanas())
	for i := range out {
		out[i].Kinds = slices.Clone(out[i].Kinds)
	}
	return out
}

// PestanasDe devuelve las pestañas que se ven con ese tipo de documento, en orden.
func PestanasDe(kind string) []Pestana {
	var out []Pestana
	for _, p := range Pestanas() {
		if p.Visible(kind) {
			out = append(out, p)
		}
	}
	return out
}

// BuscarPestana devuelve la pestaña con ese id y su posición en la taxonomía.
func BuscarPestana(id string) (Pestana, int, bool) {
	for i, p := range Pestanas() {
		if p.ID == id {
			return p, i, true
		}
	}
	return Pestana{}, -1, false
}

// ComandoDelAtajo es `comandoDelAtajo` de la web (`editor/comandos.ts`): el comando y el
// valor que dispara la combinación `teclas` en un lienzo de ese tipo, o false. `teclas` va
// escrita como en el registro («Ctrl+Shift+L», «Ctrl+Alt+1», «Ctrl+]»), con «Ctrl» por la
// tecla de atajos del sistema (⌘ en un Mac); la última tecla no distingue mayúsculas.
//
// Como en la web, el texto editable (folio, caja de Diapos) pregunta con [Documento], y los
// atajos marcados `canvas` no se devuelven: los atiende el propio editor (su historial).
// Recorre las filas en orden de id para que el resultado no dependa del de un mapa (el
// registro ya prohíbe dos filas con la misma combinación en un tipo).
func ComandoDelAtajo(teclas, kind string) (id, valor string, ok bool) {
	pedido, bien := leerTeclas(teclas)
	if !bien {
		return "", "", false
	}
	filas := comandos()
	ids := make([]string, 0, len(filas))
	for id := range filas {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		c := filas[id]
		if !c.Existe(kind) {
			continue
		}
		for _, a := range c.Shortcuts {
			if a.Canvas {
				continue
			}
			if t, ok := leerTeclas(a.Keys); ok && t == pedido {
				return id, a.Value, true
			}
		}
	}
	return "", "", false
}

// teclas es una combinación leída: los modificadores y la tecla, en minúsculas.
type teclas struct {
	shift, alt bool
	tecla      string
}

// leerTeclas lee «Ctrl+Shift+X»; sin «Ctrl» no es un atajo de comando.
func leerTeclas(s string) (teclas, bool) {
	partes := strings.Split(s, "+")
	if len(partes) < 2 || partes[0] != "Ctrl" {
		return teclas{}, false
	}
	t := teclas{tecla: strings.ToLower(partes[len(partes)-1])}
	for _, m := range partes[1 : len(partes)-1] {
		switch m {
		case "Shift":
			t.shift = true
		case "Alt":
			t.alt = true
		default:
			return teclas{}, false
		}
	}
	return t, t.tecla != ""
}
