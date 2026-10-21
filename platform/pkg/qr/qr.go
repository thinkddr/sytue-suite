// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package qr codifica QR (ISO/IEC 18004) en modo byte, nivel de corrección M,
// versiones 1..10. Clean-room a partir del estándar público y sin dependencias
// (solo stdlib). Nació en Sytue ID para el URI otpauth:// del enrolamiento TOTP y
// vive aquí porque el ERP lo necesita para el QR tributario de VERI*FACTU, que
// exige justo ese nivel M.
//
// El nivel M (~15% de recuperación) y hasta la versión 10 (213 bytes en modo byte)
// cubren cualquier URI otpauth, incluidos emails largos, y la URL de cotejo de la
// AEAT (unos 130 bytes más el número de factura). Si el dato no cabe se devuelve
// error: quien llama enseña el texto en claro como alternativa.
//
// Se pinta oscuro sobre blanco fijo (no tokens de tema) para garantizar el
// contraste que cualquier lector necesita.
package qr

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
)

// --- Especificación EC por versión (nivel M) -------------------------------
//
// El total de codewords (datos + EC) de cada fila coincide con la capacidad de
// módulos de la versión correspondiente (v1=26 … v10=346), lo que valida las
// tablas de forma cruzada.

type ecBlockGroup struct{ count, dataLen int }

type ecSpec struct {
	dataCodewords int
	ecPerBlock    int
	groups        []ecBlockGroup
}

// specsM[v-1] es la especificación EC (nivel M) de la versión v (1..10).
var specsM = []ecSpec{
	{16, 10, []ecBlockGroup{{1, 16}}},           // v1
	{28, 16, []ecBlockGroup{{1, 28}}},           // v2
	{44, 26, []ecBlockGroup{{1, 44}}},           // v3
	{64, 18, []ecBlockGroup{{2, 32}}},           // v4
	{86, 24, []ecBlockGroup{{2, 43}}},           // v5
	{108, 16, []ecBlockGroup{{4, 27}}},          // v6
	{124, 18, []ecBlockGroup{{4, 31}}},          // v7
	{154, 22, []ecBlockGroup{{2, 38}, {2, 39}}}, // v8
	{182, 22, []ecBlockGroup{{3, 36}, {2, 37}}}, // v9
	{216, 26, []ecBlockGroup{{4, 43}, {1, 44}}}, // v10
}

// alignPos[v-1] son las coordenadas (centro, fila=columna por simetría) de los
// patrones de alineación de la versión v.
var alignPos = [][]int{
	nil,
	{6, 18},
	{6, 22},
	{6, 26},
	{6, 30},
	{6, 34},
	{6, 22, 38},
	{6, 24, 42},
	{6, 26, 46},
	{6, 28, 50},
}

// --- Aritmética GF(256), polinomio primitivo 0x11d (x^8+x^4+x^3+x^2+1) ------

var (
	gfExp [512]byte
	gfLog [256]byte
)

func init() {
	x := 1
	for i := 0; i < 255; i++ {
		gfExp[i] = byte(x)
		gfLog[byte(x)] = byte(i)
		x <<= 1
		if x&0x100 != 0 {
			x ^= 0x11d
		}
	}
	// Duplicado para poder sumar exponentes sin módulo (gfExp[i] == gfExp[i-255]).
	for i := 255; i < 512; i++ {
		gfExp[i] = gfExp[i-255]
	}
}

func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[int(gfLog[a])+int(gfLog[b])]
}

// rsGenerator construye el polinomio generador de Reed–Solomon de grado `degree`
// (mónico): ∏ (x − α^i) para i=0..degree-1. gen[0] == 1.
func rsGenerator(degree int) []byte {
	gen := []byte{1}
	for i := 0; i < degree; i++ {
		next := make([]byte, len(gen)+1)
		for j := range gen {
			next[j] ^= gen[j]                    // multiplicar por x
			next[j+1] ^= gfMul(gen[j], gfExp[i]) // multiplicar por α^i
		}
		gen = next
	}
	return gen
}

// rsRemainder calcula los `degree` codewords de corrección de error de `data`
// (el resto de la división polinómica por el generador; LFSR estándar).
func rsRemainder(data []byte, degree int) []byte {
	gen := rsGenerator(degree)
	rem := make([]byte, degree)
	for _, b := range data {
		factor := b ^ rem[0]
		copy(rem, rem[1:])
		rem[degree-1] = 0
		for i := 0; i < degree; i++ {
			rem[i] ^= gfMul(gen[i+1], factor)
		}
	}
	return rem
}

// --- Codificación de datos a codewords intercalados -------------------------

// chooseVersion elige la versión más pequeña (1..10, nivel M) cuya capacidad de
// datos en modo byte alcanza para `dataLen` bytes.
func chooseVersion(dataLen int) (int, ecSpec, error) {
	for v := 1; v <= len(specsM); v++ {
		spec := specsM[v-1]
		ccBits := 8
		if v >= 10 {
			ccBits = 16
		}
		capacityBytes := (spec.dataCodewords*8 - 4 - ccBits) / 8
		if dataLen <= capacityBytes {
			return v, spec, nil
		}
	}
	return 0, ecSpec{}, errors.New("totp: datos demasiado largos para un QR (nivel M, v10)")
}

// makeCodewords construye el flujo de codewords (datos + EC intercalados) para el
// texto en modo byte y la versión/spec dadas.
func makeCodewords(data []byte, version int, spec ecSpec) []byte {
	var bits []bool
	put := func(val, n int) {
		for i := n - 1; i >= 0; i-- {
			bits = append(bits, (val>>i)&1 != 0)
		}
	}
	put(0b0100, 4) // indicador de modo byte
	ccBits := 8
	if version >= 10 {
		ccBits = 16
	}
	put(len(data), ccBits)
	for _, b := range data {
		put(int(b), 8)
	}
	// Terminador: hasta 4 ceros (menos si no cabe).
	capBits := spec.dataCodewords * 8
	if term := capBits - len(bits); term > 0 {
		if term > 4 {
			term = 4
		}
		for i := 0; i < term; i++ {
			bits = append(bits, false)
		}
	}
	// Relleno a frontera de byte.
	for len(bits)%8 != 0 {
		bits = append(bits, false)
	}
	dataCW := make([]byte, 0, spec.dataCodewords)
	for i := 0; i < len(bits); i += 8 {
		var b byte
		for j := 0; j < 8; j++ {
			if bits[i+j] {
				b |= 1 << (7 - j)
			}
		}
		dataCW = append(dataCW, b)
	}
	// Bytes de relleno alternos (0xEC, 0x11) hasta completar la capacidad.
	pad := []byte{0xEC, 0x11}
	for i := 0; len(dataCW) < spec.dataCodewords; i++ {
		dataCW = append(dataCW, pad[i%2])
	}
	return interleave(dataCW, spec)
}

// interleave divide los codewords de datos en bloques, calcula el EC de cada uno e
// intercala datos y EC en el orden que exige el estándar (por columnas).
func interleave(dataCW []byte, spec ecSpec) []byte {
	var dataBlocks, ecBlocks [][]byte
	idx := 0
	for _, g := range spec.groups {
		for b := 0; b < g.count; b++ {
			blk := dataCW[idx : idx+g.dataLen]
			idx += g.dataLen
			dataBlocks = append(dataBlocks, blk)
			ecBlocks = append(ecBlocks, rsRemainder(blk, spec.ecPerBlock))
		}
	}
	maxData := 0
	for _, b := range dataBlocks {
		if len(b) > maxData {
			maxData = len(b)
		}
	}
	out := make([]byte, 0, len(dataCW)+spec.ecPerBlock*len(ecBlocks))
	for i := 0; i < maxData; i++ {
		for _, b := range dataBlocks {
			if i < len(b) {
				out = append(out, b[i])
			}
		}
	}
	for i := 0; i < spec.ecPerBlock; i++ {
		for _, e := range ecBlocks {
			out = append(out, e[i])
		}
	}
	return out
}

// --- Matriz de módulos ------------------------------------------------------

// qrMatrix es la retícula de módulos. `dark` es el color (true=oscuro) e `isFn`
// marca los módulos de función (patrones fijos y áreas de formato/versión), que ni
// llevan datos ni se enmascaran. Coordenadas (x=columna, y=fila).
type qrMatrix struct {
	size int
	dark []bool
	isFn []bool
}

func newMatrix(size int) *qrMatrix {
	return &qrMatrix{size: size, dark: make([]bool, size*size), isFn: make([]bool, size*size)}
}

func (m *qrMatrix) get(x, y int) bool    { return m.dark[y*m.size+x] }
func (m *qrMatrix) isFunc(x, y int) bool { return m.isFn[y*m.size+x] }

// fn fija un módulo de función (color + marca).
func (m *qrMatrix) fn(x, y int, dark bool) {
	m.dark[y*m.size+x] = dark
	m.isFn[y*m.size+x] = true
}

func (m *qrMatrix) clone() *qrMatrix {
	c := newMatrix(m.size)
	copy(c.dark, m.dark)
	copy(c.isFn, m.isFn)
	return c
}

// encode construye la matriz QR completa (con la mejor máscara ya aplicada).
func encode(text string) (*qrMatrix, error) {
	version, spec, err := chooseVersion(len(text))
	if err != nil {
		return nil, err
	}
	cw := makeCodewords([]byte(text), version, spec)
	m := newMatrix(17 + 4*version)
	drawFunctionPatterns(m, version)
	drawCodewords(m, cw)
	applyBestMask(m, version)
	return m, nil
}

func drawFunctionPatterns(m *qrMatrix, version int) {
	size := m.size
	// Patrones de temporización (fila 6 y columna 6).
	for i := 0; i < size; i++ {
		dark := i%2 == 0
		m.fn(6, i, dark)
		m.fn(i, 6, dark)
	}
	// Localizadores (con separador) en las tres esquinas.
	drawFinder(m, 3, 3)
	drawFinder(m, size-4, 3)
	drawFinder(m, 3, size-4)
	// Patrones de alineación (omitiendo los que solapan los localizadores).
	pos := alignPos[version-1]
	n := len(pos)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if (i == 0 && j == 0) || (i == 0 && j == n-1) || (i == n-1 && j == 0) {
				continue
			}
			drawAlignment(m, pos[i], pos[j])
		}
	}
	// Reservar las áreas de formato y versión (los valores se escriben luego, ya
	// marcadas como función para que datos y máscara las respeten).
	drawFormatBits(m, 0)
	drawVersionInfo(m, version)
}

func drawFinder(m *qrMatrix, cx, cy int) {
	for dy := -4; dy <= 4; dy++ {
		for dx := -4; dx <= 4; dx++ {
			x, y := cx+dx, cy+dy
			if x < 0 || x >= m.size || y < 0 || y >= m.size {
				continue
			}
			d := max(abs(dx), abs(dy)) // distancia de Chebyshev
			m.fn(x, y, d != 2 && d != 4)
		}
	}
}

func drawAlignment(m *qrMatrix, cx, cy int) {
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			m.fn(cx+dx, cy+dy, max(abs(dx), abs(dy)) != 1)
		}
	}
}

// formatBits calcula los 15 bits de información de formato (nivel M = 0b00) para la
// máscara dada: BCH(15,5) con g(x)=0x537 y XOR con la máscara fija 0x5412.
func formatBits(mask int) int {
	data := mask // (0b00 << 3) | mask
	rem := data
	for i := 0; i < 10; i++ {
		rem = (rem << 1) ^ ((rem >> 9) * 0x537)
	}
	return ((data << 10) | rem) ^ 0x5412
}

func drawFormatBits(m *qrMatrix, mask int) {
	bits := formatBits(mask)
	size := m.size
	// Copia 1, alrededor del localizador superior izquierdo.
	for i := 0; i <= 5; i++ {
		m.fn(8, i, bit(bits, i))
	}
	m.fn(8, 7, bit(bits, 6))
	m.fn(8, 8, bit(bits, 7))
	m.fn(7, 8, bit(bits, 8))
	for i := 9; i < 15; i++ {
		m.fn(14-i, 8, bit(bits, i))
	}
	// Copia 2, repartida entre los localizadores superior derecho e inferior izq.
	for i := 0; i < 8; i++ {
		m.fn(size-1-i, 8, bit(bits, i))
	}
	for i := 8; i < 15; i++ {
		m.fn(8, size-15+i, bit(bits, i))
	}
	m.fn(8, size-8, true) // módulo oscuro fijo
}

// versionBits calcula los 18 bits de información de versión: BCH(18,6) con
// g(x)=0x1F25 (sin XOR). Solo se dibuja para versiones ≥ 7.
func versionBits(version int) int {
	rem := version
	for i := 0; i < 12; i++ {
		rem = (rem << 1) ^ ((rem >> 11) * 0x1F25)
	}
	return (version << 12) | rem
}

func drawVersionInfo(m *qrMatrix, version int) {
	if version < 7 {
		return
	}
	bits := versionBits(version)
	size := m.size
	for i := 0; i < 18; i++ {
		b := bit(bits, i)
		a := size - 11 + i%3
		c := i / 3
		m.fn(a, c, b) // bloque superior derecho
		m.fn(c, a, b) // bloque inferior izquierdo (simétrico)
	}
}

// drawCodewords coloca el flujo de bits en zigzag por pares de columnas, de derecha
// a izquierda, saltando la columna de temporización y los módulos de función.
func drawCodewords(m *qrMatrix, data []byte) {
	size := m.size
	i := 0 // índice de bit
	for right := size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5 // saltar la columna de temporización
		}
		for vert := 0; vert < size; vert++ {
			for j := 0; j < 2; j++ {
				x := right - j
				upward := (right+1)&2 == 0
				y := vert
				if upward {
					y = size - 1 - vert
				}
				if !m.isFunc(x, y) && i < len(data)*8 {
					if (data[i>>3]>>(7-(i&7)))&1 != 0 {
						m.dark[y*m.size+x] = true
					}
					i++
				}
			}
		}
	}
}

// maskCondition indica si el módulo (x,y) se invierte con la máscara dada.
func maskCondition(mask, x, y int) bool {
	switch mask {
	case 0:
		return (x+y)%2 == 0
	case 1:
		return y%2 == 0
	case 2:
		return x%3 == 0
	case 3:
		return (x+y)%3 == 0
	case 4:
		return (x/3+y/2)%2 == 0
	case 5:
		return (x*y)%2+(x*y)%3 == 0
	case 6:
		return ((x*y)%2+(x*y)%3)%2 == 0
	default: // 7
		return ((x+y)%2+(x*y)%3)%2 == 0
	}
}

func applyMask(m *qrMatrix, mask int) {
	for y := 0; y < m.size; y++ {
		for x := 0; x < m.size; x++ {
			if !m.isFunc(x, y) && maskCondition(mask, x, y) {
				m.dark[y*m.size+x] = !m.dark[y*m.size+x]
			}
		}
	}
}

// applyBestMask prueba las 8 máscaras (cada una con su formato/versión) y deja
// aplicada la de menor penalización, como recomienda el estándar para máxima
// legibilidad del lector.
func applyBestMask(m *qrMatrix, version int) {
	best := m
	bestScore := 1 << 30
	for mask := 0; mask < 8; mask++ {
		c := m.clone()
		applyMask(c, mask)
		drawFormatBits(c, mask)
		drawVersionInfo(c, version)
		if s := penalty(c); s < bestScore {
			bestScore = s
			best = c
		}
	}
	*m = *best
}

// penalty puntúa una matriz según las cuatro reglas del estándar (menor = mejor).
func penalty(m *qrMatrix) int {
	size := m.size
	score := 0
	// Regla 1: rachas de 5+ módulos del mismo color en filas y columnas.
	run := func(get func(k int) bool) {
		color := get(0)
		length := 1
		for k := 1; k < size; k++ {
			if get(k) == color {
				length++
				if length == 5 {
					score += 3
				} else if length > 5 {
					score++
				}
			} else {
				color = get(k)
				length = 1
			}
		}
	}
	for y := 0; y < size; y++ {
		run(func(k int) bool { return m.get(k, y) })
	}
	for x := 0; x < size; x++ {
		run(func(k int) bool { return m.get(x, k) })
	}
	// Regla 2: bloques 2x2 del mismo color.
	for y := 0; y < size-1; y++ {
		for x := 0; x < size-1; x++ {
			c := m.get(x, y)
			if c == m.get(x+1, y) && c == m.get(x, y+1) && c == m.get(x+1, y+1) {
				score += 3
			}
		}
	}
	// Regla 3: patrón 1:1:3:1:1 tipo localizador con 4 claros a un lado.
	score += 40 * finderLike(m)
	// Regla 4: desviación de la proporción de módulos oscuros respecto al 50%.
	dark := 0
	for _, b := range m.dark {
		if b {
			dark++
		}
	}
	percent := dark * 100 / (size * size)
	score += (abs(percent-50) / 5) * 10
	return score
}

// finderLike cuenta las apariciones del patrón tipo localizador (10111010000 y su
// reflejo) en todas las filas y columnas — la regla 3 de penalización.
func finderLike(m *qrMatrix) int {
	p1 := [...]bool{true, false, true, true, true, false, true, false, false, false, false}
	p2 := [...]bool{false, false, false, false, true, false, true, true, true, false, true}
	count := 0
	scan := func(get func(k int) bool, n int) {
		for i := 0; i+11 <= n; i++ {
			m1, m2 := true, true
			for k := 0; k < 11; k++ {
				b := get(i + k)
				if b != p1[k] {
					m1 = false
				}
				if b != p2[k] {
					m2 = false
				}
			}
			if m1 {
				count++
			}
			if m2 {
				count++
			}
		}
	}
	size := m.size
	for y := 0; y < size; y++ {
		scan(func(k int) bool { return m.get(k, y) }, size)
	}
	for x := 0; x < size; x++ {
		scan(func(k int) bool { return m.get(x, k) }, size)
	}
	return count
}

func bit(v, i int) bool { return (v>>i)&1 != 0 }

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// --- Salida PNG -------------------------------------------------------------

// escalaPNG: píxeles por módulo. Ocho da ~520 px en la versión más grande, que se
// escanea de sobra y sigue pesando poco al ser indexado de dos colores. Menos de
// cuatro empieza a fallar en cámaras que reescalan la imagen.
const escalaPNG = 8

// PNG genera el QR del texto como PNG (oscuro sobre blanco, zona de
// silencio de 4 módulos). Mismo codificador que el SVG; cambia solo la salida.
//
// EXISTE PORQUE EL SVG NO SIRVE EN TODAS PARTES. Escanear con la cámara funciona
// igual, pero varios gestores de TOTP ofrecen «subir una imagen del código» y
// filtran el selector por PNG/JPEG: un SVG ni aparece en la lista. Y «guardar
// imagen» sobre un `data:image/svg+xml` acaba en un fichero que esas apps
// tampoco abren. Quien enrola desde el mismo dispositivo que muestra la pantalla
// —el caso más común en escritorio— se quedaba sin esa vía.
//
// Devuelve error si el texto no cabe (v10, nivel M): el llamante recurre a la
// clave/URI en texto, que siempre se enseña.
func PNG(text string) ([]byte, error) {
	m, err := encode(text)
	if err != nil {
		return nil, err
	}
	const quiet = 4
	dim := (m.size + 2*quiet) * escalaPNG
	// Paleta de dos colores: el PNG sale indexado a 1 bit por píxel en vez de 8,
	// que para una rejilla en blanco y negro es la diferencia entre unos pocos KB
	// y bastantes más por algo que no aporta ni un tono.
	img := image.NewPaletted(image.Rect(0, 0, dim, dim), color.Palette{
		color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
		color.RGBA{R: 0x0a, G: 0x0a, B: 0x0a, A: 0xff},
	})
	for y := 0; y < m.size; y++ {
		for x := 0; x < m.size; x++ {
			if !m.get(x, y) {
				continue // el fondo ya es blanco: el índice 0 es el valor cero
			}
			px := (x + quiet) * escalaPNG
			py := (y + quiet) * escalaPNG
			for dy := 0; dy < escalaPNG; dy++ {
				fila := img.Pix[(py+dy)*img.Stride+px:]
				for dx := 0; dx < escalaPNG; dx++ {
					fila[dx] = 1
				}
			}
		}
	}
	var b bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&b, img); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
