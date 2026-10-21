// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"image/color"
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// libroConTabla es una hoja de ventas con una tabla con formato, fila de totales,
// minigráficos y una fórmula con referencias estructuradas, como la escribe la web.
const libroConTabla = `{"active":0,"sheets":[{"name":"Hoja1","cells":{` +
	`"A1":"Region","B1":"Ventas","C1":"Coste","A2":"Norte","B2":"10","C2":"4","A3":"Sur","B3":"20","C3":"5",` +
	`"A4":"Este","B4":"30","C4":"6","A5":"Oeste","B5":"40","C5":"7",` +
	`"E1":"=SUM(Ventas[Ventas])","E2":"=Ventas[@Ventas]*2","F2":"=Ventas[@Coste]+1"},` +
	`"tables":[{"name":"Ventas","range":"A1:C6","style":"blue","totalsRow":true,"totals":{"1":"sum","2":"average"}}],` +
	`"sparklines":[{"cell":"G2","range":"B2:B5","type":"line"},{"cell":"G3","range":"B2:C5","type":"column","color":"#16A34A"}]}]}`

// TestLasTablasYLosMinigraficosSeLeenYSeReescriben: un libro con estos objetos ya no se
// abre en solo lectura y sale igual que entró.
func TestLasTablasYLosMinigraficosSeLeenYSeReescriben(t *testing.T) {
	c := lienzoDePrueba()
	if !c.Cargar(libroConTabla) {
		t.Fatal("un libro con tabla y minigráficos sigue abriéndose en solo lectura")
	}
	if c.Rejilla.SoloLectura {
		t.Error("la rejilla se ha quedado cerrada")
	}
	if !igual(t, libroConTabla, c.Serializar()) {
		t.Error("el viaje de ida y vuelta ha cambiado las tablas o los minigráficos")
	}
	// Lo que la web descartaría al cargar, aquí también: una tabla sin nombre y un
	// minigráfico sin tipo no llegan al documento, y el resto sí.
	sucio := `{"active":0,"sheets":[{"name":"H","cells":{},"tables":[{"ref":"A1:B2"},{"name":"T","range":"A1:B2"}],` +
		`"sparklines":[{"cell":"A1","range":"B1:B9"}]}]}`
	d := lienzoDePrueba()
	if !d.Cargar(sucio) {
		t.Fatal("un libro con objetos mal formados se abre en solo lectura: la web los descarta y sigue")
	}
	if !igual(t, `{"active":0,"sheets":[{"name":"H","cells":{},"tables":[{"name":"T","range":"A1:B2"}]}]}`, d.Serializar()) {
		t.Error("no se descarta lo mismo que en la web")
	}
}

// TestLasEstructuradasSeCalculan: `Tabla[Columna]`, `[@Columna]` y `Tabla[@Columna]`
// dan lo mismo que en la web; renombrar la cabecera las rompe y deshacer las arregla.
func TestLasEstructuradasSeCalculan(t *testing.T) {
	c := lienzoDePrueba()
	c.Cargar(libroConTabla)
	l := c.Libro
	for _, caso := range []struct {
		c, f   int
		quiero float64
	}{{4, 0, 100}, {4, 1, 20}, {5, 1, 5}} {
		if got := l.Valor(caso.c, caso.f); got != caso.quiero {
			t.Errorf("%s vale %v y en la web %v", Ref(caso.c, caso.f), got, caso.quiero)
		}
	}
	// Cambiar un dato recalcula la suma.
	l.Escribir(1, 1, "100")
	if got := l.Valor(4, 0); got != 190.0 {
		t.Errorf("tras cambiar B2, la suma es %v y debería ser 190", got)
	}
	// Renombrar la columna deja la fórmula sin su columna: #REF!, como en la web.
	l.Escribir(1, 0, "Importe")
	if got := l.Valor(4, 0); got != formula.ErrRef {
		t.Errorf("con la columna renombrada, la suma es %v y debería ser #REF!", got)
	}
	l.Deshacer()
	if got := l.Valor(4, 0); got != 190.0 {
		t.Errorf("tras deshacer el renombrado, la suma es %v", got)
	}
}

// TestLaTablaCreceYSeGestionaDesdeLosComandos: crear, crecer al escribir al lado, estilo,
// fila de totales, su función y quitar, con los mismos comandos que la web.
func TestLaTablaCreceYSeGestionaDesdeLosComandos(t *testing.T) {
	c := lienzoDePrueba()
	l := c.Libro
	for i, v := range []string{"Mes", "Importe", "Enero", "10", "Febrero", "20"} {
		l.Escribir(i%2, i/2, v)
	}
	c.Rejilla.Seleccionar(0, 0, false)
	c.Mandar(CmdTablaAnadir, `{"range":"A1:B3","headers":true,"style":"green"}`)
	tablas := l.Tablas()
	if len(tablas) != 1 || tablas[0].Name != "Tabla1" || tablas[0].Range != "A1:B3" || tablas[0].Style != "green" {
		t.Fatalf("la tabla creada es %+v", tablas)
	}
	if af := l.Autofiltro(); af == nil || af.Row != 0 || af.C1 != 0 || af.C2 != 1 {
		t.Errorf("crear la tabla no pone el autofiltro en su cabecera: %+v", af)
	}
	// Escribir justo debajo la hace crecer, en un solo paso de deshacer.
	l.Escribir(0, 3, "Marzo")
	if got := l.Tablas()[0].Range; got != "A1:B4" {
		t.Errorf("escribir debajo deja la tabla en %s", got)
	}
	l.Deshacer()
	if got := l.Tablas()[0].Range; got != "A1:B3" || l.Crudo(0, 3) != "" {
		t.Errorf("deshacer deja la tabla en %s con A4 %q", got, l.Crudo(0, 3))
	}
	c.Rejilla.Seleccionar(1, 1, false)
	c.Mandar(CmdTablaEstilo, "amber")
	c.Mandar(CmdTablaTotales, "")
	tb := l.Tablas()[0]
	if tb.Style != "amber" || !tb.TotalsRow || tb.Range != "A1:B4" || tb.Totals["1"] != "sum" {
		t.Fatalf("tras estilo y totales la tabla es %+v", tb)
	}
	if got := l.Texto(0, 3); got != "sheet.total" {
		t.Errorf("la primera columna de la fila de totales dice %q", got)
	}
	if got := l.Texto(1, 3); got != "30" {
		t.Errorf("la suma de la fila de totales dice %q", got)
	}
	c.Mandar(CmdTablaFuncion, `{"name":"Tabla1","col":1,"fn":"max"}`)
	if got := l.Texto(1, 3); got != "20" {
		t.Errorf("con MAX la fila de totales dice %q", got)
	}
	// Filtrar Enero fuera: la fila de totales no se oculta y agrega solo lo visible.
	c.Mandar(CmdFiltrar, `{"col":0,"value":"Febrero","hidden":true}`)
	if got := l.Texto(1, 3); got != "10" {
		t.Errorf("con Febrero filtrado, el MAX de lo visible dice %q", got)
	}
	// Y ocultar las vacías tampoco se lleva la fila de totales, que en la hoja está vacía.
	c.Mandar(CmdFiltrar, `{"col":0,"value":"","hidden":true}`)
	if o := l.FilasOcultas(); len(o) < 5 || !o[2] || o[3] || !o[4] {
		t.Errorf("las filas ocultas son %v: la de Febrero y las vacías sí, y la de totales nunca", o)
	}
	c.Mandar(CmdTablaQuitar, "")
	if len(l.Tablas()) != 0 || l.Autofiltro() != nil {
		t.Errorf("quitar la tabla deja %+v y el autofiltro %+v", l.Tablas(), l.Autofiltro())
	}
}

// TestElAutocompletadoOfreceTablasYColumnas: `acAt` con tablas.
func TestElAutocompletadoOfreceTablasYColumnas(t *testing.T) {
	c := lienzoDePrueba()
	c.Cargar(libroConTabla)
	ancla := Celda{C: 7, F: 7}
	ac, ok := autocompletar("=SUM(Ven", 8, c.Libro, ancla)
	if !ok || !strings.Contains(strings.Join(ac.items, ","), "Ventas") || ac.clases[len(ac.clases)-1] != acTabla {
		t.Errorf("tras «=SUM(Ven» se ofrece %v %v", ac.items, ac.clases)
	}
	ac, ok = autocompletar("=Ventas[Co", 10, c.Libro, ancla)
	if !ok || len(ac.items) != 1 || ac.items[0] != "Coste" || ac.clases[0] != acColumna {
		t.Errorf("tras «=Ventas[Co» se ofrece %v %v", ac.items, ac.clases)
	}
	// Aceptar la columna cierra el corchete con el cursor detrás.
	c.Rejilla.Seleccionar(7, 7, false)
	c.Rejilla.Editar("=Ventas[Co")
	c.sincronizarFx(false)
	c.aceptarAC(0)
	if c.fx == nil || c.fx.texto != "=Ventas[Coste]" || c.fx.cursor != len("=Ventas[Coste]") {
		t.Errorf("aceptar la columna deja %+v", c.fx)
	}
	// Y se resalta como el rango de su columna.
	refs := formula.ExtractRefsCon("=SUM(Ventas[Coste])", c.Libro.ResolverParaResaltar(ancla))
	if len(refs) != 1 || refs[0].Rect != (formula.Rect{C1: 2, R1: 1, C2: 2, R2: 4}) {
		t.Errorf("la estructurada se resalta como %+v", refs)
	}
}

// TestLaTablaYLosMinigraficosSePintan es el guardián de píxel: la cabecera con el
// relleno del estilo, la banda en la segunda fila de datos, el contorno de la tabla y
// los trazos del minigráfico dentro de su celda.
func TestLaTablaYLosMinigraficosSePintan(t *testing.T) {
	c := lienzoDePrueba()
	c.Cargar(libroConTabla)
	c.Rejilla.Move(fyne.NewPos(0, 0))
	c.Rejilla.Resize(fyne.NewSize(800, 260))
	c.Rejilla.Seleccionar(6, 8, false) // el ancla lejos: su marco no tapa nada
	img := pinta(t, 800, 260, c.Rejilla)
	guarda(t, img)
	azul := estiloTablaDe("blue")
	cab, _ := tokens.ParseColor(azul.Cabecera)
	banda, _ := tokens.ParseColor(azul.Banda)
	borde, _ := tokens.ParseColor(azul.Borde)
	x := AnchoCabecera + AnchoCelda*1.5
	if got := px(img, x, AltoCelda+4); !cerca(got, cab) {
		t.Errorf("la cabecera de la tabla pinta %v y el estilo azul es %v", got, cab)
	}
	if got := px(img, x, AltoCelda*3+4); !cerca(got, banda) {
		t.Errorf("la segunda fila de datos pinta %v y la banda es %v", got, banda)
	}
	if got := px(img, x, AltoCelda*2+4); !cerca(got, Papel) {
		t.Errorf("la primera fila de datos pinta %v y va sin banda (papel %v)", got, Papel)
	}
	if got := px(img, AnchoCabecera+0.2, AltoCelda*4+4); !cerca(got, borde) {
		t.Errorf("el contorno izquierdo de la tabla pinta %v y el estilo dice %v", got, borde)
	}
	// El minigráfico de columnas (verde) pinta algo verde dentro de G3.
	verde, _ := tokens.ParseColor("#16A34A")
	hay := false
	for dx := float32(4); dx < AnchoCelda-4 && !hay; dx++ {
		for dy := float32(1); dy < 18 && !hay; dy++ {
			hay = cerca(px(img, AnchoCabecera+6*AnchoCelda+dx, AltoCelda*3+dy), verde)
		}
	}
	if !hay {
		t.Error("el minigráfico de columnas de G3 no pinta nada verde")
	}
}

// TestCalcusConTablasSePinta deja las capturas de escritorio y móvil de una hoja con
// tabla, totales y minigráficos para mirarlas al lado de la web (con `SYTUE_CAPTURA`).
func TestCalcusConTablasSePinta(t *testing.T) {
	s := &servicioHojas{rol: "editor", lamport: 1, hoja: libroConTabla}
	for _, tam := range []fyne.Size{fyne.NewSize(1280, 800), fyne.NewSize(360, 740)} {
		a := appDePrueba(t, s)
		a.AbrirID("h1", "")
		o := a.Objeto()
		o.Resize(tam)
		img := pinta(t, tam.Width, tam.Height, o)
		if destino := os.Getenv("SYTUE_CAPTURA"); destino != "" {
			t.Setenv("SYTUE_CAPTURA", strings.TrimSuffix(destino, ".png")+"-tablas-"+itoa(uint64(tam.Width))+".png")
			guarda(t, img)
			t.Setenv("SYTUE_CAPTURA", destino)
		}
		if a.abierto.lz.Rejilla.SoloLectura {
			t.Error("la hoja con tablas se ha abierto en solo lectura")
		}
		var m modelo
		_ = json.Unmarshal([]byte(a.abierto.lz.Serializar()), &m)
		if len(m.Sheets) != 1 || len(m.Sheets[0].Tables) != 1 || len(m.Sheets[0].Sparklines) != 2 {
			t.Errorf("lo que se guardaría pierde objetos: %+v", m.Sheets)
		}
	}
}

// libroConGrafico lleva un gráfico de columnas, uno circular y una segmentación sobre
// la tabla de ventas.
const libroConGrafico = `{"active":0,"sheets":[{"name":"Hoja1","cells":{` +
	`"A1":"Region","B1":"Ventas","C1":"Coste","A2":"Norte","B2":"10","C2":"4","A3":"Sur","B3":"20","C3":"5",` +
	`"A4":"Este","B4":"30","C4":"6","A5":"Oeste","B5":"40","C5":"7"},` +
	`"autofilter":{"row":0,"c1":0,"c2":2},` +
	`"charts":[{"id":"g1","type":"bar","range":"A1:C5","x":340,"y":20,"labels":true},` +
	`{"id":"g2","type":"donut","range":"A1:B5","x":340,"y":260,"legend":"right","w":300,"h":150}],` +
	`"slicers":[{"id":"s1","col":0,"x":700,"y":20}]}]}`

// TestLosGraficosYLasSegmentacionesSeLeenYSeGestionan: se entienden, salen igual, y los
// comandos y los gestos de la web los crean, mueven, editan y quitan.
func TestLosGraficosYLasSegmentacionesSeLeenYSeGestionan(t *testing.T) {
	c := lienzoDePrueba()
	if !c.Cargar(libroConGrafico) {
		t.Fatal("un libro con gráficos y segmentaciones sigue abriéndose en solo lectura")
	}
	if !igual(t, libroConGrafico, c.Serializar()) {
		t.Error("el viaje de ida y vuelta ha cambiado los gráficos o las segmentaciones")
	}
	l := c.Libro
	c.Mandar(CmdGrafico, `{"type":"hbar","range":"A1:B5"}`)
	gs := l.Graficos()
	if len(gs) != 3 || gs[2].Type != "hbar" || gs[2].ID == "" || *gs[2].X != 96+2*28 || *gs[2].Y != 48+2*28 {
		t.Fatalf("el gráfico nuevo es %+v", gs[len(gs)-1])
	}
	c.editarGrafico("g1", map[string]any{"stacked": true, "legend": "top", "w": 90.0})
	if g := l.Graficos()[0]; !g.Stacked || g.Legend != "top" || *g.W != TamanoMinGrafico {
		t.Errorf("tras editar, el gráfico es %+v", g)
	}
	c.moverObjeto("g1", 5000, -3)
	if g := l.Graficos()[0]; *g.X != TopePosicion || *g.Y != 0 {
		t.Errorf("mover no respeta los topes: %v,%v", *g.X, *g.Y)
	}
	c.quitarGrafico("g2")
	if len(l.Graficos()) != 2 {
		t.Errorf("quitar deja %d gráficos", len(l.Graficos()))
	}
	l.Deshacer()
	if len(l.Graficos()) != 3 {
		t.Errorf("deshacer el quitar deja %d gráficos", len(l.Graficos()))
	}
	// La segmentación filtra por el autofiltro: elegir solo «Sur» oculta el resto.
	c.segmentar(0, map[string]bool{"Sur": true})
	if o := l.FilasOcultas(); len(o) < 5 || o[2] || !o[1] || !o[3] || !o[4] {
		t.Errorf("con solo «Sur» elegido las filas ocultas son %v", o)
	}
	c.Mandar(CmdSegAnadir, `{"range":"A1:C5","col":0}`)
	if len(l.Segmentaciones()) != 1 {
		t.Error("una segunda segmentación de la misma columna no debe crearse")
	}
	c.Mandar(CmdSegAnadir, `{"range":"A1:C5","col":1}`)
	if len(l.Segmentaciones()) != 2 {
		t.Error("la segmentación de otra columna no se ha creado")
	}
	// Con la hoja protegida sin el permiso de objetos, nada se mueve ni se quita.
	c.Mandar(CmdProteger, `{}`)
	c.quitarGrafico("g1")
	if len(l.Graficos()) != 3 {
		t.Error("se ha quitado un gráfico de una hoja protegida sin permiso de objetos")
	}
}

// TestLasTarjetasSePintan es el guardián de píxel de los gráficos: la columna de la
// primera serie en su azul, dentro de la tarjeta y en su sitio, y la segmentación con
// sus filas.
func TestLasTarjetasSePintan(t *testing.T) {
	c := lienzoDePrueba()
	c.Cargar(libroConGrafico)
	c.Rejilla.Move(fyne.NewPos(0, 0))
	c.Rejilla.Resize(fyne.NewSize(1000, 500))
	c.Rejilla.Seleccionar(9, 18, false)
	img := pinta(t, 1000, 500, c.Rejilla)
	guarda(t, img)
	azul := colorDe(coloresSerie[0])
	hay := false
	// La tarjeta g1 está en (340, 20); sus columnas, debajo de la cabecera.
	for x := float32(340 + 50); x < 340+270 && !hay; x += 2 {
		for y := float32(20 + 30); y < 20+30+150 && !hay; y += 2 {
			hay = cerca(px(img, x, y), azul)
		}
	}
	if !hay {
		t.Error("la tarjeta del gráfico de columnas no pinta ninguna columna azul")
	}
	// La segmentación de la columna A enseña sus cuatro valores elegidos (seleccionados).
	// `bg-selected` es un token con alfa: sobre la tarjeta (`surface`) queda aplanado.
	s := tema.Color("selected", tema.Claro)
	sel := tokens.Aplanar(color.NRGBA{R: s.R, G: s.G, B: s.B, A: 255}, float64(s.A)/255, tema.Color("surface", tema.Claro))
	if got := px(img, 700+20, 20+float32(altoCabeceraTarjeta)+8); !cerca(got, sel) {
		t.Errorf("la primera fila de la segmentación pinta %v y elegida es %v", got, sel)
	}
}

// TestCalcusConGraficosSePinta deja las capturas de escritorio y móvil.
func TestCalcusConGraficosSePinta(t *testing.T) {
	s := &servicioHojas{rol: "editor", lamport: 1, hoja: libroConGrafico}
	for _, tam := range []fyne.Size{fyne.NewSize(1280, 800), fyne.NewSize(360, 740)} {
		a := appDePrueba(t, s)
		a.AbrirID("h1", "")
		o := a.Objeto()
		o.Resize(tam)
		img := pinta(t, tam.Width, tam.Height, o)
		if destino := os.Getenv("SYTUE_CAPTURA"); destino != "" {
			t.Setenv("SYTUE_CAPTURA", strings.TrimSuffix(destino, ".png")+"-graficos-"+itoa(uint64(tam.Width))+".png")
			guarda(t, img)
			t.Setenv("SYTUE_CAPTURA", destino)
		}
		if a.abierto.lz.Rejilla.SoloLectura {
			t.Error("la hoja con gráficos se ha abierto en solo lectura")
		}
	}
}
