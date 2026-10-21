// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package presupuesto

import (
	"context"
	"testing"
	"time"
)

func TestPrecio_Coste(t *testing.T) {
	// gemma-4-26b-a4b-it: 0,25 € entrada y 0,50 € salida por millón.
	p := Precio{Entrada: 0.25, Salida: 0.50}
	// 1.000.000 de entrada + 1.000.000 de salida = 0,75 €.
	if got := p.Coste(1_000_000, 1_000_000); got != 0.75 {
		t.Errorf("Coste = %v, quería 0.75", got)
	}
	// La peticion medida de verdad: 862 de entrada, 100 de salida.
	if got := p.Coste(862, 100); got < 2.6e-4 || got > 2.7e-4 {
		t.Errorf("Coste = %v, fuera del orden esperado (~0,00026 €)", got)
	}
}

// Sin caché no hay tope, y eso NO es un descuido: se degrada abierto para no
// dejar sin asistente a todo el mundo porque falte una pieza opcional.
func TestContador_SinCacheDejaPasar(t *testing.T) {
	p := Nuevo(nil, 1.50)
	ok, gastado := p.Permite(context.Background(), "t", "u")
	if !ok || gastado != 0 {
		t.Errorf("Permite = (%v, %v), quería (true, 0)", ok, gastado)
	}
	if total := p.Cobrar(context.Background(), "t", "u", 0.01); total != 0 {
		t.Errorf("Cobrar = %v, quería 0", total)
	}
}

// Tope <= 0 significa «sin tope», que es como se apaga la función.
func TestContador_SinTopeDejaPasar(t *testing.T) {
	if ok, _ := Nuevo(nil, 0).Permite(context.Background(), "t", "u"); !ok {
		t.Error("con tope 0 debería dejar pasar")
	}
}

// El mes va DENTRO de la clave: es lo que hace que el contador se reinicie solo
// el día 1 sin ningún proceso que lo ponga a cero.
func TestClave_CambiaConElMes(t *testing.T) {
	ene := time.Date(2026, 1, 31, 23, 59, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 1, 0, 1, 0, 0, time.UTC)
	if clave("t", "u", ene) == clave("t", "u", feb) {
		t.Error("la clave no cambia al cambiar de mes: el contador no se reiniciaría")
	}
	if got := clave("acme", "ana", ene); got != "ai:presupuesto:acme:ana:2026-01" {
		t.Errorf("clave = %q", got)
	}
}

// Dos usuarios del mismo tenant no comparten presupuesto, ni dos tenants el
// mismo usuario: el tope es POR USUARIO.
func TestClave_SeparaUsuariosYTenants(t *testing.T) {
	ahora := time.Now()
	if clave("t", "ana", ahora) == clave("t", "luis", ahora) {
		t.Error("dos usuarios comparten contador")
	}
	if clave("acme", "ana", ahora) == clave("otra", "ana", ahora) {
		t.Error("dos tenants comparten contador")
	}
}
