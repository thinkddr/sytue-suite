// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
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

// TestLosComentariosSeRepartenAlAnchoDelPanel: en el teléfono el panel ocupa la pantalla
// entera, y los comentarios se repartían al ancho del panel de escritorio, con más de
// cien píxeles libres a la derecha de cada línea (auditoría del 2026-11-24). Colocado a
// otro ancho, el cuerpo de cada hilo se reparte al suyo: el panel menos su `p-3` y el
// `border p-2` del hilo. VISTO EN ROJO con el ancho fijo de antes (245 a 390).
func TestLosComentariosSeRepartenAlAnchoDelPanel(t *testing.T) {
	test.NewTempApp(t)
	c := NuevosComentarios(FuenteComentarios{})
	c.lista = []api.DocComment{{ID: "a", AuthorName: "Ana", Body: "Creo que este párrafo debería ir antes de la introducción.", CreatedAt: time.Now()}}
	c.pintar()
	// En escritorio va en la envoltura del carril, de `AnchoPanel - Borde`.
	escritorio := AnchoPanel - tema.Borde
	for _, ancho := range []float32{escritorio, 390, escritorio} {
		c.Objeto().Resize(fyne.NewSize(ancho, 600))
		quiero := ancho - 2*MargenPanel - 2*tema.Borde - 2*tema.Espacio(2)
		var cuerpos []*ui.Parrafo
		var andar func(o fyne.CanvasObject)
		andar = func(o fyne.CanvasObject) {
			switch x := o.(type) {
			case *ui.Parrafo:
				if strings.HasPrefix(x.Texto, "Creo") {
					cuerpos = append(cuerpos, x)
				}
			case *fyne.Container:
				for _, h := range x.Objects {
					andar(h)
				}
			}
		}
		andar(c.hilos)
		if len(cuerpos) != 1 {
			t.Fatalf("a %v hay %d cuerpos de comentario", ancho, len(cuerpos))
		}
		if got := cuerpos[0].Ancho; got != quiero {
			t.Errorf("con el panel a %v el comentario se reparte a %v y le caben %v", ancho, got, quiero)
		}
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
