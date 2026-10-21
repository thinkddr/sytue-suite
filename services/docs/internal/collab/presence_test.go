// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package collab

import (
	"testing"
	"time"
)

func TestPresenceTouch(t *testing.T) {
	p := NewPresenceRegistry()
	t0 := time.Now()

	// Primer heartbeat: el propio solicitante ya cuenta como presente.
	if m := p.touchAt("doc", "connA", "userX", t0); len(m) != 1 || m[0].Conn != "connA" {
		t.Fatalf("primer touch: %v, want [connA]", m)
	}

	// Dos pestañas del MISMO usuario = dos presencias (el cliente excluye la
	// suya, así que la otra pestaña aparece como ≥1 avatar).
	m := p.touchAt("doc", "connB", "userX", t0.Add(time.Second))
	if len(m) != 2 {
		t.Fatalf("segundo touch: got %d miembros, want 2", len(m))
	}

	// connA no renueva: pasado el TTL se purga al siguiente touch de connB.
	m = p.touchAt("doc", "connB", "userX", t0.Add(PresenceTTL+2*time.Second))
	if len(m) != 1 || m[0].Conn != "connB" || m[0].Sub != "userX" {
		t.Fatalf("tras TTL: %v, want solo connB(userX)", m)
	}
}
