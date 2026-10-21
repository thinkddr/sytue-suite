// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestLaPreguntaEsUsePrompt: el campo nace con el valor, el foco cae en él, el botón de
// aceptar se apaga con el campo vacío (en blanco también), e Intro acepta — que son las
// cuatro cosas que usePrompt.tsx hace y `window.prompt` no.
func TestLaPreguntaEsUsePrompt(t *testing.T) {
	w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(1280, 800))

	var aceptado []string
	d := Pregunta("Nombre de la hoja:", "Hoja 1", "Cancelar", "Guardar", "(obligatorio)",
		func(s string) { aceptado = append(aceptado, s) })
	d.Mostrar(w.Canvas())

	campo, ok := w.Canvas().Focused().(*Entrada)
	if !ok {
		t.Fatalf("el foco ha caído en %T y tiene que caer en el campo", w.Canvas().Focused())
	}
	if campo.Texto() != "Hoja 1" {
		t.Errorf("el campo nace con %q y tenía que traer el valor actual", campo.Texto())
	}
	_, aceptar := botonesDelPie(t, d)
	if aceptar.Inerte {
		t.Error("con un valor escrito el botón de aceptar está apagado")
	}

	campo.PonerTexto("   ")
	if !aceptar.Inerte {
		t.Error("con el campo en blanco el botón de aceptar sigue encendido")
	}
	campo.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if len(aceptado) != 0 {
		t.Errorf("Intro con el campo en blanco ha aceptado %q", aceptado)
	}

	campo.PonerTexto("Ventas")
	campo.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if len(aceptado) != 1 || aceptado[0] != "Ventas" {
		t.Errorf("Intro ha aceptado %q y tenía que aceptar «Ventas»", aceptado)
	}
}

// TestLaEtiquetaDeLaPreguntaEsLaDeFieldGroup vigila de dónde salen sus medidas: la
// etiqueta en `text-sm font-semibold`, el asterisco en `text-danger-text` a `ml-0.5` y
// el campo a `gap-1.5` por debajo.
func TestLaEtiquetaDeLaPreguntaEsLaDeFieldGroup(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/packages/ui/src/components/Form.tsx")
	for _, trozo := range []string{
		`class={cx('flex flex-col gap-1.5', cls)}`,
		`class="text-text text-sm font-semibold"`,
		`class="text-danger-text ml-0.5" aria-hidden="true"`,
	} {
		if !strings.Contains(fuente, trozo) {
			t.Errorf("Form.tsx ya no contiene %q: revisad la etiqueta de Pregunta", trozo)
		}
	}

	d := Pregunta("Nombre de la hoja:", "", "Cancelar", "Guardar", "(obligatorio)", func(string) {})
	cuerpo := d.Cuerpo.(*fyne.Container)
	etiqueta := cuerpo.Objects[0].(*fyne.Container)
	cuerpo.Resize(cuerpo.MinSize())
	etiqueta.Resize(etiqueta.MinSize())
	rotulo, asterisco := etiqueta.Objects[0], etiqueta.Objects[1]
	if x, quiero := asterisco.Position().X, rotulo.MinSize().Width+tema.Espacio(0.5); x != quiero {
		t.Errorf("el asterisco cae en x=%v y va a `ml-0.5` del rótulo (%v)", x, quiero)
	}
	if y, quiero := cuerpo.Objects[1].Position().Y, tema.TextoSM.Linea+tema.Espacio(1.5); y != quiero {
		t.Errorf("el campo cae en y=%v y va a `gap-1.5` bajo la etiqueta (%v)", y, quiero)
	}
	if a, ok := asterisco.(fyne.Accessible); !ok || a.AccessibilityLabel() != "(obligatorio)" {
		t.Error("el asterisco no se anuncia como «obligatorio»: se lee el signo")
	}
}

// TestLaPreguntaOpcionalAceptaElCampoVacio es `permitirVacio` de usePrompt.tsx: el botón
// de aceptar NO se apaga con el campo vacío, Intro acepta la cadena vacía, y la etiqueta
// va sin asterisco (FieldGroup sin `required`). Y cancelar no acepta nada, que es lo que
// distingue «sin etiqueta» de «no guardes».
func TestLaPreguntaOpcionalAceptaElCampoVacio(t *testing.T) {
	w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(1280, 800))

	var aceptado []string
	d := PreguntaOpcional("Etiqueta de la versión (opcional):", "", "Cancelar", "Guardar",
		func(s string) { aceptado = append(aceptado, s) })
	d.Mostrar(w.Canvas())
	campo, ok := w.Canvas().Focused().(*Entrada)
	if !ok {
		t.Fatalf("el foco ha caído en %T y tiene que caer en el campo", w.Canvas().Focused())
	}
	_, aceptar := botonesDelPie(t, d)
	if aceptar.Inerte {
		t.Error("con la etiqueta opcional y el campo vacío, el botón de aceptar se ha apagado")
	}
	if n := len(d.Cuerpo.(*fyne.Container).Objects[0].(*fyne.Container).Objects); n != 1 {
		t.Errorf("la etiqueta opcional lleva %d piezas y es solo el rótulo, sin asterisco", n)
	}
	campo.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if len(aceptado) != 1 || aceptado[0] != "" {
		t.Errorf("Intro con el campo vacío ha aceptado %q y tenía que aceptar la cadena vacía", aceptado)
	}

	aceptado = nil
	d = PreguntaOpcional("Etiqueta de la versión (opcional):", "", "Cancelar", "Guardar",
		func(s string) { aceptado = append(aceptado, s) })
	d.Mostrar(w.Canvas())
	cancelar, _ := botonesDelPie(t, d)
	cancelar.Tapped(nil)
	if len(aceptado) != 0 {
		t.Errorf("cancelar ha aceptado %q", aceptado)
	}
}

// TestLaPreguntaOpcionalMideLoQueLaWeb: la pregunta de «Guardar versión» contra la de
// la web, caja a caja y a los dos anchos. Los números de la web son de Chromium
// (`--font-render-hinting=none`, Figtree cargada) sobre usePrompt con `permitirVacio`,
// medidos con `getBoundingClientRect` a raíz de 16px el 2026-10-03; aquí van relativos a la esquina del
// panel. Del título y de la etiqueta se compara el ancho del TEXTO (un Range): en la web
// el `<h2>` encoge a su texto y el `<label>` se estira a la columna, y aquí el título
// tiene todo el hueco de la banda y el rótulo mide lo suyo; lo que se ve es el texto.
// El 2026-10-14 el cierre de Modal.tsx pasó a shrink-0: conserva los 32 de h-8 w-8
// también a 360. El 2026-10-15, medido otra vez con el diálogo nuevo: sin filetes entre
// bandas (`pt-5 pb-2` / `py-3` / `pt-2 pb-5`) y, a 360, la hoja del teléfono: a todo el
// ancho, sin filetes a los lados ni abajo y con el asa encima de la cabecera. El
// 2026-10-16, otra vez (arnés del barrido, `herramientas/medir-pregunta.mjs`): a 360 el
// pie pone sus dos acciones a partes iguales y a 44 de alto (`PIE_TELEFONO`); el antes
// salió idéntico al del 2026-10-15 a la centésima.
func TestLaPreguntaOpcionalMideLoQueLaWeb(t *testing.T) {
	modal := webref.Read(t, "apps/frontend/packages/ui/src/components/Modal.tsx")
	boton := webref.Read(t, "apps/frontend/packages/ui/src/components/Button.tsx")
	if !strings.Contains(modal, `class="hover:bg-chrome-hover active:bg-chrome-active shrink-0"`) ||
		!strings.Contains(boton, `sm: 'h-8 w-8 p-0`) {
		t.Fatal("el cierre de Modal.tsx dejó de ser h-8 w-8 shrink-0: revisad la referencia de la pregunta")
	}
	type caja struct{ x, y, ancho, alto float32 }
	for _, c := range []struct {
		lienzo                                                  fyne.Size
		panel, titulo, etiqueta, campo, aspa, cancelar, aceptar caja
	}{
		{
			fyne.NewSize(1280, 800),
			caja{440, 294, 400, 212},       // panel
			caja{25, 24, 263.203, 26},      // título, sin los dos puntos (re-medido el 2026-10-17)
			caja{25, 74, 190.094, 18},      // etiqueta, también sin ellos
			caja{25, 98, 350, 36},          // campo
			caja{343, 21, 32, 32},          // aspa
			caja{195.078, 155, 88.406, 36}, // «Cancelar»
			caja{291.484, 155, 83.516, 36}, // «Guardar»
		},
		{
			fyne.NewSize(360, 740),
			caja{0, 521, 360, 219},    // panel: la hoja
			caja{24, 24, 263.203, 26}, // título: sin los dos puntos cabe en una línea
			caja{24, 74, 190.094, 18}, // etiqueta
			caja{24, 98, 312, 36},     // campo
			caja{304, 21, 32, 32},     // aspa: h-8 w-8 shrink-0
			caja{24, 155, 152, 44},    // «Cancelar»: media fila y 44 de alto
			caja{184, 155, 152, 44},   // «Guardar», el principal a la derecha
		},
	} {
		w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
		w.Resize(c.lienzo)
		d := PreguntaOpcional("Etiqueta de la versión (opcional):", "", "Cancelar", "Guardar", func(string) {})
		d.Mostrar(w.Canvas())
		r := test.WidgetRenderer(d).(*dialogoRenderer)
		cuerpo := d.Cuerpo.(*fyne.Container)
		cancelar, aceptar := botonesDelPie(t, d)
		absoluta := fyne.CurrentApp().Driver().AbsolutePositionForObject
		origen := absoluta(d)
		medirCon := func(nombre string, o fyne.CanvasObject, ancho float32, quiero caja) {
			p := absoluta(o).Subtract(origen)
			got := caja{p.X, p.Y, ancho, o.Size().Height}
			// ±0,25: el ancho de un rótulo queda a ≤0,17 del navegador, y la web
			// redondea a centésimas.
			if abs(got.x-quiero.x) > 0.25 || abs(got.y-quiero.y) > 0.25 ||
				abs(got.ancho-quiero.ancho) > 0.25 || abs(got.alto-quiero.alto) > 0.25 {
				t.Errorf("a %v, %s: %+v y en la web %+v", c.lienzo, nombre, got, quiero)
			}
		}
		medir := func(nombre string, o fyne.CanvasObject, quiero caja) { medirCon(nombre, o, o.Size().Width, quiero) }
		// La X del panel es absoluta; la Y no, porque la entrada lo desplaza 10 px.
		if x, tam := origen.X, d.Size(); abs(x-c.panel.x) > 0.25 || tam != fyne.NewSize(c.panel.ancho, c.panel.alto) {
			t.Errorf("a %v, el panel: x=%v %v y en la web x=%v %vx%v", c.lienzo, x, tam, c.panel.x, c.panel.ancho, c.panel.alto)
		}
		var anchoTitulo float32
		for _, linea := range lineasTitulo(d.Titulo, d.anchoUtil(d.Size().Width)) {
			anchoTitulo = max(anchoTitulo, Medir(linea, TituloDialogo, SemiNegrita).Width)
		}
		medirCon("el título", r.titulo, anchoTitulo, c.titulo)
		medir("la etiqueta", cuerpo.Objects[0].(*fyne.Container).Objects[0], c.etiqueta)
		medir("el campo", cuerpo.Objects[1], c.campo)
		medir("el aspa", d.aspa, c.aspa)
		medir("«Cancelar»", cancelar, c.cancelar)
		medir("«Guardar»", aceptar, c.aceptar)
		d.Cerrar()
	}
}

// botonesDelPie saca los dos botones del pie de una confirmación, en su orden.
func botonesDelPie(t *testing.T, d *Dialogo) (cancelar, aceptar *Boton) {
	t.Helper()
	pie, ok := d.Pie.(*fyne.Container)
	if !ok || len(pie.Objects) != 2 {
		t.Fatalf("el pie de la pregunta no son dos botones: %T", d.Pie)
	}
	return pie.Objects[0].(*Boton), pie.Objects[1].(*Boton)
}

// TestLaPreguntaSinDosPuntos: las preguntas heredadas de `window.prompt` traen sus dos
// puntos y hacen de título y de etiqueta, donde no llevan (como sinDosPuntos de la web).
func TestLaPreguntaSinDosPuntos(t *testing.T) {
	for entra, sale := range map[string]string{
		"Nombre de la macro:":                 "Nombre de la macro",
		"Etiqueta de la versión (opcional): ": "Etiqueta de la versión (opcional)",
		"Nom de la macro\u202f:":              "Nom de la macro",
		"Hora 10:30 del lunes":                "Hora 10:30 del lunes",
	} {
		if got := SinDosPuntos(entra); got != sale {
			t.Errorf("SinDosPuntos(%q) = %q; quiero %q", entra, got, sale)
		}
	}
}
