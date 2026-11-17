// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package nombrehoja

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestLaClaveEsEqualFold: dos nombres tienen la misma clave si y solo si EqualFold los
// da por iguales, también con las letras de órbita larga (la sigma, la ese larga, el
// signo de kelvin) y las que solo parecen gemelas («ß» y «SS» no lo son para Go).
func TestLaClaveEsEqualFold(t *testing.T) {
	nombres := []string{
		"Ventas", "VENTAS", "ventas", "Ventas ", "Hoja 1", "HOJA 1", "Año", "AÑO", "año",
		"ΣΟΦΙΑ", "σοφια", "σοφιας", "ſ", "s", "S", "K", "k", "K", "ß", "SS", "ẞ",
		"İstanbul", "istanbul", "Datos (2)", "datos (2)", "",
	}
	for _, a := range nombres {
		for _, b := range nombres {
			if got, quiero := clave(a) == clave(b), strings.EqualFold(a, b); got != quiero {
				t.Errorf("clave(%q) == clave(%q) es %v y EqualFold dice %v", a, b, got, quiero)
			}
		}
	}
}

func TestGemela(t *testing.T) {
	hojas := []string{"Ventas", "Resumen", "Año 2026"}
	for _, c := range []struct {
		nombre  string
		excepto int
		quiero  int
	}{
		{"VENTAS", -1, 0},
		{"ventas", 1, 0},
		{"Ventas", 0, -1}, // la propia hoja no cuenta: cambiarle las mayúsculas vale
		{"AÑO 2026", -1, 2},
		{"Compras", -1, -1},
		{"Ventas2", -1, -1},
	} {
		if got := Gemela(hojas, c.nombre, c.excepto); got != c.quiero {
			t.Errorf("Gemela(%q, excepto %d) = %d; quiero %d", c.nombre, c.excepto, got, c.quiero)
		}
	}
	if HayGemelas(hojas) || !HayGemelas(append(slices.Clone(hojas), "RESUMEN")) || !HayGemelas([]string{"A", "A"}) {
		t.Error("HayGemelas no ve lo que hay")
	}
}

func TestLibre(t *testing.T) {
	hoja := func(n int) string { return "Hoja " + strconv.Itoa(n) }
	for _, c := range []struct {
		nombres []string
		desde   int
		quiero  string
	}{
		{nil, 1, "Hoja 1"},
		{[]string{"Hoja 1", "Hoja 2"}, 3, "Hoja 3"},
		// La web y Fyne numeraban con el número de hojas: «Hoja 3» ya estaba, en
		// minúsculas o tal cual.
		{[]string{"Hoja 1", "hoja 3"}, 3, "Hoja 4"},
		{[]string{"Hoja 3", "HOJA 4", "Hoja 5"}, 3, "Hoja 6"},
	} {
		if got := Libre(c.nombres, hoja, c.desde); got != c.quiero {
			t.Errorf("Libre(%v, desde %d) = %q; quiero %q", c.nombres, c.desde, got, c.quiero)
		}
	}
	copia := func(n int) string { return "Datos (" + strconv.Itoa(n) + ")" }
	if got := Libre([]string{"Datos", "DATOS (2)"}, copia, 2); got != "Datos (3)" {
		t.Errorf("duplicar «Datos» junto a «DATOS (2)» da %q", got)
	}
}

// TestGemelaNueva: el servidor rechaza lo que CREA una gemela —una hoja nueva o un
// renombre— y acepta el libro que ya las tenía, se edite como se edite.
func TestGemelaNueva(t *testing.T) {
	for _, c := range []struct {
		que             string
		previos, nuevos []string
		quiero          string
	}{
		{"añadir una gemela", []string{"Ventas"}, []string{"Ventas", "VENTAS"}, "VENTAS"},
		{"renombrar a una gemela", []string{"Ventas", "Otra"}, []string{"Ventas", "ventas"}, "ventas"},
		{"duplicar exacta", []string{"Hoja 1"}, []string{"Hoja 1", "Hoja 1"}, "Hoja 1"},
		{"sin modelo previo nada es de antes", nil, []string{"A", "a"}, "A"},
		{"un libro con gemelas se edita", []string{"Ventas", "VENTAS"}, []string{"Ventas", "VENTAS"}, ""},
		{"y se le añaden hojas", []string{"Ventas", "VENTAS"}, []string{"Ventas", "VENTAS", "Hoja 3"}, ""},
		{"y se reordena", []string{"Ventas", "VENTAS"}, []string{"VENTAS", "Ventas"}, ""},
		{"y se deshace la gemela", []string{"Ventas", "VENTAS"}, []string{"Ventas", "Ventas 2"}, ""},
		{"pero no se le hace otra", []string{"Ventas", "VENTAS"}, []string{"Ventas", "VENTAS", "ventas"}, "ventas"},
		{"ni se repite la otra", []string{"Ventas", "VENTAS"}, []string{"VENTAS", "VENTAS"}, "VENTAS"},
		{"exactas de antes", []string{"Hoja 1", "Hoja 1"}, []string{"Hoja 1", "Hoja 1", "Hoja 2"}, ""},
		{"sin gemelas", []string{"A"}, []string{"A", "B"}, ""},
		{"cambiar las mayúsculas de la propia", []string{"ventas"}, []string{"Ventas"}, ""},
	} {
		if got := GemelaNueva(c.previos, c.nuevos); got != c.quiero {
			t.Errorf("%s: GemelaNueva(%v, %v) = %q; quiero %q", c.que, c.previos, c.nuevos, got, c.quiero)
		}
	}
}

// TestDesgemelar: la segunda de cada grupo pasa a «X (2)» —o al primer número que no
// choque con otra hoja, también con una que viene después— y la primera se queda.
func TestDesgemelar(t *testing.T) {
	for _, c := range []struct{ nombres, quiero []string }{
		{[]string{"Ventas", "Resumen"}, []string{"Ventas", "Resumen"}},
		{[]string{"Ventas", "VENTAS"}, []string{"Ventas", "VENTAS (2)"}},
		{[]string{"Ventas", "VENTAS", "ventas"}, []string{"Ventas", "VENTAS (2)", "ventas (3)"}},
		{[]string{"Hoja1", "Hoja1"}, []string{"Hoja1", "Hoja1 (2)"}},
		{[]string{"A", "a", "A (2)"}, []string{"A", "a (3)", "A (2)"}},
	} {
		got := Desgemelar(c.nombres)
		if !slices.Equal(got, c.quiero) {
			t.Errorf("Desgemelar(%v) = %v; quiero %v", c.nombres, got, c.quiero)
		}
		if HayGemelas(got) {
			t.Errorf("Desgemelar(%v) deja gemelas: %v", c.nombres, got)
		}
	}
}
