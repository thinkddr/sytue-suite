// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// sueltos cuenta los `ui.Esqueleto` sueltos que cuelgan de o: cada uno late por su
// cuenta y no anuncia la carga.
func sueltos(o fyne.CanvasObject) int {
	n := 0
	buscarObjeto(o, func(x fyne.CanvasObject) bool {
		if _, es := x.(*ui.Esqueleto); es {
			n++
		}
		return false
	})
	return n
}

// enVentana monta o sola en una ventana sin relleno de `tam` y devuelve dónde cae cada
// bloque, en coordenadas de la ventana.
func enVentana(t *testing.T, o fyne.CanvasObject, tam fyne.Size) func(fyne.CanvasObject) (fyne.Position, fyne.Size) {
	t.Helper()
	w := test.NewTempWindow(t, o)
	w.SetPadded(false)
	w.Resize(tam)
	d := fyne.CurrentApp().Driver()
	return func(b fyne.CanvasObject) (fyne.Position, fyne.Size) { return d.AbsolutePositionForObject(b), b.Size() }
}

// TestLosBloquesDeCargaDeCalendarLatenAUna: la rejilla del mes, los calendarios del
// menú y las salas del formulario cargan con las piezas comunes —un solo latido y un
// solo «Cargando» por grupo—, con la MISMA geometría que tenían con un `ui.Esqueleto`
// por bloque (las cifras salen de volcar los rectángulos antes del cambio).
//
// VISTO EN ROJO con los esqueletos sueltos de antes: 49 en la rejilla, 2 en el menú y
// 1 en las salas. Y la rejilla, con los bloques estirados de antes (nombres de 120 × 12
// y celdas de 128 × 24, sin rayas y apiladas arriba): ahora tiene la geometría de la
// web, que fija TestLaRejillaDeCargaDelMesEsLaDeLaWeb.
func TestLosBloquesDeCargaDeCalendarLatenAUna(t *testing.T) {
	p := pantallaCaptura(t)
	p.Ventana = 1280

	rejilla := p.esqueleto()
	b, es := rejilla.(*ui.BloquesEsqueleto)
	if !es || len(b.Bloques()) != 49 || sueltos(rejilla) != 0 {
		t.Fatalf("la rejilla cargando es %T con %d esqueletos sueltos", rejilla, sueltos(rejilla))
	}
	if got, quiero := b.AccessibilityLabel(), p.t("loading.calendar", nil); got != quiero {
		t.Errorf("la rejilla cargando se anuncia «%s» y es «%s»", got, quiero)
	}
	donde := enVentana(t, rejilla, fyne.NewSize(980, 700))
	// Las semanas se reparten el alto que queda bajo la fila de nombres, como el
	// `flex-1 grid-rows-6` de la web: (700 − 33) / 6.
	semana := float32(700-33) / 6
	for _, c := range []struct {
		i        int
		pos, tam fyne.Size
		radio    float32
	}{
		{0, fyne.NewSize(10, 10), fyne.NewSize(32, 12), tema.RadioMD},    // el nombre del lunes
		{7, fyne.NewSize(6, 39), fyne.NewSize(24, 24), 12},               // la primera celda
		{48, fyne.NewSize(846, 33+5*semana+6), fyne.NewSize(24, 24), 12}, // la última
	} {
		q := b.Bloques()[c.i]
		pos, tam := donde(q)
		if pos != fyne.NewPos(c.pos.Width, c.pos.Height) || tam != c.tam || q.CornerRadius != c.radio {
			t.Errorf("el bloque %d de la rejilla cae en %v, mide %v con radio %g; la web, %v, %v y %g",
				c.i, pos, tam, q.CornerRadius, c.pos, c.tam, c.radio)
		}
	}
	// Y las rayas de `border-r border-b`: la de debajo de la primera semana y la de la
	// derecha de la primera columna, por dentro de la celda.
	rayas := map[fyne.Position]fyne.Size{}
	buscarObjeto(rejilla, func(x fyne.CanvasObject) bool {
		if r, es := x.(*canvas.Rectangle); es && r.FillColor == tema.Color("border", tema.Claro) {
			pos, tam := donde(r)
			rayas[pos] = tam
		}
		return false
	})
	for pos, tam := range map[fyne.Position]fyne.Size{
		fyne.NewPos(0, 32):          fyne.NewSize(980, 1),      // el `border-b` de los nombres
		fyne.NewPos(0, 33+semana-1): fyne.NewSize(980, 1),      // el de la primera semana
		fyne.NewPos(139, 33):        fyne.NewSize(1, 6*semana), // el `border-r` del lunes
	} {
		if rayas[pos] != tam {
			t.Errorf("no hay raya de %v en %v (rayas: %v)", tam, pos, rayas)
		}
	}

	p.CargandoCalendarios = true
	menu := p.seccionMisCalendarios()
	if len(menu.Filas) != 1 || sueltos(menu) != 0 {
		t.Fatalf("el menú cargando tiene %d piezas y %d esqueletos sueltos", len(menu.Filas), sueltos(menu))
	}
	filas, es := menu.Filas[0].(*ui.FilasEsqueleto)
	if !es || filas.Filas != 2 || filas.Tinte != "sidebar-hover-bg" {
		t.Fatalf("el menú cargando es %T", menu.Filas[0])
	}
	donde = enVentana(t, menu, fyne.NewSize(240, 300))
	for i, y := range []float32{34, 72} {
		pos, tam := donde(test.WidgetRenderer(filas).Objects()[i])
		if pos != fyne.NewPos(0, y) || tam != fyne.NewSize(240, 36) {
			t.Errorf("la fila %d del menú cae en %v y mide %v", i, pos, tam)
		}
	}

	a, w := appConVentana(t, &escrituras{})
	inicio := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	e := Edicion{Inicio: inicio, Fin: inicio.Add(time.Hour), Zona: "UTC"}
	f := a.P.nuevoFormulario(e, a.destinos(), a.accionesFormulario(e))
	f.rec.lista = nil // aún no han llegado
	salas := f.bloqueRecursos()
	var grupo *ui.BloquesEsqueleto
	buscarObjeto(salas, func(x fyne.CanvasObject) bool {
		grupo, _ = x.(*ui.BloquesEsqueleto)
		return grupo != nil
	})
	if grupo == nil || len(grupo.Bloques()) != 1 || sueltos(salas) != 0 {
		t.Fatalf("las salas cargando no son un grupo de un bloque (%d sueltos)", sueltos(salas))
	}
	w.SetContent(salas)
	w.Resize(fyne.NewSize(400, 200))
	if pos, tam := fyne.CurrentApp().Driver().AbsolutePositionForObject(grupo.Bloques()[0]), grupo.Bloques()[0].Size(); pos.Y != tema.TextoSM.Linea+tema.Espacio(1.5) || tam.Height != tema.Espacio(9) {
		t.Errorf("el bloque de las salas cae en %v y mide %v", pos, tam)
	}
}

// TestLaRejillaDeCargaDelMesEsLaDeLaWeb ata las medidas del esqueleto del mes al
// `GridSkeleton` de CalendarApp.tsx: si la web cambia un bloque o un relleno, esto se
// pone en rojo en vez de dejar que las dos se separen en silencio.
func TestLaRejillaDeCargaDelMesEsLaDeLaWeb(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/apps/calendar/src/components/CalendarApp.tsx")
	i := strings.Index(fuente, "function GridSkeleton(")
	j := strings.Index(fuente[max(i, 0):], "const cols =")
	if i < 0 || j < 0 {
		t.Fatal("CalendarApp.tsx ya no tiene el `GridSkeleton` del mes")
	}
	mes := fuente[i : i+j]
	for _, c := range []struct{ trozo, porque string }{
		{`<div key={i} class="px-2.5 py-2.5">`, "el relleno del nombre del día (espacio2p5)"},
		{`<Skeleton width="2rem" height="0.75rem" />`, "el nombre del día (AnchoDiaCarga × AltoDiaCarga)"},
		{`class="border-border grid grid-cols-7 border-b"`, "el `border-b` de la fila de nombres"},
		{`class="grid flex-1 grid-cols-7 grid-rows-6"`, "las seis semanas que se reparten el alto"},
		{`class="border-border border-r border-b p-1.5"`, "las rayas y el relleno de la celda (RellenoCelda)"},
		{`<Skeleton width="1.5rem" height="1.5rem" circle />`, "el círculo de la celda (LadoCeldaCarga)"},
	} {
		if !strings.Contains(mes, c.trozo) {
			t.Errorf("GridSkeleton ya no dice %q (%s)", c.trozo, c.porque)
		}
	}
	if AnchoDiaCarga != 2*tema.Rem || AltoDiaCarga != 0.75*tema.Rem || LadoCeldaCarga != 1.5*tema.Rem ||
		espacio2p5 != tema.Espacio(2.5) || RellenoCelda != tema.Espacio(1.5) {
		t.Errorf("las medidas nativas (%v × %v, %v, %v, %v) no son las de la web",
			AnchoDiaCarga, AltoDiaCarga, LadoCeldaCarga, espacio2p5, RellenoCelda)
	}
}

// TestBuscarHuecoCargaConUnGrupoDeBloques: las dos barras de «buscar hueco» son UN
// `ui.BloquesEsqueleto` que se anuncia con `findTime.loading`, como el `role="status"`
// de FindTimePanel.tsx, el MISMO mientras dure la carga (rehacer el formulario no
// arranca otro latido) y con la geometría de antes: las cifras salen de volcar los
// rectángulos de los dos `ui.Esqueleto` sueltos.
//
// VISTO EN ROJO con los esqueletos sueltos de antes: «0 grupos y 2 sueltos».
func TestBuscarHuecoCargaConUnGrupoDeBloques(t *testing.T) {
	a, _ := appConVentana(t, &escrituras{})
	inicio := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	e := Edicion{Inicio: inicio, Fin: inicio.Add(time.Hour), Zona: "UTC"}
	f := a.P.nuevoFormulario(e, a.destinos(), a.accionesFormulario(e))
	f.construirHuecos()
	f.bh.cargando = true
	grupos := func(o fyne.CanvasObject) (g []*ui.BloquesEsqueleto) {
		buscarObjeto(o, func(x fyne.CanvasObject) bool {
			if b, es := x.(*ui.BloquesEsqueleto); es {
				g = append(g, b)
			}
			return false
		})
		return g
	}
	panel := f.panelHuecos()
	g := grupos(panel)
	if len(g) != 1 || len(g[0].Bloques()) != 2 || sueltos(panel) != 0 {
		t.Fatalf("buscar hueco cargando pinta %d grupos y %d sueltos", len(g), sueltos(panel))
	}
	if got, quiero := g[0].AccessibilityLabel(), a.P.t("findTime.loading", nil); got != quiero {
		t.Errorf("la carga se anuncia «%s» y es «%s»", got, quiero)
	}
	if otra := grupos(f.panelHuecos()); len(otra) != 1 || otra[0] != g[0] {
		t.Error("rehacer el panel cargando estrena otro grupo de bloques")
	}
	donde := enVentana(t, panel, fyne.NewSize(480, 300))
	for i, quiero := range [][2]fyne.Size{
		{{Width: 13, Height: 153}, {Width: 454, Height: 28}},
		{{Width: 13, Height: 187}, {Width: 317.8, Height: 28}},
	} {
		pos, tam := donde(g[0].Bloques()[i])
		if pos != fyne.NewPos(quiero[0].Width, quiero[0].Height) || tam != quiero[1] {
			t.Errorf("la barra %d cae en %v y mide %v; antes, %v", i, pos, tam, quiero)
		}
	}
	f.bh.cargado()
	if f.bh.carga != nil {
		t.Error("terminada la carga, el panel sigue guardando sus barras")
	}
}
