// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// Autosuma, «Limpiar texto» y el CSV contra los VECTORES COMPARTIDOS con la web
// (`packages/office/test/vectores/calcus-herramientas.json`, que recorre
// `sheets-herramientas.test.ts`): el mismo caso, el mismo resultado en las dos.
const rutaHerramientas = "../../frontend/packages/office/test/vectores/calcus-herramientas.json"

type vectoresHerramientas struct {
	Autosuma []struct {
		Nombre  string
		Numeros []string
		Llenas  []string
		Sel     string
		Topes   *struct{ Cols, Filas int }
		Sale    *[]struct{ Celda, Rango string }
	}
	Formulas []struct{ Fn, Rango, Sale string }
	Texto    []struct{ Modo, Entrada, Sale string }
	CSV      []struct {
		Filas [][]string
		Sale  string
	} `json:"csv"` //nolint:tagliatelle // el nombre lo fija la web
	Nombres []struct{ Base, Hoja, Sale string }
}

func leerHerramientas(t *testing.T) vectoresHerramientas {
	t.Helper()
	datos, err := os.ReadFile(rutaHerramientas)
	if err != nil {
		t.Fatal(err)
	}
	var v vectoresHerramientas
	if err := json.Unmarshal(datos, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func rangoDe(t *testing.T, ref string) formula.Rect {
	t.Helper()
	r, ok := formula.ParseRangeRef(ref)
	if !ok {
		t.Fatalf("referencia mala en los vectores: %s", ref)
	}
	return r
}

func TestAutosumaConLosVectoresDeLaWeb(t *testing.T) {
	v := leerHerramientas(t)
	if len(v.Autosuma) == 0 {
		t.Fatal("sin casos de autosuma")
	}
	for _, caso := range v.Autosuma {
		t.Run(caso.Nombre, func(t *testing.T) {
			numeros, llenas := map[string]bool{}, map[string]bool{}
			for _, r := range caso.Numeros {
				numeros[r] = true
			}
			for _, r := range caso.Llenas {
				llenas[r] = true
			}
			cols, filas := MaxColumnas, MaxFilas
			if caso.Topes != nil {
				cols, filas = caso.Topes.Cols, caso.Topes.Filas
			}
			s := rangoDe(t, caso.Sel)
			plan, ok := planDeAutosuma(s.C1, s.R1, s.C2, s.R2,
				func(c, f int) bool { return numeros[Ref(c, f)] },
				func(c, f int) bool { return !numeros[Ref(c, f)] && !llenas[Ref(c, f)] },
				cols, filas)
			if caso.Sale == nil {
				if ok {
					t.Fatalf("sin números al lado no hay plan, y salió %+v", plan)
				}
				return
			}
			got := []struct{ Celda, Rango string }{}
			for _, p := range plan {
				got = append(got, struct{ Celda, Rango string }{Ref(p.celda.C, p.celda.F), Ref(p.c1, p.f1) + ":" + Ref(p.c2, p.f2)})
			}
			if !ok || !reflect.DeepEqual(got, *caso.Sale) {
				t.Errorf("plan %+v (%v), quería %+v", got, ok, *caso.Sale)
			}
		})
	}
	for _, f := range v.Formulas {
		r := rangoDe(t, f.Rango)
		if got := formulaDeAutosuma(f.Fn, r.C1, r.R1, r.C2, r.R2); got != f.Sale {
			t.Errorf("%s %s: %q, quería %q", f.Fn, f.Rango, got, f.Sale)
		}
	}
}

func TestLimpiarTextoConLosVectoresDeLaWeb(t *testing.T) {
	v := leerHerramientas(t)
	modos := map[string]bool{}
	for _, caso := range v.Texto {
		modos[caso.Modo] = true
		if got := transformarTextoCelda(caso.Entrada, caso.Modo); got != caso.Sale {
			t.Errorf("%s %q: %q, quería %q", caso.Modo, caso.Entrada, got, caso.Sale)
		}
	}
	if len(modos) != len(ModosTexto) {
		t.Errorf("los vectores cubren %d modos de %d", len(modos), len(ModosTexto))
	}
}

func TestCSVConLosVectoresDeLaWeb(t *testing.T) {
	v := leerHerramientas(t)
	for _, caso := range v.CSV {
		if got := csvDeFilas(caso.Filas); got != caso.Sale {
			t.Errorf("%q: %q, quería %q", caso.Filas, got, caso.Sale)
		}
	}
	for _, caso := range v.Nombres {
		if got := nombreCSV(caso.Base, caso.Hoja); got != caso.Sale {
			t.Errorf("%q: %q, quería %q", caso.Hoja, got, caso.Sale)
		}
	}
}

// En el libro de verdad: Σ escribe la fórmula y calcula, en un paso de deshacer; sin
// números al lado no escribe nada (quien llama abre la celda).
func TestAutosumaEscribeYSeDeshaceDeUnaVez(t *testing.T) {
	l := NuevoLibro()
	for f, v := range []string{"Importe", "10", "20", "12.5"} {
		l.Escribir(0, f, v)
		l.Escribir(1, f, v)
	}
	if !l.Autosuma("SUM", 0, 1, 1, 4) {
		t.Fatal("con números en el rango tiene que haber plan")
	}
	if l.Crudo(0, 4) != "=SUM(A2:A4)" || l.Crudo(1, 4) != "=SUM(B2:B4)" || l.Texto(0, 4) != "42.5" {
		t.Errorf("A5 %q B5 %q = %q", l.Crudo(0, 4), l.Crudo(1, 4), l.Texto(0, 4))
	}
	l.Deshacer()
	if l.Crudo(0, 4) != "" || l.Crudo(1, 4) != "" {
		t.Error("las dos fórmulas tienen que irse con un solo deshacer")
	}
	if l.Autosuma("SUM", 3, 3, 3, 3) {
		t.Error("una celda sin números al lado no tiene plan")
	}
}

// «Limpiar texto» toca los literales del rango y deja las fórmulas, el formato de la celda
// y lo que este lienzo no sabría reescribir.
func TestLimpiarTextoSoloTocaLiterales(t *testing.T) {
	l := NuevoLibro()
	l.CargarEn(0, 0, 0, "<b>  ana   garcía </b>")
	l.CargarEn(0, 0, 1, "=UPPER(&quot; x &quot;)")
	l.CargarEn(0, 0, 2, "caf&eacute; ")
	l.CargarEn(0, 1, 0, "  fuera  ")
	if !l.LimpiarTexto("trim", 0, 0, 0, 2) {
		t.Fatal("había un literal que recortar")
	}
	if got := l.HTML(0, 0); got != "<b>ana garcía</b>" {
		t.Errorf("el literal recortado conserva su negrita: %q", got)
	}
	if l.HTML(0, 1) != "=UPPER(&quot; x &quot;)" || l.HTML(0, 2) != "caf&eacute; " || l.HTML(1, 0) != "  fuera  " {
		t.Error("la fórmula, la entidad desconocida y lo de fuera del rango no se tocan")
	}
	l.LimpiarTexto("title", 0, 0, 0, 0)
	if l.Crudo(0, 0) != "Ana García" {
		t.Errorf("tipo título: %q", l.Crudo(0, 0))
	}
}

// El CSV de la hoja activa: desde A1, lo que se ve y con la marca de UTF-8 delante.
func TestElCSVEsLaHojaComoSeVe(t *testing.T) {
	l := NuevoLibro()
	l.Escribir(1, 1, "Año")
	l.Escribir(2, 1, "=1+1")
	l.Escribir(1, 2, "a,b")
	if got := string(l.CSV()); got != BOMUTF8+",,\r\n,Año,2\r\n,\"a,b\",\r\n" {
		t.Errorf("CSV %q", got)
	}
	if got := string(NuevoLibro().CSV()); got != BOMUTF8 {
		t.Errorf("una hoja vacía es solo la marca: %q", got)
	}
}

// Los atajos de la rejilla que acompañan a estas herramientas: Alt+= suma, Ctrl+AvPág cambia
// de hoja y Control+Mayús+cifra pone un formato de número.
func TestAtajosDeAutosumaHojasYFormatos(t *testing.T) {
	lz := libroDePrueba(t, `{"active":0,"sheets":[{"name":"A","cells":{"A1":"1","A2":"2"}},{"name":"B","cells":{}}]}`)
	r := lz.Rejilla
	r.Seleccionar(0, 2, false)
	r.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyEqual, Modifier: fyne.KeyModifierAlt})
	if got := lz.Libro.Crudo(0, 2); got != "=SUM(A1:A2)" {
		t.Errorf("Alt+= escribe %q", got)
	}
	r.Seleccionar(0, 0, false)
	r.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.Key5, Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift})
	if got := lz.Libro.h().formatos[Celda{}].ID; got != "percent" {
		t.Errorf("Control+Mayús+5 pone %q", got)
	}
	r.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyPageDown, Modifier: fyne.KeyModifierShortcutDefault})
	if lz.Libro.Activa() != 1 {
		t.Errorf("Ctrl+AvPág lleva a la hoja %d", lz.Libro.Activa())
	}
	r.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyPageDown, Modifier: fyne.KeyModifierShortcutDefault})
	if lz.Libro.Activa() != 1 {
		t.Error("en la última hoja Ctrl+AvPág no da la vuelta")
	}
	r.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyPageUp, Modifier: fyne.KeyModifierShortcutDefault})
	if lz.Libro.Activa() != 0 {
		t.Errorf("Ctrl+RePág vuelve a la hoja %d", lz.Libro.Activa())
	}
	// Ctrl+Mayús+L, el del registro, pone y quita el autofiltro.
	r.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyL, Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift})
	if !lz.Estado().Autofiltro {
		t.Error("Ctrl+Mayús+L no puso el autofiltro")
	}
}
