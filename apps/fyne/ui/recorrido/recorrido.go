// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package recorrido es el barrido de una persona impaciente, hecho test: en una ventana
// de pruebas pulsa todo lo pulsable que se ve —y lo que va apareciendo al pulsar—, y
// cada vez que algo se pone encima lo cierra con Escape, como lo haría quien lo ha
// abierto sin querer. Apunta los pánicos y las capas que no se dejan cerrar.
//
// Es lo que el barrido del 2026-09-25 no pudo hacer en las apps reales (sin dbus ni
// llavero en el contenedor, no había sesión): aquí cada app va contra el servicio falso
// de sus propios tests, que es una sesión con datos sin credenciales de nadie.
//
// Solo lo importan tests. No depende de `ui` para no crear un ciclo con sus tests.
package recorrido

import (
	"fmt"
	"os"
	"reflect"
	"runtime/debug"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
)

// Informe es lo que ha pasado en un recorrido.
type Informe struct {
	// Pulsados y Modales cuentan lo que se ha pulsado y lo que, al pulsarlo, puso algo
	// encima.
	Pulsados, Modales int
	// Lista es lo pulsado, en orden, para ver qué se ha cubierto.
	Lista []string
	// Panicos y SinCerrar dicen qué se pulsó, con su texto, para buscarlo.
	Panicos, SinCerrar []string
}

// ponytail: una sola capa de profundidad. Dentro de un modal no se pulsa nada (enviaría
// formularios a medias); los modales que abren otros se ven cuando se abre el primero
// desde su propio test.

// Recorrer pulsa hasta `tope` cosas de la ventana, sin repetir ninguna. «Una» es su
// tipo, su texto y cuántas iguales van antes: una pantalla que se reconstruye al volver a
// ella trae widgets nuevos, y por puntero se pulsaría la misma pestaña sin fin.
func Recorrer(w fyne.Window, tope int) Informe {
	c := w.Canvas()
	var inf Informe
	vistos := map[string]bool{}
	for inf.Pulsados < tope {
		o, tp, clave := siguiente(c.Content(), vistos)
		if o == nil {
			break
		}
		vistos[clave] = true
		inf.Pulsados++
		antes := len(c.Overlays().List())
		que := describir(o)
		inf.Lista = append(inf.Lista, que)
		if p := pulsar(tp); p != "" {
			inf.Panicos = append(inf.Panicos, que+": "+p)
			quitarEncima(c, antes)
			continue
		}
		if len(c.Overlays().List()) <= antes {
			continue
		}
		inf.Modales++
		escape(c)
		if len(c.Overlays().List()) > antes {
			inf.SinCerrar = append(inf.SinCerrar, que+" abre "+nombre(c.Overlays().Top()))
			quitarEncima(c, antes)
		}
	}
	return inf
}

// Comprobar recorre la ventana y marca como error cada pánico y cada capa que no se
// cierra con Escape. `testing.Verbose()` saca además lo pulsado, para ver qué se cubrió.
func Comprobar(t testing.TB, w fyne.Window, tope int) Informe {
	t.Helper()
	if os.Getenv("SYTUE_SOLO_ROTACION") == "1" {
		t.Log("solo rotación: el recorrido de pulsaciones se ejecuta en la validación completa")
		return Informe{}
	}
	if dir := os.Getenv("SYTUE_CAPTURAS_PLAY"); dir != "" {
		return Retratar(t, w, dir, tope)
	}
	inf := Recorrer(w, tope)
	t.Log(inf.Resumen())
	if testing.Verbose() {
		t.Log(strings.Join(inf.Lista, " · "))
	}
	for _, p := range inf.Panicos {
		t.Error("pánico al pulsar " + p)
	}
	for _, m := range inf.SinCerrar {
		t.Error("no se cierra con Escape: " + m)
	}
	return inf
}

// Resumen es el informe en una línea, para el log del test.
func (i Informe) Resumen() string {
	return fmt.Sprintf("%d pulsados, %d modales, %d pánicos, %d sin cerrar",
		i.Pulsados, i.Modales, len(i.Panicos), len(i.SinCerrar))
}

// siguiente es el ÚLTIMO pulsable visible aún sin pulsar, en el orden del árbol: la
// navegación va antes que el contenido, así que con el último se recorre entera la
// pantalla a la que se llega antes de saltar a la siguiente.
func siguiente(raiz fyne.CanvasObject, vistos map[string]bool) (fyne.CanvasObject, fyne.Tappable, string) {
	veces := map[string]int{}
	var hallado fyne.CanvasObject
	var pulsable fyne.Tappable
	var clave string
	var ir func(fyne.CanvasObject)
	ir = func(o fyne.CanvasObject) {
		if o == nil || !o.Visible() {
			return
		}
		if tp, es := o.(fyne.Tappable); es {
			if d, es := o.(fyne.Disableable); !es || !d.Disabled() {
				k := describir(o)
				veces[k]++
				k = fmt.Sprintf("%s#%d", k, veces[k])
				if !vistos[k] {
					hallado, pulsable, clave = o, tp, k
				}
			}
		}
		for _, h := range hijos(o) {
			ir(h)
		}
	}
	ir(raiz)
	return hallado, pulsable, clave
}

func hijos(o fyne.CanvasObject) []fyne.CanvasObject {
	switch v := o.(type) {
	case *fyne.Container:
		return v.Objects
	case fyne.Widget:
		return test.WidgetRenderer(v).Objects()
	}
	return nil
}

// pulsar devuelve el pánico, con su primera línea de la app, o "".
func pulsar(o fyne.Tappable) (panico string) {
	defer func() {
		if r := recover(); r != nil {
			panico = fmt.Sprintf("%v @ %s", r, lineaDeLaSuite(string(debug.Stack())))
		}
	}()
	test.Tap(o)
	fyne.DoAndWait(func() {}) // lo que la pulsación dejó encolado
	return ""
}

func lineaDeLaSuite(pila string) string {
	for _, l := range strings.Split(pila, "\n") {
		l = strings.TrimSpace(l)
		if strings.Contains(l, "/apps/fyne/") && !strings.Contains(l, "/recorrido/") && strings.Contains(l, ".go:") {
			return strings.Fields(l)[0]
		}
	}
	return "?"
}

// escape es la tecla tal como la entrega el driver: al enfocado, o sin foco a la ranura
// del lienzo (`ui.asegurarTeclas` pone ahí la subida a `Descartar`).
func escape(c fyne.Canvas) {
	k := &fyne.KeyEvent{Name: fyne.KeyEscape}
	if f := c.Focused(); f != nil {
		f.TypedKey(k)
	} else if t := c.OnTypedKey(); t != nil {
		t(k)
	}
	fyne.DoAndWait(func() {})
}

// quitarEncima deja la pila como estaba para seguir recorriendo lo de debajo.
func quitarEncima(c fyne.Canvas, hasta int) {
	for len(c.Overlays().List()) > hasta {
		c.Overlays().Remove(c.Overlays().Top())
	}
}

// describir es el tipo y el primer texto que se ve dentro, que es como se busca a mano.
func describir(o fyne.CanvasObject) string {
	texto := ""
	var ir func(fyne.CanvasObject)
	ir = func(o fyne.CanvasObject) {
		if texto != "" || o == nil {
			return
		}
		if t, es := o.(*canvas.Text); es && strings.TrimSpace(t.Text) != "" {
			texto = t.Text
			return
		}
		for _, h := range hijos(o) {
			ir(h)
		}
	}
	ir(o)
	if texto == "" {
		return nombre(o)
	}
	return fmt.Sprintf("%s «%s»", nombre(o), texto)
}

func nombre(o any) string { return reflect.TypeOf(o).String() }
