// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"image/color"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// El CONTROL DE CAMBIOS de la web (`trackchanges.ts` y §«Control de cambios» de
// `canvas/exec.ts`), con el mismo HTML: lo tecleado va en `<ins data-track …>`, lo borrado
// en `<del data-track …>` —no se quita—, cada marca con su autor, su sello de tiempo y el
// color de su autor en línea (`style="--sytue-track:var(--sytue-avatar-N)"`). El modo del
// documento viaja en la sentinela del frente del HTML.
//
// Difiere de la web, a propósito, en tres sitios donde la web tiene un «MVP» declarado:
//   - borrar conserva el formato de lo borrado (la web lo aplana en un `<del>` de texto);
//   - una selección que cruza párrafos se marca párrafo a párrafo, sin fundirlos (la web
//     los funde y ese cambio de estructura no se puede rechazar);
//   - lo propio sin aceptar se RETIRA también dentro de una selección más ancha (la web
//     solo si la selección cae entera dentro de la inserción).
//
// Las tres dejan marcas que la web lee, lista y resuelve igual.

// Sentinela es `TRACK_SENTINEL`: el documento está «en control de cambios». Va al frente
// del HTML guardado y no entra en el árbol, para que nadie la borre al teclear.
const Sentinela = "<!--sytue-track-->"

// Autor es el de una marca: el nombre visible y el id estable (el `sub` de la sesión).
type Autor struct{ Nombre, ID string }

// AutorReserva es el de la web sin sesión (`currentAuthor`).
var AutorReserva = Autor{Nombre: "Autor", ID: "anon"}

// Rastreo dice si el documento está en control de cambios.
func (d *Documento) Rastreo() bool { return d.rastreo }

// PonerRastreo enciende o apaga el control de cambios: cambia la sentinela, no las marcas.
func (d *Documento) PonerRastreo(on bool) { d.rastreo = on }

// esPista dice si n es una marca del control de cambios.
func esPista(n *html.Node) bool {
	return n != nil && n.Type == html.ElementNode && n.Namespace == "" &&
		(n.DataAtom == atom.Ins || n.DataAtom == atom.Del) && tieneAtributo(n, "data-track")
}

// rePista es el `style` que escribe `trackAttrs`, y lo único que se admite en él.
var rePista = regexp.MustCompile(`^\s*--sytue-track\s*:\s*var\(\s*--sytue-(avatar-[1-6])\s*\)\s*;?\s*$`)

// colorPista es el color de una marca: el de su autor, o `--sytue-avatar-1` si no lo dice
// (`var(--sytue-track, var(--sytue-avatar-1))` de `editor.css`). El papel es blanco en los
// dos temas, así que es siempre el del tema claro.
func colorPista(n *html.Node) color.NRGBA {
	token := "avatar-1"
	if m := rePista.FindStringSubmatch(atributo(n, "style")); m != nil {
		token = m[1]
	}
	return tokens.Color(token, tokens.Light)
}

// estiloPista es cómo pinta `editor.css` una marca: en el color de su autor; lo insertado
// subrayado y con un fondo del 10 %, y lo borrado tachado y al 75 % (`opacity: 0.75`, que
// sobre el papel es la misma tinta con alfa).
func estiloPista(e Estilo, n *html.Node) Estilo {
	c := colorPista(n)
	e.Color = c
	if n.DataAtom == atom.Ins {
		e.Subrayado, e.Fondo = true, conAlfa(c, 0.1)
		return e
	}
	// El fondo que haya es el de una caja de FUERA (la inserción de otro): la opacidad
	// del borrado no lo toca.
	e.Tachado = true
	e.Color.A = uint8(float32(e.Color.A)*0.75 + 0.5)
	return e
}

// nuevaPista es la marca de `trackAttrs`, sin el texto.
func nuevaPista(etiqueta string, a Autor, ts int64) *html.Node {
	return elemento(etiqueta,
		html.Attribute{Key: "data-track"},
		html.Attribute{Key: "data-author", Val: a.Nombre},
		html.Attribute{Key: "data-author-id", Val: a.ID},
		html.Attribute{Key: "data-ts", Val: strconv.FormatInt(ts, 10)},
		html.Attribute{Key: "style", Val: "--sytue-track:var(--sytue-" + tokens.Persona(a.ID) + ")"})
}

// pistaDe es la marca más de dentro de una cadena (la que envuelve directamente el texto),
// o nil.
func pistaDe(c []*html.Node) *html.Node {
	for i := len(c) - 1; i >= 0; i-- {
		if esPista(c[i]) {
			return c[i]
		}
	}
	return nil
}

func mismaPista(a, b *html.Node) bool {
	return esPista(a) && esPista(b) && a.DataAtom == b.DataAtom && atributo(a, "data-author-id") == atributo(b, "data-author-id")
}

// propia dice si la runa i está en una inserción de ese autor aún sin aceptar.
func (e *Edicion) propia(i int, a Autor) bool {
	return slices.ContainsFunc(e.cadenas[i], func(n *html.Node) bool {
		return esPista(n) && n.DataAtom == atom.Ins && atributo(n, "data-author-id") == a.ID
	})
}

func borrada(c []*html.Node) bool {
	return slices.ContainsFunc(c, func(n *html.Node) bool { return esPista(n) && n.DataAtom == atom.Del })
}

// InsertarRastreado es `trackInsert` con el cursor en pos: lo escrito va en una inserción
// de `a`. Si lo de al lado ya es una inserción suya, crece esa; dentro de un borrado o de
// la inserción de otro, la nueva va aparte (la web la metería dentro de la del otro).
func (e *Edicion) InsertarRastreado(pos int, texto string, a Autor, ts int64) {
	pos = max(0, min(pos, len(e.runas)))
	nuevo := limpiar(texto)
	if len(nuevo) == 0 {
		return
	}
	var cadena []*html.Node
	if pos > 0 && e.propia(pos-1, a) && (!e.hayPendiente || e.pendienteEn != pos) {
		cadena = e.sinObjeto(pos - 1) // crece la suya, con su formato
	} else {
		cadena = slices.DeleteFunc(slices.Clone(e.hereda(pos)), esPista)
		cadena = append(slices.Clip(cadena), nuevaPista("ins", a, ts))
	}
	cadenas := slices.Clone(e.cadenas[:pos])
	for range nuevo {
		cadenas = append(cadenas, cadena)
	}
	e.cadenas = append(cadenas, e.cadenas[pos:]...)
	e.runas = slices.Concat(e.runas[:pos], nuevo, e.runas[pos:])
	e.hayPendiente = false
	e.fundirPistas()
	e.reescribir()
}

// BorrarRastreado es `deleteLiveRange` en [desde, hasta): lo borrado se MARCA con un
// `<del>` de `a` en vez de quitarse, y devuelve dónde queda el cursor —delante de lo
// marcado con Retroceso (`atras`), detrás si no—, como el que «salta» lo borrado, y si ha cambiado algo.
//
// Lo que NO se marca, como en la web: lo que ya estaba borrado (el cursor lo salta), lo
// que el propio autor insertó sin aceptar (se retira, sin más) y los saltos de línea e
// imágenes, que en la web no tienen texto que envolver y se quitan.
func (e *Edicion) BorrarRastreado(desde, hasta int, a Autor, ts int64, atras bool) (int, bool) {
	desde, hasta = max(0, min(desde, len(e.runas))), max(0, min(hasta, len(e.runas)))
	if desde >= hasta {
		return desde, false
	}
	// Todo dentro de UN borrado: solo se mueve el cursor, al borde de ese borrado.
	if del := pistaDe(e.cadenas[desde]); del != nil && del.DataAtom == atom.Del && e.todoEn(desde, hasta, del) {
		i, j := e.racha(desde, del)
		if atras {
			return i, false
		}
		return j, false
	}
	marca := nuevaPista("del", a, ts)
	var runas []rune
	var cadenas [][]*html.Node
	primera, ultima := -1, -1
	for i, r := range e.runas {
		c := e.cadenas[i]
		if i >= desde && i < hasta {
			switch {
			case e.propia(i, a) || r == '\n' || r == ObjetoRuna:
				continue // se quita
			case !borrada(c):
				c = append(slices.Clip(c), marca)
				if primera < 0 {
					primera = len(runas)
				}
				ultima = len(runas)
			}
		}
		runas, cadenas = append(runas, r), append(cadenas, c)
	}
	e.runas, e.cadenas = runas, cadenas
	e.hayPendiente = false
	e.fundirPistas()
	e.reescribir()
	switch {
	case primera < 0:
		return desde, true
	case atras:
		i, _ := e.racha(primera, pistaDe(e.cadenas[primera]))
		return i, true
	}
	_, j := e.racha(ultima, pistaDe(e.cadenas[ultima]))
	return j, true
}

// todoEn dice si todas las runas de [desde, hasta) llevan esa marca.
func (e *Edicion) todoEn(desde, hasta int, n *html.Node) bool {
	for i := desde; i < hasta; i++ {
		if !slices.Contains(e.cadenas[i], n) {
			return false
		}
	}
	return true
}

// racha es el tramo [i, j) de runas seguidas que lleva la marca n y contiene la runa p.
func (e *Edicion) racha(p int, n *html.Node) (int, int) {
	i, j := p, p
	for i > 0 && slices.Contains(e.cadenas[i-1], n) {
		i--
	}
	for j < len(e.cadenas) && slices.Contains(e.cadenas[j], n) {
		j++
	}
	return i, j
}

// fundirPistas es `coalesceMark`: dos marcas seguidas, del mismo tipo y del mismo autor y
// en el mismo sitio del árbol, pasan a ser UNA —la primera, con su `data-ts`, que es el
// principio de la racha—. Se hace cambiando en las cadenas el nodo de la segunda por el de
// la primera: `construir` levanta un solo elemento para las runas que lo comparten.
func (e *Edicion) fundirPistas() {
	sust := map[*html.Node]*html.Node{}
	for i := range e.cadenas {
		c := e.cadenas[i]
		for k, n := range c {
			if s, ok := sust[n]; ok {
				c = slices.Clone(c)
				c[k] = s
			}
		}
		if i > 0 {
			ant := e.cadenas[i-1]
			for k := 0; k < len(ant) && k < len(c); k++ {
				if ant[k] == c[k] {
					continue
				}
				if !mismaPista(ant[k], c[k]) {
					break
				}
				sust[c[k]] = ant[k]
				c = slices.Clone(c)
				c[k] = ant[k]
			}
		}
		e.cadenas[i] = c
	}
}

// Cambio es un cambio rastreado: una fila del panel «Revisión» (`TrackedChange`).
type Cambio struct {
	Insercion bool
	Autor     string
	Texto     string
	// TS es el `data-ts` en milisegundos, o 0 si falta o no es una fecha (`trackTime`).
	TS   int64
	nodo *html.Node
}

// Cambios lista los cambios EN ORDEN DE DOCUMENTO, con el criterio de `scanTrackMarks`:
// solo las marcas de texto llano, sin elementos dentro —Intro dentro de una inserción, o
// una negrita sobre lo rastreado, las dejan fuera, también en la web—. El ordinal de cada
// uno es su sitio en la lista.
func (d *Documento) Cambios() []Cambio {
	var out []Cambio
	var recorrer func(n *html.Node, prof int)
	recorrer = func(n *html.Node, prof int) {
		for h := n.FirstChild; h != nil && prof <= profundidadMax; h = h.NextSibling {
			if esPista(h) && soloTexto(h) {
				out = append(out, Cambio{
					Insercion: h.DataAtom == atom.Ins, Autor: atributo(h, "data-author"),
					Texto: textoDe(h), TS: selloDe(atributo(h, "data-ts")), nodo: h,
				})
				continue
			}
			recorrer(h, prof+1)
		}
	}
	recorrer(d.raiz, 0)
	return out
}

func soloTexto(n *html.Node) bool {
	for h := n.FirstChild; h != nil; h = h.NextSibling {
		if h.Type != html.TextNode {
			return false
		}
	}
	return true
}

// maxSello es el último instante que admite un `Date` de JavaScript (8,64·10¹⁵ ms).
const maxSello = 8.64e15

// selloDe es `trackTime`: el sello en ms si describe una fecha real, y 0 si no.
func selloDe(crudo string) int64 {
	n, err := strconv.ParseFloat(strings.TrimSpace(crudo), 64)
	if err != nil || n <= 0 || n > maxSello {
		return 0
	}
	return int64(n)
}

// Resolver es `resolveTrackedChanges`: acepta (o rechaza) TODOS los cambios de una vez.
// Aceptar deja el texto insertado y quita el borrado; rechazar, lo contrario. Dice si
// había alguno.
func (d *Documento) Resolver(aceptar bool) bool {
	cs := d.Cambios()
	for _, c := range cs {
		resolver(c.nodo, aceptar)
	}
	fundirTextos(d.raiz)
	return len(cs) > 0
}

// ResolverCambio es `resolveTrackedChange`: solo el de ese ordinal; fuera de rango, nada.
func (d *Documento) ResolverCambio(i int, aceptar bool) bool {
	cs := d.Cambios()
	if i < 0 || i >= len(cs) {
		return false
	}
	padre := cs[i].nodo.Parent
	resolver(cs[i].nodo, aceptar)
	fundirTextos(padre)
	return true
}

// resolver deja lo que sobrevive de una marca (`resolvedContent`): su texto, al aceptar
// una inserción o rechazar un borrado; nada, en los otros dos casos.
func resolver(n *html.Node, aceptar bool) {
	padre := n.Parent
	if aceptar == (n.DataAtom == atom.Ins) {
		for h := n.FirstChild; h != nil; h = n.FirstChild {
			n.RemoveChild(h)
			padre.InsertBefore(h, n)
		}
	}
	padre.RemoveChild(n)
}

// Donde dice en qué runas del bloque está el cambio: de la primera a la última que lleva
// su marca.
func (b Bloque) Donde(c Cambio) (desde, hasta int, ok bool) {
	pos := 0
	for _, t := range b.Tramos {
		n := len([]rune(t.Texto))
		if t.Salto || t.Objeto != "" {
			n++
		}
		if slices.Contains(t.cadena, c.nodo) {
			if !ok {
				desde, ok = pos, true
			}
			hasta = pos + n
		}
		pos += n
	}
	return desde, hasta, ok
}
