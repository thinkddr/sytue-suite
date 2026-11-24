// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package widgets

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// El contrato del formato 3 lo implementan tres lectores: este paquete, la extensión de
// WidgetKit (Snapshot.swift) y el receptor de Android (WidgetSnapshot.java). Los tres
// corren los casos de testdata/contrato.tsv; aquí, además, se comprueba que los otros
// dos dicen las mismas constantes y que de verdad leen el fichero.
const (
	ficheroContrato = "testdata/contrato.tsv"
	fuenteSwift     = "../../fyne/xcode/widgets/Snapshot.swift"
	pruebaSwift     = "../../fyne/xcode/widgets/SnapshotTests.swift"
	fuenteJava      = "../../fyne/android/java/eu/sytue/nativo/WidgetSnapshot.java"
	// fuenteJavaLector es quien lee el JSON en Android: tiene que admitir `route`.
	fuenteJavaLector = "../../fyne/android/java/eu/sytue/nativo/Widgets.java"
	pruebaJava       = "../../fyne/android/prueba/eu/sytue/nativo/WidgetSnapshotPrueba.java"
)

func casosContrato(t *testing.T) [][]string {
	t.Helper()
	f, err := os.Open(ficheroContrato)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var casos [][]string
	lineas := bufio.NewScanner(f)
	for lineas.Scan() {
		if l := lineas.Text(); l != "" && !strings.HasPrefix(l, "#") {
			campos := strings.Split(l, "\t")
			for i, c := range campos {
				if c == "-" {
					campos[i] = ""
				}
			}
			casos = append(casos, campos)
		}
	}
	if err := lineas.Err(); err != nil {
		t.Fatal(err)
	}
	return casos
}

func entero(t *testing.T, s string) int64 {
	t.Helper()
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		t.Fatalf("%q no es un número: %v", s, err)
	}
	return n
}

func TestContratoComunConLosLectoresNativos(t *testing.T) {
	cuenta := map[string]int{}
	for _, c := range casosContrato(t) {
		cuenta[c[0]]++
		switch c[0] {
		case "visible":
			if len(c) != 14 {
				t.Fatalf("caso mal formado: %q", c)
			}
			e := Entorno{Epoca: c[2], Confirmada: c[3], Marca: c[4], Cerrada: c[5] == "1", Bloqueo: c[6] == "1"}
			s := Snapshot{
				Version: int(entero(t, c[7])), Product: "mail", Active: c[8] == "1", Generation: c[9],
				UpdatedAt: entero(t, c[10]), ExpiresAt: entero(t, c[11]),
				Counters: []Counter{{Kind: "unread", Value: 3}},
			}
			if got := s.Visible(e, time.Unix(entero(t, c[12]), 0)); got != (c[13] == "1") {
				t.Errorf("visible «%s»: %v, el contrato dice %s", c[1], got, c[13])
			}
		case "rancio":
			s := Snapshot{UpdatedAt: entero(t, c[2])}
			if got := s.Rancio(time.Unix(entero(t, c[3]), 0)); got != (c[4] == "1") {
				t.Errorf("rancio «%s»: %v, el contrato dice %s", c[1], got, c[4])
			}
		case "vigente":
			zona, err := time.LoadLocation(c[2])
			if err != nil {
				t.Fatal(err)
			}
			it := Item{Title: "x", StartAt: entero(t, c[3]), EndAt: entero(t, c[4]), AllDay: c[5] == "1"}
			got := len(Vigentes([]Item{it}, "calendar", time.Unix(entero(t, c[6]), 0).In(zona))) == 1
			if got != (c[7] == "1") {
				t.Errorf("vigente «%s» en %s: %v, el contrato dice %s", c[1], c[2], got, c[7])
			}
			if len(Vigentes([]Item{it}, "mail", time.Unix(entero(t, c[6]), 0))) != 1 {
				t.Errorf("vigente «%s»: fuera de Calendar no se recorta nada", c[1])
			}
		case "ruta":
			if len(c) != 4 {
				t.Fatalf("caso mal formado: %q", c)
			}
			quiero := c[3] == "1"
			if got := ValidRoute(c[2]); got != quiero {
				t.Errorf("ruta «%s» (%q): %v, el contrato dice %s", c[1], c[2], got, c[3])
			}
			// Una fila con una ruta inválida deja el snapshot entero sin enseñar, como
			// cualquier otro campo fuera de su forma.
			e := Entorno{Epoca: "e1", Confirmada: "m1", Marca: "m1"}
			s := Snapshot{
				Version: Version, Product: "notes", Active: true, Generation: "e1",
				UpdatedAt: 1792500000, ExpiresAt: 1792500120, Items: []Item{{Title: "x", Route: c[2]}},
			}
			if got := s.Visible(e, time.Unix(1792500000, 0)); got != quiero {
				t.Errorf("ruta «%s»: el snapshot visible=%v, el contrato dice %s", c[1], got, c[3])
			}
			if got := SanitizeRoute(c[2]); (got == c[2]) != quiero && c[2] != "" {
				t.Errorf("ruta «%s»: SanitizeRoute = %q", c[1], got)
			}
		default:
			t.Fatalf("tipo de caso desconocido: %q", c[0])
		}
	}
	if cuenta["visible"] < 15 || cuenta["rancio"] < 4 || cuenta["vigente"] < 10 || cuenta["ruta"] < 20 {
		t.Fatalf("el contrato se ha quedado corto: %v", cuenta)
	}
}

// TestLosLectoresNativosDicenLasMismasConstantes: 24 h, 15 min, el formato 3 y el tope
// de la ruta de una fila, escritos en el Swift y el Java que no se compilan en este
// paquete; y sus pruebas leen el contrato, no una copia de los casos.
func TestLosLectoresNativosDicenLasMismasConstantes(t *testing.T) {
	maximo, fresco := strconv.Itoa(int(MaxAge/time.Second)), strconv.Itoa(int(Fresco/time.Second))
	version, ruta := strconv.Itoa(Version), strconv.Itoa(MaxRouteBytes)
	reglas := map[string][]*regexp.Regexp{
		fuenteSwift: {
			regexp.MustCompile(`static let maxAge: Int64 = ` + maximo + `\b`),
			regexp.MustCompile(`static let fresco: Int64 = ` + fresco + `\b`),
			regexp.MustCompile(`static let version = ` + version + `\b`),
			regexp.MustCompile(`static let maxRouteBytes = ` + ruta + `\b`),
			regexp.MustCompile(`"route"`),
		},
		fuenteJava: {
			regexp.MustCompile(`public static final long MAX_AGE = ` + maximo + `;`),
			regexp.MustCompile(`public static final long FRESCO = ` + fresco + `;`),
			regexp.MustCompile(`public static final long VERSION = ` + version + `;`),
			regexp.MustCompile(`public static final int MAX_ROUTE_BYTES = ` + ruta + `;`),
		},
		fuenteJavaLector: {regexp.MustCompile(`"route"`)},
		pruebaSwift:      {regexp.MustCompile(regexp.QuoteMeta(filepath.Base(ficheroContrato)))},
		pruebaJava:       {regexp.MustCompile(regexp.QuoteMeta(filepath.Base(ficheroContrato)))},
	}
	for fichero, rs := range reglas {
		crudo, err := os.ReadFile(fichero)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rs {
			if !r.Match(crudo) {
				t.Errorf("%s no dice %s", fichero, r)
			}
		}
	}
}
