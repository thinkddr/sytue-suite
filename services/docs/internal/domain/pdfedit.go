// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Ola P5 de Sytue PDF: editar el CONTENIDO de un PDF ajeno — su texto, sus
// imágenes y sus capas.
//
// LO PRIMERO, PORQUE ES LO QUE MÁS DECEPCIONA SI SE DESCUBRE TARDE: un PDF no
// guarda párrafos. Guarda «pon estos glifos en estas coordenadas», y el
// documento que los generó ya no está. Eso significa que **no se puede reflujo**:
// si el texto nuevo es más largo que el viejo, no hay línea siguiente a la que
// empujar palabras, porque no hay «líneas», hay posiciones. Cualquier editor que
// prometa lo contrario está reconstruyendo párrafos por heurística y estropeando
// documentos.
//
// Lo que SÍ se puede hacer, y es lo que se hace aquí:
//
//   - **sustituir un fragmento de texto por otro** en su sitio, avisando de si
//     cabe. `Analizar` lo dice ANTES de tocar nada, que es la mitad del valor;
//   - **borrar** texto, dejando el hueco;
//   - **cambiar o quitar una imagen** por su identificador;
//   - **encender y apagar capas** (OCG), que en un plano es media herramienta.
//
// Y la regla que gobierna todo el fichero: la interfaz tiene que decir qué puede
// y qué no ANTES de que el usuario lo intente, no después de estropear un
// documento. Por eso `AnalizarEdicion` existe y devuelve avisos.
package domain

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// FragmentoEditable es un trozo de texto del documento con lo que hace falta
// para sustituirlo con criterio.
type FragmentoEditable struct {
	// ID identifica el fragmento dentro de su página: es el número de operador de
	// texto, contando desde 0. Es estable mientras no se edite la página.
	ID     int     `json:"id"`
	Pagina int     `json:"pagina"`
	Texto  string  `json:"texto"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Tamano float64 `json:"tamano"`
	// AnchoDisponible es lo que mide el fragmento en puntos. Es el presupuesto:
	// un texto nuevo más ancho que esto se saldrá sobre lo que haya al lado.
	AnchoDisponible float64 `json:"ancho_disponible"`
	// FuenteIncrustada avisa de que la fuente viene dentro del documento y puede
	// ser un SUBCONJUNTO: si solo trae los glifos que ya se usaban, escribir una
	// letra nueva no dibujará nada.
	FuenteIncrustada bool `json:"fuente_incrustada"`
	// Subconjunto es cierto cuando el nombre de la fuente lleva el prefijo de seis
	// letras y un «+» que marca un subconjunto (por ejemplo «ABCDEF+Arial»).
	Subconjunto bool `json:"subconjunto"`
	// LetrasDesconocidas: la fuente no dice qué carácter es cada código (sin
	// /ToUnicode ni codificación que lo diga). Se puede borrar, no reescribir.
	LetrasDesconocidas bool `json:"letras_desconocidas,omitempty"`
}

// ErrLetrasDesconocidas es reescribir un texto cuya fuente no dice qué letras
// usa: antes salía «la fuente no tiene la letra «​»», con un carácter invisible
// que nadie entendía (85 casos en el corpus).
var ErrLetrasDesconocidas = errors.New("pdf: la fuente de ese texto no dice qué letras usa, así que no se puede reescribir")

// ImagenEditable es una imagen colocada en una página.
type ImagenEditable struct {
	// ID es el nombre del recurso XObject dentro de la página («Im0»).
	ID          string `json:"id"`
	Pagina      int    `json:"pagina"`
	Ancho       int    `json:"ancho"`
	Alto        int    `json:"alto"`
	EsMascara   bool   `json:"es_mascara"`
	Formato     string `json:"formato"`
	TamanoBytes int    `json:"tamano_bytes"`
	// SinPintar dice que el render dejará ahí un recuadro gris. Se decide
	// INTENTÁNDOLO, no por el nombre del filtro: un JPEG 2000 fuera del alcance
	// del decodificador tampoco se pinta, y callarlo dejaría el hueco mudo otra
	// vez —que es justo lo que este aviso vino a arreglar—.
	SinPintar bool `json:"sin_pintar,omitempty"`
}

// CapaDocumento es un grupo de contenido opcional (OCG), que es como un PDF
// guarda las capas de un plano o los idiomas de un manual.
type CapaDocumento struct {
	// ID es el número de objeto del OCG, que es lo que se usa para encenderlo.
	ID      int    `json:"id"`
	Nombre  string `json:"nombre"`
	Visible bool   `json:"visible"`
}

// AnalisisEdicion es lo que se le enseña al usuario ANTES de dejarle editar.
type AnalisisEdicion struct {
	Fragmentos []FragmentoEditable `json:"fragmentos"`
	Imagenes   []ImagenEditable    `json:"imagenes"`
	Capas      []CapaDocumento     `json:"capas"`
	// Avisos son los límites que aplican a ESTE documento, en cristiano.
	Avisos []string `json:"avisos"`
}

// maxFragmentosPorPagina acota el análisis: una página con cien mil operadores
// de texto existe (un mapa vectorizado) y listarlos todos no ayuda a nadie.
const maxFragmentosPorPagina = 4000

// AnalizarEdicion mira qué se puede editar de una página y con qué límites.
func (d *PDFDoc) AnalizarEdicion(pagina int) (AnalisisEdicion, error) {
	var out AnalisisEdicion
	p, err := d.Pagina(pagina)
	if err != nil {
		return out, err
	}
	contenido, err := d.contenidoDePagina(p)
	if err != nil {
		return out, err
	}

	l := &lienzo{d: d, fuentes: map[string]*fuentePDF{}, caraCSS: map[string]bool{}}
	recursos, _ := d.resolver(p.dict["Resources"]).(pdfDict)
	out.Fragmentos = l.fragmentosEditables(contenido, recursos, pagina)
	out.Imagenes = l.imagenesDePagina(recursos, pagina)
	out.Capas = d.CapasDelDocumento()

	if len(out.Fragmentos) == 0 {
		out.Avisos = append(out.Avisos, "avisoSinTexto")
	}
	for _, f := range out.Fragmentos {
		if f.Subconjunto {
			// Es el aviso que más importa: con una fuente en subconjunto, escribir
			// una letra que el documento no usaba no dibuja NADA.
			out.Avisos = append(out.Avisos, "avisoSubconjunto")
			break
		}
	}
	for _, im := range out.Imagenes {
		if im.SinPintar {
			// El render deja un recuadro gris en su sitio —no finge que ahí no hay
			// nada—, pero un gris sin explicación se lee como un defecto del
			// documento. Decir qué formato es convierte «esto está roto» en «esto
			// no lo sé pintar», que es la verdad y además dice qué hacer:
			// convertir la imagen fuera y volver.
			out.Avisos = append(out.Avisos, "avisoImagenSinPintar")
			break
		}
	}
	out.Avisos = append(out.Avisos, "avisoSinReflujo")
	return out, nil
}

/*
sePuedePintar dice si el render sacará algo de esta imagen.

Se comprueba INTENTÁNDOLO y no mirando el nombre del filtro. Antes bastaba el
nombre porque JPEG 2000 no se pintaba nunca; ahora sí se pinta, pero no
cualquiera —hay ficheros con varios tiles o con estilos de code-block que este
decodificador rechaza a propósito en vez de adivinar—. Preguntar por el nombre
diría que sí y dejaría el recuadro gris sin explicación.

Solo se intenta con JPEG 2000: los demás formatos, o los pinta el navegador, o
ya se decodifican enteros.
*/
func (l *lienzo) sePuedePintar(st *pdfStream, formato string) bool {
	if formato != "JPXDecode" {
		return true
	}
	datos, err := l.d.datosDeFlujo(st)
	if err != nil {
		return false
	}
	_, _, _, err = DecodificarJPX(datos)
	return err == nil
}

// fragmentosEditables recorre el flujo y anota cada operador de texto con su
// posición, su anchura y qué fuente lo pinta.
func (l *lienzo) fragmentosEditables(contenido []byte, recursos pdfDict, pagina int) []FragmentoEditable {
	var out []FragmentoEditable
	e := &ejecucion{l: l, recursos: recursos, estado: estadoInicial(identidad), abiertosN: []int{0}}
	e.pila = append(e.pila, e.estado)

	lx := &pdfLexer{buf: contenido}
	var operandos []pdfObj
	indice := 0
	for len(out) < maxFragmentosPorPagina {
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
			continue
		}
		op := lx.operador()
		if op == "" {
			lx.pos++
			continue
		}

		esTexto := op == "Tj" || op == "TJ" || op == "'" || op == `"`
		if esTexto {
			antesDeMostrar := e.tm
			e.aplicar(op, operandos)
			if f, ok := e.fragmentoDe(operandos, op, antesDeMostrar, indice, pagina); ok {
				out = append(out, f)
			}
			indice++
		} else {
			e.aplicar(op, operandos)
		}
		operandos = operandos[:0]
	}
	return out
}

// fragmentoDe describe el trozo que acaba de pintarse.
func (e *ejecucion) fragmentoDe(operandos []pdfObj, op string, tmAntes matriz, indice, pagina int) (FragmentoEditable, bool) {
	var texto strings.Builder
	f := e.estado.fuente
	if f == nil {
		f = fuenteDeReserva()
	}
	recoger := func(s pdfString) {
		for _, g := range f.decodificar(s) {
			texto.WriteString(g.texto)
		}
	}
	switch op {
	case "TJ":
		if len(operandos) > 0 {
			if arr, ok := e.l.d.resolver(operandos[len(operandos)-1]).(pdfArray); ok {
				for _, el := range arr {
					if s, ok := el.(pdfString); ok {
						recoger(s)
					}
				}
			}
		}
	default:
		recoger(ultimaCadena(operandos))
	}
	t := strings.TrimSpace(texto.String())
	if t == "" {
		return FragmentoEditable{}, false
	}

	// La anchura es cuánto avanzó la matriz de texto al pintarlo.
	ancho := e.tm[4] - tmAntes[4]
	if ancho < 0 {
		ancho = -ancho
	}
	return FragmentoEditable{
		ID:                 indice,
		Pagina:             pagina,
		Texto:              texto.String(),
		X:                  tmAntes[4],
		Y:                  tmAntes[5],
		Tamano:             e.estado.tamano,
		AnchoDisponible:    ancho,
		FuenteIncrustada:   strings.HasPrefix(f.familia, `"sypdf`),
		Subconjunto:        f.subconjunto,
		LetrasDesconocidas: strings.Contains(texto.String(), "\u200b"),
	}, true
}

// imagenesDePagina lista los XObjects de imagen que la página referencia.
func (l *lienzo) imagenesDePagina(recursos pdfDict, pagina int) []ImagenEditable {
	if recursos == nil {
		return nil
	}
	cat, _ := l.d.resolver(recursos["XObject"]).(pdfDict)
	if cat == nil {
		return nil
	}
	var out []ImagenEditable
	for _, nombre := range clavesOrdenadas(cat) {
		st, ok := l.d.resolver(cat[nombre]).(*pdfStream)
		if !ok {
			continue
		}
		if sub, _ := l.d.resolver(st.Dict["Subtype"]).(pdfName); sub != "Image" {
			continue
		}
		ancho, alto := l.tamanoImagen(st.Dict)
		formato := "crudo"
		for _, f := range l.d.listaDeNombres(st.Dict["Filter"]) {
			if esFiltroDeImagen(f) {
				formato = string(f)
			}
		}
		out = append(out, ImagenEditable{
			ID: string(nombre), Pagina: pagina, Ancho: ancho, Alto: alto,
			EsMascara: esMascara(l, st.Dict), Formato: formato, TamanoBytes: len(st.Raw),
			SinPintar: !l.sePuedePintar(st, formato),
		})
	}
	return out
}

// --- Sustituir texto ---------------------------------------------------------

// CambioDeTexto pide sustituir un fragmento por otro texto.
type CambioDeTexto struct {
	Pagina int    `json:"pagina"`
	ID     int    `json:"id"`
	Nuevo  string `json:"nuevo"`
}

// EditarTextoPDF sustituye los fragmentos indicados.
//
// Se reescribe el flujo de contenido de la página cambiando SOLO la cadena de
// esos operadores y ajustando su espaciado para que el texto nuevo ocupe lo
// mismo que el viejo. Todo lo demás del flujo se copia byte a byte: es lo que
// evita que editar una palabra mueva una tabla tres páginas más allá.
func EditarTextoPDF(doc *PDFDoc, cambios []CambioDeTexto) ([]byte, error) {
	if len(cambios) == 0 {
		return nil, fmt.Errorf("pdf: no se ha indicado ningún cambio")
	}
	porPagina := map[int]map[int]string{}
	for _, c := range cambios {
		if c.Pagina < 0 || c.Pagina >= doc.NumPaginas() {
			return nil, fmt.Errorf("pdf: la página %d no existe", c.Pagina+1)
		}
		if porPagina[c.Pagina] == nil {
			porPagina[c.Pagina] = map[int]string{}
		}
		porPagina[c.Pagina][c.ID] = c.Nuevo
	}

	e := NuevoEnsambladorPDF()
	for i := 0; i < doc.NumPaginas(); i++ {
		nuevos := porPagina[i]
		if len(nuevos) == 0 {
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
			return nil, err
		}
		reescrito, tocados, err := reescribirTexto(doc, contenido, p, nuevos)
		if err != nil {
			return nil, fmt.Errorf("pdf: página %d: %w", i+1, err)
		}
		if tocados == 0 {
			return nil, fmt.Errorf("pdf: en la página %d no se encontró el texto a cambiar", i+1)
		}
		if err := e.anadirPagina(doc, i, 0, opcionesPagina{contenido: reescrito}); err != nil {
			return nil, err
		}
	}
	e.ConservarFormulario(doc)
	return e.Bytes()
}

// reescribirTexto copia el flujo cambiando las cadenas de los operadores
// indicados. Devuelve cuántos se cambiaron de verdad.
// forma de operador de texto.
//
//nolint:gocyclo // Recorre el flujo de contenido token a token: un caso por
func reescribirTexto(doc *PDFDoc, contenido []byte, p PDFPagina, nuevos map[int]string) ([]byte, int, error) {
	l := &lienzo{d: doc, fuentes: map[string]*fuentePDF{}, caraCSS: map[string]bool{}}
	recursos, _ := doc.resolver(p.dict["Resources"]).(pdfDict)
	e := &ejecucion{l: l, recursos: recursos, estado: estadoInicial(identidad), abiertosN: []int{0}}
	e.pila = append(e.pila, e.estado)

	var out []byte
	lx := &pdfLexer{buf: contenido}
	var operandos []pdfObj
	var inicios []int
	indice, tocados := 0, 0
	copiadoHasta := 0

	for {
		lx.saltarEspacios()
		if lx.pos >= len(lx.buf) {
			break
		}
		c := lx.buf[lx.pos]
		if c == '/' || c == '(' || c == '<' || c == '[' || c == '+' || c == '-' || c == '.' ||
			(c >= '0' && c <= '9') {
			antes := lx.pos
			o, err := doc.parsearObjeto(lx)
			if err != nil || lx.pos == antes {
				lx.pos = antes + 1
				continue
			}
			operandos = append(operandos, o)
			inicios = append(inicios, antes)
			continue
		}
		inicioOp := lx.pos
		op := lx.operador()
		if op == "" {
			lx.pos++
			continue
		}

		esTexto := op == "Tj" || op == "TJ" || op == "'" || op == `"`
		if !esTexto {
			e.aplicar(op, operandos)
			operandos = operandos[:0]
			inicios = inicios[:0]
			continue
		}

		antesDeMostrar := e.tm
		e.aplicar(op, operandos)
		f, ok := e.fragmentoDe(operandos, op, antesDeMostrar, indice, 0)
		if ok {
			if nuevo, hay := nuevos[indice]; hay {
				inicioOperandos := inicioOperandosTexto(inicios, copiadoHasta, inicioOp, op)
				sust, err := sustitucionDeTexto(nuevo, f, e.estado, op)
				if err != nil {
					return nil, 0, err
				}
				out = append(out, contenido[copiadoHasta:inicioOperandos]...)
				out = append(out, []byte(sust)...)
				copiadoHasta = lx.pos
				tocados++
			}
		}
		// Se cuenta CADA operador de texto, como fragmentosEditables: si solo se
		// contaran los que dan fragmento, un `() Tj` vacío delante desplazaba los
		// números y se cambiaba otro texto, o ninguno («no se encontró el texto»).
		indice++
		operandos = operandos[:0]
		inicios = inicios[:0]
	}
	out = append(out, contenido[copiadoHasta:]...)
	return out, tocados, nil
}

// inicioOperandosTexto devuelve la posición donde empieza el primer operando de
// un operador de texto, tomada del propio lexer al leer cada operando.
// Retroceder byte a byte desde el operador tropezaba con paréntesis escapados
// «\(» dentro de la cadena y dejaba un prefijo sin cerrar (BOE y issue14497).
func inicioOperandosTexto(inicios []int, copiadoHasta, inicioOp int, op string) int {
	inicio := inicioOp
	n := 1
	if op == `"` {
		n = 3 // `aw ac (cadena) "`: los dos números también son suyos.
	}
	if len(inicios) > 0 {
		inicio = inicios[max(0, len(inicios)-n)]
	}
	return max(copiadoHasta, inicio)
}

// sustitucionDeTexto escribe el operador nuevo. El texto se ajusta al ANCHO del
// original con espaciado entre caracteres (`Tc`), que es la única forma de no
// desplazar lo que venga detrás en la misma línea.
//
// La cadena se escribe en la codificación DE LA FUENTE, no en WinAnsi: en una
// fuente compuesta cada letra son dos bytes, y en un subconjunto cada código
// es el que el generador quiso. Escribir WinAnsi ahí pintaba otros glifos, o
// ninguno (medido con scripts/certipdf-compat). Si la fuente no tiene alguna
// letra, se dice en vez de escribir algo que no se va a ver.
func sustitucionDeTexto(nuevo string, f FragmentoEditable, st estadoG, op string) (string, error) {
	// Separador delante: un flujo compacto puede traer el operador anterior
	// pegado a la cadena («/F1 9 Tf(abc)Tj»), y sin él lo nuevo se le uniría.
	var b strings.Builder
	b.WriteByte('\n')
	// `'` y `"` saltan de línea antes de pintar; el Tj que los sustituye no, así
	// que el salto (y los espaciados de `"`) se escriben aparte.
	switch op {
	case "'":
		b.WriteString("T* ")
	case `"`:
		b.WriteString(numPDF(st.espPal) + " Tw " + numPDF(st.espCar) + " Tc T* ")
	}
	if nuevo == "" {
		// Borrar: se emite una cadena vacía. El hueco se queda, que es honesto —el
		// contenido de al lado NO se mueve, porque en un PDF no hay reflujo.
		b.WriteString("() Tj\n")
		return b.String(), nil
	}
	fuente := st.fuente
	if fuente == nil {
		fuente = fuenteDeReserva()
	}
	cadena, err := fuente.codificar(nuevo)
	if err != nil {
		return "", err
	}

	// Anchura del texto nuevo con las métricas de la fuente vigente.
	anchoNuevo := 0.0
	glifos := fuente.decodificar(cadena)
	for _, g := range glifos {
		anchoNuevo += g.ancho / 1000 * st.tamano
	}
	// El ajuste se reparte entre los caracteres: encoger un poco es invisible,
	// encoger mucho se ve, y por eso `AnalizarEdicion` avisa antes.
	ajustado := false
	if n := len(glifos); n > 1 && f.AnchoDisponible > 0 && anchoNuevo > 0 {
		ajuste := (f.AnchoDisponible - anchoNuevo) / float64(n)
		b.WriteString(numPDF(st.espCar+ajuste) + " Tc ")
		ajustado = true
	}
	b.WriteString("<" + hex.EncodeToString(cadena) + "> Tj")
	if ajustado {
		// Se devuelve el espaciado que había, no cero: el texto que sigue lo usa.
		b.WriteString(" " + numPDF(st.espCar) + " Tc")
	}
	b.WriteString("\n")
	return b.String(), nil
}

// --- Imágenes ----------------------------------------------------------------

// SustituirImagenPDF cambia una imagen de una página por otra, o la quita si
// `nueva` viene vacía.
//
// Se conserva el HUECO: la imagen nueva se estira al mismo rectángulo que
// ocupaba la anterior, porque quien la coloca es el flujo de contenido y ese no
// se toca. Es lo que hace que sustituir un logotipo no descuadre la cabecera.
func SustituirImagenPDF(doc *PDFDoc, pagina int, id string, nueva []byte) ([]byte, error) {
	p, err := doc.Pagina(pagina)
	if err != nil {
		return nil, err
	}
	recursos, _ := doc.resolver(p.dict["Resources"]).(pdfDict)
	cat, _ := doc.resolver(recursos["XObject"]).(pdfDict)
	if cat == nil {
		return nil, fmt.Errorf("pdf: la página %d no tiene imágenes", pagina+1)
	}
	ref, hay := cat[pdfName(id)]
	if !hay {
		return nil, fmt.Errorf("pdf: en la página %d no hay ninguna imagen «%s»", pagina+1, id)
	}
	num := numDeRef(ref)
	if num <= 0 {
		return nil, fmt.Errorf("pdf: esa imagen no se puede sustituir")
	}

	e := NuevoEnsambladorPDF()
	if len(nueva) == 0 {
		// Quitar: se sustituye por una imagen de un píxel transparente. Borrar el
		// objeto dejaría un `Do` apuntando a nada, que en algunos lectores es un
		// error de sintaxis y en otros un rectángulo negro.
		e.Parchear(doc, num, func(_ *PDFEnsamblador, dst pdfDict) {
			dst["Width"], dst["Height"] = int64(1), int64(1)
			dst["ColorSpace"] = pdfName("DeviceGray")
			dst["BitsPerComponent"] = int64(8)
			delete(dst, "Filter")
			delete(dst, "DecodeParms")
			delete(dst, "SMask")
			delete(dst, "Decode")
		})
		e.reemplazos = map[int][]byte{num: {0xFF}}
	} else {
		cfg, formato, err := decodificarCabeceraImagen(nueva)
		if err != nil {
			return nil, fmt.Errorf("pdf: la imagen nueva no se puede leer")
		}
		datos, dict, err := imagenComoXObject(nueva, cfg, formato)
		if err != nil {
			return nil, err
		}
		e.Parchear(doc, num, func(_ *PDFEnsamblador, dst pdfDict) {
			for _, k := range clavesOrdenadas(dst) {
				delete(dst, k)
			}
			for k, v := range dict {
				dst[k] = v
			}
		})
		e.reemplazos = map[int][]byte{num: datos}
	}

	for i := 0; i < doc.NumPaginas(); i++ {
		if err := e.AnadirPagina(doc, i, 0); err != nil {
			return nil, err
		}
	}
	e.ConservarFormulario(doc)
	return e.Bytes()
}

// --- Capas (OCG) -------------------------------------------------------------

// CapasDelDocumento lista los grupos de contenido opcional con su estado.
func (d *PDFDoc) CapasDelDocumento() []CapaDocumento {
	raiz, _ := d.resolver(d.trailer["Root"]).(pdfDict)
	if raiz == nil {
		return nil
	}
	props, _ := d.resolver(raiz["OCProperties"]).(pdfDict)
	if props == nil {
		return nil
	}
	apagadas := map[int]bool{}
	if def, ok := d.resolver(props["D"]).(pdfDict); ok {
		if off, ok := d.resolver(def["OFF"]).(pdfArray); ok {
			for _, o := range off {
				if n := numDeRef(o); n > 0 {
					apagadas[n] = true
				}
			}
		}
	}
	todos, _ := d.resolver(props["OCGs"]).(pdfArray)
	out := make([]CapaDocumento, 0, len(todos))
	for _, o := range todos {
		num := numDeRef(o)
		g, ok := d.resolver(o).(pdfDict)
		if !ok || num <= 0 {
			continue
		}
		nombre := ""
		if s, ok := d.resolver(g["Name"]).(pdfString); ok {
			nombre = decodificarTextoPDF(s)
		}
		out = append(out, CapaDocumento{ID: num, Nombre: nombre, Visible: !apagadas[num]})
	}
	return out
}

// CambiarCapasPDF enciende y apaga capas. `visibles` son los identificadores que
// quedan encendidos; el resto se apagan.
//
// Se cambia la configuración por DEFECTO del documento (/OCProperties /D /OFF),
// que es lo que lee cualquier visor al abrirlo. El contenido de las capas
// apagadas SIGUE dentro del fichero: apagar una capa no es censurar, y quien
// necesite lo segundo tiene la redacción.
func CambiarCapasPDF(doc *PDFDoc, visibles []int) ([]byte, error) {
	capas := doc.CapasDelDocumento()
	if len(capas) == 0 {
		return nil, fmt.Errorf("pdf: este documento no tiene capas")
	}
	encendidas := make(map[int]bool, len(visibles))
	for _, v := range visibles {
		encendidas[v] = true
	}

	e := NuevoEnsambladorPDF()
	for i := 0; i < doc.NumPaginas(); i++ {
		if err := e.AnadirPagina(doc, i, 0); err != nil {
			return nil, err
		}
	}
	e.ConservarFormulario(doc)

	raiz, _ := doc.resolver(doc.trailer["Root"]).(pdfDict)
	props, _ := doc.resolver(raiz["OCProperties"]).(pdfDict)
	copia := pdfDict{}
	for _, k := range clavesOrdenadas(props) {
		copia[k] = e.copiarValor(doc, props[k])
	}
	apagadas := pdfArray{}
	todos := pdfArray{}
	for _, c := range capas {
		ref := pdfRef{Num: e.copiarObjeto(doc, c.ID)}
		todos = append(todos, ref)
		if !encendidas[c.ID] {
			apagadas = append(apagadas, ref)
		}
	}
	copia["OCGs"] = todos
	def := pdfDict{"Order": todos, "OFF": apagadas}
	if d0, ok := doc.resolver(props["D"]).(pdfDict); ok {
		if nombre, ok := d0["Name"]; ok {
			def["Name"] = e.copiarValor(doc, nombre)
		}
	}
	copia["D"] = def

	if e.extrasCatalogo == nil {
		e.extrasCatalogo = pdfDict{}
	}
	e.extrasCatalogo["OCProperties"] = copia
	return e.Bytes()
}
