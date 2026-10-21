// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// APLICAR UN TEMA a un documento, una hoja o una presentación SIN tocar su contenido
// (ROADMAP-OFFICE plantillas-1). Solo cambia lo que es presentación:
//
//   - Documento: el color y la tipografía de los títulos, el relleno y la tinta de la
//     cabecera de las tablas y la tipografía del cuerpo, como atributos de estilo de
//     los elementos. Lo que el autor ya fijó a mano no se pisa, y lo que pone el tema
//     lleva `data-tema` para poder cambiarlo o quitarlo después sin tocar lo demás.
//   - Hoja: el relleno, la tinta y la tipografía de la fila de cabecera (la primera con
//     contenido de cada hoja). Las celdas, fórmulas, gráficos y todo lo que este código no
//     conoce se conservan: el modelo se edita como JSON genérico, no como un struct que
//     descartaría lo que no declara.
//   - Presentación: el tema de la presentación (`theme`, y `themes` para la marca).
//
// Aplicar el mismo tema dos veces da el mismo resultado, y aplicar «papel» quita lo que
// puso otro tema. Antes de aplicarlo a un documento que ya existe, el servicio guarda
// una versión del estado de partida (restaurar la deshace).

// AplicarTema devuelve `contenido` (el valor del registro de contenido del tipo) con el
// tema aplicado. Un contenido que no se puede leer es ErrInvalidFormat.
func AplicarTema(kind Kind, contenido string, t TemaComun) (string, error) {
	switch kind {
	case KindDocument:
		return aplicarTemaTexto(contenido, t)
	case KindSpreadsheet:
		return aplicarTemaHoja(contenido, t)
	case KindPresentation:
		return aplicarTemaDiapos(contenido, t)
	default:
		return "", ErrInvalidKind
	}
}

// OpTema es la operación que aplica el tema al contenido vigente de un estado: un `set`
// del registro de contenido con un reloj mayor que el que tiene. cambia=false si el
// contenido ya lo tenía aplicado (no hay nada que escribir).
func OpTema(kind Kind, st *State, t TemaComun) (op Op, cambia bool, err error) {
	clave := ContentKey(kind)
	actual := st.contentString(clave)
	if actual == "" {
		return Op{}, false, fmt.Errorf("%w: el documento no tiene contenido que tematizar", ErrInvalidFormat)
	}
	nuevo, err := AplicarTema(kind, actual, t)
	if err != nil {
		return Op{}, false, err
	}
	if nuevo == actual {
		return Op{}, false, nil
	}
	valor, err := json.Marshal(nuevo)
	if err != nil {
		return Op{}, false, err
	}
	tok, err := NewToken()
	if err != nil {
		return Op{}, false, err
	}
	reloj := max(st.Registers[clave].Lamport, st.Lamport) + 1
	return Op{
		Kind: OpSet, Key: clave, Value: valor,
		Dot: Dot{Actor: "tema:" + tok[:16], Counter: 1}, Lamport: reloj,
	}, true, nil
}

// --- Documento de texto ---

func aplicarTemaTexto(contenido string, t TemaComun) (string, error) {
	cuerpo := &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body}
	nodos, err := xhtml.ParseFragment(strings.NewReader(contenido), cuerpo)
	if err != nil {
		return "", fmt.Errorf("%w: HTML ilegible: %w", ErrInvalidFormat, err)
	}
	cambio := false
	for _, n := range nodos {
		temaEnNodo(n, t, &cambio)
	}
	if conLogo, tocado := logoEnTexto(nodos, t); tocado {
		nodos, cambio = conLogo, true
	}
	if !cambio {
		return contenido, nil // nada que estilar: el HTML se queda exactamente como estaba
	}
	var b strings.Builder
	for _, n := range nodos {
		if err := xhtml.Render(&b, n); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

// temaEnNodo recorre el árbol y estila los elementos de títulos, cabecera de tabla y cuerpo.
func temaEnNodo(n *xhtml.Node, t TemaComun, cambio *bool) {
	// El párrafo del logo no es contenido: lo gestiona logoEnTexto, entero. Estilarlo aquí
	// lo cambiaba en la SEGUNDA aplicación (la primera lo añade ya recorrido el árbol).
	if n.Type == xhtml.ElementNode && attrVal(n, attrLogoMarca) != "" {
		return
	}
	if n.Type == xhtml.ElementNode && estilarElemento(n, t) {
		*cambio = true
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		temaEnNodo(c, t, cambio)
	}
}

// estilarElemento aplica el tema a un elemento y dice si lo cambió.
func estilarElemento(n *xhtml.Node, t TemaComun) bool {
	var quiere [][2]string
	switch n.DataAtom {
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		quiere = [][2]string{{"color", t.Accent}, {"font-family", fuenteCSS(t.HeadingFont)}}
	case atom.Th:
		quiere = [][2]string{{"background-color", t.Accent}, {"color", t.AccentInk}, {"font-family", fuenteCSS(t.HeadingFont)}}
	case atom.P, atom.Li, atom.Td, atom.Blockquote:
		quiere = [][2]string{{"font-family", fuenteCSS(t.BodyFont)}}
	default:
		return false
	}
	piezas := splitEstilo(attrVal(n, "style"))
	cambio := false
	// Lo que puso un tema antes se quita (y solo eso: lo que el autor fijó a mano ya
	// estaba y no figura en `data-tema`); se vuelve a poner el de ahora, o ninguno.
	if previo := attrVal(n, "data-tema"); previo != "" {
		_, previas, _ := strings.Cut(previo, ":")
		piezas = quitarPropiedades(piezas, strings.Split(previas, ","))
		cambio = true
	}
	var puestas []string
	if t.ID != IDTemaPapel {
		if n.DataAtom == atom.Th && (tienePropiedad(piezas, "background-color") || tienePropiedad(piezas, "color")) {
			quiere = nil // una cabecera con colores propios se queda con ellos, entera
		}
		for _, kv := range quiere {
			if kv[1] == "" || tienePropiedad(piezas, kv[0]) {
				continue
			}
			piezas = append(piezas, kv[0]+":"+kv[1])
			puestas = append(puestas, kv[0])
		}
		if len(puestas) > 0 {
			cambio = true
		}
	}
	// El style solo se reescribe si el tema lo tocó: lo demás se queda byte a byte.
	if !cambio {
		return false
	}
	delAttr(n, "style")
	delAttr(n, "data-tema")
	if len(piezas) > 0 {
		setAttr(n, "style", strings.Join(piezas, ";"))
	}
	// «<tema>:<propiedades que puso>»: así cambiar o quitar el tema deshace lo suyo y nada más.
	if len(puestas) > 0 {
		setAttr(n, "data-tema", t.ID+":"+strings.Join(puestas, ","))
	}
	return true
}

// fuenteCSS es la familia lista para un `font-family`: sin comillas (un nombre con
// espacios es válido así), salvo que empiece por una cifra o lleve un punto; vacía si
// no hay.
func fuenteCSS(f string) string {
	if f != "" && (strings.Contains(f, ".") || (f[0] >= '0' && f[0] <= '9')) {
		return "'" + f + "'"
	}
	return f
}

// splitEstilo parte un atributo style en sus declaraciones SIN interpretarlo: lo que no
// tiene «:» (la cola de un `url(data:…;base64,…)` partido por su «;») se conserva tal cual.
func splitEstilo(style string) []string {
	var out []string
	for _, p := range strings.Split(style, ";") {
		if strings.TrimSpace(p) != "" {
			out = append(out, strings.TrimSpace(p))
		}
	}
	return out
}

func nombrePropiedad(p string) string {
	k, _, ok := strings.Cut(p, ":")
	if !ok {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(k))
}

func tienePropiedad(piezas []string, nombre string) bool {
	return slices.ContainsFunc(piezas, func(p string) bool { return nombrePropiedad(p) == nombre })
}

func quitarPropiedades(piezas, nombres []string) []string {
	return slices.DeleteFunc(piezas, func(p string) bool { return slices.Contains(nombres, nombrePropiedad(p)) })
}

func attrVal(n *xhtml.Node, k string) string {
	for _, a := range n.Attr {
		if a.Key == k {
			return a.Val
		}
	}
	return ""
}

func setAttr(n *xhtml.Node, k, v string) {
	for i := range n.Attr {
		if n.Attr[i].Key == k {
			n.Attr[i].Val = v
			return
		}
	}
	n.Attr = append(n.Attr, xhtml.Attribute{Key: k, Val: v})
}

func delAttr(n *xhtml.Node, k string) {
	n.Attr = slices.DeleteFunc(n.Attr, func(a xhtml.Attribute) bool { return a.Key == k })
}

// --- Hoja de cálculo ---

var reRefCelda = regexp.MustCompile(`^([A-Z]{1,3})(\d{1,7})$`)

// decodificarJSON lee un objeto JSON conservando los números tal como vienen.
func decodificarJSON(contenido string) (map[string]any, error) {
	dec := json.NewDecoder(strings.NewReader(contenido))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil || m == nil {
		return nil, fmt.Errorf("%w: el contenido no es un objeto JSON", ErrInvalidFormat)
	}
	return m, nil
}

// codificarJSON es json.Marshal sin escapar <, > y &: las celdas guardan HTML.
func codificarJSON(m map[string]any) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return "", err
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// acentosConocidos son los acentos de todos los temas del catálogo y el de la marca
// vigente: una cabecera con uno de ellos como relleno se tiene por puesta por un tema.
func acentosConocidos(t TemaComun) []string {
	catalogo := CatalogoTemas()
	out := make([]string, 0, 1+len(catalogo))
	out = append(out, strings.ToUpper(t.Accent))
	for _, c := range catalogo {
		out = append(out, strings.ToUpper(c.Accent))
	}
	return out
}

// sinCambio devuelve `contenido` tal cual si `nuevo` solo difiere de él en cómo se
// serializa (el orden de las claves, los espacios): nada que escribir.
func sinCambio(contenido, nuevo string) (string, error) {
	orig, err := decodificarJSON(contenido)
	if err != nil {
		return "", err
	}
	if canon, err := codificarJSON(orig); err == nil && canon == nuevo {
		return contenido, nil
	}
	return nuevo, nil
}

func aplicarTemaHoja(contenido string, t TemaComun) (string, error) {
	m, err := decodificarJSON(contenido)
	if err != nil {
		return "", err
	}
	hojas, _ := m["sheets"].([]any)
	conocidos := acentosConocidos(t)
	for _, h := range hojas {
		if hoja, ok := h.(map[string]any); ok {
			estilarCabeceraDeHoja(hoja, t, conocidos)
		}
	}
	nuevo, err := codificarJSON(m)
	if err != nil {
		return "", err
	}
	return sinCambio(contenido, nuevo)
}

// estilarCabeceraDeHoja pone el tema en las celdas de la primera fila con contenido de la hoja.
func estilarCabeceraDeHoja(hoja map[string]any, t TemaComun, conocidos []string) {
	celdas, _ := hoja["cells"].(map[string]any)
	fila := primeraFilaConContenido(celdas)
	if fila == 0 {
		return
	}
	estilos, _ := hoja["styles"].(map[string]any)
	if estilos == nil {
		estilos = map[string]any{}
	}
	for ref, v := range celdas {
		if s, _ := v.(string); s == "" || filaDeRef(ref) != fila {
			continue
		}
		est, _ := estilos[ref].(map[string]any)
		if est == nil {
			est = map[string]any{}
		}
		if !estilarCabeceraHoja(est, t, conocidos) {
			continue
		}
		if len(est) == 0 {
			delete(estilos, ref)
		} else {
			estilos[ref] = est
		}
	}
	// Y las celdas que un bloque insertado marcó como de marca, estén donde estén.
	estilarCeldasDeTema(estilos, t, conocidos, fila)
	if len(estilos) > 0 {
		hoja["styles"] = estilos
	}
}

// filaDeRef es el número de fila (1-based) de una referencia «B3»; 0 si no lo es.
func filaDeRef(ref string) int {
	mm := reRefCelda.FindStringSubmatch(ref)
	if len(mm) < 3 {
		return 0
	}
	n, _ := strconv.Atoi(mm[2])
	return n
}

// primeraFilaConContenido es el número de la primera fila (1-based) con alguna celda no
// vacía; 0 si la hoja está vacía.
func primeraFilaConContenido(celdas map[string]any) int {
	menor := 0
	for ref, v := range celdas {
		if s, _ := v.(string); s == "" {
			continue
		}
		if n := filaDeRef(ref); n > 0 && (menor == 0 || n < menor) {
			menor = n
		}
	}
	return menor
}

// estilarCabeceraHoja pone el tema en el estilo de una celda de cabecera, si procede
// (suya, sin estilo o con el acento de algún tema), y dice si tocó el estilo.
func estilarCabeceraHoja(est map[string]any, t TemaComun, conocidos []string) bool {
	puestoPorTema := est["tema"] != nil
	relleno, _ := est["fill"].(string)
	tinta, _ := est["color"].(string)
	propia := !puestoPorTema && (relleno != "" || tinta != "") && !slices.Contains(conocidos, strings.ToUpper(relleno))
	if propia {
		return false
	}
	if puestoPorTema {
		delete(est, "fill")
		delete(est, "color")
		delete(est, "font")
		delete(est, "tema")
	}
	if t.ID == IDTemaPapel {
		return puestoPorTema
	}
	est["fill"], est["color"], est["tema"] = t.Accent, t.AccentInk, t.ID
	if t.HeadingFont != "" {
		if _, hay := est["font"]; !hay {
			est["font"] = t.HeadingFont
		}
	}
	return true
}

// --- Presentación ---

// idTemaMarcaDiapos es el id del tema propio que lleva la marca dentro de una presentación.
const idTemaMarcaDiapos = "custom-marca"

func aplicarTemaDiapos(contenido string, t TemaComun) (string, error) {
	m, err := decodificarJSON(contenido)
	if err != nil {
		return "", err
	}
	propios, _ := m["themes"].([]any)
	propios = slices.DeleteFunc(propios, func(v any) bool {
		o, _ := v.(map[string]any)
		return o != nil && o["id"] == idTemaMarcaDiapos
	})
	switch {
	case t.ID == IDTemaPapel:
		delete(m, "theme")
	case t.Marca:
		if len(propios) >= maxTemasPropios {
			return "", fmt.Errorf("%w: la presentación ya tiene %d temas propios", ErrInvalidOp, maxTemasPropios)
		}
		fuente := t.BodyFont
		if fuente == "" {
			fuente = t.HeadingFont
		}
		tema := map[string]any{"id": idTemaMarcaDiapos, "name": t.Name, "bg": t.Bg, "ink": t.Ink}
		if fuente != "" {
			tema["font"] = fuente
		}
		propios = append(propios, tema)
		m["theme"] = idTemaMarcaDiapos
	default:
		m["theme"] = t.ID
	}
	if len(propios) > 0 {
		m["themes"] = propios
	} else {
		delete(m, "themes")
	}
	logoEnDiapos(m, t)
	nuevo, err := codificarJSON(m)
	if err != nil {
		return "", err
	}
	return sinCambio(contenido, nuevo)
}
