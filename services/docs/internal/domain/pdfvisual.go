// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Comparación VISUAL de dos PDF: cada página de uno contra la del otro, píxel a píxel.
//
// `CompararPDF` compara el TEXTO y sigue siendo lo que se quiere para saber qué
// cláusula cambió. Esto responde a la otra pregunta, la que el texto no ve: ¿se movió el
// logotipo?, ¿cambió el color de la tabla?, ¿falta una firma dibujada?, ¿es el mismo
// escaneo? Por eso son dos modos y no uno que sustituya al otro.
//
// CÓMO SE DECIDE QUE DOS PÍXELES SON DISTINTOS. Dos páginas dibujadas por el mismo
// motor salen idénticas, pero dos versiones de un documento casi nunca: un texto
// desplazado media décima de punto cambia el antialiasing de todas sus letras sin
// que nadie lo vea. Marcar eso sería el ruido que hace inútil un comparador. Hay dos
// defensas, las dos configurables:
//
//   - un UMBRAL POR CANAL: dos colores cuyo canal más distinto no pasa de `Tolerancia`
//     son iguales;
//   - una TOLERANCIA DE VECINDAD: un píxel solo es distinto si no se parece a NINGUNO
//     de los de alrededor (radio `Vecindad`) en el otro dibujo, y viceversa. Un trazo
//     corrido un píxel se parece a su vecino y no cuenta; una letra que cambia por
//     otra, no. Y si aun así lo parece, el PROMEDIO 3×3 de la zona tiene que diferir
//     más que la tolerancia: es lo que absorbe un corrimiento de fracción de píxel.
//
// Medido (frontal, ARM64, texto de 8 a 14 pt, ajustes por defecto): un texto corrido
// 0,1 pt deja 0 píxeles distintos a ×1, ×1,5 y ×2; corrido 0,3 pt, ~100 de ruido de borde
// a ×1,5 (el 0,01 % de la página; sin tolerancia serían 4.700); una palabra cambiada
// deja ~90 y un signo de puntuación cambiado, 2. NO hay ajuste que separe del todo un
// corrimiento de 0,3 pt de un cambio pequeño: se eligió no perder cambios. Y a ×1 el
// texto fino se pierde (una palabra deja 0 píxeles): por eso el defecto es ×1,5.
//
// DE PÍXELES A REGIONES. Una lista de píxeles no se le enseña a nadie. Las diferencias
// se agrupan en celdas, las celdas se unen si están a menos de ~6 pt y de cada grupo sale
// una caja, en PUNTOS de página con el origen arriba a la izquierda: el cliente la pinta
// sin saber a qué escala se comparó.
//
// ENTRADA NO CONFIABLE. Son dos PDF arbitrarios, así que todo está acotado: la escala
// (`maxEscalaVisual`), los píxeles de la comparación (`maxPixelesVisual`, que BAJA la
// escala en vez de fallar: una página de 14400 pt se compara, a menos nitidez, y la
// respuesta dice a cuál), las páginas (`maxPaginasVisual`), las regiones por página
// (`maxRegionesVisual`) y el tiempo, por el contexto, entre página y página.
package domain

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
	"sort"
)

const (
	// escalaVisualPorDefecto: 1,5 = 108 ppp. A 72 una palabra de 10 pt son 5 píxeles de
	// alto y un cambio de una coma es un píxel; a 108 se ve y no pesa.
	escalaVisualPorDefecto = 1.5
	minEscalaVisual        = 0.25
	maxEscalaVisual        = 3.0

	// maxPixelesVisual es el tope de píxeles del lienzo comparado (la unión de las
	// dos páginas): ~4 Mpx = un A4 a ×2,9. Cada página son 4 bytes por píxel, así que
	// pesa 16 MiB; con la otra, la máscara y el resaltado, el pico ronda los 60 MiB por
	// petición. Muy por debajo del `maxPixelesRaster` (16 Mpx) del render suelto, a
	// propósito: aquí hay DOS páginas vivas a la vez.
	maxPixelesVisual = 4 << 20
	// minEscalaEfectiva: por debajo, una página se ve como una mancha y comparar no
	// dice nada; se declara «demasiado grande» en vez de dar un número engañoso.
	minEscalaEfectiva = 0.1

	// maxPaginasVisual: páginas que se comparan en una petición. Veinte páginas son
	// unos segundos de CPU compartida; más se dicen (`Truncado`) y se piden por tramos.
	maxPaginasVisual = 50

	// maxRegionesVisual por página. Una página con diferencias por todas partes no
	// necesita doscientas cajas: se unen más lejos hasta que quepan.
	maxRegionesVisual = 64

	toleranciaVisualPorDefecto = 48
	vecindadVisualPorDefecto   = 1
	maxVecindadVisual          = 3

	// celdaVisual es el lado, en píxeles, de la rejilla donde se agrupan las diferencias.
	celdaVisual = 4
	// huecoRegionPt es la distancia, en puntos, por debajo de la cual dos diferencias
	// forman una sola región: unas palabras de la misma línea son «un cambio».
	huecoRegionPt = 6.0
)

// OpcionesVisual afina la comparación. El valor cero es el razonable.
type OpcionesVisual struct {
	// Escala a la que se dibujan las dos páginas (1 = 72 ppp). 0 = 1,5. Se acota a
	// [0,25; 3] y se BAJA por página si el lienzo no cabe en `maxPixelesVisual`.
	Escala float64
	// Tolerancia es la diferencia máxima por canal (0-255) para que dos colores cuenten
	// como iguales. 0 = 48; para exigir igualdad estricta, 1.
	Tolerancia int
	// Vecindad es el radio, en píxeles, de la tolerancia por vecindad. 0 = 1; el
	// máximo es 3. Para quitarla, `SinVecindad`.
	Vecindad    int
	SinVecindad bool
	// MaxPaginas es cuántas páginas se comparan como mucho. 0 o más de `maxPaginasVisual`
	// = `maxPaginasVisual`.
	MaxPaginas int
}

func (o OpcionesVisual) normalizadas() OpcionesVisual {
	switch {
	case o.Escala <= 0 || math.IsNaN(o.Escala):
		o.Escala = escalaVisualPorDefecto
	case o.Escala < minEscalaVisual:
		o.Escala = minEscalaVisual
	case o.Escala > maxEscalaVisual:
		o.Escala = maxEscalaVisual
	}
	switch {
	case o.Tolerancia <= 0:
		o.Tolerancia = toleranciaVisualPorDefecto
	case o.Tolerancia > 255:
		o.Tolerancia = 255
	}
	switch {
	case o.SinVecindad:
		o.Vecindad = 0
	case o.Vecindad <= 0:
		o.Vecindad = vecindadVisualPorDefecto
	case o.Vecindad > maxVecindadVisual:
		o.Vecindad = maxVecindadVisual
	}
	if o.MaxPaginas <= 0 || o.MaxPaginas > maxPaginasVisual {
		o.MaxPaginas = maxPaginasVisual
	}
	return o
}

// RegionVisual es una caja de diferencia, en puntos de página (origen arriba-izquierda).
type RegionVisual struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Ancho float64 `json:"ancho"`
	Alto  float64 `json:"alto"`
}

// Motivos por los que una página no se pudo comparar.
const (
	OmitidaDemasiadoGrande = "demasiado_grande"
	OmitidaIlegible        = "ilegible"
)

// DiferenciaVisual es el resultado de comparar una página con su equivalente.
type DiferenciaVisual struct {
	Pagina int `json:"pagina"`
	// SoloEn es «a» o «b» si la página solo existe en uno de los documentos: no hay
	// con qué compararla y se dice, no se inventa un porcentaje.
	SoloEn string `json:"solo_en,omitempty"`
	// Omitida, con su Motivo, es una página que existe en los dos y no se pudo comparar.
	Omitida bool   `json:"omitida,omitempty"`
	Motivo  string `json:"motivo,omitempty"`
	// TamanosDistintos: las dos páginas no miden lo mismo. Se comparan sobre el lienzo
	// de la unión, y lo que una no cubre cuenta como papel en blanco.
	TamanosDistintos bool    `json:"tamanos_distintos,omitempty"`
	AnchoA           float64 `json:"ancho_a"`
	AltoA            float64 `json:"alto_a"`
	AnchoB           float64 `json:"ancho_b"`
	AltoB            float64 `json:"alto_b"`
	// Ancho y Alto son el lienzo comparado, en puntos: la referencia de las regiones.
	Ancho float64 `json:"ancho"`
	Alto  float64 `json:"alto"`
	// Escala a la que se comparó de verdad (puede ser menor que la pedida).
	Escala            float64        `json:"escala"`
	PixelesDiferentes int            `json:"pixeles_diferentes"`
	PixelesTotales    int            `json:"pixeles_totales"`
	Porcentaje        float64        `json:"porcentaje"`
	Regiones          []RegionVisual `json:"regiones"`
	// Imagen es el resaltado en PNG, solo si se pidió (`CompararVisualPagina`).
	Imagen []byte `json:"imagen,omitempty"`
}

// ComparacionVisual es el resultado de comparar dos documentos.
type ComparacionVisual struct {
	PaginasA   int `json:"paginas_a"`
	PaginasB   int `json:"paginas_b"`
	Comparadas int `json:"comparadas"`
	// Truncado: quedaron páginas sin comparar por el tope de páginas o de tiempo.
	Truncado bool `json:"truncado"`
	// Paginas lleva SOLO las que difieren (o no se pudieron comparar), como el modo de texto.
	Paginas []DiferenciaVisual `json:"paginas"`
	Iguales bool               `json:"iguales"`
}

// CompararVisual compara los dos documentos página a página. Sin imágenes: son las
// regiones y los números, y el resaltado de una página se pide aparte.
//
// El contexto se mira ENTRE páginas, no dentro de una (el intérprete no se puede
// interrumpir): si vence, devuelve lo comparado hasta ahí con `Truncado`.
func CompararVisual(ctx context.Context, a, b *PDFDoc, opts OpcionesVisual) (*ComparacionVisual, error) {
	opts = opts.normalizadas()
	na, nb := a.NumPaginas(), b.NumPaginas()
	total := max(na, nb)
	res := &ComparacionVisual{PaginasA: na, PaginasB: nb, Paginas: []DiferenciaVisual{}}
	for i := 0; i < total; i++ {
		if i >= opts.MaxPaginas || ctx.Err() != nil {
			res.Truncado = true
			break
		}
		dif := compararPaginaVisual(a, b, i, opts, false)
		res.Comparadas++
		if dif.diferente() {
			res.Paginas = append(res.Paginas, dif)
		}
	}
	res.Iguales = len(res.Paginas) == 0 && !res.Truncado
	return res, nil
}

// CompararVisualPagina compara UNA página (1-based) y devuelve además el resaltado en
// PNG. Existe aparte para que la lista de diferencias de un documento largo no cargue
// con veinte imágenes que nadie va a mirar.
func CompararVisualPagina(a, b *PDFDoc, pagina int, opts OpcionesVisual) (DiferenciaVisual, error) {
	if pagina < 1 || pagina > max(a.NumPaginas(), b.NumPaginas()) {
		return DiferenciaVisual{}, ErrPaginaFueraDeRango
	}
	return compararPaginaVisual(a, b, pagina-1, opts.normalizadas(), true), nil
}

// ErrPaginaFueraDeRango es la página pedida que ninguno de los dos documentos tiene.
var ErrPaginaFueraDeRango = errors.New("la página no existe en ninguno de los dos documentos")

func (d DiferenciaVisual) diferente() bool {
	return d.SoloEn != "" || d.Omitida || d.PixelesDiferentes > 0
}

func compararPaginaVisual(a, b *PDFDoc, i int, opts OpcionesVisual, conImagen bool) DiferenciaVisual {
	dif := DiferenciaVisual{Pagina: i + 1, Regiones: []RegionVisual{}}
	enA, enB := i < a.NumPaginas(), i < b.NumPaginas()
	var pa, pb PDFPagina
	if enA {
		p, err := a.Pagina(i)
		if err != nil {
			return omitida(dif, OmitidaIlegible)
		}
		pa = p
		dif.AnchoA, dif.AltoA = p.Ancho, p.Alto
	}
	if enB {
		p, err := b.Pagina(i)
		if err != nil {
			return omitida(dif, OmitidaIlegible)
		}
		pb = p
		dif.AnchoB, dif.AltoB = p.Ancho, p.Alto
	}
	if !enA || !enB {
		dif.SoloEn = "a"
		if !enA {
			dif.SoloEn = "b"
		}
		dif.Ancho, dif.Alto = max(dif.AnchoA, dif.AnchoB), max(dif.AltoA, dif.AltoB)
		return dif
	}

	// El lienzo es la unión de las dos páginas. Una medida no finita o nula se trata
	// como ilegible: el MediaBox lo escribe quien hizo el PDF.
	w, h := max(pa.Ancho, pb.Ancho), max(pa.Alto, pb.Alto)
	if !finitoPositivo(w) || !finitoPositivo(h) {
		return omitida(dif, OmitidaIlegible)
	}
	dif.Ancho, dif.Alto = w, h
	dif.TamanosDistintos = math.Abs(pa.Ancho-pb.Ancho) > 0.5 || math.Abs(pa.Alto-pb.Alto) > 0.5

	// La escala BAJA hasta que el lienzo cabe; los dos se dibujan a la misma.
	esc := math.Min(opts.Escala, math.Sqrt(float64(maxPixelesVisual)/(w*h)))
	if esc < minEscalaEfectiva {
		return omitida(dif, OmitidaDemasiadoGrande)
	}
	dif.Escala = esc

	ra, err := a.RenderizarPaginaRGBA(i, OpcionesRender{Escala: esc})
	if err != nil {
		return omitida(dif, OmitidaIlegible)
	}
	rb, err := b.RenderizarPaginaRGBA(i, OpcionesRender{Escala: esc})
	if err != nil {
		return omitida(dif, OmitidaIlegible)
	}

	res := compararImagenes(ra, rb, opts, esc, conImagen)
	dif.PixelesDiferentes, dif.PixelesTotales = res.diferentes, res.total
	if res.total > 0 {
		dif.Porcentaje = 100 * float64(res.diferentes) / float64(res.total)
	}
	dif.Regiones = res.regiones
	if conImagen {
		var buf bytes.Buffer
		if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, res.resaltado); err == nil {
			dif.Imagen = buf.Bytes()
		}
	}
	return dif
}

func omitida(d DiferenciaVisual, motivo string) DiferenciaVisual {
	d.Omitida, d.Motivo = true, motivo
	return d
}

func finitoPositivo(v float64) bool { return v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v) }

// --- Diferencia de píxeles ---------------------------------------------------

type resultadoImagenes struct {
	diferentes, total int
	regiones          []RegionVisual
	resaltado         *image.RGBA
}

// pixelEn da el color de (x,y) o BLANCO fuera de la imagen: lo que una página más
// pequeña no cubre es papel en blanco, no un error.
func pixelEn(img *image.RGBA, x, y int) (r, g, b uint8) {
	if x < img.Rect.Min.X || y < img.Rect.Min.Y || x >= img.Rect.Max.X || y >= img.Rect.Max.Y {
		return 255, 255, 255
	}
	o := img.PixOffset(x, y)
	return img.Pix[o], img.Pix[o+1], img.Pix[o+2]
}

func distanciaColor(r1, g1, b1, r2, g2, b2 uint8) int {
	return max(absInt(int(r1)-int(r2)), absInt(int(g1)-int(g2)), absInt(int(b1)-int(b2)))
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// seParece dice si el píxel (r,g,b) tiene un igual, dentro de la tolerancia, en la
// vecindad de radio `rad` de (x,y) en `otra`.
func seParece(r, g, b uint8, otra *image.RGBA, x, y, rad, tol int) bool {
	for dy := -rad; dy <= rad; dy++ {
		for dx := -rad; dx <= rad; dx++ {
			r2, g2, b2 := pixelEn(otra, x+dx, y+dy)
			if distanciaColor(r, g, b, r2, g2, b2) <= tol {
				return true
			}
		}
	}
	return false
}

// distanciaMedia es la distancia por canal entre el promedio 3×3 de `a` y el de `b`
// en (x,y); fuera de la imagen cuenta como papel.
func distanciaMedia(a, b *image.RGBA, x, y int) int {
	var ma, mb [3]int
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			r, g, bl := pixelEn(a, x+dx, y+dy)
			ma[0], ma[1], ma[2] = ma[0]+int(r), ma[1]+int(g), ma[2]+int(bl)
			r, g, bl = pixelEn(b, x+dx, y+dy)
			mb[0], mb[1], mb[2] = mb[0]+int(r), mb[1]+int(g), mb[2]+int(bl)
		}
	}
	return max(absInt(ma[0]-mb[0]), absInt(ma[1]-mb[1]), absInt(ma[2]-mb[2])) / 9
}

func compararImagenes(a, b *image.RGBA, opts OpcionesVisual, escala float64, conImagen bool) resultadoImagenes {
	w, h := max(a.Rect.Dx(), b.Rect.Dx()), max(a.Rect.Dy(), b.Rect.Dy())
	res := resultadoImagenes{total: w * h}
	cw, ch := (w+celdaVisual-1)/celdaVisual, (h+celdaVisual-1)/celdaVisual
	// Caja de píxeles de cada celda con diferencias: sin ella la región saldría
	// redondeada a la rejilla y la caja medida no sería la caja real.
	cajas := make([]cajaPx, cw*ch)
	if conImagen {
		res.resaltado = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	for y := 0; y < h; y++ {
		// Camino rápido: dos filas idénticas (la inmensa mayoría de una página: papel)
		// no se miran píxel a píxel.
		if filaIdentica(a, b, y, w) {
			if conImagen {
				pintarFilaIgual(res.resaltado, a, y, w)
			}
			continue
		}
		for x := 0; x < w; x++ {
			r1, g1, b1 := pixelEn(a, x, y)
			r2, g2, b2 := pixelEn(b, x, y)
			if !esDistinto(a, b, x, y, opts) {
				if conImagen {
					res.resaltado.SetRGBA(x, y, atenuado(r2, g2, b2))
				}
				continue
			}
			res.diferentes++
			cajas[(y/celdaVisual)*cw+x/celdaVisual].incluir(x, y)
			if conImagen {
				res.resaltado.SetRGBA(x, y, colorDeDiferencia(r1, g1, b1, r2, g2, b2))
			}
		}
	}
	res.regiones = agruparRegiones(cajas, cw, ch, escala)
	return res
}

// filaIdentica dice si la fila y de las dos imágenes es la misma, byte a byte. Solo si
// miden lo mismo de ancho y las dos llegan a esa fila.
func filaIdentica(a, b *image.RGBA, y, w int) bool {
	if y >= a.Rect.Dy() || y >= b.Rect.Dy() || a.Rect.Dx() != w || b.Rect.Dx() != w {
		return false
	}
	return bytes.Equal(a.Pix[y*a.Stride:y*a.Stride+4*w], b.Pix[y*b.Stride:y*b.Stride+4*w])
}

func pintarFilaIgual(dst, src *image.RGBA, y, w int) {
	for x := 0; x < w; x++ {
		r, g, b := pixelEn(src, x, y)
		dst.SetRGBA(x, y, atenuado(r, g, b))
	}
}

// esDistinto decide si el píxel (x,y) difiere, con las tres defensas contra el ruido de
// borde (ver la cabecera del fichero): el umbral por canal, la vecindad y el promedio 3×3.
func esDistinto(a, b *image.RGBA, x, y int, opts OpcionesVisual) bool {
	r1, g1, b1 := pixelEn(a, x, y)
	r2, g2, b2 := pixelEn(b, x, y)
	if distanciaColor(r1, g1, b1, r2, g2, b2) <= opts.Tolerancia {
		return false
	}
	if opts.Vecindad == 0 {
		return true
	}
	// Distinto de verdad solo si no se parece a nada de alrededor en el otro dibujo, ni
	// al revés: un trazo corrido no es un cambio.
	if seParece(r1, g1, b1, b, x, y, opts.Vecindad, opts.Tolerancia) ||
		seParece(r2, g2, b2, a, x, y, opts.Vecindad, opts.Tolerancia) {
		return false
	}
	// Un corrimiento de FRACCIÓN de píxel no deja ningún vecino igual (el borde de un trazo
	// pasa del 100 % al 55 % de tinta), pero sí deja el promedio de la zona casi igual. Se
	// exige también que difiera el promedio 3×3 (por encima de la mitad de la tolerancia):
	// ahí un cambio de verdad conserva su diferencia (una coma que pasa a punto y coma da
	// ≥ 160 de 255) y el ruido de borde queda por debajo.
	return distanciaMedia(a, b, x, y) > opts.Tolerancia/2
}

// colorDeDiferencia: más oscuro en A que en B es que había tinta y ya no (quitado, rojo);
// al revés, añadido (verde). Los mismos colores que las líneas del modo de texto.
func colorDeDiferencia(r1, g1, b1, r2, g2, b2 uint8) color.RGBA {
	if luma(r1, g1, b1) < luma(r2, g2, b2) {
		return color.RGBA{R: 220, G: 38, B: 38, A: 255}
	}
	return color.RGBA{R: 22, G: 163, B: 74, A: 255}
}

func luma(r, g, b uint8) int { return 299*int(r) + 587*int(g) + 114*int(b) }

// atenuado aclara el contenido que no cambió para que lo que sí cambió destaque.
func atenuado(r, g, b uint8) color.RGBA {
	f := func(c uint8) uint8 { return 255 - uint8((255-uint(c))*35/100) }
	return color.RGBA{R: f(r), G: f(g), B: f(b), A: 255}
}

// --- Regiones ----------------------------------------------------------------

// cajaPx es la caja, en píxeles, de lo que difiere dentro de una celda. Vacía si n == 0.
type cajaPx struct {
	x0, y0, x1, y1 int
	n              int
}

func (c *cajaPx) incluir(x, y int) {
	if c.n == 0 {
		c.x0, c.y0, c.x1, c.y1 = x, y, x, y
	} else {
		c.x0, c.y0 = min(c.x0, x), min(c.y0, y)
		c.x1, c.y1 = max(c.x1, x), max(c.y1, y)
	}
	c.n++
}

// agruparRegiones junta las celdas con diferencias que están a menos de `huecoRegionPt`
// y devuelve una caja por grupo, en puntos y en orden de lectura (arriba abajo, izquierda
// derecha), que es el orden en que la interfaz las recorre.
//
// Se hace DILATANDO la rejilla y etiquetando componentes conexas: es lineal en el
// número de celdas. La alternativa, comparar cada celda con las de su alrededor, cuesta
// el cuadrado del hueco, y el hueco crece cuando hay que fundir regiones para no pasar
// de `maxRegionesVisual`.
func agruparRegiones(cajas []cajaPx, cw, ch int, escala float64) []RegionVisual {
	hay := false
	for i := range cajas {
		if cajas[i].n > 0 {
			hay = true
			break
		}
	}
	if !hay {
		return []RegionVisual{}
	}
	hueco := int(math.Ceil(huecoRegionPt * escala / celdaVisual))
	var grupos []cajaPx
	for {
		grupos = componentes(cajas, cw, ch, hueco)
		if len(grupos) <= maxRegionesVisual || hueco >= max(cw, ch) {
			break
		}
		hueco *= 2 // demasiadas cajas: se funden las más cercanas y se vuelve a mirar
	}
	if len(grupos) > maxRegionesVisual { // no llega con el hueco máximo: una sola caja
		todo := cajaPx{}
		for _, g := range grupos {
			todo.unir(g)
		}
		grupos = []cajaPx{todo}
	}
	sort.Slice(grupos, func(i, j int) bool {
		if grupos[i].y0 != grupos[j].y0 {
			return grupos[i].y0 < grupos[j].y0
		}
		return grupos[i].x0 < grupos[j].x0
	})
	out := make([]RegionVisual, 0, len(grupos))
	for _, g := range grupos {
		out = append(out, RegionVisual{
			X:     float64(g.x0) / escala,
			Y:     float64(g.y0) / escala,
			Ancho: float64(g.x1-g.x0+1) / escala,
			Alto:  float64(g.y1-g.y0+1) / escala,
		})
	}
	return out
}

func (c *cajaPx) unir(o cajaPx) {
	if o.n == 0 {
		return
	}
	if c.n == 0 {
		*c = o
		return
	}
	c.x0, c.y0 = min(c.x0, o.x0), min(c.y0, o.y0)
	c.x1, c.y1 = max(c.x1, o.x1), max(c.y1, o.y1)
	c.n += o.n
}

// componentes dilata la rejilla en `hueco` celdas, etiqueta sus componentes conexas
// (4 vecinos) y devuelve, por componente, la caja de píxeles de las celdas ORIGINALES.
func componentes(cajas []cajaPx, cw, ch, hueco int) []cajaPx {
	dil := dilatar(cajas, cw, ch, hueco)
	etiqueta := make([]int, len(dil))
	var grupos []cajaPx
	var cola []int
	for ini := range dil {
		if !dil[ini] || etiqueta[ini] != 0 {
			continue
		}
		id := len(grupos) + 1
		grupos = append(grupos, cajaPx{})
		etiqueta[ini] = id
		cola = append(cola[:0], ini)
		for len(cola) > 0 {
			c := cola[len(cola)-1]
			cola = cola[:len(cola)-1]
			grupos[id-1].unir(cajas[c])
			x, y := c%cw, c/cw
			for _, v := range [4][2]int{{x - 1, y}, {x + 1, y}, {x, y - 1}, {x, y + 1}} {
				if v[0] < 0 || v[1] < 0 || v[0] >= cw || v[1] >= ch {
					continue
				}
				n := v[1]*cw + v[0]
				if dil[n] && etiqueta[n] == 0 {
					etiqueta[n] = id
					cola = append(cola, n)
				}
			}
		}
	}
	return grupos
}

// dilatar marca cada celda a distancia (Chebyshev) ≤ g de una celda con diferencias,
// con dos pasadas separables por sumas acumuladas: O(celdas), sin depender de g.
func dilatar(cajas []cajaPx, cw, ch, g int) []bool {
	horiz := make([]bool, len(cajas))
	pref := make([]int, max(cw, ch)+1)
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			pref[x+1] = pref[x]
			if cajas[y*cw+x].n > 0 {
				pref[x+1]++
			}
		}
		for x := 0; x < cw; x++ {
			horiz[y*cw+x] = pref[min(cw-1, x+g)+1]-pref[max(0, x-g)] > 0
		}
	}
	out := make([]bool, len(cajas))
	for x := 0; x < cw; x++ {
		for y := 0; y < ch; y++ {
			pref[y+1] = pref[y]
			if horiz[y*cw+x] {
				pref[y+1]++
			}
		}
		for y := 0; y < ch; y++ {
			out[y*cw+x] = pref[min(ch-1, y+g)+1]-pref[max(0, y-g)] > 0
		}
	}
	return out
}
