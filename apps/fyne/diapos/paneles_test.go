// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"fmt"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

const conGraficoYDiagrama = `{"slides":[{"id":"s","boxes":[` +
	`{"id":"g","x":10,"y":10,"w":40,"h":40,"html":"","kind":"chart","chart":{"type":"bar","grid":[["","Serie 1"],["Ene","10"],["Feb","15"]]}},` +
	`{"id":"d","x":55,"y":10,"w":40,"h":40,"html":"","kind":"smartart","smartart":{"layout":"hierarchy","items":[{"text":"Raíz"},{"text":"A"},{"text":"B"}]}},` +
	`{"id":"t","x":10,"y":60,"w":40,"h":20,"html":"<p>Texto</p>"}]}]}`

// buscar recorre el árbol de un objeto y devuelve lo que cumpla `f`, en orden.
func buscar[T fyne.CanvasObject](o fyne.CanvasObject) []T {
	var fuera []T
	var ir func(fyne.CanvasObject)
	ir = func(o fyne.CanvasObject) {
		if x, ok := o.(T); ok {
			fuera = append(fuera, x)
		}
		switch c := o.(type) {
		case *fyne.Container:
			for _, h := range c.Objects {
				ir(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(c).Objects() {
				ir(h)
			}
		}
	}
	ir(o)
	return fuera
}

// TestLosDatosDelGraficoSeEditan: elegir el gráfico y abrir «Editar datos» enseña una
// celda por dato (3 × 2); escribir en una cambia el modelo SIN rehacer el panel (el
// campo sigue siendo el mismo, y con él el foco); «+ Fila» añade una del ancho de la
// rejilla y «− Fila» no deja bajar de dos.
//
// VISTO EN ROJO (2026-09-23, en el frontal): rehaciendo el panel en cada
// recomposición (sin la clave de su forma), el campo del test ya no era el que se
// enseñaba tras la primera letra.
func TestLosDatosDelGraficoSeEditan(t *testing.T) {
	e, _, _ := montar(t, conGraficoYDiagrama)
	e.SeleccionarSolo("g")
	e.alternarAbajo(abajoGrafico)
	celdas := buscar[*ui.Entrada](e.panel)
	if len(celdas) != 6 {
		t.Fatalf("el panel del gráfico enseña %d celdas y son 6", len(celdas))
	}
	panel := e.panel
	test.Type(celdas[5], "9") // Feb, Serie 1
	if got := e.m.caja(0, "g").Grafico.Rejilla[2][1]; got != celdas[5].Texto() || len(got) != 3 {
		t.Errorf("la celda escrita guarda %q y el campo dice %q", got, celdas[5].Texto())
	}
	if e.panel != panel {
		t.Error("teclear en una celda ha rehecho el panel: el campo pierde el foco")
	}
	botones := buscar[*botonPanel](e.panel)
	test.Tap(botones[0]) // + Fila
	if g := e.m.caja(0, "g").Grafico; len(g.Rejilla) != 4 || len(g.Rejilla[3]) != 2 {
		t.Errorf("«+ Fila» deja %d filas y la nueva con %d columnas", len(g.Rejilla), len(g.Rejilla[3]))
	}
	botones = buscar[*botonPanel](e.panel)
	test.Tap(botones[2]) // − Fila
	test.Tap(buscar[*botonPanel](e.panel)[2])
	if g := e.m.caja(0, "g").Grafico; len(g.Rejilla) != 2 {
		t.Errorf("«− Fila» deja %d filas y el suelo es 2", len(g.Rejilla))
	}
	e.SeleccionarSolo("t")
	if len(buscar[*ui.Entrada](e.panelAbajo())) != 0 {
		t.Error("sin gráfico elegido el panel dice «elige un gráfico», sin celdas")
	}
}

// La ficha Android 360×640 mostraba el título y la ayuda pintados uno encima del
// otro. Se comprueban los textos renderizados, el cuerpo y el pie, también al
// volver del escritorio al teléfono: el alto no puede depender del ancho anterior.
func TestLaCabeceraDelGraficoNoSeSolapaEnMovil(t *testing.T) {
	for _, alto := range []float32{260, 640} {
		t.Run(fmt.Sprintf("alto-%g", alto), func(t *testing.T) {
			e, _, w := montar(t, conGraficoYDiagrama)
			e.SeleccionarSolo("g")
			e.alternarAbajo(abajoGrafico)
			// La zona real incluye escenario y panel. Se aísla del cromo superior
			// para darle el ancho disponible tras el carril de iconos del móvil.
			w.SetContent(e.zona)
			panel, ok := e.panel.(*fyne.Container)
			if !ok {
				t.Fatal("no se ha montado el panel de datos del gráfico")
			}
			cab, cuerpo, pie := panel.Objects[2], panel.Objects[3], panel.Objects[4]
			for i, ancho := range []float32{320, 360, 375, 390, 1280, 320} {
				w.Resize(fyne.NewSize(ancho, alto))
				img := w.Canvas().Capture()
				if problema := problemaCabeceraPanel(cab); problema != "" {
					t.Fatalf("%gx%g: %s", ancho, alto, problema)
				}
				textos := buscar[*canvas.Text](cab)
				partes := make([]string, 0, len(textos))
				for _, texto := range textos {
					partes = append(partes, texto.Text)
				}
				contenido := strings.Join(partes, " ")
				if !strings.Contains(contenido, e.t("slides.chartData")) || !strings.Contains(contenido, e.t("slides.chart.hint")) {
					t.Fatalf("%g: se ha perdido texto de la cabecera: %q", ancho, contenido)
				}
				if cuerpo.Position().Y < cab.Position().Y+cab.Size().Height-0.01 ||
					cuerpo.Position().Y+cuerpo.Size().Height > pie.Position().Y+0.01 ||
					pie.Position().Y+pie.Size().Height > panel.Size().Height+0.01 ||
					panel.Position().Y+panel.Size().Height > alto+0.01 {
					t.Fatalf("%g: cabecera, cuerpo o pie invaden el siguiente bloque", ancho)
				}
				guarda(t, fmt.Sprintf("grafico-panel-%g-%g-%d.png", ancho, alto, i), img)
				if i == 0 {
					// Control negativo: el mismo guardián debe rechazar dos textos
					// en el mismo píxel, como en la captura que descubrió el fallo.
					a := fyne.CurrentApp().Driver().AbsolutePositionForObject(textos[0])
					b := fyne.CurrentApp().Driver().AbsolutePositionForObject(textos[1])
					antes := textos[1].Position()
					textos[1].Move(fyne.NewPos(antes.X+a.X-b.X, antes.Y+a.Y-b.Y))
					problema := problemaCabeceraPanel(cab)
					textos[1].Move(antes)
					if problema == "" {
						t.Fatal("el guardián acepta título y ayuda superpuestos")
					}
				}
			}
		})
	}
}

func problemaCabeceraPanel(cab fyne.CanvasObject) string {
	textos := buscar[*canvas.Text](cab)
	if len(textos) < 2 {
		return "faltan el título o la ayuda"
	}
	driver := fyne.CurrentApp().Driver()
	origen := driver.AbsolutePositionForObject(cab)
	for i, texto := range textos {
		p, m := driver.AbsolutePositionForObject(texto), texto.Size()
		if p.X < origen.X+tema.Espacio(3)-0.01 || p.Y < origen.Y-0.01 ||
			p.X+m.Width > origen.X+cab.Size().Width-tema.Espacio(3)+0.01 || p.Y+m.Height > origen.Y+cab.Size().Height+0.01 {
			return fmt.Sprintf("texto fuera de la cabecera: %q en %v/%v, cabecera=%v/%v", texto.Text, p, m, origen, cab.Size())
		}
		for _, anterior := range textos[:i] {
			q, n := driver.AbsolutePositionForObject(anterior), anterior.Size()
			if max(p.X, q.X) < min(p.X+m.Width, q.X+n.Width)-0.01 && max(p.Y, q.Y) < min(p.Y+m.Height, q.Y+n.Height)-0.01 {
				return fmt.Sprintf("textos superpuestos: %q y %q", anterior.Text, texto.Text)
			}
		}
	}
	return ""
}

// TestLosElementosDelDiagramaSeEditan: en la jerarquía, la raíz no se sangra, un
// elemento no salta más de un nivel bajo el anterior, y quitar deja al menos uno.
func TestLosElementosDelDiagramaSeEditan(t *testing.T) {
	m, err := Leer([]byte(conGraficoYDiagrama))
	if err != nil {
		t.Fatal(err)
	}
	m.SangrarElemento(0, "d", 0, 1)
	m.SangrarElemento(0, "d", 1, 1)
	m.SangrarElemento(0, "d", 2, 1)
	m.SangrarElemento(0, "d", 2, 1)
	m.SangrarElemento(0, "d", 2, 1)
	d := m.caja(0, "d").Diagrama
	if d.Elementos[0].Nivel != nil || nivelDe(d.Elementos[1]) != 1 || nivelDe(d.Elementos[2]) != 2 {
		t.Errorf("niveles = %v %v %v; tocaba 0, 1 y 2", d.Elementos[0].Nivel, nivelDe(d.Elementos[1]), nivelDe(d.Elementos[2]))
	}
	m.AñadirElemento(0, "d")
	if nivelDe(d.Elementos[3]) != 2 {
		t.Error("el elemento nuevo hereda la sangría del último")
	}
	m.MoverElemento(0, "d", 1, 1)
	if d.Elementos[1].Texto != "B" {
		t.Error("bajar el segundo lo cambia por el tercero")
	}
	for range 5 {
		m.QuitarElemento(0, "d", 0)
	}
	if len(d.Elementos) != 1 {
		t.Errorf("quitar deja %d elementos y el suelo es 1", len(d.Elementos))
	}
	m.SangrarElemento(0, "d", 1, -1) // fuera de rango: nada
}

// TestLaSecuenciaDeAnimacionesSeEdita: añadir dos efectos a dos cajas los pone en la
// secuencia con su paso, cambiar el disparador del segundo a «con la anterior» los
// junta en el mismo paso, subirlo los cambia de orden y quitar el primero renumera.
func TestLaSecuenciaDeAnimacionesSeEdita(t *testing.T) {
	e, _, _ := montar(t, conGraficoYDiagrama)
	e.m.AñadirAnim(0, "t", "fade")
	e.m.AñadirAnim(0, "g", "zoom")
	plan := Plan(e.m.Diapos[0])
	if len(plan) != 2 || plan[0].Caja != "t" || plan[1].Paso != 2 {
		t.Fatalf("plan = %+v", plan)
	}
	e.m.PonerDisparo(0, "g", 0, "with")
	if Plan(e.m.Diapos[0])[1].Paso != 1 {
		t.Error("«con la anterior» comparte paso")
	}
	e.m.MoverAnim(0, 2, -1)
	if p := Plan(e.m.Diapos[0]); p[0].Caja != "g" {
		t.Errorf("subir el segundo lo pone primero: %+v", p)
	}
	e.m.QuitarAnim(0, "g", 0)
	if p := Plan(e.m.Diapos[0]); len(p) != 1 || p[0].Anim.Orden != 1 {
		t.Errorf("quitar renumera: %+v", p)
	}
	// Y el panel: con la caja elegida enseña la fila de añadir y la secuencia.
	e.SeleccionarSolo("t")
	e.alternarAbajo(abajoAnim)
	if e.abajo != abajoAnim {
		t.Fatal("el botón de animaciones abre su panel")
	}
	if n := len(buscar[*botonPanel](e.panelAnim())); n != 3 {
		t.Errorf("una animación en la secuencia son 3 botones (subir, bajar, quitar) y hay %d", n)
	}
}
