// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestTextoParaCopiar: el título, una línea en blanco y el cuerpo en texto plano (las
// viñetas con su «•»), como `textoParaCopiar` de la web; sin uno de los dos, el otro.
func TestTextoParaCopiar(t *testing.T) {
	n := Nota{Titulo: " Compra ", Contenido: "<ul><li>pan</li><li>leche</li></ul>"}
	if got, quiero := TextoParaCopiar(n), "Compra\n\n• pan\n• leche"; got != quiero {
		t.Errorf("TextoParaCopiar = %q, quiero %q", got, quiero)
	}
	if got := TextoParaCopiar(Nota{Contenido: "<p>solo cuerpo</p>"}); got != "solo cuerpo" {
		t.Errorf("sin título: %q", got)
	}
	if got := TextoParaCopiar(Nota{Titulo: "Solo título"}); got != "Solo título" {
		t.Errorf("sin cuerpo: %q", got)
	}
}

// TestEnlaceANota: el mismo `?notebook=&note=` que `urlDeNota` de la web, escapado; sin
// dirección de la web que componer, no hay enlace (y el menú no lo ofrece).
func TestEnlaceANota(t *testing.T) {
	p := &Pantalla{}
	if got := p.enlaceANota("c1", "n1"); got != "" {
		t.Errorf("sin AppURL: %q", got)
	}
	p.AppURL = func(ruta string) string { return "https://notes.sytue.test/acme" + ruta }
	if got, quiero := p.enlaceANota("c 1", "n&1"), "https://notes.sytue.test/acme/?notebook=c+1&note=n%261"; got != quiero {
		t.Errorf("enlaceANota = %q, quiero %q", got, quiero)
	}
}

// TestDiaRelativo: «Ayer» y «Mañana» respecto de hoy, también cruzando un mes, como
// `diaRelativo` de la web.
func TestDiaRelativo(t *testing.T) {
	hoy := time.Date(2026, 12, 1, 10, 0, 0, 0, time.Local)
	casos := map[string]string{
		"2026-11-30": "daily.yesterday",
		"2026-12-02": "daily.tomorrow",
		"2026-12-01": "",
		"2026-12-03": "",
	}
	for dia, quiero := range casos {
		if got := DiaRelativo(dia, hoy); got != quiero {
			t.Errorf("DiaRelativo(%s) = %q, quiero %q", dia, got, quiero)
		}
	}
}

// TestLaInformacionDiceElEstadoDeLaTareaYLosEnlaces: una tarea dice si está pendiente,
// hecha o cancelada, y toda nota cuántos enlaces salen de ella, como NoteInfo.tsx.
func TestLaInformacionDiceElEstadoDeLaTareaYLosEnlaces(t *testing.T) {
	p, _ := pantallaConServicio(t, map[string]string{})
	fila := func(filas [][2]string, rotulo string) string {
		for _, f := range filas {
			if f[0] == rotulo {
				return f[1]
			}
		}
		return "(no está)"
	}
	tr := p.tr.T
	tarea := Nota{Tipo: "todo", Marcada: true, Enlaces: []api.NoteLink{{TargetKind: "note", TargetID: "x"}, {TargetKind: "drive", TargetID: "y"}}}
	filas := p.filasInformacion(tarea, "—")
	if got := fila(filas, tr("noteInfo.task", nil)); got != tr("noteInfo.task.done", nil) {
		t.Errorf("tarea hecha: %q", got)
	}
	if got := fila(filas, tr("noteInfo.links", nil)); got != "2" {
		t.Errorf("enlaces: %q, quiero 2", got)
	}
	tarea.Cancelada = true
	if got := fila(p.filasInformacion(tarea, "—"), tr("noteInfo.task", nil)); got != tr("noteInfo.task.cancelled", nil) {
		t.Errorf("tarea cancelada: %q", got)
	}
	if got := fila(p.filasInformacion(Nota{Tipo: "text"}, "—"), tr("noteInfo.task", nil)); got != "(no está)" {
		t.Errorf("una nota de texto no es una tarea: %q", got)
	}
}
