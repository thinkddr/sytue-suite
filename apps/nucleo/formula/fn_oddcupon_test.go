// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"fmt"
	"math"
	"testing"
)

func tfCalc(t *testing.T, f string) Value {
	t.Helper()
	e := New(Options{})
	if err := e.SetCell("Hoja1", "A1", f); err != nil {
		t.Fatal(err)
	}
	v, _ := e.GetValue("Hoja1", "A1")
	return v
}

func tfNum(t *testing.T, f string) float64 {
	t.Helper()
	n, ok := tfCalc(t, f).(float64)
	if !ok {
		t.Fatalf("%s no da un número: %v", f, tfCalc(t, f))
	}
	return n
}

func tfCasi(t *testing.T, nombre string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol*math.Max(1, math.Abs(want)) {
		t.Errorf("%s = %.12g, quería %.12g", nombre, got, want)
	}
}

func tfFecha(y, m, d int) string { return fmt.Sprintf("DATE(%d,%d,%d)", y, m, d) }

// Los mismos casos que packages/formula/src/functions/oddcupon.test.ts: los ejemplos de la
// documentación de Microsoft y las identidades con PRICE.
func TestCuponesIrregularesComoLaWeb(t *testing.T) {
	f := tfFecha(2008, 11, 11) + "," + tfFecha(2021, 3, 1) + "," + tfFecha(2008, 10, 15) + "," + tfFecha(2009, 3, 1)
	l := tfFecha(2008, 2, 7) + "," + tfFecha(2008, 6, 15) + "," + tfFecha(2007, 10, 15)
	tfCasi(t, "ODDFPRICE", tfNum(t, "=ODDFPRICE("+f+",0.0785,0.0625,100,2,1)"), 113.597717, 1e-8)
	tfCasi(t, "ODDFYIELD", tfNum(t, "=ODDFYIELD("+f+",0.0575,84.5,100,2,0)"), 0.0772, 1e-4)
	tfCasi(t, "ODDLPRICE", tfNum(t, "=ODDLPRICE("+l+",0.0375,0.0405,100,2,0)"), 99.87829, 1e-6)
	tfCasi(t, "ODDLYIELD", tfNum(t, "=ODDLYIELD("+tfFecha(2008, 4, 20)+","+tfFecha(2008, 6, 15)+","+tfFecha(2007, 12, 24)+",0.0375,99.875,100,2,0)"), 0.045192, 1e-6)

	for _, base := range []int{0, 1, 4} { // con 2 y 3 el periodo regular no mide 1: ver oddcupon.test.ts
		sv := tfFecha(2008, 11, 11) + "," + tfFecha(2021, 3, 1)
		tfCasi(t, "ODDF=PRICE", tfNum(t, fmt.Sprintf("=ODDFPRICE(%s,%s,%s,0.0785,0.0625,100,2,%d)", sv, tfFecha(2008, 9, 1), tfFecha(2009, 3, 1), base)),
			tfNum(t, fmt.Sprintf("=PRICE(%s,0.0785,0.0625,100,2,%d)", sv, base)), 1e-9)
		sl := tfFecha(2008, 2, 7) + "," + tfFecha(2008, 6, 15)
		tfCasi(t, "ODDL=PRICE", tfNum(t, fmt.Sprintf("=ODDLPRICE(%s,%s,0.0375,0.0405,100,2,%d)", sl, tfFecha(2007, 12, 15), base)),
			tfNum(t, fmt.Sprintf("=PRICE(%s,0.0375,0.0405,100,2,%d)", sl, base)), 1e-9)
	}
	for _, emision := range []string{tfFecha(2008, 10, 15), tfFecha(2008, 1, 15), tfFecha(2008, 9, 20)} {
		g := tfFecha(2008, 11, 11) + "," + tfFecha(2021, 3, 1) + "," + emision + "," + tfFecha(2009, 3, 1)
		p := tfNum(t, "=ODDFPRICE("+g+",0.0785,0.0625,100,2,1)")
		tfCasi(t, "ODDFYIELD invierte", tfNum(t, fmt.Sprintf("=ODDFYIELD(%s,0.0785,%.15g,100,2,1)", g, p)), 0.0625, 1e-9)
	}
	for _, e := range []string{
		"=ODDFPRICE(" + tfFecha(2009, 4, 1) + "," + tfFecha(2021, 3, 1) + "," + tfFecha(2008, 10, 15) + "," + tfFecha(2009, 3, 1) + ",0.05,0.05,100,2,1)",
		"=ODDFPRICE(" + tfFecha(2008, 11, 11) + "," + tfFecha(2021, 3, 1) + "," + tfFecha(2008, 10, 15) + "," + tfFecha(2009, 2, 15) + ",0.05,0.05,100,2,1)",
		"=ODDFPRICE(" + f + ",0.05,0.05,100,3,1)",
		"=ODDFYIELD(" + f + ",0.05,0,100,2,1)",
		"=ODDLYIELD(" + l + ",0.0375,0,100,2,0)",
	} {
		if v := tfCalc(t, e); v != ErrNum {
			t.Errorf("%s = %v, y la web da #NUM!", e, v)
		}
	}
	if v := tfCalc(t, "=ODDFPRICE(1,2,3)"); v != ErrValue {
		t.Errorf("pocos argumentos = %v", v)
	}
}

// BESSELK contra la serie A&S 9.6.11 en precisión arbitraria (como bessel.test.ts).
func TestBesselKAPrecisionDeDoble(t *testing.T) {
	for _, c := range [][3]float64{
		{1.5, 1, 0.2773878004568438},
		{1, 0, 0.42102443824070834},
		{1, 1, 0.60190723019723458},
		{2, 2, 0.25375975456605587},
		{0.1, 0, 2.4270690247020168},
		{0.5, 3, 62.057909529930257},
		{5, 1, 0.0040446134454521646},
		{10, 2, 2.150981700693277e-5},
		{2.5, 0, 0.062347553200366189},
		{4, 5, 0.15434254872599718},
	} {
		got := tfNum(t, fmt.Sprintf("=BESSELK(%g,%g)", c[0], c[1]))
		if math.Abs(got/c[2]-1) > 1e-13 {
			t.Errorf("BESSELK(%g,%g) = %.17g, quería %.17g", c[0], c[1], got, c[2])
		}
	}
}
