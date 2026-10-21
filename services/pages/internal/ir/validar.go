// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ir

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// CodigoProblema identifica la regla incumplida.
type CodigoProblema = string

// Problema representa un error o aviso en la validación del IR.
type Problema struct {
	Path     string `json:"path"`
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Detail   string `json:"detail,omitzero"`
}

// Modo define si la validación es para borrador o para publicar.
type Modo string

const (
	ModoBorrador Modo = "borrador"
	ModoPublicar Modo = "publicar"
)

// Catalogo contiene las fuentes y fotos de biblioteca disponibles para comprobación.
type Catalogo struct {
	Fuentes    []string
	Biblioteca []string
}

var avisos = map[string]bool{
	"traduccion_falta":   true,
	"enlace_roto":        true,
	"ancla_rota":         true,
	"h1_varios":          true,
	"salto_encabezado":   true,
	"relleno_sin_minimo": true,
}

var segunModo = map[string]bool{
	"texto_vacio":    true,
	"alt_falta":      true,
	"etiqueta_falta": true,
}

type registro struct {
	modo  Modo
	lista []Problema
}

func nuevoRegistro(modo Modo) *registro {
	return &registro{modo: modo}
}

func (r *registro) add(path, code string, detail ...string) {
	sev := "error"
	if avisos[code] {
		sev = "warning"
	} else if segunModo[code] && r.modo == ModoBorrador {
		sev = "warning"
	}
	p := Problema{Path: path, Code: code, Severity: sev}
	if len(detail) > 0 && detail[0] != "" {
		p.Detail = detail[0]
	}
	r.lista = append(r.lista, p)
}

func (r *registro) resultado() []Problema {
	vistos := make(map[string]bool, len(r.lista))
	var out []Problema
	for _, p := range r.lista {
		k := p.Path + "\x00" + p.Code + "\x00" + p.Severity
		if !vistos[k] {
			vistos[k] = true
			out = append(out, p)
		}
	}
	slices.SortFunc(out, func(a, b Problema) int {
		if c := strings.Compare(a.Path, b.Path); c != 0 {
			return c
		}
		if c := strings.Compare(a.Code, b.Code); c != 0 {
			return c
		}
		return strings.Compare(a.Severity, b.Severity)
	})
	return out
}

func seg(k any) string {
	s := fmt.Sprint(k)
	s = strings.ReplaceAll(s, "~", "~0")
	s = strings.ReplaceAll(s, "/", "~1")
	return "/" + s
}

// ---------------------------------------------------------------------------------------------
// Fase estructural

var reSustitutoHex = regexp.MustCompile(`(?i)\\u([0-9a-f]{4})`)

func haySustitutoSuelto(b []byte) bool {
	matches := reSustitutoHex.FindAllSubmatchIndex(b, -1)
	if len(matches) == 0 {
		return false
	}
	for i := 0; i < len(matches); i++ {
		m := matches[i]
		h, _ := strconv.ParseUint(string(b[m[2]:m[3]]), 16, 16)
		if h >= 0xD800 && h <= 0xDBFF {
			// Alto sustituto: debe seguirle un bajo sustituto inmediatamente en la siguiente posición
			if i+1 < len(matches) && matches[i+1][0] == m[1] {
				h2, _ := strconv.ParseUint(string(b[matches[i+1][2]:matches[i+1][3]]), 16, 16)
				if h2 >= 0xDC00 && h2 <= 0xDFFF {
					i++ // par válido consumido
					continue
				}
			}
			return true
		}
		if h >= 0xDC00 && h <= 0xDFFF {
			// Bajo sustituto suelto
			return true
		}
	}
	return false
}

// ValidarCrudo realiza la fase estructural sobre el JSON crudo del IR.
func ValidarCrudo(b []byte) []Problema {
	r := nuevoRegistro(ModoBorrador)
	if haySustitutoSuelto(b) {
		r.add("", "json")
		return r.resultado()
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		r.add("", "json")
		return r.resultado()
	}
	m, ok := v.(map[string]any)
	if !ok {
		r.add("", "tipo")
		return r.resultado()
	}
	sz, err := BytesIR(v)
	if err != nil || sz > Limites.IRBytes {
		r.add("", "limite")
		return r.resultado()
	}
	if verVal, ok := m["version"]; ok {
		if num, ok := verVal.(json.Number); ok {
			if n, err := num.Int64(); err == nil && int(n) != VersionIR {
				r.add("/version", "version")
				return r.resultado()
			}
		} else if n, ok := verVal.(float64); ok && n == math.Trunc(n) && int(n) != VersionIR {
			r.add("/version", "version")
			return r.resultado()
		}
	}
	validarObjeto(m, "sitio", "", "", r)
	return r.resultado()
}

var (
	conProporcion = map[string]bool{"image": true, "embed": true, "frame": true, "instance": true}
	conLayout     = map[string]bool{"section": true, "frame": true, "form": true, "instance": true}
	conLongitud   = map[string]bool{"text": true, "email": true, "tel": true, "textarea": true}
	clavesDeNodo  = func() map[string]bool {
		res := make(map[string]bool)
		for _, t := range CamposNodo {
			for k := range t {
				res[k] = true
			}
		}
		return res
	}()
)

func partir(d string) (string, string) {
	i := strings.Index(d, ":")
	if i < 0 {
		return d, ""
	}
	return d[:i], d[i+1:]
}

func comprobar(v any, desc, path, nodo string, r *registro) {
	d := desc
	d = strings.TrimSuffix(d, "!")
	if strings.HasSuffix(d, "|null") {
		if v == nil {
			return
		}
		d = strings.TrimSuffix(d, "|null")
	}
	if len(d) == 4 && (d[0] == 'i' || d[0] == 'n') && d[1] == '[' && d[3] == ']' {
		comprobarArregloFijo(v, d, path, nodo, r)
		return
	}
	cab, resto := partir(d)
	if comprobarBasico(v, cab, path, r) {
		return
	}
	comprobarComplejo(v, cab, resto, path, nodo, r)
}

func comprobarArregloFijo(v any, d, path, nodo string, r *registro) {
	n := int(d[2] - '0')
	arr, ok := v.([]any)
	if !ok || len(arr) != n {
		r.add(path, "tipo")
		return
	}
	for i, x := range arr {
		comprobar(x, string(d[0]), path+seg(i), nodo, r)
	}
}

func comprobarBasico(v any, cab, path string, r *registro) bool {
	switch cab {
	case "s":
		if _, ok := v.(string); !ok {
			r.add(path, "tipo")
		}
		return true
	case "b":
		if _, ok := v.(bool); !ok {
			r.add(path, "tipo")
		}
		return true
	case "t":
		b, ok := v.(bool)
		if !ok {
			r.add(path, "tipo")
		} else if !b {
			r.add(path, "valor")
		}
		return true
	case "i":
		if !esEntero(v) {
			r.add(path, "tipo")
		}
		return true
	case "n":
		if !esNumero(v) {
			r.add(path, "tipo")
		}
		return true
	}
	return false
}

func comprobarComplejo(v any, cab, resto, path, nodo string, r *registro) {
	switch cab {
	case "e":
		validarEnumerado(v, resto, path, r)
	case "o":
		validarObjeto(v, resto, path, nodo, r)
	case "u":
		validarUnion(v, resto, path, nodo, r)
	case "a":
		comprobarArray(v, resto, path, nodo, r)
	case "m":
		comprobarMapa(v, resto, path, nodo, r)
	case "textos":
		comprobar(v, "m:s", path, nodo, r)
	case "ricos":
		comprobar(v, "m:a:o:tramo", path, nodo, r)
	case "nodo":
		validarNodoCrudo(v, path, r)
	case "medida":
		comprobarMedida(v, path, r)
	case "radio":
		comprobarRadio(v, path, r)
	case "fondo":
		if _, ok := v.(string); !ok {
			validarObjeto(v, "fondo_imagen", path, nodo, r)
		}
	default:
		panic("descriptor desconocido: " + cab)
	}
}

func comprobarArray(v any, resto, path, nodo string, r *registro) {
	arr, ok := v.([]any)
	if !ok {
		r.add(path, "tipo")
		return
	}
	for i, x := range arr {
		comprobar(x, resto, path+seg(i), nodo, r)
	}
}

func comprobarMapa(v any, resto, path, nodo string, r *registro) {
	m, ok := v.(map[string]any)
	if !ok {
		r.add(path, "tipo")
		return
	}
	clavesOrdenadas := slices.Sorted(maps.Keys(m))
	for _, k := range clavesOrdenadas {
		comprobar(m[k], resto, path+seg(k), nodo, r)
	}
}

func comprobarMedida(v any, path string, r *registro) {
	if s, ok := v.(string); ok {
		if s != "fill" && s != "hug" {
			r.add(path, "valor")
		}
	} else if !esEntero(v) {
		r.add(path, "tipo")
	}
}

func comprobarRadio(v any, path string, r *registro) {
	if s, ok := v.(string); ok {
		if s != "$sm" && s != "$md" && s != "$lg" {
			r.add(path, "valor")
		}
	} else if !esEntero(v) {
		r.add(path, "tipo")
	}
}

func esNumero(v any) bool {
	switch val := v.(type) {
	case json.Number:
		_, err := val.Float64()
		return err == nil
	case float64:
		return true
	case int, int64:
		return true
	default:
		return false
	}
}

func esEntero(v any) bool {
	switch val := v.(type) {
	case json.Number:
		_, err := val.Int64()
		return err == nil
	case float64:
		return val == math.Trunc(val)
	case int, int64:
		return true
	default:
		return false
	}
}

func aNumero(v any) (float64, bool) {
	switch val := v.(type) {
	case json.Number:
		f, err := val.Float64()
		return f, err == nil
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	default:
		return 0, false
	}
}

func validarEnumerado(v any, nombre, path string, r *registro) {
	valores, ok := Enumerados[nombre]
	if !ok || len(valores) == 0 {
		return
	}
	if _, isInt := valores[0].(int); isInt {
		if !esEntero(v) {
			r.add(path, "tipo")
			return
		}
		num, _ := aNumero(v)
		n := int(num)
		encontrado := false
		for _, x := range valores {
			if xi, ok := x.(int); ok && xi == n {
				encontrado = true
				break
			}
		}
		if !encontrado {
			r.add(path, "valor")
		}
		return
	}
	s, ok := v.(string)
	if !ok {
		r.add(path, "tipo")
		return
	}
	encontrado := false
	for _, x := range valores {
		if xs, ok := x.(string); ok && xs == s {
			encontrado = true
			break
		}
	}
	if !encontrado {
		r.add(path, "valor")
	}
}

func validarClaves(v map[string]any, tabla map[string]string, path, nodo string, r *registro, ajena, noAplica func(string) bool) {
	clavesOrdenadas := slices.Sorted(maps.Keys(v))
	for _, k := range clavesOrdenadas {
		d, ok := tabla[k]
		if !ok || noAplica(k) {
			if ok || ajena(k) {
				r.add(path+seg(k), "campo_no_aplica")
			} else {
				r.add(path+seg(k), "campo_desconocido")
			}
		} else {
			comprobar(v[k], d, path+seg(k), nodo, r)
		}
	}
	tablaClaves := slices.Sorted(maps.Keys(tabla))
	for _, k := range tablaClaves {
		d := tabla[k]
		if strings.HasSuffix(d, "!") {
			if _, ok := v[k]; !ok {
				r.add(path+seg(k), "obligatorio")
			}
		}
	}
}

func noAplicaObjeto(objeto, k string, v map[string]any, nodo string) bool {
	switch objeto {
	case "tamano", "tamano_delta":
		return k == "aspect_ratio" && !conProporcion[nodo]
	case "delta":
		return (k == "layout" && !conLayout[nodo]) || (k == "columns" && nodo != "product_grid")
	case "campo":
		kind, ok := v["kind"].(string)
		if !ok {
			return false
		}
		esValido := false
		if enums, ok := Enumerados["tipo_campo"]; ok {
			for _, x := range enums {
				if xs, ok := x.(string); ok && xs == kind {
					esValido = true
					break
				}
			}
		}
		if !esValido {
			return false
		}
		if k == "options" {
			return kind != "select"
		}
		if k == "max_length" {
			return !conLongitud[kind]
		}
		if k == "min" || k == "max" {
			return kind != "number"
		}
		return false
	default:
		return false
	}
}

func validarObjeto(v any, nombre, path, nodo string, r *registro) {
	m, ok := v.(map[string]any)
	if !ok {
		r.add(path, "tipo")
		return
	}
	tabla := CamposObjeto[nombre]
	validarClaves(m, tabla, path, nodo, r, func(string) bool { return false }, func(k string) bool {
		return noAplicaObjeto(nombre, k, m, nodo)
	})
	if nombre == "campo" && m["kind"] == "select" {
		if _, ok := m["options"]; !ok {
			r.add(path+"/options", "obligatorio")
		}
	}
	if nodo == "section" && (nombre == "tamano" || nombre == "tamano_delta") {
		if w, ok := m["width"]; ok {
			if esEntero(w) || w == "hug" {
				r.add(path+"/width", "valor")
			}
		}
		if h, ok := m["height"]; ok && h == "fill" {
			r.add(path+"/height", "valor")
		}
	}
}

func validarUnion(v any, nombre, path, nodo string, r *registro) {
	m, ok := v.(map[string]any)
	if !ok {
		r.add(path, "tipo")
		return
	}
	discs := Uniones[nombre]
	var presentes []string
	for _, d := range discs {
		if _, ok := m[d]; ok {
			presentes = append(presentes, d)
		}
	}
	variante := func(d string) map[string]string {
		return CamposObjeto[nombre+"_"+d]
	}
	var quedan []string
	for _, d := range presentes {
		esParteDeOtra := false
		for _, o := range presentes {
			if o != d {
				if _, ok := variante(o)[d]; ok {
					esParteDeOtra = true
					break
				}
			}
		}
		if !esParteDeOtra {
			quedan = append(quedan, d)
		}
	}
	if len(quedan) != 1 {
		r.add(path, "valor")
		return
	}
	elegida := quedan[0]
	todas := make(map[string]bool)
	for _, d := range discs {
		for k := range variante(d) {
			todas[k] = true
		}
	}
	validarClaves(m, variante(elegida), path, nodo, r, func(k string) bool { return todas[k] }, func(string) bool { return false })
}

func validarNodoCrudo(v any, path string, r *registro) {
	m, ok := v.(map[string]any)
	if !ok {
		r.add(path, "tipo")
		return
	}
	tVal, ok := m["type"]
	if !ok {
		r.add(path+"/type", "obligatorio")
		return
	}
	t, ok := tVal.(string)
	if !ok {
		r.add(path+"/type", "tipo")
		return
	}
	tabla, ok := CamposNodo[t]
	if !ok {
		r.add(path+"/type", "valor")
		return
	}
	validarClaves(m, tabla, path, t, r, func(k string) bool { return clavesDeNodo[k] }, func(string) bool { return false })
}

// ---------------------------------------------------------------------------------------------
// Fase semántica

var (
	reID         = regexp.MustCompile(`^[a-z0-9]{10}$`)
	reSlug       = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	reAncla      = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
	reClaveColor = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	reColor      = regexp.MustCompile(`^#[0-9a-f]{6}([0-9a-f]{2})?$`)
	reHora       = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d`)
	rePais       = regexp.MustCompile(`^[A-Z]{2}$`)
	reUUID       = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	reYouTube    = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	reVimeo      = regexp.MustCompile(`^\d{6,12}$`)
	reOSM        = regexp.MustCompile(`^(-?[0-9]{1,2}(\.[0-9]{1,7})?),(-?[0-9]{1,3}(\.[0-9]{1,7})?),([0-9]{1,2})$`)
	reHTTPS      = regexp.MustCompile(`^https://[^/?#]+`)
	reMailto     = regexp.MustCompile(`^mailto:.`)
	reTel        = regexp.MustCompile(`^tel:.`)
	reH          = regexp.MustCompile(`^h[1-6]$`)
	interactivos = map[string]bool{
		"button": true, "form": true, "menu": true, "embed": true,
		"product_grid": true, "cart_button": true, "language_switcher": true,
	}
	conEtiqueta = map[string]bool{"button": true, "icon": true, "cart_button": true, "menu": true}
	conEnlace   = map[string]bool{"frame": true, "image": true, "button": true}
	setIconos   = func() map[string]bool {
		res := make(map[string]bool)
		for _, ic := range Iconos {
			res[ic] = true
		}
		return res
	}()
	setSlugsReservados = func() map[string]bool {
		res := make(map[string]bool)
		for _, s := range SlugsReservados {
			res[s] = true
		}
		return res
	}()
	setAnclasReservadas = func() map[string]bool {
		res := make(map[string]bool)
		for _, a := range AnclasReservadas {
			res[a] = true
		}
		return res
	}()
	setIdiomasVisitante = func() map[string]bool {
		res := make(map[string]bool)
		for _, l := range IdiomasVisitante {
			res[l] = true
		}
		return res
	}()
	setIdiomas = func() map[string]bool {
		res := make(map[string]bool)
		for _, l := range Idiomas {
			res[l] = true
		}
		return res
	}()
)

func tieneControl(s string) bool {
	for _, r := range s {
		if (r < 0x20 && r != '\n') || (r >= 0x7f && r <= 0x9f) || r == 0x2028 || r == 0x2029 {
			return true
		}
	}
	return false
}

// urlValida comprueba si una URL es válida según el estándar del IR.
func urlValida(u string) bool {
	if len(u) > Limites.URL {
		return false
	}
	if tieneControl(u) || strings.Contains(u, " ") {
		return false
	}
	return reHTTPS.MatchString(u) || reMailto.MatchString(u) || reTel.MatchString(u)
}

func refValida(provider, ref string) bool {
	switch provider {
	case "osm":
		m := reOSM.FindStringSubmatch(ref)
		if len(m) == 0 {
			return false
		}
		lat, err1 := strconv.ParseFloat(m[1], 64)
		lon, err2 := strconv.ParseFloat(m[3], 64)
		zoom, err3 := strconv.Atoi(m[5])
		if err1 != nil || err2 != nil || err3 != nil {
			return false
		}
		return math.Abs(lat) <= 90 && math.Abs(lon) <= 180 && zoom >= 1 && zoom <= 19
	case "cast":
		return reUUID.MatchString(ref)
	case "youtube":
		return reYouTube.MatchString(ref)
	case "vimeo":
		return reVimeo.MatchString(ref)
	default:
		return false
	}
}

type claseTexto string

const (
	claseObligatorio claseTexto = "obligatorio"
	claseOpcional    claseTexto = "opcional"
	claseAlt         claseTexto = "alt"
	claseEtiqueta    claseTexto = "etiqueta"
	claseSlug        claseTexto = "slug"
)

func vacio(s string, ok bool) bool {
	if !ok {
		return true
	}
	for _, r := range s {
		if r != ' ' && r != '\n' && r != '\t' && r != '\r' {
			return false
		}
	}
	return true
}

func textoDeRicos(t TextosRicos, l string) (string, bool) {
	tramos, ok := t[l]
	if !ok {
		return "", false
	}
	var sb strings.Builder
	for _, tr := range tramos {
		sb.WriteString(tr.Text)
	}
	return sb.String(), true
}

type donde struct {
	pagina         int
	enMaestro      bool
	dentroDeEnlace bool
	profundidad    int
}

type semantica struct {
	s              *Sitio
	cat            *Catalogo
	r              *registro
	idiomas        []string
	principal      string
	componentes    map[string]*Componente
	paginas        map[string]int
	anclas         []map[string]bool
	anclasGlobales map[string]bool
}

// Validar ejecuta la fase semántica del IR sobre un *Sitio.
func Validar(s *Sitio, cat *Catalogo, modo Modo) []Problema {
	r := nuevoRegistro(modo)
	sem := &semantica{
		s:              s,
		cat:            cat,
		r:              r,
		principal:      s.Languages.Default,
		componentes:    make(map[string]*Componente),
		paginas:        make(map[string]int),
		anclasGlobales: make(map[string]bool),
	}
	sem.idiomas = append([]string{s.Languages.Default}, s.Languages.Others...)
	sem.idiomas = slices.Compact(sem.idiomas)

	for i := range s.Components {
		c := &s.Components[i]
		if _, ok := sem.componentes[c.ID]; !ok {
			sem.componentes[c.ID] = c
		}
	}
	for i := range s.Pages {
		p := &s.Pages[i]
		if _, ok := sem.paginas[p.ID]; !ok {
			sem.paginas[p.ID] = i
		}
	}
	for _, g := range []*Nodo{s.Header, s.Footer} {
		if g != nil && g.Anchor != "" {
			sem.anclasGlobales[g.Anchor] = true
		}
	}
	for i := range s.Pages {
		p := &s.Pages[i]
		a := make(map[string]bool)
		for k := range sem.anclasGlobales {
			a[k] = true
		}
		for _, sec := range p.Sections {
			if sec.Type == "section" && sec.Anchor != "" {
				a[sec.Anchor] = true
			}
		}
		sem.anclas = append(sem.anclas, a)
	}

	sem.todo()
	return r.resultado()
}

func (s *semantica) todo() {
	s.ids()
	s.lenguas()
	s.tema()
	s.global(s.s.Header, "/header", "header")
	s.global(s.s.Footer, "/footer", "footer")
	s.anclasDeCabeceraYPie()
	if len(s.s.Pages) == 0 {
		s.r.add("/pages", "portada")
	}
	if len(s.s.Pages) > Limites.Paginas {
		s.r.add("/pages"+seg(Limites.Paginas), "limite")
	}
	slugs := make(map[string]map[string]bool)
	for i := range s.s.Pages {
		s.pagina(&s.s.Pages[i], i, slugs)
	}
	if len(s.s.Components) > Limites.Componentes {
		s.r.add("/components"+seg(Limites.Componentes), "limite")
	}
	for i := range s.s.Components {
		s.componente(&s.s.Components[i], i)
	}
	s.seo()
}

func (s *semantica) ids() {
	vistos := make(map[string]bool)
	checkID := func(v, path string) {
		switch {
		case !reID.MatchString(v):
			s.r.add(path, "id_formato")
		case vistos[v]:
			s.r.add(path, "id_duplicado")
		default:
			vistos[v] = true
		}
	}
	var checkMenu func(items []ElementoMenu, path string)
	checkMenu = func(items []ElementoMenu, path string) {
		for i, it := range items {
			checkID(it.ID, path+seg(i)+"/id")
			if len(it.Children) > 0 {
				checkMenu(it.Children, path+seg(i)+"/children")
			}
		}
	}
	var checkNodo func(n *Nodo, path string)
	checkNodo = func(n *Nodo, path string) {
		checkID(n.ID, path+"/id")
		if n.Type == "form" {
			checkID(n.Form, path+"/form")
			for i, f := range n.Fields {
				checkID(f.ID, path+"/fields"+seg(i)+"/id")
			}
		}
		if n.Type == "menu" {
			checkMenu(n.Items, path+"/items")
		}
		if n.Type == "section" || n.Type == "frame" {
			for i := range n.Children {
				checkNodo(&n.Children[i], path+"/children"+seg(i))
			}
		}
	}
	if s.s.Header != nil {
		checkNodo(s.s.Header, "/header")
	}
	if s.s.Footer != nil {
		checkNodo(s.s.Footer, "/footer")
	}
	for i := range s.s.Pages {
		p := &s.s.Pages[i]
		checkID(p.ID, fmt.Sprintf("/pages%s/id", seg(i)))
		for j := range p.Sections {
			checkNodo(&p.Sections[j], fmt.Sprintf("/pages%s/sections%s", seg(i), seg(j)))
		}
	}
	for i := range s.s.Components {
		c := &s.s.Components[i]
		checkID(c.ID, fmt.Sprintf("/components%s/id", seg(i)))
		checkNodo(&c.Root, fmt.Sprintf("/components%s/root", seg(i)))
	}
}

func (s *semantica) lengua(l, path string) {
	if setIdiomasVisitante[l] {
		return
	}
	if setIdiomas[l] {
		s.r.add(path, "idioma", "no_disponible")
	} else {
		s.r.add(path, "idioma")
	}
}

func (s *semantica) lenguas() {
	d := s.s.Languages.Default
	others := s.s.Languages.Others
	s.lengua(d, "/languages/default")
	vistos := map[string]bool{d: true}
	for i, l := range others {
		path := "/languages/others" + seg(i)
		if vistos[l] {
			s.r.add(path, "idioma")
		} else {
			s.lengua(l, path)
		}
		vistos[l] = true
	}
}

func (s *semantica) control(val, path string) {
	if tieneControl(val) {
		s.r.add(path, "texto_control")
	}
}

func (s *semantica) textos(t Textos, path string, clase claseTexto) {
	if t == nil {
		if clase != claseOpcional && clase != claseSlug {
			s.vacio(path, clase)
		}
		return
	}
	idiomasSitio := make(map[string]bool)
	for _, l := range s.idiomas {
		idiomasSitio[l] = true
	}
	claves := slices.Sorted(maps.Keys(t))
	for _, l := range claves {
		v := t[l]
		if !idiomasSitio[l] {
			s.r.add(path+seg(l), "idioma")
		}
		s.control(v, path+seg(l))
		if utf8.RuneCountInString(v) > Limites.CaracteresPorTexto {
			s.r.add(path+seg(l), "limite")
		}
	}
	if clase != claseSlug {
		s.traducciones(func(l string) (string, bool) {
			val, ok := t[l]
			return val, ok
		}, path, clase)
	}
}

func (s *semantica) ricos(t TextosRicos, path string, clase claseTexto, d donde) {
	if t == nil {
		if clase != claseOpcional {
			s.vacio(path, clase)
		}
		return
	}
	idiomasSitio := make(map[string]bool)
	for _, l := range s.idiomas {
		idiomasSitio[l] = true
	}
	claves := slices.Sorted(maps.Keys(t))
	for _, l := range claves {
		tramos := t[l]
		pl := path + seg(l)
		if !idiomasSitio[l] {
			s.r.add(pl, "idioma")
		}
		n := 0
		for j, tr := range tramos {
			s.control(tr.Text, pl+seg(j)+"/text")
			n += utf8.RuneCountInString(tr.Text)
			if tr.Link != nil {
				s.enlace(tr.Link, pl+seg(j)+"/link", d)
			}
		}
		if n > Limites.CaracteresPorTexto {
			s.r.add(pl, "limite")
		}
	}
	s.traducciones(func(l string) (string, bool) {
		return textoDeRicos(t, l)
	}, path, clase)
}

func (s *semantica) vacio(path string, clase claseTexto) {
	p := path + seg(s.principal)
	switch clase {
	case claseObligatorio:
		s.r.add(p, "texto_vacio")
	case claseAlt:
		s.r.add(p, "alt_falta")
	case claseEtiqueta:
		s.r.add(p, "etiqueta_falta")
	case claseOpcional, claseSlug:
	}
}

func (s *semantica) traducciones(de func(l string) (string, bool), path string, clase claseTexto) {
	val, ok := de(s.principal)
	if vacio(val, ok) {
		s.vacio(path, clase)
		return
	}
	for _, l := range s.idiomas {
		if l != s.principal {
			v, o := de(l)
			if vacio(v, o) {
				s.r.add(path+seg(l), "traduccion_falta")
			}
		}
	}
}

func (s *semantica) tema() {
	t := s.s.Theme
	if len(t.Colors) > Limites.ColoresTema {
		s.r.add("/theme/colors", "limite")
	}
	claves := slices.Sorted(maps.Keys(t.Colors))
	for _, k := range claves {
		v := t.Colors[k]
		path := "/theme/colors" + seg(k)
		if !reClaveColor.MatchString(k) {
			s.r.add(path, "token")
		}
		if !reColor.MatchString(v) {
			s.r.add(path, "color")
		}
	}
	for _, k := range ColoresObligatorios {
		if _, ok := t.Colors[k]; !ok {
			s.r.add("/theme/colors"+seg(k), "token")
		}
	}
	for _, k := range []string{"heading", "body"} {
		f := t.Fonts.Heading
		if k == "body" {
			f = t.Fonts.Body
		}
		s.control(f, "/theme/fonts/"+k)
		if s.cat != nil && !slices.Contains(s.cat.Fuentes, f) {
			s.r.add("/theme/fonts/"+k, "fuente")
		}
	}
	checkEstilo := func(st EstiloTipografico, path string) {
		for i, n := range st.Size {
			s.rango(float64(n), 8, 160, path+"/size"+seg(i))
		}
		s.rango(st.LineHeight, 0.8, 3, path+"/line_height")
		s.rango(st.LetterSpacing, -0.1, 0.5, path+"/letter_spacing")
	}
	checkEstilo(t.TextStyles.H1, "/theme/text_styles/h1")
	checkEstilo(t.TextStyles.H2, "/theme/text_styles/h2")
	checkEstilo(t.TextStyles.H3, "/theme/text_styles/h3")
	checkEstilo(t.TextStyles.H4, "/theme/text_styles/h4")
	checkEstilo(t.TextStyles.H5, "/theme/text_styles/h5")
	checkEstilo(t.TextStyles.H6, "/theme/text_styles/h6")
	checkEstilo(t.TextStyles.Body, "/theme/text_styles/body")
	checkEstilo(t.TextStyles.Small, "/theme/text_styles/small")
	checkEstilo(t.TextStyles.Label, "/theme/text_styles/label")
	checkEstilo(t.TextStyles.Button, "/theme/text_styles/button")

	s.rango(float64(t.Radius.Sm), 0, 999, "/theme/radius/sm")
	s.rango(float64(t.Radius.Md), 0, 999, "/theme/radius/md")
	s.rango(float64(t.Radius.Lg), 0, 999, "/theme/radius/lg")
	s.rango(float64(t.Container), 640, 1600, "/theme/container")
}

func (s *semantica) rango(n, inf, sup float64, path string) {
	if n < inf || n > sup {
		s.r.add(path, "rango")
	}
}

func (s *semantica) rangoOpc(n *int, inf, sup int, path string) {
	if n != nil && (*n < inf || *n > sup) {
		s.r.add(path, "rango")
	}
}

func (s *semantica) rangoFloatOpc(n *float64, inf, sup float64, path string) {
	if n != nil && (*n < inf || *n > sup) {
		s.r.add(path, "rango")
	}
}

func (s *semantica) color(v, path string) {
	if v == "" {
		return
	}
	if strings.HasPrefix(v, "$") {
		tok := strings.TrimPrefix(v, "$")
		if _, ok := s.s.Theme.Colors[tok]; !ok {
			s.r.add(path, "token")
		}
	} else if !reColor.MatchString(v) {
		s.r.add(path, "color")
	}
}

func (s *semantica) imagen(r *ImagenRef, path string) {
	if r != nil && r.Library != nil && s.cat != nil && !slices.Contains(s.cat.Biblioteca, *r.Library) {
		s.r.add(path+"/library", "biblioteca")
	}
}

func (s *semantica) tamano(t *Tamano, path string) {
	if t == nil {
		return
	}
	if t.Width.Clave == "" {
		s.rango(float64(t.Width.Px), 1, 4000, path+"/width")
	}
	if t.Height.Clave == "" {
		s.rango(float64(t.Height.Px), 1, 4000, path+"/height")
	}
	s.rangoOpc(t.MinWidth, 0, 4000, path+"/min_width")
	s.rangoOpc(t.MaxWidth, 0, 4000, path+"/max_width")
	s.rangoOpc(t.MinHeight, 0, 4000, path+"/min_height")
	s.rangoOpc(t.MaxHeight, 0, 4000, path+"/max_height")
	s.rangoFloatOpc(t.AspectRatio, 0.1, 10, path+"/aspect_ratio")
}

func (s *semantica) tamanoDelta(t *TamanoDelta, path string) {
	if t == nil {
		return
	}
	if t.Width != nil && t.Width.Clave == "" {
		s.rango(float64(t.Width.Px), 1, 4000, path+"/width")
	}
	if t.Height != nil && t.Height.Clave == "" {
		s.rango(float64(t.Height.Px), 1, 4000, path+"/height")
	}
	if t.MinWidth.Presente && !t.MinWidth.Nulo {
		s.rango(float64(t.MinWidth.Valor), 0, 4000, path+"/min_width")
	}
	if t.MaxWidth.Presente && !t.MaxWidth.Nulo {
		s.rango(float64(t.MaxWidth.Valor), 0, 4000, path+"/max_width")
	}
	if t.MinHeight.Presente && !t.MinHeight.Nulo {
		s.rango(float64(t.MinHeight.Valor), 0, 4000, path+"/min_height")
	}
	if t.MaxHeight.Presente && !t.MaxHeight.Nulo {
		s.rango(float64(t.MaxHeight.Valor), 0, 4000, path+"/max_height")
	}
	if t.AspectRatio.Presente && !t.AspectRatio.Nulo {
		s.rango(t.AspectRatio.Valor, 0.1, 10, path+"/aspect_ratio")
	}
}

func (s *semantica) layout(l *Layout, path string) {
	if l == nil {
		return
	}
	s.rango(float64(l.Gap), 0, 400, path+"/gap")
	s.rangoOpc(l.RowGap, 0, 400, path+"/row_gap")
	for i, n := range l.Padding {
		s.rango(float64(n), 0, 400, path+"/padding"+seg(i))
	}
	s.rangoOpc(l.Columns, 1, 12, path+"/columns")
}

func (s *semantica) layoutDelta(l *LayoutDelta, path string) {
	if l == nil {
		return
	}
	s.rangoOpc(l.Gap, 0, 400, path+"/gap")
	s.rangoOpc(l.RowGap, 0, 400, path+"/row_gap")
	if l.Padding != nil {
		for i, n := range *l.Padding {
			s.rango(float64(n), 0, 400, path+"/padding"+seg(i))
		}
	}
	s.rangoOpc(l.Columns, 1, 12, path+"/columns")
}

func (s *semantica) estilo(st *Estilo, path string) {
	if st == nil {
		return
	}
	if st.Background != nil {
		if st.Background.Imagen != nil {
			s.imagen(&st.Background.Imagen.Image, path+"/background/image")
			s.color(st.Background.Imagen.Overlay, path+"/background/overlay")
		} else {
			s.color(st.Background.Color, path+"/background")
		}
	}
	s.color(st.Color, path+"/color")
	if st.Border != nil {
		s.rango(float64(st.Border.Width), 0, 20, path+"/border/width")
		s.color(st.Border.Color, path+"/border/color")
	}
	if st.Radius != nil && st.Radius.Token == "" {
		s.rango(float64(st.Radius.Px), 0, 999, path+"/radius")
	}
	s.rangoFloatOpc(st.Opacity, 0, 1, path+"/opacity")
	s.estiloTexto(st.Text, path+"/text")
}

func (s *semantica) estiloDelta(st *EstiloDelta, path string) {
	if st == nil {
		return
	}
	if st.Background.Presente && !st.Background.Nulo {
		bg := st.Background.Valor
		if bg.Imagen != nil {
			s.imagen(&bg.Imagen.Image, path+"/background/image")
			s.color(bg.Imagen.Overlay, path+"/background/overlay")
		} else {
			s.color(bg.Color, path+"/background")
		}
	}
	s.color(st.Color, path+"/color")
	if st.Border.Presente && !st.Border.Nulo {
		s.rango(float64(st.Border.Valor.Width), 0, 20, path+"/border/width")
		s.color(st.Border.Valor.Color, path+"/border/color")
	}
	if st.Radius != nil && st.Radius.Token == "" {
		s.rango(float64(st.Radius.Px), 0, 999, path+"/radius")
	}
	s.rangoFloatOpc(st.Opacity, 0, 1, path+"/opacity")
	s.estiloTexto(st.Text, path+"/text")
}

func (s *semantica) estiloTexto(t *EstiloTexto, path string) {
	if t == nil {
		return
	}
	s.rangoOpc(t.Size, 8, 160, path+"/size")
	s.rangoFloatOpc(t.LineHeight, 0.8, 3, path+"/line_height")
	s.rangoFloatOpc(t.LetterSpacing, -0.1, 0.5, path+"/letter_spacing")
}

func (s *semantica) enlace(en *Enlace, path string, d donde) {
	if en == nil {
		return
	}
	switch {
	case en.Page != nil:
		i, ok := s.paginas[*en.Page]
		if !ok {
			s.r.add(path+"/page", "enlace_roto")
		} else if en.Anchor != nil && i < len(s.anclas) && !s.anclas[i][*en.Anchor] {
			s.r.add(path+"/anchor", "ancla_rota")
		}
	case en.Anchor != nil:
		ok := false
		if d.pagina >= 0 && d.pagina < len(s.anclas) {
			ok = s.anclas[d.pagina][*en.Anchor]
		} else {
			for _, a := range s.anclas {
				if a[*en.Anchor] {
					ok = true
					break
				}
			}
			if !ok && s.anclasGlobales[*en.Anchor] {
				ok = true
			}
		}
		if !ok {
			s.r.add(path+"/anchor", "ancla_rota")
		}
	case en.URL != nil && !urlValida(*en.URL):
		s.r.add(path+"/url", "url")
	}
}

func (s *semantica) global(n *Nodo, path, kind string) {
	if n == nil {
		return
	}
	d := donde{pagina: -1, enMaestro: false, dentroDeEnlace: false, profundidad: 1}
	if n.Type != "section" {
		s.r.add(path, "anidamiento")
		s.nodo(n, path, d, nil)
		return
	}
	if n.Kind != kind {
		s.r.add(path+"/kind", "anidamiento")
	}
	s.nodo(n, path, d, nil)
}

func (s *semantica) anclasDeCabeceraYPie() {
	vistas := make(map[string]bool)
	for _, pair := range []struct {
		n    *Nodo
		path string
	}{
		{s.s.Header, "/header"},
		{s.s.Footer, "/footer"},
	} {
		if pair.n != nil && pair.n.Type == "section" {
			s.ancla(pair.n, pair.path, vistas)
		}
	}
}

func (s *semantica) ancla(n *Nodo, path string, vistas map[string]bool) {
	if n.Anchor == "" {
		return
	}
	p := path + "/anchor"
	switch {
	case !reAncla.MatchString(n.Anchor):
		s.r.add(p, "formato")
	case setAnclasReservadas[n.Anchor]:
		s.r.add(p, "ancla_reservada")
	case vistas[n.Anchor]:
		s.r.add(p, "ancla_duplicada")
	}
	vistas[n.Anchor] = true
}

func (s *semantica) pagina(p *Pagina, i int, slugs map[string]map[string]bool) {
	path := "/pages" + seg(i)
	s.textos(p.Slug, path+"/slug", claseSlug)
	for _, l := range s.idiomas {
		v, ok := p.Slug[l]
		sp := path + "/slug" + seg(l)
		switch {
		case !ok:
			s.r.add(sp, "slug_falta")
		case i == 0 || v == "":
			if (i == 0) != (v == "") {
				s.r.add(sp, "portada")
			}
		case !reSlug.MatchString(v) || len(v) > Limites.Slug:
			s.r.add(sp, "slug_formato")
		case setSlugsReservados[v]:
			s.r.add(sp, "slug_reservado")
		default:
			vistos, okVistos := slugs[l]
			if !okVistos {
				vistos = make(map[string]bool)
				slugs[l] = vistos
			}
			if vistos[v] {
				s.r.add(sp, "slug_duplicado")
			}
			vistos[v] = true
		}
	}

	s.textos(p.Title, path+"/title", claseObligatorio)
	s.textos(p.Description, path+"/description", claseObligatorio)
	s.imagen(p.OGImage, path+"/og_image")
	if len(p.Sections) > Limites.SeccionesPorPagina {
		s.r.add(path+"/sections"+seg(Limites.SeccionesPorPagina), "limite")
	}
	anclas := make(map[string]bool)
	for k := range s.anclasGlobales {
		anclas[k] = true
	}
	for j := range p.Sections {
		sec := &p.Sections[j]
		np := path + "/sections" + seg(j)
		d := donde{pagina: i, enMaestro: false, dentroDeEnlace: false, profundidad: 1}
		if sec.Type != "section" {
			s.r.add(np, "anidamiento")
		} else {
			s.ancla(sec, np, anclas)
		}
		s.nodo(sec, np, d, nil)
	}
	s.encabezados(p, path)
	if s.nodosDePagina(p) > Limites.NodosPorPagina {
		s.r.add(path, "limite")
	}
}

func (s *semantica) encabezados(p *Pagina, path string) {
	type enc struct {
		nivel int
		path  string
	}
	var lista []enc
	var visitar func(n *Nodo, np string)
	visitar = func(n *Nodo, np string) {
		if n.Type == "text" && reH.MatchString(n.Tag) {
			lvl, _ := strconv.Atoi(n.Tag[1:])
			lista = append(lista, enc{nivel: lvl, path: np})
		}
		if n.Type == "section" || n.Type == "frame" {
			for i := range n.Children {
				visitar(&n.Children[i], np+"/children"+seg(i))
			}
		}
	}
	if p.ShowHeader && s.s.Header != nil {
		visitar(s.s.Header, "/header")
	}
	for j := range p.Sections {
		visitar(&p.Sections[j], path+"/sections"+seg(j))
	}
	if p.ShowFooter && s.s.Footer != nil {
		visitar(s.s.Footer, "/footer")
	}
	h1 := false
	previo := 1
	for _, e := range lista {
		if e.nivel == 1 && h1 {
			s.r.add(e.path, "h1_varios")
		}
		if e.nivel == 1 {
			h1 = true
		}
		if e.nivel > previo+1 {
			s.r.add(e.path, "salto_encabezado")
		}
		previo = e.nivel
	}
}

func (s *semantica) nodosDePagina(p *Pagina) int {
	usados := make(map[string]bool)
	var contar func(n *Nodo) int
	contar = func(n *Nodo) int {
		if n.Type == "instance" {
			usados[n.Component] = true
		}
		tot := 1
		if n.Type == "section" || n.Type == "frame" {
			for i := range n.Children {
				tot += contar(&n.Children[i])
			}
		}
		return tot
	}
	total := 0
	for i := range p.Sections {
		total += contar(&p.Sections[i])
	}
	if p.ShowHeader && s.s.Header != nil {
		total += contar(s.s.Header)
	}
	if p.ShowFooter && s.s.Footer != nil {
		total += contar(s.s.Footer)
	}
	for c := range usados {
		if m, ok := s.componentes[c]; ok {
			total += contarNodos(&m.Root)
		}
	}
	return total
}

func (s *semantica) componente(c *Componente, i int) {
	path := "/components" + seg(i)
	s.control(c.Name, path+"/name")
	if c.Root.Type != "frame" {
		s.r.add(path+"/root", "anidamiento")
	}
	if contarNodos(&c.Root) > Limites.NodosPorComponente {
		s.r.add(path, "limite")
	}
	d := donde{pagina: -1, enMaestro: true, dentroDeEnlace: false, profundidad: 1}
	s.nodo(&c.Root, path+"/root", d, nil)
}

func (s *semantica) seo() {
	seo := s.s.SEO
	s.textos(seo.SiteName, "/seo/site_name", claseObligatorio)
	s.imagen(seo.OGImage, "/seo/og_image")
	s.imagen(seo.Favicon, "/seo/favicon")
	b := seo.Business
	if b == nil {
		return
	}
	if b.Phone != "" {
		s.control(b.Phone, "/seo/business/phone")
	}
	if b.Email != "" {
		s.control(b.Email, "/seo/business/email")
	}
	if b.Address != nil {
		if b.Address.Street != "" {
			s.control(b.Address.Street, "/seo/business/address/street")
		}
		if b.Address.City != "" {
			s.control(b.Address.City, "/seo/business/address/city")
		}
		if b.Address.PostalCode != "" {
			s.control(b.Address.PostalCode, "/seo/business/address/postal_code")
		}
		if b.Address.Region != "" {
			s.control(b.Address.Region, "/seo/business/address/region")
		}
		if !rePais.MatchString(b.Address.Country) {
			s.r.add("/seo/business/address/country", "formato")
		}
	}
	for i, h := range b.OpeningHours {
		path := "/seo/business/opening_hours" + seg(i)
		if !reHora.MatchString(h.Opens) {
			s.r.add(path+"/opens", "formato")
		}
		if !reHora.MatchString(h.Closes) {
			s.r.add(path+"/closes", "formato")
		}
	}
}

func (s *semantica) maestro(n *Nodo) *Nodo {
	if n.Type == "instance" {
		if c, ok := s.componentes[n.Component]; ok {
			return &c.Root
		}
	}
	return nil
}

func (s *semantica) esLi(n *Nodo) bool {
	r := n
	if n.Type == "instance" {
		r = s.maestro(n)
	}
	return r != nil && r.Type == "frame" && r.Tag == "li"
}

func (s *semantica) interactivo(n *Nodo) bool {
	switch n.Type {
	case "image", "frame":
		return n.Link != nil
	case "text":
		for _, tramos := range n.Content {
			for _, tr := range tramos {
				if tr.Link != nil {
					return true
				}
			}
		}
		return false
	case "instance":
		for _, o := range n.Overrides {
			if o.Link.Presente && !o.Link.Nulo {
				return true
			}
		}
		m := s.maestro(n)
		return m != nil && algunoInteractivo(m, func(x *Nodo) bool {
			return x.Type != "instance" && s.interactivo(x)
		})
	default:
		return interactivos[n.Type]
	}
}

func (s *semantica) nodo(n *Nodo, path string, d donde, padre *Nodo) {
	if d.profundidad > Limites.Profundidad {
		s.r.add(path, "limite")
		return
	}
	s.nodoComun(n, path, d)
	s.nodoBreakpoints(n.Breakpoints, path)
	s.nodoEspecifico(n, path, d, padre)
}

func (s *semantica) nodoComun(n *Nodo, path string, d donde) {
	if n.Name != "" && n.Type != "icon" {
		s.control(n.Name, path+"/name")
		if utf8.RuneCountInString(n.Name) > 60 {
			s.r.add(path+"/name", "limite")
		}
	}
	if d.dentroDeEnlace && s.interactivo(n) {
		s.r.add(path, "anidamiento")
	}
	s.tamano(n.Size, path+"/size")
	s.estilo(n.Style, path+"/style")
}

func (s *semantica) nodoBreakpoints(bp *Puntos, path string) {
	if bp == nil {
		return
	}
	for _, item := range []struct {
		name  string
		delta *Delta
	}{
		{"tablet", bp.Tablet},
		{"mobile", bp.Mobile},
	} {
		if item.delta == nil {
			continue
		}
		dp := path + "/breakpoints/" + item.name
		s.tamanoDelta(item.delta.Size, dp+"/size")
		s.layoutDelta(item.delta.Layout, dp+"/layout")
		s.estiloDelta(item.delta.Style, dp+"/style")
		s.rangoOpc(item.delta.Columns, 1, 6, dp+"/columns")
	}
}

func (s *semantica) nodoEspecifico(n *Nodo, path string, d donde, padre *Nodo) {
	switch n.Type {
	case "section", "frame":
		s.contenedor(n, path, d)
	case "text":
		s.ricos(n.Content, path+"/content", claseObligatorio, d)
	case "image":
		s.nodoImagen(n, path, d)
	case "button":
		s.nodoBoton(n, path, d)
	case "icon":
		s.nodoIcono(n, path, d, padre)
	case "embed":
		s.nodoEmbed(n, path)
	case "form":
		s.formulario(n, path, d)
	case "table":
		s.nodoTabla(n, path)
	case "product_grid":
		s.nodoProductGrid(n, path)
	case "cart_button":
		s.textos(n.Label, path+"/label", claseEtiqueta)
	case "language_switcher":
	case "menu":
		s.nodoMenu(n, path, d)
	case "instance":
		s.instancia(n, path, d)
	}
}

func (s *semantica) nodoImagen(n *Nodo, path string, d donde) {
	s.imagen(n.Src, path+"/src")
	cl := claseAlt
	if n.Decorative {
		cl = claseOpcional
	}
	s.textos(n.Alt, path+"/alt", cl)
	if n.Link != nil {
		s.enlace(n.Link, path+"/link", d)
	}
}

func (s *semantica) nodoBoton(n *Nodo, path string, d donde) {
	s.textos(n.Label, path+"/label", claseEtiqueta)
	s.enlace(n.Action, path+"/action", d)
	if n.Icon != "" && !setIconos[n.Icon] {
		s.r.add(path+"/icon", "icono")
	}
}

func (s *semantica) nodoIcono(n *Nodo, path string, d donde, padre *Nodo) {
	if !setIconos[n.Name] {
		s.r.add(path+"/name", "icono")
	}
	s.textos(n.Label, path+"/label", claseOpcional)
	if !d.dentroDeEnlace {
		return
	}
	lbl, ok := n.Label[s.principal]
	if !vacio(lbl, ok) {
		return
	}
	hermanoTexto := false
	if padre != nil && len(padre.Children) > 0 {
		for _, c := range padre.Children {
			if c.Type == "text" {
				hermanoTexto = true
				break
			}
		}
	}
	if !hermanoTexto {
		s.r.add(path+"/label"+seg(s.principal), "etiqueta_falta")
	}
}

func (s *semantica) nodoEmbed(n *Nodo, path string) {
	s.textos(n.Title, path+"/title", claseObligatorio)
	if !refValida(n.Provider, n.Ref) {
		s.r.add(path+"/ref", "incrustado_ref")
	}
}

func (s *semantica) nodoTabla(n *Nodo, path string) {
	s.textos(n.Caption, path+"/caption", claseObligatorio)
	for i, t := range n.Header {
		s.textos(t, path+"/header"+seg(i), claseOpcional)
	}
	if len(n.Header) > Limites.ColumnasTabla {
		s.r.add(path+"/header"+seg(Limites.ColumnasTabla), "limite")
	}
	if len(n.Rows) > Limites.FilasTabla {
		s.r.add(path+"/rows"+seg(Limites.FilasTabla), "limite")
	}
	for f, fila := range n.Rows {
		fp := path + "/rows" + seg(f)
		if len(fila) > Limites.ColumnasTabla {
			s.r.add(fp+seg(Limites.ColumnasTabla), "limite")
		}
		for c, t := range fila {
			s.textos(t, fp+seg(c), claseOpcional)
		}
	}
}

func (s *semantica) nodoProductGrid(n *Nodo, path string) {
	s.rango(float64(n.Limit), 1, float64(Limites.ProductosEnRejilla), path+"/limit")
	s.rango(float64(n.Columns), 1, 6, path+"/columns")
	if n.Source != nil {
		if n.Source.Category != nil {
			s.control(*n.Source.Category, path+"/source/category")
		}
		if len(n.Source.Products) > Limites.ProductosEnRejilla {
			s.r.add(path+"/source/products"+seg(Limites.ProductosEnRejilla), "limite")
		}
	}
}

func (s *semantica) nodoMenu(n *Nodo, path string, d donde) {
	s.textos(n.Label, path+"/label", claseOpcional)
	if len(n.Items) > Limites.ElementosMenu {
		s.r.add(path+"/items"+seg(Limites.ElementosMenu), "limite")
	}
	s.elementos(n.Items, path+"/items", d)
}

func (s *semantica) contenedor(n *Nodo, path string, d donde) {
	s.layout(n.Layout, path+"/layout")
	dentro := d.dentroDeEnlace
	if n.Type == "frame" && n.Link != nil {
		if n.Tag != "div" {
			s.r.add(path+"/link", "anidamiento")
		}
		s.enlace(n.Link, path+"/link", d)
		dentro = true
	}
	lista := n.Type == "frame" && (n.Tag == "ul" || n.Tag == "ol")
	flujo := n.Layout != nil && n.Layout.Direction == "row" && n.Layout.Wrap && n.Layout.Columns == nil
	for i := range n.Children {
		c := &n.Children[i]
		cp := path + "/children" + seg(i)
		if c.Type == "section" {
			s.r.add(cp, "anidamiento")
		}
		if lista != s.esLi(c) {
			s.r.add(cp, "anidamiento")
		}
		if flujo && c.Size != nil && c.Size.Width.Clave == "fill" && c.Size.MinWidth == nil {
			s.r.add(cp+"/size/width", "relleno_sin_minimo")
		}
		s.nodo(c, cp, donde{
			pagina:         d.pagina,
			enMaestro:      d.enMaestro,
			dentroDeEnlace: dentro,
			profundidad:    d.profundidad + 1,
		}, n)
	}
}

func (s *semantica) formulario(n *Nodo, path string, d donde) {
	if d.enMaestro {
		s.r.add(path, "anidamiento")
	}
	s.layout(n.Layout, path+"/layout")
	if len(n.Fields) > Limites.CamposFormulario {
		s.r.add(path+"/fields"+seg(Limites.CamposFormulario), "limite")
	}
	for i := range n.Fields {
		s.campo(&n.Fields[i], path+"/fields"+seg(i))
	}
	s.textos(n.Submit, path+"/submit", claseObligatorio)
	s.textos(n.Success, path+"/success", claseObligatorio)
	s.textos(n.Consent, path+"/consent", claseObligatorio)
}

func (s *semantica) campo(f *CampoFormulario, path string) {
	s.textos(f.Label, path+"/label", claseObligatorio)
	for i, o := range f.Options {
		s.textos(o, path+"/options"+seg(i), claseObligatorio)
	}
	s.rangoOpc(f.MaxLength, 1, 5000, path+"/max_length")
	if f.Min != nil && f.Max != nil && *f.Min > *f.Max {
		s.r.add(path+"/max", "rango")
	}
}

func (s *semantica) elementos(items []ElementoMenu, path string, d donde) {
	for i, it := range items {
		ip := path + seg(i)
		s.textos(it.Label, ip+"/label", claseObligatorio)
		s.enlace(&it.Link, ip+"/link", d)
		if len(it.Children) > 0 {
			if len(it.Children) > Limites.HijosPorElementoMenu {
				s.r.add(ip+"/children"+seg(Limites.HijosPorElementoMenu), "limite")
			}
			s.elementos(it.Children, ip+"/children", d)
		}
	}
}

func (s *semantica) instancia(n *Nodo, path string, d donde) {
	if d.enMaestro {
		s.r.add(path, "componente_anidado")
	}
	c, ok := s.componentes[n.Component]
	if !ok {
		s.r.add(path+"/component", "componente_falta")
	}
	nodos := make(map[string]*Nodo)
	if c != nil {
		recorrer(&c.Root, func(x *Nodo) {
			if _, exists := nodos[x.ID]; !exists {
				nodos[x.ID] = x
			}
		})
	}
	claves := slices.Sorted(maps.Keys(n.Overrides))
	for _, k := range claves {
		o := n.Overrides[k]
		op := path + "/overrides" + seg(k)
		destino, existe := nodos[k]
		if c != nil && !existe {
			s.r.add(op, "override_invalido")
		} else if existe {
			s.override(&o, destino, op)
		}
		s.contenidoOverride(&o, op, d)
	}
}

func (s *semantica) override(o *Override, destino *Nodo, path string) {
	t := destino.Type
	casa := map[string]bool{
		"text":   t == "text",
		"label":  conEtiqueta[t],
		"image":  t == "image",
		"alt":    t == "image",
		"link":   conEnlace[t] && (t != "frame" || destino.Tag == "div"),
		"icon":   t == "icon" || t == "button",
		"hidden": true,
		"style":  true,
	}
	if o.Text != nil && !casa["text"] {
		s.r.add(path+"/text", "override_invalido")
	}
	if o.Label != nil && !casa["label"] {
		s.r.add(path+"/label", "override_invalido")
	}
	if o.Image != nil && !casa["image"] {
		s.r.add(path+"/image", "override_invalido")
	}
	if o.Alt != nil && !casa["alt"] {
		s.r.add(path+"/alt", "override_invalido")
	}
	if o.Link.Presente && !casa["link"] {
		s.r.add(path+"/link", "override_invalido")
	}
	if o.Icon != "" && !casa["icon"] {
		s.r.add(path+"/icon", "override_invalido")
	}
}

func (s *semantica) contenidoOverride(o *Override, path string, d donde) {
	s.ricos(o.Text, path+"/text", claseOpcional, d)
	s.textos(o.Label, path+"/label", claseOpcional)
	s.textos(o.Alt, path+"/alt", claseOpcional)
	s.imagen(o.Image, path+"/image")
	if o.Link.Presente && !o.Link.Nulo {
		s.enlace(&o.Link.Valor, path+"/link", d)
	}
	if o.Icon != "" && !setIconos[o.Icon] {
		s.r.add(path+"/icon", "icono")
	}
	s.estilo(o.Style, path+"/style")
}

func recorrer(n *Nodo, fn func(n *Nodo)) {
	fn(n)
	if n.Type == "section" || n.Type == "frame" {
		for i := range n.Children {
			recorrer(&n.Children[i], fn)
		}
	}
}

func contarNodos(n *Nodo) int {
	tot := 0
	recorrer(n, func(*Nodo) { tot++ })
	return tot
}

func algunoInteractivo(n *Nodo, es func(n *Nodo) bool) bool {
	si := false
	recorrer(n, func(x *Nodo) {
		if !si && es(x) {
			si = true
		}
	})
	return si
}
