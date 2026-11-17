// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package uacheck es un verificador estructural de PDF/UA-1 (ISO 14289-1) para los PDF que
// escribe pdfdoc. NO es un validador general —para eso está veraPDF, que se usa como control
// externo en desarrollo—: lee los objetos como los escribe pdfdoc (diccionarios en claro,
// flujos de contenido en Flate) y comprueba lo que hace falta para que el escritor no
// regrese sin que nadie lo vea y sin necesitar Java en el CI:
//
//   - catálogo: MarkInfo, Lang, DisplayDocTitle, StructTreeRoot y XMP (pdfuaid:part, dc:title);
//   - contenido: todo texto y todo trazo dentro de una secuencia marcada (real o artefacto),
//     MCID sin huecos y recogidos en el ParentTree;
//   - estructura: tipos estándar, anidación válida (L›LI›Lbl/LBody, Table›TR›TH/TD…),
//     encabezados que empiezan en H1 y no saltan, Figure con /Alt, TH con Scope, tablas
//     regulares, Link con anotación (/Contents, /StructParent) y /Lang válidos;
//   - fuentes: todas incrustadas, Type0 Identity-H con /ToUnicode.
package uacheck

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	reObj     = regexp.MustCompile(`(?s)(\d+) 0 obj\n(.*?)\nendobj\n`)
	reRef     = regexp.MustCompile(`(\d+) 0 R`)
	reMCID    = regexp.MustCompile(`/MCID (\d+)`)
	reIdioma  = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)
	reSEntero = regexp.MustCompile(`/S /(\w+)`)
)

// tipos estándar de estructura que pdfdoc usa (ISO 32000-1, 14.8.4); cualquier otro exigiría
// un /RoleMap y pdfdoc no lo emite.
var estandar = map[string]bool{
	"Document": true, "H1": true, "H2": true, "H3": true, "H4": true, "H5": true, "H6": true, "P": true,
	"L": true, "LI": true, "Lbl": true, "LBody": true, "Table": true, "TR": true, "TH": true, "TD": true,
	"Figure": true, "Link": true, "Span": true, "BlockQuote": true,
}

// hijos admitidos de cada contenedor; los que no figuran admiten cualquier elemento en línea.
var permitidos = map[string][]string{
	"L": {"LI"}, "LI": {"Lbl", "LBody"}, "Table": {"TR"}, "TR": {"TH", "TD"},
	"Document": {"H1", "H2", "H3", "H4", "H5", "H6", "P", "L", "Table", "Figure", "BlockQuote"},
}

type elem struct {
	n     int
	tipo  string
	padre int
	dict  string
	hijos []int // objetos de estructura
	obj   int   // OBJR que cuelgan directamente
}

// Verificar devuelve los incumplimientos encontrados; vacío = conforme a lo que comprueba.
func Verificar(pdf []byte) []string {
	v := &verif{objs: map[int]string{}, elems: map[int]*elem{}}
	for _, m := range reObj.FindAllSubmatch(pdf, -1) {
		n, _ := strconv.Atoi(string(m[1]))
		v.objs[n] = string(m[2])
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) || len(v.objs) == 0 {
		return []string{"no es un PDF de pdfdoc"}
	}
	v.catalogo()
	v.paginas()
	v.estructura()
	v.fuentes()
	return v.errs
}

type verif struct {
	objs  map[int]string
	elems map[int]*elem
	errs  []string
	raiz  int
	pagN  []int
	mcids map[int]int // por página (objeto): cuántos MCID tiene el contenido
}

func (v *verif) fallo(f string, a ...any) { v.errs = append(v.errs, fmt.Sprintf(f, a...)) }

func ref(s, clave string) int {
	i := strings.Index(s, clave+" ")
	if i < 0 {
		return 0
	}
	m := reRef.FindStringSubmatch(s[i+len(clave):])
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func (v *verif) catalogo() {
	var cat string
	for _, b := range v.objs {
		if strings.Contains(b, "/Type /Catalog") {
			cat = b
		}
	}
	if cat == "" {
		v.fallo("sin catálogo")
		return
	}
	for _, c := range []struct{ quiero, msg string }{
		{"/MarkInfo << /Marked true >>", "7.1: falta MarkInfo/Marked true"},
		{"/ViewerPreferences << /DisplayDocTitle true >>", "7.1: falta DisplayDocTitle true"},
		{"/StructTreeRoot ", "7.1: falta StructTreeRoot"},
	} {
		if !strings.Contains(cat, c.quiero) {
			v.fallo("%s", c.msg)
		}
	}
	if l, mal := langInvalido(cat); mal || !strings.Contains(cat, "/Lang (") {
		v.fallo("7.2: falta /Lang válido en el catálogo (%q)", l)
	}
	v.raiz = ref(cat, "/StructTreeRoot")
	xmp := v.objs[ref(cat, "/Metadata")]
	if !strings.Contains(xmp, "<pdfuaid:part>1</pdfuaid:part>") {
		v.fallo("5: el XMP no declara pdfuaid:part=1")
	}
	if !strings.Contains(xmp, "<dc:title>") {
		v.fallo("7.1: el XMP no tiene dc:title")
	}
}

func inflar(body string) (string, error) {
	i := strings.Index(body, "stream\n")
	j := strings.LastIndex(body, "\nendstream")
	if i < 0 || j < i {
		return "", fmt.Errorf("sin flujo")
	}
	d := body[i+7 : j]
	if !strings.Contains(body[:i], "/FlateDecode") {
		return d, nil
	}
	r, err := zlib.NewReader(strings.NewReader(d))
	if err != nil {
		return "", err
	}
	out, err := io.ReadAll(r)
	return string(out), err
}

func (v *verif) paginas() {
	v.mcids = map[int]int{}
	for n, b := range v.objs {
		if !strings.Contains(b, "/Type /Page /Parent") {
			continue
		}
		v.pagN = append(v.pagN, n)
		if !strings.Contains(b, "/Tabs /S") {
			v.fallo("7.18.3: la página %d no declara /Tabs /S", n)
		}
		if !strings.Contains(b, "/StructParents ") {
			v.fallo("7.1: la página %d no tiene /StructParents", n)
		}
		cont, err := inflar(v.objs[ref(b, "/Contents")])
		if err != nil {
			v.fallo("página %d: contenido ilegible: %v", n, err)
			continue
		}
		v.mcids[n] = v.contenido(n, cont)
	}
	slices.Sort(v.pagN)
}

// contenido verifica el marcado de un flujo y devuelve cuántos MCID declara.
func (v *verif) contenido(pag int, cont string) int {
	prof, mcid := 0, 0
	for i, ln := range strings.Split(cont, "\n") {
		switch {
		case strings.HasSuffix(ln, "BDC") || strings.HasSuffix(ln, "BMC"):
			if prof > 0 {
				v.fallo("página %d, línea %d: secuencia marcada anidada", pag, i)
			}
			prof++
			if m := reMCID.FindStringSubmatch(ln); m != nil {
				if id, _ := strconv.Atoi(m[1]); id != mcid {
					v.fallo("página %d: MCID %d donde tocaba %d", pag, id, mcid)
				}
				mcid++
			}
		case ln == "EMC":
			prof--
			if prof < 0 {
				v.fallo("página %d, línea %d: EMC de más", pag, i)
				prof = 0
			}
		case strings.Contains(ln, " BT ") || strings.HasPrefix(ln, "BT "), strings.Contains(ln, " re f"), strings.Contains(ln, " l S"),
			strings.Contains(ln, " re S"), strings.Contains(ln, " Do"):
			if prof == 0 {
				v.fallo("7.1: página %d, línea %d: contenido sin marcar ni declarar artefacto", pag, i)
			}
		}
	}
	if prof != 0 {
		v.fallo("página %d: secuencia marcada sin cerrar", pag)
	}
	return mcid
}

var reLang = regexp.MustCompile(`/Lang \(([^)]*)\)`)

func langInvalido(dict string) (string, bool) {
	m := reLang.FindStringSubmatch(dict)
	if len(m) < 2 {
		return "", false
	}
	return m[1], !reIdioma.MatchString(m[1])
}

// cargar lee los elementos de estructura alcanzables desde n, en orden de lectura.
func (v *verif) cargar(n, padre int) {
	b := v.objs[n]
	m := reSEntero.FindStringSubmatch(b)
	if m == nil {
		v.fallo("el objeto %d de la estructura no tiene /S", n)
		return
	}
	e := &elem{n: n, tipo: m[1], padre: padre, dict: b}
	v.elems[n] = e
	if !estandar[e.tipo] {
		v.fallo("7.1: tipo de estructura no estándar /%s", e.tipo)
	}
	k := ""
	if i := strings.Index(b, "/K "); i >= 0 {
		k = b[i:]
		if j := strings.Index(k, " /A <<"); j >= 0 {
			k = k[:j]
		}
	}
	e.obj = strings.Count(k, "/OBJR")
	for _, m := range reRef.FindAllStringSubmatch(k, -1) {
		if strings.Contains(k, "/Obj "+m[1]+" 0 R") {
			continue
		}
		h, _ := strconv.Atoi(m[1])
		if strings.Contains(v.objs[h], "/Type /StructElem") {
			e.hijos = append(e.hijos, h)
			v.cargar(h, n)
		}
	}
}

func (v *verif) estructura() {
	if v.raiz == 0 {
		return
	}
	root := v.objs[v.raiz]
	top := ref(root, "/K")
	v.cargar(top, 0)
	v.encabezados(top)
	ordenados := make([]*elem, 0, len(v.elems))
	for _, e := range v.elems {
		ordenados = append(ordenados, e)
	}
	slices.SortFunc(ordenados, func(a, b *elem) int { return a.n - b.n })
	for _, e := range ordenados {
		v.reglas(e)
	}
	v.parentTree(v.objs[ref(root, "/ParentTree")])
	v.anotaciones()
}

// reglas son las comprobaciones de un elemento por su tipo.
func (v *verif) reglas(e *elem) {
	if p, ok := permitidos[e.tipo]; ok {
		for _, h := range e.hijos {
			if !slices.Contains(p, v.elems[h].tipo) {
				v.fallo("7.1: /%s no admite hijos /%s (objeto %d)", e.tipo, v.elems[h].tipo, h)
			}
		}
	}
	switch e.tipo {
	case "Figure":
		if !strings.Contains(e.dict, "/Alt (") {
			v.fallo("7.3: Figure sin /Alt (objeto %d)", e.n)
		}
	case "TH":
		if !strings.Contains(e.dict, "/Scope /") {
			v.fallo("7.5: TH sin Scope (objeto %d)", e.n)
		}
	case "Link":
		if e.obj == 0 {
			v.fallo("7.18.5: Link sin anotación (OBJR) (objeto %d)", e.n)
		}
	case "Table":
		v.tabla(e)
	}
	if l, mal := langInvalido(e.dict); mal {
		v.fallo("7.2: /Lang inválido %q (objeto %d)", l, e.n)
	}
}

// parentTree comprueba que cada página tiene tantos propietarios como MCID declara.
func (v *verif) parentTree(nums string) {
	porClave := map[int]int{}
	for _, m := range regexp.MustCompile(`(\d+) \[([^\]]*)\]`).FindAllStringSubmatch(nums, -1) {
		k, _ := strconv.Atoi(m[1])
		porClave[k] = len(reRef.FindAllString(m[2], -1))
	}
	for _, p := range v.pagN {
		clave := 0
		if m := regexp.MustCompile(`/StructParents (\d+)`).FindStringSubmatch(v.objs[p]); m != nil {
			clave, _ = strconv.Atoi(m[1])
		}
		if porClave[clave] != v.mcids[p] {
			v.fallo("7.1: la página %d tiene %d MCID y el ParentTree %d propietarios", p, v.mcids[p], porClave[clave])
		}
	}
}

func (v *verif) anotaciones() {
	for n, b := range v.objs {
		if !strings.Contains(b, "/Subtype /Link") || !strings.Contains(b, "/Type /Annot") {
			continue
		}
		if !strings.Contains(b, "/Contents ") {
			v.fallo("7.18.5: la anotación %d no tiene /Contents", n)
		}
		if !strings.Contains(b, "/StructParent ") {
			v.fallo("7.18.1: la anotación %d no está en la estructura (/StructParent)", n)
		}
	}
}

// encabezados comprueba el orden de H1…H6 en orden de lectura: empieza en H1 y no salta.
func (v *verif) encabezados(n int) {
	ultimo := 0
	var rec func(n int)
	rec = func(n int) {
		e := v.elems[n]
		if e == nil {
			return
		}
		if len(e.tipo) == 2 && e.tipo[0] == 'H' && e.tipo[1] >= '1' && e.tipo[1] <= '6' {
			lvl := int(e.tipo[1] - '0')
			if lvl > ultimo+1 {
				v.fallo("7.4.2: encabezado H%d tras H%d (salto de nivel)", lvl, ultimo)
			}
			ultimo = lvl
		}
		for _, h := range e.hijos {
			rec(h)
		}
	}
	rec(n)
}

// tabla comprueba que todas las filas ocupan las mismas columnas, contando ColSpan y las
// celdas que bajan de filas anteriores (RowSpan).
func (v *verif) tabla(t *elem) {
	enfila := map[int]int{} // columna → filas que le quedan ocupadas por una combinación vertical
	ancho := -1
	for _, tr := range t.hijos {
		cols := 0
		for _, c := range v.elems[tr].hijos {
			for enfila[cols] > 0 {
				enfila[cols]--
				cols++
			}
			cs, rs := 1, 1
			if m := regexp.MustCompile(`/ColSpan (\d+)`).FindStringSubmatch(v.elems[c].dict); m != nil {
				cs, _ = strconv.Atoi(m[1])
			}
			if m := regexp.MustCompile(`/RowSpan (\d+)`).FindStringSubmatch(v.elems[c].dict); m != nil {
				rs, _ = strconv.Atoi(m[1])
			}
			for k := range cs {
				if rs > 1 {
					enfila[cols+k] = rs - 1
				}
			}
			cols += cs
		}
		for enfila[cols] > 0 { // columnas del final ocupadas por celdas de arriba
			enfila[cols]--
			cols++
		}
		if ancho < 0 {
			ancho = cols
		} else if cols != ancho {
			v.fallo("7.5: la tabla %d no es regular (fila con %d columnas, otras con %d)", t.n, cols, ancho)
		}
	}
}

func (v *verif) fuentes() {
	for n, b := range v.objs {
		if !strings.Contains(b, "/Type /Font ") {
			continue
		}
		switch {
		case strings.Contains(b, "/Subtype /Type0"):
			if !strings.Contains(b, "/Encoding /Identity-H") || !strings.Contains(b, "/ToUnicode ") {
				v.fallo("7.21: la fuente %d no es Identity-H con /ToUnicode", n)
			}
		case strings.Contains(b, "/Subtype /CIDFontType2"):
			d := v.objs[ref(b, "/FontDescriptor")]
			if !strings.Contains(d, "/FontFile2 ") || !strings.Contains(b, "/CIDToGIDMap /Identity") {
				v.fallo("7.21.3: la fuente %d no está incrustada con CIDToGIDMap", n)
			}
		default:
			v.fallo("7.21.4: la fuente %d no está incrustada (%s)", n, strings.TrimSpace(b[:min(60, len(b))]))
		}
	}
}
