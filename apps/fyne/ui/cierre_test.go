// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// # Todo modal se cierra
//
// El guardián genérico del barrido del 2026-09-25 («hay modales que no se pueden
// cerrar»): cada superposición de la suite, con cada control de la suite enfocado dentro,
// se cierra con su aspa, con Escape, con el «atrás» del teléfono —con foco y SIN foco,
// que es como queda tras pulsar en el texto del cuerpo— y con el clic fuera.
//
// Lo que se pinta encima en las trece apps sale de estas aperturas: el inventario de
// `TestInventarioDeModales` comprueba que ninguna app abre una capa por otro camino.

// aperturas son las formas de poner algo encima. `abrir` devuelve el aspa, si la hay.
var aperturas = []struct {
	nombre string
	abrir  func(c fyne.Canvas, disparador, cuerpo fyne.CanvasObject) *Boton
}{
	{"Dialogo", func(c fyne.Canvas, _, cuerpo fyne.CanvasObject) *Boton {
		d := NuevoDialogo("Mover a…", cuerpo)
		d.Mostrar(c)
		return d.aspa
	}},
	{"Confirmacion", func(c fyne.Canvas, _, cuerpo fyne.CanvasObject) *Boton {
		d := Confirmacion("Borrar", "Cancelar", "Borrar", true, cuerpo, nil, func() {})
		d.Mostrar(c)
		return d.aspa
	}},
	{"Pregunta", func(c fyne.Canvas, _, _ fyne.CanvasObject) *Boton {
		d := Pregunta("Nombre", "", "Cancelar", "Aceptar", "obligatorio", func(string) {})
		d.Mostrar(c)
		return d.aspa
	}},
	{"Popover", func(_ fyne.Canvas, disparador, cuerpo fyne.CanvasObject) *Boton {
		MostrarPopover(disparador, cuerpo)
		return nil
	}},
	{"Emergente", func(_ fyne.Canvas, disparador, cuerpo fyne.CanvasObject) *Boton {
		MostrarEmergente(disparador, cuerpo, tema.Espacio(2))
		return nil
	}},
	{"Menu", func(_ fyne.Canvas, disparador, _ fyne.CanvasObject) *Boton {
		NuevoMenu(NuevoElementoMenu("Renombrar", iconos.Lapiz, func() {}),
			NuevoElementoMenu("Borrar", iconos.Papelera, func() {})).Mostrar(disparador)
		return nil
	}},
	{"Selector", func(c fyne.Canvas, disparador, _ fyne.CanvasObject) *Boton {
		s := NuevoSelector([]string{"Uno", "Dos"}, func(int) {})
		disparador.(*fyne.Container).Add(s)
		s.Alternar()
		return nil
	}},
}

// controles son los enfocables de la suite que pueden quedar con el foco dentro de una
// capa. Uno nuevo que atienda teclas va aquí.
func controles() []fyne.CanvasObject {
	return []fyne.CanvasObject{
		NuevaEntrada("Buscar"),
		NuevaAreaTexto("Mensaje"),
		NuevoSelector([]string{"Uno", "Dos"}, func(int) {}),
		NuevaCasilla("Recordar", func(bool) {}),
		NuevoRadio("Uno", func(bool) {}),
		NuevoConmutador("Avisos", func(bool) {}),
		NuevoDeslizador(0, 10, 5, func(float64) {}),
		NuevoBoton("Guardar", func() {}),
		NuevoBotonIcono(iconos.Cerrar, "Cerrar", func() {}),
		NuevoSegmentado("Vista", []string{"Lista", "Rejilla"}, func(int) {}),
		NuevoEditorPlano(tema.TextoSM, nil),
		NuevaPestana("General", func() {}),
		NuevoEnlace("Ayuda", tema.TextoSM, Normal, "text", func() {}),
	}
}

// enfocables recorre lo montado y devuelve cada enfocable, sin entrar en un widget que
// ya lo es.
func enfocables(o fyne.CanvasObject) []fyne.Focusable {
	var out []fyne.Focusable
	var ir func(fyne.CanvasObject)
	ir = func(o fyne.CanvasObject) {
		if f, es := o.(fyne.Focusable); es {
			out = append(out, f)
			return
		}
		if c, es := o.(*fyne.Container); es {
			for _, h := range c.Objects {
				ir(h)
			}
		}
	}
	ir(o)
	return out
}

type modoCierre struct {
	nombre string
	cerrar func(t *testing.T, c fyne.Canvas, aspa *Boton, foco fyne.Focusable)
}

func tecla(n fyne.KeyName) *fyne.KeyEvent { return &fyne.KeyEvent{Name: n} }

var modosCierre = []modoCierre{
	{"Escape con el foco dentro", func(t *testing.T, c fyne.Canvas, _ *Boton, foco fyne.Focusable) {
		c.Focus(foco)
		if c.Focused() == nil {
			t.Skip("el control no toma el foco")
		}
		c.Focused().TypedKey(tecla(fyne.KeyEscape))
	}},
	{"atrás con el foco dentro", func(t *testing.T, c fyne.Canvas, _ *Boton, foco fyne.Focusable) {
		c.Focus(foco)
		if c.Focused() == nil {
			t.Skip("el control no toma el foco")
		}
		c.Focused().TypedKey(tecla(mobile.KeyBack))
	}},
	{"Escape sin foco", func(t *testing.T, c fyne.Canvas, _ *Boton, _ fyne.Focusable) {
		c.Unfocus() // lo que deja un clic en el texto del cuerpo
		if f := c.OnTypedKey(); f != nil {
			f(tecla(fyne.KeyEscape))
		}
	}},
	{"atrás sin foco", func(t *testing.T, c fyne.Canvas, _ *Boton, _ fyne.Focusable) {
		c.Unfocus()
		if f := c.OnTypedKey(); f != nil {
			f(tecla(mobile.KeyBack))
		}
	}},
	{"clic fuera", func(t *testing.T, c fyne.Canvas, _ *Boton, _ fyne.Focusable) {
		cp, es := c.Overlays().Top().(*capa)
		if !es {
			t.Fatalf("lo de encima es %T y no una capa de la suite", c.Overlays().Top())
		}
		test.Tap(cp.fondo)
	}},
	{"aspa", func(t *testing.T, _ fyne.Canvas, aspa *Boton, _ fyne.Focusable) {
		if aspa == nil {
			t.Skip("sin aspa: un menú o un popover no la llevan")
		}
		test.Tap(aspa)
	}},
}

// TestTodoModalSeCierra es el guardián: apertura × control enfocado × forma de cerrar.
//
// La entrada con fundido (una captura del panel entero por apertura) solo se hace con el
// primer control de cada apertura, en todos los modos: el cierre durante y después del
// fundido queda cubierto, y el resto de combinaciones no repite 500 capturas que no
// miran nada más —con -race eran 235 s de los 450 del paquete—.
func TestTodoModalSeCierra(t *testing.T) {
	for _, a := range aperturas {
		for i, control := range controles() {
			for _, m := range modosCierre {
				nombre := a.nombre + "/" + nombreTipo(control) + "/" + m.nombre
				t.Run(nombre, func(t *testing.T) {
					if i > 0 {
						antes := SinMovimiento
						SinMovimiento = true
						t.Cleanup(func() { SinMovimiento = antes })
					}
					disparador := container.NewHBox(NuevoBoton("Abrir", func() {}))
					w := test.NewTempWindow(t, container.NewVBox(disparador,
						fcanvas.NewRectangle(tema.Color("bg", tema.Claro))))
					w.Resize(fyne.NewSize(1280, 800))
					c := w.Canvas()
					// Una app que ya usa la ranura global (Notepads, CertiPDF, el ERP):
					// su manejador no sabe de diálogos y no debe quedarse con Escape.
					c.SetOnTypedKey(func(*fyne.KeyEvent) {})
					cuerpo := container.NewVBox(NuevoParrafo("Texto del cuerpo", tema.TextoSM, Normal, "text", tema.Claro), control)
					aspa := a.abrir(c, disparador, cuerpo)
					if c.Overlays().Top() == nil {
						t.Fatal("no se ha abierto nada")
					}
					foco := c.Focused()
					if fs := enfocables(control); len(fs) > 0 && a.nombre != "Pregunta" && a.nombre != "Menu" && a.nombre != "Selector" {
						foco = fs[0]
					}
					if foco == nil && (m.nombre == "Escape con el foco dentro" || m.nombre == "atrás con el foco dentro") {
						t.Skip("nada enfocable dentro")
					}
					m.cerrar(t, c, aspa, foco)
					if n := len(c.Overlays().List()); n != 0 {
						t.Errorf("sigue abierto (%d capas, encima %T, foco en %T)", n, c.Overlays().Top(), c.Focused())
					}
				})
			}
		}
	}
}

func nombreTipo(o fyne.CanvasObject) string {
	switch o.(type) {
	case *Entrada:
		return "Entrada"
	case *AreaTexto:
		return "AreaTexto"
	case *Selector:
		return "Selector"
	case *Eleccion:
		return "Eleccion"
	case *Conmutador:
		return "Conmutador"
	case *Deslizador:
		return "Deslizador"
	case *Boton:
		return "Boton"
	case *BotonIcono:
		return "BotonIcono"
	case *Segmentado:
		return "Segmentado"
	case *EditorPlano:
		return "EditorPlano"
	case *Pestana:
		return "Pestana"
	case *Enlace:
		return "Enlace"
	}
	return "otro"
}
