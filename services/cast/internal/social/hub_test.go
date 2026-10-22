// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package social

import (
	"testing"

	"github.com/google/uuid"
)

func clienteDePrueba() *Cliente {
	return &Cliente{salida: make(chan Trama, colaDeSalida)}
}

func drenar(c *Cliente) []Trama {
	var out []Trama
	for {
		select {
		case t := <-c.salida:
			out = append(out, t)
		default:
			return out
		}
	}
}

func TestHubDifundeATodaLaSala(t *testing.T) {
	t.Parallel()
	h := NewHub()
	v := uuid.New()
	a, b := clienteDePrueba(), clienteDePrueba()
	if n := h.alta("acme", v, a); n != 1 {
		t.Fatalf("alta = %d, quiero 1", n)
	}
	if n := h.alta("acme", v, b); n != 2 {
		t.Fatalf("alta = %d, quiero 2", n)
	}
	h.Difundir("acme", v, Trama{Type: TramaMensaje, Body: "hola"})
	for nombre, c := range map[string]*Cliente{"a": a, "b": b} {
		if got := drenar(c); len(got) != 1 || got[0].Body != "hola" {
			t.Errorf("%s recibió %+v", nombre, got)
		}
	}
	if n := h.baja("acme", v, a); n != 1 {
		t.Fatalf("baja = %d, quiero 1", n)
	}
	h.Difundir("acme", v, Trama{Type: TramaMensaje})
	if got := drenar(a); len(got) != 0 {
		t.Errorf("a se dio de baja y recibió %+v", got)
	}
	if n := h.baja("acme", v, b); n != 0 || h.Espectadores("acme", v) != 0 {
		t.Fatalf("la sala debería quedar vacía y borrada")
	}
}

// Dos vídeos distintos no se oyen; y el mismo id en OTRA organización tampoco.
func TestHubAislaPorVideoYPorTenant(t *testing.T) {
	t.Parallel()
	h := NewHub()
	v1, v2 := uuid.New(), uuid.New()
	enV1, enV2, otraOrg := clienteDePrueba(), clienteDePrueba(), clienteDePrueba()
	h.alta("acme", v1, enV1)
	h.alta("acme", v2, enV2)
	h.alta("globex", v1, otraOrg)

	h.Difundir("acme", v1, Trama{Type: TramaMensaje, Body: "solo v1 de acme"})
	if got := drenar(enV1); len(got) != 1 {
		t.Fatalf("v1 recibió %+v", got)
	}
	if got := drenar(enV2); len(got) != 0 {
		t.Errorf("v2 oyó lo de v1: %+v", got)
	}
	if got := drenar(otraOrg); len(got) != 0 {
		t.Errorf("otra organización oyó lo de acme: %+v", got)
	}
	if h.Espectadores("acme", v1) != 1 || h.Espectadores("globex", v1) != 1 {
		t.Errorf("los recuentos se mezclan entre organizaciones")
	}
}

// Difundir a una sala vacía o inexistente no hace nada ni falla.
func TestHubDifundirSinSala(t *testing.T) {
	t.Parallel()
	h := NewHub()
	h.Difundir("acme", uuid.New(), Trama{Type: TramaMensaje})
	if h.Espectadores("acme", uuid.New()) != 0 {
		t.Fatal("una sala inexistente tiene 0 espectadores")
	}
}
