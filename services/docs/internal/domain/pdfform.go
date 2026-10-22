// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Formularios AcroForm (ola P3 de Sytue PDF): detectar los campos, rellenarlos,
// aplanarlos y firmar.
//
// ES EL CASO EMPRESARIAL QUE MÁS SE PIDE. Un parte de baja, una hoja de gastos o
// un contrato llegan como PDF con campos, y hoy la única salida de mucha gente
// es imprimir, escribir a bolígrafo y escanear. Que la suite lo rellene y lo
// devuelva firmado ahorra ese viaje entero.
//
// UN CAMPO NO ES UNA COSA, SON DOS. El CAMPO guarda el nombre y el valor; el
// WIDGET es el rectángulo que se ve en una página. Casi siempre van fundidos en
// el mismo objeto, pero un campo que aparece en varias páginas —«firma aquí» al
// pie de cada una— es un campo con varios widgets, y confundirlos hace que
// rellenar uno no rellene el resto. Aquí se distinguen siempre.
//
// Y LAS APARIENCIAS SE GENERAN. Se podría poner /NeedAppearances y dejar que las
// dibuje el lector, que es lo que hacen muchas herramientas; el resultado es un
// PDF que se ve relleno en Acrobat y VACÍO en el visor del móvil, del navegador
// y al imprimir. Aquí se escribe el dibujo, así que lo relleno se ve relleno en
// todas partes.
package domain

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// TipoCampo es la clase de campo, con los nombres del producto.
type TipoCampo string

const (
	CampoTexto       TipoCampo = "texto"
	CampoCasilla     TipoCampo = "casilla"
	CampoOpcion      TipoCampo = "opcion" // botón de radio
	CampoLista       TipoCampo = "lista"
	CampoDesplegable TipoCampo = "desplegable"
	CampoBoton       TipoCampo = "boton"
	CampoFirma       TipoCampo = "firma"
)

// CampoFormulario es un campo tal y como lo ve la interfaz.
type CampoFormulario struct {
	Nombre      string     `json:"nombre"`
	Etiqueta    string     `json:"etiqueta,omitempty"`
	Tipo        TipoCampo  `json:"tipo"`
	Valor       string     `json:"valor,omitempty"`
	Opciones    []string   `json:"opciones,omitempty"`
	Pagina      int        `json:"pagina"`
	Rect        [4]float64 `json:"rect"`
	Obligatorio bool       `json:"obligatorio"`
	SoloLectura bool       `json:"solo_lectura"`
	MaxLongitud int        `json:"max_longitud,omitempty"`
	// VariasLineas y Contrasena solo aplican a los de texto.
	VariasLineas bool `json:"varias_lineas,omitempty"`
	Contrasena   bool `json:"contrasena,omitempty"`
	// EstadoActivado es el nombre interno del estado «marcado» de una casilla, que
	// NO siempre es «Yes»: cada generador elige el suyo y ponerlo mal deja la
	// casilla marcada en el dato y vacía en la pantalla.
	EstadoActivado string `json:"estado_activado,omitempty"`
}

// maxProfundidadCampos corta un árbol de campos con un ciclo.
const maxProfundidadCampos = 32

// CamposDelFormulario devuelve los campos del documento, en el orden del árbol.
func (d *PDFDoc) CamposDelFormulario() []CampoFormulario {
	raiz, _ := d.resolver(d.trailer["Root"]).(pdfDict)
	if raiz == nil {
		return nil
	}
	forma, _ := d.resolver(raiz["AcroForm"]).(pdfDict)
	if forma == nil {
		return nil
	}
	campos, _ := d.resolver(forma["Fields"]).(pdfArray)
	if len(campos) == 0 {
		return nil
	}
	paginaDe := d.indicePorWidget()

	var out []CampoFormulario
	for _, c := range campos {
		d.recorrerCampos(c, "", pdfDict{}, paginaDe, &out, 0)
	}
	return out
}

// indicePorWidget mapea cada objeto anotación a su página, para poder decir en
// qué página está un campo sin recorrer el documento por cada uno.
func (d *PDFDoc) indicePorWidget() map[int]int {
	out := map[int]int{}
	for i := 0; i < d.NumPaginas(); i++ {
		p, err := d.Pagina(i)
		if err != nil {
			continue
		}
		anots, _ := d.resolver(p.dict["Annots"]).(pdfArray)
		for _, a := range anots {
			if ref, ok := a.(pdfRef); ok {
				out[ref.Num] = i
			}
		}
	}
	return out
}

// atributosHeredadosDeCampo son los que un campo padre pasa a sus hijos: el
// tipo, las banderas, el valor por defecto y la cadena de apariencia.
var atributosHeredadosDeCampo = []pdfName{"FT", "Ff", "V", "DV", "DA", "Opt", "MaxLen", "Q"}

// (§12.7.3) y cada tipo de campo lee cosas distintas.
//
//nolint:gocognit,gocyclo // El árbol de campos de AcroForm hereda atributos por rama
func (d *PDFDoc) recorrerCampos(ref pdfObj, prefijo string, heredado pdfDict,
	paginaDe map[int]int, out *[]CampoFormulario, prof int,
) {
	if prof > maxProfundidadCampos || len(*out) > 5000 {
		return
	}
	dict, ok := d.resolver(ref).(pdfDict)
	if !ok {
		return
	}

	efectivo := pdfDict{}
	for k, v := range heredado {
		efectivo[k] = v
	}
	for _, k := range atributosHeredadosDeCampo {
		if v, hay := dict[k]; hay {
			efectivo[k] = v
		}
	}

	nombre := prefijo
	if t, ok := d.resolver(dict["T"]).(pdfString); ok {
		// El nombre completo se construye con puntos, que es como lo referencian
		// el JavaScript del documento y cualquier herramienta de relleno.
		if nombre != "" {
			nombre += "."
		}
		nombre += decodificarTextoPDF(t)
	}

	hijos, tieneHijos := d.resolver(dict["Kids"]).(pdfArray)
	if tieneHijos {
		// Un nodo con hijos puede ser un campo con varios widgets (los hijos son
		// widgets, sin /T) o un grupo de campos (los hijos tienen /T). Se distingue
		// mirando si el primer hijo tiene nombre propio.
		soloWidgets := true
		for _, h := range hijos {
			if hd, ok := d.resolver(h).(pdfDict); ok {
				if _, tiene := hd["T"]; tiene {
					soloWidgets = false
					break
				}
			}
		}
		if !soloWidgets {
			for _, h := range hijos {
				d.recorrerCampos(h, nombre, efectivo, paginaDe, out, prof+1)
			}
			return
		}
	}

	tipo, _ := d.resolver(efectivo["FT"]).(pdfName)
	if tipo == "" {
		return // un nodo intermedio sin tipo no es un campo
	}

	c := CampoFormulario{Nombre: nombre, Tipo: tipoDeCampo(tipo, d.banderas(efectivo))}
	banderas := d.banderas(efectivo)
	c.SoloLectura = banderas&1 != 0
	c.Obligatorio = banderas&2 != 0
	c.VariasLineas = tipo == "Tx" && banderas&(1<<12) != 0
	c.Contrasena = tipo == "Tx" && banderas&(1<<13) != 0
	if tu, ok := d.resolver(dict["TU"]).(pdfString); ok {
		c.Etiqueta = decodificarTextoPDF(tu)
	}
	if v, ok := comoEntero(d.resolver(efectivo["MaxLen"])); ok {
		c.MaxLongitud = int(v)
	}
	c.Valor = d.valorDeCampo(efectivo["V"])
	c.Opciones = d.opcionesDeCampo(efectivo["Opt"])

	// El widget: o es este mismo objeto, o es su primer hijo.
	widget, numWidget := dict, numDeRef(ref)
	if tieneHijos && len(hijos) > 0 {
		if hd, ok := d.resolver(hijos[0]).(pdfDict); ok {
			widget, numWidget = hd, numDeRef(hijos[0])
		}
	}
	if pag, hay := paginaDe[numWidget]; hay {
		c.Pagina = pag
	}
	if r, ok := (&lienzo{d: d}).rectangulo(widget["Rect"]); ok {
		c.Rect = r
	}
	if tipo == "Btn" {
		c.EstadoActivado = d.estadoActivado(widget)
	}
	*out = append(*out, c)
}

func numDeRef(o pdfObj) int {
	if r, ok := o.(pdfRef); ok {
		return r.Num
	}
	return -1
}

func (d *PDFDoc) banderas(dict pdfDict) int64 {
	v, _ := comoEntero(d.resolver(dict["Ff"]))
	return v
}

func tipoDeCampo(ft pdfName, banderas int64) TipoCampo {
	switch ft {
	case "Tx":
		return CampoTexto
	case "Sig":
		return CampoFirma
	case "Btn":
		const (
			botonSinEstado = 1 << 16 // pushbutton
			radio          = 1 << 15
		)
		switch {
		case banderas&botonSinEstado != 0:
			return CampoBoton
		case banderas&radio != 0:
			return CampoOpcion
		default:
			return CampoCasilla
		}
	case "Ch":
		const combo = 1 << 17
		if banderas&combo != 0 {
			return CampoDesplegable
		}
		return CampoLista
	}
	return CampoTexto
}

func (d *PDFDoc) valorDeCampo(v pdfObj) string {
	switch x := d.resolver(v).(type) {
	case pdfString:
		return decodificarTextoPDF(x)
	case pdfName:
		if x == "Off" {
			return ""
		}
		return string(x)
	case pdfArray:
		// Una lista de selección múltiple guarda un array de valores.
		var partes []string
		for _, e := range x {
			partes = append(partes, d.valorDeCampo(e))
		}
		return strings.Join(partes, ", ")
	}
	return ""
}

func (d *PDFDoc) opcionesDeCampo(v pdfObj) []string {
	arr, ok := d.resolver(v).(pdfArray)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		switch x := d.resolver(e).(type) {
		case pdfString:
			out = append(out, decodificarTextoPDF(x))
		case pdfArray:
			// [valorInterno textoVisible]: al usuario se le enseña el segundo.
			if len(x) >= 2 {
				if s, ok := d.resolver(x[1]).(pdfString); ok {
					out = append(out, decodificarTextoPDF(s))
					continue
				}
			}
			if len(x) >= 1 {
				if s, ok := d.resolver(x[0]).(pdfString); ok {
					out = append(out, decodificarTextoPDF(s))
				}
			}
		}
	}
	return out
}

// estadoActivado busca en /AP /N el nombre del estado que NO es «Off». Ese es el
// que hay que escribir en /V y /AS para marcar la casilla.
func (d *PDFDoc) estadoActivado(widget pdfDict) string {
	ap, _ := d.resolver(widget["AP"]).(pdfDict)
	if ap == nil {
		return "Yes"
	}
	normal, _ := d.resolver(ap["N"]).(pdfDict)
	if normal == nil {
		return "Yes"
	}
	for _, k := range clavesOrdenadas(normal) {
		if k != "Off" {
			return string(k)
		}
	}
	return "Yes"
}

// --- Relleno -----------------------------------------------------------------

// RellenarFormularioPDF escribe una copia con los campos indicados rellenos. Las
// claves de `valores` son los nombres completos que devuelve CamposDelFormulario.
func RellenarFormularioPDF(doc *PDFDoc, valores map[string]string) ([]byte, error) {
	if len(valores) == 0 {
		return nil, fmt.Errorf("pdf: no se ha indicado ningún valor")
	}
	campos := doc.CamposDelFormulario()
	if len(campos) == 0 {
		return nil, fmt.Errorf("pdf: el documento no tiene formulario")
	}
	porNombre := make(map[string]CampoFormulario, len(campos))
	for _, c := range campos {
		porNombre[c.Nombre] = c
	}
	for nombre := range valores {
		if _, hay := porNombre[nombre]; !hay {
			return nil, fmt.Errorf("pdf: el formulario no tiene ningún campo llamado %q", nombre)
		}
	}

	if doc.conservaFirmas() {
		return anexarRelleno(doc, valores, porNombre)
	}

	// Con /NeedAppearances el lector tira las apariencias y redibuja cada campo
	// con la fuente de su /DA, que en un subconjunto no trae ni la «Ñ» ni, en las
	// casillas, la ZapfDingbats (Poppler: «couldn't find a font», «Unknown font
	// tag 'ZaDb'»). Se dibujan aquí TODOS los campos con valor, no solo los que se
	// acaban de rellenar, y la marca se quita si ya no queda nada por dibujar.
	redibujar := doc.necesitaApariencias()
	if redibujar {
		todos := make(map[string]string, len(campos))
		for _, c := range campos {
			if c.Valor != "" && c.Tipo != CampoCasilla && c.Tipo != CampoOpcion && c.Tipo != CampoBoton && c.Tipo != CampoFirma {
				todos[c.Nombre] = c.Valor
			}
		}
		for k, v := range valores {
			todos[k] = v
		}
		valores = todos
	}

	e := NuevoEnsambladorPDF()
	// Los parches se registran ANTES de copiar: el recorrido del grafo es quien
	// los dispara, y registrarlos después no llegaría a tiempo.
	doc.registrarParchesDeRelleno(e, valores, porNombre)
	for i := 0; i < doc.NumPaginas(); i++ {
		if err := e.AnadirPagina(doc, i, 0); err != nil {
			return nil, err
		}
	}
	e.ConservarFormulario(doc)
	if forma, ok := e.extrasCatalogo["AcroForm"].(pdfDict); ok && redibujar && !e.widgetSinApariencia() {
		delete(forma, "NeedAppearances")
	}
	return e.Bytes()
}

// necesitaApariencias dice si el formulario pide al lector que redibuje los
// campos (/NeedAppearances true).
func (d *PDFDoc) necesitaApariencias() bool {
	raiz, _ := d.resolver(d.trailer["Root"]).(pdfDict)
	forma, _ := d.resolver(raiz["AcroForm"]).(pdfDict)
	v, _ := d.resolver(forma["NeedAppearances"]).(bool)
	return v
}

// aparienciaRegenerada dibuja un campo desde su valor cuando el formulario lleva
// /NeedAppearances: el documento avisa de que sus apariencias no valen (suelen
// venir vacías, «/Tx BMC EMC») y el lector tiene que redibujarlas. Sin esto el
// valor no se veía ni salía en el texto de la página: medido en el corpus, 18
// formularios de qpdf y PDFBox con «abc», «elephant» o «delta» invisibles.
//
// Texto y desplegable enseñan su valor; una lista, sus opciones. Las casillas y
// los radios no se tocan: su dibujo es el del estado (/AS).
func (l *lienzo) aparienciaRegenerada(ad pdfDict) *pdfStream {
	if sub, _ := l.d.resolver(ad["Subtype"]).(pdfName); sub != "Widget" {
		return nil
	}
	rect, ok := l.rectangulo(ad["Rect"])
	if !ok {
		return nil
	}
	// Atributos de campo, heredados por /Parent (§12.7.3.1).
	at := pdfDict{}
	for n, nodo := 0, ad; nodo != nil && n < maxProfundidadCampos; n++ {
		for _, k := range []pdfName{"FT", "Ff", "V", "DA", "Opt"} {
			if _, ya := at[k]; !ya {
				if v, hay := nodo[k]; hay {
					at[k] = v
				}
			}
		}
		nodo, _ = l.d.resolver(nodo["Parent"]).(pdfDict)
	}
	ft, _ := l.d.resolver(at["FT"]).(pdfName)
	ff, _ := comoEntero(l.d.resolver(at["Ff"]))
	campo := CampoFormulario{
		Tipo: tipoDeCampo(ft, ff), Rect: rect,
		VariasLineas: ft == "Tx" && ff&(1<<12) != 0, Contrasena: ft == "Tx" && ff&(1<<13) != 0,
	}
	valor := l.d.valorDeCampo(at["V"])
	switch campo.Tipo {
	case CampoTexto, CampoDesplegable:
	case CampoLista:
		valor, campo.VariasLineas = strings.Join(l.d.opcionesDeCampo(at["Opt"]), "\n"), true
	default:
		return nil
	}
	if valor == "" {
		return nil
	}
	da := ""
	if s, ok := l.d.resolver(at["DA"]).(pdfString); ok {
		da = string(s)
	} else {
		raiz, _ := l.d.resolver(l.d.trailer["Root"]).(pdfDict)
		forma, _ := l.d.resolver(raiz["AcroForm"]).(pdfDict)
		if s, ok := l.d.resolver(forma["DA"]).(pdfString); ok {
			da = string(s)
		}
	}
	obj, ok := aparienciaDeCampo(pdfDict{"Rect": ad["Rect"]}, campo, valor, da, marcoDeMK(ad))
	if !ok {
		return nil
	}
	dict, _ := obj.valor.(pdfDict)
	return &pdfStream{Dict: dict, Raw: obj.flujo, sinCifrar: true}
}

// widgetSinApariencia dice si algún widget del documento nuevo sigue
// dependiendo de que el lector lo dibuje: sin /AP /N, o una casilla cuyo estado
// no está dibujado.
func (e *PDFEnsamblador) widgetSinApariencia() bool {
	valor := func(o pdfObj) (pdfObj, bool) {
		if r, ok := o.(pdfRef); ok {
			if r.Num < 1 || r.Num > len(e.cuerpos) {
				return nil, false
			}
			return e.cuerpos[r.Num-1].valor, e.cuerpos[r.Num-1].esFlujo
		}
		return o, false
	}
	sinDibujo := func(w pdfDict) bool {
		if t, _ := w["Subtype"].(pdfName); t != "Widget" {
			return false
		}
		ap, _ := valor(w["AP"])
		apd, _ := ap.(pdfDict)
		n, esFlujo := valor(apd["N"])
		if esFlujo {
			return false
		}
		estados, ok := n.(pdfDict)
		if !ok {
			return true
		}
		as, _ := w["AS"].(pdfName)
		_, dibujado := estados[as]
		return as != "" && as != "Off" && !dibujado
	}
	for _, c := range e.cuerpos {
		d, ok := c.valor.(pdfDict)
		if !ok || c.esFlujo {
			continue
		}
		if sinDibujo(d) {
			return true
		}
		annots, _ := valor(d["Annots"])
		lista, _ := annots.(pdfArray)
		for _, a := range lista {
			if w, ok := a.(pdfDict); ok && sinDibujo(w) {
				return true
			}
		}
	}
	return false
}

// anexarRelleno escribe los campos rellenos en una actualización incremental: el
// documento firmado no se reescribe y sus firmas siguen validando (pdfanexo.go).
// Los mismos parches que el camino de siempre, aplicados al objeto ORIGINAL de
// cada campo en vez de a su copia.
func anexarRelleno(doc *PDFDoc, valores map[string]string, porNombre map[string]CampoFormulario) ([]byte, error) {
	if err := admiteAnexo(doc, anexoFormulario); err != nil {
		return nil, err
	}
	e := nuevoAnexo(doc)
	doc.registrarParchesDeRelleno(e, valores, porNombre)
	nums := make([]int, 0, len(e.parches))
	for llave := range e.parches {
		nums = append(nums, llave.num)
	}
	sort.Ints(nums) // los objetos nuevos se numeran en el orden en que se crean
	cambiados := make(map[int]objetoSalida, len(nums))
	for _, num := range nums {
		obj, err := doc.objeto(num)
		dict, ok := obj.(pdfDict)
		if err != nil || !ok {
			return nil, fmt.Errorf("%w: el campo %d no se puede leer", ErrInvalidaFirmas, num)
		}
		copia := copiaLlana(dict)
		e.parches[origenObj{doc: doc, num: num}](e, copia)
		cambiados[num] = objetoSalida{valor: copia}
	}
	return anexar(doc, e, cambiados)
}

// registrarParchesDeRelleno localiza el objeto de cada campo a rellenar y anota
// qué hay que cambiarle.
func (d *PDFDoc) registrarParchesDeRelleno(e *PDFEnsamblador, valores map[string]string,
	porNombre map[string]CampoFormulario,
) {
	raiz, _ := d.resolver(d.trailer["Root"]).(pdfDict)
	if raiz == nil {
		return
	}
	forma, _ := d.resolver(raiz["AcroForm"]).(pdfDict)
	if forma == nil {
		return
	}
	campos, _ := d.resolver(forma["Fields"]).(pdfArray)
	da := ""
	if s, ok := d.resolver(forma["DA"]).(pdfString); ok {
		da = string(s)
	}
	for _, c := range campos {
		d.parchearRama(e, c, "", valores, porNombre, da, 0)
	}
}

func (d *PDFDoc) parchearRama(e *PDFEnsamblador, ref pdfObj, prefijo string,
	valores map[string]string, porNombre map[string]CampoFormulario, da string, prof int,
) {
	if prof > maxProfundidadCampos {
		return
	}
	dict, ok := d.resolver(ref).(pdfDict)
	if !ok {
		return
	}
	nombre := prefijo
	if t, ok := d.resolver(dict["T"]).(pdfString); ok {
		if nombre != "" {
			nombre += "."
		}
		nombre += decodificarTextoPDF(t)
	}
	if s, ok := d.resolver(dict["DA"]).(pdfString); ok {
		da = string(s)
	}

	if nuevo, hay := valores[nombre]; hay {
		campo := porNombre[nombre]
		hijos, _ := d.resolver(dict["Kids"]).(pdfArray)
		if campo.Tipo == CampoCasilla || campo.Tipo == CampoOpcion {
			campo.EstadoActivado = d.estadoElegido(campo, nuevo, dict, hijos)
		}
		num := numDeRef(ref)
		if num > 0 {
			enWidget := d.valorEnWidget(campo, nuevo, dict)
			e.Parchear(d, num, func(e *PDFEnsamblador, dst pdfDict) {
				aplicarValor(e, dst, campo, nuevo, da)
				if enWidget != nuevo {
					aplicarApariencia(e, dst, campo, enWidget, da)
				}
			})
		}
		// Con widgets aparte, la apariencia va en CADA uno: rellenar solo el campo
		// deja el dato bien y la pantalla vacía.
		for _, h := range hijos {
			if hn := numDeRef(h); hn > 0 {
				hd, _ := d.resolver(h).(pdfDict)
				enWidget := d.valorEnWidget(campo, nuevo, hd)
				e.Parchear(d, hn, func(e *PDFEnsamblador, dst pdfDict) {
					aplicarApariencia(e, dst, campo, enWidget, da)
				})
			}
		}
		return
	}

	if hijos, ok := d.resolver(dict["Kids"]).(pdfArray); ok {
		for _, h := range hijos {
			d.parchearRama(e, h, nombre, valores, porNombre, da, prof+1)
		}
	}
}

// estadoElegido es el estado que queda marcado en una casilla o un grupo de
// radio: el que se pide por su nombre si algún widget lo dibuja (el «2» de un
// grupo de tres), y si no, el estado activado del campo.
func (d *PDFDoc) estadoElegido(campo CampoFormulario, valor string, dict pdfDict, hijos pdfArray) string {
	// Con /Opt, el valor es el de exportación y su posición dice qué widget
	// (§12.7.4.2.4): «Female» en ["Male" "Female"] es el estado del segundo, que
	// se suele llamar «1».
	for i, o := range campo.Opciones {
		if o != valor || i >= len(hijos) {
			continue
		}
		if hd, ok := d.resolver(hijos[i]).(pdfDict); ok {
			if e := d.estadoActivado(hd); d.dibujaEstado(hd, pdfName(e)) {
				return e
			}
		}
	}
	if esAfirmativo(valor) {
		widgets := []pdfDict{dict}
		for _, h := range hijos {
			if hd, ok := d.resolver(h).(pdfDict); ok {
				widgets = append(widgets, hd)
			}
		}
		for _, w := range widgets {
			if d.dibujaEstado(w, pdfName(valor)) {
				return valor
			}
		}
	}
	if campo.EstadoActivado == "" {
		return "Yes"
	}
	return campo.EstadoActivado
}

// valorEnWidget es lo que se aplica a un widget de una casilla o un radio: el
// valor pedido si dibuja el estado elegido, y «Off» si no. Marcar con /AS un
// estado que el widget no tiene lo deja sin dibujo, y Poppler lo rehace con la
// ZapfDingbats del /DA («Unknown font tag 'ZaDb'»): en un grupo de radio, marcar
// uno encendía los tres.
func (d *PDFDoc) valorEnWidget(campo CampoFormulario, valor string, w pdfDict) string {
	if (campo.Tipo != CampoCasilla && campo.Tipo != CampoOpcion) || !esAfirmativo(valor) {
		return valor
	}
	ap, _ := d.resolver(w["AP"]).(pdfDict)
	if _, estados := d.resolver(ap["N"]).(pdfDict); !estados || d.dibujaEstado(w, pdfName(campo.EstadoActivado)) {
		return valor
	}
	return "Off"
}

// dibujaEstado dice si el widget trae dibujo para ese estado en /AP /N.
func (d *PDFDoc) dibujaEstado(w pdfDict, estado pdfName) bool {
	ap, _ := d.resolver(w["AP"]).(pdfDict)
	n, _ := d.resolver(ap["N"]).(pdfDict)
	_, hay := n[estado]
	return hay
}

// aplicarValor escribe /V (y /AS en los botones) y regenera la apariencia.
func aplicarValor(e *PDFEnsamblador, dst pdfDict, campo CampoFormulario, valor, da string) {
	switch campo.Tipo {
	case CampoCasilla, CampoOpcion:
		estado := pdfName("Off")
		if esAfirmativo(valor) {
			estado = pdfName(campo.EstadoActivado)
			if estado == "" {
				estado = "Yes"
			}
		}
		dst["V"] = estado
		dst["AS"] = estado
	default:
		dst["V"] = cadenaUTF16(valor)
	}
	aplicarApariencia(e, dst, campo, valor, da)
}

// aplicarApariencia dibuja el valor dentro del widget y lo cuelga del /AP.
func aplicarApariencia(e *PDFEnsamblador, dst pdfDict, campo CampoFormulario, valor, da string) {
	obj, ok := aparienciaDeCampo(dst, campo, valor, da, marcoDeMK(dst))
	if !ok {
		return
	}
	numAP := e.reservar()
	e.cuerpos[numAP-1] = obj
	dst["AP"] = pdfDict{"N": pdfRef{Num: numAP}}
}

// aparienciaDeCampo dibuja el valor dentro del widget. Solo tiene sentido si el
// objeto ES un widget: un campo puro sin /Rect no se pinta. Devuelve false
// cuando no hay nada que dibujar (una casilla se resuelve con /AS).
//
// `marco` pinta además el recuadro del campo, y lo dice el propio widget: es lo
// que hay en su /MK. Quien CREA un campo lo escribe (sin recuadro, uno vacío es
// invisible y nadie sabe dónde pinchar) y quien lo RELLENA lo respeta, porque
// esta función sustituye el /AP entero: el dibujo anterior —recuadro incluido—
// se pierde, y sin releer /MK el campo se quedaba sin borde en cuanto alguien
// escribía en él. Un widget ajeno que no declara /MK sigue sin recuadro, que es
// la elección de su diseñador.
func aparienciaDeCampo(dst pdfDict, campo CampoFormulario, valor, da string, marco *marcoCampo) (objetoSalida, bool) {
	if campo.Tipo == CampoCasilla || campo.Tipo == CampoOpcion {
		// Las casillas ya traen dibujado su estado marcado; basta con /AS, que se
		// fijó arriba. Redibujarlas perdería la marca que eligió el diseñador.
		if esAfirmativo(valor) {
			estado := campo.EstadoActivado
			if estado == "" {
				estado = "Yes"
			}
			dst["AS"] = pdfName(estado)
		} else {
			dst["AS"] = pdfName("Off")
		}
		return objetoSalida{}, false
	}
	if _, tieneRect := dst["Rect"]; !tieneRect {
		return objetoSalida{}, false
	}
	ancho := campo.Rect[2] - campo.Rect[0]
	alto := campo.Rect[3] - campo.Rect[1]
	if ancho <= 1 || alto <= 1 {
		return objetoSalida{}, false
	}

	tam, color := interpretarDA(da)
	if tam <= 0 {
		// Tamaño 0 en /DA significa «ajústalo tú». Se calcula del alto de la caja,
		// que es lo que hace cualquier lector.
		tam = math.Min(alto*0.66, 14)
		if campo.VariasLineas {
			tam = math.Min(alto*0.2, 11)
		}
	}
	texto := valor
	if campo.Contrasena {
		texto = strings.Repeat("•", len([]rune(valor)))
	}

	var b strings.Builder
	if marco != nil {
		b.WriteString("q\n")
		if marco.fondo != "" {
			fmt.Fprintf(&b, "%s rg 0 0 %s %s re f\n", marco.fondo, numPDF(ancho), numPDF(alto))
		}
		if marco.borde != "" && marco.grosor > 0 {
			// El trazo se centra en el camino, así que el recuadro se mete media
			// anchura hacia dentro: si no, la mitad de fuera cae fuera del /BBox y
			// el borde sale a la mitad de grueso.
			g := marco.grosor
			fmt.Fprintf(&b, "%s RG %s w %s %s %s %s re S\n", marco.borde, numPDF(g),
				numPDF(g/2), numPDF(g/2), numPDF(ancho-g), numPDF(alto-g))
		}
		b.WriteString("Q\n")
	}
	// /Tx BMC … EMC marca el bloque como «texto variable de un campo»: es lo que
	// permite a un lector volver a generarlo si el usuario lo edita.
	b.WriteString("/Tx BMC\nq\n")
	fmt.Fprintf(&b, "1 1 %s %s re W n\n", numPDF(ancho-2), numPDF(alto-2))
	fmt.Fprintf(&b, "BT %s %s %s rg /SytueCampoF %s Tf %s TL\n",
		numPDF(color[0]), numPDF(color[1]), numPDF(color[2]), numPDF(tam), numPDF(tam*1.2))

	if campo.VariasLineas {
		fmt.Fprintf(&b, "2 %s Td\n", numPDF(alto-2-tam))
		for i, linea := range partirEnLineas(texto, ancho-4, tam) {
			if i > 0 {
				b.WriteString("T*\n")
			}
			fmt.Fprintf(&b, "(%s) Tj\n", escaparCadenaContenido(linea))
		}
	} else {
		// Centrado vertical: la línea base va a un tercio del alto, que es donde
		// cae el texto de un campo de una línea en cualquier formulario.
		fmt.Fprintf(&b, "2 %s Td (%s) Tj\n", numPDF((alto-tam*0.72)/2), escaparCadenaContenido(texto))
	}
	b.WriteString("ET\nQ\nEMC\n")

	return objetoSalida{
		valor: pdfDict{
			"Type":      pdfName("XObject"),
			"Subtype":   pdfName("Form"),
			"FormType":  int64(1),
			"BBox":      pdfArray{0.0, 0.0, ancho, alto},
			"Resources": pdfDict{"Font": pdfDict{"SytueCampoF": fuenteDeCampo()}},
		},
		flujo:   []byte(b.String()),
		esFlujo: true,
	}, true
}

// marcoCampo son los operandos ya listos del recuadro de un widget: el color de
// fondo y el del borde tal y como se escriben en un flujo («0.98 0.98 0.98»), y
// el grosor del trazo. Se guardan como texto porque el espacio de color lo
// decide el NÚMERO de componentes del /MK y aquí ya está resuelto.
type marcoCampo struct {
	fondo  string
	borde  string
	grosor float64
}

// marcoDeMK lee el recuadro que el widget declara en /MK (§12.5.6.19: /BG el
// fondo, /BC el borde) y el grosor de su /BS (§12.5.4, /W, 1 por defecto).
//
// Es lo que un lector usa para redibujar el campo, así que es también lo que
// tiene que respetar quien regenera su apariencia. Devuelve nil cuando el widget
// no declara ninguno de los dos: un array VACÍO es «sin color» en el estándar, y
// no lo mismo que no traer la clave.
func marcoDeMK(dst pdfDict) *marcoCampo {
	mk, _ := dst["MK"].(pdfDict)
	if mk == nil {
		return nil
	}
	m := marcoCampo{
		fondo:  colorDeMK(mk["BG"]),
		borde:  colorDeMK(mk["BC"]),
		grosor: 1,
	}
	if bs, ok := dst["BS"].(pdfDict); ok {
		if w, hay := bs["W"]; hay {
			m.grosor = comoReal(w)
		}
	}
	if m.fondo == "" && m.borde == "" {
		return nil
	}
	return &m
}

// colorDeMK convierte un array de color de /MK en los operandos de un `rg`/`RG`.
// El espacio lo da la longitud (§12.5.6.19): 1 gris, 3 RGB, 4 CMYK. Todo se
// devuelve en RGB porque el flujo que se escribe solo usa `rg` y `RG`.
func colorDeMK(o pdfObj) string {
	arr, ok := o.(pdfArray)
	if !ok {
		return ""
	}
	v := make([]float64, len(arr))
	for i := range arr {
		v[i] = comoReal(arr[i])
	}
	switch len(v) {
	case 1:
		return fmt.Sprintf("%s %s %s", numPDF(v[0]), numPDF(v[0]), numPDF(v[0]))
	case 3:
		return fmt.Sprintf("%s %s %s", numPDF(v[0]), numPDF(v[1]), numPDF(v[2]))
	case 4:
		// CMYK → RGB por la conversión llana del §10.4: la exacta necesita un
		// perfil que el /MK no trae, y para un borde de campo sobra.
		r, g, bl := (1-v[0])*(1-v[3]), (1-v[1])*(1-v[3]), (1-v[2])*(1-v[3])
		return fmt.Sprintf("%s %s %s", numPDF(r), numPDF(g), numPDF(bl))
	default:
		return "" // 0 componentes es «sin color», y cualquier otra cosa no es un color
	}
}

func fuenteDeCampo() pdfDict {
	return pdfDict{
		"Type":     pdfName("Font"),
		"Subtype":  pdfName("Type1"),
		"BaseFont": pdfName("Helvetica"),
		"Encoding": pdfName("WinAnsiEncoding"),
	}
}

// interpretarDA saca el tamaño de fuente y el color de la cadena /DA, que es un
// trocito de flujo de contenido («/Helv 0 Tf 0 g»).
func interpretarDA(da string) (float64, [3]float64) {
	tam := 0.0
	color := [3]float64{0, 0, 0}
	campos := strings.Fields(da)
	for i, c := range campos {
		switch c {
		case "Tf":
			if i >= 1 {
				// Un valor ilegible deja los componentes a cero, que es negro: el color por
				// defecto de un campo y la degradación que ya se quería.
				_, _ = fmt.Sscanf(campos[i-1], "%f", &tam)
			}
		case "g":
			if i >= 1 {
				var v float64
				// Un valor ilegible deja los componentes a cero, que es negro: el color por
				// defecto de un campo y la degradación que ya se quería.
				_, _ = fmt.Sscanf(campos[i-1], "%f", &v)
				color = [3]float64{v, v, v}
			}
		case "rg":
			if i >= 3 {
				for j := 0; j < 3; j++ {
					// Un valor ilegible deja los componentes a cero, que es negro: el color por
					// defecto de un campo y la degradación que ya se quería.
					_, _ = fmt.Sscanf(campos[i-3+j], "%f", &color[j])
				}
			}
		}
	}
	return tam, color
}

func esAfirmativo(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "0", "no", "off", "false", "n":
		return false
	}
	return true
}

// --- Aplanado ----------------------------------------------------------------

// AplanarFormularioPDF convierte los campos en contenido fijo de la página: el
// documento sigue viéndose igual y ya no se puede editar.
//
// Es lo que hay que hacer antes de enviar un formulario relleno: sin aplanar,
// quien lo recibe puede cambiar cualquier dato y no queda rastro.
func AplanarFormularioPDF(doc *PDFDoc) ([]byte, error) {
	e := NuevoEnsambladorPDF()
	for i := 0; i < doc.NumPaginas(); i++ {
		p, err := doc.Pagina(i)
		if err != nil {
			return nil, err
		}
		anots, _ := doc.resolver(p.dict["Annots"]).(pdfArray)

		// Se separan los widgets del resto: los widgets se funden con la página y
		// las demás anotaciones —los comentarios— se quedan como estaban.
		quedan := pdfArray{}
		var aFundir []pdfDict
		for _, a := range anots {
			ad, ok := doc.resolver(a).(pdfDict)
			if !ok {
				continue
			}
			if sub, _ := doc.resolver(ad["Subtype"]).(pdfName); sub != "Widget" {
				quedan = append(quedan, a)
				continue
			}
			aFundir = append(aFundir, ad)
		}
		if err := e.anadirPagina(doc, i, 0, opcionesPagina{annots: quedan}); err != nil {
			return nil, err
		}
		if len(aFundir) > 0 {
			e.fundirWidgets(doc, aFundir)
		}
	}
	// El /AcroForm NO se conserva a propósito: aplanar es dejar de ser formulario.
	e.extrasCatalogo = pdfDict{"AcroForm": pdfNull{}}
	return e.Bytes()
}

// fundirWidgets añade las apariencias de los widgets al contenido de la última
// página, colocándolas en su /Rect.
func (e *PDFEnsamblador) fundirWidgets(doc *PDFDoc, widgets []pdfDict) {
	pp := e.ultimaPendiente()
	if pp == nil {
		return
	}
	l := &lienzo{d: doc}
	var contenido strings.Builder
	xobjects := pdfDict{}

	for i, w := range widgets {
		if l.anotacionOculta(w) {
			continue
		}
		ap := l.aparienciaNormal(w)
		if ap == nil {
			continue
		}
		rect, ok := l.rectangulo(w["Rect"])
		if !ok {
			continue
		}
		numXO := e.copiarObjeto(doc, ap.Num)
		if ap.Num == 0 {
			continue // apariencia en línea: sin número no se puede referenciar
		}
		nombre := pdfName(fmt.Sprintf("SytueApl%d", i))
		xobjects[nombre] = pdfRef{Num: numXO}

		// La apariencia se dibuja en su /BBox; hay que llevarla a /Rect, que es lo
		// mismo que hace el visor al pintarla. Sin esta escala, un campo sale del
		// tamaño de su plantilla y no del que ocupa en la página.
		m := matrizDeApariencia(l, ap, rect)
		fmt.Fprintf(&contenido, "q %s %s %s %s %s %s cm /%s Do Q\n",
			numPDF(m[0]), numPDF(m[1]), numPDF(m[2]), numPDF(m[3]), numPDF(m[4]), numPDF(m[5]),
			escaparNombrePDF(string(nombre)))
	}
	if contenido.Len() == 0 {
		return
	}

	numFlujo := e.reservar()
	e.cuerpos[numFlujo-1] = objetoSalida{valor: pdfDict{}, flujo: []byte(contenido.String()), esFlujo: true}
	switch actual := pp.dict["Contents"].(type) {
	case pdfArray:
		pp.dict["Contents"] = append(append(pdfArray{}, actual...), pdfRef{Num: numFlujo})
	case nil:
		pp.dict["Contents"] = pdfArray{pdfRef{Num: numFlujo}}
	default:
		pp.dict["Contents"] = pdfArray{actual, pdfRef{Num: numFlujo}}
	}
	pp.dict["Resources"] = e.recursosConXObjects(pp, xobjects)
	e.reserializarPendiente(pp)
}

// matrizDeApariencia calcula la transformación que encaja la /BBox de una
// apariencia en el /Rect de su anotación (§12.5.5).
func matrizDeApariencia(l *lienzo, ap *pdfStream, rect [4]float64) matriz {
	m := identidad
	if arr, ok := l.d.resolver(ap.Dict["Matrix"]).(pdfArray); ok && len(arr) >= 6 {
		for i := 0; i < 6; i++ {
			m[i] = comoReal(l.d.resolver(arr[i]))
		}
	}
	bbox, tiene := l.rectangulo(ap.Dict["BBox"])
	if !tiene {
		return m
	}
	var xs, ys []float64
	for _, pt := range [4][2]float64{{bbox[0], bbox[1]}, {bbox[2], bbox[1]}, {bbox[2], bbox[3]}, {bbox[0], bbox[3]}} {
		xs = append(xs, m[0]*pt[0]+m[2]*pt[1]+m[4])
		ys = append(ys, m[1]*pt[0]+m[3]*pt[1]+m[5])
	}
	minX, maxX, minY, maxY := minimo(xs), maximo(xs), minimo(ys), maximo(ys)
	sx, sy := 1.0, 1.0
	if maxX-minX > 1e-9 {
		sx = (rect[2] - rect[0]) / (maxX - minX)
	}
	if maxY-minY > 1e-9 {
		sy = (rect[3] - rect[1]) / (maxY - minY)
	}
	return m.por(matriz{sx, 0, 0, sy, rect[0] - minX*sx, rect[1] - minY*sy})
}

// recursosConXObjects fusiona unos XObjects nuevos en el /Resources de la página
// sin perder los que ya tenía.
func (e *PDFEnsamblador) recursosConXObjects(pp *paginaPendiente, nuevos pdfDict) pdfDict {
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
	xo := pdfDict{}
	switch ya := rec["XObject"].(type) {
	case pdfDict:
		for k, x := range ya {
			xo[k] = x
		}
	case pdfRef:
		if orig, ok := pp.origen.resolver(ya).(pdfDict); ok {
			for k, x := range orig {
				xo[k] = x
			}
		}
	}
	for k, v := range nuevos {
		xo[k] = v
	}
	rec["XObject"] = xo
	return rec
}

// --- Firma dibujada ----------------------------------------------------------

// Firma es una rúbrica puesta sobre el documento: el trazo de un dedo o un
// lápiz, o una imagen escaneada.
type Firma struct {
	Pagina int        `json:"pagina"`
	Rect   [4]float64 `json:"rect"`
	// Trazos es la rúbrica dibujada, en coordenadas de la página.
	Trazos [][][2]float64 `json:"trazos,omitempty"`
	// ImagenPNG es una firma escaneada. Manda sobre los trazos si van las dos.
	ImagenPNG []byte     `json:"-"`
	Color     [3]float64 `json:"color"`
	Grosor    float64    `json:"grosor"`
	// Pie es la línea de texto bajo la rúbrica: quién firma y cuándo.
	Pie string `json:"pie,omitempty"`
	// TodasLasPaginas repite la rúbrica en cada página, en el mismo sitio. Es lo
	// que se pide en un contrato de varias hojas, donde firmar solo la última
	// deja las demás sin marcar y cualquiera puede cambiarlas de sitio.
	TodasLasPaginas bool `json:"todas_las_paginas,omitempty"`
}

// FirmarDibujadoPDF pone una rúbrica sobre el documento y lo aplana, para que no
// se pueda mover ni quitar sin dejar rastro.
//
// AVISO QUE LA INTERFAZ TIENE QUE DAR: esto es una firma MANUSCRITA digitalizada,
// no una firma electrónica cualificada. Vale para el «conforme» de un albarán y
// no para lo que exija eIDAS avanzada; para eso está FirmarConCertificadoPDF.
func FirmarDibujadoPDF(doc *PDFDoc, f Firma) ([]byte, error) {
	if f.Pagina < 0 || f.Pagina >= doc.NumPaginas() {
		return nil, fmt.Errorf("pdf: la página %d no existe", f.Pagina+1)
	}
	if len(f.Trazos) == 0 && len(f.ImagenPNG) == 0 {
		return nil, fmt.Errorf("pdf: la firma no lleva ni trazo ni imagen")
	}
	if f.Grosor <= 0 {
		f.Grosor = 1.6
	}
	if f.Color == [3]float64{} {
		f.Color = [3]float64{0.06, 0.09, 0.16} // tinta, no negro puro
	}

	// En qué páginas cae la rúbrica. La misma anotación repetida: el trazo va en
	// coordenadas de página, así que se dibuja igual en todas.
	paginas := []int{f.Pagina}
	if f.TodasLasPaginas {
		paginas = make([]int, doc.NumPaginas())
		for i := range paginas {
			paginas[i] = i
		}
	}
	anots := make([]Anotacion, 0, len(paginas)*2)
	for _, pg := range paginas {
		anots = append(anots, Anotacion{
			Pagina: pg,
			Tipo:   AnotTinta,
			Rect:   f.Rect,
			Trazos: f.Trazos,
			Color:  f.Color,
			Grosor: f.Grosor,
		})
		if f.Pie != "" {
			anots = append(anots, Anotacion{
				Pagina: pg,
				Tipo:   AnotTextoLibre,
				Rect:   [4]float64{f.Rect[0], f.Rect[1] - 16, f.Rect[2], f.Rect[1]},
				Texto:  f.Pie,
				Color:  [3]float64{0.35, 0.37, 0.4},
				Tamano: 8,
			})
		}
	}
	conFirma, err := AnadirAnotacionesPDF(doc, anots)
	if err != nil {
		return nil, err
	}
	// Se aplana: una firma que se puede arrastrar a otro sitio no es una firma.
	d2, err := LeerPDF(conFirma)
	if err != nil {
		return nil, err
	}
	return AplanarAnotacionesPDF(d2)
}

// AplanarAnotacionesPDF funde TODAS las anotaciones en el contenido de la
// página. Después ya no se pueden mover ni borrar.
func AplanarAnotacionesPDF(doc *PDFDoc) ([]byte, error) {
	e := NuevoEnsambladorPDF()
	for i := 0; i < doc.NumPaginas(); i++ {
		p, err := doc.Pagina(i)
		if err != nil {
			return nil, err
		}
		anots, _ := doc.resolver(p.dict["Annots"]).(pdfArray)
		quedan := pdfArray{}
		var aFundir []pdfDict
		for _, a := range anots {
			ad, ok := doc.resolver(a).(pdfDict)
			if !ok {
				continue
			}
			sub, _ := doc.resolver(ad["Subtype"]).(pdfName)
			if sub == "Link" || sub == "Popup" {
				quedan = append(quedan, a)
				continue
			}
			aFundir = append(aFundir, ad)
		}
		if err := e.anadirPagina(doc, i, 0, opcionesPagina{annots: quedan}); err != nil {
			return nil, err
		}
		if len(aFundir) > 0 {
			e.fundirWidgets(doc, aFundir)
		}
	}
	// Los widgets se han fundido con la página: un /AcroForm que los nombrara
	// sería un formulario de campos sin dónde escribir.
	e.extrasCatalogo = pdfDict{"AcroForm": pdfNull{}}
	return e.Bytes()
}
