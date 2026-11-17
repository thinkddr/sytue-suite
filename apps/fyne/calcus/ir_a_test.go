// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// vectoresIrA es la sección `irA` de los vectores compartidos con la web.
type vectoresIrA struct {
	IrA struct {
		Libro struct {
			Hojas   []string
			Activa  int
			Nombres []Nombre
		}
		Casos        []casoIrA
		DesdeResumen []casoIrA `json:"desdeResumen"` //nolint:tagliatelle // el nombre lo fija la web
		Nombres      []struct {
			Activa int
			Sale   []string
		}
	} `json:"irA"` //nolint:tagliatelle // el nombre lo fija la web
}

type casoIrA struct {
	Texto string
	Sale  json.RawMessage
}

// salidaIrA es lo que la web guarda: el destino o `{error}`.
func salidaIrA(d DestinoIrA, codigo string) any {
	if codigo != "" {
		return map[string]any{"error": codigo}
	}
	return map[string]any{
		"hoja": float64(d.Hoja), "c1": float64(d.C1), "r1": float64(d.R1),
		"c2": float64(d.C2), "r2": float64(d.R2),
	}
}

// TestVectoresDeIrA: el puerto de `irA.ts` da lo mismo que la web con las mismas
// entradas —referencia, rango, otra hoja, nombre del libro y de la hoja, e inválidos—.
func TestVectoresDeIrA(t *testing.T) {
	b, err := os.ReadFile(rutaVectores)
	if err != nil {
		t.Fatalf("no se leen los vectores de la web: %v", err)
	}
	var v vectoresIrA
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("los vectores no se entienden: %v", err)
	}
	lb := v.IrA.Libro
	if len(v.IrA.Casos) < 30 {
		t.Fatalf("solo %d casos de «Ir a»: ¿se ha regenerado calcus.json?", len(v.IrA.Casos))
	}
	comprobar := func(casos []casoIrA, activa int) {
		for _, c := range casos {
			d, codigo := irA(c.Texto, lb.Hojas, activa, lb.Nombres)
			if got := salidaIrA(d, codigo); !reflect.DeepEqual(got, crudoJSON(c.Sale)) {
				t.Errorf("irA(%q) desde la hoja %d = %v, la web dice %s", c.Texto, activa, got, c.Sale)
			}
		}
	}
	comprobar(v.IrA.Casos, lb.Activa)
	comprobar(v.IrA.DesdeResumen, 3)
	for _, c := range v.IrA.Nombres {
		if got := nombresParaIrA(lb.Hojas, c.Activa, lb.Nombres); !reflect.DeepEqual(got, c.Sale) {
			t.Errorf("nombresParaIrA desde la hoja %d = %v, la web dice %v", c.Activa, got, c.Sale)
		}
	}
}

// TestBarraCuadroDeNombres: Ctrl+G y F5 llevan al cuadro, Intro salta a una celda, a un
// rango de otra hoja o a un nombre y devuelve el foco a la rejilla; lo que no lleva a
// ninguna parte avisa sin moverse, y Escape vuelve sin moverse. Es
// `sheets-atajos-hoja.test.tsx` de la web.
func TestBarraCuadroDeNombres(t *testing.T) {
	c := lienzoTraducido(t)
	c.Libro.f.nombres = []Nombre{{Name: "Cierre", Ref: "Ventas!$B$2:$B$13"}}
	w := test.NewTempWindow(t, c)
	w.Resize(fyne.NewSize(560, 300))
	var avisos []string
	c.AlAviso = func(_ ui.TonoAviso, titulo, detalle string) { avisos = append(avisos, titulo+" · "+detalle) }
	lz := w.Canvas()
	b := c.Barra

	escribir := func(texto string, tecla fyne.KeyName) {
		t.Helper()
		lz.Focus(b.Cuadro())
		if !b.CuadroEnfocado() {
			t.Fatal("el cuadro de nombres no se queda el foco")
		}
		b.Cuadro().SetText(texto)
		b.Cuadro().TypedKey(&fyne.KeyEvent{Name: tecla})
	}

	lz.Focus(c.Rejilla)
	ctrlMas(c.Rejilla, fyne.KeyG, false)
	if lz.Focused() != b.Cuadro() {
		t.Fatal("Ctrl+G no lleva el foco al cuadro de nombres")
	}
	if got := b.Cuadro().Texto(); got != "A1" {
		t.Errorf("al enfocarlo, el cuadro dice %q y no la celda activa", got)
	}
	lz.Focus(c.Rejilla)
	c.Rejilla.TypedKey(&fyne.KeyEvent{Name: fyne.KeyF5})
	if lz.Focused() != b.Cuadro() {
		t.Fatal("F5 no lleva el foco al cuadro de nombres")
	}

	escribir("c40", fyne.KeyReturn)
	if s := c.Rejilla.Seleccion(); s != (Celda{C: 2, F: 39}) || lz.Focused() != c.Rejilla {
		t.Errorf("«c40» deja la selección en %v y el foco en %T", s, lz.Focused())
	}
	if got := b.Rotulo(); got != "C40" {
		t.Errorf("el cuadro dice %q tras ir a C40", got)
	}

	escribir("ventas!AA1000:AB1200", fyne.KeyReturn)
	if c.Libro.Activa() != 1 || b.Rotulo() != "AA1000:AB1200" {
		t.Errorf("«ventas!AA1000:AB1200» deja la hoja %d y el rango %q", c.Libro.Activa(), b.Rotulo())
	}

	escribir("Cierre", fyne.KeyReturn)
	if b.Rotulo() != "B2:B13" {
		t.Errorf("el nombre «Cierre» lleva a %q y no a B2:B13", b.Rotulo())
	}

	escribir("NoHay!A1", fyne.KeyReturn)
	if len(avisos) != 1 || avisos[0] != "No se puede ir ahí · La hoja de «NoHay!A1» no existe." {
		t.Errorf("lo que no lleva a ninguna parte avisa %q", avisos)
	}
	if b.Rotulo() != "B2:B13" || lz.Focused() != b.Cuadro() || b.Cuadro().Texto() != "NoHay!A1" {
		t.Error("tras el aviso, la selección se ha movido o el cuadro ha perdido lo escrito")
	}

	escribir("Z9", fyne.KeyEscape)
	if b.Rotulo() != "B2:B13" || lz.Focused() != c.Rejilla || b.CuadroEnfocado() {
		t.Error("Escape en el cuadro mueve la selección o no devuelve el foco a la rejilla")
	}
}

// TestElCuadroNoDesbordaConUnRangoLargo: «AA1000:AB1200» cabe en el `w-24`, y lo que no
// quepa se corta con puntos dentro de la caja, como el `truncate` de la web.
func TestElCuadroNoDesbordaConUnRangoLargo(t *testing.T) {
	c := lienzoDePrueba()
	w := test.NewTempWindow(t, c)
	w.Resize(fyne.NewSize(560, 300))
	r := c.Barra.rend
	for _, rango := range [][4]int{{26, 999, 27, 1199}, {16380, 1048570, 16383, 1048575}} {
		c.Rejilla.SeleccionarDestino(rango[0], rango[1], rango[2], rango[3])
		c.Barra.Refresh()
		fin := r.rotulo.Position().X + r.rotulo.MinSize().Width
		if r.rotulo.Position().X < r.refCaja.Position().X || fin > r.refCaja.Position().X+AnchoRef+0.5 {
			t.Errorf("el rótulo %q se sale de su caja (%v–%v)", r.rotulo.Text, r.rotulo.Position().X, fin)
		}
	}
	if r.rotulo.Text == c.Barra.Rotulo() {
		t.Errorf("«%s» no cabe y no se ha cortado", c.Barra.Rotulo())
	}
}
