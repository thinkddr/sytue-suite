// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// ¿SIGUE SIENDO PDF/A? La comprobación propia al guardar.
//
// Decisión del dueño (2026-10-15): al guardar un PDF/A se conserva SOLO la marca
// pdfaid, y solo si la salida sigue cumpliendo; si nuestra edición rompe la
// conformidad, la marca se quita y la interfaz lo avisa.
//
// No hay veraPDF dentro del servicio. Lo que se comprueba es lo que NUESTRAS
// ediciones pueden introducir, medido con veraPDF sobre el corpus de
// scripts/certipdf-compat: lo copiado del original ya cumplía (la entrada se
// declaraba PDF/A), así que se miran solo los objetos que escribimos nosotros:
//
//   - fuentes sin incrustar (6.3.4 en la parte 1, 6.2.11.4 en las demás);
//   - color de dispositivo sin una intención de salida que lo cubra (6.2.3.3 y
//     6.2.4.3): RGB con un perfil CMYK, o al revés;
//   - transparencia en la parte 1 (6.4): /CA o /ca por debajo de 1, /SMask o un
//     modo de fusión que no sea /Normal, también en la propia anotación (6.5.3);
//   - anotaciones que no se imprimen o que se ocultan (6.5.3 y 6.3.2);
//   - el nivel «a» sin estructura etiquetada (la quita censurar), que baja a «b».
package domain

import (
	"bytes"
	"fmt"
	"regexp"
)

// DeclaraPDFA devuelve la parte y la conformidad que el documento declara
// («2u», «1b»), o "" si no se declara PDF/A. La capa HTTP compara la entrada y
// la salida para avisar de que se perdió.
func (d *PDFDoc) DeclaraPDFA() string {
	dec, ok := d.declaraPDFA()
	if !ok {
		return ""
	}
	return dec.parte + string(bytes.ToLower([]byte(dec.conformidad)))
}

// incumplePDFA devuelve la declaración que la salida puede llevar y, si no
// puede llevar ninguna, por qué. Un nivel «a» sin estructura etiquetada (la
// quita censurar) baja a «b» de la misma parte: decisión del dueño, y la
// interfaz avisa de que pierde el nivel accesible.
func (e *PDFEnsamblador) incumplePDFA(origen *PDFDoc, dec declaracionPDFA, catalogo pdfDict) (declaracionPDFA, string) {
	if dec.conformidad == "A" && catalogo["StructTreeRoot"] == nil {
		dec.conformidad = "B"
	}
	// Reescribir un documento firmado (con permiso) deja sus firmas sin cubrir el
	// fichero, y desde la parte 2 eso incumple (6.4.3).
	if dec.parte != "1" && len(origen.camposDeFirma()) > 0 {
		return dec, "firmas que ya no cubren el fichero"
	}
	copiados := make(map[int]bool, len(e.copiados))
	for _, n := range e.copiados {
		copiados[n] = true
	}
	nuevos := map[int]objetoSalida{}
	for i, o := range e.cuerpos {
		if !copiados[i+1] {
			nuevos[i+1] = o
		}
	}
	return dec, e.incumplenObjetos(nuevos, dec.parte, componentesIntencion(origen))
}

func (e *PDFEnsamblador) incumplenObjetos(objs map[int]objetoSalida, parte string, componentes int) string {
	for num, o := range objs {
		if motivo := e.incumpleObjeto(num, o, parte, componentes); motivo != "" {
			return motivo
		}
	}
	return ""
}

// marcaPDFAEnAnexo es la comprobación para un guardado ANEXADO (un documento
// firmado: pdfanexo.go). El catálogo no se toca —tocarlo rompería la firma—, así
// que si lo anexado no cumple, la marca se quita reescribiendo SOLO el flujo de
// metadatos, que es un cambio que admiten los validadores de firmas (y
// cambiosTras). Devuelve el objeto que hay que anexar, o false si nada cambia.
func (e *PDFEnsamblador) marcaPDFAEnAnexo(doc *PDFDoc, nuevos map[int]objetoSalida) (int, objetoSalida, bool) {
	dec, ok := doc.declaraPDFA()
	if !ok {
		return 0, objetoSalida{}, false
	}
	e.origenAnexo = doc
	if e.incumplenObjetos(nuevos, dec.parte, componentesIntencion(doc)) == "" {
		return 0, objetoSalida{}, false
	}
	raiz, _ := doc.resolver(doc.trailer["Root"]).(pdfDict)
	ref, ok := raiz["Metadata"].(pdfRef)
	st, esFlujo := doc.resolver(ref).(*pdfStream)
	if !ok || !esFlujo || doc.generacion(ref.Num) != 0 {
		return 0, objetoSalida{}, false
	}
	xmp, err := doc.datosDeFlujo(st)
	if err != nil {
		return 0, objetoSalida{}, false
	}
	return ref.Num, objetoSalida{
		valor: pdfDict{"Type": pdfName("Metadata"), "Subtype": pdfName("XML")},
		flujo: sinDeclaracionPDFA(xmp), esFlujo: true,
	}, true
}

var reDeclaracionPDFA = regexp.MustCompile(
	`<pdfaid:(part|conformance|amd|rev|corr)>[^<]*</pdfaid:(?:part|conformance|amd|rev|corr)>|` +
		`\s+pdfaid:(?:part|conformance|amd|rev|corr)\s*=\s*(?:"[^"]*"|'[^']*')`)

// sinDeclaracionPDFA quita del XMP el identificador PDF/A y deja lo demás
// (título, autor…), que en un anexo no es nuestro quitarlo.
func sinDeclaracionPDFA(xmp []byte) []byte {
	return reDeclaracionPDFA.ReplaceAll(xmp, nil)
}

// componentesIntencion es el número de componentes del perfil de la intención
// de salida (1 gris, 3 RGB, 4 CMYK), o 0 si no hay.
func componentesIntencion(d *PDFDoc) int {
	raiz, _ := d.resolver(d.trailer["Root"]).(pdfDict)
	ois, _ := d.resolver(raiz["OutputIntents"]).(pdfArray)
	for _, oi := range ois {
		od, _ := d.resolver(oi).(pdfDict)
		if st, ok := d.resolver(od["DestOutputProfile"]).(*pdfStream); ok {
			if n, ok := comoEntero(d.resolver(st.Dict["N"])); ok {
				return int(n)
			}
		}
	}
	return 0
}

// valorSalida resuelve una referencia del documento que se está escribiendo.
func (e *PDFEnsamblador) valorSalida(o pdfObj) pdfObj {
	r, ok := o.(pdfRef)
	if !ok {
		return o
	}
	if r.Num >= 1 && r.Num <= len(e.cuerpos) && e.cuerpos[r.Num-1].valor != nil {
		return e.cuerpos[r.Num-1].valor
	}
	// En un anexo, lo que no es nuevo sigue en el documento.
	if e.origenAnexo != nil {
		return e.origenAnexo.resolver(r)
	}
	return o
}

func (e *PDFEnsamblador) incumpleObjeto(num int, o objetoSalida, parte string, componentes int) string {
	dict, ok := o.valor.(pdfDict)
	if !ok {
		return ""
	}
	tipo, _ := dict["Type"].(pdfName)
	sub, _ := dict["Subtype"].(pdfName)
	if o.esFlujo && dict["Filter"] == nil && dict["Length1"] == nil && (sub == "Form" || (sub == "" && tipo == "")) {
		cs := e.espaciosColorDeFlujo(num, dict)
		if motivo := colorSinIntencion(o.flujo, componentes, cs); motivo != "" {
			return motivo
		}
	}
	if sub == "Image" && o.esFlujo {
		if motivo := imagenSinIntencion(dict["ColorSpace"], componentes); motivo != "" {
			return motivo
		}
	}
	// Los recursos de una apariencia suelen ir DIRECTOS dentro de ella (la
	// fuente, el estado gráfico): se mira todo lo que cuelga del objeto.
	return e.incumpleValor(dict, parte, 0)
}

// espaciosColorDeFlujo devuelve el diccionario /ColorSpace de los /Resources
// propios del flujo (en un XObject de formulario) o de la página que lo usa.
func (e *PDFEnsamblador) espaciosColorDeFlujo(num int, dict pdfDict) pdfDict {
	rec, _ := e.valorSalida(dict["Resources"]).(pdfDict)
	if rec == nil && num > 0 {
		for _, pp := range e.pendientes {
			if flujoDePagina(pp.dict["Contents"], num) {
				rec, _ = e.valorSalida(pp.dict["Resources"]).(pdfDict)
				break
			}
		}
	}
	cs, _ := e.valorSalida(rec["ColorSpace"]).(pdfDict)
	return cs
}

func flujoDePagina(contents pdfObj, num int) bool {
	switch c := contents.(type) {
	case pdfRef:
		return c.Num == num
	case pdfArray:
		for _, el := range c {
			if r, ok := el.(pdfRef); ok && r.Num == num {
				return true
			}
		}
	}
	return false
}

// incumpleValor recorre los diccionarios y arrays directos de un objeto.
func (e *PDFEnsamblador) incumpleValor(v pdfObj, parte string, prof int) string {
	if prof > 8 {
		return ""
	}
	switch t := v.(type) {
	case pdfRef:
		// Lo que un objeto nuevo enlaza —la fuente de /DR que usa un texto libre—
		// se mira aunque sea del original: el original puede tenerla sin incrustar
		// porque no la usaba. Solo un nivel: no se recorre el documento entero.
		if d, ok := e.valorSalida(t).(pdfDict); ok {
			return e.incumpleDict(d, parte)
		}
		if st, ok := e.valorSalida(t).(*pdfStream); ok {
			return e.incumpleDict(st.Dict, parte)
		}
	case pdfArray:
		for _, x := range t {
			if m := e.incumpleValor(x, parte, prof+1); m != "" {
				return m
			}
		}
	case pdfDict:
		if m := e.incumpleDict(t, parte); m != "" {
			return m
		}
		for _, k := range clavesOrdenadas(t) {
			if m := e.incumpleValor(t[k], parte, prof+1); m != "" {
				return m
			}
		}
	}
	return ""
}

func (e *PDFEnsamblador) incumpleDict(dict pdfDict, parte string) string {
	tipo, _ := dict["Type"].(pdfName)
	sub, _ := dict["Subtype"].(pdfName)
	switch {
	case tipo == "Font" || (sub != "" && dict["BaseFont"] != nil):
		if !e.fuenteIncrustada(dict) {
			return fmt.Sprintf("fuente sin incrustar (%v)", dict["BaseFont"])
		}
	case tipo == "Annot" || dict["Rect"] != nil && sub != "" && tipo != "XObject":
		return anotacionIncumple(dict, sub, parte)
	case parte == "1" && transparente(dict):
		// Un estado gráfico, una imagen con /SMask… En la parte 1 no hay
		// transparencia (6.4).
		return "transparencia en PDF/A-1"
	}
	return ""
}

// anotacionIncumple: toda anotación salvo /Popup se imprime y no se oculta
// (6.5.3), y en la parte 1 no es transparente.
func anotacionIncumple(dict pdfDict, sub pdfName, parte string) string {
	f, _ := comoEntero(dict["F"])
	if sub != "Popup" && (f&4 == 0 || f&(1|2|32) != 0) {
		return "anotación que no se imprime o se oculta"
	}
	if parte == "1" && transparente(dict) {
		return "anotación transparente en PDF/A-1"
	}
	return ""
}

func transparente(d pdfDict) bool {
	for _, k := range []pdfName{"CA", "ca"} {
		if v, ok := d[k]; ok && comoReal(v) < 1 {
			return true
		}
	}
	if sm, ok := d["SMask"]; ok {
		if n, _ := sm.(pdfName); n != "None" {
			return true
		}
	}
	if bm, ok := d["BM"].(pdfName); ok && bm != "Normal" && bm != "Compatible" {
		return true
	}
	return false
}

// fuenteIncrustada mira /FontFile, /FontFile2 o /FontFile3 en el descriptor
// (el de la descendiente en una Type0). Una Type3 lleva sus glifos dentro.
func (e *PDFEnsamblador) fuenteIncrustada(f pdfDict) bool {
	sub, _ := f["Subtype"].(pdfName)
	if sub == "Type3" {
		return true
	}
	if sub == "Type0" {
		desc, _ := e.valorSalida(f["DescendantFonts"]).(pdfArray)
		if len(desc) == 0 {
			return false
		}
		hija, _ := e.valorSalida(desc[0]).(pdfDict)
		return hija != nil && e.fuenteIncrustada(hija)
	}
	fd, _ := e.valorSalida(f["FontDescriptor"]).(pdfDict)
	for _, k := range []pdfName{"FontFile", "FontFile2", "FontFile3"} {
		if fd[k] != nil {
			return true
		}
	}
	return false
}

// colorSinIntencion busca en un flujo de contenido escrito por nosotros los
// operadores de color de dispositivo que ni la intención de salida ni el
// espacio por defecto (/DefaultRGB, /DefaultCMYK, /DefaultGray) de los recursos
// cubren (§6.2.4.3 de ISO 19005).
func colorSinIntencion(flujo []byte, componentes int, cs pdfDict) string {
	for _, op := range operadoresDeContenido(flujo) {
		switch op {
		case "rg", "RG":
			if componentes != 3 && cs["DefaultRGB"] == nil {
				return "color RGB sin intención de salida RGB"
			}
		case "k", "K":
			if componentes != 4 && cs["DefaultCMYK"] == nil {
				return "color CMYK sin intención de salida CMYK"
			}
		case "g", "G":
			if componentes == 0 && cs["DefaultGray"] == nil {
				return "color gris sin intención de salida"
			}
		}
	}
	return ""
}

// imagenSinIntencion es colorSinIntencion para una imagen nueva (la que deja la
// censura): un espacio de dispositivo que la intención de salida no cubre.
func imagenSinIntencion(cs pdfObj, componentes int) string {
	switch n, _ := cs.(pdfName); n {
	case "DeviceRGB":
		return colorSinIntencion([]byte("rg"), componentes, nil)
	case "DeviceCMYK":
		return colorSinIntencion([]byte("k"), componentes, nil)
	case "DeviceGray":
		return colorSinIntencion([]byte("g"), componentes, nil)
	}
	return ""
}

// calRGBDefecto es el /DefaultRGB calibrado (iluminante D65) con el que un
// flujo de apariencia en RGB cumple PDF/A (§6.2.4.3) aunque el documento no
// lleve intención de salida RGB o lleve una CMYK o gris.
func calRGBDefecto() pdfArray {
	return pdfArray{
		pdfName("CalRGB"),
		pdfDict{"WhitePoint": pdfArray{0.9505, int64(1), 1.089}},
	}
}

// calGrayDefecto es el /DefaultGray calibrado (D65) para los recuadros negros
// («0 g») de una página censurada cuando el PDF/A no declara OutputIntents.
func calGrayDefecto() pdfArray {
	return pdfArray{
		pdfName("CalGray"),
		pdfDict{"WhitePoint": pdfArray{0.9505, int64(1), 1.089}},
	}
}

// grupoTransparenciaRGB declara el espacio de fusión de transparencia de la
// página (§6.2.10 de ISO 19005-2/3/4) cuando una anotación lleva opacidad o
// modo de fusión y el documento no tiene OutputIntents.
func grupoTransparenciaRGB() pdfDict {
	return pdfDict{
		"Type": pdfName("Group"),
		"S":    pdfName("Transparency"),
		"CS":   calRGBDefecto(),
	}
}

// asegurarDefaultGray añade /DefaultGray (CalGray) a los /Resources de la
// página censurada si no lo traía ya.
func (e *PDFEnsamblador) asegurarDefaultGray(pp *paginaPendiente) {
	if pp == nil {
		return
	}
	rec := pdfDict{}
	if orig, ok := e.valorSalida(pp.dict["Resources"]).(pdfDict); ok {
		for k, x := range orig {
			rec[k] = x
		}
	}
	cs := pdfDict{}
	if orig, ok := e.valorSalida(rec["ColorSpace"]).(pdfDict); ok {
		for k, x := range orig {
			cs[k] = x
		}
	}
	if cs["DefaultGray"] == nil {
		cs["DefaultGray"] = calGrayDefecto()
		rec["ColorSpace"] = cs
		pp.dict["Resources"] = rec
		e.reserializarPendiente(pp)
	}
}

// operadoresDeContenido devuelve las palabras sueltas de un flujo de contenido
// —los operadores—, sin las cadenas, los nombres ni los números.
func operadoresDeContenido(b []byte) []string {
	var ops []string
	for i := 0; i < len(b); {
		switch c := b[i]; {
		case c == '(':
			i = finDeCadenaLiteral(b, i)
		case c == '%':
			i = finDeLinea(b, i)
		case c == '<' && i+1 < len(b) && b[i+1] != '<':
			i = finDe(b, i, '>')
		case esBlancoPS(c) || bytes.IndexByte([]byte("[]<>{}"), c) >= 0:
			i++
		default: // una palabra: es operador si empieza por letra o por ' " *
			ini := i
			for i++; i < len(b) && !esBlancoPS(b[i]) && bytes.IndexByte([]byte("/[]<>()%{}"), b[i]) < 0; i++ {
			}
			if esLetraDeOperador(b[ini]) {
				ops = append(ops, string(b[ini:i]))
			}
		}
	}
	return ops
}

func esLetraDeOperador(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '\'' || c == '"' || c == '*'
}

func finDeLinea(b []byte, i int) int {
	for i < len(b) && b[i] != '\n' && b[i] != '\r' {
		i++
	}
	return i
}

func finDe(b []byte, i int, fin byte) int {
	for i < len(b) && b[i] != fin {
		i++
	}
	return i + 1
}

// finDeCadenaLiteral devuelve dónde acaba la cadena «(…)» que empieza en i,
// con sus paréntesis anidados y sus escapes.
func finDeCadenaLiteral(b []byte, i int) int {
	prof := 0
	for ; i < len(b); i++ {
		switch b[i] {
		case '\\':
			i++
		case '(':
			prof++
		case ')':
			prof--
			if prof == 0 {
				return i + 1
			}
		}
	}
	return i
}
