// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"  // descodificador: las imágenes del folio llegan como data: URL
	_ "image/jpeg" // descodificador
	_ "image/png"  // descodificador
	"math"
	"slices"
	"strings"
	"sync"
	"unicode"

	"fyne.io/fyne/v2"
	_ "golang.org/x/image/webp" // descodificador

	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/externo"
)

// La MAQUETA del folio: los bloques del modelo repartidos en líneas y piezas con su
// posición, en px de maqueta (sin zoom) y relativos a la caja de contenido de la página
// —la esquina de dentro de los márgenes—. Es lo que hace el navegador con el
// contenteditable, y lo que la vista pinta, donde el cursor se coloca y donde un clic cae.
//
// Las reglas son las de CSS, no unas propias:
//
//   - el reparto es codicioso y corta en los espacios; una palabra que no cabe sola se
//     parte por letras, que es el `overflow-wrap: break-word` que el navegador pone a todo
//     contenteditable;
//   - los espacios del final de una línea cuelgan: no cuentan para centrar ni alinear;
//   - la caja de cada línea es la de su texto más alto con la línea base común, con el
//     semi-interlineado de CSS repartido arriba y abajo (`line-height: 1.5` es un número y
//     se hereda como número: un tramo a 24 px pide una línea de 36);
//   - los márgenes entre bloques hermanos se COLAPSAN al mayor de los dos.

// Métricas verticales de Figtree (hhea): ascendente 950 y descendente 250 por mil, que
// es la caja de 1,2 em que `canvas.Text` declara de alto.
const (
	ascFuente  = 0.95
	cajaFuente = 1.2
)

// Medidas de CONTENIDO de `editor.css`, en px literales (el folio no usa el rem de 14).
const (
	sangriaLista = 24 // `ul, ol { padding-left: 1.5em }` a 16 px
	barraCita    = 3  // `blockquote { border-left: 3px }`
	aireCita     = 12 // `padding: 2px 0 2px 12px`
	vertCita     = 2
	celdaX       = 8 // `td, th { padding: 4px 8px }`
	celdaY       = 4
)

// pieza es un trozo de una línea con un solo estilo, o una imagen.
type pieza struct {
	desde, hasta int // runas del bloque que ocupa
	texto        string
	x, ancho     float32 // desde el principio de la línea, sin la alineación
	est          rico.Estilo
	img          *rico.Imagen
	alto, arriba float32 // de una imagen: su alto y dónde empieza, desde arriba de la línea
	// atomo es una pieza que no se parte ni se junta con las de al lado: el número de
	// página (su texto es de la vista, y ocupa una runa) y el hueco de una región.
	atomo bool
}

// linea es una línea de un bloque.
type linea struct {
	desde, hasta int     // runas: [desde, hasta); un salto `<br>` queda fuera
	y, alto      float32 // desde arriba del bloque
	base         float32 // la línea base, desde arriba de la línea
	x0           float32 // el desplazamiento de la alineación (y de la sangría de primera línea)
	extra        float32 // lo que se añade a cada espacio en una línea justificada
	piezas       []pieza
	// flotas son las imágenes que flotan desde esta línea, con lo que se ve de ellas, en px
	// desde la esquina de las líneas del bloque. En `piezas` ocupan su runa sin ancho.
	flotas []flotaMaq
}

// flotaMaq es una imagen que flota ya colocada: lo que se ve de ella.
type flotaMaq struct {
	img        *rico.Imagen
	x, y, w, h float32
}

// flotante es una imagen que flota, en las coordenadas de su `apilar`: su caja de margen,
// que es lo que aparta a las líneas.
type flotante struct {
	img            *rico.Imagen
	x0, x1, y0, y1 float32
}

// flotantes son los de una columna (el folio, una celda): un flotante de un párrafo aparta
// también las líneas de los de debajo, como en CSS.
type flotantes struct{ lista []flotante }

// franja es lo que queda libre en [izq, der) a la altura y.
func (fl *flotantes) franja(y, izq, der float32) (float32, float32) {
	if fl == nil {
		return izq, der
	}
	for _, f := range fl.lista {
		if y < f.y0 || y >= f.y1 {
			continue
		}
		if f.img.Alinear == "right" {
			der = min(der, f.x0)
		} else {
			izq = max(izq, f.x1)
		}
	}
	return izq, der
}

// poner coloca un flotante de caja w × h arriba de la línea, al borde de lo libre, y
// devuelve lo que queda libre al lado.
func (fl *flotantes) poner(im *rico.Imagen, w, h, izq, der, arriba float32) (float32, float32) {
	if fl == nil {
		return izq, der
	}
	f := flotante{img: im, x0: izq, x1: izq + w, y0: arriba, y1: arriba + h}
	if im.Alinear == "right" {
		f.x0, f.x1 = der-w, der
		der = f.x0
	} else {
		izq = f.x1
	}
	fl.lista = append(fl.lista, f)
	return izq, der
}

// fondo es donde acaba lo más bajo que flota.
func (fl *flotantes) fondo() float32 {
	var y float32
	if fl != nil {
		for _, f := range fl.lista {
			y = max(y, f.y1)
		}
	}
	return y
}

// siguienteFondo es dónde acaba el primero de los que flotan a la altura y: a partir de
// ahí hay más sitio.
func (fl *flotantes) siguienteFondo(y float32) float32 {
	abajo := y
	if fl != nil {
		for _, f := range fl.lista {
			if y >= f.y0 && y < f.y1 && (abajo == y || f.y1 < abajo) {
				abajo = f.y1
			}
		}
	}
	return abajo
}

// colocarEn apunta en la línea lo que se ve de las imágenes que flotan desde ella: en la
// esquina de su caja de margen —el recorte se come con márgenes negativos— y, flotando a la
// derecha, tras el hueco.
func (fl *flotantes) colocarEn(l *linea, orgX, orgY float32) {
	if fl == nil {
		return
	}
	for _, p := range l.piezas {
		if p.img == nil || !p.img.Rodeada() {
			continue
		}
		for _, f := range fl.lista {
			if f.img != p.img {
				continue
			}
			w, h := visibleImagen(p.img, 0)
			x := f.x0
			if p.img.Alinear == "right" {
				x += rico.Hueco
			}
			l.flotas = append(l.flotas, flotaMaq{img: p.img, x: x - orgX, y: f.y0 - orgY, w: w, h: h})
		}
	}
}

// visibleImagen es lo que se ve de una imagen: su medida menos lo recortado.
func visibleImagen(im *rico.Imagen, ancho float32) (w, h float32) {
	w, h = medidaImagen(im, ancho)
	b := im.Bandas()
	return max(w-b[1]-b[3], 1), max(h-b[0]-b[2], 1)
}

// cajaFlotante es la caja de margen de una imagen que flota: lo que se ve más el hueco del
// lado del texto y el de abajo (`margin-right`/`left` y `margin-bottom` de 14).
func cajaFlotante(im *rico.Imagen) (w, h float32) {
	w, h = visibleImagen(im, 0)
	return w + rico.Hueco, h + rico.Hueco
}

// caja es un bloque ya maquetado.
type caja struct {
	b           rico.Bloque
	y, alto     float32 // la caja del bloque, desde arriba del contenido
	x           float32 // el borde izquierdo de lo que la contiene: 0, o el de su celda
	izq, ancho  float32 // la caja de sus líneas
	lineas      []linea
	largo       int     // runas del bloque
	arribaTexto float32 // dónde empiezan las líneas dentro de la caja (el aire de una cita)
	marcaAncho  float32
	marcaEstilo rico.Estilo
	marcaTexto  string
	reglaY      float32   // la raya de un <hr>, desde arriba de la caja
	tabla       *tablaMaq // la de su celda, si está en una
	recuadro    *recuadroMaq
	// colIzq y colDer son lo que abarca en x la columna del bloque, para saber en cuál cae
	// un clic; los dos a cero fuera de columnas.
	colIzq, colDer float32
}

// recuadroMaq es el borde discontinuo del índice, ya colocado: su caja entera.
type recuadroMaq struct{ y, alto, ancho float32 }

// Medidas de `.sytue-page nav[data-toc]`: `border: 1px dashed` y `padding: 8px 12px`; su
// `margin: 0 0 0.5em` es el de una tabla (`margenTabla`).
const (
	rellenoRecuadroX = 12
	rellenoRecuadroY = 8
)

// tablaMaq es una tabla ya maquetada: su caja y la de cada celda, con sus bordes.
type tablaMaq struct {
	y, alto, ancho float32
	celdas         []celdaMaq
	// sinBordes es la tabla de una caja con su propia hoja: sin rayas (ver maquetarTabla).
	sinBordes bool
}

// celdaMaq es la caja de una celda, bordes incluidos: la raya de 1 px de la izquierda y la
// de arriba son suyas, y las de la derecha y abajo, de la vecina (`border-collapse`).
type celdaMaq struct {
	x, y, w, h float32
	c          *rico.Celda
}

// medidas guarda lo ya medido: el conformador de texto es lo caro de maquetar, y un
// documento se remaqueta entero en cada tecla.
//
// ponytail: sin tope ni caducidad; un documento larguísimo editado durante horas acumula
// palabras. Relevarlo por un LRU si alguna vez se ve en memoria.
type medidas struct {
	mu sync.Mutex
	m  map[claveMedida]float32
}

type claveMedida struct {
	texto string
	tam   float32
	cara  fyne.Resource
}

var medidor = &medidas{m: map[claveMedida]float32{}}

// ancho mide un texto con el estilo de CONTENIDO.
func (m *medidas) ancho(texto string, e rico.Estilo) float32 {
	if texto == "" {
		return 0
	}
	k := claveMedida{texto, e.Tamano, caraDe(e)}
	m.mu.Lock()
	w, ok := m.m[k]
	m.mu.Unlock()
	if ok {
		return w
	}
	tam, _ := fyne.CurrentApp().Driver().RenderedTextSize(texto, e.Tamano, fyne.TextStyle{}, k.cara)
	m.mu.Lock()
	m.m[k] = tam.Width
	m.mu.Unlock()
	return tam.Width
}

// imagenes guarda las imágenes ya leídas de su data: URL, por el propio src: se maqueta y
// se pinta en cada tecla, y descodificar una foto cuesta. Lo que no se puede leer (un src
// que no es data:, un formato que no se conoce) se guarda como nil.
// ponytail: sin tope, como `medidor`.
var imagenes = struct {
	mu sync.Mutex
	m  map[string]image.Image
}{m: map[string]image.Image{}}

// imagenDe descodifica una imagen de un `data:image/…;base64,`. Una de fuera (http) no se
// pide: el folio no hace peticiones a terceros por el contenido de un documento.
func imagenDe(src string) image.Image {
	imagenes.mu.Lock()
	im, ok := imagenes.m[src]
	imagenes.mu.Unlock()
	if ok {
		return im
	}
	if cab, datos, ok := strings.Cut(src, ","); ok && strings.HasPrefix(cab, "data:image/") && strings.HasSuffix(cab, ";base64") {
		if b, err := base64.StdEncoding.DecodeString(datos); err == nil {
			im, _ = externo.Decodificar(b) // con la barrera de tamaño (FYE-04)
		}
	}
	imagenes.mu.Lock()
	imagenes.m[src] = im
	imagenes.mu.Unlock()
	return im
}

// imagenRecortada es lo que se ve de la imagen: el trozo del fichero que deja su recorte
// (`clip-path: inset(…)` sobre la imagen estirada a su caja, que es recortar el fichero en
// las mismas fracciones). Se guarda como las enteras.
func imagenRecortada(im *rico.Imagen) image.Image {
	img := imagenDe(im.Src)
	if img == nil || !im.ConRecorte() || im.Ancho <= 0 || im.Alto <= 0 {
		return img
	}
	clave := im.Src + "#" + strings.Trim(strings.ReplaceAll(fmt.Sprint(im.Recorte), " ", ","), "[]")
	imagenes.mu.Lock()
	hecha, ok := imagenes.m[clave]
	imagenes.mu.Unlock()
	if ok {
		return hecha
	}
	b := img.Bounds()
	c := im.Recorte
	r := image.Rect(b.Min.X+int(c[3]*float32(b.Dx())+0.5), b.Min.Y+int(c[0]*float32(b.Dy())+0.5),
		b.Max.X-int(c[1]*float32(b.Dx())+0.5), b.Max.Y-int(c[2]*float32(b.Dy())+0.5))
	if sub, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok && !r.Empty() {
		hecha = sub.SubImage(r)
	} else {
		hecha = img
	}
	imagenes.mu.Lock()
	imagenes.m[clave] = hecha
	imagenes.mu.Unlock()
	return hecha
}

// medidaImagen es lo que ocupa una imagen: lo que dicen sus data-* o su estilo y, si no,
// lo del fichero; una en la línea no pasa del ancho (`max-width: 100%`), y si su alto no
// está dicho (`height: auto`) encoge sin deformarse.
// ponytail: una que no se puede leer mide lo que el icono de imagen rota de Chromium,
// 16 × 16, si no dice su tamaño.
func medidaImagen(im *rico.Imagen, ancho float32) (w, h float32) {
	w, h = im.Ancho, im.Alto
	if w == 0 || h == 0 {
		nw, nh := float32(16), float32(16)
		if img := imagenDe(im.Src); img != nil && img.Bounds().Dx() > 0 && img.Bounds().Dy() > 0 {
			nw, nh = float32(img.Bounds().Dx()), float32(img.Bounds().Dy())
		}
		switch {
		case w == 0 && h == 0:
			w, h = nw, nh
		case w == 0:
			w = h * nw / nh
		default:
			h = w * nh / nw
		}
	}
	if im.Ajuste == "inline" && w > ancho && ancho > 0 {
		if im.Alto == 0 {
			h = h * ancho / w
		}
		w = ancho
	}
	return w, h
}

// caraDe es la Figtree que corresponde al peso y a la cursiva.
//
// ponytail: no hay Figtree negrita cursiva entre las cinco estáticas: un tramo que es las
// dos cosas sale en negrita recta. La web tampoco la tiene —carga solo la variable
// romana— y el navegador la INCLINA sintéticamente, igual que hace con toda cursiva; Fyne
// no sabe inclinar, así que aquí la cursiva es la de verdad. Relevar con una cara oblicua
// generada o con la variable itálica.
func caraDe(e rico.Estilo) fyne.Resource {
	// Una familia que la suite no trae se pinta con la del equipo, como el navegador;
	// si no la tiene, con Figtree.
	if f := e.Familia; f != "" && f != "Inter" && f != "Figtree" {
		if r := tema.CaraDelSistema(f, e.Peso >= 600, e.Cursiva); r != nil {
			return r
		}
	}
	switch {
	case e.Peso >= 700:
		return tema.Negra
	case e.Peso >= 600:
		return tema.SemiNegra
	case e.Cursiva:
		return tema.Cursiva
	case e.Peso >= 500:
		return tema.Media
	}
	return tema.Regular
}

// alturas son la subida y la bajada de una pieza respecto a la línea base, con su
// semi-interlineado de CSS. Un superíndice y un subíndice llevan `line-height: 0` en el
// reset de Tailwind: no empujan la línea.
func alturas(e rico.Estilo, interlineado float32) (sube, baja float32) {
	l := e.Tamano * interlineado
	if e.Vertical != 0 {
		l = 0
	}
	sube = (l-cajaFuente*e.Tamano)/2 + ascFuente*e.Tamano
	return sube, l - sube
}

// desplazamientoVertical es el `top: -0.5em` del superíndice y el `bottom: -0.25em` del
// subíndice, en px, positivo hacia abajo.
func desplazamientoVertical(e rico.Estilo) float32 {
	switch {
	case e.Vertical > 0:
		return -0.5 * e.Tamano
	case e.Vertical < 0:
		return 0.25 * e.Tamano
	}
	return 0
}

// ficha es la unidad del reparto: una palabra (con sus trozos de estilo), un espacio o
// un salto de línea.
type ficha struct {
	desde, hasta int
	trozos       []pieza
	ancho        float32
	espacio      bool
	salto        bool
	objeto       bool // una imagen: no se parte
	bloque       bool // una imagen en su propio renglón
	flota        bool // una imagen con el texto alrededor: sale de la línea
	hueco        bool // el `gap` de una región: ancho sin runas
	tab          bool // un tabulador conservado (`white-space: pre-wrap`): su ancho, hasta la parada
}

// huecoRegion es el `gap: 8px` del encabezado y del pie.
const huecoRegion = 8

// huecosFlex mete el `gap` de una región entre sus hijos, que en la web son elementos de
// un `display: flex`: una ficha sin runas, del ancho del hueco, en cada corte.
// ponytail: los hijos no se reparten el ancho ni parten dentro de sí como los de un flex;
// van en la línea, uno tras otro. Un encabezado es una línea corta.
func huecosFlex(fs []ficha, cortes []int) []ficha {
	if len(cortes) == 0 {
		return fs
	}
	out := make([]ficha, 0, len(fs)+len(cortes))
	for _, f := range fs {
		if len(cortes) > 0 && f.desde >= cortes[0] {
			hueco := pieza{desde: f.desde, hasta: f.desde, ancho: huecoRegion, atomo: true}
			out = append(out, ficha{desde: f.desde, hasta: f.desde, hueco: true, ancho: huecoRegion, trozos: []pieza{hueco}})
			cortes = cortes[1:]
		}
		out = append(out, f)
	}
	return out
}

// fichas trocea las runas del bloque en palabras, espacios, saltos e imágenes, conservando
// el estilo de cada runa. Las posiciones son las de `rico.Edicion`: un salto es una runa,
// y una imagen también. `ancho` es el de la línea, que acota el de una imagen.
func fichas(b rico.Bloque, ancho float32) []ficha {
	var out []ficha
	pos := 0
	var cur *ficha
	cerrar := func() {
		if cur != nil {
			for i := range cur.trozos {
				cur.ancho += cur.trozos[i].ancho
			}
			out = append(out, *cur)
			cur = nil
		}
	}
	añadir := func(r rune, e rico.Estilo) {
		if cur == nil {
			cur = &ficha{desde: pos}
		}
		n := len(cur.trozos)
		if n > 0 && cur.trozos[n-1].est == e {
			cur.trozos[n-1].texto += string(r)
			cur.trozos[n-1].hasta = pos + 1
		} else {
			cur.trozos = append(cur.trozos, pieza{desde: pos, hasta: pos + 1, texto: string(r), est: e})
		}
		cur.hasta = pos + 1
	}
	for _, t := range b.Tramos {
		if t.Salto {
			cerrar()
			out = append(out, ficha{desde: pos, hasta: pos + 1, salto: true})
			pos++
		}
		if t.Objeto != "" {
			// Antes y después de una imagen se puede cortar la línea, como junto a
			// cualquier objeto atómico en CSS.
			cerrar()
			out = append(out, fichaObjeto(t, pos, ancho))
			pos++
		}
		for _, r := range t.Texto {
			if f, ok := fichaBlanca(r, pos, t.Estilo); ok {
				cerrar()
				out = append(out, f)
				pos++
				continue
			}
			añadir(r, t.Estilo)
			pos++
		}
	}
	cerrar()
	if b.Region != "" {
		out = huecosFlex(out, b.CortesFlex())
	}
	// El ancho de cada trozo de palabra se mide una vez, con su texto entero: medir letra
	// a letra perdería el interletraje.
	for i := range out {
		if out[i].espacio || out[i].salto || out[i].objeto || out[i].hueco || out[i].tab {
			continue
		}
		out[i].ancho = 0
		for j := range out[i].trozos {
			tr := &out[i].trozos[j]
			tr.ancho = medidor.ancho(tr.texto, tr.est)
			out[i].ancho += tr.ancho
		}
	}
	return out
}

// fichaBlanca es la ficha de un espacio o de un tabulador. El tabulador solo llega en un bloque
// que conserva los blancos (rico.Estilo.Tabulador): avanza hasta la siguiente parada, y el ancho
// se lo da `tomar`, que sabe dónde cae.
func fichaBlanca(r rune, pos int, e rico.Estilo) (ficha, bool) {
	switch r {
	case ' ':
		p := pieza{desde: pos, hasta: pos + 1, texto: " ", est: e}
		p.ancho = medidor.ancho(" ", e)
		return ficha{desde: pos, hasta: pos + 1, trozos: []pieza{p}, ancho: p.ancho, espacio: true}, true
	case '\t':
		p := pieza{desde: pos, hasta: pos + 1, est: e, atomo: true}
		return ficha{desde: pos, hasta: pos + 1, trozos: []pieza{p}, tab: true}, true
	}
	return ficha{}, false
}

// fichaObjeto es la ficha de un objeto: una imagen con su medida. Lo que no es una imagen
// no se pinta (y el documento que lo lleva no se edita): ocupa su runa y nada más.
func fichaObjeto(t rico.Tramo, pos int, ancho float32) ficha {
	p := pieza{desde: pos, hasta: pos + 1, texto: string(rico.ObjetoRuna), est: t.Estilo}
	f := ficha{desde: pos, hasta: pos + 1, objeto: true}
	if t.CampoPagina || t.Rotulo != "" {
		// Un objeto con texto (el número de página, `white-space: nowrap`, o el enlace de
		// vuelta de una nota): el texto entero, sin partir, como una palabra.
		p.texto, p.atomo = t.Rotulo, true
		p.ancho = medidor.ancho(p.texto, p.est)
		f.trozos, f.ancho = []pieza{p}, p.ancho
		return f
	}
	if t.Objeto == "span" {
		// El marcador vacío del índice: su runa, sin nada que pintar.
		p.texto, p.atomo = "", true
	}
	if t.Imagen != nil {
		p.img = t.Imagen
		switch {
		case t.Imagen.Absoluta():
		case t.Imagen.Rodeada():
			f.flota, p.atomo = true, true // su runa, sin ancho: lo que se ve va en `flotas`
		default:
			p.ancho, p.alto = visibleImagen(t.Imagen, ancho)
		}
		f.bloque = t.Imagen.Ajuste == "topBottom"
	}
	f.trozos, f.ancho = []pieza{p}, p.ancho
	return f
}

// partirPalabra corta una palabra que no cabe sola en una línea en trozos que sí caben,
// letra a letra: el `overflow-wrap: break-word` del contenteditable.
func partirPalabra(f ficha, cabe float32) []ficha {
	var out []ficha
	cur := ficha{desde: f.desde}
	for _, tr := range f.trozos {
		runas := []rune(tr.texto)
		ini := 0
		for ini < len(runas) {
			// Cuántas runas de este trozo caben en lo que queda de línea: al menos una.
			n := 1
			for n < len(runas)-ini && cur.ancho+medidor.ancho(string(runas[ini:ini+n+1]), tr.est) <= cabe {
				n++
			}
			p := pieza{desde: tr.desde + ini, hasta: tr.desde + ini + n, texto: string(runas[ini : ini+n]), est: tr.est}
			p.ancho = medidor.ancho(p.texto, p.est)
			if cur.ancho+p.ancho > cabe && len(cur.trozos) > 0 {
				out = append(out, cur)
				cur = ficha{desde: p.desde}
			}
			cur.trozos = append(cur.trozos, p)
			cur.ancho += p.ancho
			cur.hasta = p.hasta
			ini += n
			if ini < len(runas) {
				out = append(out, cur)
				cur = ficha{desde: tr.desde + ini}
			}
		}
	}
	if len(cur.trozos) > 0 {
		out = append(out, cur)
	}
	return out
}

// tomar llena UNA línea a partir de la ficha i, en lo que dejan libre los flotantes
// [izq, der) de su altura (coordenadas de `apilar`): una palabra que no cabe sola se parte
// y el resto vuelve a la lista; una imagen que flota se coloca al principio de la línea si
// cabe al lado de lo que ya hay —si no, pasa a la siguiente— y estrecha la línea. Devuelve
// lo que va en la línea, dónde sigue, y la franja que queda.
func tomar(fs *[]ficha, i int, x, izq, der float32, fl *flotantes, arriba float32) ([]ficha, int, float32, float32) {
	var cur []ficha
	hayPalabra, hayAlgo := false, false
	for i < len(*fs) {
		f := (*fs)[i]
		cabe := der - izq
		switch {
		case f.salto:
			return append(cur, f), i + 1, izq, der
		case f.bloque:
			// Una imagen en su renglón parte la línea antes y después.
			if len(cur) > 0 {
				return cur, i, izq, der
			}
			return append(cur, f), i + 1, izq, der
		case f.flota:
			w, h := cajaFlotante(f.trozos[0].img)
			if hayAlgo && x+w > cabe {
				return cur, i, izq, der // no cabe al lado de lo escrito: a la línea de abajo
			}
			izq, der = fl.poner(f.trozos[0].img, w, h, izq, der, arriba)
			cur = append(cur, f)
		case f.espacio:
			cur = append(cur, f)
			x += f.ancho
		case f.tab:
			// Hasta la siguiente parada, contada desde el borde del bloque: una propia si queda
			// alguna por delante; si no, la de `tab-size`.
			paso := f.trozos[0].est.Tabulador
			f.ancho = paso - float32(math.Mod(float64(x), float64(paso)))
			f.trozos[0].texto = ""
			if p, ok := siguienteParada(f.trozos[0].est.Paradas, x); ok {
				f.ancho = max(0, p.Pos-x-tras(*fs, i, p.Alin))
				f.trozos[0].texto = relleno(p.Relleno, f.ancho, f.trozos[0].est)
			}
			f.trozos[0].ancho = f.ancho
			(*fs)[i] = f
			if x+f.ancho > cabe && hayPalabra {
				return cur, i, izq, der
			}
			cur = append(cur, f)
			x += f.ancho
			hayAlgo = true
		case x+f.ancho > cabe && hayPalabra:
			return cur, i, izq, der
		case f.ancho > cabe-x && !hayPalabra && len(f.trozos) > 0 && !f.objeto:
			partes := partirPalabra(f, cabe-x)
			*fs = slices.Replace(*fs, i, i+1, partes...)
			cur = append(cur, partes[0])
			x += partes[0].ancho
			hayPalabra, hayAlgo = true, true
			if len(partes) > 1 {
				return cur, i + 1, izq, der
			}
		default:
			cur = append(cur, f)
			x += f.ancho
			hayPalabra, hayAlgo = true, true
		}
		i++
	}
	return cur, i, izq, der
}

// siguienteParada es la primera parada propia más allá de x.
func siguienteParada(paradas string, x float32) (rico.Parada, bool) {
	if paradas == "" {
		return rico.Parada{}, false
	}
	for _, p := range rico.LeerParadas(paradas) {
		if p.Pos > x+0.5 {
			return p, true
		}
	}
	return rico.Parada{}, false
}

// tras es lo que se come la alineación de una parada: el ancho de lo que sigue al tabulador i
// hasta el siguiente tabulador o salto (right), la mitad (center) o hasta la coma o el punto
// decimal (decimal). ponytail: el separador es el primer «.» o «,», sin mirar el idioma.
func tras(fs []ficha, i int, alin string) float32 {
	if alin == "left" {
		return 0
	}
	var w float32
	for _, f := range fs[i+1:] {
		if f.tab || f.salto {
			break
		}
		if alin == "decimal" && !f.espacio {
			for _, tr := range f.trozos {
				if k := strings.IndexAny(tr.texto, ".,"); k >= 0 {
					return w + medidor.ancho(tr.texto[:k], tr.est)
				}
				w += tr.ancho
			}
			continue
		}
		w += f.ancho
	}
	if alin == "center" {
		return w / 2
	}
	return w
}

// relleno es el texto que llena el hueco de un tabulador con relleno: puntos, guiones o
// subrayado, tantos como caben.
func relleno(tipo string, ancho float32, e rico.Estilo) string {
	c := map[string]string{"dot": ".", "hyphen": "-", "underscore": "_", "heavy": "_", "middledot": "·"}[tipo]
	if c == "" || ancho <= 0 {
		return ""
	}
	n := int(ancho / medidor.ancho(c, e))
	return strings.Repeat(c, max(n, 0))
}

// maquetarTexto reparte un bloque de texto y le da a cada línea su alto, su línea base y
// su alineación. Va línea a línea porque el ancho de cada una depende de los flotantes que
// tenga al lado, y dónde cae, de lo que miden las de encima. (orgX, orgY) es la esquina
// de las líneas en las coordenadas de `fl`, que puede ser nil (sin flotantes).
func maquetarTexto(b rico.Bloque, ancho float32, fl *flotantes, orgX, orgY float32) (ls []linea, largo int) {
	base := b.Base
	interl := base.Interlineado
	if interl <= 0 {
		interl = 1.5
	}
	fs := fichas(b, ancho)
	if n := len(fs); n > 0 {
		largo = fs[n-1].hasta
	}
	y := float32(0)
	pos := 0
	for i, n := 0, 0; ; n++ {
		var izq, der float32
		y, izq, der = fl.libre(fs, i, orgX, orgY, y, ancho)
		x := float32(0)
		if n == 0 {
			x = base.Sangria1
		}
		var fl0 []ficha
		fl0, i, izq, der = tomar(&fs, i, x, izq, der, fl, orgY+y)
		l, m := medirLinea(fl0, base, interl, x, y, pos)
		pos = l.hasta
		if m.conSalto {
			pos = m.tras
		}
		if m.renglon >= 0 {
			l = renglonImagen(l, &l.piezas[m.renglon], ancho)
		} else {
			alinear(&l, m, base.Alineacion, izq-orgX, der-izq, i >= len(fs))
		}
		ls = append(ls, l)
		y += l.alto
		if i >= len(fs) && !m.conSalto {
			break
		}
	}
	for k := range ls {
		fl.colocarEn(&ls[k], orgX, orgY)
	}
	return fundirPiezas(ls), largo
}

// libre es dónde va la línea que empieza con la ficha i y lo que tiene libre: debajo de
// lo que flota si es una imagen «arriba y abajo» (`clear: both`), y más abajo si al lado
// de lo que flota no cabe nada.
func (fl *flotantes) libre(fs []ficha, i int, orgX, orgY, y, ancho float32) (float32, float32, float32) {
	if i < len(fs) && fs[i].bloque && fs[i].trozos[0].img != nil && fs[i].trozos[0].img.Ajuste == "topBottom" {
		y = max(y, fl.fondo()-orgY)
	}
	izq, der := fl.franja(orgY+y, orgX, orgX+ancho)
	if der-izq < minLinea && der-izq < ancho {
		if abajo := fl.siguienteFondo(orgY + y); abajo > orgY+y {
			y = abajo - orgY
			izq, der = fl.franja(orgY+y, orgX, orgX+ancho)
		}
	}
	return y, izq, der
}

// alinear coloca la línea en lo que tiene libre, [inicio, inicio+cabe), según la
// alineación del bloque. La última de un párrafo, y la que acaba en un salto, no se
// justifican.
func alinear(l *linea, m medidaLinea, al rico.Alineacion, inicio, cabe float32, ultima bool) {
	sobra := cabe - m.finTexto
	l.x0 = inicio
	switch al {
	case rico.Centro:
		l.x0 += sobra / 2
	case rico.Derecha:
		l.x0 += sobra
	case rico.Justificada:
		if !ultima && !m.conSalto && m.espacios > 0 && sobra > 0 {
			l.extra = sobra / float32(espaciosInternos(l.piezas))
		}
	case rico.Izquierda:
	}
}

// medidaLinea es lo que la alineación necesita saber de una línea ya medida.
type medidaLinea struct {
	finTexto float32 // dónde acaba lo último que no es un espacio
	espacios int
	conSalto bool // acaba en un `<br>`
	tras     int  // la runa tras el salto
	renglon  int  // la pieza de una imagen en su renglón, o -1
}

// medirLinea coloca las piezas de una línea desde x y le da su alto y su línea base: el
// puntal de la letra del bloque, lo más alto de lo que lleva y lo que pide una imagen
// apoyada abajo. Una línea con solo un flotante no ocupa.
func medirLinea(fl0 []ficha, base rico.Estilo, interl, x, y float32, pos int) (linea, medidaLinea) {
	l := linea{y: y, desde: pos, hasta: pos}
	m := medidaLinea{finTexto: x, renglon: -1}
	// El puntal: la línea mide al menos lo que pide la letra del bloque, aunque esté
	// vacía o lleve solo un superíndice.
	sube, baja := alturas(base, interl)
	soloFlota := len(fl0) > 0
	var alPie float32 // lo que pide una imagen apoyada abajo (`vertical-align: bottom`)
	for _, f := range fl0 {
		if f.salto {
			m.tras, m.conSalto, soloFlota = f.hasta, true, false
			continue
		}
		for _, tr := range f.trozos {
			tr.x = x
			x += tr.ancho
			l.piezas = append(l.piezas, tr)
			switch {
			case f.bloque:
				m.renglon = len(l.piezas) - 1
			case f.flota: // fuera de la línea: no la empuja
			default:
				sube, baja, alPie = alturaPieza(tr, interl, sube, baja, alPie)
			}
		}
		soloFlota = soloFlota && f.flota
		l.hasta = f.hasta
		if f.espacio {
			m.espacios++
		} else {
			m.finTexto = x
		}
	}
	l.base, l.alto = sube, sube+baja
	if soloFlota {
		l.base, l.alto = 0, 0
	}
	apoyarImagenes(&l, alPie)
	return l, m
}

// minLinea es lo mínimo que tiene que quedar al lado de un flotante para que una línea vaya
// a su lado; si no, baja hasta debajo de él.
// ponytail: el navegador lo decide con lo que mide lo primero que no se puede partir; aquí,
// con un ancho fijo del orden de una palabra corta.
const minLinea = 24

// alturaPieza suma una pieza a la subida y la bajada de su línea: el texto con su
// semi-interlineado; una imagen sin tocar se apoya en la línea base (sube todo su alto) y
// una gestionada va abajo de la línea (`alPie`). Una detrás o delante no cuenta.
func alturaPieza(p pieza, interl, sube, baja, alPie float32) (float32, float32, float32) {
	switch im := p.img; {
	case im == nil:
		s, bj := alturas(p.est, interl)
		return max(sube, s), max(baja, bj), alPie
	case im.Absoluta():
	case im.Gestionada:
		alPie = max(alPie, p.alto)
	default:
		sube = max(sube, p.alto)
	}
	return sube, baja, alPie
}

// apoyarImagenes alarga la línea para la imagen apoyada abajo y coloca cada imagen.
func apoyarImagenes(l *linea, alPie float32) {
	if alPie > l.alto {
		l.base, l.alto = l.base+alPie-l.alto, alPie
	}
	for k := range l.piezas {
		if pz := &l.piezas[k]; pz.img != nil && !pz.img.Absoluta() {
			pz.arriba = l.base - pz.alto
			if pz.img.Gestionada {
				pz.arriba = l.alto - pz.alto
			}
		}
	}
}

// renglonImagen es la línea de una imagen «arriba y abajo»: `display: block` con 14 px
// arriba y abajo, alineada por márgenes con su ancho ENTERO —el recorte de los lados no se
// recupera en este modo— y lo que se ve donde lo deja el recorte.
func renglonImagen(l linea, p *pieza, ancho float32) linea {
	b := p.img.Bandas()
	entero := p.ancho + b[1] + b[3]
	p.x, p.arriba = 0, rico.Hueco
	l.alto, l.base = p.alto+2*rico.Hueco, rico.Hueco+p.alto
	switch p.img.Alinear {
	case "center":
		l.x0 = (ancho - entero) / 2
	case "right":
		l.x0 = ancho - entero
	}
	l.x0 += b[3]
	return l
}

// espaciosInternos cuenta los espacios que reparten el sobrante: los del final de la
// línea cuelgan y no cuentan. Nunca devuelve 0.
func espaciosInternos(ps []pieza) int {
	fin := len(ps)
	for fin > 0 && ps[fin-1].texto == " " {
		fin--
	}
	n := 0
	for _, p := range ps[:fin] {
		n += strings.Count(p.texto, " ")
	}
	return max(n, 1)
}

// fundirPiezas junta las piezas seguidas con el MISMO estilo en una sola —«hola», « » y
// «mundo» son un `canvas.Text`, no tres— y vuelve a medir el conjunto, que es como mide
// el navegador (con el interletraje entre palabras). En una línea justificada no se
// juntan a través de los espacios, que son los que se estiran.
func fundirPiezas(ls []linea) []linea {
	for i := range ls {
		l := &ls[i]
		var out []pieza
		for _, p := range l.piezas {
			n := len(out)
			if n > 0 && out[n-1].est == p.est && out[n-1].hasta == p.desde && out[n-1].img == nil && p.img == nil &&
				!out[n-1].atomo && !p.atomo &&
				(l.extra == 0 || (p.texto != " " && !strings.HasSuffix(out[n-1].texto, " "))) {
				out[n-1].texto += p.texto
				out[n-1].hasta = p.hasta
				continue
			}
			out = append(out, p)
		}
		x := float32(0)
		if len(l.piezas) > 0 {
			x = l.piezas[0].x
		}
		for j := range out {
			out[j].x = x
			if out[j].img == nil && !out[j].atomo {
				out[j].ancho = medidor.ancho(out[j].texto, out[j].est)
			}
			x += out[j].ancho
			if l.extra > 0 && out[j].texto == " " {
				x += l.extra
			}
		}
		l.piezas = out
	}
	return ls
}

// maquetar coloca los bloques uno debajo de otro en `ancho`, con los márgenes colapsados,
// y los de una tabla en su rejilla. Devuelve las cajas y el alto del contenido: lo que
// flota por debajo del último bloque también cuenta.
func maquetar(bloques []rico.Bloque, ancho float32) ([]caja, float32) {
	return apilar(bloques, ancho, true)
}

// margenTabla es el `table { margin: 0 0 0.5em }` de `editor.css`.
const margenTabla = 8

func apilar(bloques []rico.Bloque, ancho float32, tablas bool) ([]caja, float32) {
	return apilarEn(bloques, ancho, tablas, true)
}

// apilarEn es apilar; `columnas` dice si un tramo en columnas se reparte en ellas (dentro
// de una columna, no: ya se está repartiendo).
func apilarEn(bloques []rico.Bloque, ancho float32, tablas, columnas bool) ([]caja, float32) {
	cajas := make([]caja, len(bloques))
	fl := &flotantes{}
	var y, abajo float32
	for i := 0; i < len(bloques); {
		b := bloques[i]
		if columnas && tablas && b.Columnas != nil {
			i, y = tramoColumnas(bloques, cajas, i, ancho, y, abajo)
			abajo = 0
			continue
		}
		arriba := b.Arriba
		if tablas && b.Celda != nil {
			arriba = 0
		}
		antes, previo := y, abajo
		if i == 0 {
			y += arriba
		} else {
			y += max(abajo, arriba)
		}
		if tablas && (b.Celda != nil || b.Recuadro != nil) {
			j, alto := tramoAgrupado(bloques, cajas, i, ancho, y)
			y += alto
			abajo = margenTabla // el del índice (`margin: 0 0 0.5em`) es el mismo
			if b.Base.DeHoja() {
				abajo = 0 // el reset de Tailwind no le da margen
			}
			i = j
			continue
		}
		if tablas && b.Region != "" {
			// Fuera de la corriente: no ocupa sitio ni colapsa márgenes. Su sitio lo pone
			// el folio (colocarRegiones), que es quien sabe lo que mide el papel.
			cajas[i] = maquetarRegion(b, ancho)
			y = antes
			i++
			continue
		}
		c := maquetarBloque(b, ancho, fl, y)
		cajas[i] = c
		y += c.alto
		abajo = b.Abajo
		if c.alto == 0 && len(c.lineas) > 0 && i > 0 {
			// Un párrafo con solo un flotante no mide nada, y sus márgenes COLAPSAN a través
			// de él con los de los vecinos (CSS 2 §8.3.1).
			y, abajo = antes, max(previo, arriba, b.Abajo)
		}
		i++
	}
	return cajas, max(y+abajo, fl.fondo())
}

// tramoAgrupado maqueta la tabla o el índice que empieza en el bloque i: dice dónde sigue y
// lo que mide.
func tramoAgrupado(bloques []rico.Bloque, cajas []caja, i int, ancho, y float32) (j int, alto float32) {
	b := bloques[i]
	j = i + 1
	if b.Celda != nil {
		for j < len(bloques) && bloques[j].Celda != nil && bloques[j].Celda.Tabla == b.Celda.Tabla {
			j++
		}
		return j, maquetarTabla(bloques[i:j], cajas[i:j], b.Celda.Tabla, ancho, y).alto
	}
	for j < len(bloques) && bloques[j].Recuadro == b.Recuadro {
		j++
	}
	return j, maquetarRecuadro(bloques[i:j], cajas[i:j], ancho, y).alto
}

// tramoColumnas maqueta el tramo en columnas que empieza en i y devuelve dónde sigue y la
// altura de debajo. El envoltorio es un contexto de formato propio: sus márgenes no
// colapsan con los de fuera, y el de abajo del último bloque va dentro de su alto.
func tramoColumnas(bloques []rico.Bloque, cajas []caja, i int, ancho, y, abajo float32) (int, float32) {
	j := i + 1
	for j < len(bloques) && bloques[j].Columnas == bloques[i].Columnas {
		j++
	}
	if i > 0 {
		y += abajo
	}
	return j, y + maquetarColumnas(bloques[i:j], cajas[i:j], bloques[i].Columnas.N, ancho, y)
}

// huecoColumnas es el `column-gap: 2em` del envoltorio, a la letra del folio (16).
const huecoColumnas = 32

// maquetarColumnas reparte el tramo en n columnas de CSS: todas del mismo ancho con su
// hueco, y la altura equilibrada (`column-fill: balance`). Devuelve lo que mide el tramo.
// ponytail: solo se corta ENTRE bloques; el navegador corta también dentro de un párrafo,
// línea a línea, y con párrafos largos sus columnas salen más parejas.
func maquetarColumnas(bls []rico.Bloque, out []caja, n int, ancho, y0 float32) float32 {
	w := (ancho - huecoColumnas*float32(n-1)) / float32(n)
	cajas, total := apilarEn(bls, max(w, 1), true, false)
	objetivo := total / float32(n)
	col, desde := 0, float32(0)
	var alto float32
	for k := range cajas {
		c := &cajas[k]
		// Un bloque que se sale de la columna pasa a la siguiente, si aún quedan.
		if col < n-1 && c.y+c.alto-desde > objetivo+0.5 && c.y > desde {
			alto = max(alto, c.y-desde)
			col, desde = col+1, c.y
		}
		dx := float32(col) * (w + huecoColumnas)
		c.y += y0 - desde
		c.x += dx
		c.izq += dx
		c.colIzq, c.colDer = dx-huecoColumnas/2, dx+w+huecoColumnas/2
		if col == 0 {
			c.colIzq = -1e9
		}
		if col == n-1 {
			c.colDer = 1e9
		}
		out[k] = *c
		alto = max(alto, c.y+c.alto-y0)
	}
	return max(alto, total-desde)
}

// Las medidas de `HEADER_STYLE` y `FOOTER_STYLE`: la caja de contenido mide al menos 20,
// lleva 6 de relleno hacia el cuerpo y la raya de 1, y se asoma 30 al margen desde el
// borde del contenido (`calc(var(--doc-page-margin) - 30px)`).
const (
	minRegion     = 20
	rellenoRegion = 6
	asomaRegion   = 30
)

// maquetarRegion maqueta el encabezado o el pie: su línea, en su caja de `min-height`,
// abajo en el encabezado (`align-items: flex-end`) y arriba en el pie (`flex-start`). El
// encabezado va encima del contenido; el pie lo coloca el folio, al pie del papel.
func maquetarRegion(b rico.Bloque, ancho float32) caja {
	c := maquetarBloque(b, ancho, nil, 0)
	texto := c.alto
	alto := max(minRegion, texto)
	c.alto = alto + rellenoRegion + 1
	if b.Region == rico.Encabezado {
		c.y, c.arribaTexto = -asomaRegion, alto-texto
	} else {
		c.arribaTexto = 1 + rellenoRegion
	}
	return c
}

// maquetarBloque maqueta UN bloque a su ancho, arriba en y: lista, cita, regla, tabla o
// texto. Sus líneas se apartan de lo que flota en `fl`, que puede ser nil.
func maquetarBloque(b rico.Bloque, ancho float32, fl *flotantes, y float32) caja {
	c := caja{b: b, y: y}
	c.izq = float32(b.Sangria)*sangriaLista + b.Base.MargenIzq
	if b.Tipo == rico.Cita {
		c.izq += barraCita + aireCita
		c.arribaTexto = vertCita
	}
	c.ancho = max(ancho-c.izq-b.Base.MargenDer, 1)
	switch b.Tipo {
	case rico.Regla:
		c.alto, c.reglaY = 1, 0
		return c
	case rico.SaltoPagina:
		c.alto = 2 // `border-top: 2px dashed`
		return c
	case rico.Figura:
		_, c.alto = medidaFigura(b.Ilustracion, c.ancho)
		return c
	case rico.Parrafo, rico.Titulo, rico.Cita:
	}
	c.lineas, c.largo = maquetarTexto(b, c.ancho, fl, c.izq, y+c.arribaTexto)
	for _, l := range c.lineas {
		c.alto = max(c.alto, l.y+l.alto)
	}
	c.alto += 2 * c.arribaTexto
	if b.Marca != "" {
		// La marca va FUERA, pegada al texto (`list-style-position: outside`), con el
		// estilo del principio de la línea. Una viñeta no es un carácter: el navegador
		// la DIBUJA (ver pintarVineta); un número sí es texto, «3. ».
		c.marcaEstilo = b.Base
		if len(b.Tramos) > 0 {
			c.marcaEstilo = b.Tramos[0].Estilo
		}
		c.marcaEstilo.Vertical = 0
		if b.Marca != "•" {
			c.marcaTexto = b.Marca + " "
			c.marcaAncho = medidor.ancho(c.marcaTexto, c.marcaEstilo)
		}
	}
	return c
}

// maquetarTabla reparte el ancho a partes iguales (`table-layout: fixed; width: 100%`),
// maqueta cada celda como un documento pequeño y deja sus cajas en `out`, ya en su sitio.
// Las filas miden lo que su celda más alta; una celda que abarca varias filas, si no cabe
// en ellas, alarga la última.
// ponytail: el navegador reparte ese sobrante entre las filas que abarca; aquí va entero a
// la última. Solo se nota con un rowspan que es la celda más alta de todas.
func maquetarTabla(bls []rico.Bloque, out []caja, t *rico.Tabla, ancho, y0 float32) *tablaMaq {
	tm := &tablaMaq{y: y0, ancho: ancho}
	if t.Cols == 0 {
		return tm
	}
	// Con la hoja de un contenedor (una caja de Diapos) la tabla va con el reset de
	// Tailwind: sin bordes ni relleno.
	// ponytail: y con `table-layout: auto`, que reparte las columnas por su contenido;
	// aquí siguen a partes iguales. Se releva midiendo el contenido de cada columna.
	px, py, borde := float32(celdaX), float32(celdaY), float32(1)
	if len(bls) > 0 && bls[0].Base.DeHoja() {
		px, py, borde, tm.sinBordes = 0, 0, 0, true
	}
	col := ancho / float32(t.Cols)
	type hecha struct {
		cajas []caja
		desde int
		alto  float32
	}
	hechas := make([]hecha, len(t.Celdas))
	for k, cel := range t.Celdas {
		desde := 0
		for desde < len(bls) && bls[desde].Celda != cel {
			desde++
		}
		hasta := desde
		for hasta < len(bls) && bls[hasta].Celda == cel {
			hasta++
		}
		cajas, alto := apilar(bls[desde:hasta], max(col*float32(cel.Cols)-2*px-borde, 1), false)
		hechas[k] = hecha{cajas, desde, alto + 2*py + borde}
	}
	filas := make([]float32, t.Filas)
	for k, cel := range t.Celdas {
		if cel.Filas == 1 {
			filas[cel.Fila] = max(filas[cel.Fila], hechas[k].alto)
		}
	}
	for k, cel := range t.Celdas {
		if cel.Filas > 1 {
			suma := float32(0)
			for _, f := range filas[cel.Fila : cel.Fila+cel.Filas] {
				suma += f
			}
			filas[cel.Fila+cel.Filas-1] += max(hechas[k].alto-suma, 0)
		}
	}
	ys := make([]float32, t.Filas+1)
	for f, alto := range filas {
		ys[f+1] = ys[f] + alto
	}
	tm.alto = ys[t.Filas] + borde
	for k, cel := range t.Celdas {
		cm := celdaMaq{
			x: col * float32(cel.Col), y: y0 + ys[cel.Fila],
			w: col * float32(cel.Cols), h: ys[cel.Fila+cel.Filas] - ys[cel.Fila], c: cel,
		}
		tm.celdas = append(tm.celdas, cm)
		dy := cm.y + borde + py
		switch sobra := cm.h - hechas[k].alto; cel.Vertical {
		case "middle":
			dy += sobra / 2
		case "bottom":
			dy += sobra
		}
		dx := cm.x + borde + px
		for i, c := range hechas[k].cajas {
			c.y += dy
			c.x += dx
			c.izq += dx
			c.tabla = tm
			out[hechas[k].desde+i] = c
		}
	}
	return tm
}

// maquetarRecuadro coloca los bloques del índice dentro de su borde y su relleno, que
// no deja colapsar los márgenes de dentro con los de fuera.
func maquetarRecuadro(bls []rico.Bloque, out []caja, ancho, y0 float32) *recuadroMaq {
	r := &recuadroMaq{y: y0, ancho: ancho}
	cajas, alto := apilar(bls, max(ancho-2-2*rellenoRecuadroX, 1), false)
	dx, dy := float32(1+rellenoRecuadroX), y0+1+rellenoRecuadroY
	for i, c := range cajas {
		c.y += dy
		c.x += dx
		c.izq += dx
		c.recuadro = r
		out[i] = c
	}
	r.alto = alto + 2*rellenoRecuadroY + 2
	return r
}

// celdaEn es la celda de la tabla bajo el punto (x, y): la de esa fila que lo cubre en
// horizontal, o la más cercana. Un punto por encima o por debajo cae en la fila del borde.
func (tm *tablaMaq) celdaEn(x, y float32) *rico.Celda {
	y = min(max(y, tm.y), tm.y+tm.alto-1.5)
	var mejor *rico.Celda
	dist := float32(-1)
	for _, cm := range tm.celdas {
		if y < cm.y || y >= cm.y+cm.h {
			continue
		}
		d := max(cm.x-x, x-(cm.x+cm.w), 0)
		if dist < 0 || d < dist {
			mejor, dist = cm.c, d
		}
	}
	return mejor
}

// esEspacio dice si la runa separa palabras para moverse con el teclado.
func esEspacio(r rune) bool { return unicode.IsSpace(r) || unicode.IsPunct(r) }
