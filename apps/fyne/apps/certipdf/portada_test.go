// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"image/color"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// Los guardianes de este paquete corren en la forma Carril, como los de toda la suite
// (ver tema.FormaDeLosGuardianes). Va en un `init` y no en el TestMain porque el de
// este paquete es el de la ventana real, que solo existe con su etiqueta: sin esto
// corrían en Marco y los títulos, un 10 % más grandes, movían la portada 3,6 px.
func init() { tema.FormaDeLosGuardianes() }

// recientesDePrueba son los tres del caso del arnés con que se midió la web.
func recientesDePrueba() []api.File {
	d := func(s string) time.Time { t, _ := time.Parse(time.RFC3339, s); return t }
	return []api.File{
		{ID: "1", Name: "Contrato de arrendamiento.pdf", UpdatedAt: d("2026-09-20T10:00:00Z")},
		{ID: "2", Name: "Factura 0142.pdf", UpdatedAt: d("2026-09-18T10:00:00Z")},
		{ID: "3", Name: "Acta de junio.pdf", UpdatedAt: d("2026-06-30T10:00:00Z")},
	}
}

// portadaMontada monta la pantalla sin documento con la portada y sus recientes.
func portadaMontada(t *testing.T, tam fyne.Size) (*Pantalla, fyne.Window) {
	t.Helper()
	p := NuevaPantalla(traductorES(t))
	p.AlCrear, p.AlElegir, p.AlEscanear = func() {}, func() {}, func() {}
	w := ventanaSinRelleno(p.Objeto())
	w.Resize(tam)
	p.MostrarPortada(nil, "es")
	p.Portada.Lista = recientesDePrueba()
	p.Portada.Recomponer()
	w.Resize(fyne.NewSize(tam.Width, tam.Height+1))
	w.Resize(tam)
	return p, w
}

// TestLaPortadaSeColocaComoEnLaWeb: PortadaPdf y AbrirPdf reales, medidos en Chromium
// con Figtree cargada, CSS1Compat y raíz de 16 px. A 1280, px-6 deja 1232 px y la
// rejilla 1fr/2fr tiene un gap-3 de 12 px. A 360, el texto de soltar ocupa dos líneas
// de 18 px y la zona de abrir mide 248 px; el filtro sigue max-w-[60%].
func TestLaPortadaSeColocaComoEnLaWeb(t *testing.T) {
	for _, c := range []struct {
		ancho                       float32
		acceso, abrir               [4]float32
		recientes, filtroX, filtroW float32
		tarjeta0, tarjeta1          [2]float32
		anchoTarjeta                float32
	}{
		{1280, [4]float32{24, 154, 406.65625, 78}, [4]float32{442.65625, 154, 813.34375, 230}, 424, 1000, 256, [2]float32{24, 472}, [2]float32{232, 472}, 192},
		{360, [4]float32{24, 176, 312, 78}, [4]float32{24, 266, 312, 248}, 554, 148.8125, 187.1875, [2]float32{24, 602}, [2]float32{24, 782}, 312},
	} {
		p, w := portadaMontada(t, fyne.NewSize(c.ancho, 800))
		m := p.Portada.marco
		caja := func(que string, o fyne.CanvasObject, q [4]float32) {
			t.Helper()
			a := absoluta(o)
			if !cerca32b(a.X, q[0]) || !cerca32b(a.Y, q[1]) || !cerca32b(o.Size().Width, q[2]) || !cerca32b(o.Size().Height, q[3]) {
				t.Errorf("a %g, %s está en %v y mide %v; la web: %v", c.ancho, que, a, o.Size(), q)
			}
		}
		caja("la tarjeta de «En blanco»", m.acceso, c.acceso)
		caja("la zona de abrir", m.abrir, c.abrir)
		f := absoluta(p.Portada.filtro)
		if !cerca32b(f.Y, c.recientes) || !cerca32b(f.X, c.filtroX) || !cerca32b(p.Portada.filtro.Size().Width, c.filtroW) {
			t.Errorf("a %g, el filtro está en %v de %g; la web: %g,%g de %g", c.ancho, f, p.Portada.filtro.Size().Width, c.filtroX, c.recientes, c.filtroW)
		}
		if len(m.tarjetas) != 3 {
			t.Fatalf("hay %d tarjetas", len(m.tarjetas))
		}
		caja("la primera tarjeta", m.tarjetas[0], [4]float32{c.tarjeta0[0], c.tarjeta0[1], c.anchoTarjeta, 164})
		caja("la segunda tarjeta", m.tarjetas[1], [4]float32{c.tarjeta1[0], c.tarjeta1[1], c.anchoTarjeta, 164})
		if c.ancho == 1280 {
			if x := absoluta(m.insignia).X; !cerca32b(x, 102.90625) {
				t.Errorf("la insignia está en x=%g; la web, 102,90625", x)
			}
		}
		w.Close()
	}
}

// cerca32b es la tolerancia de una medida que pasa por un rótulo (≤0,17, REGLAS §3).
func cerca32b(a, b float32) bool { return a-b < 0.2 && b-a < 0.2 }

// TestLaTarjetaDiceLoDeLaWeb: el nombre sin «.pdf» y «PDF · 20 sept 2026».
func TestLaTarjetaDiceLoDeLaWeb(t *testing.T) {
	c := nuevaTarjetaPDF(recientesDePrueba()[0], "PDF", "es", nil)
	if c.nombre != "Contrato de arrendamiento" || c.meta != "PDF · 20 sept 2026" {
		t.Errorf("la tarjeta dice %q / %q", c.nombre, c.meta)
	}
}

// TestLosRecientesSeFiltranPorNombre, sin distinguir mayúsculas; sin resultados, el
// estado vacío de la búsqueda con lo escrito.
func TestLosRecientesSeFiltranPorNombre(t *testing.T) {
	if got := Filtrados(recientesDePrueba(), "FACTURA"); len(got) != 1 || got[0].ID != "2" {
		t.Errorf("Filtrados = %+v", got)
	}
	p, w := portadaMontada(t, fyne.NewSize(1280, 800))
	defer w.Close()
	p.Portada.filtro.PonerTexto("zzz")
	v, ok := p.Portada.marco.cuerpo.(*ui.Vacio)
	if !ok || !strings.Contains(v.Titulo, "zzz") {
		t.Errorf("sin resultados el cuerpo es %T %+v", p.Portada.marco.cuerpo, v)
	}
}

// TestElGlifoPDFEsElDeLaWeb: `PdfGlyph` de `DocList.tsx`, los dos trazados y la escala.
// El grosor del sello va aparte: la web lo compensa por la escala del grupo y aquí se
// hereda sin compensar, y las dos cosas tienen que seguir siendo verdad a la vez.
func TestElGlifoPDFEsElDeLaWeb(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/packages/office/src/components/DocList.tsx")
	for _, trozo := range []string{
		`d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8zM14 3v5h5"`,
		`transform="translate(4.4 7.6) scale(0.63)"`,
		`d="M4.5 20.5h15M6.5 17.5h11v-1.8a2 2 0 0 0-1.5-1.9l-2.2-.6a1.3 1.3 0 0 1-.9-1.5l.5-2.9A3.4 3.4 0 0 0 12 3.5a3.4 3.4 0 0 0-3.4 4.3l.5 2.9a1.3 1.3 0 0 1-.9 1.5l-2.2.6a2 2 0 0 0-1.5 1.9Z"`,
	} {
		if !strings.Contains(GlifoPDF, trozo) || !strings.Contains(fuente, trozo) {
			t.Errorf("el glifo y la web no comparten %s", trozo)
		}
	}
	if !strings.Contains(fuente, `stroke-width={grosorDeGlifo(size) / 0.63}`) {
		t.Error("la web ya no compensa el grosor del sello por la escala del grupo: revisa cómo se hereda aquí")
	}
	if strings.Contains(GlifoPDF, "stroke-width") {
		t.Error("GlifoPDF fija un grosor: el del sello se hereda del glifo, que es lo que lo iguala al de la hoja")
	}
}

// TestElBordeEsDiscontinuo: en el lado de arriba, 6 de trazo y 4 de hueco desde el final
// de la esquina (medido en Chromium); la esquina, entera; y el centro, vacío.
func TestElBordeEsDiscontinuo(t *testing.T) {
	const w, h = 800, 200
	for _, c := range []struct {
		x, y float64
		si   bool
	}{
		{20 + 3, 1, true}, {20 + 7, 1, false}, {20 + 13, 1, true}, {3, 10, true}, {400, 100, false}, {1, 100 - 80 + 3, true},
	} {
		if got := ui.EnBordeDiscontinuo(c.x, c.y, w, h, float64(bordeAbrir), float64(radioAbrir)); got != c.si {
			t.Errorf("(%g,%g) en el borde = %v, quiero %v", c.x, c.y, got, c.si)
		}
	}
}

// TestUnFicheroEncimaEnciendeLaZonaDeAbrir: con un fichero arrastrado sobre la ventana,
// la zona de abrir lleva `border-accent bg-selected`, como la web con `encima`; al irse,
// vuelve a `border-border bg-surface`; y con un documento abierto no se enciende nada.
func TestUnFicheroEncimaEnciendeLaZonaDeAbrir(t *testing.T) {
	p, w := portadaMontada(t, fyne.NewSize(1280, 800))
	defer w.Close()
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	mira := func(que, fondo, borde string) {
		t.Helper()
		caja := p.Portada.marco.abrir.(*fyne.Container)
		if got := caja.Objects[0].(*canvas.Rectangle).FillColor; got != tema.Color(fondo, v) {
			t.Errorf("%s: el fondo es %v y no %s", que, got, fondo)
		}
		if got := caja.Objects[1].(*ui.BordeDiscontinuo).Token; got != borde {
			t.Errorf("%s: el borde es %s y no %s", que, got, borde)
		}
	}
	mira("sin nada encima", "surface", "border")
	fyne.DoAndWait(func() { p.ArrastrarEncima(true) })
	mira("con un fichero encima", "selected", "accent")
	fyne.DoAndWait(func() { p.ArrastrarEncima(false) })
	mira("al irse", "surface", "border")
	p.Estado = editor.Listo
	fyne.DoAndWait(func() { p.ArrastrarEncima(true) })
	if p.Portada.Encima {
		t.Error("con un documento abierto la portada se enciende igual")
	}
}

// TestLaCintaLlevaArchivoConLosDos: con los dos ganchos, Inicio empieza por «Archivo»
// con «Crear nuevo» y «Abrir un archivo», y pulsarlos llama a cada uno; sin ganchos, no
// hay grupo (que es lo que mide `TestLaCintaMideLoQueLaWeb`).
func TestLaCintaLlevaArchivoConLosDos(t *testing.T) {
	var nuevo, abrir int
	b := NuevaBarra(identidad)
	b.AlNuevo, b.AlAbrir = func() { nuevo++ }, func() { abrir++ }
	g := gruposDe(b)
	if len(g) == 0 || g[0].Rotulo != "file.menu" || len(g[0].Controles) != 2 {
		t.Fatalf("el primer grupo de Inicio es %+v", g)
	}
	for _, o := range g[0].Controles {
		o.(*Accion).Tapped(nil)
	}
	if nuevo != 1 || abrir != 1 {
		t.Errorf("pulsar «Crear nuevo» y «Abrir un archivo» llamó %d y %d veces", nuevo, abrir)
	}
	b.Elegir(PestAnotar)
	if gs := gruposDe(b); len(gs) > 0 && gs[0].Rotulo == "file.menu" {
		t.Error("«Archivo» va solo en Inicio")
	}
}

// TestLaBandaDeLaTarjetaNoLlevaLinea: en Portada.tsx la banda es `h-24 bg-bg-subtle` SIN
// el `border-b` de la tesela de Docs. Medido en Chromium: la banda llega a la fila 96
// del interior y la siguiente ya es la superficie; en Fyne salía una línea de `border`.
func TestLaBandaDeLaTarjetaNoLlevaLinea(t *testing.T) {
	_, w := portadaMontada(t, fyne.NewSize(1280, 800))
	defer w.Close()
	var tarjeta *tarjetaPDF
	todos(w.Content(), func(o fyne.CanvasObject) {
		if x, ok := o.(*tarjetaPDF); ok && tarjeta == nil {
			tarjeta = x
		}
	})
	if tarjeta == nil {
		t.Fatal("no hay tarjetas")
	}
	img := w.Canvas().Capture()
	a := absoluta(tarjeta)
	x := int(a.X + 60)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	for _, c := range []struct {
		dy    float32
		token string
	}{{1 + 95.5, "bg-subtle"}, {1 + 96.5, "surface"}} {
		got := color.NRGBAModel.Convert(img.At(x, int(a.Y+c.dy))).(color.NRGBA)
		if got != tema.Color(c.token, v) {
			t.Errorf("a %g del borde de la tarjeta se pinta %v; la web pinta `%s`", c.dy, got, c.token)
		}
	}
}

// TestLaPortadaSigueAlTemaEnCaliente: el fondo y las tarjetas se colorean al componer, y
// cambiar de tema con la app abierta (desde esta o desde otra de las trece) tiene que
// recomponerlos. En una ventana real salía el fondo claro con los textos del tema oscuro
// encima. VISTO EN ROJO sin el `AddListener` de NuevaPortada.
func TestLaPortadaSigueAlTemaEnCaliente(t *testing.T) {
	t.Cleanup(func() { tema.Elegir(""); fyne.CurrentApp().Settings().SetTheme(tema.Sytue{}) })
	tema.Elegir("claro")
	fyne.CurrentApp().Settings().SetTheme(tema.Sytue{})
	_, w := portadaMontada(t, fyne.NewSize(1280, 800))
	defer w.Close()
	esquina := func() color.NRGBA {
		c := w.Canvas().Capture()
		r, g, b, a := c.At(c.Bounds().Dx()-4, c.Bounds().Dy()-4).RGBA()
		return color.NRGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
	}
	if got, quiero := esquina(), tema.Color("bg", tema.Claro); got != quiero {
		t.Fatalf("en claro el fondo es %v y el bg %v", got, quiero)
	}
	tema.Elegir("oscuro")
	fyne.CurrentApp().Settings().SetTheme(tema.Sytue{})
	if got, quiero := esquina(), tema.Color("bg", tema.Oscuro); got != quiero {
		t.Errorf("tras pasar a oscuro el fondo es %v y el bg oscuro %v", got, quiero)
	}
}
