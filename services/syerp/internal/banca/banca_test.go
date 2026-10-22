// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package banca

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
)

// n43 arma un extracto con las posiciones del Cuaderno 43, en ISO-8859-1 como lo
// escriben los bancos.
func n43(finalOK bool) string {
	var b strings.Builder
	linea := func(s string) { b.WriteString(fmt.Sprintf("%-80s", s) + "\r\n") }
	// 11: entidad, oficina, cuenta, desde, hasta, clave, saldo inicial, divisa, modalidad, titular.
	linea("11" + "2100" + "0001" + "0123456789" + "260901" + "260930" + "2" + "00000000100000" + "978" + "3" + "SUMINISTROS ACME SL")
	// 22: libre, oficina, fecha, valor, concepto común, propio, clave, importe, documento, ref1, ref2.
	linea("22" + "    " + "0001" + "260910" + "260910" + "04" + "000" + "2" + "00000000012100" + "0000000000" + "000000000000" + "FV-2026-00042")
	linea("23" + "01" + fmt.Sprintf("%-38s", "TRANSF DE BOLT ESPA\xd1A SL") + fmt.Sprintf("%-38s", "PAGO FACTURA"))
	linea("22" + "    " + "0001" + "260915" + "260915" + "17" + "000" + "1" + "00000000000500" + "0000000000" + "000000000000" + "")
	linea("23" + "01" + fmt.Sprintf("%-38s", "COMISION MANTENIMIENTO") + fmt.Sprintf("%-38s", ""))
	final := "00000000111600"
	if !finalOK {
		final = "00000000111601"
	}
	// 33: nº debe, total debe, nº haber, total haber, clave saldo final, saldo final, divisa.
	linea("33" + "2100" + "0001" + "0123456789" + "00001" + "00000000000500" + "00001" + "00000000012100" + "2" + final + "978")
	linea("88" + "999999999999999999" + "000006")
	return b.String()
}

func TestLeerN43(t *testing.T) {
	t.Parallel()
	if !EsN43([]byte(n43(true))) || EsN43([]byte("fecha;concepto;importe\n")) {
		t.Fatal("EsN43 no distingue un Norma 43 de un CSV")
	}
	cuentas, err := LeerN43(strings.NewReader(n43(true)))
	if err != nil {
		t.Fatal(err)
	}
	c := cuentas[0]
	if len(cuentas) != 1 || c.Numero != "0123456789" || c.SaldoInicial != 100000 || c.SaldoFinal != 111600 || c.Titular != "SUMINISTROS ACME SL" {
		t.Fatalf("cuenta: %+v", c)
	}
	cobro, comision := c.Movimientos[0], c.Movimientos[1]
	if cobro.Centimos != 12100 || cobro.Referencia2 != "FV-2026-00042" || cobro.Descripcion != "TRANSF DE BOLT ESPAÑA SL PAGO FACTURA" {
		t.Fatalf("un abono entra en positivo y la Ñ se lee bien: %+v", cobro)
	}
	if comision.Centimos != -500 || !cobro.Fecha.Equal(time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("un cargo sale en negativo: %+v", comision)
	}
	if c.Nombre() != "CaixaBank ··6789" || cobro.Texto() != "TRANSF DE BOLT ESPAÑA SL PAGO FACTURA · FV-2026-00042" {
		t.Fatalf("nombre %q, texto %q", c.Nombre(), cobro.Texto())
	}
	// Dos comisiones idénticas el mismo día son dos movimientos, y reimportar da
	// las mismas huellas.
	c.Movimientos = append(c.Movimientos, comision)
	h1, h2 := Huellas(c), Huellas(c)
	if h1[1] == h1[2] || h1[2] != h2[2] || !strings.HasPrefix(h1[0], "n43:210000010123456789:") {
		t.Fatalf("huellas: %v", h1)
	}
	if _, err := LeerN43(strings.NewReader(n43(false))); !errors.Is(err, ErrN43) {
		t.Fatalf("un saldo final que no cuadra se rechaza: %v", err)
	}
	cortado := strings.Join(strings.SplitAfter(n43(true), "\n")[:4], "")
	if _, err := LeerN43(strings.NewReader(cortado)); !errors.Is(err, ErrN43) {
		t.Fatalf("un fichero cortado se rechaza: %v", err)
	}
}

func TestEmparejar(t *testing.T) {
	t.Parallel()
	dia := func(d int) time.Time { return time.Date(2026, 9, d, 0, 0, 0, 0, time.UTC) }
	acme := Pendiente{ID: "a", Ref: "FV-2026-00042", Tercero: "Bolt España, S.L.", NIF: "B12345678", Fecha: dia(1), Vencimiento: dia(30), Centimos: 12100}
	otra := Pendiente{ID: "b", Ref: "FV-2026-00043", Tercero: "Cliente Sin Pistas", Fecha: dia(2), Vencimiento: dia(30), Centimos: 12100}
	mov := Movimiento{Fecha: dia(10), Centimos: 12100, Descripcion: "TRANSF DE BOLT ESPAÑA SL", Referencia2: "FV 2026 00042"}

	if p := Emparejar(mov, []Pendiente{acme, otra}); p.Automatica == nil || p.Automatica.ID != "a" || len(p.Candidatas) != 2 {
		t.Fatalf("el número de factura en el concepto la casa sola: %+v", p)
	}
	sinPistas := Movimiento{Fecha: dia(10), Centimos: 12100, Descripcion: "TRANSFERENCIA"}
	if p := Emparejar(sinPistas, []Pendiente{acme, otra}); p.Automatica != nil || len(p.Candidatas) != 2 {
		t.Fatalf("dos facturas iguales sin pistas no se casan a ciegas: %+v", p)
	}
	if p := Emparejar(sinPistas, []Pendiente{acme}); p.Automatica != nil || len(p.Candidatas) != 1 {
		t.Fatalf("solo el importe PROPONE, no concilia: %+v", p)
	}
	porNombre := Movimiento{Fecha: dia(10), Centimos: -12100, Descripcion: "RECIBO BOLT"}
	if p := Emparejar(porNombre, []Pendiente{acme}); p.Automatica == nil {
		t.Fatalf("el nombre del tercero confirma el único candidato: %+v", p)
	}
	fuera := Movimiento{Fecha: time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC), Centimos: 12100, Referencia2: "FV-2026-00042"}
	if p := Emparejar(fuera, []Pendiente{acme}); len(p.Candidatas) != 0 {
		t.Fatalf("cinco meses tras el vencimiento ya no es esa factura: %+v", p)
	}
	if p := Emparejar(Movimiento{Fecha: dia(10), Centimos: 12000, Referencia2: "FV-2026-00042"}, []Pendiente{acme}); len(p.Candidatas) != 0 {
		t.Fatal("un importe distinto no es la factura, aunque la nombre")
	}
}

func TestMotivos(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		mov              Movimiento
		evidente, sugido string
	}{
		{Movimiento{ConceptoComun: "17", Centimos: -500, Descripcion: "COMISION MANTENIMIENTO"}, "comision_bancaria", "comision_bancaria"},
		{Movimiento{ConceptoComun: "03", Centimos: -120000, Descripcion: "AGENCIA TRIBUTARIA MOD 303"}, "impuestos", "impuestos"},
		{Movimiento{ConceptoComun: "03", Centimos: -30000, Descripcion: "TGSS COTIZACION"}, "seguridad_social", "seguridad_social"},
		{Movimiento{ConceptoComun: "04", Centimos: -150000, Descripcion: "TRASPASO A CUENTA 1234"}, "", "traspaso"},
		{Movimiento{ConceptoComun: "04", Centimos: -9000, Descripcion: "BIZUM A PEPE"}, "", ""},
	} {
		if got := MotivoEvidente(c.mov); got != c.evidente {
			t.Errorf("MotivoEvidente(%q) = %q, quiero %q", c.mov.Descripcion, got, c.evidente)
		}
		if got := MotivoProbable(c.mov); got != c.sugido {
			t.Errorf("MotivoProbable(%q) = %q, quiero %q", c.mov.Descripcion, got, c.sugido)
		}
	}
	if cuenta, _ := CuentaDeMotivo("seguridad_social", false, true); cuenta != "642000" {
		t.Errorf("la cuota de un autónomo es gasto suyo (642): %s", cuenta)
	}
	if cuenta, _ := CuentaDeMotivo("impuestos", true, false); cuenta != "470000" {
		t.Errorf("una devolución de Hacienda cancela lo que nos debe (470): %s", cuenta)
	}
	// Los motivos son las opciones del campo `motivo` de la línea de extracto: si
	// divergen, la ficha enseñaría un valor que el banco no sabe asentar.
	cat, err := catalogo.Cargar()
	if err != nil {
		t.Fatal(err)
	}
	m, _ := cat.Modelo("linea_extracto")
	for _, f := range m.Campos {
		if f.Clave == "motivo" && !slices.Equal(f.Opciones, Motivos) {
			t.Errorf("opciones del catálogo %v ≠ Motivos %v", f.Opciones, Motivos)
		}
	}
	for _, motivo := range Motivos {
		if _, ok := CuentaDeMotivo(motivo, false, false); !ok {
			t.Errorf("motivo %q sin cuenta", motivo)
		}
	}
}
