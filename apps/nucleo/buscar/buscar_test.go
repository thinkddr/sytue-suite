// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package buscar

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

// Los casos son los MISMOS que leen el test de '@sytue/i18n' y el de la función SQL:
// si uno de los tres pliega distinto, la web, Fyne y el servidor dejan de encontrar lo
// mismo con la misma búsqueda.
func TestPlegar(t *testing.T) {
	b, err := os.ReadFile("testdata/casos.json")
	if err != nil {
		t.Fatal(err)
	}
	var casos [][2]string
	if err := json.Unmarshal(b, &casos); err != nil {
		t.Fatal(err)
	}
	for _, c := range casos {
		if got := Plegar(c[0]); got != c[1] {
			t.Errorf("Plegar(%q) = %q, quiero %q", c[0], got, c[1])
		}
	}
}

func TestContiene(t *testing.T) {
	for _, c := range []struct {
		texto, aguja string
		quiero       bool
	}{
		{"Acta de la reunión", "reunion", true},
		{"Acta de la reunion", "REUNIÓN", true},
		{"Café con leche", "cafe", true},
		{"El niño pequeño", "pequeno", true},
		{"Frau Müller", "muller", true},
		{"Frau Muller", "Müller", true},
		{"Straße", "strasse", true},
		{"cualquier cosa", "", true},
		{"Café", "té", false},
	} {
		if got := Contiene(c.texto, c.aguja); got != c.quiero {
			t.Errorf("Contiene(%q, %q) = %v", c.texto, c.aguja, got)
		}
	}
}

// Los mismos casos que `rangos` de '@sytue/i18n'.
func TestRangos(t *testing.T) {
	trozos := func(texto, aguja string) []string {
		rs := []rune(texto)
		rangos := Rangos(rs, aguja)
		out := make([]string, 0, len(rangos))
		for _, r := range rangos {
			out = append(out, string(rs[r[0]:r[1]]))
		}
		return out
	}
	if got := trozos("Acta de la REUNIÓN y otra reunion", "reunion"); len(got) != 2 || got[0] != "REUNIÓN" || got[1] != "reunion" {
		t.Errorf("reunion: %q", got)
	}
	if got := trozos("café solo", "café"); len(got) != 1 || got[0] != "café" {
		t.Errorf("tilde combinada: %q", got)
	}
	if got := Rangos([]rune("Straße"), "strasse"); len(got) != 1 || got[0] != [2]int{0, 6} {
		t.Errorf("Straße: %v", got)
	}
	if got := Rangos([]rune("Straße"), "s"); len(got) != 2 || got[0] != [2]int{0, 1} || got[1] != [2]int{4, 5} {
		t.Errorf("Straße/s: %v", got)
	}
	if got := Rangos([]rune("Müller"), "u"); len(got) != 1 || got[0] != [2]int{1, 2} {
		t.Errorf("Müller/u: %v", got)
	}
	if Rangos([]rune("algo"), "") != nil {
		t.Error("aguja vacía")
	}
}

func TestTerminos(t *testing.T) {
	got := Terminos(`acta "reunión" -borrador or  cafe`)
	if strings.Join(got, "|") != "acta|reunión|cafe" {
		t.Fatalf("Terminos = %q", got)
	}
}

func TestRangosDeConsultaFundeYOrdena(t *testing.T) {
	r := []rune("Café y reunión del café")
	got := RangosDeConsulta(r, "reunion cafe caf")
	quiero := [][2]int{{0, 4}, {7, 14}, {19, 23}}
	if !slices.Equal(got, quiero) {
		t.Fatalf("RangosDeConsulta = %v, quiero %v", got, quiero)
	}
}

func TestElFragmentoRodeaLaCoincidencia(t *testing.T) {
	texto := strings.Repeat("relleno ", 60) + "aquí está la reunión de equipo " + strings.Repeat("cola ", 60)
	f := Fragmento(texto, "reunion", 80)
	if !strings.Contains(f, "reunión") {
		t.Fatalf("el fragmento no trae lo buscado: %q", f)
	}
	if !strings.HasPrefix(f, "…") || !strings.HasSuffix(f, "…") {
		t.Fatalf("faltan los puntos de corte: %q", f)
	}
	if n := len([]rune(f)); n > 82 {
		t.Fatalf("el fragmento mide %d runas", n)
	}
	// Sin coincidencia, el principio y sin «…» delante.
	if g := Fragmento(texto, "nada", 80); strings.HasPrefix(g, "…") {
		t.Fatalf("sin coincidencia debe empezar por el principio: %q", g)
	}
	// Corto, tal cual (con los blancos plegados).
	if g := Fragmento("  hola\n  mundo ", "x", 80); g != "hola mundo" {
		t.Fatalf("Fragmento corto = %q", g)
	}
}
