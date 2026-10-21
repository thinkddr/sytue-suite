// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"encoding/json"
	"math"
	"os"
	"regexp"
	"slices"
	"strconv"
	"testing"
	"time"
)

// El corpus de compatibilidad contra LibreOffice (apps/frontend/packages/formula/compat):
// las MISMAS fórmulas y los MISMOS datos que contrasta el motor TypeScript, con lo que
// LibreOffice contestó. Aquí se exige lo mismo al motor de Go, con las mismas
// divergencias declaradas: si Go diverge donde TypeScript no (o al revés), falla.
//
// Es la prueba de que la web y la app nativa dicen lo mismo de las funciones que NO
// están en trazas.json —las que llegaron después de que se grabara—, y con una fuente
// que no es ninguno de los dos motores.

const dirCompat = "../../frontend/packages/formula/compat/"

type resultadoLO struct {
	Num   *float64
	Texto *string
	Bool  *bool
	Err   string
}

func (r *resultadoLO) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch x := v.(type) {
	case float64:
		r.Num = &x
	case string:
		r.Texto = &x
	case bool:
		r.Bool = &x
	case map[string]any:
		r.Err, _ = x["error"].(string)
	}
	return nil
}

type esperadoLO struct {
	Libreoffice string         `json:"libreoffice"`
	Datos       map[string]any `json:"datos"`
	// Solo365 son las funciones de Excel 365 del catálogo (compat 'excel365'): LibreOffice 7.4 no las tiene.
	Solo365 []string `json:"solo365"`
	Casos   []struct {
		F  string      `json:"f"`
		Lo resultadoLO `json:"lo"`
	} `json:"casos"`
}

var complejoLO = regexp.MustCompile(`(?i)^([-+]?[\d.]+(?:E[-+]?\d+)?)([-+][\d.]*(?:E[-+]?\d+)?)([ij])$`)

func mismoNumeroLO(a, b float64) bool {
	return math.Abs(a-b) <= math.Max(1e-12, 1e-9*math.Max(math.Abs(a), math.Abs(b)))
}

func partesComplejo(t string) (re, im float64, ok bool) {
	m := complejoLO.FindStringSubmatch(t)
	if m == nil {
		return 0, 0, false
	}
	re, _ = strconv.ParseFloat(m[1], 64)
	switch m[2] {
	case "+":
		im = 1
	case "-":
		im = -1
	default:
		im, _ = strconv.ParseFloat(m[2], 64)
	}
	return re, im, true
}

func igualAlLO(v Value, lo resultadoLO) bool {
	switch {
	case lo.Num != nil:
		n, ok := v.(float64)
		return ok && mismoNumeroLO(n, *lo.Num)
	case lo.Bool != nil:
		b, ok := v.(bool)
		return ok && b == *lo.Bool
	case lo.Texto != nil:
		s, ok := v.(string)
		if !ok {
			return false
		}
		if a, ai, oka := partesComplejo(s); oka {
			if b, bi, okb := partesComplejo(*lo.Texto); okb {
				return mismoNumeroLO(a, b) && mismoNumeroLO(ai, bi)
			}
		}
		return s == *lo.Texto
	}
	e, ok := v.(Error)
	return ok && string(e) == lo.Err
}

func sinContrasteLO(lo resultadoLO) bool {
	return lo.Err == "#NAME?" || (len(lo.Err) > 4 && lo.Err[:4] == "Err:")
}

// funcion365 es el criterio de comparar.ts: las funciones de Excel 365 no existen en
// LibreOffice 7.4.
var funcion365 = regexp.MustCompile(`([A-Z][A-Z0-9.]*)\(`)

func TestCompatibilidadContraLibreOffice(t *testing.T) {
	var esp esperadoLO
	leerJSON(t, dirCompat+"esperado-libreoffice.json", &esp)
	var div map[string]json.RawMessage
	leerJSON(t, dirCompat+"divergencias.json", &div)

	e := New(Options{
		Now:    func() time.Time { return time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC) },
		Random: func() float64 { return 0.5 },
	})
	for ref, v := range esp.Datos {
		raw := ""
		switch x := v.(type) {
		case float64:
			raw = strconv.FormatFloat(x, 'g', -1, 64)
		case bool:
			raw = strconv.FormatBool(x)
		case string:
			raw = x
		}
		if err := e.SetCell("Hoja1", ref, raw); err != nil {
			t.Fatalf("dato %s: %v", ref, err)
		}
	}
	for i, c := range esp.Casos {
		if err := e.SetCell("Hoja1", "P"+strconv.Itoa(i+1), c.F); err != nil {
			t.Fatalf("%s: %v", c.F, err)
		}
	}

	soloExcel365 := map[string]bool{}
	for _, n := range esp.Solo365 {
		soloExcel365[n] = true
	}

	var difieren []string
	iguales := 0
	for i, c := range esp.Casos {
		v, _ := e.GetValue("Hoja1", "P"+strconv.Itoa(i+1))
		if sinContrasteLO(c.Lo) || slices.ContainsFunc(funcion365.FindAllStringSubmatch(c.F, -1), func(m []string) bool { return soloExcel365[m[1]] }) {
			continue
		}
		if igualAlLO(v, c.Lo) {
			iguales++
			continue
		}
		difieren = append(difieren, c.F)
	}
	slices.Sort(difieren)
	declaradas := make([]string, 0, len(div))
	for f := range div {
		declaradas = append(declaradas, f)
	}
	slices.Sort(declaradas)
	if !slices.Equal(difieren, declaradas) {
		t.Errorf("Go difiere de LibreOffice en lo que no se declaró (o declara lo que ya no difiere)\n  difiere: %q\n  declarado: %q", difieren, declaradas)
	}
	if iguales < 400 {
		t.Errorf("solo %d fórmulas coinciden con %s", iguales, esp.Libreoffice)
	}
}

func leerJSON(t *testing.T, ruta string, dst any) {
	t.Helper()
	b, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, dst); err != nil {
		t.Fatalf("%s: %v", ruta, err)
	}
}
