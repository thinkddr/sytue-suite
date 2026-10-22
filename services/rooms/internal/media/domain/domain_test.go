// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"testing"
	"time"
)

func TestRoomStatusValid(t *testing.T) {
	for _, s := range []RoomStatus{RoomScheduled, RoomActive, RoomEnded} {
		if !s.Valid() {
			t.Errorf("%s debería ser válido", s)
		}
	}
	if RoomStatus("paused").Valid() {
		t.Error("estado desconocido no debe ser válido")
	}
}

func TestRolePermissions(t *testing.T) {
	if !RoleHost.CanPublish() || !RoleSpeaker.CanPublish() {
		t.Error("host y speaker pueden publicar")
	}
	if RoleViewer.CanPublish() {
		t.Error("viewer no puede publicar")
	}
	if !RoleHost.IsAdmin() {
		t.Error("host es admin")
	}
	if RoleSpeaker.IsAdmin() || RoleViewer.IsAdmin() {
		t.Error("speaker/viewer no son admin")
	}
}

func TestValidRole(t *testing.T) {
	for _, r := range []Role{RoleHost, RoleSpeaker, RoleViewer} {
		if !ValidRole(r) {
			t.Errorf("%s debería ser válido", r)
		}
	}
	if ValidRole(Role("moderator")) {
		t.Error("rol desconocido no debe ser válido")
	}
}

func TestValidTitle(t *testing.T) {
	if !ValidTitle("Reunión semanal") {
		t.Error("título normal debe ser válido")
	}
	if ValidTitle("") || ValidTitle("   ") {
		t.Error("título vacío/blanco no debe ser válido")
	}
	long := make([]byte, 201)
	for i := range long {
		long[i] = 'a'
	}
	if ValidTitle(string(long)) {
		t.Error("título >200 no debe ser válido")
	}
}

func TestValidRoomName(t *testing.T) {
	ok := []string{"room-abc123", "sala_1", "a", "room-9f8e7d"}
	bad := []string{"", "Room-X", "sala espacio", "sala.con.punto", "sala/slash"}
	for _, n := range ok {
		if !ValidRoomName(n) {
			t.Errorf("ValidRoomName(%q) = false, want true", n)
		}
	}
	for _, n := range bad {
		if ValidRoomName(n) {
			t.Errorf("ValidRoomName(%q) = true, want false", n)
		}
	}
	long := make([]byte, 129)
	for i := range long {
		long[i] = 'a'
	}
	if ValidRoomName(string(long)) {
		t.Error("nombre >128 no debe ser válido")
	}
}

func TestRoomIsJoinable(t *testing.T) {
	if !(Room{Status: RoomScheduled}).IsJoinable() || !(Room{Status: RoomActive}).IsJoinable() {
		t.Error("scheduled/active deben ser joinables")
	}
	if (Room{Status: RoomEnded}).IsJoinable() {
		t.Error("ended no debe ser joinable")
	}
}

func TestRoomSourceValid(t *testing.T) {
	// Directo: no requiere RefID.
	if !(RoomSource{Kind: SourceDirect}).Valid() {
		t.Error("origen directo debe ser válido sin ref_id")
	}
	// Cross-app: requiere RefID.
	if !(RoomSource{Kind: SourceCalendar, RefID: "evt-1"}).Valid() {
		t.Error("origen calendar con ref debe ser válido")
	}
	if (RoomSource{Kind: SourceCalendar}).Valid() {
		t.Error("origen calendar sin ref no debe ser válido")
	}
	if (RoomSource{Kind: SourceChat, RefID: "   "}).Valid() {
		t.Error("origen chat con ref en blanco no debe ser válido")
	}
	// Kind desconocido.
	if (RoomSource{Kind: SourceKind("slides"), RefID: "x"}).Valid() {
		t.Error("kind desconocido no debe ser válido")
	}
	// RefID demasiado largo.
	long := make([]byte, 257)
	for i := range long {
		long[i] = 'a'
	}
	if (RoomSource{Kind: SourceChat, RefID: string(long)}).Valid() {
		t.Error("ref_id >256 no debe ser válido")
	}
}

func TestSourceKindIsCrossApp(t *testing.T) {
	if !SourceCalendar.IsCrossAppKind() || !SourceChat.IsCrossAppKind() {
		t.Error("calendar/chat son cross-app")
	}
	if SourceDirect.IsCrossAppKind() {
		t.Error("directo no es cross-app")
	}
	if !(RoomSource{Kind: SourceChat, RefID: "c1"}).IsCrossApp() {
		t.Error("RoomSource chat es cross-app")
	}
	if (RoomSource{Kind: SourceDirect}).IsCrossApp() {
		t.Error("RoomSource directo no es cross-app")
	}
}

func TestParticipantIsPresent(t *testing.T) {
	if !(Participant{}).IsPresent() {
		t.Error("sin left_at debe estar presente")
	}
	now := time.Now()
	if (Participant{LeftAt: &now}).IsPresent() {
		t.Error("con left_at no debe estar presente")
	}
}

// TestSourceDeMetadata comprueba la lectura del origen guardado en la sala.
//
// Es lo que dice a qué canal pertenecía una llamada al cerrarla. Si devuelve
// basura en vez de vacío, el aviso de «terminada» se manda a un canal
// inventado; si revienta, el cierre de la sala se lleva por delante el webhook.
func TestSourceDeMetadata(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nombre string
		meta   map[string]any
		quiero RoomSource
	}{
		{
			"origen de chat bien formado",
			map[string]any{"source": map[string]any{"kind": "chat", "ref_id": "c-1", "title": "General"}},
			RoomSource{Kind: SourceChat, RefID: "c-1", Title: "General"},
		},
		{"metadata vacía", map[string]any{}, RoomSource{}},
		{"metadata nula", nil, RoomSource{}},
		{"source que no es un objeto", map[string]any{"source": "chat"}, RoomSource{}},
		{"source nulo", map[string]any{"source": nil}, RoomSource{}},
		{
			// La metadata es JSON libre de la base de datos: un número donde se
			// esperaba una cadena no puede tumbar el cierre de la sala.
			"campos con el tipo cambiado",
			map[string]any{"source": map[string]any{"kind": 42, "ref_id": []any{"x"}}},
			RoomSource{},
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := SourceDeMetadata(c.meta); got != c.quiero {
				t.Errorf("SourceDeMetadata = %+v, quería %+v", got, c.quiero)
			}
		})
	}
}
