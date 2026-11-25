// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// notaNueva lleva lo que el editor nativo ganó el 2026-11-25: Título 6, subíndice y
// superíndice, un emoji, columnas (una con un panel), código numerado con una línea
// larga y ajuste, una tabla numerada con celdas combinadas, un desplegable cerrado, y
// una imagen y un fichero del texto (adjuntos de la nota).
const notaNueva = `<h6>Notas de la obra</h6>` +
	`<p>Fórmula del agua: H<sub>2</sub>O, superficie 12 m<sup>2</sup> ✅ 🚀</p>` +
	`<div data-columnas="2"><div data-columna=""><p>Cocina: azulejos y encimera.</p></div>` +
	`<div data-columna=""><div data-panel="aviso"><p>El fontanero viene el martes.</p></div></div></div>` +
	`<pre data-lineas="" data-ajuste=""><code data-lenguaje="go">total := cocina + salon` + "\n" +
	`resumen := fmt.Sprintf("%d cajas en total, contando las de la cocina, el salón y el trastero", total)</code></pre>` +
	`<table data-numerada=""><thead><tr><th>Cuarto</th><th>Cajas</th><th>Notas</th></tr></thead>` +
	`<tbody><tr><td rowspan="2">Cocina</td><td>12</td><td>frágil</td></tr><tr><td>3</td><td>ollas</td></tr>` +
	`<tr><td colspan="2" data-fondo="azul">Salón y pasillo</td><td>7</td></tr></tbody></table>` +
	`<details data-expandir=""><summary>Inventario completo</summary><p>Doce cajas de cocina.</p></details>` +
	`<p>Plano: <img src="adjunto:1732540000000-ab12cd" alt="plano.png"/> y <a href="adjunto:1732540000000-ef34gh" target="_blank" rel="noopener noreferrer">presupuesto.pdf</a></p>`

// adjuntosDeMuestra son los de notaNueva: un plano (un PNG de verdad) y un PDF.
func adjuntosDeMuestra(t *testing.T) json.RawMessage {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 160, 90))
	for y := range 90 {
		for x := range 160 {
			c := color.NRGBA{R: 0xe8, G: 0xf0, B: 0xfa, A: 0xff}
			if x%40 < 2 || y%30 < 2 {
				c = color.NRGBA{R: 0x1d, G: 0x4e, B: 0xd8, A: 0xff}
			}
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	datos, err := json.Marshal(map[string][]Adjunto{"attachments": {
		{ID: "1732540000000-ab12cd", Name: "plano.png", Size: int64(b.Len()), Type: "image/png", DataURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())},
		{ID: "1732540000000-ef34gh", Name: "presupuesto.pdf", Size: 4, Type: "application/pdf", DataURL: "data:application/pdf;base64,JVBERg=="},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return datos
}

// TestLasCapturasDeLosBloquesNuevos pinta notaNueva al leerla y al escribirla, en el
// teléfono y en el escritorio, en claro y en oscuro; en el teléfono, la barra pegada al
// teclado, y en el escritorio, la barra de tabla en una celda combinada. Sin `SYTUE_CAPTURAS` solo comprueba que pinta y que la nota se puede
// escribir (es fiel).
func TestLasCapturasDeLosBloquesNuevos(t *testing.T) {
	if !Leer(notaNueva).Fiel() {
		t.Fatalf("la nota de muestra no es fiel: %s", Leer(notaNueva).HTML())
	}
	datos := adjuntosDeMuestra(t)
	for _, v := range []tema.Variante{tema.Claro, tema.Oscuro} {
		for _, tam := range []fyne.Size{{Width: 390, Height: 844}, {Width: 1280, Height: 800}} {
			for _, modo := range []string{"lectura", "editor", "teclado", "tabla"} {
				movil := tam.Width < ui.CorteSM
				if modo == "teclado" && !movil || modo == "tabla" && movil {
					continue
				}
				nombre := fmt.Sprintf("nuevos-%s-%dx%d-%s", modo, int(tam.Width), int(tam.Height), map[tema.Variante]string{tema.Claro: "claro", tema.Oscuro: "oscuro"}[v])
				t.Run(nombre, func(t *testing.T) {
					conTema(t, v)
					t.Cleanup(ui.TactilDePrueba(movil))
					t.Cleanup(ui.MovilDePrueba(movil))
					p, w := montarCaptura(t, tam.Width, tam.Height, "tree")
					c := p.notas[0].Crudo
					c.Content, c.Data = notaNueva, datos
					p.notas[0] = DeNota(c)
					p.lienzo.Seleccionada = "n1"
					if modo != "lectura" {
						n, _ := p.nota("n1")
						p.abrirEditor(n)
					}
					p.Repintar()
					if modo == "tabla" {
						// El cursor en la celda combinada: la barra de tabla, con las filas
						// numeradas pulsadas y «Separar celdas» encendido.
						ed := p.lienzo.Editor
						for i, b := range ed.doc.Bloques {
							if b.Celda.EnTabla() && !b.Celda.Tabla.EsColumnas() && b.Texto() == "Cocina" {
								ed.cur, ed.ancla = Pos{i, 0}, Pos{i, 0}
							}
						}
						ed.repintar()
						if !ed.enTabla() || ed.botones["separarCelda"].Inerte {
							t.Fatal("sin la barra de tabla en la celda combinada")
						}
					}
					if modo == "teclado" {
						ed := p.lienzo.Editor
						ed.zona.enfocado = true
						ed.ponerTeclado(300)
						if !ed.enTeclado() {
							t.Fatal("la barra no va pegada al teclado")
						}
					}
					if img := w.Canvas().Capture(); img.Bounds().Dx() == 0 {
						t.Fatal("la captura ha salido vacía")
					}
					capturar(t, w, nombre)
				})
			}
		}
	}
}
