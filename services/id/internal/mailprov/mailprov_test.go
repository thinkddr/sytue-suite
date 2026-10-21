// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mailprov

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// Un 4xx no se arregla repitiendo la misma petición: hay que espaciarlo. Un 5xx o
// un fallo de red sí, porque lo que falla es el otro extremo. Confundirlos fue lo
// que dejó a producción con 4 peticiones por minuto en bucle indefinido.
func TestPosponerSoloLosPermanentes(t *testing.T) {
	t.Parallel()
	p := &Provisioner{espera: map[string]reintento{}}
	ahora := time.Now()

	transitorios := []error{
		&errHTTP{metodo: http.MethodPost, ruta: "/api/v1/aliases", status: http.StatusBadGateway},
		errors.New("mailprov: POST /api/v1/aliases: connection refused"),
	}
	for _, err := range transitorios {
		if hasta := p.posponer("fila", err, ahora); !hasta.IsZero() {
			t.Fatalf("%v se pospuso hasta %v; los transitorios se reintentan ya", err, hasta)
		}
		if len(p.espera) != 0 {
			t.Fatalf("%v dejó rastro en el mapa de espera", err)
		}
	}

	// El tramo dobla a cada fallo y se detiene en el techo.
	err := &errHTTP{metodo: http.MethodPost, ruta: "/api/v1/aliases", status: http.StatusBadRequest}
	previo := time.Duration(0)
	for i := range 12 {
		hasta := p.posponer("fila", err, ahora)
		tramo := hasta.Sub(ahora)
		switch {
		case i == 0 && tramo != esperaInicial:
			t.Fatalf("primer tramo = %v, quiero %v", tramo, esperaInicial)
		case i > 0 && tramo != min(2*previo, esperaMaxima):
			t.Fatalf("tramo %d = %v, quiero %v", i, tramo, min(2*previo, esperaMaxima))
		case tramo > esperaMaxima:
			t.Fatalf("tramo %d = %v, pasa del techo %v", i, tramo, esperaMaxima)
		}
		previo = tramo
	}
}

// El mensaje del error tipado es el que ya salía en los registros: cambiarlo
// rompería las búsquedas guardadas de quien opera.
func TestErrHTTPMensaje(t *testing.T) {
	t.Parallel()
	err := &errHTTP{metodo: http.MethodPost, ruta: "/api/v1/aliases", status: http.StatusBadRequest}
	quiero := fmt.Sprintf("mailprov: POST /api/v1/aliases respondió %d", http.StatusBadRequest)
	if err.Error() != quiero {
		t.Fatalf("Error() = %q, quiero %q", err, quiero)
	}
	if !permanente(fmt.Errorf("al crear el buzón: %w", err)) {
		t.Fatal("permanente() no ve a través de un %w")
	}
}
