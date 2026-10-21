// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package pdfdoc escribe PDF de texto con fuentes INCRUSTADAS (subconjuntos de Figtree y
// Liberation Sans, vía pdffuente), maquetando con las métricas reales de la fuente.
//
// Es el hermano de pdfsimple, que se queda como está para quien quiera el PDF mínimo de
// catorce fuentes estándar (Notepads, y Docus con «PDF accesible» desactivado). Este
// paquete es la base del PDF etiquetado: una fuente sin incrustar no puede ser PDF/UA ni
// PDF/A, y el ancho de línea de pdfsimple es una estimación (0,5 em por carácter) que con
// una fuente proporcional deja líneas cortas o desborda el margen.
//
// Solo stdlib y pdffuente. No es un motor de maquetación web: párrafos con ajuste de línea
// por palabras, paginación, y lo que cada pieza añada (marcado, tablas, imágenes).
package pdfdoc

import (
	"bytes"
	"cmp"
	"compress/zlib"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"git.sytue.eu/sytue/one/platform/pkg/pdffuente"
)

// Medidas de página en puntos: A4 con márgenes de ~2 cm (las mismas que pdfsimple).
const (
	PageW  = 595.28
	PageH  = 841.89
	Margin = 56.7
)

// Opciones del documento.
type Opciones struct {
	Titulo    string
	Idioma    string // BCP 47 del documento («es», «el»…); vacío = «es»
	Productor string
	Fecha     time.Time // cero = sin fecha de creación (PDF reproducible)
}

// Doc acumula páginas de texto. No es seguro para uso concurrente.
type Doc struct {
	op      Opciones
	pages   []*pagina
	cur     *pagina
	y       float64
	fuentes []*caraUsada
	porCara map[*pdffuente.Fuente]*caraUsada
	avisos  map[rune]int

	raiz    *Elem   // Document
	pila    []*Elem // contenedores abiertos; el último recibe los bloques nuevos
	listas  []*nivelLista
	hUltimo int // nivel del último encabezado etiquetado

	imagenes map[[32]byte]*imgUsada
	ordenImg []*imgUsada
	sinAlt   int
}

type pagina struct {
	contenido bytes.Buffer
	abierto   *Elem   // secuencia de contenido marcado abierta
	mcids     []*Elem // propietario de cada MCID: la entrada de la página en el ParentTree
	anots     []*anotacion
}

// caraUsada es una cara de la pila con su subconjunto y el nombre de recurso («F0»…).
type caraUsada struct {
	f      *pdffuente.Fuente
	sub    *pdffuente.Subconjunto
	nombre string
}

func Nuevo(op Opciones) *Doc {
	if op.Idioma == "" {
		op.Idioma = "es"
	}
	d := &Doc{op: op, porCara: map[*pdffuente.Fuente]*caraUsada{}, avisos: map[rune]int{}, raiz: &Elem{Tipo: "Document"}}
	d.pila = []*Elem{d.raiz}
	d.NuevaPagina()
	return d
}

// NuevaPagina cierra la página en curso y abre otra.
func (d *Doc) NuevaPagina() {
	if d.cur != nil {
		d.cerrarMC()
	}
	d.cur = &pagina{}
	d.pages = append(d.pages, d.cur)
	d.y = PageH - Margin
}

// Hueco baja el cursor h puntos (sin pasar de página: si no cabe, el siguiente texto la abre).
func (d *Doc) Hueco(h float64) { d.y -= h }

// Paginas es el número de páginas escritas hasta ahora.
func (d *Doc) Paginas() int { return len(d.pages) }

// Avisos devuelve cuántas veces se ha sustituido cada carácter que ninguna fuente de la
// pila trae (escrituras no europeas). Nunca se pinta .notdef: PDF/UA lo prohíbe.
func (d *Doc) Avisos() map[rune]int { return d.avisos }

func (d *Doc) cara(f *pdffuente.Fuente) *caraUsada {
	if c := d.porCara[f]; c != nil {
		return c
	}
	c := &caraUsada{f: f, sub: f.NuevoSubconjunto(), nombre: "F" + strconv.Itoa(len(d.fuentes))}
	d.fuentes = append(d.fuentes, c)
	d.porCara[f] = c
	return c
}

// salida implementa pdffuente.Escritor sobre la lista de objetos del fichero.
type salida struct {
	objs [][]byte
	zw   *zlib.Writer
}

func (s *salida) Objeto(cuerpo string) int {
	s.objs = append(s.objs, []byte(cuerpo))
	return len(s.objs)
}

func (s *salida) reservar() int { s.objs = append(s.objs, nil); return len(s.objs) }

func (s *salida) Flujo(dict string, datos []byte) int {
	var z bytes.Buffer
	// Un compresor por fichero, reutilizado: crear uno por flujo (cada página, cada fuente)
	// era el 35 % de la memoria reservada de una exportación (medido el 2026-10-04).
	if s.zw == nil {
		s.zw = zlib.NewWriter(&z)
	} else {
		s.zw.Reset(&z)
	}
	_, _ = s.zw.Write(datos)
	_ = s.zw.Close()
	var b bytes.Buffer
	b.WriteString("<< ")
	if dict != "" {
		b.WriteString(dict + " ")
	}
	b.WriteString("/Filter /FlateDecode /Length " + strconv.Itoa(z.Len()) + " >>\nstream\n")
	b.Write(z.Bytes())
	b.WriteString("\nendstream")
	s.objs = append(s.objs, b.Bytes())
	return len(s.objs)
}

// crudo escribe un flujo sin filtro (los metadatos XMP van sin comprimir: PDF/A y PDF/UA).
func (s *salida) crudo(dict string, datos []byte) int {
	var b bytes.Buffer
	b.WriteString("<< " + dict + " /Length " + strconv.Itoa(len(datos)) + " >>\nstream\n")
	b.Write(datos)
	b.WriteString("\nendstream")
	s.objs = append(s.objs, b.Bytes())
	return len(s.objs)
}

// cadena escribe s como cadena PDF: literal si es ASCII imprimible, UTF-16BE con BOM si no.
func cadena(s string) string {
	ascii := true
	for _, r := range s {
		if r < 0x20 || r > 0x7e {
			ascii = false
			break
		}
	}
	if ascii {
		return "(" + strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`).Replace(s) + ")"
	}
	var b strings.Builder
	b.WriteString("<FEFF")
	for _, u := range utf16.Encode([]rune(s)) {
		fmt.Fprintf(&b, "%04X", u)
	}
	return b.String() + ">"
}

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', 2, 64) }

// pie escribe el número de página como artefacto (7.1: ni el pie ni la cabecera son
// contenido del documento) y devuelve el contenido final de la página.
func (d *Doc) pie(i int) []byte {
	p := d.pages[i]
	var b bytes.Buffer
	b.Write(p.contenido.Bytes())
	if p == d.cur && p.abierto != nil {
		b.WriteString("EMC\n")
	}
	const tam = 9.0
	var hex strings.Builder
	w := 0.0
	for _, pz := range d.piezas([]Fragmento{{Texto: strconv.Itoa(i+1) + " / " + strconv.Itoa(len(d.pages))}}, tam, nil) {
		for _, g := range pz.glifos {
			fmt.Fprintf(&hex, "%04X", g.g)
			pz.cara.sub.Usar(g.g, g.r)
		}
		w += pz.ancho
	}
	if hex.Len() > 0 {
		c := d.cara(pdffuente.Pila(false, false)[0])
		fmt.Fprintf(&b, "/Artifact <</Type /Pagination /Subtype /Footer>> BDC\nq 0.35 g BT /%s %s Tf %s %s Td <%s> Tj ET Q\nEMC\n",
			c.nombre, ftoa(tam), ftoa((PageW-w)/2), ftoa(Margin/2), hex.String())
	}
	return b.Bytes()
}

// Bytes ensambla el fichero: el pie de cada página, las fuentes (los subconjuntos se
// escriben ahora, cuando ya se sabe qué glifos hacen falta), el contenido, las anotaciones,
// el árbol de estructura con su ParentTree, los metadatos XMP, el catálogo, xref y trailer.
func (d *Doc) Bytes() ([]byte, error) {
	titulo := cmp.Or(d.op.Titulo, "Sin título")
	productor := cmp.Or(d.op.Productor, "Sytue Docs")
	n := len(d.pages)
	contenidos := make([][]byte, n)
	for i := range d.pages {
		contenidos[i] = d.pie(i)
	}

	s := &salida{}
	catalogo, arbol := s.reservar(), s.reservar()
	fuentes := make([]string, 0, len(d.fuentes))
	for _, c := range d.fuentes {
		fn, err := c.sub.Escribir(s)
		if err != nil {
			return nil, err
		}
		fuentes = append(fuentes, "/"+c.nombre+" "+strconv.Itoa(fn)+" 0 R")
	}
	recursos := "<< /Font << " + strings.Join(fuentes, " ") + " >>" + d.escribirImagenes(s) + " >>"

	pagObj := make(map[*pagina]int, n)
	for _, p := range d.pages {
		pagObj[p] = s.reservar()
	}
	key := n
	for _, p := range d.pages {
		for _, a := range p.anots {
			a.obj, a.key = s.reservar(), key
			key++
		}
	}
	e := &emisor{s: s, pag: pagObj, duenos: map[*anotacion]*Elem{}}
	e.asignar(d.raiz)
	raizEstr := s.reservar()
	parentTree := s.reservar()

	kids := make([]string, 0, n)
	for i, p := range d.pages {
		c := s.Flujo("", contenidos[i])
		anots := ""
		if len(p.anots) > 0 {
			refs := make([]string, len(p.anots))
			for k, a := range p.anots {
				refs[k] = strconv.Itoa(a.obj) + " 0 R"
			}
			anots = " /Annots [" + strings.Join(refs, " ") + "]"
		}
		s.objs[pagObj[p]-1] = []byte("<< /Type /Page /Parent " + strconv.Itoa(arbol) + " 0 R /MediaBox [0 0 " +
			ftoa(PageW) + " " + ftoa(PageH) + "] /Resources " + recursos + " /Contents " + strconv.Itoa(c) +
			" 0 R /Tabs /S /StructParents " + strconv.Itoa(i) + anots + " >>")
		kids = append(kids, strconv.Itoa(pagObj[p])+" 0 R")
	}
	s.objs[arbol-1] = []byte("<< /Type /Pages /Kids [" + strings.Join(kids, " ") + "] /Count " + strconv.Itoa(n) + " >>")
	e.emitir(d.raiz, raizEstr)
	for _, p := range d.pages {
		for _, a := range p.anots {
			s.objs[a.obj-1] = []byte("<< /Type /Annot /Subtype /Link /Rect [" + ftoa(a.rect[0]) + " " + ftoa(a.rect[1]) + " " +
				ftoa(a.rect[2]) + " " + ftoa(a.rect[3]) + "] /Border [0 0 0] /F 4 /Contents " + cadena(a.contents) +
				" /StructParent " + strconv.Itoa(a.key) + " /A << /S /URI /URI " + cadena(a.uri) + " >> /P " +
				strconv.Itoa(pagObj[p]) + " 0 R >>")
		}
	}

	var nums strings.Builder
	for i, p := range d.pages {
		nums.WriteString(strconv.Itoa(i) + " [")
		for _, dueno := range p.mcids {
			nums.WriteString(strconv.Itoa(dueno.obj) + " 0 R ")
		}
		nums.WriteString("] ")
	}
	for _, p := range d.pages { // las claves de anotación van después y en orden: un number tree se ordena por clave
		for _, a := range p.anots {
			nums.WriteString(strconv.Itoa(a.key) + " " + strconv.Itoa(e.duenos[a].obj) + " 0 R ")
		}
	}
	s.objs[parentTree-1] = []byte("<< /Nums [" + strings.TrimSpace(nums.String()) + "] >>")
	s.objs[raizEstr-1] = []byte("<< /Type /StructTreeRoot /K " + strconv.Itoa(d.raiz.obj) + " 0 R /ParentTree " +
		strconv.Itoa(parentTree) + " 0 R /ParentTreeNextKey " + strconv.Itoa(key) + " /RoleMap << >> >>")

	meta := s.crudo("/Type /Metadata /Subtype /XML", xmp(titulo, productor, d.op.Idioma, d.op.Fecha))
	info := "<< /Title " + cadena(titulo) + " /Producer " + cadena(productor)
	if !d.op.Fecha.IsZero() {
		info += " /CreationDate (D:" + d.op.Fecha.UTC().Format("20060102150405") + "Z)"
	}
	infoN := s.Objeto(info + " >>")
	s.objs[catalogo-1] = []byte("<< /Type /Catalog /Pages " + strconv.Itoa(arbol) + " 0 R /Lang " + cadena(d.op.Idioma) +
		" /MarkInfo << /Marked true >> /ViewerPreferences << /DisplayDocTitle true >> /StructTreeRoot " +
		strconv.Itoa(raizEstr) + " 0 R /Metadata " + strconv.Itoa(meta) + " 0 R >>")

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	offs := make([]int, len(s.objs))
	for i, o := range s.objs {
		offs[i] = buf.Len()
		buf.WriteString(strconv.Itoa(i+1) + " 0 obj\n")
		buf.Write(o)
		buf.WriteString("\nendobj\n")
	}
	xref := buf.Len()
	buf.WriteString("xref\n0 " + strconv.Itoa(len(s.objs)+1) + "\n0000000000 65535 f \n")
	for _, o := range offs {
		fmt.Fprintf(&buf, "%010d 00000 n \n", o)
	}
	buf.WriteString("trailer\n<< /Size " + strconv.Itoa(len(s.objs)+1) + " /Root " + strconv.Itoa(catalogo) +
		" 0 R /Info " + strconv.Itoa(infoN) + " 0 R >>\nstartxref\n" + strconv.Itoa(xref) + "\n%%EOF\n")
	return buf.Bytes(), nil
}
