// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Miniatura de la portada: la primera página, hoja o diapositiva de un documento
// dibujada en pequeño, para que la lista de documentos enseñe QUÉ hay dentro y no
// solo de qué tipo es. El glifo del tipo sigue siendo el respaldo de la web mientras
// un documento no tiene miniatura (recién creado, vacío o de otra organización).
//
// Cada tipo sale por el camino que mejor lo representa con lo que ya existe:
//
//   - El DOCUMENTO pasa por el PDF accesible (export_pdf_ua.go) y el raster de PDF
//     (pdfraster.go): encabezados, listas, tablas e imágenes con la letra de la marca,
//     la misma maquetación que la descarga en PDF. Solo los primeros bloques: para la
//     franja superior de la primera página no hace falta maquetar un libro entero.
//   - La HOJA y la PRESENTACIÓN se dibujan aquí, directamente en un mapa de bits: el
//     PDF de una hoja es una tabla en un folio y el de una presentación, su texto en
//     orden de lectura; ninguno de los dos se parece a lo que la persona ve en Calcus
//     o en Diapos. Una rejilla con cabeceras de fila y columna, y la diapositiva con su
//     fondo, sus formas, sus imágenes y su texto en su sitio, sí.
//
// La letra es la de los PDF (pdffuente: Figtree y, de reserva, Liberation Sans para el
// griego y el cirílico), así que una miniatura en búlgaro no sale con cuadros vacíos.
package model

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
	"strconv"
	"strings"
	"sync"

	// Los decodificadores de las imágenes que puede llevar una diapositiva.
	_ "image/gif"
	_ "image/jpeg"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/pdfdoc"
	"git.sytue.eu/sytue/one/apps/nucleo/pdffuente"
)

// MiniaturaAncho es el ancho en píxeles de toda miniatura. La tesela más ancha de la
// portada mide unos 260 px CSS; 480 cubre una pantalla de densidad doble sin que el PNG
// pase de unas decenas de kilobytes.
const MiniaturaAncho = 480

// miniaturaAltoDocumento es el alto del recorte de un documento: la franja superior de
// la primera página (4:3), que es la que la tesela enseña.
const miniaturaAltoDocumento = 360

// miniaturaAltoHoja es el alto de la de una hoja (16:9, como la tesela la recorta).
const miniaturaAltoHoja = 270

// miniaturaMaxBloques acota lo que se maqueta de un documento: de sobra para llenar la
// primera página, y un documento de cien páginas no cuesta cien páginas.
const miniaturaMaxBloques = 60

// maxPixelesImagenMiniatura acota una imagen de una diapositiva ANTES de decodificarla:
// un PNG de pocos kilobytes puede declarar 50 000 × 50 000 píxeles.
const maxPixelesImagenMiniatura = 24 << 20

// ErrMiniaturaTipo: el tipo de documento no tiene miniatura.
var ErrMiniaturaTipo = errors.New("docs: tipo de documento sin miniatura")

// Miniatura dibuja la vista previa de un documento como PNG de MiniaturaAncho píxeles de
// ancho. Es determinista: el mismo estado da los mismos bytes.
func Miniatura(kind Kind, s *State) ([]byte, error) {
	var (
		img *image.RGBA
		err error
	)
	switch kind {
	case KindDocument:
		img, err = miniaturaDocumento(s)
	case KindSpreadsheet:
		img = miniaturaHoja(s)
	case KindPresentation:
		img = miniaturaDiapositiva(s)
	default:
		return nil, ErrMiniaturaTipo
	}
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img); err != nil {
		return nil, errors.New("docs: no se pudo codificar la miniatura")
	}
	return buf.Bytes(), nil
}

// --- Documento -------------------------------------------------------------------

func miniaturaDocumento(s *State) (*image.RGBA, error) {
	d := pdfdoc.Nuevo(pdfdoc.Opciones{})
	bloques := documentBlocksPDF(s)
	if len(bloques) > miniaturaMaxBloques {
		bloques = bloques[:miniaturaMaxBloques]
	}
	var inf InformePDF
	uaBloques(d, bloques, &inf)
	pdf, err := d.Bytes()
	if err != nil {
		return nil, err
	}
	doc, err := LeerPDF(pdf)
	if err != nil {
		return nil, err
	}
	p, err := doc.Pagina(0)
	if err != nil {
		return nil, err
	}
	pagina, err := doc.RenderizarPaginaRGBA(0, OpcionesRender{Escala: MiniaturaAncho / p.Ancho})
	if err != nil {
		return nil, err
	}
	// El raster deja transparente lo que no se pinta; el folio es blanco.
	out := lienzoMiniatura(MiniaturaAncho, miniaturaAltoDocumento, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	draw.Draw(out, out.Bounds(), pagina, image.Point{}, draw.Over)
	return out, nil
}

// --- Hoja ------------------------------------------------------------------------

// Medidas del lienzo de Calcus (en px del editor) y la escala a la que se dibujan.
const (
	hojaEscala        = 0.75
	hojaAnchoCabecera = 40.0 // la columna de los números de fila
	hojaTamLetra      = 13.0
)

// Colores de la rejilla: los neutros del lienzo de Calcus en claro (la miniatura es una
// foto del documento, no parte del cromo, y no cambia con el tema de quien la mira).
var (
	hojaFondo     = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	hojaCabecera  = color.NRGBA{R: 245, G: 246, B: 248, A: 255}
	hojaLinea     = color.NRGBA{R: 226, G: 229, B: 234, A: 255}
	hojaTintaCab  = color.NRGBA{R: 107, G: 114, B: 128, A: 255}
	hojaTinta     = color.NRGBA{R: 10, G: 10, B: 10, A: 255}
	slidesTintaNo = color.NRGBA{R: 148, G: 163, B: 184, A: 255}
)

func miniaturaHoja(s *State) *image.RGBA {
	img := lienzoMiniatura(MiniaturaAncho, miniaturaAltoHoja, hojaFondo)
	r := nuevaRejilla(sheetModelOrEmpty(s))
	r.caras = nuevasCaras()
	ancho, alto := float64(MiniaturaAncho), float64(miniaturaAltoHoja)
	k := hojaEscala
	cabW, cabH := hojaAnchoCabecera*k, altoSeriePx*k
	tam := hojaTamLetra * k

	// Columnas y filas que caben.
	for x, c := cabW, 0; x < ancho && c < 64; c++ {
		r.xs = append(r.xs, x)
		x += r.anchoCol(c)
	}
	for y, f := cabH, 0; y < alto && f < 64; f++ {
		r.ys = append(r.ys, y)
		y += r.altoFila(f)
	}

	// Primero los rellenos, luego la rejilla y, encima, el texto.
	for f, y := range r.ys {
		for c, x := range r.xs {
			if col, ok := colorCSS(r.sd.Styles[A1Ref(f+1, c+1)].Fill); ok {
				rellenarRect(img, x, y, r.anchoCol(c), r.altoFila(f), col)
			}
		}
	}
	for _, x := range r.xs {
		rellenarRect(img, x, 0, 1, alto, hojaLinea)
	}
	for _, y := range r.ys {
		rellenarRect(img, 0, y, ancho, 1, hojaLinea)
	}
	if r.eng != nil {
		for f := range r.ys {
			for c := range r.xs {
				r.escribirCelda(img, f, c, tam)
			}
		}
	}

	// Cabeceras de columna (A, B, C…) y de fila (1, 2, 3…), encima de todo.
	rellenarRect(img, 0, 0, ancho, cabH, hojaCabecera)
	rellenarRect(img, 0, 0, cabW, alto, hojaCabecera)
	normal := r.caras.de(false, false)
	for c, x := range r.xs {
		escribirRecortado(img, normal, tam*0.9, ColumnToLetters(c+1), x, 0, r.anchoCol(c), cabH, "center", hojaTintaCab)
		rellenarRect(img, x, 0, 1, cabH, hojaLinea)
	}
	for f, y := range r.ys {
		escribirRecortado(img, normal, tam*0.9, strconv.Itoa(f+1), 0, y, cabW, r.altoFila(f), "center", hojaTintaCab)
		rellenarRect(img, 0, y, cabW, 1, hojaLinea)
	}
	rellenarRect(img, 0, cabH, ancho, 1, hojaLinea)
	rellenarRect(img, cabW, 0, 1, alto, hojaLinea)
	return img
}

// rejillaHoja es la hoja activa ya medida: sus celdas, su motor de fórmulas y dónde
// empieza cada columna y cada fila en la miniatura.
type rejillaHoja struct {
	sd     sheetData
	nombre string
	eng    *formula.Engine // nil = libro sin hojas
	xs, ys []float64       // borde izquierdo / superior de cada columna y fila
	caras  *pilaCaras      // las de ESTA miniatura (ver pilaCaras)
}

func nuevaRejilla(m *sheetModel) *rejillaHoja {
	r := &rejillaHoja{}
	if len(m.Sheets) == 0 {
		return r
	}
	activa := m.Active
	if activa < 0 || activa >= len(m.Sheets) {
		activa = 0
	}
	e, nombres := motorDelLibro(m)
	r.sd, r.nombre, r.eng = m.Sheets[activa], nombres[activa], e
	return r
}

func (r *rejillaHoja) altoFila(f int) float64 { // f 0-based
	if px, ok := medidaValida(r.sd.RowHeights[strconv.Itoa(f)], maxAltoPx); ok {
		return px * hojaEscala
	}
	return altoSeriePx * hojaEscala
}

func (r *rejillaHoja) anchoCol(c int) float64 {
	if px, ok := medidaValida(r.sd.ColWidths[strconv.Itoa(c)], maxAnchoPx); ok {
		return px * hojaEscala
	}
	return anchoSeriePx * hojaEscala
}

// escribirCelda pinta el VALOR de una celda (no su fórmula) con su formato.
func (r *rejillaHoja) escribirCelda(img *image.RGBA, f, c int, tam float64) {
	ref := A1Ref(f+1, c+1)
	if r.sd.Cells[ref] == "" {
		return
	}
	texto := strings.TrimSpace(cellShown(r.eng, r.nombre, r.sd, ref))
	if texto == "" {
		return
	}
	k := hojaEscala
	x, y, h := r.xs[c], r.ys[f], r.altoFila(f)
	est := r.sd.Styles[ref]
	partes := formula.LeerCelda(r.sd.Cells[ref])
	tinta := hojaTinta
	if col, ok := colorCSS(est.Color); ok {
		tinta = col
	}
	if est.Size > 0 {
		tam = math.Min(float64(est.Size)*k*96/72, h*0.9)
	}
	alinear := partes.Alinear
	if alinear == "" && pareceNumero(texto) {
		alinear = "right"
	}
	cara := r.caras.de(partes.Negrita, partes.Cursiva)
	// Un texto alineado a la izquierda se desborda sobre las celdas vacías de su derecha,
	// como en el lienzo de Calcus: «Presupuesto mensual» en A1 se lee entero, y la línea
	// de la rejilla que cruza no se ve.
	w := r.anchoCol(c)
	for sig := c + 1; alinear == "" && sig < len(r.xs) && r.sd.Cells[A1Ref(f+1, sig+1)] == "" && cara.medir(tam, texto) > w-6*k; sig++ {
		fondo, ok := colorCSS(r.sd.Styles[A1Ref(f+1, sig+1)].Fill)
		if !ok {
			fondo = hojaFondo
		}
		rellenarRect(img, r.xs[sig], y+1, 1, h-1, fondo)
		w += r.anchoCol(sig)
	}
	escribirRecortado(img, cara, tam, texto, x+3*k, y, w-6*k, h, alinear, tinta)
}

// pareceNumero: Calcus alinea a la derecha lo que es un número, como toda hoja.
func pareceNumero(s string) bool {
	s = strings.TrimSpace(strings.Trim(s, "€$£%"))
	s = strings.ReplaceAll(strings.ReplaceAll(s, ".", ""), ",", ".")
	_, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return err == nil
}

// --- Presentación ----------------------------------------------------------------

// diapoAncho es el ancho del escenario del editor: las cajas se miden en % de él y la
// letra en px de él (SlidesCanvas.tsx).
const diapoAncho = 960.0

func miniaturaDiapositiva(s *State) *image.RGBA {
	m := slidesModelOf("", s)
	alto := int(math.Round(MiniaturaAncho / m.slideAspect()))
	th := m.theme()
	fondo, ok := colorCSS("#" + th.Bg)
	if !ok {
		fondo = hojaFondo
	}
	tinta, ok := colorCSS("#" + th.Ink)
	if !ok {
		tinta = hojaTinta
	}
	if len(m.Slides) == 0 {
		return lienzoMiniatura(MiniaturaAncho, alto, fondo)
	}
	sl := m.Slides[0]
	if c, ok := colorCSS(sl.Bg); ok {
		fondo = c
	}
	img := lienzoMiniatura(MiniaturaAncho, alto, fondo)
	var cajas []slideBox
	if m.Master != nil && !sl.HideMaster {
		cajas = append(cajas, m.Master.Boxes...)
	}
	cajas = append(cajas, sl.Boxes...)
	caras := nuevasCaras()
	for _, b := range cajas {
		dibujarCaja(img, caras, b, float64(MiniaturaAncho), float64(alto), tinta)
	}
	return img
}

func dibujarCaja(img *image.RGBA, caras *pilaCaras, b slideBox, w, h float64, tinta color.NRGBA) {
	x, y, cw, ch := b.X/100*w, b.Y/100*h, b.W/100*w, b.H/100*h
	if cw <= 0 || ch <= 0 {
		return
	}
	k := w / diapoAncho
	switch b.Kind {
	case "image":
		if !dibujarImagen(img, b.Src, x, y, cw, ch) {
			rellenarRect(img, x, y, cw, ch, slidesTintaNo)
		}
		return
	case "media", "chart", "smartart":
		// Un marcador sobrio, como el del escenario: la miniatura no reproduce ni calcula.
		if b.Kind == "media" && dibujarImagen(img, b.Poster, x, y, cw, ch) {
			return
		}
		rellenarRect(img, x, y, cw, ch, conAlfa(slidesTintaNo, 90))
		return
	case "shape":
		if c, ok := colorCSS("#" + shapeColor(b.Fill, shapeFillDefault)); ok && b.Shape != "line" {
			rellenarForma(img, b.Shape, x, y, cw, ch, c)
		}
		if c, ok := colorCSS("#" + shapeColor(b.Stroke, shapeStrokeDefault)); ok && b.Shape == "line" {
			rellenarRect(img, x, y+ch/2, cw, math.Max(1, 2*k), c)
		}
		// El texto de una forma va centrado en vertical (anchor="ctr" en el .pptx).
		escribirCaja(img, caras, b.HTML, x, y, cw, ch, k, tinta, true)
		return
	}
	escribirCaja(img, caras, b.HTML, x, y, cw, ch, k, tinta, false)
}

// lineaDiapo es una línea ya partida del texto de una caja.
type lineaDiapo struct {
	texto  string
	cara   *caraMiniatura
	tam    float64
	tinta  color.NRGBA
	alinea string
}

func escribirCaja(img *image.RGBA, caras *pilaCaras, htmlStr string, x, y, w, h, k float64, tinta color.NRGBA, centrar bool) {
	if strings.TrimSpace(htmlStr) == "" {
		return
	}
	var lineas []lineaDiapo
	for _, blk := range bloquesDeCaja(htmlStr) {
		if blk.para == nil {
			continue
		}
		p := *blk.para
		tam := float64(slideRunSize(p.style)) / 100
		negrita := strings.HasPrefix(p.style, "Heading")
		col := tinta
		var cursiva bool
		for _, r := range p.runs {
			if r.tam > 0 {
				tam = r.tam
			}
			if r.b {
				negrita = true
			}
			cursiva = cursiva || r.i
			if c, ok := colorCSS("#" + r.color); ok && r.color != "" {
				col = c
			}
			break // el primer tramo manda: a este tamaño no se distinguen tramos
		}
		texto := strings.TrimSpace(p.plain())
		if p.list == "bullet" {
			texto = "• " + texto
		}
		cara := caras.de(negrita, cursiva)
		px := tam * k
		for _, l := range partirLineas(cara, px, texto, w) {
			alinea := p.align
			if alinea == "" && centrar {
				alinea = "center" // el texto de una forma va centrado, como en el escenario
			}
			lineas = append(lineas, lineaDiapo{texto: l, cara: cara, tam: px, tinta: col, alinea: alinea})
		}
	}
	total := 0.0
	for _, l := range lineas {
		total += l.tam * 1.2
	}
	cy := y
	if centrar {
		cy = y + (h-total)/2
	}
	for _, l := range lineas {
		if cy > float64(img.Bounds().Dy()) {
			break
		}
		escribirLinea(img, l.cara, l.tam, l.texto, x, cy, w, l.tam*1.2, l.alinea, l.tinta)
		cy += l.tam * 1.2
	}
}

// dibujarImagen pinta una imagen embebida (data: URL) estirada a la caja, como el
// escenario. Una URL externa no se descarga nunca: false y la caja sale como marcador.
func dibujarImagen(img *image.RGBA, src string, x, y, w, h float64) bool {
	_, datos, ok := decodeDataURL(src)
	if !ok {
		return false
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(datos))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxPixelesImagenMiniatura {
		return false
	}
	src2, _, err := image.Decode(bytes.NewReader(datos))
	if err != nil {
		return false
	}
	dst := image.Rect(int(math.Round(x)), int(math.Round(y)), int(math.Round(x+w)), int(math.Round(y+h)))
	draw.ApproxBiLinear.Scale(img, dst, src2, src2.Bounds(), draw.Over, nil)
	return true
}

// --- Dibujo ----------------------------------------------------------------------

func lienzoMiniatura(w, h int, fondo color.NRGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(fondo), image.Point{}, draw.Src)
	return img
}

func conAlfa(c color.NRGBA, a uint8) color.NRGBA { c.A = a; return c }

// colorCSS lee «#RGB», «#RRGGBB» o «rgb(…)»; false si no es un color (vacío,
// «transparent», un nombre).
func colorCSS(v string) (color.NRGBA, bool) {
	hex := normalizeColor(v)
	if hex == "" {
		return color.NRGBA{}, false
	}
	n, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return color.NRGBA{}, false
	}
	return color.NRGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 255}, true //nolint:gosec // 24 bits por construcción
}

func rellenarRect(img *image.RGBA, x, y, w, h float64, c color.NRGBA) {
	r := image.Rect(int(math.Floor(x)), int(math.Floor(y)), int(math.Ceil(x+w)), int(math.Ceil(y+h))).Intersect(img.Bounds())
	if r.Empty() {
		return
	}
	draw.Draw(img, r, image.NewUniform(c), image.Point{}, draw.Over)
}

// rellenarForma dibuja las formas de Diapos que se reconocen a este tamaño (elipse,
// triángulo, rombo); el resto, con su caja.
func rellenarForma(img *image.RGBA, forma string, x, y, w, h float64, c color.NRGBA) {
	var puntos [][2]float64
	switch forma {
	case "ellipse":
		const n = 48
		for i := range n {
			a := 2 * math.Pi * float64(i) / n
			puntos = append(puntos, [2]float64{x + w/2 + w/2*math.Cos(a), y + h/2 + h/2*math.Sin(a)})
		}
	case "triangle":
		puntos = [][2]float64{{x + w/2, y}, {x + w, y + h}, {x, y + h}}
	case "diamond":
		puntos = [][2]float64{{x + w/2, y}, {x + w, y + h/2}, {x + w/2, y + h}, {x, y + h/2}}
	default:
		rellenarRect(img, x, y, w, h, c)
		return
	}
	b := img.Bounds()
	z := vector.NewRasterizer(b.Dx(), b.Dy())
	z.MoveTo(float32(puntos[0][0]), float32(puntos[0][1]))
	for _, p := range puntos[1:] {
		z.LineTo(float32(p[0]), float32(p[1]))
	}
	z.ClosePath()
	z.Draw(img, b, image.NewUniform(c), image.Point{})
}

// --- Texto -----------------------------------------------------------------------

// caraMiniatura es un estilo de la pila de pdffuente (la marca y su reserva).
type caraMiniatura struct {
	fuentes []*pdffuente.Fuente
	mu      sync.Mutex
	caras   map[[2]int]font.Face // [índice de fuente, tamaño en 1/8 px]
}

// pilaCaras son los cuatro estilos de UNA miniatura. No se comparte entre miniaturas:
// una cara de opentype guarda su búfer y su rasterizador dentro y no admite uso
// concurrente, y el servicio dibuja varias a la vez (http/miniaturas.go). Crear las
// caras es barato; las fuentes (pdffuente), que sí se pueden compartir, ya vienen
// cargadas.
type pilaCaras struct {
	estilo [4]*caraMiniatura
}

func nuevasCaras() *pilaCaras {
	p := &pilaCaras{}
	for i := range 4 {
		p.estilo[i] = &caraMiniatura{fuentes: pdffuente.Pila(i&1 != 0, i&2 != 0), caras: map[[2]int]font.Face{}}
	}
	return p
}

func (p *pilaCaras) de(negrita, cursiva bool) *caraMiniatura {
	i := 0
	if negrita {
		i |= 1
	}
	if cursiva {
		i |= 2
	}
	return p.estilo[i]
}

// cara devuelve la de la primera fuente de la pila que tiene el carácter.
func (c *caraMiniatura) cara(r rune, tam float64) font.Face {
	idx := 0
	for i, f := range c.fuentes {
		if f.Glifo(r) != 0 {
			idx = i
			break
		}
	}
	clave := [2]int{idx, int(math.Round(tam * 8))}
	c.mu.Lock()
	defer c.mu.Unlock()
	if f, ok := c.caras[clave]; ok {
		return f
	}
	f, err := opentype.NewFace(c.fuentes[idx].SFNT(), &opentype.FaceOptions{Size: float64(clave[1]) / 8, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil
	}
	c.caras[clave] = f
	return f
}

// medir es el ancho del texto en píxeles.
func (c *caraMiniatura) medir(tam float64, s string) float64 {
	var ancho fixed.Int26_6
	for _, r := range s {
		if f := c.cara(r, tam); f != nil {
			if a, ok := f.GlyphAdvance(r); ok {
				ancho += a
			}
		}
	}
	return float64(ancho) / 64
}

// partirLineas reparte el texto en líneas que caben en `ancho` (por palabras; una
// palabra más larga que la línea se queda entera y la recorta quien escribe).
func partirLineas(c *caraMiniatura, tam float64, texto string, ancho float64) []string {
	var out []string
	for _, parrafo := range strings.Split(texto, "\n") {
		linea := ""
		for _, p := range strings.Fields(parrafo) {
			prueba := p
			if linea != "" {
				prueba = linea + " " + p
			}
			if linea != "" && c.medir(tam, prueba) > ancho {
				out = append(out, linea)
				linea = p
				continue
			}
			linea = prueba
		}
		out = append(out, linea)
	}
	return out
}

// escribirRecortado escribe una línea centrada en vertical en su celda, sin salirse.
func escribirRecortado(img *image.RGBA, c *caraMiniatura, tam float64, s string, x, y, w, h float64, alinear string, tinta color.NRGBA) {
	sub, ok := img.SubImage(image.Rect(int(math.Floor(x)), int(math.Floor(y)), int(math.Ceil(x+w)), int(math.Ceil(y+h)))).(*image.RGBA)
	if !ok || sub.Bounds().Empty() {
		return
	}
	escribirLinea(sub, c, tam, s, x, y+(h-tam*1.2)/2, w, tam*1.2, alinear, tinta)
}

// escribirLinea pinta una línea de texto en la caja (x, y, w) con la línea base a 0,9 del
// alto de línea `lh`; alinear: "", "center" o "right".
func escribirLinea(img *image.RGBA, c *caraMiniatura, tam float64, s string, x, y, w, lh float64, alinear string, tinta color.NRGBA) {
	if s == "" || tam < 1 {
		return
	}
	switch alinear {
	case "center":
		x += (w - c.medir(tam, s)) / 2
	case "right":
		x += w - c.medir(tam, s)
	}
	pen := fixed.Point26_6{X: fixed.Int26_6(math.Round(x * 64)), Y: fixed.Int26_6(math.Round((y + lh*0.5 + tam*0.35) * 64))}
	fuente := image.NewUniform(tinta)
	for _, r := range s {
		f := c.cara(r, tam)
		if f == nil {
			continue
		}
		dr, mask, mp, avance, ok := f.Glyph(pen, r)
		if ok {
			draw.DrawMask(img, dr, fuente, image.Point{}, mask, mp, draw.Over)
		}
		pen.X += avance
	}
}
