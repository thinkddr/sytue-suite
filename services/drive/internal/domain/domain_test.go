// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

func TestValidName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"vacio", "", false},
		{"solo espacios", "   ", false},
		{"normal", "informe.pdf", true},
		{"con espacios internos", "mi documento.txt", true},
		{"punto", ".", false},
		{"doble punto (traversal)", "..", false},
		{"barra (traversal)", "a/b", false},
		{"barra invertida", "a\\b", false},
		{"nul byte", "a\x00b", false},
		{"control char", "a\tb", false},
		{"unicode valido", "résumé.doc", true},
		{"demasiado largo", string(make([]byte, 256)), false},
		{"limite 255", string(repeat('a', 255)), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ValidName(c.in); got != c.want {
				t.Fatalf("ValidName(%q)=%v, quiero %v", c.in, got, c.want)
			}
		})
	}
}

func repeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func TestQuotaAvailableAndRoom(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		limit     int64
		used      int64
		wantAvail int64
		room      int64
		hasRoom   bool
	}{
		{"vacia", 100, 0, 100, 100, true},
		{"parcial", 100, 60, 40, 40, true},
		{"justo llena", 100, 100, 0, 1, false},
		{"sobrepasada (defensivo)", 100, 120, 0, 1, false},
		{"cabe exacto", 100, 90, 10, 10, true},
		{"no cabe por 1", 100, 90, 10, 11, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := Quota{LimitBytes: c.limit, UsedBytes: c.used}
			if got := q.Available(); got != c.wantAvail {
				t.Fatalf("Available()=%d, quiero %d", got, c.wantAvail)
			}
			if got := q.HasRoomFor(c.room); got != c.hasRoom {
				t.Fatalf("HasRoomFor(%d)=%v, quiero %v", c.room, got, c.hasRoom)
			}
		})
	}
}

func TestValidPermissionAndShareKind(t *testing.T) {
	t.Parallel()
	if !ValidPermission(PermRead) || !ValidPermission(PermWrite) {
		t.Fatal("read/write deben ser válidos")
	}
	if ValidPermission(Permission("admin")) {
		t.Fatal("admin no es un permiso válido")
	}
	for _, k := range []ShareKind{ShareLink, ShareUser, ShareGroup} {
		if !ValidShareKind(k) {
			t.Fatalf("%q debe ser un kind válido", k)
		}
	}
	if ValidShareKind(ShareKind("public")) {
		t.Fatal("public no es un kind válido")
	}
}
