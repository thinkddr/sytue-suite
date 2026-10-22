// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Los vectores de validateValue y sanitizeDataValidation son los de
// `packages/office/test/sheets-validation.test.ts`.
func TestLaValidacionEsLaDeLaWeb(t *testing.T) {
	for _, c := range []struct {
		v     Validacion
		texto string
		vale  bool
	}{
		{Validacion{Valores: []string{"Sí", "No"}}, "Sí", true},
		{Validacion{Valores: []string{"Sí", "No"}}, "sí", false}, // exacto, con mayúsculas
		{Validacion{Valores: []string{"Sí", "No"}}, "", true},    // borrar siempre vale
		{Validacion{Tipo: "number", Op: "between", A: "1", B: "10"}, "5", true},
		{Validacion{Tipo: "number", Op: "between", A: "1", B: "10"}, "11", false},
		{Validacion{Tipo: "number", Op: "gt", A: "0"}, "12 kg", false}, // estricto en la forma
		{Validacion{Tipo: "number", Op: "gt", A: "0", Entero: true}, "2,5", false},
		{Validacion{Tipo: "number", Op: "notBetween", A: "1", B: "10"}, "0", true},
		{Validacion{Tipo: "date", Op: "ge", A: "01/01/2024"}, "15/3/2024", true},
		{Validacion{Tipo: "date", Op: "ge", A: "01/01/2024"}, "31/12/2023", false},
		{Validacion{Tipo: "date", Op: "ge", A: "01/01/2024"}, "31/02/2024", false}, // no existe
		{Validacion{Tipo: "textLength", Op: "le", A: "3"}, "abcd", false},
		{Validacion{Tipo: "textLength", Op: "le", A: "3"}, "abc", true},
	} {
		if got := c.v.cumple(c.texto, nil); got != c.vale {
			t.Errorf("validateValue(%+v, %q) = %v", c.v, c.texto, got)
		}
	}
	if n, ok := fechaTecleada("1/3/1900"); !ok || n != 61 {
		t.Errorf("parseEsDate(1/3/1900) = %v, y el día fantasma deja el 1 de marzo en 61", n)
	}
	if _, ok := (validacionCruda{Rango: "A1", Valores: []any{" ", ""}}).sanear(); ok {
		t.Error("una lista sin valores se da por buena")
	}
	if _, ok := (validacionCruda{Rango: "A1", Tipo: "number", Op: "gt", A: "abc"}).sanear(); ok {
		t.Error("un operando que no es un número se da por bueno")
	}
}

// TestLaValidacionEstrictaRechazaYLaOtraMarca: la estricta no deja escribir y avisa con
// su mensaje; la otra deja y marca en rojo; y la de lista pone el «▾» con sus valores.
func TestLaValidacionEstrictaRechazaYLaOtraMarca(t *testing.T) {
	c := lienzoDePrueba()
	r := c.Rejilla
	var aviso string
	c.AlAviso = func(_ ui.TonoAviso, _, detalle string) { aviso = detalle }
	r.Seleccionar(0, 0, false)
	c.Mandar(CmdValAnadir, `{"range":"A1:A5","values":["Sí","No"],"strict":true}`)
	r.Editar("Quizá")
	r.Confirmar(0, 1)
	if c.Libro.Crudo(0, 0) != "" || aviso != "La celda A1 solo admite: Sí, No." {
		t.Errorf("la estricta ha dejado %q y ha avisado %q", c.Libro.Crudo(0, 0), aviso)
	}
	if r.Seleccion() != (Celda{C: 0, F: 1}) {
		t.Error("tras el rechazo la selección no baja, y en la web sí")
	}

	// El «▾» de la lista, y elegir un valor lo escribe.
	pos, tam, ok := r.botonValores()
	if !ok {
		t.Fatal("la celda con una validación de lista no tiene su desplegable")
	}
	r.Tapped(&fyne.PointEvent{Position: pos.AddXY(tam.Width/2, tam.Height/2)})
	if c.valoresAbiertos() == nil {
		t.Fatal("pulsar el «▾» no abre la lista")
	}
	c.elegirValor("No")
	if c.Libro.Crudo(0, 1) != "No" || c.valoresAbiertos() != nil {
		t.Errorf("elegir «No» deja %q y la lista abierta: %v", c.Libro.Crudo(0, 1), c.valoresAbiertos() != nil)
	}

	// La no estricta marca en rojo lo que no cumple.
	c.Mandar(CmdValAnadir, `{"range":"B1","type":"number","op":"gt","a":"0"}`)
	r.Seleccionar(1, 0, false)
	r.Escribir("-3")
	r.Seleccionar(3, 3, false) // el marco de la celda ancla taparía el contorno
	if !c.Libro.marcaInvalida(1, 0) {
		t.Error("un −3 con «mayor que 0» no estricto no se marca")
	}
	img := pinta(t, 560, 300, c)
	guarda(t, img)
	if got := px(img, AnchoCabecera+AnchoCelda+AnchoCelda/2, AltoBarra+AltoCelda+0.5); !cerca(got, Peligro) {
		t.Errorf("el contorno de B1 pinta %v y es `DANGER` (%v)", got, Peligro)
	}
}
