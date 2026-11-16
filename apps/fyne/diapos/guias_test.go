// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

type rectJSON struct{ X, Y, W, H float64 }

func (r rectJSON) rect() rect { return rect{r.X, r.Y, r.W, r.H} }

// Los vectores son los mismos que prueba la web (packages/escena/src/guias.vectores.json).
func TestGuiasDeAjusteVectores(t *testing.T) {
	crudo, err := os.ReadFile("testdata/guias.vectores.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Casos []struct {
			Nombre     string
			Mov        rectJSON
			Otros      []rectJSON
			Lienzo     rectJSON
			Tolerancia json.RawMessage
			Anclas     struct{ X, Y []string }
			Esperado   AjusteGuias
		}
	}
	if err := json.Unmarshal(crudo, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Casos) == 0 {
		t.Fatal("sin vectores")
	}
	for _, c := range v.Casos {
		var tx, ty float64
		if err := json.Unmarshal(c.Tolerancia, &tx); err == nil {
			ty = tx
		} else {
			var tol struct{ X, Y float64 }
			if err := json.Unmarshal(c.Tolerancia, &tol); err != nil {
				t.Fatal(err)
			}
			tx, ty = tol.X, tol.Y
		}
		var otros []rect
		for _, o := range c.Otros {
			otros = append(otros, o.rect())
		}
		got := guiasDeAjuste(c.Mov.rect(), otros, c.Lienzo.rect(), tx, ty, c.Anclas.X, c.Anclas.Y)
		if got.Lineas == nil {
			got.Lineas = []Guia{}
		}
		if c.Esperado.Lineas == nil {
			c.Esperado.Lineas = []Guia{}
		}
		if !reflect.DeepEqual(got, c.Esperado) {
			t.Errorf("%s: %+v, quería %+v", c.Nombre, got, c.Esperado)
		}
	}
}

func TestGuiasAlArrastrarCajaYAlt(t *testing.T) {
	const fuente = `{"slides":[{"id":"s","boxes":[{"id":"a","x":10,"y":60,"w":10,"h":10,"html":"","kind":"shape","shape":"rect"},{"id":"b","x":30,"y":10,"w":20,"h":10,"html":"","kind":"shape","shape":"rect"}]}]}`
	for _, alt := range []bool{false, true} {
		e, _, _ := montar(t, fuente)
		e.SeleccionarSolo("a")
		e.capa.alt = alt
		m := marcoDe(t, e, "a")
		k := e.escenario.Escala
		m.Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(float32(9.6/100*AnchoEscenario)*k, 0)})
		x := e.m.caja(e.objetivo(), "a").X
		if !alt && (!casi(x, 20) || len(e.capa.guias) == 0) {
			t.Fatalf("sin Alt debía pegarse a 20 con guía: x=%v guías=%v", x, e.capa.guias)
		}
		if alt && (casi(x, 20) || len(e.capa.guias) != 0) {
			t.Fatalf("con Alt no debía ajustar: x=%v guías=%v", x, e.capa.guias)
		}
		m.DragEnd()
		if e.capa.guias != nil || strings.Contains(e.Serializar(), "guia") {
			t.Fatal("las guías no deben sobrevivir al soltar ni ir al modelo")
		}
	}
}

// El tirador de redimensionar lee Alt de la pulsación: un Alt pegado de antes (el KeyUp
// perdido tras Alt+Tab) no apaga las guías, y Alt pulsado sí.
func TestGuiasAlRedimensionarLeenAltDeLaPulsacion(t *testing.T) {
	const fuente = `{"slides":[{"id":"s","boxes":[{"id":"a","x":10,"y":10,"w":20,"h":20,"html":"","kind":"shape","shape":"rect"},{"id":"b","x":40,"y":70,"w":5,"h":5,"html":"","kind":"shape","shape":"rect"}]}]}`
	for _, alt := range []bool{false, true} {
		e, _, _ := montar(t, fuente)
		e.SeleccionarSolo("a")
		e.capa.alt = !alt
		a := asaDe(t, e, true)
		var mod fyne.KeyModifier
		if alt {
			mod = fyne.KeyModifierAlt
		}
		a.MouseDown(&desktop.MouseEvent{Modifier: mod})
		k := e.escenario.Escala
		a.Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(float32(9.4/100*AnchoEscenario)*k, 0)})
		w := e.m.caja(e.objetivo(), "a").W
		if !alt && (!casi(w, 30) || len(e.capa.guias) == 0) {
			t.Fatalf("sin Alt debía pegarse a w=30 con guía: w=%v guías=%v", w, e.capa.guias)
		}
		if alt && (casi(w, 30) || len(e.capa.guias) != 0) {
			t.Fatalf("con Alt no debía ajustar: w=%v guías=%v", w, e.capa.guias)
		}
		a.DragEnd()
	}
}
