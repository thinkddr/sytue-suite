// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// testdata/trazas.json es lo que el motor TypeScript CONTESTÓ mientras corrían sus
// propios tests (scripts/grabar-formulas.ts): una traza por Engine, con sus operaciones
// en orden y, de cada una, el resultado. Este test repite cada traza en el motor de Go y
// exige la misma respuesta, el mismo azar pedido las mismas veces y el mismo reloj.
//
// Un número cuenta como IDÉNTICO si se escribe igual que en JavaScript (String(n), con
// su -0). Si no, se admite una diferencia relativa pequeña y se CUENTA aparte. No es
// semántica, son dos cosas medidas:
//
//   - La libm. El grabador es JavaScriptCore sobre glibc y Go trae la suya: seno, coseno,
//     exponencial y potencia difieren en el último bit (cos(π/2) da …766e-17 allí y
//     …757e-17 aquí). De ahí salen todas las de Bessel, que suman cientos de cosenos.
//   - La FMA. En arm64 el compilador de Go funde `a*b + c` en una instrucción que no
//     redondea el producto, y JavaScript nunca lo hace. En amd64 y en WebAssembly no hay
//     fusión, y por eso ahí la tolerancia es la estricta.
//
// Un complejo es TEXTO («6.12e-17+i»): se compara parte a parte con la misma vara, medida
// contra su módulo. Y una suma que se cancela casi a cero amplifica el último bit hasta
// donde ninguna tolerancia relativa llega: esas van por lista, con su porqué.

var toleranciaRelativa = map[bool]float64{true: 1e-12, false: 1e-9}[runtime.GOARCH == "amd64" || runtime.GOARCH == "wasm"]

// cancelaciones son resultados que son el RESIDUO de sumandos mucho mayores. Se exige
// la diferencia ABSOLUTA indicada, no una relativa.
var cancelaciones = map[string]float64{
	// XNPV evaluado en su propia TIR: sumandos de ~1e4 que se anulan hasta 3,9e-8. La
	// diferencia (9e-13) es 5e-17 de los sumandos: el último bit de cuatro potencias.
	"=XNPV(0.3733625335121688, A1:A5, B1:B5)": 1e-10,
}

type traza struct {
	Origen  string                     `json:"origen"`
	Hojas   []string                   `json:"hojas"`
	Ops     [][]json.RawMessage        `json:"ops"`
	Relojes []int64                    `json:"relojes"`
	Azares  []float64                  `json:"azares"`
	Tablas  map[string]json.RawMessage `json:"tablas"`
}

type esperado struct {
	N *string  `json:"n"`
	S *string  `json:"s"`
	B *bool    `json:"b"`
	E *string  `json:"e"`
	L []string `json:"l"`
	X bool     `json:"x"`
}

type consulta struct {
	Table  string  `json:"table"`
	Column *string `json:"column"`
	Area   string  `json:"area"`
	// El grabador es TypeScript y escribe camelCase: el tag es SU contrato.
	ThisRow bool `json:"thisRow"` //nolint:tagliatelle // lo escribe el motor TS
}

type aqui struct {
	Sheet string `json:"sheet"`
	Col   int    `json:"col"`
	Row   int    `json:"row"`
}

type tablaGrabada struct {
	q   consulta
	h   aqui
	res json.RawMessage
}

func texto(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("argumento que no es texto: %s", raw)
	}
	return s
}

// resolutor reproduce lo que DEVOLVIÓ el resolutor de tablas del test original.
func resolutor(t *testing.T, tz traza) TableResolver {
	t.Helper()
	if len(tz.Tablas) == 0 {
		return nil
	}
	var grabadas []tablaGrabada
	for k, v := range tz.Tablas {
		var par []json.RawMessage
		g := tablaGrabada{res: v}
		if json.Unmarshal([]byte(k), &par) != nil || len(par) != 2 ||
			json.Unmarshal(par[0], &g.q) != nil || json.Unmarshal(par[1], &g.h) != nil {
			t.Fatalf("%s: clave de tabla ilegible: %s", tz.Origen, k)
		}
		grabadas = append(grabadas, g)
	}
	return func(q TableQuery, here Here) (*RefRange, Error) {
		for _, g := range grabadas {
			mismaCol := (g.q.Column == nil && !q.HasColumn) || (g.q.Column != nil && q.HasColumn && *g.q.Column == q.Column)
			if g.q.Table != q.Table || !mismaCol || g.q.Area != string(q.Area) || g.q.ThisRow != q.ThisRow ||
				g.h != aqui(here) {
				continue
			}
			var r struct {
				E     string `json:"e"`
				Sheet string `json:"sheet"`
				C1    int    `json:"c1"`
				R1    int    `json:"r1"`
				C2    int    `json:"c2"`
				R2    int    `json:"r2"`
			}
			if string(g.res) == "null" || json.Unmarshal(g.res, &r) != nil {
				return nil, ""
			}
			if r.E != "" {
				return nil, Error(r.E)
			}
			return &RefRange{r.Sheet, r.C1, r.R1, r.C2, r.R2}, ""
		}
		t.Errorf("%s: el motor pidió una tabla que el original no pidió: %+v en %+v", tz.Origen, q, here)
		return nil, ""
	}
}

func cerca(a, b, escala float64) bool {
	return math.Abs(a-b) <= toleranciaRelativa*math.Max(escala, math.SmallestNonzeroFloat64)
}

// comparaTexto: dos textos distintos solo se toleran si los dos son el MISMO complejo.
func comparaTexto(got any, quiere string) (identico, tolerado bool) {
	s, ok := got.(string)
	if !ok || s == quiere {
		return ok, false
	}
	a, errA := aComplejo(s)
	b, errB := aComplejo(quiere)
	if errA != "" || errB != "" || a.suf != b.suf || !strings.ContainsAny(quiere, "ij") {
		return false, false
	}
	modulo := math.Hypot(b.re, b.im)
	return false, cerca(a.re, b.re, modulo) && cerca(a.im, b.im, modulo)
}

// compara dice si `got` es lo esperado: idéntico, o dentro de la tolerancia numérica.
func compara(got any, exp esperado, formula string) (identico, tolerado bool) {
	switch {
	case exp.X:
		return true, false
	case exp.S != nil:
		return comparaTexto(got, *exp.S)
	case exp.B != nil:
		return got == *exp.B, false
	case exp.E != nil:
		return got == Error(*exp.E), false
	case exp.L != nil:
		l, _ := got.([]string)
		return slices.Equal(l, exp.L) || (len(l) == 0 && len(exp.L) == 0), false
	case exp.N != nil:
		n, ok := got.(float64)
		if !ok {
			return false, false
		}
		escrito := jsNumero(n)
		if n == 0 && math.Signbit(n) {
			escrito = "-0"
		}
		if escrito == *exp.N {
			return true, false
		}
		quiere, err := strconv.ParseFloat(*exp.N, 64)
		if err != nil {
			return false, false // «NaN» o «Infinity»: solo valen idénticos
		}
		if absoluta, ok := cancelaciones[formula]; ok {
			return false, math.Abs(n-quiere) <= absoluta
		}
		return false, cerca(n, quiere, math.Max(math.Abs(n), math.Abs(quiere)))
	}
	return false, false
}

func describe(exp esperado) string {
	switch {
	case exp.N != nil:
		return *exp.N
	case exp.S != nil:
		return strconv.Quote(*exp.S)
	case exp.B != nil:
		return strconv.FormatBool(*exp.B)
	case exp.E != nil:
		return *exp.E
	}
	return fmt.Sprint(exp.L)
}

var reFuncion = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_.]*)\(`)

type marcador struct {
	ops, identicas, toleradas int
	fallos                    []string
	porFuncion                map[string]int
	tolFuncion                map[string]int
}

// ejecuta repite una traza y dice si salió entera idéntica y si, al menos, pasó.
func (m *marcador) ejecuta(t *testing.T, tz traza) (limpia, pasa bool) {
	t.Helper()
	relojes, azares := tz.Relojes, tz.Azares
	sobra := false
	e := New(Options{
		Sheets:       tz.Hojas,
		ResolveTable: resolutor(t, tz),
		Now: func() time.Time {
			if len(relojes) == 0 {
				sobra = true
				return time.UnixMilli(0).UTC()
			}
			ms := relojes[0]
			relojes = relojes[1:]
			return time.UnixMilli(ms).UTC()
		},
		Random: func() float64 {
			if len(azares) == 0 {
				sobra = true
				return 0
			}
			r := azares[0]
			azares = azares[1:]
			return r
		},
	})
	crudo := map[string]string{}
	limpia, pasa = true, true
	for i, op := range tz.Ops {
		var exp esperado
		if err := json.Unmarshal(op[len(op)-1], &exp); err != nil {
			t.Fatalf("%s op %d: resultado ilegible: %s", tz.Origen, i, op[len(op)-1])
		}
		args := make([]string, 0, 3)
		for _, a := range op[1 : len(op)-1] {
			args = append(args, texto(t, a))
		}
		var got any
		metodo := texto(t, op[0])
		switch metodo {
		case "setCell":
			crudo[args[0]+"!"+args[1]] = args[2]
			if err := e.SetCell(args[0], args[1], args[2]); err != nil {
				got = err
			}
		case "getValue":
			got, _ = e.GetValue(args[0], args[1])
		case "getDisplay":
			got, _ = e.GetDisplay(args[0], args[1])
		case "getRaw":
			got, _ = e.GetRaw(args[0], args[1])
		case "defineName":
			got = e.DefineName(args[0], args[1], strings.Join(args[2:], ""))
		case "deleteName":
			got = e.DeleteName(args[0])
		case "definedNameList":
			got = e.DefinedNameList()
		case "sheetNames":
			got = e.SheetNames()
		case "recalc":
			e.Recalc()
		default:
			t.Fatalf("%s: operación desconocida %q", tz.Origen, metodo)
		}
		m.ops++
		formula := ""
		if len(args) >= 2 {
			formula = crudo[args[0]+"!"+args[1]]
		}
		identico, tolerado := compara(got, exp, formula)
		switch {
		case identico:
			m.identicas++
			continue
		case tolerado:
			m.toleradas++
			for _, f := range reFuncion.FindAllStringSubmatch(formula, -1) {
				m.tolFuncion[strings.ToUpper(f[1])]++
			}
		default:
			m.fallos = append(m.fallos, fmt.Sprintf("%s op %d %s %v %q: quiere %s, da %v", tz.Origen, i, metodo, args[:min(2, len(args))], formula, describe(exp), got))
			funciones := reFuncion.FindAllStringSubmatch(formula, -1)
			if len(funciones) == 0 {
				m.porFuncion["(sin función: "+strings.SplitN(tz.Origen, ":", 2)[0]+")"]++
			}
			for _, f := range funciones {
				m.porFuncion[strings.ToUpper(f[1])]++
			}
			pasa = false
		}
		limpia = false
	}
	if sobra || len(relojes) > 0 || len(azares) > 0 {
		m.fallos = append(m.fallos, fmt.Sprintf("%s: el reloj o el azar no se pidieron las mismas veces (quedan %d y %d; pidió de más: %v)", tz.Origen, len(relojes), len(azares), sobra))
		m.porFuncion["(azar o reloj)"]++
		limpia, pasa = false, false
	}
	return limpia, pasa
}

func resumen(cuentas map[string]int) string {
	nombres := make([]string, 0, len(cuentas))
	for n := range cuentas {
		nombres = append(nombres, n)
	}
	sort.Slice(nombres, func(i, j int) bool {
		if cuentas[nombres[i]] != cuentas[nombres[j]] {
			return cuentas[nombres[i]] > cuentas[nombres[j]]
		}
		return nombres[i] < nombres[j]
	})
	var b strings.Builder
	for _, n := range nombres {
		fmt.Fprintf(&b, " %s:%d", n, cuentas[n])
	}
	return b.String()
}

func TestElMotorContestaLoMismoQueElDeLaWeb(t *testing.T) {
	datos, err := os.ReadFile("testdata/trazas.json")
	if err != nil {
		t.Fatal(err)
	}
	var trazas []traza
	if err := json.Unmarshal(datos, &trazas); err != nil {
		t.Fatal(err)
	}
	m := &marcador{porFuncion: map[string]int{}, tolFuncion: map[string]int{}}
	limpias, pasan := 0, 0
	for _, tz := range trazas {
		limpia, pasa := m.ejecuta(t, tz)
		if limpia {
			limpias++
		}
		if pasa {
			pasan++
		}
	}
	pct := func(n, de int) float64 { return 100 * float64(n) / float64(de) }
	t.Logf("%s: trazas que pasan %d de %d (%.2f %%), idénticas bit a bit %d (%.2f %%); operaciones: %d idénticas + %d dentro de %g = %d de %d (%.2f %%); funciones registradas: %d",
		runtime.GOARCH, pasan, len(trazas), pct(pasan, len(trazas)), limpias, pct(limpias, len(trazas)),
		m.identicas, m.toleradas, toleranciaRelativa, m.identicas+m.toleradas, m.ops, pct(m.identicas+m.toleradas, m.ops),
		len(FunctionNames()))
	if len(m.tolFuncion) > 0 {
		t.Logf("dentro de tolerancia, por función:%s", resumen(m.tolFuncion))
	}
	if len(m.fallos) == 0 {
		return
	}
	t.Errorf("%d operaciones NO contestan lo mismo; por función:%s", len(m.fallos), resumen(m.porFuncion))
	tope := 40
	if os.Getenv("FORMULA_TRAZAS_DETALLE") != "" {
		tope = len(m.fallos)
	}
	for _, f := range m.fallos[:min(tope, len(m.fallos))] {
		t.Log(f)
	}
}
