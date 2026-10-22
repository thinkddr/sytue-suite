// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestKindValid(t *testing.T) {
	for _, k := range []Kind{KindDocument, KindSpreadsheet, KindPresentation} {
		if !k.Valid() {
			t.Errorf("Kind %q debería ser válido", k)
		}
	}
	if Kind("bogus").Valid() {
		t.Error("Kind bogus no debería ser válido")
	}
}

func TestRoleHelpers(t *testing.T) {
	for _, r := range []Role{RoleViewer, RoleCommenter, RoleEditor, RoleOwner} {
		if !r.Valid() {
			t.Errorf("Role %q debería ser válido", r)
		}
	}
	if Role("bogus").Valid() {
		t.Error("Role bogus no debería ser válido")
	}

	// AtLeast: orden creciente de poder.
	if !RoleOwner.AtLeast(RoleEditor) {
		t.Error("owner AtLeast editor")
	}
	if RoleViewer.AtLeast(RoleEditor) {
		t.Error("viewer no AtLeast editor")
	}

	tests := []struct {
		r                            Role
		read, comment, write, manage bool
	}{
		{RoleViewer, true, false, false, false},
		{RoleCommenter, true, true, false, false},
		{RoleEditor, true, true, true, false},
		{RoleOwner, true, true, true, true},
	}
	for _, tc := range tests {
		if tc.r.CanRead() != tc.read || tc.r.CanComment() != tc.comment ||
			tc.r.CanWrite() != tc.write || tc.r.CanManage() != tc.manage {
			t.Errorf("capacidades de %q incorrectas", tc.r)
		}
	}
}

func TestPrincipalKindValid(t *testing.T) {
	for _, p := range []PrincipalKind{PrincipalUser, PrincipalGroup, PrincipalTenant, PrincipalPublic} {
		if !p.Valid() {
			t.Errorf("PrincipalKind %q debería ser válido", p)
		}
	}
	if PrincipalKind("nope").Valid() {
		t.Error("PrincipalKind nope no debería ser válido")
	}
}

func TestBlobFormatValid(t *testing.T) {
	for _, f := range []BlobFormat{FormatCRDT, FormatOOXML, FormatODF, FormatPDF, FormatHTML, FormatMarkdown} {
		if !f.Valid() {
			t.Errorf("BlobFormat %q debería ser válido", f)
		}
	}
	if BlobFormat("nope").Valid() {
		t.Error("BlobFormat nope no debería ser válido")
	}
}

func TestShareLinkActive(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	if (ShareLink{}).Active(now) != true {
		t.Error("enlace sin revocación ni expiración debe estar activo")
	}
	if (ShareLink{RevokedAt: &past}).Active(now) {
		t.Error("enlace revocado no debe estar activo")
	}
	if (ShareLink{ExpiresAt: &past}).Active(now) {
		t.Error("enlace expirado no debe estar activo")
	}
	if !(ShareLink{ExpiresAt: &future}).Active(now) {
		t.Error("enlace con expiración futura debe estar activo")
	}
}

func TestResolveShare(t *testing.T) {
	link := &ShareLink{Role: RoleEditor}
	// nowAfter=false (activo) -> concede el rol.
	if r, ok := ResolveShare(link, func(*ShareLink) bool { return false }); !ok || r != RoleEditor {
		t.Errorf("ResolveShare activo: got (%q,%v)", r, ok)
	}
	// nowAfter=true (inactivo) -> deny.
	if _, ok := ResolveShare(link, func(*ShareLink) bool { return true }); ok {
		t.Error("ResolveShare inactivo debe denegar")
	}
	// link nil -> deny (sin invocar nowAfter).
	if _, ok := ResolveShare(nil, func(*ShareLink) bool { return false }); ok {
		t.Error("ResolveShare(nil) debe denegar")
	}
}

func TestResolveRoleGroupsTenantPublic(t *testing.T) {
	doc := &Document{ID: "d1", OwnerSub: "owner"}
	perms := []Permission{
		{PrincipalKind: PrincipalUser, PrincipalID: "u1", Role: RoleViewer},
		{PrincipalKind: PrincipalGroup, PrincipalID: "g1", Role: RoleEditor},
		{PrincipalKind: PrincipalTenant, PrincipalID: "acme", Role: RoleCommenter},
		{PrincipalKind: PrincipalPublic, PrincipalID: "", Role: RoleOwner},
	}

	// Owner siempre gana.
	if r, ok := ResolveRole(doc, perms, Subject{Sub: "owner"}); !ok || r != RoleOwner {
		t.Errorf("owner: got (%q,%v)", r, ok)
	}
	// Usuario u1 con permiso viewer, además grupo g1 editor -> máximo = editor.
	if r, ok := ResolveRole(doc, perms, Subject{Sub: "u1", Groups: []string{"g1"}}); !ok || r != RoleEditor {
		t.Errorf("u1+g1: got (%q,%v)", r, ok)
	}
	// Solo tenant acme -> commenter.
	if r, ok := ResolveRole(doc, perms, Subject{Sub: "x", Tenant: "acme"}); !ok || r != RoleCommenter {
		t.Errorf("tenant: got (%q,%v)", r, ok)
	}
	// Public NO se resuelve por identidad; sin otra ACL -> sin acceso.
	if _, ok := ResolveRole(doc, perms, Subject{Sub: "stranger"}); ok {
		t.Error("stranger no debe tener acceso (public no cuenta)")
	}
}

func TestOpValidate(t *testing.T) {
	valNull := json.RawMessage("null")
	tests := []struct {
		name string
		op   Op
		ok   bool
	}{
		{"insert ok", Op{Kind: OpInsert, Dot: Dot{Actor: "a", Counter: 1}, Value: json.RawMessage(`"a"`)}, true},
		{"insert sin carácter (FYE-05)", Op{Kind: OpInsert, Dot: Dot{Actor: "a", Counter: 1}, Value: valNull}, false},
		{"insert sin dot", Op{Kind: OpInsert}, false},
		{"delete ok", Op{Kind: OpDelete, Target: &Dot{Actor: "a", Counter: 1}}, true},
		{"delete sin target", Op{Kind: OpDelete}, false},
		{"delete target cero", Op{Kind: OpDelete, Target: &Dot{}}, false},
		{"set ok", Op{Kind: OpSet, Key: "k", Value: valNull}, true},
		{"set sin key", Op{Kind: OpSet}, false},
		{"kind desconocido", Op{Kind: OpKind("weird")}, false},
	}
	for _, tc := range tests {
		err := tc.op.Validate()
		if tc.ok && err != nil {
			t.Errorf("%s: esperaba nil, got %v", tc.name, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("%s: esperaba error", tc.name)
		}
	}
}

func TestDotLess(t *testing.T) {
	// Mayor counter -> Less es por counter.
	if !(Dot{Actor: "a", Counter: 1}).Less(Dot{Actor: "a", Counter: 2}) {
		t.Error("counter menor debe ser Less")
	}
	// Mismo counter: desempate por actor.
	if !(Dot{Actor: "a", Counter: 1}).Less(Dot{Actor: "b", Counter: 1}) {
		t.Error("mismo counter: actor menor debe ser Less")
	}
	if (Dot{Actor: "b", Counter: 1}).Less(Dot{Actor: "a", Counter: 1}) {
		t.Error("actor mayor no debe ser Less")
	}
}

func TestStateFromJSONInvalid(t *testing.T) {
	if _, err := StateFromJSON(json.RawMessage("{not json")); err == nil {
		t.Error("StateFromJSON con basura debe fallar")
	}
	// Vacío -> estado vacío usable.
	s, err := StateFromJSON(nil)
	if err != nil || s == nil || s.Registers == nil {
		t.Fatalf("StateFromJSON(nil): %v", err)
	}
}

func TestMaterializeAndPlainText(t *testing.T) {
	s := NewState()
	mustApply := func(op Op) {
		if _, err := s.Apply(op); err != nil {
			t.Fatalf("Apply: %v", err)
		}
	}
	ch := func(a string, c uint64, after *Dot, val string) Op {
		v, _ := json.Marshal(val)
		return Op{Kind: OpInsert, Dot: Dot{Actor: a, Counter: c}, After: after, Value: v, Lamport: c}
	}
	d1 := Dot{Actor: "a", Counter: 1}
	d2 := Dot{Actor: "a", Counter: 2}
	mustApply(ch("a", 1, nil, "H"))
	mustApply(ch("a", 2, &d1, "i"))
	mustApply(ch("a", 3, &d2, "!"))

	if got := s.PlainText(); got != "Hi!" {
		t.Errorf("PlainText = %q, want Hi!", got)
	}
	m := s.Materialize()
	if len(m.Sequence) != 3 {
		t.Errorf("Materialize sequence len = %d, want 3", len(m.Sequence))
	}

	// Registro LWW aparece en Materialize.Registers.
	rv, _ := json.Marshal("bold")
	mustApply(Op{Kind: OpSet, Key: "fmt:a:1:b", Value: rv, Dot: Dot{Actor: "a", Counter: 4}, Lamport: 4})
	m = s.Materialize()
	if _, ok := m.Registers["fmt:a:1:b"]; !ok {
		t.Error("Materialize debe incluir el registro LWW")
	}
}

func TestVersionVectorContains(t *testing.T) {
	s := NewState()
	v, _ := json.Marshal("x")
	_, _ = s.Apply(Op{Kind: OpInsert, Dot: Dot{Actor: "a", Counter: 3}, Value: v, Lamport: 1})
	vv := s.VersionVector()
	if !vv.Contains(Dot{Actor: "a", Counter: 2}) {
		t.Error("vv debe contener counter menor del mismo actor")
	}
	if vv.Contains(Dot{Actor: "a", Counter: 4}) {
		t.Error("vv no debe contener counter mayor no visto")
	}
	if vv.Contains(Dot{Actor: "b", Counter: 1}) {
		t.Error("vv no debe contener actor desconocido")
	}
	if !vv.Contains(Dot{}) {
		t.Error("vv siempre contiene el Dot cero")
	}
}
