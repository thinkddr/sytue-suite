// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/xml"
	"strings"
)

// Estilos de un .odp que el editor entiende: el relleno y el trazo de las formas, las
// puntas de los conectores, el fondo y la transición de cada diapositiva y, en el estilo
// «standard», la tinta y la fuente del tema. Un estilo de ODF hereda de otro
// (`style:parent-style-name`), y el padre suele estar en `styles.xml` mientras el hijo
// está en `content.xml`: de ahí que se resuelva contra las dos partes.

// odpStyle es lo que se lee de un estilo, sea gráfico, de página de dibujo o de texto.
// Los campos vacíos son «no lo dice»: lo hereda del padre.
type odpStyle struct {
	parent             string
	fillType, fill     string
	strokeType, stroke string
	markStart, markEnd string
	bgObjects          string // «false» = el patrón no se ve en esa diapositiva
	trType, trSub      string // smil:type y smil:subtype
	trSytue            string // sytue:transition: lo que ODF no sabe decir (morph)
	trStyle            string // presentation:transition-style, el de LibreOffice anterior a smil
	color, font        string
}

type odpStyles map[string]odpStyle

// readOdpStyles lee todos los `style:style` de una parte (los comunes y los
// automáticos), por nombre.
func readOdpStyles(part []byte) odpStyles {
	out := odpStyles{}
	dec := xml.NewDecoder(bytes.NewReader(part))
	cur := ""
	for {
		tok, err := dec.Token()
		if err != nil {
			return out
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "style" {
				cur = attrLocal(t, "name")
				out[cur] = odpStyle{parent: attrLocal(t, "parent-style-name")}
				continue
			}
			s, ok := out[cur]
			if !ok || cur == "" {
				continue
			}
			switch t.Name.Local {
			case "graphic-properties":
				s.fillType, s.fill = attrLocal(t, "fill"), attrLocal(t, "fill-color")
				s.strokeType, s.stroke = attrLocal(t, "stroke"), attrLocal(t, "stroke-color")
				s.markStart, s.markEnd = attrLocal(t, "marker-start"), attrLocal(t, "marker-end")
			case "drawing-page-properties":
				s.fillType, s.fill = attrLocal(t, "fill"), attrLocal(t, "fill-color")
				s.bgObjects = attrLocal(t, "background-objects-visible")
				s.trType, s.trSub = attrLocal(t, "type"), attrLocal(t, "subtype")
				s.trSytue, s.trStyle = attrLocal(t, "transition"), attrLocal(t, "transition-style")
			case "text-properties":
				s.color, s.font = attrLocal(t, "color"), attrLocal(t, "font-name")
			}
			out[cur] = s
		case xml.EndElement:
			if t.Name.Local == "style" {
				cur = ""
			}
		}
	}
}

// sobre devuelve el estilo `a` con lo que `b` sí dice por encima.
func (a odpStyle) sobre(b odpStyle) odpStyle {
	pick := func(x *string, y string) {
		if y != "" {
			*x = y
		}
	}
	pick(&a.fillType, b.fillType)
	pick(&a.fill, b.fill)
	pick(&a.strokeType, b.strokeType)
	pick(&a.stroke, b.stroke)
	pick(&a.markStart, b.markStart)
	pick(&a.markEnd, b.markEnd)
	pick(&a.bgObjects, b.bgObjects)
	pick(&a.trType, b.trType)
	pick(&a.trSub, b.trSub)
	pick(&a.trSytue, b.trSytue)
	pick(&a.trStyle, b.trStyle)
	pick(&a.color, b.color)
	pick(&a.font, b.font)
	return a
}

// odpLook son los estilos de las dos partes: la que se está leyendo va primero, y de la
// otra salen los padres que no estén en ella.
type odpLook struct{ primaria, otra odpStyles }

// maxHerencia acota la cadena de padres (un fichero hostil puede cerrarla en círculo).
const maxHerencia = 8

// resolver es el estilo con todo lo heredado; vacío si no existe.
func (l odpLook) resolver(name string) odpStyle {
	var cadena []odpStyle
	for i := 0; i < maxHerencia && name != ""; i++ {
		s, ok := l.primaria[name]
		if !ok {
			if s, ok = l.otra[name]; !ok {
				break
			}
		}
		cadena = append(cadena, s)
		name = s.parent
	}
	var out odpStyle
	for i := len(cadena) - 1; i >= 0; i-- {
		out = out.sobre(cadena[i])
	}
	return out
}

// relleno es el color de relleno del estilo como lo guarda el modelo: «#RRGGBB», o
// «transparent» si no se rellena; "" si el estilo no lo dice (o es un degradado o una
// trama, que el editor no tiene).
func (s odpStyle) relleno() string {
	return colorODP(s.fillType, s.fill)
}

// trazo es el color del trazo, con el mismo criterio que relleno.
func (s odpStyle) trazo() string {
	return colorODP(s.strokeType, s.stroke)
}

func colorODP(tipo, color string) string {
	if tipo == "none" {
		return "transparent"
	}
	if tipo != "" && tipo != "solid" {
		return ""
	}
	return colorHash(normalizeColor(color))
}

// fondoPropio es el color de fondo sólido de una página de dibujo (sin «#»); "" si no
// lo tiene.
func (s odpStyle) fondoPropio() string {
	if s.fillType != "solid" {
		return ""
	}
	return normalizeColor(s.fill)
}

// odpTransition es la transición del editor de una página de dibujo. El morph viene
// marcado con el atributo propio del exportador; de ODF, el empuje y el deslizamiento
// (y las barras) son «slide», y cualquier otra es un fundido —hay transición, y es la
// más discreta del editor—, igual que en el .pptx.
func odpTransition(s odpStyle) string {
	if s.trSytue == "morph" {
		return "morph"
	}
	switch s.trType {
	case "":
		switch st := s.trStyle; {
		case st == "" || st == "none":
			return ""
		case strings.HasPrefix(st, "move") || strings.HasPrefix(st, "uncover"):
			return "slide"
		}
		return "fade"
	case "pushWipe", "slideWipe", "barWipe":
		return "slide"
	}
	return "fade"
}
