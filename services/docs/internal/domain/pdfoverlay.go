// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Superponer texto sobre las páginas de un PDF existente: marcas de agua,
// encabezados y pies, y numeración —incluida la numeración Bates que piden los
// despachos—. Tres funciones de producto sobre UNA primitiva, porque las tres
// son lo mismo: pintar una cadena en un sitio de cada página.
//
// La técnica es la que permite hacerlo sin tocar el contenido original: el
// `/Contents` de una página puede ser un ARRAY de flujos que el visor concatena,
// así que basta añadir uno al final. Lo que ya había se dibuja primero y la capa
// encima; el contenido original no se reescribe ni se reinterpreta, y quitar la
// marca sería tan fácil como no añadir ese flujo.
//
// Sobre la opacidad: un PDF no tiene «alfa» en el color, se consigue con un
// estado gráfico extendido (`/ExtGState` con `/ca`). Se añade a los recursos de
// cada página que lo necesite. Sin él, una marca de agua taparía el texto de
// debajo en vez de velarlo.
package domain

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// PosicionCapa dice dónde va el texto en la página.
type PosicionCapa string

const (
	// CapaDiagonal cruza la página en diagonal, de esquina inferior izquierda a
	// superior derecha: la marca de agua clásica.
	CapaDiagonal PosicionCapa = "diagonal"
	CapaCentro   PosicionCapa = "centro"

	CapaSuperiorIzq PosicionCapa = "superior-izquierda"
	CapaSuperiorCen PosicionCapa = "superior-centro"
	CapaSuperiorDer PosicionCapa = "superior-derecha"
	CapaInferiorIzq PosicionCapa = "inferior-izquierda"
	CapaInferiorCen PosicionCapa = "inferior-centro"
	CapaInferiorDer PosicionCapa = "inferior-derecha"
)

// CapaTexto describe qué escribir y dónde.
type CapaTexto struct {
	// Texto admite dos marcadores que se sustituyen por página:
	// «{pagina}» (empezando en Desde) y «{total}». Con eso se cubren tanto la
	// numeración corriente como la Bates, que es un contador correlativo con
	// prefijo y relleno de ceros: «ACME-{pagina}» con Desde=1 y Digitos=6.
	Texto string
	// Digitos rellena «{pagina}» con ceros a la izquierda hasta esa anchura.
	// 0 = sin relleno. La numeración Bates se identifica por esto.
	Digitos int
	// Desde es el número de la primera página numerada (por defecto 1).
	Desde int

	Posicion PosicionCapa
	// Tamano en puntos. 0 usa 48 en diagonal/centro y 10 en el resto: una marca
	// de agua y un pie de página no se leen a la misma distancia.
	Tamano float64
	// Opacidad de 0 (invisible) a 1 (opaca). 0 se trata como 1.
	Opacidad float64
	// Gris de 0 (negro) a 1 (blanco). El valor cero es negro, que es lo que se
	// quiere en un pie; una marca de agua suele querer 0.5.
	Gris float64
	// Margen desde el borde para las posiciones de esquina. 0 usa 36 pt (media
	// pulgada), que es el margen habitual de un documento de oficina.
	Margen float64
	// SoloPaginas limita la capa a esas páginas (0-based). Vacío = todas.
	SoloPaginas []int
	// Debajo pinta la capa ANTES del contenido, no encima. Es la diferencia
	// entre un fondo y una marca de agua: el fondo no tapa el texto ni las
	// tablas —queda detrás, como el membrete de un papel—, y por eso puede ir
	// opaco sin estorbar a la lectura.
	Debajo bool
}

// SuperponerTextoPDF devuelve el documento con la capa añadida en cada página.
func SuperponerTextoPDF(doc *PDFDoc, capa CapaTexto) ([]byte, error) {
	if strings.TrimSpace(capa.Texto) == "" {
		return nil, fmt.Errorf("pdf: la capa no tiene texto")
	}
	if doc.NumPaginas() == 0 {
		return nil, fmt.Errorf("pdf: el documento no tiene páginas")
	}

	solo := map[int]bool{}
	for _, i := range capa.SoloPaginas {
		solo[i] = true
	}

	e := NuevoEnsambladorPDF()
	for i := 0; i < doc.NumPaginas(); i++ {
		if err := e.AnadirPagina(doc, i, 0); err != nil {
			return nil, err
		}
		if len(solo) > 0 && !solo[i] {
			continue
		}
		p, err := doc.Pagina(i)
		if err != nil {
			return nil, err
		}
		if err := e.superponerEnUltima(p, capa, i, doc.NumPaginas()); err != nil {
			return nil, err
		}
	}
	return e.Bytes()
}

// superponerEnUltima añade la capa a la página recién agregada, trabajando sobre
// su DICCIONARIO y no sobre los bytes ya serializados. La diferencia no es de
// estilo: parcheando texto, una página cuyo /Resources es una referencia
// indirecta acababa con la clave duplicada y el recurso de la capa se perdía sin
// error — el texto simplemente no se pintaba.
func (e *PDFEnsamblador) superponerEnUltima(p PDFPagina, capa CapaTexto, indice, total int) error {
	pp := e.ultimaPendiente()
	if pp == nil {
		return fmt.Errorf("pdf: no hay página a la que superponer")
	}

	texto := resolverMarcadores(capa, indice, total)
	// El ancho y el alto que ve la capa son los de la página SIN rotar: el visor
	// aplica /Rotate después, así que colocar el pie con las medidas ya rotadas
	// lo sacaría del papel en las páginas apaisadas.
	ancho, alto := p.Ancho, p.Alto
	if p.Rotacion == 90 || p.Rotacion == 270 {
		ancho, alto = alto, ancho
	}
	flujo := []byte(dibujarCapa(texto, capa, ancho, alto))

	numFlujo := e.reservar()
	e.cuerpos[numFlujo-1] = objetoSalida{valor: pdfDict{}, flujo: flujo, esFlujo: true}

	// El /Contents pasa a ser un array con lo que ya había y la capa DETRÁS, que
	// es lo que hace que se dibuje encima sin tocar el contenido original. Un
	// fondo va justo al revés: primero él, y el documento encima.
	ref := pdfRef{Num: numFlujo}
	switch actual := pp.dict["Contents"].(type) {
	case pdfArray:
		ya := append(pdfArray{}, actual...)
		if capa.Debajo {
			pp.dict["Contents"] = append(pdfArray{ref}, ya...)
		} else {
			pp.dict["Contents"] = append(ya, ref)
		}
	case nil:
		pp.dict["Contents"] = pdfArray{ref}
	default:
		if capa.Debajo {
			pp.dict["Contents"] = pdfArray{ref, actual}
		} else {
			pp.dict["Contents"] = pdfArray{actual, ref}
		}
	}

	pp.dict["Resources"] = e.recursosConCapa(pp, capa)
	e.reserializarPendiente(pp)
	return nil
}

// recursosConCapa devuelve el /Resources de la página con la fuente de la capa
// —y su estado gráfico si hay opacidad— añadidos a lo que ya tuviera.
//
// Los nombres son propios («SytueCapaF», «SytueCapaGS») para no pisar los del
// documento: cualquier PDF usa /F1, y reutilizar ese nombre cambiaría la fuente
// del texto ORIGINAL, que es reescribir el documento sin decirlo.
func (e *PDFEnsamblador) recursosConCapa(pp *paginaPendiente, capa CapaTexto) pdfDict {
	// Si el original era una referencia indirecta, se materializa resolviéndola
	// contra el documento de origen: hay que fusionar, no sustituir, o la página
	// perdería sus propias fuentes e imágenes.
	rec := pdfDict{}
	switch v := pp.dict["Resources"].(type) {
	case pdfDict:
		for k, x := range v {
			rec[k] = x
		}
	case pdfRef:
		if orig, ok := pp.origen.resolver(pp.p.dict["Resources"]).(pdfDict); ok {
			for _, k := range clavesOrdenadas(orig) {
				rec[k] = e.copiarValor(pp.origen, orig[k])
			}
		}
	}

	numFuente := e.reservar()
	e.cuerpos[numFuente-1] = objetoSalida{valor: pdfDict{
		"Type":     pdfName("Font"),
		"Subtype":  pdfName("Type1"),
		"BaseFont": pdfName("Helvetica"),
		"Encoding": pdfName("WinAnsiEncoding"),
	}}

	fuentes := pdfDict{}
	switch ya := rec["Font"].(type) {
	case pdfDict:
		for k, x := range ya {
			fuentes[k] = x
		}
	case pdfRef:
		// /Font indirecto: se conserva lo que apunta y se añade la de la capa
		// resolviéndolo, por el mismo motivo que arriba.
		if orig, ok := pp.origen.resolver(ya).(pdfDict); ok {
			for k, x := range orig {
				fuentes[k] = x
			}
		}
	}
	fuentes["SytueCapaF"] = pdfRef{Num: numFuente}
	rec["Font"] = fuentes

	if capa.Opacidad > 0 && capa.Opacidad < 1 {
		numGS := e.reservar()
		e.cuerpos[numGS-1] = objetoSalida{valor: pdfDict{
			"Type": pdfName("ExtGState"),
			// ca = alfa de relleno, CA = de trazo. El texto se rellena.
			"ca": capa.Opacidad,
			"CA": capa.Opacidad,
		}}
		estados := pdfDict{}
		if ya, ok := rec["ExtGState"].(pdfDict); ok {
			for k, x := range ya {
				estados[k] = x
			}
		}
		estados["SytueCapaGS"] = pdfRef{Num: numGS}
		rec["ExtGState"] = estados
	}
	return rec
}

// resolverMarcadores sustituye {pagina} y {total}.
func resolverMarcadores(capa CapaTexto, indice, total int) string {
	desde := capa.Desde
	if desde == 0 {
		desde = 1
	}
	n := strconv.Itoa(indice + desde)
	if capa.Digitos > 0 && len(n) < capa.Digitos {
		n = strings.Repeat("0", capa.Digitos-len(n)) + n
	}
	t := strings.ReplaceAll(capa.Texto, "{pagina}", n)
	return strings.ReplaceAll(t, "{total}", strconv.Itoa(total))
}

// dibujarCapa genera el flujo de contenido con el texto ya colocado.
func dibujarCapa(texto string, capa CapaTexto, ancho, alto float64) string {
	tam := capa.Tamano
	if tam <= 0 {
		if capa.Posicion == CapaDiagonal || capa.Posicion == CapaCentro {
			tam = 48
		} else {
			tam = 10
		}
	}
	margen := capa.Margen
	if margen <= 0 {
		margen = 36
	}
	gris := capa.Gris
	if gris < 0 {
		gris = 0
	}
	if gris > 1 {
		gris = 1
	}

	// Ancho aproximado con el 0,5 em medio de Helvetica, igual que el escritor:
	// basta para centrar y para no salirse, que es lo que se necesita aquí.
	anchoTexto := float64(len([]rune(texto))) * tam * 0.5

	var b strings.Builder
	b.WriteString("q\n") // guarda el estado gráfico: la capa no contamina nada
	if capa.Opacidad > 0 && capa.Opacidad < 1 {
		b.WriteString("/SytueCapaGS gs\n")
	}
	b.WriteString(ftoa(gris) + " g\n")
	b.WriteString("BT\n/SytueCapaF " + ftoa(tam) + " Tf\n")

	if capa.Posicion == CapaDiagonal {
		// Diagonal: se rota alrededor del centro con la matriz de texto. El
		// ángulo sale de la proporción REAL de la página para que cruce de
		// esquina a esquina; un 45° fijo se queda corto en A4.
		ang := math.Atan2(alto, ancho)
		cos, sin := math.Cos(ang), math.Sin(ang)
		cx := ancho/2 - (anchoTexto/2)*cos
		cy := alto/2 - (anchoTexto/2)*sin
		fmt.Fprintf(&b, "%s %s %s %s %s %s Tm\n",
			ftoa(cos), ftoa(sin), ftoa(-sin), ftoa(cos), ftoa(cx), ftoa(cy))
	} else {
		x, y := posicionDe(capa.Posicion, ancho, alto, anchoTexto, tam, margen)
		b.WriteString(ftoa(x) + " " + ftoa(y) + " Td\n")
	}

	b.WriteString("(" + pdfEscape(texto) + ") Tj\nET\nQ\n")
	return b.String()
}

// posicionDe calcula el punto de inserción según la esquina elegida.
func posicionDe(pos PosicionCapa, ancho, alto, anchoTexto, tam, margen float64) (float64, float64) {
	izq, cen, der := margen, (ancho-anchoTexto)/2, ancho-margen-anchoTexto
	if der < izq {
		der = izq // texto más ancho que la página: al menos que empiece dentro
	}
	if cen < izq {
		cen = izq
	}
	// La línea base va por encima del borde inferior; arriba se descuenta el alto
	// del texto para que no lo corte el canto.
	abajo, arriba := margen, alto-margen-tam

	switch pos {
	case CapaSuperiorIzq:
		return izq, arriba
	case CapaSuperiorCen:
		return cen, arriba
	case CapaSuperiorDer:
		return der, arriba
	case CapaInferiorIzq:
		return izq, abajo
	case CapaInferiorCen:
		return cen, abajo
	case CapaInferiorDer:
		return der, abajo
	case CapaCentro:
		return cen, alto / 2
	default:
		return cen, abajo
	}
}

// --- Funciones de producto ---------------------------------------------------

// MarcaDeAguaPDF cruza cada página con un texto velado en diagonal.
func MarcaDeAguaPDF(doc *PDFDoc, texto string) ([]byte, error) {
	return SuperponerTextoPDF(doc, CapaTexto{
		Texto:    texto,
		Posicion: CapaDiagonal,
		Opacidad: 0.18, // se lee sin estorbar al documento de debajo
		Gris:     0.45,
	})
}

// FondoPDF estampa un texto DETRÁS del contenido.
//
// No es una marca de agua con otro nombre: el fondo queda debajo, así que no
// tapa el texto ni las tablas del documento y puede ir mucho más marcado —un
// «BORRADOR» en gris claro se lee sin que estorbe a la lectura—. La marca de
// agua va encima y por eso tiene que ser tenue.
func FondoPDF(doc *PDFDoc, texto string) ([]byte, error) {
	return SuperponerTextoPDF(doc, CapaTexto{
		Texto:    texto,
		Posicion: CapaDiagonal,
		Debajo:   true,
		// Más opaco que la marca de agua justo porque va detrás: lo que tape es
		// el papel, no lo que hay escrito.
		Opacidad: 0.5,
		Gris:     0.75,
	})
}

// NumerarPaginasPDF escribe «3 / 10» —o el formato que se pida— al pie.
func NumerarPaginasPDF(doc *PDFDoc, formato string, pos PosicionCapa) ([]byte, error) {
	if strings.TrimSpace(formato) == "" {
		formato = "{pagina} / {total}"
	}
	if pos == "" {
		pos = CapaInferiorCen
	}
	return SuperponerTextoPDF(doc, CapaTexto{Texto: formato, Posicion: pos, Tamano: 9})
}

// NumeracionBatesPDF estampa un correlativo con prefijo y ceros a la izquierda,
// como exige la presentación de documentos en un procedimiento legal. Cada
// página lleva un identificador único e irrepetible del expediente.
func NumeracionBatesPDF(doc *PDFDoc, prefijo string, desde, digitos int) ([]byte, error) {
	if digitos <= 0 {
		digitos = 6
	}
	if desde <= 0 {
		desde = 1
	}
	return SuperponerTextoPDF(doc, CapaTexto{
		Texto:    prefijo + "{pagina}",
		Digitos:  digitos,
		Desde:    desde,
		Posicion: CapaInferiorDer,
		Tamano:   9,
	})
}

// EncabezadoPiePDF pone un texto fijo arriba o abajo. Admite los marcadores de
// página, así que también sirve para «Confidencial — pág. {pagina}».
func EncabezadoPiePDF(doc *PDFDoc, texto string, pos PosicionCapa) ([]byte, error) {
	if pos == "" {
		pos = CapaSuperiorCen
	}
	return SuperponerTextoPDF(doc, CapaTexto{Texto: texto, Posicion: pos, Tamano: 10})
}
