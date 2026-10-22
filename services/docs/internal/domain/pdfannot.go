// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Anotaciones de PDF (ola P2 de Sytue PDF): resaltar, subrayar, tachar,
// comentar, dibujar a mano alzada, formas, flechas, texto libre, sellos y
// medición sobre planos.
//
// LAS ANOTACIONES NO REESCRIBEN EL DOCUMENTO. Van como objetos aparte en el
// /Annots de la página y el contenido original no se toca. Eso es lo que permite
// borrar un resaltado sin dejar marca, comparar con el original y no degradar el
// fichero en cada guardado. La única excepción deliberada de todo el producto es
// la redacción, que sí destruye —o no sería redacción—.
//
// CADA ANOTACIÓN LLEVA SU APARIENCIA. Se podría escribir solo el tipo y las
// coordenadas y dejar que el lector la dibuje, y así lo hacen muchas
// herramientas; el problema es que entonces cada lector la pinta a su manera y
// algunos —los de los móviles, y los navegadores— no la pintan en absoluto. Con
// el flujo de apariencia (/AP /N) escrito, lo que se ve aquí es lo que se ve en
// cualquier sitio, incluido al imprimir.
package domain

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pdffuente"
)

// TipoAnotacion es la clase de anotación, con los nombres del producto y no los
// de la especificación: quien llama piensa en «resaltar», no en «/Highlight».
type TipoAnotacion string

const (
	AnotResaltado  TipoAnotacion = "resaltado"
	AnotSubrayado  TipoAnotacion = "subrayado"
	AnotTachado    TipoAnotacion = "tachado"
	AnotOndulado   TipoAnotacion = "ondulado"
	AnotNota       TipoAnotacion = "nota"
	AnotTinta      TipoAnotacion = "tinta"
	AnotRectangulo TipoAnotacion = "rectangulo"
	AnotElipse     TipoAnotacion = "elipse"
	AnotLinea      TipoAnotacion = "linea"
	AnotFlecha     TipoAnotacion = "flecha"
	AnotPoligono   TipoAnotacion = "poligono"
	AnotTextoLibre TipoAnotacion = "texto"
	AnotSello      TipoAnotacion = "sello"
	AnotMedida     TipoAnotacion = "medida"
)

// Anotacion describe una marca sobre una página. Las coordenadas van en puntos
// y en el sistema del PDF (origen abajo-izquierda), que es el mismo que devuelve
// el render, así que la interfaz no tiene que convertir nada.
type Anotacion struct {
	Pagina int           `json:"pagina"` // 0-based
	Tipo   TipoAnotacion `json:"tipo"`
	// Rect es la caja de la anotación [x0 y0 x1 y1]. Para las de texto se calcula
	// sola a partir de los cuadros si no se da.
	Rect [4]float64 `json:"rect"`
	// Cuadros son las cajas de cada línea de texto marcada. Un resaltado que cruza
	// tres líneas son tres cuadros, no un rectángulo grande que taparía el margen.
	Cuadros [][4]float64 `json:"cuadros,omitempty"`
	// Trazos son los trazos del lápiz: cada uno una polilínea de puntos.
	Trazos [][][2]float64 `json:"trazos,omitempty"`
	// Puntos es la geometría de línea, flecha, polígono y medición.
	Puntos [][2]float64 `json:"puntos,omitempty"`

	Color         [3]float64  `json:"color"`
	ColorInterior *[3]float64 `json:"color_interior,omitempty"`
	Opacidad      float64     `json:"opacidad"`
	Grosor        float64     `json:"grosor"`

	Texto  string  `json:"texto,omitempty"`
	Autor  string  `json:"autor,omitempty"`
	Tamano float64 `json:"tamano,omitempty"`
	// Etiqueta es lo que se escribe sobre una medición («12,4 m») o en un sello
	// («APROBADO»).
	Etiqueta string `json:"etiqueta,omitempty"`
	// Fecha, si es cero, la pone quien escribe.
	Fecha time.Time `json:"fecha,omitzero"`
	// letra, si no es nil, es la letra incrustada con la que se escribe el texto.
	letra *letraAnot
}

// maxAnotacionesPorDocumento acota lo que una sola llamada puede añadir. Sin
// tope, una petición con un millón de anotaciones ata el servidor con un cuerpo
// de pocos megas.
const maxAnotacionesPorDocumento = 20000

// AnadirAnotacionesPDF escribe una copia del documento con las anotaciones
// puestas. Las que ya tuviera se conservan.
func AnadirAnotacionesPDF(doc *PDFDoc, anots []Anotacion) ([]byte, error) {
	if len(anots) == 0 {
		return nil, fmt.Errorf("pdf: no se ha indicado ninguna anotación")
	}
	if len(anots) > maxAnotacionesPorDocumento {
		return nil, fmt.Errorf("pdf: demasiadas anotaciones de una vez (máximo %d)", maxAnotacionesPorDocumento)
	}

	porPagina := map[int][]Anotacion{}
	for _, a := range anots {
		if a.Pagina < 0 || a.Pagina >= doc.NumPaginas() {
			return nil, fmt.Errorf("pdf: la anotación apunta a la página %d y el documento tiene %d",
				a.Pagina+1, doc.NumPaginas())
		}
		porPagina[a.Pagina] = append(porPagina[a.Pagina], a)
	}

	if doc.conservaFirmas() {
		return anexarAnotaciones(doc, porPagina)
	}

	e := NuevoEnsambladorPDF()
	for i := 0; i < doc.NumPaginas(); i++ {
		if err := e.AnadirPagina(doc, i, 0); err != nil {
			return nil, err
		}
		if len(porPagina[i]) == 0 {
			continue
		}
		pp := e.ultimaPendiente()
		refs := make(pdfArray, 0, len(porPagina[i]))
		hayTransp := false
		for _, a := range porPagina[i] {
			if an := a.normalizada(); an.Opacidad < 1 || an.Tipo == AnotResaltado {
				hayTransp = true
			}
			refs = append(refs, pdfRef{Num: e.escribirAnotacion(a)})
		}
		if hayTransp && pp.dict["Group"] == nil {
			if dec, ok := doc.declaraPDFA(); ok && dec.parte != "1" && componentesIntencion(doc) == 0 {
				pp.dict["Group"] = grupoTransparenciaRGB()
			}
		}
		// Se AÑADEN a las que ya había: perder las anteriores al comentar sería
		// destruir el trabajo de otra persona sin decirlo.
		switch ya := pp.dict["Annots"].(type) {
		case pdfArray:
			pp.dict["Annots"] = append(append(pdfArray{}, ya...), refs...)
		default:
			pp.dict["Annots"] = refs
		}
		e.reserializarPendiente(pp)
	}
	return e.Bytes()
}

// anexarAnotaciones pone las anotaciones en una actualización incremental: el
// documento firmado no se reescribe y sus firmas siguen validando (pdfanexo.go).
// Por página cambia solo su diccionario, con el /Annots ampliado.
func anexarAnotaciones(doc *PDFDoc, porPagina map[int][]Anotacion) ([]byte, error) {
	if err := admiteAnexo(doc, anexoAnotaciones); err != nil {
		return nil, err
	}
	e := nuevoAnexo(doc)
	cambiados := map[int]objetoSalida{}
	for i := 0; i < doc.NumPaginas(); i++ {
		if len(porPagina[i]) == 0 {
			continue
		}
		num := doc.paginas[i].num
		nodo, ok := doc.resolver(pdfRef{Num: num}).(pdfDict)
		if num == 0 || !ok {
			return nil, fmt.Errorf("%w: no se sabe qué objeto es la página %d", ErrInvalidaFirmas, i+1)
		}
		pagina := copiaLlana(nodo)
		annots := arrayResuelto(doc, nodo["Annots"])
		hayTransp := false
		for _, a := range porPagina[i] {
			if an := a.normalizada(); an.Opacidad < 1 || an.Tipo == AnotResaltado {
				hayTransp = true
			}
			annots = append(annots, pdfRef{Num: e.escribirAnotacion(a)})
		}
		if hayTransp && pagina["Group"] == nil {
			if dec, ok := doc.declaraPDFA(); ok && dec.parte != "1" && componentesIntencion(doc) == 0 {
				pagina["Group"] = grupoTransparenciaRGB()
			}
		}
		pagina["Annots"] = annots
		cambiados[num] = objetoSalida{valor: pagina}
	}
	return anexar(doc, e, cambiados)
}

// escribirAnotacion crea el objeto de la anotación y el de su apariencia, y
// devuelve el número del primero.
func (e *PDFEnsamblador) escribirAnotacion(a Anotacion) int {
	a = a.normalizada()
	num := e.reservar()

	dict := pdfDict{
		"Type":    pdfName("Annot"),
		"Subtype": pdfName(subtipoPDF(a.Tipo)),
		"Rect":    rectPDF(a.Rect),
		// Bit 3 (valor 4) = imprimir. Sin él, la anotación se ve en pantalla y
		// DESAPARECE al imprimir, que es la queja clásica de quien revisa en papel.
		"F": int64(4),
		"M": pdfString(fechaPDF(a.Fecha)),
		"C": colorPDF(a.Color),
	}
	if a.Opacidad > 0 && a.Opacidad < 1 {
		dict["CA"] = a.Opacidad
	}
	if a.Texto != "" {
		dict["Contents"] = cadenaUTF16(a.Texto)
	}
	if a.Autor != "" {
		dict["T"] = cadenaUTF16(a.Autor)
	}
	if a.ColorInterior != nil {
		dict["IC"] = colorPDF(*a.ColorInterior)
	}
	a.completarDiccionario(dict)

	if a.Tipo == AnotTextoLibre || a.Tipo == AnotSello {
		a.letra = nuevaLetraAnot()
	}
	contenido, recursos := a.apariencia()
	if a.letra != nil && contenido != nil {
		num, err := 0, errors.New("sin texto")
		if !a.letra.sub.Vacio() {
			num, err = a.letra.sub.Escribir(escritorDeFuente{e})
		}
		if err == nil {
			recursos["Font"] = pdfDict{"SytueAnotF": pdfRef{Num: num}}
		} else {
			a.letra = nil // sin la Liberation, la Helvetica de siempre
			contenido, recursos = a.apariencia()
		}
	}
	if contenido != nil {
		numAP := e.reservar()
		apDict := pdfDict{
			"Type":     pdfName("XObject"),
			"Subtype":  pdfName("Form"),
			"BBox":     rectPDF(a.Rect),
			"FormType": int64(1),
		}
		if recursos != nil {
			apDict["Resources"] = recursos
		}
		e.cuerpos[numAP-1] = objetoSalida{valor: apDict, flujo: contenido, esFlujo: true}
		dict["AP"] = pdfDict{"N": pdfRef{Num: numAP}}
	}

	e.cuerpos[num-1] = objetoSalida{valor: dict}
	return num
}

// normalizada rellena los huecos: color por defecto, grosor mínimo y una caja
// que contenga de verdad lo que se va a dibujar.
func (a Anotacion) normalizada() Anotacion {
	if a.Grosor <= 0 {
		a.Grosor = 2
	}
	if a.Opacidad <= 0 || a.Opacidad > 1 {
		a.Opacidad = 1
	}
	if a.Tamano <= 0 {
		a.Tamano = 11
	}
	if a.Fecha.IsZero() {
		a.Fecha = time.Now().UTC()
	}
	if a.Color == [3]float64{} {
		a.Color = [3]float64{0.98, 0.78, 0.19} // el ámbar de la casa
	}

	// La caja se calcula de la geometría cuando no viene o viene degenerada: una
	// anotación con /Rect vacío no la enseña ningún lector, y es el error más
	// fácil de cometer desde el cliente.
	if a.Rect[2]-a.Rect[0] > 0.01 && a.Rect[3]-a.Rect[1] > 0.01 {
		return a
	}
	var xs, ys []float64
	for _, c := range a.Cuadros {
		xs = append(xs, c[0], c[2])
		ys = append(ys, c[1], c[3])
	}
	for _, tr := range a.Trazos {
		for _, p := range tr {
			xs = append(xs, p[0])
			ys = append(ys, p[1])
		}
	}
	for _, p := range a.Puntos {
		xs = append(xs, p[0])
		ys = append(ys, p[1])
	}
	if len(xs) == 0 {
		return a
	}
	// El margen deja sitio a la mitad del trazo, a la punta de la flecha y al
	// antialiasing; sin él, un dibujo pegado al borde sale recortado.
	margen := a.Grosor*2 + 4
	a.Rect = [4]float64{minimo(xs) - margen, minimo(ys) - margen, maximo(xs) + margen, maximo(ys) + margen}
	return a
}

// completarDiccionario añade las entradas propias de cada tipo.
func (a Anotacion) completarDiccionario(dict pdfDict) {
	switch a.Tipo {
	case AnotResaltado, AnotSubrayado, AnotTachado, AnotOndulado:
		dict["QuadPoints"] = cuadrosPDF(a.cuadrosEfectivos())
	case AnotNota:
		dict["Name"] = pdfName("Comment")
		dict["Open"] = false
	case AnotTinta:
		lista := make(pdfArray, 0, len(a.Trazos))
		for _, tr := range a.Trazos {
			pts := make(pdfArray, 0, 2*len(tr))
			for _, p := range tr {
				pts = append(pts, p[0], p[1])
			}
			lista = append(lista, pts)
		}
		dict["InkList"] = lista
		dict["BS"] = pdfDict{"W": a.Grosor, "S": pdfName("S")}
	case AnotLinea, AnotFlecha, AnotMedida:
		if len(a.Puntos) >= 2 {
			p0, p1 := a.Puntos[0], a.Puntos[len(a.Puntos)-1]
			dict["L"] = pdfArray{p0[0], p0[1], p1[0], p1[1]}
		}
		dict["BS"] = pdfDict{"W": a.Grosor, "S": pdfName("S")}
		if a.Tipo == AnotFlecha {
			dict["LE"] = pdfArray{pdfName("None"), pdfName("ClosedArrow")}
		}
		if a.Tipo == AnotMedida {
			// /IT lo marca como cota: un lector que lo entienda ofrece medir con
			// ella en vez de tratarla como una raya cualquiera.
			dict["IT"] = pdfName("LineDimension")
			dict["Cap"] = true
			if a.Etiqueta != "" {
				dict["Contents"] = cadenaUTF16(a.Etiqueta)
			}
		}
	case AnotPoligono:
		pts := make(pdfArray, 0, 2*len(a.Puntos))
		for _, p := range a.Puntos {
			pts = append(pts, p[0], p[1])
		}
		dict["Vertices"] = pts
		dict["BS"] = pdfDict{"W": a.Grosor, "S": pdfName("S")}
	case AnotRectangulo, AnotElipse:
		dict["BS"] = pdfDict{"W": a.Grosor, "S": pdfName("S")}
	case AnotTextoLibre:
		dict["DA"] = pdfString(fmt.Sprintf("%s %s %s rg /SytueAnotF %s Tf",
			numPDF(a.Color[0]), numPDF(a.Color[1]), numPDF(a.Color[2]), numPDF(a.Tamano)))
		dict["Q"] = int64(0) // alineado a la izquierda
	case AnotSello:
		dict["Name"] = pdfName("Draft")
	}
}

// subtipoPDF traduce al nombre de la especificación.
func subtipoPDF(t TipoAnotacion) string {
	switch t {
	case AnotResaltado:
		return "Highlight"
	case AnotSubrayado:
		return "Underline"
	case AnotTachado:
		return "StrikeOut"
	case AnotOndulado:
		return "Squiggly"
	case AnotNota:
		return "Text"
	case AnotTinta:
		return "Ink"
	case AnotRectangulo:
		return "Square"
	case AnotElipse:
		return "Circle"
	case AnotLinea, AnotFlecha, AnotMedida:
		return "Line"
	case AnotPoligono:
		return "Polygon"
	case AnotTextoLibre:
		return "FreeText"
	case AnotSello:
		return "Stamp"
	default:
		return "Square"
	}
}

func (a Anotacion) cuadrosEfectivos() [][4]float64 {
	if len(a.Cuadros) > 0 {
		return a.Cuadros
	}
	return [][4]float64{a.Rect}
}

// --- Apariencias -------------------------------------------------------------

// apariencia devuelve el flujo de contenido que dibuja la anotación y los
// recursos que necesita.
func (a Anotacion) apariencia() ([]byte, pdfDict) {
	var b strings.Builder
	recursos := pdfDict{"ColorSpace": pdfDict{"DefaultRGB": calRGBDefecto()}}

	// La opacidad no existe en el color de un PDF: hace falta un /ExtGState, y sin
	// él un resaltado TAPA el texto en vez de velarlo.
	if a.Opacidad < 1 || a.Tipo == AnotResaltado {
		gs := pdfDict{"Type": pdfName("ExtGState"), "ca": a.Opacidad, "CA": a.Opacidad}
		if a.Tipo == AnotResaltado {
			// Multiplicar es lo que hace que se lea el texto POR DEBAJO del color.
			gs["BM"] = pdfName("Multiply")
		}
		recursos["ExtGState"] = pdfDict{"SytueAnotGS": gs}
		b.WriteString("/SytueAnotGS gs\n")
	}

	r, g, bl := a.Color[0], a.Color[1], a.Color[2]
	fmt.Fprintf(&b, "%s %s %s RG %s %s %s rg %s w 1 J 1 j\n",
		numPDF(r), numPDF(g), numPDF(bl), numPDF(r), numPDF(g), numPDF(bl), numPDF(a.Grosor))

	switch a.Tipo {
	case AnotResaltado:
		for _, c := range a.cuadrosEfectivos() {
			fmt.Fprintf(&b, "%s %s %s %s re f\n",
				numPDF(c[0]), numPDF(c[1]), numPDF(c[2]-c[0]), numPDF(c[3]-c[1]))
		}
	case AnotSubrayado:
		a.lineasSobreCuadros(&b, 0.06, false)
	case AnotTachado:
		a.lineasSobreCuadros(&b, 0.45, false)
	case AnotOndulado:
		a.lineasSobreCuadros(&b, 0.04, true)
	case AnotTinta:
		for _, tr := range a.Trazos {
			dibujarPolilinea(&b, tr, false)
			b.WriteString("S\n")
		}
	case AnotRectangulo:
		a.figuraConRelleno(&b, func() {
			d := a.Grosor / 2
			fmt.Fprintf(&b, "%s %s %s %s re\n", numPDF(a.Rect[0]+d+2), numPDF(a.Rect[1]+d+2),
				numPDF(a.Rect[2]-a.Rect[0]-2*(d+2)), numPDF(a.Rect[3]-a.Rect[1]-2*(d+2)))
		})
	case AnotElipse:
		a.figuraConRelleno(&b, func() { dibujarElipse(&b, a.Rect, a.Grosor/2+2) })
	case AnotLinea, AnotFlecha, AnotMedida:
		a.dibujarLinea(&b)
	case AnotPoligono:
		dibujarPolilinea(&b, a.Puntos, true)
		if a.ColorInterior != nil {
			c := *a.ColorInterior
			fmt.Fprintf(&b, "%s %s %s rg B\n", numPDF(c[0]), numPDF(c[1]), numPDF(c[2]))
		} else {
			b.WriteString("S\n")
		}
	case AnotNota:
		dibujarChincheta(&b, a.Rect)
	case AnotSello:
		a.dibujarSello(&b, recursos)
	case AnotTextoLibre:
		a.dibujarTextoLibre(&b, recursos)
	default:
		return nil, nil
	}

	if len(recursos) == 0 {
		return []byte(b.String()), nil
	}
	return []byte(b.String()), recursos
}

// lineasSobreCuadros pinta una raya por cuadro a la altura relativa dada. El
// ondulado dibuja dientes en vez de una recta.
func (a Anotacion) lineasSobreCuadros(b *strings.Builder, altura float64, ondulado bool) {
	for _, c := range a.cuadrosEfectivos() {
		y := c[1] + (c[3]-c[1])*altura
		if !ondulado {
			fmt.Fprintf(b, "%s %s m %s %s l S\n", numPDF(c[0]), numPDF(y), numPDF(c[2]), numPDF(y))
			continue
		}
		paso := 3.0
		fmt.Fprintf(b, "%s %s m", numPDF(c[0]), numPDF(y))
		arriba := true
		for x := c[0] + paso; x < c[2]; x += paso {
			alto := y
			if arriba {
				alto = y + 2
			}
			fmt.Fprintf(b, " %s %s l", numPDF(x), numPDF(alto))
			arriba = !arriba
		}
		b.WriteString(" S\n")
	}
}

// figuraConRelleno traza la figura y decide si además se rellena.
func (a Anotacion) figuraConRelleno(b *strings.Builder, trazar func()) {
	trazar()
	if a.ColorInterior == nil {
		b.WriteString("S\n")
		return
	}
	c := *a.ColorInterior
	fmt.Fprintf(b, "%s %s %s rg B\n", numPDF(c[0]), numPDF(c[1]), numPDF(c[2]))
}

func (a Anotacion) dibujarLinea(b *strings.Builder) {
	if len(a.Puntos) < 2 {
		return
	}
	p0, p1 := a.Puntos[0], a.Puntos[len(a.Puntos)-1]
	fmt.Fprintf(b, "%s %s m %s %s l S\n", numPDF(p0[0]), numPDF(p0[1]), numPDF(p1[0]), numPDF(p1[1]))

	if a.Tipo == AnotFlecha {
		dibujarPuntaDeFlecha(b, p0, p1, a.Grosor*4+6)
	}
	if a.Tipo == AnotMedida {
		// Topes perpendiculares en los extremos: sin ellos una cota parece una
		// raya y no se sabe dónde empieza y acaba lo medido.
		dibujarTope(b, p0, p1, a.Grosor*3+4)
		dibujarTope(b, p1, p0, a.Grosor*3+4)
		if a.Etiqueta != "" {
			mx, my := (p0[0]+p1[0])/2, (p0[1]+p1[1])/2
			fmt.Fprintf(b, "BT /SytueAnotF %s Tf 1 1 1 rg %s %s Td (%s) Tj ET\n",
				numPDF(a.Tamano), numPDF(mx+4), numPDF(my+4), escaparCadenaContenido(a.Etiqueta))
		}
	}
}

// dibujarPuntaDeFlecha pinta el triángulo del extremo `hasta`.
func dibujarPuntaDeFlecha(b *strings.Builder, desde, hasta [2]float64, tam float64) {
	dx, dy := hasta[0]-desde[0], hasta[1]-desde[1]
	largo := math.Hypot(dx, dy)
	if largo < 1e-6 {
		return
	}
	ux, uy := dx/largo, dy/largo
	// Perpendicular unitaria, para abrir la punta a ambos lados.
	px, py := -uy, ux
	bx, by := hasta[0]-ux*tam, hasta[1]-uy*tam
	fmt.Fprintf(b, "%s %s m %s %s l %s %s l f\n",
		numPDF(hasta[0]), numPDF(hasta[1]),
		numPDF(bx+px*tam*0.4), numPDF(by+py*tam*0.4),
		numPDF(bx-px*tam*0.4), numPDF(by-py*tam*0.4))
}

func dibujarTope(b *strings.Builder, en, hacia [2]float64, tam float64) {
	dx, dy := hacia[0]-en[0], hacia[1]-en[1]
	largo := math.Hypot(dx, dy)
	if largo < 1e-6 {
		return
	}
	px, py := -dy/largo, dx/largo
	fmt.Fprintf(b, "%s %s m %s %s l S\n",
		numPDF(en[0]+px*tam/2), numPDF(en[1]+py*tam/2),
		numPDF(en[0]-px*tam/2), numPDF(en[1]-py*tam/2))
}

func dibujarPolilinea(b *strings.Builder, pts [][2]float64, cerrar bool) {
	if len(pts) == 0 {
		return
	}
	fmt.Fprintf(b, "%s %s m", numPDF(pts[0][0]), numPDF(pts[0][1]))
	for _, p := range pts[1:] {
		fmt.Fprintf(b, " %s %s l", numPDF(p[0]), numPDF(p[1]))
	}
	if cerrar {
		b.WriteString(" h")
	}
	b.WriteString("\n")
}

// dibujarElipse aproxima el círculo con cuatro Bézier. La constante 0,5523 es la
// que hace que el error máximo frente al arco real quede por debajo de una
// diezmilésima del radio: a ojo, y también con lupa, es un círculo.
func dibujarElipse(b *strings.Builder, r [4]float64, margen float64) {
	x0, y0, x1, y1 := r[0]+margen, r[1]+margen, r[2]-margen, r[3]-margen
	cx, cy := (x0+x1)/2, (y0+y1)/2
	rx, ry := (x1-x0)/2, (y1-y0)/2
	if rx <= 0 || ry <= 0 {
		return
	}
	const k = 0.5523
	fmt.Fprintf(b, "%s %s m\n", numPDF(cx+rx), numPDF(cy))
	fmt.Fprintf(b, "%s %s %s %s %s %s c\n", numPDF(cx+rx), numPDF(cy+ry*k), numPDF(cx+rx*k), numPDF(cy+ry), numPDF(cx), numPDF(cy+ry))
	fmt.Fprintf(b, "%s %s %s %s %s %s c\n", numPDF(cx-rx*k), numPDF(cy+ry), numPDF(cx-rx), numPDF(cy+ry*k), numPDF(cx-rx), numPDF(cy))
	fmt.Fprintf(b, "%s %s %s %s %s %s c\n", numPDF(cx-rx), numPDF(cy-ry*k), numPDF(cx-rx*k), numPDF(cy-ry), numPDF(cx), numPDF(cy-ry))
	fmt.Fprintf(b, "%s %s %s %s %s %s c\n", numPDF(cx+rx*k), numPDF(cy-ry), numPDF(cx+rx), numPDF(cy-ry*k), numPDF(cx+rx), numPDF(cy))
}

// dibujarChincheta pinta el globo de un comentario: es un icono, no texto, para
// que se vea igual en cualquier lector y a cualquier zoom.
func dibujarChincheta(b *strings.Builder, r [4]float64) {
	x0, y1 := r[0]+2, r[3]-2
	ancho, alto := math.Min(r[2]-r[0]-4, 20), math.Min(r[3]-r[1]-4, 18)
	if ancho < 6 || alto < 6 {
		return
	}
	cuerpo := alto * 0.72
	fmt.Fprintf(b, "%s %s %s %s re f\n", numPDF(x0), numPDF(y1-cuerpo), numPDF(ancho), numPDF(cuerpo))
	// El pico del globo.
	fmt.Fprintf(b, "%s %s m %s %s l %s %s l f\n",
		numPDF(x0+ancho*0.2), numPDF(y1-cuerpo),
		numPDF(x0+ancho*0.2), numPDF(y1-alto),
		numPDF(x0+ancho*0.5), numPDF(y1-cuerpo))
	// Tres rayas blancas: se lee como «hay texto dentro» sin escribir ninguno.
	b.WriteString("1 1 1 rg\n")
	for i := 1; i <= 3; i++ {
		y := y1 - cuerpo*float64(i)/4
		fmt.Fprintf(b, "%s %s %s %s re f\n",
			numPDF(x0+ancho*0.15), numPDF(y), numPDF(ancho*0.7), numPDF(math.Max(alto*0.06, 0.7)))
	}
}

func (a Anotacion) dibujarSello(b *strings.Builder, recursos pdfDict) {
	texto := a.Etiqueta
	if texto == "" {
		texto = a.Texto
	}
	if texto == "" {
		texto = "APROBADO"
	}
	x0, y0, x1, y1 := a.Rect[0]+2, a.Rect[1]+2, a.Rect[2]-2, a.Rect[3]-2
	fmt.Fprintf(b, "%s w %s %s %s %s re S\n", numPDF(math.Max(a.Grosor, 2)),
		numPDF(x0), numPDF(y0), numPDF(x1-x0), numPDF(y1-y0))

	// El tamaño se ajusta al ancho disponible: un sello con el texto saliéndose de
	// su marco es peor que uno pequeño.
	tam := math.Min((y1-y0)*0.55, (x1-x0)/(float64(len(texto))*0.62))
	if tam < 4 {
		return
	}
	anchoTexto := float64(len(texto)) * tam * 0.62
	fmt.Fprintf(b, "BT /SytueAnotF %s Tf %s %s Td %s Tj ET\n",
		numPDF(tam), numPDF((x0+x1)/2-anchoTexto/2), numPDF((y0+y1)/2-tam*0.35), a.tj(texto))
	recursos["Font"] = fuenteDeAnotacion()
}

func (a Anotacion) dibujarTextoLibre(b *strings.Builder, recursos pdfDict) {
	if a.ColorInterior != nil {
		c := *a.ColorInterior
		fmt.Fprintf(b, "%s %s %s rg %s %s %s %s re f\n",
			numPDF(c[0]), numPDF(c[1]), numPDF(c[2]),
			numPDF(a.Rect[0]), numPDF(a.Rect[1]),
			numPDF(a.Rect[2]-a.Rect[0]), numPDF(a.Rect[3]-a.Rect[1]))
		fmt.Fprintf(b, "%s %s %s rg\n", numPDF(a.Color[0]), numPDF(a.Color[1]), numPDF(a.Color[2]))
	}
	ancho := a.Rect[2] - a.Rect[0] - 8
	y := a.Rect[3] - 4 - a.Tamano
	b.WriteString("BT /SytueAnotF " + numPDF(a.Tamano) + " Tf " + numPDF(a.Tamano*1.25) + " TL\n")
	fmt.Fprintf(b, "%s %s Td\n", numPDF(a.Rect[0]+4), numPDF(y))
	for i, linea := range partirEnLineas(a.Texto, ancho, a.Tamano) {
		if i > 0 {
			b.WriteString("T*\n")
		}
		fmt.Fprintf(b, "%s Tj\n", a.tj(linea))
	}
	b.WriteString("ET\n")
	recursos["Font"] = fuenteDeAnotacion()
}

// letraAnot es la Liberation Sans incrustada (SIL OFL 1.1) con la que se escribe
// el texto de un texto libre o un sello. Con la Helvetica sin incrustar de antes
// un PDF/A anotado perdía siempre la marca —la norma exige incrustar— y el texto
// en griego o cirílico no se podía escribir (WinAnsi).
type letraAnot struct {
	f   *pdffuente.Fuente
	sub *pdffuente.Subconjunto
}

func nuevaLetraAnot() *letraAnot {
	f := pdffuente.Pila(false, false)[1]
	return &letraAnot{f: f, sub: f.NuevoSubconjunto()}
}

// tj es el operando de Tj para el texto: la cadena WinAnsi de la Helvetica, o
// los glifos de la Liberation incrustada.
func (a Anotacion) tj(texto string) string {
	return a.letra.tj(texto)
}

func (l *letraAnot) tj(texto string) string {
	if l == nil {
		return "(" + escaparCadenaContenido(texto) + ")"
	}
	var b strings.Builder
	b.WriteByte('<')
	for _, r := range texto {
		g := l.f.Glifo(r)
		l.sub.Usar(g, r)
		fmt.Fprintf(&b, "%04X", g)
	}
	b.WriteByte('>')
	return b.String()
}

// escritorDeFuente deja que pdffuente escriba su subconjunto en el ensamblador.
type escritorDeFuente struct{ e *PDFEnsamblador }

func (w escritorDeFuente) Objeto(cuerpo string) int {
	n := w.e.reservar()
	w.e.cuerpos[n-1] = objetoSalida{valor: objetoDeTexto(cuerpo)}
	return n
}

func (w escritorDeFuente) Flujo(dict string, datos []byte) int {
	n := w.e.reservar()
	w.e.cuerpos[n-1] = objetoSalida{valor: objetoDeTexto("<<" + dict + ">>"), flujo: datos, esFlujo: true}
	return n
}

func objetoDeTexto(s string) pdfObj {
	o, err := (&PDFDoc{}).parsearObjeto(&pdfLexer{buf: []byte(s)})
	if err != nil {
		return pdfNull{}
	}
	return o
}

// fuenteDeAnotacion declara la Helvetica del visor. El nombre es propio para no
// pisar el /F1 del documento, que cualquier PDF usa.
func fuenteDeAnotacion() pdfDict {
	return pdfDict{"SytueAnotF": pdfDict{
		"Type":     pdfName("Font"),
		"Subtype":  pdfName("Type1"),
		"BaseFont": pdfName("Helvetica"),
		"Encoding": pdfName("WinAnsiEncoding"),
	}}
}

// partirEnLineas ajusta el texto al ancho disponible. Se estima 0,5 em por
// carácter, que es la media de Helvetica; el texto libre de una anotación es
// corto y no compensa cargar métricas para afinar dos milímetros.
func partirEnLineas(texto string, ancho, tam float64) []string {
	porLinea := int(ancho / (tam * 0.5))
	if porLinea < 4 {
		porLinea = 4
	}
	var out []string
	for _, parrafo := range strings.Split(texto, "\n") {
		if parrafo == "" {
			out = append(out, "")
			continue
		}
		linea := ""
		for _, palabra := range strings.Fields(parrafo) {
			switch {
			case linea == "":
				linea = palabra
			case len(linea)+1+len(palabra) <= porLinea:
				linea += " " + palabra
			default:
				out = append(out, linea)
				linea = palabra
			}
		}
		out = append(out, linea)
	}
	if len(out) > 200 {
		out = out[:200] // una nota no es un documento
	}
	return out
}

// --- Ayudas de serialización -------------------------------------------------

func rectPDF(r [4]float64) pdfArray {
	return pdfArray{r[0], r[1], r[2], r[3]}
}

func colorPDF(c [3]float64) pdfArray {
	return pdfArray{c[0], c[1], c[2]}
}

// cuadrosPDF traduce a /QuadPoints, cuyo orden es el que despista: NO es el de
// las esquinas de un rectángulo, sino arriba-izquierda, arriba-derecha,
// abajo-izquierda y abajo-derecha (§12.5.6.10).
func cuadrosPDF(cs [][4]float64) pdfArray {
	out := make(pdfArray, 0, 8*len(cs))
	for _, c := range cs {
		out = append(out, c[0], c[3], c[2], c[3], c[0], c[1], c[2], c[1])
	}
	return out
}

// fechaPDF escribe una fecha en el formato del formato: «D:20260813142530Z».
func fechaPDF(t time.Time) string {
	return t.UTC().Format("D:20060102150405Z")
}

// cadenaUTF16 escribe una cadena con la marca de orden UTF-16BE, que es la única
// forma de que una tilde o un carácter cirílico sobrevivan en un PDF.
func cadenaUTF16(s string) pdfString {
	out := []byte{0xFE, 0xFF}
	for _, r := range s {
		if r > 0xFFFF {
			r1, r2 := utf16Par(r)
			out = append(out, byte(r1>>8&0xFF), byte(r1&0xFF), byte(r2>>8&0xFF), byte(r2&0xFF))
			continue
		}
		out = append(out, byte(r>>8&0xFF), byte(r&0xFF))
	}
	return out
}

func utf16Par(r rune) (uint16, uint16) {
	r -= 0x10000
	return uint16(0xD800 + (r>>10)&0x3FF), uint16(0xDC00 + r&0x3FF)
}

// numPDF formatea un número para un flujo de contenido: sin exponente, que no es
// válido en PDF, y con tres decimales, que a 72 ppp sobra.
func numPDF(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "0"
	}
	s := strings.TrimRight(fmt.Sprintf("%.3f", v), "0")
	s = strings.TrimSuffix(s, ".")
	if s == "" || s == "-" || s == "-0" {
		return "0"
	}
	return s
}

// escaparCadenaContenido escapa lo que rompe una cadena literal dentro de un
// flujo de contenido, y pasa el texto a WinAnsi, que es lo que entiende la
// Helvetica estándar.
func escaparCadenaContenido(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r > 0xFF {
			b.WriteByte('?') // fuera de WinAnsi: se marca en vez de escribir basura
			continue
		}
		switch byte(r & 0xFF) {
		case '(', ')', '\\':
			b.WriteByte('\\')
		}
		b.WriteByte(byte(r & 0xFF))
	}
	return b.String()
}

// --- Lectura de anotaciones --------------------------------------------------

// AnotacionLeida es una anotación tal y como está en el documento, para
// enseñarla en el panel de comentarios.
type AnotacionLeida struct {
	Pagina int         `json:"pagina"`
	Tipo   string      `json:"tipo"`
	Rect   [4]float64  `json:"rect"`
	Texto  string      `json:"texto,omitempty"`
	Autor  string      `json:"autor,omitempty"`
	Fecha  string      `json:"fecha,omitempty"`
	Color  *[3]float64 `json:"color,omitempty"`
	// RespuestaA es el índice, en esta misma lista, del comentario al que
	// contesta (/IRT). Nulo si no contesta a ninguno. Con esto el panel pinta el
	// hilo en vez de una fila suelta que no dice a qué responde.
	RespuestaA *int `json:"respuesta_a,omitempty"`
	// Estado de revisión (/State): «Accepted», «Rejected», «Completed»… Va en
	// inglés porque es lo que lee Acrobat; traducirlo lo rompería fuera de aquí.
	// Vacío mientras nadie lo haya revisado.
	Estado string `json:"estado,omitempty"`
}

// AnotacionesDelDocumento lista las anotaciones de todas las páginas. Es lo que
// alimenta el panel lateral de comentarios, que es donde se revisa un documento
// de verdad: leyendo la lista, no cazando globitos por las páginas.
// leerAnotacion pasa el diccionario de UNA anotación a la forma que consume la
// interfaz. Sale del recorrido porque son seis campos opcionales seguidos —cada
// uno con su comprobación de tipo— y dentro del doble bucle dejaban la función
// por encima del techo de complejidad que vigila el linter, que aquí acierta: lo
// que cuesta seguir en `AnotacionesDelDocumento` es el hilo de /IRT, no leer un
// /Rect.
func (d *PDFDoc) leerAnotacion(ad pdfDict, pagina int, tipo string) AnotacionLeida {
	leida := AnotacionLeida{Pagina: pagina, Tipo: tipo}
	if r, ok := (&lienzo{d: d}).rectangulo(ad["Rect"]); ok {
		leida.Rect = r
	}
	if s, ok := d.resolver(ad["Contents"]).(pdfString); ok {
		leida.Texto = decodificarTextoPDF(s)
	}
	if s, ok := d.resolver(ad["T"]).(pdfString); ok {
		leida.Autor = decodificarTextoPDF(s)
	}
	if s, ok := d.resolver(ad["M"]).(pdfString); ok {
		leida.Fecha = decodificarTextoPDF(s)
	}
	if arr, ok := d.resolver(ad["C"]).(pdfArray); ok && len(arr) >= 3 {
		c := [3]float64{
			comoReal(d.resolver(arr[0])),
			comoReal(d.resolver(arr[1])),
			comoReal(d.resolver(arr[2])),
		}
		leida.Color = &c
	}
	return leida
}

func (d *PDFDoc) AnotacionesDelDocumento() []AnotacionLeida {
	var out []AnotacionLeida
	// Número de objeto → posición en `out`, y quién responde a quién. El /IRT es
	// una referencia y lo que la interfaz necesita es un índice de esta lista.
	posicion := map[int]int{}
	apuntaA := map[int]int{}
	// Número de objeto del comentario → su último estado de revisión. El último
	// gana: revisar dos veces deja dos marcas y la buena es la de después.
	estadoDe := map[int]string{}
	for i := 0; i < d.NumPaginas(); i++ {
		p, err := d.Pagina(i)
		if err != nil {
			continue
		}
		anots, _ := d.resolver(p.dict["Annots"]).(pdfArray)
		for _, a := range anots {
			ad, ok := d.resolver(a).(pdfDict)
			if !ok {
				continue
			}
			// Una marca de estado no se enumera pero SÍ se lee: es lo que dice si
			// un comentario está resuelto. Se apunta a quién señala y se resuelve
			// al final, cuando ya se sabe dónde cayó cada uno.
			if d.anotaEstado(ad, estadoDe) {
				continue
			}
			// El criterio de qué cuenta vive en un solo sitio: el índice de esta
			// lista es lo que usan borrar y responder, y si cada uno contara a su
			// manera se borraría —o se respondería a— la anotación de al lado.
			if !esAnotacionDeComentario(d, ad) {
				continue
			}
			sub, _ := d.resolver(ad["Subtype"]).(pdfName)
			// Dónde vive cada anotación, para resolver los /IRT al terminar: el
			// padre puede estar en una página posterior y aún no haberse visto.
			if r, ok := a.(pdfRef); ok {
				posicion[r.Num] = len(out)
			}
			if r, ok := ad["IRT"].(pdfRef); ok {
				apuntaA[len(out)] = r.Num
			}
			out = append(out, d.leerAnotacion(ad, i, string(sub)))
		}
	}
	// Al final, cuando ya se sabe dónde cayó cada una. Un /IRT que apunte a algo
	// que no se enumeró —una emergente, o un objeto que ya no está— se deja sin
	// resolver: la respuesta se enseña suelta, que es peor que en hilo pero mejor
	// que esconderla.
	for i, num := range apuntaA {
		if p, ok := posicion[num]; ok && p != i {
			out[i].RespuestaA = &p
		}
	}
	for num, estado := range estadoDe {
		if p, ok := posicion[num]; ok {
			out[p].Estado = estado
		}
	}
	return out
}

// anotaEstado apunta el estado de revisión que lleve esta anotación y dice si
// era una marca —que no se enumera— o un comentario de verdad.
func (d *PDFDoc) anotaEstado(ad pdfDict, estadoDe map[int]string) bool {
	st, ok := d.resolver(ad["State"]).(pdfString)
	if !ok {
		return false
	}
	if r, ok := ad["IRT"].(pdfRef); ok {
		estadoDe[r.Num] = decodificarTextoPDF(st)
	}
	return true
}

// BorrarAnotacionesPDF quita las anotaciones indicadas por su índice dentro de
// la lista que devuelve AnotacionesDelDocumento.
func BorrarAnotacionesPDF(doc *PDFDoc, indices []int) ([]byte, error) {
	fuera := make(map[int]bool, len(indices))
	for _, i := range indices {
		fuera[i] = true
	}
	e := NuevoEnsambladorPDF()
	global := 0
	for i := 0; i < doc.NumPaginas(); i++ {
		p, err := doc.Pagina(i)
		if err != nil {
			return nil, err
		}
		anots, _ := doc.resolver(p.dict["Annots"]).(pdfArray)
		quedan := pdfArray{}
		for _, a := range anots {
			ad, ok := doc.resolver(a).(pdfDict)
			if !ok {
				continue
			}
			// El MISMO criterio que al enumerarlas: el índice que llega aquí sale
			// de esa lista, y contar distinto borraría el comentario de al lado.
			if !esAnotacionDeComentario(doc, ad) {
				quedan = append(quedan, a) // no se enumeran, así que tampoco se borran
				continue
			}
			if !fuera[global] {
				quedan = append(quedan, a)
			}
			global++
		}
		if err := e.anadirPagina(doc, i, 0, opcionesPagina{annots: quedan}); err != nil {
			return nil, err
		}
	}
	return e.Bytes()
}
