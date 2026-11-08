// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestRangoDePeriodoComoLaWeb son los casos de periodo.test.ts.
//
// VISTO EN ROJO: con `t+2` en el trimestre acaba en noviembre; con `m-1, 28` en el mes
// anterior, octubre se queda en el 28.
func TestRangoDePeriodoComoLaWeb(t *testing.T) {
	hoy := time.Date(2026, time.November, 5, 12, 0, 0, 0, time.Local)
	for _, c := range []struct {
		p            string
		hoy          time.Time
		desde, hasta string
	}{
		{"todo", hoy, "", ""},
		{"mes", hoy, "2026-11-01", "2026-11-30"},
		{"mes", time.Date(2028, time.February, 10, 0, 0, 0, 0, time.Local), "2028-02-01", "2028-02-29"},
		{"mesAnterior", hoy, "2026-10-01", "2026-10-31"},
		{"mesAnterior", time.Date(2027, time.January, 15, 0, 0, 0, 0, time.Local), "2026-12-01", "2026-12-31"},
		{"trimestre", hoy, "2026-10-01", "2026-12-31"},
		{"trimestre", time.Date(2026, time.April, 30, 0, 0, 0, 0, time.Local), "2026-04-01", "2026-06-30"},
		{"anio", hoy, "2026-01-01", "2026-12-31"},
		{"anioAnterior", hoy, "2025-01-01", "2025-12-31"},
	} {
		d, h := RangoDePeriodo(c.p, c.hoy)
		if d != c.desde || h != c.hasta {
			t.Errorf("%s el %s: %s…%s, quiere %s…%s", c.p, c.hoy.Format(time.DateOnly), d, h, c.desde, c.hasta)
		}
	}
	for _, p := range Periodos {
		if rotuloPeriodo[p] == "" {
			t.Errorf("%s sin rótulo", p)
		}
	}
}

// TestElLibroYElBalanceSeAcotanPorPeriodo: el selector junto al título vuelve a pedir
// la pantalla con `desde`/`hasta` y, si no hay nada, lo dice del período. Es el
// `CabeceraPeriodo` de Libro.tsx.
//
// VISTO EN ROJO: con cargarLibro llamando a `Libro(ctx, …)` sin período, la segunda
// consulta sale igual que la primera; sin `AlPeriodo` en IrA no hay selector.
func TestElLibroYElBalanceSeAcotanPorPeriodo(t *testing.T) {
	p, sf := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/balance" {
			_, _ = io.WriteString(w, `{"cuentas":[],"total_debe":0,"total_haber":0}`)
			return
		}
		_, _ = io.WriteString(w, `{"asientos":[]}`)
	})
	for _, c := range []struct {
		vista, ruta, periodo string
		objeto               func() fyne.CanvasObject
	}{
		{VistaLibro, "/api/v1/libro", "mes", func() fyne.CanvasObject { return p.libro.ObjetoLibro(tema.Claro) }},
		{VistaBalance, "/api/v1/balance", "anioAnterior", func() fyne.CanvasObject { return p.libro.ObjetoBalance(tema.Claro) }},
	} {
		p.IrA(c.vista)
		var sel *ui.Selector
		recorrer(c.objeto(), func(o fyne.CanvasObject) {
			if s, ok := o.(*ui.Selector); ok {
				sel = s
			}
		})
		if sel == nil || sel.Rotulo != "Período" || sel.Valor() != "Todo" {
			t.Fatalf("%s: sin selector de período junto al título", c.vista)
		}
		if !contiene(textosDe(c.objeto()), "Todavía no hay nada asentado") {
			t.Errorf("%s: sin corte, el vacío de siempre", c.vista)
		}
		sel.AlElegir(slices.Index(Periodos, c.periodo))
		if !contiene(textosDe(c.objeto()), "Nada asentado en este período") {
			t.Errorf("%s: con corte, el vacío del período; se pintó %v", c.vista, textosDe(c.objeto()))
		}
		desde, hasta := RangoDePeriodo(c.periodo, time.Now())
		var pedidas []string
		for _, v := range sf.vistas() {
			if strings.HasPrefix(v, "GET "+c.ruta+"?") {
				pedidas = append(pedidas, strings.TrimPrefix(v, "GET "+c.ruta+"?"))
			}
		}
		if len(pedidas) != 2 || strings.Contains(pedidas[0], "desde") ||
			!strings.Contains(pedidas[1], "desde="+desde) || !strings.Contains(pedidas[1], "hasta="+hasta) {
			t.Errorf("%s: consultas %q", c.vista, pedidas)
		}
	}
}

// TestElSelectorDePeriodoSigueConRuedaYConFallo: mientras carga y con un período que
// falla, la cabecera con el selector se queda encima de la rueda o de la banda roja; es
// lo que deja volver a «Todo». Espejo del test de Libro.test.tsx.
//
// VISTO EN ROJO: con ObjetoLibro/ObjetoBalance devolviendo solo la rueda o solo el
// fallo (lo de antes), no hay selector en ninguno de los dos estados.
func TestElSelectorDePeriodoSigueConRuedaYConFallo(t *testing.T) {
	p, _ := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("desde") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"error":{"code":"caido","message":"Caído"}}`)
			return
		}
		if r.URL.Path == "/api/v1/balance" {
			_, _ = io.WriteString(w, `{"cuentas":[],"total_debe":0,"total_haber":0}`)
			return
		}
		_, _ = io.WriteString(w, `{"asientos":[]}`)
	})
	selectorDe := func(o fyne.CanvasObject) *ui.Selector {
		var sel *ui.Selector
		recorrer(o, func(x fyne.CanvasObject) {
			if s, ok := x.(*ui.Selector); ok {
				sel = s
			}
		})
		return sel
	}
	for _, c := range []struct {
		vista  string
		objeto func() fyne.CanvasObject
	}{
		{VistaLibro, func() fyne.CanvasObject { return p.libro.ObjetoLibro(tema.Claro) }},
		{VistaBalance, func() fyne.CanvasObject { return p.libro.ObjetoBalance(tema.Claro) }},
	} {
		p.IrA(c.vista)
		sel := selectorDe(c.objeto())
		if sel == nil {
			t.Fatalf("%s: sin selector", c.vista)
		}
		// Cargando: el corte ya está puesto y aún no ha contestado nadie.
		p.libro.Periodo, p.libro.Asientos, p.libro.Balance = "mes", nil, nil
		if s := selectorDe(c.objeto()); s == nil || s.Valor() != "Este mes" {
			t.Errorf("%s: mientras carga no hay selector", c.vista)
		}
		sel.AlElegir(slices.Index(Periodos, "trimestre"))
		o := c.objeto()
		if !contiene(textosDe(o), "Caído") {
			t.Fatalf("%s: el fallo no se pinta: %v", c.vista, textosDe(o))
		}
		s := selectorDe(o)
		if s == nil {
			t.Fatalf("%s: con el período fallido no queda selector para volver", c.vista)
		}
		s.AlElegir(slices.Index(Periodos, "todo"))
		if txt := textosDe(c.objeto()); contiene(txt, "Caído") || !contiene(txt, "Todavía no hay nada asentado") {
			t.Errorf("%s: volver a «Todo» no se recupera: %v", c.vista, txt)
		}
	}
}

// TestElSelectorDePeriodoConservaElFoco: con el foco en «Período», la flecha cambia el
// corte y repinta; el selector que se ve después es el MISMO y sigue con el foco, así
// que la flecha siguiente avanza otra vez sin volver con Tab. Es lo que hace el
// `<select>` de la web.
//
// VISTO EN ROJO: con un `ui.NuevoSelector` en cada `cabecera` (lo de antes), el
// selector pintado tras el cambio es otro y el foco se queda en el viejo.
func TestElSelectorDePeriodoConservaElFoco(t *testing.T) {
	p, _ := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"asientos":[]}`)
	})
	w := test.NewWindow(p.raiz)
	w.Resize(fyne.NewSize(1280, 800))
	t.Cleanup(w.Close)
	p.lienzo = w.Canvas()
	p.IrA(VistaLibro)
	selectorPintado := func() *ui.Selector {
		var sel *ui.Selector
		recorrer(p.raiz, func(o fyne.CanvasObject) {
			if s, ok := o.(*ui.Selector); ok && s.Rotulo == "Período" {
				sel = s
			}
		})
		return sel
	}
	sel := selectorPintado()
	if sel == nil {
		t.Fatal("sin selector de período")
	}
	w.Canvas().Focus(sel)
	for i, quiere := range []string{"Este mes", "Mes anterior"} {
		w.Canvas().Focused().TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
		ahora := selectorPintado()
		if ahora == nil || ahora.Valor() != quiere || p.libro.Periodo != Periodos[i+1] {
			t.Fatalf("paso %d: pintado %v, período %q", i, ahora, p.libro.Periodo)
		}
		if w.Canvas().Focused() != ahora {
			t.Fatalf("paso %d: el foco no está en el selector que se ve", i)
		}
	}
}
