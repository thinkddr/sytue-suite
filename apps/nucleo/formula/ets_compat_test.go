// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// El corpus del suavizado exponencial contra LibreOffice (packages/formula/compat/ets-*): las
// MISMAS fórmulas, tolerancias por familia y divergencias declaradas que exige compat/ets.test.ts
// al motor de TypeScript. Si Go se sale donde TypeScript no (o al revés), falla.

type etsEsperado struct {
	Datos map[string]float64 `json:"datos"`
	Casos []struct {
		F  string      `json:"f"`
		Lo resultadoLO `json:"lo"`
	} `json:"casos"`
	Intervalos map[string]struct {
		Media float64 `json:"media"`
	} `json:"intervalos"`
}

type etsTolerancias struct {
	Tolerancias map[string]struct {
		Tol *float64 `json:"tol"`
	} `json:"tolerancias"`
	Divergencias map[string]struct {
		Quien string `json:"quien"`
	} `json:"divergencias"`
}

var etsTipoStat = regexp.MustCompile(`\.STAT[A-Z.]*\([^,]+,[^,]+,(\d)`)

// etsFamilia es `familia` de ets-comparar.ts.
func etsFamilia(f string) string {
	switch {
	case strings.Contains(f, ".PI.") || strings.Contains(f, ".CONFINT("):
		return "intervalo"
	case strings.Contains(f, ".SEASONALITY("):
		return "estacionalidad"
	}
	m := etsTipoStat.FindStringSubmatch(f)
	if m == nil {
		return "pronostico"
	}
	switch k, _ := strconv.Atoi(m[1]); {
	case k >= 1 && k <= 3:
		return "parametro"
	case k == 8:
		return "paso"
	}
	return "error"
}

func TestSuavizadoExponencialContraLibreOffice(t *testing.T) {
	var esp etsEsperado
	leerJSON(t, dirCompat+"ets-esperado-libreoffice.json", &esp)
	var tol etsTolerancias
	leerJSON(t, dirCompat+"ets-divergencias.json", &tol)

	e := New(Options{})
	for ref, v := range esp.Datos {
		if err := e.SetCell("Hoja1", ref, strconv.FormatFloat(v, 'g', -1, 64)); err != nil {
			t.Fatal(err)
		}
	}
	for i, c := range esp.Casos {
		if err := e.SetCell("Hoja1", "P"+strconv.Itoa(i+1), c.F); err != nil {
			t.Fatalf("%s: %v", c.F, err)
		}
	}
	difieren := make([]string, 0, len(tol.Divergencias))
	dentro := 0
	for i, c := range esp.Casos {
		v, _ := e.GetValue("Hoja1", "P"+strconv.Itoa(i+1))
		fam := etsFamilia(c.F)
		lo := c.Lo
		if m, ok := esp.Intervalos[c.F]; ok && fam == "intervalo" {
			lo = resultadoLO{Num: &m.Media}
		}
		n, esNum := v.(float64)
		switch {
		case lo.Num != nil && esNum:
			limite := tol.Tolerancias[fam].Tol
			if limite == nil {
				if fam == "parametro" && (n < 0 || n > 1) {
					t.Errorf("%s = %v: fuera de [0, 1]", c.F, n)
				}
				continue
			}
			if math.Abs(n-*lo.Num)/math.Max(math.Abs(*lo.Num), 1e-9) <= *limite {
				dentro++
				continue
			}
		case lo.Num == nil && !esNum:
			if err, ok := v.(Error); ok && string(err) == lo.Err {
				dentro++
				continue
			}
		}
		difieren = append(difieren, c.F)
	}
	slices.Sort(difieren)
	declaradas := make([]string, 0, len(tol.Divergencias))
	for f := range tol.Divergencias {
		declaradas = append(declaradas, f)
	}
	slices.Sort(declaradas)
	if !slices.Equal(difieren, declaradas) {
		t.Errorf("Go se sale de la tolerancia donde no se declaró (o declara lo que ya no difiere)\n  difiere: %q\n  declarado: %q", difieren, declaradas)
	}
	if dentro < 150 {
		t.Errorf("solo %d fórmulas dentro de tolerancia", dentro)
	}
}
