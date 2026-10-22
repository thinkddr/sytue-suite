// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Las herramientas de la imagen (`images.ts` y `canvas/imageDom.ts` de la web): el modelo
// de una imagen vive en sus `data-*` y su `style` se DERIVA de ellos. Se escribe lo mismo
// que la web, en el mismo orden, para que el navegador lo lea igual.

// Hueco es el `GAP` de `images.ts`: lo que separa del texto una imagen fuera de la línea.
const Hueco = 14

// MinImagen es el `MIN_SIZE` de los tiradores: el lado más pequeño de una imagen.
const MinImagen = 24

// ClampRecorte es `clampCrop`: cada lado entre 0 y 0,9 y, por eje, la suma acotada a 0,9
// —queda al menos el 10 % a la vista—, a la milésima.
func ClampRecorte(c [4]float32) [4]float32 {
	for i := range c {
		c[i] = min(max(c[i], 0), 0.9)
	}
	eje := func(a, z float32) (float32, float32) {
		if s := a + z; s > 0.9 {
			return a * 0.9 / s, z * 0.9 / s
		}
		return a, z
	}
	c[0], c[2] = eje(c[0], c[2])
	c[3], c[1] = eje(c[3], c[1])
	for i := range c {
		c[i] = float32(redondeo(float64(c[i]), 1000))
	}
	return c
}

// leerRecorte es `parseCrop`: «t,r,b,l»; lo que falte o no se lea, 0.
func leerRecorte(crudo string) [4]float32 {
	var c [4]float32
	for i, p := range strings.SplitN(crudo, ",", 4) {
		if v, err := strconv.ParseFloat(strings.TrimSpace(p), 32); err == nil && !math.IsNaN(v) {
			c[i] = float32(v)
		}
	}
	return ClampRecorte(c)
}

// ConRecorte dice si algún lado está recortado.
func (im *Imagen) ConRecorte() bool { return im.Recorte != [4]float32{} }

// Bandas son los px recortados de cada lado de la imagen entera (w × h): solo con su
// tamaño dicho, como en la web.
func (im *Imagen) Bandas() [4]float32 {
	if !im.ConRecorte() || im.Ancho <= 0 || im.Alto <= 0 {
		return [4]float32{}
	}
	return [4]float32{im.Recorte[0] * im.Alto, im.Recorte[1] * im.Ancho, im.Recorte[2] * im.Alto, im.Recorte[3] * im.Ancho}
}

// redondeo es el `Math.round(n * k) / k` de JavaScript, que redondea los medios hacia
// arriba también en los negativos (el de Go, hacia fuera del cero).
func redondeo(n, k float64) float64 { return math.Floor(n*k+0.5) / k }

// numero escribe un número como `String()` de JavaScript tras `r2`.
func numero(n float32) string {
	return strconv.FormatFloat(redondeo(float64(n), 100), 'f', -1, 64)
}

func pxImagen(n float32) string { return numero(n) + "px" }

// estiloImagen es `styleCssText(computeImageStyle(m))`: tamaño, ajuste, recorte y anclaje,
// en el orden en que la web los pone.
func estiloImagen(im *Imagen) string {
	var decl [][2]string
	poner := func(k, v string) { decl = append(decl, [2]string{k, v}) }
	puesta := func(k string) bool { return slices.ContainsFunc(decl, func(d [2]string) bool { return d[0] == k }) }
	if im.Ancho > 0 {
		poner("width", pxImagen(im.Ancho))
	}
	if im.Alto > 0 {
		poner("height", pxImagen(im.Alto))
	}
	if im.Ajuste == "inline" {
		poner("max-width", "100%")
	} else {
		poner("max-width", "none")
	}
	banda := im.Bandas()
	inset := "inset(" + pxImagen(banda[0]) + " " + pxImagen(banda[1]) + " " + pxImagen(banda[2]) + " " + pxImagen(banda[3]) + ")"
	if banda != [4]float32{} {
		poner("clip-path", inset)
	}
	var hueco [4]float32
	switch im.Ajuste {
	case "square", "tight":
		izq := im.Alinear != "right"
		if izq {
			poner("float", "left")
			hueco[1] = Hueco
		} else {
			poner("float", "right")
			hueco[3] = Hueco
		}
		hueco[2] = Hueco
		if im.Ajuste == "tight" && im.Src != "" {
			poner("shape-outside", inset)
			poner("shape-margin", "14px")
		}
	case "topBottom":
		poner("display", "block")
		poner("clear", "both")
		hueco[0], hueco[2] = Hueco, Hueco
		poner("margin-left", map[bool]string{true: "0", false: "auto"}[im.Alinear == "left"])
		poner("margin-right", map[bool]string{true: "0", false: "auto"}[im.Alinear == "right"])
	case "behind", "front":
		poner("position", "absolute")
		poner("left", pxImagen(im.X))
		poner("top", pxImagen(im.Y))
		poner("z-index", map[bool]string{true: "-1", false: "3"}[im.Ajuste == "behind"])
	default:
		poner("display", "inline-block")
		poner("vertical-align", "bottom")
	}
	for i, k := range []string{"margin-top", "margin-right", "margin-bottom", "margin-left"} {
		if puesta(k) {
			continue
		}
		if v := redondeo(float64(hueco[i]-banda[i]), 100); v != 0 {
			poner(k, pxImagen(float32(v)))
		}
	}
	partes := make([]string, len(decl))
	for i, d := range decl {
		partes[i] = d[0] + ":" + d[1]
	}
	return strings.Join(partes, ";")
}

// datosImagen es `imageDataAttrs`: lo que se escribe en `data-*`, sin lo que está en su
// valor por defecto.
func datosImagen(im *Imagen) []html.Attribute {
	var out []html.Attribute
	poner := func(k, v string) { out = append(out, html.Attribute{Key: k, Val: v}) }
	if im.Ajuste != "inline" {
		poner("data-wrap", im.Ajuste)
	}
	if im.Alinear != "left" {
		poner("data-align", im.Alinear)
	}
	if im.Ancho > 0 {
		poner("data-w", numero(im.Ancho))
	}
	if im.Alto > 0 {
		poner("data-h", numero(im.Alto))
	}
	if im.ConRecorte() {
		// Las fracciones van a la milésima (`r3`), no a la centésima de los px.
		milesima := func(v float32) string { return strconv.FormatFloat(redondeo(float64(v), 1000), 'f', -1, 64) }
		c := im.Recorte
		poner("data-crop", milesima(c[0])+","+milesima(c[1])+","+milesima(c[2])+","+milesima(c[3]))
	}
	if im.Absoluta() {
		poner("data-x", numero(im.X))
		poner("data-y", numero(im.Y))
	}
	return out
}

// clavesImagen son las que gobierna el modelo (`DATA_KEYS`): se quitan antes de reescribir.
var clavesImagen = []string{"data-wrap", "data-align", "data-w", "data-h", "data-crop", "data-x", "data-y"}

// PonerImagen es `writeModel`: vuelca el modelo a su `<img>` —los `data-*`, la marca de
// gestionada, el alt, `contenteditable` y el `style` derivado— con los atributos en el
// orden en que los deja el navegador. Quien llama ha puesto antes el tamaño (y, anclada,
// la posición) que la imagen tenía en pantalla, como `applyImage`. Dice si la imagen
// sigue en el documento.
func (d *Documento) PonerImagen(im *Imagen) bool {
	n := im.nodo
	if n == nil || !d.contiene(n) {
		return false
	}
	im.Recorte = ClampRecorte(im.Recorte)
	n.Attr = slices.DeleteFunc(n.Attr, func(a html.Attribute) bool { return a.Namespace == "" && slices.Contains(clavesImagen, a.Key) })
	n.Attr = append(n.Attr, datosImagen(im)...)
	ponerAtributo(n, "data-sytue-image", "")
	if im.Alt != "" {
		ponerAtributo(n, "alt", im.Alt)
	} else {
		n.Attr = slices.DeleteFunc(n.Attr, func(a html.Attribute) bool { return a.Namespace == "" && a.Key == "alt" })
	}
	ponerAtributo(n, "contenteditable", "false")
	ponerAtributo(n, "style", estiloImagen(im))
	return true
}

// QuitarImagen es `removeImage`: la imagen se va, y si el folio se queda sin nada, queda el
// párrafo vacío de la web.
func (d *Documento) QuitarImagen(im *Imagen) bool {
	n := im.nodo
	if n == nil || !d.contiene(n) {
		return false
	}
	padre := n.Parent
	padre.RemoveChild(n)
	fundirTextos(padre) // «a» y «b» a los lados de la imagen se leerían como UN texto
	vacio := true
	for h := d.raiz.FirstChild; h != nil; h = h.NextSibling {
		if h.Type == html.ElementNode {
			vacio = false
		}
	}
	if vacio {
		for h := d.raiz.FirstChild; h != nil; h = d.raiz.FirstChild {
			d.raiz.RemoveChild(h)
		}
		d.Asegurar()
	}
	return true
}

// contiene dice si n cuelga todavía del documento.
func (d *Documento) contiene(n *html.Node) bool {
	for p := n; p != nil; p = p.Parent {
		if p == d.raiz {
			return true
		}
	}
	return false
}

// ImagenEn es la imagen que ocupa la runa p del bloque, o nil.
func (b Bloque) ImagenEn(p int) *Imagen {
	pos := 0
	for _, t := range b.Tramos {
		if t.Salto {
			pos++
		}
		if t.Objeto != "" {
			if pos == p && t.Imagen != nil && t.nodo != nil && t.nodo.DataAtom == atom.Img {
				return t.Imagen
			}
			pos++
		}
		pos += len([]rune(t.Texto))
	}
	return nil
}
