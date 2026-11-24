// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
)

// TestLosBloquesRicosVanYVuelven: cada formato compartido con la web se lee y se vuelve a
// escribir IGUAL (HTML → modelo → HTML), y la nota es fiel, o sea que se puede editar.
//
// VISTO EN ROJO (2026-11-24): sin leer `data-panel`, `<details>`, `data-estado`, `<time>`,
// `data-tipo` ni `data-lenguaje`, cada una de estas notas salía no fiel (solo lectura) y
// perdía el atributo al reescribirse.
func TestLosBloquesRicosVanYVuelven(t *testing.T) {
	for nombre, fuente := range map[string]string{
		"panel":                `<div data-panel="info"><p>Ojo con el plazo.</p></div>`,
		"panel con más":        `<div data-panel="aviso"><p>Antes:</p><ul><li>uno</li><li>dos</li></ul></div><p>Fuera.</p>`,
		"cada panel":           `<div data-panel="nota"><p>a</p></div><div data-panel="exito"><p>b</p></div><div data-panel="error"><p>c</p></div>`,
		"desplegable":          `<details data-expandir=""><summary>Detalles</summary><p>Dentro.</p></details>`,
		"estado":               `<p>Estado: <span data-estado="verde">HECHO</span> y <span data-estado="morado">EN CURSO</span></p>`,
		"decisiones":           `<ul data-tipo="decisiones"><li>Ir en tren</li><li>Salir el lunes</li></ul>`,
		"decisiones y viñetas": `<ul data-tipo="decisiones"><li>Ir en tren</li></ul><ul><li>Billetes</li></ul>`,
		"fecha":                `<p>Entrega el <time datetime="2026-11-24">24 nov 2026</time>.</p>`,
		"código":               `<pre><code data-lenguaje="go">fmt.Println("hola")</code></pre>`,
		"tabla":                `<table><thead><tr><th>Cuarto</th><th>Cajas</th></tr></thead><tbody><tr><th scope="row">Cocina</th><td data-fondo="azul">12</td></tr></tbody></table>`,
		"tarea":                `<div class="task-row"><input type="checkbox" checked=""/> <span>Pan</span></div>`,
	} {
		t.Run(nombre, func(t *testing.T) {
			d := Leer(fuente)
			if !d.Fiel() {
				t.Fatalf("no es fiel: vuelve %s", d.HTML())
			}
			if got, quiero := normalizar(d.HTML()), normalizar(fuente); got != quiero {
				t.Fatalf("vuelve\n  %s\ny era\n  %s", got, quiero)
			}
			// Y lo que se guarda al editar, también.
			if got := Leer(d.ParaEditar().ParaGuardar()); !got.Fiel() || normalizar(got.HTML()) != normalizar(fuente) {
				t.Fatalf("al editar se guarda %s", d.ParaEditar().ParaGuardar())
			}
		})
	}
}

// TestLosValoresFueraDeLaListaSeCaen: lo que no está en las listas cerradas no se lee: el
// atributo no vuelve y la nota no es fiel (se abre en solo lectura).
func TestLosValoresFueraDeLaListaSeCaen(t *testing.T) {
	for fuente, prohibido := range map[string]string{
		`<div data-panel="rosa"><p>x</p></div>`:                               "data-panel",
		`<p><span data-estado="naranja">X</span></p>`:                         "data-estado",
		`<pre><code data-lenguaje="cobol">x</code></pre>`:                     "data-lenguaje",
		`<p><time datetime="ayer">ayer</time></p>`:                            "datetime",
		`<table><tbody><tr><td data-fondo="rosa">x</td></tr></tbody></table>`: "data-fondo",
		`<table><tbody><tr><th scope="col">x</th></tr></tbody></table>`:       "scope",
		`<ul data-tipo="otra"><li>x</li></ul>`:                                "data-tipo",
	} {
		d := Leer(fuente)
		if d.Fiel() || strings.Contains(d.HTML(), prohibido) {
			t.Errorf("%s: fiel=%v y vuelve %s", fuente, d.Fiel(), d.HTML())
		}
	}
}

// TestDentroDeUnaCajaSeEscribeComoFuera: Intro dentro de un panel sigue en él; Intro en
// la línea vacía del final sale; Retroceso al principio de la primera línea, también.
func TestDentroDeUnaCajaSeEscribeComoFuera(t *testing.T) {
	ed := NuevoEditorNota(trInterno(t), Leer(`<div data-panel="info"><p>uno</p></div>`), "")
	z := ed.zona
	z.TypedKey(&fyneKeyReturn)
	teclear(z, "dos")
	if got := ed.HTML(); got != `<div data-panel="info"><p>uno</p><p>dos</p></div>` {
		t.Fatalf("Intro en el panel deja %s", got)
	}
	z.TypedKey(&fyneKeyReturn)
	z.TypedKey(&fyneKeyReturn)
	teclear(z, "fuera")
	if got := ed.HTML(); got != `<div data-panel="info"><p>uno</p><p>dos</p></div><p>fuera</p>` {
		t.Fatalf("Intro en la línea vacía del final deja %s", got)
	}
	ed.cur, ed.ancla = Pos{0, 0}, Pos{0, 0}
	z.TypedKey(&fyneKeyBackspace)
	if got := ed.HTML(); got != `<p>uno</p><div data-panel="info"><p>dos</p></div><p>fuera</p>` {
		t.Fatalf("Retroceso al principio del panel deja %s", got)
	}
}

var (
	fyneKeyReturn    = fyne.KeyEvent{Name: fyne.KeyReturn}
	fyneKeyBackspace = fyne.KeyEvent{Name: fyne.KeyBackspace}
)
