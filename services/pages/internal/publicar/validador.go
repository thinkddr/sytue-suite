// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package publicar

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

var (
	ErrPublicacionInvalida = errors.New("publicar: salida de publicación inválida")
	ErrDemasiadosArchivos  = errors.New("publicar: demasiados archivos en la publicación")
	ErrTamanoExcedido      = errors.New("publicar: tamaño de publicación excedido")
)

// OpcionesValidacion son los parámetros de contexto para la validación de la salida.
type OpcionesValidacion struct {
	Kind            string          // "live" | "preview"
	PreviewBase     string          // Prefijo /_vista/<token> autorizado para esta subida
	CastOrigin      string          // URL origen de Cast ($PAGES_CAST_ORIGIN)
	HostsPermitidos []string        // Hosts válidos para sitemap.xml
	Idiomas         []string        // Idiomas admitidos para {L}
	RecursosValidos map[string]bool // Rutas /_astro/... que se sabe existen en el almacén
}

// FicheroSalida es cada fichero a validar en la publicación.
type FicheroSalida struct {
	Ruta      string
	Contenido []byte
}

var regexPreviewBase = regexp.MustCompile(`^/_vista/[0-9a-f]{32}$`)

var regexIslasScript = regexp.MustCompile(`^/_astro/islas-([0-9a-f]{16})\.js$`)

// ValidarSalida ejecuta todas las comprobaciones de seguridad y conformidad contra permitido.json.
func ValidarSalida(p *Permitido, ficheros []FicheroSalida, opts OpcionesValidacion) error {
	if opts.PreviewBase != "" && (opts.Kind != "preview" || !regexPreviewBase.MatchString(opts.PreviewBase)) {
		return fmt.Errorf("%w: prefijo de preview inválido", ErrPublicacionInvalida)
	}
	if err := verificarLimites(p, ficheros); err != nil {
		return err
	}

	regexRutas, err := compilarRegexRutas(p.Rutas, opts.Idiomas)
	if err != nil {
		return err
	}

	recursosEnPaquete := construirMapaRecursos(ficheros, opts.RecursosValidos)
	iframeOrigenes := resolverIframeOrigenes(p.HTML.IframeOrigenes, opts.CastOrigin)

	for _, f := range ficheros {
		if err := validarFichero(p, f, regexRutas, opts, recursosEnPaquete, iframeOrigenes); err != nil {
			return err
		}
	}

	return nil
}

func verificarLimites(p *Permitido, ficheros []FicheroSalida) error {
	if len(ficheros) > p.Limites.Ficheros {
		return fmt.Errorf("%w: %d ficheros (máximo %d)", ErrDemasiadosArchivos, len(ficheros), p.Limites.Ficheros)
	}

	var bytesTotal int64
	for _, f := range ficheros {
		bytesTotal += int64(len(f.Contenido))
	}
	if bytesTotal > p.Limites.BytesTotal {
		return fmt.Errorf("%w: %d bytes (máximo %d)", ErrTamanoExcedido, bytesTotal, p.Limites.BytesTotal)
	}
	return nil
}

func construirMapaRecursos(ficheros []FicheroSalida, validos map[string]bool) map[string]bool {
	res := make(map[string]bool, len(ficheros)+len(validos))
	for _, f := range ficheros {
		if strings.HasPrefix(f.Ruta, "_astro/") {
			res["/"+f.Ruta] = true
		}
	}
	for k, v := range validos {
		if v {
			res[k] = true
		}
	}
	return res
}

func resolverIframeOrigenes(declarados []string, castOrigin string) []string {
	origenes := make([]string, 0, len(declarados))
	for _, orig := range declarados {
		if orig == "$PAGES_CAST_ORIGIN" {
			if castOrigin != "" {
				origenes = append(origenes, castOrigin)
			}
		} else {
			origenes = append(origenes, orig)
		}
	}
	return origenes
}

func validarFichero(p *Permitido, f FicheroSalida, regexRutas []*regexp.Regexp, opts OpcionesValidacion, recursos map[string]bool, iframeOrigenes []string) error {
	if err := validarRutaPermitida(f.Ruta, regexRutas); err != nil {
		return err
	}

	if strings.HasSuffix(f.Ruta, ".js") {
		return fmt.Errorf("%w: ningún fichero .js puede subirse (%s)", ErrPublicacionInvalida, f.Ruta)
	}

	switch {
	case strings.HasSuffix(f.Ruta, ".html"):
		if int64(len(f.Contenido)) > p.Limites.BytesHTML {
			return fmt.Errorf("%w: html supera límite de 2 MiB (%s: %d bytes)", ErrTamanoExcedido, f.Ruta, len(f.Contenido))
		}
		if err := validarHTML(p, contenidoParaValidar(f.Contenido, opts.PreviewBase), opts.Kind, iframeOrigenes, recursos); err != nil {
			return fmt.Errorf("%w en %s: %w", ErrPublicacionInvalida, f.Ruta, err)
		}
	case strings.HasSuffix(f.Ruta, ".css"):
		if err := validarCSS(p, contenidoParaValidar(f.Contenido, opts.PreviewBase), recursos); err != nil {
			return fmt.Errorf("%w en %s: %w", ErrPublicacionInvalida, f.Ruta, err)
		}
	case f.Ruta == "sitemap.xml":
		if err := validarSitemap(p, f.Contenido, opts.HostsPermitidos); err != nil {
			return fmt.Errorf("%w en sitemap.xml: %w", ErrPublicacionInvalida, err)
		}
	case f.Ruta == "robots.txt":
		if err := validarRobots(p, f.Contenido); err != nil {
			return fmt.Errorf("%w en robots.txt: %w", ErrPublicacionInvalida, err)
		}
	}

	return nil
}

// El renderer comparte recursos con live, pero la preview los pide bajo su
// token. Validar una copia sin ese único prefijo conserva la allowlist y la
// comprobación de existencia, sin cambiar los bytes/hashes que se guardan.
func contenidoParaValidar(contenido []byte, previewBase string) []byte {
	if previewBase == "" {
		return contenido
	}
	return bytes.ReplaceAll(contenido, []byte(previewBase+"/_astro/"), []byte("/_astro/"))
}

func compilarRegexRutas(patrones, idiomas []string) ([]*regexp.Regexp, error) {
	idiomasPat := "([a-z]{2}(-[A-Z]{2})?)"
	if len(idiomas) > 0 {
		escapados := make([]string, len(idiomas))
		for i, id := range idiomas {
			escapados[i] = regexp.QuoteMeta(id)
		}
		idiomasPat = "(" + strings.Join(escapados, "|") + ")"
	}

	regexes := make([]*regexp.Regexp, len(patrones))
	for i, pat := range patrones {
		sustituido := strings.ReplaceAll(pat, "{L}", idiomasPat)
		re, err := regexp.Compile(sustituido)
		if err != nil {
			return nil, fmt.Errorf("publicar: compilar ruta %q: %w", pat, err)
		}
		regexes[i] = re
	}
	return regexes, nil
}

func validarRutaPermitida(ruta string, regexes []*regexp.Regexp) error {
	for _, re := range regexes {
		if re.MatchString(ruta) {
			return nil
		}
	}
	return fmt.Errorf("%w: ruta no permitida %q", ErrPublicacionInvalida, ruta)
}

func validarHTML(p *Permitido, contenido []byte, kind string, iframeOrigenes []string, recursos map[string]bool) error {
	z := html.NewTokenizer(bytes.NewReader(contenido))
	enSVG := 0
	enBody := false
	primerHijoBodyVisto := false
	var primerElementoBody html.Token

	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if errors.Is(z.Err(), io.EOF) {
				break
			}
			return fmt.Errorf("error parseando HTML: %w", z.Err())
		}

		tok := z.Token()
		tag := strings.ToLower(tok.Data)

		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			if tag == "body" {
				enBody = true
				continue
			}
			if enBody && !primerHijoBodyVisto {
				primerHijoBodyVisto = true
				primerElementoBody = tok
			}
			if tag == "svg" {
				enSVG++
			}
			if err := validarEtiquetaHTML(p, tok, tag, enSVG > 0, kind, iframeOrigenes, recursos, z); err != nil {
				return err
			}
		case html.EndTagToken:
			if tag == "svg" && enSVG > 0 {
				enSVG--
			}
			if tag == "body" {
				enBody = false
			}
		default:
			// TextToken, CommentToken, DoctypeToken
		}
	}

	if kind == "preview" {
		return verificarFranjaPreview(p, primerHijoBodyVisto, primerElementoBody)
	}
	return nil
}

func validarEtiquetaHTML(p *Permitido, tok html.Token, tag string, enSVG bool, kind string, iframeOrigenes []string, recursos map[string]bool, z *html.Tokenizer) error {
	if kind == "preview" && slices.Contains(p.VistaPrevia.SinElementos, tag) {
		return fmt.Errorf("el elemento <%s> está prohibido en vista previa", tag)
	}

	if enSVG {
		if _, ok := p.SVG.Elementos[tag]; !ok {
			return fmt.Errorf("elemento SVG <%s> no permitido", tag)
		}
		if err := validarAtributosSVG(p, tok, tag); err != nil {
			return err
		}
	} else {
		if _, ok := p.HTML.Elementos[tag]; !ok {
			return fmt.Errorf("elemento HTML <%s> no permitido", tag)
		}
		if err := validarAtributosHTML(p, tok, tag, iframeOrigenes, recursos); err != nil {
			return err
		}
	}

	if tag == "script" {
		return validarScript(tok, z)
	}
	return nil
}

func verificarFranjaPreview(p *Permitido, bodyVisto bool, primerTok html.Token) error {
	if !bodyVisto {
		return errors.New("vista previa sin contenido en body")
	}
	franja := p.VistaPrevia.Franja
	if !strings.EqualFold(primerTok.Data, franja.Elemento) {
		return fmt.Errorf("vista previa: el primer elemento de <body> debe ser <%s>, encontrado <%s>", franja.Elemento, primerTok.Data)
	}
	var tieneClase, tieneRole bool
	for _, a := range primerTok.Attr {
		k := strings.ToLower(a.Key)
		if k == "class" && slices.Contains(strings.Fields(a.Val), franja.Clase) {
			tieneClase = true
		}
		if k == "role" && a.Val == franja.Role {
			tieneRole = true
		}
	}
	if !tieneClase || !tieneRole {
		return fmt.Errorf("vista previa: la franja <%s> debe tener class=%q y role=%q", franja.Elemento, franja.Clase, franja.Role)
	}
	return nil
}

func validarAtributosSVG(p *Permitido, tok html.Token, tag string) error {
	permitidosTag := p.SVG.Elementos[tag]
	for _, a := range tok.Attr {
		key := strings.ToLower(a.Key)
		if strings.HasPrefix(key, "on") || key == "style" || key == "href" || key == "xlink:href" {
			return fmt.Errorf("atributo SVG prohibido: %s", key)
		}
		if !slices.Contains(permitidosTag, key) && !slices.Contains(p.SVG.AtributosComunes, key) {
			return fmt.Errorf("atributo SVG <%s %s> no permitido", tag, key)
		}
	}
	return nil
}

func validarAtributosHTML(p *Permitido, tok html.Token, tag string, iframeOrigenes []string, recursos map[string]bool) error {
	permitidosTag := p.HTML.Elementos[tag]
	for _, a := range tok.Attr {
		key := strings.ToLower(a.Key)
		val := a.Val

		if strings.HasPrefix(key, "on") || key == "style" || key == "srcdoc" || key == "formaction" {
			return fmt.Errorf("atributo prohibido: %s", key)
		}
		if tag == "meta" && key == "http-equiv" {
			return errors.New("<meta http-equiv> prohibido")
		}

		if !esAtributoHTMLValido(p, key, permitidosTag) {
			return fmt.Errorf("atributo HTML <%s %s> no permitido", tag, key)
		}

		reglaValor := tag + "." + key
		if permitidos, ok := p.HTML.Valores[reglaValor]; ok && !slices.Contains(permitidos, val) {
			return fmt.Errorf("valor no permitido para <%s %s=%q>", tag, key, val)
		}

		if slices.Contains(p.HTML.AtributosURL, key) {
			if err := validarURL(p, tag, key, val, iframeOrigenes); err != nil {
				return err
			}
			if err := verificarExistenciaRecurso(val, recursos); err != nil {
				return err
			}
		}

		if slices.Contains(p.HTML.AtributosSrcset, key) {
			if err := validarSrcset(val); err != nil {
				return err
			}
		}
	}
	return nil
}

func esAtributoHTMLValido(p *Permitido, key string, permitidosTag []string) bool {
	if slices.Contains(p.HTML.AtributosGlobales, key) || slices.Contains(permitidosTag, key) {
		return true
	}
	for _, pref := range p.HTML.PrefijosGlobales {
		if strings.HasPrefix(key, pref) {
			return true
		}
	}
	return false
}

func verificarExistenciaRecurso(val string, recursos map[string]bool) error {
	if !strings.HasPrefix(val, "/_astro/") || recursos == nil {
		return nil
	}
	baseRecurso, _, _ := strings.Cut(val, "?")
	baseRecurso, _, _ = strings.Cut(baseRecurso, "#")
	if !recursos[baseRecurso] && !esIslaEmbebida(baseRecurso) {
		return fmt.Errorf("recurso referenciado %q no existe", val)
	}
	return nil
}

func esIslaEmbebida(ruta string) bool {
	m := regexIslasScript.FindStringSubmatch(ruta)
	if len(m) == 2 {
		_, ok := BuscarIsla(m[1])
		return ok
	}
	return false
}

func validarURL(p *Permitido, tag, attr, val string, iframeOrigenes []string) error {
	valTrim := strings.TrimSpace(val)
	if valTrim == "" {
		return nil
	}

	if tag == "iframe" && attr == "src" {
		for _, orig := range iframeOrigenes {
			if strings.HasPrefix(valTrim, orig+"/") || valTrim == orig {
				return nil
			}
		}
		return fmt.Errorf("iframe src %q fuera de orígenes permitidos (%v)", valTrim, iframeOrigenes)
	}

	if tag == "form" && attr == "action" {
		if !strings.HasPrefix(valTrim, "/_pages/") {
			return fmt.Errorf("form action %q debe empezar por /_pages/", valTrim)
		}
		return nil
	}

	if strings.HasPrefix(valTrim, "/") {
		if strings.HasPrefix(valTrim, "//") || strings.HasPrefix(valTrim, "/\\") {
			return fmt.Errorf("URL relativa inválida con // o /\\: %q", valTrim)
		}
		return nil
	}
	if strings.HasPrefix(valTrim, "#") {
		return nil
	}

	u, err := url.Parse(valTrim)
	if err != nil {
		return fmt.Errorf("URL mal formada %q: %w", valTrim, err)
	}
	esquema := strings.ToLower(u.Scheme) + ":"
	if !slices.Contains(p.HTML.Esquemas, esquema) {
		return fmt.Errorf("esquema de URL %q no permitido para %s: %q", esquema, attr, valTrim)
	}
	return nil
}

func validarSrcset(val string) error {
	candidatos := strings.Split(val, ",")
	for _, cand := range candidatos {
		partes := strings.Fields(strings.TrimSpace(cand))
		if len(partes) == 0 {
			continue
		}
		urlCand := partes[0]
		if strings.HasPrefix(urlCand, "/") {
			if strings.HasPrefix(urlCand, "//") || strings.HasPrefix(urlCand, "/\\") {
				return fmt.Errorf("srcset URL relativa inválida: %q", urlCand)
			}
		} else {
			u, err := url.Parse(urlCand)
			if err != nil || u.Scheme != "https" {
				return fmt.Errorf("srcset URL no válida: %q", urlCand)
			}
		}
	}
	return nil
}

func validarScript(tok html.Token, z *html.Tokenizer) error {
	var scriptType, src string
	var tieneDefer bool

	for _, a := range tok.Attr {
		switch strings.ToLower(a.Key) {
		case "type":
			scriptType = a.Val
		case "src":
			src = a.Val
		case "defer":
			tieneDefer = true
		}
	}

	if scriptType == "module" && src != "" {
		return validarScriptModulo(src)
	}

	if (tieneDefer || scriptType == "") && src == "/_sytue/s.js" {
		return nil
	}

	if scriptType == "application/ld+json" && src == "" {
		return validarScriptJSONLD(z)
	}

	return fmt.Errorf("etiqueta <script> no autorizada: type=%q src=%q defer=%v", scriptType, src, tieneDefer)
}

func validarScriptModulo(src string) error {
	m := regexIslasScript.FindStringSubmatch(src)
	if len(m) != 2 {
		return fmt.Errorf("script module con src no permitido: %q", src)
	}
	hash16 := m[1]
	if _, ok := BuscarIsla(hash16); !ok {
		return fmt.Errorf("script de islas con hash desconocido: %q", hash16)
	}
	return nil
}

func validarScriptJSONLD(z *html.Tokenizer) error {
	var contenidoTexto strings.Builder
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return errors.New("script JSON-LD sin cerrar")
		}
		if tt == html.EndTagToken && strings.EqualFold(z.Token().Data, "script") {
			break
		}
		if tt == html.TextToken {
			contenidoTexto.WriteString(z.Token().Data)
		}
	}

	cuerpo := contenidoTexto.String()
	if strings.Contains(cuerpo, "</") {
		return errors.New("JSON-LD contiene secuencia prohibida </")
	}
	if !json.Valid([]byte(cuerpo)) {
		return errors.New("JSON-LD no es JSON válido")
	}
	return nil
}

func validarCSS(p *Permitido, contenido []byte, recursos map[string]bool) error {
	cssMin := strings.ToLower(string(contenido))

	for _, prohibido := range p.CSS.Prohibido {
		if strings.Contains(cssMin, prohibido) {
			return fmt.Errorf("CSS contiene regla prohibida %q", prohibido)
		}
	}

	if err := verificarReglasAtCSS(p, cssMin); err != nil {
		return err
	}

	return verificarURLsCSS(p, cssMin, recursos)
}

func verificarReglasAtCSS(p *Permitido, css string) error {
	reAt := regexp.MustCompile(`@([a-zA-Z0-9-]+)`)
	matchesAt := reAt.FindAllStringSubmatch(css, -1)
	for _, m := range matchesAt {
		if len(m) != 2 {
			continue
		}
		nombre := strings.ToLower(m[1])
		if !slices.Contains(p.CSS.ReglasAt, nombre) {
			return fmt.Errorf("regla CSS @%s no permitida", nombre)
		}
	}
	return nil
}

func verificarURLsCSS(p *Permitido, css string, recursos map[string]bool) error {
	reURL := regexp.MustCompile(`url\s*\(\s*['"]?([^'")]+)['"]?\s*\)`)
	matchesURL := reURL.FindAllStringSubmatch(css, -1)
	for _, m := range matchesURL {
		if len(m) != 2 {
			continue
		}
		target := strings.TrimSpace(m[1])
		if strings.HasPrefix(target, "data:") {
			if len(target) > p.CSS.DataURLMaxBytes {
				return fmt.Errorf("data: URL en CSS supera el máximo de %d bytes", p.CSS.DataURLMaxBytes)
			}
			continue
		}
		permitido := false
		for _, pref := range p.CSS.URLPrefijos {
			if strings.HasPrefix(target, pref) {
				permitido = true
				break
			}
		}
		if !permitido {
			return fmt.Errorf("url() en CSS con destino no permitido: %q", target)
		}
		if recursos != nil && strings.HasPrefix(target, "/_astro/") {
			baseRecurso, _, _ := strings.Cut(target, "?")
			baseRecurso, _, _ = strings.Cut(baseRecurso, "#")
			if !recursos[baseRecurso] {
				return fmt.Errorf("recurso CSS referenciado %q no existe", target)
			}
		}
	}
	return nil
}

func validarSitemap(p *Permitido, contenido []byte, hostsPermitidos []string) error {
	decoder := xml.NewDecoder(bytes.NewReader(contenido))
	elementosPermitidos := p.Sitemap.Elementos

	for {
		token, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("XML sitemap mal formado: %w", err)
		}

		el, ok := token.(xml.StartElement)
		if !ok {
			continue
		}

		name := el.Name.Local
		if !slices.Contains(elementosPermitidos, name) {
			return fmt.Errorf("elemento <%s> no permitido en sitemap.xml", name)
		}

		if permitidos, ok := p.Sitemap.Atributos[name]; ok {
			for _, attr := range el.Attr {
				attrName := attr.Name.Local
				if !slices.Contains(permitidos, attrName) {
					return fmt.Errorf("atributo %q no permitido en <%s> de sitemap.xml", attrName, name)
				}
			}
		}

		if name == "loc" {
			if err := validarLocSitemap(decoder, &el, hostsPermitidos); err != nil {
				return err
			}
		}
	}
	return nil
}

func validarLocSitemap(decoder *xml.Decoder, el *xml.StartElement, hostsPermitidos []string) error {
	var locVal string
	if err := decoder.DecodeElement(&locVal, el); err != nil {
		return err
	}
	u, err := url.Parse(strings.TrimSpace(locVal))
	if err != nil || u.Scheme != "https" {
		return fmt.Errorf("URL inválida en sitemap <loc>: %q", locVal)
	}
	if len(hostsPermitidos) == 0 {
		return nil
	}
	for _, h := range hostsPermitidos {
		if strings.EqualFold(u.Host, h) {
			return nil
		}
	}
	return fmt.Errorf("host %q en sitemap <loc> no pertenece al sitio (%v)", u.Host, hostsPermitidos)
}

func validarRobots(p *Permitido, contenido []byte) error {
	lineas := strings.Split(string(contenido), "\n")
	directivasPermitidas := p.Robots

	for i, linea := range lineas {
		l := strings.TrimSpace(linea)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		clave, _, _ := strings.Cut(l, ":")
		clave = strings.TrimSpace(clave)
		valida := false
		for _, permitida := range directivasPermitidas {
			if strings.EqualFold(clave, permitida) {
				valida = true
				break
			}
		}
		if !valida {
			return fmt.Errorf("línea %d: directiva no permitida en robots.txt: %q", i+1, clave)
		}
	}
	return nil
}
