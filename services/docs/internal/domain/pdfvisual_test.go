// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks de la comparación visual. Los PDF se generan AQUÍ, en el propio test, y
// lo que se afirma son medidas: dónde cae la caja de una palabra cambiada, cuántos
// píxeles distintos deja un desplazamiento de medio píxel, cuánta memoria cuesta una
// página de 14400 pt. Describir de memoria lo que uno cree que sale es como se da por
// bueno un comparador que marca todo o no marca nada.
package domain

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"math"
	"runtime"
	"strings"
	"testing"
	"time"
)

// paginaVisual es una página de un PDF de prueba.
type paginaVisual struct {
	ancho, alto float64
	contenido   string
}

// texto es un `BT … ET` en Helvetica con la base de la línea en (x, y) de PDF
// (origen abajo-izquierda).
func texto(x, y, cuerpo float64, s string) string {
	return fmt.Sprintf("BT /F1 %g Tf %g %g Td (%s) Tj ET\n", cuerpo, x, y, s)
}

// pdfVisual arma un PDF con las páginas dadas (cada una con su MediaBox).
func pdfVisual(t *testing.T, paginas ...paginaVisual) []byte {
	t.Helper()
	objs := make([]string, 3, 3+2*len(paginas))
	objs[0] = `<< /Type /Catalog /Pages 2 0 R >>`
	// objs[1] es Pages: se rellena abajo.
	objs[2] = `<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>`
	kids := make([]string, 0, len(paginas))
	for i, p := range paginas {
		id := 4 + 2*i
		kids = append(kids, fmt.Sprintf("%d 0 R", id))
		objs = append(objs,
			fmt.Sprintf(`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] /Contents %d 0 R /Resources << /Font << /F1 3 0 R >> >> >>`,
				p.ancho, p.alto, id+1),
			fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(p.contenido), p.contenido),
		)
	}
	objs[1] = fmt.Sprintf(`<< /Type /Pages /Kids [%s] /Count %d >>`, strings.Join(kids, " "), len(paginas))
	return pdfMinimo(t, fmt.Sprintf(`<< /Size %d /Root 1 0 R >>`, len(objs)+1), objs)
}

func abrir(t *testing.T, datos []byte) *PDFDoc {
	t.Helper()
	d, err := LeerPDF(datos)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	return d
}

func comparar(t *testing.T, a, b []byte, opts OpcionesVisual) *ComparacionVisual {
	t.Helper()
	r, err := CompararVisual(context.Background(), abrir(t, a), abrir(t, b), opts)
	if err != nil {
		t.Fatalf("CompararVisual: %v", err)
	}
	return r
}

const a4w, a4h = 595, 842

func a4(contenido string) paginaVisual { return paginaVisual{a4w, a4h, contenido} }

// cajaDeTinta es la caja, en puntos (origen arriba-izquierda), de lo que NO es papel
// en la página 0 del PDF: la medida de referencia contra la que se mide la región.
func cajaDeTinta(t *testing.T, datos []byte, escala float64) RegionVisual {
	t.Helper()
	img, err := abrir(t, datos).RenderizarPaginaRGBA(0, OpcionesRender{Escala: escala})
	if err != nil {
		t.Fatal(err)
	}
	x0, y0, x1, y1 := math.MaxInt, math.MaxInt, -1, -1
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl := pixelEn(img, x, y)
			if r < 200 || g < 200 || bl < 200 {
				x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
			}
		}
	}
	if x1 < 0 {
		t.Fatal("la página de referencia no tiene tinta")
	}
	return RegionVisual{
		X: float64(x0) / escala, Y: float64(y0) / escala,
		Ancho: float64(x1-x0+1) / escala, Alto: float64(y1-y0+1) / escala,
	}
}

func (r RegionVisual) der() float64   { return r.X + r.Ancho }
func (r RegionVisual) abajo() float64 { return r.Y + r.Alto }
func (r RegionVisual) String() string {
	return fmt.Sprintf("[%.1f,%.1f %.1f×%.1f]", r.X, r.Y, r.Ancho, r.Alto)
}

func (r RegionVisual) cubre(o RegionVisual, margen float64) bool {
	return r.X <= o.X+margen && r.Y <= o.Y+margen && r.der() >= o.der()-margen && r.abajo() >= o.abajo()-margen
}

// --- Lo que debe NO marcar ---------------------------------------------------

func TestMismoPDFNoTieneDiferencias(t *testing.T) {
	doc := pdfVisual(t, a4(texto(50, 700, 18, "Hola mundo azul")+texto(50, 600, 12, "Segunda linea")))
	for _, esc := range []float64{0.5, 1, 1.5, 2, 3} {
		r := comparar(t, doc, doc, OpcionesVisual{Escala: esc})
		if !r.Iguales || len(r.Paginas) != 0 || r.Comparadas != 1 || r.Truncado {
			t.Errorf("escala %g: el mismo documento no sale igual: %+v", esc, r)
		}
	}
	// Y la página suelta: 0 píxeles, 0 %, ninguna región, y la imagen existe.
	d, err := CompararVisualPagina(abrir(t, doc), abrir(t, doc), 1, OpcionesVisual{})
	if err != nil {
		t.Fatal(err)
	}
	if d.PixelesDiferentes != 0 || d.Porcentaje != 0 || len(d.Regiones) != 0 || len(d.Imagen) == 0 {
		t.Errorf("página idéntica: %+v", d)
	}
}

// TestUnDesplazamientoSubpixelNoSeMarca es el guardián del antialiasing. Mover una
// fracción de píxel un texto cambia el borde de todas sus letras; sin la tolerancia,
// el comparador marcaría cada glifo de la página.
func TestUnDesplazamientoSubpixelNoSeMarca(t *testing.T) {
	cuerpo := func(dx float64) []byte {
		return pdfVisual(t, a4(texto(50+dx, 700, 14, "Hola mundo, esto es una linea de prueba")+
			texto(50+dx, 660, 10, "Otra linea mas con letras pequenas y cifras 0123456789")))
	}
	base := cuerpo(0)
	// 0,1 pt: milésimas de redondeo entre dos herramientas. A cualquier escala, nada.
	for _, esc := range []float64{1, 1.5, 2} {
		if r := comparar(t, base, cuerpo(0.1), OpcionesVisual{Escala: esc}); !r.Iguales {
			t.Errorf("×%g: 0,1 pt de desplazamiento se marca con los ajustes por defecto: %+v", esc, r.Paginas)
		}
	}
	// 0,3 pt (0,45 px a ×1,5): sin tolerancia es lo que lo marca TODO; con ella, un
	// resto de ruido de borde que es una fracción pequeña.
	movido := cuerpo(0.3)
	estricto := comparar(t, base, movido, OpcionesVisual{Tolerancia: 1, SinVecindad: true})
	if len(estricto.Paginas) != 1 || estricto.Paginas[0].PixelesDiferentes < 1000 {
		t.Fatalf("sin tolerancia un desplazamiento subpíxel debería dejar miles de píxeles: %+v", estricto.Paginas)
	}
	pd := 0
	if con := comparar(t, base, movido, OpcionesVisual{}); len(con.Paginas) == 1 {
		pd = con.Paginas[0].PixelesDiferentes
	}
	if pd*20 > estricto.Paginas[0].PixelesDiferentes {
		t.Errorf("la tolerancia no quita el ruido: %d px con ella contra %d sin ella", pd, estricto.Paginas[0].PixelesDiferentes)
	}
	t.Logf("0,3 pt a ×1,5: %d px sin tolerancia, %d con ella", estricto.Paginas[0].PixelesDiferentes, pd)
}

// TestLaEscalaNoCambiaElVeredicto: el mismo par a distintas escalas dice lo mismo.
func TestLaEscalaNoCambiaElVeredicto(t *testing.T) {
	a := pdfVisual(t, a4(texto(50, 700, 14, "Hola mundo azul")))
	b := pdfVisual(t, a4(texto(50.1, 700, 14, "Hola mundo azul")))
	c := pdfVisual(t, a4(texto(50, 700, 14, "Hola mundo rojo")))
	for _, esc := range []float64{1.5, 2, 3} {
		if r := comparar(t, a, b, OpcionesVisual{Escala: esc}); !r.Iguales {
			t.Errorf("×%g: un desplazamiento de 0,1 pt se marca: %+v", esc, r.Paginas)
		}
		if r := comparar(t, a, c, OpcionesVisual{Escala: esc}); r.Iguales || len(r.Paginas[0].Regiones) != 1 {
			t.Errorf("×%g: una palabra cambiada no sale como una región: %+v", esc, r.Paginas)
		}
	}
}

// TestUnCambioDePuntuacionSeVe: lo opuesto a lo anterior. Si la tolerancia se pasa de
// generosa, una coma que pasa a punto y coma —dos píxeles— desaparece.
func TestUnCambioDePuntuacionSeVe(t *testing.T) {
	a := pdfVisual(t, a4(texto(50, 700, 14, "Hola mundo, esto es una linea de prueba.")))
	b := pdfVisual(t, a4(texto(50, 700, 14, "Hola mundo; esto es una linea de prueba.")))
	r := comparar(t, a, b, OpcionesVisual{})
	if len(r.Paginas) != 1 || len(r.Paginas[0].Regiones) != 1 {
		t.Fatalf("coma por punto y coma no se ve: %+v", r.Paginas)
	}
	// Y cae donde está el signo: tras «Hola mundo» (x ≈ 50 + 80 pt), en la línea de texto.
	if reg := r.Paginas[0].Regiones[0]; reg.X < 100 || reg.X > 140 || reg.Y < 120 || reg.Y > 150 {
		t.Errorf("la región %v no está donde el signo", reg)
	}
}

// --- Lo que debe marcar ------------------------------------------------------

// TestCambiarUnaPalabraDaUnaRegionEnSuSitio mide la caja contra la tinta de las dos
// palabras (la que sale y la que entra), dibujadas cada una sola en su página.
func TestCambiarUnaPalabraDaUnaRegionEnSuSitio(t *testing.T) {
	const esc = 2.0
	linea := func(palabra string) []byte {
		return pdfVisual(t, a4(texto(50, 700, 18, "Hola mundo ")+texto(50, 400, 12, "Pie de pagina que no cambia")+
			texto(150, 700, 18, palabra)))
	}
	solo := func(palabra string) []byte { return pdfVisual(t, a4(texto(150, 700, 18, palabra))) }
	a, b := linea("azul"), linea("verde")

	r := comparar(t, a, b, OpcionesVisual{Escala: esc})
	if len(r.Paginas) != 1 {
		t.Fatalf("esperaba una página con diferencias: %+v", r)
	}
	p := r.Paginas[0]
	if len(p.Regiones) != 1 {
		t.Fatalf("esperaba UNA región y salen %d: %v", len(p.Regiones), p.Regiones)
	}
	reg := p.Regiones[0]
	ta, tb := cajaDeTinta(t, solo("azul"), esc), cajaDeTinta(t, solo("verde"), esc)
	union := RegionVisual{X: math.Min(ta.X, tb.X), Y: math.Min(ta.Y, tb.Y)}
	union.Ancho = math.Max(ta.der(), tb.der()) - union.X
	union.Alto = math.Max(ta.abajo(), tb.abajo()) - union.Y

	// La región cae dentro de la tinta de las dos palabras y cubre la mayor parte...
	if !union.cubre(reg, 0.5) {
		t.Errorf("la región %v se sale de la tinta de las dos palabras %v", reg, union)
	}
	if reg.Ancho < 0.5*union.Ancho || reg.Alto < 0.5*union.Alto {
		t.Errorf("la región %v es mucho menor que lo que cambió %v", reg, union)
	}
	// ...y NO toca «Hola mundo» (termina antes de x=150) ni el pie (y de PDF 400).
	if reg.X < 148 {
		t.Errorf("la región %v invade el texto que no cambió", reg)
	}
	if reg.abajo() > a4h-400-8 {
		t.Errorf("la región %v baja hasta el pie", reg)
	}
	if p.PixelesDiferentes == 0 || p.Porcentaje <= 0 || p.Porcentaje > 1 {
		t.Errorf("porcentaje fuera de lo esperable para una palabra: %d px, %.4f %%", p.PixelesDiferentes, p.Porcentaje)
	}
	t.Logf("región %v · tinta de las dos palabras %v · %d px (%.4f %%)", reg, union, p.PixelesDiferentes, p.Porcentaje)
}

func TestDosCambiosLejanosSonDosRegionesEnOrdenDeLectura(t *testing.T) {
	a := pdfVisual(t, a4(texto(50, 700, 14, "Arriba uno")+texto(300, 200, 14, "Abajo dos")))
	b := pdfVisual(t, a4(texto(50, 700, 14, "Arriba XX")+texto(300, 200, 14, "Abajo YY")))
	r := comparar(t, a, b, OpcionesVisual{})
	regs := r.Paginas[0].Regiones
	if len(regs) != 2 {
		t.Fatalf("esperaba 2 regiones: %v", regs)
	}
	if regs[0].Y >= regs[1].Y {
		t.Errorf("no van en orden de lectura: %v", regs)
	}
}

func TestPalabrasContiguasDeUnaLineaSeFundenEnUnaRegion(t *testing.T) {
	a := pdfVisual(t, a4(texto(50, 700, 14, "uno dos tres cuatro")))
	b := pdfVisual(t, a4(texto(50, 700, 14, "UNO DOS TRES CUATRO")))
	if n := len(comparar(t, a, b, OpcionesVisual{}).Paginas[0].Regiones); n != 1 {
		t.Errorf("una línea cambiada entera son %d regiones, y debería ser una", n)
	}
}

func TestMuchasDiferenciasNoDanMasDeLasRegionesDelTope(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 400; i++ { // 400 cuadrados sueltos, a un salto de distancia
		fmt.Fprintf(&sb, "0 0 0 rg %d %d 4 4 re f\n", 20+(i%20)*28, 20+(i/20)*38)
	}
	a := pdfVisual(t, a4(""))
	b := pdfVisual(t, a4(sb.String()))
	r := comparar(t, a, b, OpcionesVisual{})
	if len(r.Paginas) != 1 {
		t.Fatalf("no se detectó la diferencia: %+v", r)
	}
	n := len(r.Paginas[0].Regiones)
	if n == 0 || n > maxRegionesVisual {
		t.Errorf("%d regiones: debían ser entre 1 y %d", n, maxRegionesVisual)
	}
}

func TestColorDistintoSeMarcaYUnTonoCasiIgualNo(t *testing.T) {
	caja := func(c string) []byte { return pdfVisual(t, a4(c+" 100 600 200 100 re f")) }
	rojo, otroRojo, azul := caja("1 0 0 rg"), caja("0.96 0 0 rg"), caja("0 0 1 rg")
	if r := comparar(t, rojo, otroRojo, OpcionesVisual{}); !r.Iguales {
		t.Errorf("0,96 contra 1 de rojo (10 niveles) debería caber en la tolerancia: %+v", r.Paginas)
	}
	r := comparar(t, rojo, azul, OpcionesVisual{})
	if r.Iguales || len(r.Paginas[0].Regiones) != 1 {
		t.Fatalf("rojo contra azul tiene que salir: %+v", r.Paginas)
	}
	reg := r.Paginas[0].Regiones[0]
	// 100..300 en X; la Y de PDF 600..700 es 142..242 desde arriba.
	esperada := RegionVisual{X: 100, Y: a4h - 700, Ancho: 200, Alto: 100}
	if !reg.cubre(esperada, 1) || !esperada.cubre(reg, 1) {
		t.Errorf("la caja medida %v no es la dibujada %v", reg, esperada)
	}
}

// --- Documentos que no encajan ----------------------------------------------

func TestPaginaAnadidaSeDiceYNoRompe(t *testing.T) {
	uno := pdfVisual(t, a4(texto(50, 700, 14, "Pagina uno")))
	dos := pdfVisual(t, a4(texto(50, 700, 14, "Pagina uno")), a4(texto(50, 700, 14, "Pagina dos")))
	for _, par := range [][2][]byte{{uno, dos}, {dos, uno}} {
		r := comparar(t, par[0], par[1], OpcionesVisual{})
		if r.Iguales || len(r.Paginas) != 1 || r.Paginas[0].Pagina != 2 || r.Paginas[0].SoloEn == "" {
			t.Fatalf("la página añadida no se dice: %+v", r)
		}
		if r.PaginasA+r.PaginasB != 3 || r.Comparadas != 2 {
			t.Errorf("recuentos: %+v", r)
		}
	}
	if r := comparar(t, uno, dos, OpcionesVisual{}); r.Paginas[0].SoloEn != "b" {
		t.Errorf("la página 2 solo está en B y dice %q", r.Paginas[0].SoloEn)
	}
	if r := comparar(t, dos, uno, OpcionesVisual{}); r.Paginas[0].SoloEn != "a" {
		t.Errorf("la página 2 solo está en A y dice %q", r.Paginas[0].SoloEn)
	}
}

func TestTamanosDistintosSeCompararSobreLaUnion(t *testing.T) {
	letter := paginaVisual{612, 792, ""}
	// Dos páginas en blanco de tamaños distintos: iguales, pero el tamaño se dice.
	r := comparar(t, pdfVisual(t, a4("")), pdfVisual(t, letter), OpcionesVisual{})
	if len(r.Paginas) != 0 {
		t.Errorf("dos páginas en blanco difieren por su tamaño: %+v", r.Paginas)
	}
	// Con contenido en la franja que una página no cubre, sí hay diferencia, y se
	// dice que los tamaños no coinciden y cuál es el lienzo.
	ancha := paginaVisual{612, 842, "0 0 0 rg 600 100 10 10 re f"}
	r = comparar(t, pdfVisual(t, a4("")), pdfVisual(t, ancha), OpcionesVisual{})
	if len(r.Paginas) != 1 {
		t.Fatalf("la franja que A no cubre tiene tinta en B y no se marca: %+v", r)
	}
	p := r.Paginas[0]
	if !p.TamanosDistintos || p.Ancho != 612 || p.Alto != 842 || p.AnchoA != 595 || p.AnchoB != 612 {
		t.Errorf("no declara los tamaños: %+v", p)
	}
	if reg := p.Regiones[0]; reg.X < 598 {
		t.Errorf("la región %v no está en la franja de x ≥ 600", reg)
	}
	// Una página girada 90° mide al revés y tampoco rompe.
	girada := pdfVisual(t, paginaVisual{842, 595, ""})
	if r := comparar(t, pdfVisual(t, a4("")), girada, OpcionesVisual{}); len(r.Paginas) != 0 {
		t.Errorf("A4 vertical y horizontal en blanco: %+v", r.Paginas)
	}
}

func TestElTopeDePaginasYElContextoTruncan(t *testing.T) {
	p := a4(texto(50, 700, 14, "x"))
	tres := pdfVisual(t, p, p, p)
	r := comparar(t, tres, tres, OpcionesVisual{MaxPaginas: 2})
	if !r.Truncado || r.Comparadas != 2 || r.Iguales {
		t.Errorf("tope de páginas: %+v", r)
	}
	ctx, cancelar := context.WithCancel(context.Background())
	cancelar()
	doc := abrir(t, tres)
	r2, err := CompararVisual(ctx, doc, doc, OpcionesVisual{})
	if err != nil || !r2.Truncado || r2.Comparadas != 0 || r2.Iguales {
		t.Errorf("contexto vencido: %+v (%v)", r2, err)
	}
}

func TestCompararPaginaFueraDeRangoFalla(t *testing.T) {
	d := abrir(t, pdfVisual(t, a4("")))
	for _, n := range []int{0, 2, -1} {
		if _, err := CompararVisualPagina(d, d, n, OpcionesVisual{}); err == nil {
			t.Errorf("la página %d no existe y no falla", n)
		}
	}
}

// TestElResaltadoEsUnPNGConLoQueCambio: la imagen mide el lienzo a la escala dicha y
// lleva rojo donde había tinta en A y verde donde la hay en B.
func TestElResaltadoEsUnPNGConLoQueCambio(t *testing.T) {
	a := abrir(t, pdfVisual(t, a4("0 0 0 rg 100 600 100 50 re f")))
	b := abrir(t, pdfVisual(t, a4("0 0 0 rg 300 600 100 50 re f")))
	d, err := CompararVisualPagina(a, b, 1, OpcionesVisual{Escala: 1})
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(d.Imagen))
	if err != nil {
		t.Fatalf("la imagen no es un PNG: %v", err)
	}
	if img.Bounds().Dx() != a4w || img.Bounds().Dy() != a4h {
		t.Errorf("mide %v y debería medir %d×%d", img.Bounds().Size(), a4w, a4h)
	}
	rojo, verde := image.Pt(150, a4h-625), image.Pt(350, a4h-625)
	if r, g, _, _ := img.At(rojo.X, rojo.Y).RGBA(); r>>8 < 200 || g>>8 > 80 {
		t.Errorf("donde estaba la caja de A debería haber rojo: %v", img.At(rojo.X, rojo.Y))
	}
	if r, g, _, _ := img.At(verde.X, verde.Y).RGBA(); g>>8 < 140 || r>>8 > 80 {
		t.Errorf("donde está la caja de B debería haber verde: %v", img.At(verde.X, verde.Y))
	}
	if len(d.Regiones) != 2 {
		t.Errorf("dos cajas lejanas deberían ser dos regiones: %v", d.Regiones)
	}
}

// --- Entrada hostil ----------------------------------------------------------

// TestUnaPaginaDe14400PuntosSeCompararBajandoLaEscala: 14400 pt es el máximo que admite
// la especificación (200 pulgadas). A ×1,5 serían 466 Mpx; se compara, pero a una escala
// que cabe, y la memoria se mide en vez de suponerse.
func TestUnaPaginaDe14400PuntosSeCompararBajandoLaEscala(t *testing.T) {
	enorme := func(c string) []byte {
		return pdfVisual(t, paginaVisual{14400, 14400, c})
	}
	a, b := enorme("0 0 0 rg 1000 1000 3000 3000 re f"), enorme("0 0 0 rg 1000 1000 3000 4000 re f")
	da, db := abrir(t, a), abrir(t, b)

	var antes, despues runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&antes)
	ini := time.Now()
	r, err := CompararVisual(context.Background(), da, db, OpcionesVisual{Escala: 3})
	dur := time.Since(ini)
	runtime.ReadMemStats(&despues)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Paginas) != 1 || r.Paginas[0].Omitida {
		t.Fatalf("la página enorme no se comparó: %+v", r.Paginas)
	}
	p := r.Paginas[0]
	if p.Escala >= 3 || p.PixelesTotales > maxPixelesVisual+4*int(math.Ceil(p.Ancho*p.Escala)) {
		t.Errorf("escala %g con %d píxeles: no respeta el tope de %d", p.Escala, p.PixelesTotales, maxPixelesVisual)
	}
	// La región está en puntos de PÁGINA aunque se compare a ×0,1: x de 1000 a 4000,
	// y entre 14400-5000 y 14400-4000 desde arriba (la franja añadida: 3000 × 1000).
	if len(p.Regiones) != 1 {
		t.Fatalf("regiones: %v", p.Regiones)
	}
	esp := RegionVisual{X: 1000, Y: 14400 - 5000, Ancho: 3000, Alto: 1000}
	if reg := p.Regiones[0]; math.Abs(reg.X-esp.X) > 40 || math.Abs(reg.Y-esp.Y) > 40 ||
		math.Abs(reg.Ancho-esp.Ancho) > 80 || math.Abs(reg.Alto-esp.Alto) > 80 {
		t.Errorf("región %v, esperada ~%v", p.Regiones[0], esp)
	}
	asignado := (despues.TotalAlloc - antes.TotalAlloc) >> 20
	t.Logf("14400×14400 pt: escala %.3f, %d px, %d MiB asignados, %v", p.Escala, p.PixelesTotales, asignado, dur)
	if asignado > 200 {
		t.Errorf("una página de 14400 pt asignó %d MiB: el tope de píxeles no está acotando", asignado)
	}
}

func TestUnaPaginaAbsurdaSeOmiteSinReservarNada(t *testing.T) {
	for _, medida := range []float64{1e7, 1e12} {
		doc := pdfVisual(t, paginaVisual{medida, medida, ""})
		var antes, despues runtime.MemStats
		runtime.ReadMemStats(&antes)
		r := comparar(t, doc, doc, OpcionesVisual{})
		runtime.ReadMemStats(&despues)
		if len(r.Paginas) != 1 || !r.Paginas[0].Omitida || r.Paginas[0].Motivo != OmitidaDemasiadoGrande || r.Iguales {
			t.Errorf("%g pt: %+v", medida, r.Paginas)
		}
		if mib := (despues.TotalAlloc - antes.TotalAlloc) >> 20; mib > 20 {
			t.Errorf("%g pt asignó %d MiB antes de rendirse", medida, mib)
		}
	}
}

func TestLosAjustesFueraDeRangoSeAcotan(t *testing.T) {
	o := OpcionesVisual{Escala: 99, Tolerancia: 9999, Vecindad: 50, MaxPaginas: 1 << 20}.normalizadas()
	if o.Escala != maxEscalaVisual || o.Tolerancia != 255 || o.Vecindad != maxVecindadVisual || o.MaxPaginas != maxPaginasVisual {
		t.Errorf("sin acotar: %+v", o)
	}
	o = OpcionesVisual{Escala: math.NaN()}.normalizadas()
	if o.Escala != escalaVisualPorDefecto {
		t.Errorf("NaN no cae al defecto: %+v", o)
	}
}
