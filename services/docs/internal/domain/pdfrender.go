// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Render de una página de PDF a SVG. Es la ola P1 de Sytue PDF —el visor— y la
// pieza sobre la que se apoyan todas las demás: anotar, rellenar y firmar
// necesitan un lienzo donde el usuario vea dónde está pinchando.
//
// POR QUÉ SVG Y NO UNA IMAGEN. Rasterizar un PDF es escribir un motor de
// gráficos entero: curvas, tramas, antialiasing, composición y, sobre todo,
// interpretar cada formato de fuente incrustada para dibujar sus contornos. Eso
// ya lo hace, y muy bien, el motor del navegador. Traducir los operadores del
// flujo de contenido a SVG es casi una correspondencia uno a uno —`m/l/c/re` son
// un `<path>`, `cm` es un `transform`, `Do` sobre una imagen es un `<image>`— y
// deja tres cosas gratis que una imagen no da:
//
//   - el texto es TEXTO: se selecciona, se copia y lo lee un lector de pantalla;
//   - el zoom no pierde calidad y no obliga a volver a pedir la página;
//   - una sola implementación vale para las seis plataformas, porque el
//     escritorio (Wails) y el móvil (Capacitor) son webviews.
//
// Y encaja con el invariante de la casa: cero dependencias nuevas, nada de
// PDF.js (AGPL) ni binarios externos.
//
// LO QUE ESTE RENDER NO HACE, dicho aquí para que no se descubra tarde: los
// espacios de color Lab e ICC exactos (se usa el alternativo por número de
// componentes) y los modos de fusión que SVG no tiene. Cada uno degrada a algo
// visible y razonable, nunca a una página en blanco sin aviso.
//
// Y lo que SÍ hace aunque parezca que no: las fuentes de tipo 3, cuyos glifos son
// flujos de contenido que se ejecutan con este mismo intérprete (`pdftype3.go`);
// los patrones de mosaico, que se emiten como `<pattern>` renderizando la celda; y
// los sombreados de MALLA (tipos 4-7), que SVG no sabe expresar y se traducen a
// triángulos subdivididos (`pdfmesh.go`).
package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"strconv"
	"strings"
)

// maxSVGBytes acota el SVG resultante. Un PDF puede describir un millón de
// caminos en pocos kilobytes comprimidos; sin tope, una sola página maliciosa
// llena la memoria del servidor y la del navegador que la reciba.
const maxSVGBytes = 24 << 20

// maxProfundidadXObject corta la recursión de formularios que se incluyen entre
// sí. La especificación lo prohíbe, un fichero corrupto no.
const maxProfundidadXObject = 12

// OpcionesRender ajusta la salida del render.
type OpcionesRender struct {
	// Escala multiplica el tamaño declarado del `<svg>`; el `viewBox` no cambia,
	// así que el documento se sigue midiendo en puntos y solo cambia a cuántos
	// píxeles se pinta. 1 = 72 ppp.
	Escala float64
	// SinTexto omite el texto. Lo usa la comparación de documentos, que pinta el
	// texto por su cuenta con los colores de la diferencia.
	SinTexto bool
	// SinAnotaciones omite las apariencias de /Annots. El editor las pinta él
	// mismo en una capa aparte para poder moverlas.
	SinAnotaciones bool
}

func (o OpcionesRender) escala() float64 {
	if o.Escala <= 0 {
		return 1
	}
	if o.Escala > 16 {
		return 16 // más allá no se distingue y el `width` se vuelve absurdo
	}
	return o.Escala
}

// --- Matrices ----------------------------------------------------------------

// matriz es la matriz de transformación de PDF [a b c d e f], que representa
//
//	| a b 0 |
//	| c d 0 |
//	| e f 1 |
//
// y coincide EXACTAMENTE con `matrix(a b c d e f)` de SVG: los dos mapean
// (x,y) → (a·x + c·y + e, b·x + d·y + f). Esa coincidencia es la que hace que
// este render sea una traducción y no una conversión.
type matriz [6]float64

var identidad = matriz{1, 0, 0, 1, 0, 0}

// por compone: primero m, después n. Es el orden de PDF, donde `cm` premultiplica.
func (m matriz) por(n matriz) matriz {
	return matriz{
		m[0]*n[0] + m[1]*n[2],
		m[0]*n[1] + m[1]*n[3],
		m[2]*n[0] + m[3]*n[2],
		m[2]*n[1] + m[3]*n[3],
		m[4]*n[0] + m[5]*n[2] + n[4],
		m[4]*n[1] + m[5]*n[3] + n[5],
	}
}

func (m matriz) svg() string {
	return "matrix(" + num(m[0]) + " " + num(m[1]) + " " + num(m[2]) + " " +
		num(m[3]) + " " + num(m[4]) + " " + num(m[5]) + ")"
}

// esIdentidad evita ensuciar la salida con transformaciones que no hacen nada,
// que en un PDF corriente son la mayoría.
func (m matriz) esIdentidad() bool { return m == identidad }

// num formatea un número para SVG: sin notación exponencial, sin ceros de más y
// con tres decimales, que a 72 ppp es una milésima de punto —muy por debajo de
// lo que cualquier pantalla distingue— y recorta el SVG a la mitad.
func num(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "0"
	}
	s := strconv.FormatFloat(v, 'f', 3, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	// «-0» es válido en SVG pero ensucia la salida y hace que dos matrices
	// idénticas se escriban distinto, que es un incordio al comparar y al cachear.
	if s == "" || s == "-" || s == "-0" {
		return "0"
	}
	return s
}

// --- Estado gráfico ----------------------------------------------------------

// estadoG es el estado gráfico de PDF (§8.4) recortado a lo que SVG sabe
// expresar. Lo que no cabe (retícula de semitonos, corrección gamma, perfiles
// ICC) no se guarda: no se puede pintar y guardarlo solo confunde.
type estadoG struct {
	ctm matriz

	// recorte solo lo usa la salida PNG (`pdfraster.go`). Vive en el estado
	// gráfico —y no en el lienzo— porque el recorte de PDF se deshace con `Q`:
	// copiarlo con la pila es lo que hace que el anidamiento salga gratis.
	recorte *recorteRaster

	csRelleno, csTrazo *espacioColor
	relleno, trazo     string // ya en forma CSS: «#rrggbb» o «none»
	patronRelleno      string // id de <linearGradient>/<pattern> si el color es un patrón
	patronTrazo        string
	// aproxRelleno/aproxTrazo son el color liso con el que la salida PNG sustituye
	// a un patrón que no sabe pintar. Vacío = no se pinta (ver `colorDePintura`).
	aproxRelleno string
	aproxTrazo   string
	// degRelleno es el degradado de verdad para la salida PNG cuando el patrón de
	// relleno es un sombreado axial, radial o de función; mallaRelleno son los
	// triángulos cuando el patrón es una malla (tipos 4 a 7).
	degRelleno   *degradadoRaster
	mallaRelleno []trianguloMalla
	matrizMalla  matriz
	alfaRelleno  float64
	alfaTrazo    float64

	grosor  float64
	tapa    int
	union   int
	limite  float64
	guiones string
	mezcla  string

	fuente       *fuentePDF
	tamano       float64
	espCar       float64
	espPal       float64
	escalaH      float64
	interlineado float64
	subida       float64
	modoRender   int
}

func estadoInicial(ctm matriz) estadoG {
	return estadoG{
		ctm:         ctm,
		relleno:     "#000000",
		trazo:       "#000000",
		alfaRelleno: 1,
		alfaTrazo:   1,
		grosor:      1,
		limite:      10,
		escalaH:     1,
		tamano:      0,
	}
}

// --- Lienzo ------------------------------------------------------------------

// lienzo acumula el SVG mientras se recorre el flujo de contenido.
type lienzo struct {
	d    *PDFDoc
	opts OpcionesRender
	sb   strings.Builder
	defs strings.Builder

	seq         int
	prefijoIDs  string // documento+página: los recursos SVG inline tienen ámbito global
	fuentes     map[string]*fuentePDF
	caraCSS     map[string]bool // @font-face ya emitido, por id
	ocgApagados map[int]bool    // objetos OCG en /OCProperties /D /OFF
	prof        int
	truncado    bool

	// r, si no es nil, es el mapa de bits de la salida `formato=png`. El SVG y el
	// PNG salen del MISMO recorrido del flujo de contenido: aquí no hay un segundo
	// intérprete, solo un destino más (ver `pdfraster.go`).
	r *rasterizador
	// soloRaster ahorra el SVG cuando nadie lo va a leer: la cadena, la base64 de
	// las tipografías y la de las imágenes no se usan en PNG, y construirlas
	// sería trabajo tirado.
	soloRaster bool

	// ultimaImagen es el mapa de bits que acaba de construir el camino de
	// muestras, para que el rasterizador no tenga que codificar y volver a
	// decodificar el PNG que el SVG necesita y él no.
	ultimaImagen image.Image

	// recolectando cambia lo que hace `Do` sobre una imagen: en vez de dibujarla,
	// la apunta con su colocación. Lo usa el OCR, que necesita saber DÓNDE está
	// la imagen escaneada para traducir lo que reconozca a coordenadas del PDF.
	recolectando bool
	imagenes     []ImagenColocada
	// texto, si no es nil, recoge el texto que se muestra en vez de pintarlo. Es
	// como extrae el texto `TextoDePagina`: con este intérprete y no con otro.
	texto *recolectorTexto
	// rects son los rectángulos y las líneas horizontales del contenido, en
	// coordenadas de la página. De ahí salen los campos que se proponen sobre un
	// impreso: en papel, «donde se escribe» es una caja o una raya.
	rects []rectPagina
}

// rectPagina es una caja del contenido ya transformada por la CTM.
type rectPagina struct{ x0, y0, x1, y1 float64 }

func (l *lienzo) id(prefijo string) string {
	l.seq++
	return l.prefijoIDs + prefijo + strconv.Itoa(l.seq)
}

func (d *PDFDoc) prefijoRecursosSVG(pagina int) string {
	d.huellaSVGOnce.Do(func() {
		huella := sha256.Sum256(d.buf)
		d.huellaSVG = hex.EncodeToString(huella[:])
	})
	return "sypdf-" + d.huellaSVG + "-p" + strconv.Itoa(pagina+1) + "-"
}

// esc escapa lo que va a texto o atributo XML.
var esc = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")

func escaparXML(s string) string { return esc.Replace(s) }

// escribir corta cuando el SVG se pasa del tope. No devuelve error: una página
// gigante se sirve recortada con un aviso, que es mejor que no servir nada, y
// `truncado` lo cuenta hacia arriba para que la UI lo diga.
func (l *lienzo) escribir(s string) {
	if l.truncado || l.soloRaster {
		return
	}
	if l.sb.Len()+l.defs.Len()+len(s) > maxSVGBytes {
		l.truncado = true
		return
	}
	l.sb.WriteString(s)
}

// defsEscribir es lo mismo para el bloque `<defs>`, donde acaban las tipografías
// en base64 y los recortes. En PNG no se lee ninguno, y una página de texto trae
// cientos de `<clipPath>` y varios megas de tipografía.
func (l *lienzo) defsEscribir(s string) {
	if l.soloRaster {
		return
	}
	l.defs.WriteString(s)
}

// --- Punto de entrada --------------------------------------------------------

// RenderizarPaginaSVG dibuja la página i (0-based) como un documento SVG
// completo, listo para incrustar en un `<img>`, un `srcdoc` o el DOM.
func (d *PDFDoc) RenderizarPaginaSVG(i int, opts OpcionesRender) (string, error) {
	l, p, err := d.recorrerPagina(i, opts, nil)
	if err != nil {
		return "", err
	}

	escala := opts.escala()
	var out strings.Builder
	fmt.Fprintf(&out,
		`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" `+
			`width="%s" height="%s" viewBox="0 0 %s %s" `+
			// El fondo blanco es del PAPEL, no del visor: un PDF no declara fondo
			// y sin esto una página se vería transparente sobre el tema oscuro.
			`style="background:#fff">`+"\n",
		num(p.Ancho*escala), num(p.Alto*escala), num(p.Ancho), num(p.Alto))
	if l.defs.Len() > 0 {
		out.WriteString("<defs>")
		out.WriteString(l.defs.String())
		out.WriteString("</defs>\n")
	}
	out.WriteString(l.sb.String())
	out.WriteString("</svg>")
	return out.String(), nil
}

// recorrerPagina ejecuta el contenido de la página y sus anotaciones. Es el
// recorrido ÚNICO: el SVG y el PNG salen de aquí, y por eso no hay dos
// intérpretes que puedan divergir. `r` distinto de nil pinta también el mapa de
// bits.
func (d *PDFDoc) recorrerPagina(i int, opts OpcionesRender, r *rasterizador) (*lienzo, PDFPagina, error) {
	p, err := d.Pagina(i)
	if err != nil {
		return nil, PDFPagina{}, err
	}
	contenido, err := d.contenidoDePagina(p)
	if err != nil {
		return nil, PDFPagina{}, err
	}

	l := &lienzo{
		d: d, opts: opts, fuentes: map[string]*fuentePDF{}, caraCSS: map[string]bool{},
		ocgApagados: d.ocgApagados(), r: r, soloRaster: r != nil,
	}
	if !l.soloRaster {
		// clipPath, patrones y gradientes se resuelven por ID en TODO el DOM,
		// también si cada página llega como un SVG independiente. El índice
		// local rec1/grad1 no puede referenciar un recurso de otra página.
		l.prefijoIDs = d.prefijoRecursosSVG(i)
	}
	base := matrizBase(p)
	recursos, _ := d.resolver(p.dict["Resources"]).(pdfDict)

	l.ejecutar(contenido, recursos, estadoInicial(base))
	if !opts.SinAnotaciones {
		l.anotacionesDePagina(p, base)
	}
	return l, p, nil
}

// RenderizarPaginaPNG dibuja la página i (0-based) como PNG.
//
// Es la SEGUNDA salida del mismo motor, para el cliente que no tiene detrás un
// motor de SVG con texto —hoy el escritorio Fyne, cuyo `oksvg` no implementa
// `<text>`—. La primera, el SVG, no cambia ni una coma: ver la cabecera de
// `pdfraster.go` y el ADR-0033.
func (d *PDFDoc) RenderizarPaginaPNG(i int, opts OpcionesRender) ([]byte, error) {
	img, err := d.RenderizarPaginaRGBA(i, opts)
	if err != nil {
		return nil, err
	}
	// Compresión RÁPIDA y no la de `bytesPNG`: con la de siempre, codificar era
	// entre el 40 y el 85 % de la CPU de una página. Medido en el frontal (ARM64,
	// A4 a ×2), `BestSpeed` la baja de ~100 a ~70 ms en un contrato y de ~140 a ~95 en
	// una portada, a cambio de hasta un 3 % más de bytes. Las imágenes que van
	// DENTRO del SVG siguen con la de siempre.
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, img); err != nil {
		return nil, errors.New("no se pudo codificar el PNG de la página")
	}
	return buf.Bytes(), nil
}

// RenderizarPaginaRGBA dibuja la página i (0-based) y entrega el mapa de bits SIN
// codificar. Es lo que necesita quien va a leer los píxeles —la comparación visual—:
// pasar por un PNG para volver a decodificarlo sería codificar y decodificar a cambio
// de nada. Mismo motor, mismos topes (`maxPixelesRaster`), y la imagen es de quien llama.
func (d *PDFDoc) RenderizarPaginaRGBA(i int, opts OpcionesRender) (*image.RGBA, error) {
	p, err := d.Pagina(i)
	if err != nil {
		return nil, err
	}
	r, err := nuevoRasterizador(p.Ancho, p.Alto, opts.escala())
	if err != nil {
		return nil, err
	}
	if _, _, err := d.recorrerPagina(i, opts, r); err != nil {
		return nil, err
	}
	return r.dst, nil
}

// matrizBase lleva del espacio de usuario del PDF (origen abajo-izquierda, Y
// hacia arriba, desplazado por la esquina del MediaBox) al del SVG (origen
// arriba-izquierda, Y hacia abajo), aplicando de paso /Rotate.
//
// Sin la rotación aquí, una página escaneada en horizontal se vería tumbada: el
// visor de un PDF respeta /Rotate y el usuario no sabe que existe.
func matrizBase(p PDFPagina) matriz {
	x0, y0, x1, y1 := p.Caja[0], p.Caja[1], p.Caja[2], p.Caja[3]
	switch p.Rotacion {
	case 90:
		return matriz{0, 1, 1, 0, -y0, -x0}
	case 180:
		return matriz{-1, 0, 0, 1, x1, -y0}
	case 270:
		return matriz{0, -1, -1, 0, y1, x1}
	default:
		return matriz{1, 0, 0, -1, -x0, y1}
	}
}

// --- Intérprete --------------------------------------------------------------

// ejecutar recorre un flujo de contenido. La notación de PDF es postfija: se
// apilan operandos hasta dar con la palabra del operador, que los consume.
func (l *lienzo) ejecutar(contenido []byte, recursos pdfDict, inicial estadoG) {
	if l.prof > maxProfundidadXObject || l.truncado {
		return
	}
	e := &ejecucion{
		l:         l,
		recursos:  recursos,
		estado:    inicial,
		abiertosN: []int{0},
	}
	e.pila = append(e.pila, inicial)

	lx := &pdfLexer{buf: contenido}
	var operandos []pdfObj
	for !l.truncado {
		lx.saltarEspacios()
		if lx.pos >= len(lx.buf) {
			break
		}
		c := lx.buf[lx.pos]
		if c == '/' || c == '(' || c == '<' || c == '[' || c == '+' || c == '-' || c == '.' ||
			(c >= '0' && c <= '9') {
			antes := lx.pos
			o, err := l.d.parsearObjeto(lx)
			if err != nil || lx.pos == antes {
				lx.pos = antes + 1
				continue
			}
			operandos = append(operandos, o)
			if len(operandos) > 512 {
				operandos = operandos[len(operandos)-512:]
			}
			continue
		}
		op := lx.operador()
		if op == "" {
			lx.pos++
			continue
		}
		if op == "BI" {
			// Imagen en línea: sus datos crudos no son tokens y hay que saltarlos
			// con el lexer en la mano, no dejando que los parsee.
			e.imagenEnLinea(lx)
			operandos = operandos[:0]
			continue
		}
		e.aplicar(op, operandos)
		operandos = operandos[:0]
	}
	e.cerrarTodo()
}

// Una propiedad ActualText puede apuntar al mismo objeto desde muchos grupos.
// Acotarla ANTES de decodificar evita multiplicar una cadena enorme por la pila.
const maxTextoMarcadoBytes = 64 << 10

type marcaTexto struct {
	actual                    string
	presente, emitida, oculta bool
}

// ejecucion es el intérprete de UN flujo de contenido: su pila de estados, el
// camino en construcción y los grupos SVG que ha dejado abiertos.
type ejecucion struct {
	l              *lienzo
	recursos       pdfDict
	estado         estadoG
	pila           []estadoG
	marcas         []marcaTexto
	marcasOmitidas int
	ocgOculto      int

	// abiertosN[i] cuenta los <g> abiertos en el nivel i de la pila `q`. El
	// recorte de PDF es parte del estado gráfico y se deshace con `Q`, mientras
	// que en SVG es un elemento que envuelve: llevar la cuenta es la traducción.
	abiertosN []int

	camino strings.Builder
	// cam es el MISMO camino que `camino`, en forma de operadores y no de cadena.
	// Solo se llena cuando hay rasterizador: volver a parsear el `d` que acabamos
	// de escribir sería trabajo tirado (y un segundo sitio donde equivocarse).
	cam       []segCam
	hayCamino bool
	curX      float64
	curY      float64
	iniX      float64
	iniY      float64
	recortePe int // 0 ninguno, 1 nonzero (W), 2 evenodd (W*)

	// Estado de texto entre BT y ET.
	tm, tlm  matriz
	enTexto  bool
	textoAbi bool
}

// tabla de la especificación (§8-9). Partirlo escondería esa correspondencia
// uno a uno, que es lo que hace este fichero legible frente al PDF abierto al lado.
//
//nolint:gocyclo // El switch de operadores del flujo de contenido es la
func (e *ejecucion) aplicar(op string, ops []pdfObj) {
	n := func(i int) float64 {
		// Los operandos se leen desde el FINAL: un flujo mal formado puede dejar
		// operandos huérfanos delante, y el operador siempre consume los últimos.
		idx := len(ops) - i
		if idx < 0 || idx >= len(ops) {
			return 0
		}
		return comoReal(e.l.d.resolver(ops[idx]))
	}
	nombre := func(i int) pdfName {
		idx := len(ops) - i
		if idx < 0 || idx >= len(ops) {
			return ""
		}
		v, _ := e.l.d.resolver(ops[idx]).(pdfName)
		return v
	}

	switch op {
	// --- Estado gráfico ---
	case "q":
		e.pila = append(e.pila, e.estado)
		e.abiertosN = append(e.abiertosN, 0)
	case "Q":
		// `Q` restaura el estado que guardó SU `q`, que es el que está en la cima.
		// Se restauraba el de DEBAJO —el del `q` anterior—, y eso daba igual
		// mientras el `q` abría con el estado inicial, que es lo corriente. Con
		// una matriz puesta antes del `q` (Google Slides abre con `1 0 0 -1 0 405
		// cm`), todo lo que venía tras el primer `Q` se pintaba fuera de la hoja.
		if len(e.pila) > 1 {
			e.cerrarNivel()
			e.estado = e.pila[len(e.pila)-1]
			e.pila = e.pila[:len(e.pila)-1]
		}
	case "cm":
		if len(ops) >= 6 {
			m := matriz{n(6), n(5), n(4), n(3), n(2), n(1)}
			e.estado.ctm = m.por(e.estado.ctm)
		}
	case "w":
		e.estado.grosor = n(1)
	case "J":
		e.estado.tapa = int(n(1))
	case "j":
		e.estado.union = int(n(1))
	case "M":
		e.estado.limite = n(1)
	case "d":
		e.estado.guiones = e.guionesCSS(ops)
	case "gs":
		e.aplicarExtGState(nombre(1))
	case "i", "ri":
		// Aplanado de curvas e intención de color: el navegador decide mejor.

	// --- Construcción de caminos ---
	case "m":
		e.mover(n(2), n(1))
	case "l":
		e.linea(n(2), n(1))
	case "c":
		e.curva(n(6), n(5), n(4), n(3), n(2), n(1))
	case "v":
		// Primer punto de control = punto actual.
		e.curva(e.curX, e.curY, n(4), n(3), n(2), n(1))
	case "y":
		// Segundo punto de control = punto final.
		e.curva(n(4), n(3), n(2), n(1), n(2), n(1))
	case "h":
		e.cerrarSubcamino()
	case "re":
		e.rectangulo(n(4), n(3), n(2), n(1))

	// --- Pintado de caminos ---
	case "S":
		e.pintar(false, true, false)
	case "s":
		e.cerrarSubcamino()
		e.pintar(false, true, false)
	case "f", "F":
		e.pintar(true, false, false)
	case "f*":
		e.pintar(true, false, true)
	case "B":
		e.pintar(true, true, false)
	case "B*":
		e.pintar(true, true, true)
	case "b":
		e.cerrarSubcamino()
		e.pintar(true, true, false)
	case "b*":
		e.cerrarSubcamino()
		e.pintar(true, true, true)
	case "n":
		e.pintar(false, false, false)
	case "W":
		e.recortePe = 1
	case "W*":
		e.recortePe = 2

	// --- Color ---
	case "CS":
		e.estado.csTrazo = e.l.espacioPorNombre(nombre(1), e.recursos)
		e.estado.trazo, e.estado.patronTrazo = e.estado.csTrazo.inicial(), ""
	case "cs":
		e.estado.csRelleno = e.l.espacioPorNombre(nombre(1), e.recursos)
		e.estado.relleno, e.estado.patronRelleno = e.estado.csRelleno.inicial(), ""
		e.estado.degRelleno, e.estado.mallaRelleno = nil, nil
	case "SC", "SCN":
		e.color(ops, true)
	case "sc", "scn":
		e.color(ops, false)
	case "G":
		e.estado.csTrazo, e.estado.patronTrazo = espacioGris, ""
		e.estado.trazo = cssGris(n(1))
	case "g":
		e.estado.csRelleno, e.estado.patronRelleno = espacioGris, ""
		e.estado.degRelleno, e.estado.mallaRelleno = nil, nil
		e.estado.relleno = cssGris(n(1))
	case "RG":
		e.estado.csTrazo, e.estado.patronTrazo = espacioRGB, ""
		e.estado.trazo = cssRGB(n(3), n(2), n(1))
	case "rg":
		e.estado.csRelleno, e.estado.patronRelleno = espacioRGB, ""
		e.estado.degRelleno, e.estado.mallaRelleno = nil, nil
		e.estado.relleno = cssRGB(n(3), n(2), n(1))
	case "K":
		e.estado.csTrazo, e.estado.patronTrazo = espacioCMYK, ""
		e.estado.trazo = cssCMYK(n(4), n(3), n(2), n(1))
	case "k":
		e.estado.csRelleno, e.estado.patronRelleno = espacioCMYK, ""
		e.estado.degRelleno, e.estado.mallaRelleno = nil, nil
		e.estado.relleno = cssCMYK(n(4), n(3), n(2), n(1))

	// --- Sombreado ---
	case "sh":
		e.sombrear(nombre(1))

	// --- Texto ---
	case "BT":
		e.enTexto = true
		e.tm, e.tlm = identidad, identidad
	case "ET":
		e.cerrarTexto()
		e.enTexto = false
	case "Tc":
		e.estado.espCar = n(1)
	case "Tw":
		e.estado.espPal = n(1)
	case "Tz":
		e.estado.escalaH = n(1) / 100
	case "TL":
		e.estado.interlineado = n(1)
	case "Ts":
		e.estado.subida = n(1)
	case "Tr":
		e.estado.modoRender = int(n(1))
	case "Tf":
		if len(ops) >= 2 {
			e.estado.tamano = n(1)
			e.estado.fuente = e.l.fuente(nombre(2), e.recursos)
		}
	case "Td":
		e.tlm = matriz{1, 0, 0, 1, n(2), n(1)}.por(e.tlm)
		e.tm = e.tlm
	case "TD":
		e.estado.interlineado = -n(1)
		e.tlm = matriz{1, 0, 0, 1, n(2), n(1)}.por(e.tlm)
		e.tm = e.tlm
	case "Tm":
		if len(ops) >= 6 {
			e.tlm = matriz{n(6), n(5), n(4), n(3), n(2), n(1)}
			e.tm = e.tlm
		}
	case "T*":
		e.nuevaLinea()
	case "Tj":
		e.mostrar(ultimaCadena(ops))
	case "'":
		e.nuevaLinea()
		e.mostrar(ultimaCadena(ops))
	case `"`:
		e.estado.espPal = n(3)
		e.estado.espCar = n(2)
		e.nuevaLinea()
		e.mostrar(ultimaCadena(ops))
	case "TJ":
		if len(ops) > 0 {
			if arr, ok := e.l.d.resolver(ops[len(ops)-1]).(pdfArray); ok {
				e.mostrarArray(arr)
			}
		}

	// --- XObjects ---
	case "Do":
		e.hacer(nombre(1))

	// --- Contenido marcado y compatibilidad: sin efecto visual ---
	case "BMC", "BDC":
		e.abrirMarcaTexto(op, ops)
	case "EMC":
		if e.marcasOmitidas > 0 {
			e.marcasOmitidas--
		} else if len(e.marcas) > 0 {
			if e.marcas[len(e.marcas)-1].oculta {
				e.ocgOculto--
			}
			e.marcas = e.marcas[:len(e.marcas)-1]
		}
	case "MP", "DP", "BX", "EX", "d0", "d1":
	}
}

// abrirMarcaTexto recoge el texto lógico de las ligaduras, detecta capas
// opcionales apagadas (/OC) y acota la pila de contenido marcado.
func (e *ejecucion) abrirMarcaTexto(op string, ops []pdfObj) {
	if e.marcasOmitidas > 0 || len(e.marcas) >= 256 {
		e.marcasOmitidas++
		return
	}
	m := marcaTexto{}
	if op == "BDC" && len(ops) >= 2 {
		raw := ops[len(ops)-1]
		prop := e.l.d.resolver(raw)
		if nombre, ok := prop.(pdfName); ok {
			catalogo, _ := e.l.d.resolver(e.recursos["Properties"]).(pdfDict)
			raw = catalogo[nombre]
			prop = e.l.d.resolver(raw)
		}
		if ref, ok := raw.(pdfRef); ok && e.l.ocgApagados[ref.Num] {
			m.oculta = true
			e.ocgOculto++
		}
		if dict, ok := prop.(pdfDict); ok {
			if !m.oculta && e.l.ocmdApagado(dict) {
				m.oculta = true
				e.ocgOculto++
			}
			if texto, ok := e.l.d.resolver(dict["ActualText"]).(pdfString); ok && len(texto) <= maxTextoMarcadoBytes {
				m.actual, m.presente = decodificarTextoPDF(texto), true
			}
		}
	}
	e.marcas = append(e.marcas, m)
}

func (d *PDFDoc) ocgApagados() map[int]bool {
	cat, _ := d.resolver(d.trailer["Root"]).(pdfDict)
	ocp, _ := d.resolver(cat["OCProperties"]).(pdfDict)
	def, _ := d.resolver(ocp["D"]).(pdfDict)
	if def == nil {
		return nil
	}
	apagados := map[int]bool{}
	if base, _ := d.resolver(def["BaseState"]).(pdfName); base == "OFF" {
		if todos, ok := d.resolver(ocp["OCGs"]).(pdfArray); ok {
			for _, o := range todos {
				if ref, ok := o.(pdfRef); ok {
					apagados[ref.Num] = true
				}
			}
		}
		if encendidos, ok := d.resolver(def["ON"]).(pdfArray); ok {
			for _, o := range encendidos {
				if ref, ok := o.(pdfRef); ok {
					delete(apagados, ref.Num)
				}
			}
		}
	}
	if off, ok := d.resolver(def["OFF"]).(pdfArray); ok {
		for _, o := range off {
			if ref, ok := o.(pdfRef); ok {
				apagados[ref.Num] = true
			}
		}
	}
	return apagados
}

func (l *lienzo) ocmdApagado(dict pdfDict) bool {
	if len(l.ocgApagados) == 0 {
		return false
	}
	switch v := dict["OCGs"].(type) {
	case pdfRef:
		return l.ocgApagados[v.Num]
	case pdfArray:
		for _, e := range v {
			if ref, ok := e.(pdfRef); ok && l.ocgApagados[ref.Num] {
				return true
			}
		}
	}
	return false
}

func (e *ejecucion) textoDeLaMarca(texto string) string {
	for i := len(e.marcas) - 1; i >= 0; i-- {
		m := &e.marcas[i]
		if !m.presente {
			continue
		}
		if m.emitida {
			return ""
		}
		m.emitida = true
		return m.actual
	}
	return texto
}

// ultimaCadena devuelve el último operando si es una cadena. Los operadores que
// muestran texto la llevan siempre ahí, detrás de sus operandos numéricos.
func ultimaCadena(ops []pdfObj) pdfString {
	if len(ops) == 0 {
		return nil
	}
	s, _ := ops[len(ops)-1].(pdfString)
	return s
}

// --- Caminos -----------------------------------------------------------------

// apuntar guarda el operador para el rasterizador. No hace nada en el camino de
// SVG, que es el que sigue mandando.
func (e *ejecucion) apuntar(op byte, v ...float64) {
	if e.l.r == nil {
		return
	}
	var s segCam
	s.op = op
	copy(s.p[:], v)
	e.cam = append(e.cam, s)
}

func (e *ejecucion) mover(x, y float64) {
	e.camino.WriteString("M" + num(x) + " " + num(y))
	e.apuntar('M', x, y)
	e.curX, e.curY, e.iniX, e.iniY = x, y, x, y
	e.hayCamino = true
}

func (e *ejecucion) linea(x, y float64) {
	if !e.hayCamino {
		e.mover(x, y)
		return
	}
	if e.l.recolectando {
		// Una raya de impreso se dibuja con m/l tanto o más que con `re`, y sin
		// esto la mitad de los formularios en papel no tendrían dónde escribir.
		// Se guarda como una caja de altura cero: quien la interpreta ya decide.
		e.l.apuntarRect(e, e.curX, e.curY, x-e.curX, y-e.curY)
	}
	e.camino.WriteString("L" + num(x) + " " + num(y))
	e.apuntar('L', x, y)
	e.curX, e.curY = x, y
}

func (e *ejecucion) curva(x1, y1, x2, y2, x3, y3 float64) {
	if !e.hayCamino {
		e.mover(x1, y1)
	}
	e.camino.WriteString("C" + num(x1) + " " + num(y1) + " " + num(x2) + " " + num(y2) +
		" " + num(x3) + " " + num(y3))
	e.apuntar('C', x1, y1, x2, y2, x3, y3)
	e.curX, e.curY = x3, y3
}

func (e *ejecucion) cerrarSubcamino() {
	if e.hayCamino {
		e.camino.WriteString("Z")
		e.apuntar('Z')
		e.curX, e.curY = e.iniX, e.iniY
	}
}

func (e *ejecucion) rectangulo(x, y, w, h float64) {
	e.camino.WriteString("M" + num(x) + " " + num(y) +
		"h" + num(w) + "v" + num(h) + "h" + num(-w) + "Z")
	e.apuntar('M', x, y)
	e.apuntar('L', x+w, y)
	e.apuntar('L', x+w, y+h)
	e.apuntar('L', x, y+h)
	e.apuntar('Z')
	e.curX, e.curY, e.iniX, e.iniY = x, y, x, y
	e.hayCamino = true
	if e.l.recolectando {
		e.l.apuntarRect(e, x, y, w, h)
	}
}

// apuntarRect guarda la caja en coordenadas de PÁGINA. Se transforman las
// CUATRO esquinas y no dos: con una CTM que rote, el envolvente no sale de la
// diagonal.
func (l *lienzo) apuntarRect(e *ejecucion, x, y, w, h float64) {
	if len(l.rects) > 4000 {
		return // una página con miles de trazos no es un impreso
	}
	m := e.estado.ctm
	esquinas := [4][2]float64{{x, y}, {x + w, y}, {x, y + h}, {x + w, y + h}}
	var r rectPagina
	for i, p := range esquinas {
		px := m[0]*p[0] + m[2]*p[1] + m[4]
		py := m[1]*p[0] + m[3]*p[1] + m[5]
		if i == 0 {
			r = rectPagina{px, py, px, py}
			continue
		}
		r.x0, r.y0 = math.Min(r.x0, px), math.Min(r.y0, py)
		r.x1, r.y1 = math.Max(r.x1, px), math.Max(r.y1, py)
	}
	l.rects = append(l.rects, r)
}

// pintar cierra el camino en curso: lo dibuja si toca y lo convierte en recorte
// si venía marcado con W/W*.
func (e *ejecucion) pintar(relleno, trazo, parImpar bool) {
	if e.ocgOculto > 0 {
		relleno, trazo = false, false
	}
	dAttr := e.camino.String()
	e.camino.Reset()
	hab := e.hayCamino
	e.hayCamino = false
	pend := e.recortePe
	e.recortePe = 0
	if e.l.r != nil {
		e.pintarRaster(hab, relleno, trazo, pend)
		e.cam = e.cam[:0]
	}

	if hab && (relleno || trazo) {
		var b strings.Builder
		b.WriteString(`<path d="` + dAttr + `"`)
		if !e.estado.ctm.esIdentidad() {
			b.WriteString(` transform="` + e.estado.ctm.svg() + `"`)
		}
		e.escribirRelleno(&b, relleno, parImpar)
		e.escribirTrazo(&b, trazo)
		if e.estado.mezcla != "" {
			b.WriteString(` style="mix-blend-mode:` + e.estado.mezcla + `"`)
		}
		b.WriteString("/>\n")
		e.l.escribir(b.String())
	}

	if hab && pend != 0 {
		e.aplicarRecorte(dAttr, pend == 2)
	}
}

func (e *ejecucion) escribirRelleno(b *strings.Builder, relleno, parImpar bool) {
	if !relleno {
		b.WriteString(` fill="none"`)
		return
	}
	if e.estado.patronRelleno != "" {
		b.WriteString(` fill="url(#` + e.estado.patronRelleno + `)"`)
	} else {
		b.WriteString(` fill="` + e.estado.relleno + `"`)
	}
	if parImpar {
		b.WriteString(` fill-rule="evenodd"`)
	}
	if e.estado.alfaRelleno < 1 {
		b.WriteString(` fill-opacity="` + num(e.estado.alfaRelleno) + `"`)
	}
}

func (e *ejecucion) escribirTrazo(b *strings.Builder, trazo bool) {
	if !trazo {
		return
	}
	if e.estado.patronTrazo != "" {
		b.WriteString(` stroke="url(#` + e.estado.patronTrazo + `)"`)
	} else {
		b.WriteString(` stroke="` + e.estado.trazo + `"`)
	}
	if e.estado.grosor <= 0 {
		// Grosor 0 en PDF es «la línea más fina que el dispositivo sepa pintar»
		// (§8.4.3.2). Con un 0 literal, SVG no pinta NADA: la tabla se quedaría
		// sin bordes. `non-scaling-stroke` da exactamente ese pelo de un píxel.
		b.WriteString(` stroke-width="1" vector-effect="non-scaling-stroke"`)
	} else {
		b.WriteString(` stroke-width="` + num(e.estado.grosor) + `"`)
	}
	switch e.estado.tapa {
	case 1:
		b.WriteString(` stroke-linecap="round"`)
	case 2:
		b.WriteString(` stroke-linecap="square"`)
	}
	switch e.estado.union {
	case 1:
		b.WriteString(` stroke-linejoin="round"`)
	case 2:
		b.WriteString(` stroke-linejoin="bevel"`)
	}
	if e.estado.limite != 10 && e.estado.limite > 0 {
		b.WriteString(` stroke-miterlimit="` + num(e.estado.limite) + `"`)
	}
	if e.estado.guiones != "" {
		b.WriteString(` stroke-dasharray="` + e.estado.guiones + `"`)
	}
	if e.estado.alfaTrazo < 1 {
		b.WriteString(` stroke-opacity="` + num(e.estado.alfaTrazo) + `"`)
	}
}

// aplicarRecorte abre un <g clip-path=...> que se cerrará con el `Q` de este
// nivel. El recorte de PDF es acumulativo: cada W lo INTERSECA con el vigente, y
// anidar grupos da esa intersección sin calcularla.
func (e *ejecucion) aplicarRecorte(dAttr string, parImpar bool) {
	id := e.l.id("rec")
	var b strings.Builder
	b.WriteString(`<clipPath id="` + id + `"`)
	if parImpar {
		b.WriteString(` clip-rule="evenodd"`)
	}
	b.WriteString(`><path d="` + dAttr + `"`)
	if !e.estado.ctm.esIdentidad() {
		b.WriteString(` transform="` + e.estado.ctm.svg() + `"`)
	}
	if parImpar {
		b.WriteString(` clip-rule="evenodd"`)
	}
	b.WriteString("/></clipPath>")
	e.l.defsEscribir(b.String())
	e.abrirGrupo(`<g clip-path="url(#` + id + `)">`)
}

func (e *ejecucion) abrirGrupo(etiqueta string) {
	// El texto abierto tiene que cerrarse antes: un <g> dentro de <text> no vale.
	e.cerrarTexto()
	e.l.escribir(etiqueta)
	e.abiertosN[len(e.abiertosN)-1]++
}

func (e *ejecucion) cerrarNivel() {
	e.cerrarTexto()
	for i := 0; i < e.abiertosN[len(e.abiertosN)-1]; i++ {
		e.l.escribir("</g>")
	}
	e.abiertosN = e.abiertosN[:len(e.abiertosN)-1]
}

// cerrarTodo cierra lo que el flujo dejó abierto. Un PDF con más `q` que `Q` es
// corriente; sin esto el SVG saldría con etiquetas sin cerrar y el navegador
// pintaría media página.
func (e *ejecucion) cerrarTodo() {
	e.cerrarTexto()
	for len(e.abiertosN) > 0 {
		e.cerrarNivel()
	}
}

// guionesCSS traduce `[a b] fase d` a stroke-dasharray. Un array vacío es línea
// continua, que en SVG se escribe omitiendo el atributo.
func (e *ejecucion) guionesCSS(ops []pdfObj) string {
	if len(ops) < 2 {
		return ""
	}
	arr, ok := e.l.d.resolver(ops[len(ops)-2]).(pdfArray)
	if !ok || len(arr) == 0 {
		return ""
	}
	partes := make([]string, 0, len(arr))
	todoCero := true
	for _, v := range arr {
		x := comoReal(e.l.d.resolver(v))
		if x > 0 {
			todoCero = false
		}
		partes = append(partes, num(x))
	}
	if todoCero {
		return "" // [0 0] es continua, y en SVG haría desaparecer la línea
	}
	return strings.Join(partes, ",")
}

// aplicarExtGState aplica /ExtGState, donde vive lo que no tiene operador
// propio: sobre todo la opacidad, que en PDF NO forma parte del color.
func (e *ejecucion) aplicarExtGState(nombre pdfName) {
	egs, _ := e.l.d.resolver(e.recursoDe("ExtGState", nombre)).(pdfDict)
	if egs == nil {
		return
	}
	if v, ok := egs["ca"]; ok {
		e.estado.alfaRelleno = acotar01(comoReal(e.l.d.resolver(v)))
	}
	if v, ok := egs["CA"]; ok {
		e.estado.alfaTrazo = acotar01(comoReal(e.l.d.resolver(v)))
	}
	if v, ok := egs["LW"]; ok {
		e.estado.grosor = comoReal(e.l.d.resolver(v))
	}
	if v, ok := egs["LC"]; ok {
		e.estado.tapa = int(comoReal(e.l.d.resolver(v)))
	}
	if v, ok := egs["LJ"]; ok {
		e.estado.union = int(comoReal(e.l.d.resolver(v)))
	}
	if v, ok := egs["BM"]; ok {
		e.estado.mezcla = mezclaCSS(e.l.d.listaDeNombres(v))
	}
	if v, ok := egs["D"]; ok {
		if arr, ok := e.l.d.resolver(v).(pdfArray); ok && len(arr) >= 1 {
			e.estado.guiones = e.guionesCSS(pdfArray{arr[0], int64(0)})
		}
	}
	if v, ok := egs["Font"]; ok {
		if arr, ok := e.l.d.resolver(v).(pdfArray); ok && len(arr) >= 2 {
			e.estado.tamano = comoReal(e.l.d.resolver(arr[1]))
			if fd, ok := e.l.d.resolver(arr[0]).(pdfDict); ok {
				e.estado.fuente = e.l.fuenteDeDict(fd, refClave(arr[0]))
			}
		}
	}
	// /SMask define una máscara suave por luminosidad o alfa. Traducirla exige
	// renderizar el grupo a parte y usarlo como <mask>; mientras no esté, se
	// ignora en vez de tapar el contenido con un rectángulo.
}

// mezclaCSS traduce el modo de fusión de PDF al de CSS. Los nombres coinciden
// salvo en las mayúsculas y en los dos que CSS no tiene.
func mezclaCSS(nombres []pdfName) string {
	if len(nombres) == 0 {
		return ""
	}
	switch nombres[0] {
	case "Normal", "Compatible":
		return ""
	case "Multiply":
		return "multiply"
	case "Screen":
		return "screen"
	case "Overlay":
		return "overlay"
	case "Darken":
		return "darken"
	case "Lighten":
		return "lighten"
	case "ColorDodge":
		return "color-dodge"
	case "ColorBurn":
		return "color-burn"
	case "HardLight":
		return "hard-light"
	case "SoftLight":
		return "soft-light"
	case "Difference":
		return "difference"
	case "Exclusion":
		return "exclusion"
	case "Hue":
		return "hue"
	case "Saturation":
		return "saturation"
	case "Color":
		return "color"
	case "Luminosity":
		return "luminosity"
	}
	return ""
}

func acotar01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// recursoDe busca un recurso por categoría y nombre en el diccionario vigente.
func (e *ejecucion) recursoDe(categoria, nombre pdfName) pdfObj {
	if e.recursos == nil {
		return nil
	}
	cat, _ := e.l.d.resolver(e.recursos[categoria]).(pdfDict)
	if cat == nil {
		return nil
	}
	return cat[nombre]
}

// --- Texto -------------------------------------------------------------------

func (e *ejecucion) nuevaLinea() {
	e.tlm = matriz{1, 0, 0, 1, 0, -e.estado.interlineado}.por(e.tlm)
	e.tm = e.tlm
}

// mostrarArray ejecuta TJ: cadenas alternadas con ajustes de posición en
// milésimas de em, que es como la mayoría de los PDF separan las palabras.
func (e *ejecucion) mostrarArray(arr pdfArray) {
	for _, el := range arr {
		switch v := el.(type) {
		case pdfString:
			e.mostrar(v)
		case int64, float64:
			if f := e.estado.fuente; f != nil && f.vertical {
				// En vertical el ajuste baja la línea y no lleva la escala
				// horizontal (§9.4.4).
				e.tm = matriz{1, 0, 0, 1, 0, -comoReal(v) / 1000 * e.estado.tamano}.por(e.tm)
				continue
			}
			avance := -comoReal(v) / 1000 * e.estado.tamano * e.estado.escalaH
			e.tm = matriz{1, 0, 0, 1, avance, 0}.por(e.tm)
		}
	}
}

// mostrar pinta una cadena y adelanta la matriz de texto.
func (e *ejecucion) mostrar(s pdfString) {
	if len(s) == 0 || e.l.opts.SinTexto || e.ocgOculto > 0 {
		return
	}
	f := e.estado.fuente
	if f == nil {
		f = fuenteDeReserva()
	}
	glifos := f.decodificar(s)

	// El avance total se calcula ANTES de emitir para poder fijar `textLength`:
	// es lo que hace que el texto caiga donde el PDF dice aunque el navegador
	// use otra fuente. Sin él, una línea justificada se desmonta.
	var avance, bajada float64
	for _, g := range glifos {
		a := g.ancho/1000*e.estado.tamano + e.estado.espCar
		if g.esEspacioSimple {
			a += e.estado.espPal
		}
		avance += a * e.estado.escalaH
		if f.vertical {
			b := g.alto/1000*e.estado.tamano + e.estado.espCar
			if g.esEspacioSimple {
				b += e.estado.espPal
			}
			bajada += b
		}
	}

	if e.l.texto != nil {
		if f.vertical {
			e.l.texto.anadir(e, glifos, 0, bajada)
			e.tm = matriz{1, 0, 0, 1, 0, bajada}.por(e.tm)
			return
		}
		e.l.texto.anadir(e, glifos, avance, 0)
		e.tm = matriz{1, 0, 0, 1, avance, 0}.por(e.tm)
		return
	}

	// Tipo 3 va por otro camino: cada glifo es un flujo que hay que ejecutar, y
	// el bucle adelanta la matriz de texto por su cuenta glifo a glifo. Si pinta,
	// aquí no queda nada por hacer.
	if f.type3 != nil && e.estado.tamano != 0 {
		if e.dibujarGlifosType3(f, s) {
			return
		}
	}

	if e.estado.modoRender != 7 && e.estado.tamano != 0 {
		e.emitirTexto(f, glifos, avance)
		// El PNG dibuja los CONTORNOS de los mismos glifos: es lo que un motor de
		// SVG sin `<text>` no puede hacer y por lo que existe esta salida.
		e.dibujarTexto(f, glifos)
	}
	if f.vertical {
		e.tm = matriz{1, 0, 0, 1, 0, bajada}.por(e.tm)
		return
	}
	e.tm = matriz{1, 0, 0, 1, avance, 0}.por(e.tm)
}

func (e *ejecucion) emitirTexto(f *fuentePDF, glifos []glifo, avance float64) {
	var texto strings.Builder
	for _, g := range glifos {
		texto.WriteString(g.texto)
	}
	t := texto.String()
	if strings.TrimSpace(t) == "" && e.estado.modoRender != 3 {
		return // solo espacios: no aporta píxeles y ensucia la selección
	}

	trm := matriz{e.estado.escalaH, 0, 0, 1, 0, e.estado.subida}.
		por(e.tm).por(e.estado.ctm)

	var b strings.Builder
	b.WriteString(`<text transform="` + trm.svg() + ` scale(1,-1)" xml:space="preserve"`)
	b.WriteString(` font-family="` + escaparXML(f.familia) + `" font-size="` + num(e.estado.tamano) + `"`)
	if f.negrita {
		b.WriteString(` font-weight="bold"`)
	}
	if f.italica {
		b.WriteString(` font-style="italic"`)
	}
	// El avance manda sobre la métrica de la fuente que use el navegador: si el
	// PDF dice que la cadena mide 87,4 puntos, mide 87,4 aunque la fuente
	// sustituta sea otra. `spacingAndGlyphs` reparte la diferencia en el espacio
	// entre letras Y en su anchura, que es lo que menos se nota.
	if avance > 0 && len(glifos) > 1 {
		b.WriteString(` textLength="` + num(avance/e.estado.escalaH) + `" lengthAdjust="spacingAndGlyphs"`)
	}
	if f.cmapVisual {
		e.emitirTextoPorCodigo(&b, glifos)
		t = e.textoDeLaMarca(t)
		// La capa Unicode sigue seleccionándose y copiándose como antes. Los
		// códigos privados solo eligen el contorno y nunca entran en la selección.
		b.WriteString(` fill="#000000" fill-opacity="0"`)
	} else {
		e.pintarTexto(&b)
	}
	b.WriteString(">" + escaparXML(t) + "</text>\n")
	e.l.escribir(b.String())
}

// emitirTextoPorCodigo evita colisiones de Unicode en ligaduras sin ToUnicode.
// El dibujo va por el código original, igual que PNG; su texto lógico se emite
// después en la capa seleccionable. Word usa ActualText para ligaduras como «ti»
// cuyo contorno no tiene un punto Unicode propio.
func (e *ejecucion) emitirTextoPorCodigo(base *strings.Builder, glifos []glifo) {
	var b, texto strings.Builder
	b.WriteString(base.String())
	b.WriteString(` aria-hidden="true" style="-webkit-user-select:none;user-select:none;pointer-events:none"`)
	e.pintarTexto(&b)
	for _, g := range glifos {
		texto.WriteRune(0xE000 + rune(g.codigo&0xFF))
	}
	b.WriteString(">" + texto.String() + "</text>\n")
	e.l.escribir(b.String())
}

// pintarTexto aplica el modo de render del texto (§9.3.6).
func (e *ejecucion) pintarTexto(b *strings.Builder) {
	switch e.estado.modoRender {
	case 1, 5: // solo contorno
		b.WriteString(` fill="none" stroke="` + e.estado.trazo + `" stroke-width="` + num(e.estado.grosor) + `"`)
	case 2, 6: // relleno y contorno
		b.WriteString(` fill="` + e.estado.relleno + `" stroke="` + e.estado.trazo + `" stroke-width="` + num(e.estado.grosor) + `"`)
	case 3, 7:
		// Invisible. Es la capa de texto de un PDF escaneado con OCR: se emite
		// transparente en vez de omitirla, porque `fill-opacity:0` sigue siendo
		// seleccionable y buscable. Omitirlo dejaría el escaneo sin texto.
		b.WriteString(` fill="#000000" fill-opacity="0"`)
	default:
		b.WriteString(` fill="` + e.estado.relleno + `"`)
		if e.estado.alfaRelleno < 1 {
			b.WriteString(` fill-opacity="` + num(e.estado.alfaRelleno) + `"`)
		}
	}
}

func (e *ejecucion) cerrarTexto() {
	if e.textoAbi {
		e.l.escribir("</text>")
		e.textoAbi = false
	}
}

// --- XObjects ----------------------------------------------------------------

func (e *ejecucion) hacer(nombre pdfName) {
	if e.ocgOculto > 0 {
		return
	}
	obj := e.recursoDe("XObject", nombre)
	st, ok := e.l.d.resolver(obj).(*pdfStream)
	if !ok {
		return
	}
	sub, _ := e.l.d.resolver(st.Dict["Subtype"]).(pdfName)
	switch sub {
	case "Image":
		if e.l.recolectando {
			e.l.apuntarImagen(e, st, string(nombre))
			return
		}
		e.l.dibujarImagen(e, st)
	case "Form":
		e.formulario(st)
	}
}

// formulario ejecuta un XObject de formulario: su propio flujo de contenido con
// su /Matrix, su /BBox de recorte y sus recursos. Es lo que usan las apariencias
// de anotación, los sellos y casi todo lo que se repite en un documento.
func (e *ejecucion) formulario(st *pdfStream) {
	contenido, err := e.l.d.datosDeFlujo(st)
	if err != nil {
		return
	}
	sub := e.estado
	if arr, ok := e.l.d.resolver(st.Dict["Matrix"]).(pdfArray); ok && len(arr) >= 6 {
		var m matriz
		for i := 0; i < 6; i++ {
			m[i] = comoReal(e.l.d.resolver(arr[i]))
		}
		sub.ctm = m.por(sub.ctm)
	}

	recursos, _ := e.l.d.resolver(st.Dict["Resources"]).(pdfDict)
	if recursos == nil {
		recursos = e.recursos // heredar: un formulario sin /Resources usa los de fuera
	}

	cierres := 0
	if caja, ok := e.l.d.resolver(st.Dict["BBox"]).(pdfArray); ok && len(caja) >= 4 {
		var v [4]float64
		for i := 0; i < 4; i++ {
			v[i] = comoReal(e.l.d.resolver(caja[i]))
		}
		if e.l.r != nil {
			sub.recorte = e.l.r.recorteDeCaja(v, sub.ctm, sub.recorte)
		}
		id := e.l.id("rec")
		e.l.defsEscribir(`<clipPath id="` + id + `"><path d="M` +
			num(math.Min(v[0], v[2])) + " " + num(math.Min(v[1], v[3])) +
			"h" + num(math.Abs(v[2]-v[0])) + "v" + num(math.Abs(v[3]-v[1])) +
			"h" + num(-math.Abs(v[2]-v[0])) + `Z" transform="` + sub.ctm.svg() + `"/></clipPath>`)
		e.cerrarTexto()
		e.l.escribir(`<g clip-path="url(#` + id + `)">`)
		cierres++
	}

	e.l.prof++
	e.l.ejecutar(contenido, recursos, sub)
	e.l.prof--

	for i := 0; i < cierres; i++ {
		e.l.escribir("</g>")
	}
}

// --- Anotaciones -------------------------------------------------------------

// anotacionesDePagina pinta las apariencias de /Annots. Sin esto, un PDF con
// resaltados, sellos, campos de formulario rellenos o una firma se vería VACÍO
// en esas zonas: su dibujo no está en el flujo de la página, sino en el flujo de
// apariencia de cada anotación.
func (l *lienzo) anotacionesDePagina(p PDFPagina, base matriz) {
	anots, _ := l.d.resolver(p.dict["Annots"]).(pdfArray)
	redibujar := len(anots) > 0 && l.d.necesitaApariencias()
	for _, a := range anots {
		ad, ok := l.d.resolver(a).(pdfDict)
		if !ok {
			continue
		}
		if l.anotacionOculta(ad) {
			continue
		}
		ap := l.aparienciaNormal(ad)
		if redibujar || ap == nil {
			if nueva := l.aparienciaRegenerada(ad); nueva != nil {
				ap = nueva
			}
		}
		if ap == nil {
			ap = l.aparienciaDeTextoLibre(ad)
		}
		if ap == nil {
			continue
		}
		rect, ok := l.rectangulo(ad["Rect"])
		if !ok {
			continue
		}
		l.dibujarApariencia(ap, rect, base)
	}
}

// aparienciaDeTextoLibre dibuja un /FreeText que no trae /AP: el texto de
// /Contents con el tamaño y el color de su /DA, que es lo que hace un lector
// (Poppler incluido). pypdf y otros generadores las escriben así, y antes no se
// veían ni salían en el texto de la página.
func (l *lienzo) aparienciaDeTextoLibre(ad pdfDict) *pdfStream {
	if sub, _ := l.d.resolver(ad["Subtype"]).(pdfName); sub != "FreeText" {
		return nil
	}
	contenido, _ := l.d.resolver(ad["Contents"]).(pdfString)
	rect, ok := l.rectangulo(ad["Rect"])
	if !ok || len(contenido) == 0 {
		return nil
	}
	da, _ := l.d.resolver(ad["DA"]).(pdfString)
	tam, color := interpretarDA(string(da))
	if tam <= 0 {
		tam = 12
	}
	a := Anotacion{Tipo: AnotTextoLibre, Rect: rect, Texto: decodificarTextoPDF(contenido), Color: color, Opacidad: 1, Tamano: tam}
	datos, recursos := a.apariencia()
	if datos == nil {
		return nil
	}
	dict := pdfDict{"Type": pdfName("XObject"), "Subtype": pdfName("Form"), "BBox": rectPDF(rect)}
	if recursos != nil {
		dict["Resources"] = recursos
	}
	return &pdfStream{Dict: dict, Raw: datos, sinCifrar: true}
}

// anotacionOculta respeta /F: el bit 2 es «oculta» y el 6 «no imprimir, sí
// mostrar». Se salta la oculta, que es como un campo condicional se apaga.
func (l *lienzo) anotacionOculta(ad pdfDict) bool {
	f, ok := comoEntero(l.d.resolver(ad["F"]))
	if !ok {
		return false
	}
	const oculta = 1 << 1
	if f&oculta != 0 {
		return true
	}
	sub, _ := l.d.resolver(ad["Subtype"]).(pdfName)
	// Popup es la ventanita de un comentario: se dibuja solo cuando está abierta,
	// y en un visor la abre el usuario, no el documento.
	return sub == "Popup" || sub == "Link"
}

// aparienciaNormal saca /AP /N. Puede ser un flujo directo o un diccionario de
// estados (una casilla tiene /Off y /Yes); en ese caso manda /AS.
func (l *lienzo) aparienciaNormal(ad pdfDict) *pdfStream {
	ap, _ := l.d.resolver(ad["AP"]).(pdfDict)
	if ap == nil {
		return nil
	}
	switch v := l.d.resolver(ap["N"]).(type) {
	case *pdfStream:
		return v
	case pdfDict:
		if as, ok := l.d.resolver(ad["AS"]).(pdfName); ok {
			if st, ok := l.d.resolver(v[as]).(*pdfStream); ok {
				return st
			}
			return nil
		}
		// Sin /AS, un diccionario con un solo estado es inequívoco.
		if len(v) == 1 {
			for _, only := range v {
				if st, ok := l.d.resolver(only).(*pdfStream); ok {
					return st
				}
			}
		}
	}
	return nil
}

func (l *lienzo) rectangulo(o pdfObj) ([4]float64, bool) {
	arr, ok := l.d.resolver(o).(pdfArray)
	if !ok || len(arr) < 4 {
		return [4]float64{}, false
	}
	var v [4]float64
	for i := 0; i < 4; i++ {
		v[i] = comoReal(l.d.resolver(arr[i]))
	}
	return [4]float64{
		math.Min(v[0], v[2]), math.Min(v[1], v[3]),
		math.Max(v[0], v[2]), math.Max(v[1], v[3]),
	}, true
}

// dibujarApariencia coloca el flujo de apariencia dentro de /Rect. El algoritmo
// es el del §12.5.5: se transforma la /BBox por la /Matrix, y el rectángulo que
// la envuelve se estira hasta encajar en /Rect. Saltarse este paso es el fallo
// clásico —el sello sale del tamaño equivocado o desplazado— porque casi
// siempre BBox y Rect miden distinto.
func (l *lienzo) dibujarApariencia(ap *pdfStream, rect [4]float64, base matriz) {
	contenido, err := l.d.datosDeFlujo(ap)
	if err != nil {
		return
	}
	m := identidad
	if arr, ok := l.d.resolver(ap.Dict["Matrix"]).(pdfArray); ok && len(arr) >= 6 {
		for i := 0; i < 6; i++ {
			m[i] = comoReal(l.d.resolver(arr[i]))
		}
	}
	bbox, tiene := l.rectangulo(ap.Dict["BBox"])
	ajuste := identidad
	if tiene {
		// Las cuatro esquinas transformadas: con rotación, el envolvente no es la
		// caja transformada esquina a esquina.
		xs := []float64{}
		ys := []float64{}
		for _, pt := range [4][2]float64{{bbox[0], bbox[1]}, {bbox[2], bbox[1]}, {bbox[2], bbox[3]}, {bbox[0], bbox[3]}} {
			xs = append(xs, m[0]*pt[0]+m[2]*pt[1]+m[4])
			ys = append(ys, m[1]*pt[0]+m[3]*pt[1]+m[5])
		}
		minX, maxX := minimo(xs), maximo(xs)
		minY, maxY := minimo(ys), maximo(ys)
		sx, sy := 1.0, 1.0
		if maxX-minX > 1e-9 {
			sx = (rect[2] - rect[0]) / (maxX - minX)
		}
		if maxY-minY > 1e-9 {
			sy = (rect[3] - rect[1]) / (maxY - minY)
		}
		ajuste = matriz{sx, 0, 0, sy, rect[0] - minX*sx, rect[1] - minY*sy}
	}

	st := estadoInicial(m.por(ajuste).por(base))
	l.prof++
	recursos, _ := l.d.resolver(ap.Dict["Resources"]).(pdfDict)
	l.ejecutar(contenido, recursos, st)
	l.prof--
}

func minimo(v []float64) float64 {
	m := v[0]
	for _, x := range v[1:] {
		if x < m {
			m = x
		}
	}
	return m
}

func maximo(v []float64) float64 {
	m := v[0]
	for _, x := range v[1:] {
		if x > m {
			m = x
		}
	}
	return m
}

// refClave da una clave estable para cachear una fuente. Con referencia
// indirecta es el número de objeto; sin ella, no se cachea (devuelve "").
func refClave(o pdfObj) string {
	if r, ok := o.(pdfRef); ok {
		return strconv.Itoa(r.Num) + "_" + strconv.Itoa(r.Gen)
	}
	return ""
}
