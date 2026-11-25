// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// notaDeMuestra lleva todo lo que el editor sabe pintar: los tres títulos, párrafos con
// marcas y código en línea, listas anidadas, tareas, cita, bloque de código, tabla y
// regla. Es la que enseñan las capturas del editor.
const notaDeMuestra = `<h1>Mudanza a Lisboa</h1>` +
	`<p>Lo que queda por cerrar antes del <b>jueves</b>, con el presupuesto de <i>tres</i> empresas y el código <code>MUD-26</code>.</p>` +
	`<h2>Tareas</h2>` +
	`<div class="task-row"><input type="checkbox" checked=""> <span>Pedir presupuestos</span></div>` +
	`<div class="task-row"><input type="checkbox"> <span>Reservar el camión</span></div>` +
	`<h3>Cajas</h3>` +
	`<ul><li>Cocina<ul><li>Vajilla</li><li>Sartenes</li></ul></li><li>Salón</li></ul>` +
	`<ol><li>Embalar</li><li>Etiquetar</li></ol>` +
	`<blockquote>Lo frágil, en cajas pequeñas.</blockquote>` +
	`<pre>cajas = 12 + 7</pre>` +
	`<table><tbody><tr><th>Cuarto</th><th>Cajas</th></tr><tr><td>Cocina</td><td>12</td></tr></tbody></table>` +
	`<hr><p>Fin.</p>`

// notaRica lleva los bloques ricos del formato compartido (bloques_ricos.go).
const notaRica = `<h2>Plan de la mudanza</h2>` +
	`<div data-panel="info"><p>Todo lo frágil va en cajas pequeñas.</p></div>` +
	`<p>Estado: <span data-estado="verde">EN MARCHA</span> · revisión el <time datetime="2026-11-28">28 nov 2026</time></p>` +
	`<ul data-tipo="decisiones"><li>Contratamos la empresa del presupuesto B</li></ul>` +
	`<div data-panel="aviso"><p>El ascensor no funciona los lunes.</p></div>` +
	`<details data-expandir=""><summary>Inventario</summary><p>Doce cajas de cocina y siete del salón.</p></details>` +
	`<pre><code data-lenguaje="go">total := 12 + 7</code></pre>` +
	`<table><thead><tr><th>Cuarto</th><th>Cajas</th></tr></thead><tbody><tr><th scope="row">Cocina</th><td data-fondo="amarillo">12</td></tr>` +
	`<tr><th scope="row">Salón</th><td>7</td></tr></tbody></table>` +
	`<div class="task-row"><input type="checkbox" checked=""/> <span>Pedir presupuestos</span></div>` +
	`<div class="task-row"><input type="checkbox"/> <span>Reservar el camión</span></div>`

// TestLasCapturasDelEditor pinta el editor de una nota abierta en el árbol, en el
// teléfono (390 × 844, táctil) y en el escritorio (1280 × 800), en claro y en oscuro.
// Sin `SYTUE_CAPTURAS` solo comprueba que pinta.
func TestLasCapturasDelEditor(t *testing.T) {
	for _, v := range []tema.Variante{tema.Claro, tema.Oscuro} {
		for _, tam := range []fyne.Size{{Width: 390, Height: 844}, {Width: 1280, Height: 800}} {
			for _, nota := range []struct{ nombre, html string }{{"editor", notaDeMuestra}, {"editor-ricos", notaRica}} {
				nombre := fmt.Sprintf("%s-%dx%d-%s", nota.nombre, int(tam.Width), int(tam.Height), map[tema.Variante]string{tema.Claro: "claro", tema.Oscuro: "oscuro"}[v])
				t.Run(nombre, func(t *testing.T) {
					conTema(t, v)
					movil := tam.Width < ui.CorteSM
					t.Cleanup(ui.TactilDePrueba(movil))
					t.Cleanup(ui.MovilDePrueba(movil))
					p, w := montarCaptura(t, tam.Width, tam.Height, "tree")
					c := p.notas[0].Crudo
					c.Content = nota.html
					p.notas[0] = DeNota(c)
					p.lienzo.Seleccionada = "n1"
					n, _ := p.nota("n1")
					p.abrirEditor(n)
					p.Repintar()
					if img := w.Canvas().Capture(); img.Bounds().Dx() == 0 {
						t.Fatal("la captura ha salido vacía")
					}
					capturar(t, w, nombre)
				})
			}
		}
	}
}
