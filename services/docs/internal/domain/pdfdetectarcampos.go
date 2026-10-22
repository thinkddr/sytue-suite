// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Detección de campos sobre un impreso: la «Run Form Field Recognition» de
// Foxit.
//
// El caso real es una solicitud en papel convertida a PDF: tiene rayas donde se
// escribe el nombre y cuadraditos donde se marca una opción, pero ni un solo
// campo de formulario. Crearlos a mano son veinte rectángulos dibujados uno a
// uno, y por eso nadie lo hace.
//
// Aquí NO se crean: se PROPONEN. La heurística acierta lo suficiente para
// ahorrar el trabajo y falla lo bastante como para que aprobar a ciegas
// estropee un documento —una tabla tiene celdas que parecen campos—, así que la
// última palabra es de quien lo prepara.
package domain

import (
	"sort"
	"strings"
)

// PropuestaCampo es un campo que el documento parece pedir.
type PropuestaCampo struct {
	Tipo   TipoCampo  `json:"tipo"`
	Pagina int        `json:"pagina"`
	Rect   [4]float64 `json:"rect"`
	// Etiqueta es el texto que hay justo a la izquierda o encima, que es de donde
	// sale un nombre útil: «DNI» en vez de «texto3».
	Etiqueta string `json:"etiqueta,omitempty"`
}

// Medidas de un campo en un impreso, en puntos. Salen de lo que mide una línea
// de escritura a mano en un formulario de oficina: entre 8 y 40 puntos de alto.
const (
	minAltoCampo   = 8
	maxAltoCampo   = 40
	minAnchoCampo  = 36
	maxLadoCasilla = 24
	minLadoCasilla = 7
	// Una raya de escritura: alto casi cero y ancho de sobra.
	maxAltoLinea  = 2.5
	minAnchoLinea = 50
	// Alto que se le da al campo que va SOBRE una raya. Una línea no dice cuánto
	// mide lo que se escribe encima; catorce puntos es una línea de texto normal.
	altoSobreLinea = 14
)

// maxPropuestas acota lo que se ofrece: una tabla grande genera cientos de
// celdas y proponerlas todas convierte la ayuda en trabajo.
const maxPropuestas = 60

// DetectarCamposPDF propone campos para una página.
//
// ponytail: heurística geométrica, sin OCR ni modelo. Acierta con los impresos
// de oficina —rayas y cuadraditos— y NO distingue una tabla de datos de una
// rejilla para rellenar, porque en el papel son lo mismo. Si un día hace falta
// más, el paso siguiente es mirar el texto de alrededor (una celda con número
// dentro no es un campo), no afinar los umbrales.
func (d *PDFDoc) DetectarCamposPDF(pagina int) ([]PropuestaCampo, error) {
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
	l.ejecutar(contenido, recursos, estadoInicial(matrizBase(p)))

	// Los campos que YA existen no se vuelven a proponer: duplicar uno deja dos
	// cajas superpuestas y quien rellena no sabe en cuál escribe.
	ocupados := make([]rectPagina, 0, 16)
	for _, c := range d.CamposDelFormulario() {
		if c.Pagina == pagina {
			ocupados = append(ocupados, rectPagina{c.Rect[0], c.Rect[1], c.Rect[2], c.Rect[3]})
		}
	}

	fragmentos := l.fragmentosEditables(contenido, recursos, pagina)
	var out []PropuestaCampo
	for _, r := range l.rects {
		if len(out) >= maxPropuestas {
			break
		}
		prop, ok := propuestaDe(r)
		if !ok || solapaAlguno(prop.Rect, ocupados) {
			continue
		}
		prop.Pagina = pagina
		prop.Etiqueta = etiquetaCercana(prop.Rect, fragmentos)
		out = append(out, prop)
		ocupados = append(ocupados, rectPagina{prop.Rect[0], prop.Rect[1], prop.Rect[2], prop.Rect[3]})
	}

	// De arriba abajo y de izquierda a derecha: es el orden en que se rellena un
	// impreso, y por tanto el orden en que hay que revisarlo.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Rect[1] != out[j].Rect[1] {
			return out[i].Rect[1] > out[j].Rect[1]
		}
		return out[i].Rect[0] < out[j].Rect[0]
	})
	return out, nil
}

// propuestaDe clasifica una caja del contenido.
func propuestaDe(r rectPagina) (PropuestaCampo, bool) {
	ancho, alto := r.x1-r.x0, r.y1-r.y0

	// Una raya: el campo va ENCIMA, que es donde se escribe. Incluye el alto
	// exactamente cero, que es como llega una dibujada con m/l.
	if alto <= maxAltoLinea && ancho >= minAnchoLinea {
		return PropuestaCampo{
			Tipo: CampoTexto,
			Rect: [4]float64{r.x0, r.y1, r.x1, r.y1 + altoSobreLinea},
		}, true
	}
	// Un cuadradito: una casilla. El ratio es lo que la separa de una caja de
	// texto corta —un cuadrado se marca, un rectángulo se rellena—.
	if alto >= minLadoCasilla && alto <= maxLadoCasilla &&
		ancho >= minLadoCasilla && ancho <= maxLadoCasilla &&
		ancho/alto < 1.4 && alto/ancho < 1.4 {
		return PropuestaCampo{Tipo: CampoCasilla, Rect: [4]float64{r.x0, r.y0, r.x1, r.y1}}, true
	}
	// Una caja con altura de línea de escritura: campo de texto dentro.
	if alto >= minAltoCampo && alto <= maxAltoCampo && ancho >= minAnchoCampo {
		return PropuestaCampo{Tipo: CampoTexto, Rect: [4]float64{r.x0, r.y0, r.x1, r.y1}}, true
	}
	return PropuestaCampo{}, false
}

// solapaAlguno evita proponer dos campos en el mismo sitio. Basta con que se
// pisen a medias: dos cajas casi iguales son el borde doble de un impreso, no
// dos campos.
func solapaAlguno(r [4]float64, ya []rectPagina) bool {
	area := (r[2] - r[0]) * (r[3] - r[1])
	if area <= 0 {
		return true
	}
	for _, o := range ya {
		w := min(r[2], o.x1) - max(r[0], o.x0)
		h := min(r[3], o.y1) - max(r[1], o.y0)
		if w > 0 && h > 0 && w*h > area*0.4 {
			return true
		}
	}
	return false
}

// etiquetaCercana busca el texto que nombra al campo: primero a la IZQUIERDA en
// la misma línea —que es como se maqueta un impreso— y si no, justo encima.
//
// De aquí sale un nombre que significa algo. Un formulario cuyos campos se
// llaman «texto1, texto2, texto3» obliga a adivinar al rellenarlo y al exportar
// las respuestas.
func etiquetaCercana(rect [4]float64, fragmentos []FragmentoEditable) string {
	medio := (rect[1] + rect[3]) / 2
	mejor, mejorDist := "", 0.0
	for _, f := range fragmentos {
		texto := trimEtiqueta(f.Texto)
		if texto == "" {
			continue
		}
		// Misma línea y a la izquierda.
		if f.Y <= medio+6 && f.Y >= medio-6 && f.X < rect[0] {
			if d := rect[0] - f.X; mejor == "" || d < mejorDist {
				mejor, mejorDist = texto, d
			}
		}
	}
	if mejor != "" {
		return mejor
	}
	for _, f := range fragmentos {
		texto := trimEtiqueta(f.Texto)
		if texto == "" {
			continue
		}
		// Justo encima y solapando en horizontal.
		if f.Y > rect[3] && f.Y < rect[3]+20 && f.X < rect[2] {
			if d := f.Y - rect[3]; mejor == "" || d < mejorDist {
				mejor, mejorDist = texto, d
			}
		}
	}
	return mejor
}

// trimEtiqueta limpia lo que sobra de una etiqueta de impreso: los dos puntos
// del final y los puntos de relleno de una línea de puntos.
func trimEtiqueta(s string) string {
	s = strings.TrimSpace(s)
	for s != "" {
		u := s[len(s)-1]
		if u != ':' && u != '.' && u != ' ' && u != '_' {
			break
		}
		s = s[:len(s)-1]
	}
	s = strings.TrimSpace(s)
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}
