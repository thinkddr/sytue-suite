// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLaMigaMide26Coma25 es el número medido en `drive/archivos.ancho.json`: el botón
// «Mi unidad» del `nav[aria-label="Ruta"]` mide 26,25 de alto con `3.5px 7px` de
// relleno y radio 6.
func TestLaMigaMide26Coma25(t *testing.T) {
	m := NuevaMiga("Mi unidad", nil)
	m.Actual = true
	d := MedidaMiga(m)
	if d.Height != 30 {
		t.Errorf("la miga mide %v de alto y en producción mide 26,25", d.Height)
	}
	if got := m.MinSize(); got != d {
		t.Errorf("MinSize() = %v y MedidaMiga dice %v", got, d)
	}
	// El relleno horizontal es `px-2`, o sea 8 por lado: 16 más que el rótulo.
	if got, quiero := d.Width-Medir("Mi unidad", tema.TextoBase, SemiNegrita).Width, 2*tema.Espacio(2); got != quiero {
		t.Errorf("la miga añade %v de relleno y `px-2` con el rem de 16 son %v", got, quiero)
	}
}

// TestElUltimoSegmentoVaEnSemiNegrita: `aria-current="page"` lleva `font-semibold`
// (600) y los demás `font-medium` (500). En producción se mide `12.25px/19.25px 600`
// en el actual.
//
// No es cosmética: es lo único que dice en qué carpeta estás cuando la ruta es larga.
func TestElUltimoSegmentoVaEnSemiNegrita(t *testing.T) {
	m := NuevasMigas("Ruta", []string{"Mi unidad", "Contratos", "2026"}, nil)
	_ = test.WidgetRenderer(m)
	for i, s := range m.segmentos {
		ultimo := i == len(m.segmentos)-1
		if s.Actual != ultimo {
			t.Errorf("el segmento %d dice Actual=%v y el actual es el último", i, s.Actual)
		}
		quiero := Medio
		if ultimo {
			quiero = SemiNegrita
		}
		if s.peso() != quiero {
			t.Errorf("el segmento %d va con el peso %v y tenía que ir con %v", i, s.peso(), quiero)
		}
	}
}

// TestLosGalonesSeparanLosSegmentos es el `gap-0.5` medido a los dos lados del galón
// de 14: entre dos rótulos hay 1,75 + 14 + 1,75.
func TestLosGalonesSeparanLosSegmentos(t *testing.T) {
	m := NuevasMigas("Ruta", []string{"Mi unidad", "Contratos"}, nil)
	d := MedidaMigas(m)
	m.Resize(d)
	_ = test.WidgetRenderer(m)
	m.Refresh()
	pinta(t, d.Width+10, d.Height+10, m)

	a, b := m.segmentos[0], m.segmentos[1]
	hueco := b.Position().X - (a.Position().X + a.Size().Width)
	if quiero := 2*tema.Espacio(0.5) + LadoGalonMiga; hueco != quiero {
		t.Errorf("entre dos segmentos hay %v y el galón con sus dos `gap-0.5` son %v", hueco, quiero)
	}
	if d.Height != 30 {
		t.Errorf("la ruta mide %v de alto y su segmento mide 26,25", d.Height)
	}
}

// TestLaRutaLargaSeEncogeEnVezDeDesbordar: la web les pone `min-w-0` y `truncate`, así
// que una ruta de diez carpetas se estrecha dentro de su hueco en vez de empujar la
// cabecera. Los galones no encogen (`shrink-0`).
func TestLaRutaLargaSeEncogeEnVezDeDesbordar(t *testing.T) {
	ruta := []string{"Mi unidad", "Documentación de producto", "Contratos firmados", "2026"}
	m := NuevasMigas("Ruta", ruta, nil)
	entero := MedidaMigas(m).Width
	estrecho := entero / 2
	m.Resize(fyne.NewSize(estrecho, AltoMiga))
	_ = test.WidgetRenderer(m)
	m.Refresh()
	pinta(t, entero+10, 40, m)

	ultimo := m.segmentos[len(m.segmentos)-1]
	if fin := ultimo.Position().X + ultimo.Size().Width; fin > estrecho+0.5 {
		t.Errorf("la ruta acaba en %v y solo tenía %v de sitio: se ha desbordado", fin, estrecho)
	}
	for i, g := range m.galones {
		if got := g.Objeto().Size().Width; got != LadoGalonMiga {
			t.Errorf("el galón %d mide %v y no tiene que encoger: son 14", i, got)
		}
	}
}

// TestLaMigaLlevaANingunSitioSiNoLeDanADonde comprueba el cableado del índice, que es
// donde se cuela el error clásico de este componente: todas las migas llevando a la
// última carpeta porque la variable del bucle se captura una sola vez.
func TestLaMigaLlevaANingunSitioSiNoLeDanADonde(t *testing.T) {
	visitado := -1
	m := NuevasMigas("Ruta", []string{"Mi unidad", "Contratos", "2026"}, func(i int) { visitado = i })
	_ = test.WidgetRenderer(m)
	test.Tap(m.segmentos[1])
	if visitado != 1 {
		t.Errorf("se ha pulsado el segundo segmento y ha llevado al %d", visitado)
	}
	test.Tap(m.segmentos[0])
	if visitado != 0 {
		t.Errorf("se ha pulsado el primero y ha llevado al %d", visitado)
	}
}

// TestLasMigasDeLaBarraSonDeTextoPequeno: la ruta de «Compartido conmigo» es `text-sm`
// con el mismo `py-1`, así que cada segmento mide 15,75 + 7 = 22,75 y no 26,25; y la del
// diálogo «Mover a…» mide lo del cromo. Lo que cambia entre las tres es el cuerpo, el
// fondo al pasar por encima y el galón, y nada más.
func TestLasMigasDeLaBarraSonDeTextoPequeno(t *testing.T) {
	barra := NuevasMigas("Ruta", []string{"Proyecto", "Planos"}, nil)
	barra.Estilo = MigasEnBarra
	if got := MedidaMigas(barra).Height; got != tema.TextoSM.Linea+2*tema.Espacio(1) {
		t.Errorf("la ruta de la barra mide %v de alto", got)
	}
	dialogo := NuevasMigas("Ruta", []string{"Mi unidad"}, nil)
	dialogo.Estilo = MigasEnDialogo
	if got := MedidaMigas(dialogo).Height; got != AltoMiga {
		t.Errorf("la ruta del diálogo mide %v y es la del cromo, %v", got, AltoMiga)
	}
	if MigasEnBarra.escalon() == (EstiloMigas{}).escalon() {
		t.Error("la barra y el cromo usan el mismo cuerpo de letra")
	}
	w := test.NewWindow(barra)
	defer w.Close()
	w.Resize(fyne.NewSize(300, 30))
	w.Canvas().Capture()
}

// rutaDeOcho es la del criterio de la pieza: ocho niveles, de la raíz al actual.
var rutaDeOcho = []string{"Mi unidad", "Proyectos", "Clientes", "Acme", "Contratos", "2026", "Firmados", "Anexos"}

// leidas son los rótulos de lo que se ve de la ruta, en orden de izquierda a derecha.
func leidas(m *Migas) []string {
	var out []string
	var xs []float32
	todos := append([]*Miga{}, m.segmentos...)
	if m.puntos != nil {
		todos = append(todos, m.puntos)
	}
	for _, s := range todos {
		if !s.Visible() {
			continue
		}
		i := 0
		for i < len(xs) && xs[i] < s.Position().X {
			i++
		}
		xs = append(xs[:i], append([]float32{s.Position().X}, xs[i:]...)...)
		out = append(out[:i], append([]string{s.Rotulo}, out[i:]...)...)
	}
	return out
}

// TestA360LaRutaDeOchoNivelesSePliega es el criterio de Migas.tsx: a 360 una ruta de
// ocho niveles se lee «Mi unidad › … › padre › actual», sin desbordar su hueco, y el
// «…» se anuncia por lo que hace y no como tres puntos.
func TestA360LaRutaDeOchoNivelesSePliega(t *testing.T) {
	m := NuevasMigas("Ruta", rutaDeOcho, nil)
	w := test.NewWindow(m)
	defer w.Close()
	w.Resize(fyne.NewSize(400, 60))
	m.Resize(fyne.NewSize(360, AltoMiga))
	m.Refresh()
	fyne.DoAndWait(func() { test.WidgetRenderer(m).Layout(fyne.NewSize(360, AltoMiga)) })

	if !m.Plegada() {
		t.Fatal("la ruta de ocho niveles no se ha plegado a 360")
	}
	quiero := []string{"Mi unidad", "…", "Firmados", "Anexos"}
	if got := leidas(m); !slices.Equal(got, quiero) {
		t.Errorf("a 360 se lee %q y tenía que leerse %q", got, quiero)
	}
	visibles := 0
	for _, g := range m.galones {
		if g.Objeto().Visible() {
			visibles++
		}
	}
	if visibles != 3 {
		t.Errorf("la ruta plegada pinta %d galones y son tres", visibles)
	}
	ultimo := m.segmentos[len(m.segmentos)-1]
	if fin := ultimo.Position().X + ultimo.Size().Width; fin > 360+0.5 {
		t.Errorf("la ruta plegada acaba en %v y solo tenía 360", fin)
	}
	if !ultimo.Actual {
		t.Error("el último segmento ya no es el actual")
	}
	if got := m.puntos.AccessibilityLabel(); got != textoComun("ui.migas.ocultas") || got == "…" {
		t.Errorf("el «…» se anuncia como %q", got)
	}
}

// TestElEllipsisAbreLosNivelesOcultos: el «…» despliega un menú con los cinco niveles
// que esconde, en orden, y cada uno lleva a SU índice de la ruta.
func TestElEllipsisAbreLosNivelesOcultos(t *testing.T) {
	visitado := -1
	m := NuevasMigas("Ruta", rutaDeOcho, func(i int) { visitado = i })
	w := test.NewWindow(m)
	defer w.Close()
	w.Resize(fyne.NewSize(400, 300))
	fyne.DoAndWait(func() { test.WidgetRenderer(m).Layout(fyne.NewSize(360, AltoMiga)) })

	fyne.DoAndWait(func() { test.Tap(m.puntos) })
	menu := m.MenuOcultos()
	if menu == nil {
		t.Fatal("pulsar «…» no ha abierto ningún menú")
	}
	nombres := make([]string, 0, len(menu.Elementos))
	for _, e := range menu.Elementos {
		nombres = append(nombres, e.(*ElementoMenu).Texto)
	}
	if quiero := rutaDeOcho[1:6]; !slices.Equal(nombres, quiero) {
		t.Errorf("el menú de «…» lleva %q y tenía que llevar %q", nombres, quiero)
	}
	fyne.DoAndWait(func() { test.Tap(menu.Elementos[2].(*ElementoMenu)) })
	if visitado != 3 {
		t.Errorf("«Acme» es el nivel 3 de la ruta y ha llevado al %d", visitado)
	}
	if m.MenuOcultos() != nil {
		t.Error("el menú sigue abierto después de elegir un nivel")
	}
}

// TestLaRutaQueCabeNoCambia: con sitio de sobra no hay «…», y los segmentos quedan
// donde estaban antes del pliegue: en fila, a su ancho natural. Una ruta de tres niveles
// no se pliega ni sin sitio, porque no tiene nada entre la raíz y el padre.
func TestLaRutaQueCabeNoCambia(t *testing.T) {
	m := NuevasMigas("Ruta", rutaDeOcho, nil)
	d := MedidaMigas(m)
	_ = test.WidgetRenderer(m)
	m.Resize(d)
	test.WidgetRenderer(m).Layout(d)
	if m.Plegada() || m.puntos.Visible() {
		t.Error("la ruta cabe y se ha plegado")
	}
	x := float32(0)
	for i, s := range m.segmentos {
		if i > 0 {
			x += 2*huecoMig + LadoGalonMiga
		}
		if !s.Visible() || s.Position().X != x || s.Size().Width != MedidaMiga(s).Width {
			t.Errorf("el segmento %d está en %v con %v de ancho; antes, en %v con %v",
				i, s.Position().X, s.Size().Width, x, MedidaMiga(s).Width)
		}
		x += MedidaMiga(s).Width
	}

	corta := NuevasMigas("Ruta", rutaDeOcho[:3], nil)
	_ = test.WidgetRenderer(corta)
	test.WidgetRenderer(corta).Layout(fyne.NewSize(40, AltoMiga))
	if corta.Plegada() || corta.puntos != nil {
		t.Error("una ruta de tres niveles se ha plegado")
	}
}

// TestLaRutaSeDespliegaAlEnsancharse: el pliegue se decide en cada Layout, así que al
// darle sitio otra vez vuelven todos los niveles.
func TestLaRutaSeDespliegaAlEnsancharse(t *testing.T) {
	m := NuevasMigas("Ruta", rutaDeOcho, nil)
	r := test.WidgetRenderer(m)
	r.Layout(fyne.NewSize(360, AltoMiga))
	if !m.Plegada() {
		t.Fatal("no se ha plegado a 360")
	}
	r.Layout(MedidaMigas(m))
	if got := leidas(m); !slices.Equal(got, rutaDeOcho) {
		t.Errorf("con sitio de sobra se lee %q", got)
	}
}
