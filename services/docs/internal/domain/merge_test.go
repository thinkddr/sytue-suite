// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"testing"
)

// buildText construye un State de texto a partir de una cadena, con el actor dado.
func buildText(actor, chars string) *State {
	s := NewState()
	var prev *Dot
	for i, r := range chars {
		v, _ := json.Marshal(string(r))
		dot := Dot{Actor: actor, Counter: uint64(i + 1)}
		s.Apply(Op{Kind: OpInsert, Dot: dot, After: prev, Value: v, Lamport: uint64(i + 1)})
		d := dot
		prev = &d
	}
	return s
}

// applyAll aplica un lote de operaciones a un estado.
func applyAll(t *testing.T, s *State, ops []Op) {
	t.Helper()
	for _, op := range ops {
		if _, err := s.Apply(op); err != nil {
			t.Fatalf("apply diff op: %v", err)
		}
	}
}

func TestDiffRestoresTextContent(t *testing.T) {
	// Estado actual: "Hola mundo"; versión objetivo: "Hi".
	current := buildText("A", "Hola mundo")
	target := buildText("B", "Hi")

	ops := current.Diff(target, "restore:x", 0, current.Lamport)
	if len(ops) == 0 {
		t.Fatal("Diff no produjo operaciones para estados distintos")
	}
	applyAll(t, current, ops)

	if current.PlainText() != target.PlainText() {
		t.Fatalf("restauración divergente: %q != %q", current.PlainText(), target.PlainText())
	}
}

func TestDiffIsIdempotentWhenEqual(t *testing.T) {
	current := buildText("A", "Igual")
	target := buildText("A", "Igual") // mismos Dots
	if ops := current.Diff(target, "restore:x", 0, current.Lamport); len(ops) != 0 {
		t.Fatalf("Diff de estados iguales debe ser vacío, got %d ops", len(ops))
	}
}

func TestDiffRestoresRegisters(t *testing.T) {
	current := NewState()
	v1, _ := json.Marshal("=A1+B1")
	current.Apply(Op{Kind: OpSet, Key: CellKey("Hoja1", 1, 1), Value: v1, Dot: Dot{Actor: "A", Counter: 1}, Lamport: 1})
	// Celda extra que la versión objetivo NO tiene: debe neutralizarse.
	extra, _ := json.Marshal("basura")
	current.Apply(Op{Kind: OpSet, Key: CellKey("Hoja1", 2, 2), Value: extra, Dot: Dot{Actor: "A", Counter: 2}, Lamport: 2})

	target := NewState()
	v2, _ := json.Marshal("=A1*B1")
	target.Apply(Op{Kind: OpSet, Key: CellKey("Hoja1", 1, 1), Value: v2, Dot: Dot{Actor: "B", Counter: 1}, Lamport: 10})

	ops := current.Diff(target, "restore:x", 0, current.Lamport)
	applyAll(t, current, ops)

	m := current.Materialize()
	var got string
	json.Unmarshal(m.Registers[CellKey("Hoja1", 1, 1)], &got)
	if got != "=A1*B1" {
		t.Fatalf("celda 1:1 tras restaurar = %q, quiero =A1*B1", got)
	}
	// La celda sobrante debe quedar neutralizada (null).
	if raw, ok := m.Registers[CellKey("Hoja1", 2, 2)]; ok && !isNullJSON(raw) {
		t.Fatalf("celda sobrante no neutralizada: %s", raw)
	}
}

func TestDiffRestoreConvergesAcrossReplicas(t *testing.T) {
	// Dos réplicas parten del mismo actual y aplican el MISMO diff en distinto
	// orden de recepción; deben converger al contenido de la versión.
	makeCurrent := func() *State { return buildText("A", "abcXYZ") }
	target := buildText("A", "abc") // versión: solo "abc"

	c1 := makeCurrent()
	ops := c1.Diff(target, "restore:r1", 0, c1.Lamport)
	applyAll(t, c1, ops)

	// Réplica 2 recibe las mismas ops en orden inverso.
	c2 := makeCurrent()
	rev := make([]Op, len(ops))
	for i := range ops {
		rev[i] = ops[len(ops)-1-i]
	}
	applyAll(t, c2, rev)

	if c1.PlainText() != c2.PlainText() {
		t.Fatalf("réplicas divergen: %q vs %q", c1.PlainText(), c2.PlainText())
	}
	if c1.PlainText() != "abc" {
		t.Fatalf("restauración incorrecta: %q, quiero abc", c1.PlainText())
	}
}

func TestVersionVector(t *testing.T) {
	s := buildText("A", "abc")
	s.Apply(Op{Kind: OpSet, Key: "doc:theme", Value: json.RawMessage(`"dark"`), Dot: Dot{Actor: "B", Counter: 5}, Lamport: 9})
	vv := s.VersionVector()
	if vv["A"] != 3 {
		t.Fatalf("vv[A] = %d, quiero 3", vv["A"])
	}
	if vv["B"] != 5 {
		t.Fatalf("vv[B] = %d, quiero 5", vv["B"])
	}
	if !vv.Contains(Dot{Actor: "A", Counter: 2}) {
		t.Fatal("vv debe contener A:2")
	}
	if vv.Contains(Dot{Actor: "A", Counter: 4}) {
		t.Fatal("vv no debe contener A:4 (no observado)")
	}
	if !vv.Contains(Dot{}) {
		t.Fatal("vv debe contener el Dot raíz")
	}
}

func TestRGACycleSafety(t *testing.T) {
	// Estado corrupto/malicioso: dos elementos que se referencian mutuamente en
	// After (ciclo). rgaOrder no debe colgarse ni desbordar la pila.
	s := NewState()
	a := Dot{Actor: "X", Counter: 1}
	b := Dot{Actor: "X", Counter: 2}
	va, _ := json.Marshal("a")
	vb, _ := json.Marshal("b")
	s.Elements = []element{
		{Dot: a, After: b, Value: va}, // a apunta a b
		{Dot: b, After: a, Value: vb}, // b apunta a a -> ciclo
	}
	// No debe entrar en bucle infinito; los huérfanos (sin cadena a la raíz) se
	// omiten, así que el texto queda vacío pero la llamada TERMINA.
	_ = s.PlainText()
}

func TestRGADeterministicWithConcurrentInserts(t *testing.T) {
	// Inserciones concurrentes en la raíz por dos actores: el orden debe ser
	// determinista (mayor Counter primero, desempate por actor).
	s := NewState()
	vx, _ := json.Marshal("X")
	vy, _ := json.Marshal("Y")
	s.Apply(Op{Kind: OpInsert, Dot: Dot{Actor: "A", Counter: 1}, Value: vx, Lamport: 1})
	s.Apply(Op{Kind: OpInsert, Dot: Dot{Actor: "B", Counter: 1}, Value: vy, Lamport: 1})
	got := s.PlainText()
	// Mismo Counter (1); desempate por actor: A < B, y RGA ordena Dot descendente,
	// por lo que B (mayor en Less) va primero.
	if got != "YX" {
		t.Fatalf("orden RGA determinista esperado YX, got %q", got)
	}
}
