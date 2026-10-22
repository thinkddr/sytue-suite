// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

// TestTeamRoleAtLeast verifica el orden de privilegio y —clave— el deny-by-default:
// un rol inválido/desconocido NO concede nada.
func TestTeamRoleAtLeast(t *testing.T) {
	cases := []struct {
		have, need TeamRole
		want       bool
	}{
		{TeamManager, TeamManager, true},
		{TeamManager, TeamEditor, true},
		{TeamManager, TeamViewer, true},
		{TeamEditor, TeamManager, false},
		{TeamEditor, TeamEditor, true},
		{TeamEditor, TeamViewer, true},
		{TeamViewer, TeamEditor, false},
		{TeamViewer, TeamViewer, true},
		// Deny-by-default: un rol basura no alcanza ni el mínimo.
		{TeamRole("root"), TeamViewer, false},
		{TeamRole(""), TeamViewer, false},
	}
	for _, c := range cases {
		if got := c.have.AtLeast(c.need); got != c.want {
			t.Errorf("%q.AtLeast(%q) = %v, quiero %v", c.have, c.need, got, c.want)
		}
	}
}

func TestTeamRoleCapabilities(t *testing.T) {
	if TeamViewer.CanWrite() {
		t.Error("viewer no debe poder escribir")
	}
	if !TeamEditor.CanWrite() {
		t.Error("editor debe poder escribir")
	}
	if TeamEditor.CanManage() {
		t.Error("editor no debe poder gestionar")
	}
	if !TeamManager.CanManage() {
		t.Error("manager debe poder gestionar")
	}
	// Rol inválido: sin capacidades (deny-by-default).
	bad := TeamRole("owner")
	if bad.CanWrite() || bad.CanManage() {
		t.Error("un rol inválido no debe conceder capacidades")
	}
}

func TestValidTeamRole(t *testing.T) {
	for _, r := range []TeamRole{TeamViewer, TeamEditor, TeamManager} {
		if !ValidTeamRole(r) {
			t.Errorf("%q debería ser válido", r)
		}
	}
	for _, r := range []TeamRole{"", "admin", "owner", "Viewer"} {
		if ValidTeamRole(r) {
			t.Errorf("%q no debería ser válido", r)
		}
	}
}
