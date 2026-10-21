// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package push

import (
	"testing"

	"github.com/google/uuid"
)

// CADA CUENTA RECIBE LO SUYO Y SOLO LO SUYO. Aquí viaja el asunto y el remitente
// de un correo: entregarlo a la cuenta equivocada es enseñar el correo de alguien
// a otra persona, y no habría error en ningún log porque el evento es válido.
func TestDispatchSoloALaCuentaDestino(t *testing.T) {
	t.Parallel()
	h := NewHub(4)
	a, b := uuid.New(), uuid.New()
	chA, cancelA := h.Subscribe("acme", a)
	defer cancelA()
	chB, cancelB := h.Subscribe("acme", b)
	defer cancelB()

	h.Dispatch("acme", Llegada{AccountID: a, Subject: "para A"})
	select {
	case l := <-chA:
		if l.Subject != "para A" {
			t.Errorf("A recibió %q", l.Subject)
		}
	default:
		t.Fatal("A no recibió lo suyo")
	}
	select {
	case l := <-chB:
		t.Fatalf("B recibió un correo de A: %q", l.Subject)
	default:
	}
}

// Y el tenant también separa: dos organizaciones pueden tener el mismo UUID de
// cuenta solo por casualidad, pero la clave no puede depender de que no pase.
func TestDispatchSeparaOrganizaciones(t *testing.T) {
	t.Parallel()
	h := NewHub(4)
	cuenta := uuid.New()
	ch, cancel := h.Subscribe("acme", cuenta)
	defer cancel()

	h.Dispatch("otra", Llegada{AccountID: cuenta, Subject: "de otra organización"})
	select {
	case l := <-ch:
		t.Fatalf("se entregó entre organizaciones: %q", l.Subject)
	default:
	}

	// Y LA MITAD POSITIVA, que es la que da valor a la de arriba. Sin ella el
	// test pasa aunque el reparto esté roto del todo: «no llegó nada» se cumple
	// solo con que nada funcione. Comprobado — al quitar el tenant de la clave de
	// Dispatch, la primera parte seguía en verde y solo esta lo caza.
	h.Dispatch("acme", Llegada{AccountID: cuenta, Subject: "de la mía"})
	select {
	case l := <-ch:
		if l.Subject != "de la mía" {
			t.Errorf("llegó %q", l.Subject)
		}
	default:
		t.Fatal("no llegó lo de la propia organización: el reparto no funciona")
	}
}

// UN CLIENTE LENTO NO PARA EL RELAY. Si el buzón de uno se llena se le descarta
// el aviso —el mensaje sigue en PostgreSQL y lo verá al sondear—, pero bloquear
// aquí dejaría sin avisos a TODOS los demás.
func TestDispatchNoSeBloqueaConUnClienteLleno(t *testing.T) {
	t.Parallel()
	h := NewHub(1)
	cuenta := uuid.New()
	_, cancel := h.Subscribe("acme", cuenta)
	defer cancel()

	hecho := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			h.Dispatch("acme", Llegada{AccountID: cuenta})
		}
		close(hecho)
	}()
	select {
	case <-hecho:
	default:
		// Se comprueba abajo; aquí solo se evita fallar por carrera de arranque.
	}
	<-hecho // si bloqueara, el test moriría por timeout del paquete
}

// Darse de baja dos veces no puede cerrar el canal dos veces: eso es un panic.
func TestCancelarEsIdempotente(t *testing.T) {
	t.Parallel()
	h := NewHub(2)
	_, cancel := h.Subscribe("acme", uuid.New())
	cancel()
	cancel()
	if n := h.Suscritos(); n != 0 {
		t.Errorf("quedan %d suscriptores tras darse de baja", n)
	}
}
