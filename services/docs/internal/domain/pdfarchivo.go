// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// PDF/A (archivo a largo plazo) y PDF/UA (accesibilidad) — la parte de la ola P6
// y P8 que no es render, sino DECLARAR y COMPROBAR.
//
// QUÉ SON, sin la jerga. Un PDF normal puede depender de cosas que no están
// dentro: una fuente que se supone instalada, un perfil de color del sistema, un
// vídeo que se descarga. Dentro de veinte años nada de eso existe y el documento
// se ve distinto o no se ve. **PDF/A** es la promesa de que el fichero se basta a
// sí mismo. **PDF/UA** es la otra promesa: que un lector de pantalla puede
// recorrerlo en el orden correcto y decir qué es cada cosa.
//
// LO QUE AQUÍ SE HACE Y LO QUE NO. Se COMPRUEBA (qué falta para cumplir) y se
// DECLARA (marcar el fichero y añadir lo que se puede añadir sin inventar:
// metadatos XMP, el identificador de conformidad, el idioma, el marcado). Lo que
// NO se hace es incrustar fuentes que el documento no traía ni etiquetar una
// estructura que nadie escribió: eso sería adivinar el orden de lectura de una
// maquetación, y equivocarse ahí produce un documento que dice ser accesible y
// se lee al revés — peor que uno que no lo promete.
package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// IncumplimientoArchivo es una razón concreta por la que el documento no cumple.
type IncumplimientoArchivo struct {
	// Codigo es estable y traducible; el texto lo pone la interfaz.
	Codigo string `json:"codigo"`
	// Detalle nombra el objeto o la página, para poder ir a arreglarlo.
	Detalle string `json:"detalle,omitempty"`
	// Bloqueante distingue lo que impide cumplir de lo que solo lo empeora.
	Bloqueante bool `json:"bloqueante"`
}

// InformeArchivo es el resultado de comprobar un documento.
type InformeArchivo struct {
	CumplePDFA  bool `json:"cumple_pdfa"`
	CumplePDFUA bool `json:"cumple_pdfua"`
	// DeclaraPDFUA dice que el fichero se identifica como PDF/UA-1 como pide la norma (XMP
	// con pdfuaid:part=1, MarkInfo/Marked y DisplayDocTitle). Es lo que escribe el exportador
	// accesible de Docus; CumplePDFUA, en cambio, solo mira que haya estructura e idioma.
	DeclaraPDFUA bool                    `json:"declara_pdfua"`
	Problemas    []IncumplimientoArchivo `json:"problemas"`
}

// ComprobarArchivo dice qué le falta al documento para ser PDF/A y PDF/UA.
//
// Se comprueba lo que se puede comprobar con el motor de la casa, y se dice cuál
// es el alcance: no es una validación certificada como la de veraPDF, y la
// interfaz lo tiene que decir. Prometer conformidad sin haberla verificado del
// todo es exactamente el problema que PDF/A viene a resolver.
func (d *PDFDoc) ComprobarArchivo() InformeArchivo {
	var out InformeArchivo

	// 1. Cifrado: PDF/A lo prohíbe. Un documento que necesita una contraseña para
	// abrirse no se puede archivar: dentro de veinte años nadie la tendrá.
	if d.EstaCifrado() {
		out.Problemas = append(out.Problemas, IncumplimientoArchivo{
			Codigo: "cifrado", Bloqueante: true,
		})
	}

	raiz, _ := d.resolver(d.trailer["Root"]).(pdfDict)

	// 2. Fuentes sin incrustar: es LA razón por la que un PDF se ve distinto en
	// otra máquina, y por la que existe PDF/A.
	if faltan := d.fuentesSinIncrustar(); len(faltan) > 0 {
		out.Problemas = append(out.Problemas, IncumplimientoArchivo{
			Codigo: "fuenteSinIncrustar", Detalle: strings.Join(faltan, ", "), Bloqueante: true,
		})
	}

	// 3. Contenido que depende de fuera o se ejecuta.
	if raiz != nil {
		if nombres, ok := d.resolver(raiz["Names"]).(pdfDict); ok {
			if _, hay := nombres["JavaScript"]; hay {
				out.Problemas = append(out.Problemas, IncumplimientoArchivo{
					Codigo: "conJavaScript", Bloqueante: true,
				})
			}
			if _, hay := nombres["EmbeddedFiles"]; hay {
				// PDF/A-3 sí los permite; A-1 y A-2 no. Se marca sin bloquear.
				out.Problemas = append(out.Problemas, IncumplimientoArchivo{
					Codigo: "conAdjuntos", Bloqueante: false,
				})
			}
		}
		if _, hay := raiz["OpenAction"]; hay {
			out.Problemas = append(out.Problemas, IncumplimientoArchivo{
				Codigo: "conAccionDeApertura", Bloqueante: false,
			})
		}
		// 4. Metadatos XMP: PDF/A los exige, y ahí va el identificador que declara
		// la conformidad.
		if _, hay := raiz["Metadata"]; !hay {
			out.Problemas = append(out.Problemas, IncumplimientoArchivo{
				Codigo: "sinMetadatosXMP", Bloqueante: true,
			})
		}
	}

	// 5. Lo de PDF/UA: estructura etiquetada, idioma y títulos.
	sinEtiquetas := true
	if raiz != nil {
		if _, hay := raiz["StructTreeRoot"]; hay {
			sinEtiquetas = false
		}
	}
	if sinEtiquetas {
		out.Problemas = append(out.Problemas, IncumplimientoArchivo{
			Codigo: "sinEstructura", Bloqueante: true,
		})
	}
	sinIdioma := raiz == nil || raiz["Lang"] == nil
	if sinIdioma {
		out.Problemas = append(out.Problemas, IncumplimientoArchivo{
			Codigo: "sinIdioma", Bloqueante: true,
		})
	}
	// Una imagen sin texto alternativo es la barrera de accesibilidad más común.
	if n := d.imagenesSinAlternativo(); n > 0 {
		out.Problemas = append(out.Problemas, IncumplimientoArchivo{
			Codigo: "imagenSinAlternativo", Detalle: fmt.Sprintf("%d", n), Bloqueante: false,
		})
	}

	bloqueaA, bloqueaUA := false, false
	for _, p := range out.Problemas {
		if !p.Bloqueante {
			continue
		}
		switch p.Codigo {
		case "sinEstructura", "sinIdioma":
			bloqueaUA = true
		default:
			bloqueaA = true
		}
	}
	out.CumplePDFA = !bloqueaA
	out.CumplePDFUA = !bloqueaUA && !sinEtiquetas && !sinIdioma
	out.DeclaraPDFUA = d.declaraPDFUA(raiz, out.CumplePDFUA)
	return out
}

var reDeclaraPDFUA = regexp.MustCompile(`<pdfuaid:part>\s*1\s*</pdfuaid:part>`)

// declaraPDFUA mira la identificación de la norma: MarkInfo/Marked true, DisplayDocTitle true
// y el XMP con pdfuaid:part=1. Que lo declare no lo prueba (para eso está veraPDF, que es
// el control de los tests del exportador): es lo que distingue un PDF hecho para ser
// accesible de uno que solo tiene un árbol de estructura.
func (d *PDFDoc) declaraPDFUA(raiz pdfDict, cumple bool) bool {
	if raiz == nil || !cumple {
		return false
	}
	if mi, _ := d.resolver(raiz["MarkInfo"]).(pdfDict); mi == nil || d.resolver(mi["Marked"]) != true {
		return false
	}
	if vp, _ := d.resolver(raiz["ViewerPreferences"]).(pdfDict); vp == nil || d.resolver(vp["DisplayDocTitle"]) != true {
		return false
	}
	st, _ := d.resolver(raiz["Metadata"]).(*pdfStream)
	if st == nil {
		return false
	}
	xmp, err := d.datosDeFlujo(st)
	return err == nil && reDeclaraPDFUA.Match(xmp)
}

// fuentesSinIncrustar recorre las páginas buscando fuentes cuyo descriptor no
// traiga el fichero de la tipografía. Devuelve sus nombres, sin repetir.
func (d *PDFDoc) fuentesSinIncrustar() []string {
	vistas := map[string]bool{}
	var out []string
	for i := 0; i < d.NumPaginas() && i < 200; i++ {
		p, err := d.Pagina(i)
		if err != nil {
			continue
		}
		recursos, _ := d.resolver(p.dict["Resources"]).(pdfDict)
		cat, _ := d.resolver(recursos["Font"]).(pdfDict)
		for _, k := range clavesOrdenadas(cat) {
			fd, ok := d.resolver(cat[k]).(pdfDict)
			if !ok {
				continue
			}
			nombre := ""
			if b, ok := d.resolver(fd["BaseFont"]).(pdfName); ok {
				nombre = string(b)
			}
			if nombre == "" || vistas[nombre] {
				continue
			}
			vistas[nombre] = true
			if !d.fuenteTieneFichero(fd) {
				out = append(out, nombre)
			}
		}
	}
	return out
}

func (d *PDFDoc) fuenteTieneFichero(fd pdfDict) bool {
	descendiente := fd
	if sub, _ := d.resolver(fd["Subtype"]).(pdfName); sub == "Type0" {
		if arr, ok := d.resolver(fd["DescendantFonts"]).(pdfArray); ok && len(arr) > 0 {
			if dd, ok := d.resolver(arr[0]).(pdfDict); ok {
				descendiente = dd
			}
		}
	}
	desc, ok := d.resolver(descendiente["FontDescriptor"]).(pdfDict)
	if !ok {
		return false
	}
	for _, k := range []pdfName{"FontFile", "FontFile2", "FontFile3"} {
		if _, hay := desc[k]; hay {
			return true
		}
	}
	return false
}

// imagenesSinAlternativo cuenta las imágenes que no llevan texto alternativo.
func (d *PDFDoc) imagenesSinAlternativo() int {
	n := 0
	for i := 0; i < d.NumPaginas() && i < 200; i++ {
		p, err := d.Pagina(i)
		if err != nil {
			continue
		}
		recursos, _ := d.resolver(p.dict["Resources"]).(pdfDict)
		cat, _ := d.resolver(recursos["XObject"]).(pdfDict)
		for _, k := range clavesOrdenadas(cat) {
			st, ok := d.resolver(cat[k]).(*pdfStream)
			if !ok {
				continue
			}
			if sub, _ := d.resolver(st.Dict["Subtype"]).(pdfName); sub != "Image" {
				continue
			}
			// El alternativo de una imagen vive en su elemento de estructura, no en
			// el XObject; sin árbol de estructura no hay ninguno, y eso ya lo dice
			// el problema «sinEstructura». Aquí solo se cuentan para poder decir
			// CUÁNTAS harían falta.
			n++
		}
	}
	return n
}

// --- Declarar ----------------------------------------------------------------

// NivelPDFA es la variante que se declara.
type NivelPDFA string

const (
	// PDFA2B es el nivel «visual»: el documento se verá igual dentro de veinte
	// años. Es el que puede prometer un documento sin estructura etiquetada.
	PDFA2B NivelPDFA = "2B"
	// PDFA2A añade la accesibilidad, y por eso EXIGE estructura: no se declara
	// sobre un documento que no la tenga.
	PDFA2A NivelPDFA = "2A"
)

// OpcionesArchivo describe la declaración que se quiere escribir.
type OpcionesArchivo struct {
	Nivel NivelPDFA
	// Idioma en formato BCP 47 («es-ES»). Obligatorio para PDF/UA.
	Idioma string
	Titulo string
	Autor  string
	// Marcado escribe /MarkInfo << /Marked true >>, que es lo que declara que el
	// contenido está etiquetado. Solo se pone si de verdad hay estructura.
	Fecha time.Time
}

// PrepararArchivoPDF escribe una copia con la declaración de archivo y los
// metadatos que exige.
//
// Se NIEGA a declarar lo que no puede cumplir: sin fuentes incrustadas no hay
// PDF/A, y marcar el fichero igualmente produciría un documento que MIENTE — que
// es peor que uno honesto que no lo promete, porque quien lo archive creerá que
// está a salvo.
func PrepararArchivoPDF(doc *PDFDoc, opts OpcionesArchivo) ([]byte, error) {
	informe := doc.ComprobarArchivo()
	for _, p := range informe.Problemas {
		if p.Codigo == "fuenteSinIncrustar" {
			return nil, fmt.Errorf(
				"pdf: no se puede declarar PDF/A porque estas fuentes no van dentro del fichero: %s",
				p.Detalle,
			)
		}
		if p.Codigo == "cifrado" {
			return nil, fmt.Errorf("pdf: un documento cifrado no se puede archivar; quítale la protección antes")
		}
		if p.Codigo == "conJavaScript" {
			return nil, fmt.Errorf("pdf: este documento ejecuta JavaScript al abrirse y eso no se puede archivar")
		}
	}
	if opts.Nivel == PDFA2A && !informe.CumplePDFUA {
		return nil, fmt.Errorf("pdf: el nivel 2A exige estructura etiquetada e idioma, y este documento no los tiene")
	}
	if opts.Nivel == "" {
		opts.Nivel = PDFA2B
	}
	if opts.Fecha.IsZero() {
		opts.Fecha = time.Now().UTC()
	}

	e := NuevoEnsambladorPDF()
	// PDF/A prohíbe parte de lo que un catálogo puede llevar (JavaScript,
	// adjuntos que no son PDF/A, capas en la parte 1): aquí se conserva solo la
	// lista de abajo, no el catálogo entero.
	e.sinCatalogo = true
	for i := 0; i < doc.NumPaginas(); i++ {
		if err := e.AnadirPagina(doc, i, 0); err != nil {
			return nil, err
		}
	}
	e.ConservarFormulario(doc)

	if e.extrasCatalogo == nil {
		e.extrasCatalogo = pdfDict{}
	}
	e.extrasCatalogo["Metadata"] = e.flujoXMP(xmpDeArchivo(opts))

	if opts.Idioma != "" {
		e.extrasCatalogo["Lang"] = cadenaUTF16(opts.Idioma)
	}
	// La estructura del original se conserva si la había; no se inventa.
	raiz, _ := doc.resolver(doc.trailer["Root"]).(pdfDict)
	if raiz != nil {
		for _, k := range []pdfName{"StructTreeRoot", "MarkInfo", "ViewerPreferences", "Outlines", "PageLabels"} {
			if v, hay := raiz[k]; hay {
				e.extrasCatalogo[k] = e.copiarValor(doc, v)
			}
		}
	}
	return e.Bytes()
}

// flujoXMP escribe el XMP como flujo /Metadata y devuelve su referencia.
func (e *PDFEnsamblador) flujoXMP(xmp string) pdfRef {
	num := e.reservar()
	e.cuerpos[num-1] = objetoSalida{
		valor: pdfDict{
			"Type":    pdfName("Metadata"),
			"Subtype": pdfName("XML"),
			// El XMP va SIN comprimir a propósito: la especificación de PDF/A pide
			// poder leerlo sin descomprimir nada, para que una herramienta de
			// archivo lo encuentre sin entender el resto del fichero.
		},
		flujo: []byte(xmp), esFlujo: true,
	}
	return pdfRef{Num: num}
}

// xmpDeArchivo construye el paquete XMP con el identificador de conformidad.
// Es XML a mano porque es un documento FIJO con cuatro huecos: una biblioteca de
// XMP para esto sería una dependencia por un `Sprintf`.
func xmpDeArchivo(opts OpcionesArchivo) string {
	conformidad := "B"
	if opts.Nivel == PDFA2A {
		conformidad = "A"
	}
	return xmpPDFA(declaracionPDFA{parte: "2", conformidad: conformidad}, opts)
}

// declaracionPDFA es el identificador de conformidad del XMP (pdfaid).
type declaracionPDFA struct{ parte, conformidad, revision string }

var (
	rePDFAParte       = regexp.MustCompile(`pdfaid:part\s*(?:=\s*["']|>)\s*([1-4])`)
	rePDFAConformidad = regexp.MustCompile(`pdfaid:conformance\s*(?:=\s*["']|>)\s*([ABUFEabufe])`)
	rePDFARevision    = regexp.MustCompile(`pdfaid:rev\s*(?:=\s*["']|>)\s*(\d{4})`)
)

// declaraPDFA lee del XMP del documento si se declara PDF/A, y cuál.
func (d *PDFDoc) declaraPDFA() (declaracionPDFA, bool) {
	raiz, _ := d.resolver(d.trailer["Root"]).(pdfDict)
	st, ok := d.resolver(raiz["Metadata"]).(*pdfStream)
	if !ok {
		return declaracionPDFA{}, false
	}
	xmp, err := d.datosDeFlujo(st)
	if err != nil {
		return declaracionPDFA{}, false
	}
	m := rePDFAParte.FindSubmatch(xmp)
	if m == nil {
		return declaracionPDFA{}, false
	}
	dec := declaracionPDFA{parte: string(m[1])}
	if c := rePDFAConformidad.FindSubmatch(xmp); c != nil {
		dec.conformidad = strings.ToUpper(string(c[1]))
	}
	if r := rePDFARevision.FindSubmatch(xmp); r != nil {
		dec.revision = string(r[1])
	}
	return dec, true
}

// xmpPDFA escribe el paquete XMP con la declaración y, si las hay, la fecha, el
// título y el autor de opts.
func xmpPDFA(dec declaracionPDFA, opts OpcionesArchivo) string {
	var b strings.Builder
	b.WriteString("<?xpacket begin=\"\uFEFF\" id=\"W5M0MpCehiHzreSzNTczkc9d\"?>\n")
	b.WriteString(`<x:xmpmeta xmlns:x="adobe:ns:meta/">` + "\n")
	b.WriteString(`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">` + "\n")
	b.WriteString(`<rdf:Description rdf:about="" xmlns:pdfaid="http://www.aiim.org/pdfa/ns/id/">` + "\n")
	b.WriteString(`<pdfaid:part>` + dec.parte + `</pdfaid:part>` + "\n")
	if dec.conformidad != "" {
		b.WriteString(`<pdfaid:conformance>` + dec.conformidad + `</pdfaid:conformance>` + "\n")
	}
	if dec.revision != "" {
		b.WriteString(`<pdfaid:rev>` + dec.revision + `</pdfaid:rev>` + "\n")
	}
	b.WriteString(`</rdf:Description>` + "\n")
	if opts.Fecha.IsZero() {
		b.WriteString(`</rdf:RDF>` + "\n</x:xmpmeta>\n")
		b.WriteString(strings.Repeat(strings.Repeat(" ", 99)+"\n", 4))
		b.WriteString(`<?xpacket end="w"?>`)
		return b.String()
	}
	fecha := opts.Fecha.UTC().Format(time.RFC3339)
	b.WriteString(`<rdf:Description rdf:about="" xmlns:xmp="http://ns.adobe.com/xap/1.0/">` + "\n")
	b.WriteString(`<xmp:CreateDate>` + fecha + `</xmp:CreateDate>` + "\n")
	b.WriteString(`<xmp:ModifyDate>` + fecha + `</xmp:ModifyDate>` + "\n")
	b.WriteString(`<xmp:CreatorTool>Sytue PDF</xmp:CreatorTool>` + "\n")
	b.WriteString(`</rdf:Description>` + "\n")
	b.WriteString(`<rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/">` + "\n")
	if opts.Titulo != "" {
		b.WriteString(`<dc:title><rdf:Alt><rdf:li xml:lang="x-default">` +
			escaparXML(opts.Titulo) + `</rdf:li></rdf:Alt></dc:title>` + "\n")
	}
	if opts.Autor != "" {
		b.WriteString(`<dc:creator><rdf:Seq><rdf:li>` +
			escaparXML(opts.Autor) + `</rdf:li></rdf:Seq></dc:creator>` + "\n")
	}
	b.WriteString(`</rdf:Description>` + "\n")
	b.WriteString(`</rdf:RDF>` + "\n</x:xmpmeta>\n")
	// El relleno final lo pide la especificación de XMP: deja sitio para que una
	// herramienta reescriba los metadatos sin mover el resto del fichero.
	b.WriteString(strings.Repeat(strings.Repeat(" ", 99)+"\n", 4))
	b.WriteString(`<?xpacket end="w"?>`)
	return b.String()
}
