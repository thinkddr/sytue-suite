// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

func TestColumnLettersRoundTrip(t *testing.T) {
	cases := map[int]string{1: "A", 2: "B", 26: "Z", 27: "AA", 28: "AB", 52: "AZ", 702: "ZZ", 703: "AAA"}
	for col, want := range cases {
		if got := ColumnToLetters(col); got != want {
			t.Errorf("ColumnToLetters(%d) = %q, quiero %q", col, got, want)
		}
		if got := LettersToColumn(want); got != col {
			t.Errorf("LettersToColumn(%q) = %d, quiero %d", want, got, col)
		}
	}
}

func TestLettersToColumnRejectsBad(t *testing.T) {
	for _, bad := range []string{"", "A1", "1", "A-", " A"} {
		if got := LettersToColumn(bad); got != 0 {
			t.Errorf("LettersToColumn(%q) = %d, quiero 0", bad, got)
		}
	}
}

func TestA1RefAndParse(t *testing.T) {
	if got := A1Ref(3, 2); got != "B3" {
		t.Fatalf("A1Ref(3,2) = %q, quiero B3", got)
	}
	row, col, ok := ParseA1("AA10")
	if !ok || row != 10 || col != 27 {
		t.Fatalf("ParseA1(AA10) = %d,%d,%v", row, col, ok)
	}
	if _, _, ok := ParseA1("10"); ok {
		t.Fatal("ParseA1 sin letras debe fallar")
	}
	if _, _, ok := ParseA1("AB"); ok {
		t.Fatal("ParseA1 sin número debe fallar")
	}
}

func TestCellKeyRoundTrip(t *testing.T) {
	key := CellKey("Presupuesto", 3, 2)
	if key != "cell:Presupuesto!3:2" {
		t.Fatalf("CellKey inesperada: %q", key)
	}
	sheet, row, col, ok := ParseCellKey(key)
	if !ok || sheet != "Presupuesto" || row != 3 || col != 2 {
		t.Fatalf("ParseCellKey = %q,%d,%d,%v", sheet, row, col, ok)
	}
	if a1 := CellA1(key); a1 != "Presupuesto!B3" {
		t.Fatalf("CellA1 = %q, quiero Presupuesto!B3", a1)
	}
}

func TestParseCellKeyRejectsNonCell(t *testing.T) {
	for _, bad := range []string{"fmt:A:1:bold", "doc:theme", "cell:Hoja", "cell:Hoja!1", "nope"} {
		if _, _, _, ok := ParseCellKey(bad); ok {
			t.Errorf("ParseCellKey(%q) debería fallar", bad)
		}
	}
}

func TestCellKeyWithSheetNameContainingBang(t *testing.T) {
	// El nombre de hoja puede contener '!'; usamos el ÚLTIMO '!' como separador.
	key := CellKey("a!b", 1, 1)
	sheet, row, col, ok := ParseCellKey(key)
	if !ok || sheet != "a!b" || row != 1 || col != 1 {
		t.Fatalf("ParseCellKey con '!' en hoja = %q,%d,%d,%v", sheet, row, col, ok)
	}
}

func TestFormatAndSlideKeys(t *testing.T) {
	d := Dot{Actor: "A", Counter: 7}
	if k := FormatMarkKey(d, "bold"); k != "fmt:A:7:bold" {
		t.Fatalf("FormatMarkKey = %q", k)
	}
	if k := SlidePropKey(d, "title"); k != "slide:A:7:title" {
		t.Fatalf("SlidePropKey = %q", k)
	}
	if k := DocPropKey("theme"); k != "doc:theme" {
		t.Fatalf("DocPropKey = %q", k)
	}
}
