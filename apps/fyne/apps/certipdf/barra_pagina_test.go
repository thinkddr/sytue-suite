// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// El campo de página es controlado, como el `<input type="number" value={pagina}>` de
// Barra.tsx: lo que se escribe y no lleva a otra página vuelve a enseñar la actual, en
// vez de quedarse con lo tecleado. VISTO EN ROJO con el `ir` de antes: seguía «abc».
func TestElCampoDePaginaVuelveALaActual(t *testing.T) {
	b := NuevaBarra(identidad)
	b.Pagina, b.Total = 3, 5
	var fue []int
	b.AlIrA = func(n int) { fue = append(fue, n) }
	var campo *ui.Entrada
	for _, g := range b.gruposDeVer() {
		for _, o := range test.LaidOutObjects(g) {
			if e, ok := o.(*ui.Entrada); ok && campo == nil {
				campo = e
			}
		}
	}
	if campo == nil {
		t.Fatal("no hay campo de página")
	}
	for _, c := range []struct {
		escrito string
		ira     []int
	}{{"abc", nil}, {"", nil}, {"3", nil}, {"4", []int{4}}} {
		fue = nil
		campo.PonerTexto(c.escrito)
		campo.AlTeclear(&fyne.KeyEvent{Name: fyne.KeyReturn})
		if len(fue) != len(c.ira) || (len(fue) > 0 && fue[0] != c.ira[0]) {
			t.Errorf("«%s»: va a %v, quiero %v", c.escrito, fue, c.ira)
		}
		if campo.Texto() != "3" {
			t.Errorf("«%s»: el campo enseña %q y la página sigue siendo la 3", c.escrito, campo.Texto())
		}
	}
}
