// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"encoding/json"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// La vista «Tareas» y el diario de un día pasado, en claro y en oscuro, a los dos anchos.
// Sin `SYTUE_CAPTURAS` solo comprueban que pintan.
func TestLasCapturasDeTareas(t *testing.T) {
	ayer := time.Now().AddDate(0, 0, -2)
	dia := ayer.Format(time.DateOnly)
	hecha := ayer
	tarea := func(id, titulo string, extra func(*api.Note)) api.Note {
		n := api.Note{ID: id, NotebookID: "d1", Kind: "todo", Title: titulo, UpdatedAt: ayer}
		if extra != nil {
			extra(&n)
		}
		return n
	}
	trasladada := func(n *api.Note) { n.Data = json.RawMessage(`{"carry_count":2}`) }
	notasDia := []api.Note{
		tarea("t1", "Enviar el presupuesto a Marta", trasladada),
		tarea("t2", "Revisar el contrato del local", nil),
		tarea("t3", "Llamar al fontanero", func(n *api.Note) { n.Checked, n.CheckedAt = true, &hecha }),
		tarea("t4", "Reservar mesa para el viernes", func(n *api.Note) { n.CancelledAt = &hecha }),
	}
	pendientes := []api.Tarea{
		{Note: notasDia[0], Notebook: "Día", DailyDate: dia},
		{Note: notasDia[1], Notebook: "Día", DailyDate: dia},
		{Note: api.Note{ID: "t5", NotebookID: "c2", Kind: "todo", Title: "Preparar la demo del jueves"}, Notebook: "Trabajo"},
	}
	for _, variante := range []string{"claro", "oscuro"} {
		for _, c := range []struct {
			nombre      string
			ancho, alto float32
			preparar    func(p *Pantalla)
		}{
			{"escritorio-tareas", 1280, 800, func(p *Pantalla) {
				p.vista, p.tareas = VistaTareas, estadoTareas{fase: Exito, estado: api.TareaAbierta, lista: pendientes}
			}},
			{"movil-tareas", 360, 740, func(p *Pantalla) {
				p.vista, p.tareas = VistaTareas, estadoTareas{fase: Exito, estado: api.TareaAbierta, lista: pendientes}
			}},
			{"escritorio-dia-pasado", 1280, 800, func(p *Pantalla) {
				p.activo = DeCuaderno(api.Notebook{ID: "d1", Title: "Día", Color: "#86198f", Layout: "list", DailyDate: dia})
				p.cuadernos = append(p.cuadernos, p.activo)
				p.notas = DeNotas(notasDia)
			}},
		} {
			t.Run(variante+"-"+c.nombre, func(t *testing.T) {
				tema.Elegir(variante)
				t.Cleanup(func() { tema.Elegir("") })
				p, w := montarCaptura(t, c.ancho, c.alto, "list")
				c.preparar(p)
				p.Repintar()
				if img := w.Canvas().Capture(); img.Bounds().Dx() == 0 {
					t.Fatal("la captura ha salido vacía")
				}
				capturar(t, w, c.nombre+"-"+variante)
			})
		}
	}
}
