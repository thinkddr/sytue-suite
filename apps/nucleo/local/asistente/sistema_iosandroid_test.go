// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package asistente

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// El puente del móvil con un sistema de mentira: lo que hará Ajustes y Asistir con
// Foundation Models y Gemini Nano, sin teléfono.

func TestSinModeloEsErrSinIANativaConElMotivo(t *testing.T) {
	for _, e := range []EstadoSistema{SistemaNoSoportada, SistemaApagada, SistemaDescargando, SistemaSinPuente, SistemaAntiguo, SistemaFallo} {
		p := &puenteNativo{
			nombre: "X", consultar: func() EstadoSistema { return e },
			responder: func(int64, string, string) (string, error) { t.Fatal("no debe preguntar al modelo"); return "", nil },
		}
		_, err := p.completar(context.Background(), "s", "u")
		if !errors.Is(err, ErrSinIANativa) || !strings.Contains(err.Error(), e.motivo()) {
			t.Errorf("estado %d: %v", e, err)
		}
	}
}

func TestDescargableLoPideYNoContesta(t *testing.T) {
	pedido := make(chan struct{}, 1)
	p := &puenteNativo{
		nombre: "X", consultar: func() EstadoSistema { return SistemaDescargable },
		responder: func(int64, string, string) (string, error) { return "no", nil },
		preparar:  func() { pedido <- struct{}{} },
	}
	if _, err := p.completar(context.Background(), "s", "u"); !errors.Is(err, ErrSinIANativa) {
		t.Fatal(err)
	}
	select {
	case <-pedido:
	case <-time.After(2 * time.Second):
		t.Fatal("no se pidió la descarga")
	}
}

func TestContestaYRecortaYSinTextoEsError(t *testing.T) {
	var sys, usu string
	resp := "  listo \n"
	p := &puenteNativo{
		nombre: "X", consultar: func() EstadoSistema { return SistemaLista },
		responder: func(_ int64, s, u string) (string, error) { sys, usu = s, u; return resp, nil },
	}
	got, err := p.completar(context.Background(), "sistema", "usuario")
	if err != nil || got != "listo" || sys != "sistema" || usu != "usuario" {
		t.Fatalf("%q %v (%q %q)", got, err, sys, usu)
	}
	resp = " "
	if _, err := p.completar(context.Background(), "s", "u"); err == nil {
		t.Fatal("una respuesta vacía no es una respuesta")
	}
}

func TestCancelarDejaDeEsperarYAvisaAlPuente(t *testing.T) {
	var cancelado atomic.Int64
	suelta := make(chan struct{})
	defer close(suelta)
	p := &puenteNativo{
		nombre: "X", consultar: func() EstadoSistema { return SistemaLista },
		responder: func(int64, string, string) (string, error) { <-suelta; return "tarde", nil },
		cancelar:  func(id int64) { cancelado.Store(id) },
	}
	ctx, fin := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer fin()
	if _, err := p.completar(ctx, "s", "u"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if cancelado.Load() == 0 {
		t.Fatal("no se canceló en el puente")
	}
}

func TestUnFalloDelModeloVuelveAPreguntar(t *testing.T) {
	var consultas atomic.Int32
	p := &puenteNativo{
		nombre: "X", consultar: func() EstadoSistema { consultas.Add(1); return SistemaLista },
		responder: func(int64, string, string) (string, error) { return "", errors.New("contexto demasiado largo") },
	}
	for range 2 {
		if _, err := p.completar(context.Background(), "s", "u"); err == nil || !strings.Contains(err.Error(), "demasiado largo") {
			t.Fatal(err)
		}
	}
	if consultas.Load() != 2 {
		t.Fatalf("tras un fallo hay que volver a preguntar: %d consultas", consultas.Load())
	}
}

// Ajustes pinta en el hilo de la interfaz: el estado no espera al sistema (mientras
// pregunta, SistemaComprobando) y, cuando ya se sabe, es el que dijo.
func TestElEstadoDeAjustesNoEsperaYLuegoEsElDelSistema(t *testing.T) {
	suelta := make(chan struct{})
	p := &puenteNativo{nombre: "Gemini Nano", consultar: func() EstadoSistema { <-suelta; return SistemaNoSoportada }}
	inicio := time.Now()
	if e := p.estadoSinEsperar(); e != SistemaComprobando || time.Since(inicio) > time.Second {
		t.Fatalf("%v tras %v", e, time.Since(inicio))
	}
	close(suelta)
	limite := time.Now().Add(2 * time.Second)
	for time.Now().Before(limite) {
		if e := p.estadoSinEsperar(); e == SistemaNoSoportada {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("Ajustes no sabe el estado: %v", p.estadoSinEsperar())
}

// Sin modelo, el error lleva el estado: la pantalla lo saca con errors.As para decirlo
// en su idioma, y sigue siendo ErrSinIANativa.
func TestElErrorLlevaElEstado(t *testing.T) {
	p := &puenteNativo{nombre: "X", consultar: func() EstadoSistema { return SistemaApagada }}
	_, err := p.completar(context.Background(), "s", "u")
	var es *ErrorSistema
	if !errors.As(err, &es) || es.Estado != SistemaApagada || !errors.Is(err, ErrSinIANativa) {
		t.Fatalf("%v", err)
	}
}
