// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios

package escritorio

import (
	"image/color"
	"os"
	"runtime"
	"testing"
)

// Una NSWindow solo nace en el hilo principal: el del paquete se queda dándole vueltas a
// su cola y las pruebas corren en otro.
func init() { runtime.LockOSThread() }

func TestMain(m *testing.M) {
	go func() { os.Exit(m.Run()) }()
	bucleprincipal()
}

// TestLaFranjaDelSistemaVaSinTitulo es el pedido del dueño (2026-09-25): en macOS la
// franja de los semáforos encima de la barra de la suite no lleva el título de la
// ventana, en las trece apps, porque todas pasan por ColorearBarraTitulo. Pregunta a
// una NSWindow de verdad. VISTO EN ROJO quitando la línea de `titleVisibility`.
func TestLaFranjaDelSistemaVaSinTitulo(t *testing.T) {
	w := ventanaPrueba()
	if tituloOculto(w) {
		t.Fatal("la ventana nace ya sin título: la prueba no distinguiría nada")
	}
	for _, oscuro := range []bool{false, true} {
		ColorearBarraTitulo(w, color.NRGBA{R: 30, G: 30, B: 30, A: 255}, oscuro)
		if !tituloOculto(w) {
			t.Errorf("oscuro=%v: la franja del sistema sigue enseñando el título", oscuro)
		}
	}
}
