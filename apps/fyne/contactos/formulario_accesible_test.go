// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// nombresAccesibles son los `AccessibilityLabel` de los controles del árbol, en orden.
func nombresAccesibles(o fyne.CanvasObject) []string {
	var fuera []string
	if a, ok := o.(fyne.Accessible); ok {
		fuera = append(fuera, a.AccessibilityLabel())
	}
	if c, ok := o.(*fyne.Container); ok {
		for _, h := range c.Objects {
			fuera = append(fuera, nombresAccesibles(h)...)
		}
	}
	return fuera
}

// TestLosControlesDelFormularioSeAnuncianConSuIndice: como los `aria-label` de la web,
// cada control de las filas de correo, teléfono y dirección dice qué es y de qué fila
// («Correo 2»), y el área del asistente su rótulo, no la marca de agua.
func TestLosControlesDelFormularioSeAnuncianConSuIndice(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{persona("ana", "Ana")}, ia: "{}"}
	_, f := formularioDePrueba(t, s, false)
	f.correos = append(f.correos, f.nuevaFila(tiposCorreo, 1, "ada@trabajo.test", "form.emailPlaceholder"))
	f.direcciones = append(f.direcciones, f.nuevaDireccion(api.ContactAddress{Type: "home"}))
	f.rehacer()

	nombres := nombresAccesibles(f.cuerpo)
	for _, quiero := range []string{
		"Pega una firma o una tarjeta",
		"Etiqueta del correo 1", "Correo 1", "Etiqueta del correo 2", "Correo 2",
		"Etiqueta del teléfono 1", "Teléfono 1",
		"Etiqueta de la dirección 1", "Calle, dirección 1", "Ciudad, dirección 1",
		"Provincia o región, dirección 1", "Código postal, dirección 1", "País, dirección 1",
	} {
		if !slices.Contains(nombres, quiero) {
			t.Errorf("ningún control se anuncia «%s»: %q", quiero, nombres)
		}
	}

	// Al quitar el primer correo, el que queda pasa a ser el 1 y lo dice.
	f.quitarFila(&f.correos, 0, &f.principalCorreo)
	if got := f.correos[0].valor.AccessibilityLabel(); got != "Correo 1" {
		t.Errorf("el correo que queda se anuncia %q", got)
	}
	if got := f.correos[0].tipo.AccessibilityLabel(); got != "Etiqueta del correo 1" {
		t.Errorf("su etiqueta se anuncia %q", got)
	}
}

// TestLaRuedaDeLaPapeleraDiceQueCarga: mientras llega la papelera, la rueda se anuncia
// con el `label` de la web y no con el «Cargando» genérico.
func TestLaRuedaDeLaPapeleraDiceQueCarga(t *testing.T) {
	a := appDePrueba(t, &servicioFalso{})
	x := a.Pantalla.nuevaPapelera(a.lienzo())
	var rueda *ui.Rueda
	var buscar func(o fyne.CanvasObject)
	buscar = func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *ui.Rueda:
			rueda = v
		case *fyne.Container:
			for _, h := range v.Objects {
				buscar(h)
			}
		}
	}
	buscar(x.cuerpo)
	if rueda == nil {
		t.Fatal("cargando, la papelera no enseña la rueda")
	}
	if got, want := rueda.AccessibilityLabel(), a.Pantalla.t("trash.loading", nil); got != want {
		t.Errorf("la rueda se anuncia %q y la web %q", got, want)
	}
}
