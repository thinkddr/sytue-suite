// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"image"
	"image/color"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/net/html"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// LA PROSA DE UNA NOTA: el HTML de `Documento` maquetado como lo pinta la web con
// `.sytue-prose` (apps/frontend/apps/notes/src/styles/app.css) dentro del cuerpo de la
// tarjeta (`text-base leading-relaxed break-words`).
//
// El maquetador es UNO para leer y para escribir: el editor pregunta aquí dónde cae cada
// letra (para el cursor y la selección) y qué letra hay bajo un punto (para el clic). Si
// fueran dos, el cursor acabaría a medio píxel de la letra que se ve.
//
// Lo que se mide y de dónde sale, con el rem de 14:
//
//	cuerpo        12,25 con `leading-relaxed` (× 1,625): 19,906
//	p             `mb-2 last:mb-0`
//	h1…h6         2xl, xl, lg, base, sm, sm · `mt-4 mb-1.5 leading-tight` semi-negrita
//	ul / ol       `my-1.5 pl-6`; li `my-0.5`
//	blockquote    `my-2 border-l-[3px] pl-3` en `text-muted`
//	pre           `my-2 rounded-md px-3 py-2 font-mono text-sm` sobre `bg-subtle`
//	hr            `my-3 border-t`
//	.task-row     `my-1 flex items-baseline gap-2`, la casilla de 13 de Chromium
//	code          `rounded px-1 font-mono text-[0.92em]` sobre `bg-subtle`
//	mark          `rounded-sm px-0.5` sobre #fff3a3 (#6b5d00 en oscuro)
//	a             `text-primary-text underline underline-offset-2`
//
// Los márgenes verticales se COLAPSAN entre hermanos, como en CSS: entre un párrafo
// (`mb-2`) y una lista (`my-1.5`) hay 7, no 12,25. Y no se escapan por arriba ni por
// abajo del cuerpo, que es un elemento de un `flex` y hace de contexto propio.

// Línea del cuerpo: `leading-relaxed`.
var escalonCuerpo = tema.Escalon{Tam: tema.TextoBase.Tam, Linea: tema.TextoBase.Tam * 1.625}

// Medidas de la prosa.
const (
	margenParrafo  = 2 * 0.25 * tema.Rem
	margenTituloA  = 4 * 0.25 * tema.Rem
	margenTituloB  = 1.5 * 0.25 * tema.Rem
	margenLista    = 1.5 * 0.25 * tema.Rem
	margenElemento = 0.5 * 0.25 * tema.Rem
	sangriaLista   = 6 * 0.25 * tema.Rem
	margenCita     = 2 * 0.25 * tema.Rem
	filoCita       = 3
	sangriaCita    = 3 * 0.25 * tema.Rem
	margenCodigo   = 2 * 0.25 * tema.Rem
	pxCodigo       = 3 * 0.25 * tema.Rem
	pyCodigo       = 2 * 0.25 * tema.Rem
	margenRegla    = 3 * 0.25 * tema.Rem
	margenTarea    = 1 * 0.25 * tema.Rem
	huecoTarea     = 2 * 0.25 * tema.Rem
	// LadoCasillaNativa es la casilla de Chromium sin estilo: 13 × 13.
	LadoCasillaNativa = 13
	pxCodigoLinea     = 1 * 0.25 * tema.Rem
	pxResaltado       = 0.5 * 0.25 * tema.Rem
)

// Los colores del resaltado, que la hoja escribe a mano y no son tokens.
var (
	resaltadoClaro  = color.NRGBA{R: 0xff, G: 0xf3, B: 0xa3, A: 0xff}
	resaltadoOscuro = color.NRGBA{R: 0x6b, G: 0x5d, B: 0x00, A: 0xff}
)

// estiloTrozo es cómo se pinta un tramo de texto.
type estiloTrozo struct {
	escalon            tema.Escalon
	peso               ui.Peso
	cursiva, mono      bool
	subrayado, tachado bool
	enlace             bool
	tinta              color.NRGBA
	// fondo es el del resaltado o el del código en línea, con su relleno horizontal.
	fondo    color.NRGBA
	pxFondo  float32
	mayus    bool
	href     string
	codigoLn bool
}

// Trozo es un pedazo de texto con UN estilo en una línea, con la parte del texto del
// bloque que pinta: [Desde, Hasta) en runas de `Bloque.Texto()`.
type Trozo struct {
	Texto        string
	X, Ancho     float32
	Desde, Hasta int
	estilo       estiloTrozo
	// img es la imagen, si el trozo es una (sin texto, de una letra).
	img *imagenLinea
}

// imagenLinea es un `<img>` ya medido: la imagen (nil si no la hay) y su caja, que se
// apoya en la línea base como en el navegador.
type imagenLinea struct {
	img    image.Image
	an, al float32
}

// LineaProsa es una línea maquetada: su caja (Y, Alto), su línea base y sus trozos.
type LineaProsa struct {
	Bloque       int
	Y, Alto      float32
	Base         float32
	X0, X1       float32 // donde empieza y acaba el texto que cabe (para el cursor al final)
	Desde, Hasta int
	Trozos       []Trozo
	// ZonaX0 y ZonaX1 son la caja de la celda de tabla en la que va la línea: el clic y
	// las flechas buscan la línea por ella. Las dos a cero: la línea es de lado a lado.
	ZonaX0, ZonaX1 float32
	escalon        tema.Escalon
}

// Adorno es lo que la prosa pinta además del texto: viñetas, números, casillas, el filo
// de la cita, el fondo del código, la regla.
type Adorno struct {
	Tipo         tipoAdorno
	X, Y, An, Al float32
	Texto        string
	Marcada      bool
	Bloque       int
	escalon      tema.Escalon
	// color es el de un `adornoCaja`: el fondo de una celda o una línea de su rejilla.
	color color.NRGBA
	// plegada es la viñeta de un elemento plegado mientras se escribe: va en el color de
	// marca (el `::marker` de app.css).
	plegada bool
}

type tipoAdorno uint8

const (
	adornoVineta tipoAdorno = iota
	adornoNumero
	adornoCasilla
	adornoFilo
	adornoFondo
	adornoRegla
	adornoCaja
)

// Maquetado es la prosa entera a un ancho.
type Maquetado struct {
	// imagenes es el `Documento.Imagenes` de lo que se maqueta, también para sus celdas.
	imagenes func(src string) image.Image

	Lineas  []LineaProsa
	Adornos []Adorno
	Alto    float32
	// Bloques es dónde empieza y acaba cada bloque (para el editor: ir al principio de
	// uno vacío y saber qué línea es la suya).
	Bloques []CajaBloque
}

// CajaBloque es la caja de un bloque y la X donde empieza su texto.
type CajaBloque struct {
	Y, Alto float32
	X       float32
	escalon tema.Escalon
}

// estiloBloque es lo que un bloque le da a su texto y a su caja.
type estiloBloque struct {
	escalon tema.Escalon
	peso    ui.Peso
	tinta   string
	mt, mb  float32
	x, der  float32 // relleno a izquierda y derecha dentro del cuerpo
	mono    bool
	mayus   bool
	alinear string
	pyFondo float32
}

func estiloBloqueDe(b Bloque, primero bool) estiloBloque {
	e := estiloBloque{escalon: escalonCuerpo, tinta: "text", alinear: b.Alineacion}
	switch b.Tipo {
	case Parrafo:
		if b.Etiqueta == "p" {
			e.mb = margenParrafo
		}
	case Titulo:
		esc := []tema.Escalon{tema.Texto2XL, tema.TextoXL, tema.TextoLG, tema.TextoBase, tema.TextoSM, tema.TextoSM}
		n := min(max(b.Nivel, 1), 6)
		e.escalon = tema.Escalon{Tam: esc[n-1].Tam, Linea: esc[n-1].Tam * 1.25}
		e.peso, e.mt, e.mb = ui.SemiNegrita, margenTituloA, margenTituloB
		if primero {
			e.mt = 0 // `first:mt-0`
		}
		e.mayus = n == 5
		if n == 6 {
			e.tinta = "text-muted"
		}
	case Cita:
		e.tinta, e.mt, e.mb, e.x = "text-muted", margenCita, margenCita, filoCita+sangriaCita
	case Codigo:
		e.escalon = tema.TextoSM
		e.mono, e.mt, e.mb, e.x, e.der, e.pyFondo = true, margenCodigo, margenCodigo, pxCodigo, pxCodigo, pyCodigo
	case Vineta, Numerada:
		e.mt, e.mb = margenElemento, margenElemento
		e.x = sangriaLista * float32(max(b.Nivel, 1))
	case Tarea:
		e.mt, e.mb, e.x = margenTarea, margenTarea, LadoCasillaNativa+huecoTarea
	case Regla:
		e.mt, e.mb = margenRegla, margenRegla
	case BloqueOpaco:
	}
	return e
}

// margenLista es el margen de la lista que abre o cierra un bloque de lista: el `my-1.5`
// del `ul` colapsa con el `my-0.5` de su primer y su último `li`.
func esLista(t TipoBloque) bool { return t == Vineta || t == Numerada }

// Maquetar pone el documento a ese ancho. `v` decide los colores.
//
//nolint:gocognit,gocyclo // El flujo lineal reproduce las reglas de maquetación del documento.
func Maquetar(d *Documento, ancho float32, v tema.Variante) *Maquetado {
	m := &Maquetado{}
	if d == nil {
		return m
	}
	m.imagenes = d.Imagenes
	var y, margenPrevio float32
	var numeros [8]int
	numero := 0
	// sigueLista dice si el bloque `k` es de lista y el de antes también, en la misma
	// celda: una lista no sigue a través de una tabla.
	sigueLista := func(k, otro int) bool {
		return otro >= 0 && otro < len(d.Bloques) && esLista(d.Bloques[otro].Tipo) &&
			d.Bloques[otro].Celda == d.Bloques[k].Celda
	}
	ocultos := d.ocultos()
	for i := 0; i < len(d.Bloques); i++ {
		b := d.Bloques[i]
		if i < len(ocultos) && ocultos[i] {
			// Cuelga de un elemento plegado (plegar.go): no ocupa, pero conserva su caja
			// para que los índices de `Bloques` sigan siendo los del documento.
			m.Bloques = append(m.Bloques, CajaBloque{Y: y})
			continue
		}
		if i+1 < len(ocultos) && ocultos[i+1] {
			b = conPuntos(b, v)
		}
		if b.Celda.EnTabla() {
			// La tabla no tiene margen: el de arriba entero, y abajo ninguno.
			j := i + 1
			for j < len(d.Bloques) && d.Bloques[j].Celda.Tabla == b.Celda.Tabla {
				j++
			}
			y += margenPrevio
			alto, cajas := m.tabla(d.Bloques[i:j], func(k int) int { return i + k }, y, ancho, v)
			m.Bloques = append(m.Bloques, cajas...)
			y += alto
			margenPrevio = 0
			i = j - 1
			continue
		}
		e := estiloBloqueDe(b, i == 0)
		mt := e.mt
		if esLista(b.Tipo) && !sigueLista(i, i-1) {
			mt = max(mt, margenLista)
		}
		if i > 0 {
			y += max(margenPrevio, mt) // colapso de márgenes entre hermanos
		} else {
			y += mt
		}
		mb := e.mb
		if esLista(b.Tipo) && !sigueLista(i, i+1) {
			mb = max(mb, margenLista)
		}
		if b.Tipo == Parrafo && b.Etiqueta == "p" && i == len(d.Bloques)-1 {
			mb = 0 // `last:mb-0`
		}
		// La numeración va POR NIVEL: una lista anidada cuenta desde uno y, al volver,
		// la de fuera sigue por donde iba.
		if esLista(b.Tipo) {
			n := min(max(b.Nivel, 1), len(numeros)) - 1
			if !sigueLista(i, i-1) {
				numeros = [8]int{}
			}
			for k := n + 1; k < len(numeros); k++ {
				numeros[k] = 0
			}
			if b.Tipo == Numerada {
				numeros[n]++
			} else {
				numeros[n] = 0
			}
			numero = numeros[n]
		}
		alto := m.bloque(i, b, e, y, ancho, numero, v)
		if b.Plegada && d.desplegado && len(m.Adornos) > 0 {
			if a := &m.Adornos[len(m.Adornos)-1]; a.Bloque == i && (a.Tipo == adornoVineta || a.Tipo == adornoNumero) {
				a.plegada = true
			}
		}
		m.Bloques = append(m.Bloques, CajaBloque{Y: y, Alto: alto, X: e.x, escalon: e.escalon})
		y += alto
		margenPrevio = mb
	}
	// El último margen de abajo se queda dentro del cuerpo: es un contexto propio.
	m.Alto = y + margenPrevio
	return m
}

// bloque maqueta un bloque desde `y` y devuelve su alto.
func (m *Maquetado) bloque(i int, b Bloque, e estiloBloque, y, ancho float32, numero int, v tema.Variante) float32 {
	switch b.Tipo {
	case Regla:
		m.Adornos = append(m.Adornos, Adorno{Tipo: adornoRegla, Bloque: i, X: 0, Y: y, An: ancho, Al: tema.Borde})
		return tema.Borde
	case BloqueOpaco:
		if b.Etiqueta == "table" {
			return m.tablaOpaca(i, b, y, ancho, v)
		}
	case Codigo:
		// El fondo va detrás del texto, con su relleno.
		inicio := len(m.Adornos)
		m.Adornos = append(m.Adornos, Adorno{Tipo: adornoFondo, Bloque: i, X: 0, Y: y})
		alto := m.lineas(i, b, e, y+e.pyFondo, ancho, v) + 2*e.pyFondo
		m.Adornos[inicio].An, m.Adornos[inicio].Al = ancho, alto
		return alto
	default:
	}
	alto := m.lineas(i, b, e, y, ancho, v)
	switch b.Tipo {
	case Cita:
		m.Adornos = append(m.Adornos, Adorno{Tipo: adornoFilo, Bloque: i, X: 0, Y: y, An: filoCita, Al: alto})
	case Vineta, Numerada:
		primera := m.primeraLinea(i)
		a := Adorno{Tipo: adornoVineta, Bloque: i, X: e.x, Y: primera.Y, Al: primera.Alto, escalon: e.escalon}
		if b.Tipo == Numerada {
			a.Tipo, a.Texto = adornoNumero, strconv.Itoa(numero)+"."
		}
		m.Adornos = append(m.Adornos, a)
	case Tarea:
		primera := m.primeraLinea(i)
		m.Adornos = append(m.Adornos, Adorno{
			Tipo: adornoCasilla, Bloque: i, X: 0, Y: y + (alto-LadoCasillaNativa)/2,
			An: LadoCasillaNativa, Al: LadoCasillaNativa, Marcada: b.Marcada, escalon: primera.escalon,
		})
	case Parrafo, Titulo, Codigo, Regla, BloqueOpaco:
	}
	return alto
}

func (m *Maquetado) primeraLinea(bloque int) LineaProsa {
	for _, l := range m.Lineas {
		if l.Bloque == bloque {
			return l
		}
	}
	return LineaProsa{}
}

// palabra es la unidad de reparto: letras seguidas SIN blanco, con el blanco que las
// sigue, partida en pedazos de un mismo estilo.
type pedazo struct {
	texto  string
	desde  int
	estilo estiloTrozo
	blanco bool // es el blanco de detrás
	salto  bool // `<br>`
	img    *imagenLinea
}

// ancho es lo que ocupa un pedazo en la línea.
func (p pedazo) ancho() float32 {
	if p.img != nil {
		return p.img.an
	}
	return medirTrozo(p.texto, p.estilo)
}

// letras son las que cuenta en el bloque: una imagen, una.
func (p pedazo) letras() int {
	if p.img != nil {
		return 1
	}
	return utf8.RuneCountInString(p.texto)
}

// lineas reparte el texto del bloque en líneas desde `y` y devuelve lo que ocupan. Un
// bloque vacío ocupa UNA línea: en la web un párrafo vacío (`<p><br></p>`) se ve.
func (m *Maquetado) lineas(i int, b Bloque, e estiloBloque, y, ancho float32, v tema.Variante) float32 {
	cabe := max(ancho-e.x-e.der, 1)
	pedazos := m.pedazosDe(b, e, v, cabe)
	var (
		fila   []pedazo
		x      float32
		alto   float32
		inicio = y
	)
	cerrar := func(fin int) {
		l := m.componer(i, fila, e, y, cabe, fin)
		m.Lineas = append(m.Lineas, l)
		y += l.Alto
		alto += l.Alto
		fila, x = nil, 0
	}
	pos := 0
	for k := 0; k < len(pedazos); {
		// La palabra: pedazos hasta un blanco o un salto (incluidos).
		fin := k
		var anchoPal, anchoSinBlanco float32
		for fin < len(pedazos) {
			p := pedazos[fin]
			w := p.ancho()
			if !p.blanco && !p.salto {
				anchoSinBlanco = anchoPal + w
			}
			anchoPal += w
			fin++
			if p.blanco || p.salto {
				break
			}
		}
		palabra := pedazos[k:fin]
		if len(fila) > 0 && x+anchoSinBlanco > cabe {
			cerrar(palabra[0].desde)
		}
		if x == 0 && anchoSinBlanco > cabe {
			// Una palabra más larga que la línea se parte por letras (`break-words`).
			for _, p := range partirPalabra(palabra, cabe-x) {
				if len(fila) > 0 && x+p.ancho() > cabe && !p.blanco {
					cerrar(p.desde)
				}
				fila = append(fila, p)
				x += p.ancho()
			}
		} else {
			fila = append(fila, palabra...)
			x += anchoPal
		}
		last := palabra[len(palabra)-1]
		pos = last.desde + last.letras()
		if last.salto {
			cerrar(pos)
		}
		k = fin
	}
	if len(fila) > 0 || alto == 0 {
		cerrar(pos)
	}
	return y - inicio
}

// componer arma una línea con sus pedazos: los junta en trozos del mismo estilo, los
// alinea y calcula su caja.
func (m *Maquetado) componer(i int, fila []pedazo, e estiloBloque, y, cabe float32, fin int) LineaProsa {
	l := LineaProsa{Bloque: i, Y: y, Alto: e.escalon.Linea, escalon: e.escalon, Desde: fin, Hasta: fin}
	if len(fila) > 0 {
		l.Desde = fila[0].desde
	}
	var x float32
	for _, p := range fila {
		if p.salto {
			continue
		}
		w := p.ancho()
		n := len(l.Trozos)
		if n > 0 && p.img == nil && l.Trozos[n-1].img == nil && l.Trozos[n-1].estilo == p.estilo && l.Trozos[n-1].Hasta == p.desde {
			t := &l.Trozos[n-1]
			t.Texto += p.texto
			t.Hasta += utf8.RuneCountInString(p.texto)
			t.Ancho = medirTrozo(t.Texto, t.estilo)
		} else {
			l.Trozos = append(l.Trozos, Trozo{
				Texto: p.texto, X: x, Ancho: w, Desde: p.desde,
				Hasta: p.desde + p.letras(), estilo: p.estilo, img: p.img,
			})
		}
		x = l.Trozos[len(l.Trozos)-1].X + l.Trozos[len(l.Trozos)-1].Ancho
		l.Hasta = l.Trozos[len(l.Trozos)-1].Hasta
	}
	// Los blancos del final no cuentan para alinear, igual que en el navegador.
	util := x
	if n := len(l.Trozos); n > 0 && l.Trozos[n-1].img == nil {
		t := l.Trozos[n-1]
		util = t.X + medirTrozo(strings.TrimRightFunc(t.Texto, unicode.IsSpace), t.estilo)
	}
	desp := e.x
	switch e.alinear {
	case "center":
		desp += max((cabe-util)/2, 0)
	case "right":
		desp += max(cabe-util, 0)
	}
	for k := range l.Trozos {
		l.Trozos[k].X += desp
	}
	l.X0, l.X1 = desp, desp+x
	// La línea base. Con una sola letra es la de la caja de línea del bloque, como en
	// `ColocarTexto`; con tamaños distintos o una imagen, la caja crece como en CSS: cada
	// trozo pide su media interlínea por encima y por debajo de la base (con el
	// `line-height` del bloque en proporción, que es como se hereda `leading-relaxed`), y
	// una imagen, su alto entero encima.
	razon := e.escalon.Linea / e.escalon.Tam
	arriba, abajo := metrica(e.escalon.Tam, e.escalon.Linea)
	for _, t := range l.Trozos {
		if t.img != nil {
			arriba = max(arriba, t.img.al)
			continue
		}
		a, b := metrica(t.estilo.escalon.Tam, t.estilo.escalon.Tam*razon)
		arriba, abajo = max(arriba, a), max(abajo, b)
	}
	l.Alto = max(l.Alto, arriba+abajo)
	l.Base = y + arriba
	return l
}

// metrica es lo que una letra de ese tamaño y esa interlínea ocupa por encima y por
// debajo de su línea base.
func metrica(tam, linea float32) (arriba, abajo float32) {
	arriba = (linea-altoFuenteDe(tam))/2 + ascensoFigtree*tam
	return arriba, linea - arriba
}

// Figtree: 950 de ascenso y 250 de descenso sobre 1000 (tabla hhea del .ttf de
// apps/nucleo/fuentes). La caja de la fuente es 1,2 em.
const (
	ascensoFigtree = 0.95
	cajaFigtree    = 1.2
	// tachadoFigtree y subrayadoFigtree son la posición de la raya (OS/2 y post): 0,3 em
	// por encima de la base y 0,1 em por debajo, de 0,05 em de grueso.
	tachadoFigtree   = 0.3
	subrayadoFigtree = 0.1
)

func altoFuenteDe(tam float32) float32 { return tam * cajaFigtree }

// pedazosDe convierte los tramos de un bloque en pedazos: cada tramo se parte en
// palabras y blancos, cada `<br>` es un pedazo de salto y cada imagen, uno a su medida
// (que no pasa de `cabe`, el `max-width: 100%`).
func (m *Maquetado) pedazosDe(b Bloque, e estiloBloque, v tema.Variante, cabe float32) []pedazo {
	var out []pedazo
	pos := 0
	// En una tarea, el blanco entre la casilla y su `<span>` es un hijo de un `flex` y el
	// navegador no lo pinta: cuenta como letra (el editor lo salta) pero no ocupa sitio.
	salta := inicioEditable(b)
	for _, t := range b.Tramos {
		if pos < salta {
			pos += largoTramo(t)
			continue
		}
		if t.Salto {
			out = append(out, pedazo{texto: "\n", desde: pos, salto: true})
			pos++
			continue
		}
		if t.Imagen != nil {
			out = append(out, pedazo{desde: pos, img: m.medirImagen(t.Imagen, cabe), estilo: estiloTramo(t, e, v)})
			pos++
			continue
		}
		est := estiloTramo(t, e, v)
		texto := t.Texto
		if e.mayus {
			texto = strings.ToUpper(texto)
		}
		for _, trozo := range partirBlancos(texto) {
			out = append(out, pedazo{texto: trozo, desde: pos, estilo: est, blanco: strings.TrimSpace(trozo) == ""})
			pos += utf8.RuneCountInString(trozo)
		}
	}
	return out
}

// partirBlancos parte un texto en letras y blancos alternos, sin perder nada.
func partirBlancos(s string) []string {
	var out []string
	inicio, blanco := 0, false
	for i, r := range s {
		es := unicode.IsSpace(r)
		if i > 0 && es != blanco {
			out = append(out, s[inicio:i])
			inicio = i
		}
		blanco = es
	}
	if inicio < len(s) {
		out = append(out, s[inicio:])
	}
	return out
}

// partirPalabra parte una palabra demasiado larga letra a letra, en pedazos que caben.
func partirPalabra(palabra []pedazo, cabe float32) []pedazo {
	var out []pedazo
	for _, p := range palabra {
		if p.blanco || p.salto || p.img != nil {
			out = append(out, p)
			continue
		}
		r := []rune(p.texto)
		desde := 0
		for desde < len(r) {
			hasta := desde + 1
			for hasta < len(r) && medirTrozo(string(r[desde:hasta+1]), p.estilo) <= cabe {
				hasta++
			}
			out = append(out, pedazo{texto: string(r[desde:hasta]), desde: p.desde + desde, estilo: p.estilo})
			desde = hasta
		}
	}
	return out
}

// estiloTramo resuelve las marcas de un tramo sobre el estilo de su bloque.
func estiloTramo(t Tramo, e estiloBloque, v tema.Variante) estiloTrozo {
	s := estiloTrozo{escalon: e.escalon, peso: e.peso, mono: e.mono, tinta: tema.Color(e.tinta, v), mayus: e.mayus}
	for _, marca := range t.Marcas {
		switch marca {
		case "b", "strong":
			s.peso = ui.Negrita
		case "i", "em":
			s.cursiva = true
		case "u":
			s.subrayado = true
		case "s", "strike", "del":
			s.tachado = true
		case "mark":
			s.fondo, s.pxFondo = resaltadoClaro, pxResaltado
			if v == tema.Oscuro {
				s.fondo = resaltadoOscuro
			}
		case "code":
			if !e.mono { // dentro de `pre` el código no lleva su fondo
				s.mono, s.codigoLn = true, true
				s.fondo, s.pxFondo = tema.Color("bg-subtle", v), pxCodigoLinea
				s.escalon = tema.Escalon{Tam: e.escalon.Tam * 0.92, Linea: e.escalon.Linea}
			}
		}
	}
	if t.Enlace != "" {
		s.enlace, s.subrayado, s.href = true, true, t.Enlace
		s.tinta = tema.Color("primary-text", v)
	}
	if c, ok := tokens.ParseColor(t.Color); ok && t.Color != "" {
		s.tinta = c
	}
	if c, ok := tokens.ParseColor(t.Fondo); ok && t.Fondo != "" {
		s.fondo = c
	}
	if tam, ok := tamanoCSS(t.Tamano, e.escalon.Tam); ok {
		s.escalon = tema.Escalon{Tam: tam, Linea: tam * e.escalon.Linea / e.escalon.Tam}
	}
	if monoespaciada(t.Fuente) {
		s.mono = true
	}
	return s
}

// tamanoCSS lee un `font-size` sobre el tamaño `base` de su bloque: px, pt, em, rem, % y
// las palabras de CSS (con los px de Chromium).
func tamanoCSS(v string, base float32) (float32, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	palabras := map[string]float32{
		"xx-small": 9, "x-small": 10, "small": 13, "medium": 16,
		"large": 18, "x-large": 24, "xx-large": 32, "xxx-large": 48,
	}
	if px, ok := palabras[v]; ok {
		return px, true
	}
	for _, u := range []struct {
		sufijo string
		por    float32
	}{{"rem", tema.Rem}, {"px", 1}, {"pt", 4.0 / 3}, {"em", base}, {"%", base / 100}} {
		if n, ok := strings.CutSuffix(v, u.sufijo); ok {
			f, err := strconv.ParseFloat(strings.TrimSpace(n), 32)
			if err != nil || f <= 0 {
				return 0, false
			}
			return float32(f) * u.por, true
		}
	}
	return 0, false
}

// monoespaciada dice si un `font-family` pide letra de ancho fijo.
//
// ponytail: de las seis de la barra de Mail solo se distingue la monoespaciada (Courier
// New); Arial, Georgia, Times, Verdana y Tahoma se pintan con la Figtree, porque la app no
// lleva fuentes con remates. Se escriben tal cual y el destinatario las ve bien; embeber
// una con remates (una OFL) si alguien echa en falta verlas aquí.
func monoespaciada(familia string) bool {
	f := strings.ToLower(familia)
	return strings.Contains(f, "monospace") || strings.Contains(f, "courier") || strings.Contains(f, "mono")
}

// ladoImagenRota es el hueco de una imagen que no se tiene (todavía) y no dice su tamaño:
// el icono de imagen rota de Chromium.
const ladoImagenRota = 16

// medirImagen da la caja de un `<img>`: sus `width`/`height` si los trae, o los de la
// imagen; y si es más ancha que la línea, a lo ancho y en proporción.
func (m *Maquetado) medirImagen(attrs []html.Attribute, cabe float32) *imagenLinea {
	im := &imagenLinea{an: px(attr(attrs, "width")), al: px(attr(attrs, "height"))}
	if m.imagenes != nil {
		im.img = m.imagenes(attr(attrs, "src"))
	}
	if im.img != nil {
		t := im.img.Bounds().Size()
		w, h := float32(t.X), float32(t.Y)
		switch {
		case im.an == 0 && im.al == 0:
			im.an, im.al = w, h
		case im.an == 0 && h > 0:
			im.an = im.al * w / h
		case im.al == 0 && w > 0:
			im.al = im.an * h / w
		}
	}
	if im.an <= 0 || im.al <= 0 {
		im.an, im.al = ladoImagenRota, ladoImagenRota
	}
	if im.an > cabe {
		im.an, im.al = cabe, im.al*cabe/im.an
	}
	return im
}

// caraDe es la fuente de un estilo: las cinco Figtree y, para el código, la monoespaciada
// del tema.
func caraDe(s estiloTrozo) (fyne.Resource, fyne.TextStyle) {
	switch {
	case s.mono:
		return nil, fyne.TextStyle{Monospace: true}
	case s.cursiva:
		return tema.Cursiva, fyne.TextStyle{}
	}
	return s.peso.Cara(), fyne.TextStyle{}
}

// medirTrozo es el ancho de un texto con ese estilo, relleno del fondo incluido.
func medirTrozo(texto string, s estiloTrozo) float32 {
	if texto == "" {
		return 0
	}
	cara, estilo := caraDe(s)
	tam, _ := fyne.CurrentApp().Driver().RenderedTextSize(texto, s.escalon.Tam, estilo, cara)
	return tam.Width
}

// tamEn es el tamaño de la letra de delante de `pos` en la línea (la que da estilo a lo
// que se escribe): el alto del cursor.
// El código en línea no cuenta (su 0,92 em no achica el cursor, como en el navegador).
func (l LineaProsa) tamEn(pos int) float32 {
	for k, t := range l.Trozos {
		if t.img == nil && !t.estilo.codigoLn && (pos > t.Desde || k == 0) && pos <= t.Hasta {
			return t.estilo.escalon.Tam
		}
	}
	return l.escalon.Tam
}

// XDe es la X de la letra `pos` del bloque en esa línea: donde va el cursor.
func (l LineaProsa) XDe(pos int) float32 {
	if len(l.Trozos) == 0 || pos <= l.Desde {
		if len(l.Trozos) > 0 {
			return l.Trozos[0].X
		}
		return l.X0
	}
	for _, t := range l.Trozos {
		if pos >= t.Desde && pos <= t.Hasta {
			if t.img != nil {
				return t.X + float32(pos-t.Desde)*t.Ancho
			}
			r := []rune(t.Texto)
			return t.X + medirTrozo(string(r[:pos-t.Desde]), t.estilo)
		}
	}
	u := l.Trozos[len(l.Trozos)-1]
	return u.X + u.Ancho
}

// PosEn es la letra del bloque más cercana a la X dada en esa línea: el clic.
func (l LineaProsa) PosEn(x float32) int {
	if len(l.Trozos) == 0 {
		return l.Desde
	}
	mejor, dist := l.Desde, float32(1e9)
	for _, t := range l.Trozos {
		r := []rune(t.Texto)
		if t.img != nil {
			r = []rune{Objeto}
		}
		for k := 0; k <= len(r); k++ {
			px := t.X + float32(k)*t.Ancho
			if t.img == nil {
				px = t.X + medirTrozo(string(r[:k]), t.estilo)
			}
			d := px - x
			if d < 0 {
				d = -d
			}
			if d < dist {
				mejor, dist = t.Desde+k, d
			}
		}
	}
	// El salto de un `<br>` al final de la línea no es una posición en la que quedarse.
	return min(mejor, l.Hasta)
}

// Prosa es el cuerpo de una nota en LECTURA: el documento maquetado y pintado.
//
// Pulsarla es empezar a escribir (`onEdit` de la web), salvo sobre un enlace, que se
// sigue. Es un widget por la regla 1: se pulsa y se anuncia (con el texto entero).
type Prosa struct {
	widget.BaseWidget

	Doc *Documento
	// Llenar es la tarjeta estirada: la prosa crece hasta el alto que le den.
	Llenar bool
	// Marca es lo que se pinta con el documento vacío (`note.placeholder`).
	Marca string

	AlPulsar func()
	AlEnlace func(href string)
	// AlPlegar pliega o despliega el elemento de lista i: pulsar su viñeta (plegar.go).
	// Sin él, la viñeta es texto.
	AlPlegar func(i int)
	// Previa da la ficha de un enlace a otra nota (previa.go): la vista previa y si ya
	// está; si no, llama a `listo` cuando llegue. tr traduce sus textos.
	Previa func(href string, listo func()) (VistaPrevia, bool)
	tr     *i18n.Translator

	maq   *Maquetado
	ancho float32
	// hrefPrevia es el enlace bajo el puntero; relojPrevia, el retardo antes de abrir su
	// ficha, y ficha, la ficha abierta.
	hrefPrevia  string
	relojPrevia *time.Timer
	ficha       *flotante
	// portal es donde va la ficha para que no la tape la tarjeta de debajo (flotante.go).
	portal *portal
}

var (
	_ fyne.Widget   = (*Prosa)(nil)
	_ fyne.Tappable = (*Prosa)(nil)
)

// NuevaProsa crea el cuerpo de un documento.
func NuevaProsa(d *Documento) *Prosa {
	p := &Prosa{Doc: d}
	p.ExtendBaseWidget(p)
	return p
}

// AltoPara es lo que ocupa a ese ancho: `min-h-[24px]` como suelo.
//
// Sin ancho todavía (0: no se ha colocado) se mide SIN repartir: repartir en una columna
// de un píxel pondría cada letra en su línea y el mínimo saldría de cientos de píxeles.
func (p *Prosa) AltoPara(ancho float32) float32 {
	if ancho <= 0 {
		ancho = 1e6
	}
	return max(Maquetar(p.Doc, ancho, tema.Claro).Alto, 24)
}

// AccessibilityLabel es el texto entero de la nota.
func (p *Prosa) AccessibilityLabel() string {
	var b strings.Builder
	for i, bl := range p.Doc.Bloques {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(bl.Texto())
	}
	return b.String()
}

// AccessibilityRole: se lee como texto.
func (p *Prosa) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// Tapped sigue el enlace de debajo, si lo hay, y si no empieza a escribir. Sobre la viñeta
// de un elemento con sublista, la pliega o la despliega.
func (p *Prosa) Tapped(e *fyne.PointEvent) {
	if p.AlPlegar != nil && p.maq != nil {
		if i := p.maq.viñetaEn(e.Position.X, e.Position.Y); i >= 0 && p.Doc.tieneSublista(i) {
			p.AlPlegar(i)
			return
		}
	}
	if href := p.enlaceEn(e.Position); href != "" && p.AlEnlace != nil {
		p.AlEnlace(href)
		return
	}
	llamar0(p.AlPulsar)
}

// Cursor es el de texto cuando se puede escribir, como el `cursor-text` de la web.
func (p *Prosa) Cursor() desktop.Cursor {
	if p.AlPulsar != nil {
		return desktop.TextCursor
	}
	return desktop.DefaultCursor
}

// enlaceEn es el `href` del trozo bajo ese punto.
func (p *Prosa) enlaceEn(pos fyne.Position) string {
	if p.maq == nil {
		return ""
	}
	for _, l := range p.maq.Lineas {
		if pos.Y < l.Y || pos.Y > l.Y+l.Alto {
			continue
		}
		for _, t := range l.Trozos {
			if t.estilo.href != "" && pos.X >= t.X && pos.X <= t.X+t.Ancho {
				return t.estilo.href
			}
		}
	}
	return ""
}

// CreateRenderer: los objetos se rehacen en cada Layout, que es cuando se sabe el ancho.
func (p *Prosa) CreateRenderer() fyne.WidgetRenderer {
	r := &prosaRenderer{p: p}
	return r
}

type prosaRenderer struct {
	p       *Prosa
	objetos []fyne.CanvasObject
}

func (r *prosaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, r.p.AltoPara(max(r.p.ancho, r.p.Size().Width)))
}

func (r *prosaRenderer) Objects() []fyne.CanvasObject {
	if r.p.ficha != nil && r.p.portal == nil {
		return append(r.objetos[:len(r.objetos):len(r.objetos)], r.p.ficha)
	}
	return r.objetos
}
func (r *prosaRenderer) Destroy() {}

func (r *prosaRenderer) Layout(t fyne.Size) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.p.ancho = t.Width
	r.p.maq = Maquetar(r.p.Doc, t.Width, v)
	r.objetos = PintarProsa(r.p.maq, v)
	if r.p.Doc.Vacio() && r.p.Marca != "" {
		m := ui.Texto(r.p.Marca, escalonCuerpo, ui.Normal, tema.Color("text-subtle", v))
		ui.ColocarTexto(m, escalonCuerpo, 0, 0)
		r.objetos = append(r.objetos, m)
	}
}

func (r *prosaRenderer) Refresh() {
	r.Layout(r.p.Size())
	canvas.Refresh(r.p)
}

// PintarProsa convierte un maquetado en objetos de lienzo, ya colocados.
func PintarProsa(m *Maquetado, v tema.Variante) []fyne.CanvasObject {
	var out []fyne.CanvasObject
	for _, a := range m.Adornos {
		out = append(out, pintarAdorno(a, v)...)
	}
	for _, l := range m.Lineas {
		for _, t := range l.Trozos {
			out = append(out, pintarTrozo(l, t, v)...)
		}
	}
	return out
}

func rect(c color.NRGBA, x, y, an, al, radio float32) *canvas.Rectangle {
	r := canvas.NewRectangle(c)
	r.CornerRadius = radio
	r.Move(fyne.NewPos(x, y))
	r.Resize(fyne.NewSize(an, al))
	return r
}

func pintarAdorno(a Adorno, v tema.Variante) []fyne.CanvasObject {
	switch a.Tipo {
	case adornoRegla:
		return []fyne.CanvasObject{rect(tema.Color("border", v), a.X, a.Y, a.An, a.Al, 0)}
	case adornoFilo:
		return []fyne.CanvasObject{rect(tema.Color("border", v), a.X, a.Y, a.An, a.Al, 0)}
	case adornoFondo:
		return []fyne.CanvasObject{rect(tema.Color("bg-subtle", v), a.X, a.Y, a.An, a.Al, tema.RadioMD)}
	case adornoVineta:
		// El `disc` de Chromium: un círculo de 0,4 em, a 0,75 em del texto y centrado
		// en la altura de la x.
		d := a.escalon.Tam * 0.4
		cx := a.X - a.escalon.Tam*0.75
		cy := a.Y + (a.Al-altoFuenteDe(a.escalon.Tam))/2 + ascensoFigtree*a.escalon.Tam - 0.26*a.escalon.Tam
		return []fyne.CanvasObject{rect(tema.Color(tintaVineta(a), v), cx-d/2, cy-d/2, d, d, d/2)}
	case adornoNumero:
		// «1. » acaba donde empieza el texto: el número y su espacio van por fuera.
		est := estiloTrozo{escalon: a.escalon}
		w := medirTrozo(a.Texto+" ", est)
		t := ui.Texto(a.Texto, a.escalon, ui.Normal, tema.Color(tintaVineta(a), v))
		ui.ColocarTexto(t, a.escalon, a.X-w, a.Y)
		return []fyne.CanvasObject{t}
	case adornoCasilla:
		return casillaNativa(a, v)
	case adornoCaja:
		return []fyne.CanvasObject{rect(a.color, a.X, a.Y, a.An, a.Al, 0)}
	}
	return nil
}

// casillaNativa es el `<input type=checkbox>` de Chromium con `accent-primary`, 13 × 13:
// la pieza de Chromium de `ui`, la misma que la del ERP y la de CertiPDF.
func casillaNativa(a Adorno, v tema.Variante) []fyne.CanvasObject {
	p := ui.NuevaPiezaCasilla(a.An, false, false, v)
	p.Colocar(fyne.NewPos(a.X, a.Y))
	p.Pintar(ui.EstadoCasilla{Marcada: a.Marcada}, v)
	return p.Objetos()
}

// pintarTrozo pinta un trozo: su fondo (resaltado o código), el texto y sus rayas; o la
// imagen, apoyada en la línea base.
func pintarTrozo(l LineaProsa, t Trozo, v tema.Variante) []fyne.CanvasObject {
	s := t.estilo
	if t.img != nil {
		y := l.Base - t.img.al
		if t.img.img == nil {
			// La que no se tiene: un hueco con su borde, como el icono de imagen rota.
			hueco := rect(tema.Color("bg-subtle", v), t.X, y, t.img.an, t.img.al, tema.RadioSM)
			hueco.StrokeColor, hueco.StrokeWidth = tema.Color("border", v), 1
			return []fyne.CanvasObject{hueco}
		}
		im := canvas.NewImageFromImage(t.img.img)
		im.FillMode = canvas.ImageFillStretch
		im.Move(fyne.NewPos(t.X, y))
		im.Resize(fyne.NewSize(t.img.an, t.img.al))
		return []fyne.CanvasObject{im}
	}
	var out []fyne.CanvasObject
	if s.fondo.A > 0 {
		// El fondo en línea cubre la caja de la FUENTE (no la de línea) con su relleno.
		alto := altoFuenteDe(s.escalon.Tam)
		arriba := l.Base - ascensoFigtree*s.escalon.Tam
		radio := float32(0.25 * tema.Rem)
		if !s.codigoLn {
			radio = tema.RadioSM
		}
		out = append(out, rect(s.fondo, t.X-s.pxFondo, arriba, t.Ancho+2*s.pxFondo, alto, radio))
	}
	cara, estilo := caraDe(s)
	x := canvas.NewText(t.Texto, s.tinta)
	x.TextSize, x.TextStyle, x.FontSource = s.escalon.Tam, estilo, cara
	caja := x.MinSize()
	// La base de la fuente cae en la base de la línea.
	x.Move(fyne.NewPos(t.X, l.Base-ascensoFigtree*s.escalon.Tam))
	x.Resize(caja)
	out = append(out, x)
	grueso := max(float32(1), 0.05*s.escalon.Tam)
	visible := medirTrozo(strings.TrimRightFunc(t.Texto, unicode.IsSpace), s)
	if s.subrayado {
		y := l.Base + subrayadoFigtree*s.escalon.Tam
		if s.enlace {
			y = l.Base + 2 // `underline-offset-2`
		}
		out = append(out, rect(s.tinta, t.X, y, visible, grueso, 0))
	}
	if s.tachado {
		out = append(out, rect(s.tinta, t.X, l.Base-tachadoFigtree*s.escalon.Tam, visible, grueso, 0))
	}
	return out
}

// colocarTachado pone la raya de `line-through` sobre un texto de ese escalón colocado
// con `ui.ColocarTexto` en (x, y) de su caja de línea: 0,3 em por encima de la base y un
// píxel de grueso (0,05 em redondeado, como hace el navegador), a lo ancho del texto.
func colocarTachado(r *canvas.Rectangle, e tema.Escalon, x, y, ancho float32) {
	base := y + (e.Linea-altoFuenteDe(e.Tam))/2 + ascensoFigtree*e.Tam
	r.Move(fyne.NewPos(x, base-tachadoFigtree*e.Tam))
	r.Resize(fyne.NewSize(ancho, max(float32(1), 0.05*e.Tam)))
}
