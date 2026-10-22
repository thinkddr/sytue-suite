// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package coedicion

import (
	"encoding/json"
	"strings"
	"testing"
)

// Los casos de `rga.test.ts`, con los mismos valores. Lo que importa no es que insertar
// inserte sino la CONVERGENCIA: dos réplicas con las mismas ops en distinto orden acaban
// con el mismo texto. Los del orden de hermanos son también los de
// `crdt_contrato_cliente_test.go`: una divergencia con el servidor rompe uno u otro.

// barajar es la baraja determinista de la web.
func barajar(xs []Op, semilla int) []Op {
	out := append([]Op(nil), xs...)
	s := semilla
	for i := len(out) - 1; i > 0; i-- {
		s = (s*1103515245 + 12345) & 0x7fffffff
		j := s % (i + 1)
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func aplicar(s *Secuencia, ops []Op) {
	for _, op := range ops {
		s.AplicarRemota(op)
	}
}

func TestEscribirBorrarYEmoji(t *testing.T) {
	s := NuevaSecuencia("a")
	s.InsertarLocal(0, "hola", 1)
	s.InsertarLocal(4, " mundo", 10)
	s.InsertarLocal(0, "¡", 20)
	s.InsertarLocal(5, ",", 30)
	if s.Texto() != "¡hola, mundo" {
		t.Errorf("insertar: %q", s.Texto())
	}
	s.BorrarLocal(5, 7, 40)
	s.InsertarLocal(5, "!", 50)
	if s.Texto() != "¡hola!" {
		t.Errorf("borrar y seguir: %q", s.Texto())
	}
	if ops := NuevaSecuencia("a").InsertarLocal(0, "a👍b", 1); len(ops) != 3 {
		t.Errorf("tres caracteres, no cuatro ni seis: %d", len(ops))
	}
}

// TestConvergen: dos en el mismo punto no se pierden nada; el mismo lote en cualquier
// orden da el mismo texto; un borrado adelantado no resucita; el eco no duplica. Se vio en
// ROJO con los hermanos en orden ascendente: salía «ACB» y no «CBA».
func TestConvergen(t *testing.T) {
	base := NuevaSecuencia("base")
	opsBase := base.InsertarLocal(0, "hola ", 1)
	ana, beto := NuevaSecuencia("ana"), NuevaSecuencia("beto")
	aplicar(ana, opsBase)
	aplicar(beto, opsBase)
	opsAna, opsBeto := ana.InsertarLocal(5, "Ana", 10), beto.InsertarLocal(5, "Beto", 10)
	aplicar(ana, opsBeto)
	aplicar(beto, opsAna)
	if ana.Texto() != beto.Texto() || !strings.Contains(ana.Texto(), "Ana") || !strings.Contains(ana.Texto(), "Beto") {
		t.Errorf("no convergen: %q y %q", ana.Texto(), beto.Texto())
	}

	emisor := NuevaSecuencia("emisor")
	ops := append(emisor.InsertarLocal(0, "documento", 1), emisor.InsertarLocal(0, "Un ", 20)...)
	ops = append(ops, emisor.InsertarLocal(12, " compartido", 40)...)
	for _, semilla := range []int{1, 7, 42, 1234, 99999} {
		r := NuevaSecuencia("receptor")
		aplicar(r, barajar(ops, semilla))
		if r.Texto() != "Un documento compartido" {
			t.Errorf("semilla %d: %q", semilla, r.Texto())
		}
	}

	e := NuevaSecuencia("emisor")
	ins := e.InsertarLocal(0, "XY", 1)
	del := e.BorrarLocal(0, 1, 10)
	r := NuevaSecuencia("receptor")
	aplicar(r, del)
	aplicar(r, ins)
	aplicar(r, ins) // el eco
	if r.Texto() != "Y" {
		t.Errorf("tumba anticipada e idempotencia: %q", r.Texto())
	}

	l := NuevaSecuencia("lector")
	aplicar(l, []Op{
		{Kind: "insert", Dot: Punto{"ana", 1}, Value: "A", Lamport: 1},
		{Kind: "insert", Dot: Punto{"beto", 2}, Value: "B", Lamport: 2},
		{Kind: "insert", Dot: Punto{"ana", 3}, Value: "C", Lamport: 3},
	})
	z := NuevaSecuencia("lector")
	aplicar(z, []Op{{Kind: "insert", Dot: Punto{"ana", 1}, Value: "A"}, {Kind: "insert", Dot: Punto{"zoe", 1}, Value: "Z"}})
	if l.Texto() != "CBA" || z.Texto() != "ZA" {
		t.Errorf("el orden de hermanos del servidor: %q y %q", l.Texto(), z.Texto())
	}
}

func TestOpsDesdeTexto(t *testing.T) {
	s := NuevaSecuencia("a")
	s.InsertarLocal(0, "hola", 1)
	if ops := s.OpsDesdeTexto("hola mundo", 10); len(ops) != 6 || ops[0].Kind != "insert" {
		t.Errorf("escribir al final: %d ops", len(ops))
	}
	if ops := s.OpsDesdeTexto("hola", 30); len(ops) != 6 || ops[0].Kind != "delete" || s.Texto() != "hola" {
		t.Errorf("borrar: %d ops, %q", len(ops), s.Texto())
	}
	if len(s.OpsDesdeTexto("hola", 40)) != 0 {
		t.Error("sin cambios no hay ops")
	}
	local, remoto := NuevaSecuencia("local"), NuevaSecuencia("remoto")
	aplicar(remoto, local.InsertarLocal(0, "hola mundo", 1))
	aplicar(remoto, local.OpsDesdeTexto("hola planeta corregido", 50))
	if remoto.Texto() != "hola planeta corregido" || local.Texto() != remoto.Texto() {
		t.Errorf("otra réplica llega al mismo sitio: %q", remoto.Texto())
	}
	local.OpsDesdeTexto("", 90)
	if local.Texto() != "" {
		t.Errorf("vaciar: %q", local.Texto())
	}
}

func TestRehidratarYCiclos(t *testing.T) {
	var crudos []json.RawMessage
	if err := json.Unmarshal([]byte(`[{"dot":{"a":"ana","c":1},"after":{"a":"","c":0},"value":"h"},
		{"dot":{"a":"ana","c":2},"after":{"a":"ana","c":1},"value":"o"},
		{"dot":{"a":"ana","c":3},"after":{"a":"ana","c":2},"value":"X","deleted":true},
		{"dot":{"a":"ana","c":4},"after":{"a":"ana","c":3},"value":"la"},
		{"dot":{"a":"ana","c":5},"value":{"shape":"rect"}}, {"dot":1}]`), &crudos); err != nil {
		t.Fatal(err)
	}
	c := Nueva("yo", ElementosDeEstado(crudos), "hola")
	if !c.Sincronizada() {
		t.Error("la secuencia rehidratada dice «hola»: está en sincronía")
	}
	cic := NuevaSecuencia("lector")
	aplicar(cic, []Op{
		{Kind: "insert", Dot: Punto{"x", 1}, After: &Punto{"x", 2}, Value: "A"},
		{Kind: "insert", Dot: Punto{"x", 2}, After: &Punto{"x", 1}, Value: "B"},
	})
	if len(cic.Texto()) > 2 {
		t.Error("un ciclo no se recorre dos veces")
	}
}

// TestLaPoliticaEsLaDeLaWeb: un documento de antes no se siembra con alguien presente;
// lo ajeno espera a Fusionar y el eco propio no cuenta; anular vuelve al LWW.
func TestLaPoliticaEsLaDeLaWeb(t *testing.T) {
	c := Nueva("yo", nil, "heredado")
	if c.Sincronizada() || len(c.OpsLocales("heredado!", 1, false)) != 0 || c.Sincronizada() {
		t.Fatal("con alguien presente, un documento sin secuencia no se siembra")
	}
	if ops := c.OpsLocales("heredado!", 1, true); len(ops) != 9 || !c.Sincronizada() {
		t.Fatalf("a solas se siembra: %d ops", len(ops))
	}
	otro := NuevaSecuencia("otro")
	for _, e := range c.seq.orden() {
		otro.AplicarRemota(Op{Kind: "insert", Dot: e.dot, After: &e.after, Value: e.ch})
	}
	suyas := otro.OpsDesdeTexto("¡heredado!", 200)
	c.Recibir(append(suyas, Op{Kind: "insert", Dot: Punto{"yo", 999}, Value: "Z"}))
	if txt, ok := c.Fusionar(); !ok || txt != "¡heredado!" {
		t.Errorf("fusión: %q %v", txt, ok)
	}
	if _, ok := c.Fusionar(); ok {
		t.Error("sin nada nuevo, no hay fusión")
	}
	c.Anular()
	if c.Sincronizada() {
		t.Error("anulada, manda el registro")
	}
	if op, ok := OpDeSecuencia(json.RawMessage(`{"kind":"set","dot":{"a":"x","c":1},"key":"doc:html","value":"a","lamport":1}`)); ok {
		t.Errorf("un set no es de la secuencia: %+v", op)
	}
}

// TestEscribirEnMedioDeLoAjenoQuedaEnSuSitio: quien abre un documento escrito por otro y
// teclea en medio tiene que ver su letra ahí, y el otro también. Los hermanos van de mayor
// a menor punto, así que el contador de una sesión nueva tiene que partir del mayor que
// haya en la secuencia (la regla de Lamport de la RGA). Se vio en ROJO fijando solo el
// contador propio: la «Z» salía al final del documento. `rga.test.ts` lleva el mismo caso.
func TestEscribirEnMedioDeLoAjenoQuedaEnSuSitio(t *testing.T) {
	ana := NuevaSecuencia("ana")
	ops := ana.InsertarLocal(0, "Hola mundo", 1)
	elems := make([]Elemento, 0, len(ops))
	for _, op := range ops {
		elems = append(elems, Elemento{Dot: op.Dot, After: *op.After, Value: op.Value})
	}
	beto := DesdeElementos("beto", elems)
	cambio := beto.OpsDesdeTexto("HZola mundo", 100)
	if beto.Texto() != "HZola mundo" {
		t.Fatalf("en la réplica de quien escribe: %q", beto.Texto())
	}
	aplicar(ana, cambio)
	if ana.Texto() != "HZola mundo" {
		t.Errorf("en la de la autora: %q", ana.Texto())
	}
}

// TestUnaOpAjenaAdelantaElContador: la misma regla por AplicarRemota. Es el caso gemelo de
// `rga.test.ts`: quien escribe detrás de una letra ajena queda detrás, no al final.
func TestUnaOpAjenaAdelantaElContador(t *testing.T) {
	beto := NuevaSecuencia("beto")
	aplicar(beto, []Op{
		{Kind: "insert", Dot: Punto{"ana", 1}, Value: "a", Lamport: 1},
		{Kind: "insert", Dot: Punto{"ana", 2}, After: &Punto{"ana", 1}, Value: "c", Lamport: 2},
	})
	beto.OpsDesdeTexto("abc", 10)
	if beto.Texto() != "abc" {
		t.Errorf("%q", beto.Texto())
	}
}

// TestUnInsertAjenoEsUnCaracter (FYE-05): un coautor que manda un elemento de varios
// caracteres no entra en la secuencia —antes se aceptaba, y el editor y la secuencia
// dejaban de decir lo mismo (TestUnElementoDeVariosCaracteresDiverge)—, y todo lo que
// manda un cliente legítimo, emoji compuestos incluidos, sí entra.
func TestUnInsertAjenoEsUnCaracter(t *testing.T) {
	if _, ok := OpDeSecuencia(json.RawMessage(`{"kind":"insert","dot":{"a":"eva","c":1},"value":"abc"}`)); ok {
		t.Fatal("se aceptó un insert de tres caracteres")
	}
	ops := NuevaSecuencia("ana").InsertarLocal(0, "👍🏽 é é 🇪🇸", 1)
	for _, op := range ops {
		crudo, err := json.Marshal(op)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := OpDeSecuencia(crudo); !ok {
			t.Fatalf("un insert legítimo se rechaza: %s", crudo)
		}
	}
}

// TestUnElementoDeVariosCaracteresDiverge es por qué hace falta la regla: con un
// elemento «abc» dentro, escribir una X tras la «a» deja el editor en «aXbc» y la
// secuencia en otro texto. Con elementos de uno, coinciden.
func TestUnElementoDeVariosCaracteresDiverge(t *testing.T) {
	mal := DesdeElementos("ana", []Elemento{{Dot: Punto{A: "eva", C: 1}, Value: "abc"}})
	mal.OpsDesdeTexto("aXbc", 1)
	if mal.Texto() == "aXbc" {
		t.Fatal("con un elemento de tres caracteres la secuencia ya no diverge: revisa la regla")
	}
	bien := NuevaSecuencia("eva")
	bien.InsertarLocal(0, "abc", 1)
	bien.OpsDesdeTexto("aXbc", 5)
	if bien.Texto() != "aXbc" {
		t.Fatalf("con elementos de uno: %q", bien.Texto())
	}
}
