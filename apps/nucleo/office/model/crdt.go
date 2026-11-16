// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/apps/nucleo/coedicion"
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
	// Extensions retains fields written by newer authorized clients.
	Extensions map[string]json.RawMessage `json:"-"`
	Kind       OpKind                     `json:"kind"`
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

// Element es un nodo de la secuencia RGA en el estado materializado.
type Element struct {
	Extensions map[string]json.RawMessage `json:"-"`
	Dot        Dot                        `json:"dot"`
	After      Dot                        `json:"after"`
	Value      json.RawMessage            `json:"value"`
	Deleted    bool                       `json:"deleted"`
	Lamport    uint64                     `json:"lamport"`
}

// Register es un valor LWW (último escritor gana por Lamport, desempate por actor).
type Register struct {
	Extensions map[string]json.RawMessage `json:"-"`
	Value      json.RawMessage            `json:"value"`
	Dot        Dot                        `json:"dot"`
	Lamport    uint64                     `json:"lamport"`
}

// State es el estado CRDT materializado de un documento. Es el resultado
// determinista de aplicar todas las operaciones en cualquier orden. Se
// serializa como snapshot (doc_snapshots.state) para acelerar la carga inicial.
type State struct {
	Extensions map[string]json.RawMessage `json:"-"`
	// Seq del documento que cubre este estado.
	Seq int64 `json:"seq"`
	// Lamport máximo observado (para que el servidor coloque tope a los clientes).
	Lamport uint64 `json:"lamport"`
	// Seq de la secuencia RGA (texto/diapositivas), incluidos tombstones.
	Elements []Element `json:"elements"`
	// Registers son los valores LWW por clave (celdas, formato, propiedades).
	Registers map[string]Register `json:"registers"`
}

// NewState crea un estado CRDT vacío.
func NewState() *State {
	return &State{Registers: make(map[string]Register)}
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
		s.Registers = make(map[string]Register)
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
		s.Elements = append(s.Elements, Element{
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
		s.Elements = append(s.Elements, Element{Dot: *op.Target, Deleted: true})
		return true, nil

	case OpSet:
		cur, ok := s.Registers[op.Key]
		// LWW: gana mayor Lamport; desempate por Dot (orden total).
		if ok && (cur.Lamport > op.Lamport || (cur.Lamport == op.Lamport && !cur.Dot.Less(op.Dot))) {
			return false, nil
		}
		s.Registers[op.Key] = Register{Value: op.Value, Dot: op.Dot, Lamport: op.Lamport, Extensions: cur.Extensions}
		return true, nil

	case OpPatch:
		// El servidor lo materializa como `set` antes de persistirlo (parche.go): el log no lleva parches.
	}
	return false, nil
}

// RGAOrder ordena la secuencia RGA de forma determinista: recorrido en
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
func (s *State) RGAOrder() []Element {
	children := make(map[Dot][]Element, len(s.Elements))
	for _, e := range s.Elements {
		children[e.After] = append(children[e.After], e)
	}
	for k := range children {
		c := children[k]
		sort.Slice(c, func(i, j int) bool { return c[j].Dot.Less(c[i].Dot) })
		children[k] = c
	}

	out := make([]Element, 0, len(s.Elements))
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
	ordered := s.RGAOrder()
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

// Diff restaura contenido mediante operaciones append-only. Como una tumba no
// revive, reutiliza la política de edición RGA: conserva prefijo/sufijo visibles
// iguales e inserta el centro con Dots nuevos. Las marcas de formato y propiedades
// de diapositiva de target se trasladan a las identidades resultantes. Los nodos
// antiguos y sus metadatos permanecen en el historial; no se reescribe un Dot.
// Los comentarios con anclas HTML conservan su contenido original en los registros.
// actor debe ser único para esta edición; baseCounter/baseLamport nunca retroceden.
func (s *State) Diff(target *State, actor string, baseCounter, baseLamport uint64) []Op {
	if target == nil {
		return nil
	}
	current, desired := s.RGAOrder(), target.RGAOrder()
	pre, suf := coedicion.IntervaloReemplazo(current, desired, func(a, b Element) bool { return bytes.Equal(a.Value, b.Value) })
	counter, lamport := maxDiffCounter(baseCounter, s, target), max(baseLamport, s.Lamport, target.Lamport)
	next := func() (Dot, uint64) { counter++; lamport++; return Dot{Actor: actor, Counter: counter}, lamport }
	ops := make([]Op, 0, len(current)-pre-suf+len(desired)-pre-suf)
	anchors := make(map[Dot]Dot, len(desired))
	for i := 0; i < pre; i++ {
		anchors[desired[i].Dot] = current[i].Dot
	}
	for i := 0; i < suf; i++ {
		anchors[desired[len(desired)-1-i].Dot] = current[len(current)-1-i].Dot
	}
	for _, e := range current[pre : len(current)-suf] {
		d := e.Dot
		dot, lam := next()
		ops = append(ops, Op{Kind: OpDelete, Dot: dot, Target: &d, Lamport: lam})
	}
	after := Dot{}
	if pre > 0 {
		after = current[pre-1].Dot
	}
	for _, e := range desired[pre : len(desired)-suf] {
		dot, lam := next()
		a := after
		ops = append(ops, Op{Kind: OpInsert, Dot: dot, After: &a, Value: e.Value, Lamport: lam})
		anchors[e.Dot] = dot
		after = dot
	}
	desiredRegisters := mapDiffRegisters(target.Registers, anchors)
	for _, key := range orderedRegisterKeys(s.Registers, desiredRegisters) {
		cur, hasCur := s.Registers[key]
		tgt, hasTgt := desiredRegisters[key]
		value := tgt.Value
		if hasTgt && hasCur && bytes.Equal(cur.Value, tgt.Value) {
			continue
		}
		if !hasTgt {
			if !hasCur || IsNullJSON(cur.Value) {
				continue
			}
			value = jsonNull
		}
		dot, lam := next()
		ops = append(ops, Op{Kind: OpSet, Key: key, Value: value, Dot: dot, Lamport: lam})
	}
	return ops
}

func maxDiffCounter(baseCounter uint64, s, target *State) uint64 {
	// El hermano nuevo debe preceder a los descendientes borrados, igual que una
	// inserción local. Mirar todas las identidades incluye las tumbas y otros actores.
	counter := baseCounter
	for _, state := range []*State{s, target} {
		for _, e := range state.Elements {
			counter = max(counter, e.Dot.Counter)
		}
		for _, r := range state.Registers {
			counter = max(counter, r.Dot.Counter)
		}
	}
	return counter
}

func mapDiffRegisters(target map[string]Register, anchors map[Dot]Dot) map[string]Register {
	// Estos son los dos esquemas públicos de claves ancladas a un Dot. Las claves
	// opacas de clientes futuros se conservan tal cual; no se interpreta su JSON.
	prefixes := make(map[string]string, 2*len(anchors))
	for old, newDot := range anchors {
		prefixes[FormatMarkKey(old, "")] = FormatMarkKey(newDot, "")
		prefixes[SlidePropKey(old, "")] = SlidePropKey(newDot, "")
	}
	desiredRegisters := make(map[string]Register, len(target))
	for key, register := range target {
		mapped := key
		// Los actores pueden contener ':'. Usar el prefijo completo más largo evita
		// cortar el actor por posición y admite nombres de marcas con separadores.
		for i := len(key) - 1; i >= 0; i-- {
			if key[i] == ':' {
				if prefix, ok := prefixes[key[:i+1]]; ok {
					mapped = prefix + key[i+1:]
					break
				}
			}
		}
		desiredRegisters[mapped] = register
	}
	return desiredRegisters
}

func orderedRegisterKeys(a, b map[string]Register) []string {
	keys := make(map[string]struct{}, len(a)+len(b))
	for key := range a {
		keys[key] = struct{}{}
	}
	for key := range b {
		keys[key] = struct{}{}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	slices.Sort(ordered)
	return ordered
}

// jsonNull es el literal JSON null reutilizable.
var jsonNull = json.RawMessage("null")

// IsNullJSON indica si el valor es el literal null (o vacío).
func IsNullJSON(v json.RawMessage) bool {
	return len(v) == 0 || string(v) == "null"
}

// PlainText extrae el texto plano de la secuencia (búsqueda/indexado/export
// ligero). Asume que los valores de elemento de un Document son caracteres
// codificados como JSON string; ignora los que no lo sean.
func (s *State) PlainText() string {
	ordered := s.RGAOrder()
	var b []byte
	for _, e := range ordered {
		var ch string
		if json.Unmarshal(e.Value, &ch) == nil {
			b = append(b, ch...)
		}
	}
	return string(b)
}

// DocumentHTML devuelve el HTML del documento que el editor guarda en el registro
// LWW "doc:html" (una cadena JSON). Vacío si no existe o no es una cadena.
func (s *State) DocumentHTML() string {
	return s.ContentString("doc:html")
}

// ContentString devuelve el contenido de un registro LWW cuyo valor es una cadena
// JSON (doc:html, doc:sheet, doc:slides): el editor guarda el modelo del tipo en
// una sola clave (CONTENT_KEY). "" si la clave no existe o no es una cadena.
func (s *State) ContentString(key string) string {
	r, ok := s.Registers[key]
	if !ok {
		return ""
	}
	var v string
	if json.Unmarshal(r.Value, &v) != nil {
		return ""
	}
	return v
}

// ContentKey es la clave del registro LWW donde el editor guarda el modelo
// entero de cada tipo: el HTML del documento, o el JSON de SheetModel o de
// SlidesModel. Es el `CONTENT_KEY` del frontend (editor/types.ts); separarlos
// los desincronizaría.
func ContentKey(k Kind) string {
	switch k {
	case KindSpreadsheet:
		return "doc:sheet"
	case KindPresentation:
		return "doc:slides"
	default:
		return "doc:html"
	}
}

// Contenido returns the native editable content register.
func (s *State) Contenido(kind Kind) string { return s.ContentString(ContentKey(kind)) }
