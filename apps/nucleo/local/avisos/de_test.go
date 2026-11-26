// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package avisos

import (
	"strings"
	"testing"
	"time"
)

func TestAvisosDeEventoYDeNota(t *testing.T) {
	inicio := time.Date(2026, 12, 1, 9, 30, 0, 0, time.UTC)
	a := DeEvento("e1", "Dentista", "Calle Mayor 1", inicio, false, 15)
	if a.Clave != "e1/2026-12-01T09:30:00Z/15" || !a.Cuando.Equal(inicio.Add(-15*time.Minute)) ||
		!strings.HasSuffix(a.Cuerpo, " · Calle Mayor 1") || !strings.Contains(a.Enlace, ":/abrir?clase=evento&id=e1") {
		t.Fatalf("%+v", a)
	}
	if d := DeEvento("e2", "Fiesta", "", inicio, true, 0); d.Cuerpo != "2026-12-01" {
		t.Fatalf("todo el día: %+v", d)
	}
	n := DeNota("n1", "", inicio, false)
	if n.Titulo != "Sytue Notepads" || n.Clave != "n1/2026-12-01T09:30:00Z" || !strings.Contains(n.Enlace, ":/abrir?clase=nota&id=n1") {
		t.Fatalf("%+v", n)
	}
}
