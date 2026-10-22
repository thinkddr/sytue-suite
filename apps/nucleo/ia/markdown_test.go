// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ia

import (
	"reflect"
	"strings"
	"testing"
)

// Los casos de `packages/ai/src/markdown.test.ts`, uno a uno.

func TestLaNegritaYLaCursivaSonTramos(t *testing.T) {
	quiero := []Trozo{{Texto: "hola "}, {Texto: "mundo", Negrita: true}, {Texto: " y "}, {Texto: "adiós", Cursiva: true}}
	if got := TrozosDe("hola **mundo** y *adiós*"); !reflect.DeepEqual(got, quiero) {
		t.Errorf("TrozosDe = %+v", got)
	}
}

func TestUnMarcadorImparSeQuedaComoTexto(t *testing.T) {
	if got := TrozosDe("2 * 3 = 6"); !reflect.DeepEqual(got, []Trozo{{Texto: "2 * 3 = 6"}}) {
		t.Errorf("TrozosDe = %+v", got)
	}
}

func tipos(bs []Bloque) []Tipo {
	r := make([]Tipo, len(bs))
	for i, b := range bs {
		r[i] = b.Tipo
	}
	return r
}

func TestLosTitulosSalenConSuNivel(t *testing.T) {
	if got := tipos(BloquesDe("# Uno\n## Dos\n### Tres")); !reflect.DeepEqual(got, []Tipo{H1, H2, H3}) {
		t.Errorf("tipos = %v", got)
	}
}

func TestUnParrafoQueEsSoloNegritaEsTitulo(t *testing.T) {
	b := BloquesDe("**Introducción**")
	if b[0].Tipo != H2 || !reflect.DeepEqual(b[0].Trozos, []Trozo{{Texto: "Introducción"}}) {
		t.Errorf("bloque = %+v", b[0])
	}
	if BloquesDe("**" + strings.Repeat("x", 120) + "**")[0].Tipo != Parrafo {
		t.Error("una frase larga en negrita tiene que seguir siendo un párrafo")
	}
}

func TestLasListasDistinguenVinetaDeNumerada(t *testing.T) {
	b := BloquesDe("- uno\n* dos\n1. tres\n2) cuatro")
	ord := make([]bool, 0, len(b))
	for _, x := range b {
		if x.Tipo != Punto {
			t.Errorf("%+v no es un punto", x)
		}
		ord = append(ord, x.Ordenada)
	}
	if !reflect.DeepEqual(ord, []bool{false, false, true, true}) {
		t.Errorf("ordenadas = %v", ord)
	}
}

func TestLasLineasEnBlancoNoSonBloques(t *testing.T) {
	if n := len(BloquesDe("uno\n\n\ndos")); n != 2 {
		t.Errorf("salen %d bloques", n)
	}
	if b := BloquesDe("| tabla | rara |"); b[0].Tipo != Parrafo || !strings.Contains(b[0].Trozos[0].Texto, "tabla") {
		t.Errorf("lo que no se reconoce se pierde: %+v", b)
	}
}

func TestElTextoPlanoConservaTodo(t *testing.T) {
	if got := TextoPlano(BloquesDe("# T\n- uno\ntexto **fuerte**")); got != "T\n• uno\ntexto fuerte" {
		t.Errorf("TextoPlano = %q", got)
	}
}

func TestElDocumentoDelInforme(t *testing.T) {
	got := tipos(BloquesDe("**Regulación más reciente**\n\n**Introducción**\nLa Unión Europea (UE) ha sido pionera.\n\n1. **Ámbito de aplicación**\n   - El GDPR se aplica a todas."))
	if !reflect.DeepEqual(got, []Tipo{H2, H2, Parrafo, Punto, Punto}) {
		t.Errorf("tipos = %v", got)
	}
}
