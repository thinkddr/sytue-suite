// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
)

// Lo que rompe una importación en silencio no es el parser de CSV, es la
// lectura de lo que hay dentro: un «1.234,56» leído como 1.23 es una factura
// cien veces más barata sin ningún error.
func TestDecimalCanonico(t *testing.T) {
	casos := []struct{ entrada, quiero string }{
		{"1.234,56", "1234.56"},
		{"1,234.56", "1234.56"},
		{"1234,56", "1234.56"},
		{"1234.56", "1234.56"},
		{"1.234.567,89", "1234567.89"},
		{"1,234,567.89", "1234567.89"},
		{"1.234.567", "1234567"},
		{"12,5", "12.5"},
		{"12", "12"},
		{"-1.234,5", "-1234.5"},
		{" 99,00 € ", "99.00"},
		{"1,234", "1.234"},
	}
	for _, c := range casos {
		if got := decimalCanonico(c.entrada); got != c.quiero {
			t.Errorf("decimalCanonico(%q) = %q, quiero %q", c.entrada, got, c.quiero)
		}
	}
}

func TestFechaISO(t *testing.T) {
	for entrada, quiero := range map[string]string{
		"2026-04-20": "2026-04-20", "20/04/2026": "2026-04-20", "20-04-2026": "2026-04-20",
		"20.04.2026": "2026-04-20", "2026/04/20": "2026-04-20",
	} {
		if got, err := fechaISO(entrada); err != nil || got != quiero {
			t.Errorf("fechaISO(%q) = %q, %v", entrada, got, err)
		}
	}
	if _, err := fechaISO("ayer"); err == nil {
		t.Error("una fecha ilegible debe fallar")
	}
}

func TestSeparadorDe(t *testing.T) {
	if s := separadorDe([]byte("nombre;correo;telefono\na;b;c\n")); s != ';' {
		t.Errorf("punto y coma: %q", s)
	}
	if s := separadorDe([]byte("nombre,correo\n")); s != ',' {
		t.Errorf("coma: %q", s)
	}
	if s := separadorDe([]byte("nombre\tcorreo\n")); s != '\t' {
		t.Errorf("tabulador: %q", s)
	}
	if s := separadorDe([]byte("nombre\n")); s != ',' {
		t.Errorf("sin separador cae a coma: %q", s)
	}
}

// La cabecera casa por clave O por rótulo traducido, y lo que no casa se
// devuelve por su nombre: es lo que ve quien importa para saber qué se perdió.
func TestCasarColumnas(t *testing.T) {
	cat, err := catalogo.Cargar()
	if err != nil {
		t.Fatal(err)
	}
	m, _ := cat.Modelo("contacto")
	vista := cat.VistaModelo(m, "es")
	cols, ignoradas := casarColumnas([]string{"Nombre", "correo", "Teléfono", "Fax", "externo_id", "Estado", "nombre"}, m, vista)
	claves := []string{}
	for _, c := range cols {
		if c.especial != "" {
			claves = append(claves, "*"+c.especial)
		} else {
			claves = append(claves, c.campo.Clave)
		}
	}
	quiero := []string{"nombre", "correo", "telefono", "*externo_id", "*estado"}
	if len(claves) != len(quiero) {
		t.Fatalf("columnas = %v, quiero %v (ignoradas %v)", claves, quiero, ignoradas)
	}
	for i := range quiero {
		if claves[i] != quiero[i] {
			t.Errorf("columna %d = %q, quiero %q", i, claves[i], quiero[i])
		}
	}
	// «Fax» no existe y el segundo «nombre» está repetido: ambos fuera.
	if len(ignoradas) != 2 || ignoradas[0] != "Fax" || ignoradas[1] != "nombre" {
		t.Errorf("ignoradas = %v", ignoradas)
	}
	// Un modelo sin `externo_id` no puede actualizar por él: la columna se ignora.
	gasto, _ := cat.Modelo("gasto")
	_, ignoradas = casarColumnas([]string{"externo_id", "Descripción"}, gasto, cat.VistaModelo(gasto, "es"))
	if len(ignoradas) != 1 || ignoradas[0] != "externo_id" {
		t.Errorf("externo_id en un modelo que no lo declara: ignoradas = %v", ignoradas)
	}
}

func TestConversorDeValores(t *testing.T) {
	cat, _ := catalogo.Cargar()
	m, _ := cat.Modelo("contacto")
	c := &conversor{vista: cat.VistaModelo(m, "es")}
	ctx := context.Background()
	campo := func(clave string) *catalogo.CampoVista {
		for i := range c.vista.Campos {
			if c.vista.Campos[i].Clave == clave {
				return &c.vista.Campos[i]
			}
		}
		t.Fatalf("sin campo %s", clave)
		return nil
	}
	// Selección por valor o por rótulo, sin distinguir mayúsculas.
	for _, entrada := range []string{"persona", "Persona", "PERSONA"} {
		if v, err := c.valor(ctx, campo("tipo"), entrada); err != nil || v != "persona" {
			t.Errorf("tipo %q = %v, %v", entrada, v, err)
		}
	}
	if _, err := c.valor(ctx, campo("tipo"), "marciano"); err == nil {
		t.Error("una opción inexistente debe fallar")
	}
	if v, err := c.valor(ctx, campo("nombre"), "  ACME  "); err != nil || v != "  ACME  " {
		t.Errorf("texto se pasa tal cual (lo recorta Validar): %v %v", v, err)
	}
	if v, err := c.valor(ctx, campo("empresa"), "5f2b3f2e-6a7c-4a1d-9d3e-1c2b3a4d5e6f"); err != nil || v != "5f2b3f2e-6a7c-4a1d-9d3e-1c2b3a4d5e6f" {
		t.Errorf("un UUID de relación pasa sin consultar: %v %v", v, err)
	}
}

func TestImporteUBL(t *testing.T) {
	for c, quiero := range map[int64]string{123456: "1234.56", 5: "0.05", -100: "-1.00", 0: "0.00"} {
		if got := importe(c); got != quiero {
			t.Errorf("importe(%d) = %q, quiero %q", c, got, quiero)
		}
	}
}
