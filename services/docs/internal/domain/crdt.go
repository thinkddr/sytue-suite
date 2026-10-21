// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"unicode/utf8"
)

// Estrategia de colaboración: CRDT de secuencia tipo RGA (Replicated Growable
// Array) con identidades densas e independientes y reloj Lamport. Obra original
// de Sytue (clean-room) sobre la literatura pública de CRDTs (Shapiro et al.,
// "A comprehensive study of CRDTs", 2011; Roh et al., RGA). NO se porta código
// de motores AGPL.
//
// Por qué CRDT y no OT: el servidor solo SECUENCIA y persiste operaciones (no
// transforma); la convergencia es responsabilidad determinista del modelo, lo
// que casa con un datastore único (PostgreSQL, append-only en doc_operations)
// y con clientes offline-first que reconectan y hacen replay. Sin servidor de
// transformación con estado complejo, sin Redis.
//
// El modelo de CADA tipo de documento (texto/hoja/presentación) se construye
// SOBRE estas primitivas:
//   - Document      -> secuencia RGA de caracteres + marcas de formato (LWW).
//   - Spreadsheet   -> mapa LWW de celdas (clave "hoja!fila:col" -> valor/fórmula).
//   - Presentation  -> secuencia RGA de diapositivas + LWW de propiedades.
// Aquí se implementan las primitivas comunes; los detalles por tipo se
// serializan en el campo `op` (jsonb) y se interpretan en cliente.

// Dot es la identidad global y única de un elemento CRDT: (actor, counter).
// Es estable para siempre; los borrados se marcan con tombstone, no se reusa.
type Dot struct {
	Actor   string `json:"a"` // identificador de réplica/sesión (opaco)
	Counter uint64 `json:"c"` // contador monótono por actor
}

// IsZero indica si el Dot es el valor cero (raíz de la secuencia).
func (d Dot) IsZero() bool { return d.Actor == "" && d.Counter == 0 }

// Less define un orden total determinista sobre Dots para desempatar inserciones
// concurrentes en la misma posición: mayor counter primero (RGA), y actor como
// segundo criterio estable. Garantiza convergencia en todas las réplicas.
func (d Dot) Less(o Dot) bool {
	if d.Counter != o.Counter {
		return d.Counter < o.Counter
	}
	return d.Actor < o.Actor
}

// OpKind enumera las operaciones CRDT de bajo nivel.
type OpKind string

const (
	// OpInsert inserta un elemento tras After (Dot del predecesor; cero = inicio).
	OpInsert OpKind = "insert"
	// OpDelete marca un elemento (por su Dot) como tombstone.
	OpDelete OpKind = "delete"
	// OpSet asigna un valor LWW a una clave (celdas, formato, propiedades).
	OpSet OpKind = "set"
	// OpPatch es una modificación puntual del modelo de una hoja o presentación
	// (parche.go): el servidor la aplica y la guarda como un `set` del registro.
	OpPatch OpKind = "patch"
)

// Op es una operación CRDT serializable (se persiste en doc_operations.op).
type Op struct {
	Kind OpKind `json:"kind"`
	// Dot es la identidad del elemento creado (insert) o de la asignación (set).
	Dot Dot `json:"dot"`
	// After es el predecesor en la secuencia (insert). Cero = principio.
	After *Dot `json:"after,omitempty"`
	// Target es el elemento a borrar (delete).
	Target *Dot `json:"target,omitempty"`
	// Key es la clave LWW (set): "sheet!r:c", "fmt:bold", "slide.title", etc.
	Key string `json:"key,omitempty"`
	// Value es el contenido (carácter, valor de celda, fórmula, propiedad...).
	Value json.RawMessage `json:"value,omitempty"`
	// Lamport es el reloj lógico del cliente al emitir (desempate LWW).
	Lamport uint64 `json:"lamport"`
	// OpID identifica un parche para la idempotencia (patch); queda en el log de los
	// `set` que el servidor materializa, con sus Cambios y la Base (auditoría).
	OpID string `json:"op_id,omitempty"`
	// Cambios son las modificaciones de un parche (parche.go).
	Cambios []Cambio `json:"cambios,omitempty"`
	// Base es la seq que conocía quien mandó el parche (informativa: los parches no
	// dan 409, se ordenan por seq).
	Base int64 `json:"base,omitempty"`
}

// Validate comprueba una operación que ENTRA (clean-room: validación de entrada
// estricta, no confiamos en el cliente): su estructura y que un `insert` sea un
// solo carácter.
//
// UN CARÁCTER ES UNA RUNA (un punto de código), decidido por el dueño el
// 2026-10-15 (FYE-05). Es lo que mandan los dos clientes legítimos: la web recorre
// el texto con `for (const ch of texto)` (rga.ts) y el nativo con `range texto`
// (apps/nucleo/coedicion/rga.go), así que un emoji compuesto o una letra con su
// acento combinante viajan como varias inserciones. Un elemento de varios
// caracteres rompía el orden del RGA, pensado para nodos de uno: los coautores
// dejaban de ver el mismo texto.
func (o Op) Validate() error {
	if err := o.validarForma(); err != nil {
		return err
	}
	if o.Kind == OpInsert && !unCaracter(o.Value) {
		return fmt.Errorf("%w: un insert lleva exactamente un carácter", ErrInvalidOp)
	}
	return nil
}

// unCaracter dice si el valor de un insert es una cadena JSON de una runa.
func unCaracter(v json.RawMessage) bool {
	var s string
	return json.Unmarshal(v, &s) == nil && utf8.RuneCountInString(s) == 1
}

// validarForma es la coherencia estructural. Es lo que se mira al REPRODUCIR el
// registro de operaciones (Apply): un registro anterior a la regla de un carácter
// tiene que seguir abriéndose.
func (o Op) validarForma() error {
	switch o.Kind {
	case OpInsert:
		if o.Dot.IsZero() {
			return fmt.Errorf("%w: insert sin dot", ErrInvalidOp)
		}
	case OpDelete:
		if o.Target == nil || o.Target.IsZero() {
			return fmt.Errorf("%w: delete sin target", ErrInvalidOp)
		}
	case OpSet:
		if o.Key == "" {
			return fmt.Errorf("%w: set sin key", ErrInvalidOp)
		}
	case OpPatch:
		return o.validarParche()
	default:
		return fmt.Errorf("%w: kind desconocido %q", ErrInvalidOp, o.Kind)
	}
	return nil
}

// element es un nodo de la secuencia RGA en el estado materializado.
type element struct {
	Dot     Dot             `json:"dot"`
	After   Dot             `json:"after"`
	Value   json.RawMessage `json:"value"`
	Deleted bool            `json:"deleted"`
	Lamport uint64          `json:"lamport"`
}

// register es un valor LWW (último escritor gana por Lamport, desempate por actor).
type register struct {
	Value   json.RawMessage `json:"value"`
	Dot     Dot             `json:"dot"`
	Lamport uint64          `json:"lamport"`
}

// State es el estado CRDT materializado de un documento. Es el resultado
// determinista de aplicar todas las operaciones en cualquier orden. Se
// serializa como snapshot (doc_snapshots.state) para acelerar la carga inicial.
type State struct {
	// Seq del documento que cubre este estado.
	Seq int64 `json:"seq"`
	// Lamport máximo observado (para que el servidor coloque tope a los clientes).
	Lamport uint64 `json:"lamport"`
	// Seq de la secuencia RGA (texto/diapositivas), incluidos tombstones.
	Elements []element `json:"elements"`
	// Registers son los valores LWW por clave (celdas, formato, propiedades).
	Registers map[string]register `json:"registers"`
}

// NewState crea un estado CRDT vacío.
func NewState() *State {
	return &State{Registers: make(map[string]register)}
}

// StateFromJSON deserializa un estado consolidado (snapshot inline).
func StateFromJSON(raw json.RawMessage) (*State, error) {
	s := NewState()
	if len(raw) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(raw, s); err != nil {
		return nil, fmt.Errorf("docs: snapshot CRDT inválido: %w", err)
	}
	if s.Registers == nil {
		s.Registers = make(map[string]register)
	}
	return s, nil
}

// JSON serializa el estado para persistirlo como snapshot.
func (s *State) JSON() (json.RawMessage, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("docs: serializar estado CRDT: %w", err)
	}
	return b, nil
}

// Apply integra una operación en el estado. Es idempotente y conmutativo:
// aplicar la misma operación dos veces, o en distinto orden que otra réplica,
// converge al mismo estado (propiedad CRDT). Devuelve true si el estado cambió.
func (s *State) Apply(op Op) (bool, error) {
	if err := op.validarForma(); err != nil {
		return false, err
	}
	if op.Lamport > s.Lamport {
		s.Lamport = op.Lamport
	}

	switch op.Kind {
	case OpInsert:
		// Idempotencia: ignorar si el Dot ya existe.
		for i := range s.Elements {
			if s.Elements[i].Dot == op.Dot {
				return false, nil
			}
		}
		after := Dot{}
		if op.After != nil {
			after = *op.After
		}
		s.Elements = append(s.Elements, element{
			Dot: op.Dot, After: after, Value: op.Value, Lamport: op.Lamport,
		})
		return true, nil

	case OpDelete:
		for i := range s.Elements {
			if s.Elements[i].Dot == *op.Target {
				if s.Elements[i].Deleted {
					return false, nil // ya borrado: idempotente
				}
				s.Elements[i].Deleted = true
				return true, nil
			}
		}
		// Tombstone de un elemento aún no visto: lo registramos como ya borrado
		// para mantener convergencia si el insert llega después.
		s.Elements = append(s.Elements, element{Dot: *op.Target, Deleted: true})
		return true, nil

	case OpSet:
		cur, ok := s.Registers[op.Key]
		// LWW: gana mayor Lamport; desempate por Dot (orden total).
		if ok && (cur.Lamport > op.Lamport || (cur.Lamport == op.Lamport && !cur.Dot.Less(op.Dot))) {
			return false, nil
		}
		s.Registers[op.Key] = register{Value: op.Value, Dot: op.Dot, Lamport: op.Lamport}
		return true, nil

	case OpPatch:
		// El servidor lo materializa como `set` antes de persistirlo (parche.go): el log no lleva parches.
	}
	return false, nil
}

// rgaOrder ordena la secuencia RGA de forma determinista: recorrido en
// profundidad desde la raíz, hijos del mismo padre ordenados por Dot
// descendente (RGA: la inserción más reciente queda más cerca del predecesor).
// Devuelve los elementos vivos (no tombstone) en orden de documento.
//
// Robustez (clean-room, no confiamos en el cliente): el recorrido es ITERATIVO
// (pila explícita) para no desbordar la pila en documentos muy largos, y lleva
// un conjunto `visited` que evita bucles infinitos si un `After` malicioso o
// corrupto formara un ciclo. Los elementos huérfanos (cuyo `After` no existe)
// no se emiten: un carácter solo es visible si su cadena de predecesores llega
// a la raíz.
func (s *State) rgaOrder() []element {
	children := make(map[Dot][]element, len(s.Elements))
	for _, e := range s.Elements {
		children[e.After] = append(children[e.After], e)
	}
	for k := range children {
		c := children[k]
		sort.Slice(c, func(i, j int) bool { return c[j].Dot.Less(c[i].Dot) })
		children[k] = c
	}

	out := make([]element, 0, len(s.Elements))
	visited := make(map[Dot]struct{}, len(s.Elements))

	// frame recuerda el índice del próximo hijo a visitar de un padre dado.
	type frame struct {
		parent Dot
		i      int
	}
	stack := []frame{{parent: Dot{}}}
	for len(stack) > 0 {
		top := &stack[len(stack)-1]
		kids := children[top.parent]
		if top.i >= len(kids) {
			stack = stack[:len(stack)-1]
			continue
		}
		e := kids[top.i]
		top.i++
		if _, seen := visited[e.Dot]; seen {
			continue // ciclo o Dot duplicado: no reentrar
		}
		visited[e.Dot] = struct{}{}
		if !e.Deleted {
			out = append(out, e)
		}
		stack = append(stack, frame{parent: e.Dot})
	}
	return out
}

// VersionVector resume qué operaciones ha visto el estado: por cada actor, el
// mayor Counter de un Dot conocido (vivo o tombstone). Sirve para calcular
// deltas entre réplicas sin enviar todo el log (anti-entropía) y para que el
// servidor detecte lagunas al reintegrar clientes que reconectan.
type VersionVector map[string]uint64

// VersionVector calcula el vector de versión del estado materializado.
func (s *State) VersionVector() VersionVector {
	vv := make(VersionVector, 8)
	observe := func(d Dot) {
		if d.IsZero() {
			return
		}
		if c, ok := vv[d.Actor]; !ok || d.Counter > c {
			vv[d.Actor] = d.Counter
		}
	}
	for _, e := range s.Elements {
		observe(e.Dot)
	}
	for _, r := range s.Registers {
		observe(r.Dot)
	}
	return vv
}

// Contains indica si el vector ya cubre el Dot dado (operación ya observada).
func (vv VersionVector) Contains(d Dot) bool {
	if d.IsZero() {
		return true
	}
	c, ok := vv[d.Actor]
	return ok && c >= d.Counter
}

// Materialize devuelve una vista de solo lectura del documento adecuada para
// export/búsqueda: la secuencia viva (con valores en orden) y los registros LWW.
// El cliente reconstruye el rich-text/celdas a partir de esto.
type Materialized struct {
	Sequence  []json.RawMessage          `json:"sequence"`
	Registers map[string]json.RawMessage `json:"registers"`
}

// Materialize aplana el estado CRDT a su forma legible.
func (s *State) Materialize() Materialized {
	ordered := s.rgaOrder()
	seq := make([]json.RawMessage, 0, len(ordered))
	for _, e := range ordered {
		seq = append(seq, e.Value)
	}
	regs := make(map[string]json.RawMessage, len(s.Registers))
	for k, r := range s.Registers {
		regs[k] = r.Value
	}
	return Materialized{Sequence: seq, Registers: regs}
}

// liveDots devuelve el conjunto de Dots de elementos VIVOS (no tombstone) del
// estado, con su valor, para comparaciones de merge/restore.
func (s *State) liveDots() map[Dot]element {
	m := make(map[Dot]element, len(s.Elements))
	for _, e := range s.Elements {
		if !e.Deleted {
			m[e.Dot] = e
		}
	}
	return m
}

// Diff calcula las operaciones que, aplicadas sobre el estado actual (s),
// producen un estado MATERIALMENTE igual a `target`, preservando el modelo
// append-only del CRDT: no se borra ni reescribe el log, solo se añaden
// operaciones nuevas. Es la base de la RESTAURACIÓN de versiones y de la
// reconciliación tras una importación.
//
// Estrategia (convergente y determinista):
//   - Secuencia RGA: los elementos vivos en `target` ausentes de `s` se
//     re-insertan preservando su Dot e `After` originales, en orden de documento
//     de `target` (así todo predecesor existe antes que su hijo). Los elementos
//     vivos en `s` ausentes de `target` se marcan con OpDelete (tombstone). Como
//     los Dots son estables y los inserts/deletes idempotentes, el resultado es
//     independiente del orden de recepción entre réplicas.
//   - Registros LWW: por cada clave cuyo valor difiera, se emite un OpSet con un
//     Lamport estrictamente mayor que el actual para que gane el valor de target;
//     las claves sobrantes (solo en `s`) se neutralizan a null.
//
// Las nuevas operaciones se emiten con el `actor` dado y contadores crecientes
// desde `baseCounter`+1; el reloj Lamport parte de `baseLamport`+1. El llamante
// (Service.RestoreVersion) asigna actor/base a partir de una sesión de sistema.
func (s *State) Diff(target *State, actor string, baseCounter, baseLamport uint64) []Op {
	if target == nil {
		return nil
	}
	ops := make([]Op, 0, 16)
	counter := baseCounter
	lamport := baseLamport
	next := func() (Dot, uint64) {
		counter++
		lamport++
		return Dot{Actor: actor, Counter: counter}, lamport
	}

	curLive := s.liveDots()
	tgtLive := target.liveDots()

	// 1) Elementos vivos en target ausentes de s: re-insertarlos con su identidad
	//    original en el mismo punto de la secuencia (idempotente y convergente).
	//    Se recorren en orden de documento de target para respetar dependencias
	//    (un After debe existir antes que su hijo).
	for _, e := range target.rgaOrder() {
		if _, alive := curLive[e.Dot]; alive {
			continue
		}
		after := e.After
		ops = append(ops, Op{
			Kind: OpInsert, Dot: e.Dot, After: &after, Value: e.Value, Lamport: e.Lamport,
		})
	}

	// 2) Elementos vivos en s ausentes de target: marcarlos como borrados.
	for dot := range curLive {
		if _, alive := tgtLive[dot]; alive {
			continue
		}
		d := dot
		_, lam := next()
		ops = append(ops, Op{Kind: OpDelete, Target: &d, Lamport: lam})
	}

	// 3) Registros LWW: alinear cada clave al valor de target (LWW gana con
	//    Lamport mayor). Se cubren tanto las claves de target como las que solo
	//    existen en s (se sobrescriben a "null" para neutralizarlas).
	keys := make(map[string]struct{}, len(s.Registers)+len(target.Registers))
	for k := range s.Registers {
		keys[k] = struct{}{}
	}
	for k := range target.Registers {
		keys[k] = struct{}{}
	}
	for k := range keys {
		cur, hasCur := s.Registers[k]
		tgt, hasTgt := target.Registers[k]
		switch {
		case hasTgt && (!hasCur || !bytes.Equal(cur.Value, tgt.Value)):
			dot, lam := next()
			ops = append(ops, Op{Kind: OpSet, Key: k, Value: tgt.Value, Dot: dot, Lamport: lam})
		case !hasTgt && hasCur && !isNullJSON(cur.Value):
			// Clave sobrante: neutralizar con null (el frontend la trata como vacía).
			dot, lam := next()
			ops = append(ops, Op{Kind: OpSet, Key: k, Value: jsonNull, Dot: dot, Lamport: lam})
		}
	}
	return ops
}

// jsonNull es el literal JSON null reutilizable.
var jsonNull = json.RawMessage("null")

// isNullJSON indica si el valor es el literal null (o vacío).
func isNullJSON(v json.RawMessage) bool {
	return len(v) == 0 || string(v) == "null"
}

// PlainText extrae el texto plano de la secuencia (búsqueda/indexado/export
// ligero). Asume que los valores de elemento de un Document son caracteres
// codificados como JSON string; ignora los que no lo sean.
func (s *State) PlainText() string {
	ordered := s.rgaOrder()
	var b []byte
	for _, e := range ordered {
		var ch string
		if json.Unmarshal(e.Value, &ch) == nil {
			b = append(b, ch...)
		}
	}
	return string(b)
}
