// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Creación de campos de formulario sobre un PDF que no los tenía.
//
// Es lo que convierte un impreso escaneado en algo que se rellena en pantalla, y
// hasta ahora Sytue solo sabía leer y rellenar campos ajenos.
//
// Se escribe por ANEXO (pdfsignincr.go) y no reescribiendo el fichero: si el
// documento ya venía firmado, la firma sigue verificando su rango. El fichero
// crece, que es el precio correcto por no estropear lo que ya había.
package domain

import (
	"fmt"
	"strings"
)

// CampoNuevo describe un campo a crear. Rect va en coordenadas de la PÁGINA
// (puntos, origen abajo a la izquierda), igual que todo lo demás del motor.
type CampoNuevo struct {
	Tipo     TipoCampo  `json:"tipo"`
	Nombre   string     `json:"nombre"`
	Pagina   int        `json:"pagina"`
	Rect     [4]float64 `json:"rect"`
	Valor    string     `json:"valor,omitempty"`
	Opciones []string   `json:"opciones,omitempty"`

	Etiqueta     string `json:"etiqueta,omitempty"`
	Obligatorio  bool   `json:"obligatorio,omitempty"`
	SoloLectura  bool   `json:"solo_lectura,omitempty"`
	MaxLongitud  int    `json:"max_longitud,omitempty"`
	VariasLineas bool   `json:"varias_lineas,omitempty"`
	Contrasena   bool   `json:"contrasena,omitempty"`
}

// Banderas de /Ff (§12.7.3.1 y §12.7.4). Se nombran para que el que lea el
// código no tenga que buscar qué es 4096.
const (
	ffSoloLectura  = 1 << 0  // 1
	ffObligatorio  = 1 << 1  // 2
	ffRadio        = 1 << 15 // 32768
	ffVariasLineas = 1 << 12 // 4096
	ffContrasena   = 1 << 13 // 8192
	ffDesplegable  = 1 << 17 // 131072, /Ch con caja de edición
)

// AnadirCamposPDF crea los campos indicados y devuelve el documento resultante.
func AnadirCamposPDF(doc *PDFDoc, campos []CampoNuevo) ([]byte, error) {
	if doc == nil || len(doc.buf) == 0 {
		return nil, fmt.Errorf("%w: no hay fichero de partida", ErrNoIncremental)
	}
	if len(campos) == 0 {
		return nil, fmt.Errorf("pdf: no se indicó ningún campo que crear")
	}
	if doc.cripto != nil {
		return nil, fmt.Errorf("%w: está cifrado", ErrNoIncremental)
	}
	if doc.reparado {
		return nil, fmt.Errorf("%w: la tabla de referencias estaba rota", ErrNoIncremental)
	}
	raizRef, ok := doc.trailer["Root"].(pdfRef)
	if !ok {
		return nil, fmt.Errorf("%w: el /Root no es una referencia", ErrNoIncremental)
	}
	raiz, ok := doc.resolver(raizRef).(pdfDict)
	if !ok {
		return nil, fmt.Errorf("%w: el catálogo no se pudo leer", ErrNoIncremental)
	}
	xrefAnterior, err := offsetDelUltimoXref(doc.buf)
	if err != nil {
		return nil, err
	}
	usados := nombresUsados(doc)

	siguiente := doc.maxObjeto() + 1
	reservar := func() int { n := siguiente; siguiente++; return n }
	nuevos := map[int]objetoSalida{}
	porPagina := map[int][]int{} // página → widgets nuevos
	var refsCampos []pdfObj

	for i, c := range campos {
		if c.Pagina < 0 || c.Pagina >= doc.NumPaginas() {
			return nil, fmt.Errorf("pdf: el campo %d va a la página %d, que no existe", i+1, c.Pagina+1)
		}
		if c.Rect[2]-c.Rect[0] <= 1 || c.Rect[3]-c.Rect[1] <= 1 {
			return nil, fmt.Errorf("pdf: el campo %q no tiene tamaño", c.Nombre)
		}
		pag := doc.paginas[c.Pagina]
		if pag.num == 0 {
			return nil, fmt.Errorf("%w: no se sabe qué objeto es la página %d", ErrNoIncremental, c.Pagina+1)
		}
		// Dos campos con el mismo /T son el MISMO campo para un lector: escribir en
		// uno cambiaría el otro. Se desambigua aquí en vez de dejarlo pasar.
		c.Nombre = nombreLibre(c.Nombre, i, usados)
		usados[c.Nombre] = true

		num := reservar()
		dict, ap := construirCampo(c, pag.num)
		if ap != nil {
			numAP := reservar()
			nuevos[numAP] = *ap
			dict["AP"] = pdfDict{"N": pdfRef{Num: numAP}}
		}
		nuevos[num] = objetoSalida{valor: dict}
		refsCampos = append(refsCampos, pdfRef{Num: num})
		porPagina[pag.num] = append(porPagina[pag.num], num)
	}

	// Cada página tocada se reescribe una sola vez con TODOS sus widgets nuevos:
	// hacerlo por campo dejaría varias versiones del mismo objeto en el anexo y
	// solo valdría la última, perdiendo los demás campos de esa página.
	for numPag, widgets := range porPagina {
		nodo, ok := doc.resolver(pdfRef{Num: numPag}).(pdfDict)
		if !ok {
			return nil, fmt.Errorf("%w: la página %d no es un diccionario", ErrNoIncremental, numPag)
		}
		pagina := copiaLlana(nodo)
		annots := arrayResuelto(doc, nodo["Annots"])
		for _, w := range widgets {
			annots = append(annots, pdfRef{Num: w})
		}
		pagina["Annots"] = annots
		nuevos[numPag] = objetoSalida{valor: pagina}
	}

	catalogo := copiaLlana(raiz)
	catalogo["AcroForm"] = acroFormConCampos(doc, raiz, refsCampos)
	nuevos[raizRef.Num] = objetoSalida{valor: catalogo}

	return escribirAnexo(doc, nuevos, siguiente, raizRef, xrefAnterior, nil)
}

// construirCampo devuelve el diccionario del campo —que es a la vez su widget,
// porque hay uno solo— y su apariencia, si la tiene.
func construirCampo(c CampoNuevo, numPag int) (pdfDict, *objetoSalida) {
	ff := int64(0)
	if c.SoloLectura {
		ff |= ffSoloLectura
	}
	if c.Obligatorio {
		ff |= ffObligatorio
	}

	dict := pdfDict{
		"Type":    pdfName("Annot"),
		"Subtype": pdfName("Widget"),
		"T":       cadenaUTF16(c.Nombre),
		"Rect":    rectPDF(c.Rect),
		"P":       pdfRef{Num: numPag},
		"F":       int64(4), // impreso: sin esto no sale al imprimir el formulario
		// /MK lo usa el lector para redibujar el campo cuando el usuario escribe;
		// la apariencia de abajo es para verlo antes de que ningún lector lo toque.
		"MK": pdfDict{
			"BC": pdfArray{0.45, 0.47, 0.5},
			"BG": pdfArray{0.98, 0.98, 0.98},
		},
		"DA": pdfString("0 0 0 rg /SytueCampoF 0 Tf"),
	}
	if c.Etiqueta != "" {
		dict["TU"] = cadenaUTF16(c.Etiqueta) // texto de ayuda al pasar por encima
	}

	switch c.Tipo {
	case CampoCasilla, CampoOpcion:
		dict["FT"] = pdfName("Btn")
		if c.Tipo == CampoOpcion {
			ff |= ffRadio
		}
		estado := pdfName("Off")
		if esAfirmativo(c.Valor) {
			estado = pdfName("Yes")
		}
		dict["V"], dict["AS"] = estado, estado
		dict["AP"] = pdfDict{"N": pdfDict{
			"Yes": marcaDeCasilla(c.Rect, true),
			"Off": marcaDeCasilla(c.Rect, false),
		}}
		dict["Ff"] = ff
		return dict, nil

	case CampoLista, CampoDesplegable:
		dict["FT"] = pdfName("Ch")
		if c.Tipo == CampoDesplegable {
			ff |= ffDesplegable
		}
		opciones := make(pdfArray, 0, len(c.Opciones))
		for _, o := range c.Opciones {
			opciones = append(opciones, cadenaUTF16(o))
		}
		dict["Opt"] = opciones

	case CampoFirma:
		dict["FT"] = pdfName("Sig")
		dict["Ff"] = ff
		// Un campo de firma vacío no dibuja valor: solo su recuadro.
		return dict, nil

	default: // CampoTexto y cualquier cosa que no reconozcamos
		dict["FT"] = pdfName("Tx")
		if c.VariasLineas {
			ff |= ffVariasLineas
		}
		if c.Contrasena {
			ff |= ffContrasena
		}
		if c.MaxLongitud > 0 {
			dict["MaxLen"] = int64(c.MaxLongitud)
		}
	}

	dict["Ff"] = ff
	if c.Valor != "" {
		dict["V"] = cadenaUTF16(c.Valor)
	}
	vista := CampoFormulario{
		Tipo: c.Tipo, Rect: c.Rect,
		VariasLineas: c.VariasLineas, Contrasena: c.Contrasena,
	}
	// El recuadro sale del /MK que se acaba de escribir arriba, no de una bandera:
	// así el campo se dibuja igual cuando se crea y cuando alguien lo rellena, que
	// es cuando la apariencia se regenera desde cero.
	ap, ok := aparienciaDeCampo(dict, vista, c.Valor, "0 0 0 rg /SytueCampoF 0 Tf", marcoDeMK(dict))
	if !ok {
		return dict, nil
	}
	return dict, &ap
}

// marcaDeCasilla dibuja los dos estados de una casilla: el recuadro solo, y el
// recuadro con la marca dentro. Van como flujo INLINE dentro del /AP para no
// gastar dos objetos por casilla.
func marcaDeCasilla(rect [4]float64, marcada bool) *pdfStream {
	ancho, alto := rect[2]-rect[0], rect[3]-rect[1]
	var b strings.Builder
	fmt.Fprintf(&b, "q 0.98 0.98 0.98 rg 0 0 %s %s re f\n", numPDF(ancho), numPDF(alto))
	fmt.Fprintf(&b, "0.45 0.47 0.5 RG 1 w 0.5 0.5 %s %s re S\n", numPDF(ancho-1), numPDF(alto-1))
	if marcada {
		// Una uve de trazo grueso, proporcional a la caja: se lee igual en una
		// casilla de 10 puntos que en una de 30.
		g := ancho * 0.16
		fmt.Fprintf(&b, "0.06 0.09 0.16 RG %s w 1 J 1 j\n", numPDF(g))
		fmt.Fprintf(&b, "%s %s m %s %s l %s %s l S\n",
			numPDF(ancho*0.22), numPDF(alto*0.52),
			numPDF(ancho*0.43), numPDF(alto*0.28),
			numPDF(ancho*0.78), numPDF(alto*0.72))
	}
	b.WriteString("Q\n")
	return &pdfStream{
		Dict: pdfDict{
			"Type":      pdfName("XObject"),
			"Subtype":   pdfName("Form"),
			"FormType":  int64(1),
			"BBox":      pdfArray{0.0, 0.0, ancho, alto},
			"Resources": pdfDict{"ColorSpace": pdfDict{"DefaultRGB": calRGBDefecto()}},
		},
		Raw: []byte(b.String()),
	}
}

// acroFormConCampos añade los campos al /AcroForm conservando lo que hubiera.
func acroFormConCampos(doc *PDFDoc, raiz pdfDict, refs []pdfObj) pdfDict {
	var out pdfDict
	if ya, ok := doc.resolver(raiz["AcroForm"]).(pdfDict); ok {
		out = copiaLlana(ya)
	} else {
		out = pdfDict{}
	}
	out["Fields"] = append(arrayResuelto(doc, out["Fields"]), refs...)
	// /DR es el catálogo de recursos que los /DA de los campos pueden nombrar.
	// Sin la fuente aquí, un lector que regenere la apariencia no sabe qué es
	// /SytueCampoF y dibuja el texto con lo que le parezca, o nada.
	dr, _ := doc.resolver(out["DR"]).(pdfDict)
	recursos := copiaLlana(dr)
	fuentes, _ := doc.resolver(recursos["Font"]).(pdfDict)
	fuentesNuevas := copiaLlana(fuentes)
	fuentesNuevas["SytueCampoF"] = fuenteDeCampo()
	recursos["Font"] = fuentesNuevas
	out["DR"] = recursos
	if _, ya := out["DA"]; !ya {
		out["DA"] = pdfString("0 0 0 rg /SytueCampoF 0 Tf")
	}
	return out
}

// nombresUsados recoge los nombres de campo que ya hay, para no chocar.
func nombresUsados(doc *PDFDoc) map[string]bool {
	out := map[string]bool{}
	for _, c := range doc.CamposDelFormulario() {
		out[c.Nombre] = true
	}
	return out
}

// nombreLibre devuelve un nombre que no choque con los ya usados. Un campo sin
// nombre recibe uno: sin /T no se puede rellenar ni leer su valor.
func nombreLibre(nombre string, i int, usados map[string]bool) string {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		nombre = fmt.Sprintf("campo%d", i+1)
	}
	if !usados[nombre] {
		return nombre
	}
	for n := 2; ; n++ {
		cand := fmt.Sprintf("%s_%d", nombre, n)
		if !usados[cand] {
			return cand
		}
	}
}
