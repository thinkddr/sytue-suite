// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import "testing"

// El slug de un grupo depende solo del CONJUNTO de subs, no del orden: cualquier
// permutación del mismo conjunto produce el mismo slug (idempotencia del group DM).
func TestDirectSlugNSetInvariant(t *testing.T) {
	a := directSlugN("ana", "beto", "caro")
	for _, perm := range [][]string{
		{"caro", "ana", "beto"},
		{"beto", "caro", "ana"},
		{"caro", "beto", "ana"},
	} {
		if got := directSlugN(perm...); got != a {
			t.Fatalf("slug depende del orden: %v → %q, quiero %q", perm, got, a)
		}
	}
	// Un conjunto distinto → slug distinto.
	if directSlugN("ana", "beto") == a {
		t.Fatalf("conjuntos distintos comparten slug")
	}
}

// directSlug (1:1) es exactamente directSlugN con dos subs: un DM 1:1 abierto por
// cualquiera de las dos vías cae en el mismo canal (retrocompatibilidad).
func TestDirectSlugMatchesN(t *testing.T) {
	if directSlug("ana", "beto") != directSlugN("ana", "beto") {
		t.Fatal("directSlug(a,b) debe igualar directSlugN(a,b)")
	}
	if directSlug("ana", "beto") != directSlug("beto", "ana") {
		t.Fatal("directSlug debe ser simétrico")
	}
	// Todos con el prefijo dm- y dentro del límite de 64 del slug.
	if s := directSlugN("x", "y", "z"); len(s) != 35 || s[:3] != "dm-" {
		t.Fatalf("slug mal formado: %q (len %d)", s, len(s))
	}
}
