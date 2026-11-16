// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLosHilosAgrupanYOrdenan: las raíces por fecha, cada respuesta bajo su raíz, y la
// respuesta huérfana sube a raíz, como en `CommentsPanel.tsx`.
func TestLosHilosAgrupanYOrdenan(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	raices, resp := Hilos([]api.DocComment{
		{ID: "b", CreatedAt: t0.Add(2 * time.Hour)},
		{ID: "r2", ParentID: "a", CreatedAt: t0.Add(3 * time.Hour)},
		{ID: "a", CreatedAt: t0},
		{ID: "r1", ParentID: "a", CreatedAt: t0.Add(time.Hour)},
		{ID: "h", ParentID: "ya-no-esta", CreatedAt: t0.Add(4 * time.Hour)},
	})
	if len(raices) != 3 || raices[0].ID != "a" || raices[1].ID != "b" || raices[2].ID != "h" {
		t.Fatalf("las raíces: %+v", raices)
	}
	if r := resp["a"]; len(r) != 2 || r[0].ID != "r1" || r[1].ID != "r2" {
		t.Errorf("las respuestas: %+v", r)
	}
}

// TestElPanelDeComentariosSinRedNoEnvia: el panel compartido apaga «Comentar» sin texto
// y sin red, aunque haya texto; con red y texto, se enciende.
func TestElPanelDeComentariosSinRedNoEnvia(t *testing.T) {
	test.NewTempApp(t)
	sinRed := true
	c := NuevosComentarios(FuenteComentarios{SinRed: func() bool { return sinRed }})
	if c.Objeto() == nil || !c.Enviar.Inerte {
		t.Fatal("sin texto, «Comentar» tiene que estar apagado")
	}
	c.Nuevo.PonerTexto("Hola")
	c.pintarEnviar()
	if !c.Enviar.Inerte {
		t.Error("sin red, «Comentar» tiene que estar apagado")
	}
	sinRed = false
	c.pintarEnviar()
	if c.Enviar.Inerte {
		t.Error("con red y texto, «Comentar» tiene que encenderse")
	}
}
