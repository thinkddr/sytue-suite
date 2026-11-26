// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package dominio

import (
	"strings"
	"testing"
)

func TestValoresConFormato(t *testing.T) {
	casos := []struct {
		kind Tipo
		raw  string
		want string // cadena vacía = error; "null" = vacío
	}{
		{TipoURL, `"https://Sytue.EU/a?b=1"`, `"https://sytue.eu/a?b=1"`},
		{TipoURL, `"sytue.eu/precios"`, `"https://sytue.eu/precios"`},
		{TipoURL, `"  "`, `null`},
		{TipoURL, `"javascript:alert(1)"`, ``},
		{TipoURL, `"ftp://sytue.eu"`, ``},
		{TipoURL, `"https://user:pw@sytue.eu"`, ``},
		{TipoURL, `"https://"`, ``},
		{TipoURL, `"https://a b.eu"`, ``},
		{TipoURL, `"https://sytue.eu/` + strings.Repeat("x", MaxLongURL) + `"`, ``},
		{TipoURL, `42`, ``},
		{TipoCorreo, `"Ana.Ruiz@Ejemplo.ES"`, `"Ana.Ruiz@ejemplo.es"`},
		{TipoCorreo, `"Ana <ana@ejemplo.es>"`, ``},
		{TipoCorreo, `"ana@localhost"`, ``},
		{TipoCorreo, `"ana"`, ``},
		{TipoCorreo, `"a@b.es, c@d.es"`, ``},
		{TipoTelefono, `"+34  600 12 34 56"`, `"+34 600 12 34 56"`},
		{TipoTelefono, `"(91) 555-12-12"`, `"(91) 555-12-12"`},
		{TipoTelefono, `"12"`, ``},
		{TipoTelefono, `"600+123"`, ``},
		{TipoTelefono, `"llámame"`, ``},
		{TipoMoneda, `12.345`, `12.35`},
		{TipoMoneda, `-3`, `-3`},
		{TipoMoneda, `"12"`, ``},
		{TipoMoneda, `1e300`, ``},
		{TipoValoracion, `4`, `4`},
		{TipoValoracion, `0`, `null`},
		{TipoValoracion, `6`, ``},
		{TipoValoracion, `-1`, ``},
		{TipoValoracion, `2.5`, ``},
		{TipoValoracion, `"3"`, ``},
	}
	for _, c := range casos {
		got, err := ValidarValor(Campo{Name: "c", Kind: c.kind}, []byte(c.raw))
		switch {
		case c.want == "" && err == nil:
			t.Errorf("%s %s: aceptado como %s, se esperaba error", c.kind, c.raw, got)
		case c.want != "" && err != nil:
			t.Errorf("%s %s: error %v", c.kind, c.raw, err)
		case c.want == "null" && got != nil:
			t.Errorf("%s %s: %s, se esperaba vacío", c.kind, c.raw, got)
		case c.want != "" && c.want != "null" && string(got) != c.want:
			t.Errorf("%s %s: %s, se esperaba %s", c.kind, c.raw, got, c.want)
		}
	}
}

func TestEntradaCampoDeMoneda(t *testing.T) {
	c, err := ValidarEntradaCampo(EntradaCampo{Name: "Precio", Kind: TipoMoneda})
	if err != nil || c.Currency != MonedaPorDefecto {
		t.Fatalf("sin moneda: %+v %v; se esperaba %s", c, err, MonedaPorDefecto)
	}
	if c, err = ValidarEntradaCampo(EntradaCampo{Name: "Precio", Kind: TipoMoneda, Currency: " usd "}); err != nil || c.Currency != "USD" {
		t.Fatalf("usd: %+v %v", c, err)
	}
	for _, malo := range []string{"EURO", "E1R", "€"} {
		if _, err := ValidarEntradaCampo(EntradaCampo{Name: "Precio", Kind: TipoMoneda, Currency: malo}); err == nil {
			t.Errorf("moneda %q aceptada", malo)
		}
	}
	// La moneda no se cuela en otros tipos.
	if c, _ := ValidarEntradaCampo(EntradaCampo{Name: "Web", Kind: TipoURL, Currency: "USD"}); c.Currency != "" {
		t.Errorf("una URL con moneda: %+v", c)
	}
	for _, k := range []Tipo{TipoURL, TipoCorreo, TipoTelefono, TipoValoracion} {
		if _, err := ValidarEntradaCampo(EntradaCampo{Name: "x", Kind: k}); err != nil {
			t.Errorf("%s: %v", k, err)
		}
	}
}

// Filtros, orden, exportación y conector ven estos tipos como el número o el texto que
// guardan; así los operadores de vista ya valen para ellos.
func TestEfectivoDeLosTiposConFormato(t *testing.T) {
	for k, want := range map[Tipo]Tipo{TipoMoneda: TipoNumero, TipoValoracion: TipoNumero, TipoURL: TipoTexto, TipoCorreo: TipoTexto, TipoTelefono: TipoTexto} {
		if got := (Campo{Kind: k}).Efectivo().Kind; got != want {
			t.Errorf("%s: %s, se esperaba %s", k, got, want)
		}
	}
	campos := []Campo{{ID: "p", Kind: TipoMoneda}}
	if _, err := ValidarConfig(VistaRejilla, ConfigVista{Filters: []Filtro{{Field: "p", Op: "gt", Value: []byte(`10`)}}}, campos); err != nil {
		t.Errorf("filtro > sobre moneda: %v", err)
	}
}
