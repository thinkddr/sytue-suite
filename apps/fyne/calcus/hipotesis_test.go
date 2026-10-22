// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula/analisis"
)

// libroHipotesis: un préstamo (cuota en B4) y una parábola (C2 = (A2−2)²+1).
const libroHipotesis = `{"active":0,"sheets":[{"name":"H","cells":{"A1":"100000","A2":"5","A3":"240",` +
	`"B1":"0.004","B4":"=-PMT(B1,A3,A1)","C2":"=(A2-2)^2+1"},` +
	`"scenarios":[{"name":"Base","values":{"B1":"0.004"}},{"name":"Alza","values":{"B1":"0.006"}}]}]}`

func lienzoHipotesis(t *testing.T) *Lienzo {
	t.Helper()
	lz := NuevoLienzo(NuevoLibro())
	lz.T = nil
	if !lz.Cargar(libroHipotesis) {
		t.Fatal("un libro con escenarios se abre en solo lectura")
	}
	return lz
}

// TestBuscarObjetivoYElSolverEscribenLoHallado: las sondas tantean sin tocar el libro,
// y aplicar escribe el número en la variable conservando su formato.
func TestBuscarObjetivoYElSolverEscribenLoHallado(t *testing.T) {
	lz := lienzoHipotesis(t)
	l := lz.Libro
	f, actual, motivo := l.SondaObjetivo("B4", "B1")
	if motivo != "" || actual != 0.004 {
		t.Fatalf("la sonda dice %q con la variable en %v", motivo, actual)
	}
	r := analisis.BuscarObjetivo(f, 700, analisis.OpcionesObjetivo{X0: actual})
	if !r.Resuelto || l.Crudo(1, 0) != "0.004" {
		t.Fatalf("no se resuelve (%+v) o la sonda toca el libro (%q)", r, l.Crudo(1, 0))
	}
	b, _ := json.Marshal(map[string]any{"cell": "B1", "value": r.Valor})
	lz.Mandar(CmdObjetivo, string(b))
	if cuota, ok := l.Valor(1, 3).(float64); !ok || math.Abs(cuota-700) > 1e-6 {
		t.Errorf("tras aplicar, la cuota es %v", l.Valor(1, 3))
	}
	for _, c := range []struct{ obj, vari, motivo string }{
		{"A1", "B1", "target-not-formula"}, {"B4", "B4", "var-is-formula"}, {"B4:B5", "B1", "bad-target"}, {"B4", "x", "bad-var"},
	} {
		if _, _, m := l.SondaObjetivo(c.obj, c.vari); m != c.motivo {
			t.Errorf("%s/%s dice %q y no %q", c.obj, c.vari, m, c.motivo)
		}
	}
	evaluar, x0, motivo := l.SondaSolver("C2", []string{"A2"}, nil)
	if motivo != "" {
		t.Fatal(motivo)
	}
	s := analisis.Resolver(evaluar, x0, analisis.MetaSolver{Tipo: "min"}, nil)
	if !s.Factible || math.Abs(s.Valores[0]-2) > 1e-3 {
		t.Fatalf("el mínimo de la parábola sale en %v", s.Valores)
	}
	b, _ = json.Marshal(map[string]any{"values": map[string]float64{"A2": s.Valores[0]}})
	lz.Mandar(CmdSolver, string(b))
	if v, _ := l.Valor(2, 1).(float64); math.Abs(v-1) > 1e-6 {
		t.Errorf("tras aplicar, C2 vale %v", l.Valor(2, 1))
	}
	if _, _, m := l.SondaSolver("C2", []string{"A2"}, []RestriccionTexto{{Izq: "A2", Rel: "lte", Der: "A1:A2"}}); m != "bad-constraint" {
		t.Errorf("un rango como límite dice %q", m)
	}
}

// TestLaTablaDeDatosYLosEscenarios: la tabla de una variable rellena la columna con la
// cuota de cada interés; los escenarios se aplican, se recapturan, se quitan, salen
// igual al guardar y el informe compara la cuota.
func TestLaTablaDeDatosYLosEscenarios(t *testing.T) {
	lz := lienzoHipotesis(t)
	l := lz.Libro
	for f, v := range []string{"0.003", "0.004", "0.005"} {
		l.CargarEn(0, 4, f+1, v) // E2:E4
	}
	l.CargarEn(0, 5, 0, "=B4") // F1
	lz.Mandar(CmdTablaDatos, `{"range":"E1:F4","colInput":"B1"}`)
	if l.Crudo(5, 2) != l.Texto(1, 3) {
		t.Errorf("con el 0,004 de B1, la tabla da %q y la cuota es %q", l.Crudo(5, 2), l.Texto(1, 3))
	}
	if l.Crudo(5, 1) == l.Crudo(5, 3) || l.Crudo(5, 1) == "" {
		t.Errorf("la tabla no cambia con el interés: %q y %q", l.Crudo(5, 1), l.Crudo(5, 3))
	}
	lz.Mandar(CmdEscAplicar, "Alza")
	if l.Crudo(1, 0) != "0.006" {
		t.Fatalf("aplicar «Alza» deja B1 en %q", l.Crudo(1, 0))
	}
	lz.Mandar(CmdEscAnadir, `{"name":"Baja","range":"B1"}`)
	lz.Mandar(CmdEscAnadir, `{"name":"Baja","range":"B1"}`) // repetido: no entra
	if n := len(l.Escenarios()); n != 3 {
		t.Fatalf("hay %d escenarios", n)
	}
	lz.Mandar(CmdEscQuitar, "Baja")
	lz.Mandar(CmdEscInforme, `{"target":"H1","results":["B4"]}`)
	for _, c := range []struct {
		col, fila int
		quiero    string
	}{
		{7, 0, "<b>sheet.scenario.report</b>"},
		{8, 0, "<b>sheet.scenario.current</b>"},
		{9, 0, "<b>Base</b>"},
		{10, 0, "<b>Alza</b>"},
		{7, 1, "<b>sheet.scenario.changingCells</b>"},
		{7, 2, "<b>B1</b>"},
		{9, 2, "0.004"},
		{10, 2, "0.006"},
		{7, 3, "<b>sheet.scenario.resultCellsLabel</b>"},
		{7, 4, "<b>B4</b>"},
	} {
		if got := l.HTML(c.col, c.fila); got != c.quiero {
			t.Errorf("%s es %q y no %q", Ref(c.col, c.fila), got, c.quiero)
		}
	}
	if l.Crudo(9, 4) == l.Crudo(10, 4) || l.Crudo(9, 4) == "" {
		t.Errorf("el informe no distingue la cuota de los dos escenarios: %q y %q", l.Crudo(9, 4), l.Crudo(10, 4))
	}
	otro := NuevoLienzo(NuevoLibro())
	if !otro.Cargar(lz.Serializar()) || !reflect.DeepEqual(otro.Libro.Escenarios(), l.Escenarios()) {
		t.Error("los escenarios no salen igual que entran")
	}
}

// TestLosCuatroEmergentesDeHipotesis: el grupo abre sus cuatro paneles, y el de Buscar
// objetivo busca y aplica. Con `SYTUE_CAPTURA` deja el de Buscar objetivo resuelto a
// ancho de escritorio y de móvil.
func TestLosCuatroEmergentesDeHipotesis(t *testing.T) {
	for _, tam := range []fyne.Size{fyne.NewSize(1280, 800), fyne.NewSize(360, 740)} {
		lz := lienzoHipotesis(t)
		lz.Rejilla.sel = Celda{C: 1, F: 3}
		ctx := &editor.Contexto{Tipo: editor.Hoja, Lienzo: lz}
		cuerpo := lz.cuerpoFormulas(ctx)
		// Taxonomía común con la web: el análisis de hipótesis ya no vive en «Datos».
		if botonDeCinta(lz.cuerpoDatos(ctx), "data.solver") != nil {
			t.Error("Datos conserva el análisis de hipótesis, que es de Fórmulas")
		}
		w := test.NewTempWindow(t, container.NewBorder(cuerpo, nil, nil, nil, lz))
		w.Resize(tam)
		for _, clave := range []string{"data.solver", "data.dataTable", "data.scenarios"} {
			b := botonDeCinta(cuerpo, clave)
			if b == nil {
				t.Fatalf("Fórmulas no tiene %s", clave)
			}
			b.Tapped(nil)
			if w.Canvas().Overlays().Top() == nil {
				t.Errorf("%s no abre su panel", clave)
			}
			for w.Canvas().Overlays().Top() != nil {
				w.Canvas().Overlays().Remove(w.Canvas().Overlays().Top())
			}
		}
		botonDeCinta(cuerpo, "data.goalSeek").Tapped(nil)
		entradas := func() []*ui.Entrada {
			var out []*ui.Entrada
			for _, o := range todos(w.Canvas().Overlays().Top()) {
				if e, ok := o.(*ui.Entrada); ok {
					out = append(out, e)
				}
			}
			return out
		}
		es := entradas()
		if len(es) != 3 || es[0].Texto() != "B4" {
			t.Fatalf("el panel tiene %d campos", len(es))
		}
		es[1].PonerTexto("700")
		es[2].PonerTexto("B1")
		pulsar := func(rotulo string) {
			for _, o := range todos(w.Canvas().Overlays().Top()) {
				if b, ok := o.(*editor.BotonPequeno); ok && b.Texto == rotulo && !b.Inerte {
					b.Tapped(nil)
					return
				}
			}
			t.Fatalf("no hay «%s» pulsable", rotulo)
		}
		pulsar("action.ok")
		if tam.Width == 1280 {
			guarda(t, w.Canvas().Capture())
		} else if d := os.Getenv("SYTUE_CAPTURA"); d != "" {
			t.Setenv("SYTUE_CAPTURA", strings.TrimSuffix(d, ".png")+"-360.png")
			guarda(t, w.Canvas().Capture())
		}
		pulsar("action.apply")
		if cuota, _ := lz.Libro.Valor(1, 3).(float64); math.Abs(cuota-700) > 1e-6 {
			t.Errorf("aplicar desde el panel deja la cuota en %v", lz.Libro.Valor(1, 3))
		}
	}
}
