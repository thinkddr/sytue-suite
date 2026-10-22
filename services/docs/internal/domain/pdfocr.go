// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// La capa de texto de un PDF escaneado (ola P8). Ver [ADR-0034].
//
// «OCR» son dos cosas que se confunden porque llegan juntas: RECONOCER —de
// píxeles a caracteres, que es un modelo— y ESCRIBIR LA CAPA —meter ese texto en
// el PDF de forma que se pueda buscar y copiar sin cambiar ni un píxel de lo que
// se ve—. Aquí se hace la segunda, que es formato y no visión, y por eso no
// necesita ninguna dependencia.
//
// EL TRUCO, que es viejo y muy elegante: el texto se escribe con el modo de
// render 3 —invisible— justo encima de donde está dibujado en la imagen. El
// documento se ve EXACTAMENTE igual, y sin embargo se busca, se selecciona y lo
// lee un lector de pantalla. Un escaneo pasa de ser una foto de un papel a ser un
// documento.
package domain

import (
	"fmt"
	"sort"
	"strings"
)

// TextoReconocido es una palabra o línea localizada sobre la página, en
// coordenadas del PDF (origen abajo-izquierda) y en puntos.
type TextoReconocido struct {
	Texto string `json:"texto"`
	// Rect es la caja que ocupa [x0 y0 x1 y1]. De ahí sale el tamaño de letra y
	// cuánto hay que estirar el texto para que la selección caiga encima.
	Rect [4]float64 `json:"rect"`
}

// maxTextosPorPagina acota la capa. Una página de periódico tiene miles de
// palabras; cien mil es un fichero manipulado.
const maxTextosPorPagina = 20000

// CapaDeTextoPDF añade texto invisible sobre las páginas indicadas y devuelve el
// documento resultante.
//
// La clave del mapa es el índice de página (0-based). Las páginas que no
// aparezcan se copian sin tocar.
func CapaDeTextoPDF(doc *PDFDoc, porPagina map[int][]TextoReconocido) ([]byte, error) {
	if len(porPagina) == 0 {
		return nil, fmt.Errorf("pdf: no se ha indicado ningún texto")
	}
	for pagina, textos := range porPagina {
		if pagina < 0 || pagina >= doc.NumPaginas() {
			return nil, fmt.Errorf("pdf: la página %d no existe", pagina+1)
		}
		if len(textos) > maxTextosPorPagina {
			return nil, fmt.Errorf("pdf: demasiado texto en la página %d (máximo %d fragmentos)",
				pagina+1, maxTextosPorPagina)
		}
	}

	e := NuevoEnsambladorPDF()
	for i := 0; i < doc.NumPaginas(); i++ {
		if err := e.AnadirPagina(doc, i, 0); err != nil {
			return nil, err
		}
		textos := porPagina[i]
		if len(textos) == 0 {
			continue
		}
		if err := e.anadirCapaDeTexto(textos); err != nil {
			return nil, err
		}
	}
	e.ConservarFormulario(doc)
	return e.Bytes()
}

// anadirCapaDeTexto cuelga el flujo invisible de la última página añadida.
func (e *PDFEnsamblador) anadirCapaDeTexto(textos []TextoReconocido) error {
	pp := e.ultimaPendiente()
	if pp == nil {
		return fmt.Errorf("pdf: no hay página donde poner la capa de texto")
	}

	var b strings.Builder
	// 3 Tr = invisible. Es lo que hace que el documento no cambie de aspecto.
	b.WriteString("q BT 3 Tr\n")
	for _, t := range textos {
		txt := strings.TrimSpace(t.Texto)
		if txt == "" {
			continue
		}
		ancho := t.Rect[2] - t.Rect[0]
		alto := t.Rect[3] - t.Rect[1]
		if ancho <= 0 || alto <= 0 {
			continue
		}
		// El tamaño sale de la ALTURA de la caja; la anchura se ajusta con la
		// escala horizontal (Tz). Sin ese ajuste, la selección con el ratón
		// marcaría un trozo distinto del que se ve, que es peor que no tener capa.
		// Con espaciado entre letras (Tc), como se hacía antes, un lector que vea
		// el hueco entre letras parte la palabra («a n e x o»).
		tam := alto * 0.86
		if tam < 1 {
			tam = 1
		}
		anchoNatural := float64(len([]rune(txt))) * tam * 0.5
		fmt.Fprintf(&b, "/SytueOCR %s Tf %s Tz 1 0 0 1 %s %s Tm (%s) Tj\n",
			numPDF(tam), numPDF(100*ancho/anchoNatural), numPDF(t.Rect[0]), numPDF(t.Rect[1]),
			escaparCadenaContenido(txt))
	}
	b.WriteString("ET Q\n")

	numFlujo := e.reservar()
	e.cuerpos[numFlujo-1] = objetoSalida{valor: pdfDict{}, flujo: []byte(b.String()), esFlujo: true}

	// La capa va DETRÁS del contenido: se dibuja encima de la imagen escaneada,
	// que es lo que hace que la selección caiga sobre las letras que se ven.
	switch actual := pp.dict["Contents"].(type) {
	case pdfArray:
		pp.dict["Contents"] = append(append(pdfArray{}, actual...), pdfRef{Num: numFlujo})
	case nil:
		pp.dict["Contents"] = pdfArray{pdfRef{Num: numFlujo}}
	default:
		pp.dict["Contents"] = pdfArray{actual, pdfRef{Num: numFlujo}}
	}

	numFuente := e.reservar()
	e.cuerpos[numFuente-1] = objetoSalida{valor: pdfDict{
		"Type":     pdfName("Font"),
		"Subtype":  pdfName("Type1"),
		"BaseFont": pdfName("Helvetica"),
		"Encoding": pdfName("WinAnsiEncoding"),
	}}
	pp.dict["Resources"] = e.recursosConFuente(pp, "SytueOCR", pdfRef{Num: numFuente})
	e.reserializarPendiente(pp)
	return nil
}

// recursosConFuente añade una fuente al /Resources de la página sin perder las
// que ya tenía. El nombre es propio para no pisar el /F1 del documento, que
// cambiaría la tipografía del texto ORIGINAL.
func (e *PDFEnsamblador) recursosConFuente(pp *paginaPendiente, nombre pdfName, ref pdfObj) pdfDict {
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
	fuentes := pdfDict{}
	switch ya := rec["Font"].(type) {
	case pdfDict:
		for k, x := range ya {
			fuentes[k] = x
		}
	case pdfRef:
		if orig, ok := pp.origen.resolver(ya).(pdfDict); ok {
			for k, x := range orig {
				fuentes[k] = x
			}
		}
	}
	fuentes[nombre] = ref
	rec["Font"] = fuentes
	return rec
}

// PaginasSinTexto dice qué páginas NO tienen ni una palabra, que es la señal de
// que un documento está escaneado y necesita OCR.
//
// Se usa para ofrecer la herramienta solo donde sirve: proponer «reconocer
// texto» sobre un documento que ya lo tiene es hacer trabajar al usuario para
// nada, y peor aún, duplicar la capa.
func (d *PDFDoc) PaginasSinTexto() []int {
	var out []int
	for i := 0; i < d.NumPaginas(); i++ {
		t, err := d.TextoDelContenido(i)
		if err != nil || strings.TrimSpace(t) == "" {
			out = append(out, i)
		}
	}
	return out
}

// --- La imagen de una página escaneada ---------------------------------------

// ImagenColocada es una imagen de la página junto con DÓNDE está colocada, que
// es lo que permite traducir lo que reconoce el OCR a coordenadas del PDF.
type ImagenColocada struct {
	// ID es el nombre del recurso XObject dentro de la página.
	ID string
	// Ancho y Alto en píxeles de la propia imagen.
	Ancho, Alto int
	// Datos son los bytes listos para mandar al reconocedor, y Tipo su MIME.
	Datos []byte
	Tipo  string
	// ctm coloca el cuadrado unidad de la imagen en el espacio del PDF.
	ctm matriz
}

// Pixeles es cuántos tiene la imagen. Sirve para quedarse con la más grande de
// la página, que en un escaneo ES la página.
func (im ImagenColocada) Pixeles() int { return im.Ancho * im.Alto }

// RectDePDF traduce una caja en PÍXELES de la imagen —origen arriba-izquierda,
// que es como las da un OCR— al rectángulo equivalente en el PDF.
//
// Son dos volteos encadenados y por eso se escribe una sola vez: la imagen se
// recorre de arriba abajo, el cuadrado unidad del PDF va de abajo arriba, y
// encima la CTM puede rotar y escalar. Equivocarse aquí deja la capa de texto
// desplazada media página, que es indistinguible de «el OCR funciona mal».
func (im ImagenColocada) RectDePDF(x, y, ancho, alto int) [4]float64 {
	if im.Ancho <= 0 || im.Alto <= 0 {
		return [4]float64{}
	}
	u := func(px int) float64 { return float64(px) / float64(im.Ancho) }
	v := func(py int) float64 { return 1 - float64(py)/float64(im.Alto) }

	// Las cuatro esquinas, porque con rotación el envolvente no sale de dos.
	esquinas := [4][2]float64{
		{u(x), v(y)},
		{u(x + ancho), v(y)},
		{u(x + ancho), v(y + alto)},
		{u(x), v(y + alto)},
	}
	var xs, ys []float64
	for _, c := range esquinas {
		xs = append(xs, im.ctm[0]*c[0]+im.ctm[2]*c[1]+im.ctm[4])
		ys = append(ys, im.ctm[1]*c[0]+im.ctm[3]*c[1]+im.ctm[5])
	}
	return [4]float64{minimo(xs), minimo(ys), maximo(xs), maximo(ys)}
}

// ImagenesColocadas devuelve las imágenes de una página con su colocación.
//
// Se ejecuta el MISMO intérprete que dibuja, en modo recolección: reimplementar
// el recorrido del flujo aquí significaría dos intérpretes que se desincronizan
// —y el segundo siempre acaba entendiendo peor el `cm`—.
func (d *PDFDoc) ImagenesColocadas(pagina int) ([]ImagenColocada, error) {
	p, err := d.Pagina(pagina)
	if err != nil {
		return nil, err
	}
	contenido, err := d.contenidoDePagina(p)
	if err != nil {
		return nil, err
	}
	l := &lienzo{d: d, fuentes: map[string]*fuentePDF{}, caraCSS: map[string]bool{}, recolectando: true}
	recursos, _ := d.resolver(p.dict["Resources"]).(pdfDict)
	// La matriz base NO se aplica: se quieren las coordenadas del PDF, no las del
	// SVG. La capa de texto se escribe en el flujo de contenido, que vive en el
	// espacio del PDF.
	l.ejecutar(contenido, recursos, estadoInicial(identidad))
	return l.imagenes, nil
}

// ImagenDeEscaneo devuelve la imagen que ES la página, o `false` si esa página
// no parece un escaneo.
//
// El criterio: la imagen más grande que cubre al menos el 60 % del papel. Una
// página con un logotipo y texto tiene imágenes, pero ninguna la cubre; una
// escaneada tiene UNA que la ocupa entera. Mandar un logotipo al OCR gastaría un
// segundo de CPU para reconocer el nombre de la empresa por décima vez.
func (d *PDFDoc) ImagenDeEscaneo(pagina int) (ImagenColocada, bool) {
	p, err := d.Pagina(pagina)
	if err != nil {
		return ImagenColocada{}, false
	}
	imagenes, err := d.ImagenesColocadas(pagina)
	if err != nil || len(imagenes) == 0 {
		return ImagenColocada{}, false
	}
	areaPagina := p.Ancho * p.Alto
	mejor, mejorArea := ImagenColocada{}, 0.0
	for _, im := range imagenes {
		if len(im.Datos) == 0 {
			continue
		}
		r := im.RectDePDF(0, 0, im.Ancho, im.Alto)
		area := (r[2] - r[0]) * (r[3] - r[1])
		if area > mejorArea {
			mejor, mejorArea = im, area
		}
	}
	if mejorArea <= 0 || areaPagina <= 0 || mejorArea < areaPagina*0.6 {
		return ImagenColocada{}, false
	}
	return mejor, true
}

// minAreaOCR es la fracción de la página por debajo de la cual una imagen no se
// manda al reconocedor. Un 8 % deja fuera logotipos, firmas escaneadas y viñetas
// —cuyo texto, si lo tienen, ya está en el documento— y deja dentro una banda de
// un escaneo por tiras, que es lo que esto viene a rescatar.
const minAreaOCR = 0.08

// ImagenesParaOCR devuelve TODAS las imágenes de la página que vale la pena
// reconocer, de mayor a menor.
//
// Existe porque `ImagenDeEscaneo` solo acepta una imagen que cubra el 60 % del
// papel, y hay escaneos legítimos que no lo cumplen: los que llegan partidos en
// bandas horizontales —lo normal en un TIFF de gran formato convertido a PDF— y
// los que traen márgenes generosos. Con el criterio de una sola imagen, esas
// páginas se contaban como «sin imagen» y NO se reconocían nunca; ni un error,
// ni una forma de forzarlo.
//
// El resultado se ordena por área descendente para que, si hay que recortar por
// presupuesto, se quede con lo que más texto puede tener.
func (d *PDFDoc) ImagenesParaOCR(pagina int) []ImagenColocada {
	p, err := d.Pagina(pagina)
	if err != nil {
		return nil
	}
	imagenes, err := d.ImagenesColocadas(pagina)
	if err != nil || len(imagenes) == 0 {
		return nil
	}
	areaPagina := p.Ancho * p.Alto
	if areaPagina <= 0 {
		return nil
	}

	type conArea struct {
		im   ImagenColocada
		area float64
	}
	utiles := make([]conArea, 0, len(imagenes))
	for _, im := range imagenes {
		if len(im.Datos) == 0 {
			continue
		}
		r := im.RectDePDF(0, 0, im.Ancho, im.Alto)
		area := (r[2] - r[0]) * (r[3] - r[1])
		if area < areaPagina*minAreaOCR {
			continue
		}
		utiles = append(utiles, conArea{im, area})
	}
	sort.Slice(utiles, func(i, j int) bool { return utiles[i].area > utiles[j].area })

	out := make([]ImagenColocada, 0, len(utiles))
	for _, u := range utiles {
		out = append(out, u.im)
	}
	return out
}

// apuntarImagen registra una imagen con la CTM que la coloca, sin dibujarla.
func (l *lienzo) apuntarImagen(e *ejecucion, st *pdfStream, nombre string) {
	if len(l.imagenes) > 64 {
		return // una página con cientos de imágenes no es un escaneo
	}
	ancho, alto := l.tamanoImagen(st.Dict)
	if ancho <= 0 || alto <= 0 || ancho*alto > maxPixelesImagen {
		return
	}
	im := ImagenColocada{ID: nombre, Ancho: ancho, Alto: alto, ctm: e.estado.ctm}
	im.Datos, im.Tipo = l.bytesParaReconocer(st)
	l.imagenes = append(l.imagenes, im)
}

// bytesParaReconocer devuelve la imagen en un formato que el reconocedor
// entienda.
//
// Un JPEG se manda TAL CUAL: decodificarlo y volver a codificarlo a PNG no
// mejora el reconocimiento y multiplica por diez lo que viaja por la red. Todo
// lo demás —incluido el fax CCITT, que es el caso mayoritario de un escaneo— se
// decodifica y se empaqueta en PNG.
func (l *lienzo) bytesParaReconocer(st *pdfStream) ([]byte, string) {
	datos, terminal, err := l.d.datosDeFlujoHasta(st)
	if err != nil {
		return nil, ""
	}
	if terminal == "DCTDecode" || terminal == "DCT" {
		return datos, "image/jpeg"
	}
	img, ok := l.decodificarImagen(st)
	if !ok {
		return nil, ""
	}
	png, ok := bytesPNG(img)
	if !ok {
		return nil, ""
	}
	return png, "image/png"
}
