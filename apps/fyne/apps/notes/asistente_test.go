// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// LOS GUARDIANES DEL ASISTENTE: lo que se pide (lib/ai.ts) y cómo se aplica lo que
// vuelve (`aplicarIA` de RichText.tsx).

// asistenteFalso contesta `respuesta` (o falla) y guarda lo que le han pedido.
func asistenteFalso(respuesta string, fallo error, pedido *api.PeticionIA) func(context.Context, api.PeticionIA) (string, error) {
	return func(_ context.Context, q api.PeticionIA) (string, error) {
		*pedido = q
		return respuesta, fallo
	}
}

// esperarIA deja correr la rutina del asistente hasta que acaba.
func esperarIA(t *testing.T, ed *EditorNota) {
	t.Helper()
	for i := 0; i < 200; i++ {
		fyne.DoAndWait(func() {})
		trabajando := true
		fyne.DoAndWait(func() { trabajando = ed.iaTrabajando })
		if !trabajando {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("el asistente no ha vuelto")
}

// TestLoQueSePideAlAsistente: la directiva de la acción, el tipo `note`, el idioma de la
// sesión y el texto (la nota entera sin selección; la selección si la hay).
func TestLoQueSePideAlAsistente(t *testing.T) {
	q := PeticionIA("tareas", "  llamar a Ana  ")
	if q.Tipo != "note" || q.Accion != "tareas" || q.Contexto != "llamar a Ana" ||
		!strings.HasSuffix(q.Instruccion, "Devuelve solo la lista.") {
		t.Errorf("la petición es %+v", q)
	}
	if n := len([]rune(PeticionIA("resumir", strings.Repeat("á", MaxContextoIA+5)).Contexto)); n != MaxContextoIA {
		t.Errorf("el contexto va con %d letras", n)
	}
	for _, a := range AccionesIA {
		if a.Accion != "" && directivasIA[a.Accion] == "" {
			t.Errorf("«%s» no tiene directiva", a.Accion)
		}
	}

	var pedido api.PeticionIA
	ed := NuevoEditorNota(trInterno(t), Leer("<p>uno</p><p>dos</p>"), "")
	ed.ConAsistente(asistenteFalso("x", nil, &pedido), "es")
	ed.PedirIA(func(s string) api.PeticionIA { return PeticionIA("corregir", s) })
	esperarIA(t, ed)
	if pedido.Contexto != "uno\ndos" || pedido.Idioma != "es" {
		t.Errorf("sin selección se pide %q en %q", pedido.Contexto, pedido.Idioma)
	}
}

// TestComoSeAplicaLoQueDiceElAsistente: con formato y sin selección, en el cursor con el
// formato de la nota; con selección, en su lugar; en llano y sin selección, la nota
// entera; y si falla, la nota no se toca y se dice.
//
// VISTO EN ROJO (2026-09-24, en el frontal): sin el caso del formato, la respuesta pisaba
// la nota como «## Plan<br/>- **uno**…», con las almohadillas y los asteriscos.
func TestComoSeAplicaLoQueDiceElAsistente(t *testing.T) {
	var pedido api.PeticionIA
	pedir := func(s string) api.PeticionIA { return PeticionIA("ordenar", s) }

	ed := NuevoEditorNota(trInterno(t), Leer("<p>notas</p>"), "")
	ed.ConAsistente(asistenteFalso("## Plan\n- **uno**\n- dos", nil, &pedido), "es")
	ed.PedirIA(pedir)
	esperarIA(t, ed)
	if got := ed.HTML(); got != "<p>notas</p><h2>Plan</h2><ul><li><b>uno</b></li><li>dos</li></ul>" {
		t.Errorf("con formato deja %q", got)
	}
	ed.Orden("undo", "")
	if got := ed.HTML(); got != "<p>notas</p>" {
		t.Errorf("deshacer no lo quita de una vez: %q", got)
	}

	sel := NuevoEditorNota(trInterno(t), Leer("<p>hola mundo</p>"), "")
	sel.ConAsistente(asistenteFalso("MUNDO", nil, &pedido), "es")
	sel.ancla, sel.cur = Pos{0, 5}, Pos{0, 10}
	sel.PedirIA(pedir)
	esperarIA(t, sel)
	if got := sel.HTML(); got != "<p>hola MUNDO</p>" || pedido.Contexto != "mundo" {
		t.Errorf("con selección se pide %q y deja %q", pedido.Contexto, got)
	}

	llano := NuevoEditorNota(trInterno(t), Leer("<p>uno</p><p>dos</p>"), "")
	llano.ConAsistente(asistenteFalso("Plan de mudanza", nil, &pedido), "es")
	llano.PedirIA(pedir)
	esperarIA(t, llano)
	if got := llano.HTML(); got != "Plan de mudanza" {
		t.Errorf("en llano deja %q", got)
	}

	falla := NuevoEditorNota(trInterno(t), Leer("<p>notas</p>"), "")
	falla.ConAsistente(asistenteFalso("", errors.New("503"), &pedido), "es")
	var avisos []bool
	falla.AlAsistir = func(_, fallo bool) { avisos = append(avisos, fallo) }
	falla.PedirIA(pedir)
	esperarIA(t, falla)
	if got := falla.HTML(); got != "<p>notas</p>" {
		t.Errorf("un fallo ha tocado la nota: %q", got)
	}
	if len(falla.barras()) != 2 || len(avisos) != 2 || !avisos[1] {
		t.Errorf("el fallo no se dice: %d barras, avisos %v", len(falla.barras()), avisos)
	}
}
