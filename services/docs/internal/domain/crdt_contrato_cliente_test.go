// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Contrato de ORDEN entre este CRDT y la secuencia RGA del cliente
// (`apps/frontend/packages/office/src/editor/state/rga.ts`).
//
// Las dos implementaciones tienen que ordenar igual, y esa igualdad no la
// garantiza ningún compilador: son lenguajes distintos y repositorios de código
// distintos dentro del mismo repo. Si divergen, no salta ningún error — cada
// lado materializa un texto diferente y dos personas editando el mismo
// documento ven cosas distintas, que es la manera más cara de romperse.
//
// Por eso los casos de aquí están DUPLICADOS a propósito en `rga.test.ts` con
// los mismos actores, contadores y resultado esperado. Tocar `rgaOrder()` o
// `Dot.Less` sin tocar el cliente rompe este test; tocar el cliente sin tocar
// esto rompe el suyo. Es barato y es la única red que hay.
package domain

import "testing"

// Tres inserciones concurrentes en la misma posición: todas cuelgan de la raíz,
// así que son hermanas y las desempata el orden de puntos. Descendente ⇒ el
// contador más alto va primero. Gemelo de «los hermanos van de mayor a menor
// punto, como en el servidor».
func TestOrdenHermanosCoincideConElCliente(t *testing.T) {
	s := NewState()
	for _, op := range []Op{
		{Kind: OpInsert, Dot: Dot{Actor: "ana", Counter: 1}, Value: []byte(`"A"`), Lamport: 1},
		{Kind: OpInsert, Dot: Dot{Actor: "beto", Counter: 2}, Value: []byte(`"B"`), Lamport: 2},
		{Kind: OpInsert, Dot: Dot{Actor: "ana", Counter: 3}, Value: []byte(`"C"`), Lamport: 3},
	} {
		if _, err := s.Apply(op); err != nil {
			t.Fatalf("Apply %v: %v", op.Dot, err)
		}
	}
	if got := s.PlainText(); got != "CBA" {
		t.Errorf("PlainText = %q, quería «CBA» — es lo que produce rga.ts con estos mismos puntos", got)
	}
}

// Empate de contador: desempata el actor, y el mayor va primero por el orden
// descendente. Gemelo de «empate de contador: gana el actor mayor».
func TestOrdenEmpateDeContadorCoincideConElCliente(t *testing.T) {
	s := NewState()
	for _, op := range []Op{
		{Kind: OpInsert, Dot: Dot{Actor: "ana", Counter: 1}, Value: []byte(`"A"`), Lamport: 1},
		{Kind: OpInsert, Dot: Dot{Actor: "zoe", Counter: 1}, Value: []byte(`"Z"`), Lamport: 1},
	} {
		if _, err := s.Apply(op); err != nil {
			t.Fatalf("Apply %v: %v", op.Dot, err)
		}
	}
	if got := s.PlainText(); got != "ZA" {
		t.Errorf("PlainText = %q, quería «ZA» — es lo que produce rga.ts con estos mismos puntos", got)
	}
}

// Una cadena encadenada normal (escribir de izquierda a derecha) sale en orden.
// Gemelo de «escribir letra a letra da el texto escrito».
func TestOrdenCadenaEncadenadaCoincideConElCliente(t *testing.T) {
	s := NewState()
	letras := []string{"h", "o", "l", "a"}
	var anterior Dot
	for i, l := range letras {
		op := Op{
			Kind:    OpInsert,
			Dot:     Dot{Actor: "a", Counter: uint64(i + 1)},
			Value:   []byte(`"` + l + `"`),
			Lamport: uint64(i + 1),
		}
		if i > 0 {
			p := anterior
			op.After = &p
		}
		if _, err := s.Apply(op); err != nil {
			t.Fatalf("Apply %q: %v", l, err)
		}
		anterior = op.Dot
	}
	if got := s.PlainText(); got != "hola" {
		t.Errorf("PlainText = %q, quería «hola»", got)
	}
}
