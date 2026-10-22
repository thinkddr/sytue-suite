// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Ola P7 «Proteger»: CENSURA REAL y saneado de metadatos.
//
// Es la única operación del motor que reescribe el fichero DESTRUYENDO
// contenido, y lo es a propósito. Pintar un rectángulo negro encima deja debajo
// el texto entero —seleccionable, copiable y recuperable con cualquier extractor—
// así que eso no es censurar: es un incidente de protección de datos con retraso.
// Aquí se QUITAN del flujo de contenido los operadores de texto que caen en la
// zona marcada, y lo que queda ya no contiene esos bytes.
//
// Tres decisiones que sostienen la garantía:
//
//   - **El flujo original no se copia.** Se genera uno nuevo y el viejo no viaja
//     al documento de salida. Copiarlo y luego dejar de apuntarlo NO valdría: un
//     objeto huérfano se escribe igual en el fichero y el texto seguiría dentro,
//     a un editor hexadecimal de distancia.
//   - **Se sigue la matriz de verdad.** El CTM (`cm`, `q`, `Q`) y la matriz de
//     texto se llevan completas. Dar por hecho que las coordenadas del texto son
//     las de la página es la forma silenciosa de censurar el sitio equivocado.
//   - **Lo que no se puede garantizar, falla.** Un objeto incrustado que no se
//     puede leer o decodificar, un degradado de malla cuya extensión no se conoce o
//     una imagen que no cabe en memoria devuelven un error claro. Una censura que
//     parece hecha y no lo está es peor que no tenerla.
//
// LO QUE SE GARANTIZA, dicho sin adornos, en la zona marcada:
//
//   - **Texto**: se quitan los operadores de texto que caen dentro.
//   - **Imágenes** (XObject o en línea): se REESCRIBEN los píxeles de la zona a
//     negro, con su máscara suave, y se emite una imagen NUEVA solo para esta
//     página; la original no viaja al fichero de salida (si otra página la usa, la
//     conserva esa página, intacta). Un escaneo con el texto dentro queda censurado
//     de verdad.
//   - **Vectores** (trazos y rellenos): se eliminan los operadores de dibujo que
//     tocan la zona. Un rectángulo relleno se recorta (queda lo de fuera), una
//     línea recta se corta en la zona y se conserva el resto; cualquier otra forma
//     que la toque se quita ENTERA, que es lo conservador. Un trazado de recorte
//     (`W`) no pinta y se conserva: no puede esconder texto, pero conserva su forma.
//   - **Objetos de formulario** (membretes, logotipos, sellos): se aplica lo mismo
//     a su contenido en una COPIA propia de la página; la original no viaja si
//     ninguna otra página la dibuja.
//   - La miniatura de la página (`/Thumb`) no viaja: es una foto de la página sin
//     censurar.
//
// Fuera de alcance, y dicho: los patrones de mosaico y los degradados que pintan
// contenido (no los emite ningún generador corriente), y el texto o la imagen que
// una fuente tipo 3 dibuje con su propio programa.
//
// La UNIDAD de borrado del texto es el operador, no el glifo: si el generador
// pintó la línea entera con un solo `Tj`, se va la línea entera. Se peca de más,
// nunca de menos, porque el error en la otra dirección no se ve.
package domain

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strings"
)

// ErrRedaccionNoGarantizada se devuelve cuando en la zona marcada hay algo que
// no se puede eliminar con garantías. Es un error de PRODUCTO, no un fallo: la
// interfaz debe contarlo y ofrecer una salida (rasterizar la página, recortar),
// nunca reintentar por su cuenta ni ignorarlo.
var ErrRedaccionNoGarantizada = errors.New("pdf: no se puede censurar la zona con garantías")

// maxAreasRedaccion acota las zonas de una petición: cada una se cruza con cada
// imagen y cada trazado de su página.
const maxAreasRedaccion = 1000

// AreaRedaccion es un rectángulo que censurar.
//
// Las coordenadas van en PUNTOS y en el espacio de usuario de la página: origen
// en la esquina INFERIOR IZQUIERDA de su MediaBox y eje Y hacia arriba, que es
// como piensa el PDF y al revés que un lienzo de navegador. Sin rotar: si la
// página lleva /Rotate, el visor gira lo dibujado DESPUÉS, así que quien recoge
// el rectángulo en pantalla tiene que deshacer ese giro antes de mandarlo.
type AreaRedaccion struct {
	// Pagina es 0-based.
	Pagina         int
	X1, Y1, X2, Y2 float64
}

// finitos dice si ningún número es NaN ni infinito.
func finitos(v ...float64) bool {
	for _, x := range v {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return false
		}
	}
	return true
}

// normalizar deja las esquinas en orden para que un rectángulo dibujado de
// derecha a izquierda —que es la mitad de las veces— no tenga superficie
// negativa y se descarte sin censurar nada.
func (a AreaRedaccion) normalizar() AreaRedaccion {
	if a.X1 > a.X2 {
		a.X1, a.X2 = a.X2, a.X1
	}
	if a.Y1 > a.Y2 {
		a.Y1, a.Y2 = a.Y2, a.Y1
	}
	return a
}

// catalogoSinTexto es lo que censurar conserva del catálogo porque no puede
// repetir lo tapado: el idioma, cómo se abre (diseño, modo, preferencias y una
// apertura que solo lleva a una página), las intenciones de salida de PDF/A y la
// numeración de las páginas. Se quedan fuera la estructura, el índice, /Names,
// el formulario, las capas y las firmas.
func catalogoSinTexto(doc *PDFDoc, k pdfName, v pdfObj) bool {
	switch k {
	case "Lang", "ViewerPreferences", "PageLayout", "PageMode", "OutputIntents", "PageLabels":
		return true
	case "OpenAction":
		destino := doc.resolver(v)
		if a, ok := destino.(pdfDict); ok {
			if s, _ := doc.resolver(a["S"]).(pdfName); s != "GoTo" || a["Next"] != nil {
				return false
			}
			destino = doc.resolver(a["D"])
		}
		_, esPagina := destino.(pdfArray)
		return esPagina
	}
	return false
}

// RedactarPDF devuelve el documento con las zonas marcadas censuradas de verdad:
// el texto que caía dentro deja de existir en el fichero y encima queda un
// recuadro negro, que es lo que hace visible que ahí había algo.
//
// Las páginas sin zonas marcadas se copian tal cual.
func RedactarPDF(doc *PDFDoc, areas []AreaRedaccion) ([]byte, error) {
	porPagina, err := zonasRedaccionPorPagina(doc, areas)
	if err != nil {
		return nil, err
	}

	e := NuevoEnsambladorPDF()
	// Censurar no conserva del catálogo nada que pueda llevar texto: el árbol de
	// estructura lleva /ActualText y /Alt, el índice títulos y /Names adjuntos, y
	// cualquiera de ellos puede repetir lo que se acaba de tapar.
	e.soloCatalogo = catalogoSinTexto
	// Las zonas llegan relativas a la esquina de la página; el espacio de
	// usuario puede tener el origen en otra parte (un MediaBox que no empieza
	// en 0,0 existe y es legal). Sin este desplazamiento se censuraría
	// desplazado justo en los documentos recortados.
	absolutasPorPagina := make(map[int][]AreaRedaccion, len(porPagina))
	for i, zonas := range porPagina {
		p, err := doc.Pagina(i)
		if err != nil {
			return nil, err
		}
		x0, y0 := doc.origenDeCaja(p)
		for _, z := range zonas {
			absolutasPorPagina[i] = append(absolutasPorPagina[i], AreaRedaccion{
				Pagina: i,
				X1:     z.X1 + x0, Y1: z.Y1 + y0,
				X2: z.X2 + x0, Y2: z.Y2 + y0,
			})
		}
	}
	// Los campos del formulario: viajan los de fuera; el que toca una zona, o
	// depende de uno que la toca, no viaja ni por los /Kids de su padre.
	formulario := doc.formularioAlCensurar(absolutasPorPagina)
	e.excluidos = make(map[origenObj]bool, len(formulario.excluidos))
	for n := range formulario.excluidos {
		e.excluidos[origenObj{doc: doc, num: n}] = true
	}
	// Un solo presupuesto para TODO el documento: un PDF hostil puede apuntar mil
	// veces a una imagen enorme, y cada página censurada la decodificaría de nuevo.
	pres := nuevoPresupuestoCensura()
	for i := 0; i < doc.NumPaginas(); i++ {
		absolutas := absolutasPorPagina[i]
		if len(absolutas) == 0 {
			if err := e.AnadirPagina(doc, i, 0); err != nil {
				return nil, err
			}
			continue
		}
		p, err := doc.Pagina(i)
		if err != nil {
			return nil, err
		}

		contenido, err := doc.contenidoDePagina(p)
		if err != nil {
			return nil, fmt.Errorf("pdf: página %d: %w", i+1, err)
		}
		limpio, nivel, err := doc.censurarPagina(p, contenido, absolutas, pres)
		if err != nil {
			return nil, fmt.Errorf("pdf: página %d: %w", i+1, err)
		}
		recursos, cambian, err := e.recursosCensurados(doc, nivel)
		if err != nil {
			return nil, fmt.Errorf("pdf: página %d: %w", i+1, err)
		}
		if !cambian {
			recursos = nil // la página copia los suyos
		}
		if err := e.anadirPagina(doc, i, 0, opcionesPagina{
			contenido: limpio,
			annots:    doc.annotsFueraDeZonas(p, absolutas),
			recursos:  recursos,
		}); err != nil {
			return nil, err
		}
		if componentesIntencion(doc) == 0 {
			e.asegurarDefaultGray(e.ultimaPendiente())
		}
	}
	if forma := e.formularioCensurado(doc, formulario); forma != nil {
		if e.extrasCatalogo == nil {
			e.extrasCatalogo = pdfDict{}
		}
		e.extrasCatalogo["AcroForm"] = forma
	}
	return e.Bytes()
}

// zonasRedaccionPorPagina valida las áreas antes de modificar el documento.
func zonasRedaccionPorPagina(doc *PDFDoc, areas []AreaRedaccion) (map[int][]AreaRedaccion, error) {
	if len(areas) == 0 {
		return nil, fmt.Errorf("pdf: no se ha marcado ninguna zona que censurar")
	}
	if len(areas) > maxAreasRedaccion {
		return nil, fmt.Errorf("pdf: demasiadas zonas marcadas de una vez (máximo %d)", maxAreasRedaccion)
	}
	porPagina := map[int][]AreaRedaccion{}
	for _, a := range areas {
		if a.Pagina < 0 || a.Pagina >= doc.NumPaginas() {
			return nil, fmt.Errorf("pdf: la página %d no existe (el documento tiene %d)",
				a.Pagina+1, doc.NumPaginas())
		}
		if !finitos(a.X1, a.Y1, a.X2, a.Y2) {
			return nil, fmt.Errorf("pdf: la zona marcada en la página %d no tiene coordenadas válidas", a.Pagina+1)
		}
		n := a.normalizar()
		if n.X2-n.X1 <= 0 || n.Y2-n.Y1 <= 0 {
			// Un rectángulo sin superficie no censura nada y dejaría el recuadro
			// negro igual: exactamente la falsa sensación que hay que evitar.
			return nil, fmt.Errorf("pdf: la zona marcada en la página %d no tiene superficie", a.Pagina+1)
		}
		porPagina[a.Pagina] = append(porPagina[a.Pagina], n)
	}

	return porPagina, nil
}

// SanearMetadatosPDF devuelve el documento sin nada que no haga falta para verlo.
//
// Se va TODO lo que arrastra procedencia, y se va porque el ensamblador escribe
// un catálogo y un tráiler nuevos en vez de copiar los de origen: el diccionario
// /Info (título, autor, asunto, palabras clave, aplicación de origen, fechas de
// creación y modificación), el XMP de /Root/Metadata, los identificadores /ID
// del tráiler —que siguen a un fichero entre copias— y el historial de
// actualizaciones incrementales, que desaparece al reescribir desde cero.
//
// Y lo que sí viajaba porque cuelga de las páginas: /Metadata por página,
// /PieceInfo —los datos privados de la aplicación que lo generó, donde algunos
// programas guardan el documento ORIGINAL entero— y /LastModified. Esos los
// descarta el propio ensamblador en cualquier operación (ver claveQueNoViaja),
// no solo aquí.
//
// Lo que NO toca: el texto y las imágenes del documento. Sanear no es censurar;
// para eso está RedactarPDF.
func SanearMetadatosPDF(doc *PDFDoc) ([]byte, error) {
	e := NuevoEnsambladorPDF()
	// Sanear es justo quitar lo que no está en las páginas: el catálogo no viaja.
	e.sinCatalogo = true
	for i := 0; i < doc.NumPaginas(); i++ {
		if err := e.AnadirPagina(doc, i, 0); err != nil {
			return nil, err
		}
	}
	return e.Bytes()
}

// origenDeCaja devuelve la esquina inferior izquierda del MediaBox de la página.
func (d *PDFDoc) origenDeCaja(p PDFPagina) (float64, float64) {
	caja, ok := d.resolver(p.dict["MediaBox"]).(pdfArray)
	if !ok || len(caja) < 4 {
		return 0, 0
	}
	var v [4]float64
	for i := 0; i < 4; i++ {
		v[i] = comoReal(d.resolver(caja[i]))
	}
	return math.Min(v[0], v[2]), math.Min(v[1], v[3])
}

// annotsFueraDeZonas devuelve las anotaciones de la página que NO tocan ninguna
// zona censurada, o nil si la página no tiene ninguna.
//
// Hay que filtrarlas ANTES de copiarlas, no después: una nota, un campo de
// formulario relleno o un comentario emergente sobre la zona censurada llevan el
// texto dentro (en /Contents, en /V o en su apariencia), y copiarlos para luego
// no apuntarlos dejaría el dato en el fichero igual que el flujo huérfano.
func (d *PDFDoc) annotsFueraDeZonas(p PDFPagina, zonas []AreaRedaccion) pdfArray {
	lista, ok := d.resolver(p.dict["Annots"]).(pdfArray)
	if !ok {
		return nil
	}
	fuera := make(pdfArray, 0, len(lista))
	for _, a := range lista {
		an, ok := d.resolver(a).(pdfDict)
		if !ok {
			continue // una anotación ilegible no se copia: no se puede comprobar
		}
		r, ok := d.resolver(an["Rect"]).(pdfArray)
		if !ok || len(r) < 4 {
			continue // sin /Rect no se sabe dónde está; se descarta por seguridad
		}
		var v [4]float64
		for i := 0; i < 4; i++ {
			v[i] = comoReal(d.resolver(r[i]))
		}
		c := cajaPDF{
			x1: math.Min(v[0], v[2]), y1: math.Min(v[1], v[3]),
			x2: math.Max(v[0], v[2]), y2: math.Max(v[1], v[3]),
		}
		if !c.tocaAlguna(zonas) {
			fuera = append(fuera, a)
		}
	}
	return fuera
}

// --- Geometría ---------------------------------------------------------------

// matrizPDF es la matriz afín de PDF [a b c d e f] (§8.3.3): la fila que falta
// es siempre [0 0 1], así que con seis números basta.
type matrizPDF [6]float64

var matrizIdentidad = matrizPDF{1, 0, 0, 1, 0, 0}

// por devuelve m×n: primero se aplica m y después n, que es el orden en el que
// PDF compone («cm» premultiplica sobre el CTM en curso).
func (m matrizPDF) por(n matrizPDF) matrizPDF {
	return matrizPDF{
		m[0]*n[0] + m[1]*n[2],
		m[0]*n[1] + m[1]*n[3],
		m[2]*n[0] + m[3]*n[2],
		m[2]*n[1] + m[3]*n[3],
		m[4]*n[0] + m[5]*n[2] + n[4],
		m[4]*n[1] + m[5]*n[3] + n[5],
	}
}

func (m matrizPDF) punto(x, y float64) (float64, float64) {
	return m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]
}

// cajaPDF es un rectángulo alineado con los ejes en espacio de usuario.
type cajaPDF struct{ x1, y1, x2, y2 float64 }

// cajaDeRectangulo transforma las cuatro esquinas de un rectángulo por m y
// devuelve la caja que las envuelve. Envolver es lo correcto aquí: con texto
// girado la caja sobra por las esquinas, y sobrar censura de más —que se ve— en
// vez de quedarse corta —que no se ve.
func cajaDeRectangulo(m matrizPDF, x1, y1, x2, y2 float64) cajaPDF {
	xa, ya := m.punto(x1, y1)
	xb, yb := m.punto(x2, y1)
	xc, yc := m.punto(x2, y2)
	xd, yd := m.punto(x1, y2)
	return cajaPDF{
		x1: math.Min(math.Min(xa, xb), math.Min(xc, xd)),
		y1: math.Min(math.Min(ya, yb), math.Min(yc, yd)),
		x2: math.Max(math.Max(xa, xb), math.Max(xc, xd)),
		y2: math.Max(math.Max(ya, yb), math.Max(yc, yd)),
	}
}

// tocaAlguna dice si la caja se solapa con alguna zona marcada.
func (c cajaPDF) tocaAlguna(zonas []AreaRedaccion) bool {
	for _, z := range zonas {
		if c.x2 >= z.X1 && c.x1 <= z.X2 && c.y2 >= z.Y1 && c.y1 <= z.Y2 {
			return true
		}
	}
	return false
}

// --- Anchos de fuente --------------------------------------------------------

// anchosFuente mide cuánto avanza el lápiz al pintar una cadena, que es lo que
// convierte «el texto empieza aquí» en «el texto ocupa de aquí a aquí».
//
// Cuando no se puede medir con garantías (fuente compuesta, Type3, o una de las
// 14 estándar que no declara /Widths) se dice, y quien pregunta trata el
// fragmento como si llegara hasta el borde del papel: censura de más en esa
// línea, nunca de menos.
type anchosFuente struct {
	medible bool
	primero int
	anchos  []float64 // milésimas de em, indexadas desde `primero`
	falta   float64   // /MissingWidth para los códigos fuera del array
}

// anchosDeFuente resuelve /Resources/Font/<nombre> y saca su tabla de anchos.
func (d *PDFDoc) anchosDeFuente(recursos pdfDict, nombre pdfName) anchosFuente {
	fuentes, ok := d.resolver(recursos["Font"]).(pdfDict)
	if !ok {
		return anchosFuente{}
	}
	f, ok := d.resolver(fuentes[nombre]).(pdfDict)
	if !ok {
		return anchosFuente{}
	}
	switch sub, _ := d.resolver(f["Subtype"]).(pdfName); sub {
	case "Type0":
		// Fuente compuesta: los códigos no son bytes sueltos y los anchos viven
		// en /W del descendiente con su propio formato. Medirla mal sería peor
		// que no medirla, así que no se mide.
		return anchosFuente{}
	case "Type3":
		// Sus anchos van en espacio de glifo y los escala /FontMatrix, no /1000.
		return anchosFuente{}
	}
	arr, ok := d.resolver(f["Widths"]).(pdfArray)
	if !ok || len(arr) == 0 {
		return anchosFuente{} // las 14 estándar pueden no traerlos
	}
	primero, _ := comoEntero(d.resolver(f["FirstChar"]))
	out := anchosFuente{medible: true, primero: int(primero), anchos: make([]float64, len(arr))}
	for i, v := range arr {
		out.anchos[i] = comoReal(d.resolver(v))
	}
	if desc, ok := d.resolver(f["FontDescriptor"]).(pdfDict); ok {
		out.falta = comoReal(d.resolver(desc["MissingWidth"]))
	}
	return out
}

// --- Censura del flujo de contenido ------------------------------------------

// Extensión vertical de la tinta respecto de la línea base, en fracción del
// cuerpo: los glifos suben hasta las tildes de las mayúsculas y bajan hasta el
// rabo de la «p». Generoso por arriba y por abajo, pero por debajo del
// interlineado normal (1,2) para no arrastrar la línea de al lado.
const (
	subeElTexto = 0.90
	bajaElTexto = 0.25
)

// censor lleva el estado gráfico necesario para saber DÓNDE cae cada cosa.
type censor struct {
	doc      *PDFDoc
	recursos pdfDict
	nivel    *nivelCensura // recursos de este flujo y lo que se les cambia
	pres     *presupuestoCensura
	zonas    []AreaRedaccion
	diagonal float64 // de la página: cota superior de lo que puede avanzar una línea
	prof     int     // profundidad de recursión en XObjects de formulario
	cambios  int     // cuánto se ha quitado o reescrito: 0 = el flujo queda como estaba

	ctm  matrizPDF
	lw   float64 // grosor de línea vigente, que ensancha lo que toca un trazo
	pila []estadoGrafico
	cam  *caminoCensura // trazado en construcción

	tm, tlm                 matrizPDF
	tfs, tc, tw, th, tl, ts float64
	fuente                  anchosFuente
}

// censurarPagina reescribe el contenido de una página quitando el texto de las
// zonas y deja los recuadros negros encima.
func (d *PDFDoc) censurarPagina(p PDFPagina, contenido []byte, zonas []AreaRedaccion,
	pres *presupuestoCensura,
) ([]byte, *nivelCensura, error) {
	recursos, _ := d.resolver(p.dict["Resources"]).(pdfDict)
	nivel := nuevoNivelCensura(recursos)
	c := nuevoCensor(d, nivel, pres, zonas, math.Hypot(p.Ancho, p.Alto))
	limpio, err := c.reescribir(contenido)
	if err != nil {
		return nil, nil, err
	}

	// El original va envuelto en q/Q: si el flujo se quedó con un «q» sin cerrar
	// —pasa— los recuadros heredarían su transformación y se pintarían en
	// cualquier parte. Envolver cuesta dos bytes y quita el problema entero.
	var out bytes.Buffer
	out.WriteString("q\n")
	out.Write(limpio)
	out.WriteString("\nQ\n")
	// En gris de dispositivo (0 g) y no en RGB: es el mismo negro, y un PDF/A
	// con intención de salida CMYK admite el gris pero no el RGB (6.2.4.3), así
	// que censurar ya no le quita la marca.
	for _, z := range zonas {
		fmt.Fprintf(&out, "q 0 g %s %s %s %s re f Q\n",
			ftoa(z.X1), ftoa(z.Y1), ftoa(z.X2-z.X1), ftoa(z.Y2-z.Y1))
	}
	return out.Bytes(), nivel, nil
}

func nuevoCensor(d *PDFDoc, nivel *nivelCensura, pres *presupuestoCensura,
	zonas []AreaRedaccion, diagonal float64,
) *censor {
	return &censor{
		doc: d, recursos: nivel.recursos, nivel: nivel, pres: pres, zonas: zonas, diagonal: diagonal,
		ctm: matrizIdentidad, tm: matrizIdentidad, tlm: matrizIdentidad,
		tfs: 12, th: 1, lw: 1,
	}
}

// reescribir recorre el flujo copiando los bytes tal cual y saltándose, o
// sustituyendo, lo que cae en una zona: operadores de texto, trazados, imágenes
// en línea y objetos incrustados (`Do`).
//
// Se copia por RANGOS del original en vez de volver a serializar lo leído: un
// flujo de contenido lleva imágenes incrustadas y operadores que este código no
// interpreta, y reconstruirlo entero sería reescribir el documento del usuario
// para quitarle una línea.
func (c *censor) reescribir(contenido []byte) ([]byte, error) {
	var s salidaCensura
	var pila []pdfObj
	c.cam = nil

	lx := &pdfLexer{buf: contenido}
	for {
		lx.saltarEspacios()
		if lx.pos >= len(lx.buf) {
			break
		}
		if empiezaObjeto(lx.buf[lx.pos]) {
			pila = c.apilarOperando(lx, pila)
			continue
		}
		inicioOp := lx.pos
		op := lx.operador()
		if op == "" {
			lx.pos++
			continue
		}

		hecho, err := c.tratarEspecial(&s, contenido, lx, op, inicioOp, pila)
		if err == nil && !hecho {
			err = c.copiarOperador(&s, contenido, lx, op, pila)
		}
		if err != nil {
			return nil, err
		}
		pila = pila[:0]
	}
	s.out.Write(contenido[s.corte:])
	return s.out.Bytes(), nil
}

// salidaCensura es lo que se va escribiendo y hasta dónde del original está ya
// copiado.
type salidaCensura struct {
	out   bytes.Buffer
	corte int
}

// tratarEspecial atiende lo que sustituye o quita más que un operador: las
// imágenes en línea y el pintado de un trazado. Dice si lo ha consumido.
func (c *censor) tratarEspecial(s *salidaCensura, contenido []byte, lx *pdfLexer,
	op string, inicioOp int, pila []pdfObj,
) (bool, error) {
	if op == "BI" {
		reemplazo, cambiado, fin, err := c.imagenEnLinea(contenido, lx.pos)
		if err != nil {
			return true, err
		}
		s.out.Write(contenido[s.corte:inicioOp])
		if cambiado {
			s.out.WriteString(reemplazo)
		} else {
			s.out.Write(contenido[inicioOp:fin])
		}
		s.corte, lx.pos = fin, fin
		return true, nil
	}

	// Un operador de trazado se anota ANTES de copiar su rango: `desdeSalida` es
	// donde empieza el trazado en lo ya escrito, y es lo que se trunca si después
	// resulta que toca la zona.
	if esOperadorDeTrazado(op) {
		c.sumarAlTrazado(op, pila, s.out.Len())
	}
	if op == "W" || op == "W*" {
		if c.cam == nil {
			c.cam = nuevoCaminoCensura(s.out.Len())
		}
		c.cam.recorte = true
	}
	if !esPintadoDeTrazado(op) || c.cam == nil {
		return false, nil
	}
	accion := c.resolverTrazado(op)
	switch accion.tipo {
	case trazadoSustituir:
		s.out.Truncate(c.cam.desdeSalida)
		// El rango truncado traía el blanco que separaba al operador anterior: sin
		// uno nuevo, `g` y el número siguiente se pegan.
		s.out.WriteByte('\n')
		s.out.Write(c.cam.intermedios)
		s.out.WriteString(accion.texto)
	case trazadoSoloPintado:
		// El recorte se conserva y el pintado se anula: `n` cierra el trazado sin
		// dibujar nada.
		s.out.Write(contenido[s.corte:inicioOp])
		s.out.WriteString("n\n")
	case trazadoNada:
		return false, nil
	}
	s.corte = lx.pos
	c.cam = nil
	return true, nil
}

// copiarOperador copia el rango del operador tal cual o escribe su sustituto.
func (c *censor) copiarOperador(s *salidaCensura, contenido []byte, lx *pdfLexer,
	op string, pila []pdfObj,
) error {
	reemplazo, quitar, err := c.aplicar(op, pila)
	if err != nil {
		return err
	}
	antes := s.out.Len()
	if quitar {
		c.cambios++
		// Con blancos a los dos lados: lo quitado empezaba justo detrás del
		// operador anterior, y en un flujo compacto el sustituto se le pegaba
		// («TdT*»): un operador desconocido que se comía también el anterior.
		// Medido con Poppler sobre lo censurado (131 salidas así).
		s.out.WriteByte('\n')
		s.out.WriteString(reemplazo)
		s.out.WriteByte('\n')
	} else {
		s.out.Write(contenido[s.corte:lx.pos])
	}
	// Lo que se escribe en mitad de un trazado sin ser del trazado (un color, un
	// grosor) se guarda aparte: si el trazado se quita, eso tiene que seguir.
	if c.cam != nil && !esOperadorDeTrazado(op) && !esPintadoDeTrazado(op) && op != "W" && op != "W*" {
		c.cam.intermedios = append(c.cam.intermedios, s.out.Bytes()[antes:]...)
	}
	if esPintadoDeTrazado(op) {
		c.cam = nil
	}
	s.corte = lx.pos
	return nil
}

// aplicar ejecuta un operador sobre el estado. Devuelve, para los operadores que
// pintan texto en una zona, con qué hay que sustituirlos y que hay que quitarlos.
func (c *censor) aplicar(op string, pila []pdfObj) (string, bool, error) {
	n := len(pila)
	// num(1) es el último operando, num(2) el anterior… Contar desde el final y
	// no desde el principio es lo que hace que un operando huérfano de un
	// operador anterior no desplace la lectura de todos los demás.
	num := func(desdeElFinal int) float64 {
		i := n - desdeElFinal
		if i < 0 || i >= n {
			return 0
		}
		v := comoReal(pila[i])
		return v
	}

	// Los operadores que SOLO mueven el estado van aparte, agrupados como los
	// agrupa el estándar: estado gráfico (§8.4), estado de texto (§9.3) y
	// posición de texto (§9.4.2). Abajo queda únicamente lo que puede acabar
	// censurando algo, que es lo que hay que poder leer de un vistazo.
	if c.mueveEstado(op, n, num, pila) {
		return "", false, nil
	}

	switch op {
	case "Tj":
		if n >= 1 {
			s, _ := pila[n-1].(pdfString)
			return c.pintarTexto(s, 0, "")
		}
	case "'":
		// Salta de línea y pinta. Al quitarlo hay que dejar el salto: sin él, todo
		// lo que venga detrás se sube una línea.
		c.nuevaLinea(0, -c.tl)
		if n >= 1 {
			s, _ := pila[n-1].(pdfString)
			return c.pintarTexto(s, 0, "T* ")
		}
	case "\"":
		// aw ac (cadena) " — fija espaciado de palabra y de carácter, salta de
		// línea y pinta; los tres efectos hay que conservarlos.
		if n >= 3 {
			c.tw, c.tc = num(3), num(2)
		}
		c.nuevaLinea(0, -c.tl)
		if n >= 1 {
			s, _ := pila[n-1].(pdfString)
			return c.pintarTexto(s, 0, fmt.Sprintf("%s Tw %s Tc T* ", ftoa(c.tw), ftoa(c.tc)))
		}
	case "TJ":
		if n >= 1 {
			arr, ok := pila[n-1].(pdfArray)
			if !ok {
				break
			}
			texto, ajuste := c.textoDeArray(arr)
			return c.pintarTexto(texto, ajuste, "")
		}

	case "Do":
		if n >= 1 {
			if nombre, ok := pila[n-1].(pdfName); ok {
				return c.tratarXObject(nombre)
			}
		}
	case "sh":
		if n >= 1 {
			if nombre, ok := pila[n-1].(pdfName); ok {
				return "", false, c.comprobarDegradado(nombre)
			}
		}
	}
	return "", false, nil
}

// empiezaObjeto dice si un byte abre un operando —nombre, cadena, hexadecimal,
// array o número— en vez de un operador.
func empiezaObjeto(ch byte) bool {
	return ch == '/' || ch == '(' || ch == '<' || ch == '[' || ch == '+' || ch == '-' || ch == '.' ||
		(ch >= '0' && ch <= '9')
}

// apilarOperando lee un operando y lo apila. Lo que no se deja parsear no tumba
// el flujo: se avanza un byte y se sigue, porque un operando ilegible en mitad
// del contenido no justifica dejar de censurar el resto de la página.
//
// La pila se recorta a 64: los operadores se leen desde el FINAL, así que lo
// viejo no se usa y sin tope un flujo mal formado la haría crecer sin fin.
func (c *censor) apilarOperando(lx *pdfLexer, pila []pdfObj) []pdfObj {
	antes := lx.pos
	o, err := c.doc.parsearObjeto(lx)
	if err != nil || lx.pos == antes {
		lx.pos = antes + 1
		return pila
	}
	pila = append(pila, o)
	if len(pila) > 64 {
		pila = pila[len(pila)-64:]
	}
	return pila
}

// mueveEstado aplica los operadores que solo tocan el estado y dice si el
// operador era de alguno de esos tres grupos.
func (c *censor) mueveEstado(op string, n int, num func(int) float64, pila []pdfObj) bool {
	return c.estadoGrafico(op, n, num, pila) ||
		c.estadoDeTexto(op, n, num, pila) ||
		c.posicionDeTexto(op, n, num)
}

// textoDeArray junta las cadenas de un operando de TJ y suma los ajustes de
// posición que van entre ellas, que son los que separan las letras.
func (c *censor) textoDeArray(arr pdfArray) ([]byte, float64) {
	var texto []byte
	ajuste := 0.0
	for _, e := range arr {
		switch v := e.(type) {
		case pdfString:
			texto = append(texto, v...)
		case int64, float64:
			// Un ajuste positivo tira del lápiz hacia atrás (§9.4.3).
			ajuste -= comoReal(v) / 1000 * c.tfs * c.th
		}
	}
	return texto, ajuste
}

// estadoGrafico aplica los operadores de estado gráfico (§8.4) y dice si el
// operador era suyo. La matriz actual importa aquí porque es la que convierte
// las coordenadas del texto a las de la página, que son las de las zonas.
func (c *censor) estadoGrafico(op string, n int, num func(int) float64, pila []pdfObj) bool {
	switch op {
	case "q":
		// Tope: un flujo con millones de `q` sin cerrar no puede crecer sin fin.
		if len(c.pila) < maxPilaGrafica {
			c.pila = append(c.pila, estadoGrafico{c.ctm, c.lw})
		}
	case "Q":
		if k := len(c.pila); k > 0 {
			c.ctm, c.lw = c.pila[k-1].ctm, c.pila[k-1].lw
			c.pila = c.pila[:k-1]
		}
	case "w":
		c.lw = math.Abs(num(1))
	case "gs":
		// El grosor también puede venir de un estado gráfico con nombre.
		c.grosorDeEstado(pila)
	case "cm":
		if n >= 6 {
			c.ctm = matrizPDF{num(6), num(5), num(4), num(3), num(2), num(1)}.por(c.ctm)
		}
	default:
		return false
	}
	return true
}

// estadoDeTexto aplica los operadores de estado de texto (§9.3) y dice si el
// operador era suyo. Todos ellos entran en el cálculo de lo que ocupa una
// cadena, y por tanto en si cae dentro de una zona o no.
func (c *censor) estadoDeTexto(op string, n int, num func(int) float64, pila []pdfObj) bool {
	switch op {
	case "BT":
		c.tm, c.tlm = matrizIdentidad, matrizIdentidad
	case "Tf":
		if n >= 2 {
			c.tfs = num(1)
			nombre, _ := pila[n-2].(pdfName)
			c.fuente = c.doc.anchosDeFuente(c.recursos, nombre)
		}
	case "Tc":
		c.tc = num(1)
	case "Tw":
		c.tw = num(1)
	case "Tz":
		c.th = num(1) / 100
	case "TL":
		c.tl = num(1)
	case "Ts":
		c.ts = num(1)
	default:
		return false
	}
	return true
}

// posicionDeTexto aplica los operadores que mueven el lápiz (§9.4.2) y dice si
// el operador era suyo. Los que ADEMÁS pintan —«'» y «"»— no están aquí: se
// quedan con el pintado, porque de ellos puede salir una censura.
func (c *censor) posicionDeTexto(op string, n int, num func(int) float64) bool {
	switch op {
	case "Td":
		c.nuevaLinea(num(2), num(1))
	case "TD":
		c.tl = -num(1)
		c.nuevaLinea(num(2), num(1))
	case "Tm":
		if n >= 6 {
			c.tlm = matrizPDF{num(6), num(5), num(4), num(3), num(2), num(1)}
			c.tm = c.tlm
		}
	case "T*":
		c.nuevaLinea(0, -c.tl)
	default:
		return false
	}
	return true
}

// nuevaLinea mueve el origen de línea y sitúa ahí la matriz de texto.
func (c *censor) nuevaLinea(tx, ty float64) {
	c.tlm = matrizPDF{1, 0, 0, 1, tx, ty}.por(c.tlm)
	c.tm = c.tlm
}

// pintarTexto decide si una cadena cae en zona censurada y, de paso, adelanta la
// matriz de texto lo que ocupa.
//
// `prefijo` es lo que el operador hacía ADEMÁS de pintar (saltar de línea, fijar
// espaciados) y que hay que conservar aunque el texto se vaya.
func (c *censor) pintarTexto(s []byte, ajuste float64, prefijo string) (string, bool, error) {
	avance, exacto := c.avanceDe(s)
	avance += ajuste

	m := c.tm.por(c.ctm)
	if !exacto {
		// Sin anchos no se sabe dónde acaba la cadena. Se le da de margen la
		// diagonal de la página —ninguna línea visible avanza más— y NO se
		// adelanta la matriz: así todo lo que venga detrás en la misma línea
		// sigue midiéndose desde este punto y tampoco se escapa.
		if escala := math.Hypot(m[0], m[1]); escala > 1e-9 {
			avance = c.diagonal / escala
		} else {
			avance = 0
		}
	}

	tam := math.Abs(c.tfs)
	x1, x2 := math.Min(0, avance), math.Max(0, avance)
	caja := cajaDeRectangulo(m, x1, c.ts-bajaElTexto*tam, x2, c.ts+subeElTexto*tam)
	toca := caja.tocaAlguna(c.zonas)

	if exacto {
		c.tm = matrizPDF{1, 0, 0, 1, avance, 0}.por(c.tm)
	}
	if !toca {
		return "", false, nil
	}

	var b strings.Builder
	b.WriteString(prefijo)
	// Si se sabe cuánto ocupaba, se deja un ajuste de TJ del mismo tamaño: mueve
	// el lápiz sin pintar nada y lo que siga en la línea se queda donde estaba.
	// Un TJ solo con número no dibuja: es exactamente un hueco.
	if exacto && tam > 0 && c.th != 0 {
		fmt.Fprintf(&b, "[%s] TJ", ajusteTJ(-avance*1000/(c.tfs*c.th)))
	}
	return b.String(), true, nil
}

// ajusteTJ escribe un desplazamiento de TJ en números de valor absoluto menor
// que 32767, el tope de un real en PDF/A-1 (anexo C de ISO 32000-1): los
// números seguidos de un TJ se suman. Una línea larga censurada de una vez
// dejaba un -39990 y el PDF/A-1 dejaba de cumplir lo que declaraba (cinco BOE
// del corpus).
func ajusteTJ(v float64) string {
	const tope = 32000.0
	var b strings.Builder
	// ponytail: 64 trozos cubren dos millones de milésimas de em; más allá el
	// resto va en un número, que es lo que se hacía antes.
	for n := 0; n < 64 && math.Abs(v) > tope; n++ {
		b.WriteString(ftoa(math.Copysign(tope, v)) + " ")
		v -= math.Copysign(tope, v)
	}
	b.WriteString(ftoa(v))
	return b.String()
}

// avanceDe suma lo que ocupa la cadena en el espacio de la matriz de texto.
func (c *censor) avanceDe(s []byte) (float64, bool) {
	if !c.fuente.medible {
		return 0, false
	}
	total := 0.0
	for _, b := range s {
		ancho := c.fuente.falta
		if i := int(b) - c.fuente.primero; i >= 0 && i < len(c.fuente.anchos) {
			ancho = c.fuente.anchos[i]
		}
		paso := ancho/1000*c.tfs + c.tc
		if b == ' ' {
			paso += c.tw // el espaciado de palabra solo aplica al código 32
		}
		total += paso * c.th
	}
	return total, true
}

// finDeImagenIncrustada localiza el final de un «BI … ID <bytes> EI» y devuelve
// la posición justo detrás.
//
// Si no se encuentra hay que fallar y no seguir adivinando: los datos crudos de
// una imagen contienen cualquier secuencia de bytes, así que un análisis
// desalineado se traga el resto del flujo sin quejarse y con él el texto que
// había que censurar.
func finDeImagenIncrustada(buf []byte, desde int) (int, error) {
	i := desde
	for {
		if i+1 >= len(buf) {
			return 0, fmt.Errorf("%w: hay una imagen incrustada sin datos (falta ID)",
				ErrRedaccionNoGarantizada)
		}
		// El operador ID va aislado y le sigue UN solo byte blanco (§8.9.7).
		if buf[i] == 'I' && buf[i+1] == 'D' && (i == 0 || esEspacioPDF(buf[i-1])) &&
			(i+2 >= len(buf) || esEspacioPDF(buf[i+2])) {
			i += 3
			break
		}
		i++
	}
	for ; i+1 < len(buf); i++ {
		if buf[i] != 'E' || buf[i+1] != 'I' || !esEspacioPDF(buf[i-1]) {
			continue
		}
		fin := i + 2
		if fin >= len(buf) || esEspacioPDF(buf[fin]) || esDelimitadorPDF(buf[fin]) {
			return fin, nil
		}
	}
	return 0, fmt.Errorf("%w: hay una imagen incrustada sin cerrar (falta EI)",
		ErrRedaccionNoGarantizada)
}
