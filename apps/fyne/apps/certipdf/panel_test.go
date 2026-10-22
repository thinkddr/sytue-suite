// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"image/color"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// traductorES es el catálogo de VERDAD en español: el panel interpola (`{kb}`, `{n}`), y
// una identidad no diría si la variable se sustituye.
func traductorES(t *testing.T) TraductorVars {
	t.Helper()
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		t.Fatal(err)
	}
	return tr.T
}

// TestLaTiraDelPanelEsLaDeLaWeb compara la tira con `pestanas` de `PanelLateral`, fila
// a fila, y fija por IGUALDAD qué paneles faltan: portar uno obliga a quitarlo de aquí
// en el mismo cambio, y dejar de pintar uno que estaba sale en rojo igual.
func TestLaTiraDelPanelEsLaDeLaWeb(t *testing.T) {
	t.Parallel()
	c := &Contenido{
		Total:       12,
		Indice:      []Marcador{{Titulo: "a"}, {Titulo: "b"}},
		Adjuntos:    []Adjunto{{Nombre: "factura.xml"}},
		Anotaciones: []AnotacionLeida{{}, {}, {}},
		Firmas:      []FirmaLeida{{}},
	}
	quiero := []pestanaDelPanel{
		{PanelPaginas, "pdf.panel.pages", 12},
		{PanelIndice, "pdf.panel.outline", 2},
		{PanelAdjuntos, "pdf.panel.attachments", 1},
		{PanelComentarios, "pdf.panel.comments", 3},
		{PanelCampos, "pdf.panel.fields", 0},
		{PanelFirmas, "pdf.panel.signatures", 1},
		{PanelEditar, "pdf.panel.edit", 0},
		{PanelBuscar, "pdf.panel.search", 0},
		{PanelAsistente, "pdf.ai.tab", 0},
	}
	if got := tiraDelPanel(c); !slices.Equal(got, quiero) {
		t.Errorf("la tira es\n  %v\ny la web pone\n  %v", got, quiero)
	}
	// Sin adjuntos, su pestaña NO está: la web la esconde porque casi ningún PDF los
	// lleva.
	c.Adjuntos = nil
	for _, x := range tiraDelPanel(c) {
		if x.id == PanelAdjuntos {
			t.Error("sin adjuntos la tira sigue enseñando «Adjuntos»")
		}
	}

	var pendientes []PanelActivo
	var faltan []PanelActivo
	for _, x := range quiero {
		if !panelPortado[x.id] {
			faltan = append(faltan, x.id)
		}
	}
	if !slices.Equal(faltan, pendientes) {
		t.Errorf("paneles sin pintar: %v; la lista de pendientes dice %v. Si has portado "+
			"uno, quítalo de `pendientes` y del comentario de `panelPortado`", faltan, pendientes)
	}
}

// TestElPanelSoloEnsenaLoQueSabePintar: la tira pintada lleva las tres pestañas que
// existen y ninguna más, en el orden de la web. Una pestaña que abre un panel vacío es
// fingir que está hecho.
func TestElPanelSoloEnsenaLoQueSabePintar(t *testing.T) {
	c := &Contenido{Adjuntos: []Adjunto{{Nombre: "x"}}}
	p := NuevoPanelLateral(traductorES(t), c)
	w := ventanaSinRelleno(p.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(AnchoPanel, 400))

	// [máscara [línea, tira], degradado]: la tira va dentro de la máscara del borde.
	conLinea := p.raiz.Objects[0].(*fyne.Container).Objects[0].(*fyne.Container)
	envuelta, ok := conLinea.Objects[1].(*fyne.Container).Objects[0].(*container.ThemeOverride)
	if !ok {
		t.Fatal("la tira no va envuelta sin barra de desplazamiento")
	}
	desliza, ok := envuelta.Content.(*container.Scroll)
	if !ok {
		t.Fatal("la tira no se desliza: los nueve rótulos no caben en los 288 del panel")
	}
	fila, ok := desliza.Content.(*fyne.Container)
	if !ok {
		t.Fatal("la tira no es un contenedor")
	}
	rotulos := make([]string, 0, len(fila.Objects))
	for _, o := range fila.Objects {
		rotulos = append(rotulos, o.(*pestanaPanel).rotulo)
	}
	if quiero := []string{"Páginas", "Índice", "Adjuntos", "Comentarios", "Campos", "Firmas", "Editar", "Buscar", "Asistente"}; !slices.Equal(rotulos, quiero) {
		t.Errorf("la tira pinta %q; con lo portado tienen que ser %q", rotulos, quiero)
	}
	if got := p.Abierto(); got != PanelPaginas {
		t.Errorf("el panel abre en %d y no en Páginas, que es donde abre la web", got)
	}
	p.Abrir(PanelAsistente + 1)
	if got := p.Abierto(); got != PanelPaginas {
		t.Errorf("Abrir un panel que no existe lo deja en %d: abriría un panel vacío", got)
	}
	p.Abrir(PanelFirmas)
	if got := p.Abierto(); got != PanelFirmas {
		t.Errorf("Abrir las firmas lo deja en %d", got)
	}
}

// TestLaSangriaDelIndiceEsLaDeLaWeb: `paddingInlineStart: 0.5 + min(nivel, 6) * 0.75
// rem`. Los números van escritos a mano con el rem de 16 —8, 20, 80—: sacarlos de la
// función sería medirla contra sí misma. El tope en 6 es el caso que importa: sin él un
// índice de cuarenta niveles se sale del panel.
func TestLaSangriaDelIndiceEsLaDeLaWeb(t *testing.T) {
	t.Parallel()
	for nivel, quiero := range map[int]float32{0: 8, 1: 20, 6: 80, 9: 80, -1: 8} {
		if got := sangriaIndice(nivel); got != quiero {
			t.Errorf("nivel %d: sangría %g; la web da %g", nivel, got, quiero)
		}
	}
	if got := AltoFilaIndice(); got != 24 {
		t.Errorf("la fila del índice mide %g; `py-1 text-xs` son 24", got)
	}
}

// TestUnMarcadorSinDestinoNoLleva comprueba los dos sentidos: el que tiene página lleva a
// ella —la de la web, que es 1-based sobre un marcador 0-based— y el que no, no se pulsa.
func TestUnMarcadorSinDestinoNoLleva(t *testing.T) {
	c := &Contenido{Indice: []Marcador{
		{Titulo: "Capítulo 1", Pagina: 4},
		{Titulo: "Anexo perdido", Pagina: -1},
	}}
	p := NuevoPanelLateral(traductorES(t), c)
	var fue []int
	p.AlIrA = func(n int) { fue = append(fue, n) }
	var filas []*filaIndice
	todos(p.indice(), func(o fyne.CanvasObject) {
		if f, ok := o.(*filaIndice); ok {
			filas = append(filas, f)
		}
	})
	if len(filas) != 2 {
		t.Fatalf("el índice pinta %d filas", len(filas))
	}
	for _, f := range filas {
		f.Tapped(nil)
	}
	if !slices.Equal(fue, []int{5}) {
		t.Errorf("pulsar las dos filas llevó a %v; tenía que llevar solo a la página 5", fue)
	}
	if f := filas[1]; !f.Inerte || f.AccessibilityLabel() == f.m.Titulo {
		t.Errorf("el marcador sin destino tiene que ser inerte y anunciar por qué; es "+
			"inerte=%v y se anuncia como %q", f.Inerte, f.AccessibilityLabel())
	}
}

// TestLaFichaDelAdjuntoDiceLoQueLaWeb: el tamaño con el `formatBytes` de la suite, en el
// idioma de la pantalla, el tipo y la página.
func TestLaFichaDelAdjuntoDiceLoQueLaWeb(t *testing.T) {
	// Sin t.Parallel: mide texto, y el conformador de Fyne comparte búfer entre llamadas
	// (visto con -race: harfbuzz.(*Buffer).Clear desde dos pruebas a la vez).
	p := NuevoPanelLateral(traductorES(t), &Contenido{})
	p.pant = &Pantalla{Idioma: "es"}
	casos := []struct {
		a      Adjunto
		quiero string
	}{
		{Adjunto{TamanoBytes: 0, Pagina: -1}, "0 B"},
		{Adjunto{TamanoBytes: 1536, Pagina: -1}, "1,5 KB"},
		{Adjunto{TamanoBytes: 20480, Tipo: "text/xml", Pagina: 2}, "20 KB · text/xml · Página 3"},
	}
	for _, c := range casos {
		if got := p.metaAdjunto(c.a); got != c.quiero {
			t.Errorf("%+v: %q; la web pinta %q", c.a, got, c.quiero)
		}
	}
}

// TestLaPestanaDelPanelMideLaDeLaWeb: `text-2xs px-2.5 py-2 border-b-2` son 34 de alto
// (16 + 2 × 8 + 2) y la tira, con su `border-b`, 35. El subrayado de la abierta es el
// acento y el de las demás es transparente, en la MISMA fila: si no, cambiar de pestaña
// movería el rótulo.
func TestLaPestanaDelPanelMideLaDeLaWeb(t *testing.T) {
	if got := AltoPestanaPanel(); got != 35 {
		t.Errorf("la tira mide %g; la web, 35", got)
	}
	if got := AnchoPanel; got != 288 {
		t.Errorf("el panel mide %g; `w-72` con el rem de 16 son 288", got)
	}
	for _, activa := range []bool{true, false} {
		p := nuevaPestanaPanel("Índice", 2, activa, nil)
		// La ventana ANTES de medir: sin ella no hay aplicación de prueba, y ni el tema
		// ni el conformador existen todavía.
		w := ventanaSinRelleno(container.NewWithoutLayout(p))
		if got := p.MinSize().Height; got != 34 {
			t.Errorf("activa=%v: la pestaña mide %g de alto; la web, 34", activa, got)
		}
		acento := tema.Color("accent", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
		p.Resize(p.MinSize())
		w.Resize(p.MinSize())
		img := w.Canvas().Capture()
		abajo := px(img, img.Bounds().Dx()/2, img.Bounds().Dy()-1)
		if es := cercaDe(abajo, acento); es != activa {
			t.Errorf("activa=%v: la última fila es %v (acento %v)", activa, abajo, acento)
		}
		w.Close()
	}
}

func px(img interface{ At(int, int) color.Color }, x, y int) color.NRGBA {
	r, g, b, a := img.At(x, y).RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

func cercaDe(a, b color.NRGBA) bool {
	d := func(x, y uint8) bool { return int(x)-int(y) > 2 || int(y)-int(x) > 2 }
	return !d(a.R, b.R) && !d(a.G, b.G) && !d(a.B, b.B)
}

// TestIrALlevaALaHoja: pulsar una entrada del índice mueve el documento hasta esa hoja.
// El destino se calcula con el A4 y las medidas CSS, sin pedirlo a MedidaHoja.
func TestIrALlevaALaHoja(t *testing.T) {
	p := NuevaPantalla(traductorES(t))
	p.Estado = editor.Listo
	p.Ajuste = AjusteLibre // zoom 1: la cuenta de abajo es a tamaño natural
	p.Contenido = &Contenido{Total: 3, Paginas: []Pagina{a4(), a4(), a4()}}
	for i := range p.Contenido.Paginas {
		p.Contenido.Paginas[i].Numero = i + 1
	}
	w := ventanaSinRelleno(p.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(1100, 500))
	p.IrA(3)
	// `cont.scrollTo({ top: hoja.offsetTop - 12 })`: la hoja empieza tras el `py-6` (24)
	// y dos hojas con su `gap-6`, y se deja un margen de 12 px LITERALES por encima.
	// Cada A4 ocupa 842 + gap-1.5 (6) + línea text-xs (16) = 864.
	const quiero float32 = 24 + 2*(864+24) - 12
	if got := p.desplazar.Offset.Y; got != quiero {
		t.Errorf("IrA(3) deja el documento en y=%g; la web lo deja en %g", got, quiero)
	}
	if p.PaginaActual != 3 {
		t.Errorf("IrA(3) deja la página actual en %d", p.PaginaActual)
	}
	// Acotado: «999» de un documento de tres va a la última, y vaciar el campo, a la 1.
	p.IrA(999)
	if p.PaginaActual != 3 {
		t.Errorf("IrA(999) deja la página en %d y no en la última", p.PaginaActual)
	}
	p.IrA(0)
	if p.PaginaActual != 1 || p.desplazar.Offset.Y != 12 {
		t.Errorf("IrA(0) deja la página %d en y=%g; la web va a la 1 (y=12)", p.PaginaActual, p.desplazar.Offset.Y)
	}
}

// TestLaMiniaturaSinLeerNoDiceSinTexto: abrir ya no espera a leer el texto de todas las
// páginas, así que de algunas no se sabe. Esas no dicen «Sin texto» —sería mentir— y no
// cuentan para decidir que el documento es un escaneo (`Miniaturas` y `sinCapaDeTexto`
// de `Paneles.tsx`).
func TestLaMiniaturaSinLeerNoDiceSinTexto(t *testing.T) {
	c := documentoDePrueba() // la 1 y la 2 con texto, la 3 sin leer
	p := NuevoPanelLateral(traductorES(t), c)
	_ = p.paginas()
	if got := p.miniaturas[3].texto; got != "" {
		t.Errorf("la página sin leer dice %q", got)
	}
	if p.sinCapaDeTexto() {
		t.Error("un documento con texto pasa por escaneo")
	}
	c.Paginas[2].Extracto = new("")
	_ = p.paginas()
	if got, quiero := p.miniaturas[3].texto, traductorES(t)("pdf.page.noText", nil); got != quiero {
		t.Errorf("la página leída y sin texto dice %q y no %q", got, quiero)
	}
	for i := range c.Paginas {
		c.Paginas[i].Extracto = nil
	}
	c.Paginas[0].Extracto = new("")
	if !p.sinCapaDeTexto() {
		t.Error("con la única página leída vacía no se reconoce el escaneo")
	}
}
