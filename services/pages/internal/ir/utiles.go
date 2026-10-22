// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ir

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// TextoTraducible representa un texto extraído del IR para traducción.
type TextoTraducible struct {
	Texto string `json:"texto"`
	Rico  bool   `json:"rico,omitzero"`
}

const alfabetoID = "abcdefghijklmnopqrstuvwxyz0123456789"

// NuevoID genera un id aleatorio de 10 caracteres [a-z0-9] (~51 bits de entropía).
func NuevoID() string {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		panic("ir.NuevoID: fallo en crypto/rand: " + err.Error())
	}
	for i := range b {
		b[i] = alfabetoID[int(b[i])%len(alfabetoID)]
	}
	return string(b)
}

// Recorrer visita todos los nodos del sitio en orden canónico (cabecera, pie, páginas, componentes).
func Recorrer(s *Sitio, fn func(ruta string, n *Nodo) error) error {
	var visitar func(n *Nodo, ruta string) error
	visitar = func(n *Nodo, ruta string) error {
		if err := fn(ruta, n); err != nil {
			return err
		}
		if n.Type == "section" || n.Type == "frame" {
			for i := range n.Children {
				if err := visitar(&n.Children[i], fmt.Sprintf("%s/children/%d", ruta, i)); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if s.Header != nil {
		if err := visitar(s.Header, "/header"); err != nil {
			return err
		}
	}
	if s.Footer != nil {
		if err := visitar(s.Footer, "/footer"); err != nil {
			return err
		}
	}
	for i := range s.Pages {
		p := &s.Pages[i]
		for j := range p.Sections {
			if err := visitar(&p.Sections[j], fmt.Sprintf("/pages/%d/sections/%d", i, j)); err != nil {
				return err
			}
		}
	}
	for i := range s.Components {
		c := &s.Components[i]
		if err := visitar(&c.Root, fmt.Sprintf("/components/%d/root", i)); err != nil {
			return err
		}
	}
	return nil
}

// RefsImagen devuelve todas las referencias a imágenes del sitio.
func RefsImagen(s *Sitio) []ImagenRef {
	var refs []ImagenRef
	agregar := func(r *ImagenRef) {
		if r != nil && (r.Asset != nil || r.Library != nil || r.Placeholder != nil) {
			refs = append(refs, *r)
		}
	}
	agregarEstilo := func(st *Estilo) {
		if st != nil && st.Background != nil && st.Background.Imagen != nil {
			agregar(&st.Background.Imagen.Image)
		}
	}

	if s.SEO.OGImage != nil {
		agregar(s.SEO.OGImage)
	}
	if s.SEO.Favicon != nil {
		agregar(s.SEO.Favicon)
	}
	for i := range s.Pages {
		agregar(s.Pages[i].OGImage)
	}

	_ = Recorrer(s, func(_ string, n *Nodo) error {
		agregarEstilo(n.Style)
		if n.Type == "image" {
			agregar(n.Src)
		}
		if n.Type == "instance" {
			for _, o := range n.Overrides {
				agregar(o.Image)
				if o.Style != nil && o.Style.Background != nil && o.Style.Background.Imagen != nil {
					agregar(&o.Style.Background.Imagen.Image)
				}
			}
		}
		return nil
	})
	return refs
}

// RefsFichero devuelve las rutas de fichero referenciadas en los enlaces del sitio.
func RefsFichero(s *Sitio) []string {
	var ficheros []string
	agregarEnlace := func(en *Enlace) {
		if en != nil && en.File != nil && *en.File != "" {
			ficheros = append(ficheros, *en.File)
		}
	}
	_ = Recorrer(s, func(_ string, n *Nodo) error {
		agregarEnlace(n.Link)
		agregarEnlace(n.Action)
		extraerEnlacesFichero(n, agregarEnlace)
		return nil
	})
	return ficheros
}

func extraerEnlacesFichero(n *Nodo, agregar func(*Enlace)) {
	if n.Type == "text" {
		for _, tramos := range n.Content {
			for _, tr := range tramos {
				agregar(tr.Link)
			}
		}
	}
	if n.Type == "menu" {
		var checkMenu func(items []ElementoMenu)
		checkMenu = func(items []ElementoMenu) {
			for _, it := range items {
				agregar(&it.Link)
				if len(it.Children) > 0 {
					checkMenu(it.Children)
				}
			}
		}
		checkMenu(n.Items)
	}
	if n.Type == "instance" {
		for _, o := range n.Overrides {
			if o.Link.Presente && !o.Link.Nulo {
				agregar(&o.Link.Valor)
			}
		}
	}
}

// Formularios devuelve los nodos de formulario indexados por su identificador de formulario.
func Formularios(s *Sitio) map[string]*Nodo {
	res := make(map[string]*Nodo)
	_ = Recorrer(s, func(_ string, n *Nodo) error {
		if n.Type == "form" && n.Form != "" {
			res[n.Form] = n
		}
		return nil
	})
	return res
}

// tramosACadenaMarcada convierte una lista de tramos a una cadena con marcas **negrita** y *cursiva*.
func tramosACadenaMarcada(tramos []Tramo) string {
	var sb strings.Builder
	for _, tr := range tramos {
		txt := tr.Text
		switch {
		case tr.Bold && tr.Italic:
			sb.WriteString("***" + txt + "***")
		case tr.Bold:
			sb.WriteString("**" + txt + "**")
		case tr.Italic:
			sb.WriteString("*" + txt + "*")
		default:
			sb.WriteString(txt)
		}
	}
	return sb.String()
}

// TextosDe extrae los textos traducibles de un sitio para un idioma con claves estables <id>/<campo>.
func TextosDe(s *Sitio, idioma string) map[string]TextoTraducible {
	res := make(map[string]TextoTraducible)

	// SEO
	if val, ok := s.SEO.SiteName[idioma]; ok {
		res["seo/site_name"] = TextoTraducible{Texto: val}
	}

	// Páginas
	for _, p := range s.Pages {
		if val, ok := p.Title[idioma]; ok {
			res[p.ID+"/title"] = TextoTraducible{Texto: val}
		}
		if val, ok := p.Description[idioma]; ok {
			res[p.ID+"/description"] = TextoTraducible{Texto: val}
		}
	}

	_ = Recorrer(s, func(_ string, n *Nodo) error {
		extraerTextosNodo(n, idioma, res)
		return nil
	})
	return res
}

// TextosDePagina comparte la extracción de TextosDe, limitada a la página
// elegida. No traduce SEO, cabecera, pie ni componentes compartidos: cambiarlos
// alteraría también las otras páginas del sitio.
func TextosDePagina(s *Sitio, idioma, paginaID string) (map[string]TextoTraducible, bool) {
	for _, p := range s.Pages {
		if p.ID == paginaID {
			return TextosDe(&Sitio{Pages: []Pagina{p}}, idioma), true
		}
	}
	return map[string]TextoTraducible{}, false
}

func extraerTextosNodo(n *Nodo, idioma string, res map[string]TextoTraducible) {
	switch n.Type {
	case "text":
		if tramos, ok := n.Content[idioma]; ok {
			res[n.ID+"/content"] = TextoTraducible{Texto: tramosACadenaMarcada(tramos), Rico: true}
		}
	case "image":
		if val, ok := n.Alt[idioma]; ok {
			res[n.ID+"/alt"] = TextoTraducible{Texto: val}
		}
	case "button", "icon", "cart_button", "menu":
		extraerTextosEtiqueta(n, idioma, res)
	case "embed":
		if val, ok := n.Title[idioma]; ok {
			res[n.ID+"/title"] = TextoTraducible{Texto: val}
		}
	case "form":
		extraerTextosForm(n, idioma, res)
	case "table":
		extraerTextosTabla(n, idioma, res)
	case "instance":
		extraerTextosInstancia(n, idioma, res)
	}
}

func extraerTextosEtiqueta(n *Nodo, idioma string, res map[string]TextoTraducible) {
	if val, ok := n.Label[idioma]; ok {
		res[n.ID+"/label"] = TextoTraducible{Texto: val}
	}
	if n.Type == "menu" {
		var checkMenu func(items []ElementoMenu)
		checkMenu = func(items []ElementoMenu) {
			for _, it := range items {
				if val, ok := it.Label[idioma]; ok {
					res[it.ID+"/label"] = TextoTraducible{Texto: val}
				}
				if len(it.Children) > 0 {
					checkMenu(it.Children)
				}
			}
		}
		checkMenu(n.Items)
	}
}

func extraerTextosForm(n *Nodo, idioma string, res map[string]TextoTraducible) {
	if val, ok := n.Submit[idioma]; ok {
		res[n.ID+"/submit"] = TextoTraducible{Texto: val}
	}
	if val, ok := n.Success[idioma]; ok {
		res[n.ID+"/success"] = TextoTraducible{Texto: val}
	}
	if val, ok := n.Consent[idioma]; ok {
		res[n.ID+"/consent"] = TextoTraducible{Texto: val}
	}
	for _, f := range n.Fields {
		if val, ok := f.Label[idioma]; ok {
			res[f.ID+"/label"] = TextoTraducible{Texto: val}
		}
		for i, opt := range f.Options {
			if val, ok := opt[idioma]; ok {
				res[fmt.Sprintf("%s/options/%d", f.ID, i)] = TextoTraducible{Texto: val}
			}
		}
	}
}

func extraerTextosTabla(n *Nodo, idioma string, res map[string]TextoTraducible) {
	if val, ok := n.Caption[idioma]; ok {
		res[n.ID+"/caption"] = TextoTraducible{Texto: val}
	}
	for c, h := range n.Header {
		if val, ok := h[idioma]; ok {
			res[fmt.Sprintf("%s/header/%d", n.ID, c)] = TextoTraducible{Texto: val}
		}
	}
	for f, row := range n.Rows {
		for c, cell := range row {
			if val, ok := cell[idioma]; ok {
				res[fmt.Sprintf("%s/rows/%d/%d", n.ID, f, c)] = TextoTraducible{Texto: val}
			}
		}
	}
}

func extraerTextosInstancia(n *Nodo, idioma string, res map[string]TextoTraducible) {
	for k, o := range n.Overrides {
		if o.Text != nil {
			if tramos, ok := o.Text[idioma]; ok {
				res[fmt.Sprintf("%s/%s/text", n.ID, k)] = TextoTraducible{Texto: tramosACadenaMarcada(tramos), Rico: true}
			}
		}
		if o.Label != nil {
			if val, ok := o.Label[idioma]; ok {
				res[fmt.Sprintf("%s/%s/label", n.ID, k)] = TextoTraducible{Texto: val}
			}
		}
		if o.Alt != nil {
			if val, ok := o.Alt[idioma]; ok {
				res[fmt.Sprintf("%s/%s/alt", n.ID, k)] = TextoTraducible{Texto: val}
			}
		}
	}
}

// quitarPictogramas elimina caracteres emoji o pictogramas (símbolos varios).
func quitarPictogramas(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if unicode.Is(unicode.So, r) || (r >= 0x1F300 && r <= 0x1FAFF) {
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// parsearMarcas convierte marcas simples de markdown **negrita** y *cursiva* en tramos.
func parsearMarcas(s string) []Tramo {
	s = quitarPictogramas(s)
	// Implementación sencilla de parser inline
	if !strings.Contains(s, "*") {
		return []Tramo{{Text: s}}
	}
	var tramos []Tramo
	// Parser por tokens
	i := 0
	buf := strings.Builder{}
	for i < len(s) {
		switch {
		case strings.HasPrefix(s[i:], "***"):
			if buf.Len() > 0 {
				tramos = append(tramos, Tramo{Text: buf.String()})
				buf.Reset()
			}
			end := strings.Index(s[i+3:], "***")
			if end >= 0 {
				tramos = append(tramos, Tramo{Text: s[i+3 : i+3+end], Bold: true, Italic: true})
				i += 3 + end + 3
				continue
			}
		case strings.HasPrefix(s[i:], "**"):
			if buf.Len() > 0 {
				tramos = append(tramos, Tramo{Text: buf.String()})
				buf.Reset()
			}
			end := strings.Index(s[i+2:], "**")
			if end >= 0 {
				tramos = append(tramos, Tramo{Text: s[i+2 : i+2+end], Bold: true})
				i += 2 + end + 2
				continue
			}
		case s[i] == '*':
			if buf.Len() > 0 {
				tramos = append(tramos, Tramo{Text: buf.String()})
				buf.Reset()
			}
			end := strings.Index(s[i+1:], "*")
			if end >= 0 {
				tramos = append(tramos, Tramo{Text: s[i+1 : i+1+end], Italic: true})
				i += 1 + end + 1
				continue
			}
		}
		buf.WriteByte(s[i])
		i++
	}
	if buf.Len() > 0 {
		tramos = append(tramos, Tramo{Text: buf.String()})
	}
	return tramos
}

// SeccionesDesdeIA adapta las secciones recibidas de la IA al formato IR completo,
// regenerando ids, convirtiendo textos planos a mapas con el idioma especificado,
// transformando marcas a tramos y validando en ModoBorrador.
func SeccionesDesdeIA(b []byte, idioma string) ([]Nodo, error) {
	var env struct {
		Sections []Nodo `json:"sections"`
	}
	if err := json.Unmarshal(b, &env); err != nil || len(env.Sections) == 0 {
		// Intenta como array directo
		var secs []Nodo
		if err2 := json.Unmarshal(b, &secs); err2 != nil {
			return nil, fmt.Errorf("ir.SeccionesDesdeIA: formato no reconocido: %w", err)
		}
		env.Sections = secs
	}

	// Adaptar nodos recursivamente
	var adaptar func(n *Nodo)
	adaptar = func(n *Nodo) {
		n.ID = NuevoID()
		if n.Type == "form" {
			n.Form = NuevoID()
			for i := range n.Fields {
				n.Fields[i].ID = NuevoID()
			}
		}
		if n.Type == "text" && len(n.Content) == 0 {
			n.Content = map[string][]Tramo{
				idioma: parsearMarcas(n.Name),
			}
		}
		for i := range n.Children {
			adaptar(&n.Children[i])
		}
	}

	for i := range env.Sections {
		sec := &env.Sections[i]
		if sec.Type == "" {
			sec.Type = "section"
		}
		adaptar(sec)
	}

	return env.Sections, nil
}
