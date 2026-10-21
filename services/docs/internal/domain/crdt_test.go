// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"errors"
	"testing"
)

// ch construye un Op de inserción de un carácter tras `after`.
func ch(actor string, counter uint64, after *Dot, lamport uint64, c string) Op {
	v, _ := json.Marshal(c)
	return Op{Kind: OpInsert, Dot: Dot{Actor: actor, Counter: counter}, After: after, Value: v, Lamport: lamport}
}

func TestRGAConvergenceOrderIndependent(t *testing.T) {
	// Dos réplicas insertan "AB" y "C" concurrentemente al inicio.
	a1 := ch("A", 1, nil, 1, "A")
	a2 := ch("A", 2, &Dot{Actor: "A", Counter: 1}, 2, "B")
	c1 := ch("C", 1, nil, 1, "C") // concurrente con a1 al inicio

	ops := []Op{a1, a2, c1}

	// Aplicar en dos órdenes distintos debe converger al mismo texto.
	orderings := [][]Op{
		{ops[0], ops[1], ops[2]},
		{ops[2], ops[0], ops[1]},
	}
	results := make([]string, 0, len(orderings))
	for _, ord := range orderings {
		s := NewState()
		for _, op := range ord {
			if _, err := s.Apply(op); err != nil {
				t.Fatalf("apply: %v", err)
			}
		}
		results = append(results, s.PlainText())
	}
	if results[0] != results[1] {
		t.Fatalf("CRDT no convergió: %q vs %q", results[0], results[1])
	}
	if len(results[0]) != 3 {
		t.Fatalf("esperados 3 caracteres, got %q", results[0])
	}
}

func TestApplyIdempotent(t *testing.T) {
	s := NewState()
	op := ch("A", 1, nil, 1, "X")
	changed1, _ := s.Apply(op)
	changed2, _ := s.Apply(op) // duplicado
	if !changed1 || changed2 {
		t.Fatalf("insert debe aplicarse una vez: %v %v", changed1, changed2)
	}
	if s.PlainText() != "X" {
		t.Fatalf("texto esperado X, got %q", s.PlainText())
	}
}

func TestDeleteTombstone(t *testing.T) {
	s := NewState()
	s.Apply(ch("A", 1, nil, 1, "X"))
	s.Apply(ch("A", 2, &Dot{Actor: "A", Counter: 1}, 2, "Y"))
	changed, _ := s.Apply(Op{Kind: OpDelete, Target: &Dot{Actor: "A", Counter: 1}, Lamport: 3})
	if !changed {
		t.Fatal("delete debe cambiar el estado")
	}
	if s.PlainText() != "Y" {
		t.Fatalf("tras borrar X esperado Y, got %q", s.PlainText())
	}
	// Borrar de nuevo es idempotente.
	if changed, _ := s.Apply(Op{Kind: OpDelete, Target: &Dot{Actor: "A", Counter: 1}, Lamport: 4}); changed {
		t.Fatal("delete repetido no debe cambiar el estado")
	}
}

func TestLWWRegister(t *testing.T) {
	s := NewState()
	v1, _ := json.Marshal("=A1+B1")
	v2, _ := json.Marshal("=A1*B1")
	// Lamport menor pierde, aunque llegue después.
	s.Apply(Op{Kind: OpSet, Key: "Sheet1!1:1", Value: v2, Dot: Dot{Actor: "B", Counter: 1}, Lamport: 5})
	s.Apply(Op{Kind: OpSet, Key: "Sheet1!1:1", Value: v1, Dot: Dot{Actor: "A", Counter: 1}, Lamport: 3})
	m := s.Materialize()
	var got string
	json.Unmarshal(m.Registers["Sheet1!1:1"], &got)
	if got != "=A1*B1" {
		t.Fatalf("LWW debe conservar el mayor Lamport, got %q", got)
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	s := NewState()
	s.Apply(ch("A", 1, nil, 1, "H"))
	s.Apply(ch("A", 2, &Dot{Actor: "A", Counter: 1}, 2, "i"))
	raw, err := s.JSON()
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	s2, err := StateFromJSON(raw)
	if err != nil {
		t.Fatalf("from json: %v", err)
	}
	if s2.PlainText() != "Hi" {
		t.Fatalf("round-trip esperado Hi, got %q", s2.PlainText())
	}
}

func TestResolveRole(t *testing.T) {
	doc := &Document{OwnerSub: "owner-1"}
	perms := []Permission{
		{PrincipalKind: PrincipalUser, PrincipalID: "u-2", Role: RoleViewer},
		{PrincipalKind: PrincipalGroup, PrincipalID: "org/acme:eng", Role: RoleEditor},
		{PrincipalKind: PrincipalTenant, PrincipalID: "acme", Role: RoleCommenter},
	}

	// Owner siempre owner.
	if r, ok := ResolveRole(doc, perms, Subject{Sub: "owner-1"}); !ok || r != RoleOwner {
		t.Fatalf("owner esperado, got %v %v", r, ok)
	}
	// Usuario por grupo obtiene el máximo (editor > commenter del tenant).
	if r, ok := ResolveRole(doc, perms, Subject{Sub: "u-9", Groups: []string{"org/acme:eng"}, Tenant: "acme"}); !ok || r != RoleEditor {
		t.Fatalf("editor esperado por grupo, got %v %v", r, ok)
	}
	// Usuario sin entradas pero del tenant => commenter.
	if r, ok := ResolveRole(doc, perms, Subject{Sub: "u-x", Tenant: "acme"}); !ok || r != RoleCommenter {
		t.Fatalf("commenter por tenant esperado, got %v %v", r, ok)
	}
	// Forastero sin acceso.
	if _, ok := ResolveRole(doc, perms, Subject{Sub: "stranger", Tenant: "other"}); ok {
		t.Fatal("forastero no debe tener acceso (deny-by-default)")
	}
}

// TestUnInsertQueEntraEsUnCaracter (FYE-05): el servicio rechaza un insert cuyo
// valor no es una cadena de una runa, y sigue reproduciendo un registro anterior a
// la regla que tuviera alguno.
func TestUnInsertQueEntraEsUnCaracter(t *testing.T) {
	op := func(v string) Op {
		return Op{Kind: OpInsert, Dot: Dot{Actor: "eva", Counter: 1}, Value: json.RawMessage(v)}
	}
	for _, v := range []string{`"a"`, `"😀"`, `"é"`} {
		if err := op(v).Validate(); err != nil {
			t.Errorf("%s: %v", v, err)
		}
	}
	for _, v := range []string{`"abc"`, `""`, `"👍🏽"`, `{"x":1}`, `7`} {
		if err := op(v).Validate(); !errors.Is(err, ErrInvalidOp) {
			t.Errorf("%s: %v, quiero ErrInvalidOp", v, err)
		}
	}
	var s State
	if _, err := s.Apply(op(`"abc"`)); err != nil {
		t.Fatalf("un registro antiguo con un insert de varios caracteres deja de abrirse: %v", err)
	}
}
