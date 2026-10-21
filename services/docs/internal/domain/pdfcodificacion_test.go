// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestTextoDeNombreGlifo(t *testing.T) {
	casos := map[string]string{
		"f_f_i": "ffi", "f_t": "ft", "a.sc": "a", "one.oldstyle": "1", "scedilla": "ş",
		"Idotaccent": "İ", "Gcommaaccent": "Ģ", "ohungarumlaut": "ő", "uni00660069": "fi",
		"u1F600": "😀", "eacute": "é", "fi": "ﬁ",
	}
	for n, quiero := range casos {
		if got, ok := textoDeNombreGlifo(n); !ok || got != quiero {
			t.Errorf("%s = %q, %v; quería %q", n, got, ok, quiero)
		}
	}
	if _, ok := textoDeNombreGlifo("g5167"); ok {
		t.Error("g5167 no es un nombre de Adobe")
	}
}

func textoSimple(t *testing.T, contenido string, objs ...string) string {
	t.Helper()
	var b strings.Builder
	for _, f := range fragmentosDe(t, contenido, helvetica, objs...) {
		b.WriteString(f.Texto)
	}
	return b.String()
}

// /MacRomanEncoding se leía como WinAnsi: el apóstrofo (0xD5) salía «Õ» y la
// «é» (0x8E) salía «Ž».
func TestCodificacionMacRoman(t *testing.T) {
	f := `<< /Type /Font /Subtype /TrueType /BaseFont /Arial /Encoding /MacRomanEncoding >>`
	if got := textoSimple(t, `BT /F1 12 Tf 10 50 Td (it\325s caf\216) Tj ET`, f); got != "it’s café" {
		t.Fatalf("texto = %q, quería «it’s café»", got)
	}
}

// Una Type 1 sin /Encoding usa la estándar: 0xAE es la ligadura «fi».
func TestType1SinEncodingUsaLaEstandar(t *testing.T) {
	f := `<< /Type /Font /Subtype /Type1 /BaseFont /Times-Roman >>`
	if got := textoSimple(t, `BT /F1 12 Tf 10 50 Td (\256eld) Tj ET`, f); got != "ﬁeld" {
		t.Fatalf("texto = %q, quería «ﬁeld»", got)
	}
}

func TestZapfDingbats(t *testing.T) {
	f := `<< /Type /Font /Subtype /Type1 /BaseFont /ZapfDingbats >>`
	if got := textoSimple(t, `BT /F1 12 Tf 10 50 Td (\254\266) Tj ET`, f); got != "①❶" {
		t.Fatalf("texto = %q, quería «①❶»", got)
	}
}

// Sin /Encoding, una Type 1 incrustada usa la codificación de su programa: la
// ligadura del código 12 salía como un control.
func TestType1IncrustadaUsaSuCodificacion(t *testing.T) {
	prog, len1 := type1DePrueba(t)
	extra := []byte("dup 12 /fi put\n")
	i := bytes.Index(prog, []byte("dup 65 /A put\n"))
	prog = append(append(append([]byte{}, prog[:i]...), extra...), prog[i:]...)
	len1 += len(extra)
	fuente := `<< /Type /Font /Subtype /Type1 /BaseFont /ABCDEF+Prueba /FontDescriptor 6 0 R >>`
	desc := `<< /Type /FontDescriptor /FontName /ABCDEF+Prueba /Flags 4 /FontFile 7 0 R >>`
	flujo := fmt.Sprintf("<< /Length %d /Length1 %d /Length2 %d /Length3 0 >>\nstream\n%s\nendstream",
		len(prog), len1, len(prog)-len1, prog)
	if got := textoSimple(t, `BT /F1 12 Tf 10 50 Td (\014A) Tj ET`, fuente, desc, flujo); got != "ﬁA" {
		t.Fatalf("texto = %q, quería «ﬁA»", got)
	}
}

// Los nombres de /Differences fuera de la lista corta: ligaduras con «_» y
// letras de Europa central.
func TestDifferencesConNombresDeAdobe(t *testing.T) {
	f := `<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding << /Differences [1 /f_t /scedilla /ohungarumlaut] >> >>`
	if got := textoSimple(t, `BT /F1 12 Tf 10 50 Td (\001\002\003) Tj ET`, f); got != "ftşő" {
		t.Fatalf("texto = %q, quería «ftşő»", got)
	}
}
