// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"testing"
)

func TestGestoRefrescoMailConservaBusquedaYRespetaCiclo(t *testing.T) {
	s := servicioDeEjemplo()
	b, _ := buzonDePrueba(t, s)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	b.Arrancar(ctx)
	b.consulta = "contrato"
	s.llamadas = nil
	r := &b.P.Bandeja.Refresco
	if !r.Puede() {
		t.Fatal("loaded mailbox cannot refresh")
	}
	r.Actualizar()
	if s.pidio("Search") != 1 || s.pidio("Messages") != 0 || b.consulta != "contrato" {
		t.Fatalf("pull did not preserve the search: %v, query %q", s.llamadas, b.consulta)
	}
	b.P.Bandeja.Estado = Cargando
	if r.Puede() {
		t.Fatal("loading mailbox admits another pull")
	}
	b.P.Bandeja.Estado = Vacio
	if !r.Puede() {
		t.Fatal("empty mailbox cannot refresh")
	}
	cancel()
	if r.Puede() {
		t.Fatal("cancelled mailbox admits a pull")
	}
}
