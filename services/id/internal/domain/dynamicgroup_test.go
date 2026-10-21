// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

// TestEvaluateRule cubre el motor puro: cada operador, AND/OR, atributo ausente,
// case-insensitivity y la regla vacía (no incluye a nadie).
func TestEvaluateRule(t *testing.T) {
	attrs := map[string]string{
		AttrEmail:      "ada@sales.acme.eu",
		AttrDepartment: "Sales",
		AttrTitle:      "Account Executive",
		AttrLocation:   "Madrid",
		// AttrName ausente a propósito.
	}
	cond := func(a string, op RuleOp, v string) Condition { return Condition{Attribute: a, Op: op, Value: v} }

	cases := []struct {
		name string
		rule Rule
		want bool
	}{
		{"eq match (case-insensitive)", Rule{Match: "all", Conditions: []Condition{cond(AttrDepartment, OpEquals, "sales")}}, true},
		{"eq no match", Rule{Match: "all", Conditions: []Condition{cond(AttrDepartment, OpEquals, "eng")}}, false},
		{"ne match", Rule{Match: "all", Conditions: []Condition{cond(AttrDepartment, OpNotEquals, "eng")}}, true},
		{"ne no match", Rule{Match: "all", Conditions: []Condition{cond(AttrDepartment, OpNotEquals, "sales")}}, false},
		{"contains", Rule{Match: "all", Conditions: []Condition{cond(AttrTitle, OpContains, "executive")}}, true},
		{"contains no", Rule{Match: "all", Conditions: []Condition{cond(AttrTitle, OpContains, "manager")}}, false},
		{"starts_with", Rule{Match: "all", Conditions: []Condition{cond(AttrEmail, OpStartsWith, "ada@")}}, true},
		{"starts_with no", Rule{Match: "all", Conditions: []Condition{cond(AttrEmail, OpStartsWith, "bob@")}}, false},
		{"in match", Rule{Match: "all", Conditions: []Condition{{Attribute: AttrLocation, Op: OpIn, Values: []string{"Lisboa", "madrid"}}}}, true},
		{"in no match", Rule{Match: "all", Conditions: []Condition{{Attribute: AttrLocation, Op: OpIn, Values: []string{"Lisboa", "Paris"}}}}, false},

		// AND: todas deben cumplir.
		{"and both true", Rule{Match: "all", Conditions: []Condition{cond(AttrDepartment, OpEquals, "sales"), cond(AttrLocation, OpEquals, "madrid")}}, true},
		{"and one false", Rule{Match: "all", Conditions: []Condition{cond(AttrDepartment, OpEquals, "sales"), cond(AttrLocation, OpEquals, "lisboa")}}, false},
		// OR: basta una.
		{"or one true", Rule{Match: "any", Conditions: []Condition{cond(AttrDepartment, OpEquals, "eng"), cond(AttrLocation, OpEquals, "madrid")}}, true},
		{"or none", Rule{Match: "any", Conditions: []Condition{cond(AttrDepartment, OpEquals, "eng"), cond(AttrLocation, OpEquals, "lisboa")}}, false},

		// Atributo ausente (name): "" == "" para eq vacío no aplica; con ne concreto sí cumple.
		{"absent attr eq empty-ish", Rule{Match: "all", Conditions: []Condition{cond(AttrName, OpEquals, "ada")}}, false},
		{"absent attr ne", Rule{Match: "all", Conditions: []Condition{cond(AttrName, OpNotEquals, "ada")}}, true},

		// Regla vacía / operador desconocido ⇒ no incluye.
		{"empty rule", Rule{Match: "all"}, false},
		{"unknown op", Rule{Match: "all", Conditions: []Condition{cond(AttrDepartment, RuleOp("regex"), "x")}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EvaluateRule(tc.rule, attrs); got != tc.want {
				t.Fatalf("EvaluateRule(%+v) = %v; want %v", tc.rule, got, tc.want)
			}
		})
	}
}

func TestRuleValidate(t *testing.T) {
	ok := Rule{Match: "all", Conditions: []Condition{{Attribute: AttrDepartment, Op: OpEquals, Value: "Sales"}}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("regla válida rechazada: %v", err)
	}
	bad := []Rule{
		{Match: "xor", Conditions: []Condition{{Attribute: AttrDepartment, Op: OpEquals, Value: "x"}}}, // match inválido
		{Match: "all"}, // sin condiciones
		{Match: "all", Conditions: []Condition{{Attribute: "salary", Op: OpEquals, Value: "x"}}},             // atributo fuera del catálogo
		{Match: "all", Conditions: []Condition{{Attribute: AttrDepartment, Op: OpEquals}}},                   // eq sin valor
		{Match: "all", Conditions: []Condition{{Attribute: AttrDepartment, Op: OpIn}}},                       // in sin valores
		{Match: "all", Conditions: []Condition{{Attribute: AttrDepartment, Op: RuleOp("nope"), Value: "x"}}}, // op inválido
	}
	for i, r := range bad {
		if err := r.Validate(); err == nil {
			t.Fatalf("caso inválido %d aceptado: %+v", i, r)
		}
	}
}
